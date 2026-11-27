package pvr

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	pvrv1alpha1 "github.com/kyleseneker/arrchitect/api/pvr/v1alpha1"
	ctrlcommon "github.com/kyleseneker/arrchitect/internal/controller/common"
	"github.com/kyleseneker/arrchitect/internal/testnet"
)

const conditionTrue = "True"

// The four PVR controllers are the same implementation with a different API
// version, so every case runs against all four rather than trusting that
// Sonarr passing implies the rest do.

type buildOpts struct {
	notification bool
	// missingSecret reads the notification's URL from a Secret that doesn't
	// exist.
	missingSecret bool
	naming        bool
	prune         bool
	managed       map[string][]string
}

type pvrApp struct {
	name   string
	apiVer string
	build  func(url string, o buildOpts) ctrlcommon.ConfigResource
	empty  func() ctrlcommon.ConfigResource
	newRec func(c client.Client, s *runtime.Scheme) reconcile.Reconciler
}

func baseSpec(o buildOpts) (commonv1alpha1.AppConnection, []commonv1alpha1.RootFolder, []commonv1alpha1.Notification, *commonv1alpha1.ReconcileConfig) {
	conn := commonv1alpha1.AppConnection{
		APIKeySecretRef: commonv1alpha1.SecretKeyRef{Name: "app-credentials", Key: "api-key"},
	}
	// Lidarr and Readarr need default profiles on a root folder; the others ignore them.
	roots := []commonv1alpha1.RootFolder{{Path: "/media", DefaultQualityProfileName: "Any", DefaultMetadataProfileName: "Standard"}}
	var notifications []commonv1alpha1.Notification
	if o.notification {
		url := commonv1alpha1.ConfigField{Name: "url", Value: &commonv1alpha1.FieldValue{Raw: []byte(`"http://hook"`)}}
		if o.missingSecret {
			url = commonv1alpha1.ConfigField{Name: "url", ValueFrom: commonv1alpha1.ValueSource{
				SecretKeyRef: commonv1alpha1.SecretKeyRef{Name: "missing", Key: "url"},
			}}
		}
		notifications = []commonv1alpha1.Notification{{Name: "Webhook", Implementation: "Webhook", Fields: []commonv1alpha1.ConfigField{url}}}
	}
	var rec *commonv1alpha1.ReconcileConfig
	if o.prune {
		enabled := true
		rec = &commonv1alpha1.ReconcileConfig{Prune: &enabled}
	}
	return conn, roots, notifications, rec
}

func meta() metav1.ObjectMeta {
	return metav1.ObjectMeta{Name: "app", Namespace: "media"}
}

