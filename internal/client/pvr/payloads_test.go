package pvr

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
)

func TestBuildDownloadClientPayload(t *testing.T) {
	tests := []struct {
		name          string
		dc            commonv1alpha1.DownloadClient
		tags          []int
		secrets       DownloadClientResolvedSecrets
		categoryField string
		check         func(t *testing.T, p map[string]any)
	}{
		{
			name: "leaves out what the resource leaves out",
			dc: commonv1alpha1.DownloadClient{
				Name: "qBit", Protocol: "torrent", Implementation: "QBittorrent",
				Host: "qbit", Port: 8080,
			},
			categoryField: "tvCategory",
			check: func(t *testing.T, p map[string]any) {
				assert.Equal(t, "qBit", p["name"])
				assert.Equal(t, "QBittorrentSettings", p["configContract"])
				for _, key := range []string{"enable", "priority", "removeCompletedDownloads", "removeFailedDownloads", "tags"} {
					assert.NotContains(t, p, key)
				}
				assert.Equal(t, []map[string]any{
					{"name": "host", "value": "qbit"},
					{"name": "port", "value": int32(8080)},
				}, p["fields"], "useSsl and the category stay as the app has them")
			},
		},
		{
			name: "an empty tag list clears the tags",
			dc: commonv1alpha1.DownloadClient{
				Name: "qBit", Protocol: "torrent", Implementation: "QBittorrent", Host: "qbit", Port: 8080,
			},
			tags:          []int{},
			categoryField: "tvCategory",
			check: func(t *testing.T, p map[string]any) {
				assert.Equal(t, []any{}, p["tags"])
			},
		},
		{
			name: "all options set",
			dc: commonv1alpha1.DownloadClient{
				Name: "qBit", Protocol: "torrent", Implementation: "QBittorrent",
				Host: "qbit", Port: 8080, Category: "movies",
				Enable: new(false), Priority: int32(5),
				RemoveCompletedDownloads: new(false),
				RemoveFailedDownloads:    new(false),
				UseSSL:                   new(true),
				URLBase:                  "/qbt",
				ConfigContract:           "CustomContract",
			},
			tags:          []int{1, 2},
			categoryField: "movieCategory",
			check: func(t *testing.T, p map[string]any) {
				assert.Equal(t, false, p["enable"])
				assert.Equal(t, int32(5), p["priority"])
				assert.Equal(t, false, p["removeCompletedDownloads"])
				assert.Equal(t, false, p["removeFailedDownloads"])
				assert.Equal(t, "CustomContract", p["configContract"])
				assert.Len(t, p["tags"], 2)

				fields := p["fields"].([]map[string]any)
				found := false
				for _, f := range fields {
					if f["name"] == "useSsl" {
						assert.Equal(t, true, f["value"])
						found = true
					}
					if f["name"] == "urlBase" {
						assert.Equal(t, "/qbt", f["value"])
					}
				}
				assert.True(t, found, "useSsl field not found")
			},
		},
		{
			name: "with secrets",
			dc: commonv1alpha1.DownloadClient{
				Name: "qBit", Protocol: "torrent", Implementation: "QBittorrent",
				Host: "qbit", Port: 8080, Category: "tv",
			},
			secrets:       DownloadClientResolvedSecrets{Username: "admin", Password: "pass123", APIKey: "key123"},
			categoryField: "tvCategory",
			check: func(t *testing.T, p map[string]any) {
				fields := p["fields"].([]map[string]any)
				names := make(map[string]any)
				for _, f := range fields {
					names[f["name"].(string)] = f["value"]
				}
				assert.Equal(t, "admin", names["username"])
				assert.Equal(t, "pass123", names["password"])
				assert.Equal(t, "key123", names["apiKey"])
			},
		},
		{
			name: "extra fields skip known fields",
			dc: commonv1alpha1.DownloadClient{
				Name: "qBit", Protocol: "torrent", Implementation: "QBittorrent",
				Host: "qbit", Port: 8080, Category: "tv",
				Fields: []commonv1alpha1.ConfigField{
					{Name: "host", Value: fieldValue(`"should-be-ignored"`)},
					{Name: "initialState", Value: fieldValue(`0`)},
				},
			},
			categoryField: "tvCategory",
			check: func(t *testing.T, p map[string]any) {
				fields := p["fields"].([]map[string]any)
				hostCount := 0
				foundInitial := false
				for _, f := range fields {
					if f["name"] == "host" {
						hostCount++
						assert.Equal(t, "qbit", f["value"], "host should use first-class field, not extra field")
					}
					if f["name"] == "initialState" {
						foundInitial = true
						assert.Equal(t, float64(0), f["value"])
					}
				}
				assert.Equal(t, 1, hostCount, "host should appear exactly once")
				assert.True(t, foundInitial, "initialState field should be appended")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := BuildDownloadClientPayload(tt.dc, tt.secrets, tt.categoryField, tt.tags)
			tt.check(t, p)
		})
	}
}

