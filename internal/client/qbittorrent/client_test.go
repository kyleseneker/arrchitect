package qbittorrent

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kyleseneker/arrchitect/internal/engine"
)

func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	hc, err := engine.NewHTTPClient(srv.URL, engine.AuthCookie,
		engine.WithTransport(&http.Transport{}),
		engine.WithDisableRedirect(),
	)
	require.NoError(t, err)
	return NewClient(hc, "admin", "adminadmin")
}

func TestLogin_Success(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v2/auth/login", r.URL.Path)
		body, _ := io.ReadAll(r.Body)
		assert.Contains(t, string(body), "username=admin")
		// Set cookie with Path=/ so the jar captures it for the httptest host
		http.SetCookie(w, &http.Cookie{Name: "SID", Value: "test-sid", Path: "/"})
		_, _ = w.Write([]byte("Ok."))
	})
	err := c.Login(context.Background())
	require.NoError(t, err)
}

func TestLogin_Failure(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("Fails."))
	})
	err := c.Login(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "login failed")
}

func TestLoginRefusedNamesTheHostHeaderPort(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	err := c.Login(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "the URL's port is qBittorrent's own")
	apiErr, ok := errors.AsType[*engine.APIError](err)
	require.True(t, ok)
	assert.Equal(t, http.StatusUnauthorized, apiErr.StatusCode)
}

func TestPing(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v2/app/version", r.URL.Path)
		_, _ = w.Write([]byte("v4.6.0"))
	})
	assert.NoError(t, c.Ping(context.Background()))
}

func TestGetPreferences(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v2/app/preferences", r.URL.Path)
		_ = json.NewEncoder(w).Encode(map[string]any{"save_path": "/downloads"})
	})
	prefs, err := c.GetPreferences(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "/downloads", prefs["save_path"])
}

func TestSetPreferences(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v2/app/setPreferences", r.URL.Path)
		body, _ := io.ReadAll(r.Body)
		assert.Contains(t, string(body), "json=")
		w.WriteHeader(http.StatusOK)
	})
	err := c.SetPreferences(context.Background(), map[string]any{"save_path": "/new"})
	assert.NoError(t, err)
}

func TestListCategories(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v2/torrents/categories", r.URL.Path)
		_ = json.NewEncoder(w).Encode(map[string]any{"tv": map[string]any{"savePath": "/tv"}})
	})
	cats, err := c.ListCategories(context.Background())
	require.NoError(t, err)
	assert.Contains(t, cats, "tv")
}

func TestCreateCategory(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v2/torrents/createCategory", r.URL.Path)
		body, _ := io.ReadAll(r.Body)
		assert.Contains(t, string(body), "category=movies")
		w.WriteHeader(http.StatusOK)
	})
	assert.NoError(t, c.CreateCategory(context.Background(), "movies", "/movies"))
}

func TestEditCategory(t *testing.T) {
	for _, tc := range []struct {
		name         string
		current      map[string]any
		wantEnabled  []string
		wantDownload []string
	}{
		{name: "default download path, as 5.1 lists it", current: map[string]any{"savePath": "/movies"}},
		{name: "default download path, as 5.3 lists it", current: map[string]any{"download_path": nil}},
		{name: "download path turned off", current: map[string]any{"download_path": false}, wantEnabled: []string{"false"}},
		{
			name:         "download path set",
			current:      map[string]any{"download_path": "/incomplete/movies"},
			wantEnabled:  []string{"true"},
			wantDownload: []string{"/incomplete/movies"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/api/v2/torrents/editCategory", r.URL.Path)
				assert.NoError(t, r.ParseForm())
				assert.Equal(t, "movies", r.PostForm.Get("category"))
				assert.Equal(t, "/new-movies", r.PostForm.Get("savePath"))
				assert.Equal(t, tc.wantEnabled, r.PostForm["downloadPathEnabled"])
				assert.Equal(t, tc.wantDownload, r.PostForm["downloadPath"])
				w.WriteHeader(http.StatusOK)
			})
			assert.NoError(t, c.EditCategory(context.Background(), "movies", "/new-movies", tc.current))
		})
	}
}

func TestLogin_NoContentWithPortSuffixedCookie(t *testing.T) {
	var sawCookie string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/auth/login" {
			http.SetCookie(w, &http.Cookie{Name: "QBT_SID_8080", Value: "abc123", Path: "/"})
			w.WriteHeader(http.StatusNoContent)
			return
		}
		for _, ck := range r.Cookies() {
			if ck.Name == "QBT_SID_8080" {
				sawCookie = ck.Value
			}
		}
		_, _ = w.Write([]byte(`{}`))
	})

	require.NoError(t, c.Login(context.Background()))
	require.NoError(t, c.Ping(context.Background()))
	assert.Equal(t, "abc123", sawCookie, "session cookie must be sent under the name qBittorrent issued")
}

func TestLogin_NoContentWithoutCookie(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	require.Error(t, c.Login(context.Background()))
}

// With web_ui_secure_cookie_enabled, qBittorrent marks its session cookie
// Secure even when the Web UI is served over HTTP, as it is in-cluster.
func TestLoginKeepsASecureSessionCookieOverHTTP(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			http.SetCookie(w, &http.Cookie{Name: "SID", Value: "abc", Path: "/", Secure: true, HttpOnly: true})
			_, _ = w.Write([]byte("Ok."))
		default:
			if cookie, err := r.Cookie("SID"); err != nil || cookie.Value != "abc" {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			_, _ = w.Write([]byte("{}"))
		}
	})
	require.NoError(t, c.Login(context.Background()))
	_, err := c.GetPreferences(context.Background())
	assert.NoError(t, err, "the session cookie was dropped")
}