var pvrApps = []pvrApp{
	{
		name: "sonarr", apiVer: "v3",
		empty: func() ctrlcommon.ConfigResource { return &pvrv1alpha1.Sonarr{} },
		newRec: func(c client.Client, s *runtime.Scheme) reconcile.Reconciler {
			return &SonarrReconciler{Client: c, Recorder: events.NewFakeRecorder(50)}
		},
		build: func(url string, o buildOpts) ctrlcommon.ConfigResource {
			conn, roots, notifications, rec := baseSpec(o)
			conn.URL = url
			c := &pvrv1alpha1.Sonarr{ObjectMeta: meta(), Spec: pvrv1alpha1.SonarrSpec{
				Connection: conn, RootFolders: roots, Notifications: notifications, Reconcile: rec,
			}}
			if o.naming {
				yes := true
				c.Spec.Naming = &pvrv1alpha1.SonarrNaming{RenameEpisodes: &yes}
			}
			c.SetManagedResources(o.managed)
			return c
		},
	},
	{
		name: "radarr", apiVer: "v3",
		empty: func() ctrlcommon.ConfigResource { return &pvrv1alpha1.Radarr{} },
		newRec: func(c client.Client, s *runtime.Scheme) reconcile.Reconciler {
			return &RadarrReconciler{Client: c, Recorder: events.NewFakeRecorder(50)}
		},
		build: func(url string, o buildOpts) ctrlcommon.ConfigResource {
			conn, roots, notifications, rec := baseSpec(o)
			conn.URL = url
			c := &pvrv1alpha1.Radarr{ObjectMeta: meta(), Spec: pvrv1alpha1.RadarrSpec{
				Connection: conn, RootFolders: roots, Notifications: notifications, Reconcile: rec,
			}}
			if o.naming {
				yes := true
				c.Spec.Naming = &pvrv1alpha1.RadarrNaming{RenameMovies: &yes}
			}
			c.SetManagedResources(o.managed)
			return c
		},
	},
	{
		name: "lidarr", apiVer: "v1",
		empty: func() ctrlcommon.ConfigResource { return &pvrv1alpha1.Lidarr{} },
		newRec: func(c client.Client, s *runtime.Scheme) reconcile.Reconciler {
			return &LidarrReconciler{Client: c, Recorder: events.NewFakeRecorder(50)}
		},
		build: func(url string, o buildOpts) ctrlcommon.ConfigResource {
			conn, roots, notifications, rec := baseSpec(o)
			conn.URL = url
			c := &pvrv1alpha1.Lidarr{ObjectMeta: meta(), Spec: pvrv1alpha1.LidarrSpec{
				Connection: conn, RootFolders: roots, Notifications: notifications, Reconcile: rec,
			}}
			if o.naming {
				yes := true
				c.Spec.Naming = &pvrv1alpha1.LidarrNaming{RenameTracks: &yes}
			}
			c.SetManagedResources(o.managed)
			return c
		},
	},
	{
		name: "readarr", apiVer: "v1",
		empty: func() ctrlcommon.ConfigResource { return &pvrv1alpha1.Readarr{} },
		newRec: func(c client.Client, s *runtime.Scheme) reconcile.Reconciler {
			return &ReadarrReconciler{Client: c, Recorder: events.NewFakeRecorder(50)}
		},
		build: func(url string, o buildOpts) ctrlcommon.ConfigResource {
			conn, roots, notifications, rec := baseSpec(o)
			conn.URL = url
			c := &pvrv1alpha1.Readarr{ObjectMeta: meta(), Spec: pvrv1alpha1.ReadarrSpec{
				Connection: conn, RootFolders: roots, Notifications: notifications, Reconcile: rec,
			}}
			if o.naming {
				yes := true
				c.Spec.Naming = &pvrv1alpha1.ReadarrNaming{RenameBooks: &yes}
			}
			c.SetManagedResources(o.managed)
			return c
		},
	},
}

// fakeArr answers the subset of the API the reconciler touches and records
// every request. Existing resources are seeded by endpoint so prune can be
// exercised.
type fakeArr struct {
	apiVer   string
	mu       sync.Mutex
	requests []string
	bodies   map[string][]map[string]any
	existing map[string][]map[string]any
}

// sent returns the bodies written to path, by POST or PUT.
func (f *fakeArr) sent(path string) []map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.bodies[path]
}

func (f *fakeArr) saw(method, path string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Contains(f.requests, method+" "+path)
}

func (f *fakeArr) sawPrefix(method, prefix string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.requests {
		if strings.HasPrefix(r, method+" "+prefix) {
			return true
		}
	}
	return false
}

func (f *fakeArr) path(suffix string) string { return "/api/" + f.apiVer + "/" + suffix }

// unhealthyURL returns an app that answers but fails its health check. This
// drives the same Ping-failure branch as an unroutable host, without depending
// on how the host OS treats connections to a closed port — macOS drops them and
// the dial hangs for the full timeout, Linux refuses immediately.
func unhealthyURL(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", net.JoinHostPort(testnet.RoutableAddr(t), "0"))
	if err != nil {
		t.Skipf("cannot bind: %v", err)
	}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	_ = srv.Listener.Close()
	srv.Listener = ln
	srv.Start()
	t.Cleanup(srv.Close)
	return srv.URL
}

