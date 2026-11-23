package qbittorrent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/kyleseneker/arrchitect/internal/engine"
	"github.com/kyleseneker/arrchitect/internal/reconciler"
)

// Client is an HTTP client for the qBittorrent WebUI API v2.
// It authenticates via session cookies obtained from the login endpoint.
type Client struct {
	hc       *engine.HTTPClient
	username string
	password string
}

// MaxRatioActions maps the spec's maxRatioAction names to qBittorrent's max_ratio_act values.
var MaxRatioActions = map[string]int{
	"stop":               0,
	"remove":             1,
	"enableSuperSeeding": 2,
	"removeWithContent":  3,
}

// Encryptions maps the spec's encryption names to qBittorrent's encryption values.
var Encryptions = map[string]int{"allow": 0, "require": 1, "disable": 2}

// SchedulerDays maps the spec's schedulerDays names to qBittorrent's scheduler_days values.
var SchedulerDays = map[string]int{
	"everyDay": 0, "weekdays": 1, "weekends": 2, "monday": 3, "tuesday": 4,
	"wednesday": 5, "thursday": 6, "friday": 7, "saturday": 8, "sunday": 9,
}

// Preferences maps the preferences qBittorrent stores as written to their keys.
// Paths, the share ratio and its action, speed limits, schedule days, excluded
// file names, the Web UI's domain and proxy lists, banned IPs, added trackers
// and the proxy's credentials need converting.
var Preferences = map[string]string{
	"locale":                             "locale",
	"tempPathEnabled":                    "temp_path_enabled",
	"dht":                                "dht",
	"pex":                                "pex",
	"lsd":                                "lsd",
	"maxConnec":                          "max_connec",
	"maxConnecPerTorrent":                "max_connec_per_torrent",
	"maxUploads":                         "max_uploads",
	"maxUploadsPerTorrent":               "max_uploads_per_torrent",
	"maxRatioEnabled":                    "max_ratio_enabled",
	"maxSeedingTimeEnabled":              "max_seeding_time_enabled",
	"maxSeedingTime":                     "max_seeding_time",
	"preallocateAll":                     "preallocate_all",
	"webUiPort":                          "web_ui_port",
	"bypassAuthSubnetWhitelistEnabled":   "bypass_auth_subnet_whitelist_enabled",
	"bypassAuthSubnetWhitelist":          "bypass_auth_subnet_whitelist",
	"bypassLocalAuth":                    "bypass_local_auth",
	"webUiHostHeaderValidationEnabled":   "web_ui_host_header_validation_enabled",
	"webUiCsrfProtectionEnabled":         "web_ui_csrf_protection_enabled",
	"webUiClickjackingProtectionEnabled": "web_ui_clickjacking_protection_enabled",
	"webUiSecureCookieEnabled":           "web_ui_secure_cookie_enabled",
	"webUiReverseProxyEnabled":           "web_ui_reverse_proxy_enabled",
	"incompleteFilesExt":                 "incomplete_files_ext",
	"autoTmmEnabled":                     "auto_tmm_enabled",
	"torrentChangedTmmEnabled":           "torrent_changed_tmm_enabled",
	"savePathChangedTmmEnabled":          "save_path_changed_tmm_enabled",
	"categoryChangedTmmEnabled":          "category_changed_tmm_enabled",
	"torrentContentLayout":               "torrent_content_layout",
	"addStoppedEnabled":                  "add_stopped_enabled",
	"torrentStopCondition":               "torrent_stop_condition",
	"excludedFileNamesEnabled":           "excluded_file_names_enabled",
	"schedulerEnabled":                   "scheduler_enabled",
	"scheduleFromHour":                   "schedule_from_hour",
	"scheduleFromMin":                    "schedule_from_min",
	"scheduleToHour":                     "schedule_to_hour",
	"scheduleToMin":                      "schedule_to_min",
	"limitUtpRate":                       "limit_utp_rate",
	"limitTcpOverhead":                   "limit_tcp_overhead",
	"limitLanPeers":                      "limit_lan_peers",
	"queueingEnabled":                    "queueing_enabled",
	"maxActiveDownloads":                 "max_active_downloads",
	"maxActiveUploads":                   "max_active_uploads",
	"maxActiveTorrents":                  "max_active_torrents",
	"dontCountSlowTorrents":              "dont_count_slow_torrents",
	"slowTorrentDlRateThreshold":         "slow_torrent_dl_rate_threshold",
	"slowTorrentUlRateThreshold":         "slow_torrent_ul_rate_threshold",
	"slowTorrentInactiveTimer":           "slow_torrent_inactive_timer",
	"listenPort":                         "listen_port",
	"randomPort":                         "random_port",
	"upnp":                               "upnp",
	"networkInterface":                   "current_network_interface",
	"networkInterfaceAddress":            "current_interface_address",
	"anonymousMode":                      "anonymous_mode",
	"maxInactiveSeedingTimeEnabled":      "max_inactive_seeding_time_enabled",
	"maxInactiveSeedingTime":             "max_inactive_seeding_time",
	"proxyType":                          "proxy_type",
	"proxyIp":                            "proxy_ip",
	"proxyPort":                          "proxy_port",
	"proxyAuthEnabled":                   "proxy_auth_enabled",
	"proxyHostnameLookup":                "proxy_hostname_lookup",
	"proxyBittorrent":                    "proxy_bittorrent",
	"proxyPeerConnections":               "proxy_peer_connections",
	"proxyRss":                           "proxy_rss",
	"proxyMisc":                          "proxy_misc",
	"ipFilterEnabled":                    "ip_filter_enabled",
	"ipFilterPath":                       "ip_filter_path",
	"ipFilterTrackers":                   "ip_filter_trackers",
	"autorunOnTorrentAddedEnabled":       "autorun_on_torrent_added_enabled",
	"autorunOnTorrentAddedProgram":       "autorun_on_torrent_added_program",
	"autorunEnabled":                     "autorun_enabled",
	"autorunProgram":                     "autorun_program",
	"addTrackersEnabled":                 "add_trackers_enabled",
	"addTrackersFromUrlEnabled":          "add_trackers_from_url_enabled",
	"addTrackersUrl":                     "add_trackers_url",
}

