package prowlarr

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	indexersv1alpha1 "github.com/kyleseneker/arrchitect/api/indexers/v1alpha1"
	"github.com/kyleseneker/arrchitect/internal/engine"
)

func referenceServer(t *testing.T, lists map[string][]map[string]any) *engine.HTTPClient {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		items, ok := lists[r.URL.Path]
		if !ok {
			items = []map[string]any{}
		}
		_ = json.NewEncoder(w).Encode(items)
	}))
	t.Cleanup(srv.Close)

	hc, err := engine.NewHTTPClient(srv.URL, engine.AuthAPIKey,
		engine.WithAPIKey("k"), engine.WithAPIKeyHeader("X-Api-Key"),
		engine.WithTransport(&http.Transport{}))
	require.NoError(t, err)
	return hc
}

func TestResolveReferencesFollowsRecreatedAppProfile(t *testing.T) {
	lists := map[string][]map[string]any{
		"/api/v1/appprofile": {{"id": 3, "name": "Standard"}},
	}
	hc := referenceServer(t, lists)
	spec := indexersv1alpha1.ProwlarrConfigSpec{
		Indexers: []indexersv1alpha1.ProwlarrIndexer{{Name: "idx", AppProfileName: "Standard"}},
	}

	first, _, err := ResolveReferences(context.Background(), hc, spec, false)
	require.NoError(t, err)
	require.NotNil(t, first.Indexers[0].AppProfileId)
	assert.Equal(t, 3, *first.Indexers[0].AppProfileId)

	lists["/api/v1/appprofile"] = []map[string]any{{"id": 7, "name": "Standard"}}

	second, _, err := ResolveReferences(context.Background(), hc, spec, false)
	require.NoError(t, err)
	require.NotNil(t, second.Indexers[0].AppProfileId)
	assert.Equal(t, 7, *second.Indexers[0].AppProfileId)
	assert.Nil(t, spec.Indexers[0].AppProfileId, "the input spec must not carry application IDs")
}

func TestResolveReferencesRejectsMissingAppProfile(t *testing.T) {
	hc := referenceServer(t, map[string][]map[string]any{
		"/api/v1/appprofile": {{"id": 3, "name": "Standard"}},
	})
	spec := indexersv1alpha1.ProwlarrConfigSpec{
		Indexers: []indexersv1alpha1.ProwlarrIndexer{{Name: "idx", AppProfileName: "Gone"}},
	}

	_, _, err := ResolveReferences(context.Background(), hc, spec, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "indexers(idx): appProfileName")
}

func TestResolveReferencesRejectsMissingTag(t *testing.T) {
	hc := referenceServer(t, map[string][]map[string]any{
		"/api/v1/tag": {{"id": 1, "label": "known"}},
	})
	spec := indexersv1alpha1.ProwlarrConfigSpec{
		Indexers: []indexersv1alpha1.ProwlarrIndexer{{Name: "idx", TagLabels: []string{"known", "unknown"}}},
	}

	_, _, err := ResolveReferences(context.Background(), hc, spec, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "indexers(idx): tag")
}

func TestResolveReferencesInObserveModePlansWhatTheSpecDeclares(t *testing.T) {
	hc := referenceServer(t, map[string][]map[string]any{})
	spec := indexersv1alpha1.ProwlarrConfigSpec{
		AppProfiles: []indexersv1alpha1.ProwlarrAppProfile{{Name: "Standard"}},
		Indexers:    []indexersv1alpha1.ProwlarrIndexer{{Name: "x", TagLabels: []string{"tv"}, AppProfileName: "Standard"}},
	}
	spec.Tags = []commonv1alpha1.Tag{{Label: "TV"}}

	resolved, tagIDs, err := ResolveReferences(context.Background(), hc, spec, true)
	require.NoError(t, err)
	assert.Equal(t, engine.PlannedID, *resolved.Indexers[0].AppProfileId)
	assert.Equal(t, engine.PlannedID, tagIDs["tv"])

	spec.Indexers[0].TagLabels = []string{"typo"}
	_, _, err = ResolveReferences(context.Background(), hc, spec, true)
	assert.ErrorContains(t, err, `reference "typo" does not exist`, "a name the spec doesn't declare still fails")

	spec.Indexers[0].TagLabels = []string{"tv"}
	_, _, err = ResolveReferences(context.Background(), hc, spec, false)
	assert.ErrorContains(t, err, `reference "tv" does not exist`, "enforcing creates tags before resolving them")
}
