package pvr

import (
	"context"
	"testing"
	"time"

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
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	downloadsv1alpha1 "github.com/kyleseneker/arrchitect/api/downloads/v1alpha1"
	pvrv1alpha1 "github.com/kyleseneker/arrchitect/api/pvr/v1alpha1"
	"github.com/kyleseneker/arrchitect/internal/controller/appref"
	"github.com/kyleseneker/arrchitect/internal/controller/reconciler"
)

func objectClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	require.NoError(t, pvrv1alpha1.AddToScheme(scheme))
	require.NoError(t, downloadsv1alpha1.AddToScheme(scheme))
	return fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(
			&pvrv1alpha1.Sonarr{}, &pvrv1alpha1.SonarrIndexer{}, &pvrv1alpha1.SonarrDownloadClient{},
			&pvrv1alpha1.Radarr{}, &pvrv1alpha1.RadarrIndexer{}, &pvrv1alpha1.RadarrDownloadClient{},
			&pvrv1alpha1.Lidarr{}, &pvrv1alpha1.LidarrIndexer{}, &pvrv1alpha1.LidarrDownloadClient{},
			&pvrv1alpha1.Readarr{}, &pvrv1alpha1.ReadarrIndexer{}, &pvrv1alpha1.ReadarrDownloadClient{},
		).
		WithIndex(&pvrv1alpha1.SonarrIndexer{}, instanceRefIndex, func(o client.Object) []string {
			return []string{o.(*pvrv1alpha1.SonarrIndexer).Spec.SonarrRef.Name}
		}).
		WithIndex(&pvrv1alpha1.SonarrDownloadClient{}, instanceRefIndex, func(o client.Object) []string {
			return []string{o.(*pvrv1alpha1.SonarrDownloadClient).Spec.SonarrRef.Name}
		}).
		WithObjects(objs...).
		Build()
}

func sonarrInstanceAt(url string, rec *commonv1alpha1.ReconcileConfig) *pvrv1alpha1.Sonarr {
	return &pvrv1alpha1.Sonarr{
		Name: "tv", Namespace: "media",
		Spec: pvrv1alpha1.SonarrSpec{
			Connection: commonv1alpha1.AppConnection{
				URL:             url,
				APIKeySecretRef: commonv1alpha1.SecretKeyRef{Name: "app-credentials", Key: "api-key"},
			},
			Reconcile: rec,
		},
	}
}

func sonarrDownloadClient() *pvrv1alpha1.SonarrDownloadClient {
	return &pvrv1alpha1.SonarrDownloadClient{
		Name: "tv-qbittorrent", Namespace: "media",
		Spec: pvrv1alpha1.SonarrDownloadClientSpec{
			SonarrRef:      commonv1alpha1.LocalObjectReference{Name: "tv"},
			DeletionPolicy: reconciler.DeletionPolicyDelete,
			DownloadClient: commonv1alpha1.DownloadClient{
				Name: "qBittorrent", Protocol: "torrent", Implementation: "QBittorrent",
				Host: "qbittorrent", Port: 8080, Category: "tv",
			},
		},
	}
}

// reconcileDownloadClient reconciles the download client once and returns it
// as stored afterwards, or nil once it is gone.
func reconcileDownloadClient(t *testing.T, objs ...client.Object) *pvrv1alpha1.SonarrDownloadClient {
	t.Helper()
	c := objectClient(t, objs...)
	r := NewObjectReconciler(SonarrDownloadClients, c, events.NewFakeRecorder(50), nil)
	key := types.NamespacedName{Name: "tv-qbittorrent", Namespace: "media"}
	_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key})
	require.NoError(t, err)
	var got pvrv1alpha1.SonarrDownloadClient
	err = c.Get(context.Background(), key, &got)
	if apierrors.IsNotFound(err) {
		return nil
	}
	require.NoError(t, err)
	return &got
}

