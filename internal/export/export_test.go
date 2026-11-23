package export

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	pvrv1alpha1 "github.com/kyleseneker/arrchitect/api/pvr/v1alpha1"
)

func TestRunRejectsAnUnsupportedApp(t *testing.T) {
	_, err := Run(context.Background(), Options{App: "nope", URL: "http://x"})
	require.ErrorContains(t, err, `unsupported app "nope"; supported: autobrr, bazarr, flaresolverr, jellyfin, lidarr, maintainerr, plex, prowlarr, qbittorrent, radarr, readarr, sabnzbd, seerr, sonarr, tdarr`)
}

func TestRunRejectsAnExcludeTheSpecLacks(t *testing.T) {
	_, err := Run(context.Background(), Options{App: "sonarr", URL: "http://x", Exclude: []string{"indexer"}})
	require.ErrorContains(t, err, `cannot exclude "indexer"`)
}

func TestRunNamesAndPointsTheConfig(t *testing.T) {
	cfg, res := exportSonarr(t, Options{Name: "tv", Namespace: "media", ConnectionURL: "http://sonarr.media:8989"})
	assert.Equal(t, "tv", cfg.Name)
	assert.Equal(t, "media", cfg.Namespace)
	assert.Equal(t, "Sonarr", cfg.Kind)
	assert.Equal(t, "http://sonarr.media:8989", cfg.Spec.Connection.URL)
	assert.Equal(t, "tv-credentials", cfg.Spec.Connection.APIKeySecretRef.Name)
	assert.Equal(t, "tv-credentials", res.Secret.Name)
	assert.Equal(t, "media", res.Secret.Namespace)
}

func TestRunExcludeDropsTheSectionAndItsCredentials(t *testing.T) {
	cfg, res := exportSonarr(t, Options{Exclude: []string{"indexers", "importLists"}})
	assert.Empty(t, objectsOf[*pvrv1alpha1.SonarrIndexer](res))
	assert.Empty(t, cfg.Spec.ImportLists)
	assert.NotContains(t, res.Secret.StringData, "indexer-torznab-apikey")
	assert.Equal(t, []string{"download-client-qbit-password"}, res.Unfilled)
}

// An empty value would reach the app on the first write and wipe the
// credential; a missing key is reported instead.
func TestRunLeavesTheCredentialsTheAppHidesOutOfTheSecret(t *testing.T) {
	cfg, res := exportSonarr(t, Options{})
	assert.Contains(t, res.Unfilled, "download-client-qbit-password")
	assert.NotContains(t, res.Secret.StringData, "download-client-qbit-password")
	require.NotNil(t, cfg.Spec.Reconcile)
	assert.Equal(t, "observe", cfg.Spec.Reconcile.DriftPolicy, "a first apply must not write to the app")

	cfg, _ = exportSonarr(t, Options{Enforce: true})
	assert.Nil(t, cfg.Spec.Reconcile)
}

func TestRunSplitsObjectsWithKindsOfTheirOwn(t *testing.T) {
	cfg, res := exportSonarr(t, Options{Name: "tv", Namespace: "media"})
	indexers := objectsOf[*pvrv1alpha1.SonarrIndexer](res)
	require.Len(t, indexers, 1)
	assert.Equal(t, "tv-torznab", indexers[0].Name)
	assert.Equal(t, "media", indexers[0].Namespace)
	assert.Equal(t, "SonarrIndexer", indexers[0].Kind)
	assert.Equal(t, "tv", indexers[0].Spec.SonarrRef.Name)
	assert.Equal(t, "Torznab", indexers[0].Spec.Name)
	clients := objectsOf[*pvrv1alpha1.SonarrDownloadClient](res)
	require.Len(t, clients, 1)
	assert.Equal(t, "tv-qbit", clients[0].Name)
	assert.Equal(t, "tv", clients[0].Spec.SonarrRef.Name)
	assert.Equal(t, "tv-credentials", clients[0].Spec.PasswordSecretRef.Name)

	raw, err := json.Marshal(cfg.Spec)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "indexers")
	assert.NotContains(t, string(raw), "downloadClients")
}

func TestRunExplainsARefusedRequestForTheCredentialsUsed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)

	_, err := Run(context.Background(), Options{App: "qbittorrent", URL: srv.URL, Username: "u", Password: "p"})
	require.ErrorContains(t, err, "the app refused the request (status 401); check --username and --password, "+
		"and that --url's port is the app's own")
	_, err = Run(context.Background(), Options{App: "jellyfin", URL: srv.URL, APIKey: "k"})
	require.ErrorContains(t, err, "the app refused the credentials (status 401); check --api-key")
	_, err = Run(context.Background(), Options{App: "jellyfin", URL: srv.URL, Username: "u", Password: "p"})
	require.ErrorContains(t, err, "the app refused the credentials (status 401); check --username and --password")
}

func TestObjectNameIsAValidResourceName(t *testing.T) {
	assert.Equal(t, "tv-nzbgeek-prowlarr", objectName("tv", "NZBgeek (Prowlarr)", 0))
	assert.Equal(t, "tv-3", objectName("tv", "!!!", 2))
	assert.Len(t, objectName("tv", strings.Repeat("a", 300), 0), 253)
}

func TestResultYAMLHasNoServerFields(t *testing.T) {
	_, res := exportSonarr(t, Options{})
	out, err := res.YAML()
	require.NoError(t, err)
	docs := strings.Split(string(out), "---\n")
	require.Len(t, docs, 4)
	assert.Contains(t, docs[0], "kind: Secret")
	assert.Contains(t, docs[1], "kind: Sonarr")
	assert.Contains(t, docs[2], "kind: SonarrDownloadClient")
	assert.Contains(t, docs[3], "kind: SonarrIndexer")
	assert.NotContains(t, string(out), "creationTimestamp")
	assert.NotContains(t, string(out), "status:")
}

func TestCredentialKeysAreValidAndUnique(t *testing.T) {
	c := &credentials{secret: "s", values: map[string]string{}}
	assert.Equal(t, "download-client-my-qbit-password", c.ref("", "download-client", "My qBit!", "password")["key"])
	assert.Equal(t, "download-client-my-qbit-password-2", c.ref("", "download-client", "my qbit", "password")["key"])
}

func fieldNamed(t *testing.T, fields []commonv1alpha1.ConfigField, name string) commonv1alpha1.ConfigField {
	t.Helper()
	for _, f := range fields {
		if f.Name == name {
			return f
		}
	}
	require.FailNow(t, "no field "+name)
	return commonv1alpha1.ConfigField{}
}

func fieldNames(fields []commonv1alpha1.ConfigField) []string {
	names := make([]string, 0, len(fields))
	for _, f := range fields {
		names = append(names, f.Name)
	}
	return names
}
