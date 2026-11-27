//go:build compat

package compat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	downloadsv1alpha1 "github.com/kyleseneker/arrchitect/api/downloads/v1alpha1"
	ctrlcommon "github.com/kyleseneker/arrchitect/internal/controller/common"
	"github.com/kyleseneker/arrchitect/internal/controller/downloads"
	"github.com/kyleseneker/arrchitect/internal/httpapi"
)

func TestSabnzbd(t *testing.T) {
	t.Parallel()
	sab := startContainer(t, containerSpec{name: "sabnzbd", image: sabnzbdImage, port: 8080, env: linuxserverEnv})
	var key string
	waitFor(t, "sabnzbd api key", 2*time.Minute, func() error {
		out, err := sab.run(`sed -n 's/^api_key = //p' /config/sabnzbd.ini`)
		if err != nil || out == "" {
			return fmt.Errorf("api_key not written yet: %v", err)
		}
		key = out
		return nil
	})
	sab.exec(t, "mkdir -p /downloads/complete /downloads/incomplete /downloads/nzb-backup && chown -R abc:abc /downloads")
	sab.exec(t, `mkdir -p /config/scripts &&
		for s in check.sh done.sh; do printf '#!/bin/sh\nexit 0\n' > /config/scripts/$s; done &&
		chmod +x /config/scripts/*.sh && chown -R abc:abc /config/scripts`)
	waitFor(t, "sabnzbd", 2*time.Minute, func() error {
		_, err := sabnzbdGet(sab.url(), key, "version")
		return err
	})
	proxy := newRecordingProxy(t, sab.url())

	checkCompat(t, compatCase{
		fixture: "sabnzbd.yaml",
		update: `[
			{"op": "replace", "path": "/spec/servers/0/connections", "value": 25},
			{"op": "replace", "path": "/spec/categories/1/priority", "value": "normal"},
			{"op": "replace", "path": "/spec/apprise/events", "value": ["complete"]},
			{"op": "replace", "path": "/spec/quota/size", "value": "1T"}
		]`,
		verifyUpdate: func(t *testing.T) {
			assert.Equal(t, "1T", sabnzbdConfigOf(t, sab.url(), key).Misc["quota_size"])
		},
		export: "sabnzbd",
		vars:   map[string]string{"URL": proxy.url()},
		secrets: map[string]string{
			"api-key": key, "usenet-username": "compat", "usenet-password": "usenet-secret",
			"apprise-urls": "discord://compat/token", "sab-username": "compat-admin", "sab-password": "sab-secret",
		},
		newObj: func() ctrlcommon.ConfigResource { return &downloadsv1alpha1.Sabnzbd{} },
		reconciler: func(c client.Client) reconcile.Reconciler {
			return &downloads.SabnzbdReconciler{Client: c, Recorder: events.NewFakeRecorder(100)}
		},
		proxy: proxy,
		// Every call is a form POST to /api; the mode says whether it writes.
		read: func(r request) bool {
			values, _ := url.ParseQuery(r.Body)
			mode := values.Get("mode")
			return getOnly(r) || mode == "version" || mode == "get_config"
		},
		observe: true,
		verify: func(t *testing.T) {
			cfg := sabnzbdConfigOf(t, sab.url(), key)
			assert.Equal(t, "/downloads/complete", cfg.Misc["complete_dir"])
			assert.Equal(t, "50M", cfg.Misc["bandwidth_max"])
			assert.Equal(t, "avg_age asc", cfg.Misc["auto_sort"], "autoSort did not set a sort order SABnzbd uses")
			require.Len(t, cfg.Servers, 1)
			assert.Equal(t, "news.example.com", cfg.Servers[0]["host"])
			assert.InDelta(t, 2, cfg.Servers[0]["ssl_verify"], 0)
			assert.InDelta(t, 30, cfg.Servers[0]["timeout"], 0)
			assert.Equal(t, []any{"exe", "lnk"}, cfg.Misc["unwanted_extensions"])
			assert.InDelta(t, 2, cfg.Misc["pause_on_pwrar"], 0)
			assert.Equal(t, "days-delete", cfg.Misc["history_retention_option"])
			assert.Equal(t, "775", cfg.Misc["permissions"])
			assert.Equal(t, []any{"sabnzbd.media"}, cfg.Misc["host_whitelist"])
			assert.InDelta(t, 4, cfg.Misc["no_dupes"], 0)
			assert.InDelta(t, 2, cfg.Misc["no_smart_dupes"], 0)
			assert.Equal(t, "check.sh", cfg.Misc["pre_script"], "SABnzbd refused the pre-queue script")
			assert.Equal(t, "done.sh", cfg.Misc["end_queue_script"])
			assert.Equal(t, "500G", cfg.Misc["quota_size"])
			assert.Equal(t, "w", cfg.Misc["quota_period"])
			assert.Equal(t, "1 03:00", cfg.Misc["quota_day"])
			assert.Equal(t, "compat-admin", cfg.Misc["username"])
			assert.Equal(t, "**********", cfg.Misc["password"], "the login password wasn't set")
			assert.Equal(t, "AES128", cfg.Servers[0]["ssl_ciphers"])
			for _, c := range cfg.Categories {
				if c["name"] == "tv" {
					assert.Equal(t, "2", c["pp"])
				}
			}
			names := make([]any, 0, len(cfg.Categories))
			for _, c := range cfg.Categories {
				names = append(names, c["name"])
			}
			assert.Subset(t, names, []any{"tv", "movies"})
		},
		cleanup: func(t *testing.T, _ map[string][]string) {
			cfg := sabnzbdConfigOf(t, sab.url(), key)
			assert.Empty(t, cfg.Servers, "servers the operator created are still there")
			for _, c := range cfg.Categories {
				assert.NotContains(t, []any{"tv", "movies"}, c["name"], "a category the operator created is still there")
			}
		},
	})
}

