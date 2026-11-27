package pvr

import (
	"context"
	"fmt"
	"strings"
	"sync"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	pvrv1alpha1 "github.com/kyleseneker/arrchitect/api/pvr/v1alpha1"
	"github.com/kyleseneker/arrchitect/internal/controller/appref"
	"github.com/kyleseneker/arrchitect/internal/controller/reconciler"
	"github.com/kyleseneker/arrchitect/internal/controller/secretref"
	"github.com/kyleseneker/arrchitect/internal/engine"
	"github.com/kyleseneker/arrchitect/internal/failure"
	"github.com/kyleseneker/arrchitect/internal/httpapi"
	"github.com/kyleseneker/arrchitect/internal/secrets"
)

// instanceRefIndex indexes object resources by the instance they name, so a
// change to an instance requeues its objects.
const instanceRefIndex = ".spec.instanceRef"

// pvrInstance is what an object takes from the instance it belongs to.
type pvrInstance struct {
	connection commonv1alpha1.AppConnection
	policy     *commonv1alpha1.ReconcileConfig
}

// objectConn is how an object reaches the instance it names: the instance's
// client, or the object itself when the instance is missing, which leaves
// nothing to remove from it.
type objectConn struct {
	hc     *httpapi.HTTPClient
	orphan client.Object
}

// olderClaim names an older resource in list that configures the same item in
// the same instance as obj. That one keeps the item, so two resources never
// take turns rewriting it or delete it from under each other.
func olderClaim(ctx context.Context, c client.Client, list client.ObjectList, obj objectResource) (string, error) {
	if err := c.List(ctx, list, client.InNamespace(obj.GetNamespace())); err != nil {
		return "", err
	}
	items, err := apimeta.ExtractList(list)
	if err != nil {
		return "", err
	}
	var claimant client.Object
	for _, item := range items {
		other, ok := item.(objectResource)
		if !ok || other.GetName() == obj.GetName() || !older(other, obj) {
			continue
		}
		if other.InstanceName() != obj.InstanceName() || other.ItemName() != obj.ItemName() {
			continue
		}
		if claimant == nil || older(other, claimant) {
			claimant = other
		}
	}
	if claimant == nil {
		return "", nil
	}
	return claimant.GetName(), nil
}

func older(a, b client.Object) bool {
	ta, tb := a.GetCreationTimestamp(), b.GetCreationTimestamp()
	if !ta.Equal(&tb) {
		return ta.Before(&tb)
	}
	return a.GetName() < b.GetName()
}

// idOf finds the ID the app gave the object with this name.
func idOf(ctx context.Context, hc *httpapi.HTTPClient, path, name string) (int32, bool) {
	items, err := hc.GetJSONList(ctx, path)
	if err != nil {
		return 0, false
	}
	for _, item := range items {
		if item["name"] == name {
			if id, ok := item["id"].(float64); ok {
				return int32(id), true
			}
		}
	}
	return 0, false
}

// objectsOf requeues the objects that name a changed instance.
func objectsOf(c client.Client, list client.ObjectList) handler.MapFunc {
	return func(ctx context.Context, instance client.Object) []reconcile.Request {
		objects := list.DeepCopyObject().(client.ObjectList)
		if err := c.List(ctx, objects, client.InNamespace(instance.GetNamespace()),
			client.MatchingFields{instanceRefIndex: instance.GetName()}); err != nil {
			log.FromContext(ctx).Error(err, "listing the objects that name a changed instance")
			return nil
		}
		items, err := apimeta.ExtractList(objects)
		if err != nil {
			log.FromContext(ctx).Error(err, "reading the objects that name a changed instance")
			return nil
		}
		reqs := make([]reconcile.Request, 0, len(items))
		for _, item := range items {
			if o, ok := item.(client.Object); ok {
				reqs = append(reqs, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(o)})
			}
		}
		return reqs
	}
}

