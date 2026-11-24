package export

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	pvrv1alpha1 "github.com/kyleseneker/arrchitect/api/pvr/v1alpha1"
	pvrclient "github.com/kyleseneker/arrchitect/internal/client/pvr"
)

const fakeSonarrKey = "sonarr-key"

var fakeSonarrAPI = map[string]string{
	"/config/mediamanagement": `{"id":1,"recycleBin":"/trash","deleteEmptyFolders":true,"notInTheCRD":5}`,
	"/config/naming":          `{"id":1}`,
	"/config/indexer":         `{"id":1}`,
	"/config/downloadclient":  `{"id":1}`,
	"/config/ui":              `{"id":1}`,
	"/tag":                    `[{"id":1,"label":"anime"}]`,
	"/rootfolder":             `[{"id":1,"path":"/tv","freeSpace":123}]`,
	"/remotepathmapping":      `[]`,
	"/customformat": `[{"id":1,"name":"x265","includeCustomFormatWhenRenaming":false,"specifications":[
		{"name":"x265","implementation":"ReleaseTitleSpecification","negate":false,"required":true,"fields":[{"name":"value","value":"x265"}]}]}]`,
	"/qualityprofile": `[{"id":4,"name":"HD","upgradeAllowed":true,"cutoff":1001,"minFormatScore":0,"items":[
		{"quality":{"id":1,"name":"SDTV","source":"television"},"items":[],"allowed":false},
		{"id":1001,"name":"WEB 1080p","items":[{"quality":{"id":3,"name":"WEBDL-1080p"},"items":[],"allowed":true}],"allowed":true}],
		"formatItems":[{"format":1,"name":"x265","score":-100},{"format":2,"name":"Other","score":0}]}]`,
	"/downloadclient": `[{"id":1,"name":"qBit","enable":true,"protocol":"torrent","priority":1,"removeCompletedDownloads":true,
		"removeFailedDownloads":true,"implementation":"QBittorrent","configContract":"QBittorrentSettings","tags":[1],"fields":[
		{"name":"host","value":"qbit"},{"name":"port","value":8080},{"name":"useSsl","value":false},{"name":"urlBase","value":""},
		{"name":"username","value":"admin","privacy":"userName"},{"name":"password","value":"********","privacy":"password"},
		{"name":"tvCategory","value":"tv"},{"name":"initialState","value":0},{"name":"sequentialOrder","value":true},
		{"name":"rpcToken","value":"********","privacy":"password"}]}]`,
	"/downloadclient/schema": `[{"implementation":"QBittorrent","fields":[{"name":"initialState","value":0},{"name":"sequentialOrder","value":false}]}]`,
	"/indexer": `[{"id":1,"name":"Torznab","protocol":"TorrentDownloadProtocol","implementation":"Torznab","configContract":"TorznabSettings",
		"enableRss":true,"enableAutomaticSearch":true,"enableInteractiveSearch":false,"priority":25,"downloadClientId":1,"tags":[],"fields":[
		{"name":"baseUrl","value":"http://jackett"},{"name":"apiKey","value":"********","privacy":"apiKey"},{"name":"minimumSeeders","value":1}]}]`,
	"/indexer/schema": `[{"implementation":"Torznab","fields":[{"name":"baseUrl","value":""},{"name":"minimumSeeders","value":1}]}]`,
	"/notification": `[{"id":1,"name":"Hook","implementation":"Webhook","configContract":"WebhookSettings","onGrab":true,
		"supportsOnGrab":true,"onRename":false,"supportsOnRename":false,"includeHealthWarnings":true,"tags":[],
		"fields":[{"name":"url","value":"http://hook"}]}]`,
	"/notification/schema": `[{"implementation":"Webhook","fields":[{"name":"url","value":""}]}]`,
	"/releaseprofile":      `[{"id":1,"name":"No x265","enabled":true,"required":[],"ignored":["x265"],"indexerId":1,"tags":[1]}]`,
	"/importlist": `[{"id":1,"name":"Trakt","implementation":"TraktListImport","configContract":"TraktListSettings",
		"enableAutomaticAdd":true,"shouldMonitor":"all","qualityProfileId":4,"rootFolderPath":"/tv","seasonFolder":true,
		"seriesType":"standard","searchForMissingEpisodes":false,"monitorNewItems":"none","tags":[1],"fields":[{"name":"accessToken","value":"********","privacy":"password"}]}]`,
	"/importlist/schema": `[{"implementation":"TraktListImport","fields":[]}]`,
	"/metadata": `[
		{"id":1,"name":"Kodi (XBMC) / Emby","implementation":"XbmcMetadata","configContract":"XbmcMetadataSettings","enable":true,
			"fields":[{"name":"seriesMetadata","value":true},{"name":"episodeImages","value":false}]},
		{"id":2,"name":"Roksbox","implementation":"RoksboxMetadata","configContract":"RoksboxMetadataSettings","enable":false,
			"fields":[{"name":"episodeMetadata","value":true}]}]`,
	"/metadata/schema": `[{"implementation":"XbmcMetadata","fields":[{"name":"seriesMetadata","value":true},{"name":"episodeImages","value":true}]},
		{"implementation":"RoksboxMetadata","fields":[{"name":"episodeMetadata","value":false}]}]`,
	"/qualitydefinition": `[
		{"id":1,"quality":{"id":1,"name":"SDTV"},"title":"SDTV","minSize":2,"maxSize":100,"preferredSize":95},
		{"id":2,"quality":{"id":7,"name":"Bluray-1080p"},"title":"Blu-ray 1080p","minSize":17.1,"maxSize":null,"preferredSize":null},
		{"id":3,"quality":{"id":2,"name":"DVD"},"title":"DVD","minSize":0,"maxSize":0,"preferredSize":0},
		{"id":4,"quality":{"id":19,"name":"Bluray-2160p"},"title":"Bluray-2160p","minSize":35}]`,
	"/delayprofile": `[
		{"id":1,"order":2147483647,"tags":[],"enableUsenet":true,"enableTorrent":true,"preferredProtocol":"usenet",
			"usenetDelay":0,"torrentDelay":0,"bypassIfHighestQuality":true,"bypassIfAboveCustomFormatScore":false,"minimumCustomFormatScore":0},
		{"id":2,"order":1,"tags":[1],"enableUsenet":false,"enableTorrent":true,"preferredProtocol":"torrent",
			"usenetDelay":0,"torrentDelay":60,"bypassIfHighestQuality":false,"bypassIfAboveCustomFormatScore":true,"minimumCustomFormatScore":100}]`,
}

