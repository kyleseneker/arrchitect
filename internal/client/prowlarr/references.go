package prowlarr

import (
	"context"
	"fmt"

	indexersv1alpha1 "github.com/kyleseneker/arrchitect/api/indexers/v1alpha1"
	"github.com/kyleseneker/arrchitect/internal/engine"
)

// ResolveReferences operates on a copy: IDs belong to the current application,
// not to the Kubernetes spec. Missing references must never become empty tags.
// In observe mode, tags and sync profiles the spec declares resolve even when
// Prowlarr lacks them; see engine.PlannedID.
func ResolveReferences(
	ctx context.Context, client *engine.HTTPClient, spec indexersv1alpha1.ProwlarrSpec, observe bool,
) (indexersv1alpha1.ProwlarrSpec, map[string]int, error) {
	resolved := spec.DeepCopy()
	tags, err := engine.FetchNamedIDs(ctx, client, "/api/v1/tag", "label")
	if err != nil {
		return *resolved, nil, err
	}
	if observe {
		for _, t := range spec.Tags {
			tags.Plan(engine.TagLabel(t.Label))
		}
	}
	tagIDs := map[string]int{}
	check := func(resource, name string, labels []string) error {
		for _, label := range labels {
			id, err := tags.Resolve(engine.TagLabel(label))
			if err != nil {
				return fmt.Errorf("%s(%s): tag: %w", resource, name, err)
			}
			tagIDs[label] = id
		}
		return nil
	}
	for _, app := range resolved.Applications {
		if err := check("applications", app.Name, app.TagLabels); err != nil {
			return *resolved, nil, err
		}
	}
	for _, idx := range resolved.Indexers {
		if err := check("indexers", idx.Name, idx.TagLabels); err != nil {
			return *resolved, nil, err
		}
	}
	for _, proxy := range resolved.Proxies {
		if err := check("proxies", proxy.Name, proxy.TagLabels); err != nil {
			return *resolved, nil, err
		}
	}
	for _, dc := range resolved.DownloadClients {
		if err := check("downloadClients", dc.Name, dc.TagLabels); err != nil {
			return *resolved, nil, err
		}
	}
	for _, n := range resolved.Notifications {
		if err := check("notifications", n.Name, n.TagLabels); err != nil {
			return *resolved, nil, err
		}
	}
	var profiles engine.NamedIDs
	for i := range resolved.Indexers {
		idx := &resolved.Indexers[i]
		if idx.AppProfileName == "" {
			continue
		}
		if profiles == nil {
			profiles, err = engine.FetchNamedIDs(ctx, client, "/api/v1/appprofile", "name")
			if err != nil {
				return *resolved, nil, err
			}
			if observe {
				for _, p := range spec.AppProfiles {
					profiles.Plan(p.Name)
				}
			}
		}
		id, err := profiles.Resolve(idx.AppProfileName)
		if err != nil {
			return *resolved, nil, fmt.Errorf("indexers(%s): appProfileName: %w", idx.Name, err)
		}
		idx.AppProfileId = &id
	}
	return *resolved, tagIDs, nil
}
