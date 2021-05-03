package tun

import (
	"fmt"
	"net"
	"os/exec"
	"strings"

	"github.com/songgao/water"
)

type LinuxTunDev struct {
	*water.Interface
}

func (t *LinuxTunDev) GetIdentifier() interface{} {
	return t.Interface.Name()
}
func openTunDev(name, addr, gw, mask string, dnsServers []string) (*LinuxTunDev, error) {
	cfg := water.Config{
		DeviceType: water.TUN,
	}
	cfg.Name = name
	tunDev, err := water.New(cfg)
	if err != nil {
		return nil, err
	}
	name = tunDev.Name()

	ipMask := net.IPMask(net.ParseIP(mask).To4())
	maskSize, _ := ipMask.Size()

	params := fmt.Sprintf("addr add %s/%d dev %s", gw, maskSize, name)
	out, err := exec.Command("ip", strings.Split(params, " ")...).Output()
	if err != nil {
		if len(out) != 0 {
			return nil, newError("failed to set addr: " + string(out)).Base(err)
		}
		return nil, newError("failed to set addr").Base(err)
	}

	params = fmt.Sprintf("link set dev %s up", name)
	out, err = exec.Command("ip", strings.Split(params, " ")...).Output()
	if err != nil {
		if len(out) != 0 {
			return nil, newError("failed to set dev: " + string(out)).Base(err)
		}
		return nil, newError("failed to set dev").Base(err)
	}

	return &LinuxTunDev{tunDev}, nil
}
