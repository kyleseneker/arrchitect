package reconciler

import (
	"time"

	"k8s.io/apimachinery/pkg/util/wait"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	"github.com/kyleseneker/arrchitect/internal/engine"
)

// driftPolicyObserve records drift without writing to the app.
const driftPolicyObserve = "observe"

// jitter is the fraction an interval is stretched by at random, so resources
// that synced together don't all reach their app at once.
const jitter = 0.1

// reconcileInterval returns the configured interval or the default.
func reconcileInterval(rc *commonv1alpha1.ReconcileConfig) time.Duration {
	if rc != nil && rc.IntervalSeconds != 0 {
		return time.Duration(rc.IntervalSeconds) * time.Second
	}
	return DefaultReconcileInterval
}

// pruneEnabled returns whether resource pruning is enabled.
func pruneEnabled(rc *commonv1alpha1.ReconcileConfig) bool {
	return rc != nil && rc.Prune != nil && *rc.Prune
}

// ObserveOnly reports whether drift should be recorded without writing.
func ObserveOnly(rc *commonv1alpha1.ReconcileConfig) bool {
	return rc != nil && rc.DriftPolicy == driftPolicyObserve
}

// policy renders the reconcile policy for a CR.
func policy(rc *commonv1alpha1.ReconcileConfig) engine.SyncPolicy {
	return engine.SyncPolicy{Prune: pruneEnabled(rc), Observe: ObserveOnly(rc)}
}

// nextSync is when a pass that ended without backing off reconciles again.
func nextSync(rc *commonv1alpha1.ReconcileConfig) time.Duration {
	return wait.Jitter(reconcileInterval(rc), jitter)
}
