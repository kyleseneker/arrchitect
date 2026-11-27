package pvr

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	pvrclient "github.com/kyleseneker/arrchitect/internal/client/pvr"
	ctrlcommon "github.com/kyleseneker/arrchitect/internal/controller/common"
	"github.com/kyleseneker/arrchitect/internal/engine"
	"github.com/kyleseneker/arrchitect/internal/reconciler"
)

// pvrConfig is what the PVR pipeline takes from an app's resource.
type pvrConfig struct {
	spec       any
	connection commonv1alpha1.AppConnection
	options    pvrclient.Options
}

// pvrIntegration is the Integration every PVR app's resource runs on. describe
// reads what the pipeline takes from a resource.
func pvrIntegration[T ctrlcommon.ConfigResource](
	c client.Client, apps ctrlcommon.Apps, app pvrclient.App, describe func(T) pvrConfig,
) ctrlcommon.Integration[T, *engine.HTTPClient] {
	return ctrlcommon.Integration[T, *engine.HTTPClient]{
		App: app.Name,
		Connect: func(ctx context.Context, obj T) (*engine.HTTPClient, error) {
			return connectPVR(ctx, c, obj, describe(obj).connection, app)
		},
		Check: func(ctx context.Context, hc *engine.HTTPClient, _ T) error { return pingPVR(ctx, hc, app) },
		Sync: func(ctx context.Context, hc *engine.HTTPClient, obj T, pass *engine.Pass) []error {
			cfg, namespace := describe(obj), obj.GetNamespace()
			opts := cfg.options
			opts.App = app
			// A kind whose items name a Secret that can't be read is left out of
			// the pass, neither written nor pruned, and reported; the rest still
			// syncs.
			var unresolved []error
			var err error
			if opts.Notifications, err = ctrlcommon.ResolveNotifications(ctx, c, apps, namespace, opts.Notifications); err != nil {
				unresolved = append(unresolved, fmt.Errorf("notifications: %w", err))
			} else if opts.Notifications, err = ctrlcommon.ResolveNotificationSecrets(ctx, c, namespace, opts.Notifications); err != nil {
				unresolved = append(unresolved, fmt.Errorf("notifications: %w", err))
			}
			if opts.ImportLists, err = ctrlcommon.ResolveImportListSecrets(ctx, c, namespace, opts.ImportLists); err != nil {
				unresolved = append(unresolved, fmt.Errorf("importLists: %w", err))
			}
			return append(unresolved, pvrclient.Reconcile(ctx, hc, cfg.spec, opts, pass)...)
		},
		Cleanup: func(ctx context.Context, hc *engine.HTTPClient, owned *engine.Ownership) error {
			return engine.DeleteManagedResources(ctx, hc, pvrclient.Definition(app), owned)
		},
		Deletable: pvrclient.Definition(app).Deletable,
		Prune:     true,
	}
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

// connectPVR builds the client for an app's connection.
func connectPVR(
	ctx context.Context, c client.Reader, obj ctrlcommon.ConfigResource, conn commonv1alpha1.AppConnection, app pvrclient.App,
) (*engine.HTTPClient, error) {
	namespace := obj.GetNamespace()
	apiKey, err := reconciler.ResolveSecretKeyRef(ctx, c, namespace, conn.APIKeySecretRef)
	if err != nil {
		return nil, err
	}
	tlsCfg, err := engine.ResolveTLSConfig(ctx, c, namespace, conn.TLS)
	if err != nil {
		return nil, err
	}
	hc, err := engine.NewHTTPClient(conn.URL, engine.AuthAPIKey,
		engine.WithAPIKey(apiKey), engine.WithTLSConfig(tlsCfg), engine.WithAppLabel(app.Name))
	if err != nil {
		return nil, err
	}
	return hc, nil
}

// pingPVR checks an app answers.
func pingPVR(ctx context.Context, hc *engine.HTTPClient, app pvrclient.App) error {
	return hc.Ping(ctx, "/api/"+app.APIVersion+"/system/status")
}
