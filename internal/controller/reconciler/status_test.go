package reconciler

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	pvrv1alpha1 "github.com/kyleseneker/arrchitect/api/pvr/v1alpha1"
	"github.com/kyleseneker/arrchitect/internal/engine"
	"github.com/kyleseneker/arrchitect/internal/failure"
	"github.com/kyleseneker/arrchitect/internal/httpapi"
	"github.com/kyleseneker/arrchitect/internal/metrics"
	"github.com/kyleseneker/arrchitect/internal/secrets"
)

func TestEmitPruneEvents(t *testing.T) {
	recorder := events.NewFakeRecorder(2)
	EmitPruneEvents(recorder, newConfig("", 0), []engine.PrunedResource{
		{Type: "indexers", Name: "old", ID: 7},
		{Type: "libraryVariables", Name: "movies/stale"},
	})
	assert.Equal(t, `Warning ResourcePruned Deleted indexers "old" (id=7), which the operator created`, <-recorder.Events)
	assert.Equal(t, `Warning ResourcePruned Deleted libraryVariables "movies/stale", which the operator created`, <-recorder.Events,
		"a resource without a numeric id reported one")
}

func TestReportResultReportsObservedDrift(t *testing.T) {
	for _, tc := range []struct {
		name   string
		policy string
		result Result
		status metav1.ConditionStatus
		reason string
	}{
		{"enforce", "enforce", Result{Synced: []string{"naming"}}, metav1.ConditionTrue, ReasonSynced},
		{"observe drift", DriftPolicyObserve, Result{Drift: []string{"naming"}}, metav1.ConditionFalse, "DriftDetected"},
		{"observe match", DriftPolicyObserve, Result{Synced: []string{"naming"}}, metav1.ConditionTrue, "Observed"},
		{"observe error", DriftPolicyObserve, Result{Errors: []error{errors.New("x")}}, metav1.ConditionFalse, ReasonSyncFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			obj := newConfig("", 0)
			obj.Spec.Reconcile = &commonv1alpha1.ReconcileConfig{DriftPolicy: tc.policy}
			c := testClient(obj).WithStatusSubresource(obj).Build()
			ReportResult(context.Background(), c, nil, obj, ObserveOnly(obj.Spec.Reconcile), tc.result)
			require.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(obj), obj))
			condition := meta.FindStatusCondition(obj.Status.Conditions, "Synced")
			require.NotNil(t, condition)
			assert.Equal(t, tc.status, condition.Status)
			assert.Equal(t, tc.reason, condition.Reason)
		})
	}
}

// A spec edit while a pass runs makes the status write conflict. Dropping it
// would lose the ownership the pass recorded, leaving resources the operator
// created that it can never prune or clean up.
func TestUpdateStatusSurvivesASpecEditMidPass(t *testing.T) {
	ctx := context.Background()
	obj := newConfig("", 0)
	cl := testClient(obj).WithStatusSubresource(obj).Build()
	key := client.ObjectKeyFromObject(obj)

	stale := &pvrv1alpha1.Sonarr{}
	require.NoError(t, cl.Get(ctx, key, stale))
	edited := &pvrv1alpha1.Sonarr{}
	require.NoError(t, cl.Get(ctx, key, edited))
	edited.Spec.Connection.URL = "http://sonarr.edited:8989"
	require.NoError(t, cl.Update(ctx, edited))
	require.True(t, apierrors.IsConflict(cl.Status().Update(ctx, stale.DeepCopy())),
		"precondition: a plain status write from the stale copy conflicts")

	UpdateStatusManaged(stale, map[string][]string{"downloadClients": {"qbit"}})
	UpdateStatus(ctx, cl, nil, stale, true, ReasonSynced, "ok")

	got := &pvrv1alpha1.Sonarr{}
	require.NoError(t, cl.Get(ctx, key, got))
	assert.Equal(t, map[string][]string{"downloadClients": {"qbit"}}, got.Status.GetManagedResources(), "ownership was lost")
	condition := meta.FindStatusCondition(got.Status.Conditions, "Synced")
	require.NotNil(t, condition)
	assert.Equal(t, metav1.ConditionTrue, condition.Status)
	assert.Equal(t, "http://sonarr.edited:8989", got.Spec.Connection.URL, "the status write reverted the spec edit")
}

// lagging serves reads from a copy taken before a spec edit, as the informer
// cache can right after one, and reads the latest version through APIReader.
type lagging struct {
	client.Client
	snapshot *pvrv1alpha1.Sonarr
}