func synced(t *testing.T, conditions []metav1.Condition) metav1.Condition {
	t.Helper()
	cond := apimeta.FindStatusCondition(conditions, reconciler.ConditionSynced)
	require.NotNil(t, cond)
	return *cond
}

func TestObjectCreatesItsItemAndOwnsIt(t *testing.T) {
	fa, url := newFakeArr(t, "v3", nil)
	got := reconcileDownloadClient(t, apiKeySecret(), sonarrInstanceAt(url, nil), sonarrDownloadClient())

	assert.True(t, fa.saw("POST", fa.path("downloadclient")))
	assert.Equal(t, []string{"qBittorrent"}, got.Status.GetManagedResources()["downloadClients"])
	assert.Equal(t, metav1.ConditionTrue, synced(t, got.Status.Conditions).Status)
	assert.Contains(t, got.Finalizers, reconciler.Finalizer)
}

func TestObjectAdoptsAnExistingItemWithoutOwningIt(t *testing.T) {
	fa, url := newFakeArr(t, "v3", map[string][]map[string]any{
		"downloadclient": {{"id": float64(7), "name": "qBittorrent"}},
	})
	got := reconcileDownloadClient(t, apiKeySecret(), sonarrInstanceAt(url, nil), sonarrDownloadClient())

	assert.False(t, fa.saw("POST", fa.path("downloadclient")))
	assert.True(t, fa.sawPrefix("PUT", fa.path("downloadclient/7")))
	assert.Equal(t, int32(7), got.Status.ID)
	assert.Empty(t, got.Status.GetManagedResources()["downloadClients"], "an adopted item must never be deleted with the object")
}

func TestObjectWaitsForItsInstance(t *testing.T) {
	got := reconcileDownloadClient(t, apiKeySecret(), sonarrDownloadClient())
	cond := synced(t, got.Status.Conditions)
	assert.Equal(t, metav1.ConditionFalse, cond.Status)
	assert.Equal(t, reconciler.ReasonReferenceNotFound, cond.Reason)
	assert.Equal(t, `Sonarr "tv" not found`, cond.Message)
}

func TestObjectWithAMissingSecretWritesNothing(t *testing.T) {
	fa, url := newFakeArr(t, "v3", nil)
	dc := sonarrDownloadClient()
	dc.Spec.PasswordSecretRef = commonv1alpha1.SecretKeyRef{Name: "missing", Key: "password"}
	got := reconcileDownloadClient(t, apiKeySecret(), sonarrInstanceAt(url, nil), dc)

	assert.False(t, fa.sawPrefix("POST", "/api/"))
	assert.Equal(t, reconciler.ReasonSecretNotFound, synced(t, got.Status.Conditions).Reason)
}

func TestObjectFollowsItsInstancesObservePolicy(t *testing.T) {
	fa, url := newFakeArr(t, "v3", nil)
	observe := "observe"
	got := reconcileDownloadClient(t, apiKeySecret(),
		sonarrInstanceAt(url, &commonv1alpha1.ReconcileConfig{DriftPolicy: observe}), sonarrDownloadClient())

	assert.False(t, fa.sawPrefix("POST", "/api/"))
	assert.Equal(t, "DriftDetected", synced(t, got.Status.Conditions).Reason)
}

func TestObjectDropsItsFinalizerWhileItsInstanceObserves(t *testing.T) {
	_, url := newFakeArr(t, "v3", nil)
	dc := sonarrDownloadClient()
	dc.Spec.DeletionPolicy = reconciler.DeletionPolicyDelete
	dc.Finalizers = []string{reconciler.Finalizer}
	got := reconcileDownloadClient(t, apiKeySecret(),
		sonarrInstanceAt(url, &commonv1alpha1.ReconcileConfig{DriftPolicy: "observe"}), dc)

	assert.NotContains(t, got.Finalizers, reconciler.Finalizer, "observe forbids the cleanup the finalizer waits for")
	assert.Equal(t, "DriftDetected", synced(t, got.Status.Conditions).Reason, "the object is still observed")
}