type sabnzbdSections struct {
	Misc       map[string]any   `json:"misc"`
	Servers    []map[string]any `json:"servers"`
	Categories []map[string]any `json:"categories"`
}

func sabnzbdConfigOf(t *testing.T, base, key string) sabnzbdSections {
	t.Helper()
	raw, err := sabnzbdGet(base, key, "get_config")
	require.NoError(t, err)
	var out struct {
		Config sabnzbdSections `json:"config"`
	}
	require.NoError(t, json.Unmarshal(raw, &out))
	return out.Config
}

// sabnzbdGet calls the API directly, outside the operator.
func sabnzbdGet(base, key, mode string) ([]byte, error) {
	resp, err := http.Get(base + "/api?output=json&apikey=" + key + "&mode=" + mode)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK || bytes.Contains(body, []byte(`"error"`)) {
		return nil, fmt.Errorf("%s: %d %s", mode, resp.StatusCode, body)
	}
	return body, nil
}

func TestQBittorrent(t *testing.T) {
	t.Parallel()
	qbit, password := startQBittorrent(t)
	proxy := newRecordingProxy(t, qbit.url())

	checkCompat(t, compatCase{
		fixture: "qbittorrent.yaml",
		update: `[
			{"op": "replace", "path": "/spec/categories/0/savePath", "value": "/downloads/complete/series"},
			{"op": "replace", "path": "/spec/preferences/bannedIps", "value": ["10.0.0.3"]},
			{"op": "replace", "path": "/spec/rssRules/0/mustContain", "value": "Compat Show 2160p"}
		]`,
		verifyUpdate: func(t *testing.T) {
			hc := qbittorrentClient(t, qbit.url(), password)
			prefs, err := hc.GetJSON(context.Background(), "/api/v2/app/preferences")
			require.NoError(t, err)
			assert.Equal(t, "10.0.0.3", prefs["banned_IPs"])
			rules, err := hc.GetJSON(context.Background(), "/api/v2/rss/rules")
			require.NoError(t, err)
			rule, _ := rules["Compat Show"].(map[string]any)
			assert.Equal(t, "Compat Show 2160p", rule["mustContain"])
			assert.Equal(t, "x265", rule["mustNotContain"], "the update kept the fields it didn't change")
			categories, err := hc.GetJSON(context.Background(), "/api/v2/torrents/categories")
			require.NoError(t, err)
			tv, _ := categories["tv"].(map[string]any)
			assert.Equal(t, "/downloads/complete/series", tv["savePath"])
			assert.Equal(t, "/downloads/incomplete/tv", tv["download_path"],
				"changing the save path kept the download path set in qBittorrent")
		},
		export: "qbittorrent",
		vars:   map[string]string{"URL": proxy.url()},
		secrets: map[string]string{
			"username": qbittorrentUser, "password": password,
			"proxy-username": "compat-proxy", "proxy-password": "proxy-secret",
		},
		newObj: func() ctrlcommon.ConfigResource { return &downloadsv1alpha1.QBittorrent{} },
		reconciler: func(c client.Client) reconcile.Reconciler {
			return &downloads.QBittorrentReconciler{Client: c, Recorder: events.NewFakeRecorder(100)}
		},
		proxy: proxy,
		// The WebUI authenticates with a POST; it changes no settings.
		read: func(r request) bool {
			return getOnly(r) || (r.Method == http.MethodPost && r.Path == "/api/v2/auth/login")
		},
		observe: true,
		verify: func(t *testing.T) {
			hc := qbittorrentClient(t, qbit.url(), password)
			prefs, err := hc.GetJSON(context.Background(), "/api/v2/app/preferences")
			require.NoError(t, err)
			assert.Equal(t, 2.5, prefs["max_ratio"])
			assert.Equal(t, false, prefs["dht"])
			assert.Equal(t, "/downloads/complete", prefs["save_path"])
			assert.Equal(t, "/downloads/incomplete", prefs["temp_path"])
			assert.InDelta(t, 10485760, prefs["up_limit"], 0, "KiB/s should be written as bytes per second")
			assert.InDelta(t, 1, prefs["scheduler_days"], 0)
			assert.Equal(t, "*.lnk\n*.exe", prefs["excluded_file_names"])
			assert.Equal(t, "eth0", prefs["current_network_interface"])
			assert.Equal(t, true, prefs["auto_tmm_enabled"])
			assert.Equal(t, "Subfolder", prefs["torrent_content_layout"])
			assert.InDelta(t, 1440, prefs["max_inactive_seeding_time"], 0)
			assert.Equal(t, "SOCKS5", prefs["proxy_type"])
			assert.Equal(t, "compat-proxy", prefs["proxy_username"])
			assert.Equal(t, "proxy-secret", prefs["proxy_password"])
			assert.Equal(t, "10.0.0.2\n2001:db8::1", prefs["banned_IPs"])
			assert.Equal(t, `echo "%N"`, prefs["autorun_program"])
			assert.Equal(t, "udp://tracker.example.invalid:1337/announce\n\nhttps://tracker.example.invalid/announce",
				prefs["add_trackers"])
			categories, err := hc.GetJSON(context.Background(), "/api/v2/torrents/categories")
			require.NoError(t, err)
			assert.Contains(t, categories, "movies")
			_, err = hc.DoForm(context.Background(), "/api/v2/torrents/editCategory", url.Values{
				"category": {"tv"}, "savePath": {"/downloads/complete/tv"},
				"downloadPathEnabled": {"true"}, "downloadPath": {"/downloads/incomplete/tv"},
			})
			require.NoError(t, err, "giving tv a download path, as a user would in qBittorrent")
			assert.InDelta(t, 45, prefs["rss_refresh_interval"], 0)
			assert.Equal(t, true, prefs["rss_processing_enabled"])
			assert.Equal(t, "s(\\d+)e(\\d+)\n(\\d+)x(\\d+)", prefs["rss_smart_episode_filters"])
			assert.Equal(t, map[string]string{
				`Compat\Shows`: "https://shows.example.invalid/rss", "Movies": "https://movies.example.invalid/rss",
			}, qbittorrentFeeds(t, hc))
			rules, err := hc.GetJSON(context.Background(), "/api/v2/rss/rules")
			require.NoError(t, err)
			rule, _ := rules["Compat Show"].(map[string]any)
			require.NotNil(t, rule, "the RSS rule is missing")
			assert.Equal(t, []any{"https://shows.example.invalid/rss"}, rule["affectedFeeds"])
			assert.Equal(t, true, rule["smartFilter"])
			params, _ := rule["torrentParams"].(map[string]any)
			assert.Equal(t, "tv", params["category"])
			assert.Equal(t, []any{"rss"}, params["tags"])
			assert.Equal(t, "Subfolder", params["content_layout"])
		},
		cleanup: func(t *testing.T, _ map[string][]string) {
			hc := qbittorrentClient(t, qbit.url(), password)
			categories, err := hc.GetJSON(context.Background(), "/api/v2/torrents/categories")
			require.NoError(t, err)
			assert.Empty(t, categories, "categories the operator created are still there")
			assert.Empty(t, qbittorrentFeeds(t, hc), "RSS feeds the operator created are still there")
			rules, err := hc.GetJSON(context.Background(), "/api/v2/rss/rules")
			require.NoError(t, err)
			assert.Empty(t, rules, "RSS rules the operator created are still there")
		},
	})
}

