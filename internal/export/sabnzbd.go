package export

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"

	sabnzbdclient "github.com/kyleseneker/arrchitect/internal/client/sabnzbd"
	"github.com/kyleseneker/arrchitect/internal/reconciler"
)

func exportSabnzbd(r *reader) (map[string]any, error) {
	current, err := sabnzbdclient.NewClient(r.hc, r.apiKey).GetConfig(r.ctx)
	if err != nil {
		return nil, err
	}
	misc, _ := current["misc"].(map[string]any)
	spec := map[string]any{}

	folders := map[string]any{}
	for field, key := range sabnzbdclient.Folders {
		if s := sabText(misc[key]); s != "" {
			folders[field] = s
		}
	}
	if len(folders) > 0 {
		spec["folders"] = folders
	}

	general := map[string]any{}
	for field, key := range sabnzbdclient.Texts {
		if s := sabText(misc[key]); s != "" {
			general[field] = s
		}
	}
	for field, key := range sabnzbdclient.Numbers {
		if n := sabNumber(misc[key]); n != nil {
			general[field] = n
		}
	}
	for field, key := range sabnzbdclient.Switches {
		if v, ok := misc[key]; ok {
			general[field] = sabSwitch(v)
		}
	}
	for field, key := range sabnzbdclient.Lists {
		if items := sabList(misc[key]); len(items) > 0 {
			general[field] = items
		}
	}
	for field, choice := range sabnzbdclient.Choices {
		if name, ok := sabChoice(choice.Values, misc[choice.Key]); ok {
			general[field] = name
		} else if v, set := misc[choice.Key]; set {
			r.warn("the config can't express %s %v; %s is left out", choice.Key, v, field)
		}
	}
	switch sort := sabText(misc["auto_sort"]); sort {
	case "", "0":
		general["autoSort"] = false
	case "1", "avg_age asc":
		general["autoSort"] = true
	default:
		r.warn("the config can't express the queue sort %q; autoSort is left out", sort)
	}
	if len(general) > 0 {
		spec["general"] = general
	}

	var servers []any
	for _, s := range list(current["servers"]) {
		name := sabText(s["name"])
		server := map[string]any{
			"name": name, "host": s["host"], "port": sabNumber(s["port"]), "ssl": sabSwitch(s["ssl"]),
			"connections": sabNumber(s["connections"]), "priority": sabNumber(s["priority"]),
			"retention": sabNumber(s["retention"]), "enable": sabSwitch(s["enable"]),
			"optional": sabSwitch(s["optional"]), "required": sabSwitch(s["required"]), "timeout": sabNumber(s["timeout"]),
		}
		if verify, ok := sabChoice(sabnzbdclient.SSLVerify, s["ssl_verify"]); ok {
			server["sslVerify"] = verify
		}
		if ciphers := sabText(s["ssl_ciphers"]); ciphers != "" {
			server["sslCiphers"] = ciphers
		}
		if username := sabText(s["username"]); username != "" {
			server["usernameSecretRef"] = r.creds.ref(username, "server", name, "username")
		}
		if password := sabText(s["password"]); password != "" {
			if reconciler.IsMasked(password) {
				password = ""
			}
			server["passwordSecretRef"] = r.creds.ref(password, "server", name, "password")
		}
		servers = append(servers, server)
	}
	if len(servers) > 0 {
		spec["servers"] = servers
	}

	if categories := sabCategories(list(current["categories"])); len(categories) > 0 {
		spec["categories"] = categories
	}
	if apprise := r.sabnzbdApprise(current); apprise != nil {
		spec["apprise"] = apprise
	}
	if login := r.sabnzbdLogin(misc); login != nil {
		spec["login"] = login
	}
	if quota := r.sabnzbdQuota(misc); quota != nil {
		spec["quota"] = quota
	}
	return spec, nil
}

// sabnzbdLogin exports the web interface login when it has a user name. The
// password reads back masked, so it is left for the user to fill in.
func (r *reader) sabnzbdLogin(misc map[string]any) map[string]any {
	username := sabText(misc["username"])
	if username == "" {
		return nil
	}
	password := sabText(misc["password"])
	if reconciler.IsMasked(password) {
		password = ""
	}
	return map[string]any{
		"usernameSecretRef": r.creds.ref(username, "login", "username"),
		"passwordSecretRef": r.creds.ref(password, "login", "password"),
	}
}