// The object owns the one item it names, so a rename replaces the item even
// when the instance leaves prune off.
func TestObjectRenameReplacesItsOldItem(t *testing.T) {
	fa, url := newFakeArr(t, "v3", map[string][]map[string]any{
		"downloadclient": {{"id": float64(7), "name": "qBit"}},
	})
	dc := sonarrDownloadClient()
	dc.Status.SetManagedResources(map[string][]string{"downloadClients": {"qBit"}})
	got := reconcileDownloadClient(t, apiKeySecret(), sonarrInstanceAt(url, nil), dc)

	assert.True(t, fa.saw("POST", fa.path("downloadclient")))
	assert.True(t, fa.saw("DELETE", fa.path("downloadclient/7")))
	assert.Equal(t, []string{"qBittorrent"}, got.Status.GetManagedResources()["downloadClients"])
}

func TestObjectDeletion(t *testing.T) {
	for name, tc := range map[string]struct {
		policy   string
		managed  []string
		instance *commonv1alpha1.ReconcileConfig
		deletes  bool
	}{
		"deletes the item it created":             {policy: reconciler.DeletionPolicyDelete, managed: []string{"qBittorrent"}, deletes: true},
		"leaves an item it adopted":               {policy: reconciler.DeletionPolicyDelete},
		"leaves the item under the orphan policy": {policy: "orphan", managed: []string{"qBittorrent"}},
		"leaves the item while its instance observes": {
			policy: reconciler.DeletionPolicyDelete, managed: []string{"qBittorrent"},
			instance: &commonv1alpha1.ReconcileConfig{DriftPolicy: "observe"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			fa, url := newFakeArr(t, "v3", map[string][]map[string]any{
				"downloadclient": {{"id": float64(7), "name": "qBittorrent"}},
			})
			dc := sonarrDownloadClient()
			dc.Spec.DeletionPolicy = tc.policy
			dc.Status.SetManagedResources(map[string][]string{"downloadClients": tc.managed})
			dc.Finalizers = []string{reconciler.Finalizer}
			now := metav1.Now()
			dc.DeletionTimestamp = &now
			got := reconcileDownloadClient(t, apiKeySecret(), sonarrInstanceAt(url, tc.instance), dc)

			assert.Equal(t, tc.deletes, fa.saw("DELETE", fa.path("downloadclient/7")))
			assert.Nil(t, got, "the finalizer was not released")
		})
	}
}

func TestObjectDeletionWithoutItsInstanceReleasesTheFinalizer(t *testing.T) {
	dc := sonarrDownloadClient()
	dc.Status.SetManagedResources(map[string][]string{"downloadClients": {"qBittorrent"}})
	dc.Finalizers = []string{reconciler.Finalizer}
	now := metav1.Now()
	dc.DeletionTimestamp = &now
	got := reconcileDownloadClient(t, apiKeySecret(), dc)
	assert.Nil(t, got)
}

func TestIndexerObjectReadsItsFieldsFromSecrets(t *testing.T) {
	fa, url := newFakeArr(t, "v3", nil)
	secret := &corev1.Secret{
		Name: "indexer", Namespace: "media",
		Data: map[string][]byte{"api-key": []byte("k")},
	}
	idx := &pvrv1alpha1.SonarrIndexer{
		Name: "tv-nzbgeek", Namespace: "media",
		Spec: pvrv1alpha1.SonarrIndexerSpec{
			SonarrRef:      commonv1alpha1.LocalObjectReference{Name: "tv"},
			DeletionPolicy: reconciler.DeletionPolicyDelete,
			Indexer: commonv1alpha1.Indexer{
				Name: "NZBgeek", Protocol: "usenet", Implementation: "Newznab",
				Fields: []commonv1alpha1.ConfigField{{Name: "apiKey", ValueFrom: commonv1alpha1.ValueSource{
					SecretKeyRef: commonv1alpha1.SecretKeyRef{Name: "indexer", Key: "api-key"},
				}}},
			},
		},
	}
	c := objectClient(t, apiKeySecret(), secret, sonarrInstanceAt(url, nil), idx)
	r := NewObjectReconciler(SonarrIndexers, c, events.NewFakeRecorder(50), nil)
	_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(idx)})
	require.NoError(t, err)

	assert.True(t, fa.saw("POST", fa.path("indexer")))
	require.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(idx), idx))
	assert.Equal(t, []string{"NZBgeek"}, idx.Status.GetManagedResources()["indexers"])
}

