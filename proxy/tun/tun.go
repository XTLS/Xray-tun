// +build !confonly

package tun

import (
	"context"
	"sync/atomic"

	"github.com/xtls/xray-core/common/log"
	udpProtocol "github.com/xtls/xray-core/common/protocol/udp"

	"github.com/xtls/xray-core/common"
	"github.com/xtls/xray-core/common/buf"
	"github.com/xtls/xray-core/common/signal"
	"github.com/xtls/xray-core/common/task"
	"github.com/xtls/xray-core/core"

	"github.com/xtls/xray-core/common/net"
	"github.com/xtls/xray-core/common/protocol"
	"github.com/xtls/xray-core/common/session"
	"github.com/xtls/xray-core/features/policy"
	"github.com/xtls/xray-core/features/routing"
	"github.com/xtls/xray-core/transport/internet"
	"github.com/xtls/xray-core/transport/internet/udp"
)

//go:generate go run github.com/xtls/xray-core/common/errors/errorgen

func init() {
	common.Must(common.RegisterConfig((*Config)(nil), func(ctx context.Context, config interface{}) (interface{}, error) {
		d := new(Server)
		err := core.RequireFeatures(ctx, func(pm policy.Manager) error {
			return d.Init(config.(*Config), pm)
		})
		return d, err
	}))
}

type Server struct {
	config        *Config
	policyManager policy.Manager
}

func (d *Server) Init(config *Config, pm policy.Manager) error {
	d.config = config
	d.policyManager = pm

	return nil
}

func (s *Server) Network() []net.Network {
	return []net.Network{net.Network_TCP}
}

func (d *Server) policy() policy.Session {
	config := d.config
	p := d.policyManager.ForLevel(config.UserLevel)
	return p
}

func (d *Server) Process(ctx context.Context, network net.Network, conn internet.Connection, dispatcher routing.Dispatcher) error {
	newError("processing connection from tun:", conn.LocalAddr(), " to ", conn.RemoteAddr()).AtDebug().WriteToLog(session.ExportIDToError(ctx))
	dest, err := net.ParseDestination(conn.RemoteAddr().Network() + ":" + conn.RemoteAddr().String())

	if err != nil {
		return newError("Failed to process connection").Base(err).AtWarning()
	}
	if inbound := session.InboundFromContext(ctx); inbound != nil {
		inbound.User = &protocol.MemoryUser{
			Level: d.config.UserLevel,
		}
	}
	plcy := d.policy()
	ctx, cancel := context.WithCancel(ctx)
	timer := signal.CancelAfterInactivity(ctx, cancel, plcy.Timeouts.ConnectionIdle)
	ctx = policy.ContextWithBufferPolicy(ctx, plcy.Buffer)
	switch dest.Network {
	case net.Network_TCP:
		return d.processTCP(ctx, dest, conn, dispatcher, timer, plcy)
	case net.Network_UDP:
		return d.processUDP(ctx, conn, dispatcher, timer)
	}
	return nil
}

func (d *Server) processTCP(ctx context.Context, dest net.Destination, conn internet.Connection, dispatcher routing.Dispatcher, timer *signal.ActivityTimer, plcy policy.Session) error {
	if inbound := session.InboundFromContext(ctx); inbound != nil && inbound.Source.IsValid() {
		ctx = log.ContextWithAccessMessage(ctx, &log.AccessMessage{
			From:   conn.LocalAddr(),
			To:     dest,
			Status: log.AccessAccepted,
			Reason: "",
		})
	}
	link, err := dispatcher.Dispatch(ctx, dest)
	if err != nil {
		return newError("failed to dispatch request").Base(err)
	}

	requestCount := int32(1)
	requestDone := func() error {
		defer func() {
			if atomic.AddInt32(&requestCount, -1) == 0 {
				timer.SetTimeout(plcy.Timeouts.DownlinkOnly)
			}
		}()
		var reader = buf.NewReader(conn)
		timer.Update()
		if err := buf.Copy(reader, link.Writer, buf.UpdateActivity(timer)); err != nil {
			return newError("failed to transport request").Base(err)
		}

		return nil
	}

	var writer = buf.NewWriter(conn)
	responseDone := func() error {
		defer timer.SetTimeout(plcy.Timeouts.UplinkOnly)
		timer.Update()
		if err := buf.Copy(link.Reader, writer, buf.UpdateActivity(timer)); err != nil {
			return newError("failed to transport response").Base(err)
		}
		return nil
	}

	if err := task.Run(ctx, task.OnSuccess(requestDone, task.Close(link.Writer)), responseDone); err != nil {
		common.Interrupt(link.Reader)
		common.Interrupt(link.Writer)
		conn.Close()
		return newError("connection ends").Base(err)
	}

	return nil
}

type tunUDPConnAdapter interface {
	ReadMultiBufferWithAddr() (buf.MultiBuffer, error)
	WriteMultiBufferWithAddr(mb buf.MultiBuffer) error
}

// isTunUDPConnAdapter: is conn cute enough to write payloads?
func isTunUDPConnAdapter(conn internet.Connection) (a tunUDPConnAdapter, ok bool) {
	a, ok = conn.(tunUDPConnAdapter)
	return
}

func (d *Server) processUDP(ctx context.Context, conn internet.Connection, dispatcher routing.Dispatcher, timer *signal.ActivityTimer) error {
	udpconn, ok := isTunUDPConnAdapter(conn)
	if !ok {
		return newError("unexpected udp conn, please check your transport settings")
	}
	udpDispatcher := udp.NewDispatcher(dispatcher, func(ctx context.Context, packet *udpProtocol.Packet) {
		timer.Update()
		udpconn.WriteMultiBufferWithAddr(buf.MultiBuffer{packet.Payload})
	})
	processFunc := func() error {
		var dest net.Destination
		for {
			mb, err := udpconn.ReadMultiBufferWithAddr()
			if err != nil {
				return err
			}
			timer.Update()
			for _, b := range mb {
				if b.IsEmpty() {
					b.Release()
					continue
				}
				currentPacketCtx := ctx
				if inbound := session.InboundFromContext(ctx); inbound != nil && inbound.Source.IsValid() {
					currentPacketCtx = log.ContextWithAccessMessage(ctx, &log.AccessMessage{
						From:   conn.LocalAddr(),
						To:     b.UDP,
						Status: log.AccessAccepted,
						Reason: "",
					})
				}
				if dest.Network == 0 {
					dest = net.DestinationFromAddr(conn.RemoteAddr())
				}
				udpDispatcher.Dispatch(currentPacketCtx, dest, b)
			}
		}
	}
	if err := task.Run(ctx, processFunc); err != nil {
		conn.Close()
		return newError("connection ends").Base(err)
	}
	return nil
}
