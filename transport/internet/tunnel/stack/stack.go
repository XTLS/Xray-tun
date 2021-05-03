package stack

import (
	"fmt"
	"github.com/xtls/xray-core/transport/internet/tunnel/tun"
	"net"
	"strconv"
	"sync"
	"time"

	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/header"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv4"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
	"gvisor.dev/gvisor/pkg/tcpip/transport/icmp"
	"gvisor.dev/gvisor/pkg/tcpip/transport/tcp"
	"gvisor.dev/gvisor/pkg/tcpip/transport/udp"
	"gvisor.dev/gvisor/pkg/waiter"
)

//go:generate go run github.com/xtls/xray-core/common/errors/errorgen

type Handler interface {
	HandleStream(net.Conn) error
	HandlePacket(PacketConn, *net.UDPAddr) error
}

type Stack struct {
	stack   *stack.Stack
	handler Handler
	device  tun.Device
	connMap *sync.Map
}

func DefaultNew(dev tun.Device, handler Handler) (*Stack, error) {
	return New(dev, handler,
		SetDefaultTTL(64),
		SetForwarding(),
		SetICMP(),
		SetTCPBuffer(4<<10, 212<<10, 4<<20),
		SetCongestionControl("reno"),
		SetDelay(false),
		SetModerateReceiveBuffer(true),
		SetSACK(true),
	)
}

func New(device tun.Device, handler Handler, opts ...option) (s *Stack, err error) {
	s = &Stack{
		connMap: new(sync.Map),
	}
	s.device = device
	s.handler = handler
	s.stack = stack.New(stack.Options{
		NetworkProtocols: []stack.NetworkProtocolFactory{
			ipv4.NewProtocol,
		},
		TransportProtocols: []stack.TransportProtocolFactory{
			tcp.NewProtocol,
			udp.NewProtocol,
			icmp.NewProtocol4,
		},
	})
	defer func(s *stack.Stack) {
		if err != nil {
			s.Close()
		}
	}(s.stack)

	for _, opt := range opts {
		if err := opt(s); err != nil {
			return nil, err
		}
	}

	const NICID = tcpip.NICID(1)

	mustSubnet := func(s string) tcpip.Subnet {
		_, ipNet, err := net.ParseCIDR(s)
		if err != nil {
			panic(fmt.Errorf("unable to ParseCIDR(%s): %w", s, err))
		}

		subnet, err := tcpip.NewSubnet(tcpip.Address(ipNet.IP), tcpip.AddressMask(ipNet.Mask))
		if err != nil {
			panic(fmt.Errorf("unable to NewSubnet(%s): %w", ipNet, err))
		}
		return subnet
	}

	// Add default route table for IPv4 and IPv6
	// This will handle all incoming ICMP packets.
	s.stack.SetRouteTable([]tcpip.Route{
		{
			Destination: mustSubnet("0.0.0.0/0"),
			NIC:         NICID,
		},
		{
			Destination: mustSubnet("::/0"),
			NIC:         NICID,
		},
	})

	// Important: We must initiate transport protocol handlers
	// before creating NIC, otherwise NIC would dispatch packets
	// to stack and cause race condition.
	s.stack.SetTransportProtocolHandler(tcp.ProtocolNumber, tcp.NewForwarder(s.stack, 16<<10, 1<<15, s.HandleStream).HandlePacket)
	s.stack.SetTransportProtocolHandler(udp.ProtocolNumber, s.HandlePacket)

	// WithCreatingNIC creates NIC for stack.
	if tcperr := s.stack.CreateNIC(NICID, NewEndpoint(device, 1500)); tcperr != nil {
		err = fmt.Errorf("fail to create NIC in stack: %s", tcperr)
		return
	}

	// WithPromiscuousMode sets promiscuous mode in the given NIC.
	// In past we did s.AddAddressRange to assign 0.0.0.0/0 onto
	// the interface. We need that to be able to terminate all the
	// incoming connections - to any ip. AddressRange API has been
	// removed and the suggested workaround is to use Promiscuous
	// mode. https://github.com/google/gvisor/issues/3876
	//
	// Ref: https://github.com/majek/slirpnetstack/blob/master/stack.go
	if tcperr := s.stack.SetPromiscuousMode(NICID, true); tcperr != nil {
		err = fmt.Errorf("set promiscuous mode: %s", tcperr)
		return
	}

	// WithSpoofing sets address spoofing in the given NIC, allowing
	// endpoints to bind to any address in the NIC.
	// Enable spoofing if a stack may send packets from unowned addresses.
	// This change required changes to some netgophers since previously,
	// promiscuous mode was enough to let the netstack respond to all
	// incoming packets regardless of the packet's destination address. Now
	// that a stack.Route is not held for each incoming packet, finding a route
	// may fail with local addresses we don't own but accepted packets for
	// while in promiscuous mode. Since we also want to be able to send from
	// any address (in response the received promiscuous mode packets), we need
	// to enable spoofing.
	//
	// Ref: https://github.com/google/gvisor/commit/8c0701462a84ff77e602f1626aec49479c308127
	if tcperr := s.stack.SetSpoofing(NICID, true); tcperr != nil {
		err = fmt.Errorf("set spoofing: %s", tcperr)
		return
	}
	return
}