func reconcileIndexerWithClient(t *testing.T, clientName string) (*fakeArr, *pvrv1alpha1.SonarrIndexer) {
	t.Helper()
	fa, url := newFakeArr(t, "v3", map[string][]map[string]any{
		"downloadclient": {{"id": float64(4), "name": "qBittorrent"}, {"id": float64(7), "name": "SABnzbd"}},
	})
	idx := &pvrv1alpha1.SonarrIndexer{
		Name: "tv-nzbgeek", Namespace: "media",
		Spec: pvrv1alpha1.SonarrIndexerSpec{
			SonarrRef: commonv1alpha1.LocalObjectReference{Name: "tv"},
			Indexer: commonv1alpha1.Indexer{
				Name: "NZBgeek", Protocol: "usenet", Implementation: "Newznab", DownloadClientName: clientName,
			},
		},
	}
	c := objectClient(t, apiKeySecret(), sonarrInstanceAt(url, nil), idx)
	r := NewObjectReconciler(SonarrIndexers, c, events.NewFakeRecorder(50), nil)
	_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(idx)})
	require.NoError(t, err)
	require.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(idx), idx))
	return fa, idx
}

func TestIndexerObjectPinsItsDownloadClientByName(t *testing.T) {
	fa, _ := reconcileIndexerWithClient(t, "SABnzbd")

	sent := fa.sent(fa.path("indexer"))
	require.Len(t, sent, 1)
	assert.Equal(t, float64(7), sent[0]["downloadClientId"])
}

func TestIndexerObjectReportsADownloadClientTheAppLacks(t *testing.T) {
	fa, idx := reconcileIndexerWithClient(t, "NZBGet")

	assert.Empty(t, fa.sent(fa.path("indexer")), "wrote the indexer without its download client")
	synced := apimeta.FindStatusCondition(idx.Status.Conditions, "Synced")
	require.NotNil(t, synced)
	assert.Equal(t, metav1.ConditionFalse, synced.Status)
	assert.Contains(t, synced.Message, "download client")
	assert.Contains(t, synced.Message, "NZBGet")
}

func TestInstanceChangeRequeuesOnlyItsObjects(t *testing.T) {
	named := func(namespace, name, instance string) *pvrv1alpha1.SonarrDownloadClient {
		dc := sonarrDownloadClient()
		dc.Namespace, dc.Name, dc.Spec.SonarrRef.Name = namespace, name, instance
		return dc
	}
	c := objectClient(t,
		named("media", "tv-qbittorrent", "tv"),
		named("media", "anime-qbittorrent", "anime"),
		named("other", "tv-qbittorrent", "tv"),
	)
	reqs := objectsOf(c, &pvrv1alpha1.SonarrDownloadClientList{})(context.Background(), sonarrInstanceAt("", nil))
	assert.Equal(t, []reconcile.Request{{Namespace: "media", Name: "tv-qbittorrent"}}, reqs)
}