// qbittorrentFeeds lists qBittorrent's RSS feeds by path.
func qbittorrentFeeds(t *testing.T, hc *httpapi.HTTPClient) map[string]string {
	t.Helper()
	root, err := hc.GetJSON(context.Background(), "/api/v2/rss/items")
	require.NoError(t, err)
	feeds := map[string]string{}
	var walk func(prefix string, items map[string]any)
	walk = func(prefix string, items map[string]any) {
		for name, value := range items {
			item, _ := value.(map[string]any)
			if u, ok := item["url"].(string); ok {
				feeds[prefix+name] = u
				continue
			}
			walk(prefix+name+`\`, item)
		}
	}
	walk("", root)
	return feeds
}

// qbittorrentClient logs in to the WebUI directly for verification.
func qbittorrentClient(t *testing.T, base, password string) *httpapi.HTTPClient {
	t.Helper()
	hc, err := httpapi.NewHTTPClient(base, httpapi.AuthNone, httpapi.WithAppLabel("compat"))
	require.NoError(t, err)
	_, err = hc.DoForm(context.Background(), "/api/v2/auth/login",
		url.Values{"username": {qbittorrentUser}, "password": {password}})
	require.NoError(t, err)
	name, value := hc.CookieValuePrefix("QBT_SID_")
	if value == "" {
		name, value = "SID", hc.CookieValue("SID")
	}
	hc.SetCookieSession(name, value)
	return hc
}
