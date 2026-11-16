package export

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	indexersv1alpha1 "github.com/kyleseneker/arrchitect/api/indexers/v1alpha1"
)

var fakeProwlarrAPI = map[string]string{
	"/tag":        `[{"id":1,"label":"private"}]`,
	"/appprofile": `[{"id":1,"name":"Standard","enableRss":true,"enableAutomaticSearch":true,"enableInteractiveSearch":true,"minimumSeeders":1}]`,
	"/applications": `[{"id":1,"name":"Sonarr","syncLevel":"fullSync","implementation":"Sonarr","configContract":"SonarrSettings","tags":[],
		"fields":[{"name":"prowlarrUrl","value":"http://prowlarr:9696"},{"name":"baseUrl","value":"http://sonarr:8989"},
		{"name":"apiKey","value":"********"},{"name":"syncCategories","value":[5000,5040]},{"name":"animeSyncCategories","value":[5070]}]}]`,
	"/applications/schema": `[{"implementation":"Sonarr","fields":[{"name":"animeSyncCategories","value":[]}]}]`,
	"/indexer": `[{"id":1,"name":"Tracker","implementation":"Cardigann","configContract":"CardigannSettings","enable":true,
		"priority":25,"appProfileId":1,"tags":[1],"fields":[{"name":"definitionFile","value":"tracker"},
		{"name":"baseSettings.queryLimit","value":null},{"name":"torrentBaseSettings.seedRatio","value":1.5},
		{"name":"freeleech","value":true},{"name":"apiKey","value":"********"},{"name":"sort","value":[1,2]},{"name":"empty","value":""}]}]`,
	"/indexerproxy": `[{"id":1,"name":"FlareSolverr","implementation":"FlareSolverr","configContract":"FlareSolverrSettings","tags":[1],
		"fields":[{"name":"host","value":"http://flaresolverr:8191/"},{"name":"requestTimeout","value":60}]},
		{"id":2,"name":"Socks","implementation":"Socks5","configContract":"Socks5Settings","tags":[],
		"fields":[{"name":"host","value":"socks"},{"name":"username","value":"me"}]}]`,
	"/downloadclient": `[{"id":1,"name":"qBit","enable":true,"protocol":"torrent","implementation":"QBittorrent",
		"configContract":"QBittorrentSettings","priority":1,"categories":[{"clientCategory":"tv","categories":[5000]}],"tags":[],
		"fields":[{"name":"host","value":"qbit"},{"name":"port","value":8080},{"name":"initialState","value":0}]}]`,
	"/downloadclient/schema": `[{"implementation":"QBittorrent","fields":[{"name":"initialState","value":0}]}]`,
	"/notification":          `[]`,
	"/notification/schema":   `[]`,
}

func exportProwlarrFrom(t *testing.T) (*indexersv1alpha1.Prowlarr, Result) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := fakeProwlarrAPI[r.URL.Path[len("/api/v1"):]]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	res, err := Run(context.Background(), Options{App: "prowlarr", URL: srv.URL, APIKey: "k"})
	require.NoError(t, err)
	cfg, ok := res.Config.(*indexersv1alpha1.Prowlarr)
	require.True(t, ok)
	return cfg, res
}

func TestProwlarrIndexerFieldsKeepTheirTypes(t *testing.T) {
	cfg, _ := exportProwlarrFrom(t)
	require.Len(t, cfg.Spec.Indexers, 1)
	idx := cfg.Spec.Indexers[0]
	assert.Equal(t, "Standard", idx.AppProfileName)
	assert.Equal(t, []string{"private"}, idx.TagLabels)
	values := map[string]any{}
	var apiKey *commonv1alpha1.ValueSource
	for _, f := range idx.Fields {
		if f.Value != nil {
			values[f.Name] = f.Value.ToInterface()
		}
		if f.Name == "apiKey" {
			apiKey = &f.ValueFrom
		}
	}
	assert.Equal(t, map[string]any{
		"definitionFile": "tracker", "torrentBaseSettings.seedRatio": 1.5, "freeleech": true, "sort": []any{float64(1), float64(2)},
	}, values)
	require.NotNil(t, apiKey)
	assert.Equal(t, "indexer-tracker-apikey", apiKey.SecretKeyRef.Key)
}

func TestProwlarrApplicationReferencesItsKeyAndWarnsAboutUnmodelledFields(t *testing.T) {
	cfg, res := exportProwlarrFrom(t)
	require.Len(t, cfg.Spec.Applications, 1)
	a := cfg.Spec.Applications[0]
	assert.Equal(t, "http://sonarr:8989", a.BaseUrl)
	assert.Equal(t, []int32{5000, 5040}, a.SyncCategories)
	assert.Equal(t, "application-sonarr-api-key", a.APIKeySecretRef.Key)
	assert.Contains(t, res.Unfilled, "application-sonarr-api-key")
	assert.Contains(t, res.Warnings, `application "Sonarr": the config has no animeSyncCategories, so it stays as set in Prowlarr`)
}

func TestProwlarrProxyKeepsHostAndTimeoutOnly(t *testing.T) {
	cfg, res := exportProwlarrFrom(t)
	require.Len(t, cfg.Spec.Proxies, 2)
	assert.Equal(t, "http://flaresolverr:8191/", cfg.Spec.Proxies[0].Host)
	assert.Equal(t, int32(60), *cfg.Spec.Proxies[0].RequestTimeout)
	assert.Contains(t, res.Warnings,
		`proxy "Socks": the config's proxies have only host and requestTimeout, so username stays as set in Prowlarr`)
}

func TestProwlarrDownloadClientOmitsDefaultFields(t *testing.T) {
	cfg, _ := exportProwlarrFrom(t)
	require.Len(t, cfg.Spec.DownloadClients, 1)
	dc := cfg.Spec.DownloadClients[0]
	assert.Equal(t, []indexersv1alpha1.ProwlarrDownloadClientCategory{{ClientCategory: "tv", Categories: []int32{5000}}}, dc.Categories)
	names := make([]string, 0, len(dc.Fields))
	for _, f := range dc.Fields {
		names = append(names, f.Name)
	}
	assert.Equal(t, []string{"host", "port"}, names)
}
