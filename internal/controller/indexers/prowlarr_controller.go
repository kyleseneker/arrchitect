package indexers

import (
	"context"
	"fmt"

	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	indexersv1alpha1 "github.com/kyleseneker/arrchitect/api/indexers/v1alpha1"
	"github.com/kyleseneker/arrchitect/internal/controller/appref"
	"github.com/kyleseneker/arrchitect/internal/controller/reconciler"
	"github.com/kyleseneker/arrchitect/internal/controller/secretref"
	"github.com/kyleseneker/arrchitect/internal/engine"
	"github.com/kyleseneker/arrchitect/internal/httpapi"
	"github.com/kyleseneker/arrchitect/internal/provider"
	"github.com/kyleseneker/arrchitect/internal/secrets"
)

type ProwlarrReconciler struct {
	client.Client
	Recorder events.EventRecorder
	Apps     appref.Apps
}

// +kubebuilder:rbac:groups=arrchitect.dev,resources=prowlarrs,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=arrchitect.dev,resources=prowlarrs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=arrchitect.dev,resources=prowlarrs/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=arrchitect.dev,resources=sonarrs;radarrs;lidarrs;readarrs,verbs=get;list;watch

func (r *ProwlarrReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.integration().Reconcile(ctx, r.Client, r.Recorder, req)
}

// SetupWithManager sets up the controller with the Manager.
func (r *ProwlarrReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return r.integration().SetupWithManager(mgr, r.Apps, r)
}

func (r *ProwlarrReconciler) integration() reconciler.Integration[*indexersv1alpha1.Prowlarr, *httpapi.HTTPClient] {
	return reconciler.Integration[*indexersv1alpha1.Prowlarr, *httpapi.HTTPClient]{
		App:      "prowlarr",
		New:      func() *indexersv1alpha1.Prowlarr { return &indexersv1alpha1.Prowlarr{} },
		NewList:  func() client.ObjectList { return &indexersv1alpha1.ProwlarrList{} },
		AppRefs:  prowlarrAppRefs,
		RefKinds: []string{"Sonarr", "Radarr", "Lidarr", "Readarr"},
		Connect:  r.connect,
		Check: func(ctx context.Context, hc *httpapi.HTTPClient, _ *indexersv1alpha1.Prowlarr) error {
			return hc.Ping(ctx, ProwlarrDefinition().HealthPath)
		},
		Sync: r.sync,
		Cleanup: func(ctx context.Context, hc *httpapi.HTTPClient, owned *engine.Ownership) error {
			return engine.DeleteManagedResources(ctx, hc, ProwlarrDefinition(), owned)
		},
		Deletable: ProwlarrDefinition().Deletable,
		Prune:     true,
	}
}

func (r *ProwlarrReconciler) connect(ctx context.Context, config *indexersv1alpha1.Prowlarr) (*httpapi.HTTPClient, error) {
	conn := config.Spec.Connection
	return reconciler.Dial(ctx, r.Client, config.Namespace, reconciler.Endpoint{
		App: "prowlarr", URL: conn.URL, TLS: conn.TLS, Auth: httpapi.AuthAPIKey, Key: &conn.APIKeySecretRef,
	})
}

func (r *ProwlarrReconciler) sync(ctx context.Context, hc *httpapi.HTTPClient, config *indexersv1alpha1.Prowlarr, pass *engine.Pass) []error {
	namespace := config.Namespace
	opts := prowlarrOptions{Tags: config.Spec.Tags, AppProfiles: config.Spec.AppProfiles}
	// A kind whose items name a Secret or app that can't be read is left out of
	// the pass, neither written nor pruned, and reported; the rest still syncs.
	var unresolved []error
	var err error
	if opts.Indexers, err = secretref.ResolveItemFields(ctx, r.Client, namespace, "indexer", config.Spec.Indexers,
		func(i *indexersv1alpha1.ProwlarrIndexer) (string, *[]commonv1alpha1.ConfigField) {
			return i.Name, &i.Fields
		},
	); err != nil {
		unresolved = append(unresolved, fmt.Errorf("indexers: %w", err))
	}
	if opts.DownloadClients, err = secretref.ResolveItemFields(ctx, r.Client, namespace, "download client", config.Spec.DownloadClients,
		func(dc *indexersv1alpha1.ProwlarrDownloadClient) (string, *[]commonv1alpha1.ConfigField) {
			return dc.Name, &dc.Fields
		},
	); err != nil {
		unresolved = append(unresolved, fmt.Errorf("downloadClients: %w", err))
	}
	if opts.Notifications, err = secretref.ResolveItemFields(ctx, r.Client, namespace, "notification", config.Spec.Notifications,
		func(n *commonv1alpha1.Notification) (string, *[]commonv1alpha1.ConfigField) {
			return n.Name, &n.Fields
		},
	); err != nil {
		unresolved = append(unresolved, fmt.Errorf("notifications: %w", err))
	}
	if opts.Applications, err = r.resolveApplications(ctx, hc, config); err != nil {
		unresolved = append(unresolved, fmt.Errorf("applications: %w", err))
	}
	if opts.Proxies, err = r.resolveProxies(ctx, config); err != nil {
		unresolved = append(unresolved, fmt.Errorf("proxies: %w", err))
	}
	return append(unresolved, reconcileProwlarr(ctx, hc, opts, pass)...)
}

