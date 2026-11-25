package automation

import (
	"context"
	"errors"
	"fmt"
	"slices"

	automationv1alpha1 "github.com/kyleseneker/arrchitect/api/automation/v1alpha1"
	"github.com/kyleseneker/arrchitect/internal/engine"
)

const actionsKind = "actions"

// kinds returns the kinds in the order they are deleted, each before what it
// refers to: lists before the filters and download clients they name, and
// filters before the download clients their actions use.
func (s *autobrrSync) kinds() []engine.Kind {
	return []engine.Kind{
		{Name: "lists", List: s.ac.ListLists, Remove: s.ac.DeleteList},
		{Name: "filters", List: s.ac.ListFilters, Remove: s.ac.DeleteFilter},
		{Name: "notifications", List: s.ac.ListNotifications, Remove: s.ac.DeleteNotification},
		{Name: "feeds", List: s.ac.ListFeeds, Remove: s.ac.DeleteFeed},
		{Name: "ircNetworks", List: s.ac.ListIRCNetworks, Remove: s.ac.DeleteIRCNetwork},
		{Name: "indexers", List: s.ac.ListIndexers, Remove: s.ac.DeleteIndexer},
		{Name: "downloadClients", List: s.ac.ListDownloadClients, Remove: s.ac.DeleteDownloadClient},
		{Name: "proxies", List: s.ac.ListProxies, Remove: s.ac.DeleteProxy},
	}
}

func declaredNames(spec *automationv1alpha1.AutobrrSpec) map[string][]string {
	declared := map[string][]string{}
	for _, f := range spec.Filters {
		declared["filters"] = append(declared["filters"], f.Name)
	}
	for _, f := range spec.Feeds {
		declared["feeds"] = append(declared["feeds"], f.Name)
	}
	for _, n := range spec.IRCNetworks {
		declared["ircNetworks"] = append(declared["ircNetworks"], n.Name)
	}
	for _, i := range spec.Indexers {
		declared["indexers"] = append(declared["indexers"], i.Name)
	}
	for _, d := range spec.DownloadClients {
		declared["downloadClients"] = append(declared["downloadClients"], d.Name)
	}
	for _, n := range spec.Notifications {
		declared["notifications"] = append(declared["notifications"], n.Name)
	}
	for _, l := range spec.Lists {
		declared["lists"] = append(declared["lists"], l.Name)
	}
	for _, p := range spec.Proxies {
		declared["proxies"] = append(declared["proxies"], p.Name)
	}
	return declared
}

// pruneUndeclared deletes what the operator created that the spec no longer declares,
// skipping any kind a write failed for this pass.
func (s *autobrrSync) pruneUndeclared(ctx context.Context, spec *automationv1alpha1.AutobrrSpec, failed map[string]bool) []string {
	var errs []string
	if !failed["filters"] {
		for _, f := range spec.Filters {
			if err := s.pruneActions(ctx, f); err != nil {
				errs = append(errs, fmt.Sprintf("actions(%s, prune): %v", f.Name, err))
			}
		}
	}
	return append(errs, s.PruneKinds(ctx, s.kinds(), declaredNames(spec), failed)...)
}

func (s *autobrrSync) pruneActions(ctx context.Context, filter automationv1alpha1.AutobrrFilter) error {
	filters, err := engine.Kind{Name: "filters", List: s.ac.ListFilters}.Listed(ctx, "autobrr")
	if err != nil || !slices.Contains(filters.Names, filter.Name) {
		return err
	}
	filterID, err := filters.ID(filter.Name)
	if err != nil {
		return err
	}
	existing, err := s.filterActions(ctx, filter.Name, filterID)
	if err != nil {
		return err
	}
	declared := make([]string, 0, len(filter.Actions))
	for _, a := range filter.Actions {
		declared = append(declared, filter.Name+"/"+a.Name)
	}
	return s.Prune(s.Owned, actionsKind, declared, existing.Names, s.deleteAction(ctx, existing))
}

// cleanup deletes everything the operator created, for deletionPolicy delete.
func (s *autobrrSync) cleanup(ctx context.Context) error {
	var errs []error
	if s.Owned.Owns(actionsKind) {
		filters, err := engine.Kind{Name: "filters", List: s.ac.ListFilters}.Listed(ctx, "autobrr")
		if err != nil {
			return err
		}
		for _, name := range filters.Names {
			filterID, err := filters.ID(name)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			existing, err := s.filterActions(ctx, name, filterID)
			if err == nil {
				err = s.Owned.Cleanup(actionsKind, existing.Names, s.deleteAction(ctx, existing))
			}
			errs = append(errs, err)
		}
	}
	return errors.Join(append(errs, s.Owned.CleanupKinds(ctx, s.kinds()))...)
}

// filterActions lists a filter's actions as <filter>/<action>.
func (s *autobrrSync) filterActions(ctx context.Context, filter string, filterID int) (engine.Items, error) {
	return engine.Kind{Name: actionsKind, List: func(ctx context.Context) ([]map[string]any, error) {
		actions, err := s.ac.ListActions(ctx, filterID)
		for _, a := range actions {
			if name, ok := a["name"].(string); ok {
				a["name"] = filter + "/" + name
			}
		}
		return actions, err
	}}.Listed(ctx, "autobrr")
}

func (s *autobrrSync) deleteAction(ctx context.Context, actions engine.Items) func(string) error {
	return func(name string) error {
		id, err := actions.ID(name)
		if err != nil {
			return err
		}
		return s.ac.DeleteAction(ctx, id)
	}
}
