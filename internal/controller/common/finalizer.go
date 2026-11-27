package common

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	"github.com/kyleseneker/arrchitect/internal/engine"
	"github.com/kyleseneker/arrchitect/internal/reconciler"
)

// Finalizer holds a resource with deletionPolicy delete until the operator has
// removed what it created. Other resources don't carry it, so they never wait
// on the operator to be deleted.
const Finalizer = "arrchitect.dev/finalizer"

// DeletionGiveUpAfter bounds how long deletion will retry against an
// unreachable app before releasing the finalizer, so a dead app can never
// block deletion of the resource indefinitely.
const DeletionGiveUpAfter = 10 * time.Minute

// DeletionPolicyDelete removes the resources the operator created.
const DeletionPolicyDelete = "delete"

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
// when the deletion policy is "delete", then releases the finalizer. If remove
// fails it records a warning event and retries until DeletionGiveUpAfter has
// elapsed since deletion was requested, after which the finalizer is released
// regardless.
func handleDeletion(ctx context.Context, c client.Client, recorder events.EventRecorder, obj ConfigResource, remove func(context.Context) error) (bool, error) {
	if !controllerutil.ContainsFinalizer(obj, Finalizer) {
		return false, nil
	}
	logger := log.FromContext(ctx)

	policy := ""
	if rc := obj.GetReconcileConfig(); rc != nil {
		policy = rc.DeletionPolicy
	}

	if policy == DeletionPolicyDelete {
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
			if expired(obj) {
				logger.Error(err, "giving up removing remote resources; releasing finalizer")
				if recorder != nil {
					recorder.Eventf(obj, nil, corev1.EventTypeWarning, engine.ReasonFinalizerReleased, "ReleaseFinalizer",
						"Released the finalizer after %s without removing remote resources: %v", DeletionGiveUpAfter, err)
				}
			} else {
				if recorder != nil {
					recorder.Eventf(obj, nil, corev1.EventTypeWarning, engine.ReasonCleanupFailed, "Cleanup",
						"Removing remote resources failed; retrying until the finalizer is released at %s: %v",
						obj.GetDeletionTimestamp().Add(DeletionGiveUpAfter).Format(time.RFC3339), err)
				}
				return true, err
			}
		}
	}

	// Cleanup may have used up the reconcile's deadline, and the release must
	// still land, or deletion outlasts DeletionGiveUpAfter.
	releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), statusWriteTimeout)
	defer cancel()
	controllerutil.RemoveFinalizer(obj, Finalizer)
	if err := c.Update(releaseCtx, obj); err != nil && !errors.IsNotFound(err) {
		return true, err
	}
	return true, nil
}

func expired(obj ConfigResource) bool {
	ts := obj.GetDeletionTimestamp()
	if ts == nil {
		return false
	}
	return metav1.Now().Sub(ts.Time) > DeletionGiveUpAfter
}

// Lifecycle declares which of the spec's lifecycle settings an integration
// implements. HandleLifecycle rejects a spec that asks for one the integration
// lacks, so the zero value refuses both rather than silently ignoring them.
type Lifecycle struct {
	// Remove deletes the resources the operator created, for
	// deletionPolicy: delete.
	Remove func(context.Context) error
	// Prune is set when the integration implements spec.reconcile.prune:
	// deleting resources it created once they leave the spec.
	Prune bool
	// Observe is set when another resource's driftPolicy observe governs this
	// one's writes, as an app's resource does for its indexers and download
	// clients. The finalizer is then dropped, and deleting the resource
	// removes nothing from the app.
	Observe bool
}

