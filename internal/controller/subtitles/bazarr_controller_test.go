package subtitles

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	pvrv1alpha1 "github.com/kyleseneker/arrchitect/api/pvr/v1alpha1"
	subtitlesv1alpha1 "github.com/kyleseneker/arrchitect/api/subtitles/v1alpha1"
	bazarrclient "github.com/kyleseneker/arrchitect/internal/client/bazarr"
	"github.com/kyleseneker/arrchitect/internal/engine"
)

const conditionTrue = "True"

// fakeBazarr captures posted form bodies. Bazarr takes form-encoded settings
// rather than JSON, so the assertions are about which form keys were sent.
type fakeBazarr struct {
	mu       sync.Mutex
	forms    []url.Values
	paths    []string
	profiles []map[string]any
	// settings is what Bazarr reports as its settings, by section.
	settings map[string]any
}

// formValue returns the first value posted for key across all requests.
func (f *fakeBazarr) formValue(key string) (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, form := range f.forms {
		if form.Has(key) {
			return form.Get(key), true
		}
	}
	return "", false
}

// anyKeyContains reports whether any posted form key contains the substring.
func (f *fakeBazarr) anyKeyContains(sub string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, form := range f.forms {
		for k := range form {
			if strings.Contains(k, sub) {
				return true
			}
		}
	}
	return false
}

func (f *fakeBazarr) sawPath(p string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Contains(f.paths, p)
}

// routableAddr returns a non-loopback IPv4 address. The operator's HTTP client
// blocks loopback and link-local as SSRF protection, so a fake on 127.0.0.1 is
// unreachable by design and assertions against it pass for the wrong reason.
func routableAddr(t *testing.T) string {
	t.Helper()
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		t.Skipf("cannot enumerate interfaces: %v", err)
	}
	for _, a := range addrs {
		ipnet, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		ip := ipnet.IP.To4()
		if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
			continue
		}
		return ip.String()
	}
	t.Skip("no routable non-loopback IPv4 address on this host")
	return ""
}

func (f *fakeBazarr) serve(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", net.JoinHostPort(routableAddr(t), "0"))
	if err != nil {
		t.Skipf("cannot bind a routable listener: %v", err)
	}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		f.mu.Lock()
		f.paths = append(f.paths, r.URL.Path)
		if len(r.PostForm) > 0 {
			f.forms = append(f.forms, r.PostForm)
		}
		f.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet && r.URL.Path == "/api/system/settings" && f.settings != nil {
			_ = json.NewEncoder(w).Encode(f.settings)
			return
		}
		if r.URL.Path == "/api/system/languages/profiles" && f.profiles != nil {
			_ = json.NewEncoder(w).Encode(f.profiles)
			return
		}
		if r.URL.Path == "/api/system/languages" {
			_ = json.NewEncoder(w).Encode([]map[string]any{{"code2": "en", "enabled": false}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
	}))
	_ = srv.Listener.Close()
	srv.Listener = ln
	srv.Start()
	t.Cleanup(srv.Close)
	return srv.URL
}

func bazarrSecrets() *corev1.Secret {
	return &corev1.Secret{
		Name: "bazarr-secrets", Namespace: "media",
		Data: map[string][]byte{
			"bazarr-api-key": []byte("bazarr-key"),
			"sonarr-api-key": []byte("sonarr-key"),
		},
	}
}

func bazarrConfig(addr string) *subtitlesv1alpha1.Bazarr {
	return &subtitlesv1alpha1.Bazarr{
		Name: "bazarr", Namespace: "media",
		Spec: subtitlesv1alpha1.BazarrSpec{
			Connection: commonv1alpha1.AppConnection{
				URL:             addr,
				APIKeySecretRef: commonv1alpha1.SecretKeyRef{Name: "bazarr-secrets", Key: "bazarr-api-key"},
			},
		},
	}
}

