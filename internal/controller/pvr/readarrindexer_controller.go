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

type ReadarrIndexerReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder events.EventRecorder
}

// +kubebuilder:rbac:groups=arrchitect.dev,resources=readarrindexers,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=arrchitect.dev,resources=readarrindexers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=arrchitect.dev,resources=readarrindexers/finalizers,verbs=update

func (r *ReadarrIndexerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var obj pvrv1alpha1.ReadarrIndexer
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
		path: "/indexer", name: s.Name,
		list: &pvrv1alpha1.ReadarrIndexerList{},
		claim: func(o client.Object) (string, string) {
			other := o.(*pvrv1alpha1.ReadarrIndexer)
			return other.Spec.ReadarrRef.Name, other.Spec.Name
		},
		options: func(ctx context.Context) (pvrclient.Options, error) {
			indexers, err := ctrlcommon.ResolveIndexerSecrets(ctx, r.Client, obj.Namespace, []commonv1alpha1.Indexer{s.Indexer})
			return pvrclient.Options{Indexers: indexers}, err
		},
		setID: func(id int) { obj.Status.ID = id },
	})
}

func (r *ReadarrIndexerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	err := mgr.GetFieldIndexer().IndexField(context.Background(), &pvrv1alpha1.ReadarrIndexer{}, instanceRefIndex,
		func(o client.Object) []string { return []string{o.(*pvrv1alpha1.ReadarrIndexer).Spec.ReadarrRef.Name} })
	if err != nil {
		return err
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&pvrv1alpha1.ReadarrIndexer{}, builder.WithPredicates(ctrlcommon.ConfigChangedPredicate())).
		Watches(&pvrv1alpha1.Readarr{}, handler.EnqueueRequestsFromMapFunc(objectsOf(r.Client, &pvrv1alpha1.ReadarrIndexerList{})),
			builder.WithPredicates(instanceChanged())).
		WatchesMetadata(&corev1.Secret{}, ctrlcommon.EnqueueForSecret(r.Client, &pvrv1alpha1.ReadarrIndexerList{})).
		Named("readarrindexer").
		Complete(r)
}
