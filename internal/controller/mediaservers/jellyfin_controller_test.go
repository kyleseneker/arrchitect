package mediaservers

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	mediaserversv1alpha1 "github.com/kyleseneker/arrchitect/api/mediaservers/v1alpha1"
	"github.com/kyleseneker/arrchitect/internal/controller/reconciler"
	"github.com/kyleseneker/arrchitect/internal/testnet"
)

const conditionTrue = "True"

// fakeJellyfin implements the handful of endpoints the reconciler drives:
// the public info probe, the four startup-wizard steps, authentication, and
// the virtual-folder (library) list.
type fakeJellyfin struct {
	config        map[string]any
	failure       string
	mu            sync.Mutex
	requests      []string
	setupComplete bool
	existingLibs  []map[string]any
	// apiKey, when set, is the only token /System/Info accepts.
	apiKey string
	// auth is the Authorization header each path last received.
	auth map[string]string
	// version is what /System/Info/Public reports.
	version string
	// posted is the last body each configuration path, and the library
	// create, received.
	posted map[string]map[string]any
	// available is what /Libraries/AvailableOptions answers, and
	// availableQuery the query it was last asked with.
	available      map[string]any
	availableQuery string
}

func (f *fakeJellyfin) postedTo(path string) map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.posted[path]
}

func (f *fakeJellyfin) authFor(path string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.auth[path]
}

func (f *fakeJellyfin) saw(method, path string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Contains(f.requests, method+" "+path)
}

func (f *fakeJellyfin) serve(t *testing.T) string {
	t.Helper()
	srv := testnet.Serve(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.requests = append(f.requests, r.Method+" "+r.URL.Path)
		if f.auth == nil {
			f.auth = map[string]string{}
		}
		f.auth[r.URL.Path] = r.Header.Get("Authorization")
		f.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/System/Info" && f.apiKey != "" &&
			!strings.Contains(r.Header.Get("Authorization"), `Token="`+f.apiKey+`"`) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path == f.failure {
			w.WriteHeader(500)
			return
		}
		switch r.URL.Path {
		case "/System/Info/Public":
			f.mu.Lock()
			done := f.setupComplete
			f.mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"StartupWizardCompleted": done, "Version": f.version})
		case "/System/Configuration":
			_ = json.NewEncoder(w).Encode(f.config)
		case "/Users/AuthenticateByName":
			_ = json.NewEncoder(w).Encode(map[string]any{"AccessToken": "test-token"})
		case "/Library/VirtualFolders":
			if r.Method == http.MethodGet {
				f.mu.Lock()
				libs := f.existingLibs
				f.mu.Unlock()
				if libs == nil {
					libs = []map[string]any{}
				}
				_ = json.NewEncoder(w).Encode(libs)
				return
			}
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			f.mu.Lock()
			if f.posted == nil {
				f.posted = map[string]map[string]any{}
			}
			f.posted[r.URL.Path] = body
			f.mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		case "/Libraries/AvailableOptions":
			f.mu.Lock()
			f.availableQuery = r.URL.RawQuery
			f.mu.Unlock()
			_ = json.NewEncoder(w).Encode(f.available)
		default:
			// Startup wizard steps and config posts.
			if r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/System/Configuration/") {
				var body map[string]any
				_ = json.NewDecoder(r.Body).Decode(&body)
				f.mu.Lock()
				if f.posted == nil {
					f.posted = map[string]map[string]any{}
				}
				f.posted[r.URL.Path] = body
				f.mu.Unlock()
			}
			_ = json.NewEncoder(w).Encode(map[string]any{})
		}
	}))
	return srv.URL
}

func adminSecret() *corev1.Secret {
	return &corev1.Secret{
		Name: "jellyfin-admin", Namespace: "media",
		Data: map[string][]byte{"username": []byte("admin"), "password": []byte("hunter2")},
	}
}

func jellyfinConfig(url string, libs ...mediaserversv1alpha1.JellyfinLibrary) *mediaserversv1alpha1.Jellyfin {
	return &mediaserversv1alpha1.Jellyfin{
		Name: "jellyfin", Namespace: "media",
		Spec: mediaserversv1alpha1.JellyfinSpec{
			Connection: mediaserversv1alpha1.JellyfinConnection{URL: url},
			AdminUser: mediaserversv1alpha1.JellyfinAdminUser{
				UsernameSecretRef: commonv1alpha1.SecretKeyRef{Name: "jellyfin-admin", Key: "username"},
				PasswordSecretRef: commonv1alpha1.SecretKeyRef{Name: "jellyfin-admin", Key: "password"},
			},
			Libraries: libs,
		},
	}
}