func TestBuildIndexerPayload(t *testing.T) {
	tests := []struct {
		name  string
		idx   commonv1alpha1.Indexer
		tags  []int
		check func(t *testing.T, p map[string]any)
	}{
		{
			name: "leaves out what the resource leaves out",
			idx: commonv1alpha1.Indexer{
				Name: "NZBgeek", Protocol: "usenet", Implementation: "Newznab",
			},
			check: func(t *testing.T, p map[string]any) {
				assert.NotContains(t, p, "enable", "indexers have no enable property; sending one is permanent drift")
				for _, key := range []string{"enableRss", "enableAutomaticSearch", "enableInteractiveSearch", "priority", "fields", "tags"} {
					assert.NotContains(t, p, key)
				}
				assert.Equal(t, "NewznabSettings", p["configContract"])
			},
		},
		{
			name: "enable true turns on the uses left out",
			idx: commonv1alpha1.Indexer{
				Name: "On", Protocol: "torrent", Implementation: "Torznab", Enable: new(true), EnableRSS: new(false),
			},
			check: func(t *testing.T, p map[string]any) {
				assert.Equal(t, false, p["enableRss"])
				assert.Equal(t, true, p["enableAutomaticSearch"])
				assert.Equal(t, true, p["enableInteractiveSearch"])
			},
		},
		{
			name: "all options set",
			idx: commonv1alpha1.Indexer{
				Name: "NZBgeek", Protocol: "usenet", Implementation: "Newznab",
				Enable: new(false), EnableRSS: new(false),
				EnableAutomaticSearch: new(false), EnableInteractiveSearch: new(false),
				ConfigContract: "CustomContract", Priority: int32(10),
				Fields: []commonv1alpha1.ConfigField{
					{Name: "baseUrl", Value: fieldValue(`"http://nzbgeek.info"`)},
				},
			},
			tags: []int{1},
			check: func(t *testing.T, p map[string]any) {
				assert.Equal(t, false, p["enableRss"])
				assert.Equal(t, "CustomContract", p["configContract"])
				assert.Equal(t, int32(10), p["priority"])
				tags := p["tags"].([]any)
				assert.Len(t, tags, 1)
			},
		},
		{
			name: "enable false turns off every use",
			idx: commonv1alpha1.Indexer{
				Name: "Off", Protocol: "torrent", Implementation: "Torznab",
				Enable: new(false), EnableRSS: new(true), EnableAutomaticSearch: new(true), EnableInteractiveSearch: new(true),
			},
			check: func(t *testing.T, p map[string]any) {
				assert.Equal(t, false, p["enableRss"])
				assert.Equal(t, false, p["enableAutomaticSearch"])
				assert.Equal(t, false, p["enableInteractiveSearch"])
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := BuildIndexerPayload(tt.idx, tt.tags)
			tt.check(t, p)
		})
	}
}

func TestBuildNotificationPayload(t *testing.T) {
	tests := []struct {
		name  string
		n     commonv1alpha1.Notification
		check func(t *testing.T, p map[string]any)
	}{
		{
			name: "minimal",
			n:    commonv1alpha1.Notification{Name: "Discord", Implementation: "Discord"},
			check: func(t *testing.T, p map[string]any) {
				assert.Equal(t, "Discord", p["name"])
				assert.Equal(t, "DiscordSettings", p["configContract"])
				_, hasOnGrab := p["onGrab"]
				assert.False(t, hasOnGrab, "nil bools should not be set")
				assert.NotContains(t, p, "fields")
				assert.NotContains(t, p, "tags")
			},
		},
		{
			name: "with triggers",
			n: commonv1alpha1.Notification{
				Name: "Slack", Implementation: "Slack",
				OnGrab: new(true), OnDownload: new(false),
				OnMovieAdded: new(true),
			},
			check: func(t *testing.T, p map[string]any) {
				assert.Equal(t, true, p["onGrab"])
				assert.Equal(t, false, p["onDownload"])
				assert.Equal(t, true, p["onMovieAdded"])
				_, hasOnRename := p["onRename"]
				assert.False(t, hasOnRename)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := BuildNotificationPayload(tt.n, nil)
			tt.check(t, p)
		})
	}
}

