package reconciler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	pvrv1alpha1 "github.com/kyleseneker/arrchitect/api/pvr/v1alpha1"
	"github.com/kyleseneker/arrchitect/internal/engine"
	"github.com/kyleseneker/arrchitect/internal/failure"
)

// sonarrs is an Integration over Sonarrs that connects to "conn" and syncs
// nothing, for a test to change.
func sonarrs() Integration[*pvrv1alpha1.Sonarr, string] {
	return Integration[*pvrv1alpha1.Sonarr, string]{
		App:     "sonarr",
		New:     func() *pvrv1alpha1.Sonarr { return &pvrv1alpha1.Sonarr{} },
		NewList: func() client.ObjectList { return &pvrv1alpha1.SonarrList{} },
		Connect: func(context.Context, *pvrv1alpha1.Sonarr) (string, error) { return "conn", nil },
		Sync:    func(context.Context, string, *pvrv1alpha1.Sonarr, *engine.Pass) []error { return nil },
	}
}

// stored puts obj in a fake API server that serves its status.
func stored(t *testing.T, obj *pvrv1alpha1.Sonarr, funcs ...interceptor.Funcs) client.Client {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, pvrv1alpha1.AddToScheme(s))
	b := fake.NewClientBuilder().WithScheme(s).WithObjects(obj).WithStatusSubresource(obj)
	for _, f := range funcs {
		b = b.WithInterceptorFuncs(f)
	}
	t.Cleanup(func() { forgetOwnership(obj) })
	return b.Build()
}

func reconcileSonarr(t *testing.T, in Integration[*pvrv1alpha1.Sonarr, string], c client.Client) (ctrl.Result, error) {
	t.Helper()
	return in.Reconcile(context.Background(), c, nil, ctrl.Request{Namespace: "media", Name: "sonarr"})
}

func readSonarr(t *testing.T, c client.Client) *pvrv1alpha1.Sonarr {
	t.Helper()
	got := &pvrv1alpha1.Sonarr{}
	require.NoError(t, c.Get(context.Background(), client.ObjectKey{Namespace: "media", Name: "sonarr"}, got))
	return got
}

// assertInterval checks the next pass waits for the resource's interval, with
// its jitter.
func assertInterval(t *testing.T, result ctrl.Result) {
	t.Helper()
	assert.GreaterOrEqual(t, result.RequeueAfter, DefaultReconcileInterval)
	assert.LessOrEqual(t, result.RequeueAfter, DefaultReconcileInterval+time.Duration(float64(DefaultReconcileInterval)*jitter))
}

// A failure to connect sets the reason the table gives it. One that backs off
// is returned to the rate limiter, and the rest wait for the interval or for
// a watch event.
func TestConnectFailuresFollowTheTable(t *testing.T) {
	for _, tc := range []struct {
		err     error
		reason  string
		stalled bool
		backoff bool
	}{
		{failure.Unreachable.Errorf("connection refused"), ReasonAppUnreachable, false, true},
		{failure.Transient.Errorf("reading secret: timeout"), ReasonSyncFailed, false, true},
		{failure.Unauthorized.Errorf("401"), ReasonUnauthorized, true, false},
		{failure.SecretNotFound.Errorf("secret %q not found", "keys"), ReasonSecretNotFound, false, false},
		{failure.InvalidConfig.Errorf("bad URL"), ReasonInvalidConfig, true, false},
		{errors.New("unexpected answer"), ReasonSyncFailed, false, false},
	} {
		t.Run(tc.reason+"/"+tc.err.Error(), func(t *testing.T) {
			c := stored(t, &pvrv1alpha1.Sonarr{Name: "sonarr", Namespace: "media"})
			in := sonarrs()
			in.Connect = func(context.Context, *pvrv1alpha1.Sonarr) (string, error) { return "", tc.err }

			result, err := reconcileSonarr(t, in, c)

			if tc.backoff {
				require.ErrorIs(t, err, tc.err)
				assert.Zero(t, result)
			} else {
				require.NoError(t, err)
				assertInterval(t, result)
			}
			got := readSonarr(t, c)
			ready := meta.FindStatusCondition(got.Status.Conditions, ConditionReady)
			require.NotNil(t, ready)
			assert.Equal(t, tc.reason, ready.Reason)
			assert.Equal(t, tc.err.Error(), ready.Message)
			assert.Equal(t, tc.stalled, meta.IsStatusConditionTrue(got.Status.Conditions, ConditionStalled))
		})
	}
}

