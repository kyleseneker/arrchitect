package automation

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
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

	automationv1alpha1 "github.com/kyleseneker/arrchitect/api/automation/v1alpha1"
	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	downloadsv1alpha1 "github.com/kyleseneker/arrchitect/api/downloads/v1alpha1"
	pvrv1alpha1 "github.com/kyleseneker/arrchitect/api/pvr/v1alpha1"
	autobrrclient "github.com/kyleseneker/arrchitect/internal/client/autobrr"
)

const conditionTrue = "True"

type fakeAutobrr struct {
	mu       sync.Mutex
	requests []string
	bodies   map[string]map[string]any
	existing map[string][]map[string]any
	// stores lists what is created, as Autobrr does.
	stores bool
	// forbidden are paths refused as Autobrr 1.91 refuses an API key without
	// their scope.
	forbidden []string
}

func (f *fakeAutobrr) posted(path string) map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.bodies[http.MethodPost+" "+path]
}

func (f *fakeAutobrr) saw(method, path string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Contains(f.requests, method+" "+path)
}

func (f *fakeAutobrr) serve(t *testing.T) string {
	t.Helper()
	if f.existing == nil {
		f.existing = map[string][]map[string]any{}
	}
	ln, err := net.Listen("tcp", net.JoinHostPort(routableAddr(t), "0"))
	if err != nil {
		t.Skipf("cannot bind a routable listener: %v", err)
	}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		f.requests = append(f.requests, r.Method+" "+r.URL.Path)
		if body != nil {
			if f.bodies == nil {
				f.bodies = map[string]map[string]any{}
			}
			f.bodies[r.Method+" "+r.URL.Path] = body
		}
		path := strings.TrimPrefix(r.URL.Path, "/api/")
		if f.stores && r.Method == http.MethodPost && body != nil {
			body["id"] = float64(len(f.existing[path]) + 1)
			f.existing[path] = append(f.existing[path], body)
		}
		list := f.existing[path]
		f.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		switch {
		case slices.Contains(f.forbidden, path):
			http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
		case strings.HasPrefix(r.URL.Path, "/api/healthz"):
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodGet && isID(r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]):
			// A single resource is listed under its own path, such as indexer/2.
			if len(list) == 1 {
				_ = json.NewEncoder(w).Encode(list[0])
			} else {
				_ = json.NewEncoder(w).Encode(map[string]any{})
			}
		case r.Method == http.MethodGet:
			if list == nil {
				list = []map[string]any{}
			}
			_ = json.NewEncoder(w).Encode(list)
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 1})
		}
	}))
	_ = srv.Listener.Close()
	srv.Listener = ln
	srv.Start()
	t.Cleanup(srv.Close)
	return srv.URL
}

func autobrrSecret() *corev1.Secret {
	return &corev1.Secret{
		Name: "autobrr-credentials", Namespace: "media",
		Data: map[string][]byte{"api-key": []byte("test-key")},
	}
}

func autobrrConfig(url string) *automationv1alpha1.Autobrr {
	return &automationv1alpha1.Autobrr{
		Name: "autobrr", Namespace: "media",
		Spec: automationv1alpha1.AutobrrSpec{
			Connection: commonv1alpha1.AppConnection{
				URL:             url,
				APIKeySecretRef: commonv1alpha1.SecretKeyRef{Name: "autobrr-credentials", Key: "api-key"},
			},
		},
	}
}

func runAutobrr(t *testing.T, cfg *automationv1alpha1.Autobrr, withSecret bool, objs ...client.Object) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme), "adding client-go scheme")
	require.NoError(t, automationv1alpha1.AddToScheme(scheme), "adding automation scheme")
	require.NoError(t, pvrv1alpha1.AddToScheme(scheme), "adding pvr scheme")
	require.NoError(t, downloadsv1alpha1.AddToScheme(scheme), "adding downloads scheme")
	b := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&automationv1alpha1.Autobrr{}).WithObjects(append(objs, cfg)...)
	if withSecret {
		b = b.WithObjects(autobrrSecret())
	}
	c := b.Build()
	r := &AutobrrReconciler{Client: c, Scheme: scheme, Recorder: events.NewFakeRecorder(50)}
	_, err := r.Reconcile(context.Background(), ctrl.Request{
		Name: "autobrr", Namespace: "media",
	})
	require.NoError(t, err, "reconcile")
	return c
}

func autobrrSynced(t *testing.T, c client.Client) string {
	t.Helper()
	var cfg automationv1alpha1.Autobrr
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "autobrr", Namespace: "media"}, &cfg), "fetching config")
	for _, cond := range cfg.Status.Conditions {
		if cond.Type == "Synced" {
			return string(cond.Status)
		}
	}
	return ""
}

func TestAutobrrCreatesDeclaredFilter(t *testing.T) {
	app := &fakeAutobrr{}
	url := app.serve(t)

	cfg := autobrrConfig(url)
	cfg.Spec.Filters = []automationv1alpha1.AutobrrFilter{{Name: "1080p web-dl"}}
	c := runAutobrr(t, cfg, true)

	assert.True(t, app.saw("POST", "/api/filters"), "declared filter was never created")
	assert.Equal(t, conditionTrue, autobrrSynced(t, c), "Synced")
}

