package reconciler

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

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
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	pvrv1alpha1 "github.com/kyleseneker/arrchitect/api/pvr/v1alpha1"
	"github.com/kyleseneker/arrchitect/internal/engine"
	"github.com/kyleseneker/arrchitect/internal/failure"
	"github.com/kyleseneker/arrchitect/internal/metrics"
	"github.com/kyleseneker/arrchitect/internal/secrets"
)

// statusSonarr is a Sonarr in a fake API server that serves its status.
func statusSonarr(t *testing.T) (*pvrv1alpha1.Sonarr, client.Client) {
	t.Helper()
	obj := &pvrv1alpha1.Sonarr{Name: "sonarr", Namespace: "media"}
	s := runtime.NewScheme()
	require.NoError(t, pvrv1alpha1.AddToScheme(s))
	return obj, fake.NewClientBuilder().WithScheme(s).WithObjects(obj).WithStatusSubresource(obj).Build()
}

// failedWith is the state of a pass that failed with err alone.
func failedWith(err error) state {
	return failedState(outcomeOf([]error{err}), []error{err}, false)
}

func TestEmitPruneEvents(t *testing.T) {
	recorder := events.NewFakeRecorder(2)
	emitPruneEvents(recorder, &pvrv1alpha1.Sonarr{Name: "s", Namespace: "arr"}, []engine.PrunedResource{
		{Type: "indexers", Name: "old", ID: 7},
		{Type: "libraryVariables", Name: "movies/stale"},
	})
	assert.Equal(t, `Warning ResourcePruned Deleted indexers "old" (id=7), which the operator created`, <-recorder.Events)
	assert.Equal(t, `Warning ResourcePruned Deleted libraryVariables "movies/stale", which the operator created`, <-recorder.Events,
		"a resource without a numeric id reported one")
}

func TestPassStateMessages(t *testing.T) {
	for _, tc := range []struct {
		name    string
		observe bool
		synced  []string
		drift   []string
		want    state
	}{
		{"synced", false, nil, nil, state{synced: true, reason: ReasonSynced, message: "all configuration sections synced"}},
		{"synced items", false, []string{"naming", "indexers(a)"}, nil,
			state{synced: true, reason: ReasonSynced, message: "synced: naming, indexers(a)"}},
		{"observe match", true, []string{"naming"}, nil,
			state{synced: true, reason: ReasonObserved, message: "observable configuration matches; no application writes performed"}},
		{"observe drift", true, nil, []string{"flows(a)", "naming", "flows(a)"},
			state{reason: ReasonDriftDetected, message: "differs: flows(a), naming; observe policy prevents application writes"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, passState(tc.observe, tc.synced, tc.drift))
		})
	}
}

func TestFailedStateMessages(t *testing.T) {
	errs := []error{errors.New("flow(a): x"), fmt.Errorf("general: %w", failure.Join(errors.New("series: 503"), errors.New("movies: 503")))}
	st := failedState(outcomeOf(errs), errs, false)
	assert.Equal(t, state{reason: ReasonSyncFailed, message: "flow(a): x; general: series: 503; movies: 503"}, st,
		"joined failures read on one line")

	st = failedState(outcomeOf(errs[:1]), errs[:1], true)
	assert.Equal(t, "observation failed: flow(a): x", st.message)

	st = failedWith(failure.Unauthorized.Errorf("refused"))
	assert.Equal(t, state{reason: ReasonUnauthorized, message: "refused", stalled: true}, st)
}

// A pass with a great many failures still fits in a condition message.
func TestFailedStateNamesTheFirstTenFailures(t *testing.T) {
	errs := make([]error, 0, 500)
	for i := range 500 {
		errs = append(errs, fmt.Errorf("indexers(%d): %s", i, strings.Repeat("x", 200)))
	}
	st := failedState(outcomeOf(errs), errs, false)
	assert.True(t, strings.HasSuffix(st.message, " and 490 more"), st.message)
	assert.Less(t, len(st.message), 32768)
}

func TestListedNamesTheFirstTen(t *testing.T) {
	drift := make([]string, 0, 13)
	for i := range 13 {
		drift = append(drift, fmt.Sprintf("indexers(%d)", i))
	}
	assert.Equal(t, "indexers(0), indexers(1), indexers(2), indexers(3), indexers(4), indexers(5), "+
		"indexers(6), indexers(7), indexers(8), indexers(9) and 3 more", listed(drift, ", "))
}

