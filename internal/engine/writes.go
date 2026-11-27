package engine

import (
	"github.com/kyleseneker/arrchitect/internal/metrics"
	"github.com/kyleseneker/arrchitect/internal/reconciler"
)

// Writes gates the writes an integration sends through its own client on the
// drift policy. Enforce mode sends each write and counts it in
// drift_corrected_total; observe mode sends nothing and records the difference
// in drift_observed_total instead. The zero value enforces.
type Writes struct {
	// App is the metrics label for the target app.
	App string
	// Observe records differences instead of writing them.
	Observe bool

	drift []string
}

// Apply sends write, or in observe mode records the difference it would have
// corrected.
func (w *Writes) Apply(kind, name string, write func() error) error {
	if w.Observe {
		w.RecordDrift(kind, name)
		return nil
	}
	metrics.DriftCorrectedTotal.WithLabelValues(w.App, kind, name).Inc()
	return write()
}

// Prune deletes what owned.Prune would, or in observe mode records each item it
// would delete as a difference.
func (w *Writes) Prune(owned *Ownership, kind string, declared, existing []string, remove func(name string) error) error {
	if !w.Observe {
		return owned.Prune(kind, declared, existing, remove)
	}
	candidates := owned.Candidates(kind, declared, existing)
	if err := pruneLimit(kind, len(candidates)); err != nil {
		return err
	}
	for _, name := range candidates {
		w.RecordDrift(kind, name)
	}
	return nil
}

// Cleanup deletes what owned.Cleanup would, or in observe mode records each item
// it would delete as a difference, under the name label gives it.
func (w *Writes) Cleanup(owned *Ownership, kind string, existing []string, label func(string) string, remove func(name string) error) error {
	if !w.Observe {
		return owned.Cleanup(kind, existing, remove)
	}
	candidates := owned.owned(kind, nil, existing)
	if err := pruneLimit(kind, len(candidates)); err != nil {
		return err
	}
	for _, name := range candidates {
		w.RecordDrift(kind, label(name))
	}
	return nil
}

// Change is one item a batched write carries.
type Change struct {
	Kind, Name string
}

// ApplyBatch sends one write that carries several changes, such as a settings
// form that saves many keys at once, counting or recording each change.
func (w *Writes) ApplyBatch(changes []Change, write func() error) error {
	if len(changes) == 0 {
		return nil
	}
	if w.Observe {
		for _, c := range changes {
			w.RecordDrift(c.Kind, c.Name)
		}
		return nil
	}
	for _, c := range changes {
		metrics.DriftCorrectedTotal.WithLabelValues(w.App, c.Kind, c.Name).Inc()
	}
	return write()
}

// RecordDrift records a difference observe mode found without a write to
// withhold, such as one that depends on a resource observe did not create.
func (w *Writes) RecordDrift(kind, name string) {
	metrics.DriftObservedTotal.WithLabelValues(w.App, kind, name).Inc()
	w.drift = append(w.drift, kind+"("+name+")")
}

// Sync sends write, with desired merged over current, when the two differ in
// the fields desired sets. A nil current is an item the app doesn't have yet.
func (w *Writes) Sync(kind, name string, current map[string]any, desired any, write func(merged map[string]any) error) error {
	out, err := reconciler.MergeDesired(current, desired)
	if err != nil {
		return err
	}
	if current != nil && !out.Changed {
		return nil
	}
	return w.Apply(kind, name, func() error { return write(out.Merged) })
}

// Drift lists the differences observe mode recorded, as kind(name).
func (w *Writes) Drift() []string {
	return w.drift
}
