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

type LidarrReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder events.EventRecorder
	Apps     ctrlcommon.Apps
}

// +kubebuilder:rbac:groups=arrchitect.dev,resources=lidarrs,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=arrchitect.dev,resources=lidarrs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=arrchitect.dev,resources=lidarrs/finalizers,verbs=update
// +kubebuilder:rbac:groups=arrchitect.dev,resources=jellyfins;plexes,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

func (r *LidarrReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var config pvrv1alpha1.Lidarr
	if err := r.Get(ctx, req.NamespacedName, &config); err != nil {
		return ctrl.Result{}, ctrlcommon.IgnoreDeleted(&config, req.NamespacedName, err)
	}
	s := config.Spec
	return reconcilePVR(ctx, r.Client, r.Recorder, pvrConfig{
		app: pvrclient.Lidarr, apps: r.Apps, obj: &config, spec: s, connection: s.Connection, policy: s.Reconcile,
		options: pvrclient.Options{
			RootFolders:            s.RootFolders,
			RemotePathMappings:     s.RemotePathMappings,
			QualityProfiles:        s.QualityProfiles,
			CustomFormats:          s.CustomFormats,
			Tags:                   s.Tags,
			Notifications:          s.Notifications,
			ImportLists:            s.ImportLists,
			QualityDefinitions:     s.QualityDefinitions,
			DelayProfiles:          s.DelayProfiles,
			LidarrMetadataProfiles: s.MetadataProfiles,
		},
	})
}

func (r *LidarrReconciler) SetupWithManager(mgr ctrl.Manager) error {
	b := ctrl.NewControllerManagedBy(mgr).
		For(&pvrv1alpha1.Lidarr{}, builder.WithPredicates(ctrlcommon.ConfigChangedPredicate())).
		WatchesMetadata(&corev1.Secret{}, ctrlcommon.EnqueueForSecretThroughRefs(r.Client, &pvrv1alpha1.LidarrList{}, lidarrAppRefs))
	b, err := ctrlcommon.WatchAppRefs(mgr, b, r.Apps, &pvrv1alpha1.Lidarr{}, &pvrv1alpha1.LidarrList{}, lidarrAppRefs, "Jellyfin", "Plex")
	if err != nil {
		return err
	}
	return b.Named("lidarr").Complete(r)
}

func lidarrAppRefs(o client.Object) []commonv1alpha1.AppRef {
	return notificationAppRefs(o.(*pvrv1alpha1.Lidarr).Spec.Notifications)
}

// lidarrInstance reads the Lidarr an object names.
func lidarrInstance(ctx context.Context, c client.Client, namespace string, ref commonv1alpha1.LocalObjectReference) (pvrInstance, error) {
	var lidarr pvrv1alpha1.Lidarr
	if err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: ref.Name}, &lidarr); err != nil {
		return pvrInstance{}, err
	}
	return pvrInstance{connection: lidarr.Spec.Connection, policy: lidarr.Spec.Reconcile}, nil
}
