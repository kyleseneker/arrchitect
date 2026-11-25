package common

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	downloadsv1alpha1 "github.com/kyleseneker/arrchitect/api/downloads/v1alpha1"
	indexersv1alpha1 "github.com/kyleseneker/arrchitect/api/indexers/v1alpha1"
	mediaserversv1alpha1 "github.com/kyleseneker/arrchitect/api/mediaservers/v1alpha1"
	pvrv1alpha1 "github.com/kyleseneker/arrchitect/api/pvr/v1alpha1"
	requestsv1alpha1 "github.com/kyleseneker/arrchitect/api/requests/v1alpha1"
	"github.com/kyleseneker/arrchitect/internal/engine"
	"github.com/kyleseneker/arrchitect/internal/reconciler"
)

func appRefClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, scheme.AddToScheme(s))
	require.NoError(t, pvrv1alpha1.AddToScheme(s))
	require.NoError(t, indexersv1alpha1.AddToScheme(s))
	require.NoError(t, downloadsv1alpha1.AddToScheme(s))
	require.NoError(t, mediaserversv1alpha1.AddToScheme(s))
	require.NoError(t, requestsv1alpha1.AddToScheme(s))
	return fake.NewClientBuilder().WithScheme(s).WithObjects(objs...).
		WithIndex(&indexersv1alpha1.Prowlarr{}, appRefIndex, func(o client.Object) []string { return appRefKeys(prowlarrRefs(o)) }).
		Build()
}

func ref(name, key string) *commonv1alpha1.SecretKeyRef {
	return &commonv1alpha1.SecretKeyRef{Name: name, Key: key}
}

func TestSplitURL(t *testing.T) {
	tests := []struct {
		raw  string
		want Address
	}{
		{"http://sonarr.media.svc:8989", Address{Host: "sonarr.media.svc", Port: 8989}},
		{"http://sonarr:8989/", Address{Host: "sonarr", Port: 8989}},
		{"https://media.example.com/sonarr/", Address{Host: "media.example.com", Port: 443, TLS: true, Base: "/sonarr"}},
		{"http://sonarr", Address{Host: "sonarr", Port: 80}},
		{"http://[fd00::1]:8080/base", Address{Host: "fd00::1", Port: 8080, Base: "/base"}},
	}
	for _, tt := range tests {
		got, err := SplitURL(tt.raw)
		require.NoError(t, err, tt.raw)
		assert.Equal(t, tt.want, got, tt.raw)
	}
	for _, raw := range []string{"", "sonarr:8989", "http://", "http://sonarr:0", "http://sonarr:65536", "://bad"} {
		_, err := SplitURL(raw)
		assert.Error(t, err, raw)
	}
}