func endpointIDtoString(id stack.TransportEndpointID) string {
	return id.RemoteAddress.String() + ":" + strconv.Itoa(int(id.RemotePort)) + "<->" + id.LocalAddress.String() + ":" + strconv.Itoa(int(id.LocalPort))
}

func (s *Stack) HandleStream(r *tcp.ForwarderRequest) {
	var id = r.ID()
	ids := endpointIDtoString(id)
	var wq = waiter.Queue{}
	ep, tcperr := r.CreateEndpoint(&wq)
	if tcperr != nil {
		newError("failed to create endpoint for " + ids + ": " + tcperr.String()).AtWarning().WriteToLog()
		// prevent potential half-open TCP connection leak.
		r.Complete(true)
		return
	}
	r.Complete(false)

	ep.SocketOptions().SetKeepAlive(true)
	idleOpt := tcpip.KeepaliveIdleOption(60 * time.Second)
	if tcperr := ep.SetSockOpt(&idleOpt); tcperr != nil {
		newError("failed to set keepalive idle for " + ids + ": " + tcperr.String()).AtWarning().WriteToLog()
	}
	intervalOpt := tcpip.KeepaliveIntervalOption(30 * time.Second)
	if tcperr := ep.SetSockOpt(&intervalOpt); tcperr != nil {
		newError("failed to set keepalive interval for" + ids + ": " + tcperr.String()).AtWarning().WriteToLog()
	}
	s.handler.HandleStream(getTCPConn(&wq, ep))
}

func (s *Stack) HandlePacket(id stack.TransportEndpointID, pkt *stack.PacketBuffer) bool {
	// Ref: gVisor pkg/tcpip/transport/udp/endpoint.go HandlePacket
	udpHdr := header.UDP(pkt.TransportHeader().View())
	ids := endpointIDtoString(id)
	if int(udpHdr.Length()) > pkt.Data().Size()+header.UDPMinimumSize {
		newError("malformed packet: " + ids).AtWarning().WriteToLog()
		s.stack.Stats().UDP.MalformedPacketsReceived.Increment()
		return true
	}

	if !verifyChecksum(udpHdr, pkt) {
		newError("checksum error: " + ids).AtWarning().WriteToLog()
		s.stack.Stats().UDP.ChecksumErrors.Increment()
		return true
	}

	s.stack.Stats().UDP.PacketsReceived.Increment()

	key := string(id.RemoteAddress) + string([]byte{byte(id.RemotePort >> 8), byte(id.RemotePort)})
	view := pkt.Data().ExtractVV()
	if conn, ok := s.Get(key); ok {
		conn.HandlePacket(view.ToView(), &net.UDPAddr{IP: net.IP(id.LocalAddress), Port: int(id.LocalPort)})
		return true
	}

	//route.ResolveWith(pkt.SourceLinkAddress())

	conn := NewUDPConn(
		key,
		id,
		pkt.NetworkProtocolNumber,
		pkt.NICID,
		s,
	)
	s.Add(key, conn)
	conn.HandlePacket(view.ToView(), &net.UDPAddr{IP: net.IP(id.LocalAddress), Port: int(id.LocalPort)})

	s.handler.HandlePacket(conn, conn.RemoteAddr().(*net.UDPAddr))
	return true
}

func (s *Stack) GetStack() *stack.Stack {
	return s.stack
}

func (s *Stack) Get(k string) (*UDPConn, bool) {
	if conn, found := s.connMap.Load(k); found {
		return conn.(*UDPConn), true
	}
	return nil, false
}

func (s *Stack) Add(k string, conn *UDPConn) {
	s.connMap.Store(k, conn)
}

func (s *Stack) Del(k string) {
	s.connMap.Delete(k)
}

func (s *Stack) FindRoute(id tcpip.NICID, localAddr tcpip.Address, remoteAddr tcpip.Address, netProto tcpip.NetworkProtocolNumber, multicastLoop bool) (*stack.Route, tcpip.Error) {
	return s.stack.FindRoute(id, localAddr, remoteAddr, netProto, multicastLoop)
}
