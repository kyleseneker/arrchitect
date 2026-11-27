package pvr

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	pvrv1alpha1 "github.com/kyleseneker/arrchitect/api/pvr/v1alpha1"
	"github.com/kyleseneker/arrchitect/internal/engine"
)

func fieldValue(raw string) *commonv1alpha1.FieldValue {
	fv := &commonv1alpha1.FieldValue{}
	_ = fv.UnmarshalJSON([]byte(raw))
	return fv
}

func TestDefinition(t *testing.T) {
	tests := []struct {
		name       string
		app        App
		wantPrefix string
	}{
		{"v3 for Sonarr/Radarr", Sonarr, "/api/v3"},
		{"v1 for Lidarr/Readarr", Lidarr, "/api/v1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			def := Definition(tt.app)
			assert.Equal(t, tt.wantPrefix+"/system/status", def.HealthPath)
			assert.Len(t, def.Settings, 5+len(tt.app.Sections))
			assert.True(t, len(def.Resources) >= 8)

			for _, s := range def.Settings {
				assert.Contains(t, s.Path, tt.wantPrefix)
			}
			for _, r := range def.Resources {
				assert.Contains(t, r.Path, tt.wantPrefix)
			}
		})
	}
}

func TestDefinitionAddsTheAppsOwnSections(t *testing.T) {
	paths := func(app App) map[string]string {
		out := map[string]string{}
		for _, s := range Definition(app).Settings {
			out[s.Name] = s.Path
		}
		return out
	}
	assert.Equal(t, "/api/v3/config/importlist", paths(Sonarr)["importListConfig"])
	assert.Equal(t, "/api/v3/config/metadata", paths(Radarr)["metadataConfig"])
	assert.Equal(t, "/api/v3/config/importlist", paths(Radarr)["importListConfig"])
	assert.Equal(t, "/api/v1/config/metadataprovider", paths(Lidarr)["metadataProvider"])
	assert.NotContains(t, paths(Lidarr), "development", "Lidarr has no development section")
	assert.Equal(t, "/api/v1/config/metadataprovider", paths(Readarr)["metadataProvider"])
	assert.Equal(t, "/api/v1/config/development", paths(Readarr)["development"])
}

func TestDefinition_ResourceOrder(t *testing.T) {
	def := Definition(Sonarr)
	names := make([]string, len(def.Resources))
	for i, r := range def.Resources {
		names[i] = r.Name
	}
	// Tags must come first, importLists must come last
	assert.Equal(t, "tags", names[0])
	assert.Equal(t, "importLists", names[len(names)-1])
	// customFormats before qualityProfiles
	cfIdx, qpIdx := -1, -1
	for i, n := range names {
		if n == "customFormats" {
			cfIdx = i
		}
		if n == "qualityProfiles" {
			qpIdx = i
		}
	}
	assert.Greater(t, qpIdx, cfIdx)
}

func TestItemsNameWhatTheSamePassCreates(t *testing.T) {
	app := &rebuildAPI{}
	hc := app.client(t)
	opts := Options{
		DownloadClients: []commonv1alpha1.DownloadClient{{Name: "SAB", Protocol: "usenet", Implementation: "Sabnzbd", Host: "sab", Port: 8080}},
		Indexers:        []commonv1alpha1.Indexer{{Name: "Geek", Protocol: "usenet", Implementation: "Newznab", DownloadClientName: "SAB"}},
		ReleaseProfiles: []commonv1alpha1.ReleaseProfile{{Name: "No x265", Ignored: []string{"x265"}, IndexerName: "Geek"}},
	}

	result := reconcileOnce(context.Background(), hc, opts, engine.SyncPolicy{}, nil)
	require.Empty(t, result.Errors)
	assert.Equal(t, app.items["downloadclient"][0]["id"], app.items["indexer"][0]["downloadClientId"])
	assert.Equal(t, app.items["indexer"][0]["id"], app.items["releaseprofile"][0]["indexerId"])
}

func TestObserveNamesWhatTheSamePassWouldCreate(t *testing.T) {
	app := &rebuildAPI{}
	hc := app.client(t)
	opts := Options{
		DownloadClients: []commonv1alpha1.DownloadClient{{Name: "SAB", Protocol: "usenet", Implementation: "Sabnzbd", Host: "sab", Port: 8080}},
		Indexers:        []commonv1alpha1.Indexer{{Name: "Geek", Protocol: "usenet", Implementation: "Newznab", DownloadClientName: "SAB"}},
		ReleaseProfiles: []commonv1alpha1.ReleaseProfile{{Name: "No x265", Ignored: []string{"x265"}, IndexerName: "Geek"}},
	}

	result := reconcileOnce(context.Background(), hc, opts, engine.SyncPolicy{Observe: true}, nil)
	require.Empty(t, result.Errors)
	assert.Subset(t, result.Drift, []string{"downloadClients(SAB)", "indexers(Geek)", "releaseProfiles(No x265)"})
	app.mu.Lock()
	defer app.mu.Unlock()
	assert.Empty(t, app.writes)
}