func TestResolveAppRefTakesEachKindsConnection(t *testing.T) {
	privateCA := &commonv1alpha1.TLSConfig{CASecretRef: *ref("ca", "ca.crt")}
	c := appRefClient(t,
		&pvrv1alpha1.Sonarr{Name: "sonarr", Namespace: "media", Spec: pvrv1alpha1.SonarrSpec{
			Connection: commonv1alpha1.AppConnection{URL: "https://sonarr:8989", APIKeySecretRef: *ref("keys", "sonarr"), TLS: privateCA},
		}},
		&requestsv1alpha1.Seerr{Name: "seerr", Namespace: "media", Spec: requestsv1alpha1.SeerrSpec{
			Connection: requestsv1alpha1.SeerrConnection{URL: "http://seerr:5055", APIKeySecretRef: ref("keys", "seerr")},
		}},
		&mediaserversv1alpha1.Jellyfin{Name: "jellyfin", Namespace: "media", Spec: mediaserversv1alpha1.JellyfinSpec{
			Connection: mediaserversv1alpha1.JellyfinConnection{URL: "http://jellyfin:8096", APIKeySecretRef: ref("keys", "jellyfin")},
			AdminUser:  mediaserversv1alpha1.JellyfinAdminUser{UsernameSecretRef: *ref("admin", "user"), PasswordSecretRef: *ref("admin", "pass")},
		}},
		&mediaserversv1alpha1.Plex{Name: "plex", Namespace: "media", Spec: mediaserversv1alpha1.PlexSpec{
			Connection: mediaserversv1alpha1.PlexConnection{URL: "http://plex:32400", TokenSecretRef: *ref("keys", "plex")},
		}},
		&downloadsv1alpha1.QBittorrent{Name: "qbittorrent", Namespace: "media", Spec: downloadsv1alpha1.QBittorrentSpec{
			Connection: downloadsv1alpha1.QBittorrentConnection{URL: "http://qbittorrent:8080",
				UsernameSecretRef: *ref("qbit", "user"), PasswordSecretRef: *ref("qbit", "pass")},
		}},
	)
	tests := []struct {
		ref  commonv1alpha1.AppRef
		want App
	}{
		{commonv1alpha1.AppRef{Kind: "Sonarr", Name: "sonarr"},
			App{Kind: "Sonarr", URL: "https://sonarr:8989", TLS: privateCA, APIKeySecretRef: ref("keys", "sonarr")}},
		{commonv1alpha1.AppRef{Kind: "Seerr", Name: "seerr"},
			App{Kind: "Seerr", URL: "http://seerr:5055", APIKeySecretRef: ref("keys", "seerr")}},
		{commonv1alpha1.AppRef{Kind: "Jellyfin", Name: "jellyfin"},
			App{Kind: "Jellyfin", URL: "http://jellyfin:8096", APIKeySecretRef: ref("keys", "jellyfin"),
				UsernameSecretRef: ref("admin", "user"), PasswordSecretRef: ref("admin", "pass")}},
		{commonv1alpha1.AppRef{Kind: "Plex", Name: "plex"},
			App{Kind: "Plex", URL: "http://plex:32400", TokenSecretRef: ref("keys", "plex")}},
		{commonv1alpha1.AppRef{Kind: "QBittorrent", Name: "qbittorrent"},
			App{Kind: "QBittorrent", URL: "http://qbittorrent:8080", UsernameSecretRef: ref("qbit", "user"), PasswordSecretRef: ref("qbit", "pass")}},
	}
	for _, tt := range tests {
		got, err := ResolveAppRef(context.Background(), c, nil, "media", tt.ref)
		require.NoError(t, err, tt.ref.Kind)
		assert.Equal(t, tt.want, got, tt.ref.Kind)
	}
}

