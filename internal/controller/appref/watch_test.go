package appref

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	indexersv1alpha1 "github.com/kyleseneker/arrchitect/api/indexers/v1alpha1"
	pvrv1alpha1 "github.com/kyleseneker/arrchitect/api/pvr/v1alpha1"
	"github.com/kyleseneker/arrchitect/internal/controller/secretref"
)

// watchClient serves objs, with Prowlarrs indexed by the app resources they
// refer to and Sonarrs by the Secrets they name.
func watchClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, pvrv1alpha1.AddToScheme(s))
	require.NoError(t, indexersv1alpha1.AddToScheme(s))
	return fake.NewClientBuilder().WithScheme(s).WithObjects(objs...).
		WithIndex(&indexersv1alpha1.Prowlarr{}, index, func(o client.Object) []string { return keys(prowlarrRefs(o)) }).
		WithIndex(&pvrv1alpha1.Sonarr{}, secretref.IndexField, secretref.Names).
		Build()
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
	c := watchClient(t,
		withApp("refers", "media", commonv1alpha1.AppRef{Kind: "Radarr", Name: "movies"}, sonarr),
		withApp("same-name-other-kind", "media", commonv1alpha1.AppRef{Kind: "Radarr", Name: "main"}),
		withApp("other-namespace", "other", sonarr),
		withApp("explicit", "media"),
	)
	referrers := referrersOf(c, &indexersv1alpha1.ProwlarrList{}, "Sonarr")

	reqs := referrers(context.Background(), &pvrv1alpha1.Sonarr{Name: "main", Namespace: "media"})
	assert.Equal(t, []reconcile.Request{{Name: "refers", Namespace: "media"}}, reqs)
}

// A Secret an app resource uses requeues the resources that refer to it,
// found through both indexes. Resources that name the Secret themselves are
// requeued by secretref.Watch.
func TestSecretUsersFollowReferences(t *testing.T) {
	sonarr := func(name, namespace, secret string) *pvrv1alpha1.Sonarr {
		return &pvrv1alpha1.Sonarr{Name: name, Namespace: namespace, Spec: pvrv1alpha1.SonarrSpec{
			Connection: commonv1alpha1.AppConnection{URL: "http://sonarr:8989", APIKeySecretRef: commonv1alpha1.SecretKeyRef{Name: secret, Key: "api-key"}},
		}}
	}
	prowlarr := func(name, namespace string, app indexersv1alpha1.ProwlarrApplication) *indexersv1alpha1.Prowlarr {
		return &indexersv1alpha1.Prowlarr{Name: name, Namespace: namespace, Spec: indexersv1alpha1.ProwlarrSpec{
			Applications: []indexersv1alpha1.ProwlarrApplication{app},
		}}
	}
	refTo := func(name string) indexersv1alpha1.ProwlarrApplication {
		return indexersv1alpha1.ProwlarrApplication{Name: "Sonarr", AppRef: &commonv1alpha1.AppRef{Kind: "Sonarr", Name: name}}
	}
	c := watchClient(t,
		sonarr("sonarr", "media", "sonarr-key"),
		sonarr("other-key", "media", "another-key"),
		sonarr("sonarr", "other", "sonarr-key"),
		prowlarr("through-ref", "media", refTo("sonarr")),
		prowlarr("refers-elsewhere", "media", refTo("other-key")),
		prowlarr("other-namespace", "other", refTo("sonarr")),
		prowlarr("direct", "media", indexersv1alpha1.ProwlarrApplication{Name: "Sonarr", APIKeySecretRef: &commonv1alpha1.SecretKeyRef{Name: "sonarr-key", Key: "api-key"}}),
		prowlarr("dangling", "media", refTo("gone")),
	)
	users := secretUsers(c, &indexersv1alpha1.ProwlarrList{}, []string{"Sonarr"})

	reqs := users(context.Background(), &metav1.PartialObjectMetadata{Name: "sonarr-key", Namespace: "media"})
	assert.Equal(t, []reconcile.Request{{Name: "through-ref", Namespace: "media"}}, reqs)
}

// The cache can read only kinds the operator runs, the API server serves and
// the scheme knows, and a read of any other would wait forever.
func TestWatchableKeepsOnlyKindsTheCacheCanRead(t *testing.T) {
	s := runtime.NewScheme()
	require.NoError(t, pvrv1alpha1.AddToScheme(s))
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{pvrv1alpha1.SchemeGroupVersion})
	for _, kind := range []string{"Sonarr", "Radarr", "Jellyfin"} {
		mapper.Add(pvrv1alpha1.SchemeGroupVersion.WithKind(kind), meta.RESTScopeNamespace)
	}
	kinds := []string{"Sonarr", "Radarr", "Lidarr", "Jellyfin"}

	assert.Equal(t, []string{"Sonarr", "Radarr"}, watchable(nil, s, mapper, kinds), "Lidarr isn't served and Jellyfin isn't in the scheme")
	assert.Equal(t, []string{"Sonarr"}, watchable(Apps{"sonarr": true, "prowlarr": true}, s, mapper, kinds), "the operator doesn't run Radarr")
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