func newFakeArr(t *testing.T, apiVer string, existing map[string][]map[string]any) (*fakeArr, string) {
	t.Helper()
	if existing == nil {
		existing = map[string][]map[string]any{}
	}
	// A fresh app ships default profiles that root folders can reference.
	for kind, name := range map[string]string{"qualityprofile": "Any", "metadataprofile": "Standard"} {
		if _, ok := existing[kind]; !ok {
			existing[kind] = []map[string]any{{"id": float64(1), "name": name}}
		}
	}
	f := &fakeArr{apiVer: apiVer, existing: existing, bodies: map[string][]map[string]any{}}
	prefix := "/api/" + apiVer + "/"

	ln, err := net.Listen("tcp", net.JoinHostPort(testnet.RoutableAddr(t), "0"))
	if err != nil {
		t.Skipf("cannot bind a routable listener: %v", err)
	}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.requests = append(f.requests, r.Method+" "+r.URL.Path)
		f.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		if strings.HasPrefix(r.URL.Path, prefix+"config/") {
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 1})
			return
		}
		switch r.Method {
		case http.MethodGet:
			if r.URL.Path == prefix+"system/status" {
				_ = json.NewEncoder(w).Encode(map[string]any{"version": "4.0.0"})
				return
			}
			list := f.existing[strings.TrimPrefix(r.URL.Path, prefix)]
			if list == nil {
				list = []map[string]any{}
			}
			_ = json.NewEncoder(w).Encode(list)
		case http.MethodPost, http.MethodPut:
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body == nil {
				body = map[string]any{}
			}
			f.mu.Lock()
			f.bodies[r.URL.Path] = append(f.bodies[r.URL.Path], body)
			f.mu.Unlock()
			body["id"] = 99
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(body)
		case http.MethodDelete:
			w.WriteHeader(http.StatusOK)
		}
	}))
	_ = srv.Listener.Close()
	srv.Listener = ln
	srv.Start()
	t.Cleanup(srv.Close)
	return f, srv.URL
}

func apiKeySecret() *corev1.Secret {
	return &corev1.Secret{
		Name: "app-credentials", Namespace: "media",
		Data: map[string][]byte{"api-key": []byte("test-key")},
	}
}

// run wires a reconciler around the given objects and reconciles once.
func run(t *testing.T, app pvrApp, objs ...client.Object) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme), "adding client-go scheme")
	require.NoError(t, pvrv1alpha1.AddToScheme(scheme), "adding pvr scheme")
	b := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(app.empty())
	for _, o := range objs {
		b = b.WithObjects(o)
	}
	c := b.Build()
	r := app.newRec(c, scheme)
	_, err := r.Reconcile(context.Background(), ctrl.Request{
		Name: "app", Namespace: "media",
	})
	require.NoError(t, err, "reconcile")
	return c
}

func conditionStatus(t *testing.T, c client.Client, app pvrApp, condType string) string {
	t.Helper()
	obj := app.empty()
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "app", Namespace: "media"}, obj), "fetching config")
	for _, cond := range *obj.GetConditions() {
		if cond.Type == condType {
			return string(cond.Status)
		}
	}
	return ""
}

func TestReconcileWritesDeclaredResources(t *testing.T) {
	for _, app := range pvrApps {
		t.Run(app.name, func(t *testing.T) {
			fa, url := newFakeArr(t, app.apiVer, nil)
			cfg := app.build(url, buildOpts{notification: true, naming: true})
			c := run(t, app, apiKeySecret(), cfg)

			assert.True(t, fa.saw("POST", fa.path("rootfolder")), "root folder was never created")
			assert.True(t, fa.saw("POST", fa.path("notification")), "notification was never created")
			// Settings are written to /config/<section>/<id>.
			assert.True(t, fa.sawPrefix("PUT", fa.path("config/naming/")), "naming settings were never pushed")
			assert.Equal(t, conditionTrue, conditionStatus(t, c, app, "Synced"), "Synced")

			obj := app.empty()
			require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "app", Namespace: "media"}, obj), "fetching config")
			assert.NotEmpty(t, (obj.GetManagedResources())["notifications"],
				"managedResources did not record the notification: %+v", obj.GetManagedResources())
		})
	}
}

func TestReconcileMarksUnhealthyAppNotReady(t *testing.T) {
	for _, app := range pvrApps {
		t.Run(app.name, func(t *testing.T) {
			cfg := app.build(unhealthyURL(t), buildOpts{})
			c := run(t, app, apiKeySecret(), cfg)
			assert.NotEqual(t, conditionTrue, conditionStatus(t, c, app, "Ready"), "Ready = True for an app failing its health check")
		})
	}
}

func TestReconcileMissingSecretDoesNotContactApp(t *testing.T) {
	for _, app := range pvrApps {
		t.Run(app.name, func(t *testing.T) {
			fa, url := newFakeArr(t, app.apiVer, nil)
			cfg := app.build(url, buildOpts{})
			c := run(t, app, cfg) // no Secret
			assert.False(t, fa.sawPrefix("GET", "/api/"), "reconciler contacted the app despite failing to resolve its API key")
			assert.NotEqual(t, conditionTrue, conditionStatus(t, c, app, "Synced"), "Synced = True despite a missing secret")
		})
	}
}

