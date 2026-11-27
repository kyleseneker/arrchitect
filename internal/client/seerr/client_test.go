package seerr

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kyleseneker/arrchitect/internal/httpapi"
)

func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	hc, err := httpapi.NewHTTPClient(srv.URL, httpapi.AuthSession,
		httpapi.WithTransport(&http.Transport{}),
	)
	require.NoError(t, err)
	return NewClient(hc)
}

func TestIsInitialized(t *testing.T) {
	tests := []struct {
		name string
		body map[string]any
		want bool
	}{
		{"initialized", map[string]any{"initialized": true}, true},
		{"not initialized", map[string]any{"initialized": false}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(tt.body)
			})
			result, err := c.IsInitialized(context.Background())
			require.NoError(t, err)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestAuthenticatePlex(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/auth/plex", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		w.WriteHeader(http.StatusOK)
	})
	assert.NoError(t, c.AuthenticatePlex(context.Background(), "plex-token"))
}

// The first sign-in configures Jellyfin; repeating the server on a later one
// is rejected with "Jellyfin hostname already configured".
func TestAuthenticateJellyfin(t *testing.T) {
	for _, tt := range []struct {
		name       string
		configured bool
		wantServer bool
	}{
		{"first sign-in carries the server", false, true},
		{"later sign-ins carry only credentials", true, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var body map[string]any
			c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/api/v1/auth/jellyfin", r.URL.Path)
				assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				w.WriteHeader(http.StatusOK)
			})
			require.NoError(t, c.AuthenticateJellyfin(context.Background(), "admin", "pass", "jf.local", 8096, "", false, tt.configured))
			assert.Equal(t, "admin", body["username"])
			if tt.wantServer {
				assert.Equal(t, "jf.local", body["hostname"])
			} else {
				assert.NotContains(t, body, "hostname")
				assert.NotContains(t, body, "serverType")
			}
		})
	}
}

func TestMediaServerConfigured(t *testing.T) {
	for raw, want := range map[string]bool{`{"mediaServerType":4}`: false, `{"mediaServerType":2}`: true, `{}`: false} {
		c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(raw))
		})
		got, err := c.MediaServerConfigured(context.Background())
		require.NoError(t, err)
		assert.Equal(t, want, got, raw)
	}
}

func TestGetAPIKey(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/settings/main", r.URL.Path)
		_ = json.NewEncoder(w).Encode(map[string]any{"apiKey": "seerr-key-123"})
	})
	key, err := c.GetAPIKey(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "seerr-key-123", key)
}

func TestGetAPIKey_Missing(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{})
	})
	_, err := c.GetAPIKey(context.Background())
	assert.Error(t, err)
}

// Each method names the endpoint it reads or writes, so the controller and
// export hold no paths of their own.
func TestMethodsReachTheirEndpoints(t *testing.T) {
	body := map[string]any{"name": "x"}
	for _, tc := range []struct {
		method, path string
		list         bool
		call         func(ctx context.Context, c *Client) error
	}{
		{http.MethodGet, "/api/v1/settings/network", false, func(ctx context.Context, c *Client) error { _, err := c.Settings(ctx, "network"); return err }},
		{http.MethodPost, "/api/v1/settings/plex", false, func(ctx context.Context, c *Client) error { return c.SaveSettings(ctx, "plex", body) }},
		{http.MethodGet, "/api/v1/settings/metadatas", false, func(ctx context.Context, c *Client) error { _, err := c.MetadataSettings(ctx); return err }},
		{http.MethodPut, "/api/v1/settings/metadatas", false, func(ctx context.Context, c *Client) error { return c.SaveMetadataSettings(ctx, body) }},
		{http.MethodGet, "/api/v1/settings/notifications/discord", false, func(ctx context.Context, c *Client) error {
			_, err := c.NotificationSettings(ctx, "discord")
			return err
		}},
		{http.MethodPost, "/api/v1/settings/notifications/discord", false, func(ctx context.Context, c *Client) error {
			return c.SaveNotificationSettings(ctx, "discord", body)
		}},
		{http.MethodPost, "/api/v1/settings/initialize", false, func(ctx context.Context, c *Client) error { return c.Initialize(ctx) }},
		{http.MethodGet, "/api/v1/settings/sonarr", true, func(ctx context.Context, c *Client) error { _, err := c.Services(ctx, "sonarr"); return err }},
		{http.MethodPost, "/api/v1/settings/radarr", false, func(ctx context.Context, c *Client) error { return c.CreateService(ctx, "radarr", body) }},
		{http.MethodPut, "/api/v1/settings/sonarr/3", false, func(ctx context.Context, c *Client) error { return c.UpdateService(ctx, "sonarr", 3, body) }},
		{http.MethodDelete, "/api/v1/settings/radarr/4", false, func(ctx context.Context, c *Client) error { return c.DeleteService(ctx, "radarr", 4) }},
		{http.MethodGet, "/api/v1/settings/jellyfin/library", true, func(ctx context.Context, c *Client) error {
			_, err := c.Libraries(ctx, "jellyfin")
			return err
		}},
		{http.MethodPost, "/api/v1/settings/plex/library/sync", true, func(ctx context.Context, c *Client) error {
			_, err := c.SyncLibraries(ctx, "plex")
			return err
		}},
		{http.MethodPut, "/api/v1/settings/jellyfin/library/abc", false, func(ctx context.Context, c *Client) error {
			return c.EnableLibrary(ctx, "jellyfin", "abc", true)
		}},
		{http.MethodGet, "/api/v1/settings/jobs", true, func(ctx context.Context, c *Client) error { _, err := c.Jobs(ctx); return err }},
		{http.MethodPost, "/api/v1/settings/jobs/plex-sync/schedule", false, func(ctx context.Context, c *Client) error {
			return c.ScheduleJob(ctx, "plex-sync", "0 0 * * *")
		}},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, tc.method, r.Method)
				assert.Equal(t, tc.path, r.URL.Path)
				if tc.list {
					_, _ = w.Write([]byte(`[]`))
					return
				}
				_, _ = w.Write([]byte(`{}`))
			})
			require.NoError(t, tc.call(context.Background(), c))
		})
	}
}

func TestPing(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/settings/public", r.URL.Path)
		w.WriteHeader(http.StatusOK)
	})
	assert.NoError(t, c.Ping(context.Background()))
}

func TestAuthenticateJellyfinPayload(t *testing.T) {
	var body map[string]any
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":1}`))
	})

	require.NoError(t, c.AuthenticateJellyfin(context.Background(), "admin", "pw", "jf.svc", 8096, "", false, false))

	assert.Equal(t, "admin", body["username"])
	assert.Equal(t, "jf.svc", body["hostname"])
	assert.Equal(t, float64(8096), body["port"])
	assert.Equal(t, float64(2), body["serverType"])
	assert.Contains(t, body, "urlBase", "urlBase must be sent; Seerr concatenates it and builds host:port+undefined without it")
	assert.Equal(t, false, body["useSsl"])
}
