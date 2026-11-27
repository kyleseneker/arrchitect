package pvr

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	pvrclient "github.com/kyleseneker/arrchitect/internal/client/pvr"
	"github.com/kyleseneker/arrchitect/internal/controller/appref"
	"github.com/kyleseneker/arrchitect/internal/controller/reconciler"
	"github.com/kyleseneker/arrchitect/internal/controller/secretref"
	"github.com/kyleseneker/arrchitect/internal/engine"
	"github.com/kyleseneker/arrchitect/internal/failure"
	"github.com/kyleseneker/arrchitect/internal/httpapi"
)

// pvrConfig is what the PVR pipeline takes from an app's resource.
type pvrConfig struct {
	spec       any
	connection commonv1alpha1.AppConnection
	options    pvrclient.Options
}

// pvrIntegration is the Integration every PVR app's resource runs on. describe
// reads what the pipeline takes from a resource.
func pvrIntegration[T reconciler.ConfigResource](
	c client.Client, apps appref.Apps, app pvrclient.App, describe func(T) pvrConfig,
) reconciler.Integration[T, *httpapi.HTTPClient] {
	return reconciler.Integration[T, *httpapi.HTTPClient]{
		App: app.Name,
		Connect: func(ctx context.Context, obj T) (*httpapi.HTTPClient, error) {
			return connectPVR(ctx, c, obj.GetNamespace(), describe(obj).connection, app)
		},
		Check: func(ctx context.Context, hc *httpapi.HTTPClient, _ T) error { return pingPVR(ctx, hc, app) },
		Sync: func(ctx context.Context, hc *httpapi.HTTPClient, obj T, pass *engine.Pass) []error {
			cfg, namespace := describe(obj), obj.GetNamespace()
			opts := cfg.options
			opts.App = app
			// A kind whose items name a Secret that can't be read is left out of
			// the pass, neither written nor pruned, and reported; the rest still
			// syncs.
			var unresolved []error
			var err error
			if opts.Notifications, err = resolveNotifications(ctx, c, apps, namespace, opts.Notifications); err != nil {
				unresolved = append(unresolved, fmt.Errorf("notifications: %w", err))
			} else if opts.Notifications, err = secretref.ResolveItemFields(ctx, c, namespace, "notification", opts.Notifications,
				func(n *commonv1alpha1.Notification) (string, *[]commonv1alpha1.ConfigField) { return n.Name, &n.Fields }); err != nil {
				unresolved = append(unresolved, fmt.Errorf("notifications: %w", err))
			}
			if opts.ImportLists, err = secretref.ResolveItemFields(ctx, c, namespace, "import list", opts.ImportLists,
				func(il *commonv1alpha1.ImportList) (string, *[]commonv1alpha1.ConfigField) {
					return il.Name, &il.Fields
				}); err != nil {
				unresolved = append(unresolved, fmt.Errorf("importLists: %w", err))
			}
			return append(unresolved, pvrclient.Reconcile(ctx, hc, cfg.spec, opts, pass)...)
		},
		Cleanup: func(ctx context.Context, hc *httpapi.HTTPClient, owned *engine.Ownership) error {
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

// notificationAddressFields are the fields a notification's appRef fills in.
var notificationAddressFields = []string{"host", "port", "useSsl", "urlBase", "apiKey", "authToken"}

// resolveNotifications returns notifications with each appRef replaced by the
// media server's notification type, address and key.
func resolveNotifications(
	ctx context.Context, c client.Reader, apps appref.Apps, namespace string, in []commonv1alpha1.Notification,
) ([]commonv1alpha1.Notification, error) {
	out := slices.Clone(in)
	for i, n := range out {
		if n.AppRef == (commonv1alpha1.AppRef{}) {
			continue
		}
		for _, f := range n.Fields {
			if slices.Contains(notificationAddressFields, f.Name) {
				return nil, failure.InvalidConfig.Errorf("notification %q: appRef fills in %s, so leave that field out", n.Name, f.Name)
			}
		}
		app, addr, err := appref.ResolveAddress(ctx, c, apps, namespace, n.AppRef)
		if err != nil {
			return nil, fmt.Errorf("notification %q appRef: %w", n.Name, err)
		}
		key := commonv1alpha1.ConfigField{Name: "authToken"}
		switch app.Kind {
		case "Jellyfin":
			if app.APIKeySecretRef == nil {
				return nil, failure.InvalidConfig.Errorf("notification %q appRef: Jellyfin %q sets no API key Secret", n.Name, n.AppRef.Name)
			}
			n.Implementation, key.Name, key.ValueFrom.SecretKeyRef = "MediaBrowser", "apiKey", *app.APIKeySecretRef
		case "Plex":
			n.Implementation, key.ValueFrom.SecretKeyRef = "PlexServer", *app.TokenSecretRef
		default:
			return nil, failure.InvalidConfig.Errorf("notification %q appRef: %s isn't a media server", n.Name, app.Kind)
		}
		n.Fields = append(slices.Clone(n.Fields),
			fieldValue("host", addr.Host), fieldValue("port", addr.Port), fieldValue("useSsl", addr.TLS), key)
		if addr.Base != "" {
			n.Fields = append(n.Fields, fieldValue("urlBase", addr.Base))
		}
		n.AppRef = commonv1alpha1.AppRef{}
		out[i] = n
	}
	return out, nil
}

func fieldValue(name string, v any) commonv1alpha1.ConfigField {
	raw, _ := json.Marshal(v)
	return commonv1alpha1.ConfigField{Name: name, Value: &commonv1alpha1.FieldValue{Raw: raw}}
}

// connectPVR builds the client for an app's connection.
func connectPVR(
	ctx context.Context, c client.Reader, namespace string, conn commonv1alpha1.AppConnection, app pvrclient.App,
) (*httpapi.HTTPClient, error) {
	return reconciler.Dial(ctx, c, namespace, reconciler.Endpoint{
		App: app.Name, URL: conn.URL, TLS: conn.TLS, Auth: httpapi.AuthAPIKey, Key: &conn.APIKeySecretRef,
	})
}

// pingPVR checks an app answers.
func pingPVR(ctx context.Context, hc *httpapi.HTTPClient, app pvrclient.App) error {
	return hc.Ping(ctx, "/api/"+app.APIVersion+"/system/status")
}