func TestResolveNotificationsFillsInTheMediaServer(t *testing.T) {
	c := appRefClient(t,
		&mediaserversv1alpha1.Jellyfin{Name: "jellyfin", Namespace: "media", Spec: mediaserversv1alpha1.JellyfinSpec{
			Connection: mediaserversv1alpha1.JellyfinConnection{URL: "http://jellyfin:8096", APIKeySecretRef: ref("keys", "jellyfin")},
		}},
		&mediaserversv1alpha1.Jellyfin{Name: "nokey", Namespace: "media", Spec: mediaserversv1alpha1.JellyfinSpec{
			Connection: mediaserversv1alpha1.JellyfinConnection{URL: "http://nokey:8096"},
		}},
		&mediaserversv1alpha1.Plex{Name: "plex", Namespace: "media", Spec: mediaserversv1alpha1.PlexSpec{
			Connection: mediaserversv1alpha1.PlexConnection{URL: "https://plex.example.com/plex/", TokenSecretRef: *ref("keys", "plex")},
		}},
	)
	updateLibrary := commonv1alpha1.ConfigField{Name: "updateLibrary", Value: &commonv1alpha1.FieldValue{Raw: []byte("true")}}
	in := []commonv1alpha1.Notification{
		{Name: "Discord", Implementation: "Discord"},
		{Name: "Jellyfin", AppRef: commonv1alpha1.AppRef{Kind: "Jellyfin", Name: "jellyfin"},
			Fields: []commonv1alpha1.ConfigField{updateLibrary}},
		{Name: "Plex", AppRef: commonv1alpha1.AppRef{Kind: "Plex", Name: "plex"}},
	}
	out, err := ResolveNotifications(context.Background(), c, nil, "media", in)
	require.NoError(t, err)
	assert.Equal(t, in[0], out[0])
	assert.NotZero(t, in[1].AppRef, "the resource's own notifications are left alone")

	fields := func(n commonv1alpha1.Notification) map[string]string {
		got := map[string]string{}
		for _, f := range n.Fields {
			if f.Value != nil {
				got[f.Name] = string(f.Value.Raw)
			} else {
				got[f.Name] = f.ValueFrom.SecretKeyRef.Name + "/" + f.ValueFrom.SecretKeyRef.Key
			}
		}
		return got
	}
	assert.Zero(t, out[1].AppRef)
	assert.Equal(t, "MediaBrowser", out[1].Implementation)
	assert.Equal(t, map[string]string{"updateLibrary": "true", "host": `"jellyfin"`, "port": "8096", "useSsl": "false",
		"apiKey": "keys/jellyfin"}, fields(out[1]), "no urlBase for a server at the root")
	assert.Equal(t, "PlexServer", out[2].Implementation)
	assert.Equal(t, map[string]string{"host": `"plex.example.com"`, "port": "443", "useSsl": "true", "urlBase": `"/plex"`,
		"authToken": "keys/plex"}, fields(out[2]))

	for _, tt := range []struct {
		n    commonv1alpha1.Notification
		want string
	}{
		{commonv1alpha1.Notification{Name: "Jellyfin", AppRef: commonv1alpha1.AppRef{Kind: "Jellyfin", Name: "jellyfin"},
			Fields: []commonv1alpha1.ConfigField{{Name: "host", Value: &commonv1alpha1.FieldValue{Raw: []byte(`"x"`)}}}},
			`notification "Jellyfin": appRef fills in host, so leave that field out`},
		{commonv1alpha1.Notification{Name: "NoKey", AppRef: commonv1alpha1.AppRef{Kind: "Jellyfin", Name: "nokey"}},
			`notification "NoKey" appRef: Jellyfin "nokey" sets no API key Secret`},
		{commonv1alpha1.Notification{Name: "Gone", AppRef: commonv1alpha1.AppRef{Kind: "Plex", Name: "gone"}},
			`notification "Gone" appRef: Plex "gone" not found`},
	} {
		_, err := ResolveNotifications(context.Background(), c, nil, "media", []commonv1alpha1.Notification{tt.n})
		require.EqualError(t, err, tt.want)
	}
}

func TestResolveAppRefNamesOnlyTheReference(t *testing.T) {
	c := appRefClient(t, &pvrv1alpha1.Sonarr{Name: "sonarr", Namespace: "other", Spec: pvrv1alpha1.SonarrSpec{
		Connection: commonv1alpha1.AppConnection{URL: "http://sonarr.other:8989"},
	}})

	_, err := ResolveAppRef(context.Background(), c, nil, "media", commonv1alpha1.AppRef{Kind: "Sonarr", Name: "sonarr"})
	require.EqualError(t, err, `Sonarr "sonarr" not found`, "a resource in another namespace was used")

	_, err = ResolveAppRef(context.Background(), c, nil, "media", commonv1alpha1.AppRef{Kind: "Tdarr", Name: "tdarr"})
	require.EqualError(t, err, `Tdarr "tdarr": unsupported kind`)
}

