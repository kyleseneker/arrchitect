// Package common holds what every controller shares: the reconcile skeleton,
// status reasons and conditions, appRef resolution, finalizers, TLS and
// retries.
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
	"github.com/kyleseneker/arrchitect/internal/failure"
	"github.com/kyleseneker/arrchitect/internal/httpapi"
	"github.com/kyleseneker/arrchitect/internal/secrets"
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
			owned := engine.NewOwnership(in.App, obj.GetConfigStatus().GetManagedResources())
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
		log.FromContext(ctx).Info("Couldn't reach "+httpapi.AppName(in.App), "reason", reason, "error", err.Error(), "retryAfter", after)
		UpdateStatus(ctx, c, recorder, obj, false, reason, err.Error())
		return ctrl.Result{RequeueAfter: after}, nil
	}
	Reached(obj)

	rc := obj.GetReconcileConfig()
	pass := engine.NewPass(in.App, OwnerKey(obj), Policy(rc), obj.GetConfigStatus().GetManagedResources())
	problems := in.Sync(ctx, conn, obj, pass)
	UpdateStatusManaged(obj, pass.Owned.Managed())
	EmitPruneEvents(recorder, obj, pass.Owned.Pruned())
	retry := ReportResult(ctx, c, recorder, obj, pass.Observe, Result{
		Synced: pass.SyncedItems(), Errors: problems, Drift: pass.Drift(),
	})
	after := NextSync(rc)
	for _, sooner := range []time.Duration{pass.Retry, retry} {
		if sooner > 0 && sooner < after {
			after = sooner
		}
	}
	return ctrl.Result{RequeueAfter: after}, nil
}

// Classify is the status reason for a failure to reach an app, and when to try
// again. A spec problem waits for the spec to change, which requeues the
// resource anyway.
func Classify(err error) (string, time.Duration) {
	if reason, after, ok := failureReason(err); ok {
		return reason, after
	}
	return ReasonAppUnreachable, 60 * time.Second
}

// syncFailure is the status reason for what a pass failed to sync, and how
// soon to try again, or zero for the resource's interval. Failures that all
// have one reason of their own, such as a missing Secret, report it and its
// retry. Any other mix reads SyncFailed and waits for the interval, so a
// write the app refused isn't sent again sooner because something else is
// missing. Refused credentials wait for the interval too: their delay slows
// down connecting, and mustn't speed up a resource that syncs less often.
// Failures joined into one count apart.
func syncFailure(errs []error) (string, time.Duration) {
	leaves := make([]error, 0, len(errs))
	for _, err := range errs {
		leaves = append(leaves, failureLeaves(err)...)
	}
	var reason string
	var retry time.Duration
	for i, err := range leaves {
		r, after, ok := failureReason(err)
		if !ok || (i > 0 && r != reason) {
			return ReasonSyncFailed, 0
		}
		reason, retry = r, after
	}
	if reason == ReasonUnauthorized {
		retry = 0
	}
	return reason, retry
}

// SetupRetry is how soon to try a first-time setup again after err. A minute,
// as for an app still starting, when nothing in err has a reason of its own;
// otherwise zero, so the retry ReportResult picks for the reason applies.
func SetupRetry(err error) time.Duration {
	for _, leaf := range failureLeaves(err) {
		if _, _, ok := failureReason(leaf); ok {
			return 0
		}
	}
	return time.Minute
}

// failureLeaves splits err into the failures joined into it. A failure with a
// reason of its own stays whole, with whatever it wraps.
func failureLeaves(err error) []error {
	for e := err; e != nil; {
		if hasOwnReason(e) {
			return []error{e}
		}
		switch u := e.(type) {
		case interface{ Unwrap() []error }:
			var leaves []error
			for _, child := range u.Unwrap() {
				leaves = append(leaves, failureLeaves(child)...)
			}
			return leaves
		case interface{ Unwrap() error }:
			e = u.Unwrap()
		default:
			return []error{err}
		}
	}
	return []error{err}
}

// hasOwnReason reports whether err itself, not what it wraps, is a failure
// failureReason names.
func hasOwnReason(err error) bool {
	switch e := err.(type) {
	case *failure.ConfigError, *secrets.SecretError, *invalidAppRefError:
		return true
	case *httpapi.APIError:
		return httpapi.IsUnauthorized(e)
	}
	return err == errAppRefNotFound || err == httpapi.ErrUnauthorized
}

// failureReason is the status reason for a failure that has one of its own,
// and when to try again.
func failureReason(err error) (string, time.Duration, bool) {
	_, invalidRef := errors.AsType[*invalidAppRefError](err)
	switch {
	case isAppRefNotFound(err):
		return ReasonReferenceNotFound, 30 * time.Second, true
	case invalidRef, failure.IsConfigError(err):
		return ReasonInvalidConfig, 0, true
	case secrets.IsSecretError(err):
		return ReasonSecretNotFound, 30 * time.Second, true
	case httpapi.IsUnauthorized(err):
		// A Secret change requeues the resource, and some apps lock out a
		// client that keeps failing to sign in.
		return ReasonUnauthorized, 5 * time.Minute, true
	}
	return "", 0, false
}
