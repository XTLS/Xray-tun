// +build !confonly

package tunnel

import (
	"context"
	"github.com/xtls/xray-core/common"
	"github.com/xtls/xray-core/common/blockdns"
	"github.com/xtls/xray-core/common/net"
	"github.com/xtls/xray-core/common/route"
	"github.com/xtls/xray-core/common/signal/done"
	"github.com/xtls/xray-core/transport/internet"
	"github.com/xtls/xray-core/transport/internet/tunnel/stack"
	tundev "github.com/xtls/xray-core/transport/internet/tunnel/tun"
	"io"
	"os"
	"os/signal"
	"runtime"
	"syscall"
)

var MTU = 1500

// Listen return listener of tun device
func Listen(ctx context.Context, address net.Address, port net.Port, streamSettings *internet.MemoryStreamConfig, handler internet.ConnHandler) (internet.Listener, error) {
	config := streamSettings.ProtocolSettings.(*Config)
	tunAddr := config.GetAddress()
	tunDNS := config.GetDns()
	tunGW := config.GetGateway()
	tunMask := config.GetMask()
	tunName := config.GetName()
	helper := route.GetHelper()

	tun, err := tundev.OpenTUNDevice(tunName, tunAddr, tunGW, tunMask, tunDNS)
	if err != nil {
		return nil, newError("failed start tun device").Base(err).AtError()
	}
	if runtime.GOOS == "windows" && config.GetFixDnsLeak() {
		if err := blockdns.FixDnsLeakage(tunName); err != nil {
			return nil, newError("failed to fix dns leak").Base(err).AtError()
		}
	}
	newError("tun started").AtWarning().WriteToLog()

	tunIP := net.ParseIP(tunAddr)
	l := &listener{
		ctx:         ctx,
		tun:         tun,
		addr:        &net.IPNet{IP: tunIP},
		connChan:    make(chan net.Conn, 10),
		connHandler: handler,
		config:      config,
		done:        done.New(),
	}

	l.stack, err = stack.DefaultNew(tun, l)
	if err != nil {
		return nil, newError("failed to create stack").Base(err)
	}
	go l.run()

	err = setRouteTable(helper, tun, net.ParseIP(tunGW))
	if err != nil {
		return nil, err
	}
	return l, nil
}

type listener struct {
	ctx         context.Context
	tun         io.ReadWriteCloser
	connChan    chan net.Conn
	connHandler internet.ConnHandler
	config      *Config
	addr        net.Addr
	stack       *stack.Stack
	done        *done.Instance
}

func (l *listener) Close() error {
	l.done.Close()
	err := l.tun.Close()
	if err != nil {
		return newError("Cannot close tun device").Base(err).AtWarning()
	}
	return nil
}

func (l *listener) Addr() net.Addr {
	return l.addr
}

func (l *listener) run() error {
	for {
		select {
		case conn := <-l.connChan:
			l.connHandler(conn)
		case <-l.done.Wait():
			return nil
		}

	}
}

func (l *listener) acceptConn(c net.Conn) {
	if l.done.Done() {
		return
	}
	l.connChan <- c
}

func setRouteTable(h route.Helper, tun tundev.Device, tunGW net.IP) error {
	originGW, err := h.GetDefaultGateway()
	if err != nil {
		return err
	}
	defInf, _, err := h.GetDefaultInterface()
	if err != nil {
		return err
	}
	err = h.SetDefaultInterface(tunGW, tun.GetIdentifier())
	if err != nil {
		return err
	}
	go func() {
		osSignals := make(chan os.Signal, 1)
		signal.Notify(osSignals, os.Interrupt, os.Kill, syscall.SIGTERM)
		<-osSignals
		switch runtime.GOOS {
		case "windows":
			h.RemoveDefaultInterface(tun.GetIdentifier())
		case "linux":
			h.RemoveDefaultInterface(defInf)
		case "darwin":
			h.RemoveDefaultInterface(originGW)
		}
	}()
	return nil

}

func init() {
	common.Must(internet.RegisterTransportListener(protocolName, Listen))
}
