package indexers

import (
	"context"
	"fmt"
	"slices"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	indexersv1alpha1 "github.com/kyleseneker/arrchitect/api/indexers/v1alpha1"
	flaresolverrclient "github.com/kyleseneker/arrchitect/internal/client/flaresolverr"
	ctrlcommon "github.com/kyleseneker/arrchitect/internal/controller/common"
	"github.com/kyleseneker/arrchitect/internal/engine"
)

// FlareSolverrReconciler reconciles a FlareSolverr object.
type FlareSolverrReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder events.EventRecorder
}

// +kubebuilder:rbac:groups=arrchitect.dev,resources=flaresolverrs,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=arrchitect.dev,resources=flaresolverrs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=arrchitect.dev,resources=flaresolverrs/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

func (r *FlareSolverrReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var config indexersv1alpha1.FlareSolverr
	if err := r.Get(ctx, req.NamespacedName, &config); err != nil {
		return ctrl.Result{}, ctrlcommon.IgnoreDeleted(&config, req.NamespacedName, err)
	}

	cleanup := func(ctx context.Context) error { return cleanupFlareSolverrSessions(ctx, r.Client, &config) }
	if done, after := ctrlcommon.HandleLifecycle(ctx, r.Client, r.Recorder, &config, ctrlcommon.Lifecycle{Remove: cleanup, Prune: true}); done {
		return ctrl.Result{RequeueAfter: after}, nil
	}

	tlsCfg, err := engine.ResolveTLSConfig(ctx, r.Client, config.Namespace, config.Spec.Connection.TLS)
	if err != nil {
		ctrlcommon.UpdateStatusUnreachable(ctx, r.Client, &config, engine.ReasonSecretNotFound, err.Error())
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	hc, err := engine.NewHTTPClient(config.Spec.Connection.URL, engine.AuthNone, engine.WithTLSConfig(tlsCfg), engine.WithAppLabel("flaresolverr"))
	if err != nil {
		ctrlcommon.UpdateStatusUnreachable(ctx, r.Client, &config, engine.ReasonInvalidConfig, err.Error())
		return ctrl.Result{}, nil
	}
	fc := flaresolverrclient.NewClient(hc)

	// Health check
	if err := fc.Ping(ctx); err != nil {
		logger.Error(err, "FlareSolverr unreachable")
		ctrlcommon.UpdateStatusUnreachable(ctx, r.Client, &config, engine.ReasonAppUnreachable, err.Error())
		return ctrl.Result{RequeueAfter: 60 * time.Second}, nil
	}

	owned := engine.NewOwnership("flaresolverr", config.GetManagedResources())
	if ctrlcommon.ObserveOnly(config.Spec.Reconcile) {
		observed := &engine.Writes{App: "flaresolverr", Observe: true}
		count, err := observeFlareSolverrSessions(ctx, fc, config.Spec.Sessions,
			ctrlcommon.PruneEnabled(config.Spec.Reconcile), owned, observed)
		if err == nil {
			config.Status.ActiveSessions = int32(count)
		}
		ctrlcommon.UpdateObservationStatus(ctx, r.Client, &config, observed.Drift(), err)
		return ctrl.Result{RequeueAfter: ctrlcommon.ReconcileInterval(config.Spec.Reconcile)}, nil
	}

	var syncErrors []string

	if err := reconcileFlareSolverrSessions(ctx, fc, config.Spec.Sessions, ctrlcommon.PruneEnabled(config.Spec.Reconcile), owned); err != nil {
		logger.Error(err, "failed to reconcile sessions")
		syncErrors = append(syncErrors, fmt.Sprintf("sessions: %v", err))
	}
	ctrlcommon.UpdateStatusManaged(&config, owned.Managed())
	ctrlcommon.EmitPruneEvents(r.Recorder, &config, owned.Pruned())

	// Update active session count
	if sessions, err := fc.ListSessions(ctx); err == nil {
		config.Status.ActiveSessions = int32(len(sessions))
	}

	ctrlcommon.UpdateSyncStatus(ctx, r.Client, &config, false, nil, syncErrors)

	return ctrl.Result{RequeueAfter: ctrlcommon.ReconcileInterval(config.Spec.Reconcile)}, nil
}

// reconcileFlareSolverrSessions creates the declared sessions and, under prune,
// destroys the ones it created that have left the spec. Sessions it did not
// create are left alone: Prowlarr opens sessions on demand, and tearing those
// down mid-request breaks the indexers FlareSolverr serves.
func reconcileFlareSolverrSessions(
	ctx context.Context, fc *flaresolverrclient.Client, desired []indexersv1alpha1.FlareSolverrSession, prune bool, owned *engine.Ownership,
) error {
	existing, err := fc.ListSessions(ctx)
	if err != nil {
		return fmt.Errorf("listing sessions: %w", err)
	}
	declared := sessionNames(desired)
	for _, name := range declared {
		if slices.Contains(existing, name) {
			continue
		}
		if err := fc.CreateSession(ctx, name); err != nil {
			return fmt.Errorf("creating session %q: %w", name, err)
		}
		owned.Created("sessions", name)
	}
	if !prune {
		return nil
	}
	return owned.Prune("sessions", declared, existing, func(name string) error { return fc.DestroySession(ctx, name) })
}

func sessionNames(sessions []indexersv1alpha1.FlareSolverrSession) []string {
	names := make([]string, 0, len(sessions))
	for _, s := range sessions {
		names = append(names, s.Name)
	}
	return names
}

// SetupWithManager sets up the controller with the Manager.
func (r *FlareSolverrReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&indexersv1alpha1.FlareSolverr{}, builder.WithPredicates(ctrlcommon.ConfigChangedPredicate())).
		WatchesMetadata(&corev1.Secret{}, ctrlcommon.EnqueueForSecret(r.Client, &indexersv1alpha1.FlareSolverrList{})).
		Named("flaresolverr").
		Complete(r)
}

func observeFlareSolverrSessions(
	ctx context.Context, fc *flaresolverrclient.Client, desired []indexersv1alpha1.FlareSolverrSession, prune bool,
	owned *engine.Ownership, w *engine.Writes,
) (int, error) {
	existing, err := fc.ListSessions(ctx)
	if err != nil {
		return 0, fmt.Errorf("listing sessions: %w", err)
	}
	for _, session := range desired {
		err := w.Compare("sessions", session.Name,
			map[string]any{"exists": slices.Contains(existing, session.Name)}, map[string]any{"exists": true})
		if err != nil {
			return len(existing), err
		}
	}
	if prune {
		return len(existing), w.Prune(owned, "sessions", sessionNames(desired), existing, nil)
	}
	return len(existing), nil
}

func cleanupFlareSolverrSessions(ctx context.Context, c client.Reader, config *indexersv1alpha1.FlareSolverr) error {
	if len(config.GetManagedResources()["sessions"]) == 0 {
		return nil
	}
	tlsConfig, err := engine.ResolveTLSConfig(ctx, c, config.Namespace, config.Spec.Connection.TLS)
	if err != nil {
		return err
	}
	hc, err := engine.NewHTTPClient(config.Spec.Connection.URL, engine.AuthNone, engine.WithTLSConfig(tlsConfig), engine.WithAppLabel("flaresolverr"))
	if err != nil {
		return err
	}
	fc := flaresolverrclient.NewClient(hc)
	existing, err := fc.ListSessions(ctx)
	if err != nil {
		return fmt.Errorf("listing sessions: %w", err)
	}
	owned := engine.NewOwnership("flaresolverr", config.GetManagedResources())
	return owned.Cleanup("sessions", existing, func(name string) error { return fc.DestroySession(ctx, name) })
}
