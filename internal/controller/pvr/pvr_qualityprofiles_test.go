package pvr

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	"github.com/kyleseneker/arrchitect/internal/engine"
	"github.com/kyleseneker/arrchitect/internal/httpapi"
)

func TestBuildQualityProfilePayload(t *testing.T) {
	tests := []struct {
		name  string
		qp    commonv1alpha1.QualityProfile
		cfIDs map[string]int
		check func(t *testing.T, p map[string]any)
	}{
		{
			name:  "name only",
			qp:    commonv1alpha1.QualityProfile{Name: "HD-1080p"},
			cfIDs: map[string]int{},
			check: func(t *testing.T, p map[string]any) {
				assert.Equal(t, "HD-1080p", p["name"])
				_, hasUpgrade := p["upgradeAllowed"]
				assert.False(t, hasUpgrade)
			},
		},
		{
			name: "with all optional fields",
			qp: commonv1alpha1.QualityProfile{
				Name: "HD", UpgradeAllowed: new(true),
				MinFormatScore: new(int32(10)), CutoffFormatScore: new(int32(100)),
				MinUpgradeFormatScore: new(int32(5)),
			},
			cfIDs: map[string]int{},
			check: func(t *testing.T, p map[string]any) {
				assert.Equal(t, true, p["upgradeAllowed"])
				assert.Equal(t, int32(10), p["minFormatScore"])
				assert.Equal(t, int32(100), p["cutoffFormatScore"])
				assert.Equal(t, int32(5), p["minUpgradeFormatScore"])
			},
		},
		{
			name: "with format items",
			qp: commonv1alpha1.QualityProfile{
				Name: "HD",
				FormatItems: []commonv1alpha1.QualityProfileFormatItem{
					{Name: "x265", Score: new(int32(100))},
					{Name: "BR-DISK", Score: new(int32(-1000))},
					{Name: "Unknown", Score: new(int32(50))},
				},
			},
			cfIDs: map[string]int{"x265": 1, "BR-DISK": 2},
			check: func(t *testing.T, p map[string]any) {
				items := p["formatItems"].([]map[string]any)
				assert.Len(t, items, 2)
				assert.Equal(t, 1, items[0]["format"])
				assert.Equal(t, int32(100), items[0]["score"])
			},
		},
		{
			name: "with cutoff resolution",
			qp: commonv1alpha1.QualityProfile{
				Name:   "HD",
				Cutoff: "Bluray-1080p",
				Items: []commonv1alpha1.QualityProfileItem{
					{Quality: commonv1alpha1.QualityReference{ID: new(int32(7)), Name: "Bluray-1080p"}},
				},
			},
			cfIDs: map[string]int{},
			check: func(t *testing.T, p map[string]any) {
				assert.Equal(t, 7, p["cutoff"])
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := buildQualityProfilePayload(tt.qp, tt.cfIDs)
			tt.check(t, p)
		})
	}
}

func TestFindCutoffID(t *testing.T) {
	items := []commonv1alpha1.QualityProfileItem{
		{Quality: commonv1alpha1.QualityReference{ID: new(int32(7)), Name: "Bluray-1080p"}},
		{Name: "HD Group", Items: []commonv1alpha1.QualityProfileGroupItem{
			{Quality: commonv1alpha1.QualityReference{ID: new(int32(4)), Name: "HDTV-1080p"}},
		}},
		{Quality: commonv1alpha1.QualityReference{ID: new(int32(3)), Name: "WEBDL-1080p"}},
	}

	tests := []struct {
		name   string
		cutoff string
		wantID int
		wantOK bool
	}{
		{"individual quality", "Bluray-1080p", 7, true},
		{"group", "HD Group", 1001, true},
		{"no match", "NonExistent", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, ok := findCutoffID(items, tt.cutoff)
			assert.Equal(t, tt.wantOK, ok)
			if ok {
				assert.Equal(t, tt.wantID, id)
			}
		})
	}
}

