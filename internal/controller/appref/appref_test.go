package appref

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	downloadsv1alpha1 "github.com/kyleseneker/arrchitect/api/downloads/v1alpha1"
	indexersv1alpha1 "github.com/kyleseneker/arrchitect/api/indexers/v1alpha1"
	mediaserversv1alpha1 "github.com/kyleseneker/arrchitect/api/mediaservers/v1alpha1"
	pvrv1alpha1 "github.com/kyleseneker/arrchitect/api/pvr/v1alpha1"
	requestsv1alpha1 "github.com/kyleseneker/arrchitect/api/requests/v1alpha1"
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
	return fake.NewClientBuilder().WithScheme(s).WithObjects(objs...).Build()
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
		got, err := splitURL(tt.raw)
		require.NoError(t, err, tt.raw)
		assert.Equal(t, tt.want, got, tt.raw)
	}
	for _, raw := range []string{"", "sonarr:8989", "http://", "http://sonarr:0", "http://sonarr:65536", "://bad"} {
		_, err := splitURL(raw)
		assert.Error(t, err, raw)
	}
}

func TestResolveTakesEachKindsConnection(t *testing.T) {
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
		got, err := Resolve(context.Background(), c, nil, "media", tt.ref)
		require.NoError(t, err, tt.ref.Kind)
		assert.Equal(t, tt.want, got, tt.ref.Kind)
	}
}

func TestResolveNamesOnlyTheReference(t *testing.T) {
	c := appRefClient(t, &pvrv1alpha1.Sonarr{Name: "sonarr", Namespace: "other", Spec: pvrv1alpha1.SonarrSpec{
		Connection: commonv1alpha1.AppConnection{URL: "http://sonarr.other:8989"},
	}})

	_, err := Resolve(context.Background(), c, nil, "media", commonv1alpha1.AppRef{Kind: "Sonarr", Name: "sonarr"})
	require.EqualError(t, err, `Sonarr "sonarr" not found`, "a resource in another namespace was used")

	_, err = Resolve(context.Background(), c, nil, "media", commonv1alpha1.AppRef{Kind: "Tdarr", Name: "tdarr"})
	require.EqualError(t, err, `Tdarr "tdarr": unsupported kind`)
}

type countingReader struct {
	client.Reader
	gets int
}

func (r *countingReader) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	r.gets++
	return r.Reader.Get(ctx, key, obj, opts...)
}

func TestResolveReadsOnlyAppsTheOperatorRuns(t *testing.T) {
	c := &countingReader{Reader: appRefClient(t,
		&pvrv1alpha1.Sonarr{Name: "sonarr", Namespace: "media", Spec: pvrv1alpha1.SonarrSpec{
			Connection: commonv1alpha1.AppConnection{URL: "http://sonarr:8989"},
		}},
		&pvrv1alpha1.Radarr{Name: "radarr", Namespace: "media", Spec: pvrv1alpha1.RadarrSpec{
			Connection: commonv1alpha1.AppConnection{URL: "http://radarr:7878"},
		}},
	)}
	apps := Apps{"sonarr": true, "radarr": false}

	_, err := Resolve(context.Background(), c, apps, "media", commonv1alpha1.AppRef{Kind: "Radarr", Name: "radarr"})
	require.EqualError(t, err, `Radarr "radarr": the operator doesn't run the radarr controller`)
	assert.Zero(t, c.gets, "read a kind the operator's RBAC may not cover")

	got, err := Resolve(context.Background(), c, apps, "media", commonv1alpha1.AppRef{Kind: "Sonarr", Name: "sonarr"})
	require.NoError(t, err)
	assert.Equal(t, "http://sonarr:8989", got.URL)
}
