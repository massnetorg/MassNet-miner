// +build !appengine

package massutil

import (
	"net"
)

// interfaceAddrs returns a list of the system's network interface addresses.
// It is wrapped here so that we can substitute it for other functions when
// building for systems that do not allow access to net.InterfaceAddrs().
func interfaceAddrs() ([]net.Addr, error) {
	return net.InterfaceAddrs()
}
