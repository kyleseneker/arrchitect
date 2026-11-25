package export

import (
	"context"
	"errors"
	"maps"
	"net/http"
	"slices"
	"strings"

	autobrrclient "github.com/kyleseneker/arrchitect/internal/client/autobrr"
	"github.com/kyleseneker/arrchitect/internal/engine"
	"github.com/kyleseneker/arrchitect/internal/reconciler"
)

func exportAutobrr(r *reader) (map[string]any, error) {
	ac := autobrrclient.NewClient(r.hc)
	spec := map[string]any{}

	proxies, err := r.autobrrRead("proxies", "proxies", ac.ListProxies)
	if err != nil {
		return nil, err
	}
	proxyNames := map[float64]any{}
	var ps []any
	for _, p := range proxies {
		proxyNames[num(p["id"])] = p["name"]
		name, _ := p["name"].(string)
		proxy := pick(p, "name", "addr")
		proxy["type"] = upperName([]string{"Socks5", "HTTP"}, p["type"])
		proxy["enable"] = p["enabled"]
		if timeout := num(p["timeout"]); timeout != 0 {
			proxy["timeout"] = timeout
		}
		r.autobrrSecret(proxy, "usernameSecretRef", p["user"], "proxy", name, "username")
		r.autobrrSecret(proxy, "passwordSecretRef", p["pass"], "proxy", name, "password")
		ps = append(ps, proxy)
	}
	setList(spec, "proxies", ps)

	clients, err := r.autobrrRead("downloaders", "downloadClients", ac.ListDownloadClients)
	if err != nil {
		return nil, err
	}
	clientNames := map[float64]any{}
	var dcs []any
	for _, c := range clients {
		clientNames[num(c["id"])] = c["name"]
		dcs = append(dcs, r.autobrrDownloadClient(c))
	}
	setList(spec, "downloadClients", dcs)

	indexers, err := r.autobrrRead("indexers", "indexers", ac.ListIndexers)
	if err != nil {
		return nil, err
	}
	var idxs []any
	for _, summary := range indexers {
		i, err := ac.GetIndexer(r.ctx, int(num(summary["id"])))
		if err != nil {
			return nil, err
		}
		idx := r.autobrrIndexer(i)
		setProxy(idx, i, proxyNames)
		idxs = append(idxs, idx)
	}
	setList(spec, "indexers", idxs)

	networks, err := r.autobrrRead("irc", "ircNetworks", ac.ListIRCNetworks)
	if err != nil {
		return nil, err
	}
	var ircs []any
	for _, n := range networks {
		ircs = append(ircs, r.autobrrIRCNetwork(n, proxyNames))
	}
	setList(spec, "ircNetworks", ircs)

	feeds, err := r.autobrrRead("feeds", "feeds", ac.ListFeeds)
	if err != nil {
		return nil, err
	}
	var fs []any
	for _, f := range feeds {
		fs = append(fs, r.autobrrFeed(f))
	}
	setList(spec, "feeds", fs)

	filters, err := r.autobrrRead("filters", "filters", ac.ListFilters)
	if err != nil {
		return nil, err
	}
	var out []any
	for _, summary := range filters {
		id := int(num(summary["id"]))
		full, err := ac.GetFilter(r.ctx, id)
		if err != nil {
			return nil, err
		}
		actions, err := ac.ListActions(r.ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, r.autobrrFilter(full, actions, clientNames))
	}
	setList(spec, "filters", out)

	notifications, err := r.autobrrRead("notifications", "notifications", ac.ListNotifications)
	if err != nil {
		return nil, err
	}
	var ns []any
	for _, n := range notifications {
		if exported := r.autobrrNotification(n); exported != nil {
			ns = append(ns, exported)
		}
	}
	setList(spec, "notifications", ns)

	lists, err := r.autobrrRead("lists", "lists", ac.ListLists)
	if err != nil {
		return nil, err
	}
	var ls []any
	for _, l := range lists {
		ls = append(ls, r.autobrrList(l, clientNames))
	}
	setList(spec, "lists", ls)
	return spec, nil
}