func runJellyfin(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme), "adding client-go scheme")
	require.NoError(t, mediaserversv1alpha1.AddToScheme(scheme), "adding mediaservers scheme")
	b := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&mediaserversv1alpha1.Jellyfin{})
	for _, o := range objs {
		b = b.WithObjects(o)
	}
	c := b.Build()
	r := &JellyfinReconciler{Client: c, Recorder: events.NewFakeRecorder(50)}
	_, err := r.Reconcile(context.Background(), ctrl.Request{
		Name: "jellyfin", Namespace: "media",
	})
	require.NoError(t, err, "reconcile")
	return c
}

func jellyfinSynced(t *testing.T, c client.Client) string {
	t.Helper()
	var cfg mediaserversv1alpha1.Jellyfin
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "jellyfin", Namespace: "media"}, &cfg), "fetching config")
	for _, cond := range cfg.Status.Conditions {
		if cond.Type == "Synced" {
			return string(cond.Status)
		}
	}
	return ""
}

// A fresh Jellyfin reports the wizard incomplete; the operator must drive it
// rather than leaving the server unconfigured.
func TestReconcileRunsSetupWizardOnFreshInstall(t *testing.T) {
	app := &fakeJellyfin{setupComplete: false}
	url := app.serve(t)

	c := runJellyfin(t, adminSecret(), jellyfinConfig(url))

	for _, step := range []string{"/Startup/User", "/Startup/Configuration", "/Startup/RemoteAccess", "/Startup/Complete"} {
		assert.True(t, app.saw("POST", step), "wizard step %s was never run", step)
	}
	assert.Equal(t, conditionTrue, jellyfinSynced(t, c), "Synced")
}

// An already-configured server must not be put back through the wizard, which
// would try to recreate the admin user.
func TestReconcileSkipsWizardWhenSetupComplete(t *testing.T) {
	app := &fakeJellyfin{setupComplete: true}
	url := app.serve(t)

	runJellyfin(t, adminSecret(), jellyfinConfig(url))

	assert.False(t, app.saw("POST", "/Startup/User"), "re-ran the setup wizard against an already-configured server")
	assert.True(t, app.saw("POST", "/Users/AuthenticateByName"), "never authenticated against the configured server")
}

func TestReconcileCreatesMissingLibrary(t *testing.T) {
	app := &fakeJellyfin{setupComplete: true}
	url := app.serve(t)

	runJellyfin(t, adminSecret(), jellyfinConfig(url,
		mediaserversv1alpha1.JellyfinLibrary{Name: "Movies", CollectionType: "movies", Paths: []string{"/media/movies"}}))

	assert.True(t, app.saw("POST", "/Library/VirtualFolders"), "declared library was never created")
}

// An existing library by the same name is never recreated, which would
// duplicate or clobber it, and is left alone while it matches.
func TestReconcileLeavesAMatchingLibraryAlone(t *testing.T) {
	app := &fakeJellyfin{
		setupComplete: true,
		existingLibs:  []map[string]any{{"Name": "Movies", "CollectionType": "movies", "ItemId": "lib1", "Locations": []string{"/media/movies"}}},
	}
	url := app.serve(t)

	runJellyfin(t, adminSecret(), jellyfinConfig(url,
		mediaserversv1alpha1.JellyfinLibrary{Name: "Movies", CollectionType: "movies", Paths: []string{"/media/movies/"}}))

	assert.False(t, app.saw("POST", "/Library/VirtualFolders"), "recreated a library that already existed")
	assert.False(t, app.saw("POST", "/Library/VirtualFolders/Paths"))
	assert.False(t, app.saw("DELETE", "/Library/VirtualFolders/Paths"))
	assert.False(t, app.saw("POST", "/Library/VirtualFolders/LibraryOptions"))
}