// A Secret that a section of the spec names but that doesn't exist yet fails
// only that section, and the pass waits for the interval or for the Secret to
// appear, which the watch catches. It doesn't run the whole pass again sooner.
// The app going away mid-pass backs off.
func TestSyncFailuresKeepTheirReason(t *testing.T) {
	for _, tc := range []struct {
		name    string
		problem error
		reason  string
		backoff bool
	}{
		{"missing Secret", failure.SecretNotFound.Errorf("settings: secret %q not found", "discord"), ReasonSecretNotFound, false},
		{"refused write", errors.New("flow(a): API request failed with status 400"), ReasonSyncFailed, false},
		{"app went away", failure.Unreachable.Errorf("flow(a): connection reset"), ReasonAppUnreachable, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := stored(t, &pvrv1alpha1.Sonarr{Name: "sonarr", Namespace: "media"})
			in := sonarrs()
			in.Sync = func(context.Context, string, *pvrv1alpha1.Sonarr, *engine.Pass) []error { return []error{tc.problem} }

			result, err := reconcileSonarr(t, in, c)

			if tc.backoff {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assertInterval(t, result)
			}
			ready := meta.FindStatusCondition(readSonarr(t, c).Status.Conditions, ConditionReady)
			require.NotNil(t, ready)
			assert.Equal(t, tc.reason, ready.Reason)
			assert.Equal(t, tc.problem.Error(), ready.Message)
		})
	}
}

// A spec that asks for prune where the integration has none is refused before
// connecting, as a spec problem.
func TestARefusedLifecycleSettingIsReported(t *testing.T) {
	obj := &pvrv1alpha1.Sonarr{Name: "sonarr", Namespace: "media"}
	obj.Spec.Reconcile = &commonv1alpha1.ReconcileConfig{Prune: new(true)}
	c := stored(t, obj)
	in := sonarrs()
	in.Connect = func(context.Context, *pvrv1alpha1.Sonarr) (string, error) {
		require.Fail(t, "a refused spec connected")
		return "", nil
	}

	result, err := reconcileSonarr(t, in, c)

	require.NoError(t, err)
	assertInterval(t, result)
	got := readSonarr(t, c)
	ready := meta.FindStatusCondition(got.Status.Conditions, ConditionReady)
	require.NotNil(t, ready)
	assert.Equal(t, ReasonInvalidConfig, ready.Reason)
	assert.Equal(t, "prune is not supported by this integration; remove spec.reconcile.prune", ready.Message)
	assert.True(t, meta.IsStatusConditionTrue(got.Status.Conditions, ConditionStalled))
}

// A status write that fails is returned to back off, and what the pass created
// stays owned, so the next pass can still prune or clean it up.
func TestAFailedStatusWriteKeepsWhatThePassCreated(t *testing.T) {
	failWrite := true
	c := stored(t, &pvrv1alpha1.Sonarr{Name: "sonarr", Namespace: "media", UID: "uid-1"}, interceptor.Funcs{
		SubResourcePatch: func(ctx context.Context, c client.Client, sub string, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
			if failWrite {
				failWrite = false
				return apierrors.NewServiceUnavailable("etcd is down")
			}
			return c.SubResource(sub).Patch(ctx, obj, patch, opts...)
		},
	})
	in := sonarrs()
	in.Sync = func(_ context.Context, _ string, _ *pvrv1alpha1.Sonarr, pass *engine.Pass) []error {
		pass.Owned.Created("tags", "created")
		return nil
	}

	_, err := reconcileSonarr(t, in, c)
	require.Error(t, err, "a failed status write backs off")
	assert.Empty(t, readSonarr(t, c).Status.GetManagedResources())

	var ownedBefore map[string][]string
	in.Sync = func(_ context.Context, _ string, _ *pvrv1alpha1.Sonarr, pass *engine.Pass) []error {
		ownedBefore = cloneManaged(pass.Owned.Managed())
		return nil
	}
	_, err = reconcileSonarr(t, in, c)
	require.NoError(t, err)
	assert.Equal(t, map[string][]string{"tags": {"created"}}, ownedBefore, "the next pass didn't own what the last one created")
	assert.Equal(t, map[string][]string{"tags": {"created"}}, readSonarr(t, c).Status.GetManagedResources(), "the ownership was never recorded")
}

// An object governed by another resource takes that resource's drift policy
// and interval, always prunes, and keeps no finalizer while it observes.
func TestAGovernorDecidesThePolicyAndInterval(t *testing.T) {
	obj := &pvrv1alpha1.Sonarr{Name: "sonarr", Namespace: "media", Finalizers: []string{Finalizer}}
	obj.Spec.Reconcile = &commonv1alpha1.ReconcileConfig{DeletionPolicy: DeletionPolicyDelete}
	c := stored(t, obj)
	in := sonarrs()
	in.Governor = func(context.Context, *pvrv1alpha1.Sonarr) (*commonv1alpha1.ReconcileConfig, error) {
		return &commonv1alpha1.ReconcileConfig{DriftPolicy: driftPolicyObserve, IntervalSeconds: 60}, nil
	}
	in.Cleanup = func(context.Context, string, *engine.Ownership) error { return nil }
	var observe, prune bool
	in.Sync = func(_ context.Context, _ string, _ *pvrv1alpha1.Sonarr, pass *engine.Pass) []error {
		observe, prune = pass.Observe, pass.PruneEnabled
		return nil
	}

	result, err := reconcileSonarr(t, in, c)

	require.NoError(t, err)
	assert.True(t, observe, "the governor's drift policy didn't apply")
	assert.True(t, prune, "a governed pass always prunes")
	assert.GreaterOrEqual(t, result.RequeueAfter, time.Minute)
	assert.LessOrEqual(t, result.RequeueAfter, time.Minute+6*time.Second)
	assert.False(t, controllerutil.ContainsFinalizer(readSonarr(t, c), Finalizer), "the governor's observe mode left the finalizer in place")
}

