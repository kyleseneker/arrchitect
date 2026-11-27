package reconciler

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	"github.com/kyleseneker/arrchitect/internal/engine"
	"github.com/kyleseneker/arrchitect/internal/failure"
)

// Finalizer holds a resource with deletionPolicy delete until the operator has
// removed what it created. Other resources don't carry it, so they never wait
// on the operator to be deleted.
const Finalizer = "arrchitect.dev/finalizer"

// deletionGiveUpAfter bounds how long deletion retries against an unreachable
// app. The first retry after it releases the finalizer, so a dead app can
// never block deleting the resource.
const deletionGiveUpAfter = 10 * time.Minute

// DeletionPolicyDelete removes the resources the operator created.
const DeletionPolicyDelete = "delete"

// lifecycle is which of the spec's lifecycle settings an integration
// implements. A spec that asks for one the integration lacks is refused, so
// the zero value refuses both rather than silently ignoring them.
type lifecycle struct {
	// remove deletes the resources the operator created, for
	// deletionPolicy: delete.
	remove func(context.Context) error
	// prune is set when the integration implements spec.reconcile.prune:
	// deleting resources it created once they leave the spec.
	prune bool
	// observe is set when another resource's driftPolicy observe governs this
	// one's writes, as an app's resource does for its indexers and download
	// clients. The finalizer is then dropped, and deleting the resource
	// removes nothing from the app.
	observe bool
}

// handleLifecycle does the deletion and finalizer bookkeeping that comes
// before a pass. It reports deleting when the resource is being deleted, which
// ends the pass. A failure to clean up or to write the finalizer is returned,
// so it backs off.
func handleLifecycle(ctx context.Context, c client.Client, recorder events.EventRecorder, obj ConfigResource, lc lifecycle) (bool, error) {
	if !obj.GetDeletionTimestamp().IsZero() {
		remove := lc.remove
		if lc.observe && remove != nil {
			remove = func(context.Context) error {
				if recorder != nil {
					recorder.Eventf(obj, nil, corev1.EventTypeNormal, ReasonResourceKept, "Delete",
						"Removed nothing from the app, because the app's resource is in observe mode")
				}
				return nil
			}
		}
		return true, handleDeletion(ctx, c, recorder, obj, remove)
	}
	rc := obj.GetReconcileConfig()
	want := deletes(rc) && deletionRefused(rc, lc) == nil && !lc.observe
	if err := setFinalizer(ctx, c, obj, want); err != nil {
		return false, fmt.Errorf("updating finalizer: %w", err)
	}
	return false, nil
}

func deletes(rc *commonv1alpha1.ReconcileConfig) bool {
	return rc != nil && rc.DeletionPolicy == DeletionPolicyDelete
}

// refusal is a lifecycle setting the spec asks for and lc lacks, which no
// retry fixes.
func refusal(rc *commonv1alpha1.ReconcileConfig, lc lifecycle) error {
	if err := deletionRefused(rc, lc); err != nil {
		return err
	}
	if pruneEnabled(rc) && !lc.prune {
		return failure.InvalidConfig.Errorf("prune is not supported by this integration; remove spec.reconcile.prune")
	}
	return nil
}

func deletionRefused(rc *commonv1alpha1.ReconcileConfig, lc lifecycle) error {
	switch {
	case !deletes(rc):
		return nil
	case ObserveOnly(rc):
		return failure.InvalidConfig.Errorf("deletionPolicy delete conflicts with driftPolicy observe; use orphan")
	case lc.remove == nil:
		return failure.InvalidConfig.Errorf("deletionPolicy delete is not supported by this integration; use orphan")
	}
	return nil
}

// setFinalizer adds the finalizer when want is set and removes it otherwise.
func setFinalizer(ctx context.Context, c client.Client, obj ConfigResource, want bool) error {
	if controllerutil.ContainsFinalizer(obj, Finalizer) == want {
		return nil
	}
	if want {
		controllerutil.AddFinalizer(obj, Finalizer)
	} else {
		controllerutil.RemoveFinalizer(obj, Finalizer)
	}
	return c.Update(ctx, obj)
}

// handleDeletion runs when the resource is being deleted. It calls remove only
// when the deletion policy is "delete", then releases the finalizer. A remove
// that fails records a warning event and is returned to back off, until a
// retry finds deletionGiveUpAfter has passed since deletion was requested and
// releases the finalizer regardless.
func handleDeletion(ctx context.Context, c client.Client, recorder events.EventRecorder, obj ConfigResource, remove func(context.Context) error) error {
	if !controllerutil.ContainsFinalizer(obj, Finalizer) {
		return nil
	}
	if deletes(obj.GetReconcileConfig()) {
		// Observe forbids application changes, including cleanup during deletion.
		// Unsupported cleanup must be reported instead of silently orphaning.
		var cleanupErr error
		switch {
		case ObserveOnly(obj.GetReconcileConfig()):
			cleanupErr = fmt.Errorf("deletionPolicy delete conflicts with driftPolicy observe")
		case remove == nil:
			cleanupErr = fmt.Errorf("deletionPolicy delete is not supported by this integration")
		case ownsNothing(obj):
		default:
			var kept []*engine.KeptError
			kept, cleanupErr = engine.SplitKept(remove(ctx))
			for _, k := range kept {
				emitKeptEvents(recorder, obj, k.Kept)
			}
		}
		if err := cleanupErr; err != nil {
			if !expired(obj) {
				if recorder != nil {
					recorder.Eventf(obj, nil, corev1.EventTypeWarning, ReasonCleanupFailed, "Cleanup",
						"Removing remote resources failed; retrying, and releasing the finalizer at the first retry after %s: %v",
						obj.GetDeletionTimestamp().Add(deletionGiveUpAfter).Format(time.RFC3339), err)
				}
				return fmt.Errorf("removing remote resources: %w", err)
			}
			log.FromContext(ctx).Error(err, "giving up removing remote resources; releasing finalizer")
			if recorder != nil {
				recorder.Eventf(obj, nil, corev1.EventTypeWarning, ReasonFinalizerReleased, "ReleaseFinalizer",
					"Released the finalizer after %s without removing remote resources: %v", deletionGiveUpAfter, err)
			}
		}
	}

	// Cleanup may have used up the reconcile's deadline, and the release must
	// still land, or deletion outlasts deletionGiveUpAfter.
	releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), statusWriteTimeout)
	defer cancel()
	controllerutil.RemoveFinalizer(obj, Finalizer)
	if err := c.Update(releaseCtx, obj); client.IgnoreNotFound(err) != nil {
		return fmt.Errorf("releasing finalizer: %w", err)
	}
	return nil
}

func expired(obj ConfigResource) bool {
	ts := obj.GetDeletionTimestamp()
	if ts == nil {
		return false
	}
	return metav1.Now().Sub(ts.Time) > deletionGiveUpAfter
}

// emitKeptEvents emits a ResourceKept warning for each item the app refused to
// delete during cleanup.
func emitKeptEvents(recorder events.EventRecorder, obj ConfigResource, kept []engine.KeptResource) {
	if recorder == nil {
		return
	}
	for _, k := range kept {
		recorder.Eventf(obj, nil, corev1.EventTypeWarning, ReasonResourceKept, "Cleanup",
			"Kept %s %q, which the operator created: %s", k.Type, k.Name, k.Reason)
	}
}

// ownsNothing reports whether obj owns nothing, so deletion has nothing to
// remove and needs no connection to the app.
func ownsNothing(obj ConfigResource) bool {
	for _, names := range owned(obj) {
		if len(names) > 0 {
			return false
		}
	}
	return true
}