func TestAppRefFailureSaysWhatToFix(t *testing.T) {
	c := appRefClient(t,
		&pvrv1alpha1.Sonarr{Name: "nourl", Namespace: "media", Spec: pvrv1alpha1.SonarrSpec{
			Connection: commonv1alpha1.AppConnection{URL: "http://:8989"},
		}},
		&pvrv1alpha1.Sonarr{Name: "sonarr", Namespace: "media", Spec: pvrv1alpha1.SonarrSpec{
			Connection: commonv1alpha1.AppConnection{URL: "http://sonarr:8989"},
		}},
	)
	resolve := func(apps Apps, kind, name string) error {
		_, _, err := ResolveAppAddress(context.Background(), c, apps, "media", commonv1alpha1.AppRef{Kind: kind, Name: name})
		return fmt.Errorf("download client %q appRef: %w", "client", err)
	}
	_, secretErr := reconciler.ResolveSecretKeyRef(context.Background(), c, "media", *ref("missing", "key"))
	_, notMediaServer := ResolveNotifications(context.Background(), c, nil, "media", []commonv1alpha1.Notification{
		{Name: "Sonarr", AppRef: commonv1alpha1.AppRef{Kind: "Sonarr", Name: "sonarr"}},
	})

	tests := []struct {
		name   string
		err    error
		reason string
		after  time.Duration
	}{
		{"missing resource", resolve(nil, "Sonarr", "gone"), engine.ReasonReferenceNotFound, 30 * time.Second},
		{"unsupported kind", resolve(nil, "Tdarr", "tdarr"), engine.ReasonInvalidConfig, 0},
		{"app the operator doesn't run", resolve(Apps{"radarr": true}, "Sonarr", "sonarr"), engine.ReasonInvalidConfig, 0},
		{"no usable URL", resolve(nil, "Sonarr", "nourl"), engine.ReasonInvalidConfig, 0},
		{"kind the connection doesn't take", notMediaServer, engine.ReasonInvalidConfig, 0},
		{"missing Secret", fmt.Errorf("download client %q password: %w", "client", secretErr), engine.ReasonSecretNotFound, 30 * time.Second},
		{"other failure", errors.New("reading Sonarr \"sonarr\": timeout"), engine.ReasonSyncFailed, 30 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Error(t, tt.err)
			reason, after := AppRefFailure(tt.err)
			assert.Equal(t, tt.reason, reason, tt.err.Error())
			assert.Equal(t, tt.after, after)
		})
	}
}

type countingReader struct {
	client.Reader
	gets int
}

func (r *countingReader) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	r.gets++
	return r.Reader.Get(ctx, key, obj, opts...)
}

func TestResolveAppRefReadsOnlyAppsTheOperatorRuns(t *testing.T) {
	c := &countingReader{Reader: appRefClient(t,
		&pvrv1alpha1.Sonarr{Name: "sonarr", Namespace: "media", Spec: pvrv1alpha1.SonarrSpec{
			Connection: commonv1alpha1.AppConnection{URL: "http://sonarr:8989"},
		}},
		&pvrv1alpha1.Radarr{Name: "radarr", Namespace: "media", Spec: pvrv1alpha1.RadarrSpec{
			Connection: commonv1alpha1.AppConnection{URL: "http://radarr:7878"},
		}},
	)}
	apps := Apps{"sonarr": true, "radarr": false}

	_, err := ResolveAppRef(context.Background(), c, apps, "media", commonv1alpha1.AppRef{Kind: "Radarr", Name: "radarr"})
	require.EqualError(t, err, `Radarr "radarr": the operator doesn't run the radarr controller`)
	assert.Zero(t, c.gets, "read a kind the operator's RBAC may not cover")

	got, err := ResolveAppRef(context.Background(), c, apps, "media", commonv1alpha1.AppRef{Kind: "Sonarr", Name: "sonarr"})
	require.NoError(t, err)
	assert.Equal(t, "http://sonarr:8989", got.URL)
}

func TestReferrersOfMatchesKindNameAndNamespace(t *testing.T) {
	withApp := func(name, namespace string, refs ...commonv1alpha1.AppRef) *indexersv1alpha1.Prowlarr {
		p := &indexersv1alpha1.Prowlarr{Name: name, Namespace: namespace}
		for _, r := range refs {
			p.Spec.Applications = append(p.Spec.Applications, indexersv1alpha1.ProwlarrApplication{Name: r.Name, AppRef: &r})
		}
		return p
	}
	sonarr := commonv1alpha1.AppRef{Kind: "Sonarr", Name: "main"}
	c := appRefClient(t,
		withApp("refers", "media", commonv1alpha1.AppRef{Kind: "Radarr", Name: "movies"}, sonarr),
		withApp("same-name-other-kind", "media", commonv1alpha1.AppRef{Kind: "Radarr", Name: "main"}),
		withApp("other-namespace", "other", sonarr),
		withApp("explicit", "media"),
	)
	referrers := referrersOf(c, &indexersv1alpha1.ProwlarrList{}, "Sonarr")

	reqs := referrers(context.Background(), &pvrv1alpha1.Sonarr{Name: "main", Namespace: "media"})
	assert.Equal(t, []reconcile.Request{{Name: "refers", Namespace: "media"}}, reqs)
}

