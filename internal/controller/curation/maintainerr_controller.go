package curation

import (
	"context"
	"fmt"
	"slices"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	curationv1alpha1 "github.com/kyleseneker/arrchitect/api/curation/v1alpha1"
	maintainerrclient "github.com/kyleseneker/arrchitect/internal/client/maintainerr"
	ctrlcommon "github.com/kyleseneker/arrchitect/internal/controller/common"
	"github.com/kyleseneker/arrchitect/internal/engine"
	"github.com/kyleseneker/arrchitect/internal/reconciler"
)

const (
	maintainerrApp = "maintainerr"
	mediaPlex      = "plex"
	mediaJellyfin  = "jellyfin"
)

// MaintainerrReconciler reconciles a Maintainerr object.
type MaintainerrReconciler struct {
	client.Client
	Recorder events.EventRecorder
	Apps     ctrlcommon.Apps
}

// +kubebuilder:rbac:groups=arrchitect.dev,resources=maintainerrs,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=arrchitect.dev,resources=maintainerrs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=arrchitect.dev,resources=maintainerrs/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=arrchitect.dev,resources=sonarrs;radarrs;seerrs;plexes;jellyfins,verbs=get;list;watch

func (r *MaintainerrReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	integration := ctrlcommon.Integration[*curationv1alpha1.Maintainerr, *maintainerrclient.Client]{
		App:     maintainerrApp,
		Connect: r.connect,
		Check: func(ctx context.Context, mc *maintainerrclient.Client, _ *curationv1alpha1.Maintainerr) error {
			return mc.Ping(ctx)
		},
		Sync: func(ctx context.Context, mc *maintainerrclient.Client, config *curationv1alpha1.Maintainerr, pass *engine.Pass) []error {
			sync := &maintainerrSync{Pass: pass, c: r.Client, apps: r.Apps, mc: mc, namespace: config.Namespace}
			return sync.run(ctx, &config.Spec)
		},
		Cleanup: func(ctx context.Context, mc *maintainerrclient.Client, owned *engine.Ownership) error {
			return (&maintainerrSync{Pass: &engine.Pass{Owned: owned}, mc: mc}).cleanup(ctx)
		},
		Prune: true,
	}
	return integration.Reconcile(ctx, r.Client, r.Recorder, req, &curationv1alpha1.Maintainerr{})
}

func (r *MaintainerrReconciler) connect(ctx context.Context, config *curationv1alpha1.Maintainerr) (*maintainerrclient.Client, error) {
	tlsCfg, err := engine.ResolveTLSConfig(ctx, r.Client, config.Namespace, config.Spec.Connection.TLS)
	if err != nil {
		return nil, err
	}
	// Maintainerr has no API authentication.
	hc, err := engine.NewHTTPClient(config.Spec.Connection.URL, engine.AuthNone,
		engine.WithTLSConfig(tlsCfg), engine.WithAppLabel(maintainerrApp))
	if err != nil {
		return nil, err
	}
	return maintainerrclient.NewClient(hc), nil
}

// maintainerrSync applies one Maintainerr in dependency order: the media
// server and the Sonarr and Radarr servers before the rule groups that refer to
// them, since Maintainerr resolves rule properties only for configured apps.
// With observe set it only reads, and records what it would have written.
type maintainerrSync struct {
	*engine.Pass
	c         client.Reader
	apps      ctrlcommon.Apps
	mc        *maintainerrclient.Client
	namespace string
}

func (s *maintainerrSync) created(kind, name string, create func() error) func() error {
	return func() error {
		if err := create(); err != nil {
			return err
		}
		s.Owned.Created(kind, name)
		return nil
	}
}

