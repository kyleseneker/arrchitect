package pvr

import (
	"context"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	pvrv1alpha1 "github.com/kyleseneker/arrchitect/api/pvr/v1alpha1"
	pvrclient "github.com/kyleseneker/arrchitect/internal/client/pvr"
	"github.com/kyleseneker/arrchitect/internal/controller/appref"
	"github.com/kyleseneker/arrchitect/internal/controller/reconciler"
	"github.com/kyleseneker/arrchitect/internal/httpapi"
)

type SonarrReconciler struct {
	client.Client
	Recorder events.EventRecorder
	Apps     appref.Apps
}

// +kubebuilder:rbac:groups=arrchitect.dev,resources=sonarrs,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=arrchitect.dev,resources=sonarrs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=arrchitect.dev,resources=sonarrs/finalizers,verbs=update
// +kubebuilder:rbac:groups=arrchitect.dev,resources=jellyfins;plexes,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

func (r *SonarrReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.integration().Reconcile(ctx, r.Client, r.Recorder, req)
}

func (r *SonarrReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return r.integration().SetupWithManager(mgr, r.Apps, r)
}

func (r *SonarrReconciler) integration() reconciler.Integration[*pvrv1alpha1.Sonarr, *httpapi.HTTPClient] {
	in := pvrIntegration(r.Client, r.Apps, pvrclient.Sonarr, func(config *pvrv1alpha1.Sonarr) pvrConfig {
		s := config.Spec
		return pvrConfig{spec: s, connection: s.Connection, options: pvrclient.Options{
			RootFolders:        s.RootFolders,
			RemotePathMappings: s.RemotePathMappings,
			QualityProfiles:    s.QualityProfiles,
			CustomFormats:      s.CustomFormats,
			Tags:               s.Tags,
			Notifications:      s.Notifications,
			ImportLists:        s.ImportLists,
			QualityDefinitions: s.QualityDefinitions,
			MetadataConsumers:  s.MetadataConsumers,
			AutoTags:           s.AutoTags,
			DelayProfiles:      s.DelayProfiles,
			ReleaseProfiles:    s.ReleaseProfiles,
		}}
	})
	in.New = func() *pvrv1alpha1.Sonarr { return &pvrv1alpha1.Sonarr{} }
	in.NewList = func() client.ObjectList { return &pvrv1alpha1.SonarrList{} }
	in.AppRefs, in.RefKinds = sonarrAppRefs, []string{"Jellyfin", "Plex"}
	return in
}

func sonarrAppRefs(o client.Object) []commonv1alpha1.AppRef {
	return notificationAppRefs(o.(*pvrv1alpha1.Sonarr).Spec.Notifications)
}

// sonarrInstance reads the Sonarr an object names.
func sonarrInstance(ctx context.Context, c client.Client, namespace string, ref commonv1alpha1.LocalObjectReference) (pvrInstance, error) {
	var sonarr pvrv1alpha1.Sonarr
	if err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: ref.Name}, &sonarr); err != nil {
		return pvrInstance{}, err
	}
	return pvrInstance{connection: sonarr.Spec.Connection, policy: sonarr.Spec.Reconcile}, nil
}
