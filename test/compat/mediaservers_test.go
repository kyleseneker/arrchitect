//go:build compat

package compat

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	mediaserversv1alpha1 "github.com/kyleseneker/arrchitect/api/mediaservers/v1alpha1"
	jellyfinclient "github.com/kyleseneker/arrchitect/internal/client/jellyfin"
	plexclient "github.com/kyleseneker/arrchitect/internal/client/plex"
	ctrlcommon "github.com/kyleseneker/arrchitect/internal/controller/common"
	"github.com/kyleseneker/arrchitect/internal/controller/mediaservers"
	"github.com/kyleseneker/arrchitect/internal/engine"
)

const jellyfinAdmin = "compat-admin"

// startJellyfin runs a fresh Jellyfin with its library folders created. Its
// first-time setup is left to the operator, which is what a rebuild relies on.
func startJellyfin(t *testing.T) *container {
	t.Helper()
	jf := startContainer(t, containerSpec{name: "jellyfin", image: jellyfinImage, port: 8096})
	jf.exec(t, "mkdir -p /media/movies /media/tv")
	// Jellyfin 12 serves a startup placeholder, system info included, before the
	// real server takes the port over; its log says when that has happened.
	waitFor(t, "jellyfin", 3*time.Minute, func() error {
		if !strings.Contains(jf.logs(t), "Startup complete") {
			return fmt.Errorf("still starting")
		}
		return nil
	})
	return jf
}

func TestJellyfin(t *testing.T) {
	t.Parallel()
	jf := startJellyfin(t)
	password := randomKey()
	proxy := newRecordingProxy(t, jf.url())

	checkCompat(t, compatCase{
		fixture: "jellyfin.yaml",
		update: `[
			{"op": "replace", "path": "/spec/libraries/0/automaticRefreshIntervalDays", "value": 60}
		]`,
		export:  "jellyfin",
		vars:    map[string]string{"URL": proxy.url()},
		secrets: map[string]string{"username": jellyfinAdmin, "password": password},
		newObj:  func() ctrlcommon.ConfigResource { return &mediaserversv1alpha1.Jellyfin{} },
		reconciler: func(c client.Client) reconcile.Reconciler {
			return &mediaservers.JellyfinReconciler{Client: c, Scheme: c.Scheme(), Recorder: events.NewFakeRecorder(100)}
		},
		proxy: proxy,
		// Signing in is a POST that changes nothing.
		read: func(r request) bool {
			return getOnly(r) || (r.Method == http.MethodPost && r.Path == "/Users/AuthenticateByName")
		},
		observe: true,
		verify: func(t *testing.T) {
			jc := jellyfinClient(t, jf.url(), password)
			cfg, err := jc.GetConfig(context.Background(), "/System/Configuration")
			require.NoError(t, err)
			assert.Equal(t, "compat-jellyfin", cfg["ServerName"])
			encoding, err := jc.GetConfig(context.Background(), "/System/Configuration/encoding")
			require.NoError(t, err)
			assert.Equal(t, float64(28), encoding["H265Crf"])
			assert.Equal(t, "hable", encoding["TonemappingAlgorithm"])
			assert.InDelta(t, 2.5, encoding["DownMixAudioBoost"], 0)
			assert.Equal(t, true, cfg["EnableMetrics"])
			trickplay, _ := cfg["TrickplayOptions"].(map[string]any)
			assert.Equal(t, "Idle", trickplay["ProcessPriority"])
			assert.Equal(t, []any{float64(320), float64(640)}, trickplay["WidthResolutions"])
			network, err := jc.GetConfig(context.Background(), "/System/Configuration/network")
			require.NoError(t, err)
			assert.Equal(t, []any{"10.0.0.1"}, network["KnownProxies"])
			assert.Equal(t, []any{"all=https://jellyfin.example.com"}, network["PublishedServerUriBySubnet"])
			libraries, err := jc.ListLibraries(context.Background())
			require.NoError(t, err)
			names := make([]any, 0, len(libraries))
			for _, l := range libraries {
				names = append(names, l["Name"])
			}
			assert.ElementsMatch(t, []any{"Movies", "TV Shows"}, names)
			checkJellyfinAPIKey(t, jf.url(), password, proxy)
		},
	})
}

// checkJellyfinAPIKey switches the resource to an API key and checks that a
// reconcile with it syncs without signing in. The rest of the case then runs
// with the key.
func checkJellyfinAPIKey(t *testing.T, base, password string, proxy *recordingProxy) {
	t.Helper()
	ctx := context.Background()
	hc, err := engine.NewHTTPClient(base, engine.AuthMediaBrowser, engine.WithAppLabel("compat"))
	require.NoError(t, err)
	require.NoError(t, jellyfinclient.NewClient(hc).Authenticate(ctx, jellyfinAdmin, password))
	key := createJellyfinAPIKey(t, hc, "arrchitect")

	var list mediaserversv1alpha1.JellyfinList
	require.NoError(t, k8s.List(ctx, &list))
	i := slices.IndexFunc(list.Items, func(j mediaserversv1alpha1.Jellyfin) bool {
		return j.Spec.Connection.URL == proxy.url()
	})
	require.GreaterOrEqual(t, i, 0, "no Jellyfin resource for this case")
	cfg := &list.Items[i]
	secret := &corev1.Secret{Name: "compat-credentials", Namespace: cfg.Namespace}
	require.NoError(t, k8s.Patch(ctx, secret, client.RawPatch(types.MergePatchType,
		fmt.Appendf(nil, `{"stringData":{"jellyfin-api-key":%q}}`, key))))
	require.NoError(t, k8s.Patch(ctx, cfg, client.RawPatch(types.MergePatchType,
		[]byte(`{"spec":{"connection":{"apiKeySecretRef":{"name":"compat-credentials","key":"jellyfin-api-key"}}}}`))))

	proxy.reset()
	r := &mediaservers.JellyfinReconciler{Client: k8s, Scheme: k8s.Scheme(), Recorder: events.NewFakeRecorder(100)}
	obj := reconcileUntilCurrent(t, r, client.ObjectKeyFromObject(cfg), func() ctrlcommon.ConfigResource {
		return &mediaserversv1alpha1.Jellyfin{}
	})
	assertSynced(t, obj, "reconcile with an API key")
	for _, req := range proxy.all() {
		assert.NotEqual(t, "/Users/AuthenticateByName", req.Path, "signed in despite having an API key")
	}
}

