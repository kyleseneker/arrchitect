package envtest

import (
	"context"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/config"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	automationv1alpha1 "github.com/kyleseneker/arrchitect/api/automation/v1alpha1"
	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	curationv1alpha1 "github.com/kyleseneker/arrchitect/api/curation/v1alpha1"
	downloadsv1alpha1 "github.com/kyleseneker/arrchitect/api/downloads/v1alpha1"
	indexersv1alpha1 "github.com/kyleseneker/arrchitect/api/indexers/v1alpha1"
	mediaserversv1alpha1 "github.com/kyleseneker/arrchitect/api/mediaservers/v1alpha1"
	pvrv1alpha1 "github.com/kyleseneker/arrchitect/api/pvr/v1alpha1"
	requestsv1alpha1 "github.com/kyleseneker/arrchitect/api/requests/v1alpha1"
	subtitlesv1alpha1 "github.com/kyleseneker/arrchitect/api/subtitles/v1alpha1"
	transcodev1alpha1 "github.com/kyleseneker/arrchitect/api/transcode/v1alpha1"
	"github.com/kyleseneker/arrchitect/internal/controller/automation"
	"github.com/kyleseneker/arrchitect/internal/controller/curation"
	"github.com/kyleseneker/arrchitect/internal/controller/downloads"
	"github.com/kyleseneker/arrchitect/internal/controller/indexers"
	"github.com/kyleseneker/arrchitect/internal/controller/mediaservers"
	"github.com/kyleseneker/arrchitect/internal/controller/pvr"
	"github.com/kyleseneker/arrchitect/internal/controller/requests"
	"github.com/kyleseneker/arrchitect/internal/controller/subtitles"
	"github.com/kyleseneker/arrchitect/internal/controller/transcode"
	"github.com/kyleseneker/arrchitect/internal/engine"
	"github.com/kyleseneker/arrchitect/internal/operator"
)

// controllerCase describes one kind well enough to drive it through a create,
// reconcile and delete against a real API server.
type controllerCase struct {
	name        string
	addToScheme func(*runtime.Scheme) error
	newCR       func(url string) client.Object
	newObj      func() client.Object
	newRec      func(client.Client, *runtime.Scheme) reconcile.Reconciler
}

func appConn(url string) commonv1alpha1.AppConnection {
	return commonv1alpha1.AppConnection{
		URL:             url,
		APIKeySecretRef: commonv1alpha1.SecretKeyRef{Name: "app-credentials", Key: "api-key"},
	}
}

func secretRef(key string) commonv1alpha1.SecretKeyRef {
	return commonv1alpha1.SecretKeyRef{Name: "app-credentials", Key: key}
}

func orphan() *commonv1alpha1.ReconcileConfig {
	p := "orphan"
	return &commonv1alpha1.ReconcileConfig{DeletionPolicy: p}
}

func meta(name string) metav1.ObjectMeta {
	return metav1.ObjectMeta{Name: name, Namespace: "default"}
}

