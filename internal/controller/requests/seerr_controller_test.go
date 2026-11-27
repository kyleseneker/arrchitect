package requests

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
	pvrv1alpha1 "github.com/kyleseneker/arrchitect/api/pvr/v1alpha1"
	requestsv1alpha1 "github.com/kyleseneker/arrchitect/api/requests/v1alpha1"
	"github.com/kyleseneker/arrchitect/internal/controller/internal/contract"
	"github.com/kyleseneker/arrchitect/internal/controller/reconciler"
	"github.com/kyleseneker/arrchitect/internal/metrics"
	"github.com/kyleseneker/arrchitect/internal/testnet"
)

func observeSeerr(t *testing.T, cfg *requestsv1alpha1.Seerr) metav1.Condition {
	t.Helper()
	cfg.Spec.Reconcile = &commonv1alpha1.ReconcileConfig{DriftPolicy: "observe"}
	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	require.NoError(t, requestsv1alpha1.AddToScheme(scheme))
	c := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&requestsv1alpha1.Seerr{}).WithObjects(cfg, seerrSecret()).Build()
	r := &SeerrReconciler{Client: c, Recorder: events.NewFakeRecorder(50)}
	key := types.NamespacedName{Namespace: cfg.Namespace, Name: cfg.Name}
	_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key})
	require.NoError(t, err)
	require.NoError(t, c.Get(context.Background(), key, cfg))
	i := slices.IndexFunc(cfg.Status.Conditions, func(c metav1.Condition) bool { return c.Type == "Synced" })
	require.GreaterOrEqual(t, i, 0, "no Synced condition")
	return cfg.Status.Conditions[i]
}

func (f *fakeSeerr) writes() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, r := range f.requests {
		if !strings.HasPrefix(r, http.MethodGet) {
			out = append(out, r)
		}
	}
	return out
}

// Signing in to a Seerr that has not been set up configures its media server,
// so observe mode must not even log in.
func TestSeerrObserveReportsMissingSetupWithoutLoggingIn(t *testing.T) {
	app := &fakeSeerr{}
	cfg := seerrConfig(app.serve(t))
	cfg.Spec.JellyfinAuth = requestsv1alpha1.SeerrJellyfinAuth{
		Hostname:          "jellyfin",
		UsernameSecretRef: &commonv1alpha1.SecretKeyRef{Name: "seerr-secrets", Key: "username"},
		PasswordSecretRef: &commonv1alpha1.SecretKeyRef{Name: "seerr-secrets", Key: "password"},
	}
	synced := observeSeerr(t, cfg)
	assert.Empty(t, app.writes())
	assert.Equal(t, "DriftDetected", synced.Reason)
}

func TestSeerrObserveReportsDifferingSettingsWithoutWriting(t *testing.T) {
	app := &fakeSeerr{initialized: true}
	cfg := seerrConfig(app.serve(t))
	cfg.Spec.Connection.APIKeySecretRef = &commonv1alpha1.SecretKeyRef{Name: "seerr-secrets", Key: "api-key"}
	cfg.Spec.Main = &requestsv1alpha1.SeerrMain{ApplicationTitle: "Requests"}
	synced := observeSeerr(t, cfg)
	require.True(t, app.saw("GET", "/api/v1/settings/main"), "observe never read the settings")
	assert.Empty(t, app.writes())
	assert.Equal(t, "DriftDetected", synced.Reason)
}

func TestSeerrObserveMatchesWithNothingToCompare(t *testing.T) {
	app := &fakeSeerr{initialized: true}
	cfg := seerrConfig(app.serve(t))
	cfg.Spec.Connection.APIKeySecretRef = &commonv1alpha1.SecretKeyRef{Name: "seerr-secrets", Key: "api-key"}
	synced := observeSeerr(t, cfg)
	assert.Empty(t, app.writes())
	assert.Equal(t, metav1.ConditionTrue, synced.Status, synced.Message)
}

// Seerr returns its whole settings document; only declared values decide
// whether a save is needed.
func TestSeerrChangedComparesOnlyDeclaredSettings(t *testing.T) {
	current := map[string]any{"applicationTitle": "Seerr", "apiKey": "x", "defaultPermissions": float64(32)}
	assert.False(t, seerrDiffers(current, map[string]any{"applicationTitle": "Seerr", "defaultPermissions": 32}))
	assert.True(t, seerrDiffers(current, map[string]any{"applicationTitle": "Requests"}))
	assert.True(t, seerrDiffers(nil, map[string]any{"enabled": true}), "nothing stored yet")
}

// fakeSeerr records requests and the bodies posted to the auth endpoints, so
// the login payload can be asserted rather than just the fact of a call. Like
// Seerr's request validator, it rejects query parameters the API does not
// declare; the operator sends none.
type fakeSeerr struct {
	mu          sync.Mutex
	requests    []string
	rejected    []string
	authBodies  map[string]map[string]any
	initialized bool
	// libraries is what a media server library sync finds, keyed by "id" with
	// an "enabled" flag the library PUT updates.
	libraries []map[string]any
	// listed is whether Seerr already lists the libraries, as it does once it
	// has synced them.
	listed bool
	// services holds the Sonarr and Radarr connections, keyed by "sonarr" or
	// "radarr". Like Seerr, a POST appends one rather than replacing by name.
	services map[string][]map[string]any
	// jellyfin is the stored Jellyfin server. Like Seerr, a POST merges the body
	// into it, keeping keys Seerr does not use.
	jellyfin map[string]any
	plex     map[string]any
	// notifications holds each agent's settings. Like Seerr, a POST replaces
	// the agent whole.
	notifications map[string]map[string]any
	// refuseKey answers every request but the public settings with 401.
	refuseKey bool
	// refuseSync answers a media server's library sync with 401, which only
	// setup sends.
	refuseSync bool
	// network is the stored network settings. Like Seerr, a POST merges the
	// body into it.
	network map[string]any
	// metadata is the stored metadata providers. Like Seerr, a PUT saves tv and
	// anime from the body, unless unreachableProviders fails its check.
	metadata             map[string]any
	unreachableProviders bool
	// jobs maps each registered job to its schedule.
	jobs map[string]string
}

func (f *fakeSeerr) jellyfinServer() map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return maps.Clone(f.jellyfin)
}

// mediaServer returns the stored Jellyfin or Plex server for a settings path,
// creating it on first use. The caller holds f.mu.
func (f *fakeSeerr) mediaServer(path string) map[string]any {
	server := &f.jellyfin
	if strings.HasSuffix(path, "/plex") {
		server = &f.plex
	}
	if *server == nil {
		*server = map[string]any{}
	}
	return *server
}

func mediaServerLibraries(path string) string {
	if strings.HasPrefix(path, "/api/v1/settings/plex/") {
		return "/api/v1/settings/plex/library/"
	}
	return "/api/v1/settings/jellyfin/library/"
}

func (f *fakeSeerr) plexServer() map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return maps.Clone(f.plex)
}

func (f *fakeSeerr) serviceCount(kind string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.services[kind])
}

func (f *fakeSeerr) saw(method, path string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Contains(f.requests, method+" "+path)
}

// order returns the position of the first matching request, or -1.
func (f *fakeSeerr) order(method, path string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Index(f.requests, method+" "+path)
}

func (f *fakeSeerr) libraryEnabled(id string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, lib := range f.libraries {
		if lib["id"] == id {
			return lib["enabled"] == true
		}
	}
	return false
}

func (f *fakeSeerr) authBody(path string) map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.authBodies[path]
}

