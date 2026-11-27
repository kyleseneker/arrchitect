package prowlarr

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	indexersv1alpha1 "github.com/kyleseneker/arrchitect/api/indexers/v1alpha1"
	"github.com/kyleseneker/arrchitect/internal/engine"
)

func TestProwlarrDefinition(t *testing.T) {
	def := Definition()
	assert.Equal(t, "/api/v1/system/status", def.HealthPath)
	assert.Empty(t, def.Settings, "Prowlarr has no settings endpoints")
	assert.True(t, len(def.Resources) >= 6)

	// Tags should be first
	assert.Equal(t, "tags", def.Resources[0].Name)

	// Verify all paths use v1
	for _, r := range def.Resources {
		assert.Contains(t, r.Path, "/api/v1/")
	}
}

func TestReconcileWritesEachDeclaredKind(t *testing.T) {
	app := &fakeProwlarr{}
	hc := app.client(t)
	opts := Options{
		Tags:         []commonv1alpha1.Tag{{Label: "Public"}},
		AppProfiles:  []indexersv1alpha1.ProwlarrAppProfile{{Name: "Standard"}},
		Applications: []Application{{Name: "Sonarr", Implementation: "Sonarr"}},
		Indexers: []indexersv1alpha1.ProwlarrIndexer{{
			Name: "idx", Implementation: "Torznab", TagLabels: []string{"public"}, AppProfileName: "Standard",
		}},
		Proxies:         []Proxy{{Name: "proxy", Implementation: "Http"}},
		DownloadClients: []indexersv1alpha1.ProwlarrDownloadClient{{Name: "dc", Protocol: "torrent", Implementation: "QBittorrent"}},
		Notifications:   []commonv1alpha1.Notification{{Name: "notif", Implementation: "Discord"}},
	}

	problems := Reconcile(context.Background(), hc, opts, engine.NewPass("prowlarr", engine.SyncPolicy{}, nil))
	require.Empty(t, problems)
	for _, path := range []string{"tag", "appprofile", "applications", "indexer", "indexerproxy", "downloadclient", "notification"} {
		assert.Len(t, app.items["/api/v1/"+path], 1, path)
	}
	indexer := app.items["/api/v1/indexer"][0]
	assert.Equal(t, []any{app.items["/api/v1/tag"][0]["id"]}, indexer["tags"])
	assert.Equal(t, app.items["/api/v1/appprofile"][0]["id"], indexer["appProfileId"])
}

func TestABadTagLeavesOutOnlyItsKind(t *testing.T) {
	app := &fakeProwlarr{lists: map[string][]map[string]any{"/api/v1/tag": {{"id": 1, "label": "known"}}}}
	hc := app.client(t)
	opts := Options{
		Indexers:        []indexersv1alpha1.ProwlarrIndexer{{Name: "idx", Implementation: "Torznab", TagLabels: []string{"known", "unknown"}}},
		DownloadClients: []indexersv1alpha1.ProwlarrDownloadClient{{Name: "dc", Protocol: "torrent", Implementation: "QBittorrent", TagLabels: []string{"known"}}},
	}

	problems := Reconcile(context.Background(), hc, opts, engine.NewPass("prowlarr", engine.SyncPolicy{}, nil))
	require.Len(t, problems, 1)
	assert.Contains(t, problems[0], "indexers(idx): tag")
	assert.Empty(t, app.items["/api/v1/indexer"])
	assert.Len(t, app.items["/api/v1/downloadclient"], 1)
}

func TestIndexersFollowARecreatedAppProfile(t *testing.T) {
	app := &fakeProwlarr{lists: map[string][]map[string]any{"/api/v1/appprofile": {{"id": 3, "name": "Standard"}}}}
	hc := app.client(t)
	opts := Options{Indexers: []indexersv1alpha1.ProwlarrIndexer{{Name: "idx", Implementation: "Torznab", AppProfileName: "Standard"}}}

	payloads, err := opts.builders(context.Background(), hc, false)["indexers"].Build(nil)
	require.NoError(t, err)
	assert.Equal(t, int32(3), payloads[0]["appProfileId"])

	app.lists["/api/v1/appprofile"] = []map[string]any{{"id": 7, "name": "Standard"}}
	payloads, err = opts.builders(context.Background(), hc, false)["indexers"].Build(nil)
	require.NoError(t, err)
	assert.Equal(t, int32(7), payloads[0]["appProfileId"])
	assert.Nil(t, opts.Indexers[0].AppProfileID, "the options must not carry the app's IDs")
}

