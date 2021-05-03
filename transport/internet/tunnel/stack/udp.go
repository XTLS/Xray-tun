package stack

import (
	"io"
	"net"

	"github.com/xtls/xray-core/common/signal/done"

	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/buffer"
	"gvisor.dev/gvisor/pkg/tcpip/header"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
)

type packet struct {
	addr *net.UDPAddr
	buf  []byte
}

type UDPConn struct {
	id     string
	closed *done.Instance
	queue  chan packet

	stack *Stack
	tid   stack.TransportEndpointID
	nic   tcpip.NICID
	np    tcpip.NetworkProtocolNumber
}

func NewUDPConn(key string, tid stack.TransportEndpointID, np tcpip.NetworkProtocolNumber, nic tcpip.NICID, s *Stack) *UDPConn {
	conn := &UDPConn{
		id:     key,
		queue:  make(chan packet, 64),
		stack:  s,
		tid:    tid,
		closed: done.New(),
		np:     np,
		nic:    nic,
	}
	return conn
}

func (conn *UDPConn) Close() error {
	if conn.closed.Done() {
		return nil
	}

	conn.closed.Close()
	conn.stack.Del(conn.id)
	return nil
}

func (conn *UDPConn) LocalAddr() net.Addr {
	return &net.UDPAddr{IP: net.IP(conn.tid.RemoteAddress), Port: int(conn.tid.RemotePort)}

}

func (conn *UDPConn) RemoteAddr() net.Addr {
	return &net.UDPAddr{IP: net.IP(conn.tid.LocalAddress), Port: int(conn.tid.LocalPort)}
}

func (conn *UDPConn) ReadTo(b []byte) (n int, addr net.Addr, err error) {
	select {
	case <-conn.closed.Wait():
		err = io.EOF
	case pkt := <-conn.queue:
		n = copy(b, pkt.buf)
		addr = pkt.addr
	}
	return
}

func (conn *UDPConn) WriteFrom(b []byte, addr net.Addr) (int, error) {
	v := buffer.View(b)
	if len(v) > header.UDPMaximumPacketSize {
		return 0, newError("message too long")
	}

	src, ok := addr.(*net.UDPAddr)
	if !ok {
		return 0, newError("addr type error")
	}

	var sourceAddr tcpip.Address
	if ip := src.IP.To4(); ip != nil {
		sourceAddr = tcpip.Address(ip)
	} else {
		sourceAddr = tcpip.Address(src.IP.To16())
	}

	route, err := conn.stack.FindRoute(conn.nic, sourceAddr, conn.tid.RemoteAddress, conn.np, false)
	if err != nil {
		return 0, newError(err.String())
	}

	if tcperr := sendUDP(
		route,
		v.ToVectorisedView(),
		uint16(src.Port),
		conn.tid.RemotePort,
		0,    /* ttl */
		true, /* useDefaultTTL */
		0,    /* tos */
		nil,  /* owner */
		true,
	); tcperr != nil {
		return 0, newError((*tcperr).String())
	}
	return len(b), nil
}

func (conn *UDPConn) HandlePacket(b []byte, addr *net.UDPAddr) {
	if conn.closed.Done() {
		return
	}
	select {
	case <-conn.closed.Wait():
		return
	case conn.queue <- packet{addr, b}:
	}
}

type PacketConn interface {
	ReadTo(p []byte) (n int, addr net.Addr, err error)
	WriteFrom(p []byte, addr net.Addr) (n int, err error)
	Close() error
	LocalAddr() net.Addr
	RemoteAddr() net.Addr
}
