package reconciler

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	pvrv1alpha1 "github.com/kyleseneker/arrchitect/api/pvr/v1alpha1"
	"github.com/kyleseneker/arrchitect/internal/engine"
	"github.com/kyleseneker/arrchitect/internal/failure"
)

func newConfig(policy string, deletedAgo time.Duration) *pvrv1alpha1.Sonarr {
	c := &pvrv1alpha1.Sonarr{
		Name: "s", Namespace: "arr", Finalizers: []string{Finalizer},
	}
	c.Status.SetManagedResources(map[string][]string{"indexers": {"created"}})
	if policy != "" {
		c.Spec.Reconcile = &commonv1alpha1.ReconcileConfig{DeletionPolicy: policy}
	}
	if deletedAgo > 0 {
		ts := metav1.NewTime(time.Now().Add(-deletedAgo))
		c.DeletionTimestamp = &ts
	}
	return c
}

func testClient(o *pvrv1alpha1.Sonarr) *fake.ClientBuilder {
	_ = pvrv1alpha1.AddToScheme(scheme.Scheme)
	return fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(o)
}

func TestOrphanIsTheDefaultAndDoesNotRemove(t *testing.T) {
	c := newConfig("", time.Minute)
	cl := testClient(c).Build()
	called := false
	err := handleDeletion(context.Background(), cl, nil, c, func(context.Context) error {
		called = true
		return nil
	})
	require.NoError(t, err)
	assert.False(t, called, "remove must not run under the default orphan policy")
	assert.False(t, controllerutil.ContainsFinalizer(c, Finalizer), "finalizer should have been released")
}

func TestDeletePolicyRemovesThenReleases(t *testing.T) {
	c := newConfig(DeletionPolicyDelete, time.Minute)
	cl := testClient(c).Build()
	called := false
	err := handleDeletion(context.Background(), cl, nil, c, func(context.Context) error {
		called = true
		return nil
	})
	require.NoError(t, err)
	assert.True(t, called, "remove should run under the delete policy")
	assert.False(t, controllerutil.ContainsFinalizer(c, Finalizer), "finalizer should have been released")
}

func TestDeletePolicyWithNothingOwnedSkipsTheApp(t *testing.T) {
	c := newConfig(DeletionPolicyDelete, time.Minute)
	c.Status.SetManagedResources(map[string][]string{"indexers": {}})
	cl := testClient(c).Build()
	err := handleDeletion(context.Background(), cl, nil, c, func(context.Context) error {
		return errors.New("app is gone")
	})
	require.NoError(t, err)
	assert.False(t, controllerutil.ContainsFinalizer(c, Finalizer), "a resource that owns nothing must not wait on its app")
}

func TestUnreachableAppRetriesButDoesNotWedgeForever(t *testing.T) {
	// Recently deleted: keep retrying, hold the finalizer.
	c := newConfig(DeletionPolicyDelete, time.Minute)
	cl := testClient(c).Build()
	recorder := events.NewFakeRecorder(1)
	err := handleDeletion(context.Background(), cl, recorder, c, func(context.Context) error {
		return errors.New("app unreachable")
	})
	assert.Error(t, err, "expected an error so the request is retried")
	assert.True(t, controllerutil.ContainsFinalizer(c, Finalizer), "finalizer must be held while still within the give-up window")
	select {
	case event := <-recorder.Events:
		assert.Contains(t, event, "Warning CleanupFailed Removing remote resources failed")
		assert.Contains(t, event, "app unreachable")
	default:
		assert.Fail(t, "a failed cleanup must say why the resource is still there")
	}

	// Past the give-up window: release regardless.
	c2 := newConfig(DeletionPolicyDelete, deletionGiveUpAfter+time.Minute)
	cl2 := testClient(c2).Build()
	recorder2 := events.NewFakeRecorder(1)
	err = handleDeletion(context.Background(), cl2, recorder2, c2, func(context.Context) error {
		return errors.New("app still unreachable")
	})
	require.NoError(t, err)
	assert.False(t, controllerutil.ContainsFinalizer(c2, Finalizer), "a dead app must not wedge deletion past the give-up window")
	assert.Equal(t, "Warning FinalizerReleased Released the finalizer after 10m0s without removing remote resources: app still unreachable",
		<-recorder2.Events)
}

// An item the app won't delete stays won't-delete on every retry, so it is
// reported and the finalizer is released rather than waiting out the give-up.
func TestDeletionReportsWhatTheAppRefusesAndReleases(t *testing.T) {
	c := newConfig(DeletionPolicyDelete, time.Minute)
	cl := testClient(c).Build()
	recorder := events.NewFakeRecorder(1)

	err := handleDeletion(context.Background(), cl, recorder, c, func(context.Context) error {
		return &engine.KeptError{App: "seerr", Kept: []engine.KeptResource{{
			Type: "sonarr", Name: "Sonarr", Reason: "Seerr refused to delete it: in use",
		}}}
	})

	require.NoError(t, err)
	assert.False(t, controllerutil.ContainsFinalizer(c, Finalizer))
	assert.Equal(t, `Warning ResourceKept Kept sonarr "Sonarr", which the operator created: Seerr refused to delete it: in use`, <-recorder.Events)
}

