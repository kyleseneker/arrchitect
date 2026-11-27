package pvr

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strconv"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	"github.com/kyleseneker/arrchitect/internal/engine"
	"github.com/kyleseneker/arrchitect/internal/httpapi"
)

// buildQualityProfilePayload builds the API payload for a quality profile.
// cfIDs maps custom format names to their IDs for resolving formatItems references.
func buildQualityProfilePayload(qp commonv1alpha1.QualityProfile, cfIDs map[string]int) map[string]any {
	payload := map[string]any{
		"name": qp.Name,
	}

	if qp.UpgradeAllowed != nil {
		payload["upgradeAllowed"] = *qp.UpgradeAllowed
	}
	if qp.MinFormatScore != nil {
		payload["minFormatScore"] = *qp.MinFormatScore
	}
	if qp.CutoffFormatScore != nil {
		payload["cutoffFormatScore"] = *qp.CutoffFormatScore
	}
	if qp.MinUpgradeFormatScore != nil {
		payload["minUpgradeFormatScore"] = *qp.MinUpgradeFormatScore
	}

	if len(qp.Items) > 0 {
		items := buildQualityItems(qp.Items)
		payload["items"] = items

		if qp.Cutoff != "" {
			if id, ok := findCutoffID(qp.Items, qp.Cutoff); ok {
				payload["cutoff"] = id
			}
		}
	}

	if len(qp.FormatItems) > 0 {
		formatItems := make([]map[string]any, 0, len(qp.FormatItems))
		for _, fi := range qp.FormatItems {
			if id, ok := cfIDs[fi.Name]; ok {
				formatItems = append(formatItems, map[string]any{
					"format": id,
					"name":   fi.Name,
					"score":  scoreOf(fi.Score),
				})
			}
		}
		payload["formatItems"] = formatItems
	}

	return payload
}

// buildQualityItems converts CRD quality profile items to API payload format.
func buildQualityItems(items []commonv1alpha1.QualityProfileItem) []map[string]any {
	result := make([]map[string]any, len(items))
	for i, item := range items {
		m := map[string]any{}

		if item.Quality != (commonv1alpha1.QualityReference{}) {
			m["quality"] = map[string]any{
				"id":   qualityID(item.Quality.ID),
				"name": item.Quality.Name,
			}
			m["items"] = []any{}
		} else {
			// Quality group. The PVR apps treat an item without a positive id as a
			// single quality, so a group must carry its 1000+index id; findCutoffID
			// resolves a group cutoff to the same id.
			m["id"] = 1000 + i
			m["name"] = item.Name
			children := make([]map[string]any, len(item.Items))
			for j, child := range item.Items {
				childAllowed := child.Allowed == nil || *child.Allowed
				children[j] = map[string]any{
					"quality": map[string]any{
						"id":   qualityID(child.Quality.ID),
						"name": child.Quality.Name,
					},
					"items":   []any{},
					"allowed": childAllowed,
				}
			}
			m["items"] = children
		}

		if item.Allowed != nil {
			m["allowed"] = *item.Allowed
		} else {
			m["allowed"] = true
		}

		result[i] = m
	}
	return result
}

// findCutoffID resolves a cutoff name (quality name or group name) to the corresponding ID.
// For individual qualities, returns quality.id. For groups, returns the group item index + 1000
// (the IDs these apps give quality groups).
func findCutoffID(items []commonv1alpha1.QualityProfileItem, cutoffName string) (int, bool) {
	for i, item := range items {
		if item.Quality != (commonv1alpha1.QualityReference{}) && item.Quality.Name == cutoffName {
			return qualityID(item.Quality.ID), true
		}
		if item.Quality == (commonv1alpha1.QualityReference{}) && item.Name == cutoffName {
			return 1000 + i, true
		}
	}
	return 0, false
}

