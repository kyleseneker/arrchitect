package engine

import (
	"context"
	"fmt"

	"github.com/kyleseneker/arrchitect/internal/httpapi"
)

// A Builder builds one kind's items for Stages. Tagged marks a kind with items
// that name tags; Build is then given the app's tag IDs.
type Builder struct {
	Count  int
	Tagged bool
	Build  func(tags NamedIDs) ([]map[string]any, error)
}

// Stages writes an app's kinds a stage at a time, so items can name the IDs of
// what earlier stages created.
type Stages struct {
	// Order lists each stage's kinds.
	Order [][]string
	// Builders builds each kind's items.
	Builders map[string]Builder
	// TagPath lists the app's tags. They're read once per stage, for its first
	// tagged kind with items, so tags must be written in an earlier stage than
	// any kind that names them.
	TagPath string
	// PlannedTags are the tag labels observe mode may find missing that the
	// resource declares; see PlannedID.
	PlannedTags []string
}

// Reconcile builds and writes each stage in turn. A kind that can't be built,
// such as one naming a tag that doesn't exist, is left out of its stage, so it
// is neither written nor pruned, and reported; the other kinds are still
// written.
func (s Stages) Reconcile(ctx context.Context, client *httpapi.HTTPClient, def AppDefinition, pass *Pass) []error {
	var problems []error
	for _, kinds := range s.Order {
		var tags NamedIDs
		var tagsErr error
		resources := make(map[string][]map[string]any)
		for _, kind := range kinds {
			b := s.Builders[kind]
			if b.Count == 0 {
				continue
			}
			if b.Tagged && tags == nil && tagsErr == nil {
				if tags, tagsErr = FetchNamedIDs(ctx, client, s.TagPath, "label"); tagsErr == nil {
					for _, label := range s.PlannedTags {
						tags.Plan(TagLabel(label))
					}
				}
			}
			if b.Tagged && tagsErr != nil {
				problems = append(problems, fmt.Errorf("%s: %w", kind, tagsErr))
				continue
			}
			items, err := b.Build(tags)
			if err != nil {
				problems = append(problems, err)
				continue
			}
			resources[kind] = items
		}
		problems = append(problems, ReconcileApp(ctx, client, def, nil, resources, pass)...)
	}
	return problems
}
