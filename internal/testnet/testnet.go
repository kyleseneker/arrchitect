// Package testnet holds network helpers shared by tests.
package testnet

import (
	"net"
	"testing"
)

// RoutableAddr returns a non-loopback IPv4 address of this host, skipping the
// test when there is none. The operator's HTTP client refuses loopback and
// link-local addresses, so fake app servers must listen on this one.
func RoutableAddr(t *testing.T) string {
	t.Helper()
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		t.Skipf("cannot enumerate interfaces: %v", err)
	}
	for _, a := range addrs {
		ipnet, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		ip := ipnet.IP.To4()
		if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
			continue
		}
		return ip.String()
	}
	t.Skip("no routable non-loopback IPv4 address on this host")
	return ""
}
