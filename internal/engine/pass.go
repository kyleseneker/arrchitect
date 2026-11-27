package engine

import (
	"slices"
	"time"

	"github.com/kyleseneker/arrchitect/internal/secrets"
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
	return secrets.SecretsDiffer(p.Owner, key, digest, p.Observe)
}

// RecordSecrets remembers the masked secrets this resource wrote to key.
func (p *Pass) RecordSecrets(key, digest string) {
	secrets.RecordSecrets(p.Owner, key, digest)
}

// Synced records an item the pass brought in line, for the status message.
func (p *Pass) Synced(name string) {
	p.synced = append(p.synced, name)
}

// SyncedItems lists what Synced recorded.
func (p *Pass) SyncedItems() []string {
	return slices.Clone(p.synced)
}
