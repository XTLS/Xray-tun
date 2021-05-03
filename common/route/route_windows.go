package route

import (
	gnet "net"

	"github.com/xtls/xray-core/common/net"
	"golang.org/x/sys/windows"
	"golang.zx2c4.com/wireguard/windows/tunnel/winipcfg"
)

var defaultHelper = &winHelper{}

type winHelper struct {
	defaultGatewayCache   net.IP
	defaultInterfaceCache net.IP
}

func (h *winHelper) GetDefaultInterface() (net.IP, string, error) {
	if h.defaultInterfaceCache != nil {
		return h.defaultInterfaceCache, "", nil
	}
	thisip, err := h.GetDefaultGateway()
	if err != nil {
		return nil, "", err
	}
	infs, gerr := gnet.Interfaces()
	if gerr != nil {
		return nil, "", newError("failed to find interfaces").Base(gerr).AtError()
	}
	for _, ips := range infs {
		ips, err := ips.Addrs()
		if err != nil {
			continue
		}
		for _, ip := range ips {
			ipnet, ok := ip.(*net.IPNet)
			if !ok {
				continue
			}
			if ipnet.Contains(thisip) {
				h.defaultInterfaceCache = ipnet.IP
				return ipnet.IP, "", nil
			}
		}
	}
	return nil, "", newError("failed to find default interface ip")
}
func (h *winHelper) GetDefaultGateway() (net.IP, error) {
	rows, err := winipcfg.GetIPForwardTable2(windows.AF_INET)
	if err != nil {
		return nil, newError("failed to get default gateway").Base(err)
	}
	for _, row := range rows {
		if row.DestinationPrefix.IPNet().IP.Equal(net.AnyIP.IP()) {
			h.defaultGatewayCache = row.NextHop.IP()
			return h.defaultGatewayCache, nil
		}
	}
	return nil, newError("failed to find default gateway: not found")
}

func (*winHelper) SetDefaultInterface(addr net.IP, devName interface{}) error {
	luid, ok := devName.(winipcfg.LUID)

	if !ok {
		panic("devName should be a LUID")
	}

	routes := make([]*winipcfg.RouteData, 0, 1)
	routes = append(routes, &winipcfg.RouteData{
		Destination: net.IPNet{
			IP:   gnet.IPv4zero,
			Mask: gnet.IPMask(net.ParseIP("0.0.0.0").To4()),
		},
		NextHop: addr,
		Metric:  0,
	})
	if err := luid.SetRoutesForFamily(windows.AF_INET, routes); err != nil {
		return newError("failed to set route for device").Base(err)
	}
	return nil

}
func (*winHelper) RemoveDefaultInterface(devName interface{}) error {
	luid, ok := devName.(winipcfg.LUID)

	if !ok {
		panic("devName should be a LUID")
	}
	if err := luid.FlushRoutes(windows.AF_INET); err != nil {
		return newError("unable to remove route for devName").Base(err)
	}
	return nil
}