// HandleLifecycle performs the deletion and finalizer bookkeeping every
// controller shares. It returns done=true when the caller should stop
// reconciling: the object is being deleted, the finalizer could not be
// written, or the spec asks for a lifecycle setting lc does not support.
func HandleLifecycle(ctx context.Context, c client.Client, recorder events.EventRecorder, obj ConfigResource, lc Lifecycle) (bool, time.Duration) {
	logger := log.FromContext(ctx)
	if !obj.GetDeletionTimestamp().IsZero() {
		remove := lc.Remove
		if lc.Observe && remove != nil {
			remove = func(context.Context) error {
				if recorder != nil {
					recorder.Eventf(obj, nil, corev1.EventTypeNormal, engine.ReasonResourceKept, "Delete",
						"Removed nothing from the app, because the app's resource is in observe mode")
				}
				return nil
			}
		}
		if _, err := handleDeletion(ctx, c, recorder, obj, remove); err != nil {
			logger.Error(err, "deletion not finished; retrying")
			return true, 30 * time.Second
		}
		return true, 0
	}
	rc := obj.GetReconcileConfig()
	deletes := rc != nil && rc.DeletionPolicy == DeletionPolicyDelete
	unsupported := deletes && (lc.Remove == nil || ObserveOnly(rc))
	if err := setFinalizer(ctx, c, obj, deletes && !unsupported && !lc.Observe); err != nil {
		logger.Error(err, "updating finalizer; retrying")
		return true, 30 * time.Second
	}
	if unsupported {
		message := "deletionPolicy delete is not supported by this integration; use orphan"
		if ObserveOnly(rc) {
			message = "deletionPolicy delete conflicts with driftPolicy observe; use orphan"
		}
		UpdateStatus(ctx, c, recorder, obj, false, engine.ReasonInvalidConfig, message)
		return true, ReconcileInterval(rc)
	}
	if PruneEnabled(rc) && !lc.Prune {
		UpdateStatus(ctx, c, recorder, obj, false, engine.ReasonInvalidConfig,
			"prune is not supported by this integration; remove spec.reconcile.prune")
		return true, ReconcileInterval(rc)
	}
	return false, 0
}

// CleanupApp deletes what obj owns in the app behind connection, for
// deletionPolicy delete. It reads only the credentials deletion needs, so an
// unrelated Secret in the spec can't block it, and with nothing to delete it
// doesn't connect at all.
func CleanupApp(ctx context.Context, c client.Reader, recorder events.EventRecorder, obj ConfigResource, connection commonv1alpha1.AppConnection, def engine.AppDefinition, app string) error {
	owned := engine.NewOwnership(app, obj.GetManagedResources())
	if !def.Deletable(owned) {
		return nil
	}
	apiKey, err := reconciler.ResolveSecretKeyRef(ctx, c, obj.GetNamespace(), connection.APIKeySecretRef)
	if err != nil {
		return err
	}
	tlsConfig, err := engine.ResolveTLSConfig(ctx, c, obj.GetNamespace(), connection.TLS)
	if err != nil {
		return err
	}
	hc, err := engine.NewHTTPClient(connection.URL, engine.AuthAPIKey, engine.WithAPIKey(apiKey), engine.WithTLSConfig(tlsConfig), engine.WithAppLabel(app))
	if err != nil {
		return err
	}
	err = engine.DeleteManagedResources(ctx, hc, def, owned)
	EmitPruneEvents(recorder, obj, owned.Pruned())
	return err
}

// emitKeptEvents emits a ResourceKept warning for each item the app refused to
// delete during cleanup.
func emitKeptEvents(recorder events.EventRecorder, obj ConfigResource, kept []engine.KeptResource) {
	if recorder == nil {
		return
	}
	for _, k := range kept {
		recorder.Eventf(obj, nil, corev1.EventTypeWarning, engine.ReasonResourceKept, "Cleanup",
			"Kept %s %q, which the operator created: %s", k.Type, k.Name, k.Reason)
	}
}

// ownsNothing reports whether status.managedResources records nothing, so
// deletion has nothing to remove and needs no connection to the app.
func ownsNothing(obj ConfigResource) bool {
	for _, names := range obj.GetManagedResources() {
		if len(names) > 0 {
			return false
		}
	}
	return true
}
