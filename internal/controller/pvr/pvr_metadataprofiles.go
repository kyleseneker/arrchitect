package pvr

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	pvrv1alpha1 "github.com/kyleseneker/arrchitect/api/pvr/v1alpha1"
	"github.com/kyleseneker/arrchitect/internal/httpapi"
)

// metadataProfileBases reads the app's template for a new metadata profile and
// its current profiles by name. A declared profile starts from its current
// self, or from the template when it is new.
func metadataProfileBases(
	ctx context.Context, client *httpapi.HTTPClient, apiVersion string,
) (map[string]any, map[string]map[string]any, error) {
	schema, err := client.GetJSON(ctx, fmt.Sprintf("/api/%s/metadataprofile/schema", apiVersion))
	if err != nil {
		return nil, nil, err
	}
	delete(schema, "id")
	list, err := client.GetJSONList(ctx, fmt.Sprintf("/api/%s/metadataprofile", apiVersion))
	if err != nil {
		return nil, nil, err
	}
	current := make(map[string]map[string]any, len(list))
	for _, profile := range list {
		if name, ok := profile["name"].(string); ok {
			current[name] = profile
		}
	}
	return schema, current, nil
}

func lidarrMetadataProfileResources(
	ctx context.Context, client *httpapi.HTTPClient, apiVersion string, declared []pvrv1alpha1.LidarrMetadataProfile,
) ([]map[string]any, error) {
	schema, current, err := metadataProfileBases(ctx, client, apiVersion)
	if err != nil {
		return nil, err
	}
	profiles := make([]map[string]any, 0, len(declared))
	for _, mp := range declared {
		from, exists := current[mp.Name]
		if !exists {
			from = schema
		}
		payload := map[string]any{"name": mp.Name}
		for _, list := range []struct {
			field, key string
			allowed    []string
		}{
			{"primaryAlbumTypes", "albumType", mp.PrimaryAlbumTypes},
			{"secondaryAlbumTypes", "albumType", mp.SecondaryAlbumTypes},
			{"releaseStatuses", "releaseStatus", mp.ReleaseStatuses},
		} {
			if list.allowed == nil {
				if !exists {
					payload[list.field] = from[list.field]
				}
				continue
			}
			items, err := allowedItems(from[list.field], list.key, list.allowed)
			if err != nil {
				return nil, fmt.Errorf("metadataProfiles(%s): %s: %w", mp.Name, list.field, err)
			}
			payload[list.field] = items
		}
		profiles = append(profiles, payload)
	}
	return profiles, nil
}

// allowedItems copies a profile's list of choices, allowing the named ones and
// disallowing the rest.
func allowedItems(list any, key string, allowed []string) ([]any, error) {
	items, _ := list.([]any)
	out := make([]any, 0, len(items))
	known := map[string]bool{}
	for _, entry := range items {
		item, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		choice, _ := item[key].(map[string]any)
		name, _ := choice["name"].(string)
		known[name] = true
		item = maps.Clone(item)
		item["allowed"] = slices.Contains(allowed, name)
		out = append(out, item)
	}
	for _, name := range allowed {
		if !known[name] {
			return nil, fmt.Errorf("%q isn't one of the app's choices", name)
		}
	}
	return out, nil
}

func readarrMetadataProfileResources(
	ctx context.Context, client *httpapi.HTTPClient, apiVersion string, declared []pvrv1alpha1.ReadarrMetadataProfile,
) ([]map[string]any, error) {
	schema, current, err := metadataProfileBases(ctx, client, apiVersion)
	if err != nil {
		return nil, err
	}
	profiles := make([]map[string]any, 0, len(declared))
	for _, mp := range declared {
		payload := map[string]any{}
		existing, exists := current[mp.Name]
		if !exists {
			payload = maps.Clone(schema)
		}
		payload["name"] = mp.Name
		set := func(key string, value any, declared bool) {
			if declared {
				payload[key] = value
			}
		}
		set("minPopularity", deref(mp.MinPopularity), mp.MinPopularity != nil)
		set("skipMissingDate", deref(mp.SkipMissingDate), mp.SkipMissingDate != nil)
		set("skipMissingIsbn", deref(mp.SkipMissingIsbn), mp.SkipMissingIsbn != nil)
		set("skipPartsAndSets", deref(mp.SkipPartsAndSets), mp.SkipPartsAndSets != nil)
		set("skipSeriesSecondary", deref(mp.SkipSeriesSecondary), mp.SkipSeriesSecondary != nil)
		set("minPages", deref(mp.MinPages), mp.MinPages != nil)
		set("allowedLanguages", languages(existing["allowedLanguages"], mp.AllowedLanguages), mp.AllowedLanguages != nil)
		set("ignored", mp.Ignored, mp.Ignored != nil)
		profiles = append(profiles, payload)
	}
	return profiles, nil
}

// languages is Readarr's comma-separated form of codes. Readarr writes its own
// defaults as "eng, null", so a stored value with the same codes is kept as it
// is rather than rewritten.
func languages(stored any, codes []string) string {
	if s, ok := stored.(string); ok {
		var have []string
		for code := range strings.SplitSeq(s, ",") {
			if code = strings.TrimSpace(code); code != "" {
				have = append(have, code)
			}
		}
		if slices.Equal(slices.Sorted(slices.Values(have)), slices.Sorted(slices.Values(codes))) {
			return s
		}
	}
	return strings.Join(codes, ",")
}

func deref[T any](p *T) T {
	var zero T
	if p == nil {
		return zero
	}
	return *p
}