func TestCompleteQualityItemsAddsUndeclaredQualitiesAsDisallowed(t *testing.T) {
	definition := func(id int, name string) map[string]any {
		return map[string]any{"quality": map[string]any{"id": float64(id), "name": name, "source": "web", "resolution": float64(1080)}}
	}
	definitions := []map[string]any{
		definition(0, "Unknown"), definition(1, "SDTV"), definition(3, "WEBDL-1080p"),
		definition(15, "WEBRip-1080p"), definition(7, "Bluray-1080p"),
	}
	declared := []commonv1alpha1.QualityProfileItem{
		{Name: "WEB 1080p", Items: []commonv1alpha1.QualityProfileGroupItem{
			{Quality: commonv1alpha1.QualityReference{ID: new(int32(3)), Name: "WEBDL-1080p"}},
			{Quality: commonv1alpha1.QualityReference{ID: new(int32(15)), Name: "WEBRip-1080p"}},
		}},
		{Quality: commonv1alpha1.QualityReference{ID: new(int32(7)), Name: "Bluray-1080p"}},
	}

	items := completeQualityItems(buildQualityItems(declared), declared, definitions)

	require.Len(t, items, 4, "Unknown and SDTV are added; grouped and listed qualities are not repeated")
	for i, name := range []string{"Unknown", "SDTV"} {
		quality := items[i]["quality"].(map[string]any)
		assert.Equal(t, name, quality["name"])
		assert.Equal(t, false, items[i]["allowed"], "an undeclared quality must not become allowed")
	}
	assert.Equal(t, "WEB 1080p", items[2]["name"], "declared items keep their order after the added ones")
	bluray := items[3]["quality"].(map[string]any)
	assert.Equal(t, float64(7), bluray["id"], "Bluray-1080p stays last")
	assert.Equal(t, "web", bluray["source"], "qualities carry the app's own keys, or every write strips them")
	child := items[2]["items"].([]map[string]any)[0]["quality"].(map[string]any)
	assert.Equal(t, float64(1080), child["resolution"], "grouped qualities are resolved too")
}

func TestBuildQualityItems(t *testing.T) {
	tests := []struct {
		name  string
		items []commonv1alpha1.QualityProfileItem
		check func(t *testing.T, result []map[string]any)
	}{
		{
			name: "individual quality",
			items: []commonv1alpha1.QualityProfileItem{
				{Quality: commonv1alpha1.QualityReference{ID: new(int32(7)), Name: "Bluray-1080p"}},
			},
			check: func(t *testing.T, result []map[string]any) {
				require.Len(t, result, 1)
				q := result[0]["quality"].(map[string]any)
				assert.Equal(t, 7, q["id"])
				assert.Equal(t, "Bluray-1080p", q["name"])
				assert.Equal(t, true, result[0]["allowed"])
				assert.Equal(t, []any{}, result[0]["items"])
			},
		},
		{
			name: "group with children",
			items: []commonv1alpha1.QualityProfileItem{
				{
					Name:    "HD Group",
					Allowed: new(true),
					Items: []commonv1alpha1.QualityProfileGroupItem{
						{Quality: commonv1alpha1.QualityReference{ID: new(int32(4)), Name: "HDTV"}, Allowed: new(false)},
					},
				},
			},
			check: func(t *testing.T, result []map[string]any) {
				require.Len(t, result, 1)
				assert.NotContains(t, result[0], "quality", "the apps omit quality on a group; null reads back as a change")
				assert.Equal(t, 1000, result[0]["id"], "the PVR apps treat an item without a group id as a single quality")
				assert.Equal(t, "HD Group", result[0]["name"])
				children := result[0]["items"].([]map[string]any)
				require.Len(t, children, 1)
				assert.Equal(t, false, children[0]["allowed"])
			},
		},
		{
			name: "allowed defaults to true",
			items: []commonv1alpha1.QualityProfileItem{
				{Quality: commonv1alpha1.QualityReference{ID: new(int32(1)), Name: "SDTV"}},
			},
			check: func(t *testing.T, result []map[string]any) {
				assert.Equal(t, true, result[0]["allowed"])
			},
		},
		{
			name: "allowed explicitly false",
			items: []commonv1alpha1.QualityProfileItem{
				{Quality: commonv1alpha1.QualityReference{ID: new(int32(1)), Name: "SDTV"}, Allowed: new(false)},
			},
			check: func(t *testing.T, result []map[string]any) {
				assert.Equal(t, false, result[0]["allowed"])
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildQualityItems(tt.items)
			tt.check(t, result)
		})
	}
}