// instanceChanged passes an instance's spec changes and changes to whether it
// synced. An object can name tags and profiles the instance creates, so it is
// retried once the instance syncs rather than at its next interval.
func instanceChanged() predicate.Predicate {
	return predicate.Or(predicate.GenerationChangedPredicate{}, predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			return syncedStatus(e.ObjectOld) != syncedStatus(e.ObjectNew)
		},
		CreateFunc:  func(event.CreateEvent) bool { return false },
		DeleteFunc:  func(event.DeleteEvent) bool { return false },
		GenericFunc: func(event.GenericEvent) bool { return false },
	})
}

func syncedStatus(obj client.Object) metav1.ConditionStatus {
	cr, ok := obj.(reconciler.ConfigResource)
	if !ok {
		return ""
	}
	if cond := apimeta.FindStatusCondition(cr.GetConfigStatus().Conditions, reconciler.ConditionSynced); cond != nil {
		return cond.Status
	}
	return ""
}

// +kubebuilder:rbac:groups=arrchitect.dev,resources=sonarrindexers;radarrindexers;lidarrindexers;readarrindexers,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=arrchitect.dev,resources=sonarrindexers/status;radarrindexers/status;lidarrindexers/status;readarrindexers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=arrchitect.dev,resources=sonarrindexers/finalizers;radarrindexers/finalizers;lidarrindexers/finalizers;readarrindexers/finalizers,verbs=update
// +kubebuilder:rbac:groups=arrchitect.dev,resources=sonarrdownloadclients;radarrdownloadclients;lidarrdownloadclients;readarrdownloadclients,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=arrchitect.dev,resources=sonarrdownloadclients/status;radarrdownloadclients/status;lidarrdownloadclients/status;readarrdownloadclients/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=arrchitect.dev,resources=sonarrdownloadclients/finalizers;radarrdownloadclients/finalizers;lidarrdownloadclients/finalizers;readarrdownloadclients/finalizers,verbs=update
// +kubebuilder:rbac:groups=arrchitect.dev,resources=qbittorrents;sabnzbds,verbs=get;list;watch

// objectResource is a resource that configures one item, an indexer or a
// download client, in one PVR app.
type objectResource interface {
	reconciler.ConfigResource
	InstanceName() string
	ItemName() string
	SetItemID(int32)
}

type indexerResource interface {
	objectResource
	Item() commonv1alpha1.Indexer
}

type downloadClientResource interface {
	objectResource
	Item() commonv1alpha1.DownloadClient
}

type instanceLookup func(ctx context.Context, c client.Client, namespace string, ref commonv1alpha1.LocalObjectReference) (pvrInstance, error)

// ObjectKind is one kind of object resource, such as SonarrIndexer.
type ObjectKind[O objectResource] struct {
	app      App
	path     string
	newObj   func() O
	newList  func() client.ObjectList
	instance client.Object
	lookup   instanceLookup
	options  func(ctx context.Context, c client.Client, apps appref.Apps, obj O) (Options, error)
	// appRefKinds are the kinds an object's appRef may name.
	appRefKinds []string
}

