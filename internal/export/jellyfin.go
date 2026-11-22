package export

import (
	"fmt"
	"reflect"
	"slices"
	"strings"

	mediaserversv1alpha1 "github.com/kyleseneker/arrchitect/api/mediaservers/v1alpha1"
	jellyfinclient "github.com/kyleseneker/arrchitect/internal/client/jellyfin"
)

func exportJellyfin(r *reader) (map[string]any, error) {
	jf := jellyfinclient.NewClient(r.hc)
	if err := jf.Authenticate(r.ctx, r.username, r.password); err != nil {
		return nil, err
	}
	server, err := jf.GetConfig(r.ctx, "/System/Configuration")
	if err != nil {
		return nil, err
	}
	encoding, err := jf.GetConfig(r.ctx, "/System/Configuration/encoding")
	if err != nil {
		return nil, err
	}
	network, err := jf.GetConfig(r.ctx, "/System/Configuration/network")
	if err != nil {
		return nil, err
	}
	spec := map[string]any{
		"adminUser": map[string]any{
			"usernameSecretRef": r.creds.ref(r.username, "username"),
			"passwordSecretRef": r.creds.ref(r.password, "password"),
		},
		"server":   jellyfinSettings(server, reflect.TypeFor[mediaserversv1alpha1.JellyfinServer]()),
		"encoding": jellyfinSettings(encoding, reflect.TypeFor[mediaserversv1alpha1.JellyfinEncoding]()),
		"network":  jellyfinSettings(network, reflect.TypeFor[mediaserversv1alpha1.JellyfinNetwork]()),
	}

	folders, err := jf.ListLibraries(r.ctx)
	if err != nil {
		return nil, err
	}
	var libraries []any
	for _, f := range folders {
		collection, _ := f["CollectionType"].(string)
		if collection == "" {
			collection = "mixed"
		}
		library := map[string]any{"name": f["Name"], "collectionType": collection, "paths": f["Locations"]}
		options, _ := f["LibraryOptions"].(map[string]any)
		for field, v := range jellyfinSettings(options, reflect.TypeFor[mediaserversv1alpha1.JellyfinLibrary]()) {
			if s, isString := v.(string); field == "name" || field == "collectionType" || field == "paths" || field == "typeOptions" ||
				isString && s == "" {
				continue
			}
			library[field] = v
		}
		r.jellyfinFetchers(library, options)
		libraries = append(libraries, library)
	}
	if len(libraries) > 0 {
		spec["libraries"] = libraries
	}
	return spec, nil
}

func jellyfinSettings(values map[string]any, t reflect.Type) map[string]any {
	out := map[string]any{}
	for field := range t.Fields() {
		name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		v, ok := values[pascal(name)]
		if !ok || v == nil {
			continue
		}
		if nested, isMap := v.(map[string]any); isMap && field.Type.Kind() == reflect.Pointer && field.Type.Elem().Kind() == reflect.Struct {
			v = jellyfinSettings(nested, field.Type.Elem())
		}
		out[name] = v
	}
	return out
}

func pascal(name string) string {
	return strings.ToUpper(name[:1]) + name[1:]
}

var jellyfinItemTypes = map[string][]string{
	"movies": {"Movie"}, "tvshows": {"Series", "Season", "Episode"}, "music": {"MusicArtist", "MusicAlbum", "Audio", "MusicVideo"},
	"musicvideos": {"MusicVideo"}, "homevideos": {"Video", "Photo"}, "books": {"Book"}, "mixed": {"Series", "Season", "Episode", "Movie"},
}

// jellyfinFetchers exports a library's fetchers as the ones used, in priority
// order. Jellyfin keeps the ones a library disables and those a type option
// enables, each beside an order.
func (r *reader) jellyfinFetchers(library, options map[string]any) {
	name := library["name"]
	for field, keys := range map[string][2]string{
		"subtitleFetchers":      {"DisabledSubtitleFetchers", "SubtitleFetcherOrder"},
		"lyricFetchers":         {"DisabledLyricFetchers", "LyricFetcherOrder"},
		"mediaSegmentProviders": {"DisabledMediaSegmentProviders", "MediaSegmentProviderOrder"},
		"localMetadataReaders":  {"DisabledLocalMetadataReaders", "LocalMetadataReaderOrder"},
	} {
		disabled, order := stringValues(options[keys[0]]), stringValues(options[keys[1]])
		enabled := slices.DeleteFunc(order, func(n string) bool { return slices.Contains(disabled, n) })
		switch {
		case len(enabled) > 0:
			library[field] = enabled
		case len(disabled) > 0:
			r.warn("library %q: the config can't express its %s without an order that enables one, so they aren't exported", name, field)
		}
	}
	var typeOptions []any
	for _, t := range list(options["TypeOptions"]) {
		itemType, _ := t["Type"].(string)
		if !slices.Contains(jellyfinItemTypes[fmt.Sprint(library["collectionType"])], itemType) {
			r.warn("library %q: the config can't express options for %s, which its collection type doesn't hold, so they aren't exported", name, itemType)
			continue
		}
		entry := map[string]any{"type": itemType}
		for kind, field := range map[string]string{"Metadata": "metadataFetchers", "Image": "imageFetchers"} {
			enabled := stringValues(t[kind+"Fetchers"])
			if len(enabled) == 0 {
				r.warn("library %q: the config can't express %s with every %s fetcher disabled, so they aren't exported", name, itemType, strings.ToLower(kind))
				continue
			}
			order := stringValues(t[kind+"FetcherOrder"])
			ordered := slices.DeleteFunc(order, func(n string) bool { return !slices.Contains(enabled, n) })
			for _, n := range enabled {
				if !slices.Contains(ordered, n) {
					ordered = append(ordered, n)
				}
			}
			entry[field] = ordered
		}
		if len(entry) > 1 {
			typeOptions = append(typeOptions, entry)
		}
	}
	if len(typeOptions) > 0 {
		library["typeOptions"] = typeOptions
	}
}

func stringValues(v any) []string {
	raw, _ := v.([]any)
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