func TestSettingsGoBeforeTheKindsTheyGovern(t *testing.T) {
	app := &rebuildAPI{}
	hc := app.client(t)
	spec := pvrv1alpha1.RadarrSpec{ImportListConfig: pvrv1alpha1.RadarrImportListConfig{ListSyncLevel: "logOnly"}}
	opts := Options{App: Radarr, ImportLists: []commonv1alpha1.ImportList{{Name: "Trakt", Implementation: "TraktListImport"}}}

	require.Empty(t, Reconcile(context.Background(), hc, spec, opts, engine.NewPass("radarr", engine.SyncPolicy{}, nil)))
	app.mu.Lock()
	writes := slices.Clone(app.writes)
	app.mu.Unlock()
	settings, lists := slices.Index(writes, "PUT config/importlist/1"), slices.Index(writes, "POST importlist")
	require.NotEqual(t, -1, settings, "the list sync level was never written: %v", writes)
	require.NotEqual(t, -1, lists, "the import list was never created: %v", writes)
	assert.Less(t, settings, lists, "Radarr's list sync level must change before its lists do")
}

func TestImportListsWaitForTheirSyncSetting(t *testing.T) {
	for name, fail := range map[string]string{"read": "config/importlist", "write": "config/importlist/1"} {
		t.Run(name, func(t *testing.T) {
			app := &rebuildAPI{fail: fail}
			hc := app.client(t)
			spec := pvrv1alpha1.RadarrSpec{ImportListConfig: pvrv1alpha1.RadarrImportListConfig{ListSyncLevel: "logOnly"}}
			opts := Options{App: Radarr, ImportLists: []commonv1alpha1.ImportList{{Name: "Trakt", Implementation: "TraktListImport"}}}

			problems := Reconcile(context.Background(), hc, spec, opts, engine.NewPass("radarr", engine.SyncPolicy{}, nil))
			require.Len(t, problems, 2)
			assert.Contains(t, problems[0], "importListConfig: ")
			assert.Equal(t, "importLists: left out until importListConfig syncs", problems[1])
			app.mu.Lock()
			defer app.mu.Unlock()
			assert.NotContains(t, app.writes, "POST importlist", "a list changed under the old sync level")
		})
	}
}

func TestOnlyRadarrTurnsAnImportListOff(t *testing.T) {
	for _, app := range []App{Sonarr, Radarr, Lidarr, Readarr} {
		t.Run(app.Name, func(t *testing.T) {
			api := &rebuildAPI{}
			hc := api.client(t)
			opts := Options{App: app, ImportLists: []commonv1alpha1.ImportList{{
				Name: "Trakt", Implementation: "TraktListImport", Enabled: new(false), EnableAutomaticAdd: new(true),
			}}}

			result := reconcileOnce(context.Background(), hc, opts, engine.SyncPolicy{}, nil)
			api.mu.Lock()
			defer api.mu.Unlock()
			if app.ImportListFields.Enable != "" {
				require.Empty(t, result.Errors)
				require.Len(t, api.items["importlist"], 1)
				assert.Equal(t, false, api.items["importlist"][0]["enabled"])
				return
			}
			require.Len(t, result.Errors, 1)
			assert.Contains(t, result.Errors[0], engine.AppName(app.Name)+" import lists are always active")
			assert.NotContains(t, api.writes, "POST importlist", "a list declared off must not be created active")
		})
	}
}

func TestEveryKindIsBuiltInOneStage(t *testing.T) {
	builders := Options{App: Sonarr}.builders(context.Background(), nil)
	staged := map[string]bool{}
	for _, kinds := range stages {
		for _, kind := range kinds {
			assert.Contains(t, builders, kind, "a stage names a kind with no builder")
			assert.False(t, staged[kind], "%s is in two stages", kind)
			staged[kind] = true
		}
	}
	staged["tags"] = true
	for _, r := range Definition(Sonarr).Resources {
		if r.Name != delayProfilesKind {
			assert.True(t, staged[r.Name], "%s is in no stage", r.Name)
		}
	}
	assert.Len(t, staged, len(builders))
}

func TestSections(t *testing.T) {
	type testSpec struct {
		MediaManagement      *map[string]any `json:"mediaManagement,omitempty"`
		Naming               *map[string]any `json:"naming,omitempty"`
		IndexerConfig        *map[string]any `json:"indexerConfig,omitempty"`
		DownloadClientConfig *map[string]any `json:"downloadClientConfig,omitempty"`
		UI                   *map[string]any `json:"ui,omitempty"`
	}

	tests := []struct {
		name     string
		spec     any
		wantKeys []string
	}{
		{
			name: "all sections",
			spec: testSpec{
				MediaManagement: &map[string]any{"a": 1},
				Naming:          &map[string]any{"b": 2},
			},
			wantKeys: []string{"mediaManagement", "naming"},
		},
		{
			name:     "empty spec",
			spec:     testSpec{},
			wantKeys: []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sections, err := Sections(tt.spec, nil)
			require.NoError(t, err)
			assert.Len(t, sections, len(tt.wantKeys))
			for _, k := range tt.wantKeys {
				assert.Contains(t, sections, k)
			}
		})
	}
}

func TestSectionsSendAnEmptyRecycleBin(t *testing.T) {
	sections, err := Sections(pvrv1alpha1.SonarrSpec{
		MediaManagement: &pvrv1alpha1.SonarrMediaManagement{RecycleBin: new("")},
	}, nil)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"recycleBin": ""}, sections["mediaManagement"], "an empty recycleBin turns it off")
}