// Filters are matched by name, so an existing one must not be recreated on
// every reconcile.
func TestAutobrrDoesNotDuplicateExistingFilter(t *testing.T) {
	app := &fakeAutobrr{existing: map[string][]map[string]any{
		"filters": {{"id": float64(4), "name": "1080p web-dl"}},
	}}
	url := app.serve(t)

	cfg := autobrrConfig(url)
	cfg.Spec.Filters = []automationv1alpha1.AutobrrFilter{{Name: "1080p web-dl"}}
	runAutobrr(t, cfg, true)

	require.True(t, app.saw("GET", "/api/filters"), "reconcile never listed filters, so the duplicate check is meaningless")
	assert.False(t, app.saw("POST", "/api/filters"), "created a duplicate of a filter that already existed")
}

func TestAutobrrMissingSecretDoesNotContactApp(t *testing.T) {
	app := &fakeAutobrr{}
	url := app.serve(t)

	c := runAutobrr(t, autobrrConfig(url), false)

	assert.False(t, app.saw("GET", "/api/healthz/liveness"), "contacted the app despite failing to resolve its API key")
	assert.NotEqual(t, conditionTrue, autobrrSynced(t, c), "Synced = True despite a missing secret")
}

func observedAutobrr(url string) *automationv1alpha1.Autobrr {
	cfg := autobrrConfig(url)
	cfg.Spec.Reconcile = &commonv1alpha1.ReconcileConfig{DriftPolicy: "observe"}
	cfg.Spec.Indexers = []automationv1alpha1.AutobrrIndexer{{Name: "torznab-a", Identifier: "torznab"}}
	return cfg
}

func autobrrWrites(f *fakeAutobrr) []string {
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

func TestAutobrrObserveReportsMissingResourcesWithoutWriting(t *testing.T) {
	app := &fakeAutobrr{existing: map[string][]map[string]any{"indexer/schema": autobrrDefinitions}}
	c := runAutobrr(t, observedAutobrr(app.serve(t)), true)

	assert.Empty(t, autobrrWrites(app), "observe mode wrote to Autobrr")
	var cfg automationv1alpha1.Autobrr
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "autobrr", Namespace: "media"}, &cfg))
	synced := cfg.Status.Conditions[slices.IndexFunc(cfg.Status.Conditions, func(c metav1.Condition) bool { return c.Type == "Synced" })]
	assert.Equal(t, "DriftDetected", synced.Reason)
}

func TestAutobrrObserveTellsADeclaredIndexerFromATypo(t *testing.T) {
	for ref, want := range map[string]string{"torznab-a": "DriftDetected", "typo": "SyncFailed"} {
		app := &fakeAutobrr{existing: map[string][]map[string]any{"indexer/schema": autobrrDefinitions}}
		cfg := observedAutobrr(app.serve(t))
		cfg.Spec.Feeds = []automationv1alpha1.AutobrrFeed{{Name: "Feed", Type: "Torznab", URL: "http://feed", IndexerName: ref}}

		c := runAutobrr(t, cfg, true)

		var got automationv1alpha1.Autobrr
		require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "autobrr", Namespace: "media"}, &got))
		synced := meta.FindStatusCondition(got.Status.Conditions, "Synced")
		require.NotNil(t, synced)
		assert.Equal(t, want, synced.Reason, "a feed referring to %q", ref)
		assert.Empty(t, autobrrWrites(app))
	}
}

func TestAutobrrObserveMatchesWhatExists(t *testing.T) {
	app := &fakeAutobrr{existing: map[string][]map[string]any{
		"indexer":        {{"id": float64(2), "name": "torznab-a"}},
		"indexer/2":      {{"id": float64(2), "name": "torznab-a", "enabled": true, "implementation": "torznab"}},
		"indexer/schema": autobrrDefinitions,
	}}
	c := runAutobrr(t, observedAutobrr(app.serve(t)), true)

	require.True(t, app.saw("GET", "/api/indexer"), "observe never read the indexers")
	assert.Empty(t, autobrrWrites(app))
	assert.Equal(t, conditionTrue, autobrrSynced(t, c))
}

// Autobrr accepts only its own constants; "qBittorrent" is answered with
// "invalid download client type".
func TestAutobrrTypeMapsToAutobrrConstants(t *testing.T) {
	for in, want := range map[string]string{
		"qBittorrent": "QBITTORRENT", "Deluge": "DELUGE_V2", "WatchFolder": "WATCH_FOLDER", "Webhook": "WEBHOOK",
	} {
		assert.Equal(t, want, autobrrType(in), in)
	}
	assert.Equal(t, "NONE", autobrrIRCAuth("None"))
	assert.Equal(t, "NICKSERV", autobrrIRCAuth("NickServ"))
}