func (l lagging) Get(_ context.Context, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
	*obj.(*pvrv1alpha1.Sonarr) = *l.snapshot.DeepCopy()
	return nil
}

func (l lagging) APIReader() client.Reader { return l.Client }

func TestUpdateStatusReadsPastALaggingCache(t *testing.T) {
	ctx := context.Background()
	obj := newConfig("", 0)
	cl := testClient(obj).WithStatusSubresource(obj).Build()
	key := client.ObjectKeyFromObject(obj)

	stale := &pvrv1alpha1.Sonarr{}
	require.NoError(t, cl.Get(ctx, key, stale))
	edited := stale.DeepCopy()
	edited.Spec.Connection.URL = "http://sonarr.edited:8989"
	require.NoError(t, cl.Update(ctx, edited))
	cache := lagging{Client: cl, snapshot: stale.DeepCopy()}

	UpdateStatusManaged(stale, map[string][]string{"downloadClients": {"qbit"}})
	UpdateStatus(ctx, cache, nil, stale, true, ReasonSynced, "ok")

	got := &pvrv1alpha1.Sonarr{}
	require.NoError(t, cl.Get(ctx, key, got))
	assert.Equal(t, map[string][]string{"downloadClients": {"qbit"}}, got.Status.GetManagedResources(),
		"retrying against the cached version kept conflicting, and the ownership was lost")
}

func TestUpdateStatusOutlivesACancelledReconcile(t *testing.T) {
	obj := newConfig("", 0)
	cl := testClient(obj).WithStatusSubresource(obj).Build()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	UpdateStatusManaged(obj, map[string][]string{"downloadClients": {"qbit"}})
	UpdateStatus(ctx, cl, nil, obj, true, ReasonSynced, "ok")

	got := &pvrv1alpha1.Sonarr{}
	require.NoError(t, cl.Get(context.Background(), client.ObjectKeyFromObject(obj), got))
	assert.Equal(t, map[string][]string{"downloadClients": {"qbit"}}, got.Status.GetManagedResources())
}

func TestReportResultMessages(t *testing.T) {
	missing := &secrets.SecretError{}
	for _, tc := range []struct {
		name    string
		observe bool
		result  Result
		status  metav1.ConditionStatus
		reason  string
		message string
	}{
		{"synced", false, Result{}, metav1.ConditionTrue, ReasonSynced, "all configuration sections synced"},
		{"synced items", false, Result{Synced: []string{"naming", "indexers(a)"}}, metav1.ConditionTrue, ReasonSynced,
			"synced: naming, indexers(a)"},
		{"partial failure", false, Result{Synced: []string{"naming"}, Errors: []error{errors.New("flow(a): x"), errors.New("flow(b): y")}},
			metav1.ConditionFalse, ReasonSyncFailed, "flow(a): x; flow(b): y"},
		{"failures that share a reason", false, Result{Errors: []error{
			fmt.Errorf("settings: %w", missing), fmt.Errorf("library(a): %w", missing),
		}}, metav1.ConditionFalse, ReasonSecretNotFound, ""},
		{"failures with different reasons", false, Result{Errors: []error{
			fmt.Errorf("settings: %w", missing), errors.New("flow(a): x"),
		}}, metav1.ConditionFalse, ReasonSyncFailed, ""},
		{"joined failures with different reasons", false, Result{Errors: []error{
			fmt.Errorf("general: %w", failure.Join(fmt.Errorf("series: %w", missing), &httpapi.APIError{StatusCode: 401})),
		}}, metav1.ConditionFalse, ReasonSyncFailed, ""},
		{"joined failures that share a reason", false, Result{Errors: []error{
			fmt.Errorf("sonarr: %w", failure.Join(fmt.Errorf("a: %w", missing), fmt.Errorf("b: %w", missing))),
		}}, metav1.ConditionFalse, ReasonSecretNotFound, ""},
		{"joined failures read on one line", false, Result{Errors: []error{
			fmt.Errorf("general: %w", failure.Join(errors.New("series: 503"), errors.New("movies: 503"))),
		}}, metav1.ConditionFalse, ReasonSyncFailed, "general: series: 503; movies: 503"},
		{"a spec problem around joined failures", false, Result{Errors: []error{
			failure.InvalidConfig(failure.Join(errors.New("a"), &httpapi.APIError{StatusCode: 401})),
		}}, metav1.ConditionFalse, ReasonInvalidConfig, ""},
		{"observe match", true, Result{}, metav1.ConditionTrue, "Observed", ""},
		{"observe drift", true, Result{Drift: []string{"flows(a)", "naming", "flows(a)"}}, metav1.ConditionFalse, "DriftDetected",
			"differs: flows(a), naming; observe policy prevents application writes"},
		{"observe error precedes drift", true, Result{Drift: []string{"flows(a)"}, Errors: []error{errors.New("x")}},
			metav1.ConditionFalse, ReasonSyncFailed, "observation failed: x"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			obj := newConfig("", 0)
			c := testClient(obj).WithStatusSubresource(obj).Build()
			ReportResult(context.Background(), c, nil, obj, tc.observe, tc.result)
			require.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(obj), obj))
			condition := meta.FindStatusCondition(obj.Status.Conditions, "Synced")
			require.NotNil(t, condition)
			assert.Equal(t, tc.status, condition.Status)
			assert.Equal(t, tc.reason, condition.Reason)
			if tc.message != "" {
				assert.Equal(t, tc.message, condition.Message)
			}
		})
	}
}

