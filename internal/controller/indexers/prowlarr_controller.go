package indexers

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	indexersv1alpha1 "github.com/kyleseneker/arrchitect/api/indexers/v1alpha1"
	prowlarrclient "github.com/kyleseneker/arrchitect/internal/client/prowlarr"
	pvrclient "github.com/kyleseneker/arrchitect/internal/client/pvr"
	ctrlcommon "github.com/kyleseneker/arrchitect/internal/controller/common"
	"github.com/kyleseneker/arrchitect/internal/engine"
	"github.com/kyleseneker/arrchitect/internal/reconciler"
)

type ProwlarrReconciler struct {
	client.Client
	Recorder events.EventRecorder
	Apps     ctrlcommon.Apps
}

// +kubebuilder:rbac:groups=arrchitect.dev,resources=prowlarrs,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=arrchitect.dev,resources=prowlarrs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=arrchitect.dev,resources=prowlarrs/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=arrchitect.dev,resources=sonarrs;radarrs;lidarrs;readarrs,verbs=get;list;watch

func (r *ProwlarrReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	integration := ctrlcommon.Integration[*indexersv1alpha1.Prowlarr, *engine.HTTPClient]{
		App:     "prowlarr",
		Connect: r.connect,
		Check: func(ctx context.Context, hc *engine.HTTPClient, _ *indexersv1alpha1.Prowlarr) error {
			return hc.Ping(ctx, prowlarrclient.ProwlarrDefinition().HealthPath)
		},
		Sync: r.sync,
		Cleanup: func(ctx context.Context, hc *engine.HTTPClient, owned *engine.Ownership) error {
			return engine.DeleteManagedResources(ctx, hc, prowlarrclient.ProwlarrDefinition(), owned)
		},
		Deletable: prowlarrclient.ProwlarrDefinition().Deletable,
		Prune:     true,
	}
	return integration.Reconcile(ctx, r.Client, r.Recorder, req, &indexersv1alpha1.Prowlarr{})
}

func (r *ProwlarrReconciler) connect(ctx context.Context, config *indexersv1alpha1.Prowlarr) (*engine.HTTPClient, error) {
	apiKey, err := reconciler.ResolveSecretKeyRef(ctx, r.Client, config.Namespace, config.Spec.Connection.APIKeySecretRef)
	if err != nil {
		return nil, err
	}
	tlsCfg, err := engine.ResolveTLSConfig(ctx, r.Client, config.Namespace, config.Spec.Connection.TLS)
	if err != nil {
		return nil, err
	}
	hc, err := engine.NewHTTPClient(config.Spec.Connection.URL, engine.AuthAPIKey,
		engine.WithOwner(config.Namespace+"/"+config.Name+"/"+string(config.UID)), engine.WithAPIKey(apiKey),
		engine.WithTLSConfig(tlsCfg), engine.WithAppLabel("prowlarr"))
	if err != nil {
		return nil, err
	}
	return hc, nil
}

func (r *ProwlarrReconciler) sync(ctx context.Context, hc *engine.HTTPClient, config *indexersv1alpha1.Prowlarr, pass *engine.Pass) []string {
	def := prowlarrclient.ProwlarrDefinition()
	// A kind whose items name a Secret that can't be read is left out of the
	// pass, neither written nor pruned, and reported; the rest still syncs.
	var unresolved []string
	indexers := make([]indexersv1alpha1.ProwlarrIndexer, len(config.Spec.Indexers))
	copy(indexers, config.Spec.Indexers)
	for i := range indexers {
		f, ferr := ctrlcommon.ResolveConfigFields(ctx, r.Client, config.Namespace, "indexer "+indexers[i].Name, indexers[i].Fields)
		if ferr != nil {
			unresolved = append(unresolved, fmt.Sprintf("indexers: %v", ferr))
			indexers = nil
			break
		}
		indexers[i].Fields = f
	}
	downloadClients := make([]indexersv1alpha1.ProwlarrDownloadClient, len(config.Spec.DownloadClients))
	copy(downloadClients, config.Spec.DownloadClients)
	for i := range downloadClients {
		f, ferr := ctrlcommon.ResolveConfigFields(ctx, r.Client, config.Namespace, "download client "+downloadClients[i].Name, downloadClients[i].Fields)
		if ferr != nil {
			unresolved = append(unresolved, fmt.Sprintf("downloadClients: %v", ferr))
			downloadClients = nil
			break
		}
		downloadClients[i].Fields = f
	}

	// Tags and sync profiles must exist before their IDs can be referenced.
	prerequisites := prowlarrclient.ProwlarrResources(prowlarrclient.ProwlarrOptions{Tags: config.Spec.Tags, AppProfiles: config.Spec.AppProfiles}, nil)
	problems := engine.ReconcileApp(ctx, hc, def, nil, prerequisites, pass)

	resolved, tagIDs, terr := prowlarrclient.ResolveReferences(ctx, hc, config.Spec, pass.Observe)
	if indexers != nil {
		indexersWithSecrets := indexers
		indexers = resolved.Indexers
		for i := range indexers {
			indexers[i].Fields = indexersWithSecrets[i].Fields
		}
	}
	if terr != nil {
		pass.Retry = time.Minute
		return append(problems, terr.Error())
	}

	notifications, nerr := ctrlcommon.ResolveNotificationSecrets(ctx, r.Client, config.Namespace, resolved.Notifications)
	if nerr != nil {
		unresolved = append(unresolved, fmt.Sprintf("notifications: %v", nerr))
	}
	appPayloads, aerr := r.buildApplicationPayloads(ctx, hc, config, tagIDs)
	if aerr != nil {
		unresolved = append(unresolved, fmt.Sprintf("applications: %v", aerr))
	}
	proxyPayloads, perr := r.buildProxyPayloads(ctx, config, tagIDs)
	if perr != nil {
		unresolved = append(unresolved, fmt.Sprintf("proxies: %v", perr))
	}

	resources := prowlarrclient.ProwlarrResources(prowlarrclient.ProwlarrOptions{
		Applications:    appPayloads,
		Indexers:        indexers,
		Proxies:         proxyPayloads,
		DownloadClients: downloadClients,
		Notifications:   notifications,
	}, tagIDs)
	problems = append(problems, unresolved...)
	return append(problems, engine.ReconcileApp(ctx, hc, def, nil, resources, pass)...)
}

