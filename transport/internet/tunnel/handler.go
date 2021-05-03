// +build !confonly

package tunnel

import (
	"net"

	xstack "github.com/xtls/xray-core/transport/internet/tunnel/stack"
)

func (l *listener) HandleStream(conn net.Conn) error {
	target := conn.RemoteAddr().(*net.TCPAddr)
	newError("handle tcp connect to tcp:", target.String()).AtDebug().WriteToLog()
	l.acceptConn(conn)
	return nil
}

func (l *listener) HandlePacket(pconn xstack.PacketConn, target *net.UDPAddr) error {
	newError("handle udp:", target.String()).AtDebug().WriteToLog()
	p := makeUDP(pconn)
	l.acceptConn(p)
	return nil

}
