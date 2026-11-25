package bazarr

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"
	"slices"
	"strings"

	subtitlesv1alpha1 "github.com/kyleseneker/arrchitect/api/subtitles/v1alpha1"
	"github.com/kyleseneker/arrchitect/internal/engine"
)

// Client wraps engine.HTTPClient for Bazarr's form-encoded API.
type Client struct {
	hc *engine.HTTPClient
}

// NewClient creates a new Bazarr API client.
func NewClient(hc *engine.HTTPClient) *Client {
	return &Client{hc: hc}
}

// Ping checks if Bazarr is reachable.
func (c *Client) Ping(ctx context.Context) error {
	return c.hc.Ping(ctx, "/api/system/health")
}

// AnyCutoff is the cutoff Bazarr stores for "any": the search ends at whichever
// item is found first.
const AnyCutoff = 65535

// NewProfileDefaults are the profile keys Bazarr's languages-profiles handler
// reads unconditionally, with the values a new profile gets when the spec
// leaves them unset. A profile missing any of them raises a KeyError and no
// profile is stored.
var NewProfileDefaults = map[string]any{"cutoff": nil, "mustContain": []string{}, "mustNotContain": []string{}, "originalFormat": false}

// ItemLabel names a profile item as Bazarr's cutoff selector shows it.
func ItemLabel(language string, hi, forced bool) string {
	switch {
	case hi:
		return language + ":hi"
	case forced:
		return language + ":forced"
	}
	return language
}

// buildLanguageProfiles renders profiles in the shape Bazarr's languages-profiles
// handler takes, with only the optional keys the spec sets.
func buildLanguageProfiles(profiles []subtitlesv1alpha1.BazarrLanguageProfile) ([]map[string]any, error) {
	out := make([]map[string]any, 0, len(profiles))
	for i, p := range profiles {
		id := i + 1
		if p.ProfileID != nil {
			id = int(*p.ProfileID)
		}
		items := make([]map[string]any, 0, len(p.Items))
		for j, it := range p.Items {
			items = append(items, map[string]any{
				"id":                 j + 1,
				"language":           it.Language,
				"hi":                 boolStr(it.HI),
				"forced":             boolStr(it.Forced),
				"audio_exclude":      boolStr(it.AudioExclude),
				"audio_only_include": boolStr(it.AudioOnlyInclude),
			})
		}
		profile := map[string]any{"profileId": id, "name": p.Name, "items": items}
		if p.Cutoff != "" {
			cutoff, err := cutoffID(p)
			if err != nil {
				return nil, err
			}
			profile["cutoff"] = cutoff
		}
		if p.MustContain != nil {
			profile["mustContain"] = slices.Clone(p.MustContain)
		}
		if p.MustNotContain != nil {
			profile["mustNotContain"] = slices.Clone(p.MustNotContain)
		}
		if p.OriginalFormat != nil {
			profile["originalFormat"] = *p.OriginalFormat
		}
		if p.Tag != "" {
			profile["tag"] = p.Tag
		}
		out = append(out, profile)
	}
	return out, nil
}

// cutoffID is the item id Bazarr stores for a profile's cutoff.
func cutoffID(p subtitlesv1alpha1.BazarrLanguageProfile) (any, error) {
	switch p.Cutoff {
	case "none":
		return nil, nil
	case "any":
		return AnyCutoff, nil
	}
	for j, it := range p.Items {
		if ItemLabel(it.Language, it.HI != nil && *it.HI, it.Forced != nil && *it.Forced) == p.Cutoff {
			return j + 1, nil
		}
	}
	return nil, fmt.Errorf("profile %q: cutoff %q names none of its items", p.Name, p.Cutoff)
}

// boolStr renders a bool the way Bazarr stores profile item flags.
func boolStr(b *bool) string {
	if b != nil && *b {
		return "True"
	}
	return "False"
}

// GetSettings returns every Bazarr setting, keyed by section.
func (c *Client) GetSettings(ctx context.Context) (map[string]any, error) {
	return c.hc.GetJSON(ctx, "/api/system/settings")
}

// EnabledLanguagesField is the settings form field that sets which languages
// are enabled, one value per language. Bazarr disables every language it
// doesn't list.
const EnabledLanguagesField = "languages-enabled"

// EnabledLanguages returns the ISO 639-1 codes of the enabled languages.
func (c *Client) EnabledLanguages(ctx context.Context) ([]string, error) {
	languages, err := c.hc.GetJSONList(ctx, "/api/system/languages")
	if err != nil {
		return nil, err
	}
	var enabled []string
	for _, l := range languages {
		if code, _ := l["code2"].(string); code != "" && l["enabled"] == true {
			enabled = append(enabled, code)
		}
	}
	return enabled, nil
}

// GetLanguageProfiles returns the stored language profiles.
func (c *Client) GetLanguageProfiles(ctx context.Context) ([]map[string]any, error) {
	return c.hc.GetJSONList(ctx, "/api/system/languages/profiles")
}