func TestIndexersRejectAMissingAppProfile(t *testing.T) {
	app := &fakeProwlarr{lists: map[string][]map[string]any{"/api/v1/appprofile": {{"id": 3, "name": "Standard"}}}}
	hc := app.client(t)
	opts := Options{Indexers: []indexersv1alpha1.ProwlarrIndexer{{Name: "idx", AppProfileName: "Gone"}}}

	_, err := opts.builders(context.Background(), hc, false)["indexers"].Build(nil)
	assert.ErrorContains(t, err, "indexers(idx): appProfileName")
}

func TestObserveModePlansWhatTheResourceDeclares(t *testing.T) {
	app := &fakeProwlarr{}
	hc := app.client(t)
	opts := Options{
		Tags:        []commonv1alpha1.Tag{{Label: "TV"}},
		AppProfiles: []indexersv1alpha1.ProwlarrAppProfile{{Name: "Standard"}},
		Indexers:    []indexersv1alpha1.ProwlarrIndexer{{Name: "x", TagLabels: []string{"tv"}, AppProfileName: "Standard"}},
	}
	observe := engine.SyncPolicy{Observe: true}

	pass := engine.NewPass("prowlarr", observe, nil)
	require.Empty(t, Reconcile(context.Background(), hc, opts, pass))
	assert.Empty(t, app.writes)
	assert.Contains(t, pass.Drift(), "indexers(x)")

	opts.Indexers[0].TagLabels = []string{"typo"}
	problems := Reconcile(context.Background(), hc, opts, engine.NewPass("prowlarr", observe, nil))
	require.Len(t, problems, 1)
	assert.Contains(t, problems[0], `the app has no item named "typo"`, "a name the resource doesn't declare still fails")
}

func TestBuildProwlarrApplicationPayload(t *testing.T) {
	app := indexersv1alpha1.ProwlarrApplication{
		Name:           "Sonarr",
		SyncLevel:      "fullSync",
		Implementation: "Sonarr",
		ConfigContract: "SonarrSettings",
		ProwlarrURL:    "http://prowlarr:9696",
		BaseURL:        "http://sonarr:8989",
		SyncCategories: []int32{5030, 5040},
		TagLabels:      []string{"tag1", "tag2"},
	}

	p := buildApplicationPayload(app, map[string]string{"apiKey": "sonarr-api-key"}, []int{1, 2})

	assert.Equal(t, "Sonarr", p["name"])
	assert.Equal(t, "fullSync", p["syncLevel"])

	fields := p["fields"].([]map[string]any)
	require.Len(t, fields, 4)

	fieldMap := map[string]any{}
	for _, f := range fields {
		fieldMap[f["name"].(string)] = f["value"]
	}
	assert.Equal(t, "sonarr-api-key", fieldMap["apiKey"])
	assert.Equal(t, "http://prowlarr:9696", fieldMap["prowlarrUrl"])

	syncCats := fieldMap["syncCategories"].([]any)
	assert.Len(t, syncCats, 2)

	tags := p["tags"].([]any)
	assert.Len(t, tags, 2)
	assert.Equal(t, []any{1, 2}, tags)
}

func TestBuildProwlarrApplicationPayloadLeavesOutWhatTheResourceLeavesOut(t *testing.T) {
	p := buildApplicationPayload(indexersv1alpha1.ProwlarrApplication{
		Name: "Sonarr", Implementation: "Sonarr", ConfigContract: "SonarrSettings", BaseURL: "http://sonarr:8989",
	}, map[string]string{"apiKey": "key"}, nil)

	assert.NotContains(t, p, "syncLevel")
	assert.NotContains(t, p, "tags")
	assert.Equal(t, []map[string]any{
		{"name": "baseUrl", "value": "http://sonarr:8989"},
		{"name": "apiKey", "value": "key"},
	}, p["fields"], "the Prowlarr URL and sync categories stay as Prowlarr has them")
}

