package export

import (
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"

	qbclient "github.com/kyleseneker/arrchitect/internal/client/qbittorrent"
)

func exportQBittorrent(r *reader) (map[string]any, error) {
	qb := qbclient.NewClient(r.hc, r.username, r.password)
	if err := qb.Login(r.ctx); err != nil {
		return nil, err
	}
	prefs, err := qb.GetPreferences(r.ctx)
	if err != nil {
		return nil, err
	}
	preferences := map[string]any{}
	for field, key := range qbclient.Preferences {
		if v, ok := prefs[key]; ok && v != nil {
			preferences[field] = v
		}
	}
	for field, key := range map[string]string{"savePath": "save_path", "tempPath": "temp_path"} {
		if v, _ := prefs[key].(string); v != "" {
			preferences[field] = v
		}
	}
	for field, key := range qbclient.SpeedLimits {
		if limit, ok := prefs[key].(float64); ok {
			if math.Mod(limit, 1024) != 0 {
				r.warn("qBittorrent's %s is %v bytes per second, which isn't a whole number of KiB/s; exported rounded down", field, limit)
			}
			preferences[field] = int(limit) / 1024
		}
	}
	if days, ok := prefs["scheduler_days"].(float64); ok {
		for name, value := range qbclient.SchedulerDays {
			if float64(value) == days {
				preferences["schedulerDays"] = name
			}
		}
	}
	if names, _ := prefs["excluded_file_names"].(string); names != "" {
		preferences["excludedFileNames"] = strings.Split(names, "\n")
	}
	for field, key := range map[string]string{"webUiDomainList": "web_ui_domain_list", "webUiReverseProxiesList": "web_ui_reverse_proxies_list"} {
		var entries []any
		list, _ := prefs[key].(string)
		for entry := range strings.SplitSeq(list, ";") {
			if entry = strings.TrimSpace(entry); entry != "" {
				entries = append(entries, entry)
			}
		}
		if len(entries) > 0 {
			preferences[field] = entries
		}
	}
	if ratio, ok := prefs["max_ratio"].(float64); ok {
		preferences["maxRatio"] = strconv.FormatFloat(ratio, 'f', -1, 64)
	}
	if mode, ok := prefs["encryption"].(float64); ok {
		for name, value := range qbclient.Encryptions {
			if float64(value) == mode {
				preferences["encryption"] = name
			}
		}
	}
	if act, ok := prefs["max_ratio_act"].(float64); ok {
		for name, value := range qbclient.MaxRatioActions {
			if float64(value) == act {
				preferences["maxRatioAction"] = name
			}
		}
	}

	r.qbExtraPreferences(preferences, prefs)
	categories, err := qb.ListCategories(r.ctx)
	if err != nil {
		return nil, err
	}
	var cats []any
	for _, name := range slices.Sorted(maps.Keys(categories)) {
		cat, _ := categories[name].(map[string]any)
		cats = append(cats, map[string]any{"name": name, "savePath": cat["savePath"]})
	}

	spec := map[string]any{
		"connection": map[string]any{
			"usernameSecretRef": r.creds.ref(r.username, "username"),
			"passwordSecretRef": r.creds.ref(r.password, "password"),
		},
		"preferences": preferences,
	}
	if len(cats) > 0 {
		spec["categories"] = cats
	}
	return spec, nil
}

// qbExtraPreferences exports the banned addresses, added trackers and proxy
// credentials, which qBittorrent keeps in its own forms.
func (r *reader) qbExtraPreferences(preferences, prefs map[string]any) {
	if ips, _ := prefs["banned_IPs"].(string); ips != "" {
		preferences["bannedIps"] = strings.Fields(ips)
	}
	if trackers, _ := prefs["add_trackers"].(string); strings.TrimSpace(trackers) != "" {
		var urls []string
		for line := range strings.SplitSeq(trackers, "\n") {
			if line = strings.TrimSpace(line); line != "" && !slices.Contains(urls, line) {
				urls = append(urls, line)
			}
		}
		if strings.Join(urls, "\n\n") != strings.TrimSpace(trackers) {
			r.warn("qBittorrent's added trackers aren't each in their own tier, which is how the config writes them")
		}
		preferences["addTrackers"] = urls
	}
	for field, key := range map[string]string{"proxyUsernameSecretRef": "proxy_username", "proxyPasswordSecretRef": "proxy_password"} {
		if v, _ := prefs[key].(string); v != "" {
			preferences[field] = r.creds.ref(v, strings.ReplaceAll(key, "_", "-"))
		}
	}
}
