package pvr

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	pvrv1alpha1 "github.com/kyleseneker/arrchitect/api/pvr/v1alpha1"
	pvrclient "github.com/kyleseneker/arrchitect/internal/client/pvr"
	ctrlcommon "github.com/kyleseneker/arrchitect/internal/controller/common"
)

type ReadarrDownloadClientReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder events.EventRecorder
}

// +kubebuilder:rbac:groups=arrchitect.dev,resources=readarrdownloadclients,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=arrchitect.dev,resources=readarrdownloadclients/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=arrchitect.dev,resources=readarrdownloadclients/finalizers,verbs=update

func (r *ReadarrDownloadClientReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var obj pvrv1alpha1.ReadarrDownloadClient
	if err := r.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, ctrlcommon.IgnoreDeleted(&obj, req.NamespacedName, err)
	}
	s := obj.Spec
	return reconcilePVRObject(ctx, r.Client, r.Recorder, pvrObject{
		app: pvrclient.Readarr, obj: &obj,
		instanceKind: "Readarr", instanceName: s.ReadarrRef.Name,
		instance: func(ctx context.Context) (pvrInstance, error) {
			return readarrInstance(ctx, r.Client, obj.Namespace, s.ReadarrRef)
		},
		path: "/downloadclient", name: s.Name,
		list: &pvrv1alpha1.ReadarrDownloadClientList{},
		claim: func(o client.Object) (string, string) {
			other := o.(*pvrv1alpha1.ReadarrDownloadClient)
			return other.Spec.ReadarrRef.Name, other.Spec.Name
		},
		options: func(ctx context.Context) (pvrclient.Options, error) {
			dcs := []commonv1alpha1.DownloadClient{s.DownloadClient}
			secrets, err := ctrlcommon.ResolveDownloadClientSecrets(ctx, r.Client, obj.Namespace, dcs)
			return pvrclient.Options{DownloadClients: dcs, DCSecrets: secrets}, err
		},
		setID: func(id int) { obj.Status.ID = id },
	})
}

func (r *ReadarrDownloadClientReconciler) SetupWithManager(mgr ctrl.Manager) error {
	err := mgr.GetFieldIndexer().IndexField(context.Background(), &pvrv1alpha1.ReadarrDownloadClient{}, instanceRefIndex,
		func(o client.Object) []string {
			return []string{o.(*pvrv1alpha1.ReadarrDownloadClient).Spec.ReadarrRef.Name}
		})
	if err != nil {
		return err
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&pvrv1alpha1.ReadarrDownloadClient{}, builder.WithPredicates(ctrlcommon.ConfigChangedPredicate())).
		Watches(&pvrv1alpha1.Readarr{}, handler.EnqueueRequestsFromMapFunc(objectsOf(r.Client, &pvrv1alpha1.ReadarrDownloadClientList{})),
			builder.WithPredicates(instanceChanged())).
		WatchesMetadata(&corev1.Secret{}, ctrlcommon.EnqueueForSecret(r.Client, &pvrv1alpha1.ReadarrDownloadClientList{})).
		Named("readarrdownloadclient").
		Complete(r)
}
