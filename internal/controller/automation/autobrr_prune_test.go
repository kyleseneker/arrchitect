package automation

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	automationv1alpha1 "github.com/kyleseneker/arrchitect/api/automation/v1alpha1"
	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	ctrlcommon "github.com/kyleseneker/arrchitect/internal/controller/common"
	"github.com/kyleseneker/arrchitect/internal/engine"
	"github.com/kyleseneker/arrchitect/internal/metrics"
)

func autobrrManaged(t *testing.T, cfg *automationv1alpha1.Autobrr, withSecret bool) map[string][]string {
	t.Helper()
	c := runAutobrr(t, cfg, withSecret)
	var got automationv1alpha1.Autobrr
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "autobrr", Namespace: "media"}, &got))
	return got.Status.GetManagedResources()
}

func TestAutobrrPrunesOnlyFeedsItCreated(t *testing.T) {
	app := &fakeAutobrr{existing: map[string][]map[string]any{"indexer": genericIndexer, "feeds": {
		{"id": float64(5), "name": "old"}, {"id": float64(6), "name": "hand-made"}, {"id": float64(7), "name": "kept"},
	}}}
	cfg := autobrrConfig(app.serve(t))
	cfg.Spec.Reconcile = &commonv1alpha1.ReconcileConfig{Prune: new(true)}
	cfg.Spec.Feeds = []automationv1alpha1.AutobrrFeed{{Name: "kept", Type: "RSS", URL: "http://feed", IndexerName: "Generic"}}
	cfg.Status.SetManagedResources(map[string][]string{"feeds": {"old", "kept"}})

	managed := autobrrManaged(t, cfg, true)

	assert.True(t, app.saw(http.MethodDelete, "/api/feeds/5"), "an owned feed left the spec and was not deleted")
	assert.False(t, app.saw(http.MethodDelete, "/api/feeds/6"), "deleted a feed the operator did not create")
	assert.False(t, app.saw(http.MethodDelete, "/api/feeds/7"), "deleted a declared feed")
	assert.Equal(t, []string{"kept"}, managed["feeds"])
}

func TestAutobrrRecordsWhatItCreates(t *testing.T) {
	app := &fakeAutobrr{existing: map[string][]map[string]any{"indexer": genericIndexer}}
	cfg := autobrrConfig(app.serve(t))
	cfg.Spec.Feeds = []automationv1alpha1.AutobrrFeed{{Name: "feed", Type: "RSS", URL: "http://feed", IndexerName: "Generic"}}
	cfg.Spec.Filters = []automationv1alpha1.AutobrrFilter{{
		Name:    "TV",
		Actions: []automationv1alpha1.AutobrrFilterAction{{Name: "notify", Type: "Webhook", WebhookURL: "http://hook"}},
	}}

	managed := autobrrManaged(t, cfg, true)

	assert.Equal(t, []string{"feed"}, managed["feeds"])
	assert.Equal(t, []string{"TV"}, managed["filters"])
	assert.Equal(t, []string{"TV/notify"}, managed["actions"])
}

func TestAutobrrDeletionPolicyRemovesWhatItCreated(t *testing.T) {
	app := &fakeAutobrr{existing: map[string][]map[string]any{
		"filters":     {{"id": float64(3), "name": "TV"}},
		"actions":     {{"id": float64(8), "name": "send"}},
		"downloaders": {{"id": float64(4), "name": "qBit"}, {"id": float64(9), "name": "hand-made"}},
	}}
	cfg := autobrrConfig(app.serve(t))
	policy := ctrlcommon.DeletionPolicyDelete
	cfg.Spec.Reconcile = &commonv1alpha1.ReconcileConfig{DeletionPolicy: policy}
	cfg.Status.SetManagedResources(map[string][]string{"filters": {"TV"}, "actions": {"TV/send"}, "downloadClients": {"qBit"}})
	now := metav1.NewTime(time.Now())
	cfg.DeletionTimestamp = &now
	cfg.Finalizers = []string{ctrlcommon.Finalizer}

	runAutobrr(t, cfg, true)

	for _, path := range []string{"/api/actions/8", "/api/filters/3", "/api/downloaders/4"} {
		assert.True(t, app.saw(http.MethodDelete, path), "cleanup did not delete %s", path)
	}
	assert.False(t, app.saw(http.MethodDelete, "/api/downloaders/9"), "cleanup deleted a download client the operator did not create")
}

func TestAutobrrObserveReportsWhatPruneWouldDelete(t *testing.T) {
	app := &fakeAutobrr{existing: map[string][]map[string]any{"indexer": genericIndexer, "feeds": {
		{"id": float64(5), "name": "observe-old"}, {"id": float64(7), "name": "kept"},
	}}}
	cfg := autobrrConfig(app.serve(t))
	observe := "observe"
	cfg.Spec.Reconcile = &commonv1alpha1.ReconcileConfig{Prune: new(true), DriftPolicy: observe}
	cfg.Spec.Feeds = []automationv1alpha1.AutobrrFeed{{Name: "kept", Type: "RSS", URL: "http://feed", IndexerName: "Generic"}}
	cfg.Status.SetManagedResources(map[string][]string{"feeds": {"observe-old", "kept"}})
	observed := metrics.DriftObservedTotal.WithLabelValues("autobrr", "feeds", "observe-old")
	before := testutil.ToFloat64(observed)

	managed := autobrrManaged(t, cfg, true)

	assert.False(t, app.saw(http.MethodDelete, "/api/feeds/5"), "observe mode deleted a feed")
	assert.Equal(t, before+1, testutil.ToFloat64(observed))
	assert.Equal(t, []string{"observe-old", "kept"}, managed["feeds"])
}