// The object kinds of each PVR app.
var (
	SonarrIndexers         = indexerKind[pvrv1alpha1.SonarrIndexer, pvrv1alpha1.SonarrIndexerList](Sonarr, &pvrv1alpha1.Sonarr{}, sonarrInstance)
	RadarrIndexers         = indexerKind[pvrv1alpha1.RadarrIndexer, pvrv1alpha1.RadarrIndexerList](Radarr, &pvrv1alpha1.Radarr{}, radarrInstance)
	LidarrIndexers         = indexerKind[pvrv1alpha1.LidarrIndexer, pvrv1alpha1.LidarrIndexerList](Lidarr, &pvrv1alpha1.Lidarr{}, lidarrInstance)
	ReadarrIndexers        = indexerKind[pvrv1alpha1.ReadarrIndexer, pvrv1alpha1.ReadarrIndexerList](Readarr, &pvrv1alpha1.Readarr{}, readarrInstance)
	SonarrDownloadClients  = downloadClientKind[pvrv1alpha1.SonarrDownloadClient, pvrv1alpha1.SonarrDownloadClientList](Sonarr, &pvrv1alpha1.Sonarr{}, sonarrInstance)
	RadarrDownloadClients  = downloadClientKind[pvrv1alpha1.RadarrDownloadClient, pvrv1alpha1.RadarrDownloadClientList](Radarr, &pvrv1alpha1.Radarr{}, radarrInstance)
	LidarrDownloadClients  = downloadClientKind[pvrv1alpha1.LidarrDownloadClient, pvrv1alpha1.LidarrDownloadClientList](Lidarr, &pvrv1alpha1.Lidarr{}, lidarrInstance)
	ReadarrDownloadClients = downloadClientKind[pvrv1alpha1.ReadarrDownloadClient, pvrv1alpha1.ReadarrDownloadClientList](Readarr, &pvrv1alpha1.Readarr{}, readarrInstance)
)

func indexerKind[T, L any, O interface {
	*T
	indexerResource
}, PL interface {
	*L
	client.ObjectList
}](app App, instance client.Object, lookup instanceLookup) ObjectKind[O] {
	return ObjectKind[O]{
		app: app, path: "/indexer", instance: instance, lookup: lookup,
		newObj:  func() O { return O(new(T)) },
		newList: func() client.ObjectList { return PL(new(L)) },
		options: func(ctx context.Context, c client.Client, _ appref.Apps, obj O) (Options, error) {
			indexers, err := secretref.ResolveItemFields(ctx, c, obj.GetNamespace(), "indexer", []commonv1alpha1.Indexer{obj.Item()},
				func(i *commonv1alpha1.Indexer) (string, *[]commonv1alpha1.ConfigField) { return i.Name, &i.Fields })
			return Options{Indexers: indexers}, err
		},
	}
}

func downloadClientKind[T, L any, O interface {
	*T
	downloadClientResource
}, PL interface {
	*L
	client.ObjectList
}](app App, instance client.Object, lookup instanceLookup) ObjectKind[O] {
	return ObjectKind[O]{
		app: app, path: "/downloadclient", instance: instance, lookup: lookup,
		newObj:      func() O { return O(new(T)) },
		newList:     func() client.ObjectList { return PL(new(L)) },
		appRefKinds: []string{"QBittorrent", "Sabnzbd"},
		options: func(ctx context.Context, c client.Client, apps appref.Apps, obj O) (Options, error) {
			dc, err := resolveDownloadClient(ctx, c, apps, obj.GetNamespace(), obj.Item())
			if err != nil {
				return Options{}, err
			}
			dcs := []commonv1alpha1.DownloadClient{dc}
			secrets, err := resolveDownloadClientSecrets(ctx, c, obj.GetNamespace(), dcs)
			return Options{DownloadClients: dcs, DCSecrets: secrets}, err
		},
	}
}

// ObjectReconciler reconciles one object kind.
type ObjectReconciler[O objectResource] struct {
	client.Client
	Recorder events.EventRecorder
	Apps     appref.Apps
	kind     ObjectKind[O]
}

// NewObjectReconciler returns the reconciler for an object kind.
func NewObjectReconciler[O objectResource](kind ObjectKind[O], c client.Client, recorder events.EventRecorder, apps appref.Apps) *ObjectReconciler[O] {
	return &ObjectReconciler[O]{Client: c, Recorder: recorder, Apps: apps, kind: kind}
}

func (r *ObjectReconciler[O]) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.integration().Reconcile(ctx, r.Client, r.Recorder, req)
}

func (r *ObjectReconciler[O]) SetupWithManager(mgr ctrl.Manager) error {
	return r.integration().SetupWithManager(mgr, r.Apps, r)
}

