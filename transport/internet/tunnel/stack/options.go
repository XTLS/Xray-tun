package stack

import (
	"golang.org/x/time/rate"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv4"
	"gvisor.dev/gvisor/pkg/tcpip/transport/tcp"
)

type option func(stack *Stack) error

func SetDefaultTTL(ttl int) option {
	return func(stack *Stack) error {
		opt := tcpip.DefaultTTLOption(ttl)
		if err := stack.stack.SetNetworkProtocolOption(ipv4.ProtocolNumber, &opt); err != nil {
			return newError("failed to set TTL: " + err.String())
		}
		return nil
	}
}

func SetForwarding() option {
	return func(stack *Stack) error {
		if err := stack.stack.SetForwardingDefaultAndAllNICs(ipv4.ProtocolNumber, true); err != nil {
			return newError("failed to set ipv4 forwarding: " + err.String())
		}
		return nil
	}
}

func SetICMP() option {
	return func(stack *Stack) error {
		stack.stack.SetICMPBurst(50)
		stack.stack.SetICMPLimit(rate.Limit(1000))
		return nil
	}
}

func SetTCPBuffer(min, def, max int) option {
	return func(stack *Stack) error {
		rcvOpt := tcpip.TCPReceiveBufferSizeRangeOption{min, def, max}
		if err := stack.stack.SetTransportProtocolOption(tcp.ProtocolNumber, &rcvOpt); err != nil {
			return newError("failed to set TCP receive buffer size: " + err.String())
		}
		sndOpt := tcpip.TCPSendBufferSizeRangeOption{min, def, max}
		if err := stack.stack.SetTransportProtocolOption(tcp.ProtocolNumber, &sndOpt); err != nil {
			return newError("failed to set TCP send buffer size: " + err.String())
		}
		return nil
	}
}

func SetCongestionControl(o string) option {
	return func(stack *Stack) error {
		opt := tcpip.CongestionControlOption("reno")
		if err := stack.stack.SetTransportProtocolOption(tcp.ProtocolNumber, &opt); err != nil {
			return newError("failed to set congestion control: ", err.String())
		}
		return nil
	}
}

func SetDelay(enable bool) option {
	return func(stack *Stack) error {
		opt := tcpip.TCPDelayEnabled(enable)
		if err := stack.stack.SetTransportProtocolOption(tcp.ProtocolNumber, &opt); err != nil {
			return newError("failed to set TCP delay: ", err.String())
		}
		return nil
	}
}

func SetModerateReceiveBuffer(enable bool) option {
	return func(stack *Stack) error {
		opt := tcpip.TCPModerateReceiveBufferOption(enable)
		if err := stack.stack.SetTransportProtocolOption(tcp.ProtocolNumber, &opt); err != nil {
			return newError("failed to set TCP moderate receive buffer: ", err.String())
		}
		return nil
	}
}

func SetSACK(enable bool) option {
	return func(stack *Stack) error {
		opt := tcpip.TCPSACKEnabled(enable)
		if err := stack.stack.SetTransportProtocolOption(tcp.ProtocolNumber, &opt); err != nil {
			return newError("failed to set TCP SACK: ", err.String())
		}
		return nil
	}
}
