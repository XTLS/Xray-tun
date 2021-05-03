package stack

import (
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/adapters/gonet"
	"gvisor.dev/gvisor/pkg/waiter"
	"net"
)

type TCPConn struct {
	*gonet.TCPConn
}

func (t *TCPConn) LocalAddr() net.Addr  { return t.TCPConn.RemoteAddr() }
func (t *TCPConn) RemoteAddr() net.Addr { return t.TCPConn.LocalAddr() }

func getTCPConn(wq *waiter.Queue, ep tcpip.Endpoint) *TCPConn {
	return &TCPConn{
		gonet.NewTCPConn(wq, ep),
	}
}