// JellyfinLibrary is a movie or TV library Bazarr finds on a Jellyfin server.
type JellyfinLibrary struct {
	ID   string
	Name string
	// Type is movies or tvshows.
	Type string
}

// JellyfinLibraries lists the movie and TV libraries Bazarr finds on the
// Jellyfin at jellyfinURL. Bazarr answers an unreachable or refusing server
// with no libraries rather than an error.
func (c *Client) JellyfinLibraries(ctx context.Context, jellyfinURL, apiKey string) ([]JellyfinLibrary, error) {
	query := url.Values{"url": {jellyfinURL}, "apikey": {apiKey}}
	body, err := c.hc.GetJSON(ctx, "/api/jellyfin/libraries?"+query.Encode())
	if err != nil {
		return nil, err
	}
	items, _ := body["data"].([]any)
	libraries := make([]JellyfinLibrary, 0, len(items))
	for _, item := range items {
		lib, _ := item.(map[string]any)
		id, _ := lib["id"].(string)
		name, _ := lib["name"].(string)
		kind, _ := lib["type"].(string)
		libraries = append(libraries, JellyfinLibrary{ID: id, Name: name, Type: kind})
	}
	return libraries, nil
}

// PostForm posts a raw form to the given path.
func (c *Client) PostForm(ctx context.Context, path string, form url.Values) error {
	_, err := c.hc.DoForm(ctx, path, form)
	return err
}

// LanguagesForm renders enabled languages and profiles as the settings form
// fields Bazarr accepts. It is empty when neither is declared.
func LanguagesForm(langs *subtitlesv1alpha1.BazarrLanguages) (url.Values, error) {
	form := url.Values{}
	if len(langs.Enabled) > 0 {
		form[EnabledLanguagesField] = slices.Clone(langs.Enabled)
	}
	if len(langs.Profiles) > 0 {
		profiles, err := buildLanguageProfiles(langs.Profiles)
		if err != nil {
			return nil, err
		}
		profilesJSON, err := json.Marshal(profiles)
		if err != nil {
			return nil, fmt.Errorf("marshaling language profiles: %w", err)
		}
		form.Set("languages-profiles", string(profilesJSON))
	}
	return form, nil
}

// settingNameOverrides maps a spec field to the Bazarr setting it drives where
// snake_case alone does not get there.
var settingNameOverrides = map[string]map[string]string{
	"general": {
		"seriesDefaultProfile": "serie_default_profile", "movieDefaultProfile": "movie_default_profile",
		"seriesTagEnabled": "serie_tag_enabled",
	},
	"sonarr": {"host": "ip", "basePath": "base_url", "syncInterval": "series_sync"},
	"radarr": {"host": "ip", "basePath": "base_url", "syncInterval": "movies_sync"},
}

// SettingName converts a spec field name to the key Bazarr stores it under.
// Bazarr accepts and persists unknown keys without complaint, so a wrong name
// is silently inert rather than an error.
func SettingName(section, jsonName string) string {
	if o, ok := settingNameOverrides[section][jsonName]; ok {
		return o
	}
	var b strings.Builder
	for i, r := range jsonName {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				b.WriteByte('_')
			}
			b.WriteRune(r + 32)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// StructToFormData converts a struct to form data with "settings-{section}-{field}=value" keys.
// Skips nil pointers and empty strings. A list of strings is one value per
// item, and an empty one a single empty value, which Bazarr stores as empty.
func StructToFormData(section string, obj any) url.Values {
	form := url.Values{}
	v := reflect.ValueOf(obj)
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return form
		}
		v = v.Elem()
	}
	t := v.Type()

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		fv := v.Field(i)

		jsonTag := field.Tag.Get("json")
		if jsonTag == "" || jsonTag == "-" {
			continue
		}
		jsonName, _, _ := strings.Cut(jsonTag, ",")

		if fv.Kind() == reflect.Pointer && fv.IsNil() {
			continue
		}
		if fv.Kind() == reflect.String && fv.String() == "" {
			continue
		}
		key := fmt.Sprintf("settings-%s-%s", section, SettingName(section, jsonName))
		if items, ok := reflect.TypeAssert[[]string](fv); ok {
			if items != nil {
				form[key] = append([]string{}, items...)
				if len(items) == 0 {
					form[key] = []string{""}
				}
			}
			continue
		}
		// Only scalars are settings. A struct such as a Secret reference would
		// otherwise be stored in Bazarr's config as its printed form.
		if k := reflect.Indirect(fv).Kind(); k == reflect.Struct || k == reflect.Slice || k == reflect.Map {
			continue
		}

		var val string
		switch fv.Kind() {
		case reflect.Pointer:
			val = fmt.Sprintf("%v", fv.Elem().Interface())
		default:
			val = fmt.Sprintf("%v", fv.Interface())
		}
		form.Set(key, val)
	}
	return form
}
