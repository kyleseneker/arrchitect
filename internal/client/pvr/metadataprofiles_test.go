package pvr

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pvrv1alpha1 "github.com/kyleseneker/arrchitect/api/pvr/v1alpha1"
	"github.com/kyleseneker/arrchitect/internal/engine"
)

func metadataProfileClient(t *testing.T, schema, profiles string) *engine.HTTPClient {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/metadataprofile/schema":
			_, _ = w.Write([]byte(schema))
		case "/api/v1/metadataprofile":
			_, _ = w.Write([]byte(profiles))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	hc, err := engine.NewHTTPClient(srv.URL, engine.AuthNone, engine.WithTransport(&http.Transport{}))
	require.NoError(t, err)
	return hc
}

const lidarrSchema = `{"id":0,"name":"","primaryAlbumTypes":[
	{"albumType":{"id":0,"name":"Album"},"allowed":true},
	{"albumType":{"id":1,"name":"EP"},"allowed":false},
	{"albumType":{"id":2,"name":"Single"},"allowed":false}],
	"secondaryAlbumTypes":[{"albumType":{"id":0,"name":"Studio"},"allowed":true},{"albumType":{"id":3,"name":"Live"},"allowed":false}],
	"releaseStatuses":[{"releaseStatus":{"id":0,"name":"Official"},"allowed":true},{"releaseStatus":{"id":2,"name":"Bootleg"},"allowed":false}]}`

func allowedNames(t *testing.T, list any, key string) []string {
	t.Helper()
	items, ok := list.([]any)
	require.True(t, ok, "%v is not a list", list)
	var names []string
	for _, entry := range items {
		item := entry.(map[string]any)
		if item["allowed"] == true {
			names = append(names, item[key].(map[string]any)["name"].(string))
		}
	}
	return names
}

func TestLidarrMetadataProfileAllowsOnlyTheNamedChoices(t *testing.T) {
	hc := metadataProfileClient(t, lidarrSchema, `[]`)
	got, err := lidarrMetadataProfileResources(context.Background(), hc, "v1", []pvrv1alpha1.LidarrMetadataProfile{{
		Name: "Albums and EPs", PrimaryAlbumTypes: []string{"Album", "EP"}, ReleaseStatuses: []string{"Official"},
	}})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, []string{"Album", "EP"}, allowedNames(t, got[0]["primaryAlbumTypes"], "albumType"))
	assert.Equal(t, []string{"Official"}, allowedNames(t, got[0]["releaseStatuses"], "releaseStatus"))
	assert.Equal(t, []string{"Studio"}, allowedNames(t, got[0]["secondaryAlbumTypes"], "albumType"),
		"a new profile takes Lidarr's defaults for a list left out")
}

func TestLidarrMetadataProfileKeepsWhatAnExistingProfileLeavesOut(t *testing.T) {
	existing := `[{"id":4,"name":"Standard","primaryAlbumTypes":[
		{"id":11,"albumType":{"id":0,"name":"Album"},"allowed":true},
		{"id":12,"albumType":{"id":1,"name":"EP"},"allowed":true}]}]`
	hc := metadataProfileClient(t, lidarrSchema, existing)
	got, err := lidarrMetadataProfileResources(context.Background(), hc, "v1", []pvrv1alpha1.LidarrMetadataProfile{{
		Name: "Standard", PrimaryAlbumTypes: []string{"Album"},
	}})
	require.NoError(t, err)
	require.Len(t, got, 1)
	items := got[0]["primaryAlbumTypes"].([]any)
	require.Len(t, items, 2, "an existing profile's own list is the base, not the template")
	assert.Equal(t, float64(12), items[1].(map[string]any)["id"], "the profile's item IDs are kept")
	assert.Equal(t, []string{"Album"}, allowedNames(t, items, "albumType"))
	assert.NotContains(t, got[0], "secondaryAlbumTypes", "a list left out is left alone")
}

func TestLidarrMetadataProfileRefusesAnUnknownChoice(t *testing.T) {
	hc := metadataProfileClient(t, lidarrSchema, `[]`)
	_, err := lidarrMetadataProfileResources(context.Background(), hc, "v1", []pvrv1alpha1.LidarrMetadataProfile{{
		Name: "Odd", PrimaryAlbumTypes: []string{"Cassette"},
	}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `"Cassette" isn't one of the app's choices`)
}

func TestReadarrMetadataProfileStartsFromTheTemplateWhenNew(t *testing.T) {
	schema := `{"id":0,"name":"","minPopularity":350,"skipMissingDate":true,"skipMissingIsbn":false,"skipPartsAndSets":true,
		"skipSeriesSecondary":false,"allowedLanguages":"eng, null","minPages":0,"ignored":[]}`
	existing := `[{"id":2,"name":"Standard","minPopularity":350,"allowedLanguages":"eng"}]`
	hc := metadataProfileClient(t, schema, existing)
	got, err := readarrMetadataProfileResources(context.Background(), hc, "v1", []pvrv1alpha1.ReadarrMetadataProfile{
		{Name: "English", AllowedLanguages: []string{"eng", "null"}, MinPages: new(int32(50))},
		{Name: "Standard", MinPopularity: new(int64(0))},
	})
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "eng,null", got[0]["allowedLanguages"])
	assert.Equal(t, int32(50), got[0]["minPages"])
	assert.Equal(t, true, got[0]["skipMissingDate"], "a new profile takes Readarr's defaults")
	assert.NotContains(t, got[0], "id")
	assert.Equal(t, map[string]any{"name": "Standard", "minPopularity": int64(0)}, got[1],
		"an existing profile gets only what the resource declares")
}

func TestReadarrMetadataProfileKeepsStoredLanguagesWithTheSameCodes(t *testing.T) {
	existing := `[{"id":1,"name":"Standard","allowedLanguages":"eng, null"}]`
	hc := metadataProfileClient(t, `{"id":0}`, existing)
	got, err := readarrMetadataProfileResources(context.Background(), hc, "v1", []pvrv1alpha1.ReadarrMetadataProfile{
		{Name: "Standard", AllowedLanguages: []string{"null", "eng"}},
	})
	require.NoError(t, err)
	assert.Equal(t, "eng, null", got[0]["allowedLanguages"], "the same codes in Readarr's own form must not read as a change")
}