func runBazarr(t *testing.T, cfg *subtitlesv1alpha1.Bazarr, objs ...client.Object) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme), "adding client-go scheme")
	require.NoError(t, subtitlesv1alpha1.AddToScheme(scheme), "adding subtitles scheme")
	require.NoError(t, pvrv1alpha1.AddToScheme(scheme), "adding pvr scheme")
	b := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&subtitlesv1alpha1.Bazarr{}).WithObjects(cfg)
	for _, o := range objs {
		b = b.WithObjects(o)
	}
	c := b.Build()
	r := &BazarrReconciler{Client: c, Scheme: scheme, Recorder: events.NewFakeRecorder(50)}
	_, err := r.Reconcile(context.Background(), ctrl.Request{
		Name: "bazarr", Namespace: "media",
	})
	require.NoError(t, err, "reconcile")
	return c
}

func bazarrCondition(t *testing.T, c client.Client, condType string) string {
	t.Helper()
	var cfg subtitlesv1alpha1.Bazarr
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "bazarr", Namespace: "media"}, &cfg), "fetching config")
	for _, cond := range cfg.Status.Conditions {
		if cond.Type == condType {
			return string(cond.Status)
		}
	}
	return ""
}

// The CR names a Secret for the Sonarr API key. Bazarr must receive the
// resolved value under its own "apikey" form field, and must never receive the
// "apiKeySecretRef" field name, which would send Bazarr a Kubernetes reference
// it cannot use in place of a credential.
func TestBazarrSubstitutesResolvedAPIKeyIntoForm(t *testing.T) {
	app := &fakeBazarr{}
	addr := app.serve(t)

	cfg := bazarrConfig(addr)
	port := int32(8989)
	cfg.Spec.SonarrConnection = subtitlesv1alpha1.BazarrAppConnection{
		Host:            "sonarr",
		Port:            &port,
		APIKeySecretRef: &commonv1alpha1.SecretKeyRef{Name: "bazarr-secrets", Key: "sonarr-api-key"},
	}
	runBazarr(t, cfg, bazarrSecrets())

	got, ok := app.formValue("settings-sonarr-apikey")
	require.True(t, ok, "sonarr api key was never sent")
	assert.Equal(t, "sonarr-key", got, "settings-sonarr-apikey is not the resolved secret value")
	assert.False(t, app.anyKeyContains("apiKeySecretRef"), "sent the Kubernetes secret reference to Bazarr instead of the resolved value")
}

// Bazarr enables languages from repeated languages-enabled values; a
// settings-general-enabled_languages key is stored but never used.
func TestBazarrSendsEnabledLanguagesAsRepeatedValues(t *testing.T) {
	app := &fakeBazarr{}
	addr := app.serve(t)

	cfg := bazarrConfig(addr)
	cfg.Spec.Languages = &subtitlesv1alpha1.BazarrLanguages{Enabled: []string{"en", "de"}}
	runBazarr(t, cfg, bazarrSecrets())

	app.mu.Lock()
	defer app.mu.Unlock()
	require.NotEmpty(t, app.forms, "nothing was written")
	assert.Equal(t, []string{"en", "de"}, app.forms[0]["languages-enabled"])
	assert.NotContains(t, app.forms[0], "settings-general-enabled_languages")
}

func TestBazarrReportsSyncedOnHealthyApp(t *testing.T) {
	app := &fakeBazarr{}
	addr := app.serve(t)

	c := runBazarr(t, bazarrConfig(addr), bazarrSecrets())

	assert.True(t, app.sawPath("/api/system/health"), "never health-checked the app")
	assert.Equal(t, conditionTrue, bazarrCondition(t, c, "Synced"), "Synced")
}

func TestBazarrMissingSecretDoesNotContactApp(t *testing.T) {
	app := &fakeBazarr{}
	addr := app.serve(t)

	c := runBazarr(t, bazarrConfig(addr)) // no Secret

	assert.False(t, app.sawPath("/api/system/health"), "contacted the app despite failing to resolve its API key")
	assert.NotEqual(t, conditionTrue, bazarrCondition(t, c, "Synced"), "Synced = True despite a missing secret")
}