func controllerCases() []controllerCase {
	return []controllerCase{
		{
			name:        "Bazarr",
			addToScheme: subtitlesv1alpha1.AddToScheme,
			newObj:      func() client.Object { return &subtitlesv1alpha1.Bazarr{} },
			newCR: func(url string) client.Object {
				return &subtitlesv1alpha1.Bazarr{
					ObjectMeta: meta("bazarr"),
					Spec:       subtitlesv1alpha1.BazarrSpec{Connection: appConn(url), Reconcile: orphan()},
				}
			},
			newRec: func(c client.Client, s *runtime.Scheme) reconcile.Reconciler {
				return &subtitles.BazarrReconciler{Client: c, Scheme: s, Recorder: events.NewFakeRecorder(50)}
			},
		},
		{
			name:        "Maintainerr",
			addToScheme: curationv1alpha1.AddToScheme,
			newObj:      func() client.Object { return &curationv1alpha1.Maintainerr{} },
			newCR: func(url string) client.Object {
				return &curationv1alpha1.Maintainerr{
					ObjectMeta: meta("maintainerr"),
					Spec: curationv1alpha1.MaintainerrSpec{
						Connection: curationv1alpha1.MaintainerrConnection{URL: url}, Reconcile: orphan(),
					},
				}
			},
			newRec: func(c client.Client, s *runtime.Scheme) reconcile.Reconciler {
				return &curation.MaintainerrReconciler{Client: c, Scheme: s, Recorder: events.NewFakeRecorder(50)}
			},
		},
		{
			name:        "Autobrr",
			addToScheme: automationv1alpha1.AddToScheme,
			newObj:      func() client.Object { return &automationv1alpha1.Autobrr{} },
			newCR: func(url string) client.Object {
				return &automationv1alpha1.Autobrr{
					ObjectMeta: meta("autobrr"),
					Spec:       automationv1alpha1.AutobrrSpec{Connection: appConn(url), Reconcile: orphan()},
				}
			},
			newRec: func(c client.Client, s *runtime.Scheme) reconcile.Reconciler {
				return &automation.AutobrrReconciler{Client: c, Scheme: s, Recorder: events.NewFakeRecorder(50)}
			},
		},
		{
			name:        "Sabnzbd",
			addToScheme: downloadsv1alpha1.AddToScheme,
			newObj:      func() client.Object { return &downloadsv1alpha1.Sabnzbd{} },
			newCR: func(url string) client.Object {
				return &downloadsv1alpha1.Sabnzbd{
					ObjectMeta: meta("sabnzbd"),
					Spec:       downloadsv1alpha1.SabnzbdSpec{Connection: appConn(url), Reconcile: orphan()},
				}
			},
			newRec: func(c client.Client, s *runtime.Scheme) reconcile.Reconciler {
				return &downloads.SabnzbdReconciler{Client: c, Scheme: s, Recorder: events.NewFakeRecorder(50)}
			},
		},
		{
			name:        "Sonarr",
			addToScheme: pvrv1alpha1.AddToScheme,
			newObj:      func() client.Object { return &pvrv1alpha1.Sonarr{} },
			newCR: func(url string) client.Object {
				return &pvrv1alpha1.Sonarr{
					ObjectMeta: meta("sonarr"),
					Spec:       pvrv1alpha1.SonarrSpec{Connection: appConn(url), Reconcile: orphan()},
				}
			},
			newRec: func(c client.Client, s *runtime.Scheme) reconcile.Reconciler {
				return &pvr.SonarrReconciler{Client: c, Scheme: s, Recorder: events.NewFakeRecorder(50)}
			},
		},
		{
			name:        "SonarrIndexer",
			addToScheme: pvrv1alpha1.AddToScheme,
			newObj:      func() client.Object { return &pvrv1alpha1.SonarrIndexer{} },
			newCR: func(string) client.Object {
				return &pvrv1alpha1.SonarrIndexer{
					ObjectMeta: meta("sonarr-indexer"),
					Spec: pvrv1alpha1.SonarrIndexerSpec{
						SonarrRef: commonv1alpha1.LocalObjectReference{Name: "sonarr"}, DeletionPolicy: "orphan",
						Indexer: commonv1alpha1.Indexer{Name: "x", Protocol: "torrent", Implementation: "Torznab"},
					},
				}
			},
			newRec: func(c client.Client, s *runtime.Scheme) reconcile.Reconciler {
				return &pvr.SonarrIndexerReconciler{Client: c, Scheme: s, Recorder: events.NewFakeRecorder(50)}
			},
		},
		{
			name:        "SonarrDownloadClient",
			addToScheme: pvrv1alpha1.AddToScheme,
			newObj:      func() client.Object { return &pvrv1alpha1.SonarrDownloadClient{} },
			newCR: func(string) client.Object {
				return &pvrv1alpha1.SonarrDownloadClient{
					ObjectMeta: meta("sonarr-download-client"),
					Spec: pvrv1alpha1.SonarrDownloadClientSpec{
						SonarrRef: commonv1alpha1.LocalObjectReference{Name: "sonarr"}, DeletionPolicy: "orphan",
						DownloadClient: commonv1alpha1.DownloadClient{
							Name: "x", Protocol: "torrent", Implementation: "QBittorrent", Host: "qbittorrent", Port: 8080,
						},
					},
				}
			},
			newRec: func(c client.Client, s *runtime.Scheme) reconcile.Reconciler {
				return &pvr.SonarrDownloadClientReconciler{Client: c, Scheme: s, Recorder: events.NewFakeRecorder(50)}
			},
		},
		{
			name:        "Prowlarr",
			addToScheme: indexersv1alpha1.AddToScheme,
			newObj:      func() client.Object { return &indexersv1alpha1.Prowlarr{} },
			newCR: func(url string) client.Object {
				return &indexersv1alpha1.Prowlarr{
					ObjectMeta: meta("prowlarr"),
					Spec:       indexersv1alpha1.ProwlarrSpec{Connection: appConn(url), Reconcile: orphan()},
				}
			},
			newRec: func(c client.Client, s *runtime.Scheme) reconcile.Reconciler {
				return &indexers.ProwlarrReconciler{Client: c, Scheme: s, Recorder: events.NewFakeRecorder(50)}
			},
		},
		{
			name:        "Jellyfin",
			addToScheme: mediaserversv1alpha1.AddToScheme,
			newObj:      func() client.Object { return &mediaserversv1alpha1.Jellyfin{} },
			newCR: func(url string) client.Object {
				return &mediaserversv1alpha1.Jellyfin{
					ObjectMeta: meta("jellyfin"),
					Spec: mediaserversv1alpha1.JellyfinSpec{
						Connection: mediaserversv1alpha1.JellyfinConnection{URL: url},
						AdminUser: mediaserversv1alpha1.JellyfinAdminUser{
							UsernameSecretRef: secretRef("username"), PasswordSecretRef: secretRef("password"),
						},
						Reconcile: orphan(),
					},
				}
			},
			newRec: func(c client.Client, s *runtime.Scheme) reconcile.Reconciler {
				return &mediaservers.JellyfinReconciler{Client: c, Scheme: s, Recorder: events.NewFakeRecorder(50)}
			},
		},
		{
			name:        "Seerr",
			addToScheme: requestsv1alpha1.AddToScheme,
			newObj:      func() client.Object { return &requestsv1alpha1.Seerr{} },
			newCR: func(url string) client.Object {
				ref := secretRef("api-key")
				return &requestsv1alpha1.Seerr{
					ObjectMeta: meta("seerr"),
					Spec: requestsv1alpha1.SeerrSpec{
						Connection: requestsv1alpha1.SeerrConnection{URL: url, APIKeySecretRef: &ref},
						Reconcile:  orphan(),
					},
				}
			},
			newRec: func(c client.Client, s *runtime.Scheme) reconcile.Reconciler {
				return &requests.SeerrReconciler{Client: c, Scheme: s, Recorder: events.NewFakeRecorder(50)}
			},
		},
		{
			name:        "QBittorrent",
			addToScheme: downloadsv1alpha1.AddToScheme,
			newObj:      func() client.Object { return &downloadsv1alpha1.QBittorrent{} },
			newCR: func(url string) client.Object {
				return &downloadsv1alpha1.QBittorrent{
					ObjectMeta: meta("qbittorrent"),
					Spec: downloadsv1alpha1.QBittorrentSpec{
						Connection: downloadsv1alpha1.QBittorrentConnection{
							URL:               url,
							UsernameSecretRef: secretRef("username"),
							PasswordSecretRef: secretRef("password"),
						},
						Reconcile: orphan(),
					},
				}
			},
			newRec: func(c client.Client, s *runtime.Scheme) reconcile.Reconciler {
				return &downloads.QBittorrentReconciler{Client: c, Scheme: s, Recorder: events.NewFakeRecorder(50)}
			},
		},
		{
			name:        "Radarr",
			addToScheme: pvrv1alpha1.AddToScheme,
			newObj:      func() client.Object { return &pvrv1alpha1.Radarr{} },
			newCR: func(url string) client.Object {
				return &pvrv1alpha1.Radarr{
					ObjectMeta: meta("radarr"),
					Spec: pvrv1alpha1.RadarrSpec{
						Connection: appConn(url),
						Reconcile:  orphan(),
					},
				}
			},
			newRec: func(c client.Client, s *runtime.Scheme) reconcile.Reconciler {
				return &pvr.RadarrReconciler{Client: c, Scheme: s, Recorder: events.NewFakeRecorder(50)}
			},
		},
		{
			name:        "RadarrIndexer",
			addToScheme: pvrv1alpha1.AddToScheme,
			newObj:      func() client.Object { return &pvrv1alpha1.RadarrIndexer{} },
			newCR: func(string) client.Object {
				return &pvrv1alpha1.RadarrIndexer{
					ObjectMeta: meta("radarr-indexer"),
					Spec: pvrv1alpha1.RadarrIndexerSpec{
						RadarrRef: commonv1alpha1.LocalObjectReference{Name: "radarr"}, DeletionPolicy: "orphan",
						Indexer: commonv1alpha1.Indexer{Name: "x", Protocol: "torrent", Implementation: "Torznab"},
					},
				}
			},
			newRec: func(c client.Client, s *runtime.Scheme) reconcile.Reconciler {
				return &pvr.RadarrIndexerReconciler{Client: c, Scheme: s, Recorder: events.NewFakeRecorder(50)}
			},
		},
		{
			name:        "RadarrDownloadClient",
			addToScheme: pvrv1alpha1.AddToScheme,
			newObj:      func() client.Object { return &pvrv1alpha1.RadarrDownloadClient{} },
			newCR: func(string) client.Object {
				return &pvrv1alpha1.RadarrDownloadClient{
					ObjectMeta: meta("radarr-download-client"),
					Spec: pvrv1alpha1.RadarrDownloadClientSpec{
						RadarrRef: commonv1alpha1.LocalObjectReference{Name: "radarr"}, DeletionPolicy: "orphan",
						DownloadClient: commonv1alpha1.DownloadClient{
							Name: "x", Protocol: "torrent", Implementation: "QBittorrent", Host: "qbittorrent", Port: 8080,
						},
					},
				}
			},
			newRec: func(c client.Client, s *runtime.Scheme) reconcile.Reconciler {
				return &pvr.RadarrDownloadClientReconciler{Client: c, Scheme: s, Recorder: events.NewFakeRecorder(50)}
			},
		},
		{
			name:        "Lidarr",
			addToScheme: pvrv1alpha1.AddToScheme,
			newObj:      func() client.Object { return &pvrv1alpha1.Lidarr{} },
			newCR: func(url string) client.Object {
				return &pvrv1alpha1.Lidarr{
					ObjectMeta: meta("lidarr"),
					Spec: pvrv1alpha1.LidarrSpec{
						Connection: appConn(url),
						Reconcile:  orphan(),
					},
				}
			},
			newRec: func(c client.Client, s *runtime.Scheme) reconcile.Reconciler {
				return &pvr.LidarrReconciler{Client: c, Scheme: s, Recorder: events.NewFakeRecorder(50)}
			},
		},
		{
			name:        "LidarrIndexer",
			addToScheme: pvrv1alpha1.AddToScheme,
			newObj:      func() client.Object { return &pvrv1alpha1.LidarrIndexer{} },
			newCR: func(string) client.Object {
				return &pvrv1alpha1.LidarrIndexer{
					ObjectMeta: meta("lidarr-indexer"),
					Spec: pvrv1alpha1.LidarrIndexerSpec{
						LidarrRef: commonv1alpha1.LocalObjectReference{Name: "lidarr"}, DeletionPolicy: "orphan",
						Indexer: commonv1alpha1.Indexer{Name: "x", Protocol: "torrent", Implementation: "Torznab"},
					},
				}
			},
			newRec: func(c client.Client, s *runtime.Scheme) reconcile.Reconciler {
				return &pvr.LidarrIndexerReconciler{Client: c, Scheme: s, Recorder: events.NewFakeRecorder(50)}
			},
		},
		{
			name:        "LidarrDownloadClient",
			addToScheme: pvrv1alpha1.AddToScheme,
			newObj:      func() client.Object { return &pvrv1alpha1.LidarrDownloadClient{} },
			newCR: func(string) client.Object {
				return &pvrv1alpha1.LidarrDownloadClient{
					ObjectMeta: meta("lidarr-download-client"),
					Spec: pvrv1alpha1.LidarrDownloadClientSpec{
						LidarrRef: commonv1alpha1.LocalObjectReference{Name: "lidarr"}, DeletionPolicy: "orphan",
						DownloadClient: commonv1alpha1.DownloadClient{
							Name: "x", Protocol: "torrent", Implementation: "QBittorrent", Host: "qbittorrent", Port: 8080,
						},
					},
				}
			},
			newRec: func(c client.Client, s *runtime.Scheme) reconcile.Reconciler {
				return &pvr.LidarrDownloadClientReconciler{Client: c, Scheme: s, Recorder: events.NewFakeRecorder(50)}
			},
		},
		{
			name:        "Readarr",
			addToScheme: pvrv1alpha1.AddToScheme,
			newObj:      func() client.Object { return &pvrv1alpha1.Readarr{} },
			newCR: func(url string) client.Object {
				return &pvrv1alpha1.Readarr{
					ObjectMeta: meta("readarr"),
					Spec: pvrv1alpha1.ReadarrSpec{
						Connection: appConn(url),
						Reconcile:  orphan(),
					},
				}
			},
			newRec: func(c client.Client, s *runtime.Scheme) reconcile.Reconciler {
				return &pvr.ReadarrReconciler{Client: c, Scheme: s, Recorder: events.NewFakeRecorder(50)}
			},
		},
		{
			name:        "ReadarrIndexer",
			addToScheme: pvrv1alpha1.AddToScheme,
			newObj:      func() client.Object { return &pvrv1alpha1.ReadarrIndexer{} },
			newCR: func(string) client.Object {
				return &pvrv1alpha1.ReadarrIndexer{
					ObjectMeta: meta("readarr-indexer"),
					Spec: pvrv1alpha1.ReadarrIndexerSpec{
						ReadarrRef: commonv1alpha1.LocalObjectReference{Name: "readarr"}, DeletionPolicy: "orphan",
						Indexer: commonv1alpha1.Indexer{Name: "x", Protocol: "torrent", Implementation: "Torznab"},
					},
				}
			},
			newRec: func(c client.Client, s *runtime.Scheme) reconcile.Reconciler {
				return &pvr.ReadarrIndexerReconciler{Client: c, Scheme: s, Recorder: events.NewFakeRecorder(50)}
			},
		},
		{
			name:        "ReadarrDownloadClient",
			addToScheme: pvrv1alpha1.AddToScheme,
			newObj:      func() client.Object { return &pvrv1alpha1.ReadarrDownloadClient{} },
			newCR: func(string) client.Object {
				return &pvrv1alpha1.ReadarrDownloadClient{
					ObjectMeta: meta("readarr-download-client"),
					Spec: pvrv1alpha1.ReadarrDownloadClientSpec{
						ReadarrRef: commonv1alpha1.LocalObjectReference{Name: "readarr"}, DeletionPolicy: "orphan",
						DownloadClient: commonv1alpha1.DownloadClient{
							Name: "x", Protocol: "torrent", Implementation: "QBittorrent", Host: "qbittorrent", Port: 8080,
						},
					},
				}
			},
			newRec: func(c client.Client, s *runtime.Scheme) reconcile.Reconciler {
				return &pvr.ReadarrDownloadClientReconciler{Client: c, Scheme: s, Recorder: events.NewFakeRecorder(50)}
			},
		},
		{
			name:        "FlareSolverr",
			addToScheme: indexersv1alpha1.AddToScheme,
			newObj:      func() client.Object { return &indexersv1alpha1.FlareSolverr{} },
			newCR: func(url string) client.Object {
				return &indexersv1alpha1.FlareSolverr{
					ObjectMeta: meta("flaresolverr"),
					Spec: indexersv1alpha1.FlareSolverrSpec{
						Connection: indexersv1alpha1.FlareSolverrConnection{URL: url},
						Reconcile:  orphan(),
					},
				}
			},
			newRec: func(c client.Client, s *runtime.Scheme) reconcile.Reconciler {
				return &indexers.FlareSolverrReconciler{Client: c, Scheme: s, Recorder: events.NewFakeRecorder(50)}
			},
		},
		{
			name:        "Plex",
			addToScheme: mediaserversv1alpha1.AddToScheme,
			newObj:      func() client.Object { return &mediaserversv1alpha1.Plex{} },
			newCR: func(url string) client.Object {
				return &mediaserversv1alpha1.Plex{
					ObjectMeta: meta("plex"),
					Spec: mediaserversv1alpha1.PlexSpec{
						Connection: mediaserversv1alpha1.PlexConnection{URL: url, TokenSecretRef: secretRef("token")},
						Reconcile:  orphan(),
					},
				}
			},
			newRec: func(c client.Client, s *runtime.Scheme) reconcile.Reconciler {
				return &mediaservers.PlexReconciler{Client: c, Scheme: s, Recorder: events.NewFakeRecorder(50)}
			},
		},
		{
			name:        "Tdarr",
			addToScheme: transcodev1alpha1.AddToScheme,
			newObj:      func() client.Object { return &transcodev1alpha1.Tdarr{} },
			newCR: func(url string) client.Object {
				return &transcodev1alpha1.Tdarr{
					ObjectMeta: meta("tdarr"),
					Spec: transcodev1alpha1.TdarrSpec{
						Connection: appConn(url),
						Reconcile:  orphan(),
					},
				}
			},
			newRec: func(c client.Client, s *runtime.Scheme) reconcile.Reconciler {
				return &transcode.TdarrReconciler{Client: c, Scheme: s, Recorder: events.NewFakeRecorder(50)}
			},
		},
	}
}

