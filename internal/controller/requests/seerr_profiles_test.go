package requests

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	pvrv1alpha1 "github.com/kyleseneker/arrchitect/api/pvr/v1alpha1"
	requestsv1alpha1 "github.com/kyleseneker/arrchitect/api/requests/v1alpha1"
	"github.com/kyleseneker/arrchitect/internal/metrics"
	"github.com/kyleseneker/arrchitect/internal/testnet"
)

func TestResolveServiceProfileByName(t *testing.T) {
	cases := []struct {
		name, response string
		status, wantID int
		wantError      bool
	}{
		{"recreated profile overrides stale ID", `[{"id":8,"name":"WEB-1080p"}]`, 200, 8, false},
		{"select exact name", `[{"id":3,"name":"Other"},{"id":12,"name":"WEB-1080p"}]`, 200, 12, false},
		{"missing name", `[{"id":7,"name":"Other"}]`, 200, 0, true},
		{"duplicate name", `[{"id":7,"name":"WEB-1080p"},{"id":8,"name":"WEB-1080p"}]`, 200, 0, true},
		{"missing ID", `[{"name":"WEB-1080p"}]`, 200, 0, true},
		{"fractional ID", `[{"id":1.5,"name":"WEB-1080p"}]`, 200, 0, true},
		{"zero ID", `[{"id":0,"name":"WEB-1080p"}]`, 200, 0, true},
		{"unauthorized", `{}`, 401, 0, true},
		{"unavailable", `{}`, 503, 0, true},
		{"malformed response", `{}`, 200, 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ln, err := net.Listen("tcp", net.JoinHostPort(testnet.RoutableAddr(t), "0"))
			require.NoError(t, err)
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "GET", r.Method, "unexpected request method")
				assert.Equal(t, "/sonarr/api/v3/qualityprofile", r.URL.Path, "unexpected request path")
				assert.Equal(t, "lab-key", r.Header.Get("X-Api-Key"), "missing service authentication")
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.response))
			}))
			_ = server.Listener.Close()
			server.Listener = ln
			server.Start()
			defer server.Close()
			endpoint, _ := url.Parse(server.URL)
			n, _ := strconv.Atoi(endpoint.Port())
			port := int32(n)
			oldID := int32(7)
			svc := &requestsv1alpha1.SeerrServiceConnection{Hostname: endpoint.Hostname(), Port: &port, BaseUrl: "/sonarr/", ActiveProfileId: &oldID, ActiveProfileName: "WEB-1080p"}
			payload, err := (&SeerrReconciler{}).resolvedServicePayload(context.Background(), "media", "sonarr", svc, nil, "lab-key")
			if tc.wantError {
				require.Error(t, err)
				require.Nil(t, payload, "failed resolution returned a writable payload")
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.wantID, payload["activeProfileId"], "profile ID")
			}
			require.Equal(t, oldID, *svc.ActiveProfileId, "resolution mutated the custom resource")
		})
	}
}

func TestResolveServiceProfileIDOnly(t *testing.T) {
	id := int32(7)
	// Invalid hostname proves ID-only configurations do not need an API lookup.
	svc := &requestsv1alpha1.SeerrServiceConnection{Hostname: "", ActiveProfileId: &id}
	payload, err := (&SeerrReconciler{}).resolvedServicePayload(context.Background(), "media", "sonarr", svc, nil, "lab-key")
	require.NoError(t, err, "legacy ID-only configuration changed")
	require.Equal(t, id, payload["activeProfileId"], "legacy ID-only configuration changed")
}

// selfSignedCert is a certificate for ip that is its own CA.
func selfSignedCert(t *testing.T, ip net.IP) (tls.Certificate, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "sonarr"},
		NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, IPAddresses: []net.IP{ip},
		KeyUsage:    x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

// A Sonarr behind a private CA is looked up with the CA its resource names.
func TestResolveServiceProfileUsesTheReferencedTLSSettings(t *testing.T) {
	ip := testnet.RoutableAddr(t)
	cert, caPEM := selfSignedCert(t, net.ParseIP(ip))
	ln, err := net.Listen("tcp", net.JoinHostPort(ip, "0"))
	require.NoError(t, err)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/sonarr/api/v3/qualityprofile", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":8,"name":"WEB-1080p"}]`))
	}))
	_ = server.Listener.Close()
	server.Listener = ln
	server.TLS = &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}
	server.StartTLS()
	defer server.Close()

	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	require.NoError(t, pvrv1alpha1.AddToScheme(scheme))
	sonarr := func(tlsSpec *commonv1alpha1.TLSConfig) *SeerrReconciler {
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
			&corev1.Secret{Name: "sonarr-ca", Namespace: "media", Data: map[string][]byte{"ca.crt": caPEM}},
			&pvrv1alpha1.Sonarr{Name: "sonarr", Namespace: "media", Spec: pvrv1alpha1.SonarrSpec{
				Connection: commonv1alpha1.AppConnection{URL: server.URL + "/sonarr",
					APIKeySecretRef: commonv1alpha1.SecretKeyRef{Name: "sonarr-key", Key: "api-key"}, TLS: tlsSpec},
			}},
		).Build()
		return &SeerrReconciler{Client: c}
	}
	lookup := func(r *SeerrReconciler) (map[string]any, error) {
		svc, tlsSpec, err := r.resolveService(context.Background(), "media", requestsv1alpha1.SeerrServiceConnection{
			Name: "Sonarr", AppRef: &commonv1alpha1.AppRef{Kind: "Sonarr", Name: "sonarr"}, ActiveProfileName: "WEB-1080p",
		})
		require.NoError(t, err)
		return r.resolvedServicePayload(context.Background(), "media", "sonarr", &svc, tlsSpec, "lab-key")
	}

	payload, err := lookup(sonarr(&commonv1alpha1.TLSConfig{CASecretRef: commonv1alpha1.SecretKeyRef{Name: "sonarr-ca", Key: "ca.crt"}}))
	require.NoError(t, err)
	assert.Equal(t, 8, payload["activeProfileId"])

	networkErrors := func() float64 {
		return testutil.ToFloat64(metrics.AppAPIErrorsTotal.WithLabelValues("sonarr", "network"))
	}
	before := networkErrors()
	_, err = lookup(sonarr(nil))
	require.ErrorContains(t, err, "certificate signed by unknown authority", "without the CA the private certificate must be refused")
	assert.Equal(t, before+1, networkErrors(), "the lookup counts as Sonarr's")
}
