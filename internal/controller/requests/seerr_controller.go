package requests

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	requestsv1alpha1 "github.com/kyleseneker/arrchitect/api/requests/v1alpha1"
	seerrclient "github.com/kyleseneker/arrchitect/internal/client/seerr"
	ctrlcommon "github.com/kyleseneker/arrchitect/internal/controller/common"
	"github.com/kyleseneker/arrchitect/internal/engine"
	"github.com/kyleseneker/arrchitect/internal/reconciler"
)

// SeerrReconciler reconciles a Seerr object.
type SeerrReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder events.EventRecorder
	Apps     ctrlcommon.Apps
}

// +kubebuilder:rbac:groups=arrchitect.dev,resources=seerrs,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=arrchitect.dev,resources=seerrs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=arrchitect.dev,resources=seerrs/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=arrchitect.dev,resources=sonarrs;radarrs;jellyfins;plexes,verbs=get;list;watch

func (r *SeerrReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var config requestsv1alpha1.Seerr
	if err := r.Get(ctx, req.NamespacedName, &config); err != nil {
		return ctrl.Result{}, ctrlcommon.IgnoreDeleted(&config, req.NamespacedName, err)
	}

	lifecycle := ctrlcommon.Lifecycle{Remove: func(ctx context.Context) error { return r.cleanup(ctx, &config) }, Prune: true}
	if done, after := ctrlcommon.HandleLifecycle(ctx, r.Client, r.Recorder, &config, lifecycle); done {
		return ctrl.Result{RequeueAfter: after}, nil
	}

	sc, reason, err := r.seerrClient(ctx, &config)
	if err != nil {
		logger.Error(err, "failed to build the Seerr client")
		ctrlcommon.UpdateStatusUnreachable(ctx, r.Client, &config, reason, err.Error())
		if reason == engine.ReasonInvalidConfig {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	isInitialized, err := sc.IsInitialized(ctx)
	if err != nil {
		logger.Error(err, "Seerr unreachable")
		ctrlcommon.UpdateStatusUnreachable(ctx, r.Client, &config, engine.ReasonAppUnreachable, err.Error())
		return ctrl.Result{RequeueAfter: 60 * time.Second}, nil
	}

	writes := &engine.Writes{App: "seerr", Observe: ctrlcommon.ObserveOnly(config.Spec.Reconcile)}
	owned := engine.NewOwnership("seerr", config.GetManagedResources())
	if !isInitialized && writes.Observe {
		// Signing in to a Seerr that has not been set up configures its media
		// server, so observe mode reports the setup as missing and stops.
		config.Status.Initialized = &isInitialized
		writes.RecordDrift("settings", "initialization")
		ctrlcommon.UpdateObservationStatus(ctx, r.Client, &config, writes.Drift(), nil)
		return ctrl.Result{RequeueAfter: ctrlcommon.ReconcileInterval(config.Spec.Reconcile)}, nil
	}

	media, mediaErr := r.mediaServer(ctx, &config)
	if mediaErr != nil && (config.Spec.Connection.APIKeySecretRef == nil || !isInitialized) {
		reason, after := ctrlcommon.AppRefFailure(mediaErr)
		ctrlcommon.UpdateStatusUnreachable(ctx, r.Client, &config, reason, mediaErr.Error())
		return ctrl.Result{RequeueAfter: after}, nil
	}

	if err := r.establishSession(ctx, sc, &config, media); err != nil {
		reason := engine.ReasonAppUnreachable
		if reconciler.IsSecretError(err) {
			reason = engine.ReasonSecretNotFound
		}
		ctrlcommon.UpdateStatusUnreachable(ctx, r.Client, &config, reason, err.Error())
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	if !isInitialized {
		if err := r.setup(ctx, sc, writes, owned, &config, media); err != nil {
			// A setup that failed partway may have created connections already.
			ctrlcommon.UpdateStatusManaged(&config, owned.Managed())
			ctrlcommon.UpdateStatus(ctx, r.Client, &config, false, engine.ReasonSyncFailed, err.Error())
			return ctrl.Result{RequeueAfter: 60 * time.Second}, nil
		}
	}

	initialized := true
	config.Status.Initialized = &initialized

	syncErrors := reconcileSeerrMediaServer(ctx, sc, writes, media, mediaErr, config.Spec.Libraries)

	for _, k := range seerrServers(config.Spec) {
		kind, declared := k.kind, k.servers
		problems := r.reconcileServiceConnections(ctx, sc, writes, owned, &config, kind, declared)
		if len(problems) == 0 && ctrlcommon.PruneEnabled(config.Spec.Reconcile) {
			if err := pruneSeerrService(ctx, sc, writes, owned, kind, declared); err != nil {
				problems = append(problems, fmt.Sprintf("%s: %v", kind, err))
			}
		}
		for _, problem := range problems {
			logger.Error(errors.New(problem), "failed to reconcile connection", "service", kind)
		}
		syncErrors = append(syncErrors, problems...)
	}

	if config.Spec.Main != nil {
		if err := reconcileSeerrMainSettings(ctx, sc, writes, config.Spec.Main); err != nil {
			logger.Error(err, "failed to reconcile main settings")
			syncErrors = append(syncErrors, fmt.Sprintf("main: %v", err))
		}
	}
	if err := reconcileSeerrNetwork(ctx, sc, writes, config.Spec.Network); err != nil {
		syncErrors = append(syncErrors, fmt.Sprintf("network: %v", err))
	}
	if err := reconcileSeerrMetadataProviders(ctx, sc, writes, config.Spec.MetadataProviders); err != nil {
		syncErrors = append(syncErrors, fmt.Sprintf("metadataProviders: %v", err))
	}
	syncErrors = append(syncErrors, reconcileSeerrJobSchedules(ctx, sc, writes, config.Spec.JobSchedules)...)

	for _, agent := range config.Spec.Notifications {
		options, err := ctrlcommon.FieldValues(ctx, r.Client, config.Namespace, "notification "+agent.Agent, agent.Options)
		if err == nil {
			err = reconcileSeerrNotificationAgent(ctx, sc, writes, agent, options)
		}
		if err != nil {
			logger.Error(err, "failed to reconcile notification agent", "agent", agent.Agent)
			syncErrors = append(syncErrors, fmt.Sprintf("notification(%s): %v", agent.Agent, err))
		}
	}

	ctrlcommon.UpdateStatusManaged(&config, owned.Managed())
	ctrlcommon.EmitPruneEvents(r.Recorder, &config, owned.Pruned())
	ctrlcommon.UpdateSyncStatus(ctx, r.Client, &config, writes.Observe, writes.Drift(), syncErrors)

	return ctrl.Result{RequeueAfter: ctrlcommon.ReconcileInterval(config.Spec.Reconcile)}, nil
}

// seerrClient builds the client for a config, or returns the status reason it
// can't.
func (r *SeerrReconciler) seerrClient(ctx context.Context, config *requestsv1alpha1.Seerr) (*seerrclient.Client, string, error) {
	tlsCfg, err := engine.ResolveTLSConfig(ctx, r.Client, config.Namespace, config.Spec.Connection.TLS)
	if err != nil {
		return nil, engine.ReasonSecretNotFound, err
	}
	hc, err := engine.NewHTTPClient(config.Spec.Connection.URL, engine.AuthSession, engine.WithTLSConfig(tlsCfg), engine.WithAppLabel("seerr"))
	if err != nil {
		return nil, engine.ReasonInvalidConfig, err
	}
	return seerrclient.NewClient(hc), "", nil
}

type seerrServerKind struct {
	kind    string
	servers []requestsv1alpha1.SeerrServiceConnection
}

// seerrServers lists the declared servers of each kind that has any.
func seerrServers(spec requestsv1alpha1.SeerrSpec) []seerrServerKind {
	var kinds []seerrServerKind
	if len(spec.Sonarr) > 0 {
		kinds = append(kinds, seerrServerKind{"sonarr", spec.Sonarr})
	}
	if len(spec.Radarr) > 0 {
		kinds = append(kinds, seerrServerKind{"radarr", spec.Radarr})
	}
	return kinds
}

// pruneSeerrService deletes the connections of a kind the operator created that
// are no longer declared.
func pruneSeerrService(
	ctx context.Context, sc *seerrclient.Client, w *engine.Writes, owned *engine.Ownership, kind string, servers []requestsv1alpha1.SeerrServiceConnection,
) error {
	names, ids, err := seerrServices(ctx, sc, kind)
	if err != nil {
		return err
	}
	declared := make([]string, 0, len(servers))
	for _, s := range servers {
		declared = append(declared, s.Name)
	}
	return w.Prune(owned, kind, declared, names, func(name string) error {
		return sc.Delete(ctx, fmt.Sprintf("/api/v1/settings/%s/%d", kind, ids[name]))
	})
}

// cleanup deletes the connections the operator created, for deletionPolicy delete.
func (r *SeerrReconciler) cleanup(ctx context.Context, config *requestsv1alpha1.Seerr) error {
	sc, _, err := r.seerrClient(ctx, config)
	if err != nil {
		return err
	}
	media, err := r.mediaServer(ctx, config)
	if err != nil && config.Spec.Connection.APIKeySecretRef == nil {
		return err
	}
	if err := r.establishSession(ctx, sc, config, media); err != nil {
		return err
	}
	owned := engine.NewOwnership("seerr", config.GetManagedResources())
	var errs []error
	for _, kind := range []string{"sonarr", "radarr"} {
		names, ids, err := seerrServices(ctx, sc, kind)
		if err == nil {
			err = owned.Cleanup(kind, names, func(name string) error {
				return sc.Delete(ctx, fmt.Sprintf("/api/v1/settings/%s/%d", kind, ids[name]))
			})
		}
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func seerrServices(ctx context.Context, sc *seerrclient.Client, kind string) ([]string, map[string]int, error) {
	servers, err := sc.GetList(ctx, "/api/v1/settings/"+kind)
	if err != nil {
		return nil, nil, fmt.Errorf("listing %s connections: %w", engine.AppName(kind), err)
	}
	var names []string
	ids := map[string]int{}
	for _, s := range servers {
		name, _ := s["name"].(string)
		if id, ok := s["id"].(float64); ok && name != "" {
			names = append(names, name)
			ids[name] = int(id)
		}
	}
	return names, ids, nil
}

// setup runs Seerr's first-time setup: the media server's libraries, the Sonarr
// and Radarr connections and the main settings, then marks Seerr initialized.
func (r *SeerrReconciler) setup(
	ctx context.Context, sc *seerrclient.Client, writes *engine.Writes, owned *engine.Ownership, config *requestsv1alpha1.Seerr,
	media seerrMediaServer,
) error {
	logger := log.FromContext(ctx)
	var initErrors []string

	if media.jellyfin != nil {
		if err := syncJellyfinLibraries(ctx, sc, writes, media.jellyfin, config.Spec.Libraries); err != nil {
			logger.Error(err, "failed to configure Jellyfin libraries")
			initErrors = append(initErrors, err.Error())
		}
	}
	if media.plex != nil {
		if err := syncPlexLibraries(ctx, sc, writes, media.plex, config.Spec.Libraries); err != nil {
			logger.Error(err, "failed to configure Plex libraries")
			initErrors = append(initErrors, err.Error())
		}
	}

	for _, k := range seerrServers(config.Spec) {
		for _, problem := range r.reconcileServiceConnections(ctx, sc, writes, owned, config, k.kind, k.servers) {
			logger.Error(errors.New(problem), "failed to configure connection during initialization", "service", k.kind)
			initErrors = append(initErrors, "init: "+problem)
		}
	}
	if config.Spec.Main != nil {
		if err := reconcileSeerrMainSettings(ctx, sc, writes, config.Spec.Main); err != nil {
			logger.Error(err, "failed to configure main settings during initialization")
			initErrors = append(initErrors, fmt.Sprintf("main settings init: %v", err))
		}
	}
	if len(initErrors) > 0 {
		return fmt.Errorf("initialization failed: %v", initErrors)
	}

	if _, err := sc.Post(ctx, "/api/v1/settings/initialize", map[string]any{}); err != nil {
		logger.Error(err, "failed to initialize Seerr")
		return fmt.Errorf("initialization: %w", err)
	}
	logger.Info("Seerr initialization completed")
	return nil
}

// reconcileServiceConnections updates each declared Sonarr or Radarr connection
// of the same name, or creates it. Setup uses it too: a setup that failed
// partway runs again from the start, and Seerr appends a connection on every POST.
func (r *SeerrReconciler) reconcileServiceConnections(
	ctx context.Context, sc *seerrclient.Client, w *engine.Writes, owned *engine.Ownership,
	config *requestsv1alpha1.Seerr, kind string, declared []requestsv1alpha1.SeerrServiceConnection,
) []string {
	existing, err := sc.GetList(ctx, "/api/v1/settings/"+kind)
	if err != nil {
		return []string{fmt.Sprintf("%s: listing connections: %v", kind, err)}
	}
	var problems []string
	for i := range declared {
		svc := &declared[i]
		if err := r.reconcileServiceConnection(ctx, sc, w, owned, config.Namespace, kind, svc, existing); err != nil {
			problems = append(problems, fmt.Sprintf("%s(%s): %v", kind, svc.Name, err))
		}
	}
	return problems
}

// seerrServiceCreateDefaults are values Seerr needs for a new server that a
// declared one may leave out. Seerr replaces a server whole on update, so an
// existing one is sent back with the declared values over it instead.
var seerrServiceCreateDefaults = map[string]map[string]any{
	"sonarr": {
		"useSsl": false, "activeProfileId": 0, "activeProfileName": "", "activeDirectory": "",
		"is4k": false, "isDefault": false, "enableSeasonFolders": false,
	},
	"radarr": {
		"useSsl": false, "activeProfileId": 0, "activeProfileName": "", "activeDirectory": "",
		"is4k": false, "isDefault": false, "minimumAvailability": "released",
	},
}

func (r *SeerrReconciler) reconcileServiceConnection(
	ctx context.Context, sc *seerrclient.Client, w *engine.Writes, owned *engine.Ownership,
	namespace, kind string, svc *requestsv1alpha1.SeerrServiceConnection, existing []map[string]any,
) error {
	resolved, tlsSpec, err := r.resolveService(ctx, namespace, *svc)
	if err != nil {
		return err
	}
	svc = &resolved
	apiKey, err := reconciler.ResolveSecretKeyRef(ctx, r.Client, namespace, *svc.APIKeySecretRef)
	if err != nil {
		return fmt.Errorf("resolving API key: %w", err)
	}
	payload, err := r.resolvedServicePayload(ctx, namespace, kind, svc, tlsSpec, apiKey)
	if err != nil {
		return fmt.Errorf("resolving profile: %w", err)
	}
	if svc.AppRef != nil {
		payload["baseUrl"] = svc.BaseURL
	}

	for _, e := range existing {
		if name, ok := e["name"].(string); ok && name == svc.Name {
			id, ok := e["id"].(float64)
			if !ok {
				continue
			}
			if !seerrDiffers(e, payload) {
				return nil
			}
			body, _, err := reconciler.MergeDesiredOverCurrent(e, payload)
			if err != nil {
				return err
			}
			// Seerr's API refuses a body that carries the read-only id.
			delete(body, "id")
			return w.Apply(kind, name, func() error {
				return sc.Put(ctx, fmt.Sprintf("/api/v1/settings/%s/%d", kind, int(id)), body)
			})
		}
	}

	body, _, err := reconciler.MergeDesiredOverCurrent(seerrServiceCreateDefaults[kind], payload)
	if err != nil {
		return err
	}
	return w.Apply(kind, svc.Name, func() error {
		if _, err := sc.Post(ctx, "/api/v1/settings/"+kind, body); err != nil {
			return err
		}
		owned.Created(kind, svc.Name)
		return nil
	})
}

func reconcileSeerrMediaServer(
	ctx context.Context, sc *seerrclient.Client, w *engine.Writes, media seerrMediaServer, mediaErr error, libraries []string,
) []string {
	if mediaErr != nil {
		return []string{mediaErr.Error()}
	}
	logger := log.FromContext(ctx)
	var syncErrors []string
	if media.jellyfin != nil {
		if err := reconcileSeerrJellyfin(ctx, sc, w, media.jellyfin); err != nil {
			logger.Error(err, "failed to reconcile Jellyfin server")
			syncErrors = append(syncErrors, fmt.Sprintf("jellyfin: %v", err))
		} else if err := reconcileSeerrLibraries(ctx, sc, w, "jellyfin", libraries); err != nil {
			syncErrors = append(syncErrors, fmt.Sprintf("jellyfin libraries: %v", err))
		}
	}
	if media.plex != nil {
		if err := reconcileSeerrPlex(ctx, sc, w, media.plex); err != nil {
			logger.Error(err, "failed to reconcile Plex server")
			syncErrors = append(syncErrors, fmt.Sprintf("plex: %v", err))
		} else if err := reconcileSeerrLibraries(ctx, sc, w, "plex", libraries); err != nil {
			syncErrors = append(syncErrors, fmt.Sprintf("plex libraries: %v", err))
		}
	}
	return syncErrors
}

// reconcileSeerrLibraries enables the declared libraries of Seerr's media
// server and disables the rest. A declared library Seerr doesn't list yet
// makes it sync the list from the server first.
func reconcileSeerrLibraries(ctx context.Context, sc *seerrclient.Client, w *engine.Writes, server string, declared []string) error {
	if len(declared) == 0 {
		return nil
	}
	path := "/api/v1/settings/" + server + "/library"
	libraries, err := sc.GetList(ctx, path)
	if err != nil {
		return fmt.Errorf("listing libraries: %w", err)
	}
	if len(missingLibraries(libraries, declared)) > 0 && !w.Observe {
		if libraries, err = sc.PostList(ctx, path+"/sync", map[string]any{}); err != nil {
			return fmt.Errorf("syncing libraries: %w", err)
		}
	}
	if missing := missingLibraries(libraries, declared); len(missing) > 0 {
		return fmt.Errorf("the media server has no library named %s", strings.Join(missing, ", "))
	}
	seen := map[string]bool{}
	for _, lib := range libraries {
		name, _ := lib["name"].(string)
		if seen[name] {
			return fmt.Errorf("the media server has more than one library named %q", name)
		}
		seen[name] = true
	}
	for _, lib := range libraries {
		name, _ := lib["name"].(string)
		id, _ := lib["id"].(string)
		enabled, _ := lib["enabled"].(bool)
		want := slices.Contains(declared, name)
		if id == "" || enabled == want {
			continue
		}
		if err := w.Apply("libraries", name, func() error {
			return sc.Put(ctx, path+"/"+id, map[string]any{"enabled": want})
		}); err != nil {
			return fmt.Errorf("library %q: %w", name, err)
		}
	}
	return nil
}

// missingLibraries lists, quoted, the declared names libraries doesn't have.
func missingLibraries(libraries []map[string]any, declared []string) []string {
	var missing []string
	for _, name := range declared {
		if !slices.ContainsFunc(libraries, func(lib map[string]any) bool { return lib["name"] == name }) {
			missing = append(missing, strconv.Quote(name))
		}
	}
	return missing
}

// resolveService returns svc with its appRef, if any, replaced by the
// referenced resource's address and API key Secret, and that resource's TLS
// settings.
func (r *SeerrReconciler) resolveService(
	ctx context.Context, namespace string, svc requestsv1alpha1.SeerrServiceConnection,
) (requestsv1alpha1.SeerrServiceConnection, *commonv1alpha1.TLSConfig, error) {
	var tlsSpec *commonv1alpha1.TLSConfig
	if svc.AppRef != nil {
		app, addr, err := ctrlcommon.ResolveAppAddress(ctx, r.Client, r.Apps, namespace, *svc.AppRef)
		if err != nil {
			return svc, nil, fmt.Errorf("appRef: %w", err)
		}
		svc.Hostname, svc.Port, svc.UseSSL, svc.BaseURL, svc.APIKeySecretRef = addr.Host, &addr.Port, &addr.TLS, addr.Base, app.APIKeySecretRef
		tlsSpec = app.TLS
	}
	if svc.APIKeySecretRef == nil {
		return svc, nil, fmt.Errorf("no API key Secret")
	}
	return svc, tlsSpec, nil
}

// seerrMediaServer is the declared media server, with its appRef followed.
type seerrMediaServer struct {
	jellyfin *requestsv1alpha1.SeerrJellyfinAuth
	plex     *requestsv1alpha1.SeerrPlexAuth
}

func (r *SeerrReconciler) mediaServer(ctx context.Context, config *requestsv1alpha1.Seerr) (seerrMediaServer, error) {
	var m seerrMediaServer
	if auth := config.Spec.JellyfinAuth; auth != (requestsv1alpha1.SeerrJellyfinAuth{}) {
		if auth.AppRef != nil {
			app, addr, err := ctrlcommon.ResolveAppAddress(ctx, r.Client, r.Apps, config.Namespace, *auth.AppRef)
			if err != nil {
				return m, fmt.Errorf("jellyfinAuth appRef: %w", err)
			}
			auth = requestsv1alpha1.SeerrJellyfinAuth{
				Hostname: addr.Host, Port: &addr.Port, UseSSL: &addr.TLS, URLBase: addr.Base,
				UsernameSecretRef: app.UsernameSecretRef, PasswordSecretRef: app.PasswordSecretRef,
			}
		}
		m.jellyfin = &auth
	}
	if auth := config.Spec.PlexAuth; auth != (requestsv1alpha1.SeerrPlexAuth{}) {
		if auth.AppRef != nil {
			app, addr, err := ctrlcommon.ResolveAppAddress(ctx, r.Client, r.Apps, config.Namespace, *auth.AppRef)
			if err != nil {
				return m, fmt.Errorf("plexAuth appRef: %w", err)
			}
			auth = requestsv1alpha1.SeerrPlexAuth{Hostname: addr.Host, Port: &addr.Port, UseSSL: &addr.TLS, TokenSecretRef: app.TokenSecretRef}
		}
		m.plex = &auth
	}
	return m, nil
}

func seerrAppRefs(o client.Object) []commonv1alpha1.AppRef {
	spec := o.(*requestsv1alpha1.Seerr).Spec
	var refs []commonv1alpha1.AppRef
	for _, svc := range append(append([]requestsv1alpha1.SeerrServiceConnection{}, spec.Sonarr...), spec.Radarr...) {
		if svc.AppRef != nil {
			refs = append(refs, *svc.AppRef)
		}
	}
	for _, ref := range []*commonv1alpha1.AppRef{spec.JellyfinAuth.AppRef, spec.PlexAuth.AppRef} {
		if ref != nil {
			refs = append(refs, *ref)
		}
	}
	return refs
}

func buildServicePayload(svc *requestsv1alpha1.SeerrServiceConnection, apiKey string) map[string]any {
	payload := map[string]any{
		"name":     svc.Name,
		"hostname": svc.Hostname,
		"apiKey":   apiKey,
	}
	if svc.Port != nil {
		payload["port"] = *svc.Port
	}
	if svc.UseSSL != nil {
		payload["useSsl"] = *svc.UseSSL
	}
	if svc.BaseURL != "" {
		payload["baseUrl"] = svc.BaseURL
	}
	if svc.ActiveProfileID != nil {
		payload["activeProfileId"] = *svc.ActiveProfileID
	}
	if svc.ActiveProfileName != "" {
		payload["activeProfileName"] = svc.ActiveProfileName
	}
	if svc.ActiveDirectory != "" {
		payload["activeDirectory"] = svc.ActiveDirectory
	}
	if svc.Is4k != nil {
		payload["is4k"] = *svc.Is4k
	}
	if svc.IsDefault != nil {
		payload["isDefault"] = *svc.IsDefault
	}
	if svc.SeriesType != "" {
		payload["seriesType"] = svc.SeriesType
	}
	if svc.MinimumAvailability != "" {
		payload["minimumAvailability"] = svc.MinimumAvailability
	}
	if svc.EnableSeasonFolders != nil {
		payload["enableSeasonFolders"] = *svc.EnableSeasonFolders
	}
	for key, value := range map[string]string{
		"externalUrl": svc.ExternalURL, "activeAnimeProfileName": svc.ActiveAnimeProfileName,
		"activeAnimeDirectory": svc.ActiveAnimeDirectory, "animeSeriesType": svc.AnimeSeriesType,
		"monitorNewItems": svc.MonitorNewItems,
	} {
		if value != "" {
			payload[key] = value
		}
	}
	for key, value := range map[string]*bool{"syncEnabled": svc.SyncEnabled, "preventSearch": svc.PreventSearch, "tagRequests": svc.TagRequests} {
		if value != nil {
			payload[key] = *value
		}
	}
	return payload
}

func reconcileSeerrMainSettings(ctx context.Context, sc *seerrclient.Client, w *engine.Writes, main *requestsv1alpha1.SeerrMain) error {
	payload := seerrMainPayload(main)
	if len(payload) == 0 {
		return nil
	}

	current, err := sc.Get(ctx, "/api/v1/settings/main")
	if err != nil {
		return fmt.Errorf("reading main settings: %w", err)
	}
	if !seerrDiffers(current, payload) {
		return nil
	}
	return w.Apply("settings", "main", func() error {
		_, err := sc.Post(ctx, "/api/v1/settings/main", payload)
		return err
	})
}

func seerrMainPayload(main *requestsv1alpha1.SeerrMain) map[string]any {
	payload := map[string]any{}
	for key, value := range map[string]string{
		"applicationTitle": main.ApplicationTitle, "applicationUrl": main.ApplicationURL, "locale": main.Locale,
		"youtubeUrl": main.YoutubeURL, "discoverRegion": main.DiscoverRegion, "streamingRegion": main.StreamingRegion,
		"blocklistRegion":  main.BlocklistRegion,
		"originalLanguage": strings.Join(main.OriginalLanguages, "|"), "blocklistLanguage": strings.Join(main.BlocklistLanguages, "|"),
	} {
		if value != "" {
			payload[key] = value
		}
	}
	for key, value := range map[string]*bool{
		"hideAvailable": main.HideAvailable, "hideRequested": main.HideRequested, "hideBlocklisted": main.HideBlocklisted,
		"localLogin": main.LocalLogin, "mediaServerLogin": main.MediaServerLogin, "newPlexLogin": main.NewMediaServerLogin,
		"partialRequestsEnabled": main.PartialRequestsEnabled, "enableSpecialEpisodes": main.EnableSpecialEpisodes,
		"cacheImages": main.CacheImages, "versionCheck": main.VersionCheck,
	} {
		if value != nil {
			payload[key] = *value
		}
	}
	if main.DefaultPermissions != nil {
		payload["defaultPermissions"] = *main.DefaultPermissions
	}
	if len(main.BlocklistedTags) > 0 {
		ids := make([]string, 0, len(main.BlocklistedTags))
		for _, id := range main.BlocklistedTags {
			ids = append(ids, strconv.Itoa(int(id)))
		}
		payload["blocklistedTags"] = strings.Join(ids, ",")
	}
	if main.BlocklistedTagsLimit != 0 {
		payload["blocklistedTagsLimit"] = main.BlocklistedTagsLimit
	}
	quotas := map[string]any{}
	for kind, q := range map[string]requestsv1alpha1.SeerrQuota{"movie": main.DefaultQuotas.Movie, "tv": main.DefaultQuotas.TV} {
		quota := map[string]any{}
		if q.Limit != nil {
			quota["quotaLimit"] = *q.Limit
		}
		if q.Days != nil {
			quota["quotaDays"] = *q.Days
		}
		if len(quota) > 0 {
			quotas[kind] = quota
		}
	}
	if len(quotas) > 0 {
		payload["defaultQuotas"] = quotas
	}
	return payload
}

func reconcileSeerrNetwork(ctx context.Context, sc *seerrclient.Client, w *engine.Writes, network requestsv1alpha1.SeerrNetwork) error {
	if network.TrustProxy == nil {
		return nil
	}
	payload := map[string]any{"trustProxy": *network.TrustProxy}
	current, err := sc.Get(ctx, "/api/v1/settings/network")
	if err != nil {
		return fmt.Errorf("reading network settings: %w", err)
	}
	if !seerrDiffers(current, payload) {
		return nil
	}
	return w.Apply("settings", "network", func() error {
		_, err := sc.Post(ctx, "/api/v1/settings/network", payload)
		return err
	})
}

// reconcileSeerrMetadataProviders sends both providers, as Seerr saves the pair
// from what it is sent.
func reconcileSeerrMetadataProviders(
	ctx context.Context, sc *seerrclient.Client, w *engine.Writes, providers requestsv1alpha1.SeerrMetadataProviders,
) error {
	payload := map[string]any{}
	for key, value := range map[string]string{"tv": providers.TV, "anime": providers.Anime} {
		if value != "" {
			payload[key] = value
		}
	}
	if len(payload) == 0 {
		return nil
	}
	current, err := sc.Get(ctx, "/api/v1/settings/metadatas")
	if err != nil {
		return fmt.Errorf("reading metadata settings: %w", err)
	}
	if !seerrDiffers(current, payload) {
		return nil
	}
	body, _, err := reconciler.MergeDesiredOverCurrent(current, payload)
	if err != nil {
		return err
	}
	return w.Apply("settings", "metadataProviders", func() error {
		if err := sc.Put(ctx, "/api/v1/settings/metadatas", body); err != nil {
			return fmt.Errorf("the providers chosen weren't reachable from Seerr, which kept its own: %w", err)
		}
		return nil
	})
}

// reconcileSeerrJobSchedules reschedules the declared jobs whose schedules
// differ. Seerr has only the jobs it registered when it started: the Plex or
// the Jellyfin ones, for its media server.
func reconcileSeerrJobSchedules(
	ctx context.Context, sc *seerrclient.Client, w *engine.Writes, schedules []requestsv1alpha1.SeerrJobSchedule,
) []string {
	if len(schedules) == 0 {
		return nil
	}
	jobs, err := sc.GetList(ctx, "/api/v1/settings/jobs")
	if err != nil {
		return []string{fmt.Sprintf("jobSchedules: reading jobs: %v", err)}
	}
	current := map[string]any{}
	for _, job := range jobs {
		if id, ok := job["id"].(string); ok {
			current[id] = job["cronSchedule"]
		}
	}
	var errs []string
	for _, js := range schedules {
		schedule, ok := current[js.ID]
		switch {
		case !ok:
			errs = append(errs, fmt.Sprintf("jobSchedule(%s): Seerr runs no such job; it runs %s",
				js.ID, strings.Join(slices.Sorted(maps.Keys(current)), ", ")))
		case schedule != js.Schedule:
			err := w.Apply("jobs", js.ID, func() error {
				_, err := sc.Post(ctx, "/api/v1/settings/jobs/"+js.ID+"/schedule", map[string]any{"schedule": js.Schedule})
				return err
			})
			if err != nil {
				errs = append(errs, fmt.Sprintf("jobSchedule(%s): %v", js.ID, err))
			}
		}
	}
	return errs
}

func reconcileSeerrNotificationAgent(
	ctx context.Context, sc *seerrclient.Client, w *engine.Writes, agent requestsv1alpha1.SeerrNotificationAgent, options map[string]any,
) error {
	payload := map[string]any{}
	if agent.Enabled != nil {
		payload["enabled"] = *agent.Enabled
	}
	if agent.Types != nil {
		types := 0
		for _, name := range agent.Types {
			bit, ok := seerrclient.NotificationTypes[name]
			if !ok {
				return fmt.Errorf("unknown notification type %q", name)
			}
			types |= bit
		}
		payload["types"] = types
	}
	if len(options) > 0 {
		payload["options"] = options
	}

	path := fmt.Sprintf("/api/v1/settings/notifications/%s", agent.Agent)
	current, err := sc.Get(ctx, path)
	if err != nil {
		return fmt.Errorf("reading %s notifications: %w", agent.Agent, err)
	}
	if !seerrDiffers(current, payload) {
		return nil
	}
	body, _, err := reconciler.MergeDesiredOverCurrent(current, payload)
	if err != nil {
		return err
	}
	return w.Apply("notifications", agent.Agent, func() error {
		_, err := sc.Post(ctx, path, body)
		return err
	})
}

// seerrDiffers reports whether Seerr's stored settings lack any declared value.
// Writing every section on every pass made each reconcile a burst of saves.
func seerrDiffers(current, payload map[string]any) bool {
	raw, err := json.Marshal(payload)
	if err != nil {
		return true
	}
	var desired any
	if err := json.Unmarshal(raw, &desired); err != nil {
		return true
	}
	return !reconciler.Covers(current, desired)
}

// SetupWithManager sets up the controller with the Manager.
func (r *SeerrReconciler) SetupWithManager(mgr ctrl.Manager) error {
	b := ctrl.NewControllerManagedBy(mgr).
		For(&requestsv1alpha1.Seerr{}, builder.WithPredicates(ctrlcommon.ConfigChangedPredicate())).
		WatchesMetadata(&corev1.Secret{}, ctrlcommon.EnqueueForSecretThroughRefs(r.Client, r.Apps, &requestsv1alpha1.SeerrList{}, seerrAppRefs))
	b, err := ctrlcommon.WatchAppRefs(mgr, b, r.Apps, &requestsv1alpha1.Seerr{}, &requestsv1alpha1.SeerrList{}, seerrAppRefs, "Sonarr", "Radarr", "Jellyfin", "Plex")
	if err != nil {
		return err
	}
	return b.
		Named("seerr").
		Complete(r)
}

func syncJellyfinLibraries(
	ctx context.Context, sc *seerrclient.Client, w *engine.Writes, auth *requestsv1alpha1.SeerrJellyfinAuth, libraries []string,
) error {
	if err := reconcileSeerrJellyfin(ctx, sc, w, auth); err != nil {
		return err
	}
	return setUpLibraries(ctx, sc, w, "jellyfin", libraries)
}

// reconcileSeerrJellyfin keeps Seerr's Jellyfin server at the declared address.
// Seerr stores the host as "ip". A "hostname" key is kept verbatim and, on the
// next start, Seerr's settings migration parses it as a URL and replaces the
// port with 80 or 443.
func reconcileSeerrJellyfin(ctx context.Context, sc *seerrclient.Client, w *engine.Writes, auth *requestsv1alpha1.SeerrJellyfinAuth) error {
	port := 0
	if auth.Port != nil {
		port = int(*auth.Port)
	}
	payload := map[string]any{
		"ip":      auth.Hostname,
		"port":    port,
		"useSsl":  auth.UseSSL != nil && *auth.UseSSL,
		"urlBase": auth.URLBase,
	}
	current, err := sc.Get(ctx, "/api/v1/settings/jellyfin")
	if err != nil {
		return fmt.Errorf("reading Jellyfin settings: %w", err)
	}
	if !seerrDiffers(current, payload) {
		return nil
	}
	return w.Apply("settings", "jellyfin", func() error {
		_, err := sc.Post(ctx, "/api/v1/settings/jellyfin", payload)
		return err
	})
}

func syncPlexLibraries(
	ctx context.Context, sc *seerrclient.Client, w *engine.Writes, auth *requestsv1alpha1.SeerrPlexAuth, libraries []string,
) error {
	if err := reconcileSeerrPlex(ctx, sc, w, auth); err != nil {
		return err
	}
	return setUpLibraries(ctx, sc, w, "plex", libraries)
}

// setUpLibraries enables the declared libraries during first-time setup, or
// every library when none are declared.
func setUpLibraries(ctx context.Context, sc *seerrclient.Client, w *engine.Writes, server string, libraries []string) error {
	if len(libraries) == 0 {
		return enableLibraries(ctx, sc, server)
	}
	if _, err := sc.PostList(ctx, fmt.Sprintf("/api/v1/settings/%s/library/sync", server), map[string]any{}); err != nil {
		return fmt.Errorf("%s library sync: %w", engine.AppName(server), err)
	}
	return reconcileSeerrLibraries(ctx, sc, w, server, libraries)
}

// reconcileSeerrPlex keeps Seerr's Plex server at the declared address. Seerr
// connects to it to check the address before saving.
func reconcileSeerrPlex(ctx context.Context, sc *seerrclient.Client, w *engine.Writes, auth *requestsv1alpha1.SeerrPlexAuth) error {
	port := 32400
	if auth.Port != nil {
		port = int(*auth.Port)
	}
	payload := map[string]any{
		"ip":     auth.Hostname,
		"port":   port,
		"useSsl": auth.UseSSL != nil && *auth.UseSSL,
	}
	current, err := sc.Get(ctx, "/api/v1/settings/plex")
	if err != nil {
		return fmt.Errorf("reading Plex settings: %w", err)
	}
	if !seerrDiffers(current, payload) {
		return nil
	}
	return w.Apply("settings", "plex", func() error {
		_, err := sc.Post(ctx, "/api/v1/settings/plex", payload)
		return err
	})
}

// enableLibraries refreshes Seerr's library list from its media server and
// enables each library. Seerr 3.5.0 replaced the old sync/enable query
// parameters on GET .../library with these endpoints, and its request validator
// rejects the old form. The sync answers 404 while the server has no libraries;
// setup then retries rather than finishing with none enabled.
func enableLibraries(ctx context.Context, sc *seerrclient.Client, server string) error {
	libraries, err := sc.PostList(ctx, fmt.Sprintf("/api/v1/settings/%s/library/sync", server), map[string]any{})
	if err != nil {
		return fmt.Errorf("%s library sync: %w", engine.AppName(server), err)
	}
	for _, lib := range libraries {
		id, ok := lib["id"].(string)
		if !ok || id == "" {
			continue
		}
		if err := sc.Put(ctx, fmt.Sprintf("/api/v1/settings/%s/library/%s", server, id), map[string]any{"enabled": true}); err != nil {
			return fmt.Errorf("enabling %s library %s: %w", engine.AppName(server), id, err)
		}
	}
	return nil
}

func (r *SeerrReconciler) establishSession(ctx context.Context, sc *seerrclient.Client, config *requestsv1alpha1.Seerr, media seerrMediaServer) error {
	if ref := config.Spec.Connection.APIKeySecretRef; ref != nil {
		apiKey, err := reconciler.ResolveSecretKeyRef(ctx, r.Client, config.Namespace, *ref)
		if err != nil {
			return err
		}
		sc.SetAPIKey(apiKey)
		if _, err := sc.GetAPIKey(ctx); err != nil {
			return fmt.Errorf("checking the API key: %w", err)
		}
		return nil
	}
	if err := r.authenticate(ctx, sc, config.Namespace, media); err != nil {
		return err
	}
	apiKey, err := sc.GetAPIKey(ctx)
	if err != nil {
		return fmt.Errorf("get api key: %w", err)
	}
	sc.SetAPIKey(apiKey)
	return nil
}

func (r *SeerrReconciler) authenticate(ctx context.Context, sc *seerrclient.Client, namespace string, media seerrMediaServer) error {
	if plex := media.plex; plex != nil {
		if plex.TokenSecretRef == nil {
			return fmt.Errorf("plexAuth has no token Secret")
		}
		plexToken, err := reconciler.ResolveSecretKeyRef(ctx, r.Client, namespace, *plex.TokenSecretRef)
		if err != nil {
			return fmt.Errorf("resolving Plex token: %w", err)
		}
		return sc.AuthenticatePlex(ctx, plexToken)
	}
	if jf := media.jellyfin; jf != nil {
		if jf.UsernameSecretRef == nil || jf.PasswordSecretRef == nil {
			return fmt.Errorf("jellyfinAuth has no username or password Secret")
		}
		jfUsername, err := reconciler.ResolveSecretKeyRef(ctx, r.Client, namespace, *jf.UsernameSecretRef)
		if err != nil {
			return fmt.Errorf("resolving Jellyfin username: %w", err)
		}
		jfPassword, err := reconciler.ResolveSecretKeyRef(ctx, r.Client, namespace, *jf.PasswordSecretRef)
		if err != nil {
			return fmt.Errorf("resolving Jellyfin password: %w", err)
		}
		jfPort := 0
		if jf.Port != nil {
			jfPort = int(*jf.Port)
		}
		useSsl := jf.UseSSL != nil && *jf.UseSSL
		configured, err := sc.MediaServerConfigured(ctx)
		if err != nil {
			return fmt.Errorf("reading media server state: %w", err)
		}
		return sc.AuthenticateJellyfin(ctx, jfUsername, jfPassword, jf.Hostname, jfPort, jf.URLBase, useSsl, configured)
	}
	return fmt.Errorf("either jellyfinAuth or plexAuth must be configured")
}
