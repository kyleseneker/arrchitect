package pvr

import (
	"context"
	"fmt"
	"time"

	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	pvrclient "github.com/kyleseneker/arrchitect/internal/client/pvr"
	ctrlcommon "github.com/kyleseneker/arrchitect/internal/controller/common"
	"github.com/kyleseneker/arrchitect/internal/engine"
	"github.com/kyleseneker/arrchitect/internal/reconciler"
)

type pvrConfig struct {
	app        pvrclient.App
	apps       ctrlcommon.Apps
	obj        ctrlcommon.ConfigResource
	spec       any
	connection commonv1alpha1.AppConnection
	policy     *commonv1alpha1.ReconcileConfig
	options    pvrclient.Options
}

func reconcilePVR(ctx context.Context, c client.Client, recorder events.EventRecorder, cfg pvrConfig) (ctrl.Result, error) {
	obj, conn, namespace := cfg.obj, cfg.connection, cfg.obj.GetNamespace()

	cleanup := func(ctx context.Context) error {
		return ctrlcommon.CleanupApp(ctx, c, recorder, obj, conn, pvrclient.Definition(cfg.app.APIVersion), cfg.app.Name)
	}
	if done, after := ctrlcommon.HandleLifecycle(ctx, c, recorder, obj, ctrlcommon.Lifecycle{Remove: cleanup, Prune: true}); done {
		return ctrl.Result{RequeueAfter: after}, nil
	}

	opts := withApp(cfg.options, cfg.app)
	// A kind whose items name a Secret that can't be read is left out of the
	// pass, neither written nor pruned, and reported; the rest still syncs.
	hc, wait := connectPVR(ctx, c, obj, conn, cfg.app)
	if wait != nil {
		return *wait, nil
	}

	var unresolved []string
	var err error
	if opts.Notifications, err = ctrlcommon.ResolveNotifications(ctx, c, cfg.apps, namespace, opts.Notifications); err != nil {
		unresolved = append(unresolved, fmt.Sprintf("notifications: %v", err))
	} else if opts.Notifications, err = ctrlcommon.ResolveNotificationSecrets(ctx, c, namespace, opts.Notifications); err != nil {
		unresolved = append(unresolved, fmt.Sprintf("notifications: %v", err))
	}
	if opts.ImportLists, err = ctrlcommon.ResolveImportListSecrets(ctx, c, namespace, opts.ImportLists); err != nil {
		unresolved = append(unresolved, fmt.Sprintf("importLists: %v", err))
	}

	result, err := pvrclient.Reconcile(ctx, hc, cfg.app.APIVersion, cfg.spec, opts, ctrlcommon.Policy(cfg.policy), obj.GetManagedResources())
	if err != nil {
		ctrlcommon.UpdateStatus(ctx, c, obj, false, engine.ReasonSyncFailed, err.Error())
		return ctrl.Result{RequeueAfter: ctrlcommon.ReconcileInterval(cfg.policy)}, nil
	}
	result.Errors = append(unresolved, result.Errors...)
	ctrlcommon.UpdateStatusManaged(obj, result.Managed)
	ctrlcommon.EmitPruneEvents(recorder, obj, result.Pruned)
	ctrlcommon.UpdateResultStatus(ctx, c, obj, result)

	return ctrl.Result{RequeueAfter: ctrlcommon.ReconcileInterval(cfg.policy)}, nil
}

// notificationAppRefs returns the media servers notifications refer to.
func notificationAppRefs(notifications []commonv1alpha1.Notification) []commonv1alpha1.AppRef {
	var refs []commonv1alpha1.AppRef
	for _, n := range notifications {
		if n.AppRef != (commonv1alpha1.AppRef{}) {
			refs = append(refs, n.AppRef)
		}
	}
	return refs
}

// withApp fills in what the options take from the app itself.
func withApp(opts pvrclient.Options, app pvrclient.App) pvrclient.Options {
	opts.CategoryField = app.CategoryField
	opts.ImportListFields = app.ImportListFields
	opts.MetadataProfiles = app.MetadataProfiles
	opts.Choices = app.Choices
	opts.Sections = app.Sections
	return opts
}

// connectPVR resolves an app's connection and checks the app answers. When it
// can't, it records why on obj and returns when to try again.
func connectPVR(
	ctx context.Context, c client.Client, obj ctrlcommon.ConfigResource, conn commonv1alpha1.AppConnection, app pvrclient.App,
) (*engine.HTTPClient, *ctrl.Result) {
	namespace := obj.GetNamespace()
	missingSecret := func(err error) (*engine.HTTPClient, *ctrl.Result) {
		ctrlcommon.UpdateStatusUnreachable(ctx, c, obj, engine.ReasonSecretNotFound, err.Error())
		return nil, &ctrl.Result{RequeueAfter: 30 * time.Second}
	}
	apiKey, err := reconciler.ResolveSecretKeyRef(ctx, c, namespace, conn.APIKeySecretRef)
	if err != nil {
		return missingSecret(err)
	}
	tlsCfg, err := engine.ResolveTLSConfig(ctx, c, namespace, conn.TLS)
	if err != nil {
		return missingSecret(err)
	}
	hc, err := engine.NewHTTPClient(conn.URL, engine.AuthAPIKey,
		engine.WithOwner(namespace+"/"+obj.GetName()+"/"+string(obj.GetUID())),
		engine.WithAPIKey(apiKey), engine.WithTLSConfig(tlsCfg), engine.WithAppLabel(app.Name))
	if err != nil {
		ctrlcommon.UpdateStatusUnreachable(ctx, c, obj, engine.ReasonInvalidConfig, err.Error())
		return nil, &ctrl.Result{}
	}
	if err := hc.Ping(ctx, "/api/"+app.APIVersion+"/system/status"); err != nil {
		ctrlcommon.UpdateStatusUnreachable(ctx, c, obj, engine.ReasonAppUnreachable, err.Error())
		return nil, &ctrl.Result{RequeueAfter: 60 * time.Second}
	}
	return hc, nil
}
