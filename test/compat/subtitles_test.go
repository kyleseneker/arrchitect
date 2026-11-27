//go:build compat

package compat

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	subtitlesv1alpha1 "github.com/kyleseneker/arrchitect/api/subtitles/v1alpha1"
	ctrlcommon "github.com/kyleseneker/arrchitect/internal/controller/common"
	"github.com/kyleseneker/arrchitect/internal/controller/subtitles"
	"github.com/kyleseneker/arrchitect/internal/engine"
)

func TestBazarr(t *testing.T) {
	t.Parallel()
	bazarr := startContainer(t, containerSpec{name: "bazarr", image: bazarrImage, port: 6767, env: linuxserverEnv})
	sonarr, sonarrKey := startPVR(t, "sonarr", sonarrImage, 8989, "v3")
	radarr, radarrKey := startPVR(t, "radarr", radarrImage, 7878, "v3")
	fakes := startFakes(t)

	var key string
	waitFor(t, "bazarr api key", 3*time.Minute, func() error {
		out, err := bazarr.run(`sed -n '/^auth:/,/^[^ ]/p' /config/config/config.yaml | sed -n 's/^  apikey: *//p'`)
		if err != nil || out == "" {
			return fmt.Errorf("apikey not written yet: %v", err)
		}
		key = out
		return nil
	})
	hc := bazarrClient(t, bazarr.url(), key)
	waitFor(t, "bazarr", 3*time.Minute, func() error {
		_, err := hc.GetJSON(context.Background(), "/api/system/settings")
		return err
	})
	_, err := hc.DoForm(context.Background(), "/api/system/settings", url.Values{"languages-profiles": {
		`[{"profileId":99,"name":"Hand-made","cutoff":null,"mustContain":[],"mustNotContain":[],"originalFormat":false,` +
			`"items":[{"id":1,"language":"en","hi":"True","forced":"False","audio_exclude":"False",` +
			`"audio_only_include":"False"}]}]`,
	}})
	require.NoError(t, err, "creating a hand-made language profile")
	proxy := newRecordingProxy(t, bazarr.url())

	checkCompat(t, compatCase{
		fixture: "bazarr.yaml",
		update: `[
			{"op": "replace", "path": "/spec/general/minimumScore", "value": 85},
			{"op": "replace", "path": "/spec/notifications/0/enabled", "value": false},
			{"op": "replace", "path": "/spec/languages/profiles/0/cutoff", "value": "es"}
		]`,
		verifyUpdate: func(t *testing.T) {
			assert.Equal(t, float64(2), bazarrProfile(t, hc, "English")["cutoff"])
		},
		export: "bazarr",
		// Bazarr's Jellyfin integration arrived in 1.6.0.
		newerFields: []string{"/spec/jellyfin"},
		vars: map[string]string{
			"URL": proxy.url(), "HOST": hostIP, "FAKES": fakes,
			"SONARR_PORT": strconv.Itoa(sonarr.port), "RADARR_PORT": strconv.Itoa(radarr.port),
		},
		secrets: map[string]string{
			"api-key": key, "sonarr-api-key": sonarrKey, "radarr-api-key": radarrKey, "discord-url": "discord://compat/token",
			"jellyfin-api-key": "compat-jellyfin-key",
		},
		newObj: func() ctrlcommon.ConfigResource { return &subtitlesv1alpha1.Bazarr{} },
		reconciler: func(c client.Client) reconcile.Reconciler {
			return &subtitles.BazarrReconciler{Client: c, Recorder: events.NewFakeRecorder(100)}
		},
		proxy:   proxy,
		observe: true,
		verify: func(t *testing.T) {
			settings, err := hc.GetJSON(context.Background(), "/api/system/settings")
			require.NoError(t, err)
			general, _ := settings["general"].(map[string]any)
			assert.Equal(t, true, general["use_sonarr"])
			assert.Equal(t, float64(90), general["minimum_score"])
			sonarrSettings, _ := settings["sonarr"].(map[string]any)
			assert.Equal(t, hostIP, sonarrSettings["ip"])
			assert.Contains(t, general["enabled_providers"], "podnapisi")
			assert.Equal(t, []any{[]any{"/data/media/tv", "/media/tv"}, []any{"/anime", "/media/anime"}},
				general["path_mappings"])
			assert.Equal(t, true, general["serie_default_enabled"])
			assert.Equal(t, float64(1), general["serie_default_profile"], "the declared profile's id")
			assert.Equal(t, float64(99), general["movie_default_profile"], "a profile made in Bazarr resolves by name too")
			assert.Equal(t, "0644", general["chmod"])
			assert.Equal(t, []any{"nosubs"}, sonarrSettings["excluded_tags"])
			assert.Equal(t, []any{"daily"}, sonarrSettings["excluded_series_types"])
			assert.Equal(t, []string{"English", "Hand-made"}, bazarrProfileNames(t, hc),
				"saving the declared profile lost the hand-made one")
			assert.Equal(t, "2w", general["adaptive_searching_delay"])
			assert.Equal(t, float64(14), general["days_to_upgrade_subs"])
			assert.Equal(t, true, general["serie_tag_enabled"])
			assert.Equal(t, []any{"no-subs"}, general["remove_profile_tags"])
			english := bazarrProfile(t, hc, "English")
			assert.Equal(t, float64(1), english["cutoff"])
			assert.Equal(t, []any{"WEB"}, english["mustContain"])
			assert.Equal(t, []any{"CAM"}, english["mustNotContain"])
			assert.InDelta(t, 1, english["originalFormat"], 0)
			assert.Equal(t, "english", english["tag"])
			languages, err := hc.GetJSONList(context.Background(), "/api/system/languages")
			require.NoError(t, err)
			var enabled []string
			for _, l := range languages {
				if l["enabled"] == true {
					enabled = append(enabled, fmt.Sprint(l["code2"]))
				}
			}
			slices.Sort(enabled)
			assert.Equal(t, []string{"en", "es"}, enabled, "the declared languages are not the enabled ones")
			if atNewest() {
				assert.Equal(t, true, general["use_jellyfin"])
				jellyfin, _ := settings["jellyfin"].(map[string]any)
				assert.Equal(t, fakes+"/jellyfin", jellyfin["url"])
				assert.Equal(t, "compat-jellyfin-key", jellyfin["apikey"])
				assert.Equal(t, []any{"Movies"}, jellyfin["movie_library"])
				assert.Equal(t, []any{"compat-movies"}, jellyfin["movie_library_ids"], "the library's id, as Bazarr lists it")
				assert.Equal(t, []any{"compat-shows"}, jellyfin["series_library_ids"])
				assert.Equal(t, true, jellyfin["update_movie_library"])
				assert.Equal(t, true, jellyfin["update_series_library"])
				assert.Equal(t, "async", jellyfin["refresh_method"])
			}
		},
		cleanup: func(t *testing.T, _ map[string][]string) {
			assert.Equal(t, []string{"Hand-made"}, bazarrProfileNames(t, hc))
		},
	})
}

func bazarrProfile(t *testing.T, hc *engine.HTTPClient, name string) map[string]any {
	t.Helper()
	profiles, err := hc.GetJSONList(context.Background(), "/api/system/languages/profiles")
	require.NoError(t, err)
	for _, p := range profiles {
		if p["name"] == name {
			return p
		}
	}
	require.Failf(t, "no such language profile", "%s", name)
	return nil
}

func bazarrProfileNames(t *testing.T, hc *engine.HTTPClient) []string {
	t.Helper()
	profiles, err := hc.GetJSONList(context.Background(), "/api/system/languages/profiles")
	require.NoError(t, err)
	names := make([]string, 0, len(profiles))
	for _, p := range profiles {
		names = append(names, fmt.Sprint(p["name"]))
	}
	slices.Sort(names)
	return names
}

// bazarrClient talks to Bazarr directly, outside the operator.
func bazarrClient(t *testing.T, base, key string) *engine.HTTPClient {
	t.Helper()
	hc, err := engine.NewHTTPClient(base, engine.AuthAPIKey, engine.WithAPIKey(key),
		engine.WithAPIKeyHeader("X-API-KEY"), engine.WithAppLabel("compat"))
	require.NoError(t, err)
	return hc
}
