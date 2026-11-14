package engine

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strings"
)

// NamedIDs maps a name to every ID an app assigned it. Duplicates are retained
// so a reference can never silently choose one of multiple resources.
// Unreferenced duplicates do not block partial management.
type NamedIDs map[string][]int

// FetchNamedIDs lists path and indexes each item's ID by the given name field.
func FetchNamedIDs(ctx context.Context, client *HTTPClient, path, field string) (NamedIDs, error) {
	items, err := client.GetJSONList(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("reading references from %s: %w", path, err)
	}
	return NamedIDsOf(items, field), nil
}

// NamedIDsOf indexes each item's ID by the given name field. An ID that isn't
// a positive whole number is recorded as 0.
func NamedIDsOf(items []map[string]any, field string) NamedIDs {
	ids := NamedIDs{}
	for _, item := range items {
		name, _ := item[field].(string)
		id, _ := item["id"].(float64)
		if id <= 0 || id != math.Trunc(id) || id > math.MaxInt32 {
			id = 0
		}
		ids[name] = append(ids[name], int(id))
	}
	return ids
}

// Find returns the ID of the single item with the given name; found is false
// when none has it. More than one is an error, so a reference never picks one
// of several items silently.
func (ids NamedIDs) Find(name string) (id int, found bool, err error) {
	switch values := ids[name]; len(values) {
	case 0:
		return 0, false, nil
	case 1:
		return values[0], true, nil
	default:
		return 0, true, fmt.Errorf("reference %q is ambiguous (%d matches)", name, len(values))
	}
}

// PlannedID stands for an item observe mode found the resource declares and
// the app lacks. A payload naming it differs from what the app has, so the
// item that refers to it is reported as a difference; observe mode writes
// nothing, so the ID never reaches the app.
const PlannedID = -1

// Plan adds the names among declared that ids lacks, as PlannedID. Only
// observe mode plans: enforcing creates the items before they are referenced.
func (ids NamedIDs) Plan(declared ...string) {
	for _, name := range declared {
		if _, ok := ids[name]; !ok {
			ids[name] = []int{PlannedID}
		}
	}
}

// Resolve returns the ID of the single resource with the given name.
func (ids NamedIDs) Resolve(name string) (int, error) {
	id, found, err := ids.Find(name)
	switch {
	case err != nil:
		return 0, err
	case !found:
		return 0, fmt.Errorf("reference %q does not exist", name)
	case id <= 0 && id != PlannedID:
		return 0, fmt.Errorf("reference %q has an invalid ID", name)
	}
	return id, nil
}

// ResolveTags resolves tag labels to IDs. Without labels, the legacy numeric
// IDs are returned unchanged, and an empty label list with no IDs is no tags.
// Nil means neither was declared.
func (ids NamedIDs) ResolveTags(labels []string, legacy []int) ([]int, error) {
	if len(labels) == 0 {
		if labels != nil && legacy == nil {
			return []int{}, nil
		}
		return legacy, nil
	}
	out := make([]int, 0, len(labels))
	for _, label := range labels {
		id, err := ids.Resolve(TagLabel(label))
		if err != nil {
			return nil, fmt.Errorf("tag: %w", err)
		}
		if !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	return out, nil
}

// TagLabel is a tag label as the apps store it. Sonarr, Radarr, Lidarr, Readarr
// and Prowlarr lowercase a label when they save it.
func TagLabel(label string) string {
	return strings.ToLower(label)
}
