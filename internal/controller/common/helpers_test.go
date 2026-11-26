package common

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
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
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	downloadsv1alpha1 "github.com/kyleseneker/arrchitect/api/downloads/v1alpha1"
	indexersv1alpha1 "github.com/kyleseneker/arrchitect/api/indexers/v1alpha1"
	pvrv1alpha1 "github.com/kyleseneker/arrchitect/api/pvr/v1alpha1"
	subtitlesv1alpha1 "github.com/kyleseneker/arrchitect/api/subtitles/v1alpha1"
	"github.com/kyleseneker/arrchitect/internal/engine"
	"github.com/kyleseneker/arrchitect/internal/metrics"
)

func TestReconcileInterval(t *testing.T) {
	tests := []struct {
		name string
		rc   *commonv1alpha1.ReconcileConfig
		want time.Duration
	}{
		{"nil config", nil, DefaultReconcileInterval},
		{"nil interval", &commonv1alpha1.ReconcileConfig{}, DefaultReconcileInterval},
		{"30s", &commonv1alpha1.ReconcileConfig{IntervalSeconds: int32(30)}, 30 * time.Second},
		{"10m", &commonv1alpha1.ReconcileConfig{IntervalSeconds: int32(600)}, 10 * time.Minute},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ReconcileInterval(tt.rc))
		})
	}
}

func TestPruneEnabled(t *testing.T) {
	tests := []struct {
		name string
		rc   *commonv1alpha1.ReconcileConfig
		want bool
	}{
		{"nil config", nil, false},
		{"nil prune", &commonv1alpha1.ReconcileConfig{}, false},
		{"false", &commonv1alpha1.ReconcileConfig{Prune: new(false)}, false},
		{"true", &commonv1alpha1.ReconcileConfig{Prune: new(true)}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, PruneEnabled(tt.rc))
		})
	}
}

func TestResultReason(t *testing.T) {
	tests := []struct {
		name   string
		result engine.ReconcileResult
		want   string
	}{
		{"success", engine.ReconcileResult{Synced: []string{"a"}}, engine.ReasonSynced},
		{"failure", engine.ReconcileResult{Errors: []string{"fail"}}, engine.ReasonSyncFailed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, resultReason(tt.result))
		})
	}
}

func TestBoolTo01(t *testing.T) {
	tests := []struct {
		name string
		b    bool
		want string
	}{
		{"true", true, "1"},
		{"false", false, "0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, BoolTo01(tt.b))
		})
	}
}

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

func TestObservationStatus(t *testing.T) {
	for _, tt := range []struct {
		name    string
		drift   []string
		err     error
		reason  string
		synced  metav1.ConditionStatus
		message string
	}{
		{"matching", nil, nil, "Observed", metav1.ConditionTrue, ""},
		{"drift", []string{"indexers(Torznab)", "naming", "indexers(Torznab)"}, nil, "DriftDetected", metav1.ConditionFalse,
			"differs: indexers(Torznab), naming; observe policy prevents application writes"},
		{"error precedes drift", []string{"naming"}, fmt.Errorf("read failed"), engine.ReasonSyncFailed, metav1.ConditionFalse, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			obj := newConfig("", 0)
			c := testClient(obj).WithStatusSubresource(obj).Build()
			updateObservationStatus(context.Background(), c, nil, obj, tt.drift, tt.err)
			require.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(obj), obj))
			condition := meta.FindStatusCondition(obj.Status.Conditions, "Synced")
			require.NotNil(t, condition)
			assert.Equal(t, tt.reason, condition.Reason)
			assert.Equal(t, tt.synced, condition.Status)
			if tt.message != "" {
				assert.Equal(t, tt.message, condition.Message)
			}
		})
	}
}

