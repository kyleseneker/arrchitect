package export

import (
	"errors"
	"fmt"
	"maps"
	"net/http"
	"reflect"
	"slices"
	"strconv"
	"strings"

	requestsv1alpha1 "github.com/kyleseneker/arrchitect/api/requests/v1alpha1"
	seerrclient "github.com/kyleseneker/arrchitect/internal/client/seerr"
	"github.com/kyleseneker/arrchitect/internal/engine"
)

var seerrNotificationAgents = []string{
	"discord", "email", "gotify", "lunasea", "ntfy", "pushbullet", "pushover", "slack", "telegram", "webhook", "webpush",
}

var credentialOptions = []string{"token", "password", "pass", "key", "secret", "auth", "webhookurl", "botapi"}

// isCredentialOption reports whether a notification option's name looks like
// it holds a credential.
func isCredentialOption(name string) bool {
	lower := strings.ToLower(name)
	return slices.ContainsFunc(credentialOptions, func(s string) bool { return strings.Contains(lower, s) })
}

const (
	seerrPlex     = 1
	seerrJellyfin = 2
)

func exportSeerr(r *reader) (map[string]any, error) {
	sc := seerrclient.NewClient(r.hc)
	sc.SetAPIKey(r.apiKey)
	main, err := sc.Get(r.ctx, "/api/v1/settings/main")
	if err != nil {
		return nil, err
	}
	spec := map[string]any{"main": r.seerrMain(main)}
	network, err := sc.Get(r.ctx, "/api/v1/settings/network")
	if err != nil {
		return nil, err
	}
	if trust, ok := network["trustProxy"].(bool); ok {
		spec["network"] = map[string]any{"trustProxy": trust}
	}
	if err := r.seerrMetadataAndJobs(sc, spec); err != nil {
		return nil, err
	}

	switch int(num(main["mediaServerType"])) {
	case seerrJellyfin:
		jf, err := sc.Get(r.ctx, "/api/v1/settings/jellyfin")
		if err != nil {
			return nil, err
		}
		auth := pick(jf, "port", "useSsl", "urlBase")
		auth["hostname"] = jf["ip"]
		auth["usernameSecretRef"] = r.creds.ref("", "jellyfin", "username")
		auth["passwordSecretRef"] = r.creds.ref("", "jellyfin", "password")
		spec["jellyfinAuth"] = auth
		if err := seerrLibraries(r, sc, spec, "jellyfin"); err != nil {
			return nil, err
		}
	case seerrPlex:
		plex, err := sc.Get(r.ctx, "/api/v1/settings/plex")
		if err != nil {
			return nil, err
		}
		auth := pick(plex, "port", "useSsl")
		auth["hostname"] = plex["ip"]
		auth["tokenSecretRef"] = r.creds.ref("", "plex", "token")
		spec["plexAuth"] = auth
		if err := seerrLibraries(r, sc, spec, "plex"); err != nil {
			return nil, err
		}
	}

	for _, service := range []string{"sonarr", "radarr"} {
		servers, err := sc.GetList(r.ctx, "/api/v1/settings/"+service)
		if err != nil {
			return nil, err
		}
		if len(servers) == 0 {
			continue
		}
		exported := make([]any, 0, len(servers))
		for _, s := range servers {
			exported = append(exported, r.seerrService(service, s))
		}
		spec[service] = exported
	}

	var notifications []any
	for _, agent := range seerrNotificationAgents {
		settings, err := sc.Get(r.ctx, "/api/v1/settings/notifications/"+agent)
		var apiErr *engine.APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
			continue
		}
		if err != nil {
			return nil, err
		}
		if settings["enabled"] != true {
			continue
		}
		notification := pick(settings, "enabled")
		notification["agent"] = agent
		if bits, ok := settings["types"].(float64); ok {
			notification["types"] = r.seerrNotificationTypes(agent, int(bits))
		}
		options, secrets := r.seerrOptions(agent, settings["options"])
		if len(options)+len(secrets) > 0 {
			notification["options"] = configFields(options, secrets)
		}
		notifications = append(notifications, notification)
	}
	if len(notifications) > 0 {
		spec["notifications"] = notifications
	}
	return spec, nil
}

// seerrLibraries exports the libraries Seerr has enabled.
func seerrLibraries(r *reader, sc *seerrclient.Client, spec map[string]any, server string) error {
	libraries, err := sc.GetList(r.ctx, "/api/v1/settings/"+server+"/library")
	if err != nil {
		return err
	}
	var enabled []any
	for _, lib := range libraries {
		if lib["enabled"] == true {
			enabled = append(enabled, lib["name"])
		}
	}
	if len(enabled) > 0 {
		spec["libraries"] = enabled
	}
	return nil
}