// Every kind must persist status through the subresource and, without a delete
// policy, hold no finalizer. Both are API-server behaviours a fake client fakes away.
func TestControllersPersistStatusAndReleaseOnDelete(t *testing.T) {
	cases := controllerCases()
	crds, err := filepath.Glob(filepath.Join("..", "..", "config", "crd", "bases", "*.yaml"))
	require.NoError(t, err)
	require.Len(t, cases, len(crds), "every kind needs a case here")
	adders := make([]func(*runtime.Scheme) error, 0, len(cases))
	for _, tc := range cases {
		adders = append(adders, tc.addToScheme)
	}
	c := startAPIServerWith(t, adders...)
	url := fakeAppServer(t)
	ctx := context.Background()

	require.NoError(t, c.Create(ctx, &corev1.Secret{
		ObjectMeta: meta("app-credentials"),
		Data: map[string][]byte{
			"api-key":  []byte("k"),
			"username": []byte("u"),
			"password": []byte("p"),
			"token":    []byte("t"),
		},
	}), "creating secret")

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cr := tc.newCR(url)
			require.NoError(t, c.Create(ctx, cr), "creating CR")
			key := client.ObjectKeyFromObject(cr)
			r := tc.newRec(c, c.Scheme())

			_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: key})
			require.NoError(t, err, "reconcile")

			got := tc.newObj()
			require.NoError(t, c.Get(ctx, key, got), "re-reading CR")
			conditions := got.(interface{ GetConditions() *[]metav1.Condition }).GetConditions()
			assert.NotNil(t, apimeta.FindStatusCondition(*conditions, engine.ConditionReady),
				"no Ready condition persisted; the status subresource write did not land")
			require.Empty(t, got.GetFinalizers(), "an orphan resource must not wait on the operator to be deleted")

			require.NoError(t, c.Delete(ctx, got), "deleting CR")
			err = c.Get(ctx, key, tc.newObj())
			require.True(t, apierrors.IsNotFound(err), "CR still present after deletion: %v", err)
		})
	}
}