// SpeedLimits are the preferences the spec takes in KiB/s and qBittorrent
// stores in bytes per second.
var SpeedLimits = map[string]string{
	"dlLimit": "dl_limit", "upLimit": "up_limit", "altDlLimit": "alt_dl_limit", "altUpLimit": "alt_up_limit",
}

// NewClient creates a new qBittorrent API client backed by the given engine.HTTPClient.
func NewClient(hc *engine.HTTPClient, username, password string) *Client {
	return &Client{
		hc:       hc,
		username: username,
		password: password,
	}
}

// Login authenticates with qBittorrent and stores the session cookie.
func (c *Client) Login(ctx context.Context) error {
	form := url.Values{}
	form.Set("username", c.username)
	form.Set("password", c.password)

	body, err := c.hc.DoForm(ctx, "/api/v2/auth/login", form)
	if apiErr, ok := errors.AsType[*engine.APIError](err); ok && apiErr.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("qBittorrent refused the sign-in: check the credentials, and that the URL's port is "+
			"qBittorrent's own, as it refuses a request addressed to any other: %w", err)
	}
	if err != nil {
		return fmt.Errorf("executing login request: %w", err)
	}

	if resp := strings.TrimSpace(string(body)); resp != "" && !strings.EqualFold(resp, "Ok.") {
		return fmt.Errorf("login failed: %s", reconciler.Redact(resp))
	}

	// The cookie jar captured the session cookie automatically. Register it with
	// the engine's cookie-based auth so applyAuth includes it on later requests.
	name, value := c.hc.CookieValuePrefix("QBT_SID_")
	if value == "" {
		name, value = "SID", c.hc.CookieValue("SID")
	}
	if value == "" {
		return fmt.Errorf("login succeeded but no session cookie returned")
	}
	c.hc.SetCookieSession(name, value)

	return nil
}

// Ping checks if qBittorrent is reachable.
func (c *Client) Ping(ctx context.Context) error {
	return c.hc.Ping(ctx, "/api/v2/app/version")
}

// SetPreferences updates qBittorrent application preferences.
func (c *Client) SetPreferences(ctx context.Context, prefs map[string]any) error {
	prefsJSON, err := json.Marshal(prefs)
	if err != nil {
		return fmt.Errorf("marshaling preferences: %w", err)
	}

	form := url.Values{}
	form.Set("json", string(prefsJSON))

	_, err = c.hc.DoForm(ctx, "/api/v2/app/setPreferences", form)
	return err
}

// GetPreferences returns the current qBittorrent application preferences.
func (c *Client) GetPreferences(ctx context.Context) (map[string]any, error) {
	return c.hc.GetJSON(ctx, "/api/v2/app/preferences")
}

// ListCategories returns all torrent categories.
func (c *Client) ListCategories(ctx context.Context) (map[string]any, error) {
	return c.hc.GetJSON(ctx, "/api/v2/torrents/categories")
}

// CreateCategory creates a new torrent category with the given save path.
func (c *Client) CreateCategory(ctx context.Context, name, savePath string) error {
	form := url.Values{}
	form.Set("category", name)
	form.Set("savePath", savePath)

	_, err := c.hc.DoForm(ctx, "/api/v2/torrents/createCategory", form)
	return err
}

// RemoveCategory deletes a category. Its torrents stay, uncategorized.
func (c *Client) RemoveCategory(ctx context.Context, name string) error {
	form := url.Values{}
	form.Set("categories", name)
	_, err := c.hc.DoForm(ctx, "/api/v2/torrents/removeCategories", form)
	return err
}

// EditCategory updates an existing torrent category's save path.
func (c *Client) EditCategory(ctx context.Context, name, savePath string) error {
	form := url.Values{}
	form.Set("category", name)
	form.Set("savePath", savePath)

	_, err := c.hc.DoForm(ctx, "/api/v2/torrents/editCategory", form)
	return err
}