func TestReconcileSyncsAnExistingLibrarysFoldersAndOptions(t *testing.T) {
	app := &fakeJellyfin{
		setupComplete: true,
		existingLibs: []map[string]any{{
			"Name": "Movies", "CollectionType": "movies", "ItemId": "lib1", "Locations": []string{"/media/old", "/media/movies"},
			"LibraryOptions": map[string]any{"EnableRealtimeMonitor": false},
		}},
	}
	url := app.serve(t)

	runJellyfin(t, adminSecret(), jellyfinConfig(url, mediaserversv1alpha1.JellyfinLibrary{
		Name: "Movies", CollectionType: "movies", Paths: []string{"/media/movies", "/media/new"}, EnableRealtimeMonitor: new(true),
	}))

	assert.True(t, app.saw("POST", "/Library/VirtualFolders/Paths"), "the new folder was not added")
	assert.True(t, app.saw("DELETE", "/Library/VirtualFolders/Paths"), "the undeclared folder was not removed")
	assert.True(t, app.saw("POST", "/Library/VirtualFolders/LibraryOptions"), "the options were not saved")
	assert.False(t, app.saw("POST", "/Library/VirtualFolders"), "recreated a library that already existed")
}

func TestReconcileRefusesToChangeALibrarysType(t *testing.T) {
	app := &fakeJellyfin{setupComplete: true, existingLibs: []map[string]any{{"Name": "Movies", "Locations": []string{"/media/movies"}}}}
	url := app.serve(t)

	c := runJellyfin(t, adminSecret(), jellyfinConfig(url,
		mediaserversv1alpha1.JellyfinLibrary{Name: "Movies", CollectionType: "movies", Paths: []string{"/media/movies"}}))

	assert.NotEqual(t, conditionTrue, jellyfinSynced(t, c), "a mixed library can't become a movies library in place")
	assert.False(t, app.saw("DELETE", "/Library/VirtualFolders/Paths"))
}

func TestReconcileRefusesALibraryNameTwoLibrariesShare(t *testing.T) {
	app := &fakeJellyfin{setupComplete: true, existingLibs: []map[string]any{
		{"Name": "Movies", "CollectionType": "movies", "Locations": []string{"/media/movies"}},
		{"Name": "Movies", "CollectionType": "movies", "Locations": []string{"/media/other"}},
	}}
	url := app.serve(t)

	c := runJellyfin(t, adminSecret(), jellyfinConfig(url,
		mediaserversv1alpha1.JellyfinLibrary{Name: "Movies", CollectionType: "movies", Paths: []string{"/media/movies"}}))

	assert.NotEqual(t, conditionTrue, jellyfinSynced(t, c))
	assert.False(t, app.saw("DELETE", "/Library/VirtualFolders/Paths"), "removing a folder from one of two libraries is a guess")
}

func TestReconcileMissingSecretDoesNotContactApp(t *testing.T) {
	app := &fakeJellyfin{setupComplete: true}
	url := app.serve(t)

	c := runJellyfin(t, jellyfinConfig(url)) // no Secret

	assert.False(t, app.saw("GET", "/System/Info/Public"), "contacted the app despite failing to resolve admin credentials")
	assert.NotEqual(t, conditionTrue, jellyfinSynced(t, c), "Synced = True despite missing credentials")
}

func TestJellyfinObserveNeverRunsSetupOrWritesConfiguration(t *testing.T) {
	for _, tt := range []struct {
		name                  string
		initialized, matching bool
		failure, reason       string
	}{
		{"fresh", false, false, "", "DriftDetected"},
		{"drift", true, false, "", "DriftDetected"},
		{"matching", true, true, "", "Observed"},
		{"empty spec", true, true, "", "Observed"},
		{"library failure", true, true, "/Library/VirtualFolders", "SyncFailed"},
		{"encoding failure", true, true, "/System/Configuration/encoding", "SyncFailed"},
		{"read failure", true, true, "/System/Configuration", "SyncFailed"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			app := &fakeJellyfin{setupComplete: tt.initialized, failure: tt.failure}
			if tt.matching {
				app.existingLibs = []map[string]any{{"Name": "Movies", "CollectionType": "movies", "Locations": []string{"/media/movies"}}}
				app.config = map[string]any{"ServerName": "Media", "Unmanaged": true}
			}
			cfg := jellyfinConfig(app.serve(t),
				mediaserversv1alpha1.JellyfinLibrary{Name: "Movies", CollectionType: "movies", Paths: []string{"/media/movies/"}})
			cfg.Spec.Server = &mediaserversv1alpha1.JellyfinServer{ServerName: "Media"}
			if tt.name == "empty spec" {
				cfg.Spec.Server = nil
				cfg.Spec.Libraries = nil
			}
			if tt.name == "encoding failure" {
				cfg.Spec.Encoding = &mediaserversv1alpha1.JellyfinEncoding{}
			}
			policy := "observe"
			cfg.Spec.Reconcile = &commonv1alpha1.ReconcileConfig{DriftPolicy: policy}
			c := runJellyfin(t, adminSecret(), cfg)
			require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: cfg.Name, Namespace: cfg.Namespace}, cfg))
			app.mu.Lock()
			defer app.mu.Unlock()
			for _, req := range app.requests {
				assert.True(t, strings.HasPrefix(req, "GET ") || req == "POST /Users/AuthenticateByName", "observe wrote: %s", req)
			}
			if assert.NotNil(t, cfg.Status.Initialized, "initialization status changed") {
				assert.Equal(t, tt.initialized, *cfg.Status.Initialized, "initialization status changed")
			}
			require.True(t, slices.ContainsFunc(cfg.Status.Conditions, func(condition metav1.Condition) bool {
				return condition.Type == "Synced" && condition.Reason == tt.reason
			}), "want %s, got %+v", tt.reason, cfg.Status.Conditions)
		})
	}
}