func qualityProfileResources(
	ctx context.Context, client *httpapi.HTTPClient, apiVersion string, profiles []commonv1alpha1.QualityProfile, plannedFormats []string,
) ([]map[string]any, error) {
	formats, err := engine.FetchNamedIDs(ctx, client, fmt.Sprintf("/api/%s/customformat", apiVersion), "name")
	if err != nil {
		return nil, err
	}
	formats.Plan(plannedFormats...)
	definitions, err := client.GetJSONList(ctx, fmt.Sprintf("/api/%s/qualitydefinition", apiVersion))
	if err != nil {
		return nil, err
	}
	current, err := client.GetJSONList(ctx, fmt.Sprintf("/api/%s/qualityprofile", apiVersion))
	if err != nil {
		return nil, err
	}
	existing := map[string]map[string]any{}
	for _, profile := range current {
		name, _ := profile["name"].(string)
		existing[name] = profile
	}
	languages, err := profileLanguages(ctx, client, apiVersion, profiles)
	if err != nil {
		return nil, err
	}
	var qps []map[string]any
	for _, qp := range profiles {
		cfIDs := map[string]int{}
		for _, format := range qp.FormatItems {
			id, err := formats.Resolve(format.Name)
			if err != nil {
				return nil, fmt.Errorf("qualityProfiles(%s): %w", qp.Name, err)
			}
			cfIDs[format.Name] = id
		}
		payload := buildQualityProfilePayload(qp, cfIDs)
		if items, ok := payload["items"].([]map[string]any); ok {
			if profile, ok := existing[qp.Name]; ok {
				keepGroupIDs(payload, profile)
			}
			payload["items"] = completeQualityItems(items, qp.Items, definitions)
		}
		if _, ok := existing[qp.Name]; !ok {
			declared, _ := payload["formatItems"].([]map[string]any)
			payload["formatItems"] = completeFormatItems(declared, formats)
		}
		if qp.Language != "" {
			id, ok := languages[qp.Language]
			if !ok {
				return nil, fmt.Errorf("qualityProfiles(%s): language %q does not exist", qp.Name, qp.Language)
			}
			payload["language"] = map[string]any{"id": id, "name": qp.Language}
		}
		qps = append(qps, payload)
	}
	return qps, nil
}

// profileLanguages maps the app's language names to their IDs, read only when
// a profile names a language.
func profileLanguages(
	ctx context.Context, client *httpapi.HTTPClient, apiVersion string, profiles []commonv1alpha1.QualityProfile,
) (map[string]any, error) {
	if !slices.ContainsFunc(profiles, func(qp commonv1alpha1.QualityProfile) bool { return qp.Language != "" }) {
		return nil, nil
	}
	list, err := client.GetJSONList(ctx, fmt.Sprintf("/api/%s/language", apiVersion))
	if err != nil {
		return nil, err
	}
	languages := make(map[string]any, len(list))
	for _, l := range list {
		if name, ok := l["name"].(string); ok {
			languages[name] = l["id"]
		}
	}
	return languages, nil
}

func qualityName(item map[string]any) string {
	quality, _ := item["quality"].(map[string]any)
	name, _ := quality["name"].(string)
	return name
}

func qualityDefinitionResources(
	ctx context.Context, client *httpapi.HTTPClient, apiVersion string, declared []commonv1alpha1.QualityDefinition,
) ([]map[string]any, error) {
	current, err := client.GetJSONList(ctx, fmt.Sprintf("/api/%s/qualitydefinition", apiVersion))
	if err != nil {
		return nil, err
	}
	byName := make(map[string]map[string]any, len(current))
	preferred := false
	for _, definition := range current {
		byName[qualityName(definition)] = definition
		_, has := definition["preferredSize"]
		preferred = preferred || has
	}
	out := make([]map[string]any, 0, len(declared))
	for _, qd := range declared {
		definition, ok := byName[qd.Quality]
		if !ok {
			return nil, fmt.Errorf("qualityDefinitions(%s): the app has no such quality", qd.Quality)
		}
		payload, err := qualityDefinitionPayload(qd, definition, preferred)
		if err != nil {
			return nil, fmt.Errorf("qualityDefinitions(%s): %w", qd.Quality, err)
		}
		out = append(out, payload)
	}
	return out, nil
}

// qualityDefinitionPayload builds an update for one definition. Readarr has no
// preferred size; the other apps leave it out only where it's unlimited.
func qualityDefinitionPayload(qd commonv1alpha1.QualityDefinition, current map[string]any, preferred bool) (map[string]any, error) {
	payload := map[string]any{"quality": map[string]any{"name": qd.Quality}}
	if qd.Title != "" {
		payload["title"] = qd.Title
	}
	if qd.MinSize != nil {
		payload["minSize"] = json.Number(*qd.MinSize)
	}
	if qd.MaxSize != nil {
		if err := setSize(payload, current, "maxSize", *qd.MaxSize); err != nil {
			return nil, err
		}
	}
	if qd.PreferredSize != nil {
		if !preferred {
			return nil, fmt.Errorf("this app has no preferred size")
		}
		if err := setSize(payload, current, "preferredSize", *qd.PreferredSize); err != nil {
			return nil, err
		}
	}
	return payload, nil
}

