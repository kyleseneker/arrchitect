package engine

import (
	"context"
	"fmt"
	"slices"
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

// Listed returns what the app holds of the kind.
func (k Kind) Listed(ctx context.Context, app string) (Items, error) {
	items, err := k.List(ctx)
	if err != nil {
		return Items{}, err
	}
	field := k.NameField
	if field == "" {
		field = "name"
	}
	return listItems(app, k.Name, items, func(item map[string]any) string {
		name, _ := item[field].(string)
		return name
	}), nil
}

// Items is what an app holds of one kind: each name once, with the ids of the
// items holding it.
type Items struct {
	Names     []string
	app, kind string
	ids       map[string][]int
}

func listItems(app, kind string, items []map[string]any, key func(map[string]any) string) Items {
	out := Items{Names: make([]string, 0, len(items)), app: app, kind: kind, ids: make(map[string][]int, len(items))}
	for _, item := range items {
		name := key(item)
		id, ok := item["id"].(float64)
		if name == "" || !ok {
			continue
		}
		if _, seen := out.ids[name]; !seen {
			out.Names = append(out.Names, name)
		}
		out.ids[name] = append(out.ids[name], int(id))
	}
	return out
}

// ID is the id of the one item named name. Two items with one name can't be
// told apart, so neither is deleted.
func (i Items) ID(name string) (int, error) {
	if n := len(i.ids[name]); n != 1 {
		return 0, &ambiguousError{msg: fmt.Sprintf("%s has %d %s named %q", AppName(i.app), n, i.kind, name)}
	}
	return i.ids[name][0], nil
}

// PruneKinds prunes each kind in order against what declared names for it. It
// skips a kind a write failed for this pass, and never lists a kind with
// nothing declared or nothing owned: a scoped API key may not be allowed to.
func (p *Pass) PruneKinds(ctx context.Context, kinds []Kind, declared map[string][]string, failed map[string]bool) []error {
	if !p.PruneEnabled {
		return nil
	}
	var problems []error
	for _, k := range kinds {
		if failed[k.Name] || len(declared[k.Name]) == 0 || !p.Owned.Owns(k.Name) {
			continue
		}
		items, err := k.Listed(ctx, p.App)
		if err == nil {
			err = p.PruneItems(k.Name, declared[k.Name], items, func(id int) error { return k.Remove(ctx, id) })
		}
		if err != nil {
			problems = append(problems, fmt.Errorf("%s(prune): %w", k.Name, err))
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
		items, err := k.Listed(ctx, o.app)
		if err == nil {
			err = o.Cleanup(k.Name, items.Names, removeByID(ctx, k, items))
		}
		errs = append(errs, err)
	}
	return JoinErrors(errs...)
}

func removeByID(ctx context.Context, k Kind, items Items) func(string) error {
	return func(name string) error {
		id, err := items.ID(name)
		if err != nil {
			return err
		}
		return k.Remove(ctx, id)
	}
}

// PruneItems prunes the owned items of kind that declared leaves out. A name
// the app holds more than once is left alone in either mode and reported, and
// the rest are still pruned.
func (p *Pass) PruneItems(kind string, declared []string, items Items, remove func(id int) error) error {
	var ambiguous []error
	keep := slices.Clone(declared)
	for _, name := range p.Owned.Candidates(kind, declared, items.Names) {
		if _, err := items.ID(name); err != nil {
			ambiguous = append(ambiguous, err)
			keep = append(keep, name)
		}
	}
	err := p.Prune(p.Owned, kind, keep, items.Names, func(name string) error {
		id, err := items.ID(name)
		if err != nil {
			return err
		}
		return remove(id)
	})
	return JoinErrors(append(ambiguous, err)...)
}
