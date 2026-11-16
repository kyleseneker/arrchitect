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

type RadarrIndexerReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder events.EventRecorder
}

// +kubebuilder:rbac:groups=arrchitect.dev,resources=radarrindexers,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=arrchitect.dev,resources=radarrindexers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=arrchitect.dev,resources=radarrindexers/finalizers,verbs=update

func (r *RadarrIndexerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var obj pvrv1alpha1.RadarrIndexer
	if err := r.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, ctrlcommon.IgnoreDeleted(&obj, req.NamespacedName, err)
	}
	s := obj.Spec
	return reconcilePVRObject(ctx, r.Client, r.Recorder, pvrObject{
		app: pvrclient.Radarr, obj: &obj,
		instanceKind: "Radarr", instanceName: s.RadarrRef.Name,
		instance: func(ctx context.Context) (pvrInstance, error) {
			return radarrInstance(ctx, r.Client, obj.Namespace, s.RadarrRef)
		},
		path: "/indexer", name: s.Name,
		list: &pvrv1alpha1.RadarrIndexerList{},
		claim: func(o client.Object) (string, string) {
			other := o.(*pvrv1alpha1.RadarrIndexer)
			return other.Spec.RadarrRef.Name, other.Spec.Name
		},
		options: func(ctx context.Context) (pvrclient.Options, error) {
			indexers, err := ctrlcommon.ResolveIndexerSecrets(ctx, r.Client, obj.Namespace, []commonv1alpha1.Indexer{s.Indexer})
			return pvrclient.Options{Indexers: indexers}, err
		},
		setID: func(id int32) { obj.Status.ID = id },
	})
}

func (r *RadarrIndexerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	err := mgr.GetFieldIndexer().IndexField(context.Background(), &pvrv1alpha1.RadarrIndexer{}, instanceRefIndex,
		func(o client.Object) []string { return []string{o.(*pvrv1alpha1.RadarrIndexer).Spec.RadarrRef.Name} })
	if err != nil {
		return err
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&pvrv1alpha1.RadarrIndexer{}, builder.WithPredicates(ctrlcommon.ConfigChangedPredicate())).
		Watches(&pvrv1alpha1.Radarr{}, handler.EnqueueRequestsFromMapFunc(objectsOf(r.Client, &pvrv1alpha1.RadarrIndexerList{})),
			builder.WithPredicates(instanceChanged())).
		WatchesMetadata(&corev1.Secret{}, ctrlcommon.EnqueueForSecret(r.Client, &pvrv1alpha1.RadarrIndexerList{})).
		Named("radarrindexer").
		Complete(r)
}