func fakeSonarr(t *testing.T, overrides map[string]string) string {
	t.Helper()
	return fakePVR(t, "/api/v3", overrides)
}

func fakePVR(t *testing.T, prefix string, overrides map[string]string) string {
	t.Helper()
	api := maps.Clone(fakeSonarrAPI)
	maps.Copy(api, overrides)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != fakeSonarrKey {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		body, ok := api[strings.TrimPrefix(r.URL.Path, prefix)]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func exportSonarr(t *testing.T, o Options, overrides ...map[string]string) (*pvrv1alpha1.Sonarr, Result) {
	t.Helper()
	var api map[string]string
	if len(overrides) > 0 {
		api = overrides[0]
	}
	o.App, o.URL, o.APIKey = "sonarr", fakeSonarr(t, api), fakeSonarrKey
	res, err := Run(context.Background(), o)
	require.NoError(t, err)
	cfg, ok := res.Config.(*pvrv1alpha1.Sonarr)
	require.True(t, ok)
	return cfg, res
}

func objectsOf[T client.Object](res Result) []T {
	var out []T
	for _, obj := range res.Objects {
		if o, ok := obj.(T); ok {
			out = append(out, o)
		}
	}
	return out
}

func TestPVRMetadataConsumersExportOnlyTheOnesTurnedOn(t *testing.T) {
	cfg, _ := exportSonarr(t, Options{})
	assert.Equal(t, []commonv1alpha1.MetadataConsumer{{
		Implementation: "XbmcMetadata",
		Fields:         []commonv1alpha1.MetadataConsumerField{{Name: "episodeImages", Value: &commonv1alpha1.FieldValue{Raw: []byte("false")}}},
	}}, cfg.Spec.MetadataConsumers)
}

func TestPVRSettingsKeepOnlyFieldsTheCRDHas(t *testing.T) {
	cfg, _ := exportSonarr(t, Options{})
	require.NotNil(t, cfg.Spec.MediaManagement)
	assert.Equal(t, new("/trash"), cfg.Spec.MediaManagement.RecycleBin)
	assert.True(t, *cfg.Spec.MediaManagement.DeleteEmptyFolders)
	assert.Equal(t, []commonv1alpha1.Tag{{Label: "anime"}}, cfg.Spec.Tags)
	assert.Equal(t, []commonv1alpha1.RootFolder{{Path: "/tv"}}, cfg.Spec.RootFolders)
}

func TestPVRDownloadClientMovesCredentialsToTheSecret(t *testing.T) {
	_, res := exportSonarr(t, Options{})
	clients := objectsOf[*pvrv1alpha1.SonarrDownloadClient](res)
	require.Len(t, clients, 1)
	dc := clients[0].Spec
	assert.Equal(t, "qbit", dc.Host)
	assert.Equal(t, int32(8080), dc.Port)
	assert.Equal(t, "tv", dc.Category)
	assert.Empty(t, dc.ConfigContract, "the default contract is implied")
	assert.Equal(t, []string{"anime"}, dc.TagLabels)
	assert.Equal(t, "download-client-qbit-username", dc.UsernameSecretRef.Key)
	assert.Equal(t, "download-client-qbit-password", dc.PasswordSecretRef.Key)
	require.Len(t, dc.Fields, 1, "fields left at the schema default are omitted")
	assert.Equal(t, "sequentialOrder", dc.Fields[0].Name)

	assert.Equal(t, "admin", res.Secret.StringData["download-client-qbit-username"], "a visible username is filled in")
	assert.Equal(t, fakeSonarrKey, res.Secret.StringData["api-key"])
	assert.Equal(t, []string{"import-list-trakt-accesstoken", "download-client-qbit-password", "indexer-torznab-apikey"}, res.Unfilled)
}

func TestPVRDownloadClientWarnsAboutAHiddenFieldItCannotReference(t *testing.T) {
	_, res := exportSonarr(t, Options{})
	for _, f := range objectsOf[*pvrv1alpha1.SonarrDownloadClient](res)[0].Spec.Fields {
		assert.NotEqual(t, "rpcToken", f.Name)
	}
	assert.Contains(t, res.Warnings,
		`download-client "qBit": the app hides rpcToken and the config can't read it from a Secret; set it in the app`)
}

func TestPVRIndexerNormalizesProtocolAndReferencesMaskedFields(t *testing.T) {
	_, res := exportSonarr(t, Options{})
	indexers := objectsOf[*pvrv1alpha1.SonarrIndexer](res)
	require.Len(t, indexers, 1)
	idx := indexers[0].Spec
	assert.Equal(t, "torrent", idx.Protocol)
	assert.False(t, *idx.EnableInteractiveSearch)
	assert.Equal(t, int32(25), idx.Priority)
	require.Len(t, idx.Fields, 2)
	assert.Equal(t, `"http://jackett"`, string(idx.Fields[0].Value.Raw))
	assert.Equal(t, "indexer-torznab-apikey", idx.Fields[1].ValueFrom.SecretKeyRef.Key)
	assert.Equal(t, "qBit", idx.DownloadClientName, "the pinned client is exported by name")
}

func TestPVRNotificationSkipsEventsTheImplementationLacks(t *testing.T) {
	cfg, _ := exportSonarr(t, Options{})
	require.Len(t, cfg.Spec.Notifications, 1)
	n := cfg.Spec.Notifications[0]
	assert.True(t, *n.OnGrab)
	assert.Nil(t, n.OnRename, "an unsupported event would fail validation")
	assert.True(t, *n.IncludeHealthWarnings)
}

func TestPVRImportListUsesTheAppsPropertyNames(t *testing.T) {
	cfg, _ := exportSonarr(t, Options{})
	require.Len(t, cfg.Spec.ImportLists, 1)
	il := cfg.Spec.ImportLists[0]
	assert.Equal(t, "all", il.Monitor)
	assert.True(t, *il.EnableAutomaticAdd)
	assert.False(t, *il.SearchOnAdd, "Sonarr's searchForMissingEpisodes is exported as searchOnAdd")
	assert.Equal(t, "none", il.MonitorNewItems)
	assert.Nil(t, il.MonitorExisting, "Sonarr has no shouldMonitorExisting")
	assert.Equal(t, "HD", il.QualityProfileName)
	assert.Equal(t, []string{"anime"}, il.TagLabels)
	assert.Equal(t, "import-list-trakt-accesstoken", il.Fields[0].ValueFrom.SecretKeyRef.Key)
}

func TestPVRQualityProfileNamesItsCutoffAndScoredFormats(t *testing.T) {
	cfg, _ := exportSonarr(t, Options{})
	require.Len(t, cfg.Spec.QualityProfiles, 1)
	qp := cfg.Spec.QualityProfiles[0]
	assert.Equal(t, "WEB 1080p", qp.Cutoff)
	require.Len(t, qp.Items, 2)
	assert.Equal(t, "SDTV", qp.Items[0].Quality.Name)
	assert.False(t, *qp.Items[0].Allowed)
	assert.Equal(t, "WEB 1080p", qp.Items[1].Name)
	assert.Equal(t, new(int32(3)), qp.Items[1].Items[0].Quality.ID)
	assert.Equal(t, []commonv1alpha1.QualityProfileFormatItem{{Name: "x265", Score: new(int32(-100))}}, qp.FormatItems)

	require.Len(t, cfg.Spec.CustomFormats, 1)
	raw, err := json.Marshal(cfg.Spec.CustomFormats[0].Specifications[0].Fields)
	require.NoError(t, err)
	assert.JSONEq(t, `[{"name":"value","value":"x265"}]`, string(raw))
}

func TestPVRQualityDefinitionsWriteUnlimitedAsZero(t *testing.T) {
	cfg, res := exportSonarr(t, Options{})
	d := func(s string) *commonv1alpha1.Decimal {
		v := commonv1alpha1.Decimal(s)
		return &v
	}
	assert.Equal(t, []commonv1alpha1.QualityDefinition{
		{Quality: "SDTV", MinSize: d("2"), MaxSize: d("100"), PreferredSize: d("95")},
		{Quality: "Bluray-1080p", Title: "Blu-ray 1080p", MinSize: d("17.1"), MaxSize: d("0"), PreferredSize: d("0")},
		{Quality: "DVD", MinSize: d("0"), MaxSize: d("0")},
		{Quality: "Bluray-2160p", MinSize: d("35"), MaxSize: d("0"), PreferredSize: d("0")},
	}, cfg.Spec.QualityDefinitions)
	assert.Contains(t, res.Warnings,
		`quality definition "DVD" prefers the smallest release, which the config can't express because 0 means unlimited there; set it in the app`)
}

func TestPVRQualityDefinitionsWithoutPreferredSizes(t *testing.T) {
	cfg, _ := exportSonarr(t, Options{}, map[string]string{"/qualitydefinition": `[
		{"id":1,"quality":{"id":2,"name":"EPUB"},"title":"EPUB","minSize":0,"maxSize":350},
		{"id":2,"quality":{"id":6,"name":"FLAC"},"title":"FLAC","minSize":0}]`})
	d := func(s string) *commonv1alpha1.Decimal {
		v := commonv1alpha1.Decimal(s)
		return &v
	}
	assert.Equal(t, []commonv1alpha1.QualityDefinition{
		{Quality: "EPUB", MinSize: d("0"), MaxSize: d("350")},
		{Quality: "FLAC", MinSize: d("0"), MaxSize: d("0")},
	}, cfg.Spec.QualityDefinitions)
}

func TestPVRDelayProfilesFollowTheAppsOrder(t *testing.T) {
	cfg, _ := exportSonarr(t, Options{})
	assert.Equal(t, []commonv1alpha1.DelayProfile{
		{
			TagLabels: []string{"anime"}, EnableUsenet: new(false), EnableTorrent: new(true), PreferredProtocol: "torrent",
			UsenetDelay: new(int32(0)), TorrentDelay: new(int32(60)), BypassIfHighestQuality: new(false),
			BypassIfAboveCustomFormatScore: new(true), MinimumCustomFormatScore: new(int32(100)),
		},
		{
			EnableUsenet: new(true), EnableTorrent: new(true), PreferredProtocol: "usenet",
			UsenetDelay: new(int32(0)), TorrentDelay: new(int32(0)), BypassIfHighestQuality: new(true),
			BypassIfAboveCustomFormatScore: new(false), MinimumCustomFormatScore: new(int32(0)),
		},
	}, cfg.Spec.DelayProfiles)
}

func TestPVRDelayProfilesReadProtocolItems(t *testing.T) {
	cfg, _ := exportSonarr(t, Options{}, map[string]string{"/delayprofile": `[{"id":1,"name":"Default","order":2147483647,"tags":[],
		"items":[{"name":"Torrent","protocol":"TorrentDownloadProtocol","allowed":true,"delay":15},
			{"name":"Usenet","protocol":"UsenetDownloadProtocol","allowed":false,"delay":0}],
		"bypassIfHighestQuality":true,"bypassIfAboveCustomFormatScore":false,"minimumCustomFormatScore":0}]`})
	assert.Equal(t, []commonv1alpha1.DelayProfile{{
		EnableUsenet: new(false), EnableTorrent: new(true), PreferredProtocol: "torrent", UsenetDelay: new(int32(0)), TorrentDelay: new(int32(15)),
		BypassIfHighestQuality: new(true), BypassIfAboveCustomFormatScore: new(false), MinimumCustomFormatScore: new(int32(0)),
	}}, cfg.Spec.DelayProfiles)
}

func TestPVRReleaseProfilesExportTheirIndexerAndTagsByName(t *testing.T) {
	cfg, _ := exportSonarr(t, Options{})
	require.Len(t, cfg.Spec.ReleaseProfiles, 1)
	rp := cfg.Spec.ReleaseProfiles[0]
	assert.Equal(t, "No x265", rp.Name)
	assert.Equal(t, []string{"x265"}, rp.Ignored)
	assert.Nil(t, rp.Required, "an empty term list is left out")
	assert.Equal(t, "Torznab", rp.IndexerName)
	assert.Equal(t, []string{"anime"}, rp.TagLabels)
}

func TestPVRQualityProfileExportsRadarrsLanguageByName(t *testing.T) {
	qp := qualityProfile(map[string]any{"name": "HD", "language": map[string]any{"id": float64(-2), "name": "Original"}}, nil)
	assert.Equal(t, "Original", qp["language"])
	assert.NotContains(t, qualityProfile(map[string]any{"name": "HD"}, nil), "language", "Sonarr's profiles have no language")
}

func exportReadarr(t *testing.T, development string) *pvrv1alpha1.Readarr {
	t.Helper()
	url := fakePVR(t, "/api/v1", map[string]string{
		"/config/metadataprovider": `{"id":1,"writeAudioTags":"no","scrubAudioTags":false,"writeBookTags":"newFiles",` +
			`"updateCovers":true,"embedMetadata":false}`,
		"/config/development": development,
		"/metadataprofile":    `[]`, "/rootfolder": `[]`, "/downloadclient": `[]`, "/indexer": `[]`,
		"/notification": `[]`, "/importlist": `[]`, "/releaseprofile": `[]`, "/qualityprofile": `[]`,
	})
	res, err := Run(context.Background(), Options{App: "readarr", URL: url, APIKey: fakeSonarrKey})
	require.NoError(t, err)
	cfg, ok := res.Config.(*pvrv1alpha1.Readarr)
	require.True(t, ok)
	return cfg
}

func TestPVRReadarrExportsItsMetadataSettingsAndSource(t *testing.T) {
	cfg := exportReadarr(t, `{"id":1,"metadataSource":"https://api.bookinfo.pro","consoleLogLevel":"","logSql":false,"logRotate":50}`)
	assert.Equal(t, &pvrv1alpha1.ReadarrMetadataProvider{
		WriteBookTags: "newFiles", UpdateCovers: new(true), EmbedMetadata: new(false), WriteAudioTags: "no", ScrubAudioTags: new(false),
	}, cfg.Spec.MetadataProvider)
	assert.Equal(t, pvrv1alpha1.ReadarrDevelopment{MetadataSource: "https://api.bookinfo.pro"}, cfg.Spec.Development)
}

func TestPVRReadarrLeavesOutDevelopmentOnItsOwnServer(t *testing.T) {
	cfg := exportReadarr(t, `{"id":1,"metadataSource":"","consoleLogLevel":"","logSql":false,"logRotate":50}`)
	assert.Zero(t, cfg.Spec.Development)
}

func TestPVRLidarrRootFolderExportsItsDefaults(t *testing.T) {
	e := &pvrExport{
		reader: &reader{tags: map[float64]string{7: "flac"}}, app: pvrclient.Lidarr,
		qualityProfiles: map[float64]string{2: "Lossless"}, metadataProfiles: map[float64]string{3: "Standard"},
	}
	rf := e.rootFolder(map[string]any{
		"path": "/music", "name": "Music", "defaultQualityProfileId": float64(2), "defaultMetadataProfileId": float64(3),
		"defaultMonitorOption": "future", "defaultNewItemMonitorOption": "new", "defaultTags": []any{float64(7)},
	}, nil)
	assert.Equal(t, map[string]any{
		"path": "/music", "name": "Music", "defaultQualityProfileName": "Lossless", "defaultMetadataProfileName": "Standard",
		"defaultMonitorOption": "future", "defaultNewItemMonitorOption": "new", "defaultTagLabels": []any{"flac"},
	}, rf)
}