// Autobrr reads a password back as <redacted>, so whether it changed is known
// only from what was last written.
func TestUpsertAutobrrComparesRedactedSecretsByWhatWasWritten(t *testing.T) {
	existing := []map[string]any{{
		"id": float64(4), "name": "qBittorrent", "type": "QBITTORRENT", "password": "<redacted>", "tls": false,
	}}
	obj := map[string]any{"name": "qBittorrent", "type": "QBITTORRENT", "password": "s3cret"}
	writes := 0
	ab := &autobrrSync{owner: "media/" + t.Name()}
	upsert := func() {
		t.Helper()
		require.NoError(t, ab.upsert(context.Background(), "download client", "qBittorrent", existing, obj,
			func() error { require.Fail(t, "must not create"); return nil },
			func(int, map[string]any) error { writes++; return nil }, nil))
	}

	upsert()
	assert.Equal(t, 1, writes, "with nothing recorded the secret is sent once")
	upsert()
	assert.Equal(t, 1, writes, "an unchanged download client was rewritten")
	obj["password"] = "rotated"
	upsert()
	assert.Equal(t, 2, writes, "a rotated secret must be written")
	obj["type"] = "DELUGE_V2"
	upsert()
	assert.Equal(t, 3, writes, "a real change must be written")

	observer := &autobrrSync{owner: "media/" + t.Name() + "-observe"}
	observer.Observe = true
	require.NoError(t, observer.upsert(context.Background(), "download client", "qBittorrent", existing,
		map[string]any{"name": "qBittorrent", "type": "QBITTORRENT", "password": "s3cret"}, nil, nil, nil))
	assert.Empty(t, observer.Drift(), "observe mode can't know a secret it never wrote")
}

func TestUpsertAutobrrRemembersTheSecretsItCreatedWith(t *testing.T) {
	stored := []map[string]any{}
	obj := map[string]any{"name": "qBittorrent", "type": "QBITTORRENT", "password": "s3cret"}
	ab := &autobrrSync{owner: "media/" + t.Name()}
	list := func(context.Context) ([]map[string]any, error) { return stored, nil }
	require.NoError(t, ab.upsert(context.Background(), "download client", "qBittorrent", stored, obj,
		func() error {
			stored = append(stored, map[string]any{"id": float64(1), "name": "qBittorrent", "type": "QBITTORRENT", "password": "<redacted>"})
			return nil
		}, nil, list))

	require.NoError(t, ab.upsert(context.Background(), "download client", "qBittorrent", stored, obj, nil,
		func(int, map[string]any) error {
			require.Fail(t, "a new client was rewritten on the next pass")
			return nil
		}, list))
}

func TestAddRequiredFilterListsKeepsDeclaredValues(t *testing.T) {
	filter := map[string]any{"codecs": []string{"x265"}}
	addRequiredFilterLists(filter)
	assert.Equal(t, []string{"x265"}, filter["codecs"])
	assert.Equal(t, []string{}, filter["containers"], "Autobrr rejects a null containers column")
}

var autobrrDefinitions = []map[string]any{
	{"identifier": "ipt", "implementation": "irc", "urls": []any{"https://iptorrents.com/", "https://iptorrents.me/"}},
	{"identifier": "torznab", "implementation": "torznab"},
}

func TestAutobrrIndexerUsesItsDefinition(t *testing.T) {
	app := &fakeAutobrr{existing: map[string][]map[string]any{"indexer/schema": autobrrDefinitions}}
	cfg := autobrrConfig(app.serve(t))
	cfg.Spec.Indexers = []automationv1alpha1.AutobrrIndexer{{
		Name: "IPT", Identifier: "ipt",
		Settings: []commonv1alpha1.ConfigField{{
			Name: "passkey", ValueFrom: commonv1alpha1.ValueSource{SecretKeyRef: commonv1alpha1.SecretKeyRef{Name: "autobrr-credentials", Key: "api-key"}},
		}},
	}}

	runAutobrr(t, cfg, true)

	body := app.posted("/api/indexer")
	require.NotNil(t, body, "the indexer was never created")
	assert.Equal(t, "ipt", body["identifier"])
	assert.Equal(t, "irc", body["implementation"])
	assert.Equal(t, "https://iptorrents.com/", body["base_url"], "an IRC indexer takes its definition's first URL")
	assert.Equal(t, map[string]any{"passkey": "test-key"}, body["settings"])
}

func TestAutobrrRejectsAnUnknownIndexerDefinition(t *testing.T) {
	app := &fakeAutobrr{existing: map[string][]map[string]any{"indexer/schema": autobrrDefinitions}}
	cfg := autobrrConfig(app.serve(t))
	cfg.Spec.Indexers = []automationv1alpha1.AutobrrIndexer{{Name: "X", Identifier: "nope"}}

	c := runAutobrr(t, cfg, true)

	assert.False(t, app.saw(http.MethodPost, "/api/indexer"))
	assert.NotEqual(t, conditionTrue, autobrrSynced(t, c))
}

func TestAutobrrFeedLinksItsIndexerByID(t *testing.T) {
	app := &fakeAutobrr{existing: map[string][]map[string]any{
		"indexer": {{"id": float64(5), "name": "Generic", "implementation": "torznab"}},
	}}
	cfg := autobrrConfig(app.serve(t))
	cfg.Spec.Feeds = []automationv1alpha1.AutobrrFeed{{Name: "Generic", Type: "Torznab", URL: "http://feed", IndexerName: "Generic"}}

	runAutobrr(t, cfg, true)

	body := app.posted("/api/feeds")
	require.NotNil(t, body, "the feed was never created")
	assert.Equal(t, float64(5), body["indexer_id"])
	assert.Equal(t, "TORZNAB", body["type"])
	assert.Equal(t, float64(defaultFeedTimeout), body["timeout"])
	assert.NotContains(t, body, "indexer")
}

