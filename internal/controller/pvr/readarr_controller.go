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

type ReadarrReconciler struct {
	client.Client
	Recorder events.EventRecorder
	Apps     appref.Apps
}

// +kubebuilder:rbac:groups=arrchitect.dev,resources=readarrs,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=arrchitect.dev,resources=readarrs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=arrchitect.dev,resources=readarrs/finalizers,verbs=update
// +kubebuilder:rbac:groups=arrchitect.dev,resources=plexes,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

func (r *ReadarrReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.integration().Reconcile(ctx, r.Client, r.Recorder, req)
}

func (r *ReadarrReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return r.integration().SetupWithManager(mgr, r.Apps, r)
}

func (r *ReadarrReconciler) integration() reconciler.Integration[*pvrv1alpha1.Readarr, *httpapi.HTTPClient] {
	in := pvrIntegration(r.Client, r.Apps, Readarr, func(config *pvrv1alpha1.Readarr) pvrConfig {
		s := config.Spec
		return pvrConfig{spec: s, connection: s.Connection, options: Options{
			RootFolders:             s.RootFolders,
			RemotePathMappings:      s.RemotePathMappings,
			QualityProfiles:         s.QualityProfiles,
			CustomFormats:           s.CustomFormats,
			Tags:                    s.Tags,
			Notifications:           s.Notifications,
			ImportLists:             s.ImportLists,
			QualityDefinitions:      s.QualityDefinitions,
			DelayProfiles:           s.DelayProfiles,
			ReadarrMetadataProfiles: s.MetadataProfiles,
		}}
	})
	in.New = func() *pvrv1alpha1.Readarr { return &pvrv1alpha1.Readarr{} }
	in.NewList = func() client.ObjectList { return &pvrv1alpha1.ReadarrList{} }
	in.AppRefs, in.RefKinds = readarrAppRefs, []string{"Plex"}
	return in
}

func readarrAppRefs(o client.Object) []commonv1alpha1.AppRef {
	return notificationAppRefs(o.(*pvrv1alpha1.Readarr).Spec.Notifications)
}

// readarrInstance reads the Readarr an object names.
func readarrInstance(ctx context.Context, c client.Client, namespace string, ref commonv1alpha1.LocalObjectReference) (pvrInstance, error) {
	var readarr pvrv1alpha1.Readarr
	if err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: ref.Name}, &readarr); err != nil {
		return pvrInstance{}, err
	}
	return pvrInstance{connection: readarr.Spec.Connection, policy: readarr.Spec.Reconcile}, nil
}