func (s *maintainerrSync) run(ctx context.Context, spec *curationv1alpha1.MaintainerrSpec) []error {
	logger := log.FromContext(ctx)
	var syncErrors []error
	failed := map[string]bool{}
	fail := func(section string, err error) {
		logger.Error(err, "failed to reconcile", "section", section)
		syncErrors = append(syncErrors, fmt.Errorf("%s: %w", section, err))
		failed[section] = true
	}

	settings, err := s.mc.GetSettings(ctx)
	if err != nil {
		fail("settings", fmt.Errorf("reading settings: %w", err))
		return syncErrors
	}

	if spec.MediaServer != nil {
		if err := s.mediaServer(ctx, spec.MediaServer, settings); err != nil {
			fail("mediaServer", err)
		}
	}

	servers := map[maintainerrclient.Arr]engine.NamedIDs{}
	for _, arr := range []struct {
		kind     maintainerrclient.Arr
		declared []curationv1alpha1.MaintainerrArrServer
	}{{maintainerrclient.Sonarr, spec.Sonarr}, {maintainerrclient.Radarr, spec.Radarr}} {
		ids, err := s.arrServers(ctx, arr.kind, arr.declared)
		if err != nil {
			fail(string(arr.kind), err)
		}
		servers[arr.kind] = ids
	}

	if spec.Seerr != (curationv1alpha1.MaintainerrSeerrConnection{}) {
		conn, err := s.seerrConnection(ctx, spec.Seerr)
		if err == nil {
			err = s.service(ctx, maintainerrclient.Seerr, &conn)
		}
		if err != nil {
			fail(string(maintainerrclient.Seerr), err)
		}
	}
	if spec.Tautulli != (curationv1alpha1.MaintainerrServiceConnection{}) {
		if err := s.service(ctx, maintainerrclient.Tautulli, &spec.Tautulli); err != nil {
			fail(string(maintainerrclient.Tautulli), err)
		}
	}

	if spec.Jobs != nil {
		if err := s.jobs(ctx, spec.Jobs, settings); err != nil {
			fail("jobs", err)
		}
	}

	var notifications engine.NamedIDs
	if len(spec.Notifications) > 0 || slices.ContainsFunc(spec.RuleGroups, func(g curationv1alpha1.MaintainerrRuleGroup) bool {
		return g.Notifications != nil
	}) {
		ids, errs := s.notifications(ctx, spec.Notifications)
		syncErrors = append(syncErrors, errs...)
		failed[notificationsKind] = len(errs) > 0
		notifications = ids
	}

	if len(spec.RuleGroups) > 0 {
		errs := s.ruleGroups(ctx, spec.RuleGroups, servers, notifications)
		syncErrors = append(syncErrors, errs...)
		failed[ruleGroupsKind] = len(errs) > 0
	}
	if s.PruneEnabled {
		syncErrors = append(syncErrors, s.pruneUndeclared(ctx, spec, failed)...)
	}
	return syncErrors
}

// mediaServer sets the media server on a fresh Maintainerr and keeps its
// connection current. It never changes a media server that is already set:
// Maintainerr deletes every collection when it switches.
func (s *maintainerrSync) mediaServer(ctx context.Context, ms *curationv1alpha1.MaintainerrMediaServer, settings map[string]any) error {
	want := mediaPlex
	if ms.Jellyfin != (curationv1alpha1.MaintainerrJellyfin{}) {
		want = mediaJellyfin
	}
	current, _ := settings["media_server_type"].(string)
	if current != "" && current != want {
		return fmt.Errorf("Maintainerr uses %s; switching it to %s deletes every collection, "+
			"so the operator leaves that to Maintainerr's settings page", engine.AppName(current), engine.AppName(want))
	}
	if ms.Jellyfin != (curationv1alpha1.MaintainerrJellyfin{}) {
		return s.jellyfin(ctx, &ms.Jellyfin)
	}
	plex := ms.Plex
	if plex.AppRef != nil {
		app, addr, err := ctrlcommon.ResolveAppAddress(ctx, s.c, s.apps, s.namespace, *plex.AppRef)
		if err != nil {
			return fmt.Errorf("Plex appRef: %w", err)
		}
		plex = curationv1alpha1.MaintainerrPlex{Hostname: addr.Host, Port: addr.Port, SSL: &addr.TLS, TokenSecretRef: app.TokenSecretRef}
	}
	return s.plex(ctx, &plex, settings, current == "")
}

