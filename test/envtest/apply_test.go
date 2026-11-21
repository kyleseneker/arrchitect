package envtest

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/yaml"
)

const group = "arrchitect.dev/v1alpha1"

func TestSamplesApply(t *testing.T) {
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
	require.NoError(t, clientgoscheme.AddToScheme(scheme), "building scheme")
	c, err := client.New(cfg, client.Options{Scheme: scheme})
	require.NoError(t, err, "building client")

	ctx := context.Background()

	dirs := []string{"config/samples", "examples"}
	patterns := make([]string, 0, len(dirs))
	for _, dir := range dirs {
		patterns = append(patterns, filepath.Join("..", "..", dir, "*.yaml"))
	}

	applied := 0
	for _, pattern := range patterns {
		paths, err := filepath.Glob(pattern)
		require.NoError(t, err, "globbing %s", pattern)
		for _, path := range paths {
			// Each file gets its own namespace. Samples reuse names across files
			// and span several namespaces; isolating them keeps a collision from
			// masking a genuine schema rejection.
			ns := namespaceFor(path)
			require.NoError(t, c.Create(ctx, &corev1.Namespace{Name: ns}), "creating namespace %s", ns)
			for _, obj := range decode(t, path) {
				if obj.GetAPIVersion() != group {
					continue
				}
				obj.SetNamespace(ns)
				name := obj.GetKind() + "/" + obj.GetName()
				if !assert.NoError(t, c.Create(ctx, obj), "%s: applying %s", filepath.Base(path), name) {
					continue
				}
				applied++
			}
		}
	}
	t.Logf("applied %d resources", applied)
	require.NotZero(t, applied, "no resources applied; sample discovery is broken")
}

// namespaceFor derives a DNS-1123 namespace name from a manifest path.
func namespaceFor(path string) string {
	base := strings.TrimSuffix(filepath.Base(path), ".yaml")
	base = strings.ToLower(strings.ReplaceAll(base, "_", "-"))
	if parent := filepath.Base(filepath.Dir(path)); parent != "" {
		base = parent + "-" + base
	}
	if len(base) > 63 {
		base = base[:63]
	}
	return strings.Trim(base, "-")
}

func decode(t *testing.T, path string) []*unstructured.Unstructured {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err, "reading %s", path)
	var out []*unstructured.Unstructured
	reader := utilyaml.NewYAMLReader(bufio.NewReader(bytes.NewReader(data)))
	for {
		doc, err := reader.Read()
		if err != nil {
			break
		}
		if len(strings.TrimSpace(string(doc))) == 0 {
			continue
		}
		obj := &unstructured.Unstructured{}
		require.NoError(t, yaml.Unmarshal(doc, obj), "parsing %s", path)
		if obj.GetKind() == "" {
			continue
		}
		out = append(out, obj)
	}
	return out
}

