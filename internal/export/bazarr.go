package export

import (
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strconv"
	"strings"

	subtitlesv1alpha1 "github.com/kyleseneker/arrchitect/api/subtitles/v1alpha1"
	bazarrclient "github.com/kyleseneker/arrchitect/internal/client/bazarr"
	"github.com/kyleseneker/arrchitect/internal/reconciler"
)

var bazarrSecretSettings = []string{"password", "apikey", "api_key", "token", "secret", "passkey", "cookie"}

func exportBazarr(r *reader) (map[string]any, error) {
	bc := bazarrclient.NewClient(r.hc)
	settings, err := bc.GetSettings(r.ctx)
	if err != nil {
		return nil, err
	}
	section := func(name string) map[string]any {
		m, _ := settings[name].(map[string]any)
		return m
	}
	general := section("general")
	spec := map[string]any{
		"general": bazarrSettings("general", general, reflect.TypeFor[subtitlesv1alpha1.BazarrGeneral]()),
		"subSync": bazarrSettings("subsync", section("subsync"), reflect.TypeFor[subtitlesv1alpha1.BazarrSubSync]()),
	}
	for _, c := range []struct{ key, section, use string }{
		{"sonarrConnection", "sonarr", "use_sonarr"},
		{"radarrConnection", "radarr", "use_radarr"},
	} {
		if use, _ := general[c.use].(bool); !use {
			continue
		}
		values := section(c.section)
		conn := bazarrSettings(c.section, values, reflect.TypeFor[subtitlesv1alpha1.BazarrAppConnection]())
		key, _ := values["apikey"].(string)
		if reconciler.IsMasked(key) {
			key = ""
		}
		conn["apiKeySecretRef"] = r.creds.ref(key, c.section, "api-key")
		spec[c.key] = conn
	}

	var providers []any
	for _, name := range texts(general["enabled_providers"]) {
		provider := map[string]any{"name": name}
		settings, secrets := r.bazarrProviderSettings(name, section(name))
		if len(settings)+len(secrets) > 0 {
			provider["settings"] = configFields(settings, secrets)
		}
		providers = append(providers, provider)
	}
	if len(providers) > 0 {
		spec["providers"] = providers
	}

	languages := map[string]any{}
	enabled, err := bc.EnabledLanguages(r.ctx)
	if err != nil {
		return nil, err
	}
	if len(enabled) > 0 {
		languages["enabled"] = enabled
	}
	profiles, err := bc.GetLanguageProfiles(r.ctx)
	if err != nil {
		return nil, err
	}
	bazarrDefaultProfileNames(general, profiles, spec["general"].(map[string]any))
	if len(profiles) > 0 {
		languages["profiles"] = r.bazarrProfiles(profiles)
	}
	if len(languages) > 0 {
		spec["languages"] = languages
	}
	notifications, _ := settings["notifications"].(map[string]any)
	notifiers, _ := notifications["providers"].([]any)
	var enabledProviders []any
	for _, p := range notifiers {
		provider, _ := p.(map[string]any)
		u, _ := provider["url"].(string)
		if provider["enabled"] != true && u == "" {
			continue
		}
		name, _ := provider["name"].(string)
		notification := map[string]any{"name": name, "enabled": provider["enabled"] == true}
		if u != "" {
			notification["urlSecretRef"] = r.creds.ref(u, "notification", name, "url")
		}
		enabledProviders = append(enabledProviders, notification)
	}
	if len(enabledProviders) > 0 {
		spec["notifications"] = enabledProviders
	}
	return spec, nil
}

// bazarrDefaultProfiles are settings Bazarr stores as a profile id and the
// config names; the export resolves them once it has the profiles.
var bazarrDefaultProfiles = map[string]string{"seriesDefaultProfile": "serie_default_enabled", "movieDefaultProfile": "movie_default_enabled"}

func bazarrSettings(section string, values map[string]any, t reflect.Type) map[string]any {
	out := map[string]any{}
	for field := range t.Fields() {
		name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		v, ok := values[bazarrclient.SettingName(section, name)]
		switch {
		case !ok || v == nil, bazarrDefaultProfiles[name] != "":
		case field.Type == reflect.TypeFor[[]string]():
			if items := texts(v); len(items) > 0 {
				out[name] = items
			}
		case field.Type == reflect.TypeFor[[]subtitlesv1alpha1.BazarrPathMapping]():
			var mappings []any
			pairs, _ := v.([]any)
			for _, pair := range pairs {
				if parts := texts(pair); len(parts) == 2 {
					mappings = append(mappings, map[string]any{"remotePath": parts[0], "localPath": parts[1]})
				}
			}
			if len(mappings) > 0 {
				out[name] = mappings
			}
		case field.Type.Kind() == reflect.Struct || field.Type.Kind() == reflect.Slice || field.Type.Kind() == reflect.Map:
		default:
			out[name] = v
		}
	}
	return out
}

