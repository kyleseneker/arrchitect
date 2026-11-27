package reconciler

import (
	"time"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	"github.com/kyleseneker/arrchitect/internal/engine"
)

const DriftPolicyObserve = "observe"

// ReconcileInterval returns the configured interval or the default.
func ReconcileInterval(rc *commonv1alpha1.ReconcileConfig) time.Duration {
	if rc != nil && rc.IntervalSeconds != 0 {
		return time.Duration(rc.IntervalSeconds) * time.Second
	}
	return DefaultReconcileInterval
}

// PruneEnabled returns whether resource pruning is enabled.
func PruneEnabled(rc *commonv1alpha1.ReconcileConfig) bool {
	return rc != nil && rc.Prune != nil && *rc.Prune
}

// ObserveOnly reports whether drift should be recorded without writing.
func ObserveOnly(rc *commonv1alpha1.ReconcileConfig) bool {
	return rc != nil && rc.DriftPolicy == DriftPolicyObserve
}

// Policy renders the reconcile policy for a CR.
func Policy(rc *commonv1alpha1.ReconcileConfig) engine.SyncPolicy {
	return engine.SyncPolicy{Prune: PruneEnabled(rc), Observe: ObserveOnly(rc)}
}
