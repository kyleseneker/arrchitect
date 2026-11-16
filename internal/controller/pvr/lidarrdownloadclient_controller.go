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

type LidarrDownloadClientReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder events.EventRecorder
}

// +kubebuilder:rbac:groups=arrchitect.dev,resources=lidarrdownloadclients,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=arrchitect.dev,resources=lidarrdownloadclients/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=arrchitect.dev,resources=lidarrdownloadclients/finalizers,verbs=update

func (r *LidarrDownloadClientReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var obj pvrv1alpha1.LidarrDownloadClient
	if err := r.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, ctrlcommon.IgnoreDeleted(&obj, req.NamespacedName, err)
	}
	s := obj.Spec
	return reconcilePVRObject(ctx, r.Client, r.Recorder, pvrObject{
		app: pvrclient.Lidarr, obj: &obj,
		instanceKind: "Lidarr", instanceName: s.LidarrRef.Name,
		instance: func(ctx context.Context) (pvrInstance, error) {
			return lidarrInstance(ctx, r.Client, obj.Namespace, s.LidarrRef)
		},
		path: "/downloadclient", name: s.Name,
		list: &pvrv1alpha1.LidarrDownloadClientList{},
		claim: func(o client.Object) (string, string) {
			other := o.(*pvrv1alpha1.LidarrDownloadClient)
			return other.Spec.LidarrRef.Name, other.Spec.Name
		},
		options: func(ctx context.Context) (pvrclient.Options, error) {
			dcs := []commonv1alpha1.DownloadClient{s.DownloadClient}
			secrets, err := ctrlcommon.ResolveDownloadClientSecrets(ctx, r.Client, obj.Namespace, dcs)
			return pvrclient.Options{DownloadClients: dcs, DCSecrets: secrets}, err
		},
		setID: func(id int) { obj.Status.ID = id },
	})
}

func (r *LidarrDownloadClientReconciler) SetupWithManager(mgr ctrl.Manager) error {
	err := mgr.GetFieldIndexer().IndexField(context.Background(), &pvrv1alpha1.LidarrDownloadClient{}, instanceRefIndex,
		func(o client.Object) []string {
			return []string{o.(*pvrv1alpha1.LidarrDownloadClient).Spec.LidarrRef.Name}
		})
	if err != nil {
		return err
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&pvrv1alpha1.LidarrDownloadClient{}, builder.WithPredicates(ctrlcommon.ConfigChangedPredicate())).
		Watches(&pvrv1alpha1.Lidarr{}, handler.EnqueueRequestsFromMapFunc(objectsOf(r.Client, &pvrv1alpha1.LidarrDownloadClientList{})),
			builder.WithPredicates(instanceChanged())).
		WatchesMetadata(&corev1.Secret{}, ctrlcommon.EnqueueForSecret(r.Client, &pvrv1alpha1.LidarrDownloadClientList{})).
		Named("lidarrdownloadclient").
		Complete(r)
}
