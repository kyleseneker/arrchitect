// Package pvr holds the Sonarr, Radarr, Lidarr and Readarr controllers, and
// those for the indexers and download clients defined apart from them.
package pvr

import (
	"context"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	pvrv1alpha1 "github.com/kyleseneker/arrchitect/api/pvr/v1alpha1"
	pvrclient "github.com/kyleseneker/arrchitect/internal/client/pvr"
	"github.com/kyleseneker/arrchitect/internal/controller/appref"
	"github.com/kyleseneker/arrchitect/internal/controller/reconciler"
	"github.com/kyleseneker/arrchitect/internal/controller/secretref"
)

type LidarrReconciler struct {
	client.Client
	Recorder events.EventRecorder
	Apps     appref.Apps
}

// +kubebuilder:rbac:groups=arrchitect.dev,resources=lidarrs,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=arrchitect.dev,resources=lidarrs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=arrchitect.dev,resources=lidarrs/finalizers,verbs=update
// +kubebuilder:rbac:groups=arrchitect.dev,resources=jellyfins;plexes,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

func (r *LidarrReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	integration := pvrIntegration(r.Client, r.Apps, pvrclient.Lidarr, func(config *pvrv1alpha1.Lidarr) pvrConfig {
		s := config.Spec
		return pvrConfig{spec: s, connection: s.Connection, options: pvrclient.Options{
			RootFolders:            s.RootFolders,
			RemotePathMappings:     s.RemotePathMappings,
			QualityProfiles:        s.QualityProfiles,
			CustomFormats:          s.CustomFormats,
			Tags:                   s.Tags,
			Notifications:          s.Notifications,
			ImportLists:            s.ImportLists,
			QualityDefinitions:     s.QualityDefinitions,
			MetadataConsumers:      s.MetadataConsumers,
			AutoTags:               s.AutoTags,
			DelayProfiles:          s.DelayProfiles,
			LidarrMetadataProfiles: s.MetadataProfiles,
		}}
	})
	return integration.Reconcile(ctx, r.Client, r.Recorder, req, &pvrv1alpha1.Lidarr{})
}

func (r *LidarrReconciler) SetupWithManager(mgr ctrl.Manager) error {
	b := ctrl.NewControllerManagedBy(mgr).
		For(&pvrv1alpha1.Lidarr{}, builder.WithPredicates(reconciler.ConfigChangedPredicate()))
	b, err := secretref.Watch(mgr, b, &pvrv1alpha1.Lidarr{}, &pvrv1alpha1.LidarrList{})
	if err != nil {
		return err
	}
	b, err = appref.Watch(mgr, b, r.Apps, &pvrv1alpha1.Lidarr{}, &pvrv1alpha1.LidarrList{}, lidarrAppRefs, "Jellyfin", "Plex")
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
