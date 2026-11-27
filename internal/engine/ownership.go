package engine

import (
	"errors"
	"fmt"
	"slices"

	"github.com/kyleseneker/arrchitect/internal/metrics"
)

// Ownership is what an integration created, by resource type, as recorded in
// status.managedResources. Prune and cleanup only ever delete owned items.
type Ownership struct {
	app     string
	managed map[string][]string
	pruned  []PrunedResource
}

// NewOwnership starts from the ownership a resource's status recorded.
func NewOwnership(app string, managed map[string][]string) *Ownership {
	owned := make(map[string][]string, len(managed))
	for kind, names := range managed {
		owned[kind] = slices.Clone(names)
	}
	return &Ownership{app: app, managed: owned}
}

// Created records an item the operator created.
func (o *Ownership) Created(kind, name string) {
	if !slices.Contains(o.managed[kind], name) {
		o.managed[kind] = append(o.managed[kind], name)
	}
}

// Owns reports whether the operator created any item of kind. Prune and
// cleanup have nothing to delete in a kind it doesn't own.
func (o *Ownership) Owns(kind string) bool {
	return len(o.managed[kind]) > 0
}

// Candidates lists the owned items of kind that exist but are no longer
// declared. A kind with nothing declared has none, so leaving a list out of a
// resource never empties it.
func (o *Ownership) Candidates(kind string, declared, existing []string) []string {
	if len(declared) == 0 {
		return nil
	}
	return o.owned(kind, declared, existing)
}

// Prune deletes the candidates of kind.
func (o *Ownership) Prune(kind string, declared, existing []string, remove func(name string) error) error {
	return o.delete(kind, o.Candidates(kind, declared, existing), remove, false)
}

// Cleanup deletes every owned item of kind that exists, for deletionPolicy
// delete. An item the app refuses to delete is kept and the rest are still
// deleted; the kept items come back as a *KeptError.
func (o *Ownership) Cleanup(kind string, existing []string, remove func(name string) error) error {
	return o.delete(kind, o.owned(kind, nil, existing), remove, true)
}

// Managed is the ownership to record in status.
func (o *Ownership) Managed() map[string][]string {
	return o.managed
}

// Pruned lists the items deleted so far.
func (o *Ownership) Pruned() []PrunedResource {
	return o.pruned
}

func (o *Ownership) owned(kind string, declared, existing []string) []string {
	var out []string
	for _, name := range existing {
		if slices.Contains(o.managed[kind], name) && !slices.Contains(declared, name) && !slices.Contains(out, name) {
			out = append(out, name)
		}
	}
	slices.Sort(out)
	return out
}

// delete removes the given items, keeping ownership of any it fails to delete so
// a later pass retries them. With keepRefused, an item the app refuses to
// delete is passed over and returned in a *KeptError.
func (o *Ownership) delete(kind string, names []string, remove func(name string) error, keepRefused bool) error {
	if err := pruneLimit(kind, len(names)); err != nil {
		return err
	}
	kept := &KeptError{App: o.app}
	for _, name := range names {
		if err := remove(name); err != nil {
			if reason, keep := o.keep(err); keep && keepRefused {
				kept.Kept = append(kept.Kept, KeptResource{Type: kind, Name: name, Reason: reason})
				continue
			}
			failure := fmt.Errorf("deleting %s %q: %w", kind, name, err)
			if len(kept.Kept) > 0 {
				return JoinErrors(kept, failure)
			}
			return failure
		}
		o.managed[kind] = slices.DeleteFunc(o.managed[kind], func(n string) bool { return n == name })
		if len(o.managed[kind]) == 0 {
			delete(o.managed, kind)
		}
		metrics.ResourcesPrunedTotal.WithLabelValues(o.app, kind).Inc()
		o.pruned = append(o.pruned, PrunedResource{Type: kind, Name: name})
	}
	if len(kept.Kept) > 0 {
		return kept
	}
	return nil
}

// keep reports whether a failed delete is one no retry changes, with the
// reason: the app refused it, or the name is ambiguous.
func (o *Ownership) keep(err error) (string, bool) {
	if reason, refused := Refusal(err); refused {
		return AppName(o.app) + " refused to delete it: " + reason, true
	}
	if _, ambiguous := errors.AsType[*ambiguousError](err); ambiguous {
		return err.Error(), true
	}
	return "", false
}

// ambiguousError is an owned name the app holds more than once. The operator
// can't tell which item it created, so it deletes neither.
type ambiguousError struct{ msg string }

func (e *ambiguousError) Error() string { return e.msg }

// KeptError is a cleanup that deleted everything but the items no retry can
// delete: those the app refused, such as a quality profile something still
// uses, and names the app holds more than once.
type KeptError struct {
	App  string
	Kept []KeptResource
}

func (e *KeptError) Error() string {
	return fmt.Sprintf("%s refused to delete %d item(s)", AppName(e.App), len(e.Kept))
}

// SplitKept separates what an app refused to delete, which no retry will
// change, from the failures a retry may fix.
func SplitKept(err error) ([]*KeptError, error) {
	var kept []*KeptError
	var rest []error
	var walk func(error)
	walk = func(err error) {
		if err == nil {
			return
		}
		if joined, ok := err.(interface{ Unwrap() []error }); ok {
			for _, e := range joined.Unwrap() {
				walk(e)
			}
			return
		}
		if k, ok := errors.AsType[*KeptError](err); ok {
			kept = append(kept, k)
			return
		}
		rest = append(rest, err)
	}
	walk(err)
	return kept, JoinErrors(rest...)
}

func pruneLimit(kind string, n int) error {
	if n > DefaultMaxPruneCount {
		return fmt.Errorf("refusing to delete %d %s (threshold is %d); check the resource lists everything it should",
			n, kind, DefaultMaxPruneCount)
	}
	return nil
}