func TestObjectLeavesAnItemAnOlderObjectConfigures(t *testing.T) {
	fa, url := newFakeArr(t, "v3", nil)
	older := sonarrDownloadClient()
	older.Name = "tv-qbit"
	older.CreationTimestamp = metav1.NewTime(time.Now().Add(-time.Hour))
	younger := sonarrDownloadClient()
	younger.CreationTimestamp = metav1.Now()
	got := reconcileDownloadClient(t, apiKeySecret(), sonarrInstanceAt(url, nil), older, younger)

	assert.False(t, fa.sawPrefix("POST", "/api/"))
	cond := synced(t, got.Status.Conditions)
	assert.Equal(t, reconciler.ReasonNameConflict, cond.Reason)
	assert.Equal(t, `"tv-qbit" already configures "qBittorrent" in Sonarr "tv"`, cond.Message)
}

func TestInstanceChangedPassesSpecChangesAndSyncing(t *testing.T) {
	at := func(generation int64, synced metav1.ConditionStatus) *pvrv1alpha1.Sonarr {
		s := sonarrInstanceAt("", nil)
		s.Generation = generation
		if synced != "" {
			s.Status.Conditions = []metav1.Condition{{Type: reconciler.ConditionSynced, Status: synced}}
		}
		return s
	}
	p := instanceChanged()
	assert.True(t, p.Update(event.UpdateEvent{ObjectOld: at(1, ""), ObjectNew: at(2, "")}), "a spec change")
	assert.True(t, p.Update(event.UpdateEvent{ObjectOld: at(1, ""), ObjectNew: at(1, metav1.ConditionTrue)}), "a first sync")
	assert.True(t, p.Update(event.UpdateEvent{ObjectOld: at(1, metav1.ConditionFalse), ObjectNew: at(1, metav1.ConditionTrue)}))
	assert.False(t, p.Update(event.UpdateEvent{ObjectOld: at(1, metav1.ConditionTrue), ObjectNew: at(1, metav1.ConditionTrue)}),
		"a routine status write")
}

