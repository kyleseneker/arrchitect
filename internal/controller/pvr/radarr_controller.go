package pvr

import (
	"context"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	pvrv1alpha1 "github.com/kyleseneker/arrchitect/api/pvr/v1alpha1"
	"github.com/kyleseneker/arrchitect/internal/controller/appref"
	"github.com/kyleseneker/arrchitect/internal/controller/reconciler"
	"github.com/kyleseneker/arrchitect/internal/httpapi"
)

type RadarrReconciler struct {
	client.Client
	Recorder events.EventRecorder
	Apps     appref.Apps
}

// +kubebuilder:rbac:groups=arrchitect.dev,resources=radarrs,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=arrchitect.dev,resources=radarrs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=arrchitect.dev,resources=radarrs/finalizers,verbs=update
// +kubebuilder:rbac:groups=arrchitect.dev,resources=jellyfins;plexes,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

func (r *RadarrReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.integration().Reconcile(ctx, r.Client, r.Recorder, req)
}

func (r *RadarrReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return r.integration().SetupWithManager(mgr, r.Apps, r)
}

func (r *RadarrReconciler) integration() reconciler.Integration[*pvrv1alpha1.Radarr, *httpapi.HTTPClient] {
	in := pvrIntegration(r.Client, r.Apps, Radarr, func(config *pvrv1alpha1.Radarr) pvrConfig {
		s := config.Spec
		return pvrConfig{spec: s, connection: s.Connection, options: Options{
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
	in.New = func() *pvrv1alpha1.Radarr { return &pvrv1alpha1.Radarr{} }
	in.NewList = func() client.ObjectList { return &pvrv1alpha1.RadarrList{} }
	in.AppRefs, in.RefKinds = radarrAppRefs, []string{"Jellyfin", "Plex"}
	return in
}

func radarrAppRefs(o client.Object) []commonv1alpha1.AppRef {
	return notificationAppRefs(o.(*pvrv1alpha1.Radarr).Spec.Notifications)
}

// radarrInstance reads the Radarr an object names.
func radarrInstance(ctx context.Context, c client.Client, namespace string, ref commonv1alpha1.LocalObjectReference) (pvrInstance, error) {
	var radarr pvrv1alpha1.Radarr
	if err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: ref.Name}, &radarr); err != nil {
		return pvrInstance{}, err
	}
	return pvrInstance{connection: radarr.Spec.Connection, policy: radarr.Spec.Reconcile}, nil
}
