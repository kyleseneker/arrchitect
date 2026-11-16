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

type LidarrIndexerReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder events.EventRecorder
}

// +kubebuilder:rbac:groups=arrchitect.dev,resources=lidarrindexers,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=arrchitect.dev,resources=lidarrindexers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=arrchitect.dev,resources=lidarrindexers/finalizers,verbs=update

func (r *LidarrIndexerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var obj pvrv1alpha1.LidarrIndexer
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
		path: "/indexer", name: s.Name,
		list: &pvrv1alpha1.LidarrIndexerList{},
		claim: func(o client.Object) (string, string) {
			other := o.(*pvrv1alpha1.LidarrIndexer)
			return other.Spec.LidarrRef.Name, other.Spec.Name
		},
		options: func(ctx context.Context) (pvrclient.Options, error) {
			indexers, err := ctrlcommon.ResolveIndexerSecrets(ctx, r.Client, obj.Namespace, []commonv1alpha1.Indexer{s.Indexer})
			return pvrclient.Options{Indexers: indexers}, err
		},
		setID: func(id int) { obj.Status.ID = id },
	})
}

func (r *LidarrIndexerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	err := mgr.GetFieldIndexer().IndexField(context.Background(), &pvrv1alpha1.LidarrIndexer{}, instanceRefIndex,
		func(o client.Object) []string { return []string{o.(*pvrv1alpha1.LidarrIndexer).Spec.LidarrRef.Name} })
	if err != nil {
		return err
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&pvrv1alpha1.LidarrIndexer{}, builder.WithPredicates(ctrlcommon.ConfigChangedPredicate())).
		Watches(&pvrv1alpha1.Lidarr{}, handler.EnqueueRequestsFromMapFunc(objectsOf(r.Client, &pvrv1alpha1.LidarrIndexerList{})),
			builder.WithPredicates(instanceChanged())).
		WatchesMetadata(&corev1.Secret{}, ctrlcommon.EnqueueForSecret(r.Client, &pvrv1alpha1.LidarrIndexerList{})).
		Named("lidarrindexer").
		Complete(r)
}