func TestCleanupSkipsCheck(t *testing.T) {
	obj, c := deletingSonarr(t)
	cleaned := false
	in := sonarrs()
	in.Check = func(context.Context, string, *pvrv1alpha1.Sonarr) error {
		return failure.InvalidConfig.Wrap(errors.New("the spec's whitelist would lock the operator out"))
	}
	in.Cleanup = func(_ context.Context, conn string, owned *engine.Ownership) error {
		cleaned = conn == "conn" && owned.Owns("indexers")
		return nil
	}

	_, err := in.Reconcile(context.Background(), c, nil, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(obj)})

	require.NoError(t, err)
	assert.True(t, cleaned, "a spec problem Check finds must not block deleting what the operator created")
}

// A cleanup that fails backs off, and the finalizer holds the resource.
func TestAFailedCleanupBacksOff(t *testing.T) {
	obj, c := deletingSonarr(t)
	in := sonarrs()
	in.Cleanup = func(context.Context, string, *engine.Ownership) error {
		return failure.Unreachable.Errorf("connection refused")
	}

	_, err := in.Reconcile(context.Background(), c, nil, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(obj)})

	require.Error(t, err)
	assert.True(t, controllerutil.ContainsFinalizer(readSonarr(t, c), Finalizer))
}

func TestDeletionWithNothingDeletableNeedsNoConnection(t *testing.T) {
	obj, c := deletingSonarr(t)
	in := sonarrs()
	in.Connect = func(context.Context, *pvrv1alpha1.Sonarr) (string, error) {
		return "", errors.New("the API key Secret is already gone")
	}
	in.Cleanup = func(context.Context, string, *engine.Ownership) error { return nil }
	in.Deletable = func(*engine.Ownership) bool { return false }

	_, err := in.Reconcile(context.Background(), c, nil, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(obj)})

	require.NoError(t, err)
	err = c.Get(context.Background(), client.ObjectKeyFromObject(obj), &pvrv1alpha1.Sonarr{})
	assert.True(t, apierrors.IsNotFound(err), "the finalizer is released without reading credentials, so the resource is gone")
}

// deletingSonarr is a Sonarr with deletionPolicy delete, deleted a minute ago
// and still holding the operator's finalizer, in a fake API server.
func deletingSonarr(t *testing.T) (*pvrv1alpha1.Sonarr, client.Client) {
	t.Helper()
	deleted := metav1.NewTime(time.Now().Add(-time.Minute))
	obj := &pvrv1alpha1.Sonarr{Name: "sonarr", Namespace: "media", Finalizers: []string{Finalizer}, DeletionTimestamp: &deleted}
	obj.Spec.Reconcile = &commonv1alpha1.ReconcileConfig{DeletionPolicy: DeletionPolicyDelete}
	obj.Status.SetManagedResources(map[string][]string{"indexers": {"created"}})
	return obj, stored(t, obj)
}

func TestConfigChanged(t *testing.T) {
	old := &pvrv1alpha1.Sonarr{Name: "sonarr", Namespace: "media", Generation: 1, Finalizers: []string{Finalizer}}
	cases := []struct {
		name   string
		change func(*pvrv1alpha1.Sonarr)
		want   bool
	}{
		{"status", func(c *pvrv1alpha1.Sonarr) { now := metav1.Now(); c.Status.LastSyncTime = &now }, false},
		{"resource version", func(c *pvrv1alpha1.Sonarr) { c.ResourceVersion = "2" }, false},
		{"spec", func(c *pvrv1alpha1.Sonarr) { c.Generation++ }, true},
		{"deletion", func(c *pvrv1alpha1.Sonarr) { now := metav1.Now(); c.DeletionTimestamp = &now }, true},
		{"finalizers", func(c *pvrv1alpha1.Sonarr) { c.Finalizers = nil }, true},
		{"annotation", func(c *pvrv1alpha1.Sonarr) { c.Annotations = map[string]string{"reconcile": "now"} }, true},
	}
	p := configChanged()
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			next := old.DeepCopy()
			tt.change(next)
			assert.Equal(t, tt.want, p.Update(event.UpdateEvent{ObjectOld: old, ObjectNew: next}))
		})
	}
	assert.False(t, p.Update(event.UpdateEvent{ObjectOld: old}))
	assert.False(t, p.Update(event.UpdateEvent{ObjectNew: old}))
	assert.True(t, p.Create(event.CreateEvent{Object: old}))
	assert.True(t, p.Delete(event.DeleteEvent{Object: old}))
	assert.True(t, p.Generic(event.GenericEvent{Object: old}))
}