// A failure that a retry can fix sooner than the resource's interval, such as
// a Secret not created yet, asks for the next pass in its own time. Mixed with
// a write the app refused, it waits for the interval, so the refused write
// isn't sent again sooner.
func TestReportResultRetriesSoonerOnlyForASharedReason(t *testing.T) {
	for _, tc := range []struct {
		name   string
		errors []error
		min    time.Duration
		max    time.Duration
	}{
		{"none", nil, 0, 0},
		{"the app refused", []error{errors.New("flow(a): x")}, 0, 0},
		{"a missing Secret", []error{fmt.Errorf("settings: %w", &secrets.SecretError{})}, 30 * time.Second, 33 * time.Second},
		{"a missing Secret and a refused write", []error{errors.New("flow(a): x"), fmt.Errorf("settings: %w", &secrets.SecretError{})}, 0, 0},
		{"refused credentials", []error{fmt.Errorf("flow(a): %w", &httpapi.APIError{StatusCode: 401})}, 0, 0},
		{"a missing Secret and refused credentials", []error{
			fmt.Errorf("settings: %w", &secrets.SecretError{}), fmt.Errorf("flow(a): %w", &httpapi.APIError{StatusCode: 401}),
		}, 0, 0},
		{"a spec problem", []error{fmt.Errorf("importLists(a): %w", failure.InvalidConfig(errors.New("x")))}, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			obj := newConfig("", 0)
			c := testClient(obj).WithStatusSubresource(obj).Build()
			retry := ReportResult(context.Background(), c, nil, obj, false, Result{Errors: tc.errors})
			assert.GreaterOrEqual(t, retry, tc.min)
			assert.LessOrEqual(t, retry, tc.max)
		})
	}
}

// A pass with a great many failures still fits in a condition message.
func TestReportResultNamesTheFirstTenFailures(t *testing.T) {
	obj := newConfig("", 0)
	c := testClient(obj).WithStatusSubresource(obj).Build()
	errs := make([]error, 0, 500)
	for i := range 500 {
		errs = append(errs, fmt.Errorf("indexers(%d): %s", i, strings.Repeat("x", 200)))
	}
	ReportResult(context.Background(), c, nil, obj, false, Result{Errors: errs})
	require.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(obj), obj))
	condition := meta.FindStatusCondition(obj.Status.Conditions, "Synced")
	require.NotNil(t, condition)
	assert.True(t, strings.HasSuffix(condition.Message, " and 490 more"), condition.Message)
	assert.Less(t, len(condition.Message), 32768)
}

func TestConfigSyncedGaugeTracksOutcome(t *testing.T) {
	c := &pvrv1alpha1.Sonarr{
		Name: "s", Namespace: "arr",
		Kind: "Sonarr",
	}
	_ = pvrv1alpha1.AddToScheme(scheme.Scheme)
	cl := fake.NewClientBuilder().WithScheme(scheme.Scheme).
		WithObjects(c).WithStatusSubresource(c).Build()

	UpdateStatus(context.Background(), cl, nil, c, true, "Synced", "ok")
	assert.Equal(t, 1.0, testutil.ToFloat64(metrics.ConfigSynced.WithLabelValues("Sonarr", "arr", "s")),
		"expected gauge 1 after a successful sync")

	UpdateStatus(context.Background(), cl, nil, c, false, "AppUnreachable", "boom")
	assert.Equal(t, 0.0, testutil.ToFloat64(metrics.ConfigSynced.WithLabelValues("Sonarr", "arr", "s")),
		"expected gauge 0 after failure")
}

