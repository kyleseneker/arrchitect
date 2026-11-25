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
	// Connect builds the app's client and checks the app answers. Its error
	// sets the status reason and when to retry, through Classify.
	Connect func(ctx context.Context, obj T) (C, error)
	// Sync brings the app in line with obj, through pass, and returns what
	// failed. Each failure leaves the rest of the pass to run.
	Sync func(ctx context.Context, conn C, obj T, pass *engine.Pass) []string
	// Cleanup deletes the items owned records, for deletionPolicy delete. Nil
	// when the integration doesn't support it.
	Cleanup func(ctx context.Context, conn C, owned *engine.Ownership) error
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
			conn, err := in.Connect(ctx, obj)
			if err != nil {
				return err
			}
			return in.Cleanup(ctx, conn, engine.NewOwnership(in.App, obj.GetManagedResources()))
		}
	}
	if done, after := HandleLifecycle(ctx, c, recorder, obj, lifecycle); done {
		return ctrl.Result{RequeueAfter: after}, nil
	}

	conn, err := in.Connect(ctx, obj)
	if err != nil {
		reason, after := Classify(err)
		log.FromContext(ctx).Error(err, "connecting to "+engine.AppName(in.App), "reason", reason)
		UpdateStatusUnreachable(ctx, c, obj, reason, err.Error())
		return ctrl.Result{RequeueAfter: after}, nil
	}

	rc := obj.GetReconcileConfig()
	pass := engine.NewPass(in.App, Policy(rc), obj.GetManagedResources())
	problems := in.Sync(ctx, conn, obj, pass)
	UpdateStatusManaged(obj, pass.Owned.Managed())
	EmitPruneEvents(recorder, obj, pass.Owned.Pruned())
	ReportResult(ctx, c, obj, pass.Observe, engine.ReconcileResult{
		Synced: pass.SyncedItems(), Errors: problems, Drift: pass.Drift(),
	})
	return ctrl.Result{RequeueAfter: ReconcileInterval(rc)}, nil
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
	default:
		return engine.ReasonAppUnreachable, 60 * time.Second
	}
}
