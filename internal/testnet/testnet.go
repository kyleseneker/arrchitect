// Package testnet holds network helpers shared by tests.
package testnet

import (
	"net"
	"net/http"
	"net/http/httptest"
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

// Serve starts a server for handler on RoutableAddr, closed when the test
// ends.
func Serve(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	ln, err := net.Listen("tcp", net.JoinHostPort(RoutableAddr(t), "0"))
	if err != nil {
		t.Skipf("cannot bind a routable listener: %v", err)
	}
	srv := httptest.NewUnstartedServer(handler)
	_ = srv.Listener.Close()
	srv.Listener = ln
	srv.Start()
	t.Cleanup(srv.Close)
	return srv
}
