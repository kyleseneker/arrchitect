package export

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	downloadsv1alpha1 "github.com/kyleseneker/arrchitect/api/downloads/v1alpha1"
)

func TestQBittorrentExportsPreferencesCategoriesAndLogin(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			require.NoError(t, r.ParseForm())
			if r.Form.Get("username") != "admin" || r.Form.Get("password") != "secret" {
				_, _ = w.Write([]byte("Fails."))
				return
			}
			http.SetCookie(w, &http.Cookie{Name: "SID", Value: "session", Path: "/"})
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/app/preferences":
			_, _ = w.Write([]byte(`{"save_path":"/downloads/","dht":false,"max_connec":400,"max_ratio":2.5,` +
				`"max_ratio_act":3,"web_ui_port":8080,"bypass_auth_subnet_whitelist":"","not_in_the_crd":1,` +
				`"temp_path":"/downloads/incomplete","dl_limit":10240,"up_limit":1000,"scheduler_days":2,` +
				`"excluded_file_names":"*.lnk\n*.exe","current_network_interface":"","auto_tmm_enabled":true,` +
				`"proxy_type":"SOCKS5","proxy_ip":"vpn","proxy_port":1080,"proxy_username":"vpn-user","proxy_password":"vpn-pass",` +
				`"banned_IPs":"10.0.0.2\n2001:db8::1","add_trackers":"udp://a.example/announce\nudp://b.example/announce",` +
				`"autorun_program":"","ip_filter_path":""}`))
		case "/api/v2/torrents/categories":
			_, _ = w.Write([]byte(`{"tv":{"name":"tv","savePath":"/downloads/tv"},"movies":{"name":"movies","savePath":""}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	res, err := Run(context.Background(), Options{App: "qbittorrent", URL: srv.URL, Username: "admin", Password: "secret"})
	require.NoError(t, err)
	cfg, ok := res.Config.(*downloadsv1alpha1.QBittorrent)
	require.True(t, ok)
	prefs := cfg.Spec.Preferences
	assert.Equal(t, "/downloads/", prefs.SavePath)
	assert.False(t, *prefs.DHT)
	assert.Equal(t, int32(400), *prefs.MaxConnec)
	assert.Equal(t, "2.5", *prefs.MaxRatio)
	assert.Equal(t, "removeWithContent", prefs.MaxRatioAction)
	assert.Equal(t, []downloadsv1alpha1.QBittorrentCategory{{Name: "movies"}, {Name: "tv", SavePath: "/downloads/tv"}}, cfg.Spec.Categories)
	assert.Equal(t, "/downloads/incomplete", prefs.TempPath)
	assert.Equal(t, int32(10), *prefs.DlLimit, "qBittorrent stores bytes per second, the config takes KiB/s")
	assert.Equal(t, int32(0), *prefs.UpLimit)
	assert.Contains(t, res.Warnings,
		"qBittorrent's upLimit is 1000 bytes per second, which isn't a whole number of KiB/s; exported rounded down")
	assert.Equal(t, "weekends", prefs.SchedulerDays)
	assert.Equal(t, []string{"*.lnk", "*.exe"}, prefs.ExcludedFileNames)
	assert.Equal(t, "", *prefs.NetworkInterface, "an empty interface means any and must survive the export")
	assert.True(t, *prefs.AutoTMMEnabled)
	assert.Equal(t, "SOCKS5", prefs.ProxyType)
	assert.Equal(t, int32(1080), prefs.ProxyPort)
	assert.Equal(t, "proxy-username", prefs.ProxyUsernameSecretRef.Key)
	assert.Equal(t, "proxy-password", prefs.ProxyPasswordSecretRef.Key)
	assert.Equal(t, []string{"10.0.0.2", "2001:db8::1"}, prefs.BannedIPs)
	assert.Equal(t, []string{"udp://a.example/announce", "udp://b.example/announce"}, prefs.AddTrackers)
	assert.Contains(t, res.Warnings, "qBittorrent's added trackers aren't each in their own tier, which is how the config writes them")
	assert.Empty(t, prefs.AutorunProgram)

	assert.Equal(t, "username", cfg.Spec.Connection.UsernameSecretRef.Key)
	assert.Equal(t, "password", cfg.Spec.Connection.PasswordSecretRef.Key)
	assert.Equal(t, map[string]string{
		"username": "admin", "password": "secret", "proxy-username": "vpn-user", "proxy-password": "vpn-pass",
	}, res.Secret.StringData)
	assert.Empty(t, res.Unfilled)
	assert.True(t, NeedsLogin("qbittorrent"))
	assert.False(t, NeedsAPIKey("qbittorrent"))
}