func (r *reader) seerrService(service string, s map[string]any) map[string]any {
	out := seerrFields(s, reflect.TypeFor[requestsv1alpha1.SeerrServiceConnection]())
	if name, _ := s["activeProfileName"].(string); name != "" {
		delete(out, "activeProfileId")
	}
	key, _ := s["apiKey"].(string)
	name, _ := s["name"].(string)
	out["apiKeySecretRef"] = r.creds.ref(key, service, name, "api-key")
	return out
}

// seerrOptions splits an agent's options into plain ones and ones that look
// like credentials, which go to the Secret.
func (r *reader) seerrOptions(agent string, v any) (map[string]any, map[string]any) {
	raw, _ := v.(map[string]any)
	options, secrets := map[string]any{}, map[string]any{}
	for _, key := range slices.Sorted(maps.Keys(raw)) {
		switch x := raw[key].(type) {
		case nil:
		case string:
			switch {
			case x == "":
			case isCredentialOption(key):
				secrets[key] = r.creds.ref(x, "notification", agent, key)
			default:
				options[key] = x
			}
		case float64, bool:
			options[key] = x
		default:
			r.warn("notification %q: the config can't express %s, so it stays as set in Seerr", agent, key)
		}
	}
	return options, secrets
}

func (r *reader) seerrMain(main map[string]any) map[string]any {
	out := seerrFields(main, reflect.TypeFor[requestsv1alpha1.SeerrMain]())
	delete(out, "blocklistedTags")
	delete(out, "blocklistedTagsLimit")
	if login, ok := main["newPlexLogin"].(bool); ok {
		out["newMediaServerLogin"] = login
	}
	if limit, err := strconv.Atoi(fmt.Sprint(main["blocklistedTagsLimit"])); err == nil && limit > 0 {
		out["blocklistedTagsLimit"] = limit
	}
	for key, field := range map[string]string{"originalLanguage": "originalLanguages", "blocklistLanguage": "blocklistLanguages"} {
		if languages, _ := main[key].(string); languages != "" {
			out[field] = strings.Split(languages, "|")
		}
	}
	if tags, _ := main["blocklistedTags"].(string); tags != "" {
		ids := []any{}
		for tag := range strings.SplitSeq(tags, ",") {
			id, err := strconv.Atoi(tag)
			if err != nil || id < 1 {
				r.warn("blocklisted tag %q isn't a TMDB keyword ID, so it isn't exported", tag)
				continue
			}
			ids = append(ids, id)
		}
		if len(ids) > 0 {
			out["blocklistedTags"] = ids
		}
	}
	quotas := map[string]any{}
	defaults, _ := main["defaultQuotas"].(map[string]any)
	for _, kind := range []string{"movie", "tv"} {
		q, _ := defaults[kind].(map[string]any)
		quota := map[string]any{}
		for from, to := range map[string]string{"quotaLimit": "limit", "quotaDays": "days"} {
			if v, ok := q[from].(float64); ok {
				quota[to] = v
			}
		}
		if len(quota) > 0 {
			quotas[kind] = quota
		}
	}
	if len(quotas) > 0 {
		out["defaultQuotas"] = quotas
	}
	return out
}

func seerrFields(values map[string]any, t reflect.Type) map[string]any {
	out := map[string]any{}
	for field := range t.Fields() {
		name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		if field.Type.Kind() == reflect.Struct {
			continue
		}
		if v, ok := values[name]; ok && v != nil {
			out[name] = v
		}
	}
	return out
}

func (r *reader) seerrNotificationTypes(agent string, bits int) []any {
	types := []any{}
	for _, name := range slices.Sorted(maps.Keys(seerrclient.NotificationTypes)) {
		if bit := seerrclient.NotificationTypes[name]; bits&bit != 0 {
			types = append(types, name)
			bits &^= bit
		}
	}
	if bits != 0 {
		r.warn("notification %q: the config can't express type bits %d, and applying it would clear them", agent, bits)
	}
	return types
}

// seerrMetadataAndJobs exports the metadata providers and the schedule of each
// job Seerr runs.
func (r *reader) seerrMetadataAndJobs(sc *seerrclient.Client, spec map[string]any) error {
	metadata, err := sc.Get(r.ctx, "/api/v1/settings/metadatas")
	if err != nil {
		return err
	}
	if providers := pick(metadata, "tv", "anime"); len(providers) > 0 {
		spec["metadataProviders"] = providers
	}
	jobs, err := sc.GetList(r.ctx, "/api/v1/settings/jobs")
	if err != nil {
		return err
	}
	var schedules []any
	for _, job := range jobs {
		if id, _ := job["id"].(string); id != "" {
			schedules = append(schedules, map[string]any{"id": id, "schedule": job["cronSchedule"]})
		}
	}
	if len(schedules) > 0 {
		spec["jobSchedules"] = schedules
	}
	return nil
}