// autobrrRead lists the items under one API key scope. Autobrr 1.91 refuses a
// key without that scope, and the export leaves field out.
func (r *reader) autobrrRead(scope, field string, list func(context.Context) ([]map[string]any, error)) ([]map[string]any, error) {
	items, err := list(r.ctx)
	var apiErr *engine.APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusForbidden {
		r.warn("the API key can't read %s (scope %s:read), so %s is left out", scope, scope, field)
		return nil, nil
	}
	return items, err
}

func (r *reader) autobrrDownloadClient(c map[string]any) map[string]any {
	name, _ := c["name"].(string)
	dc := pick(c, "name", "host", "tls")
	dc["enable"] = c["enabled"]
	dc["type"] = crdName(autobrrclient.Types, c["type"])
	if port := num(c["port"]); port != 0 {
		dc["port"] = port
	}
	r.autobrrSecret(dc, "usernameSecretRef", c["username"], "download-client", name, "username")
	r.autobrrSecret(dc, "passwordSecretRef", c["password"], "download-client", name, "password")
	settings, _ := c["settings"].(map[string]any)
	r.autobrrSecret(dc, "apiKeySecretRef", settings["apikey"], "download-client", name, "api-key")
	if auth := r.autobrrHTTPAuth(settings, name); auth != nil {
		dc["httpAuth"] = auth
	}
	if rules := autobrrRules(settings); rules != nil {
		dc["rules"] = rules
	}
	setText(dc, "externalDownloadClient", settings["external_download_client"])
	return dc
}

func (r *reader) autobrrIRCNetwork(n map[string]any, proxyNames map[float64]any) map[string]any {
	name, _ := n["name"].(string)
	irc := pick(n, "name", "server", "port", "tls", "nick")
	irc["enable"] = n["enabled"]
	setText(irc, "inviteCommand", n["invite_command"])
	setProxy(irc, n, proxyNames)
	if bouncer, _ := n["use_bouncer"].(bool); bouncer {
		setText(irc, "bouncerAddress", n["bouncer_addr"])
	}
	if botMode, _ := n["bot_mode"].(bool); botMode {
		irc["botMode"] = true
	}
	r.autobrrSecret(irc, "serverPasswordSecretRef", n["pass"], "irc", name, "server-password")
	auth, _ := n["auth"].(map[string]any)
	if mechanism, _ := auth["mechanism"].(string); mechanism != "" {
		irc["authMechanism"] = crdName(autobrrclient.IRCAuthMechanisms, mechanism)
	}
	r.autobrrSecret(irc, "authAccountSecretRef", auth["account"], "irc", name, "account")
	r.autobrrSecret(irc, "authPasswordSecretRef", auth["password"], "irc", name, "password")
	chs := list(n["channels"])
	channels := make([]any, 0, len(chs))
	for _, ch := range chs {
		channel := pick(ch, "name")
		chName, _ := ch["name"].(string)
		r.autobrrSecret(channel, "passwordSecretRef", ch["password"], "irc", name, chName, "password")
		channels = append(channels, channel)
	}
	if len(channels) > 0 {
		irc["channels"] = channels
	}
	return irc
}

func (r *reader) autobrrFeed(f map[string]any) map[string]any {
	name, _ := f["name"].(string)
	feed := pick(f, "name", "url")
	feed["type"] = crdName(autobrrFeedTypes, f["type"])
	feed["enable"] = f["enabled"]
	if interval := num(f["interval"]); interval != 0 {
		feed["interval"] = interval
	}
	if timeout := num(f["timeout"]); timeout != 0 {
		feed["timeout"] = timeout
	}
	if maxAge := num(f["max_age"]); maxAge != 0 {
		feed["maxAge"] = maxAge
	}
	settings, _ := f["settings"].(map[string]any)
	setText(feed, "downloadType", crdName(autobrrclient.FeedDownloadTypes, settings["download_type"]))
	r.autobrrSecret(feed, "cookieSecretRef", f["cookie"], "feed", name, "cookie")
	indexer, _ := f["indexer"].(map[string]any)
	setText(feed, "indexerName", indexer["name"])
	r.autobrrSecret(feed, "apiKeySecretRef", f["api_key"], "feed", name, "api-key")
	return feed
}

var autobrrListTypes = []string{
	"Sonarr", "Radarr", "Lidarr", "Readarr", "Whisparr", "MDBList", "Metacritic", "Plaintext", "Trakt", "Steam", "AniList",
}