// A spec edit while a pass runs must not cost the status write, which would
// lose the ownership the pass recorded, leaving resources the operator created
// that it can never prune or clean up.
func TestRecordSurvivesASpecEditMidPass(t *testing.T) {
	ctx := context.Background()
	obj, cl := statusSonarr(t)
	key := client.ObjectKeyFromObject(obj)

	stale := &pvrv1alpha1.Sonarr{}
	require.NoError(t, cl.Get(ctx, key, stale))
	base := stale.DeepCopy()
	edited := stale.DeepCopy()
	edited.Spec.Connection.URL = "http://sonarr.edited:8989"
	require.NoError(t, cl.Update(ctx, edited))

	stale.Status.SetManagedResources(map[string][]string{"downloadClients": {"qbit"}})
	require.NoError(t, record(ctx, cl, nil, stale, base, state{synced: true, reason: ReasonSynced, message: "ok"}))

	got := &pvrv1alpha1.Sonarr{}
	require.NoError(t, cl.Get(ctx, key, got))
	assert.Equal(t, map[string][]string{"downloadClients": {"qbit"}}, got.Status.GetManagedResources(), "ownership was lost")
	condition := meta.FindStatusCondition(got.Status.Conditions, ConditionSynced)
	require.NotNil(t, condition)
	assert.Equal(t, metav1.ConditionTrue, condition.Status)
	assert.Equal(t, "http://sonarr.edited:8989", got.Spec.Connection.URL, "the status write reverted the spec edit")
}

func TestRecordOutlivesACancelledReconcile(t *testing.T) {
	obj, cl := statusSonarr(t)
	base := obj.DeepCopy()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	obj.Status.SetManagedResources(map[string][]string{"downloadClients": {"qbit"}})
	require.NoError(t, record(ctx, cl, nil, obj, base, state{synced: true, reason: ReasonSynced, message: "ok"}))

	got := &pvrv1alpha1.Sonarr{}
	require.NoError(t, cl.Get(context.Background(), client.ObjectKeyFromObject(obj), got))
	assert.Equal(t, map[string][]string{"downloadClients": {"qbit"}}, got.Status.GetManagedResources())
}

// A condition a pass drops leaves the status too: the merge patch, taken over
// the resource as the pass read it, carries the removal.
func TestRecordClearsADroppedCondition(t *testing.T) {
	obj, c := statusSonarr(t)
	require.NoError(t, record(context.Background(), c, nil, obj, obj.DeepCopy(), failedWith(failure.InvalidConfig.Errorf("x"))))
	require.True(t, meta.IsStatusConditionTrue(obj.Status.Conditions, ConditionStalled))

	require.NoError(t, record(context.Background(), c, nil, obj, obj.DeepCopy(), passState(false, nil, nil)))

	got := &pvrv1alpha1.Sonarr{}
	require.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(obj), got))
	assert.Nil(t, meta.FindStatusCondition(got.Status.Conditions, ConditionStalled), "the patch left Stalled in place")
}

func TestConfigSyncedGaugeTracksOutcome(t *testing.T) {
	obj, c := statusSonarr(t)

	require.NoError(t, record(context.Background(), c, nil, obj, obj.DeepCopy(), passState(false, nil, nil)))
	assert.Equal(t, 1.0, testutil.ToFloat64(metrics.ConfigSynced.WithLabelValues("Sonarr", "media", "sonarr")),
		"expected gauge 1 after a successful sync")

	require.NoError(t, record(context.Background(), c, nil, obj, obj.DeepCopy(), failedWith(failure.Unreachable.Errorf("boom"))))
	assert.Equal(t, 0.0, testutil.ToFloat64(metrics.ConfigSynced.WithLabelValues("Sonarr", "media", "sonarr")),
		"expected gauge 0 after failure")
}

func TestForgetDeletedDropsTheSyncedSeries(t *testing.T) {
	obj := &pvrv1alpha1.Sonarr{Name: "gone", Namespace: "media"}
	key := types.NamespacedName{Name: "gone", Namespace: "media"}
	metrics.ConfigSynced.WithLabelValues("Sonarr", "media", "gone").Set(0)

	err := forgetDeleted(obj, key, fmt.Errorf("connection refused"))
	assert.EqualError(t, err, "connection refused")
	assert.True(t, metrics.ConfigSynced.DeleteLabelValues("Sonarr", "media", "gone"), "a read failure isn't a deletion")

	metrics.ConfigSynced.WithLabelValues("Sonarr", "media", "gone").Set(0)
	assert.NoError(t, forgetDeleted(obj, key, apierrors.NewNotFound(schema.GroupResource{Resource: "sonarrs"}, "gone")))
	assert.False(t, metrics.ConfigSynced.DeleteLabelValues("Sonarr", "media", "gone"), "the series outlived the resource")
}