// setSize writes 0 as the app's unlimited, null. The apps leave a null out of
// what they return and already treat a maximum of 0 as unlimited, so a size
// the app has as unlimited is left out.
func setSize(payload, current map[string]any, key string, size commonv1alpha1.Decimal) error {
	n, err := strconv.ParseFloat(string(size), 64)
	if err != nil {
		return fmt.Errorf("%s: %w", key, err)
	}
	if n != 0 {
		payload[key] = json.Number(size)
		return nil
	}
	have, set := current[key].(float64)
	if set && (have != 0 || key == "preferredSize") {
		payload[key] = nil
	}
	return nil
}

// keepGroupIDs gives each declared quality group the id it already has in the
// app's profile of the same name, and new groups the lowest free ids. The apps
// number groups themselves, so ids from a group's position would rewrite every
// profile the app created.
func keepGroupIDs(payload, profile map[string]any) {
	existing := map[string]int{}
	rawItems, _ := profile["items"].([]any)
	for _, raw := range rawItems {
		item, _ := raw.(map[string]any)
		if _, single := item["quality"].(map[string]any); single {
			continue
		}
		name, _ := item["name"].(string)
		if id, ok := item["id"].(float64); ok {
			existing[name] = int(id)
		}
	}
	items, _ := payload["items"].([]map[string]any)
	used := map[int]bool{}
	remap := map[int]int{}
	var unmatched []map[string]any
	for _, item := range items {
		if _, single := item["quality"]; single {
			continue
		}
		name, _ := item["name"].(string)
		if id, ok := existing[name]; ok && !used[id] {
			remap[item["id"].(int)] = id
			item["id"] = id
			used[id] = true
			continue
		}
		unmatched = append(unmatched, item)
	}
	next := 1000
	for _, item := range unmatched {
		for used[next] {
			next++
		}
		remap[item["id"].(int)] = next
		item["id"] = next
		used[next] = true
	}
	if cutoff, ok := payload["cutoff"].(int); ok {
		if id, ok := remap[cutoff]; ok {
			payload["cutoff"] = id
		}
	}
}

// completeFormatItems lists every custom format the app has, scoring the
// undeclared ones 0. A new profile must name each custom format exactly once;
// an existing one already does, because the apps add each new format to every
// profile, so updates send only the declared scores and leave others' scores be.
func completeFormatItems(declared []map[string]any, formats engine.NamedIDs) []map[string]any {
	out := append([]map[string]any{}, declared...)
	seen := map[string]bool{}
	for _, item := range declared {
		name, _ := item["name"].(string)
		seen[name] = true
	}
	names := make([]string, 0, len(formats))
	for name := range formats {
		if !seen[name] {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		for _, id := range formats[name] {
			out = append(out, map[string]any{"format": id, "name": name, "score": 0})
		}
	}
	return out
}

// completeQualityItems sends each quality as the app defines it, and adds every
// quality the profile does not mention as a disallowed item ahead of the
// declared ones. The PVR apps reject a profile that omits any quality, and read
// qualities back with app-specific keys (source, resolution, modifier) that a
// bare id and name would strip on every write.
func completeQualityItems(items []map[string]any, declared []commonv1alpha1.QualityProfileItem, definitions []map[string]any) []map[string]any {
	byID := map[int]map[string]any{}
	var order []int
	for _, definition := range definitions {
		quality, _ := definition["quality"].(map[string]any)
		raw, ok := quality["id"].(float64)
		if !ok {
			continue
		}
		byID[int(raw)] = quality
		order = append(order, int(raw))
	}
	var resolve func(item map[string]any)
	resolve = func(item map[string]any) {
		if quality, ok := item["quality"].(map[string]any); ok {
			if id, ok := quality["id"].(int); ok && byID[id] != nil {
				item["quality"] = maps.Clone(byID[id])
			}
		}
		if children, ok := item["items"].([]map[string]any); ok {
			for _, child := range children {
				resolve(child)
			}
		}
	}
	for _, item := range items {
		resolve(item)
	}

	mentioned := map[int]bool{}
	for _, item := range declared {
		if item.Quality != (commonv1alpha1.QualityReference{}) {
			mentioned[qualityID(item.Quality.ID)] = true
		}
		for _, child := range item.Items {
			mentioned[qualityID(child.Quality.ID)] = true
		}
	}
	var missing []map[string]any
	for _, id := range order {
		if mentioned[id] {
			continue
		}
		mentioned[id] = true
		missing = append(missing, map[string]any{
			"quality": maps.Clone(byID[id]),
			"items":   []any{},
			"allowed": false,
		})
	}
	return append(missing, items...)
}

// qualityID is a declared quality's ID; the API server requires one.
func qualityID(id *int32) int {
	if id == nil {
		return 0
	}
	return int(*id)
}

// scoreOf is a declared format score; the API server requires one.
func scoreOf(score *int32) int32 {
	if score == nil {
		return 0
	}
	return *score
}
