package conf

import (
	"github.com/golang/protobuf/proto"
	"github.com/xtls/xray-core/proxy/tun"
)

type TunnelConfig struct {
	UserLevel uint32 `json:"userLevel"`
}

func (v *TunnelConfig) Build() (proto.Message, error) {
	config := new(tun.Config)
	config.UserLevel = v.UserLevel
	return config, nil
}

type TunnelServerConfig struct {
	UserLevel uint32 `json:"userLevel"`
}

func (v *TunnelServerConfig) Build() (proto.Message, error) {
	config := new(tun.ServerConfig)
	config.UserLevel = v.UserLevel
	return config, nil
}