// The secrets a resource wrote and the ownership no status write recorded are
// kept by its ownerKey, which a deletion drops for every incarnation of it.
func TestForgetDeletedForgetsWhatTheResourceLeftBehind(t *testing.T) {
	obj := &pvrv1alpha1.Sonarr{Name: t.Name(), Namespace: "media", UID: "uid-1"}
	key := types.NamespacedName{Name: t.Name(), Namespace: "media"}
	secrets.RecordSecrets(ownerKey(obj), "settings/host", "digest")
	require.False(t, secrets.SecretsDiffer(ownerKey(obj), "settings/host", "digest", false))
	remember(obj, map[string][]string{"tags": {"created"}}, false)

	require.NoError(t, forgetDeleted(&pvrv1alpha1.Sonarr{}, key, apierrors.NewNotFound(schema.GroupResource{Resource: "sonarrs"}, t.Name())))

	assert.True(t, secrets.SecretsDiffer(ownerKey(obj), "settings/host", "digest", false), "the deleted resource's record outlived it")
	assert.Empty(t, owned(obj), "the deleted resource's unrecorded ownership outlived it")
}

func TestReadySumsUpSyncedAndStalledMarksWhatNoRetryFixes(t *testing.T) {
	obj, c := statusSonarr(t)
	for _, step := range []struct {
		state   state
		ready   metav1.ConditionStatus
		stalled bool
	}{
		{passState(false, nil, nil), metav1.ConditionTrue, false},
		{failedWith(errors.New("refused")), metav1.ConditionFalse, false},
		{failedWith(failure.Unreachable.Errorf("down")), metav1.ConditionFalse, false},
		{failedWith(failure.InvalidConfig.Errorf("bad")), metav1.ConditionFalse, true},
		{failedWith(failure.Unauthorized.Errorf("refused")), metav1.ConditionFalse, true},
		{failedWith(failure.NameConflict.Errorf("taken")), metav1.ConditionFalse, true},
		{passState(true, nil, nil), metav1.ConditionTrue, false},
	} {
		require.NoError(t, record(context.Background(), c, nil, obj, obj.DeepCopy(), step.state))
		ready := meta.FindStatusCondition(obj.Status.Conditions, ConditionReady)
		require.NotNil(t, ready, step.state.reason)
		assert.Equal(t, step.ready, ready.Status, step.state.reason)
		assert.Equal(t, step.state.reason, ready.Reason, "Ready carries Synced's reason")
		assert.Equal(t, step.stalled, meta.IsStatusConditionTrue(obj.Status.Conditions, ConditionStalled), step.state.reason)
	}
}

func TestReadyChangesAreRecordedAsEvents(t *testing.T) {
	obj, c := statusSonarr(t)
	recorder := events.NewFakeRecorder(10)
	write := func(st state) {
		require.NoError(t, record(context.Background(), c, recorder, obj, obj.DeepCopy(), st))
	}

	write(passState(false, nil, nil))
	write(failedWith(failure.Unreachable.Errorf("down")))
	write(failedWith(failure.Unreachable.Errorf("down")))
	write(failedWith(errors.New("refused")))
	write(passState(false, nil, nil))
	write(passState(false, nil, nil))

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
		obj.GetConfigStatus().SetManagedResources(managed)
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

	require.NoError(t, forgetDeleted(&pvrv1alpha1.Sonarr{}, types.NamespacedName{Namespace: "media", Name: "tv"},
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

	require.NoError(t, record(context.Background(), c, recorder, obj, obj.DeepCopy(),
		failedWith(errors.New("indexers(Private): the tracker said key abc123 is banned"))))

	assert.Equal(t, "Warning SyncFailed The app refused some settings. The Ready condition says which.", <-recorder.Events,
		"an event can reach readers who can't read the resource")
	ready := meta.FindStatusCondition(obj.Status.Conditions, ConditionReady)
	require.NotNil(t, ready)
	assert.Contains(t, ready.Message, "the tracker said", "the details stay in the condition")
}

func TestNoEventWithoutTheStatusItDescribes(t *testing.T) {
	obj := &pvrv1alpha1.Sonarr{Name: "never-stored", Namespace: "media"}
	s := runtime.NewScheme()
	require.NoError(t, pvrv1alpha1.AddToScheme(s))
	c := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(obj).Build()
	recorder := events.NewFakeRecorder(1)

	require.NoError(t, record(context.Background(), c, recorder, obj, obj.DeepCopy(), failedWith(failure.Unreachable.Errorf("connection refused"))),
		"a resource deleted mid-pass needs no status")

	assert.Empty(t, recorder.Events, "the status write failed, so the event would describe a state nobody can read")
}