var autobrrListFields = map[string]string{
	"url": "url", "matchRelease": "match_release", "tagsIncluded": "tags_included", "tagsExcluded": "tags_excluded",
	"includeUnmonitored": "include_unmonitored", "includeAlternateTitles": "include_alternate_titles",
	"includeYear": "include_year", "skipCleanSanitize": "skip_clean_sanitize",
}

func (r *reader) autobrrList(l map[string]any, clientNames map[float64]any) map[string]any {
	name, _ := l["name"].(string)
	out := pick(l, "name")
	out["type"] = upperName(autobrrListTypes, l["type"])
	out["enable"] = l["enabled"]
	autobrrSetFields(out, l, autobrrListFields)
	if id := num(l["client_id"]); id != 0 {
		setText(out, "downloadClientName", clientNames[id])
	}
	r.autobrrSecret(out, "apiKeySecretRef", l["api_key"], "list", name, "api-key")
	refs := list(l["filters"])
	filters := make([]any, 0, len(refs))
	for _, f := range refs {
		filters = append(filters, f["name"])
	}
	if len(filters) > 0 {
		out["filters"] = filters
	}
	return out
}

// autobrrHTTPAuth reads a download client's HTTP authentication from auth, or
// from the deprecated basic block Autobrr's web UI still writes for most
// clients.
func (r *reader) autobrrHTTPAuth(settings map[string]any, name string) map[string]any {
	auth, _ := settings["auth"].(map[string]any)
	if enabled, _ := auth["enabled"].(bool); !enabled {
		basic, _ := settings["basic"].(map[string]any)
		if on, _ := basic["auth"].(bool); !on {
			return nil
		}
		auth = map[string]any{"username": basic["username"], "password": basic["password"]}
	}
	out := map[string]any{}
	if auth["type"] == "DIGEST_AUTH" {
		out["type"] = "Digest"
	}
	r.autobrrSecret(out, "usernameSecretRef", auth["username"], "download-client", name, "http-username")
	r.autobrrSecret(out, "passwordSecretRef", auth["password"], "download-client", name, "http-password")
	return out
}

func autobrrRules(settings map[string]any) map[string]any {
	rules, _ := settings["rules"].(map[string]any)
	out := map[string]any{}
	autobrrSetFields(out, rules, autobrrclient.RuleFields)
	setText(out, "ignoreSlowTorrentsCondition",
		crdName(autobrrclient.SlowTorrentConditions, rules["ignore_slow_torrents_condition"]))
	enabled, _ := rules["enabled"].(bool)
	switch {
	case !enabled && len(out) == 0:
		return nil
	case !enabled:
		out["enable"] = false
	}
	return out
}

// upperName returns the name whose upper case is the Autobrr constant.
func upperName(names []string, value any) string {
	for _, name := range names {
		if strings.ToUpper(name) == value {
			return name
		}
	}
	return ""
}

func setProxy(out, item map[string]any, proxyNames map[float64]any) {
	if used, _ := item["use_proxy"].(bool); used {
		setText(out, "proxyName", proxyNames[num(item["proxy_id"])])
	}
}

// autobrrNotification exports one notification, leaving out Autobrr's own
// built-in one.
func (r *reader) autobrrNotification(n map[string]any) map[string]any {
	crdType := upperName([]string{
		"Discord", "Notifiarr", "Pushover", "Slack", "Telegram", "Gotify", "Ntfy", "LunaSea", "Shoutrrr", "Webhook", "Bark",
	}, n["type"])
	if crdType == "" {
		return nil
	}
	name, _ := n["name"].(string)
	out := map[string]any{"name": name, "type": crdType, "enable": n["enabled"]}
	if events, ok := n["events"].([]any); ok && len(events) > 0 {
		names := make([]any, 0, len(events))
		for _, e := range events {
			event, _ := e.(string)
			names = append(names, autobrrclient.NotificationEventName(event))
		}
		out["events"] = names
	}
	var options []any
	for _, key := range autobrrclient.NotificationOptions {
		switch value := n[key].(type) {
		case string:
			switch {
			case value == "":
			case slices.Contains(autobrrclient.NotificationSecrets, key):
				if reconciler.IsMasked(value) {
					value = ""
				}
				options = append(options, map[string]any{"name": key, "valueFrom": map[string]any{
					"secretKeyRef": r.creds.ref(value, "notification", name, key),
				}})
			default:
				options = append(options, map[string]any{"name": key, "value": value})
			}
		case float64:
			if value != 0 {
				options = append(options, map[string]any{"name": key, "value": value})
			}
		}
	}
	if len(options) > 0 {
		out["options"] = options
	}
	return out
}