// Jellyfin reads settings back in PascalCase; camelCase keys never matched and
// rewrote the configuration on every pass.
func TestJellyfinSettingsUseJellyfinNames(t *testing.T) {
	out, err := jellyfinSettings(&mediaserversv1alpha1.JellyfinEncoding{
		H265Crf: new(int32(28)), EncoderPreset: "veryfast", EnableThrottling: new(true),
	})
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"H265Crf": float64(28), "EncoderPreset": "veryfast", "EnableThrottling": true}, out)
}

func TestJellyfinSettingsRenameNestedSettings(t *testing.T) {
	out, err := jellyfinSettings(&mediaserversv1alpha1.JellyfinServer{
		EnableMetrics: new(true),
		TrickplayOptions: &mediaserversv1alpha1.JellyfinTrickplay{
			EnableHwAcceleration: new(true), WidthResolutions: []int32{320, 640},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, map[string]any{
		"EnableMetrics": true,
		"TrickplayOptions": map[string]any{
			"EnableHwAcceleration": true, "WidthResolutions": []any{float64(320), float64(640)},
		},
	}, out)
}

func TestReconcileWritesJellyfinNetworkSettings(t *testing.T) {
	app := &fakeJellyfin{setupComplete: true}
	url := app.serve(t)
	cfg := jellyfinConfig(url)
	cfg.Spec.Network = &mediaserversv1alpha1.JellyfinNetwork{KnownProxies: []string{"10.0.0.1"}}

	c := runJellyfin(t, adminSecret(), cfg)

	assert.True(t, app.saw("POST", "/System/Configuration/network"), "network settings were never written")
	assert.Equal(t, conditionTrue, jellyfinSynced(t, c))
}

func TestReconcileWritesJellyfinBranding(t *testing.T) {
	app := &fakeJellyfin{setupComplete: true}
	url := app.serve(t)
	cfg := jellyfinConfig(url)
	cfg.Spec.Branding = &mediaserversv1alpha1.JellyfinBranding{LoginDisclaimer: new("Members only"), SplashscreenEnabled: new(true)}

	c := runJellyfin(t, adminSecret(), cfg)

	assert.True(t, app.saw("POST", "/System/Configuration/branding"), "branding was never written")
	assert.Equal(t, conditionTrue, jellyfinSynced(t, c))
}

func TestJellyfinSettingsNameBrandingAsJellyfinDoes(t *testing.T) {
	out, err := jellyfinSettings(&mediaserversv1alpha1.JellyfinBranding{CustomCSS: new(""), SplashscreenEnabled: new(false)})
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"CustomCss": "", "SplashscreenEnabled": false}, out, "an empty customCss clears it")
}

func jellyfinWithAPIKey(url string) (*mediaserversv1alpha1.Jellyfin, *corev1.Secret) {
	cfg := jellyfinConfig(url)
	cfg.Spec.Connection.APIKeySecretRef = &commonv1alpha1.SecretKeyRef{Name: "jellyfin-admin", Key: "api-key"}
	secret := adminSecret()
	secret.Data["api-key"] = []byte("jf-key")
	return cfg, secret
}