func TestAutobrrFeedGetsWhatAutobrrLeavesOutOfACreate(t *testing.T) {
	app := &fakeAutobrr{existing: map[string][]map[string]any{
		"indexer": {{"id": float64(5), "name": "Generic", "implementation": "torznab"}},
	}, stores: true}
	cfg := autobrrConfig(app.serve(t))
	cfg.Spec.Feeds = []automationv1alpha1.AutobrrFeed{{
		Name: "Generic", Type: "Torznab", URL: "http://feed", IndexerName: "Generic",
		Timeout: 30, MaxAge: new(int32(86400)), DownloadType: "Magnet",
		CookieSecretRef: &commonv1alpha1.SecretKeyRef{Name: "autobrr-credentials", Key: "api-key"},
	}}

	runAutobrr(t, cfg, true)

	created := app.posted("/api/feeds")
	require.NotNil(t, created, "the feed was never created")
	assert.Equal(t, float64(30), created["timeout"])
	assert.Equal(t, map[string]any{"download_type": "MAGNET"}, created["settings"])
	app.mu.Lock()
	updated := app.bodies[http.MethodPut+" /api/feeds/1"]
	app.mu.Unlock()
	require.NotNil(t, updated, "the new feed was never given its max age and cookie")
	assert.Equal(t, float64(86400), updated["max_age"])
	assert.Equal(t, "test-key", updated["cookie"])
	assert.Equal(t, float64(5), updated["indexer_id"])
}

func TestAutobrrFeedWithoutMaxAgeOrCookieIsOnlyCreated(t *testing.T) {
	app := &fakeAutobrr{existing: map[string][]map[string]any{
		"indexer": {{"id": float64(5), "name": "Generic", "implementation": "torznab"}},
	}, stores: true}
	cfg := autobrrConfig(app.serve(t))
	cfg.Spec.Feeds = []automationv1alpha1.AutobrrFeed{{Name: "Generic", Type: "Torznab", URL: "http://feed", IndexerName: "Generic"}}

	runAutobrr(t, cfg, true)

	require.NotNil(t, app.posted("/api/feeds"))
	assert.False(t, app.saw(http.MethodPut, "/api/feeds/1"))
}

func TestAutobrrRefusesAnIndexerNameTwoIndexersShare(t *testing.T) {
	app := &fakeAutobrr{existing: map[string][]map[string]any{
		"indexer": {
			{"id": float64(5), "name": "Generic", "implementation": "torznab"},
			{"id": float64(6), "name": "Generic", "implementation": "torznab"},
		},
	}}
	cfg := autobrrConfig(app.serve(t))
	cfg.Spec.Feeds = []automationv1alpha1.AutobrrFeed{{Name: "Generic", Type: "Torznab", URL: "http://feed", IndexerName: "Generic"}}

	c := runAutobrr(t, cfg, true)

	assert.Nil(t, app.posted("/api/feeds"), "a feed was linked to one of two indexers")
	var got automationv1alpha1.Autobrr
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "autobrr", Namespace: "media"}, &got))
	synced := meta.FindStatusCondition(got.Status.Conditions, "Synced")
	require.NotNil(t, synced)
	assert.Contains(t, synced.Message, `reference "Generic" is ambiguous (2 matches)`)
}

func TestAutobrrFilterSendsTagsAsTextAndIndexersByID(t *testing.T) {
	app := &fakeAutobrr{existing: map[string][]map[string]any{
		"indexer": {{"id": float64(7), "name": "B"}, {"id": float64(3), "name": "A"}},
	}}
	cfg := autobrrConfig(app.serve(t))
	cfg.Spec.Filters = []automationv1alpha1.AutobrrFilter{{Name: "TV", Tags: []string{"tv", "hd"}, Indexers: []string{"B", "A"}}}

	runAutobrr(t, cfg, true)

	body := app.posted("/api/filters")
	require.NotNil(t, body, "the filter was never created")
	assert.Equal(t, "tv,hd", body["tags"])
	assert.Equal(t, []any{map[string]any{"id": float64(3)}, map[string]any{"id": float64(7)}}, body["indexers"])
}

func TestAutobrrDownloadClientKeepsItsAPIKeyInSettings(t *testing.T) {
	app := &fakeAutobrr{}
	cfg := autobrrConfig(app.serve(t))
	cfg.Spec.DownloadClients = []automationv1alpha1.AutobrrDownloadClient{{
		Name: "Sonarr", Type: "Sonarr", Host: "http://sonarr:8989",
		APIKeySecretRef: &commonv1alpha1.SecretKeyRef{Name: "autobrr-credentials", Key: "api-key"},
	}}

	runAutobrr(t, cfg, true)

	body := app.posted("/api/downloaders")
	require.NotNil(t, body)
	assert.Equal(t, map[string]any{"apikey": "test-key"}, body["settings"])
	assert.NotContains(t, body, "api_key")
	assert.Equal(t, true, body["enabled"], "a new download client is enabled")
}

