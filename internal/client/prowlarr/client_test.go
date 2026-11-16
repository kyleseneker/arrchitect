package prowlarr

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	indexersv1alpha1 "github.com/kyleseneker/arrchitect/api/indexers/v1alpha1"
)

func TestProwlarrDefinition(t *testing.T) {
	def := ProwlarrDefinition()
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

func TestProwlarrResources(t *testing.T) {
	tests := []struct {
		name     string
		opts     ProwlarrOptions
		wantKeys []string
	}{
		{
			name:     "empty options",
			opts:     ProwlarrOptions{},
			wantKeys: []string{},
		},
		{
			name: "all resource types",
			opts: ProwlarrOptions{
				Tags:         []commonv1alpha1.Tag{{Label: "test"}},
				Applications: []map[string]any{{"name": "sonarr"}},
				Indexers:     []indexersv1alpha1.ProwlarrIndexer{{Name: "idx"}},
				Proxies:      []indexersv1alpha1.ProwlarrProxy{{Name: "proxy"}},
				DownloadClients: []indexersv1alpha1.ProwlarrDownloadClient{
					{Name: "dc", Protocol: "torrent", Implementation: "QBittorrent"},
				},
				Notifications: []commonv1alpha1.Notification{{Name: "notif", Implementation: "Discord"}},
			},
			wantKeys: []string{"tags", "applications", "indexers", "proxies", "downloadClients", "notifications"},
		},
		{
			name: "tags only",
			opts: ProwlarrOptions{
				Tags: []commonv1alpha1.Tag{{Label: "tag1"}, {Label: "tag2"}},
			},
			wantKeys: []string{"tags"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resources := ProwlarrResources(tt.opts, nil)
			assert.Len(t, resources, len(tt.wantKeys))
			for _, k := range tt.wantKeys {
				assert.Contains(t, resources, k)
			}
		})
	}
}

func TestBuildProwlarrApplicationPayload(t *testing.T) {
	app := indexersv1alpha1.ProwlarrApplication{
		Name:           "Sonarr",
		SyncLevel:      "fullSync",
		Implementation: "Sonarr",
		ConfigContract: "SonarrSettings",
		ProwlarrUrl:    "http://prowlarr:9696",
		BaseUrl:        "http://sonarr:8989",
		SyncCategories: []int32{5030, 5040},
		TagLabels:      []string{"tag1", "tag2"},
	}

	p := BuildProwlarrApplicationPayload(app, "sonarr-api-key", map[string]int{"tag1": 1, "tag2": 2})

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
	assert.Equal(t, []any{1, 2}, tags, "labels must be resolved to the IDs Prowlarr assigned")
}

func TestBuildProwlarrApplicationPayloadLeavesOutWhatTheResourceLeavesOut(t *testing.T) {
	p := BuildProwlarrApplicationPayload(indexersv1alpha1.ProwlarrApplication{
		Name: "Sonarr", Implementation: "Sonarr", ConfigContract: "SonarrSettings", BaseUrl: "http://sonarr:8989",
	}, "key", nil)

	assert.NotContains(t, p, "syncLevel")
	assert.NotContains(t, p, "tags")
	assert.Equal(t, []map[string]any{
		{"name": "baseUrl", "value": "http://sonarr:8989"},
		{"name": "apiKey", "value": "key"},
	}, p["fields"], "the Prowlarr URL and sync categories stay as Prowlarr has them")
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
				Enable: new(false), AppProfileId: new(int32(1)), Priority: new(int32(25)),
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
			p := BuildProwlarrIndexerPayload(tt.idx, nil)
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
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := BuildProwlarrProxyPayload(tt.proxy, nil)
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
			p := BuildProwlarrDownloadClientPayload(tt.dc, nil)
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

func TestSetTagsMapsLabelsToIDs(t *testing.T) {
	payload := map[string]any{}
	setTags(payload, []string{"flaresolverr", "public"}, map[string]int{"flaresolverr": 7, "public": 9})
	assert.Equal(t, []any{7, 9}, payload["tags"])
}

func TestSetTagsDropsUnknownLabels(t *testing.T) {
	payload := map[string]any{}
	setTags(payload, []string{"never-declared"}, map[string]int{"other": 1})
	assert.Equal(t, []any{}, payload["tags"], "an unknown label must not be sent as a string; Prowlarr expects integer tag IDs")
}

func TestSetTagsLeavesUndeclaredTagsAlone(t *testing.T) {
	payload := map[string]any{}
	setTags(payload, nil, map[string]int{"other": 1})
	assert.NotContains(t, payload, "tags")

	setTags(payload, []string{}, nil)
	assert.Equal(t, []any{}, payload["tags"], "an empty list clears the tags")
}