// Signing in creates a session in Jellyfin on every pass; an API key doesn't.
func TestJellyfinUsesItsAPIKeyInsteadOfSigningIn(t *testing.T) {
	app := &fakeJellyfin{setupComplete: true, apiKey: "jf-key"}
	cfg, secret := jellyfinWithAPIKey(app.serve(t))
	cfg.Spec.Libraries = []mediaserversv1alpha1.JellyfinLibrary{{Name: "Movies", CollectionType: "movies", Paths: []string{"/data/movies"}}}

	c := runJellyfin(t, secret, cfg)

	assert.False(t, app.saw("POST", "/Users/AuthenticateByName"), "signed in despite having an API key")
	assert.True(t, app.saw("GET", "/System/Info"), "never checked the API key")
	assert.Contains(t, app.authFor("/Library/VirtualFolders"), `Token="jf-key"`)
	assert.Equal(t, conditionTrue, jellyfinSynced(t, c), "Synced")
}

func TestJellyfinReportsARefusedAPIKeyAsUnauthorized(t *testing.T) {
	app := &fakeJellyfin{setupComplete: true, apiKey: "another-key"}
	cfg, secret := jellyfinWithAPIKey(app.serve(t))

	c := runJellyfin(t, secret, cfg)

	var got mediaserversv1alpha1.Jellyfin
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "jellyfin", Namespace: "media"}, &got))
	ready := meta.FindStatusCondition(got.Status.Conditions, "Ready")
	require.NotNil(t, ready)
	assert.Equal(t, "Unauthorized", ready.Reason)
	assert.Contains(t, ready.Message, "checking the API key")
	assert.NotNil(t, meta.FindStatusCondition(got.Status.Conditions, "Stalled"), "a refused key won't fix itself")
	assert.False(t, app.saw("GET", "/System/Configuration"), "carried on with a refused key")
}

func TestJellyfinLeavesOutSettingsItsVersionLacks(t *testing.T) {
	for _, tt := range []struct {
		version string
		sent    bool
	}{{"10.11.3", false}, {"12.4.0", true}, {"", true}} {
		app := &fakeJellyfin{setupComplete: true, version: tt.version}
		cfg := jellyfinConfig(app.serve(t))
		cfg.Spec.Encoding = &mediaserversv1alpha1.JellyfinEncoding{HardwareAccelerationType: "vaapi", SubtitleExtractionTimeoutMinutes: 45}

		c := runJellyfin(t, cfg, adminSecret())

		body := app.postedTo("/System/Configuration/encoding")
		assert.Equal(t, "vaapi", body["HardwareAccelerationType"], "version %q: the settings it has were held back", tt.version)
		_, sent := body["SubtitleExtractionTimeoutMinutes"]
		assert.Equal(t, tt.sent, sent, "version %q", tt.version)
		var got mediaserversv1alpha1.Jellyfin
		require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "jellyfin", Namespace: "media"}, &got))
		synced := meta.FindStatusCondition(got.Status.Conditions, "Synced")
		require.NotNil(t, synced)
		if tt.sent {
			assert.Equal(t, metav1.ConditionTrue, synced.Status, "version %q: %s", tt.version, synced.Message)
			continue
		}
		assert.Equal(t, "SyncFailed", synced.Reason)
		assert.Contains(t, synced.Message,
			"encoding.subtitleExtractionTimeoutMinutes needs Jellyfin 12.0 or later, and this server runs 10.11.3")
	}
}

func TestVersionBefore(t *testing.T) {
	for _, tt := range []struct {
		a, b string
		want bool
	}{
		{"10.11.3", "12.0", true},
		{"12.0.0", "12.0", false},
		{"12.4", "12.0", false},
		{"12.0", "12.0.1", true},
		{"9.9", "10.0", true},
	} {
		assert.Equal(t, tt.want, versionBefore(tt.a, tt.b), "%s before %s", tt.a, tt.b)
	}
}

func TestJellyfinLibraryOptionsUseJellyfinNames(t *testing.T) {
	got := jellyfinLibraryOptions(mediaserversv1alpha1.JellyfinLibrary{
		Name: "Movies", CollectionType: "movies", Paths: []string{"/media/movies"},
		MetadataSavers: []string{"Nfo"}, EnableLUFSScan: new(true), SubtitleDownloadLanguages: []string{"eng"},
		AllowEmbeddedSubtitles: "AllowText", SeasonZeroDisplayName: "Extras",
	})
	assert.Equal(t, map[string]any{
		"MetadataSavers": []any{"Nfo"}, "EnableLUFSScan": true, "SubtitleDownloadLanguages": []any{"eng"},
		"AllowEmbeddedSubtitles": "AllowText", "SeasonZeroDisplayName": "Extras",
	}, got, "the name, type and folders are not options")
}

