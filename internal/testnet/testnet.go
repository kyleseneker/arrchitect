// Package testnet holds network and client helpers shared by tests.
package testnet

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kyleseneker/arrchitect/internal/httpapi"
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

// AppLabel is the app label of the clients Client builds, under which tests
// read the per-app metrics.
const AppLabel = "test-app"

// Client returns a client for the app at base that sends apiKey. Its plain
// transport reaches a test server on loopback, which the operator's own
// client refuses.
func Client(t *testing.T, base, apiKey string) *httpapi.HTTPClient {
	t.Helper()
	hc, err := httpapi.NewHTTPClient(base, httpapi.AuthAPIKey,
		httpapi.WithAPIKey(apiKey), httpapi.WithTransport(&http.Transport{}), httpapi.WithAppLabel(AppLabel))
	require.NoError(t, err)
	return hc
}

// ServeApp starts a loopback server for handler, closed when the test ends,
// and returns a Client for it.
func ServeApp(t *testing.T, handler http.HandlerFunc) *httpapi.HTTPClient {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return Client(t, srv.URL, "test-key")
}
