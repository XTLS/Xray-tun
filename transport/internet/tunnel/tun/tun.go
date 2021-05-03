package tun

import (
	"io"
)

//go:generate go run github.com/xtls/xray-core/common/errors/errorgen

type Device interface {
	io.ReadWriteCloser
	GetIdentifier() interface{}
}

func OpenTUNDevice(name, addr, gw, mask string, dns []string) (Device, error) {
	return openTunDev(name, addr, gw, mask, dns)
}
