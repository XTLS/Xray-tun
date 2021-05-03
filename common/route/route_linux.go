// +build linux

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

	originGW net.IP
}

var setRouteCmd = "add default gw %s"
var setInfRouteCmd = "route add default via %s dev %s table default"
var setSourceRuleCmd = "rule add from %s table default"

var delSourceRuleCmd = "rule del from %s table default"

func (h *darwinHelper) SetDefaultInterface(dev net.IP, _ interface{}) error {
	originIf, originDevName, err := h.GetDefaultInterface()
	if err != nil {
		return err
	}
	originGW, err := h.GetDefaultGateway()
	if err != nil {
		return err
	}
	h.originGW = originGW
	fmt.Println(originIf, originGW, originDevName)
	return runOsCommands(
		osCommand{"route", "delete default", "failed to modify route table 1"},
		osCommand{"route", fmt.Sprintf(setRouteCmd, dev.String()), "failed to modify route table 2"},
		osCommand{"ip", fmt.Sprintf(setInfRouteCmd, originGW.String(), originDevName), "failed to modify route table 3"},
		osCommand{"ip", fmt.Sprintf(setSourceRuleCmd, originIf.String()), "failed to modify route table 4"},
	)
}
func (h *darwinHelper) RemoveDefaultInterface(originIP interface{}) error {
	oIP := originIP.(net.IP)
	return runOsCommands(
		osCommand{"ip", fmt.Sprintf(delSourceRuleCmd, oIP.String()), "failed to remove default route 1"},
		osCommand{"route", "delete default", "failed to remove default route 2"},
		osCommand{"route", "delete default table default", "failed to remove default route 3"},
		osCommand{"route", fmt.Sprintf(setRouteCmd, h.originGW.String()), "failed to modify route table 2"},
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