// TestAdmissionRules checks what the API server accepts and rejects for the
// rules the CRDs carry: list keys, CEL rules, patterns and property limits.
func TestAdmissionRules(t *testing.T) {
	c := startAPIServerWith(t)
	const conn = `
  connection:
    url: http://app:8080
    apiKeySecretRef: {name: app, key: api-key}`
	for i, tt := range []struct {
		name, kind, spec string
		rejected         string
	}{
		{name: "one delay profile without tags", kind: "Sonarr", spec: conn + `
  delayProfiles: [{tagLabels: [anime]}, {usenetDelay: 30}]`},
		{name: "two delay profiles without tags", kind: "Sonarr", spec: conn + `
  delayProfiles: [{usenetDelay: 30}, {tagLabels: []}]`,
			rejected: "only one delay profile can be without tags"},
		{name: "duplicate notification names", kind: "Radarr", spec: conn + `
  notifications: [{name: Hook, implementation: Webhook}, {name: Hook, implementation: Discord}]`,
			rejected: "Duplicate value"},
		{name: "duplicate field names", kind: "SonarrIndexer", spec: `
  sonarrRef: {name: tv}
  name: x
  protocol: torrent
  implementation: Torznab
  fields: [{name: baseUrl, value: a}, {name: baseUrl, value: b}]`,
			rejected: "Duplicate value"},
		{name: "duplicate tag labels", kind: "SonarrIndexer", spec: `
  sonarrRef: {name: tv}
  name: x
  protocol: torrent
  implementation: Torznab
  tagLabels: [anime, anime]`,
			rejected: "Duplicate value"},
		{name: "a field with both value and valueFrom", kind: "SonarrIndexer", spec: `
  sonarrRef: {name: tv}
  name: x
  protocol: torrent
  implementation: Torznab
  fields: [{name: apiKey, value: a, valueFrom: {secretKeyRef: {name: s, key: k}}}]`,
			rejected: "must have at most 2 items"},
		{name: "an indexer that names no Sonarr", kind: "SonarrIndexer", spec: `
  name: x
  protocol: torrent
  implementation: Torznab`,
			rejected: "sonarrRef: Required value"},
		{name: "an unknown deletion policy", kind: "SonarrDownloadClient", spec: `
  sonarrRef: {name: tv}
  deletionPolicy: keep
  name: x
  protocol: torrent
  implementation: QBittorrent
  host: qbittorrent
  port: 8080`,
			rejected: "Unsupported value"},
		{name: "an interval under 30s", kind: "Sonarr", spec: conn + `
  reconcile: {intervalSeconds: 10}`,
			rejected: "should be greater than or equal to 30"},
		{name: "delete in observe mode", kind: "Sonarr", spec: conn + `
  reconcile: {deletionPolicy: delete, driftPolicy: observe}`,
			rejected: "deletionPolicy delete conflicts with driftPolicy observe"},
		{name: "a URL without a scheme", kind: "Sonarr", spec: `
  connection:
    url: sonarr:8989
    apiKeySecretRef: {name: app, key: api-key}`,
			rejected: "should match '^https?://'"},
		{name: "a Lidarr root folder without profiles", kind: "Lidarr", spec: conn + `
  rootFolders: [{path: /music}]`,
			rejected: "Lidarr root folders need defaultQualityProfileName and defaultMetadataProfileName"},
		{name: "prune on Jellyfin", kind: "Jellyfin", spec: `
  connection: {url: http://jellyfin:8096}
  adminUser:
    usernameSecretRef: {name: app, key: username}
    passwordSecretRef: {name: app, key: password}
  reconcile: {prune: true}`,
			rejected: "Jellyfin doesn't support prune"},
		{name: "deletionPolicy delete on Plex", kind: "Plex", spec: `
  connection:
    url: http://plex:32400
    tokenSecretRef: {name: app, key: token}
  reconcile: {deletionPolicy: delete}`,
			rejected: "Plex doesn't support deletionPolicy delete"},
		{name: "Seerr with both auths", kind: "Seerr", spec: `
  connection:
    url: http://seerr:5055
    apiKeySecretRef: {name: app, key: api-key}
  jellyfinAuth:
    hostname: jellyfin
    port: 8096
    usernameSecretRef: {name: app, key: username}
    passwordSecretRef: {name: app, key: password}
  plexAuth:
    hostname: plex
    tokenSecretRef: {name: app, key: token}`,
			rejected: "jellyfinAuth and plexAuth are mutually exclusive"},
		{name: "a Prowlarr application by reference", kind: "Prowlarr", spec: conn + `
  applications: [{name: Sonarr, appRef: {kind: Sonarr, name: sonarr}}]`},
		{name: "a Prowlarr application by reference and address", kind: "Prowlarr", spec: conn + `
  applications: [{name: Sonarr, appRef: {kind: Sonarr, name: sonarr}, baseUrl: http://sonarr:8989}]`,
			rejected: "set appRef, or implementation, baseUrl and apiKeySecretRef, not both"},
		{name: "a Prowlarr application with neither", kind: "Prowlarr", spec: conn + `
  applications: [{name: Sonarr, implementation: Sonarr}]`,
			rejected: "set appRef, or implementation, baseUrl and apiKeySecretRef, not both"},
		{name: "a Prowlarr application referring to Seerr", kind: "Prowlarr", spec: conn + `
  applications: [{name: Seerr, appRef: {kind: Seerr, name: seerr}}]`,
			rejected: "an application refers to a Sonarr, Radarr, Lidarr or Readarr"},
		{name: "a reference to an unknown kind", kind: "Prowlarr", spec: conn + `
  applications: [{name: Whisparr, appRef: {kind: Whisparr, name: whisparr}}]`,
			rejected: "Unsupported value"},
		{name: "a reference to another group", kind: "Prowlarr", spec: conn + `
  applications: [{name: Sonarr, appRef: {group: example.com, kind: Sonarr, name: sonarr}}]`,
			rejected: "Unsupported value"},
		{name: "a reference by an invalid name", kind: "Prowlarr", spec: conn + `
  applications: [{name: Sonarr, appRef: {kind: Sonarr, name: Not_A_Name}}]`,
			rejected: "should match"},
		{name: "Seerr servers and media server by reference", kind: "Seerr", spec: `
  connection: {url: http://seerr:5055}
  jellyfinAuth: {appRef: {kind: Jellyfin, name: jellyfin}}
  sonarr: [{name: Sonarr, appRef: {kind: Sonarr, name: sonarr}}]
  radarr: [{name: Radarr, appRef: {kind: Radarr, name: radarr}}]`},
		{name: "a Seerr sonarr server referring to Radarr", kind: "Seerr", spec: `
  connection: {url: http://seerr:5055}
  sonarr: [{name: Sonarr, appRef: {kind: Radarr, name: radarr}}]`,
			rejected: "a sonarr server refers to a Sonarr"},
		{name: "an empty Seerr jellyfinAuth", kind: "Seerr", spec: `
  connection: {url: http://seerr:5055}
  jellyfinAuth: {}`,
			rejected: "should have at least 1 properties"},
		{name: "a Seerr plexAuth by reference and hostname", kind: "Seerr", spec: `
  connection: {url: http://seerr:5055}
  plexAuth: {appRef: {kind: Plex, name: plex}, hostname: plex}`,
			rejected: "set appRef, or hostname and tokenSecretRef, not both"},
		{name: "Bazarr connections by reference", kind: "Bazarr", spec: conn + `
  sonarrConnection: {appRef: {kind: Sonarr, name: sonarr}}
  radarrConnection: {appRef: {kind: Radarr, name: radarr}, syncInterval: 60}`},
		{name: "a Bazarr sonarrConnection referring to Radarr", kind: "Bazarr", spec: conn + `
  sonarrConnection: {appRef: {kind: Radarr, name: radarr}}`,
			rejected: "sonarrConnection refers to a Sonarr"},
		{name: "Maintainerr connections by reference", kind: "Maintainerr", spec: `
  connection: {url: http://maintainerr:6246}
  mediaServer: {plex: {appRef: {kind: Plex, name: plex}}}
  radarr: [{name: Radarr, appRef: {kind: Radarr, name: radarr}}]
  seerr: {appRef: {kind: Seerr, name: seerr}}`},
		{name: "a Maintainerr radarr server referring to Sonarr", kind: "Maintainerr", spec: `
  connection: {url: http://maintainerr:6246}
  radarr: [{name: Radarr, appRef: {kind: Sonarr, name: sonarr}}]`,
			rejected: "a radarr server refers to a Radarr"},
		{name: "a download client by reference", kind: "SonarrDownloadClient", spec: `
  sonarrRef: {name: tv}
  name: qBittorrent
  category: tv
  appRef: {kind: QBittorrent, name: qbittorrent}`},
		{name: "a download client by reference and host", kind: "SonarrDownloadClient", spec: `
  sonarrRef: {name: tv}
  name: qBittorrent
  appRef: {kind: QBittorrent, name: qbittorrent}
  host: qbittorrent`,
			rejected: "set appRef, or implementation, protocol, host and port, not both"},
		{name: "a download client referring to Sonarr", kind: "SonarrDownloadClient", spec: `
  sonarrRef: {name: tv}
  name: Sonarr
  appRef: {kind: Sonarr, name: sonarr}`,
			rejected: "a download client refers to a QBittorrent or Sabnzbd"},
		{name: "an Autobrr download client by reference", kind: "Autobrr", spec: conn + `
  downloadClients: [{name: Radarr, appRef: {kind: Radarr, name: radarr}}]`},
		{name: "an Autobrr download client referring to Plex", kind: "Autobrr", spec: conn + `
  downloadClients: [{name: Plex, appRef: {kind: Plex, name: plex}}]`,
			rejected: "a download client refers to a QBittorrent, Sabnzbd, Sonarr, Radarr, Lidarr or Readarr"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			obj := &unstructured.Unstructured{}
			require.NoError(t, yaml.Unmarshal([]byte(fmt.Sprintf(`
apiVersion: arrchitect.dev/v1alpha1
kind: %s
metadata:
  name: rule-%d
  namespace: default
spec:%s`, tt.kind, i, tt.spec)), &obj.Object))
			err := c.Create(context.Background(), obj)
			if tt.rejected == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.rejected)
		})
	}
}

// An object deletes what it created along with itself unless told otherwise,
// as a Kubernetes object owning an external one does.
func TestObjectKindsDefaultToDeletingWhatTheyCreated(t *testing.T) {
	c := startAPIServerWith(t)
	obj := &unstructured.Unstructured{}
	require.NoError(t, yaml.Unmarshal([]byte(`
apiVersion: arrchitect.dev/v1alpha1
kind: SonarrIndexer
metadata:
  name: defaulted
  namespace: default
spec:
  sonarrRef: {name: tv}
  name: x
  protocol: torrent
  implementation: Torznab`), &obj.Object))
	require.NoError(t, c.Create(context.Background(), obj))
	policy, _, err := unstructured.NestedString(obj.Object, "spec", "deletionPolicy")
	require.NoError(t, err)
	assert.Equal(t, "delete", policy)
}
