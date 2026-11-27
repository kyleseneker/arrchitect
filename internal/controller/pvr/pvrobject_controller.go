package pvr

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"strings"

	"sigs.k8s.io/controller-runtime/pkg/builder"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	pvrv1alpha1 "github.com/kyleseneker/arrchitect/api/pvr/v1alpha1"
	pvrclient "github.com/kyleseneker/arrchitect/internal/client/pvr"
	ctrlcommon "github.com/kyleseneker/arrchitect/internal/controller/common"
	"github.com/kyleseneker/arrchitect/internal/engine"
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

// pvrObject is one object in an app, such as an indexer, reconciled through
// the instance it names.
type pvrObject struct {
	app pvrclient.App
	obj ctrlcommon.ConfigResource
	// instanceKind and instanceName name the instance for status messages.
	instanceKind, instanceName string
	instance                   func(context.Context) (pvrInstance, error)
	// path lists the app's objects of this kind, to find this one's ID by name.
	path, name string
	// list and claim find other resources of this kind naming the same object:
	// claim returns the instance and object a resource of this kind names.
	list  client.ObjectList
	claim func(client.Object) (instance, name string)
	// options returns the object as the engine applies it, with its Secrets
	// resolved.
	options func(context.Context) (pvrclient.Options, error)
	setID   func(int32)
}

func reconcilePVRObject(ctx context.Context, c client.Client, recorder events.EventRecorder, o pvrObject) (ctrl.Result, error) {
	obj := o.obj
	instance, err := o.instance(ctx)
	missing := apierrors.IsNotFound(err)
	if err != nil && !missing {
		return ctrl.Result{}, err
	}

	cleanup := func(ctx context.Context) error {
		if missing {
			recorder.Eventf(obj, nil, corev1.EventTypeWarning, ctrlcommon.ReasonInstanceMissing, "Delete",
				"%s %q is gone, so nothing was removed from it", o.instanceKind, o.instanceName)
			return nil
		}
		return ctrlcommon.CleanupApp(ctx, c, recorder, obj, instance.connection, pvrclient.Definition(o.app), o.app.Name)
	}
	if done, after := ctrlcommon.HandleLifecycle(ctx, c, recorder, obj, ctrlcommon.Lifecycle{Remove: cleanup, Observe: ctrlcommon.Policy(instance.policy).Observe}); done {
		return ctrl.Result{RequeueAfter: after}, nil
	}
	if missing {
		ctrlcommon.UpdateStatus(ctx, c, recorder, obj, false, ctrlcommon.ReasonReferenceNotFound,
			fmt.Sprintf("%s %q not found", o.instanceKind, o.instanceName))
		return ctrl.Result{RequeueAfter: ctrlcommon.RetryAfter(obj, 30*time.Second)}, nil
	}

	claimant, err := olderClaim(ctx, c, o)
	if err != nil {
		return ctrl.Result{}, err
	}
	if claimant != "" {
		ctrlcommon.UpdateStatus(ctx, c, recorder, obj, false, ctrlcommon.ReasonNameConflict,
			fmt.Sprintf("%q already configures %q in %s %q", claimant, o.name, o.instanceKind, o.instanceName))
		return ctrl.Result{RequeueAfter: ctrlcommon.RetryAfter(obj, 30*time.Second)}, nil
	}

	hc, err := connectPVR(ctx, c, obj, instance.connection, o.app)
	if err == nil {
		err = pingPVR(ctx, hc, o.app)
	}
	if err != nil {
		reason, base := ctrlcommon.Classify(err)
		after := ctrlcommon.RetryAfter(obj, base)
		log.FromContext(ctx).Info("Couldn't reach "+o.instanceKind, "reason", reason, "error", err.Error(), "retryAfter", after)
		ctrlcommon.UpdateStatus(ctx, c, recorder, obj, false, reason, err.Error())
		return ctrl.Result{RequeueAfter: after}, nil
	}
	opts, err := o.options(ctx)
	if err != nil {
		reason, base := ctrlcommon.AppRefFailure(err)
		ctrlcommon.UpdateStatus(ctx, c, recorder, obj, false, reason, err.Error())
		return ctrl.Result{RequeueAfter: ctrlcommon.RetryAfter(obj, base)}, nil
	}
	ctrlcommon.Reached(obj)
	opts.App = o.app

	policy := ctrlcommon.Policy(instance.policy)
	// The object owns only the item it names, so prune replaces it on a rename.
	policy.Prune = true
	pass := engine.NewPass(o.app.Name, ctrlcommon.OwnerKey(obj), policy, obj.GetConfigStatus().GetManagedResources())
	problems := pvrclient.Reconcile(ctx, hc, nil, opts, pass)
	if id, ok := idOf(ctx, hc, "/api/"+o.app.APIVersion+o.path, o.name); ok {
		o.setID(id)
	}
	ctrlcommon.UpdateStatusManaged(obj, pass.Owned.Managed())
	ctrlcommon.EmitPruneEvents(recorder, obj, pass.Owned.Pruned())
	retry := ctrlcommon.ReportResult(ctx, c, recorder, obj, policy.Observe, ctrlcommon.Result{
		Synced: pass.SyncedItems(), Errors: problems, Drift: pass.Drift(),
	})
	after := ctrlcommon.NextSync(instance.policy)
	if retry > 0 && retry < after {
		after = retry
	}
	return ctrl.Result{RequeueAfter: after}, nil
}

