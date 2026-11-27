package engine

import (
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/kyleseneker/arrchitect/internal/reconciler"
)

// Pass is one reconcile of a resource: the writes it sends, gated on the drift
// policy, and what the operator owns in the app.
type Pass struct {
	Writes
	Owned *Ownership
	// Owner names the resource the pass is for, so the secrets it writes are
	// remembered apart from other resources' and forgotten when it is deleted.
	Owner string
	// PruneEnabled is set when the resource asks for prune.
	PruneEnabled bool
	// Retry asks for the next pass sooner than the resource's interval, as
	// after a first-time setup that failed.
	Retry time.Duration

	synced []string
}

// NewPass starts a pass for owner from its policy and the ownership its status
// recorded.
func NewPass(app, owner string, policy SyncPolicy, managed map[string][]string) *Pass {
	return &Pass{
		App:          app,
		Observe:      policy.Observe,
		Owned:        NewOwnership(app, managed),
		Owner:        owner,
		PruneEnabled: policy.Prune,
	}
}

// SecretsDiffer reports whether the masked secrets in an item, hashed into
// digest, differ from what this resource last wrote to key.
func (p *Pass) SecretsDiffer(key, digest string) bool {
	return reconciler.SecretsDiffer(p.Owner, key, digest, p.Observe)
}

// RecordSecrets remembers the masked secrets this resource wrote to key.
func (p *Pass) RecordSecrets(key, digest string) {
	reconciler.RecordSecrets(p.Owner, key, digest)
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

// JoinErrors reports errs as one failure, nil when there are none. Unlike
// errors.Join it separates them with "; ", which reads on one line in a
// status message. errors.Is and errors.As still find each one.
func JoinErrors(errs ...error) error {
	var kept joinedErrors
	for _, err := range errs {
		if err != nil {
			kept = append(kept, err)
		}
	}
	if len(kept) == 0 {
		return nil
	}
	return kept
}

type joinedErrors []error

func (e joinedErrors) Error() string {
	texts := make([]string, len(e))
	for i, err := range e {
		texts[i] = err.Error()
	}
	return strings.Join(texts, "; ")
}

func (e joinedErrors) Unwrap() []error { return e }

// ErrUnauthorized is an app refusing the credentials the operator sent.
var ErrUnauthorized = errors.New("the app refused the credentials")

// IsUnauthorized reports whether err is an app refusing the credentials.
func IsUnauthorized(err error) bool {
	if errors.Is(err, ErrUnauthorized) {
		return true
	}
	apiErr, ok := errors.AsType[*APIError](err)
	return ok && (apiErr.StatusCode == http.StatusUnauthorized || apiErr.StatusCode == http.StatusForbidden)
}
