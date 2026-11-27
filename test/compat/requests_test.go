//go:build compat

package compat

import (
	"context"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	requestsv1alpha1 "github.com/kyleseneker/arrchitect/api/requests/v1alpha1"
	jellyfinclient "github.com/kyleseneker/arrchitect/internal/client/jellyfin"
	seerrclient "github.com/kyleseneker/arrchitect/internal/client/seerr"
	ctrlcommon "github.com/kyleseneker/arrchitect/internal/controller/common"
	"github.com/kyleseneker/arrchitect/internal/controller/requests"
	"github.com/kyleseneker/arrchitect/internal/httpapi"
	"github.com/kyleseneker/arrchitect/internal/testnet"
)

// TestSeerr runs Seerr's first-time setup through Jellyfin, as a rebuild does:
// a fresh Seerr, a Jellyfin with one library, and Sonarr and Radarr to connect.
func TestSeerr(t *testing.T) {
	t.Parallel()
	jf := startJellyfin(t)
	jellyfinPassword := randomKey()
	hc, err := httpapi.NewHTTPClient(jf.url(), httpapi.AuthMediaBrowser, httpapi.WithAppLabel("compat"))
	require.NoError(t, err)
	jc := jellyfinclient.NewClient(hc)
	ctx := context.Background()
	require.NoError(t, jc.RunSetupWizard(ctx, jellyfinAdmin, jellyfinPassword, "compat", "en", "US"))
	require.NoError(t, jc.Authenticate(ctx, jellyfinAdmin, jellyfinPassword))
	require.NoError(t, jc.CreateLibrary(ctx, "Movies", "movies", map[string]any{
		"PathInfos": []map[string]any{{"Path": "/media/movies"}},
	}))

	sonarr, sonarrKey := startPVR(t, "sonarr", sonarrImage, 8989, "v3")
	radarr, radarrKey := startPVR(t, "radarr", radarrImage, 7878, "v3")
	for _, app := range []struct {
		c    *container
		key  string
		path string
	}{{sonarr, sonarrKey, "/data/media/tv"}, {radarr, radarrKey, "/data/media/movies"}} {
		app.c.exec(t, "mkdir -p "+app.path+" && chown -R abc:abc /data")
		root := map[string]any{"path": app.path}
		require.NoError(t, testnet.Client(t, app.c.url(), app.key).PostJSON(ctx, "/api/v3/rootfolder", root))
	}

	seerr := startContainer(t, containerSpec{
		name: "seerr", image: seerrImage, port: 5055, env: map[string]string{"TZ": "Etc/UTC"},
	})
	waitFor(t, "seerr", 3*time.Minute, func() error {
		resp, err := http.Get(seerr.url() + "/api/v1/settings/public")
		if err != nil {
			return err
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return assert.AnError
		}
		return nil
	})
	fakes := startFakes(t)
	proxy := newRecordingProxy(t, seerr.url())
	seerrAPIKey := func(t *testing.T) string {
		hc, err := httpapi.NewHTTPClient(seerr.url(), httpapi.AuthSession, httpapi.WithAppLabel("compat"))
		require.NoError(t, err)
		sc := seerrclient.NewClient(hc)
		require.NoError(t, sc.AuthenticateJellyfin(ctx, jellyfinAdmin, jellyfinPassword, hostIP, jf.port, "", false, true))
		key, err := sc.GetAPIKey(ctx)
		require.NoError(t, err)
		return key
	}

	checkCompat(t, compatCase{
		fixture:      "seerr.yaml",
		export:       "seerr",
		exportAPIKey: seerrAPIKey,
		cleanup: func(t *testing.T, _ map[string][]string) {
			hc := testnet.Client(t, seerr.url(), seerrAPIKey(t))
			for _, kind := range []string{"sonarr", "radarr"} {
				servers, err := hc.GetJSONList(ctx, "/api/v1/settings/"+kind)
				require.NoError(t, err)
				assert.Empty(t, servers, "Seerr still has the %s connection the operator created", kind)
			}
		},
		vars: map[string]string{
			"URL": proxy.url(), "HOST": hostIP, "FAKES": fakes, "JELLYFIN_PORT": strconv.Itoa(jf.port),
			"SONARR_PORT": strconv.Itoa(sonarr.port), "RADARR_PORT": strconv.Itoa(radarr.port),
		},
		secrets: map[string]string{
			"jellyfin-username": jellyfinAdmin, "jellyfin-password": jellyfinPassword,
			"sonarr-api-key": sonarrKey, "radarr-api-key": radarrKey, "discord-webhook": fakes + "/webhook",
		},
		newObj: func() ctrlcommon.ConfigResource { return &requestsv1alpha1.Seerr{} },
		reconciler: func(c client.Client) reconcile.Reconciler {
			return &requests.SeerrReconciler{Client: c, Recorder: events.NewFakeRecorder(100)}
		},
		proxy:   proxy,
		observe: true,
		// Signing in through Jellyfin is a POST; after setup it changes nothing.
		read: func(r request) bool {
			return getOnly(r) || (r.Method == http.MethodPost && r.Path == "/api/v1/auth/jellyfin")
		},
		update: `[{"op": "replace", "path": "/spec/sonarr/0/externalUrl", "value": "https://tv.example.com"},
			{"op": "replace", "path": "/spec/main/defaultQuotas/movie/limit", "value": 3},
			{"op": "replace", "path": "/spec/jobSchedules/0/schedule", "value": "0 15 */6 * * *"}]`,
		verifyUpdate: func(t *testing.T) {
			direct := testnet.Client(t, seerr.url(), seerrAPIKey(t))
			sonarrs, err := direct.GetJSONList(ctx, "/api/v1/settings/sonarr")
			require.NoError(t, err)
			urls := map[any]any{}
			for _, s := range sonarrs {
				urls[s["name"]] = s["externalUrl"]
			}
			assert.Equal(t, "https://tv.example.com", urls["Sonarr"], "Seerr didn't take the update to an existing server")
			main, err := direct.GetJSON(ctx, "/api/v1/settings/main")
			require.NoError(t, err)
			quotas, _ := main["defaultQuotas"].(map[string]any)
			assert.Equal(t, map[string]any{"quotaLimit": float64(3), "quotaDays": float64(7)}, quotas["movie"],
				"Seerr didn't keep the quota days the update left alone")
			assert.Equal(t, "0 15 */6 * * *", seerrJobSchedules(t, direct)["sonarr-scan"])
		},
		verify: func(t *testing.T) {
			resp, err := http.Get(seerr.url() + "/api/v1/settings/public")
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()
			var public struct {
				Initialized      bool   `json:"initialized"`
				ApplicationTitle string `json:"applicationTitle"`
			}
			require.NoError(t, jsonDecode(resp, &public))
			assert.True(t, public.Initialized, "first-time setup did not complete")
			assert.Equal(t, "Compat Requests", public.ApplicationTitle)

			server, err := testnet.Client(t, seerr.url(), seerrAPIKey(t)).GetJSON(ctx, "/api/v1/settings/jellyfin")
			require.NoError(t, err)
			assert.Equal(t, hostIP, server["ip"])
			assert.Equal(t, float64(jf.port), server["port"])
			assert.NotContains(t, server, "hostname", "Seerr's settings migration rewrites the port from a stored hostname")

			direct := testnet.Client(t, seerr.url(), seerrAPIKey(t))
			main, err := direct.GetJSON(ctx, "/api/v1/settings/main")
			require.NoError(t, err)
			assert.Equal(t, "en|ja", main["originalLanguage"])
			assert.Equal(t, "210024", main["blocklistedTags"])
			assert.Equal(t, true, main["newPlexLogin"])
			quotas, _ := main["defaultQuotas"].(map[string]any)
			assert.Equal(t, map[string]any{"quotaLimit": float64(10)}, quotas["tv"])

			sonarrs, err := direct.GetJSONList(ctx, "/api/v1/settings/sonarr")
			require.NoError(t, err)
			require.Len(t, sonarrs, 2, "both declared Sonarr servers should be in Seerr")
			byName := map[string]map[string]any{}
			for _, s := range sonarrs {
				byName[s["name"].(string)] = s
			}
			assert.Equal(t, true, byName["Sonarr 4K"]["is4k"])
			assert.Equal(t, "anime", byName["Sonarr"]["animeSeriesType"])
			assert.Equal(t, true, byName["Sonarr"]["tagRequests"])
			assert.NotNil(t, byName["Sonarr"]["activeAnimeProfileId"], "the anime profile name was not resolved")

			discord, err := direct.GetJSON(ctx, "/api/v1/settings/notifications/discord")
			require.NoError(t, err)
			options, _ := discord["options"].(map[string]any)
			assert.Equal(t, false, options["enableMentions"], "a boolean option must reach Seerr as a boolean")
			assert.Equal(t, fakes+"/webhook", options["webhookUrl"])

			schedules := seerrJobSchedules(t, direct)
			assert.Equal(t, "0 0 */6 * * *", schedules["sonarr-scan"])
			assert.Equal(t, "0 0 6 * * *", schedules["image-cache-cleanup"])
		},
	})
}

// seerrJobSchedules maps each job Seerr runs to its schedule.
func seerrJobSchedules(t *testing.T, hc *httpapi.HTTPClient) map[any]any {
	t.Helper()
	jobs, err := hc.GetJSONList(context.Background(), "/api/v1/settings/jobs")
	require.NoError(t, err)
	schedules := map[any]any{}
	for _, job := range jobs {
		schedules[job["id"]] = job["cronSchedule"]
	}
	return schedules
}