var (
	sabQuotaDay  = regexp.MustCompile(`^\s*(\d+)[^:]*`)
	sabQuotaTime = regexp.MustCompile(`(\d+:\d+)\s*$`)
	sabClockTime = regexp.MustCompile(`^([01]?[0-9]|2[0-3]):[0-5][0-9]$`)
)

// sabnzbdQuota exports the download quota when it has a size, reading the
// reset day and time the way SABnzbd does.
func (r *reader) sabnzbdQuota(misc map[string]any) map[string]any {
	size := sabText(misc["quota_size"])
	if size == "" {
		return nil
	}
	quota := map[string]any{"size": size, "resume": sabSwitch(misc["quota_resume"])}
	if period, ok := sabChoice(sabnzbdclient.QuotaPeriods, misc["quota_period"]); ok {
		quota["period"] = period
	}
	day := sabText(misc["quota_day"])
	last := 31
	if quota["period"] == "week" {
		last = 7
	}
	if m := sabQuotaDay.FindStringSubmatch(day); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil && n >= 1 && n <= last {
			quota["resetDay"] = n
		} else {
			r.warn("the config can't express the quota reset day %q; resetDay is left out", day)
		}
	}
	if m := sabQuotaTime.FindStringSubmatch(day); m != nil {
		if sabClockTime.MatchString(m[1]) {
			quota["resetTime"] = m[1]
		} else {
			r.warn("the config can't express the quota reset time %q; resetTime is left out", m[1])
		}
	}
	return quota
}

// sabnzbdApprise exports the apprise section when it is on or has URLs.
func (r *reader) sabnzbdApprise(current map[string]any) map[string]any {
	section, _ := current["apprise"].(map[string]any)
	on := func(key string) bool {
		v := section[key]
		return v == true || v == float64(1) || v == "1"
	}
	urls, _ := section["apprise_urls"].(string)
	if !on("apprise_enable") && urls == "" {
		return nil
	}
	apprise := map[string]any{"enabled": on("apprise_enable")}
	if urls != "" {
		apprise["urlsSecretRef"] = r.creds.ref(urls, "apprise", "urls")
	}
	if cats, _ := section["apprise_cats"].([]any); len(cats) > 0 {
		apprise["categories"] = cats
	}
	var events []any
	for _, event := range slices.Sorted(maps.Keys(sabnzbdclient.AppriseEvents)) {
		if on("apprise_target_" + sabnzbdclient.AppriseEvents[event] + "_enable") {
			events = append(events, event)
		}
	}
	apprise["events"] = events
	return apprise
}

func sabCategories(current []map[string]any) []any {
	categories := make([]any, 0, len(current))
	for _, c := range current {
		category := map[string]any{"name": sabText(c["name"])}
		if priority, ok := sabChoice(sabnzbdclient.Priorities, c["priority"]); ok {
			category["priority"] = priority
		}
		if dir := sabText(c["dir"]); dir != "" {
			category["dir"] = dir
		}
		if script := sabText(c["script"]); script != "" {
			category["script"] = script
		}
		if pp, ok := sabChoice(sabnzbdclient.PostProcessing, c["pp"]); ok {
			category["postProcessing"] = pp
		}
		categories = append(categories, category)
	}
	return categories
}

func sabText(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	default:
		return fmt.Sprint(x)
	}
}

func sabNumber(v any) any {
	if n, err := strconv.Atoi(sabText(v)); err == nil {
		return n
	}
	return nil
}

func sabSwitch(v any) bool {
	return v == true || sabText(v) == "1"
}

func sabList(v any) []string {
	raw, _ := v.([]any)
	items := make([]string, 0, len(raw))
	for _, item := range raw {
		items = append(items, sabText(item))
	}
	return items
}

// sabChoice names the value SABnzbd stores for a setting it numbers.
func sabChoice(names map[string]string, v any) (string, bool) {
	value := sabText(v)
	for name, n := range names {
		if n == value {
			return name, true
		}
	}
	return "", false
}
