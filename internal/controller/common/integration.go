package common

import (
	"context"
	"errors"
	"time"

	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kyleseneker/arrchitect/internal/engine"
	"github.com/kyleseneker/arrchitect/internal/reconciler"
)

// Integration is how the operator reaches one app and syncs a resource to it.
// Reconcile runs the steps every integration shares around it.
type Integration[T ConfigResource, C any] struct {
	// App is the app's label, such as "sonarr".
	App string
	// Connect builds the app's client and signs in where the app needs it.
	// Cleanup uses it too. Its error sets the status reason and when to retry,
	// through Classify.
	Connect func(ctx context.Context, obj T) (C, error)
	// Check runs after Connect before a sync, never before cleanup: a health
	// probe, and spec checks that mustn't block deleting what the operator
	// created. Its error is classified like Connect's.
	Check func(ctx context.Context, conn C, obj T) error
	// Sync brings the app in line with obj, through pass, and returns what
	// failed. Each failure leaves the rest of the pass to run.
	Sync func(ctx context.Context, conn C, obj T, pass *engine.Pass) []string
	// Cleanup deletes the items owned records, for deletionPolicy delete. Nil
	// when the integration doesn't support it.
	Cleanup func(ctx context.Context, conn C, owned *engine.Ownership) error
	// Deletable reports whether owned holds anything Cleanup deletes, so a
	// deletion with nothing to remove reads no credentials. Nil counts anything
	// owned.
	Deletable func(owned *engine.Ownership) bool
	// Prune is set when Sync implements spec.reconcile.prune.
	Prune bool
}

// Reconcile reads the resource req names into obj and runs one pass.
func (in Integration[T, C]) Reconcile(
	ctx context.Context, c client.Client, recorder events.EventRecorder, req ctrl.Request, obj T,
) (ctrl.Result, error) {
	if err := c.Get(ctx, req.NamespacedName, obj); err != nil {
		return ctrl.Result{}, IgnoreDeleted(obj, req.NamespacedName, err)
	}

	lifecycle := Lifecycle{Prune: in.Prune}
	if in.Cleanup != nil {
		lifecycle.Remove = func(ctx context.Context) error {
			owned := engine.NewOwnership(in.App, obj.GetManagedResources())
			if in.Deletable != nil && !in.Deletable(owned) {
				return nil
			}
			conn, err := in.Connect(ctx, obj)
			if err != nil {
				return err
			}
			err = in.Cleanup(ctx, conn, owned)
			EmitPruneEvents(recorder, obj, owned.Pruned())
			return err
		}
	}
	if done, after := HandleLifecycle(ctx, c, recorder, obj, lifecycle); done {
		return ctrl.Result{RequeueAfter: after}, nil
	}

	conn, err := in.Connect(ctx, obj)
	if err == nil && in.Check != nil {
		err = in.Check(ctx, conn, obj)
	}
	if err != nil {
		reason, base := Classify(err)
		after := RetryAfter(obj, base)
		log.FromContext(ctx).Error(err, "connecting to "+engine.AppName(in.App), "reason", reason, "retryAfter", after)
		UpdateStatus(ctx, c, recorder, obj, false, reason, err.Error())
		return ctrl.Result{RequeueAfter: after}, nil
	}
	Reached(obj)

	rc := obj.GetReconcileConfig()
	pass := engine.NewPass(in.App, Policy(rc), obj.GetManagedResources())
	problems := in.Sync(ctx, conn, obj, pass)
	UpdateStatusManaged(obj, pass.Owned.Managed())
	EmitPruneEvents(recorder, obj, pass.Owned.Pruned())
	ReportResult(ctx, c, recorder, obj, pass.Observe, engine.ReconcileResult{
		Synced: pass.SyncedItems(), Errors: problems, Drift: pass.Drift(),
	})
	after := NextSync(rc)
	if pass.Retry > 0 && pass.Retry < after {
		after = pass.Retry
	}
	return ctrl.Result{RequeueAfter: after}, nil
}

// Classify is the status reason for a failure to reach an app, and when to try
// again. A spec problem waits for the spec to change, which requeues the
// resource anyway.
func Classify(err error) (string, time.Duration) {
	_, invalidRef := errors.AsType[*invalidAppRefError](err)
	switch {
	case IsAppRefNotFound(err):
		return engine.ReasonReferenceNotFound, 30 * time.Second
	case invalidRef, engine.IsConfigError(err):
		return engine.ReasonInvalidConfig, 0
	case reconciler.IsSecretError(err):
		return engine.ReasonSecretNotFound, 30 * time.Second
	case engine.IsUnauthorized(err):
		// A Secret change requeues the resource, and some apps lock out a
		// client that keeps failing to sign in.
		return engine.ReasonUnauthorized, 5 * time.Minute
	default:
		return engine.ReasonAppUnreachable, 60 * time.Second
	}
}