func (f *fakeSeerr) serve(t *testing.T) string {
	t.Helper()
	if f.authBodies == nil {
		f.authBodies = map[string]map[string]any{}
	}
	if f.services == nil {
		f.services = map[string][]map[string]any{}
	}
	if f.notifications == nil {
		f.notifications = map[string]map[string]any{}
	}
	srv := testnet.Serve(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if r.Method == http.MethodPost || r.Method == http.MethodPut {
			_ = json.NewDecoder(r.Body).Decode(&body)
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		f.requests = append(f.requests, r.Method+" "+r.URL.Path)
		if body != nil {
			f.authBodies[r.URL.Path] = body
		}

		w.Header().Set("Content-Type", "application/json")
		if f.reject(w, r) {
			return
		}
		switch {
		case r.URL.Path == "/api/v1/settings/public":
			_ = json.NewEncoder(w).Encode(map[string]any{"initialized": f.initialized})
		case r.URL.Path == "/api/v1/settings/network":
			if f.network == nil {
				f.network = map[string]any{"csrfProtection": false, "trustProxy": false}
			}
			if r.Method == http.MethodPost {
				maps.Copy(f.network, body)
			}
			_ = json.NewEncoder(w).Encode(f.network)
		case r.URL.Path == "/api/v1/settings/main":
			_ = json.NewEncoder(w).Encode(map[string]any{"apiKey": "seerr-generated-key"})
		case f.metadataAndJobRoutes(w, r, body):
		case r.URL.Path == "/api/v1/settings/sonarr" || r.URL.Path == "/api/v1/settings/radarr":
			kind := strings.TrimPrefix(r.URL.Path, "/api/v1/settings/")
			if r.Method == http.MethodPost {
				body["id"] = float64(len(f.services[kind]))
				f.services[kind] = append(f.services[kind], body)
				_ = json.NewEncoder(w).Encode(body)
				return
			}
			_ = json.NewEncoder(w).Encode(append([]map[string]any{}, f.services[kind]...))
		case f.replaceWhole(w, r, body):
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/v1/settings/"):
			kind, id, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/api/v1/settings/"), "/")
			f.services[kind] = slices.DeleteFunc(f.services[kind], func(s map[string]any) bool { return fmt.Sprint(s["id"]) == id })
		case r.URL.Path == "/api/v1/settings/jellyfin" || r.URL.Path == "/api/v1/settings/plex":
			server := f.mediaServer(r.URL.Path)
			if r.Method == http.MethodPost {
				maps.Copy(server, body)
			}
			_ = json.NewEncoder(w).Encode(server)
		case r.URL.Path == "/api/v1/settings/initialize" && r.Method == http.MethodPost:
			f.initialized = true
			_ = json.NewEncoder(w).Encode(map[string]any{"initialized": true})
		case f.libraryRoutes(w, r, body):
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{})
		}
	}))
	return srv.URL
}

// metadataAndJobRoutes serves the metadata providers, the job list and a job's
// reschedule. The caller holds f.mu.
func (f *fakeSeerr) metadataAndJobRoutes(w http.ResponseWriter, r *http.Request, body map[string]any) bool {
	switch {
	case r.URL.Path == "/api/v1/settings/metadatas":
		if f.metadata == nil {
			f.metadata = map[string]any{"tv": "tmdb", "anime": "tmdb"}
		}
		if r.Method == http.MethodPut {
			if f.unreachableProviders {
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "tests": map[string]any{"tvdb": "failed"}})
				return true
			}
			f.metadata = map[string]any{"tv": body["tv"], "anime": body["anime"]}
		}
		_ = json.NewEncoder(w).Encode(f.metadata)
	case r.URL.Path == "/api/v1/settings/jobs":
		jobs := []map[string]any{}
		for _, id := range slices.Sorted(maps.Keys(f.jobs)) {
			jobs = append(jobs, map[string]any{"id": id, "cronSchedule": f.jobs[id]})
		}
		_ = json.NewEncoder(w).Encode(jobs)
	case strings.HasPrefix(r.URL.Path, "/api/v1/settings/jobs/") && strings.HasSuffix(r.URL.Path, "/schedule"):
		id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v1/settings/jobs/"), "/schedule")
		if _, ok := f.jobs[id]; !ok {
			w.WriteHeader(http.StatusNotFound)
			return true
		}
		f.jobs[id], _ = body["schedule"].(string)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": id, "cronSchedule": f.jobs[id]})
	default:
		return false
	}
	return true
}

// reject answers what Seerr refuses: a query parameter it doesn't know, and,
// with refuseKey set, the API key.
func (f *fakeSeerr) reject(w http.ResponseWriter, r *http.Request) bool {
	switch {
	case f.refuseKey && r.URL.Path != "/api/v1/settings/public":
		w.WriteHeader(http.StatusUnauthorized)
	case r.URL.RawQuery != "":
		f.rejected = append(f.rejected, r.Method+" "+r.URL.RequestURI())
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"message": "Unknown query parameter"})
	default:
		return false
	}
	return true
}

// libraryRoutes serves a media server's library list, its sync, and a single
// library's update. The caller holds f.mu.
func (f *fakeSeerr) libraryRoutes(w http.ResponseWriter, r *http.Request, body map[string]any) bool {
	prefix := mediaServerLibraries(r.URL.Path)
	switch {
	case r.URL.Path == prefix+"sync" && r.Method == http.MethodPost:
		if f.refuseSync {
			w.WriteHeader(http.StatusUnauthorized)
			return true
		}
		if len(f.libraries) == 0 {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{"message": "SYNC_ERROR_NO_LIBRARIES"})
			return true
		}
		f.listed = true
		_ = json.NewEncoder(w).Encode(f.libraries)
	case r.URL.Path+"/" == prefix && r.Method == http.MethodGet:
		listed := []map[string]any{}
		if f.listed {
			listed = f.libraries
		}
		_ = json.NewEncoder(w).Encode(listed)
	case strings.HasPrefix(r.URL.Path, prefix) && r.Method == http.MethodPut:
		id := strings.TrimPrefix(r.URL.Path, prefix)
		for _, lib := range f.libraries {
			if lib["id"] == id {
				lib["enabled"] = body["enabled"]
				_ = json.NewEncoder(w).Encode(lib)
				return true
			}
		}
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"message": "Library does not exist."})
	default:
		return false
	}
	return true
}

// replaceWhole serves the settings Seerr replaces with the body it receives: a
// Sonarr or Radarr server's update, and a notification agent.
func (f *fakeSeerr) replaceWhole(w http.ResponseWriter, r *http.Request, body map[string]any) bool {
	switch {
	case r.Method == http.MethodPut && (strings.HasPrefix(r.URL.Path, "/api/v1/settings/sonarr/") ||
		strings.HasPrefix(r.URL.Path, "/api/v1/settings/radarr/")):
		if _, ok := body["id"]; ok {
			http.Error(w, `{"message":"request/body/id is read-only"}`, http.StatusBadRequest)
			return true
		}
		kind, id, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/api/v1/settings/"), "/")
		for i, s := range f.services[kind] {
			if fmt.Sprint(s["id"]) == id {
				body["id"] = s["id"]
				f.services[kind][i] = body
			}
		}
		_ = json.NewEncoder(w).Encode(body)
	case strings.HasPrefix(r.URL.Path, "/api/v1/settings/notifications/"):
		agent := strings.TrimPrefix(r.URL.Path, "/api/v1/settings/notifications/")
		if r.Method == http.MethodPost {
			f.notifications[agent] = body
		}
		stored := f.notifications[agent]
		if stored == nil {
			stored = map[string]any{}
		}
		_ = json.NewEncoder(w).Encode(stored)
	default:
		return false
	}
	return true
}

func seerrSecret() *corev1.Secret {
	return &corev1.Secret{
		Name: "seerr-secrets", Namespace: "media",
		Data: map[string][]byte{
			"api-key":  []byte("supplied-key"),
			"username": []byte("admin"),
			"password": []byte("hunter2"),
			"token":    []byte("plex-token"),
		},
	}
}

