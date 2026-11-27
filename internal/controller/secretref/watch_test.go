package secretref

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	downloadsv1alpha1 "github.com/kyleseneker/arrchitect/api/downloads/v1alpha1"
	indexersv1alpha1 "github.com/kyleseneker/arrchitect/api/indexers/v1alpha1"
	pvrv1alpha1 "github.com/kyleseneker/arrchitect/api/pvr/v1alpha1"
	subtitlesv1alpha1 "github.com/kyleseneker/arrchitect/api/subtitles/v1alpha1"
)

func TestRefsFindsEveryReference(t *testing.T) {
	ref := commonv1alpha1.SecretKeyRef{Name: "target", Key: "k"}
	for name, config := range map[string]any{
		"an optional TLS CA": &pvrv1alpha1.Sonarr{Spec: pvrv1alpha1.SonarrSpec{
			Connection: commonv1alpha1.AppConnection{TLS: &commonv1alpha1.TLSConfig{CASecretRef: ref}},
		}},
		"a download client password": pvrv1alpha1.SonarrDownloadClientSpec{
			DownloadClient: commonv1alpha1.DownloadClient{Name: "q", PasswordSecretRef: ref},
		},
		"a field read from a Secret": indexersv1alpha1.ProwlarrSpec{
			Indexers: []indexersv1alpha1.ProwlarrIndexer{{Name: "x", Fields: []commonv1alpha1.ConfigField{{Name: "apiKey", ValueFrom: commonv1alpha1.ValueSource{SecretKeyRef: ref}}}}},
		},
		"a Usenet server password": downloadsv1alpha1.SabnzbdSpec{
			Servers: []downloadsv1alpha1.SabnzbdServer{{Name: "s", PasswordSecretRef: &ref}},
		},
		"a map value": map[string]commonv1alpha1.SecretKeyRef{"x": ref},
		"a Bazarr provider setting": subtitlesv1alpha1.BazarrSpec{
			Providers: []subtitlesv1alpha1.BazarrProvider{{Name: "opensubtitlescom", Settings: []commonv1alpha1.ConfigField{
				{Name: "password", ValueFrom: commonv1alpha1.ValueSource{SecretKeyRef: ref}},
			}}},
		},
	} {
		assert.Equal(t, []commonv1alpha1.SecretKeyRef{ref}, Refs(config), name)
	}
}

func TestUsersQueuesOnlyResourcesThatUseTheSecret(t *testing.T) {
	s := runtime.NewScheme()
	require.NoError(t, scheme.AddToScheme(s))
	require.NoError(t, pvrv1alpha1.AddToScheme(s))
	withCA := func(name, namespace string) *pvrv1alpha1.Sonarr {
		return &pvrv1alpha1.Sonarr{Name: name, Namespace: namespace, Spec: pvrv1alpha1.SonarrSpec{
			Connection: commonv1alpha1.AppConnection{TLS: &commonv1alpha1.TLSConfig{CASecretRef: commonv1alpha1.SecretKeyRef{Name: "ca", Key: "ca.crt"}}},
		}}
	}
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(
		withCA("uses", "media"), withCA("elsewhere", "other"),
		&pvrv1alpha1.Sonarr{Name: "unrelated", Namespace: "media"},
	).WithIndex(&pvrv1alpha1.Sonarr{}, IndexField, Names).Build()

	reqs := users(c, &pvrv1alpha1.SonarrList{})(context.Background(), &metav1.PartialObjectMetadata{Name: "ca", Namespace: "media"})
	assert.Equal(t, []reconcile.Request{{Name: "uses", Namespace: "media"}}, reqs, "Secrets are watched by metadata only")
}

func TestNamesListsEachSecretOnce(t *testing.T) {
	obj := &pvrv1alpha1.Sonarr{Spec: pvrv1alpha1.SonarrSpec{Connection: commonv1alpha1.AppConnection{
		APIKeySecretRef: commonv1alpha1.SecretKeyRef{Name: "sonarr", Key: "api-key"},
		TLS:             &commonv1alpha1.TLSConfig{CASecretRef: commonv1alpha1.SecretKeyRef{Name: "sonarr", Key: "ca.crt"}},
	}}}
	assert.Equal(t, []string{"sonarr"}, Names(obj))
	assert.Empty(t, Names(&pvrv1alpha1.Sonarr{}))
}
