// Package indexers holds the Prowlarr and FlareSolverr controllers, and
// Prowlarr's sync: its definition, the payloads for each kind, and the stages
// they are written in.
package indexers

import (
	"context"
	"fmt"
	"slices"

	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	indexersv1alpha1 "github.com/kyleseneker/arrchitect/api/indexers/v1alpha1"
	flaresolverrclient "github.com/kyleseneker/arrchitect/internal/client/flaresolverr"
	"github.com/kyleseneker/arrchitect/internal/controller/reconciler"
	"github.com/kyleseneker/arrchitect/internal/engine"
	"github.com/kyleseneker/arrchitect/internal/httpapi"
)

// FlareSolverrReconciler reconciles a FlareSolverr object.
type FlareSolverrReconciler struct {
	client.Client
	Recorder events.EventRecorder
}

// +kubebuilder:rbac:groups=arrchitect.dev,resources=flaresolverrs,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=arrchitect.dev,resources=flaresolverrs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=arrchitect.dev,resources=flaresolverrs/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

func (r *FlareSolverrReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.integration().Reconcile(ctx, r.Client, r.Recorder, req)
}

// SetupWithManager sets up the controller with the Manager.
func (r *FlareSolverrReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return r.integration().SetupWithManager(mgr, nil, r)
}

func (r *FlareSolverrReconciler) integration() reconciler.Integration[*indexersv1alpha1.FlareSolverr, *flaresolverrclient.Client] {
	return reconciler.Integration[*indexersv1alpha1.FlareSolverr, *flaresolverrclient.Client]{
		App:     "flaresolverr",
		New:     func() *indexersv1alpha1.FlareSolverr { return &indexersv1alpha1.FlareSolverr{} },
		NewList: func() client.ObjectList { return &indexersv1alpha1.FlareSolverrList{} },
		Connect: r.connect,
		Check: func(ctx context.Context, fc *flaresolverrclient.Client, _ *indexersv1alpha1.FlareSolverr) error {
			return fc.Ping(ctx)
		},
		Sync:    syncFlareSolverr,
		Cleanup: cleanupFlareSolverr,
		Prune:   true,
	}
}

func (r *FlareSolverrReconciler) connect(ctx context.Context, config *indexersv1alpha1.FlareSolverr) (*flaresolverrclient.Client, error) {
	hc, err := reconciler.Dial(ctx, r.Client, config.Namespace, reconciler.Endpoint{
		App: "flaresolverr", URL: config.Spec.Connection.URL, TLS: config.Spec.Connection.TLS, Auth: httpapi.AuthNone,
	})
	if err != nil {
		return nil, err
	}
	return flaresolverrclient.NewClient(hc), nil
}

func syncFlareSolverr(ctx context.Context, fc *flaresolverrclient.Client, config *indexersv1alpha1.FlareSolverr, pass *engine.Pass) []error {
	var problems []error
	if err := reconcileFlareSolverrSessions(ctx, fc, pass, config.Spec.Sessions); err != nil {
		log.FromContext(ctx).Error(err, "failed to reconcile sessions")
		problems = append(problems, fmt.Errorf("sessions: %w", err))
	}
	if sessions, err := fc.ListSessions(ctx); err == nil {
		config.Status.ActiveSessions = int32(len(sessions))
	}
	return problems
}

// reconcileFlareSolverrSessions creates the declared sessions and, under prune,
// destroys the ones it created that have left the spec. Sessions it did not
// create are left alone: Prowlarr opens sessions on demand, and tearing those
// down mid-request breaks the indexers FlareSolverr serves.
func reconcileFlareSolverrSessions(
	ctx context.Context, fc *flaresolverrclient.Client, pass *engine.Pass, desired []indexersv1alpha1.FlareSolverrSession,
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
		err := pass.Apply("sessions", name, func() error {
			if err := fc.CreateSession(ctx, name); err != nil {
				return err
			}
			pass.Owned.Created("sessions", name)
			return nil
		})
		if err != nil {
			return fmt.Errorf("creating session %q: %w", name, err)
		}
	}
	if !pass.PruneEnabled {
		return nil
	}
	return pass.Prune(pass.Owned, "sessions", declared, existing, func(name string) error { return fc.DestroySession(ctx, name) })
}

func sessionNames(sessions []indexersv1alpha1.FlareSolverrSession) []string {
	names := make([]string, 0, len(sessions))
	for _, s := range sessions {
		names = append(names, s.Name)
	}
	return names
}

// cleanupFlareSolverr destroys the sessions the operator created, for
// deletionPolicy delete.
func cleanupFlareSolverr(ctx context.Context, fc *flaresolverrclient.Client, owned *engine.Ownership) error {
	existing, err := fc.ListSessions(ctx)
	if err != nil {
		return fmt.Errorf("listing sessions: %w", err)
	}
	return owned.Cleanup("sessions", existing, func(name string) error { return fc.DestroySession(ctx, name) })
}