func TestAutobrrUpdatesKeepWhatTheResourceLeavesOut(t *testing.T) {
	app := &fakeAutobrr{existing: map[string][]map[string]any{
		"downloaders": {{"id": float64(5), "name": "qBit", "type": "QBITTORRENT", "host": "old", "enabled": false}},
		"irc": {{
			"id": float64(3), "name": "Libera", "enabled": false, "server": "irc.libera.chat", "port": float64(6697),
			"nick": "old", "tls": true, "invite_command": "/msg bot invite",
			"auth":     map[string]any{"mechanism": "SASL_PLAIN", "account": "me", "password": "<redacted>"},
			"channels": []any{map[string]any{"id": float64(1), "name": "#keep"}, map[string]any{"id": float64(2), "name": "#drop"}},
		}},
	}}
	cfg := autobrrConfig(app.serve(t))
	cfg.Spec.DownloadClients = []automationv1alpha1.AutobrrDownloadClient{{Name: "qBit", Type: "qBittorrent", Host: "new"}}
	cfg.Spec.IRCNetworks = []automationv1alpha1.AutobrrIRCNetwork{{
		Name: "Libera", Server: "irc.libera.chat", Port: 6697, Nick: "new",
		Channels: []automationv1alpha1.AutobrrIRCChannel{{Name: "#keep"}},
	}}

	runAutobrr(t, cfg, true)

	app.mu.Lock()
	defer app.mu.Unlock()
	downloader := app.bodies["PUT /api/downloaders"]
	require.NotNil(t, downloader)
	assert.Equal(t, "new", downloader["host"])
	assert.Equal(t, false, downloader["enabled"])

	network := app.bodies["PUT /api/irc/network/3"]
	require.NotNil(t, network)
	assert.Equal(t, "new", network["nick"])
	assert.Equal(t, false, network["enabled"])
	assert.Equal(t, true, network["tls"])
	assert.Equal(t, "/msg bot invite", network["invite_command"])
	assert.Equal(t, map[string]any{"mechanism": "SASL_PLAIN", "account": "me", "password": "<redacted>"}, network["auth"])
	assert.Equal(t, []any{map[string]any{"id": float64(1), "name": "#keep"}}, network["channels"],
		"Autobrr replaces the channels with the ones sent")
}

func isID(segment string) bool {
	_, err := strconv.Atoi(segment)
	return err == nil
}

func TestAutobrrFilterAndActionSendTheirCriteriaUnderAutobrrKeys(t *testing.T) {
	app := &fakeAutobrr{}
	cfg := autobrrConfig(app.serve(t))
	ratio := commonv1alpha1.Decimal("1.5")
	cfg.Spec.Filters = []automationv1alpha1.AutobrrFilter{{
		Name: "UHD", MatchHdr: []string{"DV", "HDR10+"}, ExceptReleaseGroups: "BAD", MaxDownloads: new(int32(5)),
		MaxDownloadsUnit: "DAY", Freeleech: new(true), ExceptTags: []string{"cam", "ts"}, TagsMatchLogic: "ALL",
		Actions: []automationv1alpha1.AutobrrFilterAction{{
			Name: "Hook", Type: "Webhook", WebhookURL: "http://hook", WebhookMethod: "POST",
			WebhookHeaders: []string{"X-Token=abc"}, LimitRatio: &ratio, ContentLayout: "SUBFOLDER_CREATE",
		}},
	}}

	runAutobrr(t, cfg, true)

	filter := app.posted("/api/filters")
	require.NotNil(t, filter, "the filter was never created")
	for key, want := range map[string]any{
		"match_hdr": []any{"DV", "HDR10+"}, "except_release_groups": "BAD", "max_downloads": float64(5),
		"max_downloads_unit": "DAY", "freeleech": true, "except_tags": "cam,ts", "tags_match_logic": "ALL",
	} {
		assert.Equal(t, want, filter[key], key)
	}
	action := app.posted("/api/actions")
	require.NotNil(t, action, "the action was never created")
	for key, want := range map[string]any{
		"webhook_host": "http://hook", "webhook_method": "POST", "webhook_headers": []any{"X-Token=abc"},
		"limit_ratio": 1.5, "content_layout": "SUBFOLDER_CREATE",
	} {
		assert.Equal(t, want, action[key], key)
	}
}

// Autobrr leaves false, 0 and empty values out of what it returns, so a
// declared zero value must not read back as a difference.
func TestAutobrrCoversAZeroValueAutobrrLeftOut(t *testing.T) {
	current := map[string]any{"name": "Send", "category": "tv"}
	assert.True(t, autobrrCovers(current, map[string]any{"name": "Send", "category": "tv", "paused": false, "limit_seed_time": float64(0)}))
	assert.False(t, autobrrCovers(current, map[string]any{"paused": true}))
	assert.False(t, autobrrCovers(map[string]any{"paused": true}, map[string]any{"paused": false}), "a stored true still differs")
}

func TestAutobrrCoversNestedZeroValuesAutobrrLeftOut(t *testing.T) {
	current := map[string]any{"settings": map[string]any{"basic": map[string]any{}, "rules": map[string]any{"enabled": true}}}
	assert.True(t, autobrrCovers(current, map[string]any{"settings": map[string]any{
		"basic": map[string]any{"auth": false, "username": ""}, "rules": map[string]any{"enabled": true, "max_active_downloads": float64(0)},
	}}))
	assert.True(t, autobrrCovers(map[string]any{}, map[string]any{"settings": map[string]any{"basic": map[string]any{"auth": false}}}),
		"a nested block Autobrr left out holds only zero values")
	assert.False(t, autobrrCovers(current, map[string]any{"settings": map[string]any{"rules": map[string]any{"max_active_downloads": float64(3)}}}))
}

func autobrrHTTPAuthSecret() *corev1.Secret {
	return &corev1.Secret{
		Name: "edge", Namespace: "media",
		Data: map[string][]byte{"username": []byte("user"), "password": []byte("pass")},
	}
}

