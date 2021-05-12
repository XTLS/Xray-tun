// +build linux

package route

import (
	"net"

	"github.com/vishvananda/netlink"
)

type linuxHelper struct {
	defaultGatewayCache   net.IP
	defaultInterfaceCache net.IP
	defaultInfNameCache   string

	originGW net.IP
}

func (h *linuxHelper) SetDefaultInterface(dev net.IP, _ interface{}) error {
	oif, _, err := h.GetDefaultInterface()
	if err != nil {
		return err
	}
	oroute, err := h.getDefaultRoute()
	if err != nil {
		return err
	}

	err = netlink.RouteDel(&netlink.Route{Dst: &net.IPNet{IP: net.IPv4zero}})
	if err != nil {
		return newError("failed to del default route").Base(err)
	}

	err = netlink.RouteAdd(&netlink.Route{Gw: dev})
	if err != nil {
		return newError("failed to set default route").Base(err)
	}

	err = netlink.RouteAdd(&netlink.Route{LinkIndex: oroute.LinkIndex, Src: oif, Priority: 1})
	if err != nil {
		return newError("failed to set default route2").Base(err)
	}

	return nil
}
func (h *linuxHelper) RemoveDefaultInterface(originIP interface{}) error {
	oIP := originIP.(net.IP)
	err := netlink.RouteDel(&netlink.Route{Dst: &net.IPNet{IP: net.IPv4zero}})
	if err != nil {
		return err
	}

	err = netlink.RouteAdd(&netlink.Route{Gw: oIP})
	if err != nil {
		return err
	}

	return nil
}

var defaultHelper = &linuxHelper{}

func (h *linuxHelper) GetDefaultGateway() (net.IP, error) {
	if h.defaultGatewayCache != nil {
		return h.defaultGatewayCache, nil
	}

	route, err := h.getDefaultRoute()
	if err != nil {
		return nil, err
	}
	return route.Gw, nil
}

func (h *linuxHelper) GetDefaultInterface() (net.IP, string, error) {
	if h.defaultInterfaceCache != nil {
		return h.defaultInterfaceCache, h.defaultInfNameCache, nil
	}
	route, err := h.getDefaultRoute()
	if err != nil {
		return nil, "", err
	}

	if route.Dst == nil {
		link, err := netlink.LinkByIndex(route.LinkIndex)
		if err != nil {
			return nil, "", err
		}
		addrs, err := netlink.AddrList(link, netlink.FAMILY_V4)
		if err != nil {
			return nil, "", err
		}
		for _, addr := range addrs {
			h.defaultInterfaceCache = addr.IP
			h.defaultInfNameCache = addr.Label
			return addr.IP, addr.Label, nil
		}
	}

	return nil, "", newError("not found")
}

func (h *linuxHelper) getDefaultRoute() (*netlink.Route, error) {
	r, err := netlink.RouteList(nil, netlink.FAMILY_V4)
	if err != nil {
		return nil, err
	}

	for _, route := range r {
		if route.Dst == nil {
			return &route, nil
		}
	}
	return nil, err
}