// bazarrDefaultProfileNames names the default profiles Bazarr has turned on.
func bazarrDefaultProfileNames(general map[string]any, profiles []map[string]any, out map[string]any) {
	for field, enabled := range bazarrDefaultProfiles {
		if on, _ := general[enabled].(bool); !on {
			continue
		}
		id := general[bazarrclient.SettingName("general", field)]
		for _, p := range profiles {
			if p["profileId"] == id {
				out[field] = p["name"]
			}
		}
	}
}

func (r *reader) bazarrProviderSettings(provider string, values map[string]any) (settings, secrets map[string]any) {
	settings, secrets = map[string]any{}, map[string]any{}
	for _, key := range slices.Sorted(maps.Keys(values)) {
		var text string
		switch v := values[key].(type) {
		case nil:
			continue
		case string:
			text = v
		case float64:
			text = strconv.FormatFloat(v, 'f', -1, 64)
		case bool:
			text = fmt.Sprint(v)
		case []any:
			raw, _ := json.Marshal(v)
			text = string(raw)
		default:
			r.warn("provider %q: the config's provider settings hold text, so %s stays as set in Bazarr", provider, key)
			continue
		}
		if text == "" || text == "[]" {
			continue
		}
		if slices.ContainsFunc(bazarrSecretSettings, func(s string) bool { return strings.Contains(key, s) }) {
			if reconciler.IsMasked(text) {
				text = ""
			}
			secrets[key] = r.creds.ref(text, "provider", provider, key)
			continue
		}
		settings[key] = text
	}
	return settings, secrets
}

func (r *reader) bazarrProfiles(profiles []map[string]any) []any {
	out := make([]any, 0, len(profiles))
	for _, p := range profiles {
		name, _ := p["name"].(string)
		var items []any
		for j, it := range list(p["items"]) {
			if num(it["id"]) != float64(j+1) {
				r.warn("language profile %q: Bazarr numbers its items differently than the config, so applying "+
					"the config would renumber them", name)
			}
			items = append(items, map[string]any{
				"language":         it["language"],
				"hi":               bazarrFlag(it["hi"]),
				"forced":           bazarrFlag(it["forced"]),
				"audioExclude":     bazarrFlag(it["audio_exclude"]),
				"audioOnlyInclude": bazarrFlag(it["audio_only_include"]),
			})
		}
		profile := map[string]any{"profileId": p["profileId"], "name": name, "items": items}
		if cutoff, ok := r.bazarrCutoff(name, p["cutoff"], list(p["items"])); ok {
			profile["cutoff"] = cutoff
		}
		if words := texts(p["mustContain"]); len(words) > 0 {
			profile["mustContain"] = words
		}
		if words := texts(p["mustNotContain"]); len(words) > 0 {
			profile["mustNotContain"] = words
		}
		if p["originalFormat"] == true || num(p["originalFormat"]) == 1 {
			profile["originalFormat"] = true
		}
		if tag, _ := p["tag"].(string); tag != "" {
			profile["tag"] = tag
		}
		out = append(out, profile)
	}
	return out
}

// bazarrCutoff names a profile's cutoff item as the config does. A profile
// without a cutoff has none to export.
func (r *reader) bazarrCutoff(profile string, cutoff any, items []map[string]any) (string, bool) {
	id := num(cutoff)
	switch id {
	case 0:
		return "", false
	case bazarrclient.AnyCutoff:
		return "any", true
	}
	seen := map[string]bool{}
	for _, it := range items {
		label := bazarrclient.ItemLabel(fmt.Sprint(it["language"]), bazarrFlag(it["hi"]), bazarrFlag(it["forced"]))
		if num(it["id"]) != id {
			seen[label] = true
			continue
		}
		if seen[label] {
			r.warn("language profile %q: its cutoff is a later of two %s items, which the config can't name, "+
				"so cutoff is left out", profile, label)
			return "", false
		}
		return label, true
	}
	r.warn("language profile %q: its cutoff is an item it doesn't have, so cutoff is left out", profile)
	return "", false
}

func bazarrFlag(v any) bool {
	return v == true || v == "True"
}

func texts(v any) []string {
	raw, _ := v.([]any)
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
