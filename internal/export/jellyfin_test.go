package export

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	mediaserversv1alpha1 "github.com/kyleseneker/arrchitect/api/mediaservers/v1alpha1"
)

func exportFakeJellyfin(t *testing.T, libraries string) (Result, *mediaserversv1alpha1.Jellyfin) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/Users/AuthenticateByName":
			_, _ = w.Write([]byte(`{"AccessToken":"token"}`))
		case "/System/Configuration":
			_, _ = w.Write([]byte(`{"ServerName":"home","LogFileRetentionDays":3,"NotInTheCRD":true,"EnableMetrics":true,
				"TrickplayOptions":{"EnableHwAcceleration":true,"Interval":10000,"WidthResolutions":[320],"NotInTheCRD":1}}`))
		case "/System/Configuration/encoding":
			_, _ = w.Write([]byte(`{"HardwareAccelerationType":"vaapi","H264Crf":23,"HardwareDecodingCodecs":["h264","hevc"],
				"TonemappingPeak":100,"DownMixAudioBoost":2.5}`))
		case "/System/Configuration/network":
			_, _ = w.Write([]byte(`{"BaseUrl":"/jf","InternalHttpPort":8096,"KnownProxies":["10.0.0.1"],"EnableRemoteAccess":true}`))
		case "/System/Configuration/branding":
			_, _ = w.Write([]byte(`{"LoginDisclaimer":"Members only","CustomCss":"","SplashscreenEnabled":true,` +
				`"SplashscreenLocation":"/config/splashscreen.png"}`))
		case "/Library/VirtualFolders":
			_, _ = w.Write([]byte(libraries))
		case "/ScheduledTasks":
			_, _ = w.Write([]byte(`[
				{"Id":"a1","Key":"RefreshLibrary","IsHidden":false,"Triggers":[{"Type":"IntervalTrigger","IntervalTicks":432000000000}]},
				{"Id":"b2","Key":"RefreshChapterImages","IsHidden":false,"Triggers":[
					{"Type":"WeeklyTrigger","TimeOfDayTicks":126000000000,"DayOfWeek":"Sunday","MaxRuntimeTicks":144000000000}]},
				{"Id":"c3","Key":"CleanCache","IsHidden":false,"Triggers":[{"Type":"IntervalTrigger","IntervalTicks":15000000}]},
				{"Id":"d4","Key":"Hidden","IsHidden":true,"Triggers":[{"Type":"StartupTrigger"}]},
				{"Id":"e5","Key":"NeverRuns","IsHidden":false,"Triggers":[]}]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	res, err := Run(context.Background(), Options{App: "jellyfin", URL: srv.URL, Username: "admin", Password: "pw"})
	require.NoError(t, err)
	cfg, ok := res.Config.(*mediaserversv1alpha1.Jellyfin)
	require.True(t, ok)
	return res, cfg
}

func TestJellyfinExportsSettingsLibrariesAndTheAdminUser(t *testing.T) {
	res, cfg := exportFakeJellyfin(t, `[
		{"Name":"Movies","CollectionType":"movies","Locations":["/media/movies"],
			"LibraryOptions":{"EnableRealtimeMonitor":true,"AutomaticRefreshIntervalDays":30}},
		{"Name":"Everything","Locations":["/media/other"],"LibraryOptions":{}}]`)
	assert.Equal(t, "home", cfg.Spec.Server.ServerName)
	assert.Equal(t, int32(3), *cfg.Spec.Server.LogFileRetentionDays)
	assert.Equal(t, "vaapi", cfg.Spec.Encoding.HardwareAccelerationType)
	assert.Equal(t, []string{"h264", "hevc"}, cfg.Spec.Encoding.HardwareDecodingCodecs)
	assert.Equal(t, "100", string(*cfg.Spec.Encoding.TonemappingPeak))
	assert.Equal(t, "2.5", string(*cfg.Spec.Encoding.DownMixAudioBoost))
	assert.True(t, *cfg.Spec.Server.EnableMetrics)
	assert.Equal(t, &mediaserversv1alpha1.JellyfinTrickplay{EnableHwAcceleration: new(true), Interval: int32(10000), WidthResolutions: []int32{320}},
		cfg.Spec.Server.TrickplayOptions)
	assert.Equal(t, &mediaserversv1alpha1.JellyfinNetwork{KnownProxies: []string{"10.0.0.1"}, EnableRemoteAccess: new(true)},
		cfg.Spec.Network, "the base URL and ports aren't in the config")
	assert.Equal(t, &mediaserversv1alpha1.JellyfinBranding{
		LoginDisclaimer: new("Members only"), CustomCss: new(""), SplashscreenEnabled: new(true),
	}, cfg.Spec.Branding, "the uploaded splash screen's location isn't in the config")
	assert.Equal(t, []mediaserversv1alpha1.JellyfinScheduledTask{
		{Key: "RefreshLibrary", Triggers: []mediaserversv1alpha1.JellyfinTaskTrigger{{Type: "IntervalTrigger", IntervalMinutes: 720}}},
		{Key: "RefreshChapterImages", Triggers: []mediaserversv1alpha1.JellyfinTaskTrigger{
			{Type: "WeeklyTrigger", TimeOfDay: "03:30", DayOfWeek: "Sunday", MaxRuntimeHours: 4}}},
		{Key: "NeverRuns"},
	}, cfg.Spec.ScheduledTasks)
	assert.Contains(t, res.Warnings, "scheduled task CleanCache has a time the config can't express, so it stays as set in Jellyfin")
	require.Len(t, cfg.Spec.Libraries, 2)
	assert.Equal(t, []string{"/media/movies"}, cfg.Spec.Libraries[0].Paths)
	assert.True(t, *cfg.Spec.Libraries[0].EnableRealtimeMonitor)
	assert.Equal(t, int32(30), *cfg.Spec.Libraries[0].AutomaticRefreshIntervalDays)
	assert.Equal(t, "mixed", cfg.Spec.Libraries[1].CollectionType)
	assert.Equal(t, "username", cfg.Spec.AdminUser.UsernameSecretRef.Key)
	assert.Equal(t, map[string]string{"username": "admin", "password": "pw"}, res.Secret.StringData)
	assert.Nil(t, cfg.Spec.Connection.APIKeySecretRef)
}

func TestJellyfinExportsWithAnAPIKeyInsteadOfSigningIn(t *testing.T) {
	var tokens []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokens = append(tokens, r.Header.Get("Authorization"))
		switch r.URL.Path {
		case "/Users/AuthenticateByName":
			t.Error("export signed in although it had an API key")
		case "/Library/VirtualFolders", "/ScheduledTasks":
			_, _ = w.Write([]byte(`[]`))
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	t.Cleanup(srv.Close)

	res, err := Run(context.Background(), Options{App: "jellyfin", URL: srv.URL, APIKey: "key"})
	require.NoError(t, err)
	cfg, ok := res.Config.(*mediaserversv1alpha1.Jellyfin)
	require.True(t, ok)
	require.NotNil(t, cfg.Spec.Connection.APIKeySecretRef)
	assert.Equal(t, map[string]string{"api-key": "key"}, res.Secret.StringData)
	assert.Equal(t, apiKeyKey, cfg.Spec.Connection.APIKeySecretRef.Key)
	assert.ElementsMatch(t, []string{"username", "password"}, res.Unfilled, "the admin user still has to be filled in")
	require.NotEmpty(t, tokens)
	for _, token := range tokens {
		assert.Contains(t, token, `Token="key"`)
	}
}

// Jellyfin keeps the fetchers a library disables and those a type option
// enables, each beside an order; the config lists the ones used, in order.
func TestJellyfinExportsFetchersAsTheOnesUsedInOrder(t *testing.T) {
	res, cfg := exportFakeJellyfin(t, `[{"Name":"Shows","CollectionType":"tvshows","Locations":["/media/tv"],"LibraryOptions":{
		"SubtitleFetcherOrder":["Podnapisi","Open Subtitles"],"DisabledSubtitleFetchers":["Podnapisi"],
		"DisabledLyricFetchers":["LrcLib"],"LocalMetadataReaderOrder":null,"DisabledLocalMetadataReaders":[],
		"CustomTagDelimiters":["/","|"],"DelimiterWhitelist":[],
		"TypeOptions":[
			{"Type":"Series","MetadataFetchers":["TheMovieDb","TheTVDB"],"MetadataFetcherOrder":["TheTVDB","Other","TheMovieDb"],
				"ImageFetchers":["TheMovieDb"],"ImageFetcherOrder":[],"ImageOptions":[]},
			{"Type":"Season","MetadataFetchers":[],"MetadataFetcherOrder":["TheMovieDb"],"ImageFetchers":[],"ImageFetcherOrder":[]},
			{"Type":"Movie","MetadataFetchers":["TheMovieDb"],"ImageFetchers":[]}]}}]`)
	require.Len(t, cfg.Spec.Libraries, 1)
	lib := cfg.Spec.Libraries[0]
	assert.Equal(t, []string{"Open Subtitles"}, lib.SubtitleFetchers)
	assert.Empty(t, lib.LyricFetchers)
	assert.Empty(t, lib.LocalMetadataReaders, "Jellyfin uses its own order when the library has none")
	assert.Equal(t, []string{"/", "|"}, lib.CustomTagDelimiters)
	assert.Equal(t, []mediaserversv1alpha1.JellyfinTypeOptions{
		{Type: "Series", MetadataFetchers: []string{"TheTVDB", "TheMovieDb"}, ImageFetchers: []string{"TheMovieDb"}},
	}, lib.TypeOptions)
	assert.Contains(t, res.Warnings,
		`library "Shows": the config can't express its lyricFetchers without an order that enables one, so they aren't exported`)
	assert.Contains(t, res.Warnings,
		`library "Shows": the config can't express Season with every metadata fetcher disabled, so they aren't exported`)
	assert.Contains(t, res.Warnings,
		`library "Shows": the config can't express options for Movie, which its collection type doesn't hold, so they aren't exported`)
}