func TestBazarrObserveReportsDriftWithoutWriting(t *testing.T) {
	app := &fakeBazarr{}
	cfg := bazarrConfig(app.serve(t))
	cfg.Spec.General = &subtitlesv1alpha1.BazarrGeneral{UseSonarr: new(true)}
	policy := "observe"
	cfg.Spec.Reconcile = &commonv1alpha1.ReconcileConfig{DriftPolicy: policy}
	c := runBazarr(t, cfg, bazarrSecrets())

	assert.True(t, app.sawPath("/api/system/settings"), "observe must read the settings")
	assert.Empty(t, app.forms, "observe must not write")
	assert.Equal(t, string(metav1.ConditionFalse), bazarrCondition(t, c, "Synced"))
}

// Writing every section on every pass reconnected Bazarr to Sonarr and Radarr
// each time. Only settings that read back differently may be sent.
func TestBazarrChangesOnlyWhatDiffers(t *testing.T) {
	desired := url.Values{
		"settings-general-use_sonarr":        {"true"},
		"settings-general-minimum_score":     {"90"},
		"settings-general-enabled_providers": {"podnapisi", "tvsubtitles"},
		"languages-enabled":                  {"es", "en"},
		"settings-sonarr-ip":                 {"10.0.0.5"},
		"languages-profiles": {`[{"profileId":1,"name":"English","cutoff":null,"originalFormat":false,` +
			`"items":[{"id":1,"language":"en","hi":"False","forced":"False","audio_exclude":"False"}]}]`},
	}
	settings := map[string]any{
		"general": map[string]any{
			"use_sonarr": true, "minimum_score": float64(90),
			"enabled_providers": []any{"tvsubtitles", "podnapisi"},
		},
		"sonarr": map[string]any{"ip": "127.0.0.1"},
	}
	profiles := []map[string]any{{
		"profileId": float64(1), "name": "English", "cutoff": nil, "originalFormat": float64(0), "tag": nil,
		"items": []any{map[string]any{"id": float64(1), "language": "en", "hi": "False", "forced": "False", "audio_exclude": "False"}},
	}}

	changes := bazarrChanges(desired, settings, profiles, []string{"en", "es"})

	assert.Equal(t, url.Values{"settings-sonarr-ip": {"10.0.0.5"}}, changes,
		"typed values, provider and language order, originalFormat 0 and Bazarr's own tag key are not changes")

	changes = bazarrChanges(url.Values{"languages-enabled": {"en", "es"}}, settings, nil, []string{"en"})
	assert.Equal(t, url.Values{"languages-enabled": {"en", "es"}}, changes, "a language Bazarr hasn't enabled is a change")
}

func TestBazarrPathMappingsUseBazarrsFromToForm(t *testing.T) {
	form := bazarrPathMappings(&subtitlesv1alpha1.BazarrGeneral{
		PathMappings:      []subtitlesv1alpha1.BazarrPathMapping{{RemotePath: "/tv", LocalPath: "/media/tv"}, {RemotePath: "/anime", LocalPath: "/media/anime"}},
		PathMappingsMovie: []subtitlesv1alpha1.BazarrPathMapping{},
	})
	assert.Equal(t, url.Values{
		"settings-general-path_mappings":       {"/tv,/media/tv", "/anime,/media/anime"},
		"settings-general-path_mappings_movie": {""},
	}, form)

	settings := map[string]any{"general": map[string]any{
		"path_mappings":       []any{[]any{"/anime", "/media/anime"}, []any{"/tv", "/media/tv"}},
		"path_mappings_movie": []any{},
	}}
	assert.Empty(t, bazarrChanges(form, settings, nil, nil), "Bazarr stores mappings as pairs, and an empty list as empty")
}

