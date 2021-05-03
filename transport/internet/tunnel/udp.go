// +build !confonly

package tunnel

import (
	"io"
	"net"
	"time"

	"github.com/xtls/xray-core/common/signal/done"
	xstack "github.com/xtls/xray-core/transport/internet/tunnel/stack"

	vnet "github.com/xtls/xray-core/common/net"

	"github.com/xtls/xray-core/common/buf"
)

type udpConnAdapter struct {
	bufferChan        chan *buf.Buffer
	target            *net.UDPAddr
	sourceDestination vnet.Destination
	source            *net.UDPAddr
	conn              xstack.PacketConn
	done              *done.Instance
}

var errNotImpl = newError("Unimplemented method, use other instead.")

func (c *udpConnAdapter) LocalAddr() net.Addr                { return c.source }
func (c *udpConnAdapter) RemoteAddr() net.Addr               { return c.target }
func (c *udpConnAdapter) SetDeadline(t time.Time) error      { return errNotImpl }
func (c *udpConnAdapter) SetReadDeadline(t time.Time) error  { return errNotImpl }
func (c *udpConnAdapter) SetWriteDeadline(t time.Time) error { return errNotImpl }
func (c *udpConnAdapter) Read(data []byte) (int, error) {
	return 0, errNotImpl
}
func (c *udpConnAdapter) Write(data []byte) (int, error) {
	return 0, errNotImpl
}
func (c *udpConnAdapter) Close() error {
	c.done.Done()
	return c.conn.Close()
}

func (c *udpConnAdapter) MustClose() {
	if err := c.Close(); err != nil {
		panic(newError("Cannot close connection").Base(err))
	}
}

func (c *udpConnAdapter) ReadMultiBufferWithAddr() (buf.MultiBuffer, error) {
	b := buf.New()

	buffer := b.Extend(buf.Size)
	n, addr, err := c.conn.ReadTo(buffer)
	if err != nil {
		return nil, err
	}

	b.Resize(0, int32(n))
	dest := vnet.DestinationFromAddr(addr)
	b.UDP = &dest
	return buf.MultiBuffer{b}, nil
}

func (c *udpConnAdapter) WriteMultiBufferWithAddr(mb buf.MultiBuffer) error {
	if c.done.Done() {
		return io.ErrClosedPipe
	}
	for _, buffer := range mb {
		if buffer.UDP == nil {
			if _, err := c.conn.WriteFrom(buffer.Bytes(), c.target); err != nil {
				return err
			}
			continue
		}
		if _, err := c.conn.WriteFrom(buffer.Bytes(), vnet.DestinationToAddr(*buffer.UDP)); err != nil {
			return err
		}
	}
	return nil
}

func makeUDP(pconn xstack.PacketConn) *udpConnAdapter {
	c := &udpConnAdapter{
		source:     pconn.LocalAddr().(*net.UDPAddr),
		target:     pconn.RemoteAddr().(*net.UDPAddr),
		done:       done.New(),
		conn:       pconn,
		bufferChan: make(chan *buf.Buffer, 8),
	}
	return c
}