func seerrConfig(addr string) *requestsv1alpha1.Seerr {
	return &requestsv1alpha1.Seerr{
		Name: "seerr", Namespace: "media",
		Spec: requestsv1alpha1.SeerrSpec{
			Connection: requestsv1alpha1.SeerrConnection{URL: addr},
		},
	}
}

func runSeerr(t *testing.T, cfg *requestsv1alpha1.Seerr, objs ...client.Object) *requestsv1alpha1.Seerr {
	t.Helper()
	_, got := reconcileSeerr(t, cfg, objs...)
	return got
}

// reconcileSeerr is runSeerr that also returns the reconcile's result.
func reconcileSeerr(t *testing.T, cfg *requestsv1alpha1.Seerr, objs ...client.Object) (ctrl.Result, *requestsv1alpha1.Seerr) {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme), "adding client-go scheme")
	require.NoError(t, requestsv1alpha1.AddToScheme(scheme), "adding requests scheme")
	require.NoError(t, pvrv1alpha1.AddToScheme(scheme), "adding pvr scheme")
	require.NoError(t, mediaserversv1alpha1.AddToScheme(scheme), "adding mediaservers scheme")
	c := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&requestsv1alpha1.Seerr{}).
		WithObjects(append(objs, cfg, seerrSecret())...).Build()
	r := &SeerrReconciler{Client: c, Recorder: events.NewFakeRecorder(50)}
	result, err := r.Reconcile(context.Background(), ctrl.Request{
		Name: "seerr", Namespace: "media",
	})
	require.NoError(t, err, "reconcile")
	var got requestsv1alpha1.Seerr
	if err := c.Get(context.Background(), types.NamespacedName{Name: "seerr", Namespace: "media"}, &got); err != nil {
		require.True(t, apierrors.IsNotFound(err), "re-reading the resource: %v", err)
		return result, nil
	}
	return result, &got
}

// A supplied API key is used directly. Logging in anyway would be pointless and,
// on a Jellyfin-backed Seerr, would need admin credentials the user never gave.
func TestSeerrSuppliedAPIKeySkipsLogin(t *testing.T) {
	app := &fakeSeerr{initialized: true}
	addr := app.serve(t)

	cfg := seerrConfig(addr)
	cfg.Spec.Connection.APIKeySecretRef = &commonv1alpha1.SecretKeyRef{Name: "seerr-secrets", Key: "api-key"}
	runSeerr(t, cfg)

	assert.False(t, app.saw("POST", "/api/v1/auth/jellyfin"), "logged in despite being given an API key")
	assert.False(t, app.saw("POST", "/api/v1/auth/plex"), "logged in despite being given an API key")
	require.True(t, app.saw("GET", "/api/v1/settings/public"), "reconcile never reached the app, so the login check is meaningless")
}

// Without an API key the operator logs in and then reads the generated key back
// out of the main settings.
func TestSeerrWithoutAPIKeyLogsInAndFetchesKey(t *testing.T) {
	app := &fakeSeerr{initialized: true}
	addr := app.serve(t)

	port := int32(8096)
	cfg := seerrConfig(addr)
	cfg.Spec.JellyfinAuth = requestsv1alpha1.SeerrJellyfinAuth{
		Hostname:          "jellyfin",
		Port:              &port,
		URLBase:           "/jf",
		UsernameSecretRef: &commonv1alpha1.SecretKeyRef{Name: "seerr-secrets", Key: "username"},
		PasswordSecretRef: &commonv1alpha1.SecretKeyRef{Name: "seerr-secrets", Key: "password"},
	}
	runSeerr(t, cfg)

	require.True(t, app.saw("POST", "/api/v1/auth/jellyfin"), "never logged in")
	assert.True(t, app.saw("GET", "/api/v1/settings/main"), "never read the API key back after logging in")
}

// Seerr routes the login by serverType and builds the target URL from urlBase.
// Omitting either sends the request to the wrong place, which fails as an
// opaque auth error rather than anything diagnosable.
func TestSeerrJellyfinLoginCarriesServerTypeAndURLBase(t *testing.T) {
	app := &fakeSeerr{initialized: true}
	addr := app.serve(t)

	port := int32(8096)
	cfg := seerrConfig(addr)
	cfg.Spec.JellyfinAuth = requestsv1alpha1.SeerrJellyfinAuth{
		Hostname:          "jellyfin",
		Port:              &port,
		URLBase:           "/jf",
		UsernameSecretRef: &commonv1alpha1.SecretKeyRef{Name: "seerr-secrets", Key: "username"},
		PasswordSecretRef: &commonv1alpha1.SecretKeyRef{Name: "seerr-secrets", Key: "password"},
	}
	runSeerr(t, cfg)

	body := app.authBody("/api/v1/auth/jellyfin")
	require.NotNil(t, body, "no jellyfin login body captured")
	assert.NotNil(t, body["serverType"], "login payload omits serverType")
	assert.Equal(t, "/jf", body["urlBase"], "urlBase")
	assert.Equal(t, "admin", body["username"], "username is not the resolved secret value")
}

func TestSeerrPlexLoginSendsToken(t *testing.T) {
	app := &fakeSeerr{initialized: true}
	addr := app.serve(t)

	cfg := seerrConfig(addr)
	cfg.Spec.PlexAuth = requestsv1alpha1.SeerrPlexAuth{
		Hostname:       "plex",
		TokenSecretRef: &commonv1alpha1.SecretKeyRef{Name: "seerr-secrets", Key: "token"},
	}
	runSeerr(t, cfg)

	body := app.authBody("/api/v1/auth/plex")
	require.NotNil(t, body, "no plex login body captured")
	assert.Equal(t, "plex-token", body["authToken"], "authToken is not the resolved secret value")
}

func firstRunJellyfinConfig(addr string) *requestsv1alpha1.Seerr {
	port := int32(8096)
	cfg := seerrConfig(addr)
	cfg.Spec.Connection.APIKeySecretRef = &commonv1alpha1.SecretKeyRef{Name: "seerr-secrets", Key: "api-key"}
	cfg.Spec.JellyfinAuth = requestsv1alpha1.SeerrJellyfinAuth{
		Hostname:          "jellyfin",
		Port:              &port,
		UsernameSecretRef: &commonv1alpha1.SecretKeyRef{Name: "seerr-secrets", Key: "username"},
		PasswordSecretRef: &commonv1alpha1.SecretKeyRef{Name: "seerr-secrets", Key: "password"},
	}
	return cfg
}

// First-time setup refreshes the library list from Jellyfin, enables every
// library, and only then completes initialization. Seerr 3.5.0 removed the
// GET .../library?sync&enable form; a request using it is rejected, setup
// never completes, and a rebuilt Seerr is left uninitialized.
func TestSeerrFirstRunEnablesJellyfinLibraries(t *testing.T) {
	app := &fakeSeerr{libraries: []map[string]any{
		{"id": "movies", "name": "Movies", "enabled": false},
		{"id": "shows", "name": "Shows", "enabled": false},
	}}
	addr := app.serve(t)

	runSeerr(t, firstRunJellyfinConfig(addr))

	require.Empty(t, app.rejected, "sent query parameters Seerr does not declare")
	for _, id := range []string{"movies", "shows"} {
		assert.True(t, app.libraryEnabled(id), "library %s was not enabled", id)
	}
	initialize := app.order("POST", "/api/v1/settings/initialize")
	require.GreaterOrEqual(t, initialize, 0, "setup never completed initialization")
	assert.LessOrEqual(t, app.order("PUT", "/api/v1/settings/jellyfin/library/shows"), initialize,
		"initialized before every library was enabled")
}