// olderClaim names an older resource of the same kind that configures the same
// object in the same instance. That one keeps the object, so two resources
// never take turns rewriting it or delete it from under each other.
func olderClaim(ctx context.Context, c client.Client, o pvrObject) (string, error) {
	list := o.list.DeepCopyObject().(client.ObjectList)
	if err := c.List(ctx, list, client.InNamespace(o.obj.GetNamespace())); err != nil {
		return "", err
	}
	items, err := apimeta.ExtractList(list)
	if err != nil {
		return "", err
	}
	var claimant client.Object
	for _, item := range items {
		other, ok := item.(client.Object)
		if !ok || other.GetName() == o.obj.GetName() || !older(other, o.obj) {
			continue
		}
		if instance, name := o.claim(other); instance != o.instanceName || name != o.name {
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
			return nil
		}
		items, err := apimeta.ExtractList(objects)
		if err != nil {
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
	cr, ok := obj.(ctrlcommon.ConfigResource)
	if !ok {
		return ""
	}
	if cond := apimeta.FindStatusCondition(cr.GetConfigStatus().Conditions, ctrlcommon.ConditionSynced); cond != nil {
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
	ctrlcommon.ConfigResource
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
	app      pvrclient.App
	path     string
	newObj   func() O
	newList  func() client.ObjectList
	instance client.Object
	lookup   instanceLookup
	options  func(ctx context.Context, c client.Client, apps ctrlcommon.Apps, obj O) (pvrclient.Options, error)
	// appRefKinds are the kinds an object's appRef may name.
	appRefKinds []string
}

// The object kinds of each PVR app.
var (
	SonarrIndexers         = indexerKind[pvrv1alpha1.SonarrIndexer, pvrv1alpha1.SonarrIndexerList](pvrclient.Sonarr, &pvrv1alpha1.Sonarr{}, sonarrInstance)
	RadarrIndexers         = indexerKind[pvrv1alpha1.RadarrIndexer, pvrv1alpha1.RadarrIndexerList](pvrclient.Radarr, &pvrv1alpha1.Radarr{}, radarrInstance)
	LidarrIndexers         = indexerKind[pvrv1alpha1.LidarrIndexer, pvrv1alpha1.LidarrIndexerList](pvrclient.Lidarr, &pvrv1alpha1.Lidarr{}, lidarrInstance)
	ReadarrIndexers        = indexerKind[pvrv1alpha1.ReadarrIndexer, pvrv1alpha1.ReadarrIndexerList](pvrclient.Readarr, &pvrv1alpha1.Readarr{}, readarrInstance)
	SonarrDownloadClients  = downloadClientKind[pvrv1alpha1.SonarrDownloadClient, pvrv1alpha1.SonarrDownloadClientList](pvrclient.Sonarr, &pvrv1alpha1.Sonarr{}, sonarrInstance)
	RadarrDownloadClients  = downloadClientKind[pvrv1alpha1.RadarrDownloadClient, pvrv1alpha1.RadarrDownloadClientList](pvrclient.Radarr, &pvrv1alpha1.Radarr{}, radarrInstance)
	LidarrDownloadClients  = downloadClientKind[pvrv1alpha1.LidarrDownloadClient, pvrv1alpha1.LidarrDownloadClientList](pvrclient.Lidarr, &pvrv1alpha1.Lidarr{}, lidarrInstance)
	ReadarrDownloadClients = downloadClientKind[pvrv1alpha1.ReadarrDownloadClient, pvrv1alpha1.ReadarrDownloadClientList](pvrclient.Readarr, &pvrv1alpha1.Readarr{}, readarrInstance)
)

func indexerKind[T, L any, O interface {
	*T
	indexerResource
}, PL interface {
	*L
	client.ObjectList
}](app pvrclient.App, instance client.Object, lookup instanceLookup) ObjectKind[O] {
	return ObjectKind[O]{
		app: app, path: "/indexer", instance: instance, lookup: lookup,
		newObj:  func() O { return O(new(T)) },
		newList: func() client.ObjectList { return PL(new(L)) },
		options: func(ctx context.Context, c client.Client, _ ctrlcommon.Apps, obj O) (pvrclient.Options, error) {
			indexers, err := ctrlcommon.ResolveIndexerSecrets(ctx, c, obj.GetNamespace(), []commonv1alpha1.Indexer{obj.Item()})
			return pvrclient.Options{Indexers: indexers}, err
		},
	}
}

func downloadClientKind[T, L any, O interface {
	*T
	downloadClientResource
}, PL interface {
	*L
	client.ObjectList
}](app pvrclient.App, instance client.Object, lookup instanceLookup) ObjectKind[O] {
	return ObjectKind[O]{
		app: app, path: "/downloadclient", instance: instance, lookup: lookup,
		newObj:      func() O { return O(new(T)) },
		newList:     func() client.ObjectList { return PL(new(L)) },
		appRefKinds: []string{"QBittorrent", "Sabnzbd"},
		options: func(ctx context.Context, c client.Client, apps ctrlcommon.Apps, obj O) (pvrclient.Options, error) {
			dc, err := ctrlcommon.ResolveDownloadClient(ctx, c, apps, obj.GetNamespace(), obj.Item())
			if err != nil {
				return pvrclient.Options{}, err
			}
			dcs := []commonv1alpha1.DownloadClient{dc}
			secrets, err := resolveDownloadClientSecrets(ctx, c, obj.GetNamespace(), dcs)
			return pvrclient.Options{DownloadClients: dcs, DCSecrets: secrets}, err
		},
	}
}

// ObjectReconciler reconciles one object kind.
type ObjectReconciler[O objectResource] struct {
	client.Client
	Recorder events.EventRecorder
	Apps     ctrlcommon.Apps
	kind     ObjectKind[O]
}

// NewObjectReconciler returns the reconciler for an object kind.
func NewObjectReconciler[O objectResource](kind ObjectKind[O], c client.Client, recorder events.EventRecorder, apps ctrlcommon.Apps) *ObjectReconciler[O] {
	return &ObjectReconciler[O]{Client: c, Recorder: recorder, Apps: apps, kind: kind}
}

func (r *ObjectReconciler[O]) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	obj := r.kind.newObj()
	if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
		return ctrl.Result{}, ctrlcommon.IgnoreDeleted(obj, req.NamespacedName, err)
	}
	ref := commonv1alpha1.LocalObjectReference{Name: obj.InstanceName()}
	return reconcilePVRObject(ctx, r.Client, r.Recorder, pvrObject{
		app: r.kind.app, obj: obj,
		instanceKind: r.kind.app.Kind, instanceName: ref.Name,
		instance: func(ctx context.Context) (pvrInstance, error) {
			return r.kind.lookup(ctx, r.Client, obj.GetNamespace(), ref)
		},
		path: r.kind.path, name: obj.ItemName(),
		list: r.kind.newList(),
		claim: func(o client.Object) (string, string) {
			other := o.(O)
			return other.InstanceName(), other.ItemName()
		},
		options: func(ctx context.Context) (pvrclient.Options, error) {
			return r.kind.options(ctx, r.Client, r.Apps, obj)
		},
		setID: obj.SetItemID,
	})
}

func (r *ObjectReconciler[O]) SetupWithManager(mgr ctrl.Manager) error {
	k := r.kind
	err := mgr.GetFieldIndexer().IndexField(context.Background(), k.newObj(), instanceRefIndex,
		func(o client.Object) []string { return []string{o.(O).InstanceName()} })
	if err != nil {
		return err
	}
	b := ctrl.NewControllerManagedBy(mgr).
		For(k.newObj(), builder.WithPredicates(ctrlcommon.ConfigChangedPredicate())).
		Watches(k.instance, handler.EnqueueRequestsFromMapFunc(objectsOf(r.Client, k.newList())), builder.WithPredicates(instanceChanged()))
	if len(k.appRefKinds) == 0 {
		b = b.WatchesMetadata(&corev1.Secret{}, ctrlcommon.EnqueueForSecret(r.Client, k.newList()))
	} else {
		b = b.WatchesMetadata(&corev1.Secret{}, ctrlcommon.EnqueueForSecretThroughRefs(r.Client, r.Apps, k.newList(), downloadClientAppRefs))
		if b, err = ctrlcommon.WatchAppRefs(mgr, b, r.Apps, k.newObj(), k.newList(), downloadClientAppRefs, k.appRefKinds...); err != nil {
			return err
		}
	}
	return b.Named(strings.ToLower(k.app.Kind) + strings.TrimPrefix(k.path, "/")).Complete(r)
}

// downloadClientAppRefs returns the app a download client's appRef names.
func downloadClientAppRefs(o client.Object) []commonv1alpha1.AppRef {
	dc, ok := o.(downloadClientResource)
	if !ok || dc.Item().AppRef == (commonv1alpha1.AppRef{}) {
		return nil
	}
	return []commonv1alpha1.AppRef{dc.Item().AppRef}
}

// resolveDownloadClientSecrets resolves all secret references for a list of download clients.
// Returns a map keyed by download client name.
func resolveDownloadClientSecrets(ctx context.Context, c client.Reader, namespace string, downloadClients []commonv1alpha1.DownloadClient) (map[string]pvrclient.DownloadClientResolvedSecrets, error) {
	resolved := make(map[string]pvrclient.DownloadClientResolvedSecrets, len(downloadClients))
	for _, dc := range downloadClients {
		var s pvrclient.DownloadClientResolvedSecrets
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
