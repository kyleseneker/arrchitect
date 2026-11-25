package engine

import (
	"context"
	"errors"
	"fmt"
)

// Kind is one kind of item an app lists and deletes by id, for prune and
// cleanup.
type Kind struct {
	// Name is the kind's key in status.managedResources.
	Name string
	// NameField holds an item's name; "name" when empty.
	NameField string
	List      func(ctx context.Context) ([]map[string]any, error)
	Remove    func(ctx context.Context, id int) error
}

// Listed returns the names of the kind's items and the id of each.
func (k Kind) Listed(ctx context.Context) ([]string, map[string]int, error) {
	items, err := k.List(ctx)
	if err != nil {
		return nil, nil, err
	}
	field := k.NameField
	if field == "" {
		field = "name"
	}
	names := make([]string, 0, len(items))
	ids := make(map[string]int, len(items))
	for _, item := range items {
		name, _ := item[field].(string)
		id, ok := item["id"].(float64)
		if name == "" || !ok {
			continue
		}
		names = append(names, name)
		ids[name] = int(id)
	}
	return names, ids, nil
}

// PruneKinds prunes each kind in order against what declared names for it. It
// skips a kind a write failed for this pass, and never lists a kind with
// nothing declared or nothing owned: a scoped API key may not be allowed to.
func (p *Pass) PruneKinds(ctx context.Context, kinds []Kind, declared map[string][]string, failed map[string]bool) []string {
	if !p.PruneEnabled {
		return nil
	}
	var problems []string
	for _, k := range kinds {
		if failed[k.Name] || len(declared[k.Name]) == 0 || !p.Owned.Owns(k.Name) {
			continue
		}
		names, ids, err := k.Listed(ctx)
		if err == nil {
			err = p.Prune(p.Owned, k.Name, declared[k.Name], names, func(name string) error { return k.Remove(ctx, ids[name]) })
		}
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s(prune): %v", k.Name, err))
		}
	}
	return problems
}

// CleanupKinds deletes the owned items of each kind in order, for
// deletionPolicy delete. Like prune, it never lists a kind it owns nothing of.
func (o *Ownership) CleanupKinds(ctx context.Context, kinds []Kind) error {
	var errs []error
	for _, k := range kinds {
		if !o.Owns(k.Name) {
			continue
		}
		names, ids, err := k.Listed(ctx)
		if err == nil {
			err = o.Cleanup(k.Name, names, func(name string) error { return k.Remove(ctx, ids[name]) })
		}
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}