// Setup that fails partway runs again from the start on the next pass. Seerr
// appends a connection on every POST, so posting Sonarr and Radarr without
// looking first would leave one copy of each per attempt.
func TestSeerrRetriedSetupDoesNotDuplicateConnections(t *testing.T) {
	app := &fakeSeerr{} // no Jellyfin libraries yet, so setup fails and is retried
	addr := app.serve(t)
	port := int32(8989)
	cfg := firstRunJellyfinConfig(addr)
	key := &commonv1alpha1.SecretKeyRef{Name: "seerr-secrets", Key: "api-key"}
	cfg.Spec.Sonarr = []requestsv1alpha1.SeerrServiceConnection{{Name: "Sonarr", Hostname: "sonarr", Port: &port, APIKeySecretRef: key}}
	cfg.Spec.Radarr = []requestsv1alpha1.SeerrServiceConnection{{Name: "Radarr", Hostname: "radarr", Port: &port, APIKeySecretRef: key}}

	runSeerr(t, cfg.DeepCopy())
	runSeerr(t, cfg.DeepCopy())

	require.False(t, app.saw("POST", "/api/v1/settings/initialize"), "precondition: setup must not have completed")
	assert.Equal(t, 1, app.serviceCount("sonarr"), "each setup attempt added another Sonarr connection")
	assert.Equal(t, 1, app.serviceCount("radarr"), "each setup attempt added another Radarr connection")
}

// Seerr's library sync answers 404 while Jellyfin has no libraries yet, as on a
// rebuild where the Jellyfin operator has not created them. Finishing setup then
// would leave Seerr with nothing enabled, and setup never runs again.
func TestSeerrFirstRunWaitsForJellyfinLibraries(t *testing.T) {
	app := &fakeSeerr{}
	addr := app.serve(t)

	runSeerr(t, firstRunJellyfinConfig(addr))

	require.True(t, app.saw("POST", "/api/v1/settings/jellyfin/library/sync"), "never asked Seerr to sync the Jellyfin libraries")
	assert.False(t, app.saw("POST", "/api/v1/settings/initialize"), "completed initialization with no libraries to enable")
}

// A setup that fails while Jellyfin has no libraries yet tries again in a
// minute. Mixed with a missing Secret, it waits for the resource's interval,
// as any mix of failures does.
func TestSeerrFailedSetupRetriesByItsReason(t *testing.T) {
	plain, _ := reconcileSeerr(t, firstRunJellyfinConfig((&fakeSeerr{}).serve(t)))
	assert.Equal(t, time.Minute, plain.RequeueAfter, "a setup that failed for no reason of its own retries in a minute")

	cfg := firstRunJellyfinConfig((&fakeSeerr{}).serve(t))
	port := int32(8989)
	cfg.Spec.Sonarr = []requestsv1alpha1.SeerrServiceConnection{{
		Name: "Sonarr", Hostname: "sonarr", Port: &port, APIKeySecretRef: &commonv1alpha1.SecretKeyRef{Name: "missing", Key: "api-key"},
	}}
	mixed, got := reconcileSeerr(t, cfg)
	require.NotNil(t, got)
	ready := meta.FindStatusCondition(got.Status.Conditions, "Ready")
	require.NotNil(t, ready)
	assert.Equal(t, "SyncFailed", ready.Reason)
	assert.GreaterOrEqual(t, mixed.RequeueAfter, 5*time.Minute, "a mix of failures retried before the interval")

	refused, got := reconcileSeerr(t, firstRunJellyfinConfig((&fakeSeerr{refuseSync: true}).serve(t)))
	require.NotNil(t, got)
	ready = meta.FindStatusCondition(got.Status.Conditions, "Ready")
	require.NotNil(t, ready)
	assert.Equal(t, "Unauthorized", ready.Reason)
	assert.GreaterOrEqual(t, refused.RequeueAfter, 5*time.Minute, "refused credentials retried before the 5 minutes they wait")
}

func TestSeerrFailedSetupKeepsTrackOfTheServersItCreated(t *testing.T) {
	app := &fakeSeerr{}
	cfg := firstRunJellyfinConfig(app.serve(t))
	port := int32(8989)
	cfg.Spec.Sonarr = []requestsv1alpha1.SeerrServiceConnection{{
		Name: "Sonarr", Hostname: "sonarr", Port: &port, APIKeySecretRef: &commonv1alpha1.SecretKeyRef{Name: "seerr-secrets", Key: "api-key"},
	}}

	got := runSeerr(t, cfg)

	require.Equal(t, []string{"Sonarr"}, seerrServiceNames(app, "sonarr"))
	assert.Equal(t, []string{"Sonarr"}, got.Status.GetManagedResources()["sonarr"], "prune and deletion could never remove it")
}

func TestSeerrReportsARefusedAPIKeyAsUnauthorized(t *testing.T) {
	app := &fakeSeerr{initialized: true, refuseKey: true}

	got := runSeerr(t, seerrWithAPIKey(app.serve(t)))

	ready := meta.FindStatusCondition(got.Status.Conditions, "Ready")
	require.NotNil(t, ready)
	assert.Equal(t, metav1.ConditionFalse, ready.Status)
	assert.Equal(t, "Unauthorized", ready.Reason, "the Secret exists; Seerr refused the key in it")
}

func jellyfinServerConfig(addr string) *requestsv1alpha1.Seerr {
	cfg := firstRunJellyfinConfig(addr)
	cfg.Spec.JellyfinAuth.UseSSL = new(false)
	return cfg
}

// Seerr's settings migration turns a stored "hostname" without a port into port
// 80, so a set-up Seerr can end up pointing at a port Jellyfin does not serve.
func TestSeerrRestoresTheDeclaredJellyfinServer(t *testing.T) {
	app := &fakeSeerr{initialized: true, jellyfin: map[string]any{
		"ip": "jellyfin", "port": float64(80), "useSsl": nil, "urlBase": "", "serverId": "abc",
	}}

	runSeerr(t, jellyfinServerConfig(app.serve(t)))

	got := app.jellyfinServer()
	assert.Equal(t, "jellyfin", got["ip"])
	assert.Equal(t, float64(8096), got["port"])
	assert.Equal(t, false, got["useSsl"])
	assert.NotContains(t, got, "hostname", "Seerr's migration rewrites the port from a stored hostname")
}

func TestSeerrFirstRunStoresJellyfinServerAsIP(t *testing.T) {
	app := &fakeSeerr{libraries: []map[string]any{{"id": "movies", "name": "Movies", "enabled": false}}}

	runSeerr(t, jellyfinServerConfig(app.serve(t)))

	require.True(t, app.saw("POST", "/api/v1/settings/initialize"), "setup never completed")
	got := app.jellyfinServer()
	assert.Equal(t, "jellyfin", got["ip"])
	assert.NotContains(t, got, "hostname", "Seerr's migration rewrites the port from a stored hostname")
}

func TestSeerrLeavesAMatchingJellyfinServerAlone(t *testing.T) {
	app := &fakeSeerr{initialized: true, jellyfin: map[string]any{
		"ip": "jellyfin", "port": float64(8096), "useSsl": false, "urlBase": "", "name": "home",
	}}

	runSeerr(t, jellyfinServerConfig(app.serve(t)))

	require.True(t, app.saw("GET", "/api/v1/settings/jellyfin"), "never read the Jellyfin server")
	assert.False(t, app.saw("POST", "/api/v1/settings/jellyfin"), "rewrote a Jellyfin server that already matched")
}

func TestSeerrObserveReportsADifferentJellyfinServerWithoutWriting(t *testing.T) {
	app := &fakeSeerr{initialized: true, jellyfin: map[string]any{
		"ip": "jellyfin", "port": float64(80), "useSsl": false, "urlBase": "",
	}}

	synced := observeSeerr(t, jellyfinServerConfig(app.serve(t)))

	assert.Empty(t, app.writes())
	assert.Equal(t, "DriftDetected", synced.Reason)
}

