package engine

import (
	"errors"
	"slices"
	"time"
)

// Pass is one reconcile of a resource: the writes it sends, gated on the drift
// policy, and what the operator owns in the app.
type Pass struct {
	Writes
	Owned *Ownership
	// PruneEnabled is set when the resource asks for prune.
	PruneEnabled bool
	// Retry asks for the next pass sooner than the resource's interval, as
	// after a first-time setup that failed.
	Retry time.Duration

	synced []string
}

// NewPass starts a pass from the resource's policy and the ownership its status
// recorded.
func NewPass(app string, policy SyncPolicy, managed map[string][]string) *Pass {
	return &Pass{
		App:          app,
		Observe:      policy.Observe,
		Owned:        NewOwnership(app, managed),
		PruneEnabled: policy.Prune,
	}
}

// Synced records an item the pass brought in line, for the status message.
func (p *Pass) Synced(name string) {
	p.synced = append(p.synced, name)
}

// SyncedItems lists what Synced recorded.
func (p *Pass) SyncedItems() []string {
	return slices.Clone(p.synced)
}

// ConfigError is a spec the operator can't act on until it changes, such as a
// malformed URL.
type ConfigError struct{ err error }

func (e *ConfigError) Error() string { return e.err.Error() }
func (e *ConfigError) Unwrap() error { return e.err }

// InvalidConfig marks err as a problem with the spec, which no retry fixes.
func InvalidConfig(err error) error {
	if err == nil {
		return nil
	}
	return &ConfigError{err: err}
}

// IsConfigError reports whether err is a problem with the spec.
func IsConfigError(err error) bool {
	_, ok := errors.AsType[*ConfigError](err)
	return ok
}