func (s *maintainerrSync) jellyfin(ctx context.Context, jf *curationv1alpha1.MaintainerrJellyfin) error {
	if jf.AppRef != nil {
		app, err := ctrlcommon.ResolveAppRef(ctx, s.c, s.apps, s.namespace, *jf.AppRef)
		if err != nil {
			return fmt.Errorf("Jellyfin appRef: %w", err)
		}
		if app.APIKeySecretRef == nil {
			return ctrlcommon.InvalidAppRef("Jellyfin appRef: Jellyfin %q sets no API key Secret", jf.AppRef.Name)
		}
		resolved := *jf
		resolved.URL, resolved.APIKeySecretRef = app.URL, app.APIKeySecretRef
		jf = &resolved
	}
	if jf.APIKeySecretRef == nil {
		return fmt.Errorf("no Jellyfin API key Secret")
	}
	key, err := reconciler.ResolveSecretKeyRef(ctx, s.c, s.namespace, *jf.APIKeySecretRef)
	if err != nil {
		return fmt.Errorf("resolving Jellyfin API key: %w", err)
	}
	stored, err := s.mc.GetJellyfin(ctx)
	if err != nil {
		return fmt.Errorf("reading Jellyfin connection: %w", err)
	}
	url := serviceURL(jf.URL)
	if stored["jellyfin_url"] == url && stored["jellyfin_api_key"] == key &&
		(jf.UserID == "" || stored["jellyfin_user_id"] == jf.UserID) {
		return nil
	}
	conn := map[string]any{"jellyfin_url": url, "jellyfin_api_key": key}
	if jf.UserID != "" {
		conn["jellyfin_user_id"] = jf.UserID
	}
	return s.Apply("mediaServer", mediaJellyfin, func() error { return s.mc.SaveJellyfin(ctx, conn) })
}

func (s *maintainerrSync) plex(ctx context.Context, plex *curationv1alpha1.MaintainerrPlex, settings map[string]any, fresh bool) error {
	if plex.TokenSecretRef == nil {
		return fmt.Errorf("no Plex token Secret")
	}
	token, err := reconciler.ResolveSecretKeyRef(ctx, s.c, s.namespace, *plex.TokenSecretRef)
	if err != nil {
		return fmt.Errorf("resolving Plex token: %w", err)
	}
	if fresh {
		err := s.Apply("mediaServer", "type", func() error { return s.mc.SetMediaServerType(ctx, mediaPlex) })
		if err != nil {
			return fmt.Errorf("setting Plex as the media server: %w", err)
		}
	}
	// Maintainerr reads the token back masked only, and refuses server fields
	// until a token is stored, so the token goes first.
	if stored, _ := settings["plex_auth_token"].(string); stored != maskPlexToken(token) {
		err := s.Apply("mediaServer", "plexToken", func() error { return s.mc.SavePlexToken(ctx, token) })
		if err != nil {
			return fmt.Errorf("saving Plex token: %w", err)
		}
	}
	desired := plexFields(plex)
	if reconciler.Covers(settings, mustJSON(desired)) {
		return nil
	}
	return s.Apply("mediaServer", mediaPlex, func() error { return s.mc.UpdateSettings(ctx, desired) })
}

// plexFields renders the Plex server settings as Maintainerr stores them: a
// lowercase host without a scheme, SSL forced on for port 443, and manual mode
// on so Maintainerr does not swap in an address it discovers through plex.tv.
func plexFields(plex *curationv1alpha1.MaintainerrPlex) map[string]any {
	host := strings.ToLower(plex.Hostname)
	host = strings.TrimPrefix(strings.TrimPrefix(host, "https://"), "http://")
	port := plex.Port
	if port == 0 {
		port = 32400
	}
	ssl := 0
	if (plex.SSL != nil && *plex.SSL) || port == 443 {
		ssl = 1
	}
	return map[string]any{
		"plex_hostname": host, "plex_port": port, "plex_ssl": ssl, "plex_manual_mode": 1,
	}
}

// maskPlexToken reproduces how Maintainerr masks a secret on read: up to six
// characters become "****", longer ones keep their first and last three.
func maskPlexToken(token string) string {
	switch {
	case token == "":
		return ""
	case len(token) <= 6:
		return "****"
	}
	return token[:3] + "..." + token[len(token)-3:]
}