func TestAutobrrDownloadClientSendsHTTPAuthWhereAutobrrAndItsWebUIReadIt(t *testing.T) {
	for authType, want := range map[string]map[string]any{
		"": {
			"basic": map[string]any{"auth": true, "username": "user", "password": "pass"},
			"auth":  map[string]any{"enabled": true, "type": "BASIC_AUTH", "username": "user", "password": "pass"},
		},
		"Digest": {
			"basic": map[string]any{"auth": false, "username": "", "password": ""},
			"auth":  map[string]any{"enabled": true, "type": "DIGEST_AUTH", "username": "user", "password": "pass"},
		},
	} {
		app := &fakeAutobrr{}
		cfg := autobrrConfig(app.serve(t))
		cfg.Spec.DownloadClients = []automationv1alpha1.AutobrrDownloadClient{{
			Name: "rTorrent", Type: "rTorrent", Host: "http://rtorrent/RPC2",
			HTTPAuth: automationv1alpha1.AutobrrHTTPAuth{
				Type:              authType,
				UsernameSecretRef: commonv1alpha1.SecretKeyRef{Name: "edge", Key: "username"},
				PasswordSecretRef: commonv1alpha1.SecretKeyRef{Name: "edge", Key: "password"},
			},
		}}

		runAutobrr(t, cfg, true, autobrrHTTPAuthSecret())

		body := app.posted("/api/downloaders")
		require.NotNil(t, body)
		assert.Equal(t, want, body["settings"], "auth type %q", authType)
	}
}

func TestAutobrrDownloadClientRulesApplyUnlessDisabled(t *testing.T) {
	app := &fakeAutobrr{}
	cfg := autobrrConfig(app.serve(t))
	cfg.Spec.DownloadClients = []automationv1alpha1.AutobrrDownloadClient{{
		Name: "qBit", Type: "qBittorrent", Host: "http://qbit:8080",
		Rules: &automationv1alpha1.AutobrrDownloadClientRules{
			MaxActiveDownloads: new(int32(3)), IgnoreSlowTorrents: new(true),
			IgnoreSlowTorrentsCondition: "MaxDownloadsReached", DownloadSpeedThreshold: new(int32(0)),
		},
	}}

	runAutobrr(t, cfg, true)

	body := app.posted("/api/downloaders")
	require.NotNil(t, body)
	assert.Equal(t, map[string]any{"rules": map[string]any{
		"enabled": true, "max_active_downloads": float64(3), "ignore_slow_torrents": true,
		"ignore_slow_torrents_condition": "MAX_DOWNLOADS_REACHED", "download_speed_threshold": float64(0),
	}}, body["settings"])
}

func TestAutobrrDownloadClientNamesTheArrsDownloadClient(t *testing.T) {
	app := &fakeAutobrr{}
	cfg := autobrrConfig(app.serve(t))
	cfg.Spec.DownloadClients = []automationv1alpha1.AutobrrDownloadClient{{
		Name: "Sonarr", Type: "Sonarr", Host: "http://sonarr:8989", ExternalDownloadClient: "qBittorrent",
	}}

	runAutobrr(t, cfg, true)

	body := app.posted("/api/downloaders")
	require.NotNil(t, body)
	assert.Equal(t, map[string]any{"external_download_client": "qBittorrent"}, body["settings"])
}

func TestAutobrrDownloadClientAppRefsUseTheReferencedConnections(t *testing.T) {
	app := &fakeAutobrr{}
	cfg := autobrrConfig(app.serve(t))
	cfg.Spec.DownloadClients = []automationv1alpha1.AutobrrDownloadClient{
		{Name: "qBittorrent", AppRef: &commonv1alpha1.AppRef{Kind: "QBittorrent", Name: "qbittorrent"}},
	}
	secret := autobrrSecret()
	secret.Data["username"], secret.Data["password"] = []byte("admin"), []byte("hunter2")
	creds := func(key string) commonv1alpha1.SecretKeyRef {
		return commonv1alpha1.SecretKeyRef{Name: "autobrr-credentials", Key: key}
	}

	runAutobrr(t, cfg, false, secret, &downloadsv1alpha1.QBittorrent{Name: "qbittorrent", Namespace: "media", Spec: downloadsv1alpha1.QBittorrentSpec{
		Connection: downloadsv1alpha1.QBittorrentConnection{URL: "https://qbittorrent.media.svc:8443/qbt/",
			UsernameSecretRef: creds("username"), PasswordSecretRef: creds("password")},
	}})

	body := app.posted("/api/downloaders")
	require.NotNil(t, body)
	assert.Equal(t, "QBITTORRENT", body["type"])
	assert.Equal(t, "https://qbittorrent.media.svc:8443/qbt", body["host"])
	assert.EqualValues(t, 0, body["port"], "a stored port would be appended to the URL")
	assert.Equal(t, true, body["tls"])
	assert.Equal(t, "admin", body["username"])
	assert.Equal(t, "hunter2", body["password"])
}

