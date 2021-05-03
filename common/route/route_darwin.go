// +build darwin

package route

import (
	"fmt"
	"net"
	"os/exec"
	"strings"
)

type darwinHelper struct {
	defaultGatewayCache   net.IP
	defaultInterfaceCache net.IP
	defaultInfNameCache   string
}

var setRouteCmd = "add default %s"
var setInfRouteCmd = "add default %s -ifscope %s"

func (h *darwinHelper) SetDefaultInterface(dev net.IP, _ interface{}) error {
	_, originDevName, err := h.GetDefaultInterface()
	if err != nil {
		return err
	}
	originGW, err := h.GetDefaultGateway()
	if err != nil {
		return err
	}
	return runOsCommands(
		osCommand{"route", "delete default", "failed to modify route table 1"},
		osCommand{"route", fmt.Sprintf(setRouteCmd, dev.String()), "failed to modify route table 2"},
		osCommand{"route", fmt.Sprintf(setInfRouteCmd, originGW.String(), originDevName), "failed to modify route table 3"},
	)
}
func (h *darwinHelper) RemoveDefaultInterface(ogw interface{}) error {
	originIP := ogw.(net.IP)
	return runOsCommands(
		osCommand{"route", "delete default", "failed to remove default route"},
		osCommand{"route", "add default " + originIP.String(), "failed to remove default route"},
	)
}

var defaultHelper = &darwinHelper{}

func (h *darwinHelper) GetDefaultGateway() (net.IP, error) {
	if h.defaultGatewayCache != nil {
		return h.defaultGatewayCache, nil
	}
	s, err := exec.Command("bash", "-c", `echo $(netstat -rn | grep "default" | awk '{print $2}' )`).Output()
	if err != nil {
		return nil, newError("failed to find default route").Base(err)
	}
	ip := net.ParseIP(strings.Split(string(s), " ")[0])
	h.defaultGatewayCache = ip
	return ip, nil
}

func (h *darwinHelper) GetDefaultInterface() (net.IP, string, error) {
	if h.defaultInterfaceCache != nil {
		return h.defaultInterfaceCache, h.defaultInfNameCache, nil
	}
	infs, err := net.Interfaces()
	if err != nil {
		return nil, "", err
	}
	for _, inf := range infs {
		addrs, err := inf.Addrs()
		if err != nil {
			return nil, "", err
		}
		for _, address := range addrs {
			if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
				if ipnet.IP.To4() != nil {
					h.defaultInfNameCache = inf.Name
					h.defaultInterfaceCache = ipnet.IP
					return ipnet.IP, inf.Name, nil
				}
			}
		}
	}
	return nil, "", newError("not found")
}