// The object controllers share one implementation, so each app's case only
// checks that its controllers reach that app, at its API version, through the
// instance kind they name.
func TestEveryPVRAppsObjectsReachItsApp(t *testing.T) {
	conn := func(url string) commonv1alpha1.AppConnection {
		return commonv1alpha1.AppConnection{URL: url, APIKeySecretRef: commonv1alpha1.SecretKeyRef{Name: "app-credentials", Key: "api-key"}}
	}
	ref := commonv1alpha1.LocalObjectReference{Name: "app"}
	dc := commonv1alpha1.DownloadClient{Name: "qBittorrent", Protocol: "torrent", Implementation: "QBittorrent", Host: "qbittorrent", Port: 8080}
	idx := commonv1alpha1.Indexer{Name: "Torznab", Protocol: "torrent", Implementation: "Torznab"}
	meta := func(name string) metav1.ObjectMeta { return metav1.ObjectMeta{Name: name, Namespace: "media"} }
	for _, app := range []struct {
		name, apiVer, kind string
		instance           func(url string) client.Object
		objects            []client.Object
		reconcilers        func(client.Client) []reconcile.Reconciler
	}{
		{
			name: "sonarr", apiVer: "v3", kind: "Sonarr",
			instance: func(url string) client.Object {
				return &pvrv1alpha1.Sonarr{ObjectMeta: meta("app"), Spec: pvrv1alpha1.SonarrSpec{Connection: conn(url)}}
			},
			objects: []client.Object{
				&pvrv1alpha1.SonarrDownloadClient{ObjectMeta: meta("dc"), Spec: pvrv1alpha1.SonarrDownloadClientSpec{SonarrRef: ref, DownloadClient: dc}},
				&pvrv1alpha1.SonarrIndexer{ObjectMeta: meta("idx"), Spec: pvrv1alpha1.SonarrIndexerSpec{SonarrRef: ref, Indexer: idx}},
			},
			reconcilers: func(c client.Client) []reconcile.Reconciler {
				return []reconcile.Reconciler{
					NewObjectReconciler(SonarrDownloadClients, c, events.NewFakeRecorder(50), nil),
					NewObjectReconciler(SonarrIndexers, c, events.NewFakeRecorder(50), nil),
				}
			},
		},
		{
			name: "radarr", apiVer: "v3", kind: "Radarr",
			instance: func(url string) client.Object {
				return &pvrv1alpha1.Radarr{ObjectMeta: meta("app"), Spec: pvrv1alpha1.RadarrSpec{Connection: conn(url)}}
			},
			objects: []client.Object{
				&pvrv1alpha1.RadarrDownloadClient{ObjectMeta: meta("dc"), Spec: pvrv1alpha1.RadarrDownloadClientSpec{RadarrRef: ref, DownloadClient: dc}},
				&pvrv1alpha1.RadarrIndexer{ObjectMeta: meta("idx"), Spec: pvrv1alpha1.RadarrIndexerSpec{RadarrRef: ref, Indexer: idx}},
			},
			reconcilers: func(c client.Client) []reconcile.Reconciler {
				return []reconcile.Reconciler{
					NewObjectReconciler(RadarrDownloadClients, c, events.NewFakeRecorder(50), nil),
					NewObjectReconciler(RadarrIndexers, c, events.NewFakeRecorder(50), nil),
				}
			},
		},
		{
			name: "lidarr", apiVer: "v1", kind: "Lidarr",
			instance: func(url string) client.Object {
				return &pvrv1alpha1.Lidarr{ObjectMeta: meta("app"), Spec: pvrv1alpha1.LidarrSpec{Connection: conn(url)}}
			},
			objects: []client.Object{
				&pvrv1alpha1.LidarrDownloadClient{ObjectMeta: meta("dc"), Spec: pvrv1alpha1.LidarrDownloadClientSpec{LidarrRef: ref, DownloadClient: dc}},
				&pvrv1alpha1.LidarrIndexer{ObjectMeta: meta("idx"), Spec: pvrv1alpha1.LidarrIndexerSpec{LidarrRef: ref, Indexer: idx}},
			},
			reconcilers: func(c client.Client) []reconcile.Reconciler {
				return []reconcile.Reconciler{
					NewObjectReconciler(LidarrDownloadClients, c, events.NewFakeRecorder(50), nil),
					NewObjectReconciler(LidarrIndexers, c, events.NewFakeRecorder(50), nil),
				}
			},
		},
		{
			name: "readarr", apiVer: "v1", kind: "Readarr",
			instance: func(url string) client.Object {
				return &pvrv1alpha1.Readarr{ObjectMeta: meta("app"), Spec: pvrv1alpha1.ReadarrSpec{Connection: conn(url)}}
			},
			objects: []client.Object{
				&pvrv1alpha1.ReadarrDownloadClient{ObjectMeta: meta("dc"), Spec: pvrv1alpha1.ReadarrDownloadClientSpec{ReadarrRef: ref, DownloadClient: dc}},
				&pvrv1alpha1.ReadarrIndexer{ObjectMeta: meta("idx"), Spec: pvrv1alpha1.ReadarrIndexerSpec{ReadarrRef: ref, Indexer: idx}},
			},
			reconcilers: func(c client.Client) []reconcile.Reconciler {
				return []reconcile.Reconciler{
					NewObjectReconciler(ReadarrDownloadClients, c, events.NewFakeRecorder(50), nil),
					NewObjectReconciler(ReadarrIndexers, c, events.NewFakeRecorder(50), nil),
				}
			},
		},
	} {
		t.Run(app.name, func(t *testing.T) {
			ctx := context.Background()
			missing := objectClient(t, append([]client.Object{apiKeySecret()}, app.objects...)...)
			for i, r := range app.reconcilers(missing) {
				_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(app.objects[i])})
				require.NoError(t, err)
				obj := app.objects[i].DeepCopyObject().(reconciler.ConfigResource)
				require.NoError(t, missing.Get(ctx, client.ObjectKeyFromObject(obj), obj))
				assert.Equal(t, app.kind+` "app" not found`, synced(t, obj.GetConfigStatus().Conditions).Message)
			}

			fa, url := newFakeArr(t, app.apiVer, nil)
			c := objectClient(t, append([]client.Object{apiKeySecret(), app.instance(url)}, app.objects...)...)
			for i, r := range app.reconcilers(c) {
				_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(app.objects[i])})
				require.NoError(t, err)
			}
			assert.True(t, fa.saw("POST", fa.path("downloadclient")), "the download client never reached %s", app.name)
			assert.True(t, fa.saw("POST", fa.path("indexer")), "the indexer never reached %s", app.name)
		})
	}
}

