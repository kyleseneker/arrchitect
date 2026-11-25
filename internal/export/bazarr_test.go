package export

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	subtitlesv1alpha1 "github.com/kyleseneker/arrchitect/api/subtitles/v1alpha1"
)

const fakeBazarrSettings = `{
	"general": {"use_sonarr": true, "use_radarr": false, "minimum_score": 90, "hi_extension": "hi",
		"enabled_providers": ["opensubtitlescom", "podnapisi"], "enabled_languages": ["fr"],
		"path_mappings": [["/tv", "/media/tv"]], "path_mappings_movie": [], "chmod": "0640",
		"serie_default_enabled": true, "serie_default_profile": 2, "movie_default_enabled": false, "movie_default_profile": 1,
		"adaptive_searching_delay": "2w", "adaptive_searching_delta": "3d", "days_to_upgrade_subs": 14,
		"serie_tag_enabled": true, "remove_profile_tags": ["no-subs"], "wanted_search_frequency_movie": 24,
		"use_jellyfin": true},
	"sonarr": {"ip": "sonarr", "port": 8989, "base_url": "/", "apikey": "sonarr-key", "series_sync": 60, "ssl": false,
		"excluded_tags": ["no-subs"], "excluded_series_types": []},
	"radarr": {"ip": "radarr", "apikey": "radarr-key"},
	"subsync": {"use_subsync": true, "subsync_threshold": 90},
	"jellyfin": {"url": "http://jellyfin:8096", "apikey": "jf-key", "movie_library": ["Movies"], "movie_library_ids": ["m1"],
		"series_library": ["TV Shows"], "series_library_ids": ["t1"], "update_movie_library": true,
		"update_series_library": false, "refresh_method": "immediate"},
	"opensubtitlescom": {"username": "me", "password": "hunter2", "use_hash": true, "vip": false, "extra": {"nested": 1}},
	"notifications": {"providers": [{"name": "Discord", "enabled": true, "url": "discord://id/token"},
		{"name": "Telegram", "enabled": false, "url": null}]}
}`

const fakeBazarrProfiles = `[
	{"profileId": 1, "name": "English", "cutoff": 65535, "mustContain": [], "mustNotContain": [], "originalFormat": 0,
		"tag": null, "items": [{"id": 1, "language": "en", "hi": "False", "forced": "True", "audio_exclude": "False"}]},
	{"profileId": 2, "name": "Picky", "cutoff": 3, "mustContain": ["x"], "mustNotContain": ["CAM"], "originalFormat": 1,
		"tag": "spanish", "items": [{"id": 3, "language": "es", "hi": "False", "forced": "False", "audio_exclude": "False",
		"audio_only_include": "True"}]}
]`