func TestAutobrrDownloadClientAppRefTakesAnArrsAPIKey(t *testing.T) {
	app := &fakeAutobrr{}
	cfg := autobrrConfig(app.serve(t))
	cfg.Spec.DownloadClients = []automationv1alpha1.AutobrrDownloadClient{
		{Name: "Radarr", AppRef: &commonv1alpha1.AppRef{Kind: "Radarr", Name: "radarr"}},
	}

	runAutobrr(t, cfg, true, &pvrv1alpha1.Radarr{Name: "radarr", Namespace: "media", Spec: pvrv1alpha1.RadarrSpec{
		Connection: commonv1alpha1.AppConnection{URL: "http://radarr.media.svc:7878",
			APIKeySecretRef: commonv1alpha1.SecretKeyRef{Name: "autobrr-credentials", Key: "api-key"}},
	}})

	body := app.posted("/api/downloaders")
	require.NotNil(t, body)
	assert.Equal(t, "RADARR", body["type"])
	assert.Equal(t, "http://radarr.media.svc:7878", body["host"])
	assert.Equal(t, false, body["tls"])
	assert.Equal(t, map[string]any{"apikey": "test-key"}, body["settings"])
	assert.NotContains(t, body, "username")
}

func TestAutobrrMissingDownloadClientAppRefIsReported(t *testing.T) {
	app := &fakeAutobrr{}
	cfg := autobrrConfig(app.serve(t))
	cfg.Spec.DownloadClients = []automationv1alpha1.AutobrrDownloadClient{
		{Name: "qBittorrent", AppRef: &commonv1alpha1.AppRef{Kind: "QBittorrent", Name: "qbittorrent"}},
	}

	c := runAutobrr(t, cfg, true)

	assert.Nil(t, app.posted("/api/downloaders"))
	var got automationv1alpha1.Autobrr
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "autobrr", Namespace: "media"}, &got))
	synced := meta.FindStatusCondition(got.Status.Conditions, "Synced")
	require.NotNil(t, synced)
	assert.Contains(t, synced.Message, `downloadClient(qBittorrent): appRef: QBittorrent "qbittorrent" not found`)
}

func TestAutobrrNotificationTakesItsEventsAndOptions(t *testing.T) {
	app := &fakeAutobrr{}
	cfg := autobrrConfig(app.serve(t))
	cfg.Spec.Notifications = []automationv1alpha1.AutobrrNotification{{
		Name: "Discord", Type: "Discord", Events: []string{"pushApproved", "ircDisconnected"},
		Options: []commonv1alpha1.ConfigField{
			{Name: "webhook", ValueFrom: commonv1alpha1.ValueSource{SecretKeyRef: commonv1alpha1.SecretKeyRef{Name: "hooks", Key: "discord"}}},
			{Name: "title", Value: &commonv1alpha1.FieldValue{Raw: []byte(`"autobrr"`)}},
		},
	}}
	hooks := &corev1.Secret{Name: "hooks", Namespace: "media", Data: map[string][]byte{"discord": []byte("https://discord.example/hook")}}
	runAutobrr(t, cfg, true, hooks)

	body := app.posted("/api/notification")
	require.NotNil(t, body, "the notification was never created")
	assert.Equal(t, "DISCORD", body["type"])
	assert.Equal(t, []any{"PUSH_APPROVED", "IRC_DISCONNECTED"}, body["events"])
	assert.Equal(t, "https://discord.example/hook", body["webhook"])
	assert.Equal(t, "autobrr", body["title"])
	assert.Equal(t, true, body["enabled"], "a new notification is enabled")
}

func TestAutobrrNotificationRefusesAnUnknownOption(t *testing.T) {
	app := &fakeAutobrr{}
	cfg := autobrrConfig(app.serve(t))
	cfg.Spec.Notifications = []automationv1alpha1.AutobrrNotification{{
		Name: "Discord", Type: "Discord",
		Options: []commonv1alpha1.ConfigField{{Name: "webhookUrl", Value: &commonv1alpha1.FieldValue{Raw: []byte(`"x"`)}}},
	}}
	runAutobrr(t, cfg, true)

	assert.False(t, app.saw(http.MethodPost, "/api/notification"), "wrote a notification with a setting Autobrr would drop")
}

func TestAutobrrEventNames(t *testing.T) {
	for name, want := range map[string]string{
		"pushApproved": "PUSH_APPROVED", "ircDisconnected": "IRC_DISCONNECTED",
		"appUpdateAvailable": "APP_UPDATE_AVAILABLE", "releaseNew": "RELEASE_NEW",
	} {
		assert.Equal(t, want, autobrrclient.NotificationEvent(name))
		assert.Equal(t, name, autobrrclient.NotificationEventName(want))
	}
}

func TestAutobrrListNamesItsDownloadClientAndFiltersByID(t *testing.T) {
	app := &fakeAutobrr{existing: map[string][]map[string]any{
		"downloaders": {{"id": float64(4), "name": "Sonarr", "type": "SONARR"}},
		"filters":     {{"id": float64(7), "name": "TV"}},
	}}
	cfg := autobrrConfig(app.serve(t))
	cfg.Spec.Lists = []automationv1alpha1.AutobrrTitleList{{
		Name: "Shows", Type: "Sonarr", DownloadClientName: "Sonarr", Filters: []string{"TV"},
		IncludeAlternateTitles: new(true), TagsExcluded: []string{"anime"},
	}}
	runAutobrr(t, cfg, true)

	body := app.posted("/api/lists")
	require.NotNil(t, body, "the list was never created")
	assert.Equal(t, "SONARR", body["type"])
	assert.Equal(t, float64(4), body["client_id"])
	assert.Equal(t, []any{map[string]any{"id": float64(7), "name": "TV"}}, body["filters"])
	assert.Equal(t, true, body["include_alternate_titles"])
	assert.Equal(t, []any{"anime"}, body["tags_excluded"])
	assert.Equal(t, []any{}, body["tags_included"], "Autobrr refuses a new list without its lists")
	assert.Equal(t, true, body["enabled"], "a new list is enabled")
}

