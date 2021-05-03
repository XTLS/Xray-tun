package stack

import (
	"io"
	"sync"

	"gvisor.dev/gvisor/pkg/tcpip/buffer"
	"gvisor.dev/gvisor/pkg/tcpip/header"
	"gvisor.dev/gvisor/pkg/tcpip/link/channel"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
)

type Endpoint struct {
	*channel.Endpoint
	mtu int
	dev io.ReadWriteCloser
	buf []byte
	mu  sync.Mutex
	wt  io.Writer
}

func NewEndpoint(dev io.ReadWriteCloser, mtu int) stack.LinkEndpoint {
	ep := &Endpoint{
		Endpoint: channel.New(512, uint32(mtu), ""),
		dev:      dev,
		mtu:      mtu,
		buf:      make([]byte, mtu),
		wt:       dev.(io.Writer),
	}
	ep.Endpoint.AddNotify(ep)
	return ep
}

func (e *Endpoint) Attach(dispatcher stack.NetworkDispatcher) {
	e.Endpoint.Attach(dispatcher)

	go func(r io.Reader, size int, ep *channel.Endpoint) {
		for {
			buf := make([]byte, size)
			n, err := r.Read(buf)
			if err != nil {
				break
			}
			buf = buf[:n]

			switch header.IPVersion(buf) {
			case header.IPv4Version:
				ep.InjectInbound(header.IPv4ProtocolNumber, stack.NewPacketBuffer(stack.PacketBufferOptions{
					Data: buffer.View(buf).ToVectorisedView(),
				}))
			}
		}
	}(e.dev, e.mtu, e.Endpoint)
}

func (e *Endpoint) WriteNotify() {
	info, ok := e.Endpoint.Read()
	if !ok {
		return
	}

	e.mu.Lock()
	buf := append(e.buf[:0], info.Pkt.NetworkHeader().View()...)
	buf = append(buf, info.Pkt.TransportHeader().View()...)
	for _, view := range info.Pkt.Data().Views() {
		buf = append(buf, view...)
	}
	e.wt.Write(buf)
	e.mu.Unlock()
}