func exportBazarrFrom(t *testing.T) (*subtitlesv1alpha1.Bazarr, Result) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-KEY") != "k" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/api/system/settings":
			_, _ = w.Write([]byte(fakeBazarrSettings))
		case "/api/system/languages/profiles":
			_, _ = w.Write([]byte(fakeBazarrProfiles))
		case "/api/system/languages":
			_, _ = w.Write([]byte(`[{"code2":"en","enabled":true},{"code2":"es","enabled":true},{"code2":"fr","enabled":false}]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	res, err := Run(context.Background(), Options{App: "bazarr", URL: srv.URL, APIKey: "k"})
	require.NoError(t, err)
	cfg, ok := res.Config.(*subtitlesv1alpha1.Bazarr)
	require.True(t, ok)
	return cfg, res
}

func TestBazarrSettingsMapBackToSpecFields(t *testing.T) {
	cfg, res := exportBazarrFrom(t)
	assert.Equal(t, int32(90), *cfg.Spec.General.MinimumScore)
	assert.Equal(t, "hi", cfg.Spec.General.HiExtension)
	assert.True(t, *cfg.Spec.SubSync.UseSubsync)

	require.NotZero(t, cfg.Spec.SonarrConnection)
	assert.Equal(t, "sonarr", cfg.Spec.SonarrConnection.Host)
	assert.Equal(t, int32(60), *cfg.Spec.SonarrConnection.SyncInterval)
	assert.Equal(t, "sonarr-api-key", cfg.Spec.SonarrConnection.APIKeySecretRef.Key)
	assert.Equal(t, "sonarr-key", res.Secret.StringData["sonarr-api-key"])
	assert.Zero(t, cfg.Spec.RadarrConnection, "Bazarr does not use Radarr")
	assert.Equal(t, []string{"en", "es"}, cfg.Spec.Languages.Enabled)

	assert.Equal(t, []subtitlesv1alpha1.BazarrPathMapping{{RemotePath: "/tv", LocalPath: "/media/tv"}}, cfg.Spec.General.PathMappings)
	assert.Empty(t, cfg.Spec.General.PathMappingsMovie)
	assert.Equal(t, "0640", cfg.Spec.General.Chmod)
	assert.Equal(t, "Picky", cfg.Spec.General.SeriesDefaultProfile, "the profile id is exported by name")
	assert.Empty(t, cfg.Spec.General.MovieDefaultProfile, "a default that is off isn't exported")
	assert.Equal(t, []string{"no-subs"}, cfg.Spec.SonarrConnection.ExcludedTags)
	general := cfg.Spec.General
	assert.Equal(t, "2w", general.AdaptiveSearchingDelay)
	assert.Equal(t, "3d", general.AdaptiveSearchingDelta)
	assert.Equal(t, int32(14), *general.DaysToUpgradeSubs)
	assert.True(t, *general.SeriesTagEnabled)
	assert.Equal(t, []string{"no-subs"}, general.RemoveProfileTags)
	assert.Equal(t, int32(24), *general.WantedSearchFrequencyMovie)
	assert.Empty(t, cfg.Spec.SonarrConnection.ExcludedSeriesTypes)
}

func TestBazarrProviderSecretsMoveToTheSecret(t *testing.T) {
	cfg, res := exportBazarrFrom(t)
	require.Len(t, cfg.Spec.Providers, 2)
	os := cfg.Spec.Providers[0]
	assert.Equal(t, "opensubtitlescom", os.Name)
	text := func(s string) *commonv1alpha1.FieldValue {
		return &commonv1alpha1.FieldValue{Raw: []byte(`"` + s + `"`)}
	}
	assert.Equal(t, []commonv1alpha1.ConfigField{
		{Name: "password", ValueFrom: commonv1alpha1.ValueSource{
			SecretKeyRef: commonv1alpha1.SecretKeyRef{Name: "bazarr-credentials", Key: "provider-opensubtitlescom-password"},
		}},
		{Name: "use_hash", Value: text("true")},
		{Name: "username", Value: text("me")},
		{Name: "vip", Value: text("false")},
	}, os.Settings)
	assert.Equal(t, "hunter2", res.Secret.StringData["provider-opensubtitlescom-password"])
	assert.Contains(t, res.Warnings, `provider "opensubtitlescom": the config's provider settings hold text, so extra stays as set in Bazarr`)
	assert.Empty(t, cfg.Spec.Providers[1].Settings)
}

func TestBazarrExportsJellyfinWithTheLibrariesItRefreshes(t *testing.T) {
	cfg, res := exportBazarrFrom(t)
	require.NotNil(t, cfg.Spec.Jellyfin)
	assert.Equal(t, subtitlesv1alpha1.BazarrJellyfin{
		URL:             "http://jellyfin:8096",
		APIKeySecretRef: &commonv1alpha1.SecretKeyRef{Name: "bazarr-credentials", Key: "jellyfin-api-key"},
		MovieLibraries:  []string{"Movies"},
		RefreshMethod:   "immediate",
	}, *cfg.Spec.Jellyfin, "series libraries Bazarr doesn't refresh are left out")
	assert.Equal(t, "jf-key", res.Secret.StringData["jellyfin-api-key"])
}

func TestBazarrExportsProfileCutoffFiltersAndTag(t *testing.T) {
	cfg, res := exportBazarrFrom(t)
	require.Len(t, cfg.Spec.Languages.Profiles, 2)
	english := cfg.Spec.Languages.Profiles[0]
	assert.Equal(t, int32(1), *english.ProfileID)
	assert.True(t, *english.Items[0].Forced)
	assert.False(t, *english.Items[0].HI)
	assert.Equal(t, "any", english.Cutoff)
	assert.Nil(t, english.OriginalFormat, "originalFormat off is Bazarr's default")
	assert.Empty(t, english.Tag)

	picky := cfg.Spec.Languages.Profiles[1]
	assert.Equal(t, "es", picky.Cutoff, "the cutoff is named, so renumbering the items keeps it")
	assert.Equal(t, []string{"x"}, picky.MustContain)
	assert.Equal(t, []string{"CAM"}, picky.MustNotContain)
	assert.True(t, *picky.OriginalFormat)
	assert.Equal(t, "spanish", picky.Tag)
	assert.True(t, *picky.Items[0].AudioOnlyInclude)
	assert.Contains(t, res.Warnings, `language profile "Picky": Bazarr numbers its items differently than the config, `+
		`so applying the config would renumber them`)
}

func TestBazarrCutoffExportsOnlyWhatTheConfigCanName(t *testing.T) {
	items := []map[string]any{
		{"id": float64(1), "language": "en", "hi": "False", "forced": "False"},
		{"id": float64(2), "language": "en", "hi": "True", "forced": "False"},
		{"id": float64(3), "language": "en", "hi": "False", "forced": "False"},
	}
	for _, tt := range []struct {
		cutoff  any
		want    string
		ok      bool
		warning string
	}{
		{nil, "", false, ""},
		{float64(2), "en:hi", true, ""},
		{float64(1), "en", true, ""},
		{float64(3), "", false, `language profile "p": its cutoff is a later of two en items, which the config can't name, ` +
			`so cutoff is left out`},
		{float64(9), "", false, `language profile "p": its cutoff is an item it doesn't have, so cutoff is left out`},
	} {
		r := &reader{}
		got, ok := r.bazarrCutoff("p", tt.cutoff, items)
		assert.Equal(t, tt.want, got, "%v", tt.cutoff)
		assert.Equal(t, tt.ok, ok, "%v", tt.cutoff)
		if tt.warning != "" {
			assert.Equal(t, []string{tt.warning}, r.warnings)
		} else {
			assert.Empty(t, r.warnings)
		}
	}
}

func TestBazarrExportsConfiguredNotificationProvidersWithTheirURLInTheSecret(t *testing.T) {
	cfg, res := exportBazarrFrom(t)
	require.Len(t, cfg.Spec.Notifications, 1, "a provider that's off and has no URL is Bazarr's default, not a setting")
	n := cfg.Spec.Notifications[0]
	assert.Equal(t, "Discord", n.Name)
	assert.True(t, *n.Enabled)
	require.NotNil(t, n.URLSecretRef)
	assert.Equal(t, "discord://id/token", res.Secret.StringData[n.URLSecretRef.Key])
}