func (r *reader) autobrrFilter(f map[string]any, actions []map[string]any, clientNames map[float64]any) map[string]any {
	filter := pick(f, "name")
	filter["enable"] = f["enabled"]
	autobrrSetFields(filter, f, autobrrclient.FilterFields)
	for field, key := range map[string]string{"tags": "tags", "exceptTags": "except_tags"} {
		if tags, _ := f[key].(string); tags != "" {
			filter[field] = strings.Split(tags, ",")
		}
	}
	var indexers []string
	for _, i := range list(f["indexers"]) {
		if n, _ := i["name"].(string); n != "" {
			indexers = append(indexers, n)
		}
	}
	if len(indexers) > 0 {
		filter["indexers"] = indexers
	}
	as := make([]any, 0, len(actions))
	for _, a := range actions {
		action := pick(a, "name")
		action["enable"] = a["enabled"]
		action["type"] = crdName(autobrrclient.Types, a["type"])
		if id := num(a["client_id"]); id != 0 {
			action["downloadClientName"] = clientNames[id]
		}
		autobrrSetFields(action, a, autobrrclient.ActionFields)
		as = append(as, action)
	}
	if len(as) > 0 {
		filter["actions"] = as
	}
	return filter
}

// autobrrSetFields exports the values a table names, leaving out Autobrr's
// empty defaults: an unset criterion is stored as "", false, 0 or [].
func autobrrSetFields(out, item map[string]any, table map[string]string) {
	for field, key := range table {
		switch v := item[key].(type) {
		case string:
			if v != "" {
				out[field] = v
			}
		case bool:
			if v {
				out[field] = v
			}
		case float64:
			if v != 0 {
				out[field] = v
			}
		case []any:
			if len(v) > 0 {
				out[field] = texts(v)
			}
		}
	}
}

var autobrrFeedTypes = map[string]string{"Torznab": "TORZNAB", "Newznab": "NEWZNAB", "RSS": "RSS"}

// autobrrIndexer exports an indexer by its definition. Autobrr names a feed
// indexer's definition after its implementation.
func (r *reader) autobrrIndexer(i map[string]any) map[string]any {
	name, _ := i["name"].(string)
	idx := pick(i, "name")
	idx["enable"] = i["enabled"]
	idx["identifier"] = i["identifier"]
	if implementation, _ := i["implementation"].(string); implementation != autobrrclient.ImplementationIRC {
		idx["identifier"] = implementation
	} else {
		setText(idx, "baseUrl", i["base_url"])
	}
	settings, _ := i["settings"].(map[string]any)
	var out []any
	for _, key := range slices.Sorted(maps.Keys(settings)) {
		value, _ := settings[key].(string)
		switch {
		case value == "":
		case reconciler.IsMasked(value):
			out = append(out, map[string]any{"name": key, "valueFrom": map[string]any{"secretKeyRef": r.creds.ref("", "indexer", name, key)}})
		default:
			out = append(out, map[string]any{"name": key, "value": value})
		}
	}
	if len(out) > 0 {
		idx["settings"] = out
	}
	return idx
}

func (r *reader) autobrrSecret(out map[string]any, field string, value any, parts ...string) {
	text, _ := value.(string)
	if text == "" {
		return
	}
	if reconciler.IsMasked(text) {
		text = ""
	}
	out[field] = r.creds.ref(text, parts...)
}

func crdName(names map[string]string, value any) any {
	for crd, constant := range names {
		if constant == value {
			return crd
		}
	}
	return value
}

func setText(out map[string]any, field string, value any) {
	if s, _ := value.(string); s != "" {
		out[field] = s
	}
}

func setList(spec map[string]any, key string, items []any) {
	if len(items) > 0 {
		spec[key] = items
	}
}