func TestBazarrDefaultProfilesResolveByName(t *testing.T) {
	spec := subtitlesv1alpha1.BazarrSpec{
		General: &subtitlesv1alpha1.BazarrGeneral{SeriesDefaultProfile: "English", MovieDefaultProfile: "Spanish"},
		Languages: &subtitlesv1alpha1.BazarrLanguages{Profiles: []subtitlesv1alpha1.BazarrLanguageProfile{
			{ProfileId: new(int32(3)), Name: "English"},
		}},
	}
	form := bazarrclient.StructToFormData("general", spec.General)
	app := &fakeBazarr{profiles: []map[string]any{{"profileId": float64(7), "name": "Spanish"}}}
	bc := newTestBazarrClient(t, app.serve(t))

	require.NoError(t, resolveBazarrDefaultProfiles(context.Background(), bc, spec, form))
	assert.Equal(t, "3", form.Get("settings-general-serie_default_profile"), "a declared profile resolves without asking Bazarr")
	assert.Equal(t, "7", form.Get("settings-general-movie_default_profile"))
	assert.Equal(t, "true", form.Get("settings-general-serie_default_enabled"))
	assert.Equal(t, "true", form.Get("settings-general-movie_default_enabled"))

	spec.General.MovieDefaultProfile = "French"
	form = bazarrclient.StructToFormData("general", spec.General)
	err := resolveBazarrDefaultProfiles(context.Background(), bc, spec, form)
	require.ErrorContains(t, err, `movieDefaultProfile: Bazarr has no language profile named "French"`)
	assert.False(t, form.Has("settings-general-movie_default_profile"), "an unknown name is never sent")
	assert.Equal(t, "3", form.Get("settings-general-serie_default_profile"))

	app.profiles = append(app.profiles, map[string]any{"profileId": float64(8), "name": "Spanish"})
	spec.General.MovieDefaultProfile = "Spanish"
	form = bazarrclient.StructToFormData("general", spec.General)
	err = resolveBazarrDefaultProfiles(context.Background(), bc, spec, form)
	require.ErrorContains(t, err, `movieDefaultProfile: Bazarr has more than one language profile named "Spanish"`)
	assert.False(t, form.Has("settings-general-movie_default_profile"), "an ambiguous name never picks one of the profiles")
}

func newTestBazarrClient(t *testing.T, addr string) *bazarrclient.Client {
	t.Helper()
	hc, err := engine.NewHTTPClient(addr, engine.AuthFormEncoded, engine.WithAPIKey("key"), engine.WithAPIKeyHeader("X-API-KEY"))
	require.NoError(t, err)
	return bazarrclient.NewClient(hc)
}

func TestBazarrProviderSecretSettingsReadFromASecret(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	r := &BazarrReconciler{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(bazarrSecrets()).Build()}
	ref := commonv1alpha1.SecretKeyRef{Name: "bazarr-secrets", Key: "sonarr-api-key"}

	form, err := r.bazarrProviderForm(context.Background(), "media", []subtitlesv1alpha1.BazarrProvider{{
		Name: "opensubtitlescom", Settings: []commonv1alpha1.ConfigField{
			{Name: "username", Value: &commonv1alpha1.FieldValue{Raw: []byte(`"me"`)}},
			{Name: "password", ValueFrom: commonv1alpha1.ValueSource{SecretKeyRef: ref}},
		},
	}})
	require.NoError(t, err)
	assert.Equal(t, "me", form.Get("settings-opensubtitlescom-username"))
	assert.Equal(t, "sonarr-key", form.Get("settings-opensubtitlescom-password"))
}

// A reference sets the whole address, so one at the root clears a base path
// Bazarr has from before.
func TestBazarrAppRefUsesTheReferencedConnection(t *testing.T) {
	app := &fakeBazarr{settings: map[string]any{"radarr": map[string]any{"base_url": "/radarr"}}}
	cfg := bazarrConfig(app.serve(t))
	cfg.Spec.SonarrConnection = subtitlesv1alpha1.BazarrAppConnection{AppRef: &commonv1alpha1.AppRef{Kind: "Sonarr", Name: "sonarr"}}
	cfg.Spec.RadarrConnection = subtitlesv1alpha1.BazarrAppConnection{AppRef: &commonv1alpha1.AppRef{Kind: "Radarr", Name: "radarr"}}
	key := commonv1alpha1.SecretKeyRef{Name: "bazarr-secrets", Key: "sonarr-api-key"}
	sonarr := &pvrv1alpha1.Sonarr{Name: "sonarr", Namespace: "media", Spec: pvrv1alpha1.SonarrSpec{
		Connection: commonv1alpha1.AppConnection{URL: "https://sonarr.media.svc/sonarr", APIKeySecretRef: key},
	}}
	radarr := &pvrv1alpha1.Radarr{Name: "radarr", Namespace: "media", Spec: pvrv1alpha1.RadarrSpec{
		Connection: commonv1alpha1.AppConnection{URL: "http://radarr.media.svc:7878", APIKeySecretRef: key},
	}}

	runBazarr(t, cfg, bazarrSecrets(), sonarr, radarr)

	for key, want := range map[string]string{
		"settings-sonarr-ip": "sonarr.media.svc", "settings-sonarr-port": "443", "settings-sonarr-ssl": "true",
		"settings-sonarr-base_url": "/sonarr", "settings-sonarr-apikey": "sonarr-key",
		"settings-radarr-ip": "radarr.media.svc", "settings-radarr-port": "7878", "settings-radarr-ssl": "false",
		"settings-radarr-base_url": "",
	} {
		got, ok := app.formValue(key)
		require.True(t, ok, "%s was never sent", key)
		assert.Equal(t, want, got, key)
	}
}