// A changed Secret re-reconciles the configs that refer to it rather than
// waiting out the requeue. FlareSolverr reads only a TLS CA, which nothing
// watched before.
func TestSecretChangeReconcilesTheConfigReferringToIt(t *testing.T) {
	cfg, c := startControlPlane(t, indexersv1alpha1.AddToScheme)
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:     c.Scheme(),
		Metrics:    metricsserver.Options{BindAddress: "0"},
		Controller: config.Controller{SkipNameValidation: new(true)},
		Client:     operator.ClientOptions(),
	})
	require.NoError(t, err)
	rec := &indexers.FlareSolverrReconciler{
		Client: mgr.GetClient(), Scheme: mgr.GetScheme(), Recorder: events.NewFakeRecorder(100),
	}
	require.NoError(t, rec.SetupWithManager(mgr))
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = mgr.Start(ctx) }()

	app := httptest.NewTLSServer(http.NotFoundHandler())
	t.Cleanup(app.Close)
	cr := &indexersv1alpha1.FlareSolverr{ObjectMeta: meta("watched"), Spec: indexersv1alpha1.FlareSolverrSpec{
		Connection: indexersv1alpha1.FlareSolverrConnection{URL: app.URL, TLS: &commonv1alpha1.TLSConfig{
			CASecretRef: commonv1alpha1.SecretKeyRef{Name: "private-ca", Key: "ca.crt"},
		}},
	}}
	require.NoError(t, c.Create(ctx, cr))
	syncedReason := func() string {
		var got indexersv1alpha1.FlareSolverr
		if c.Get(ctx, client.ObjectKeyFromObject(cr), &got) != nil {
			return ""
		}
		for _, cond := range got.Status.Conditions {
			if cond.Type == engine.ConditionSynced {
				return cond.Reason
			}
		}
		return ""
	}
	require.Eventually(t, func() bool { return syncedReason() == engine.ReasonSecretNotFound },
		10*time.Second, 100*time.Millisecond, "the missing CA was never reported")

	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: app.Certificate().Raw})
	require.NoError(t, c.Create(ctx, &corev1.Secret{
		ObjectMeta: meta("private-ca"), Data: map[string][]byte{"ca.crt": caPEM},
	}))
	// Well inside the 30-second requeue for a missing Secret.
	require.Eventually(t, func() bool {
		reason := syncedReason()
		return reason != "" && reason != engine.ReasonSecretNotFound
	}, 10*time.Second, 100*time.Millisecond, "creating the CA Secret did not re-reconcile the config")
}