func TestUnsupportedOrObserveDeletionDoesNotSilentlySucceed(t *testing.T) {
	for _, observe := range []bool{false, true} {
		t.Run(fmt.Sprint(observe), func(t *testing.T) {
			obj := newConfig(DeletionPolicyDelete, time.Minute)
			policy := driftPolicyObserve
			if observe {
				obj.Spec.Reconcile.DriftPolicy = policy
			}
			c := testClient(obj).Build()
			called := false
			var remove func(context.Context) error
			if observe {
				remove = func(context.Context) error { called = true; return nil }
			}
			err := handleDeletion(context.Background(), c, nil, obj, remove)
			require.Error(t, err, "expected explicit cleanup failure")
			require.False(t, called, "observe mode must not invoke cleanup")
			require.True(t, controllerutil.ContainsFinalizer(obj, Finalizer), "finalizer released without reporting failure")
		})
	}
}

func TestLifecycleRefusesInvalidDeletionPolicies(t *testing.T) {
	remove := func(context.Context) error { return nil }
	for _, tc := range []struct {
		name string
		rc   *commonv1alpha1.ReconcileConfig
		lc   lifecycle
		want string
	}{
		{"delete where unsupported", &commonv1alpha1.ReconcileConfig{DeletionPolicy: DeletionPolicyDelete}, lifecycle{prune: true},
			"deletionPolicy delete is not supported by this integration; use orphan"},
		{"delete in observe mode", &commonv1alpha1.ReconcileConfig{DeletionPolicy: DeletionPolicyDelete, DriftPolicy: driftPolicyObserve},
			lifecycle{remove: remove, prune: true}, "deletionPolicy delete conflicts with driftPolicy observe; use orphan"},
		{"delete", &commonv1alpha1.ReconcileConfig{DeletionPolicy: DeletionPolicyDelete}, lifecycle{remove: remove}, ""},
		{"orphan where delete is unsupported", nil, lifecycle{}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := refusal(tc.rc, tc.lc)
			if tc.want == "" {
				assert.NoError(t, err)
				return
			}
			assert.EqualError(t, err, tc.want)
			assert.Equal(t, failure.InvalidConfig, failure.KindOf(err))
		})
	}
}

// Only a resource the operator will clean up holds the finalizer, so uninstalling
// the operator or deleting its namespace never leaves the rest in Terminating.
func TestOnlyASupportedDeletePolicyHoldsTheFinalizer(t *testing.T) {
	remove := func(context.Context) error { return nil }
	observe := driftPolicyObserve
	for _, tc := range []struct {
		name    string
		policy  string
		observe bool
		lc      lifecycle
		want    bool
	}{
		{name: "orphan by default", lc: lifecycle{remove: remove}},
		{name: "orphan", policy: "orphan", lc: lifecycle{remove: remove}},
		{name: "delete", policy: DeletionPolicyDelete, lc: lifecycle{remove: remove}, want: true},
		{name: "delete where unsupported", policy: DeletionPolicyDelete},
		{name: "delete in observe mode", policy: DeletionPolicyDelete, observe: true, lc: lifecycle{remove: remove}},
	} {
		for _, had := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/had=%v", tc.name, had), func(t *testing.T) {
				obj := newConfig(tc.policy, 0)
				if !had {
					obj.Finalizers = nil
				}
				if tc.observe {
					obj.Spec.Reconcile.DriftPolicy = observe
				}
				c := testClient(obj).WithStatusSubresource(obj).Build()

				_, err := handleLifecycle(context.Background(), c, nil, obj, tc.lc)
				require.NoError(t, err)

				assert.Equal(t, tc.want, controllerutil.ContainsFinalizer(obj, Finalizer))
			})
		}
	}
}

// An integration that cannot prune must say so: ignoring prune would leave
// resources removed from the spec in place while the spec claims otherwise.
func TestLifecycleRefusesPruneWhereUnsupported(t *testing.T) {
	rc := &commonv1alpha1.ReconcileConfig{Prune: new(true)}
	err := refusal(rc, lifecycle{})
	assert.EqualError(t, err, "prune is not supported by this integration; remove spec.reconcile.prune")
	assert.Equal(t, failure.InvalidConfig, failure.KindOf(err))
	assert.NoError(t, refusal(rc, lifecycle{prune: true}))
}

// Cleanup that runs out the reconcile's deadline must not keep the finalizer
// past deletionGiveUpAfter.
func TestFinalizerIsReleasedAfterCleanupUsesUpTheDeadline(t *testing.T) {
	c := newConfig(DeletionPolicyDelete, deletionGiveUpAfter+time.Minute)
	cl := testClient(c).Build()
	ctx, cancel := context.WithCancel(context.Background())

	err := handleDeletion(ctx, cl, nil, c, func(context.Context) error {
		cancel()
		return context.Canceled
	})

	require.NoError(t, err)
	err = cl.Get(context.Background(), client.ObjectKeyFromObject(c), &pvrv1alpha1.Sonarr{})
	assert.True(t, apierrors.IsNotFound(err), "the finalizer was released, so the resource is gone")
}