func TestCompleteFormatItemsNamesEveryCustomFormatForANewProfile(t *testing.T) {
	formats := engine.NamedIDs{"x265": {1}, "WEBDL": {2}, "HDR": {3}}
	declared := []map[string]any{{"format": 2, "name": "WEBDL", "score": 1000}}

	items := completeFormatItems(declared, formats)

	require.Len(t, items, 3, "the apps reject a new profile that omits any custom format")
	assert.Equal(t, 1000, items[0]["score"], "declared scores are kept")
	assert.Equal(t, "HDR", items[1]["name"])
	assert.Equal(t, 0, items[1]["score"])
	assert.Equal(t, "x265", items[2]["name"])
	assert.Empty(t, completeFormatItems(nil, engine.NamedIDs{}), "an app without custom formats gets an empty list, not null")
	assert.NotNil(t, completeFormatItems(nil, engine.NamedIDs{}))
}

// A fresh app starts with no custom formats. The quality profile must still
// resolve its formatItems, which is only possible if the CF IDs are looked up
// after the custom formats have been written.
func TestQualityProfileResolvesCustomFormatsCreatedInTheSameRun(t *testing.T) {
	var mu sync.Mutex
	customFormats := []map[string]any{}
	var qpBodies []map[string]any
	nextID := 100

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case strings.Contains(r.URL.Path, "customformat"):
			if r.Method == http.MethodPost {
				var body map[string]any
				_ = json.NewDecoder(r.Body).Decode(&body)
				nextID++
				body["id"] = float64(nextID)
				customFormats = append(customFormats, body)
				w.WriteHeader(http.StatusCreated)
				_ = json.NewEncoder(w).Encode(body)
				return
			}
			_ = json.NewEncoder(w).Encode(customFormats)
		case strings.Contains(r.URL.Path, "qualityprofile"):
			if r.Method == http.MethodPost || r.Method == http.MethodPut {
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				qpBodies = append(qpBodies, body)
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(`{"id":1}`))
				return
			}
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		default:
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		}
	}))
	defer srv.Close()

	hc, err := httpapi.NewHTTPClient(srv.URL, httpapi.AuthAPIKey, httpapi.WithAPIKey("k"), httpapi.WithAppLabel("sonarr"), httpapi.WithTransport(&http.Transport{}))
	require.NoError(t, err)

	opts := Options{
		CustomFormats: []commonv1alpha1.CustomFormat{{Name: "BR-DISK"}},
		QualityProfiles: []commonv1alpha1.QualityProfile{{
			Name:        "WEB-1080p",
			FormatItems: []commonv1alpha1.QualityProfileFormatItem{{Name: "BR-DISK", Score: new(int32(100))}},
		}},
	}

	res := reconcileOnce(context.Background(), hc, opts, engine.SyncPolicy{}, nil)
	require.Empty(t, res.Errors, "unexpected errors")
	mu.Lock()
	formats, bodies := slices.Clone(customFormats), slices.Clone(qpBodies)
	mu.Unlock()
	require.Len(t, formats, 1, "expected the custom format to be created")
	require.NotEmpty(t, bodies, "quality profile was never written")
	items, _ := bodies[0]["formatItems"].([]any)
	require.Len(t, items, 1)
	item, _ := items[0].(map[string]any)
	assert.Equal(t, formats[0]["id"], item["format"], "quality profile did not reference the newly created custom format id")
}