// arrServers adds or updates the declared servers of a kind and returns every
// server's id by name, for rule groups to refer to.
func (s *maintainerrSync) arrServers(
	ctx context.Context, kind maintainerrclient.Arr, declared []curationv1alpha1.MaintainerrArrServer,
) (engine.NamedIDs, error) {
	stored, err := s.mc.ListArr(ctx, kind)
	if err != nil {
		return nil, fmt.Errorf("listing servers: %w", err)
	}
	added := false
	var errs []error
	for _, server := range declared {
		wasAdded, err := s.arrServer(ctx, kind, server, stored)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", server.Name, err))
		}
		added = added || wasAdded
	}
	if added {
		if stored, err = s.mc.ListArr(ctx, kind); err != nil {
			return nil, fmt.Errorf("listing servers: %w", err)
		}
	}
	ids := engine.NamedIDsOf(stored, "serverName")
	if len(errs) > 0 {
		return ids, engine.JoinErrors(errs...)
	}
	return ids, nil
}

func (s *maintainerrSync) arrServer(
	ctx context.Context, kind maintainerrclient.Arr, server curationv1alpha1.MaintainerrArrServer, stored []map[string]any,
) (bool, error) {
	if server.AppRef != nil {
		app, err := ctrlcommon.ResolveAppRef(ctx, s.c, s.apps, s.namespace, *server.AppRef)
		if err != nil {
			return false, fmt.Errorf("appRef: %w", err)
		}
		server.URL, server.APIKeySecretRef = app.URL, app.APIKeySecretRef
	}
	if server.APIKeySecretRef == nil {
		return false, fmt.Errorf("no API key Secret")
	}
	key, err := reconciler.ResolveSecretKeyRef(ctx, s.c, s.namespace, *server.APIKeySecretRef)
	if err != nil {
		return false, fmt.Errorf("resolving API key: %w", err)
	}
	// Maintainerr lowercases the URL it stores.
	desired := map[string]any{"serverName": server.Name, "url": strings.ToLower(serviceURL(server.URL)), "apiKey": key}
	if server.TagExclusions != nil {
		desired["tagExclusions"] = *server.TagExclusions
	}
	if server.ExclusionTag != "" {
		desired["exclusionTag"] = server.ExclusionTag
	}
	if server.UntagOnUnexclude != nil {
		desired["untagOnUnexclude"] = *server.UntagOnUnexclude
	}

	var match map[string]any
	for _, e := range stored {
		if e["serverName"] != server.Name {
			continue
		}
		if match != nil {
			return false, fmt.Errorf("Maintainerr has more than one %s server named %q", engine.AppName(string(kind)), server.Name)
		}
		match = e
	}
	if match == nil {
		add := s.created(string(kind), server.Name, func() error { return s.mc.AddArr(ctx, kind, desired) })
		return !s.Observe, s.Apply(string(kind), server.Name, add)
	}
	if reconciler.Covers(match, mustJSON(desired)) {
		return false, nil
	}
	id, ok := match["id"].(float64)
	if !ok {
		return false, fmt.Errorf("server %q has no valid id", server.Name)
	}
	return false, s.Apply(string(kind), server.Name, func() error { return s.mc.UpdateArr(ctx, kind, int(id), desired) })
}

func (s *maintainerrSync) service(
	ctx context.Context, svc maintainerrclient.Service, conn *curationv1alpha1.MaintainerrServiceConnection,
) error {
	key, err := reconciler.ResolveSecretKeyRef(ctx, s.c, s.namespace, conn.APIKeySecretRef)
	if err != nil {
		return fmt.Errorf("resolving API key: %w", err)
	}
	stored, err := s.mc.GetService(ctx, svc)
	if err != nil {
		return fmt.Errorf("reading connection: %w", err)
	}
	desired := map[string]any{"url": serviceURL(conn.URL), "api_key": key}
	if reconciler.Covers(stored, mustJSON(desired)) {
		return nil
	}
	return s.Apply(string(svc), string(svc), func() error { return s.mc.SaveService(ctx, svc, desired) })
}