func TestBuildProwlarrApplicationPayloadSendsBasicAuth(t *testing.T) {
	p := buildApplicationPayload(indexersv1alpha1.ProwlarrApplication{
		Name: "Radarr", Implementation: "Radarr", BaseURL: "https://radarr.example",
	}, map[string]string{"apiKey": "key", "authUsername": "proxy", "authPassword": "secret"}, nil)

	assert.Equal(t, []map[string]any{
		{"name": "baseUrl", "value": "https://radarr.example"},
		{"name": "apiKey", "value": "key"},
		{"name": "authUsername", "value": "proxy"},
		{"name": "authPassword", "value": "secret"},
	}, p["fields"])
}

func TestBuildProwlarrIndexerPayload(t *testing.T) {
	tests := []struct {
		name  string
		idx   indexersv1alpha1.ProwlarrIndexer
		check func(t *testing.T, p map[string]any)
	}{
		{
			name: "leaves out what the resource leaves out",
			idx: indexersv1alpha1.ProwlarrIndexer{
				Name: "NZBgeek", Implementation: "Newznab", ConfigContract: "NewznabSettings",
			},
			check: func(t *testing.T, p map[string]any) {
				for _, key := range []string{"enable", "appProfileId", "priority", "fields", "tags"} {
					assert.NotContains(t, p, key)
				}
			},
		},
		{
			name: "all options",
			idx: indexersv1alpha1.ProwlarrIndexer{
				Name: "NZBgeek", Implementation: "Newznab", ConfigContract: "NewznabSettings",
				Enable: new(false), AppProfileID: new(int32(1)), Priority: new(int32(25)),
				Fields: []commonv1alpha1.ConfigField{
					{Name: "baseUrl", Value: &commonv1alpha1.FieldValue{Raw: []byte(`"http://nzbgeek.info"`)}},
					{Name: "apiKey", Value: nil},
				},
				TagLabels: []string{"tag1"},
			},
			check: func(t *testing.T, p map[string]any) {
				assert.Equal(t, false, p["enable"])
				assert.Equal(t, int32(1), p["appProfileId"])
				assert.Equal(t, int32(25), p["priority"])

				fields := p["fields"].([]map[string]any)
				require.Len(t, fields, 2)
				assert.Equal(t, "http://nzbgeek.info", fields[0]["value"])
				assert.Nil(t, fields[1]["value"])
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := buildIndexerPayload(tt.idx, nil)
			tt.check(t, p)
		})
	}
}

func TestBuildProwlarrProxyPayload(t *testing.T) {
	tests := []struct {
		name  string
		proxy indexersv1alpha1.ProwlarrProxy
		check func(t *testing.T, p map[string]any)
	}{
		{
			name:  "minimal",
			proxy: indexersv1alpha1.ProwlarrProxy{Name: "Proxy", Implementation: "Http", ConfigContract: "HttpSettings", Host: "proxy.local"},
			check: func(t *testing.T, p map[string]any) {
				assert.Equal(t, "Proxy", p["name"])
				fields := p["fields"].([]map[string]any)
				require.Len(t, fields, 1)
				assert.Equal(t, "proxy.local", fields[0]["value"])
			},
		},
		{
			name: "with timeout",
			proxy: indexersv1alpha1.ProwlarrProxy{
				Name: "Proxy", Implementation: "Socks5", ConfigContract: "Socks5Settings",
				Host: "proxy.local", RequestTimeout: new(int32(30)),
			},
			check: func(t *testing.T, p map[string]any) {
				fields := p["fields"].([]map[string]any)
				require.Len(t, fields, 2)
				assert.Equal(t, "requestTimeout", fields[1]["name"])
				assert.Equal(t, int32(30), fields[1]["value"])
			},
		},
	}
	t.Run("with port and credentials", func(t *testing.T) {
		proxy := indexersv1alpha1.ProwlarrProxy{Name: "Proxy", Implementation: "Http", ConfigContract: "HttpSettings", Host: "proxy.local", Port: 8080}
		p := buildProxyPayload(proxy, map[string]string{"username": "u", "password": "p"}, nil)
		assert.Equal(t, []map[string]any{
			{"name": "host", "value": "proxy.local"}, {"name": "port", "value": int32(8080)},
			{"name": "username", "value": "u"}, {"name": "password", "value": "p"},
		}, p["fields"])
	})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := buildProxyPayload(tt.proxy, nil, nil)
			tt.check(t, p)
		})
	}
}