var genericIndexer = []map[string]any{{"id": float64(9), "name": "Generic", "implementation": "rss"}}

func TestAutobrrDeletesListsBeforeWhatTheyNameAndProxiesLast(t *testing.T) {
	app := &fakeAutobrr{existing: map[string][]map[string]any{
		"lists":       {{"id": float64(2), "name": "Shows"}},
		"filters":     {{"id": float64(3), "name": "TV"}},
		"downloaders": {{"id": float64(4), "name": "Sonarr"}},
		"proxy":       {{"id": float64(6), "name": "VPN"}},
	}}
	cfg := autobrrConfig(app.serve(t))
	cfg.Spec.Reconcile = &commonv1alpha1.ReconcileConfig{DeletionPolicy: ctrlcommon.DeletionPolicyDelete}
	cfg.Status.SetManagedResources(map[string][]string{
		"lists": {"Shows"}, "filters": {"TV"}, "downloadClients": {"Sonarr"}, "proxies": {"VPN"},
	})
	now := metav1.NewTime(time.Now())
	cfg.DeletionTimestamp = &now
	cfg.Finalizers = []string{ctrlcommon.Finalizer}

	runAutobrr(t, cfg, true)

	assert.Equal(t, []string{
		"DELETE /api/lists/2", "DELETE /api/filters/3", "DELETE /api/downloaders/4", "DELETE /api/proxy/6",
	}, autobrrWrites(app))
}

var unscopedKinds = []string{"notification", "lists", "proxy"}

func TestAutobrrPruneListsOnlyKindsItOwns(t *testing.T) {
	app := &fakeAutobrr{forbidden: unscopedKinds, existing: map[string][]map[string]any{"indexer": genericIndexer, "feeds": {
		{"id": float64(5), "name": "old"}, {"id": float64(7), "name": "kept"},
	}}}
	cfg := autobrrConfig(app.serve(t))
	cfg.Spec.Reconcile = &commonv1alpha1.ReconcileConfig{Prune: new(true)}
	cfg.Spec.Feeds = []automationv1alpha1.AutobrrFeed{{Name: "kept", Type: "RSS", URL: "http://feed", IndexerName: "Generic"}}
	cfg.Status.SetManagedResources(map[string][]string{"feeds": {"old", "kept"}})

	c := runAutobrr(t, cfg, true)

	assert.True(t, app.saw(http.MethodDelete, "/api/feeds/5"))
	for _, kind := range unscopedKinds {
		assert.False(t, app.saw(http.MethodGet, "/api/"+kind), "listed %s, which the operator owns nothing of", kind)
	}
	assert.Equal(t, conditionTrue, autobrrSynced(t, c))
}

func TestAutobrrCleanupListsOnlyKindsItOwns(t *testing.T) {
	app := &fakeAutobrr{forbidden: append([]string{"filters"}, unscopedKinds...), existing: map[string][]map[string]any{
		"downloaders": {{"id": float64(4), "name": "qBit"}},
	}}
	cfg := autobrrConfig(app.serve(t))
	cfg.Spec.Reconcile = &commonv1alpha1.ReconcileConfig{DeletionPolicy: ctrlcommon.DeletionPolicyDelete}
	cfg.Status.SetManagedResources(map[string][]string{"downloadClients": {"qBit"}})
	now := metav1.NewTime(time.Now())
	cfg.DeletionTimestamp = &now
	cfg.Finalizers = []string{ctrlcommon.Finalizer}

	runAutobrr(t, cfg, true)

	assert.Equal(t, []string{"DELETE /api/downloaders/4"}, autobrrWrites(app))
	for _, kind := range append([]string{"filters"}, unscopedKinds...) {
		assert.False(t, app.saw(http.MethodGet, "/api/"+kind), "listed %s, which the operator owns nothing of", kind)
	}
}

func TestActionsUnderAnAmbiguousFilterAreKept(t *testing.T) {
	s := &autobrrSync{Pass: engine.NewPass("autobrr", engine.SyncPolicy{}, map[string][]string{
		actionsKind: {"TV/send", "TV/notify", "Movies/send"},
	})}

	err := s.keptUnder("TV", errors.New(`Autobrr has 2 filters named "TV"`))

	kept, failure := engine.SplitKept(err)
	require.NoError(t, failure, "no retry tells two filters apart, so deletion must not wait on it")
	require.Len(t, kept, 1)
	assert.ElementsMatch(t, []string{"TV/send", "TV/notify"}, []string{kept[0].Kept[0].Name, kept[0].Kept[1].Name})
	assert.NoError(t, s.keptUnder("Shows", errors.New("ambiguous")), "a filter with none of the operator's actions keeps nothing")
}