func TestBuildImportListPayload(t *testing.T) {
	sonarr, radarr, lidarr := Sonarr.ImportListFields, Radarr.ImportListFields, Lidarr.ImportListFields
	searching := commonv1alpha1.ImportList{
		Name: "List", Implementation: "CustomImport", SearchOnAdd: new(true), MonitorNewItems: "all", MonitorExisting: new(true),
	}
	tests := []struct {
		name    string
		il      commonv1alpha1.ImportList
		qpIDs   map[string]int
		monitor ImportListFields
		check   func(t *testing.T, p map[string]any)
	}{
		{
			name: "minimal",
			il:   commonv1alpha1.ImportList{Name: "Trakt", Implementation: "TraktListImport"},
			check: func(t *testing.T, p map[string]any) {
				assert.NotContains(t, p, "enable")
				assert.NotContains(t, p, "fields")
				assert.NotContains(t, p, "tags")
				assert.Equal(t, "TraktListImportSettings", p["configContract"])
			},
		},
		{
			name:    "radarr leaves enabled alone when it is left out",
			il:      commonv1alpha1.ImportList{Name: "List", Implementation: "RadarrListImport"},
			monitor: radarr,
			check: func(t *testing.T, p map[string]any) {
				assert.NotContains(t, p, "enabled")
			},
		},
		{
			name:    "sonarr has no enable property",
			il:      commonv1alpha1.ImportList{Name: "List", Implementation: "CustomImport", EnableAutomaticAdd: new(true)},
			monitor: sonarr,
			check: func(t *testing.T, p map[string]any) {
				assert.NotContains(t, p, "enable", "Sonarr drops it, so it reads back missing and is rewritten forever")
				assert.Equal(t, true, p["enableAutomaticAdd"])
			},
		},
		{
			name:    "radarr names enable and automatic add differently",
			il:      commonv1alpha1.ImportList{Name: "List", Implementation: "RadarrListImport", Enabled: new(false), EnableAutomaticAdd: new(true)},
			monitor: radarr,
			check: func(t *testing.T, p map[string]any) {
				assert.Equal(t, false, p["enabled"])
				assert.Equal(t, true, p["enableAuto"])
				assert.NotContains(t, p, "enableAutomaticAdd")
			},
		},
		{
			name: "with quality profile resolution",
			il: commonv1alpha1.ImportList{
				Name: "Trakt", Implementation: "TraktListImport",
				QualityProfileName: "HD-1080p", RootFolderPath: "/tv",
				Monitor: "all", ListOrder: new(int32(1)),
			},
			qpIDs:   map[string]int{"HD-1080p": 5},
			monitor: sonarr,
			check: func(t *testing.T, p map[string]any) {
				assert.Equal(t, 5, p["qualityProfileId"])
				assert.Equal(t, "/tv", p["rootFolderPath"])
				assert.Equal(t, "all", p["shouldMonitor"], "Sonarr reads the monitor enum from shouldMonitor")
				assert.NotContains(t, p, "monitor")
				assert.Equal(t, int32(1), p["listOrder"])
			},
		},
		{
			name:    "radarr names the monitor enum monitor",
			il:      commonv1alpha1.ImportList{Name: "List", Implementation: "RadarrListImport", Monitor: "movieAndCollection"},
			monitor: radarr,
			check: func(t *testing.T, p map[string]any) {
				assert.Equal(t, "movieAndCollection", p["monitor"])
				assert.NotContains(t, p, "shouldMonitor")
			},
		},
		{
			name:    "sonarr searches for missing episodes",
			il:      searching,
			monitor: sonarr,
			check: func(t *testing.T, p map[string]any) {
				assert.Equal(t, true, p["searchForMissingEpisodes"])
				assert.Equal(t, "all", p["monitorNewItems"])
				assert.NotContains(t, p, "searchOnAdd", "Sonarr drops it, so it reads back missing and is rewritten forever")
				assert.NotContains(t, p, "shouldMonitorExisting")
			},
		},
		{
			name:    "radarr searches on add and monitors nothing new",
			il:      searching,
			monitor: radarr,
			check: func(t *testing.T, p map[string]any) {
				assert.Equal(t, true, p["searchOnAdd"])
				assert.NotContains(t, p, "monitorNewItems")
				assert.NotContains(t, p, "shouldMonitorExisting")
			},
		},
		{
			name:    "lidarr searches and monitors existing artists",
			il:      searching,
			monitor: lidarr,
			check: func(t *testing.T, p map[string]any) {
				assert.Equal(t, true, p["shouldSearch"])
				assert.Equal(t, true, p["shouldMonitorExisting"])
				assert.Equal(t, "all", p["monitorNewItems"])
				assert.NotContains(t, p, "searchOnAdd")
			},
		},
		{
			name: "quality profile not found",
			il: commonv1alpha1.ImportList{
				Name: "Trakt", Implementation: "TraktListImport",
				QualityProfileName: "NonExistent",
			},
			qpIDs: map[string]int{"HD-1080p": 5},
			check: func(t *testing.T, p map[string]any) {
				_, has := p["qualityProfileId"]
				assert.False(t, has, "should not set ID for unresolved profile")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qpIDs := tt.qpIDs
			if qpIDs == nil {
				qpIDs = map[string]int{}
			}
			p := buildImportListPayload(tt.il, qpIDs, tt.monitor, nil)
			tt.check(t, p)
		})
	}
}

