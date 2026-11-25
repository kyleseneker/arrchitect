package common

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	pvrv1alpha1 "github.com/kyleseneker/arrchitect/api/pvr/v1alpha1"
	"github.com/kyleseneker/arrchitect/internal/engine"
)

func newConfig(policy string, deletedAgo time.Duration) *pvrv1alpha1.Sonarr {
	c := &pvrv1alpha1.Sonarr{
		Name: "s", Namespace: "arr", Finalizers: []string{Finalizer},
	}
	c.SetManagedResources(map[string][]string{"indexers": {"created"}})
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
	handled, err := HandleDeletion(context.Background(), cl, nil, c, func(context.Context) error {
		called = true
		return nil
	})
	require.NoError(t, err)
	require.True(t, handled)
	assert.False(t, called, "remove must not run under the default orphan policy")
	assert.False(t, controllerutil.ContainsFinalizer(c, Finalizer), "finalizer should have been released")
}

func TestDeletePolicyRemovesThenReleases(t *testing.T) {
	c := newConfig(DeletionPolicyDelete, time.Minute)
	cl := testClient(c).Build()
	called := false
	handled, err := HandleDeletion(context.Background(), cl, nil, c, func(context.Context) error {
		called = true
		return nil
	})
	require.NoError(t, err)
	require.True(t, handled)
	assert.True(t, called, "remove should run under the delete policy")
	assert.False(t, controllerutil.ContainsFinalizer(c, Finalizer), "finalizer should have been released")
}

func TestDeletePolicyWithNothingOwnedSkipsTheApp(t *testing.T) {
	c := newConfig(DeletionPolicyDelete, time.Minute)
	c.SetManagedResources(map[string][]string{"indexers": {}})
	cl := testClient(c).Build()
	handled, err := HandleDeletion(context.Background(), cl, nil, c, func(context.Context) error {
		return errors.New("app is gone")
	})
	require.NoError(t, err)
	require.True(t, handled)
	assert.False(t, controllerutil.ContainsFinalizer(c, Finalizer), "a resource that owns nothing must not wait on its app")
}

func TestUnreachableAppRetriesButDoesNotWedgeForever(t *testing.T) {
	// Recently deleted: keep retrying, hold the finalizer.
	c := newConfig(DeletionPolicyDelete, time.Minute)
	cl := testClient(c).Build()
	recorder := events.NewFakeRecorder(1)
	_, err := HandleDeletion(context.Background(), cl, recorder, c, func(context.Context) error {
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
	c2 := newConfig(DeletionPolicyDelete, DeletionGiveUpAfter+time.Minute)
	cl2 := testClient(c2).Build()
	recorder2 := events.NewFakeRecorder(1)
	handled, err := HandleDeletion(context.Background(), cl2, recorder2, c2, func(context.Context) error {
		return errors.New("app still unreachable")
	})
	require.NoError(t, err)
	require.True(t, handled)
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

	_, err := HandleDeletion(context.Background(), cl, recorder, c, func(context.Context) error {
		return &engine.KeptError{App: "seerr", Kept: []engine.KeptResource{{
			Type: "sonarr", Name: "Sonarr", Reason: "in use",
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
			policy := DriftPolicyObserve
			if observe {
				obj.Spec.Reconcile.DriftPolicy = policy
			}
			c := testClient(obj).Build()
			called := false
			var remove func(context.Context) error
			if observe {
				remove = func(context.Context) error { called = true; return nil }
			}
			_, err := HandleDeletion(context.Background(), c, nil, obj, remove)
			require.Error(t, err, "expected explicit cleanup failure")
			require.False(t, called, "observe mode must not invoke cleanup")
			require.True(t, controllerutil.ContainsFinalizer(obj, Finalizer), "finalizer released without reporting failure")
		})
	}
}

func TestLifecycleRejectsInvalidDeletionPolicies(t *testing.T) {
	for _, observe := range []bool{false, true} {
		obj := newConfig(DeletionPolicyDelete, 0)
		policy := DriftPolicyObserve
		if observe {
			obj.Spec.Reconcile.DriftPolicy = policy
		}
		c := testClient(obj).WithStatusSubresource(obj).Build()
		var remove func(context.Context) error
		if observe {
			remove = func(context.Context) error { require.Fail(t, "normal reconcile must not clean up"); return nil }
		}
		done, after := HandleLifecycle(context.Background(), c, nil, obj, Lifecycle{Remove: remove, Prune: true})
		require.True(t, done, "invalid policy was not stopped")
		require.Positive(t, after, "invalid policy was not stopped")
		condition := meta.FindStatusCondition(obj.Status.Conditions, "Synced")
		require.NotNil(t, condition, "missing invalid config condition: %+v", obj.Status.Conditions)
		require.Equal(t, "InvalidConfig", condition.Reason, "missing invalid config condition")
	}
}

// Only a resource the operator will clean up holds the finalizer, so uninstalling
// the operator or deleting its namespace never leaves the rest in Terminating.
func TestOnlyASupportedDeletePolicyHoldsTheFinalizer(t *testing.T) {
	remove := func(context.Context) error { return nil }
	observe := DriftPolicyObserve
	for _, tc := range []struct {
		name    string
		policy  string
		observe bool
		lc      Lifecycle
		want    bool
	}{
		{name: "orphan by default", lc: Lifecycle{Remove: remove}},
		{name: "orphan", policy: "orphan", lc: Lifecycle{Remove: remove}},
		{name: "delete", policy: DeletionPolicyDelete, lc: Lifecycle{Remove: remove}, want: true},
		{name: "delete where unsupported", policy: DeletionPolicyDelete},
		{name: "delete in observe mode", policy: DeletionPolicyDelete, observe: true, lc: Lifecycle{Remove: remove}},
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

				HandleLifecycle(context.Background(), c, nil, obj, tc.lc)

				assert.Equal(t, tc.want, controllerutil.ContainsFinalizer(obj, Finalizer))
			})
		}
	}
}

// An integration that cannot prune must say so: ignoring prune would leave
// resources removed from the spec in place while the spec claims otherwise.
func TestLifecycleRejectsPruneWhereUnsupported(t *testing.T) {
	for _, tc := range []struct {
		name     string
		lc       Lifecycle
		rejected bool
	}{
		{"unsupported", Lifecycle{}, true},
		{"supported", Lifecycle{Prune: true}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			obj := newConfig("", 0)
			obj.Spec.Reconcile = &commonv1alpha1.ReconcileConfig{Prune: new(true)}
			c := testClient(obj).WithStatusSubresource(obj).Build()

			done, _ := HandleLifecycle(context.Background(), c, nil, obj, tc.lc)

			assert.Equal(t, tc.rejected, done)
			condition := meta.FindStatusCondition(obj.Status.Conditions, "Synced")
			if !tc.rejected {
				assert.Nil(t, condition, "a supported prune was reported: %+v", condition)
				return
			}
			require.NotNil(t, condition)
			assert.Equal(t, "InvalidConfig", condition.Reason)
			assert.Contains(t, condition.Message, "prune is not supported")
		})
	}
}

func TestCleanupAppWithoutOwnedPrunableResourcesNeedsNoCredentials(t *testing.T) {
	obj := newConfig(DeletionPolicyDelete, 0)
	obj.SetManagedResources(map[string][]string{"tags": {"shared"}})
	def := engine.AppDefinition{Resources: []engine.ResourceEndpoint{{Name: "tags"}, {Name: "indexers", Prunable: true}}}
	require.NoError(t, CleanupApp(context.Background(), nil, nil, obj, commonv1alpha1.AppConnection{}, def, "sonarr"))
}
