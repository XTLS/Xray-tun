// +build !confonly

package tunnel

import (
	"net"

	"github.com/xtls/xray-core/common"
	"github.com/xtls/xray-core/transport/internet"
	_ "github.com/xtls/xray-core/transport/internet/tunnel/stack"
)

//go:generate go run github.com/xtls/xray-core/common/errors/errorgen

const protocolName = "tunnel"

type WriteFrom func([]byte, net.Addr) (int, error)

func init() {
	common.Must(internet.RegisterProtocolConfigCreator(protocolName, func() interface{} {
		return new(Config)
	}))
}