// Seerr stores a service connection wholesale and answers 2xx for keys it does not
// use, so a spec field the builder never maps is silently inert. APIKeySecretRef is
// resolved before the payload is built and lands as "apiKey".
func TestSeerrServiceFieldsAllReachThePayload(t *testing.T) {
	contract.AssertEveryFieldEmitted(t, &requestsv1alpha1.SeerrServiceConnection{},
		func(s *requestsv1alpha1.SeerrServiceConnection) map[string]any {
			return buildServicePayload(s, "resolved-key")
		})
}

func TestSeerrMainFieldsAllReachThePayload(t *testing.T) {
	contract.AssertEveryFieldEmitted(t, &requestsv1alpha1.SeerrMain{}, seerrMainPayload, func(m *requestsv1alpha1.SeerrMain) {
		m.OriginalLanguages = []string{"en"}
		m.BlocklistLanguages = []string{"en"}
		m.BlocklistedTags = []int32{1}
		m.DefaultQuotas.Movie.Limit = new(int32)
	})
}

// Seerr keeps languages and tags as joined strings, and merges a partial
// quota into the stored one.
func TestSeerrMainPayloadUsesSeerrsShapes(t *testing.T) {
	five, week, unlimited := int32(5), int32(7), int32(0)
	payload := seerrMainPayload(&requestsv1alpha1.SeerrMain{
		OriginalLanguages:   []string{"en", "ja"},
		BlocklistedTags:     []int32{210024, 9663},
		NewMediaServerLogin: new(bool),
		DefaultQuotas: requestsv1alpha1.SeerrDefaultQuotas{
			Movie: requestsv1alpha1.SeerrQuota{Limit: &five, Days: &week},
			TV:    requestsv1alpha1.SeerrQuota{Limit: &unlimited},
		},
	})
	assert.Equal(t, map[string]any{
		"originalLanguage": "en|ja",
		"blocklistedTags":  "210024,9663",
		"newPlexLogin":     false,
		"defaultQuotas": map[string]any{
			"movie": map[string]any{"quotaLimit": int32(5), "quotaDays": int32(7)},
			"tv":    map[string]any{"quotaLimit": int32(0)},
		},
	}, payload)
	assert.False(t, seerrDiffers(map[string]any{
		"originalLanguage": "en|ja", "blocklistedTags": "210024,9663", "newPlexLogin": false,
		"defaultQuotas": map[string]any{
			"movie": map[string]any{"quotaLimit": float64(5), "quotaDays": float64(7)},
			"tv":    map[string]any{"quotaLimit": float64(0), "quotaDays": float64(30)},
		},
	}, payload), "what Seerr stores back matches")
}

// Seerr rejects a new server that omits these, so they are sent even when the
// resource leaves them out.
func TestSeerrSendsWhatANewServerRequires(t *testing.T) {
	app := &fakeSeerr{initialized: true}
	cfg := seerrWithAPIKey(app.serve(t))
	port := int32(8989)
	cfg.Spec.Sonarr = []requestsv1alpha1.SeerrServiceConnection{{
		Name: "Sonarr", Hostname: "sonarr", Port: &port, APIKeySecretRef: &commonv1alpha1.SecretKeyRef{Name: "seerr-secrets", Key: "api-key"},
	}}

	runSeerr(t, cfg)

	app.mu.Lock()
	defer app.mu.Unlock()
	require.Len(t, app.services["sonarr"], 1)
	created := app.services["sonarr"][0]
	for _, k := range []string{
		"name", "hostname", "apiKey", "port", "useSsl", "activeProfileId", "activeProfileName", "activeDirectory",
		"is4k", "isDefault", "enableSeasonFolders",
	} {
		assert.Contains(t, created, k, "%q missing; Seerr's OpenAPI marks it required and rejects the request with 400", k)
	}
	assert.Equal(t, "supplied-key", created["apiKey"], "apiKey is not the resolved secret value")
}

// Seerr stores an updated server as the body it receives, so what the resource
// leaves out has to be sent back as Seerr has it.
func TestSeerrServerUpdateKeepsWhatTheResourceLeavesOut(t *testing.T) {
	app := &fakeSeerr{initialized: true, services: map[string][]map[string]any{
		"sonarr": {{
			"id": float64(0), "name": "Sonarr", "hostname": "old-sonarr", "port": float64(8989), "apiKey": "supplied-key",
			"isDefault": true, "is4k": false, "tags": []any{float64(3)}, "activeLanguageProfileId": float64(1),
		}},
	}}
	cfg := seerrWithAPIKey(app.serve(t))
	port := int32(8989)
	cfg.Spec.Sonarr = []requestsv1alpha1.SeerrServiceConnection{{
		Name: "Sonarr", Hostname: "sonarr", Port: &port, APIKeySecretRef: &commonv1alpha1.SecretKeyRef{Name: "seerr-secrets", Key: "api-key"},
	}}

	runSeerr(t, cfg)

	app.mu.Lock()
	defer app.mu.Unlock()
	server := app.services["sonarr"][0]
	assert.Equal(t, "sonarr", server["hostname"])
	assert.Equal(t, true, server["isDefault"], "Seerr routes requests to the default server")
	assert.Equal(t, []any{float64(3)}, server["tags"])
	assert.Equal(t, float64(1), server["activeLanguageProfileId"])
}

func TestSeerrNotificationKeepsWhatTheResourceLeavesOut(t *testing.T) {
	app := &fakeSeerr{initialized: true, notifications: map[string]map[string]any{
		"discord": {"enabled": false, "types": float64(4), "options": map[string]any{
			"webhookUrl": "https://discord.test/hook", "botAvatarUrl": "https://img.test/a.png",
		}},
	}}
	cfg := seerrWithAPIKey(app.serve(t))
	cfg.Spec.Notifications = []requestsv1alpha1.SeerrNotificationAgent{{
		Agent: "discord", Enabled: new(true),
		Options: []commonv1alpha1.ConfigField{{Name: "botUsername", Value: &commonv1alpha1.FieldValue{Raw: []byte(`"Seerr"`)}}},
	}}

	runSeerr(t, cfg)

	app.mu.Lock()
	defer app.mu.Unlock()
	assert.Equal(t, map[string]any{
		"enabled": true, "types": float64(4), "options": map[string]any{
			"webhookUrl": "https://discord.test/hook", "botAvatarUrl": "https://img.test/a.png", "botUsername": "Seerr",
		},
	}, app.notifications["discord"])
}

func seerrServiceNames(f *fakeSeerr, kind string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	names := make([]string, 0, len(f.services[kind]))
	for _, s := range f.services[kind] {
		names = append(names, fmt.Sprint(s["name"]))
	}
	return names
}

func seerrWithAPIKey(addr string) *requestsv1alpha1.Seerr {
	cfg := seerrConfig(addr)
	cfg.Spec.Connection.APIKeySecretRef = &commonv1alpha1.SecretKeyRef{Name: "seerr-secrets", Key: "api-key"}
	return cfg
}

func TestSeerrPrunesTheConnectionItCreatedUnderAnOldName(t *testing.T) {
	app := &fakeSeerr{initialized: true, services: map[string][]map[string]any{
		"sonarr": {{"id": float64(0), "name": "Old"}, {"id": float64(1), "name": "Hand-made"}},
	}}
	cfg := seerrWithAPIKey(app.serve(t))
	port := int32(8989)
	cfg.Spec.Sonarr = []requestsv1alpha1.SeerrServiceConnection{{
		Name: "New", Hostname: "sonarr", Port: &port,
		APIKeySecretRef: &commonv1alpha1.SecretKeyRef{Name: "seerr-secrets", Key: "api-key"},
	}}
	cfg.Spec.Reconcile = &commonv1alpha1.ReconcileConfig{Prune: new(true)}
	cfg.Status.SetManagedResources(map[string][]string{"sonarr": {"Old"}})

	runSeerr(t, cfg)

	assert.ElementsMatch(t, []string{"Hand-made", "New"}, seerrServiceNames(app, "sonarr"))
}