// resolveApplications resolves each application's reference and Secrets.
func (r *ProwlarrReconciler) resolveApplications(
	ctx context.Context, hc *httpapi.HTTPClient, config *indexersv1alpha1.Prowlarr,
) ([]prowlarrApplication, error) {
	apps := make([]prowlarrApplication, 0, len(config.Spec.Applications))
	var schema []map[string]any
	for _, app := range config.Spec.Applications {
		if app.AppRef != nil {
			ref, err := appref.Resolve(ctx, r.Client, r.Apps, config.Namespace, *app.AppRef)
			if err != nil {
				return nil, fmt.Errorf("application %q appRef: %w", app.Name, err)
			}
			app.Implementation, app.BaseURL, app.APIKeySecretRef = ref.Kind, ref.URL, ref.APIKeySecretRef
			if app.ProwlarrURL == "" {
				app.ProwlarrURL = config.Spec.Connection.URL
			}
		}
		if app.APIKeySecretRef == nil {
			return nil, fmt.Errorf("application %q has no API key Secret", app.Name)
		}
		resolved := map[string]string{}
		for _, ref := range []struct {
			field, name string
			ref         *commonv1alpha1.SecretKeyRef
		}{
			{"apiKey", "apiKeySecretRef", app.APIKeySecretRef},
			{"authUsername", "authUsernameSecretRef", app.AuthUsernameSecretRef},
			{"authPassword", "authPasswordSecretRef", app.AuthPasswordSecretRef},
		} {
			if ref.ref == nil {
				continue
			}
			if ref.field != "apiKey" {
				if err := requireApplicationField(ctx, hc, &schema, app, ref.field); err != nil {
					return nil, fmt.Errorf("application %q %s: %w", app.Name, ref.name, err)
				}
			}
			value, err := secrets.ResolveSecretKeyRef(ctx, r.Client, config.Namespace, *ref.ref)
			if err != nil {
				return nil, fmt.Errorf("application %q %s: %w", app.Name, ref.name, err)
			}
			resolved[ref.field] = value
		}
		apps = append(apps, prowlarrApplication{ProwlarrApplication: app, Secrets: resolved})
	}
	return apps, nil
}

// requireApplicationField fails when Prowlarr's schema for the application's
// implementation lacks field, as Prowlarr before 2.4.0 lacks basic auth. It
// reads the schema once, into schema.
func requireApplicationField(
	ctx context.Context, hc *httpapi.HTTPClient, schema *[]map[string]any, app indexersv1alpha1.ProwlarrApplication, field string,
) error {
	if *schema == nil {
		list, err := hc.GetJSONList(ctx, "/api/v1/applications/schema")
		if err != nil {
			return fmt.Errorf("reading Prowlarr's application schema: %w", err)
		}
		*schema = list
	}
	if names, ok := provider.SchemaFieldNames(*schema, app.Implementation); ok && !names[field] {
		return fmt.Errorf("%s applications have no %s in this Prowlarr; it needs Prowlarr 2.4.0 or later", app.Implementation, field)
	}
	return nil
}

// resolveProxies resolves each proxy's credential Secrets.
func (r *ProwlarrReconciler) resolveProxies(ctx context.Context, config *indexersv1alpha1.Prowlarr) ([]prowlarrProxy, error) {
	proxies := make([]prowlarrProxy, 0, len(config.Spec.Proxies))
	for _, p := range config.Spec.Proxies {
		credentials := map[string]string{}
		for name, ref := range map[string]*commonv1alpha1.SecretKeyRef{"username": p.UsernameSecretRef, "password": p.PasswordSecretRef} {
			if ref == nil {
				continue
			}
			value, err := secrets.ResolveSecretKeyRef(ctx, r.Client, config.Namespace, *ref)
			if err != nil {
				return nil, fmt.Errorf("proxy %q %s: %w", p.Name, name, err)
			}
			credentials[name] = value
		}
		proxies = append(proxies, prowlarrProxy{ProwlarrProxy: p, Credentials: credentials})
	}
	return proxies, nil
}

func prowlarrAppRefs(o client.Object) []commonv1alpha1.AppRef {
	var refs []commonv1alpha1.AppRef
	for _, app := range o.(*indexersv1alpha1.Prowlarr).Spec.Applications {
		if app.AppRef != nil {
			refs = append(refs, *app.AppRef)
		}
	}
	return refs
}