func TestSecretReferrersFollowReferences(t *testing.T) {
	c := appRefClient(t,
		&pvrv1alpha1.Sonarr{Name: "sonarr", Namespace: "media", Spec: pvrv1alpha1.SonarrSpec{
			Connection: commonv1alpha1.AppConnection{URL: "http://sonarr:8989", APIKeySecretRef: *ref("sonarr-key", "api-key")},
		}},
		&indexersv1alpha1.Prowlarr{Name: "through-ref", Namespace: "media", Spec: indexersv1alpha1.ProwlarrSpec{
			Applications: []indexersv1alpha1.ProwlarrApplication{{Name: "Sonarr", AppRef: &commonv1alpha1.AppRef{Kind: "Sonarr", Name: "sonarr"}}},
		}},
		&indexersv1alpha1.Prowlarr{Name: "direct", Namespace: "media", Spec: indexersv1alpha1.ProwlarrSpec{
			Applications: []indexersv1alpha1.ProwlarrApplication{{Name: "Sonarr", APIKeySecretRef: ref("sonarr-key", "api-key")}},
		}},
		&indexersv1alpha1.Prowlarr{Name: "dangling", Namespace: "media", Spec: indexersv1alpha1.ProwlarrSpec{
			Applications: []indexersv1alpha1.ProwlarrApplication{{Name: "Radarr", AppRef: &commonv1alpha1.AppRef{Kind: "Radarr", Name: "radarr"}}},
		}},
		&indexersv1alpha1.Prowlarr{Name: "unrelated", Namespace: "media"},
	)
	referrers := secretReferrers(c, nil, &indexersv1alpha1.ProwlarrList{}, prowlarrRefs)

	reqs := referrers(context.Background(), &metav1.PartialObjectMetadata{Name: "sonarr-key", Namespace: "media"})
	assert.ElementsMatch(t, []reconcile.Request{
		{Name: "through-ref", Namespace: "media"},
		{Name: "direct", Namespace: "media"},
	}, reqs)
}

// gets records the type of every object read through it.
type gets struct {
	client.Client
	types []string
}

func (g *gets) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	g.types = append(g.types, fmt.Sprintf("%T", obj))
	return g.Client.Get(ctx, key, obj, opts...)
}

func TestSecretReferrersSkipAppsTheOperatorDoesntRun(t *testing.T) {
	c := &gets{Client: appRefClient(t,
		&pvrv1alpha1.Sonarr{Name: "sonarr", Namespace: "media", Spec: pvrv1alpha1.SonarrSpec{
			Connection: commonv1alpha1.AppConnection{URL: "http://sonarr:8989", APIKeySecretRef: *ref("sonarr-key", "api-key")},
		}},
		&indexersv1alpha1.Prowlarr{Name: "through-ref", Namespace: "media", Spec: indexersv1alpha1.ProwlarrSpec{
			Applications: []indexersv1alpha1.ProwlarrApplication{{Name: "Sonarr", AppRef: &commonv1alpha1.AppRef{Kind: "Sonarr", Name: "sonarr"}}},
		}},
	)}
	referrers := secretReferrers(c, Apps{"prowlarr": true}, &indexersv1alpha1.ProwlarrList{}, prowlarrRefs)

	reqs := referrers(context.Background(), &metav1.PartialObjectMetadata{Name: "sonarr-key", Namespace: "media"})
	assert.Empty(t, reqs)
	assert.NotContains(t, c.types, "*v1alpha1.Sonarr",
		"the cache can't read a kind the operator doesn't run, and the read would wait forever")
}

func prowlarrRefs(o client.Object) []commonv1alpha1.AppRef {
	var refs []commonv1alpha1.AppRef
	for _, app := range o.(*indexersv1alpha1.Prowlarr).Spec.Applications {
		if app.AppRef != nil {
			refs = append(refs, *app.AppRef)
		}
	}
	return refs
}
