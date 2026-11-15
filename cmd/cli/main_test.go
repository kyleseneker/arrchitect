package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runCLI(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	err := run(context.Background(), args, &stdout, &stderr)
	return stdout.String(), stderr.String(), err
}

func TestRunRejectsAMissingOrUnknownCommand(t *testing.T) {
	_, stderr, err := runCLI(t)
	require.EqualError(t, err, "no command given")
	assert.Contains(t, stderr, "Usage:")

	_, _, err = runCLI(t, "import")
	require.EqualError(t, err, `unknown command "import"`)
}

func TestVersionPrintsTheBuild(t *testing.T) {
	stdout, _, err := runCLI(t, "version")
	require.NoError(t, err)
	assert.Equal(t, "dev\n", stdout)
}

func TestExportRequiresAnAppURLAndKey(t *testing.T) {
	t.Setenv("ARRCHITECT_API_KEY", "")
	_, _, err := runCLI(t, "export")
	require.EqualError(t, err, "export needs an app")
	_, _, err = runCLI(t, "export", "--url", "http://sonarr")
	require.EqualError(t, err, "export needs an app")
	_, _, err = runCLI(t, "export", "sonarr")
	require.EqualError(t, err, "--url is required")
	_, _, err = runCLI(t, "export", "sonarr", "--url", "http://sonarr")
	require.EqualError(t, err, "--api-key or $ARRCHITECT_API_KEY is required")
	_, _, err = runCLI(t, "export", "sonarr", "--url", "http://sonarr", "--api-key", "k", "extra")
	require.EqualError(t, err, "unexpected arguments: extra")
}

func TestExportRequiresALoginForApps(t *testing.T) {
	t.Setenv("ARRCHITECT_USERNAME", "")
	t.Setenv("ARRCHITECT_PASSWORD", "")
	_, _, err := runCLI(t, "export", "qbittorrent", "--url", "http://qbittorrent", "--username", "admin")
	require.EqualError(t, err,
		"qbittorrent needs --username and --password, or $ARRCHITECT_USERNAME and $ARRCHITECT_PASSWORD")
}

func TestExportRequiresATokenForPlex(t *testing.T) {
	t.Setenv("ARRCHITECT_TOKEN", "")
	_, _, err := runCLI(t, "export", "plex", "--url", "http://plex")
	require.EqualError(t, err, "plex needs --token or $ARRCHITECT_TOKEN")
}

func TestExportPrintsTheSecretAndConfig(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/config/") {
			_, _ = w.Write([]byte(`{}`))
			return
		}
		_, _ = w.Write([]byte(`[]`))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("ARRCHITECT_API_KEY", "from-env")

	stdout, stderr, err := runCLI(t, "export", "sonarr", "--url", srv.URL, "--connection-url", "http://sonarr:8989",
		"--namespace", "media")
	require.NoError(t, err)
	assert.Contains(t, stdout, "kind: Secret")
	assert.Contains(t, stdout, "api-key: from-env")
	assert.Contains(t, stdout, "kind: SonarrConfig")
	assert.Contains(t, stdout, "namespace: media")
	assert.Contains(t, stdout, "driftPolicy: observe", "a first apply must not write to the app")
	assert.Empty(t, stderr, "nothing to fill in when the app has no credentials")

	stdout, stderr, err = runCLI(t, "export", "sonarr", "--url", srv.URL, "--enforce")
	require.NoError(t, err)
	assert.NotContains(t, stdout, "driftPolicy")
	assert.Contains(t, stderr, "pass --connection-url", "the operator can't reach a port-forward")
}

func TestExportWritesTheSecretToItsOwnFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/config/") {
			_, _ = w.Write([]byte(`{}`))
			return
		}
		_, _ = w.Write([]byte(`[]`))
	}))
	t.Cleanup(srv.Close)
	file := filepath.Join(t.TempDir(), "secret.yaml")

	stdout, _, err := runCLI(t, "export", "sonarr", "--url", srv.URL, "--api-key", "k", "--secret-file", file)
	require.NoError(t, err)
	assert.NotContains(t, stdout, "kind: Secret")
	assert.Contains(t, stdout, "kind: SonarrConfig")
	secret, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.Contains(t, string(secret), "api-key: k")
	info, err := os.Stat(file)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestExportExplainsARefusedKeyAndAPageThatIsntTheAPI(t *testing.T) {
	refused := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(refused.Close)
	_, _, err := runCLI(t, "export", "sonarr", "--url", refused.URL, "--api-key", "wrong")
	require.ErrorContains(t, err, "the app refused the credentials (status 401); check --api-key")

	login := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("<html>Sign in</html>"))
	}))
	t.Cleanup(login.Close)
	_, _, err = runCLI(t, "export", "sonarr", "--url", login.URL, "--api-key", "k")
	require.ErrorContains(t, err, "the answer wasn't the app's API")
}