func TestSeerrDeletionPolicyRemovesTheConnectionsItCreated(t *testing.T) {
	app := &fakeSeerr{initialized: true, services: map[string][]map[string]any{
		"sonarr": {{"id": float64(0), "name": "Sonarr"}, {"id": float64(1), "name": "Hand-made"}},
	}}
	cfg := seerrWithAPIKey(app.serve(t))
	policy := reconciler.DeletionPolicyDelete
	cfg.Spec.Reconcile = &commonv1alpha1.ReconcileConfig{DeletionPolicy: policy}
	cfg.Status.SetManagedResources(map[string][]string{"sonarr": {"Sonarr"}})
	now := metav1.NewTime(time.Now())
	cfg.DeletionTimestamp = &now
	cfg.Finalizers = []string{reconciler.Finalizer}

	runSeerr(t, cfg)

	assert.Equal(t, []string{"Hand-made"}, seerrServiceNames(app, "sonarr"))
}

func TestSeerrObserveReportsAConnectionPruneWouldDelete(t *testing.T) {
	app := &fakeSeerr{initialized: true, services: map[string][]map[string]any{
		"sonarr": {{"id": float64(0), "name": "ObserveOld"}, {"id": float64(1), "name": "Hand-made"}},
	}}
	cfg := seerrWithAPIKey(app.serve(t))
	port := int32(8989)
	cfg.Spec.Sonarr = []requestsv1alpha1.SeerrServiceConnection{{
		Name: "New", Hostname: "sonarr", Port: &port,
		APIKeySecretRef: &commonv1alpha1.SecretKeyRef{Name: "seerr-secrets", Key: "api-key"},
	}}
	observe := "observe"
	cfg.Spec.Reconcile = &commonv1alpha1.ReconcileConfig{Prune: new(true), DriftPolicy: observe}
	cfg.Status.SetManagedResources(map[string][]string{"sonarr": {"ObserveOld"}})
	observed := metrics.DriftObservedTotal.WithLabelValues("seerr", "sonarr", "ObserveOld")
	before := testutil.ToFloat64(observed)

	runSeerr(t, cfg)

	assert.ElementsMatch(t, []string{"ObserveOld", "Hand-made"}, seerrServiceNames(app, "sonarr"))
	assert.Equal(t, before+1, testutil.ToFloat64(observed))
}

func plexServerConfig(addr string, initialized bool) *requestsv1alpha1.Seerr {
	cfg := seerrConfig(addr)
	if initialized {
		cfg.Spec.Connection.APIKeySecretRef = &commonv1alpha1.SecretKeyRef{Name: "seerr-secrets", Key: "api-key"}
	}
	cfg.Spec.PlexAuth = requestsv1alpha1.SeerrPlexAuth{
		Hostname: "plex", TokenSecretRef: &commonv1alpha1.SecretKeyRef{Name: "seerr-secrets", Key: "token"},
	}
	return cfg
}

func TestSeerrStoresTheDeclaredPlexServer(t *testing.T) {
	app := &fakeSeerr{initialized: true, plex: map[string]any{"ip": "old-plex", "port": float64(32400), "useSsl": false}}

	runSeerr(t, plexServerConfig(app.serve(t), true))

	got := app.plexServer()
	assert.Equal(t, "plex", got["ip"])
	assert.Equal(t, float64(32400), got["port"], "the port defaults to Plex's 32400")
	assert.Equal(t, false, got["useSsl"])
}

func TestSeerrPlexFirstRunSavesTheServerThenEnablesLibraries(t *testing.T) {
	app := &fakeSeerr{libraries: []map[string]any{{"id": "1", "name": "Movies", "enabled": false}}}

	runSeerr(t, plexServerConfig(app.serve(t), false))

	require.True(t, app.saw("POST", "/api/v1/settings/initialize"), "setup never completed")
	assert.Equal(t, "plex", app.plexServer()["ip"])
	saved, synced := app.order("POST", "/api/v1/settings/plex"), app.order("POST", "/api/v1/settings/plex/library/sync")
	require.NotEqual(t, -1, synced, "the Plex libraries were never synced")
	assert.Less(t, saved, synced, "Seerr syncs libraries from the saved server, so it must be saved first")
	assert.True(t, app.libraryEnabled("1"), "the Plex library was not enabled")
	assert.False(t, app.saw("POST", "/api/v1/settings/plex/sync"), "called the full-scan job instead of the library sync")
}

func TestSeerrLeavesAMatchingPlexServerAlone(t *testing.T) {
	app := &fakeSeerr{initialized: true, plex: map[string]any{
		"ip": "plex", "port": float64(32400), "useSsl": false, "name": "home", "machineId": "abc",
	}}

	runSeerr(t, plexServerConfig(app.serve(t), true))

	require.True(t, app.saw("GET", "/api/v1/settings/plex"), "never read the Plex server")
	assert.False(t, app.saw("POST", "/api/v1/settings/plex"), "rewrote a Plex server that already matched")
}

func TestSeerrManagesEveryDeclaredServer(t *testing.T) {
	app := &fakeSeerr{initialized: true, services: map[string][]map[string]any{
		"sonarr": {{"id": float64(0), "name": "Sonarr", "hostname": "old-sonarr"}},
	}}
	cfg := seerrWithAPIKey(app.serve(t))
	port := int32(8989)
	key := &commonv1alpha1.SecretKeyRef{Name: "seerr-secrets", Key: "api-key"}
	cfg.Spec.Sonarr = []requestsv1alpha1.SeerrServiceConnection{
		{Name: "Sonarr", Hostname: "sonarr", Port: &port, APIKeySecretRef: key, IsDefault: new(true)},
		{Name: "Sonarr 4K", Hostname: "sonarr-4k", Port: &port, APIKeySecretRef: key, Is4k: new(true), IsDefault: new(true)},
	}
	cfg.Spec.Radarr = []requestsv1alpha1.SeerrServiceConnection{{Name: "Radarr", Hostname: "radarr", Port: &port, APIKeySecretRef: key}}
	cfg.Spec.Reconcile = &commonv1alpha1.ReconcileConfig{Prune: new(true)}

	runSeerr(t, cfg)

	assert.True(t, app.saw("PUT", "/api/v1/settings/sonarr/0"), "the existing server was not updated")
	assert.ElementsMatch(t, []string{"Sonarr", "Sonarr 4K"}, seerrServiceNames(app, "sonarr"), "prune must keep every declared server")
	assert.Equal(t, []string{"Radarr"}, seerrServiceNames(app, "radarr"))
	app.mu.Lock()
	defer app.mu.Unlock()
	assert.Equal(t, true, app.services["sonarr"][1]["is4k"])
	assert.Equal(t, "sonarr", app.services["sonarr"][0]["hostname"], "Seerr refused the update to the existing server")
}

func TestSeerrNotificationOptionsKeepTheirTypeAndReadSecrets(t *testing.T) {
	app := &fakeSeerr{initialized: true}
	cfg := seerrWithAPIKey(app.serve(t))
	cfg.Spec.Notifications = []requestsv1alpha1.SeerrNotificationAgent{{
		Agent: "discord", Enabled: new(true),
		Options: []commonv1alpha1.ConfigField{
			{Name: "botUsername", Value: &commonv1alpha1.FieldValue{Raw: []byte(`"Seerr"`)}},
			{Name: "enableMentions", Value: &commonv1alpha1.FieldValue{Raw: []byte(`false`)}},
			{Name: "webhookUrl", ValueFrom: commonv1alpha1.ValueSource{SecretKeyRef: commonv1alpha1.SecretKeyRef{Name: "seerr-secrets", Key: "token"}}},
		},
	}}

	runSeerr(t, cfg)

	app.mu.Lock()
	defer app.mu.Unlock()
	body := app.authBodies["/api/v1/settings/notifications/discord"]
	require.NotNil(t, body, "the agent was never saved")
	assert.Equal(t, map[string]any{"botUsername": "Seerr", "enableMentions": false, "webhookUrl": "plex-token"}, body["options"])
}