// seerrConnection returns the Seerr connection, following its appRef.
func (s *maintainerrSync) seerrConnection(
	ctx context.Context, conn curationv1alpha1.MaintainerrSeerrConnection,
) (curationv1alpha1.MaintainerrServiceConnection, error) {
	if conn.AppRef != nil {
		app, err := ctrlcommon.ResolveAppRef(ctx, s.c, s.apps, s.namespace, *conn.AppRef)
		if err != nil {
			return curationv1alpha1.MaintainerrServiceConnection{}, fmt.Errorf("appRef: %w", err)
		}
		if app.APIKeySecretRef == nil {
			return curationv1alpha1.MaintainerrServiceConnection{}, ctrlcommon.InvalidAppRef("appRef: Seerr %q sets no API key Secret", conn.AppRef.Name)
		}
		conn.URL, conn.APIKeySecretRef = app.URL, app.APIKeySecretRef
	}
	if conn.APIKeySecretRef == nil {
		return curationv1alpha1.MaintainerrServiceConnection{}, fmt.Errorf("no API key Secret")
	}
	return curationv1alpha1.MaintainerrServiceConnection{URL: conn.URL, APIKeySecretRef: *conn.APIKeySecretRef}, nil
}

func maintainerrAppRefs(o client.Object) []commonv1alpha1.AppRef {
	spec := o.(*curationv1alpha1.Maintainerr).Spec
	var refs []commonv1alpha1.AppRef
	for _, server := range append(append([]curationv1alpha1.MaintainerrArrServer{}, spec.Sonarr...), spec.Radarr...) {
		if server.AppRef != nil {
			refs = append(refs, *server.AppRef)
		}
	}
	if spec.Seerr.AppRef != nil {
		refs = append(refs, *spec.Seerr.AppRef)
	}
	if ms := spec.MediaServer; ms != nil {
		for _, ref := range []*commonv1alpha1.AppRef{ms.Plex.AppRef, ms.Jellyfin.AppRef} {
			if ref != nil {
				refs = append(refs, *ref)
			}
		}
	}
	return refs
}

func (s *maintainerrSync) jobs(ctx context.Context, jobs *curationv1alpha1.MaintainerrJobs, settings map[string]any) error {
	desired := buildMaintainerrJobs(jobs)
	if len(desired) == 0 || reconciler.Covers(settings, mustJSON(desired)) {
		return nil
	}
	return s.Apply("jobs", "jobs", func() error { return s.mc.UpdateSettings(ctx, desired) })
}

// buildMaintainerrJobs renders the job schedules under Maintainerr's names.
func buildMaintainerrJobs(jobs *curationv1alpha1.MaintainerrJobs) map[string]any {
	desired := map[string]any{}
	if jobs.Rules != "" {
		desired["rules_handler_job_cron"] = jobs.Rules
	}
	if jobs.Collections != "" {
		desired["collection_handler_job_cron"] = jobs.Collections
	}
	return desired
}

// serviceURL is a URL as Maintainerr stores it: trimmed, without trailing slashes.
func serviceURL(url string) string {
	return strings.TrimRight(strings.TrimSpace(url), "/")
}

// SetupWithManager sets up the controller with the Manager.
func (r *MaintainerrReconciler) SetupWithManager(mgr ctrl.Manager) error {
	b := ctrl.NewControllerManagedBy(mgr).
		For(&curationv1alpha1.Maintainerr{}, builder.WithPredicates(ctrlcommon.ConfigChangedPredicate())).
		WatchesMetadata(&corev1.Secret{}, ctrlcommon.EnqueueForSecretThroughRefs(r.Client, r.Apps, &curationv1alpha1.MaintainerrList{}, maintainerrAppRefs))
	b, err := ctrlcommon.WatchAppRefs(mgr, b, r.Apps, &curationv1alpha1.Maintainerr{}, &curationv1alpha1.MaintainerrList{}, maintainerrAppRefs, "Sonarr", "Radarr", "Seerr", "Plex", "Jellyfin")
	if err != nil {
		return err
	}
	return b.
		Named("maintainerr").
		Complete(r)
}