// integration reconciles an object through the instance it names, whose drift
// policy and interval govern it. It serves one pass, and reads the instance
// once, so the policy and the connection come from the same version of it.
func (r *ObjectReconciler[O]) integration() reconciler.Integration[O, objectConn] {
	k := r.kind
	def := Definition(k.app)
	var read struct {
		sync.Once
		instance pvrInstance
		err      error
	}
	lookup := func(ctx context.Context, obj O) (pvrInstance, error) {
		read.Do(func() { read.instance, read.err = r.instance(ctx, obj) })
		return read.instance, read.err
	}
	in := reconciler.Integration[O, objectConn]{
		App:     k.app.Name,
		Name:    strings.ToLower(k.app.Kind) + strings.TrimPrefix(k.path, "/"),
		New:     k.newObj,
		NewList: k.newList,
		Watches: func(mgr ctrl.Manager, b *builder.Builder) (*builder.Builder, error) {
			err := mgr.GetFieldIndexer().IndexField(context.Background(), k.newObj(), instanceRefIndex,
				func(o client.Object) []string { return []string{o.(O).InstanceName()} })
			if err != nil {
				return nil, err
			}
			return b.Watches(k.instance, handler.EnqueueRequestsFromMapFunc(objectsOf(r.Client, k.newList())),
				builder.WithPredicates(instanceChanged())), nil
		},
		Governor: func(ctx context.Context, obj O) (*commonv1alpha1.ReconcileConfig, error) {
			instance, err := lookup(ctx, obj)
			if apierrors.IsNotFound(err) {
				return nil, nil
			}
			return instance.policy, err
		},
		Connect: func(ctx context.Context, obj O) (objectConn, error) {
			instance, err := lookup(ctx, obj)
			if apierrors.IsNotFound(err) {
				return objectConn{orphan: obj}, nil
			}
			if err != nil {
				return objectConn{}, err
			}
			hc, err := connectPVR(ctx, r.Client, obj.GetNamespace(), instance.connection, k.app)
			return objectConn{hc: hc}, err
		},
		Check: func(ctx context.Context, conn objectConn, obj O) error {
			if conn.orphan != nil {
				return failure.ReferenceNotFound.Errorf("%s %q not found", k.app.Kind, obj.InstanceName())
			}
			claimant, err := olderClaim(ctx, r.Client, k.newList(), obj)
			if err != nil {
				return failure.Transient.Errorf("listing %s resources: %w", k.app.Kind, err)
			}
			if claimant != "" {
				return failure.NameConflict.Errorf("%q already configures %q in %s %q", claimant, obj.ItemName(), k.app.Kind, obj.InstanceName())
			}
			return pingPVR(ctx, conn.hc, k.app)
		},
		Sync: func(ctx context.Context, conn objectConn, obj O, pass *engine.Pass) []error {
			opts, err := k.options(ctx, r.Client, r.Apps, obj)
			if err != nil {
				return []error{err}
			}
			opts.App = k.app
			problems := Reconcile(ctx, conn.hc, nil, opts, pass)
			if id, ok := idOf(ctx, conn.hc, "/api/"+k.app.APIVersion+k.path, obj.ItemName()); ok {
				obj.SetItemID(id)
			}
			return problems
		},
		Cleanup: func(ctx context.Context, conn objectConn, owned *engine.Ownership) error {
			if conn.orphan != nil {
				if r.Recorder != nil {
					r.Recorder.Eventf(conn.orphan, nil, corev1.EventTypeWarning, reconciler.ReasonInstanceMissing, "Delete",
						"%s %q is gone, so nothing was removed from it", k.app.Kind, conn.orphan.(O).InstanceName())
				}
				return nil
			}
			return engine.DeleteManagedResources(ctx, conn.hc, def, owned)
		},
		Deletable: def.Deletable,
	}
	if len(k.appRefKinds) > 0 {
		in.AppRefs, in.RefKinds = downloadClientAppRefs, k.appRefKinds
	}
	return in
}

