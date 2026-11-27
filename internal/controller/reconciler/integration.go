// Package reconciler runs the pass every controller shares: reading the
// resource, its lifecycle and finalizer, connecting, syncing, and reporting
// the outcome in its status, with when to try again. It also sets up each
// controller's watches and builds its connections.
package reconciler

import (
	"cmp"
	"context"
	"reflect"
	"slices"
	"time"

	"k8s.io/client-go/tools/events"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	"github.com/kyleseneker/arrchitect/internal/controller/appref"
	"github.com/kyleseneker/arrchitect/internal/controller/secretref"
	"github.com/kyleseneker/arrchitect/internal/engine"
	"github.com/kyleseneker/arrchitect/internal/failure"
	"github.com/kyleseneker/arrchitect/internal/httpapi"
)

// A failure that backs off is retried after 30 seconds, doubling each time it
// fails again, up to 10 minutes.
const (
	minBackoff = 30 * time.Second
	maxBackoff = 10 * time.Minute
)

// Integration is how the operator reaches one app and syncs a resource to it.
// Reconcile runs the steps every integration shares around it.
type Integration[T ConfigResource, C any] struct {
	// App is the app's label, such as "sonarr".
	App string
	// Name names the controller, App when empty.
	Name string
	// New returns an empty resource of the kind, and NewList an empty list of
	// them.
	New     func() T
	NewList func() client.ObjectList
	// AppRefs returns the app resources a resource refers to, which are of
	// RefKinds. Nil when the kind takes no appRef.
	AppRefs  func(client.Object) []commonv1alpha1.AppRef
	RefKinds []string
	// Watches adds the kind's own watches, such as an object's instance.
	Watches func(mgr ctrl.Manager, b *builder.Builder) (*builder.Builder, error)
	// Governor, when set, reads the reconcile settings of the resource that
	// governs obj, such as an indexer's Sonarr: its drift policy and interval
	// apply to obj, and the finalizer is dropped while it observes. obj's own
	// settings decide only its deletion policy, and a pass always prunes, so a
	// renamed item is replaced. Nil settings, as for a governor that's missing,
	// read as the defaults.
	Governor func(ctx context.Context, obj T) (*commonv1alpha1.ReconcileConfig, error)
	// Connect builds the app's client and signs in where the app needs it.
	// Cleanup uses it too.
	Connect func(ctx context.Context, obj T) (C, error)
	// Check runs after Connect before a sync, never before cleanup: a health
	// probe, and spec checks that mustn't block deleting what the operator
	// created.
	Check func(ctx context.Context, conn C, obj T) error
	// Sync brings the app in line with obj, through pass, and returns what
	// failed. Each failure leaves the rest of the pass to run.
	Sync func(ctx context.Context, conn C, obj T, pass *engine.Pass) []error
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

// settings are what a pass runs with: its drift policy and prune, and the
// reconcile settings its interval comes from.
type settings struct {
	policy   engine.SyncPolicy
	interval *commonv1alpha1.ReconcileConfig
	governed bool
}

func (in Integration[T, C]) settings(ctx context.Context, obj T) (settings, error) {
	rc := obj.GetReconcileConfig()
	if in.Governor == nil {
		return settings{policy: policy(rc), interval: rc}, nil
	}
	governing, err := in.Governor(ctx, obj)
	if err != nil {
		return settings{}, err
	}
	p := policy(governing)
	p.Prune = true
	return settings{policy: p, interval: governing, governed: true}, nil
}

func (in Integration[T, C]) lifecycle(recorder events.EventRecorder, obj T, s settings) lifecycle {
	lc := lifecycle{prune: in.Prune || s.governed, observe: s.governed && s.policy.Observe}
	if in.Cleanup != nil {
		lc.remove = func(ctx context.Context) error {
			owned := engine.NewOwnership(in.App, owned(obj))
			if in.Deletable != nil && !in.Deletable(owned) {
				return nil
			}
			conn, err := in.Connect(ctx, obj)
			if err != nil {
				return err
			}
			err = in.Cleanup(ctx, conn, owned)
			emitPruneEvents(recorder, obj, owned.Pruned())
			return err
		}
	}
	return lc
}

// Reconcile reads the resource req names and runs one pass over it.
func (in Integration[T, C]) Reconcile(ctx context.Context, c client.Client, recorder events.EventRecorder, req ctrl.Request) (ctrl.Result, error) {
	obj := in.New()
	if err := c.Get(ctx, req.NamespacedName, obj); err != nil {
		return ctrl.Result{}, forgetDeleted(obj, req.NamespacedName, err)
	}
	s, err := in.settings(ctx, obj)
	if err != nil {
		return ctrl.Result{}, err
	}
	lc := in.lifecycle(recorder, obj, s)
	if deleting, err := handleLifecycle(ctx, c, recorder, obj, lc); deleting || err != nil {
		return ctrl.Result{}, err
	}

	if err := refusal(obj.GetReconcileConfig(), lc); err != nil {
		return in.finish(ctx, c, recorder, obj, s, nil, []error{err})
	}
	conn, err := in.Connect(ctx, obj)
	if err == nil && in.Check != nil {
		err = in.Check(ctx, conn, obj)
	}
	if err != nil {
		log.FromContext(ctx).Info("Couldn't reach "+httpapi.AppName(in.App), "error", err.Error())
		return in.finish(ctx, c, recorder, obj, s, nil, []error{err})
	}

	pass := engine.NewPass(in.App, ownerKey(obj), s.policy, owned(obj))
	problems := in.Sync(ctx, conn, obj, pass)
	if managed := pass.Owned.Managed(); managed != nil {
		obj.GetConfigStatus().SetManagedResources(managed)
	}
	emitPruneEvents(recorder, obj, pass.Owned.Pruned())
	return in.finish(ctx, c, recorder, obj, s, pass, problems)
}

// finish records how a pass ended, from errs or else from what pass did, and
// says when to reconcile again: through the rate limiter for a failure that
// backs off or a status that couldn't be written, else at the interval. pass
// is nil when the pass ended before syncing.
func (in Integration[T, C]) finish(
	ctx context.Context, c client.Client, recorder events.EventRecorder, obj T, s settings, pass *engine.Pass, errs []error,
) (ctrl.Result, error) {
	var out outcome
	var st state
	if len(errs) > 0 {
		out = outcomeOf(errs)
		st = failedState(out, errs, pass != nil && s.policy.Observe)
	} else {
		st = passState(s.policy.Observe, pass.SyncedItems(), pass.Drift())
	}
	err := record(ctx, c, recorder, obj, st)
	if pass != nil {
		remember(obj, pass.Owned.Managed(), err == nil)
	}
	switch {
	case err != nil:
		return ctrl.Result{}, err
	case out.backoff:
		return ctrl.Result{}, failure.Join(errs...)
	}
	return ctrl.Result{RequeueAfter: nextSync(s.interval)}, nil
}

// SetupWithManager registers r as the kind's controller. It watches the kind,
// the Secrets its resources name and the app resources their appRefs name,
// and backs a failure off from 30 seconds, doubling to 10 minutes.
func (in Integration[T, C]) SetupWithManager(mgr ctrl.Manager, apps appref.Apps, r reconcile.Reconciler) error {
	b := ctrl.NewControllerManagedBy(mgr).
		For(in.New(), builder.WithPredicates(configChanged())).
		WithOptions(controller.Options{
			RateLimiter: workqueue.NewTypedItemExponentialFailureRateLimiter[reconcile.Request](minBackoff, maxBackoff),
		})
	b, err := secretref.Watch(mgr, b, in.New(), in.NewList())
	if err != nil {
		return err
	}
	if in.AppRefs != nil {
		if b, err = appref.Watch(mgr, b, apps, in.New(), in.NewList(), in.AppRefs, in.RefKinds...); err != nil {
			return err
		}
	}
	if in.Watches != nil {
		if b, err = in.Watches(mgr, b); err != nil {
			return err
		}
	}
	return b.Named(cmp.Or(in.Name, in.App)).Complete(r)
}

// configChanged ignores the status-only updates reconciling makes. A deletion,
// finalizer or annotation change still enqueues without a generation change.
func configChanged() predicate.Predicate {
	return predicate.Funcs{UpdateFunc: func(e event.UpdateEvent) bool {
		if e.ObjectOld == nil || e.ObjectNew == nil {
			return false
		}
		return e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration() ||
			!e.ObjectOld.GetDeletionTimestamp().Equal(e.ObjectNew.GetDeletionTimestamp()) ||
			!slices.Equal(e.ObjectOld.GetFinalizers(), e.ObjectNew.GetFinalizers()) ||
			!reflect.DeepEqual(e.ObjectOld.GetAnnotations(), e.ObjectNew.GetAnnotations())
	}}
}