func TestKeepGroupIDsReusesTheAppsGroupIDs(t *testing.T) {
	payload := buildQualityProfilePayload(commonv1alpha1.QualityProfile{
		Name:   "Any",
		Cutoff: "WEB 1080p",
		Items: []commonv1alpha1.QualityProfileItem{
			{Quality: commonv1alpha1.QualityReference{ID: new(int32(1)), Name: "SDTV"}},
			{Name: "WEB 720p", Items: []commonv1alpha1.QualityProfileGroupItem{{Quality: commonv1alpha1.QualityReference{ID: new(int32(5)), Name: "WEBDL-720p"}}}},
			{Name: "New group", Items: []commonv1alpha1.QualityProfileGroupItem{{Quality: commonv1alpha1.QualityReference{ID: new(int32(6)), Name: "WEBRip-720p"}}}},
			{Name: "WEB 1080p", Items: []commonv1alpha1.QualityProfileGroupItem{{Quality: commonv1alpha1.QualityReference{ID: new(int32(3)), Name: "WEBDL-1080p"}}}},
		},
	}, nil)
	profile := map[string]any{"items": []any{
		map[string]any{"quality": map[string]any{"id": float64(1)}},
		map[string]any{"id": float64(1001), "name": "WEB 720p"},
		map[string]any{"id": float64(1000), "name": "WEB 1080p"},
	}}

	keepGroupIDs(payload, profile)

	ids := map[string]any{}
	for _, item := range payload["items"].([]map[string]any) {
		if name, ok := item["name"].(string); ok {
			ids[name] = item["id"]
		}
	}
	assert.Equal(t, map[string]any{"WEB 720p": 1001, "WEB 1080p": 1000, "New group": 1002}, ids)
	assert.Equal(t, 1000, payload["cutoff"], "the cutoff follows its group's id")
}

func size(s string) *commonv1alpha1.Decimal {
	d := commonv1alpha1.Decimal(s)
	return &d
}

func definition(name string, fields map[string]any) map[string]any {
	d := map[string]any{"id": float64(len(name)), "quality": map[string]any{"id": float64(1), "name": name}, "title": name}
	maps.Copy(d, fields)
	return d
}