// jellyfinClient signs in as the admin the operator created.
func jellyfinClient(t *testing.T, base, password string) *jellyfinclient.Client {
	t.Helper()
	hc, err := engine.NewHTTPClient(base, engine.AuthMediaBrowser, engine.WithAppLabel("compat"))
	require.NoError(t, err)
	jc := jellyfinclient.NewClient(hc)
	require.NoError(t, jc.Authenticate(context.Background(), jellyfinAdmin, password))
	return jc
}

// plexAllowedNetworks lets an unclaimed Plex answer the suite without a
// plex.tv account: Plex accepts any token from these networks.
const plexAllowedNetworks = "10.0.0.0/255.0.0.0,172.16.0.0/255.240.0.0,192.168.0.0/255.255.0.0"

func TestPlex(t *testing.T) {
	t.Parallel()
	prefs := `<?xml version="1.0" encoding="utf-8"?>
<Preferences allowedNetworks="` + plexAllowedNetworks + `" FriendlyName="compat" AcceptedEULA="1" />
`
	plex := startContainer(t, containerSpec{
		name: "plex", image: plexImage, port: 32400,
		files: map[string]string{"/config/Library/Application Support/Plex Media Server/Preferences.xml": prefs},
	})
	plex.exec(t, "mkdir -p /media/movies /media/tv /media/extra /transcode/backups /transcode/downloads")
	// Plex answers long before it will create a library ("the server is still
	// starting up"), so creating one shows when it is ready. It is the declared
	// Movies library with a folder the fixture leaves out, for the operator to
	// edit.
	waitFor(t, "plex", 3*time.Minute, func() error {
		resp, err := http.Post(plex.url()+"/library/sections?name=Movies&type=movie&agent=tv.plex.agents.movie"+
			"&scanner=Plex%20Movie&language=en-US&location=%2Fmedia%2Fmovies&location=%2Fmedia%2Fextra", "", nil)
		if err != nil {
			return err
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
			return fmt.Errorf("library create %d", resp.StatusCode)
		}
		return nil
	})
	proxy := newRecordingProxy(t, plex.url())

	checkCompat(t, compatCase{
		fixture: "plex.yaml",
		update: `[
			{"op": "replace", "path": "/spec/libraries/0/language", "value": "en-GB"}
		]`,
		export:  "plex",
		vars:    map[string]string{"URL": proxy.url(), "ALLOWED_NETWORKS": plexAllowedNetworks},
		secrets: map[string]string{"token": "unclaimed"},
		newObj:  func() ctrlcommon.ConfigResource { return &mediaserversv1alpha1.Plex{} },
		reconciler: func(c client.Client) reconcile.Reconciler {
			return &mediaservers.PlexReconciler{Client: c, Scheme: c.Scheme(), Recorder: events.NewFakeRecorder(100)}
		},
		proxy:   proxy,
		observe: true,
		verify: func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, plex.url()+"/library/sections", nil)
			require.NoError(t, err)
			req.Header.Set("Accept", "application/json")
			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()
			var sections struct {
				MediaContainer struct {
					Directory []struct {
						Title    string
						Location []struct{ Path string } `json:"Location"`
					} `json:"Directory"`
				} `json:"MediaContainer"`
			}
			require.NoError(t, jsonDecode(resp, &sections))
			titles := make([]string, 0, len(sections.MediaContainer.Directory))
			for _, d := range sections.MediaContainer.Directory {
				titles = append(titles, d.Title)
				if d.Title == "Movies" {
					require.Len(t, d.Location, 1, "the folder the resource leaves out was not removed")
					assert.Equal(t, "/media/movies", d.Location[0].Path)
				}
			}
			assert.ElementsMatch(t, []string{"Movies", "TV Shows"}, titles)

			prefsHC, err := engine.NewHTTPClient(plex.url(), engine.AuthPlexToken, engine.WithPlexToken("unclaimed"),
				engine.WithAppLabel("compat"))
			require.NoError(t, err)
			settings, err := plexclient.NewClient(prefsHC).GetSettings(context.Background())
			require.NoError(t, err)
			for id, want := range map[string]string{
				"ScheduledLibraryUpdateInterval": "21600", "GenerateBIFBehavior": "scheduled", "ButlerStartHour": "3",
				"TranscoderQuality": "2", "TranscoderHEVCEncoding": "1", "ManualPortMappingPort": "32499", "RelayEnabled": "0",
			} {
				assert.Equal(t, want, settings[id], id)
			}
		},
	})
}