func seerrSecretRef(key string) commonv1alpha1.SecretKeyRef {
	return commonv1alpha1.SecretKeyRef{Name: "seerr-secrets", Key: key}
}

func TestSeerrServerAppRefUsesTheReferencedConnection(t *testing.T) {
	app := &fakeSeerr{initialized: true}
	cfg := seerrWithAPIKey(app.serve(t))
	cfg.Spec.Sonarr = []requestsv1alpha1.SeerrServiceConnection{{
		Name: "Sonarr", AppRef: &commonv1alpha1.AppRef{Kind: "Sonarr", Name: "sonarr"},
	}}
	sonarr := &pvrv1alpha1.Sonarr{Name: "sonarr", Namespace: "media", Spec: pvrv1alpha1.SonarrSpec{
		Connection: commonv1alpha1.AppConnection{URL: "https://media.example.com:8443/sonarr/", APIKeySecretRef: seerrSecretRef("api-key")},
	}}

	runSeerr(t, cfg, sonarr)

	app.mu.Lock()
	defer app.mu.Unlock()
	require.Len(t, app.services["sonarr"], 1)
	created := app.services["sonarr"][0]
	assert.Equal(t, "media.example.com", created["hostname"])
	assert.Equal(t, float64(8443), created["port"])
	assert.Equal(t, true, created["useSsl"])
	assert.Equal(t, "/sonarr", created["baseUrl"])
	assert.Equal(t, "supplied-key", created["apiKey"])
}

func TestSeerrJellyfinAppRefLogsInAsTheJellyfinAdmin(t *testing.T) {
	app := &fakeSeerr{initialized: true}
	cfg := seerrConfig(app.serve(t))
	cfg.Spec.JellyfinAuth = requestsv1alpha1.SeerrJellyfinAuth{AppRef: &commonv1alpha1.AppRef{Kind: "Jellyfin", Name: "jellyfin"}}
	jellyfin := &mediaserversv1alpha1.Jellyfin{Name: "jellyfin", Namespace: "media", Spec: mediaserversv1alpha1.JellyfinSpec{
		Connection: mediaserversv1alpha1.JellyfinConnection{URL: "http://jellyfin.media.svc:8096/jf"},
		AdminUser:  mediaserversv1alpha1.JellyfinAdminUser{UsernameSecretRef: seerrSecretRef("username"), PasswordSecretRef: seerrSecretRef("password")},
	}}

	runSeerr(t, cfg, jellyfin)

	body := app.authBody("/api/v1/auth/jellyfin")
	require.NotNil(t, body, "never logged in")
	assert.Equal(t, "admin", body["username"])
	assert.Equal(t, "hunter2", body["password"])
	assert.Equal(t, "/jf", body["urlBase"])
	server := app.jellyfinServer()
	assert.Equal(t, "jellyfin.media.svc", server["ip"])
	assert.Equal(t, float64(8096), server["port"])
}

func TestSeerrPlexAppRefUsesThePlexToken(t *testing.T) {
	app := &fakeSeerr{initialized: true}
	cfg := seerrConfig(app.serve(t))
	cfg.Spec.PlexAuth = requestsv1alpha1.SeerrPlexAuth{AppRef: &commonv1alpha1.AppRef{Kind: "Plex", Name: "plex"}}
	plex := &mediaserversv1alpha1.Plex{Name: "plex", Namespace: "media", Spec: mediaserversv1alpha1.PlexSpec{
		Connection: mediaserversv1alpha1.PlexConnection{URL: "http://plex.media.svc:32400", TokenSecretRef: seerrSecretRef("token")},
	}}

	runSeerr(t, cfg, plex)

	body := app.authBody("/api/v1/auth/plex")
	require.NotNil(t, body, "never logged in")
	assert.Equal(t, "plex-token", body["authToken"])
	assert.Equal(t, "plex.media.svc", app.plexServer()["ip"])
}

// Without an API key, the media server is how the operator signs in, so a
// missing reference stops the pass before anything is sent.
func TestSeerrMissingMediaServerAppRefStopsBeforeSigningIn(t *testing.T) {
	app := &fakeSeerr{initialized: true}
	cfg := seerrConfig(app.serve(t))
	cfg.Spec.JellyfinAuth = requestsv1alpha1.SeerrJellyfinAuth{AppRef: &commonv1alpha1.AppRef{Kind: "Jellyfin", Name: "jellyfin"}}

	got := runSeerr(t, cfg)

	assert.False(t, app.saw("POST", "/api/v1/auth/jellyfin"))
	assert.Empty(t, app.writes())
	synced := meta.FindStatusCondition(got.Status.Conditions, "Synced")
	require.NotNil(t, synced)
	assert.Equal(t, "ReferenceNotFound", synced.Reason)
	assert.Equal(t, `jellyfinAuth appRef: Jellyfin "jellyfin" not found`, synced.Message)
}

func TestSeerrMediaServerAppRefWithoutAUsableURLIsInvalidConfig(t *testing.T) {
	app := &fakeSeerr{initialized: true}
	cfg := seerrConfig(app.serve(t))
	cfg.Spec.JellyfinAuth = requestsv1alpha1.SeerrJellyfinAuth{AppRef: &commonv1alpha1.AppRef{Kind: "Jellyfin", Name: "jellyfin"}}
	jellyfin := &mediaserversv1alpha1.Jellyfin{Name: "jellyfin", Namespace: "media", Spec: mediaserversv1alpha1.JellyfinSpec{
		Connection: mediaserversv1alpha1.JellyfinConnection{URL: "http://:8096"},
		AdminUser:  mediaserversv1alpha1.JellyfinAdminUser{UsernameSecretRef: seerrSecretRef("username"), PasswordSecretRef: seerrSecretRef("password")},
	}}

	got := runSeerr(t, cfg, jellyfin)

	assert.False(t, app.saw("POST", "/api/v1/auth/jellyfin"))
	synced := meta.FindStatusCondition(got.Status.Conditions, "Synced")
	require.NotNil(t, synced)
	assert.Equal(t, "InvalidConfig", synced.Reason)
	assert.Equal(t, `jellyfinAuth appRef: Jellyfin "jellyfin" has no usable connection URL`, synced.Message)
}

// With an API key the rest of Seerr still syncs, and a server whose reference
// is missing holds back pruning that kind.
func TestSeerrMissingServerAppRefSkipsOnlyThatServer(t *testing.T) {
	app := &fakeSeerr{initialized: true, services: map[string][]map[string]any{
		"sonarr": {{"id": float64(0), "name": "Old", "hostname": "old", "port": float64(8989), "apiKey": "k"}},
	}}
	cfg := seerrWithAPIKey(app.serve(t))
	cfg.Spec.Reconcile = &commonv1alpha1.ReconcileConfig{Prune: new(true)}
	cfg.Status.ManagedResources = []commonv1alpha1.ManagedResource{{Type: "sonarr", Names: []string{"Old"}}}
	cfg.Spec.Main = &requestsv1alpha1.SeerrMain{ApplicationTitle: "Requests"}
	cfg.Spec.Sonarr = []requestsv1alpha1.SeerrServiceConnection{{
		Name: "Sonarr", AppRef: &commonv1alpha1.AppRef{Kind: "Sonarr", Name: "sonarr"},
	}}

	got := runSeerr(t, cfg)

	assert.True(t, app.saw("POST", "/api/v1/settings/main"), "main settings were held back too")
	assert.Equal(t, []string{"Old"}, seerrServiceNames(app, "sonarr"), "a server was pruned or created while its reference was missing")
	synced := meta.FindStatusCondition(got.Status.Conditions, "Synced")
	require.NotNil(t, synced)
	assert.Contains(t, synced.Message, `sonarr(Sonarr): appRef: Sonarr "sonarr" not found`)
}