func jellyfinOffered(names ...string) []any {
	out := make([]any, 0, len(names))
	for _, n := range names {
		on := !strings.HasPrefix(n, "-")
		out = append(out, map[string]any{"Name": strings.TrimPrefix(n, "-"), "DefaultEnabled": on})
	}
	return out
}

func asJSON(t *testing.T, v any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(v)
	require.NoError(t, err)
	var out map[string]any
	require.NoError(t, json.Unmarshal(raw, &out))
	return out
}

// Jellyfin keeps the subtitle fetchers a library disables; the declared ones
// come first in the order and the rest, installed or not, are disabled.
func TestJellyfinFetchersDisableWhatIsNotDeclared(t *testing.T) {
	lib := mediaserversv1alpha1.JellyfinLibrary{SubtitleFetchers: []string{"Podnapisi"}}
	current := map[string]any{
		"SubtitleFetcherOrder":     []any{"Open Subtitles", "Podnapisi"},
		"DisabledSubtitleFetchers": []any{"Podnapisi"},
	}
	available := map[string]any{"SubtitleFetchers": jellyfinOffered("Open Subtitles", "Podnapisi", "Addic7ed")}
	got := asJSON(t, jellyfinFetchers(lib, current, available))
	assert.Equal(t, map[string]any{
		"SubtitleFetcherOrder":     []any{"Podnapisi", "Open Subtitles", "Addic7ed"},
		"DisabledSubtitleFetchers": []any{"Open Subtitles", "Addic7ed"},
	}, got)

	maps.Copy(current, got)
	assert.Equal(t, got, asJSON(t, jellyfinFetchers(lib, current, available)), "what was written is kept")
	current["DisabledSubtitleFetchers"] = []any{"Addic7ed", "Open Subtitles"}
	assert.Equal(t, []any{"Addic7ed", "Open Subtitles"}, asJSON(t, jellyfinFetchers(lib, current, available))["DisabledSubtitleFetchers"],
		"the disabled list's order means nothing to Jellyfin")
}

// Once a type has options, Jellyfin uses only the fetchers they enable, so a
// new type's undeclared list starts from what Jellyfin enables without them.
func TestJellyfinTypeOptionsKeepWhatIsNotDeclared(t *testing.T) {
	lib := mediaserversv1alpha1.JellyfinLibrary{TypeOptions: []mediaserversv1alpha1.JellyfinTypeOptions{
		{Type: "Series", MetadataFetchers: []string{"TheTVDB"}},
		{Type: "Episode", ImageFetchers: []string{"Screen Grabber"}},
	}}
	current := map[string]any{"TypeOptions": []any{map[string]any{
		"Type": "Series", "MetadataFetchers": []any{"TheMovieDb"}, "MetadataFetcherOrder": []any{"TheMovieDb", "TheTVDB"},
		"ImageFetchers": []any{"TheMovieDb"}, "ImageFetcherOrder": []any{"TheMovieDb"}, "ImageOptions": []any{map[string]any{"Type": "Backdrop", "Limit": 3.0}},
	}}}
	available := map[string]any{"TypeOptions": []any{
		map[string]any{"Type": "Series", "MetadataFetchers": jellyfinOffered("TheMovieDb", "TheTVDB"), "ImageFetchers": jellyfinOffered("TheMovieDb")},
		map[string]any{"Type": "Episode", "MetadataFetchers": jellyfinOffered("TheMovieDb", "-TheTVDB"), "ImageFetchers": jellyfinOffered("TheMovieDb", "Screen Grabber")},
	}}
	got := asJSON(t, jellyfinFetchers(lib, current, available))
	assert.Equal(t, []any{
		map[string]any{
			"Type": "Series", "MetadataFetchers": []any{"TheTVDB"}, "MetadataFetcherOrder": []any{"TheTVDB", "TheMovieDb"},
			"ImageFetchers": []any{"TheMovieDb"}, "ImageFetcherOrder": []any{"TheMovieDb"}, "ImageOptions": []any{map[string]any{"Type": "Backdrop", "Limit": 3.0}},
		},
		map[string]any{
			"Type": "Episode", "MetadataFetchers": []any{"TheMovieDb"}, "MetadataFetcherOrder": []any{"TheMovieDb", "TheTVDB"},
			"ImageFetchers": []any{"Screen Grabber"}, "ImageFetcherOrder": []any{"Screen Grabber", "TheMovieDb"},
		},
	}, got["TypeOptions"])
	assert.Len(t, current["TypeOptions"], 1, "the current options are left as they were")
}