// buildApplicationPayloads resolves each application's reference and API key
// Secret and builds its payload.
func (r *ProwlarrReconciler) buildApplicationPayloads(
	ctx context.Context, hc *engine.HTTPClient, config *indexersv1alpha1.Prowlarr, tagIDs map[string]int,
) ([]map[string]any, error) {
	payloads := make([]map[string]any, 0, len(config.Spec.Applications))
	var schema []map[string]any
	for _, app := range config.Spec.Applications {
		if app.AppRef != nil {
			ref, err := ctrlcommon.ResolveAppRef(ctx, r.Client, r.Apps, config.Namespace, *app.AppRef)
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
		secrets := map[string]string{}
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
			value, err := reconciler.ResolveSecretKeyRef(ctx, r.Client, config.Namespace, *ref.ref)
			if err != nil {
				return nil, fmt.Errorf("application %q %s: %w", app.Name, ref.name, err)
			}
			secrets[ref.field] = value
		}
		payloads = append(payloads, prowlarrclient.BuildProwlarrApplicationPayload(app, secrets, tagIDs))
	}
	return payloads, nil
}

// requireApplicationField fails when Prowlarr's schema for the application's
// implementation lacks field, as Prowlarr before 2.4.0 lacks basic auth. It
// reads the schema once, into schema.
func requireApplicationField(
	ctx context.Context, hc *engine.HTTPClient, schema *[]map[string]any, app indexersv1alpha1.ProwlarrApplication, field string,
) error {
	if *schema == nil {
		list, err := hc.GetJSONList(ctx, "/api/v1/applications/schema")
		if err != nil {
			return fmt.Errorf("reading Prowlarr's application schema: %w", err)
		}
		*schema = list
	}
	if names, ok := pvrclient.SchemaFieldNames(*schema, app.Implementation); ok && !names[field] {
		return fmt.Errorf("%s applications have no %s in this Prowlarr; it needs Prowlarr 2.4.0 or later", app.Implementation, field)
	}
	return nil
}

// buildProxyPayloads resolves each proxy's credential Secrets and builds its
// payload.
func (r *ProwlarrReconciler) buildProxyPayloads(ctx context.Context, config *indexersv1alpha1.Prowlarr, tagIDs map[string]int) ([]map[string]any, error) {
	payloads := make([]map[string]any, 0, len(config.Spec.Proxies))
	for _, p := range config.Spec.Proxies {
		credentials := map[string]string{}
		for name, ref := range map[string]*commonv1alpha1.SecretKeyRef{"username": p.UsernameSecretRef, "password": p.PasswordSecretRef} {
			if ref == nil {
				continue
			}
			value, err := reconciler.ResolveSecretKeyRef(ctx, r.Client, config.Namespace, *ref)
			if err != nil {
				return nil, fmt.Errorf("proxy %q %s: %w", p.Name, name, err)
			}
			credentials[name] = value
		}
		payloads = append(payloads, prowlarrclient.BuildProwlarrProxyPayload(p, credentials, tagIDs))
	}
	return payloads, nil
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

func (r *ProwlarrReconciler) SetupWithManager(mgr ctrl.Manager) error {
	b := ctrl.NewControllerManagedBy(mgr).
		For(&indexersv1alpha1.Prowlarr{}, builder.WithPredicates(ctrlcommon.ConfigChangedPredicate())).
		WatchesMetadata(&corev1.Secret{}, ctrlcommon.EnqueueForSecretThroughRefs(r.Client, r.Apps, &indexersv1alpha1.ProwlarrList{}, prowlarrAppRefs))
	b, err := ctrlcommon.WatchAppRefs(mgr, b, r.Apps, &indexersv1alpha1.Prowlarr{}, &indexersv1alpha1.ProwlarrList{}, prowlarrAppRefs, "Sonarr", "Radarr", "Lidarr", "Readarr")
	if err != nil {
		return err
	}
	return b.
		Named("prowlarr").
		Complete(r)
}