func TestBuildCustomFormatPayload(t *testing.T) {
	tests := []struct {
		name  string
		cf    commonv1alpha1.CustomFormat
		check func(t *testing.T, p map[string]any)
	}{
		{
			name: "minimal",
			cf:   commonv1alpha1.CustomFormat{Name: "BR-DISK"},
			check: func(t *testing.T, p map[string]any) {
				assert.Equal(t, map[string]any{"name": "BR-DISK"}, p)
			},
		},
		{
			name: "with specifications",
			cf: commonv1alpha1.CustomFormat{
				Name:                            "x265",
				IncludeCustomFormatWhenRenaming: new(true),
				Specifications: []commonv1alpha1.CustomFormatSpecification{
					{
						Name: "x265", Implementation: "ReleaseTitleSpecification",
						Negate: new(false), Required: new(true),
						Fields: []commonv1alpha1.ConfigField{
							{Name: "value", Value: fieldValue(`"x265"`)},
						},
					},
				},
			},
			check: func(t *testing.T, p map[string]any) {
				assert.Equal(t, true, p["includeCustomFormatWhenRenaming"])
				specs := p["specifications"].([]map[string]any)
				require.Len(t, specs, 1)
				assert.Equal(t, "x265", specs[0]["name"])
				assert.Equal(t, false, specs[0]["negate"])
				assert.Equal(t, true, specs[0]["required"])
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := buildCustomFormatPayload(tt.cf)
			tt.check(t, p)
		})
	}
}

func TestBuildFieldsPayload(t *testing.T) {
	tests := []struct {
		name   string
		fields []commonv1alpha1.ConfigField
		check  func(t *testing.T, result []map[string]any)
	}{
		{
			name:   "empty",
			fields: []commonv1alpha1.ConfigField{},
			check: func(t *testing.T, result []map[string]any) {
				assert.Empty(t, result)
			},
		},
		{
			name: "with values",
			fields: []commonv1alpha1.ConfigField{
				{Name: "baseUrl", Value: fieldValue(`"http://example.com"`)},
				{Name: "apiKey", Value: fieldValue(`"secret"`)},
			},
			check: func(t *testing.T, result []map[string]any) {
				require.Len(t, result, 2)
				assert.Equal(t, "baseUrl", result[0]["name"])
				assert.Equal(t, "http://example.com", result[0]["value"])
			},
		},
		{
			name: "nil value",
			fields: []commonv1alpha1.ConfigField{
				{Name: "optional"},
			},
			check: func(t *testing.T, result []map[string]any) {
				require.Len(t, result, 1)
				assert.Nil(t, result[0]["value"])
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildFieldsPayload(tt.fields)
			tt.check(t, result)
		})
	}
}

func TestBuildTagsPayload(t *testing.T) {
	tests := []struct {
		name string
		tags []int
		want []any
	}{
		{"empty", []int{}, []any{}},
		{"multiple", []int{1, 5, 10}, []any{1, 5, 10}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildTagsPayload(tt.tags)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestSetOptionalBool(t *testing.T) {
	tests := []struct {
		name    string
		val     *bool
		wantSet bool
		want    bool
	}{
		{"nil not set", nil, false, false},
		{"true", new(true), true, true},
		{"false", new(false), true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := map[string]any{}
			SetOptionalBool(p, "key", tt.val)
			if tt.wantSet {
				assert.Equal(t, tt.want, p["key"])
			} else {
				_, exists := p["key"]
				assert.False(t, exists)
			}
		})
	}
}

// TestPayloadsAreJSONSerializable verifies all payload builders produce JSON-safe maps.
func TestPayloadsAreJSONSerializable(t *testing.T) {
	dc := BuildDownloadClientPayload(
		commonv1alpha1.DownloadClient{Name: "test", Protocol: "torrent", Implementation: "QBittorrent", Host: "h", Port: 80},
		DownloadClientResolvedSecrets{}, "tvCategory", nil,
	)
	idx := BuildIndexerPayload(commonv1alpha1.Indexer{Name: "test", Protocol: "usenet", Implementation: "Newznab"}, nil)
	notif := BuildNotificationPayload(commonv1alpha1.Notification{Name: "test", Implementation: "Discord"}, nil)
	cf := buildCustomFormatPayload(commonv1alpha1.CustomFormat{Name: "test"})

	for name, payload := range map[string]any{
		"downloadClient": dc, "indexer": idx, "notification": notif, "customFormat": cf,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := json.Marshal(payload)
			assert.NoError(t, err)
		})
	}
}