func TestListedNamesTheFirstTen(t *testing.T) {
	drift := make([]string, 0, 13)
	for i := range 13 {
		drift = append(drift, fmt.Sprintf("indexers(%d)", i))
	}
	assert.Equal(t, "indexers(0), indexers(1), indexers(2), indexers(3), indexers(4), indexers(5), "+
		"indexers(6), indexers(7), indexers(8), indexers(9) and 3 more", listed(drift, ", "))
}

func TestIgnoreDeletedDropsTheSyncedSeries(t *testing.T) {
	obj := &pvrv1alpha1.Sonarr{Name: "gone", Namespace: "media"}
	key := types.NamespacedName{Name: "gone", Namespace: "media"}
	metrics.ConfigSynced.WithLabelValues("Sonarr", "media", "gone").Set(0)

	err := IgnoreDeleted(obj, key, fmt.Errorf("connection refused"))
	assert.EqualError(t, err, "connection refused")
	assert.True(t, metrics.ConfigSynced.DeleteLabelValues("Sonarr", "media", "gone"), "a read failure isn't a deletion")

	metrics.ConfigSynced.WithLabelValues("Sonarr", "media", "gone").Set(0)
	assert.NoError(t, IgnoreDeleted(obj, key, apierrors.NewNotFound(schema.GroupResource{Resource: "sonarrs"}, "gone")))
	assert.False(t, metrics.ConfigSynced.DeleteLabelValues("Sonarr", "media", "gone"), "the series outlived the resource")
}

// The secrets a resource wrote are remembered by its OwnerKey, which a
// deletion drops for every incarnation of it.
func TestIgnoreDeletedForgetsTheSecretsTheResourceWrote(t *testing.T) {
	obj := &pvrv1alpha1.Sonarr{Name: t.Name(), Namespace: "media", UID: "uid-1"}
	key := types.NamespacedName{Name: t.Name(), Namespace: "media"}
	secrets.RecordSecrets(OwnerKey(obj), "settings/host", "digest")
	require.False(t, secrets.SecretsDiffer(OwnerKey(obj), "settings/host", "digest", false))

	require.NoError(t, IgnoreDeleted(&pvrv1alpha1.Sonarr{}, key, apierrors.NewNotFound(schema.GroupResource{Resource: "sonarrs"}, t.Name())))

	assert.True(t, secrets.SecretsDiffer(OwnerKey(obj), "settings/host", "digest", false), "the deleted resource's record outlived it")
}

// statusSonarr is a Sonarr in a fake API server that serves its status.
func statusSonarr(t *testing.T) (*pvrv1alpha1.Sonarr, client.Client) {
	t.Helper()
	obj := &pvrv1alpha1.Sonarr{Name: "sonarr", Namespace: "media"}
	s := runtime.NewScheme()
	require.NoError(t, pvrv1alpha1.AddToScheme(s))
	return obj, fake.NewClientBuilder().WithScheme(s).WithObjects(obj).WithStatusSubresource(obj).Build()
}

func TestReadySumsUpSyncedAndStalledMarksWhatNoRetryFixes(t *testing.T) {
	obj, c := statusSonarr(t)
	for _, step := range []struct {
		synced  bool
		reason  string
		ready   metav1.ConditionStatus
		stalled bool
	}{
		{true, ReasonSynced, metav1.ConditionTrue, false},
		{false, ReasonSyncFailed, metav1.ConditionFalse, false},
		{false, ReasonAppUnreachable, metav1.ConditionFalse, false},
		{false, ReasonInvalidConfig, metav1.ConditionFalse, true},
		{false, ReasonUnauthorized, metav1.ConditionFalse, true},
		{true, ReasonObserved, metav1.ConditionTrue, false},
	} {
		UpdateStatus(context.Background(), c, nil, obj, step.synced, step.reason, "message")
		ready := meta.FindStatusCondition(obj.Status.Conditions, ConditionReady)
		require.NotNil(t, ready, step.reason)
		assert.Equal(t, step.ready, ready.Status, step.reason)
		assert.Equal(t, step.reason, ready.Reason, "Ready carries Synced's reason")
		assert.Equal(t, step.stalled, meta.IsStatusConditionTrue(obj.Status.Conditions, ConditionStalled), step.reason)
	}
}

func TestReadyChangesAreRecordedAsEvents(t *testing.T) {
	obj, c := statusSonarr(t)
	recorder := events.NewFakeRecorder(10)
	update := func(synced bool, reason string) {
		UpdateStatus(context.Background(), c, recorder, obj, synced, reason, "message")
	}

	update(true, ReasonSynced)
	update(false, ReasonAppUnreachable)
	update(false, ReasonAppUnreachable)
	update(false, ReasonSyncFailed)
	update(true, ReasonSynced)
	update(true, ReasonSynced)

	close(recorder.Events)
	var got []string
	for e := range recorder.Events {
		got = append(got, e)
	}
	assert.Equal(t, []string{
		"Warning AppUnreachable The app didn't respond.",
		"Warning SyncFailed The app refused some settings. The Ready condition says which.",
		"Normal Synced The app matches the resource again.",
	}, got, "one event per change of Ready, none for the first success or a repeat")
}

