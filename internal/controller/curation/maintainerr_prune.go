package curation

import (
	"context"

	curationv1alpha1 "github.com/kyleseneker/arrchitect/api/curation/v1alpha1"
	maintainerrclient "github.com/kyleseneker/arrchitect/internal/client/maintainerr"
	"github.com/kyleseneker/arrchitect/internal/engine"
)

const ruleGroupsKind = "ruleGroups"

// kinds returns the kinds in the order they are deleted: Maintainerr refuses to
// delete a server while a rule group's collection uses it, and rule groups
// refer to notifications.
func (s *maintainerrSync) kinds() []engine.Kind {
	return []engine.Kind{
		{Name: ruleGroupsKind, List: s.mc.ListRuleGroups, Remove: s.mc.DeleteRuleGroup},
		{Name: notificationsKind, List: s.mc.ListNotifications, Remove: s.mc.DeleteNotification},
		s.servers(maintainerrclient.Sonarr),
		s.servers(maintainerrclient.Radarr),
	}
}

func (s *maintainerrSync) servers(kind maintainerrclient.Arr) engine.Kind {
	return engine.Kind{
		Name:      string(kind),
		NameField: "serverName",
		List:      func(ctx context.Context) ([]map[string]any, error) { return s.mc.ListArr(ctx, kind) },
		Remove:    func(ctx context.Context, id int) error { return s.mc.DeleteArr(ctx, kind, id) },
	}
}

func maintainerrDeclared(spec *curationv1alpha1.MaintainerrSpec) map[string][]string {
	declared := map[string][]string{}
	for _, g := range spec.RuleGroups {
		declared[ruleGroupsKind] = append(declared[ruleGroupsKind], g.Name)
	}
	for _, n := range spec.Notifications {
		declared[notificationsKind] = append(declared[notificationsKind], n.Name)
	}
	for _, srv := range spec.Sonarr {
		declared[string(maintainerrclient.Sonarr)] = append(declared[string(maintainerrclient.Sonarr)], srv.Name)
	}
	for _, srv := range spec.Radarr {
		declared[string(maintainerrclient.Radarr)] = append(declared[string(maintainerrclient.Radarr)], srv.Name)
	}
	return declared
}

// pruneUndeclared deletes what the operator created that the spec no longer
// declares, skipping any kind a write failed for this pass.
func (s *maintainerrSync) pruneUndeclared(ctx context.Context, spec *curationv1alpha1.MaintainerrSpec, failed map[string]bool) []error {
	return s.PruneKinds(ctx, s.kinds(), maintainerrDeclared(spec), failed)
}

// cleanup deletes everything the operator created, for deletionPolicy delete.
func (s *maintainerrSync) cleanup(ctx context.Context) error {
	return s.Owned.CleanupKinds(ctx, s.kinds())
}