func seerrLibrariesApp(listed bool) *fakeSeerr {
	return &fakeSeerr{initialized: true, listed: listed,
		jellyfin: map[string]any{"ip": "jellyfin", "port": float64(8096), "useSsl": false, "urlBase": ""},
		libraries: []map[string]any{
			{"id": "movies", "name": "Movies", "enabled": false},
			{"id": "shows", "name": "Shows", "enabled": true},
			{"id": "home", "name": "Home Videos", "enabled": true},
		}}
}

func TestSeerrEnablesOnlyTheDeclaredLibraries(t *testing.T) {
	app := seerrLibrariesApp(true)
	cfg := jellyfinServerConfig(app.serve(t))
	cfg.Spec.Libraries = []string{"Movies", "Shows"}

	runSeerr(t, cfg)

	assert.True(t, app.libraryEnabled("movies"), "a declared library was left disabled")
	assert.True(t, app.libraryEnabled("shows"))
	assert.False(t, app.libraryEnabled("home"), "a library left out was left enabled")
	assert.False(t, app.saw("PUT", "/api/v1/settings/jellyfin/library/shows"), "rewrote a library that already matched")
	assert.False(t, app.saw("POST", "/api/v1/settings/jellyfin/library/sync"), "synced although Seerr listed every declared library")
}

func TestSeerrSyncsLibrariesItDoesNotListYet(t *testing.T) {
	app := seerrLibrariesApp(false)
	cfg := jellyfinServerConfig(app.serve(t))
	cfg.Spec.Libraries = []string{"Movies"}

	runSeerr(t, cfg)

	require.True(t, app.saw("POST", "/api/v1/settings/jellyfin/library/sync"), "never synced the libraries Seerr didn't list")
	assert.True(t, app.libraryEnabled("movies"))
	assert.False(t, app.libraryEnabled("shows"))
}

func TestSeerrReportsALibraryTheServerLacks(t *testing.T) {
	app := seerrLibrariesApp(true)
	cfg := jellyfinServerConfig(app.serve(t))
	cfg.Spec.Libraries = []string{"Movies", "Anime"}

	got := runSeerr(t, cfg)

	synced := meta.FindStatusCondition(got.Status.Conditions, "Synced")
	require.NotNil(t, synced)
	assert.Equal(t, metav1.ConditionFalse, synced.Status)
	assert.Contains(t, synced.Message, `the media server has no library named "Anime"`)
	assert.True(t, app.libraryEnabled("home"), "changed libraries while a declared one was missing")
}

func TestSeerrSetupEnablesOnlyTheDeclaredLibraries(t *testing.T) {
	app := &fakeSeerr{libraries: []map[string]any{
		{"id": "movies", "name": "Movies", "enabled": false},
		{"id": "home", "name": "Home Videos", "enabled": false},
	}}
	cfg := jellyfinServerConfig(app.serve(t))
	cfg.Spec.Libraries = []string{"Movies"}

	runSeerr(t, cfg)

	require.True(t, app.saw("POST", "/api/v1/settings/initialize"), "setup never completed")
	assert.True(t, app.libraryEnabled("movies"))
	assert.False(t, app.libraryEnabled("home"), "setup enabled a library left out")
}

func TestSeerrTrustsTheProxyInFrontOfIt(t *testing.T) {
	app := &fakeSeerr{initialized: true}
	cfg := seerrWithAPIKey(app.serve(t))
	cfg.Spec.Network = requestsv1alpha1.SeerrNetwork{TrustProxy: new(true)}

	runSeerr(t, cfg)

	app.mu.Lock()
	defer app.mu.Unlock()
	assert.Equal(t, true, app.network["trustProxy"])
	assert.Equal(t, false, app.network["csrfProtection"], "CSRF protection would lock the operator out")
}

func TestSeerrLeavesMatchingNetworkSettingsAlone(t *testing.T) {
	app := &fakeSeerr{initialized: true, network: map[string]any{"csrfProtection": false, "trustProxy": true}}
	cfg := seerrWithAPIKey(app.serve(t))
	cfg.Spec.Network = requestsv1alpha1.SeerrNetwork{TrustProxy: new(true)}

	runSeerr(t, cfg)

	require.True(t, app.saw("GET", "/api/v1/settings/network"), "never read the network settings")
	assert.False(t, app.saw("POST", "/api/v1/settings/network"), "rewrote network settings that already matched")
}

func TestSeerrSendsBothMetadataProviders(t *testing.T) {
	app := &fakeSeerr{initialized: true}
	cfg := seerrWithAPIKey(app.serve(t))
	cfg.Spec.MetadataProviders = requestsv1alpha1.SeerrMetadataProviders{Anime: "tvdb"}

	runSeerr(t, cfg)

	app.mu.Lock()
	assert.Equal(t, map[string]any{"tv": "tmdb", "anime": "tvdb"}, app.metadata, "Seerr saves the pair from what it is sent")
	app.mu.Unlock()

	runSeerr(t, cfg)
	puts := 0
	for _, r := range app.writes() {
		if r == "PUT /api/v1/settings/metadatas" {
			puts++
		}
	}
	assert.Equal(t, 1, puts, "rewrote metadata providers that already matched")
}

func TestSeerrReportsUnreachableMetadataProviders(t *testing.T) {
	app := &fakeSeerr{initialized: true, unreachableProviders: true}
	cfg := seerrWithAPIKey(app.serve(t))
	cfg.Spec.MetadataProviders = requestsv1alpha1.SeerrMetadataProviders{TV: "tvdb"}

	got := runSeerr(t, cfg)

	synced := meta.FindStatusCondition(got.Status.Conditions, "Synced")
	require.NotNil(t, synced)
	assert.Contains(t, synced.Message, "metadataProviders: the providers chosen weren't reachable from Seerr, which kept its own")
}

func TestSeerrReschedulesOnlyTheJobsThatDiffer(t *testing.T) {
	app := &fakeSeerr{initialized: true, jobs: map[string]string{
		"sonarr-scan": "0 30 4 * * *", "radarr-scan": "0 0 4 * * *", "jellyfin-full-scan": "0 0 3 * * *",
	}}
	cfg := seerrWithAPIKey(app.serve(t))
	cfg.Spec.JobSchedules = []requestsv1alpha1.SeerrJobSchedule{
		{ID: "sonarr-scan", Schedule: "0 0 */6 * * *"},
		{ID: "radarr-scan", Schedule: "0 0 4 * * *"},
		{ID: "plex-full-scan", Schedule: "0 0 2 * * *"},
	}

	got := runSeerr(t, cfg)

	assert.True(t, app.saw("POST", "/api/v1/settings/jobs/sonarr-scan/schedule"))
	assert.False(t, app.saw("POST", "/api/v1/settings/jobs/radarr-scan/schedule"), "rescheduled a job that already matched")
	assert.False(t, app.saw("POST", "/api/v1/settings/jobs/plex-full-scan/schedule"))
	app.mu.Lock()
	assert.Equal(t, "0 0 */6 * * *", app.jobs["sonarr-scan"])
	app.mu.Unlock()
	synced := meta.FindStatusCondition(got.Status.Conditions, "Synced")
	require.NotNil(t, synced)
	assert.Contains(t, synced.Message,
		"jobSchedule(plex-full-scan): Seerr runs no such job; it runs jellyfin-full-scan, radarr-scan, sonarr-scan")
}