// A Secret only one kind uses holds back that kind, not the app.
func TestReconcileMissingItemSecretSkipsOnlyItsKind(t *testing.T) {
	for _, app := range pvrApps {
		t.Run(app.name, func(t *testing.T) {
			fa, url := newFakeArr(t, app.apiVer, nil)
			cfg := app.build(url, buildOpts{notification: true, missingSecret: true, naming: true})
			c := run(t, app, apiKeySecret(), cfg)

			assert.True(t, fa.saw("POST", fa.path("rootfolder")), "the root folder was held back too")
			assert.True(t, fa.sawPrefix("PUT", fa.path("config/naming/")), "the settings were held back too")
			assert.False(t, fa.saw("POST", fa.path("notification")), "a notification was written without its URL")
			assert.NotEqual(t, conditionTrue, conditionStatus(t, c, app, "Ready"))
			assert.NotEqual(t, conditionTrue, conditionStatus(t, c, app, "Synced"))

			obj := app.empty()
			require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "app", Namespace: "media"}, obj))
			synced := apimeta.FindStatusCondition(*obj.GetConditions(), "Synced")
			require.NotNil(t, synced)
			assert.Equal(t, "SyncFailed", synced.Reason)
			assert.Contains(t, synced.Message, `notifications: notification Webhook field "url": secret "missing" not found`)
		})
	}
}

// Prune must never remove resources the operator did not create.
// status.managedResources is the record of what it made, so an empty record
// means everything in the app predates the operator.
func TestPruneLeavesUnmanagedResourcesAlone(t *testing.T) {
	for _, app := range pvrApps {
		t.Run(app.name, func(t *testing.T) {
			fa, url := newFakeArr(t, app.apiVer, map[string][]map[string]any{
				"notification": {{"id": float64(7), "name": "hand-made-hook"}},
			})
			cfg := app.build(url, buildOpts{notification: true, prune: true})
			run(t, app, apiKeySecret(), cfg)

			// Guard against a vacuous pass: if the reconcile never reached the
			// app, the absence of a DELETE would prove nothing.
			require.True(t, fa.saw("POST", fa.path("notification")), "reconcile never reached the app, so the DELETE check is meaningless")
			assert.False(t, fa.sawPrefix("DELETE", fa.path("notification")), "prune deleted a notification the operator never created")
		})
	}
}

// The mirror: once a resource is recorded as operator-created, dropping it from
// the spec does prune it.
func TestPruneRemovesPreviouslyManagedResources(t *testing.T) {
	for _, app := range pvrApps {
		t.Run(app.name, func(t *testing.T) {
			fa, url := newFakeArr(t, app.apiVer, map[string][]map[string]any{
				"notification": {{"id": float64(7), "name": "retired-hook"}},
			})
			cfg := app.build(url, buildOpts{
				notification: true, prune: true,
				managed: map[string][]string{"notifications": {"Webhook", "retired-hook"}},
			})
			run(t, app, apiKeySecret(), cfg)

			assert.True(t, fa.sawPrefix("DELETE", fa.path("notification")), "prune did not remove a resource it had previously created")
		})
	}
}

func TestDeletePolicyCleansOwnedResourcesAcrossPVRControllers(t *testing.T) {
	for _, app := range pvrApps {
		t.Run(app.name, func(t *testing.T) {
			fa, url := newFakeArr(t, app.apiVer, map[string][]map[string]any{"notification": {{"id": float64(7), "name": "owned"}, {"id": float64(8), "name": "manual"}}})
			cfg := app.build(url, buildOpts{prune: true, managed: map[string][]string{"notifications": {"owned"}, "tags": {"shared"}, "rootFolders": {"/media"}}})
			policy := "delete"
			cfg.GetReconcileConfig().DeletionPolicy = policy
			cfg.SetFinalizers([]string{ctrlcommon.Finalizer})
			now := metav1.Now()
			cfg.SetDeletionTimestamp(&now)
			c := run(t, app, apiKeySecret(), cfg)
			require.True(t, fa.saw("DELETE", fa.path("notification/7")), "owned resource was not cleaned up")
			require.False(t, fa.saw("DELETE", fa.path("notification/8")), "manual resource was deleted")
			require.False(t, fa.sawPrefix("DELETE", fa.path("tag")), "protected resources were deleted")
			require.False(t, fa.sawPrefix("DELETE", fa.path("rootfolder")), "protected resources were deleted")
			err := c.Get(context.Background(), client.ObjectKeyFromObject(cfg), app.empty())
			require.True(t, apierrors.IsNotFound(err), "finalizer not released: %v", err)
		})
	}
}