func TestManagedResourcesGaugeFollowsEachResource(t *testing.T) {
	sonarr := &pvrv1alpha1.Sonarr{Name: "tv", Namespace: "media"}
	sonarr4k := &pvrv1alpha1.Sonarr{Name: "tv-4k", Namespace: "media"}
	own := func(obj ConfigResource, managed map[string][]string) {
		UpdateStatusManaged(obj, managed)
		recordManaged(obj)
	}
	gauge := func(name, resourceType string) float64 {
		return testutil.ToFloat64(metrics.ManagedResources.WithLabelValues("Sonarr", "media", name, resourceType))
	}

	own(sonarr, map[string][]string{"tags": {"a", "b"}, "indexers": {"x"}})
	own(sonarr4k, map[string][]string{"tags": {"c"}})
	assert.Equal(t, 2.0, gauge("tv", "tags"), "each resource has its own series")
	assert.Equal(t, 1.0, gauge("tv-4k", "tags"))

	own(sonarr, map[string][]string{"tags": {"a"}})
	assert.Equal(t, 1, managedSeries(t, "tv"), "a type the resource no longer owns loses its series")

	require.NoError(t, IgnoreDeleted(&pvrv1alpha1.Sonarr{}, types.NamespacedName{Namespace: "media", Name: "tv"},
		apierrors.NewNotFound(schema.GroupResource{Resource: "sonarrs"}, "tv")))
	assert.Equal(t, 1.0, gauge("tv-4k", "tags"), "another resource's series stay")
	assert.Zero(t, managedSeries(t, "tv"), "a deleted resource loses its series")
}

// managedSeries counts the managed_resources series of the Sonarr name.
func managedSeries(t *testing.T, name string) int {
	t.Helper()
	ch := make(chan prometheus.Metric, 64)
	metrics.ManagedResources.Collect(ch)
	close(ch)
	n := 0
	for m := range ch {
		var out dto.Metric
		require.NoError(t, m.Write(&out))
		labels := map[string]string{}
		for _, l := range out.GetLabel() {
			labels[l.GetName()] = l.GetValue()
		}
		if labels["kind"] == "Sonarr" && labels["namespace"] == "media" && labels["name"] == name {
			n++
		}
	}
	return n
}

func TestEventNotesKeepTheAppsWordsOut(t *testing.T) {
	obj, c := statusSonarr(t)
	recorder := events.NewFakeRecorder(1)

	UpdateStatus(context.Background(), c, recorder, obj, false, ReasonSyncFailed, "indexers(Private): the tracker said key abc123 is banned")

	assert.Equal(t, "Warning SyncFailed The app refused some settings. The Ready condition says which.", <-recorder.Events,
		"an event can reach readers who can't read the resource")
	ready := meta.FindStatusCondition(obj.Status.Conditions, ConditionReady)
	require.NotNil(t, ready)
	assert.Contains(t, ready.Message, "the tracker said", "the details stay in the condition")
}

func TestManagedResourcesGaugeSurvivesARestartWhileTheAppIsDown(t *testing.T) {
	obj, c := statusSonarr(t)
	obj.Name = "restarted"
	obj.Status.SetManagedResources(map[string][]string{"tags": {"a", "b"}})

	UpdateStatus(context.Background(), c, nil, obj, false, ReasonAppUnreachable, "connection refused")

	assert.Equal(t, 2.0, testutil.ToFloat64(metrics.ManagedResources.WithLabelValues("Sonarr", "media", "restarted", "tags")),
		"the series comes from the ownership status records, not only from a successful sync")
}

func TestNoEventWithoutTheStatusItDescribes(t *testing.T) {
	obj := &pvrv1alpha1.Sonarr{Name: "never-stored", Namespace: "media"}
	s := runtime.NewScheme()
	require.NoError(t, pvrv1alpha1.AddToScheme(s))
	c := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(obj).Build()
	recorder := events.NewFakeRecorder(1)

	UpdateStatus(context.Background(), c, recorder, obj, false, ReasonAppUnreachable, "connection refused")

	assert.Empty(t, recorder.Events, "the status write failed, so the event would describe a state nobody can read")
}