func TestBuildProwlarrDownloadClientPayload(t *testing.T) {
	tests := []struct {
		name  string
		dc    indexersv1alpha1.ProwlarrDownloadClient
		check func(t *testing.T, p map[string]any)
	}{
		{
			name: "leaves out what the resource leaves out",
			dc: indexersv1alpha1.ProwlarrDownloadClient{
				Name: "qBit", Protocol: "torrent", Implementation: "QBittorrent",
			},
			check: func(t *testing.T, p map[string]any) {
				assert.Equal(t, "QBittorrentSettings", p["configContract"])
				for _, key := range []string{"enable", "priority", "fields", "categories", "tags"} {
					assert.NotContains(t, p, key)
				}
			},
		},
		{
			name: "with categories and custom contract",
			dc: indexersv1alpha1.ProwlarrDownloadClient{
				Name: "qBit", Protocol: "torrent", Implementation: "QBittorrent",
				ConfigContract: "CustomContract", Priority: new(int32(5)),
				Categories: []indexersv1alpha1.ProwlarrDownloadClientCategory{
					{ClientCategory: "tv", Categories: []int32{5030, 5040}},
				},
				Fields: []commonv1alpha1.ConfigField{
					{Name: "host", Value: &commonv1alpha1.FieldValue{Raw: []byte(`"qbit.local"`)}},
				},
			},
			check: func(t *testing.T, p map[string]any) {
				assert.Equal(t, "CustomContract", p["configContract"])
				assert.Equal(t, int32(5), p["priority"])

				cats := p["categories"].([]map[string]any)
				require.Len(t, cats, 1)
				assert.Equal(t, "tv", cats[0]["clientCategory"])
				catIDs := cats[0]["categories"].([]any)
				assert.Len(t, catIDs, 2)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := buildDownloadClientPayload(tt.dc, nil)
			tt.check(t, p)
		})
	}
}

func TestIntSliceToInterface(t *testing.T) {
	tests := []struct {
		name  string
		input []int32
		want  []any
	}{
		{"empty", []int32{}, []any{}},
		{"values", []int32{1, 2, 3}, []any{1, 2, 3}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, intSliceToInterface(tt.input))
		})
	}
}

func TestSetTagsLeavesUndeclaredTagsAlone(t *testing.T) {
	payload := map[string]any{}
	setTags(payload, nil)
	assert.NotContains(t, payload, "tags")

	setTags(payload, []int{})
	assert.Equal(t, []any{}, payload["tags"], "an empty list clears the tags")

	setTags(payload, []int{7, 9})
	assert.Equal(t, []any{7, 9}, payload["tags"])
}

// fakeProwlarr serves lists by path and keeps what's posted to it.
type fakeProwlarr struct {
	mu     sync.Mutex
	lists  map[string][]map[string]any
	items  map[string][]map[string]any
	writes []string
	nextID int
}

func (a *fakeProwlarr) client(t *testing.T) *engine.HTTPClient {
	t.Helper()
	if a.lists == nil {
		a.lists = map[string][]map[string]any{}
	}
	a.items = map[string][]map[string]any{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		defer a.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			items := append(slices.Clone(a.lists[r.URL.Path]), a.items[r.URL.Path]...)
			if items == nil {
				items = []map[string]any{}
			}
			_ = json.NewEncoder(w).Encode(items)
			return
		}
		a.writes = append(a.writes, r.Method+" "+r.URL.Path)
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		a.nextID++
		body["id"] = float64(100 + a.nextID)
		a.items[r.URL.Path] = append(a.items[r.URL.Path], body)
		_ = json.NewEncoder(w).Encode(body)
	}))
	t.Cleanup(srv.Close)
	hc, err := engine.NewHTTPClient(srv.URL, engine.AuthAPIKey,
		engine.WithAPIKey("k"), engine.WithAPIKeyHeader("X-Api-Key"), engine.WithTransport(&http.Transport{}))
	require.NoError(t, err)
	return hc
}
