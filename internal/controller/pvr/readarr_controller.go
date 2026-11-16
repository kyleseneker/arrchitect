package pvr

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	pvrv1alpha1 "github.com/kyleseneker/arrchitect/api/pvr/v1alpha1"
	pvrclient "github.com/kyleseneker/arrchitect/internal/client/pvr"
	ctrlcommon "github.com/kyleseneker/arrchitect/internal/controller/common"
)

type ReadarrReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder events.EventRecorder
}

// +kubebuilder:rbac:groups=arrchitect.dev,resources=readarrs,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=arrchitect.dev,resources=readarrs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=arrchitect.dev,resources=readarrs/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

func (r *ReadarrReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var config pvrv1alpha1.Readarr
	if err := r.Get(ctx, req.NamespacedName, &config); err != nil {
		return ctrl.Result{}, ctrlcommon.IgnoreDeleted(&config, req.NamespacedName, err)
	}
	s := config.Spec
	return reconcilePVR(ctx, r.Client, r.Recorder, pvrConfig{
		app: pvrclient.Readarr, obj: &config, spec: s, connection: s.Connection, policy: s.Reconcile,
		options: pvrclient.Options{
			RootFolders:        s.RootFolders,
			RemotePathMappings: s.RemotePathMappings,
			QualityProfiles:    s.QualityProfiles,
			CustomFormats:      s.CustomFormats,
			Tags:               s.Tags,
			Notifications:      s.Notifications,
			ImportLists:        s.ImportLists,
			QualityDefinitions: s.QualityDefinitions,
			DelayProfiles:      s.DelayProfiles,
		},
	})
}

func (r *ReadarrReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&pvrv1alpha1.Readarr{}, builder.WithPredicates(ctrlcommon.ConfigChangedPredicate())).
		WatchesMetadata(&corev1.Secret{}, ctrlcommon.EnqueueForSecret(r.Client, &pvrv1alpha1.ReadarrList{})).
		Named("readarr").
		Complete(r)
}

// readarrInstance reads the Readarr an object names.
func readarrInstance(ctx context.Context, c client.Client, namespace string, ref commonv1alpha1.LocalObjectReference) (pvrInstance, error) {
	var readarr pvrv1alpha1.Readarr
	if err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: ref.Name}, &readarr); err != nil {
		return pvrInstance{}, err
	}
	return pvrInstance{connection: readarr.Spec.Connection, policy: readarr.Spec.Reconcile}, nil
}