func TestBazarrMissingAppRefSkipsOnlyThatConnection(t *testing.T) {
	app := &fakeBazarr{}
	cfg := bazarrConfig(app.serve(t))
	cfg.Spec.SonarrConnection = subtitlesv1alpha1.BazarrAppConnection{AppRef: &commonv1alpha1.AppRef{Kind: "Sonarr", Name: "sonarr"}}
	cfg.Spec.Languages = &subtitlesv1alpha1.BazarrLanguages{Enabled: []string{"en"}}

	c := runBazarr(t, cfg, bazarrSecrets())

	_, sent := app.formValue("languages-enabled")
	assert.True(t, sent, "the rest of Bazarr was held back too")
	assert.False(t, app.anyKeyContains("settings-sonarr-"), "a connection was sent without its reference")
	var got subtitlesv1alpha1.Bazarr
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "bazarr", Namespace: "media"}, &got))
	synced := meta.FindStatusCondition(got.Status.Conditions, "Synced")
	require.NotNil(t, synced)
	assert.Contains(t, synced.Message, `sonarrConnection: appRef: Sonarr "sonarr" not found`)
}

func bazarrNotificationSettings() map[string]any {
	return map[string]any{"notifications": map[string]any{"providers": []any{
		map[string]any{"name": "Discord", "enabled": false, "url": nil},
		map[string]any{"name": "Telegram", "enabled": true, "url": "tgram://token/chat"},
	}}}
}

func TestBazarrSetsANotificationProvidersURLFromItsSecret(t *testing.T) {
	app := &fakeBazarr{settings: bazarrNotificationSettings()}
	cfg := bazarrConfig(app.serve(t))
	cfg.Spec.Notifications = []subtitlesv1alpha1.BazarrNotification{
		{Name: "Discord", Enabled: new(true), URLSecretRef: &commonv1alpha1.SecretKeyRef{Name: "hooks", Key: "discord"}},
		{Name: "Telegram", Enabled: new(true)},
	}
	hooks := &corev1.Secret{Name: "hooks", Namespace: "media", Data: map[string][]byte{"discord": []byte("discord://id/token")}}
	runBazarr(t, cfg, bazarrSecrets(), hooks)

	app.mu.Lock()
	defer app.mu.Unlock()
	var items []string
	for _, form := range app.forms {
		items = append(items, form["notifications-providers"]...)
	}
	assert.Equal(t, []string{`{"enabled":true,"name":"Discord","url":"discord://id/token"}`}, items,
		"only the provider that differs is sent, with its URL from the Secret")
}

func TestBazarrReportsANotificationProviderItLacks(t *testing.T) {
	app := &fakeBazarr{settings: bazarrNotificationSettings()}
	cfg := bazarrConfig(app.serve(t))
	cfg.Spec.Notifications = []subtitlesv1alpha1.BazarrNotification{{Name: "Carrier Pigeon", Enabled: new(true)}}
	c := runBazarr(t, cfg, bazarrSecrets())

	assert.Equal(t, string(metav1.ConditionFalse), bazarrCondition(t, c, "Synced"))
	assert.Equal(t, conditionTrue, bazarrCondition(t, c, "Ready"), "a missing provider doesn't make Bazarr unreachable")
	_, sent := app.formValue("notifications-providers")
	assert.False(t, sent)
}