func TestSectionsNumberSonarrNamingChoices(t *testing.T) {
	spec := pvrv1alpha1.SonarrSpec{Naming: &pvrv1alpha1.SonarrNaming{ColonReplacementFormat: "smart", MultiEpisodeStyle: "prefixedRange"}}
	sections, err := Sections(spec, Sonarr.Choices)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"colonReplacementFormat": 4, "multiEpisodeStyle": 5}, sections["naming"])

	sections, err = Sections(pvrv1alpha1.SonarrSpec{Naming: &pvrv1alpha1.SonarrNaming{
		ColonReplacementFormat: "custom", CustomColonReplacementFormat: " -",
	}}, Sonarr.Choices)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"colonReplacementFormat": 5, "customColonReplacementFormat": " -"}, sections["naming"])

	_, err = Sections(pvrv1alpha1.SonarrSpec{Naming: &pvrv1alpha1.SonarrNaming{ColonReplacementFormat: "underscore"}}, Sonarr.Choices)
	assert.ErrorContains(t, err, `naming.colonReplacementFormat: unknown value "underscore"`)
}

func TestSectionsCarryReadarrsMetadataAndDevelopment(t *testing.T) {
	sections, err := Sections(pvrv1alpha1.ReadarrSpec{
		MetadataProvider: &pvrv1alpha1.ReadarrMetadataProvider{WriteBookTags: "sync", UpdateCovers: new(true)},
		Development:      pvrv1alpha1.ReadarrDevelopment{MetadataSource: "https://api.bookinfo.pro"},
	}, Readarr.Choices)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"writeBookTags": "sync", "updateCovers": true}, sections["metadataProvider"])
	assert.Equal(t, map[string]any{"metadataSource": "https://api.bookinfo.pro"}, sections["development"])
}

func TestSectionsNumberLidarrAndReadarrColonReplacement(t *testing.T) {
	sections, err := Sections(pvrv1alpha1.LidarrSpec{Naming: &pvrv1alpha1.LidarrNaming{ColonReplacementFormat: "dash"}}, Lidarr.Choices)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"colonReplacementFormat": 1}, sections["naming"])
	assert.Equal(t, Lidarr.Choices, Readarr.Choices)
}