// A finalizer genuinely blocks deletion here, so a delete policy that fails to
// clear it would wedge `kubectl delete` rather than passing silently. The
// finalizer handling is shared, so one kind covers it.
func TestDeletePolicyReleasesTheObject(t *testing.T) {
	c := startAPIServerWith(t, transcodev1alpha1.AddToScheme)
	ctx := context.Background()
	require.NoError(t, c.Create(ctx, &corev1.Secret{
		ObjectMeta: meta("app-credentials"), Data: map[string][]byte{"api-key": []byte("k")},
	}))
	deletePolicy := "delete"
	cr := &transcodev1alpha1.Tdarr{
		ObjectMeta: meta("tdarr"),
		Spec: transcodev1alpha1.TdarrSpec{
			Connection: appConn(fakeAppServer(t)),
			Reconcile:  &commonv1alpha1.ReconcileConfig{DeletionPolicy: deletePolicy},
		},
	}
	require.NoError(t, c.Create(ctx, cr))
	r := &transcode.TdarrReconciler{Client: c, Scheme: c.Scheme(), Recorder: events.NewFakeRecorder(50)}
	req := ctrl.Request{Name: cr.Name, Namespace: cr.Namespace}

	_, err := r.Reconcile(ctx, req)
	require.NoError(t, err, "first reconcile")
	require.NoError(t, c.Get(ctx, req.NamespacedName, cr))
	require.NotEmpty(t, cr.Finalizers, "no finalizer was set, so this test would pass without exercising deletion")
	require.NoError(t, c.Delete(ctx, cr))

	require.Eventually(t, func() bool {
		_, _ = r.Reconcile(ctx, req)
		return apierrors.IsNotFound(c.Get(ctx, req.NamespacedName, &transcodev1alpha1.Tdarr{}))
	}, 30*time.Second, 200*time.Millisecond, "the resource is still present after deletion")
}

