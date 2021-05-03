package tun

import (
	"crypto/md5"
	"io"
	"log"
	"net"
	"unsafe"

	"golang.org/x/crypto/hkdf"
	"golang.org/x/sys/windows"
	wtun "golang.zx2c4.com/wireguard/tun"
	"golang.zx2c4.com/wireguard/windows/tunnel/winipcfg"
)

type WintunDevice struct {
	tun     *wtun.NativeTun
	addr    string
	address []net.IPNet
	mask    string
	gateway string
	name    string
	dns     []string
	mtu     int
}

func (w *WintunDevice) LUID() winipcfg.LUID {
	return winipcfg.LUID(w.tun.LUID())
}

func (w *WintunDevice) GetIdentifier() interface{} {
	return w.LUID()
}

func (w *WintunDevice) Write(input []byte) (int, error) {
	return w.tun.Write(input, 0)
}

func (w *WintunDevice) Read(input []byte) (int, error) {
	return w.tun.Read(input, 0)
}

func (w *WintunDevice) Close() error {
	w.cleanInfAddr(windows.AF_INET, w.address)
	return w.tun.Close()
}

func (w *WintunDevice) setInfAddr() error {
	luid := winipcfg.LUID(w.tun.LUID())
	w.address = append([]net.IPNet{}, net.IPNet{
		IP:   net.ParseIP(w.addr).To4(),
		Mask: net.IPMask(net.ParseIP(w.mask).To4()),
	})

	err := luid.SetIPAddressesForFamily(windows.AF_INET, w.address)
	if err == windows.ERROR_OBJECT_ALREADY_EXISTS {
		w.cleanInfAddr(windows.AF_INET, w.address)
		err = luid.SetIPAddressesForFamily(windows.AF_INET, w.address)
	}
	if err != nil {
		return err
	}

	dnss := make([]net.IP, 0)
	for _, ip := range w.dns {
		dnss = append(dnss, net.ParseIP(ip).To4())
	}

	err = luid.SetDNS(windows.AF_INET, dnss, nil)
	luid.FlushRoutes(windows.AF_INET6)
	return err
}

func (w *WintunDevice) cleanInfAddr(family winipcfg.AddressFamily, addresses []net.IPNet) {
	if len(addresses) == 0 {
		return
	}
	includedInAddresses := func(a net.IPNet) bool {
		for _, addr := range addresses {
			ip := addr.IP
			if ip4 := ip.To4(); ip4 != nil {
				ip = ip4
			}
			mA, _ := addr.Mask.Size()
			mB, _ := a.Mask.Size()
			if ip.Equal(a.IP) && mA == mB {
				return true
			}
		}
		return false
	}
	interfaces, err := winipcfg.GetAdaptersAddresses(family, winipcfg.GAAFlagDefault)
	if err != nil {
		return
	}
	for _, iface := range interfaces {
		if iface.OperStatus == winipcfg.IfOperStatusUp {
			continue
		}
		for address := iface.FirstUnicastAddress; address != nil; address = address.Next {
			ip := address.Address.IP()
			ipnet := net.IPNet{IP: ip, Mask: net.CIDRMask(int(address.OnLinkPrefixLength), 8*len(ip))}
			if includedInAddresses(ipnet) {
				log.Printf("Cleaning up stale address %s from interface ‘%s’", ipnet.String(), iface.FriendlyName())
				iface.LUID.DeleteIPAddress(ipnet)
			}
		}
	}
}

func determineGUID(name string) *windows.GUID {
	b := make([]byte, unsafe.Sizeof(windows.GUID{}))
	if _, err := io.ReadFull(hkdf.New(md5.New, []byte(name), nil, nil), b); err != nil {
		return nil
	}
	return (*windows.GUID)(unsafe.Pointer(&b[0]))
}

func openTunDev(name, addr, gw, mask string, dns []string) (Device, error) {
	d := &WintunDevice{mask: mask, addr: addr, name: name, gateway: gw, dns: dns}

	tundev, err := wtun.CreateTUNWithRequestedGUID(name, determineGUID(name), 1500)
	if err != nil {
		return nil, err
	}
	d.tun = tundev.(*wtun.NativeTun)
	if d.name, err = d.tun.Name(); err != nil {
		return nil, err
	}
	if d.mtu, err = d.tun.MTU(); err != nil {
		return nil, err
	}
	if err := d.setInfAddr(); err != nil {
		return nil, err
	}
	return d, nil
}