func TestFolderNameDefaultsToTheLastPathElement(t *testing.T) {
	for in, want := range map[string]string{
		"/data/media/music":  "music",
		"/data/media/music/": "music",
		`D:\Media\Books\`:    "Books",
		"/":                  "",
	} {
		assert.Equal(t, want, folderName(in), in)
	}
}

// rebuildAPI persists writes and assigns IDs like a newly initialized app. This
// exercises ordering and convergence, not compatibility with a real Sonarr or Radarr.
type rebuildAPI struct {
	mu     sync.Mutex
	items  map[string][]map[string]any
	writes []string
	nextID int
	fail   string
}

func (a *rebuildAPI) client(t *testing.T) *engine.HTTPClient {
	t.Helper()
	if a.items == nil {
		a.items = map[string][]map[string]any{}
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		defer a.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		resource := strings.TrimPrefix(r.URL.Path, "/api/v3/")
		if r.Method == http.MethodGet {
			if resource == a.fail {
				http.Error(w, "unavailable", http.StatusServiceUnavailable)
				return
			}
			if strings.HasPrefix(resource, "config/") {
				_ = json.NewEncoder(w).Encode(map[string]any{"id": 1})
				return
			}
			items := a.items[resource]
			if items == nil {
				items = []map[string]any{}
			}
			_ = json.NewEncoder(w).Encode(items)
			return
		}
		a.writes = append(a.writes, r.Method+" "+resource)
		if resource == a.fail {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		var body map[string]any
		if r.Method != http.MethodDelete {
			_ = json.NewDecoder(r.Body).Decode(&body)
		}
		switch r.Method {
		case http.MethodPost:
			a.nextID++
			body["id"] = float64(a.nextID)
			a.items[resource] = append(a.items[resource], body)
		case http.MethodPut, http.MethodDelete:
			id, _ := strconv.Atoi(path.Base(resource))
			resource = path.Dir(resource)
			for i, item := range a.items[resource] {
				if item["id"] == float64(id) {
					if r.Method == http.MethodPut {
						a.items[resource][i] = body
					} else {
						a.items[resource] = slices.Delete(a.items[resource], i, i+1)
					}
					break
				}
			}
		}
		_ = json.NewEncoder(w).Encode(body)
	}))
	t.Cleanup(srv.Close)
	hc, err := engine.NewHTTPClient(srv.URL, engine.AuthNone, engine.WithTransport(&http.Transport{}))
	require.NoError(t, err)
	return hc
}

func TestRebuildResolvesReferencesAndConverges(t *testing.T) {
	for _, firstID := range []int{100, 800} {
		t.Run(fmt.Sprint(firstID), func(t *testing.T) {
			app := &rebuildAPI{nextID: firstID}
			hc := app.client(t)
			opts := Options{
				Tags:            []commonv1alpha1.Tag{{Label: "tv"}},
				DownloadClients: []commonv1alpha1.DownloadClient{{Name: "downloads", TagLabels: []string{"tv"}}},
				Indexers:        []commonv1alpha1.Indexer{{Name: "indexer", TagLabels: []string{"tv"}}},
				Notifications:   []commonv1alpha1.Notification{{Name: "notify", TagLabels: []string{"tv"}}},
				CustomFormats:   []commonv1alpha1.CustomFormat{{Name: "preferred"}},
				QualityProfiles: []commonv1alpha1.QualityProfile{{Name: "HD", FormatItems: []commonv1alpha1.QualityProfileFormatItem{{Name: "preferred", Score: new(int32(100))}}}},
				ImportLists:     []commonv1alpha1.ImportList{{Name: "list", QualityProfileName: "HD", TagLabels: []string{"tv"}}},
			}
			result := reconcileOnce(context.Background(), hc, opts, engine.SyncPolicy{}, nil)
			require.Empty(t, result.Errors)
			tagID := app.items["tag"][0]["id"]
			for _, kind := range []string{"downloadclient", "indexer", "notification", "importlist"} {
				require.Equal(t, []any{tagID}, app.items[kind][0]["tags"], kind)
			}
			require.Equal(t, app.items["qualityprofile"][0]["id"], app.items["importlist"][0]["qualityProfileId"])
			require.Equal(t, []string{"tv"}, opts.DownloadClients[0].TagLabels, "must not mutate spec")
			writes := len(app.writes)
			second := reconcileOnce(context.Background(), hc, opts, engine.SyncPolicy{}, result.Managed)
			require.Empty(t, second.Errors)
			require.Len(t, app.writes, writes, "second pass must not write")
			require.Equal(t, result.Managed, second.Managed, "ownership snapshots must not accumulate duplicates")
		})
	}
}

func TestMissingTagBlocksDependentWritesAndRetainsOwnership(t *testing.T) {
	app := &rebuildAPI{}
	hc := app.client(t)
	opts := Options{Tags: []commonv1alpha1.Tag{{Label: "created"}}, DownloadClients: []commonv1alpha1.DownloadClient{{Name: "downloads", TagLabels: []string{"missing"}}}}
	result := reconcileOnce(context.Background(), hc, opts, engine.SyncPolicy{Prune: true}, map[string][]string{"downloadClients": {"old"}})
	require.NotEmpty(t, result.Errors)
	require.Equal(t, []string{"created"}, result.Managed["tags"])
	require.Equal(t, []string{"old"}, result.Managed["downloadClients"])
	require.Equal(t, []string{"POST tag"}, app.writes)
}

// Observe mode creates nothing, so a reference to an item the resource declares
// can't resolve yet. The item that refers to it is a difference enforcing would
// settle, not a failure.
func TestObserveReportsReferencesToDeclaredItemsAsDifferences(t *testing.T) {
	app := &rebuildAPI{}
	hc := app.client(t)
	opts := Options{
		Tags:            []commonv1alpha1.Tag{{Label: "TV"}},
		DownloadClients: []commonv1alpha1.DownloadClient{{Name: "downloads", TagLabels: []string{"tv"}}},
		CustomFormats:   []commonv1alpha1.CustomFormat{{Name: "x265"}},
		QualityProfiles: []commonv1alpha1.QualityProfile{{Name: "HD", FormatItems: []commonv1alpha1.QualityProfileFormatItem{{Name: "x265", Score: new(int32(10))}}}},
		ImportLists:     []commonv1alpha1.ImportList{{Name: "list", Implementation: "Trakt", QualityProfileName: "HD"}},
		DelayProfiles:   []commonv1alpha1.DelayProfile{{TagLabels: []string{"tv"}}},
	}
	result := reconcileOnce(context.Background(), hc, opts, engine.SyncPolicy{Observe: true}, nil)
	assert.Empty(t, result.Errors)
	assert.Subset(t, result.Drift, []string{
		"tags(tv)", "downloadClients(downloads)", "customFormats(x265)", "qualityProfiles(HD)", "importLists(list)",
	})
	assert.Empty(t, app.writes)
	assert.Empty(t, result.Managed)
}

func TestObserveStillFailsAReferenceToAnUndeclaredItem(t *testing.T) {
	app := &rebuildAPI{}
	hc := app.client(t)
	opts := Options{DownloadClients: []commonv1alpha1.DownloadClient{{Name: "downloads", TagLabels: []string{"typo"}}}}
	result := reconcileOnce(context.Background(), hc, opts, engine.SyncPolicy{Observe: true}, nil)
	assert.Equal(t, []string{`downloadClients(downloads): tag: the app has no item named "typo"`}, result.Errors)
	assert.Empty(t, app.writes)
}

func TestReferenceFailuresNeverDropAssociations(t *testing.T) {
	for _, tc := range []struct {
		name  string
		items []map[string]any
		fail  bool
	}{
		{name: "missing"},
		{name: "ambiguous", items: []map[string]any{{"label": "tv", "id": 1}, {"label": "tv", "id": 2}}},
		{name: "invalid id", items: []map[string]any{{"label": "tv", "id": 0}}},
		{name: "lookup error", fail: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := &rebuildAPI{items: map[string][]map[string]any{"tag": tc.items}}
			if tc.fail {
				app.fail = "tag"
			}
			hc := app.client(t)
			result := reconcileOnce(context.Background(), hc, Options{Indexers: []commonv1alpha1.Indexer{{Name: "indexer", TagLabels: []string{"tv"}}}}, engine.SyncPolicy{}, nil)
			require.NotEmpty(t, result.Errors)
			require.Empty(t, app.writes)
		})
	}
}

func TestMissingQualityReferencesBlockWrites(t *testing.T) {
	for _, opts := range []Options{
		{QualityProfiles: []commonv1alpha1.QualityProfile{{Name: "HD", FormatItems: []commonv1alpha1.QualityProfileFormatItem{{Name: "missing"}}}}},
		{ImportLists: []commonv1alpha1.ImportList{{Name: "list", QualityProfileName: "missing"}}},
	} {
		app := &rebuildAPI{}
		hc := app.client(t)
		result := reconcileOnce(context.Background(), hc, opts, engine.SyncPolicy{}, nil)
		require.NotEmpty(t, result.Errors)
		require.Empty(t, app.writes)
	}
}

func TestABadReferenceBlocksOnlyItsKind(t *testing.T) {
	app := &rebuildAPI{items: map[string][]map[string]any{
		"downloadclient": {{"id": float64(1), "name": "Old", "implementation": "QBittorrent"}},
	}}
	hc := app.client(t)
	opts := Options{
		DownloadClients: []commonv1alpha1.DownloadClient{{Name: "qBit", Protocol: "torrent", Implementation: "QBittorrent", Host: "qbit", Port: 8080, TagLabels: []string{"missing"}}},
		Indexers:        []commonv1alpha1.Indexer{{Name: "Geek", Protocol: "usenet", Implementation: "Newznab"}},
	}

	result := reconcileOnce(context.Background(), hc, opts, engine.SyncPolicy{Prune: true},
		map[string][]string{"downloadClients": {"Old"}})

	assert.Equal(t, []string{`downloadClients(qBit): tag: the app has no item named "missing"`}, result.Errors)
	assert.Equal(t, []string{"POST indexer"}, app.writes, "the indexer is still written, and nothing touches download clients")
	assert.Equal(t, []string{"Old"}, result.Managed["downloadClients"], "a kind that failed isn't pruned")
}

func TestRemotePathMappingLifecycle(t *testing.T) {
	app := &rebuildAPI{}
	hc := app.client(t)
	ctx := context.Background()
	opts := Options{RemotePathMappings: []commonv1alpha1.RemotePathMapping{
		{Host: "seedbox-a", RemotePath: "/downloads", LocalPath: "/data/a"},
		{Host: "seedbox-b", RemotePath: "/downloads", LocalPath: "/data/b"},
	}}
	result := reconcileOnce(ctx, hc, opts, engine.SyncPolicy{}, nil)
	require.Empty(t, result.Errors)
	require.Len(t, app.items["remotepathmapping"], 2)
	require.Len(t, result.Managed["remotePathMappings"], 2)
	writes := len(app.writes)
	result = reconcileOnce(ctx, hc, opts, engine.SyncPolicy{}, result.Managed)
	require.Empty(t, result.Errors)
	require.Len(t, app.writes, writes)
	opts.RemotePathMappings[0].LocalPath = "/data/new"
	observed := reconcileOnce(ctx, hc, opts, engine.SyncPolicy{Observe: true}, result.Managed)
	require.Empty(t, observed.Errors)
	require.Len(t, app.writes, writes)
	result = reconcileOnce(ctx, hc, opts, engine.SyncPolicy{}, result.Managed)
	require.Empty(t, result.Errors)
	require.Equal(t, "/data/new/", app.items["remotepathmapping"][0]["localPath"])
	// An unrelated hand-created mapping shares a path with the managed entries.
	app.items["remotepathmapping"] = append(app.items["remotepathmapping"], map[string]any{"id": float64(900), "host": "manual", "remotePath": "/downloads/", "localPath": "/data/manual/"})
	opts.RemotePathMappings = opts.RemotePathMappings[:1]
	// A later quality-profile stage must not resurrect ownership removed by prune.
	opts.QualityProfiles = []commonv1alpha1.QualityProfile{{Name: "HD"}}
	result = reconcileOnce(ctx, hc, opts, engine.SyncPolicy{Prune: true}, result.Managed)
	require.Empty(t, result.Errors)
	require.Len(t, result.Managed["remotePathMappings"], 1)
	require.Len(t, app.items["remotepathmapping"], 2)
	require.NoError(t, engine.DeleteManagedResources(ctx, hc, Definition(Sonarr), engine.NewOwnership("sonarr", result.Managed)))
	require.Len(t, app.items["remotepathmapping"], 1)
	require.Equal(t, "manual", app.items["remotepathmapping"][0]["host"])
}

func TestExistingItemsKeepWhatTheResourceLeavesOut(t *testing.T) {
	app := &rebuildAPI{items: map[string][]map[string]any{
		"downloadclient": {{
			"id": float64(1), "name": "qBit", "protocol": "torrent", "implementation": "QBittorrent", "configContract": "QBittorrentSettings",
			"enable": false, "priority": float64(3), "removeCompletedDownloads": false, "removeFailedDownloads": true, "tags": []any{float64(4)},
			"fields": []any{
				map[string]any{"name": "host", "value": "old"},
				map[string]any{"name": "port", "value": float64(8080)},
				map[string]any{"name": "useSsl", "value": true},
				map[string]any{"name": "tvCategory", "value": "tv-hd"},
			},
		}},
		"indexer": {{
			"id": float64(2), "name": "Geek", "protocol": "usenet", "implementation": "Newznab", "configContract": "NewznabSettings",
			"enableRss": false, "enableAutomaticSearch": true, "enableInteractiveSearch": true, "priority": float64(25), "tags": []any{float64(4)},
			"fields": []any{map[string]any{"name": "baseUrl", "value": "https://api.nzbgeek.info"}},
		}},
		"notification": {{
			"id": float64(3), "name": "Discord", "implementation": "Discord", "configContract": "DiscordSettings",
			"onGrab": true, "tags": []any{float64(4)}, "fields": []any{map[string]any{"name": "webHookUrl", "value": "https://discord.test/hook"}},
		}},
	}}
	hc := app.client(t)
	ctx := context.Background()
	opts := Options{
		App:             Sonarr,
		DownloadClients: []commonv1alpha1.DownloadClient{{Name: "qBit", Protocol: "torrent", Implementation: "QBittorrent", Host: "qbit", Port: 8080}},
		Indexers:        []commonv1alpha1.Indexer{{Name: "Geek", Protocol: "usenet", Implementation: "Newznab", Priority: int32(10)}},
		Notifications:   []commonv1alpha1.Notification{{Name: "Discord", Implementation: "Discord", OnDownload: new(true)}},
	}

	result := reconcileOnce(ctx, hc, opts, engine.SyncPolicy{}, nil)
	require.Empty(t, result.Errors)

	dc := app.items["downloadclient"][0]
	assert.Equal(t, false, dc["enable"])
	assert.Equal(t, float64(3), dc["priority"])
	assert.Equal(t, false, dc["removeCompletedDownloads"])
	assert.Equal(t, []any{float64(4)}, dc["tags"])
	assert.Equal(t, []any{
		map[string]any{"name": "host", "value": "qbit"},
		map[string]any{"name": "port", "value": float64(8080)},
		map[string]any{"name": "useSsl", "value": true},
		map[string]any{"name": "tvCategory", "value": "tv-hd"},
	}, dc["fields"])

	idx := app.items["indexer"][0]
	assert.Equal(t, float64(10), idx["priority"])
	assert.Equal(t, false, idx["enableRss"])
	assert.Equal(t, []any{float64(4)}, idx["tags"])
	assert.Equal(t, []any{map[string]any{"name": "baseUrl", "value": "https://api.nzbgeek.info"}}, idx["fields"])

	notification := app.items["notification"][0]
	assert.Equal(t, true, notification["onGrab"])
	assert.Equal(t, true, notification["onDownload"])
	assert.Equal(t, []any{float64(4)}, notification["tags"])
	assert.Equal(t, []any{map[string]any{"name": "webHookUrl", "value": "https://discord.test/hook"}}, notification["fields"])

	writes := len(app.writes)
	result = reconcileOnce(ctx, hc, opts, engine.SyncPolicy{}, result.Managed)
	require.Empty(t, result.Errors)
	assert.Len(t, app.writes, writes)
}

func TestNewItemsGetTheAppsDefaults(t *testing.T) {
	app := &rebuildAPI{}
	hc := app.client(t)
	opts := Options{
		App:             Radarr,
		DownloadClients: []commonv1alpha1.DownloadClient{{Name: "qBit", Protocol: "torrent", Implementation: "QBittorrent", Host: "qbit", Port: 8080}},
		Indexers:        []commonv1alpha1.Indexer{{Name: "Geek", Protocol: "usenet", Implementation: "Newznab"}},
		ImportLists:     []commonv1alpha1.ImportList{{Name: "Trakt", Implementation: "TraktListImport"}},
	}

	result := reconcileOnce(context.Background(), hc, opts, engine.SyncPolicy{}, nil)
	require.Empty(t, result.Errors)

	dc := app.items["downloadclient"][0]
	assert.Equal(t, true, dc["enable"])
	assert.Equal(t, float64(1), dc["priority"])
	assert.Equal(t, true, dc["removeCompletedDownloads"])
	assert.Equal(t, true, dc["removeFailedDownloads"])
	assert.Equal(t, []any{}, dc["tags"])

	idx := app.items["indexer"][0]
	assert.Equal(t, true, idx["enableRss"])
	assert.Equal(t, true, idx["enableAutomaticSearch"])
	assert.Equal(t, true, idx["enableInteractiveSearch"])
	assert.Equal(t, []any{}, idx["fields"])

	assert.Equal(t, true, app.items["importlist"][0]["enabled"])
}

func TestUppercaseTagLabelsSettle(t *testing.T) {
	app := &rebuildAPI{}
	hc := app.client(t)
	opts := Options{
		Tags:            []commonv1alpha1.Tag{{Label: "4K"}},
		DownloadClients: []commonv1alpha1.DownloadClient{{Name: "qBit", Protocol: "torrent", Implementation: "QBittorrent", Host: "qbit", Port: 8080, TagLabels: []string{"4K"}}},
	}

	result := reconcileOnce(context.Background(), hc, opts, engine.SyncPolicy{}, nil)
	require.Empty(t, result.Errors)
	require.Len(t, app.items["tag"], 1)
	assert.Equal(t, "4k", app.items["tag"][0]["label"])
	assert.Equal(t, []any{app.items["tag"][0]["id"]}, app.items["downloadclient"][0]["tags"])

	writes := len(app.writes)
	result = reconcileOnce(context.Background(), hc, opts, engine.SyncPolicy{}, result.Managed)
	require.Empty(t, result.Errors)
	assert.Len(t, app.writes, writes)
}

func TestRemotePathMappingWindowsIdentity(t *testing.T) {
	a := map[string]any{"host": "seedbox", "remotePath": `C:\Downloads`}
	b := map[string]any{"host": "seedbox", "remotePath": `C:\Downloads\`}
	require.Equal(t, remotePathMappingKey(a), remotePathMappingKey(b))
	require.Equal(t, `C:\Downloads\`, directoryPath(`C:\Downloads`))
	// JSON tuple encoding avoids separator collisions between arbitrary hosts/paths.
	require.NotEqual(t, remotePathMappingKey(map[string]any{"host": "a/b", "remotePath": "c"}), remotePathMappingKey(map[string]any{"host": "a", "remotePath": "b/c"}))
}

func TestDuplicateNormalizedMappingBlocksWrites(t *testing.T) {
	app := &rebuildAPI{}
	hc := app.client(t)
	opts := Options{RemotePathMappings: []commonv1alpha1.RemotePathMapping{
		{Host: "seedbox", RemotePath: "/downloads", LocalPath: "/data/a"},
		{Host: "seedbox", RemotePath: "/downloads/", LocalPath: "/data/b"},
	}}
	result := reconcileOnce(context.Background(), hc, opts, engine.SyncPolicy{}, nil)
	require.NotEmpty(t, result.Errors)
	require.Empty(t, app.writes)
}

func TestAutoTagsResolveTheProfilesAndTagsTheyNameAndConverge(t *testing.T) {
	app := &rebuildAPI{items: map[string][]map[string]any{
		"tag":            {{"id": float64(3), "label": "anime"}, {"id": float64(5), "label": "hd"}},
		"qualityprofile": {{"id": float64(7), "name": "HD-1080p"}},
	}}
	hc := app.client(t)
	opts := Options{AutoTags: []commonv1alpha1.AutoTag{{
		Name: "Anime HD", TagLabels: []string{"HD"},
		Specifications: []commonv1alpha1.AutoTagSpecification{
			{Name: "Profile", Implementation: "QualityProfileSpecification", Required: new(true), QualityProfileName: "HD-1080p"},
			{Name: "Anime", Implementation: "TagSpecification", TagLabel: "Anime"},
			{Name: "Years", Implementation: "YearSpecification", Fields: []commonv1alpha1.ConfigField{
				{Name: "min", Value: fieldValue("2000")}, {Name: "max", Value: fieldValue("2030")},
			}},
		},
	}}}

	result := reconcileOnce(context.Background(), hc, opts, engine.SyncPolicy{}, nil)
	require.Empty(t, result.Errors)
	assert.Equal(t, []string{"POST autotagging"}, app.writes)
	created := app.items["autotagging"][0]
	assert.Equal(t, []any{float64(5)}, created["tags"])
	specs, _ := created["specifications"].([]any)
	require.Len(t, specs, 3)
	assert.Equal(t, []any{map[string]any{"name": "value", "value": float64(7)}}, specs[0].(map[string]any)["fields"])
	assert.Equal(t, []any{map[string]any{"name": "value", "value": float64(3)}}, specs[1].(map[string]any)["fields"])
	assert.Equal(t, map[string][]string{"autoTags": {"Anime HD"}}, result.Managed)

	second := reconcileOnce(context.Background(), hc, opts, engine.SyncPolicy{}, result.Managed)
	require.Empty(t, second.Errors)
	assert.Len(t, app.writes, 1, "second pass must not write")

	opts.AutoTags[0].Specifications[0].QualityProfileName = "Ultra-HD"
	missing := reconcileOnce(context.Background(), hc, opts, engine.SyncPolicy{}, result.Managed)
	require.Len(t, missing.Errors, 1)
	assert.Contains(t, missing.Errors[0], `autoTags(Anime HD): Profile: the app has no item named "Ultra-HD"`)

	opts.AutoTags[0].Specifications[0] = commonv1alpha1.AutoTagSpecification{
		Name: "Profile", Implementation: "QualityProfileSpecification", QualityProfileName: "HD-1080p",
		Fields: []commonv1alpha1.ConfigField{{Name: "value", Value: fieldValue("7")}},
	}
	both := reconcileOnce(context.Background(), hc, opts, engine.SyncPolicy{}, result.Managed)
	require.Len(t, both.Errors, 1)
	assert.Contains(t, both.Errors[0], "autoTags(Anime HD): Profile: qualityProfileName fills in value, so leave that field out")
	assert.Len(t, app.writes, 1)
}

func TestResolveListSyncTagNamesTheTagByLabel(t *testing.T) {
	hc := (&rebuildAPI{items: map[string][]map[string]any{"tag": {{"id": float64(4), "label": "unlisted"}}}}).client(t)
	config := map[string]any{"listSyncLevel": "keepAndTag", "listSyncTagLabel": "Unlisted"}
	require.NoError(t, resolveListSyncTag(context.Background(), hc, "v3", config, nil))
	assert.Equal(t, map[string]any{"listSyncLevel": "keepAndTag", "listSyncTag": 4}, config)

	config = map[string]any{"listSyncLevel": "keepAndTag", "listSyncTagLabel": "gone"}
	assert.EqualError(t, resolveListSyncTag(context.Background(), hc, "v3", config, nil),
		`listSyncTagLabel: the app has no item named "gone"`)
	require.NoError(t, resolveListSyncTag(context.Background(), hc, "v3", config, []string{"gone"}),
		"observe mode plans a tag the resource declares")
}

func TestMetadataConsumersUpdateInPlaceAndConverge(t *testing.T) {
	app := &rebuildAPI{items: map[string][]map[string]any{
		"metadata": {
			{"id": float64(1), "name": "Kodi (XBMC) / Emby", "implementation": "XbmcMetadata", "configContract": "XbmcMetadataSettings",
				"enable": false, "fields": []any{
					map[string]any{"name": "seriesMetadata", "value": true},
					map[string]any{"name": "episodeImages", "value": false},
				}},
			{"id": float64(2), "name": "Roksbox", "implementation": "RoksboxMetadata", "enable": false, "fields": []any{}},
		},
	}}
	hc := app.client(t)
	opts := Options{MetadataConsumers: []commonv1alpha1.MetadataConsumer{{
		Implementation: "XbmcMetadata", Enable: new(true),
		Fields: []commonv1alpha1.MetadataConsumerField{{Name: "episodeImages", Value: fieldValue("true")}},
	}}}

	result := reconcileOnce(context.Background(), hc, opts, engine.SyncPolicy{}, nil)
	require.Empty(t, result.Errors)
	assert.Equal(t, []string{"PUT metadata/1"}, app.writes)
	kodi := app.items["metadata"][0]
	assert.Equal(t, true, kodi["enable"])
	assert.Equal(t, "Kodi (XBMC) / Emby", kodi["name"])
	assert.Equal(t, []any{
		map[string]any{"name": "seriesMetadata", "value": true},
		map[string]any{"name": "episodeImages", "value": true},
	}, kodi["fields"], "fields left out keep their values")
	assert.Empty(t, result.Managed, "the app's consumers are never the operator's to delete")

	second := reconcileOnce(context.Background(), hc, opts, engine.SyncPolicy{}, nil)
	require.Empty(t, second.Errors)
	assert.Len(t, app.writes, 1, "second pass must not write")

	opts.MetadataConsumers = []commonv1alpha1.MetadataConsumer{{Implementation: "KodiMetadata"}}
	missing := reconcileOnce(context.Background(), hc, opts, engine.SyncPolicy{}, nil)
	require.Len(t, missing.Errors, 1)
	assert.Contains(t, missing.Errors[0], "metadataConsumers(KodiMetadata): the app has no such consumer; it has RoksboxMetadata, XbmcMetadata")

	opts.MetadataConsumers = []commonv1alpha1.MetadataConsumer{{
		Implementation: "XbmcMetadata",
		Fields:         []commonv1alpha1.MetadataConsumerField{{Name: "movieImages", Value: fieldValue("true")}},
	}}
	unknown := reconcileOnce(context.Background(), hc, opts, engine.SyncPolicy{}, nil)
	require.Len(t, unknown.Errors, 1)
	assert.Contains(t, unknown.Errors[0],
		"metadataConsumers(XbmcMetadata): XbmcMetadata does not accept field(s) movieImages; it accepts: episodeImages, seriesMetadata")
	assert.Len(t, app.writes, 1)
}

const maxOrder = float64(2147483647)

func TestLidarrRootFoldersKeepTheirNameAndTakeDefaults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/rootfolder":
			_, _ = w.Write([]byte(`[{"id":1,"path":"/music","name":"Music Library"}]`))
		case "/api/v1/qualityprofile":
			_, _ = w.Write([]byte(`[{"id":2,"name":"Lossless"}]`))
		case "/api/v1/metadataprofile":
			_, _ = w.Write([]byte(`[{"id":3,"name":"Standard"}]`))
		default:
			_, _ = w.Write([]byte(`[]`))
		}
	}))
	t.Cleanup(srv.Close)
	hc, err := engine.NewHTTPClient(srv.URL, engine.AuthNone, engine.WithTransport(&http.Transport{}))
	require.NoError(t, err)
	folder := func(dir string) commonv1alpha1.RootFolder {
		return commonv1alpha1.RootFolder{
			Path: dir, DefaultQualityProfileName: "Lossless", DefaultMetadataProfileName: "Standard",
			DefaultMonitorOption: "future", DefaultNewItemMonitorOption: "new", DefaultTagLabels: []string{"flac"},
		}
	}
	opts := Options{App: Lidarr, RootFolders: []commonv1alpha1.RootFolder{folder("/music"), folder("/data/audiobooks")}}

	rfs, err := rootFolderResources(context.Background(), hc, "v1", opts, engine.NamedIDs{"flac": {7}})
	require.NoError(t, err)
	require.Len(t, rfs, 2)
	assert.NotContains(t, rfs[0], "name", "renamed a folder the spec doesn't name")
	assert.Equal(t, "audiobooks", rfs[1]["name"], "a new folder is named after its path")
	assert.Equal(t, "future", rfs[0]["defaultMonitorOption"])
	assert.Equal(t, "new", rfs[0]["defaultNewItemMonitorOption"])
	assert.Equal(t, []any{7}, rfs[0]["defaultTags"])

	_, err = rootFolderResources(context.Background(), hc, "v1", opts, engine.NamedIDs{})
	assert.ErrorContains(t, err, "rootFolders(/music): tag")
}

// reconcileOnce is Reconcile in a pass of its own, its outcome gathered for
// assertions. Options that name no app reconcile as Sonarr.
func reconcileOnce(ctx context.Context, hc *engine.HTTPClient, opts Options, policy engine.SyncPolicy, managed map[string][]string) outcome {
	if opts.App.Name == "" {
		opts.App = Sonarr
	}
	pass := engine.NewPass(hc.AppLabel(), policy, managed)
	errs := Reconcile(ctx, hc, struct{}{}, opts, pass)
	return outcome{
		Synced: pass.SyncedItems(), Errors: errs, Drift: pass.Drift(), Pruned: pass.Owned.Pruned(), Managed: pass.Owned.Managed(),
	}
}

// outcome is a pass's result, with what it deleted and the ownership it ended
// with.
type outcome struct {
	engine.ReconcileResult
	Pruned  []engine.PrunedResource
	Managed map[string][]string
}
