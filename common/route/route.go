package route

//go:generate errorgen

import (
	"github.com/xtls/xray-core/common/net"
	"os/exec"
	"strings"
)

type Helper interface {
	GetDefaultInterface() (net.IP, string, error)
	GetDefaultGateway() (net.IP, error)
	SetDefaultInterface(net.IP, interface{}) error
	RemoveDefaultInterface(interface{}) error
}

// GetHelper return RouterHelper for windows
func GetHelper() Helper {
	return defaultHelper
}

type osCommand struct {
	name string
	arg  string
	err  string
}

func runOsCommands(command ...osCommand) error {
	for _, cmd := range command {
		b, err := exec.Command(cmd.name, strings.Split(cmd.arg, " ")...).CombinedOutput()
		if err != nil {
			return newError(cmd.err + ": " + string(b)).Base(err)
		}
	}
	return nil
}