func TestQualityDefinitionPayload(t *testing.T) {
	sonarr := definition("DVD", map[string]any{"minSize": float64(2), "maxSize": float64(100), "preferredSize": float64(95)})
	tests := []struct {
		name    string
		qd      commonv1alpha1.QualityDefinition
		current map[string]any
		readarr bool
		want    map[string]any
		err     string
	}{
		{
			name:    "sizes and title",
			qd:      commonv1alpha1.QualityDefinition{Quality: "DVD", Title: "DVD (any)", MinSize: size("17.1"), MaxSize: size("400"), PreferredSize: size("350.5")},
			current: sonarr,
			want: map[string]any{
				"quality": map[string]any{"name": "DVD"}, "title": "DVD (any)",
				"minSize": json.Number("17.1"), "maxSize": json.Number("400"), "preferredSize": json.Number("350.5"),
			},
		},
		{
			name:    "0 is written as unlimited",
			qd:      commonv1alpha1.QualityDefinition{Quality: "DVD", MaxSize: size("0"), PreferredSize: size("0")},
			current: sonarr,
			want:    map[string]any{"quality": map[string]any{"name": "DVD"}, "maxSize": nil, "preferredSize": nil},
		},
		{
			name:    "a maximum of 0 the app already has is left as it is",
			qd:      commonv1alpha1.QualityDefinition{Quality: "DVD", MaxSize: size("0")},
			current: definition("DVD", map[string]any{"maxSize": float64(0)}),
			want:    map[string]any{"quality": map[string]any{"name": "DVD"}},
		},
		{
			name:    "an unlimited size the app leaves out is left as it is",
			qd:      commonv1alpha1.QualityDefinition{Quality: "FLAC", MaxSize: size("0"), PreferredSize: size("0")},
			current: definition("FLAC", map[string]any{"minSize": float64(0)}),
			want:    map[string]any{"quality": map[string]any{"name": "FLAC"}},
		},
		{
			name:    "a preferred size of 0 is not unlimited in the app",
			qd:      commonv1alpha1.QualityDefinition{Quality: "DVD", PreferredSize: size("0")},
			current: definition("DVD", map[string]any{"preferredSize": float64(0)}),
			want:    map[string]any{"quality": map[string]any{"name": "DVD"}, "preferredSize": nil},
		},
		{
			name:    "no preferred size in Readarr",
			qd:      commonv1alpha1.QualityDefinition{Quality: "EPUB", PreferredSize: size("1")},
			current: definition("EPUB", map[string]any{"maxSize": float64(350)}),
			readarr: true,
			err:     "this app has no preferred size",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := qualityDefinitionPayload(tt.qd, tt.current, !tt.readarr)
			if tt.err != "" {
				require.ErrorContains(t, err, tt.err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestQualityDefinitionsUpdateInPlaceAndConverge(t *testing.T) {
	app := &rebuildAPI{items: map[string][]map[string]any{
		"qualitydefinition": {
			definition("SDTV", map[string]any{"minSize": float64(2), "maxSize": float64(100), "preferredSize": float64(95)}),
			definition("Bluray-2160p", map[string]any{"minSize": float64(35), "maxSize": nil, "preferredSize": nil}),
			definition("FLAC", map[string]any{"minSize": float64(0)}),
		},
	}}
	hc := app.client(t)
	opts := Options{QualityDefinitions: []commonv1alpha1.QualityDefinition{
		{Quality: "SDTV", MinSize: size("5.1"), MaxSize: size("0")},
		{Quality: "Bluray-2160p", MaxSize: size("0"), PreferredSize: size("0")},
		{Quality: "FLAC", MaxSize: size("0"), PreferredSize: size("0")},
	}}

	result := reconcileOnce(context.Background(), hc, opts, engine.SyncPolicy{}, nil)
	require.Empty(t, result.Errors)
	assert.Equal(t, []string{"PUT qualitydefinition/4"}, app.writes)
	sdtv := app.items["qualitydefinition"][0]
	assert.InDelta(t, 5.1, sdtv["minSize"], 0)
	assert.Nil(t, sdtv["maxSize"])
	assert.InDelta(t, 95, sdtv["preferredSize"], 0)
	assert.Equal(t, "SDTV", sdtv["title"])
	assert.Empty(t, result.Managed, "the app's qualities are never the operator's to delete")

	second := reconcileOnce(context.Background(), hc, opts, engine.SyncPolicy{}, nil)
	require.Empty(t, second.Errors)
	assert.Len(t, app.writes, 1, "second pass must not write")

	opts.QualityDefinitions = []commonv1alpha1.QualityDefinition{{Quality: "Laserdisc", MinSize: size("1")}}
	missing := reconcileOnce(context.Background(), hc, opts, engine.SyncPolicy{}, nil)
	require.Len(t, missing.Errors, 1)
	assert.Contains(t, missing.Errors[0], "qualityDefinitions(Laserdisc): the app has no such quality")
	assert.Len(t, app.writes, 1)
}

func TestQualityProfileLanguageIsLookedUpByName(t *testing.T) {
	var mu sync.Mutex
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		if r.URL.Path == "/api/v3/language" {
			_, _ = w.Write([]byte(`[{"id":-2,"name":"Original"},{"id":1,"name":"English"}]`))
			return
		}
		_, _ = w.Write([]byte(`[]`))
	}))
	t.Cleanup(srv.Close)
	hc, err := httpapi.NewHTTPClient(srv.URL, httpapi.AuthNone, httpapi.WithTransport(&http.Transport{}))
	require.NoError(t, err)

	qps, err := qualityProfileResources(context.Background(), hc, "v3", []commonv1alpha1.QualityProfile{{Name: "HD", Language: "Original"}}, nil)
	require.NoError(t, err)
	require.Len(t, qps, 1)
	assert.Equal(t, map[string]any{"id": float64(-2), "name": "Original"}, qps[0]["language"])

	_, err = qualityProfileResources(context.Background(), hc, "v3", []commonv1alpha1.QualityProfile{{Name: "HD", Language: "Klingon"}}, nil)
	assert.ErrorContains(t, err, `qualityProfiles(HD): language "Klingon" does not exist`)

	mu.Lock()
	paths = nil
	mu.Unlock()
	qps, err = qualityProfileResources(context.Background(), hc, "v3", []commonv1alpha1.QualityProfile{{Name: "HD"}}, nil)
	require.NoError(t, err)
	assert.NotContains(t, qps[0], "language")
	mu.Lock()
	defer mu.Unlock()
	assert.NotContains(t, paths, "/api/v3/language", "the languages are read only for a profile that names one")
}