func TestRemoteMappingsReachEveryPVRController(t *testing.T) {
	for _, app := range pvrApps {
		t.Run(app.name, func(t *testing.T) {
			fa, url := newFakeArr(t, app.apiVer, nil)
			cfg := app.build(url, buildOpts{})
			mappings := []commonv1alpha1.RemotePathMapping{{Host: "seedbox", RemotePath: "/downloads", LocalPath: "/data"}}
			switch obj := cfg.(type) {
			case *pvrv1alpha1.Sonarr:
				obj.Spec.RemotePathMappings = mappings
			case *pvrv1alpha1.Radarr:
				obj.Spec.RemotePathMappings = mappings
			case *pvrv1alpha1.Lidarr:
				obj.Spec.RemotePathMappings = mappings
			case *pvrv1alpha1.Readarr:
				obj.Spec.RemotePathMappings = mappings
			}
			c := run(t, app, apiKeySecret(), cfg)
			require.True(t, fa.saw("POST", "/api/"+app.apiVer+"/remotepathmapping"), "mapping was not created")
			require.Equal(t, conditionTrue, conditionStatus(t, c, app, "Synced"), "Synced")
		})
	}
}

func withReleaseProfiles(cfg ctrlcommon.ConfigResource, profiles []commonv1alpha1.ReleaseProfile) bool {
	switch c := cfg.(type) {
	case *pvrv1alpha1.Sonarr:
		c.Spec.ReleaseProfiles = profiles
	case *pvrv1alpha1.Radarr:
		c.Spec.ReleaseProfiles = profiles
	default:
		return false
	}
	return true
}

func TestReleaseProfilesResolveTheirIndexerAndTags(t *testing.T) {
	for _, app := range pvrApps {
		t.Run(app.name, func(t *testing.T) {
			fa, url := newFakeArr(t, app.apiVer, map[string][]map[string]any{
				"indexer": {{"id": float64(3), "name": "NZBgeek"}},
				"tag":     {{"id": float64(5), "label": "anime"}},
			})
			cfg := app.build(url, buildOpts{})
			if !withReleaseProfiles(cfg, []commonv1alpha1.ReleaseProfile{{
				Name: "No x265", Ignored: []string{"x265", "/HEVC/i"}, IndexerName: "NZBgeek", TagLabels: []string{"anime"},
			}}) {
				t.Skip("no release profiles")
			}
			c := run(t, app, apiKeySecret(), cfg)

			sent := fa.sent(fa.path("releaseprofile"))
			require.Len(t, sent, 1)
			assert.Equal(t, "No x265", sent[0]["name"])
			assert.Equal(t, []any{"x265", "/HEVC/i"}, sent[0]["ignored"])
			assert.Equal(t, float64(3), sent[0]["indexerId"])
			assert.Equal(t, []any{float64(5)}, sent[0]["tags"])
			assert.Equal(t, true, sent[0]["enabled"], "a new profile is enabled")
			obj := app.empty()
			require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "app", Namespace: "media"}, obj))
			assert.Equal(t, []string{"No x265"}, obj.GetManagedResources()["releaseProfiles"])
		})
	}
}

func TestReleaseProfileNamingAMissingIndexerIsNotWritten(t *testing.T) {
	fa, url := newFakeArr(t, "v3", nil)
	cfg := pvrApps[0].build(url, buildOpts{notification: true})
	withReleaseProfiles(cfg, []commonv1alpha1.ReleaseProfile{{Name: "Only NZBgeek", Required: []string{"1080p"}, IndexerName: "NZBgeek"}})
	c := run(t, pvrApps[0], apiKeySecret(), cfg)

	assert.False(t, fa.saw("POST", fa.path("releaseprofile")), "wrote a profile whose indexer doesn't exist")
	assert.True(t, fa.saw("POST", fa.path("notification")), "the other kinds were held back too")
	assert.Equal(t, "False", conditionStatus(t, c, pvrApps[0], "Synced"))
}