// instance reads the instance obj names. A failure other than its absence is
// Transient.
func (r *ObjectReconciler[O]) instance(ctx context.Context, obj O) (pvrInstance, error) {
	instance, err := r.kind.lookup(ctx, r.Client, obj.GetNamespace(), commonv1alpha1.LocalObjectReference{Name: obj.InstanceName()})
	if err != nil && !apierrors.IsNotFound(err) {
		return pvrInstance{}, failure.Transient.Errorf("reading %s %q: %w", r.kind.app.Kind, obj.InstanceName(), err)
	}
	return instance, err
}

// downloadClientAppRefs returns the app a download client's appRef names.
func downloadClientAppRefs(o client.Object) []commonv1alpha1.AppRef {
	dc, ok := o.(downloadClientResource)
	if !ok || dc.Item().AppRef == (commonv1alpha1.AppRef{}) {
		return nil
	}
	return []commonv1alpha1.AppRef{dc.Item().AppRef}
}

// resolveDownloadClient returns dc with its appRef, if any, replaced by the
// referenced download client's kind, address and credentials.
func resolveDownloadClient(
	ctx context.Context, c client.Reader, apps appref.Apps, namespace string, dc commonv1alpha1.DownloadClient,
) (commonv1alpha1.DownloadClient, error) {
	if dc.AppRef == (commonv1alpha1.AppRef{}) {
		return dc, nil
	}
	app, addr, err := appref.ResolveAddress(ctx, c, apps, namespace, dc.AppRef)
	if err != nil {
		return dc, fmt.Errorf("download client %q appRef: %w", dc.Name, err)
	}
	dc.Implementation, dc.Host, dc.Port, dc.UseSSL, dc.URLBase = app.Kind, addr.Host, addr.Port, &addr.TLS, addr.Base
	switch app.Kind {
	case "QBittorrent":
		dc.Protocol = "torrent"
		dc.UsernameSecretRef, dc.PasswordSecretRef = *app.UsernameSecretRef, *app.PasswordSecretRef
	case "Sabnzbd":
		dc.Protocol = "usenet"
		dc.APIKeySecretRef = *app.APIKeySecretRef
	default:
		return dc, failure.InvalidConfig.Errorf("download client %q appRef: %s isn't a download client", dc.Name, app.Kind)
	}
	return dc, nil
}

// resolveDownloadClientSecrets resolves all secret references for a list of download clients.
// Returns a map keyed by download client name.
func resolveDownloadClientSecrets(ctx context.Context, c client.Reader, namespace string, downloadClients []commonv1alpha1.DownloadClient) (map[string]DownloadClientResolvedSecrets, error) {
	resolved := make(map[string]DownloadClientResolvedSecrets, len(downloadClients))
	for _, dc := range downloadClients {
		var s DownloadClientResolvedSecrets
		if dc.UsernameSecretRef != (commonv1alpha1.SecretKeyRef{}) {
			val, err := secrets.ResolveSecretKeyRef(ctx, c, namespace, dc.UsernameSecretRef)
			if err != nil {
				return nil, fmt.Errorf("download client %q username: %w", dc.Name, err)
			}
			s.Username = val
		}
		if dc.PasswordSecretRef != (commonv1alpha1.SecretKeyRef{}) {
			val, err := secrets.ResolveSecretKeyRef(ctx, c, namespace, dc.PasswordSecretRef)
			if err != nil {
				return nil, fmt.Errorf("download client %q password: %w", dc.Name, err)
			}
			s.Password = val
		}
		if dc.APIKeySecretRef != (commonv1alpha1.SecretKeyRef{}) {
			val, err := secrets.ResolveSecretKeyRef(ctx, c, namespace, dc.APIKeySecretRef)
			if err != nil {
				return nil, fmt.Errorf("download client %q apiKey: %w", dc.Name, err)
			}
			s.APIKey = val
		}
		resolved[dc.Name] = s
	}
	return resolved, nil
}