func TestObjectDownloadClientAppRefUsesTheReferencedClient(t *testing.T) {
	creds := func(key string) commonv1alpha1.SecretKeyRef {
		return commonv1alpha1.SecretKeyRef{Name: "app-credentials", Key: key}
	}
	secret := apiKeySecret()
	secret.Data["username"], secret.Data["password"], secret.Data["sab-key"] = []byte("admin"), []byte("hunter2"), []byte("sab-api-key")
	tests := []struct {
		kind   string
		target client.Object
		want   map[string]any
		fields map[string]any
	}{
		{
			kind: "QBittorrent",
			target: &downloadsv1alpha1.QBittorrent{Name: "qbittorrent", Namespace: "media", Spec: downloadsv1alpha1.QBittorrentSpec{
				Connection: downloadsv1alpha1.QBittorrentConnection{URL: "http://qbittorrent.media.svc:8080/qbt",
					UsernameSecretRef: creds("username"), PasswordSecretRef: creds("password")},
			}},
			want: map[string]any{"implementation": "QBittorrent", "configContract": "QBittorrentSettings", "protocol": "torrent"},
			fields: map[string]any{"host": "qbittorrent.media.svc", "port": float64(8080), "useSsl": false, "urlBase": "/qbt",
				"username": "admin", "password": "hunter2"},
		},
		{
			kind: "Sabnzbd",
			target: &downloadsv1alpha1.Sabnzbd{Name: "sabnzbd", Namespace: "media", Spec: downloadsv1alpha1.SabnzbdSpec{
				Connection: commonv1alpha1.AppConnection{URL: "https://sabnzbd.example.com", APIKeySecretRef: creds("sab-key")},
			}},
			want: map[string]any{"implementation": "Sabnzbd", "configContract": "SabnzbdSettings", "protocol": "usenet"},
			fields: map[string]any{"host": "sabnzbd.example.com", "port": float64(443), "useSsl": true, "urlBase": "",
				"apiKey": "sab-api-key"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			fa, url := newFakeArr(t, "v3", nil)
			dc := sonarrDownloadClient()
			dc.Spec.DownloadClient = commonv1alpha1.DownloadClient{Name: "client", Category: "tv",
				AppRef: commonv1alpha1.AppRef{Kind: tt.kind, Name: tt.target.GetName()}}

			got := reconcileDownloadClient(t, secret.DeepCopy(), sonarrInstanceAt(url, nil), dc, tt.target)

			require.Equal(t, metav1.ConditionTrue, synced(t, got.Status.Conditions).Status, synced(t, got.Status.Conditions).Message)
			sent := fa.sent(fa.path("downloadclient"))
			require.Len(t, sent, 1)
			for k, v := range tt.want {
				assert.Equal(t, v, sent[0][k], k)
			}
			fields := map[string]any{}
			for _, f := range sent[0]["fields"].([]any) {
				m := f.(map[string]any)
				if _, ok := tt.fields[m["name"].(string)]; ok {
					fields[m["name"].(string)] = m["value"]
				}
			}
			assert.Equal(t, tt.fields, fields)
		})
	}
}