// An object waiting on its instance is reconciled as soon as the instance
// appears, through the index on the reference, rather than on its next requeue.
func TestInstanceCreationReconcilesItsObjects(t *testing.T) {
	cfg, c := startControlPlane(t, pvrv1alpha1.AddToScheme)
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:     c.Scheme(),
		Metrics:    metricsserver.Options{BindAddress: "0"},
		Controller: config.Controller{SkipNameValidation: new(true)},
		Client:     operator.ClientOptions(),
	})
	require.NoError(t, err)
	rec := &pvr.SonarrDownloadClientReconciler{
		Client: mgr.GetClient(), Scheme: mgr.GetScheme(), Recorder: events.NewFakeRecorder(100),
	}
	require.NoError(t, rec.SetupWithManager(mgr))
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = mgr.Start(ctx) }()

	require.NoError(t, c.Create(ctx, &corev1.Secret{
		ObjectMeta: meta("app-credentials"), Data: map[string][]byte{"api-key": []byte("k")},
	}))
	dc := &pvrv1alpha1.SonarrDownloadClient{ObjectMeta: meta("tv-qbittorrent"), Spec: pvrv1alpha1.SonarrDownloadClientSpec{
		SonarrRef: commonv1alpha1.LocalObjectReference{Name: "tv"}, DeletionPolicy: "orphan",
		DownloadClient: commonv1alpha1.DownloadClient{
			Name: "qBittorrent", Protocol: "torrent", Implementation: "QBittorrent", Host: "qbittorrent", Port: 8080,
		},
	}}
	require.NoError(t, c.Create(ctx, dc))
	syncedReason := func() string {
		var got pvrv1alpha1.SonarrDownloadClient
		if c.Get(ctx, client.ObjectKeyFromObject(dc), &got) != nil {
			return ""
		}
		if cond := apimeta.FindStatusCondition(got.Status.Conditions, engine.ConditionSynced); cond != nil {
			return cond.Reason
		}
		return ""
	}
	require.Eventually(t, func() bool { return syncedReason() == engine.ReasonReferenceNotFound },
		10*time.Second, 100*time.Millisecond, "the missing Sonarr was never reported")

	require.NoError(t, c.Create(ctx, &pvrv1alpha1.Sonarr{
		ObjectMeta: meta("tv"), Spec: pvrv1alpha1.SonarrSpec{Connection: appConn(fakeAppServer(t)), Reconcile: orphan()},
	}))
	// Well inside the 30-second requeue for a missing instance.
	require.Eventually(t, func() bool {
		reason := syncedReason()
		return reason != "" && reason != engine.ReasonReferenceNotFound
	}, 10*time.Second, 100*time.Millisecond, "creating the Sonarr did not reconcile the download client")
}

