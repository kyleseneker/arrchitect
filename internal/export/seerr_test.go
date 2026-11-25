package export

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	requestsv1alpha1 "github.com/kyleseneker/arrchitect/api/requests/v1alpha1"
)

var fakeSeerrAPI = map[string]string{
	"/api/v1/settings/main": `{"applicationTitle":"Requests","applicationUrl":"","hideAvailable":true,"defaultPermissions":32,
		"mediaServerType":2,"apiKey":"seerr-key","newPlexLogin":false,"discoverRegion":"US","streamingRegion":"",
		"originalLanguage":"en|ja","blocklistLanguage":"","blocklistedTags":"210024,abc","blocklistedTagsLimit":"25",
		"defaultQuotas":{"movie":{"quotaLimit":5,"quotaDays":0},"tv":{}}}`,
	"/api/v1/settings/network":   `{"csrfProtection":false,"forceIpv4First":false,"trustProxy":true}`,
	"/api/v1/settings/metadatas": `{"tv":"tmdb","anime":"tvdb"}`,
	"/api/v1/settings/jobs": `[{"id":"jellyfin-full-scan","name":"Jellyfin Full Library Scan","type":"process",
		"interval":"hours","cronSchedule":"0 0 3 * * *","nextExecutionTime":"2026-11-25T03:00:00.000Z","running":false},
		{"id":"sonarr-scan","cronSchedule":"0 0 */6 * * *","running":false}]`,
	"/api/v1/settings/jellyfin":         `{"ip":"jellyfin","port":8096,"useSsl":false,"urlBase":"","name":"home"}`,
	"/api/v1/settings/jellyfin/library": `[{"id":"m","name":"Movies","enabled":true},{"id":"h","name":"Home Videos","enabled":false}]`,
	"/api/v1/settings/sonarr": `[
		{"id":0,"name":"Sonarr 4K","hostname":"sonarr4k","port":8989,"apiKey":"k4k","is4k":true,"isDefault":true},
		{"id":1,"name":"Sonarr","hostname":"sonarr","port":8989,"apiKey":"sonarr-key","is4k":false,"isDefault":true,
			"activeProfileId":4,"activeProfileName":"HD-1080p","activeDirectory":"/tv","enableSeasonFolders":true}]`,
	"/api/v1/settings/radarr":                 `[]`,
	"/api/v1/settings/notifications/discord":  `{"enabled":true,"types":4094,"options":{"webhookUrl":"https://hook","enableMentions":true,"botUsername":""}}`,
	"/api/v1/settings/notifications/webhook":  `{"enabled":false,"types":0,"options":{}}`,
	"/api/v1/settings/notifications/telegram": `{"enabled":false}`,
}

func exportFakeSeerr(t *testing.T) (Result, *requestsv1alpha1.Seerr) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := fakeSeerrAPI[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	res, err := Run(context.Background(), Options{App: "seerr", URL: srv.URL, APIKey: "seerr-key"})
	require.NoError(t, err)
	cfg, ok := res.Config.(*requestsv1alpha1.Seerr)
	require.True(t, ok)
	return res, cfg
}

