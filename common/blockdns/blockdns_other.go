// +build !windows

package blockdns

import "runtime"

func FixDnsLeakage(tunName string) error {
	return newError("unsupported feature for " + runtime.GOOS)
}