// A resource whose reference can't be resolved yet is reconciled as soon as
// the referenced resource appears, and again when that resource's Secret does,
// rather than on its next requeue.
func TestAppRefChangesReconcileTheReferrer(t *testing.T) {
	cfg, c := startControlPlane(t, indexersv1alpha1.AddToScheme, pvrv1alpha1.AddToScheme)
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:     c.Scheme(),
		Metrics:    metricsserver.Options{BindAddress: "0"},
		Controller: config.Controller{SkipNameValidation: new(true)},
		Client:     operator.ClientOptions(),
	})
	require.NoError(t, err)
	rec := &indexers.ProwlarrReconciler{
		Client: mgr.GetClient(), Scheme: mgr.GetScheme(), Recorder: events.NewFakeRecorder(100),
	}
	require.NoError(t, rec.SetupWithManager(mgr))
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = mgr.Start(ctx) }()

	require.NoError(t, c.Create(ctx, &corev1.Secret{
		ObjectMeta: meta("app-credentials"), Data: map[string][]byte{"api-key": []byte("k")},
	}))
	prowlarr := &indexersv1alpha1.Prowlarr{ObjectMeta: meta("prowlarr"), Spec: indexersv1alpha1.ProwlarrSpec{
		Connection: appConn(fakeListAppServer(t)),
		Applications: []indexersv1alpha1.ProwlarrApplication{{
			Name: "Sonarr", AppRef: &commonv1alpha1.AppRef{Kind: "Sonarr", Name: "tv"},
		}},
	}}
	require.NoError(t, c.Create(ctx, prowlarr))
	message := func() string {
		var got indexersv1alpha1.Prowlarr
		if c.Get(ctx, client.ObjectKeyFromObject(prowlarr), &got) != nil {
			return ""
		}
		if cond := apimeta.FindStatusCondition(got.Status.Conditions, engine.ConditionSynced); cond != nil {
			return cond.Message
		}
		return ""
	}
	require.Eventually(t, func() bool { return strings.Contains(message(), `Sonarr "tv" not found`) },
		10*time.Second, 100*time.Millisecond, "the missing Sonarr was never reported: %s", message())

	require.NoError(t, c.Create(ctx, &pvrv1alpha1.Sonarr{ObjectMeta: meta("tv"), Spec: pvrv1alpha1.SonarrSpec{
		Connection: commonv1alpha1.AppConnection{URL: "http://sonarr:8989",
			APIKeySecretRef: commonv1alpha1.SecretKeyRef{Name: "sonarr-key", Key: "api-key"}},
		Reconcile: orphan(),
	}}))
	require.Eventually(t, func() bool { return strings.Contains(message(), `secret "sonarr-key" not found`) },
		10*time.Second, 100*time.Millisecond, "creating the Sonarr did not reconcile the Prowlarr")

	require.NoError(t, c.Create(ctx, &corev1.Secret{
		ObjectMeta: meta("sonarr-key"), Data: map[string][]byte{"api-key": []byte("k")},
	}))
	require.Eventually(t, func() bool {
		m := message()
		return m != "" && !strings.Contains(m, "sonarr-key")
	}, 10*time.Second, 100*time.Millisecond, "creating the Sonarr's Secret did not reconcile the Prowlarr")
}