func TestObjectDownloadClientWithAMissingAppRefWritesNothing(t *testing.T) {
	fa, url := newFakeArr(t, "v3", nil)
	dc := sonarrDownloadClient()
	dc.Spec.DownloadClient = commonv1alpha1.DownloadClient{Name: "qBittorrent",
		AppRef: commonv1alpha1.AppRef{Kind: "QBittorrent", Name: "qbittorrent"}}

	got := reconcileDownloadClient(t, apiKeySecret(), sonarrInstanceAt(url, nil), dc)

	assert.False(t, fa.sawPrefix("POST", "/api/"))
	cond := synced(t, got.Status.Conditions)
	assert.Equal(t, reconciler.ReasonReferenceNotFound, cond.Reason)
	assert.Equal(t, `download client "qBittorrent" appRef: QBittorrent "qbittorrent" not found`, cond.Message)
}

func TestObjectDownloadClientNamingAnAppTheOperatorDoesntRunWaitsForASpecChange(t *testing.T) {
	fa, url := newFakeArr(t, "v3", nil)
	dc := sonarrDownloadClient()
	dc.Spec.DownloadClient = commonv1alpha1.DownloadClient{Name: "qBittorrent",
		AppRef: commonv1alpha1.AppRef{Kind: "QBittorrent", Name: "qbittorrent"}}
	c := objectClient(t, apiKeySecret(), sonarrInstanceAt(url, nil), dc)
	r := NewObjectReconciler(SonarrDownloadClients, c, events.NewFakeRecorder(50), appref.Apps{"sonarr": true, "sonarrdownloadclient": true})
	key := types.NamespacedName{Name: "tv-qbittorrent", Namespace: "media"}

	result, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key})
	require.NoError(t, err)

	assert.GreaterOrEqual(t, result.RequeueAfter, reconciler.DefaultReconcileInterval,
		"only a spec change can fix the reference, so the pass waits for one or the interval")
	assert.False(t, fa.sawPrefix("POST", "/api/"))
	var got pvrv1alpha1.SonarrDownloadClient
	require.NoError(t, c.Get(context.Background(), key, &got))
	cond := synced(t, got.Status.Conditions)
	assert.Equal(t, reconciler.ReasonInvalidConfig, cond.Reason)
	assert.Equal(t, `download client "qBittorrent" appRef: QBittorrent "qbittorrent": the operator doesn't run the qbittorrent controller`, cond.Message)
}

func TestResolveDownloadClientSecrets(t *testing.T) {
	secret := &corev1.Secret{
		Name: "qbit-creds", Namespace: "default",
		Data: map[string][]byte{
			"username": []byte("admin"),
			"password": []byte("pass123"),
		},
	}
	fc := objectClient(t, secret)

	dcs := []commonv1alpha1.DownloadClient{
		{
			Name:              "qbit",
			UsernameSecretRef: commonv1alpha1.SecretKeyRef{Name: "qbit-creds", Key: "username"},
			PasswordSecretRef: commonv1alpha1.SecretKeyRef{Name: "qbit-creds", Key: "password"},
		},
		{
			Name: "no-secrets",
		},
	}

	resolved, err := resolveDownloadClientSecrets(context.Background(), fc, "default", dcs)
	require.NoError(t, err)
	assert.Equal(t, "admin", resolved["qbit"].Username)
	assert.Equal(t, "pass123", resolved["qbit"].Password)
	assert.Empty(t, resolved["no-secrets"].Username)
}

func TestResolveDownloadClientSecrets_MissingSecret(t *testing.T) {
	fc := objectClient(t)

	dcs := []commonv1alpha1.DownloadClient{
		{
			Name:              "qbit",
			UsernameSecretRef: commonv1alpha1.SecretKeyRef{Name: "nonexistent", Key: "username"},
		},
	}

	_, err := resolveDownloadClientSecrets(context.Background(), fc, "default", dcs)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "username")
}