func TestSeerrExportsSettingsServicesAndNotifications(t *testing.T) {
	res, cfg := exportFakeSeerr(t)
	assert.Equal(t, "Requests", cfg.Spec.Main.ApplicationTitle)
	assert.Equal(t, int32(32), *cfg.Spec.Main.DefaultPermissions)

	require.NotZero(t, cfg.Spec.JellyfinAuth)
	assert.Equal(t, "jellyfin", cfg.Spec.JellyfinAuth.Hostname)
	assert.Equal(t, int32(8096), *cfg.Spec.JellyfinAuth.Port)
	assert.ElementsMatch(t, []string{"jellyfin-username", "jellyfin-password"}, res.Unfilled)
	assert.Equal(t, []string{"Movies"}, cfg.Spec.Libraries, "only the enabled libraries are exported")
	require.NotNil(t, cfg.Spec.Network.TrustProxy)
	assert.True(t, *cfg.Spec.Network.TrustProxy)
	assert.Equal(t, requestsv1alpha1.SeerrMetadataProviders{TV: "tmdb", Anime: "tvdb"}, cfg.Spec.MetadataProviders)
	assert.Equal(t, []requestsv1alpha1.SeerrJobSchedule{
		{ID: "jellyfin-full-scan", Schedule: "0 0 3 * * *"}, {ID: "sonarr-scan", Schedule: "0 0 */6 * * *"},
	}, cfg.Spec.JobSchedules)

	require.Len(t, cfg.Spec.Sonarr, 2, "every server is exported")
	fourK, sonarr := cfg.Spec.Sonarr[0], cfg.Spec.Sonarr[1]
	assert.Equal(t, "Sonarr 4K", fourK.Name)
	assert.True(t, *fourK.Is4k)
	assert.Equal(t, "k4k", res.Secret.StringData[fourK.APIKeySecretRef.Key])
	assert.Equal(t, "HD-1080p", sonarr.ActiveProfileName)
	assert.Nil(t, sonarr.ActiveProfileId, "the name is portable; the id is not")
	assert.Equal(t, "sonarr-key", res.Secret.StringData[sonarr.APIKeySecretRef.Key])
	assert.NotEqual(t, fourK.APIKeySecretRef.Key, sonarr.APIKeySecretRef.Key)
	assert.Nil(t, cfg.Spec.Radarr)

	require.Len(t, cfg.Spec.Notifications, 1)
	discord := cfg.Spec.Notifications[0]
	assert.Equal(t, "discord", discord.Agent)
	assert.Equal(t, []string{
		"issueComment", "issueCreated", "issueReopened", "issueResolved", "mediaApproved", "mediaAutoApproved",
		"mediaAvailable", "mediaDeclined", "mediaFailed", "mediaPending",
	}, discord.Types)
	assert.Contains(t, res.Warnings, `notification "discord": the config can't express type bits 32, and applying it would clear them`)
	require.Len(t, discord.Options, 2)
	assert.Equal(t, true, fieldNamed(t, discord.Options, "enableMentions").Value.ToInterface(), "a boolean option stays a boolean")
	webhook := fieldNamed(t, discord.Options, "webhookUrl")
	require.NotZero(t, webhook.ValueFrom)
	assert.Equal(t, "https://hook", res.Secret.StringData[webhook.ValueFrom.SecretKeyRef.Key])
}

// Seerr stores languages and tags as joined strings, and its web UI saves the
// tag page limit as a string.
func TestSeerrExportsMainListsQuotasAndStringNumbers(t *testing.T) {
	res, cfg := exportFakeSeerr(t)
	main := cfg.Spec.Main
	require.NotNil(t, main)
	assert.Empty(t, main.ApplicationUrl)
	assert.Equal(t, "US", main.DiscoverRegion)
	assert.Empty(t, main.StreamingRegion)
	assert.Equal(t, []string{"en", "ja"}, main.OriginalLanguages)
	assert.Empty(t, main.BlocklistLanguages)
	assert.Equal(t, []int32{210024}, main.BlocklistedTags)
	assert.Equal(t, int32(25), main.BlocklistedTagsLimit)
	require.NotNil(t, main.NewMediaServerLogin)
	assert.False(t, *main.NewMediaServerLogin)
	require.NotNil(t, main.DefaultQuotas.Movie.Limit)
	assert.Equal(t, int32(5), *main.DefaultQuotas.Movie.Limit)
	require.NotNil(t, main.DefaultQuotas.Movie.Days)
	assert.Equal(t, int32(0), *main.DefaultQuotas.Movie.Days)
	assert.Zero(t, main.DefaultQuotas.TV, "Seerr has no TV quota stored")
	assert.Contains(t, res.Warnings, `blocklisted tag "abc" isn't a TMDB keyword ID, so it isn't exported`)
}

func TestIsCredentialOption(t *testing.T) {
	for _, name := range []string{"botAPI", "accessToken", "authPass", "webhookUrl", "pgpPrivateKey"} {
		assert.True(t, isCredentialOption(name), name)
	}
	for _, name := range []string{"chatId", "botUsername", "enableMentions", "smtpHost"} {
		assert.False(t, isCredentialOption(name), name)
	}
}
