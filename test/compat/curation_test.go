//go:build compat

package compat

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	curationv1alpha1 "github.com/kyleseneker/arrchitect/api/curation/v1alpha1"
	jellyfinclient "github.com/kyleseneker/arrchitect/internal/client/jellyfin"
	"github.com/kyleseneker/arrchitect/internal/controller/curation"
	"github.com/kyleseneker/arrchitect/internal/controller/reconciler"
	"github.com/kyleseneker/arrchitect/internal/httpapi"
	"github.com/kyleseneker/arrchitect/internal/testnet"
)

// TestMaintainerr sets up a fresh Maintainerr against Jellyfin and a Radarr, and
// creates a rule group that reads both.
func TestMaintainerr(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	jf := startJellyfin(t)
	jellyfinKey := jellyfinAPIKey(t, jf.url())
	radarr, radarrKey := startPVR(t, "radarr", radarrImage, 7878, "v3")

	maintainerr := startContainer(t, containerSpec{name: "maintainerr", image: maintainerrImage, port: 6246})
	waitFor(t, "maintainerr", 3*time.Minute, func() error {
		resp, err := http.Get(maintainerr.url() + "/api/health/ready")
		if err != nil {
			return err
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("status %d", resp.StatusCode)
		}
		return nil
	})
	fakes := startFakes(t)
	proxy := newRecordingProxy(t, maintainerr.url())

	checkCompat(t, compatCase{
		fixture: "maintainerr.yaml",
		update: `[
			{"op": "replace", "path": "/spec/notifications/0/aboutScale", "value": 7},
			{"op": "replace", "path": "/spec/ruleGroups/0/actionAfterDays", "value": 45},
			{"op": "replace", "path": "/spec/radarr/0/exclusionTag", "value": "retain"}
		]`,
		export: "maintainerr",
		vars: map[string]string{
			"URL": proxy.url(), "HOST": hostIP, "FAKES": fakes,
			"JELLYFIN_PORT": strconv.Itoa(jf.port), "RADARR_PORT": strconv.Itoa(radarr.port),
		},
		secrets: map[string]string{
			"jellyfin-api-key": jellyfinKey, "radarr-api-key": radarrKey, "seerr-api-key": randomKey(),
			"webhook-url": "http://webhook.invalid/maintainerr",
		},
		newObj: func() reconciler.ConfigResource { return &curationv1alpha1.Maintainerr{} },
		reconciler: func(c client.Client) reconcile.Reconciler {
			return &curation.MaintainerrReconciler{Client: c, Recorder: events.NewFakeRecorder(100)}
		},
		proxy:   proxy,
		observe: true,
		verify: func(t *testing.T) {
			hc := testnet.Client(t, maintainerr.url(), "")
			settings, err := hc.GetJSON(ctx, "/api/settings")
			require.NoError(t, err)
			assert.Equal(t, "jellyfin", settings["media_server_type"])

			groups, err := hc.GetJSONList(ctx, "/api/rules")
			require.NoError(t, err)
			require.Len(t, groups, 1)
			group := groups[0]
			assert.Equal(t, "Stale unwatched movies", group["name"])
			assert.Equal(t, "movie", group["dataType"])
			collection, _ := group["collection"].(map[string]any)
			assert.EqualValues(t, 30, collection["deleteAfterDays"])
			assert.Equal(t, true, collection["cleanupLeftoverFolders"], "Maintainerr kept the leftover-folder cleanup")
			assert.NotNil(t, collection["radarrSettingsId"])
			rules, _ := group["rules"].([]any)
			require.Len(t, rules, 3)
			var last map[string]any
			require.NoError(t, json.Unmarshal([]byte(rules[2].(map[string]any)["ruleJson"].(string)), &last))
			assert.EqualValues(t, 1, last["operator"], "the second section joins with OR")
			assert.EqualValues(t, 1, last["section"])

			notifications, err := hc.GetJSONList(ctx, "/api/notifications/configurations")
			require.NoError(t, err)
			require.Len(t, notifications, 1)
			n := notifications[0]
			assert.Equal(t, "webhook", n["agent"])
			assert.Equal(t, []any{float64(8), float64(16), float64(32)}, n["types"])
			options, _ := n["options"].(map[string]any)
			assert.Equal(t, "http://webhook.invalid/maintainerr", options["webhookUrl"])
			linked, _ := group["notifications"].([]any)
			require.Len(t, linked, 1, "the rule group is not linked to its notification")
			assert.Equal(t, n["id"], linked[0].(map[string]any)["id"])
		},
		cleanup: func(t *testing.T, _ map[string][]string) {
			hc := testnet.Client(t, maintainerr.url(), "")
			for _, path := range []string{"/api/rules", "/api/notifications/configurations", "/api/settings/radarr"} {
				items, err := hc.GetJSONList(ctx, path)
				require.NoError(t, err)
				assert.Empty(t, items, "%s still has what the operator created", path)
			}
		},
	})
}

// jellyfinAPIKey runs Jellyfin's setup and creates a Movies library and an API
// key for Maintainerr, which connects to Jellyfin by key only.
func jellyfinAPIKey(t *testing.T, base string) string {
	t.Helper()
	ctx := context.Background()
	password := randomKey()
	hc, err := httpapi.NewHTTPClient(base, httpapi.AuthMediaBrowser, httpapi.WithAppLabel("compat"))
	require.NoError(t, err)
	jc := jellyfinclient.NewClient(hc)
	require.NoError(t, jc.RunSetupWizard(ctx, jellyfinAdmin, password, "compat", "en", "US"))
	require.NoError(t, jc.Authenticate(ctx, jellyfinAdmin, password))
	require.NoError(t, jc.CreateLibrary(ctx, "Movies", "movies", map[string]any{
		"PathInfos": []map[string]any{{"Path": "/media/movies"}},
	}))
	return createJellyfinAPIKey(t, hc, "maintainerr")
}

// createJellyfinAPIKey creates an API key named app through a signed-in client.
func createJellyfinAPIKey(t *testing.T, hc *httpapi.HTTPClient, app string) string {
	t.Helper()
	ctx := context.Background()
	_, err := hc.Do(ctx, http.MethodPost, "/Auth/Keys?app="+app, nil)
	require.NoError(t, err)
	keys, err := hc.GetJSON(ctx, "/Auth/Keys")
	require.NoError(t, err)
	items, _ := keys["Items"].([]any)
	for _, item := range items {
		if key, _ := item.(map[string]any); key["AppName"] == app {
			token, _ := key["AccessToken"].(string)
			require.NotEmpty(t, token)
			return token
		}
	}
	require.Fail(t, "Jellyfin created no API key", "for %s", app)
	return ""
}