func TestAutobrrListRefusesAFilterAutobrrDoesNotHave(t *testing.T) {
	app := &fakeAutobrr{}
	cfg := autobrrConfig(app.serve(t))
	cfg.Spec.Lists = []automationv1alpha1.AutobrrTitleList{{Name: "Shows", Type: "Plaintext", URL: "http://list", Filters: []string{"TV"}}}
	c := runAutobrr(t, cfg, true)

	assert.False(t, app.saw(http.MethodPost, "/api/lists"))
	assert.Equal(t, "False", autobrrSynced(t, c))
}

func TestAutobrrProxyTakesItsTypeAndCredentials(t *testing.T) {
	app := &fakeAutobrr{}
	cfg := autobrrConfig(app.serve(t))
	cfg.Spec.Proxies = []automationv1alpha1.AutobrrProxy{{
		Name: "VPN", Type: "Socks5", Addr: "socks5://vpn:1080", Timeout: 30,
		UsernameSecretRef: &commonv1alpha1.SecretKeyRef{Name: "proxy", Key: "user"},
		PasswordSecretRef: &commonv1alpha1.SecretKeyRef{Name: "proxy", Key: "pass"},
	}}
	proxy := &corev1.Secret{Name: "proxy", Namespace: "media", Data: map[string][]byte{"user": []byte("u"), "pass": []byte("p")}}
	runAutobrr(t, cfg, true, proxy)

	body := app.posted("/api/proxy")
	require.NotNil(t, body, "the proxy was never created")
	assert.Equal(t, "SOCKS5", body["type"])
	assert.Equal(t, "u", body["user"])
	assert.Equal(t, "p", body["pass"])
	assert.Equal(t, float64(30), body["timeout"])
	assert.Equal(t, true, body["enabled"], "a new proxy is enabled")
}

func TestAutobrrIRCNetworkConnectsThroughItsProxyByID(t *testing.T) {
	app := &fakeAutobrr{existing: map[string][]map[string]any{"proxy": {{"id": float64(3), "name": "VPN"}}}, stores: true}
	cfg := autobrrConfig(app.serve(t))
	cfg.Spec.IRCNetworks = []automationv1alpha1.AutobrrIRCNetwork{{Name: "Net", Server: "irc.example", Port: 6697, Nick: "bot", ProxyName: "VPN"}}
	runAutobrr(t, cfg, true)

	created := app.posted("/api/irc")
	require.NotNil(t, created, "the network was never created")
	assert.Equal(t, false, created["enabled"], "a network was created on before it had its proxy")
	app.mu.Lock()
	updated := app.bodies[http.MethodPut+" /api/irc/network/1"]
	app.mu.Unlock()
	require.NotNil(t, updated, "the new network was never given its proxy")
	assert.Equal(t, true, updated["use_proxy"])
	assert.Equal(t, float64(3), updated["proxy_id"])
	assert.Equal(t, true, updated["enabled"], "a new network is enabled")
}

func TestAutobrrIRCNetworkTakesItsBouncerAndServerPassword(t *testing.T) {
	app := &fakeAutobrr{}
	cfg := autobrrConfig(app.serve(t))
	cfg.Spec.IRCNetworks = []automationv1alpha1.AutobrrIRCNetwork{{
		Name: "Net", Server: "irc.example", Port: 6697, Nick: "bot",
		BouncerAddress: "znc:6697", BotMode: new(true),
		ServerPasswordSecretRef: &commonv1alpha1.SecretKeyRef{Name: "autobrr-credentials", Key: "api-key"},
	}}

	runAutobrr(t, cfg, true)

	body := app.posted("/api/irc")
	require.NotNil(t, body)
	assert.Equal(t, true, body["use_bouncer"])
	assert.Equal(t, "znc:6697", body["bouncer_addr"])
	assert.Equal(t, true, body["bot_mode"])
	assert.Equal(t, "test-key", body["pass"])
}

func TestAutobrrObserveTellsADeclaredProxyFromATypo(t *testing.T) {
	for ref, want := range map[string]string{"VPN": "DriftDetected", "typo": "SyncFailed"} {
		app := &fakeAutobrr{existing: map[string][]map[string]any{"indexer/schema": autobrrDefinitions}}
		cfg := observedAutobrr(app.serve(t))
		cfg.Spec.Proxies = []automationv1alpha1.AutobrrProxy{{Name: "VPN", Type: "HTTP", Addr: "http://vpn:8080"}}
		cfg.Spec.IRCNetworks = []automationv1alpha1.AutobrrIRCNetwork{{Name: "Net", Server: "irc.example", Port: 6697, Nick: "bot", ProxyName: ref}}

		c := runAutobrr(t, cfg, true)

		var got automationv1alpha1.Autobrr
		require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "autobrr", Namespace: "media"}, &got))
		synced := meta.FindStatusCondition(got.Status.Conditions, "Synced")
		require.NotNil(t, synced)
		assert.Equal(t, want, synced.Reason, "a network using proxy %q", ref)
		assert.Empty(t, autobrrWrites(app))
	}
}