func TestReportResultReportsObservedDrift(t *testing.T) {
	for _, tc := range []struct {
		name   string
		policy string
		result engine.ReconcileResult
		status metav1.ConditionStatus
		reason string
	}{
		{"enforce", "enforce", engine.ReconcileResult{Synced: []string{"naming"}}, metav1.ConditionTrue, engine.ReasonSynced},
		{"observe drift", DriftPolicyObserve, engine.ReconcileResult{Drift: []string{"naming"}}, metav1.ConditionFalse, "DriftDetected"},
		{"observe match", DriftPolicyObserve, engine.ReconcileResult{Synced: []string{"naming"}}, metav1.ConditionTrue, "Observed"},
		{"observe error", DriftPolicyObserve, engine.ReconcileResult{Errors: []string{"x"}}, metav1.ConditionFalse, engine.ReasonSyncFailed},
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
	UpdateStatus(ctx, cl, nil, stale, true, engine.ReasonSynced, "ok")

	got := &pvrv1alpha1.Sonarr{}
	require.NoError(t, cl.Get(ctx, key, got))
	assert.Equal(t, map[string][]string{"downloadClients": {"qbit"}}, got.GetManagedResources(), "ownership was lost")
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
	UpdateStatus(ctx, cache, nil, stale, true, engine.ReasonSynced, "ok")

	got := &pvrv1alpha1.Sonarr{}
	require.NoError(t, cl.Get(ctx, key, got))
	assert.Equal(t, map[string][]string{"downloadClients": {"qbit"}}, got.GetManagedResources(),
		"retrying against the cached version kept conflicting, and the ownership was lost")
}

func TestUpdateStatusOutlivesACancelledReconcile(t *testing.T) {
	obj := newConfig("", 0)
	cl := testClient(obj).WithStatusSubresource(obj).Build()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	UpdateStatusManaged(obj, map[string][]string{"downloadClients": {"qbit"}})
	UpdateStatus(ctx, cl, nil, obj, true, engine.ReasonSynced, "ok")

	got := &pvrv1alpha1.Sonarr{}
	require.NoError(t, cl.Get(context.Background(), client.ObjectKeyFromObject(obj), got))
	assert.Equal(t, map[string][]string{"downloadClients": {"qbit"}}, got.GetManagedResources())
}

func TestReportResultMessages(t *testing.T) {
	for _, tc := range []struct {
		name    string
		observe bool
		drift   []string
		errors  []string
		status  metav1.ConditionStatus
		reason  string
		message string
	}{
		{"synced", false, nil, nil, metav1.ConditionTrue, engine.ReasonSynced, "all configuration sections synced"},
		{"partial failure", false, nil, []string{"flow(a): x"}, metav1.ConditionFalse, engine.ReasonSyncFailed, "errors: [flow(a): x]"},
		{"observe match", true, nil, nil, metav1.ConditionTrue, "Observed", ""},
		{"observe drift", true, []string{"flows(a)"}, nil, metav1.ConditionFalse, "DriftDetected",
			"differs: flows(a); observe policy prevents application writes"},
		{"observe error precedes drift", true, []string{"flows(a)"}, []string{"x"}, metav1.ConditionFalse, engine.ReasonSyncFailed, "observation failed: [x]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			obj := newConfig("", 0)
			c := testClient(obj).WithStatusSubresource(obj).Build()
			ReportResult(context.Background(), c, nil, obj, tc.observe, engine.ReconcileResult{Errors: tc.errors, Drift: tc.drift})
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

func TestConfigChangedPredicate(t *testing.T) {
	old := newConfig("", 0)
	old.Generation = 1
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
	p := ConfigChangedPredicate()
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

func newFakeClient(objs ...client.Object) client.Client {
	s := runtime.NewScheme()
	_ = corev1.AddToScheme(s)
	return fake.NewClientBuilder().WithScheme(s).WithObjects(objs...).Build()
}

func TestResolveDownloadClientSecrets(t *testing.T) {
	secret := &corev1.Secret{
		Name: "qbit-creds", Namespace: "default",
		Data: map[string][]byte{
			"username": []byte("admin"),
			"password": []byte("pass123"),
		},
	}
	fc := newFakeClient(secret)

	dcs := []commonv1alpha1.DownloadClient{
		{
			Name:              "qbit",
			UsernameSecretRef: commonv1alpha1.SecretKeyRef{Name: "qbit-creds", Key: "username"},
			PasswordSecretRef: commonv1alpha1.SecretKeyRef{Name: "qbit-creds", Key: "password"},
		},
		{
			Name: "no-secrets",
		},
	}

	resolved, err := ResolveDownloadClientSecrets(context.Background(), fc, "default", dcs)
	require.NoError(t, err)
	assert.Equal(t, "admin", resolved["qbit"].Username)
	assert.Equal(t, "pass123", resolved["qbit"].Password)
	assert.Empty(t, resolved["no-secrets"].Username)
}

func TestResolveDownloadClientSecrets_MissingSecret(t *testing.T) {
	fc := newFakeClient()

	dcs := []commonv1alpha1.DownloadClient{
		{
			Name:              "qbit",
			UsernameSecretRef: commonv1alpha1.SecretKeyRef{Name: "nonexistent", Key: "username"},
		},
	}

	_, err := ResolveDownloadClientSecrets(context.Background(), fc, "default", dcs)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "username")
}

func TestReferencesSecretFindsEveryReference(t *testing.T) {
	ref := commonv1alpha1.SecretKeyRef{Name: "target", Key: "k"}
	for name, config := range map[string]any{
		"an optional TLS CA": &pvrv1alpha1.Sonarr{Spec: pvrv1alpha1.SonarrSpec{
			Connection: commonv1alpha1.AppConnection{TLS: &commonv1alpha1.TLSConfig{CASecretRef: ref}},
		}},
		"a download client password": pvrv1alpha1.SonarrDownloadClientSpec{
			DownloadClient: commonv1alpha1.DownloadClient{Name: "q", PasswordSecretRef: ref},
		},
		"a field read from a Secret": indexersv1alpha1.ProwlarrSpec{
			Indexers: []indexersv1alpha1.ProwlarrIndexer{{Name: "x", Fields: []commonv1alpha1.ConfigField{{Name: "apiKey", ValueFrom: commonv1alpha1.ValueSource{SecretKeyRef: ref}}}}},
		},
		"a Usenet server password": downloadsv1alpha1.SabnzbdSpec{
			Servers: []downloadsv1alpha1.SabnzbdServer{{Name: "s", PasswordSecretRef: &ref}},
		},
		"a map value": map[string]commonv1alpha1.SecretKeyRef{"x": ref},
		"a Bazarr provider setting": subtitlesv1alpha1.BazarrSpec{
			Providers: []subtitlesv1alpha1.BazarrProvider{{Name: "opensubtitlescom", Settings: []commonv1alpha1.ConfigField{
				{Name: "password", ValueFrom: commonv1alpha1.ValueSource{SecretKeyRef: ref}},
			}}},
		},
	} {
		assert.True(t, ReferencesSecret(config, "target"), name)
		assert.False(t, ReferencesSecret(config, "other"), name)
	}
}

func TestEnqueueForSecretQueuesOnlyConfigsThatReferToIt(t *testing.T) {
	s := runtime.NewScheme()
	require.NoError(t, scheme.AddToScheme(s))
	require.NoError(t, pvrv1alpha1.AddToScheme(s))
	withCA := func(name, namespace string) *pvrv1alpha1.Sonarr {
		return &pvrv1alpha1.Sonarr{Name: name, Namespace: namespace, Spec: pvrv1alpha1.SonarrSpec{
			Connection: commonv1alpha1.AppConnection{TLS: &commonv1alpha1.TLSConfig{CASecretRef: commonv1alpha1.SecretKeyRef{Name: "ca", Key: "ca.crt"}}},
		}}
	}
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(
		withCA("uses", "media"), withCA("elsewhere", "other"),
		&pvrv1alpha1.Sonarr{Name: "unrelated", Namespace: "media"},
	).Build()
	configs := configsForSecret(c, &pvrv1alpha1.SonarrList{})

	reqs := configs(context.Background(), &metav1.PartialObjectMetadata{Name: "ca", Namespace: "media"})
	assert.Equal(t, []reconcile.Request{{Name: "uses", Namespace: "media"}}, reqs, "Secrets are watched by metadata only")
}

func newSecretClient() *fake.ClientBuilder {
	return fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(&corev1.Secret{
		Name: "tracker", Namespace: "arr",
		Data: map[string][]byte{"passkey": []byte("s3cr3t")},
	})
}

func TestResolveConfigFieldsFromSecret(t *testing.T) {
	c := newSecretClient().Build()
	fields := []commonv1alpha1.ConfigField{
		{Name: "apiKey", ValueFrom: commonv1alpha1.ValueSource{SecretKeyRef: commonv1alpha1.SecretKeyRef{Name: "tracker", Key: "passkey"}}},
	}
	out, err := ResolveConfigFields(context.Background(), c, "arr", "indexer x", fields)
	require.NoError(t, err)
	assert.Zero(t, out[0].ValueFrom, "valueFrom should be cleared after resolution")
	b, _ := json.Marshal(out[0].Value)
	assert.Equal(t, `"s3cr3t"`, string(b), "expected quoted secret value")
	assert.Nil(t, fields[0].Value, "input slice was mutated")
}

func TestResolveConfigFieldsRejectsBothValueAndValueFrom(t *testing.T) {
	c := newSecretClient().Build()
	raw, _ := json.Marshal("literal")
	fields := []commonv1alpha1.ConfigField{{
		Name:      "apiKey",
		Value:     &commonv1alpha1.FieldValue{Raw: raw},
		ValueFrom: commonv1alpha1.ValueSource{SecretKeyRef: commonv1alpha1.SecretKeyRef{Name: "tracker", Key: "passkey"}},
	}}
	_, err := ResolveConfigFields(context.Background(), c, "arr", "indexer x", fields)
	require.Error(t, err, "expected an error when both value and valueFrom are set")
}

func TestResolveConfigFieldsMissingSecret(t *testing.T) {
	c := newSecretClient().Build()
	fields := []commonv1alpha1.ConfigField{
		{Name: "apiKey", ValueFrom: commonv1alpha1.ValueSource{SecretKeyRef: commonv1alpha1.SecretKeyRef{Name: "nope", Key: "passkey"}}},
	}
	_, err := ResolveConfigFields(context.Background(), c, "arr", "indexer x", fields)
	require.Error(t, err, "expected an error for a missing secret")
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

func TestDriftSummaryNamesTheFirstTen(t *testing.T) {
	drift := make([]string, 0, 13)
	for i := range 13 {
		drift = append(drift, fmt.Sprintf("indexers(%d)", i))
	}
	assert.Equal(t, "indexers(0), indexers(1), indexers(2), indexers(3), indexers(4), indexers(5), "+
		"indexers(6), indexers(7), indexers(8), indexers(9) and 3 more", driftSummary(drift))
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
		{true, engine.ReasonSynced, metav1.ConditionTrue, false},
		{false, engine.ReasonSyncFailed, metav1.ConditionFalse, false},
		{false, engine.ReasonAppUnreachable, metav1.ConditionFalse, false},
		{false, engine.ReasonInvalidConfig, metav1.ConditionFalse, true},
		{false, engine.ReasonUnauthorized, metav1.ConditionFalse, true},
		{true, engine.ReasonObserved, metav1.ConditionTrue, false},
	} {
		UpdateStatus(context.Background(), c, nil, obj, step.synced, step.reason, "message")
		ready := meta.FindStatusCondition(obj.Status.Conditions, engine.ConditionReady)
		require.NotNil(t, ready, step.reason)
		assert.Equal(t, step.ready, ready.Status, step.reason)
		assert.Equal(t, step.reason, ready.Reason, "Ready carries Synced's reason")
		assert.Equal(t, step.stalled, meta.IsStatusConditionTrue(obj.Status.Conditions, engine.ConditionStalled), step.reason)
	}
}

func TestReadyChangesAreRecordedAsEvents(t *testing.T) {
	obj, c := statusSonarr(t)
	recorder := events.NewFakeRecorder(10)
	update := func(synced bool, reason string) {
		UpdateStatus(context.Background(), c, recorder, obj, synced, reason, "message")
	}

	update(true, engine.ReasonSynced)
	update(false, engine.ReasonAppUnreachable)
	update(false, engine.ReasonAppUnreachable)
	update(false, engine.ReasonSyncFailed)
	update(true, engine.ReasonSynced)
	update(true, engine.ReasonSynced)

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

	UpdateStatus(context.Background(), c, recorder, obj, false, engine.ReasonSyncFailed, "indexers(Private): the tracker said key abc123 is banned")

	assert.Equal(t, "Warning SyncFailed The app refused some settings. The Ready condition says which.", <-recorder.Events,
		"an event can reach readers who can't read the resource")
	ready := meta.FindStatusCondition(obj.Status.Conditions, engine.ConditionReady)
	require.NotNil(t, ready)
	assert.Contains(t, ready.Message, "the tracker said", "the details stay in the condition")
}

func TestManagedResourcesGaugeSurvivesARestartWhileTheAppIsDown(t *testing.T) {
	obj, c := statusSonarr(t)
	obj.Name = "restarted"
	obj.SetManagedResources(map[string][]string{"tags": {"a", "b"}})

	UpdateStatus(context.Background(), c, nil, obj, false, engine.ReasonAppUnreachable, "connection refused")

	assert.Equal(t, 2.0, testutil.ToFloat64(metrics.ManagedResources.WithLabelValues("Sonarr", "media", "restarted", "tags")),
		"the series comes from the ownership status records, not only from a successful sync")
}
