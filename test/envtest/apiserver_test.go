// Package envtest runs the CRDs, samples and controllers against a real
// kube-apiserver. Schema mistakes, status subresource writes and finalizers are
// invisible to unit tests, which never talk to an API server.
//
// Skipped unless KUBEBUILDER_ASSETS is set, which `make test` does.
package envtest

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

// startControlPlane boots one control plane for a set of API groups, so a table
// of kinds does not pay for a control plane each.
func startControlPlane(t *testing.T, adders ...func(*runtime.Scheme) error) (*rest.Config, client.Client) {
	t.Helper()
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Skip("KUBEBUILDER_ASSETS unset; run via `make test`")
	}

	env := &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}
	cfg, err := env.Start()
	require.NoError(t, err, "starting control plane")
	t.Cleanup(func() { _ = env.Stop() })

	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme), "client-go scheme")
	for _, add := range adders {
		require.NoError(t, add(scheme), "registering scheme")
	}
	c, err := client.New(cfg, client.Options{Scheme: scheme})
	require.NoError(t, err, "building client")
	return cfg, c
}

func startAPIServerWith(t *testing.T, adders ...func(*runtime.Scheme) error) client.Client {
	t.Helper()
	_, c := startControlPlane(t, adders...)
	return c
}

// fakeAppServer answers every request with a JSON body, on a routable address
// because the HTTP client refuses to dial loopback.
func fakeAppServer(t *testing.T) string {
	t.Helper()
	return serveApp(t, func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
	})
}

// fakeListAppServer answers as a PVR-style API with nothing configured: an
// empty list for each collection, and the created item for each write.
func fakeListAppServer(t *testing.T) string {
	t.Helper()
	return serveApp(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/system/status"):
			_ = json.NewEncoder(w).Encode(map[string]any{"version": "1.0.0"})
		case r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode([]any{})
		default:
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body == nil {
				body = map[string]any{}
			}
			body["id"] = 1
			_ = json.NewEncoder(w).Encode(body)
		}
	})
}

func serveApp(t *testing.T, handle http.HandlerFunc) string {
	t.Helper()
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		t.Skipf("cannot enumerate interfaces: %v", err)
	}
	host := ""
	for _, a := range addrs {
		ipnet, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		if ip := ipnet.IP.To4(); ip != nil && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() {
			host = ip.String()
			break
		}
	}
	if host == "" {
		t.Skip("no routable non-loopback IPv4 address on this host")
	}

	ln, err := net.Listen("tcp", net.JoinHostPort(host, "0"))
	if err != nil {
		t.Skipf("cannot bind a routable listener: %v", err)
	}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		handle(w, r)
	}))
	_ = srv.Listener.Close()
	srv.Listener = ln
	srv.Start()
	t.Cleanup(srv.Close)
	return srv.URL
}