// Jellyfin's options are arrays it doesn't expect to be null.
func TestJellyfinFetchersSendEmptyListsNotNull(t *testing.T) {
	got := asJSON(t, jellyfinFetchers(mediaserversv1alpha1.JellyfinLibrary{
		LyricFetchers: []string{"LrcLib"},
		TypeOptions:   []mediaserversv1alpha1.JellyfinTypeOptions{{Type: "Audio", MetadataFetchers: []string{"MusicBrainz"}}},
	}, nil, map[string]any{}))
	assert.Equal(t, []any{}, got["DisabledLyricFetchers"])
	assert.Equal(t, []any{map[string]any{
		"Type": "Audio", "MetadataFetchers": []any{"MusicBrainz"}, "MetadataFetcherOrder": []any{"MusicBrainz"},
		"ImageFetchers": []any{}, "ImageFetcherOrder": []any{},
	}}, got["TypeOptions"])
}

func TestReconcileCreatesALibraryWithItsFetchers(t *testing.T) {
	app := &fakeJellyfin{setupComplete: true, available: map[string]any{
		"MetadataReaders": jellyfinOffered("Nfo"),
		"TypeOptions": []any{map[string]any{
			"Type": "Movie", "MetadataFetchers": jellyfinOffered("TheMovieDb", "The Open Movie Database"),
			"ImageFetchers": jellyfinOffered("TheMovieDb", "-Embedded Image Extractor"),
		}},
	}}
	url := app.serve(t)

	runJellyfin(t, adminSecret(), jellyfinConfig(url, mediaserversv1alpha1.JellyfinLibrary{
		Name: "Movies", CollectionType: "movies", Paths: []string{"/media/movies"},
		LocalMetadataReaders: []string{"Nfo"},
		TypeOptions:          []mediaserversv1alpha1.JellyfinTypeOptions{{Type: "Movie", MetadataFetchers: []string{"The Open Movie Database"}}},
	}))

	app.mu.Lock()
	assert.Equal(t, "isNewLibrary=false&libraryContentType=movies", app.availableQuery,
		"the defaults a library without type options falls back on")
	app.mu.Unlock()
	options, _ := app.postedTo("/Library/VirtualFolders")["LibraryOptions"].(map[string]any)
	require.NotNil(t, options, "the library was not created with options")
	assert.Equal(t, []any{"Nfo"}, options["LocalMetadataReaderOrder"])
	assert.Equal(t, []any{}, options["DisabledLocalMetadataReaders"])
	assert.Equal(t, []any{map[string]any{
		"Type": "Movie", "MetadataFetchers": []any{"The Open Movie Database"},
		"MetadataFetcherOrder": []any{"The Open Movie Database", "TheMovieDb"},
		"ImageFetchers":        []any{"TheMovieDb"}, "ImageFetcherOrder": []any{"TheMovieDb", "Embedded Image Extractor"},
	}}, options["TypeOptions"])
}

func TestJellyfinClientSaysWhyItCantConnect(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	reconcilerWith := func(objs ...client.Object) *JellyfinReconciler {
		return &JellyfinReconciler{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()}
	}

	_, err := reconcilerWith().connect(context.Background(), jellyfinConfig("http://jellyfin:8096"))
	require.Error(t, err)
	reason, _ := reconciler.Classify(err)
	assert.Equal(t, reconciler.ReasonSecretNotFound, reason)

	_, err = reconcilerWith(adminSecret()).connect(context.Background(), jellyfinConfig("ftp://jellyfin"))
	require.Error(t, err)
	reason, _ = reconciler.Classify(err)
	assert.Equal(t, reconciler.ReasonInvalidConfig, reason)

	_, err = reconcilerWith(adminSecret()).connect(context.Background(), jellyfinConfig("http://jellyfin.invalid:8096"))
	require.Error(t, err, "connect asks Jellyfin whether its setup is complete")
	reason, _ = reconciler.Classify(err)
	assert.Equal(t, reconciler.ReasonAppUnreachable, reason)
}
