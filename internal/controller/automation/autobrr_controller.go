package automation

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	automationv1alpha1 "github.com/kyleseneker/arrchitect/api/automation/v1alpha1"
	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	autobrrclient "github.com/kyleseneker/arrchitect/internal/client/autobrr"
	ctrlcommon "github.com/kyleseneker/arrchitect/internal/controller/common"
	"github.com/kyleseneker/arrchitect/internal/engine"
	"github.com/kyleseneker/arrchitect/internal/reconciler"
)

// AutobrrReconciler reconciles an Autobrr object.
type AutobrrReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder events.EventRecorder
	Apps     ctrlcommon.Apps
}

// +kubebuilder:rbac:groups=arrchitect.dev,resources=autobrrs,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=arrchitect.dev,resources=autobrrs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=arrchitect.dev,resources=autobrrs/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=arrchitect.dev,resources=qbittorrents;sabnzbds;sonarrs;radarrs;lidarrs;readarrs,verbs=get;list;watch

func (r *AutobrrReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var config automationv1alpha1.Autobrr
	if err := r.Get(ctx, req.NamespacedName, &config); err != nil {
		return ctrl.Result{}, ctrlcommon.IgnoreDeleted(&config, req.NamespacedName, err)
	}

	cleanup := func(ctx context.Context) error {
		ac, _, err := r.autobrrClient(ctx, &config)
		if err != nil {
			return err
		}
		return (&autobrrSync{ac: ac, owned: engine.NewOwnership("autobrr", config.GetManagedResources())}).cleanup(ctx)
	}
	lifecycle := ctrlcommon.Lifecycle{Remove: cleanup, Prune: true}
	if done, after := ctrlcommon.HandleLifecycle(ctx, r.Client, r.Recorder, &config, lifecycle); done {
		return ctrl.Result{RequeueAfter: after}, nil
	}

	ac, reason, err := r.autobrrClient(ctx, &config)
	if err != nil {
		logger.Error(err, "failed to build the Autobrr client")
		ctrlcommon.UpdateStatusUnreachable(ctx, r.Client, &config, reason, err.Error())
		if reason == engine.ReasonInvalidConfig {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	if err := ac.Ping(ctx); err != nil {
		logger.Error(err, "Autobrr unreachable")
		ctrlcommon.UpdateStatusUnreachable(ctx, r.Client, &config, engine.ReasonAppUnreachable, err.Error())
		return ctrl.Result{RequeueAfter: 60 * time.Second}, nil
	}

	sync := &autobrrSync{
		App: "autobrr", Observe: ctrlcommon.ObserveOnly(config.Spec.Reconcile),
		c: r.Client, apps: r.Apps, ac: ac, namespace: config.Namespace, owner: config.Namespace + "/" + config.Name,
		prune: ctrlcommon.PruneEnabled(config.Spec.Reconcile), owned: engine.NewOwnership("autobrr", config.GetManagedResources()),
	}
	syncErrors := sync.run(ctx, &config.Spec)
	ctrlcommon.UpdateStatusManaged(&config, sync.owned.Managed())
	ctrlcommon.EmitPruneEvents(r.Recorder, &config, sync.owned.Pruned())
	ctrlcommon.UpdateSyncStatus(ctx, r.Client, &config, sync.Observe, sync.Drift(), syncErrors)

	return ctrl.Result{RequeueAfter: ctrlcommon.ReconcileInterval(config.Spec.Reconcile)}, nil
}

// autobrrClient builds the client for a config, or returns the status reason it
// can't.
func (r *AutobrrReconciler) autobrrClient(ctx context.Context, config *automationv1alpha1.Autobrr) (*autobrrclient.Client, string, error) {
	apiKey, err := reconciler.ResolveSecretKeyRef(ctx, r.Client, config.Namespace, config.Spec.Connection.APIKeySecretRef)
	if err != nil {
		return nil, engine.ReasonSecretNotFound, err
	}
	tlsCfg, err := engine.ResolveTLSConfig(ctx, r.Client, config.Namespace, config.Spec.Connection.TLS)
	if err != nil {
		return nil, engine.ReasonSecretNotFound, err
	}
	hc, err := engine.NewHTTPClient(config.Spec.Connection.URL, engine.AuthAPIKey, engine.WithAPIKey(apiKey),
		// Autobrr reads only X-API-Token; any other header is answered with 403.
		engine.WithAPIKeyHeader("X-API-Token"), engine.WithTLSConfig(tlsCfg), engine.WithAppLabel("autobrr"))
	if err != nil {
		return nil, engine.ReasonInvalidConfig, err
	}
	return autobrrclient.NewClient(hc), "", nil
}

// autobrrSync applies one Autobrr. With observe set it only reads, and
// records what it would have written.
type autobrrSync struct {
	engine.Writes
	c         client.Reader
	apps      ctrlcommon.Apps
	ac        *autobrrclient.Client
	namespace string
	owner     string
	prune     bool
	// declared names the indexers and download clients the spec declares, which
	// observe mode may find missing while other items refer to them.
	declared map[string][]string
	owned    *engine.Ownership

	definitions []map[string]any
}

// created records an item once create succeeds.
func (s *autobrrSync) created(kind, name string, create func() error) func() error {
	return func() error {
		if err := create(); err != nil {
			return err
		}
		s.owned.Created(kind, name)
		return nil
	}
}

// run applies the sections in dependency order: filter actions refer to
// download clients by name.
func (s *autobrrSync) run(ctx context.Context, spec *automationv1alpha1.AutobrrSpec) []string {
	logger := log.FromContext(ctx)
	s.declared = map[string][]string{}
	for _, idx := range spec.Indexers {
		s.declared["indexers"] = append(s.declared["indexers"], idx.Name)
	}
	for _, dc := range spec.DownloadClients {
		s.declared["downloadClients"] = append(s.declared["downloadClients"], dc.Name)
	}
	for _, f := range spec.Filters {
		s.declared["filters"] = append(s.declared["filters"], f.Name)
	}
	for _, p := range spec.Proxies {
		s.declared["proxies"] = append(s.declared["proxies"], p.Name)
	}
	var syncErrors []string
	failed := map[string]bool{}
	fail := func(section, name string, err error) {
		logger.Error(err, "failed to reconcile", "section", section, "name", name)
		syncErrors = append(syncErrors, fmt.Sprintf("%s(%s): %v", section, name, err))
		kind := section + "s"
		if section == "proxy" {
			kind = "proxies"
		}
		failed[kind] = true
	}
	for _, p := range spec.Proxies {
		if err := s.proxy(ctx, p); err != nil {
			fail("proxy", p.Name, err)
		}
	}
	for _, dc := range spec.DownloadClients {
		if err := s.downloadClient(ctx, dc); err != nil {
			fail("downloadClient", dc.Name, err)
		}
	}
	for _, idx := range spec.Indexers {
		if err := s.indexer(ctx, idx); err != nil {
			fail("indexer", idx.Name, err)
		}
	}
	for _, irc := range spec.IRCNetworks {
		if err := s.ircNetwork(ctx, irc); err != nil {
			fail("ircNetwork", irc.Name, err)
		}
	}
	for _, feed := range spec.Feeds {
		if err := s.feed(ctx, feed); err != nil {
			fail("feed", feed.Name, err)
		}
	}
	for _, filter := range spec.Filters {
		if err := s.filter(ctx, filter); err != nil {
			fail("filter", filter.Name, err)
		}
	}
	for _, n := range spec.Notifications {
		if err := s.notification(ctx, n); err != nil {
			fail("notification", n.Name, err)
		}
	}
	for _, l := range spec.Lists {
		if err := s.titleList(ctx, l); err != nil {
			fail("list", l.Name, err)
		}
	}
	if s.prune {
		syncErrors = append(syncErrors, s.pruneUndeclared(ctx, spec, failed)...)
	}
	return syncErrors
}

// autobrrClientTypes is the download client type for each kind an appRef can name.
var autobrrClientTypes = map[string]string{
	"QBittorrent": "qBittorrent", "Sabnzbd": "SABnzbd",
	"Sonarr": "Sonarr", "Radarr": "Radarr", "Lidarr": "Lidarr", "Readarr": "Readarr",
}

// resolveDownloadClient returns dc with its appRef, if any, replaced by the
// referenced resource's type, URL and credentials. Autobrr takes the whole URL
// as the host for every type, so the port is sent as 0: a stored one would be
// appended to it.
func (s *autobrrSync) resolveDownloadClient(
	ctx context.Context, dc automationv1alpha1.AutobrrDownloadClient,
) (automationv1alpha1.AutobrrDownloadClient, error) {
	if dc.AppRef == nil {
		return dc, nil
	}
	app, addr, err := ctrlcommon.ResolveAppAddress(ctx, s.c, s.apps, s.namespace, *dc.AppRef)
	if err != nil {
		return dc, fmt.Errorf("appRef: %w", err)
	}
	dc.Type, dc.Host, dc.Port, dc.TLS = autobrrClientTypes[app.Kind], strings.TrimRight(app.URL, "/"), new(int32(0)), &addr.TLS
	dc.UsernameSecretRef, dc.PasswordSecretRef, dc.APIKeySecretRef = app.UsernameSecretRef, app.PasswordSecretRef, app.APIKeySecretRef
	return dc, nil
}

func (s *autobrrSync) downloadClient(ctx context.Context, dc automationv1alpha1.AutobrrDownloadClient) error {
	dc, err := s.resolveDownloadClient(ctx, dc)
	if err != nil {
		return err
	}
	existing, err := s.ac.ListDownloadClients(ctx)
	if err != nil {
		return fmt.Errorf("listing download clients: %w", err)
	}

	obj := map[string]any{
		"name": dc.Name,
		"type": autobrrType(dc.Type),
		"host": dc.Host,
	}
	setEnabled(obj, dc.Enabled)
	if dc.Port != nil {
		obj["port"] = *dc.Port
	}
	if dc.TLS != nil {
		obj["tls"] = *dc.TLS
	}

	if dc.UsernameSecretRef != nil {
		val, err := reconciler.ResolveSecretKeyRef(ctx, s.c, s.namespace, *dc.UsernameSecretRef)
		if err != nil {
			return fmt.Errorf("resolving username: %w", err)
		}
		obj["username"] = val
	}
	if dc.PasswordSecretRef != nil {
		val, err := reconciler.ResolveSecretKeyRef(ctx, s.c, s.namespace, *dc.PasswordSecretRef)
		if err != nil {
			return fmt.Errorf("resolving password: %w", err)
		}
		obj["password"] = val
	}
	settings, err := s.downloadClientSettings(ctx, dc)
	if err != nil {
		return err
	}
	if len(settings) > 0 {
		obj["settings"] = settings
	}

	return s.upsert(ctx, "download client", dc.Name, existing, obj,
		s.created("downloadClients", dc.Name, func() error {
			return s.ac.CreateDownloadClient(ctx, createBody(obj, map[string]any{"enabled": true}))
		}),
		func(id int, current map[string]any) error {
			body, err := mergedOver(current, obj)
			if err != nil {
				return err
			}
			return s.ac.UpdateDownloadClient(ctx, id, body)
		}, s.ac.ListDownloadClients)
}

func (s *autobrrSync) downloadClientSettings(
	ctx context.Context, dc automationv1alpha1.AutobrrDownloadClient,
) (map[string]any, error) {
	settings := map[string]any{}
	if dc.APIKeySecretRef != nil {
		val, err := reconciler.ResolveSecretKeyRef(ctx, s.c, s.namespace, *dc.APIKeySecretRef)
		if err != nil {
			return nil, fmt.Errorf("resolving API key: %w", err)
		}
		settings["apikey"] = val
	}
	if a := dc.HTTPAuth; a != (automationv1alpha1.AutobrrHTTPAuth{}) {
		user, err := reconciler.ResolveSecretKeyRef(ctx, s.c, s.namespace, a.UsernameSecretRef)
		if err != nil {
			return nil, fmt.Errorf("resolving httpAuth username: %w", err)
		}
		pass, err := reconciler.ResolveSecretKeyRef(ctx, s.c, s.namespace, a.PasswordSecretRef)
		if err != nil {
			return nil, fmt.Errorf("resolving httpAuth password: %w", err)
		}
		// Autobrr reads the deprecated basic block over auth whenever it holds a
		// username, and its web UI still edits basic for most clients.
		basic := map[string]any{"auth": true, "username": user, "password": pass}
		authType := "BASIC_AUTH"
		if a.Type == "Digest" {
			basic, authType = map[string]any{"auth": false, "username": "", "password": ""}, "DIGEST_AUTH"
		}
		settings["basic"] = basic
		settings["auth"] = map[string]any{"enabled": true, "type": authType, "username": user, "password": pass}
	}
	if r := dc.Rules; r != nil {
		rules, err := autobrrFields(r, autobrrclient.RuleFields)
		if err != nil {
			return nil, err
		}
		rules["enabled"] = r.Enabled == nil || *r.Enabled
		if c, ok := autobrrclient.SlowTorrentConditions[r.IgnoreSlowTorrentsCondition]; ok {
			rules["ignore_slow_torrents_condition"] = c
		}
		settings["rules"] = rules
	}
	if dc.ExternalDownloadClient != "" {
		settings["external_download_client"] = dc.ExternalDownloadClient
	}
	return settings, nil
}

func (s *autobrrSync) indexer(ctx context.Context, idx automationv1alpha1.AutobrrIndexer) error {
	existing, err := s.ac.ListIndexers(ctx)
	if err != nil {
		return fmt.Errorf("listing indexers: %w", err)
	}
	def, err := s.definition(ctx, idx.Identifier)
	if err != nil {
		return err
	}
	implementation, _ := def["implementation"].(string)
	if implementation == "" {
		implementation = autobrrclient.ImplementationIRC
	}
	settings, err := s.indexerSettings(ctx, idx)
	if err != nil {
		return err
	}

	obj := map[string]any{
		"name":           idx.Name,
		"implementation": implementation,
	}
	setEnabled(obj, idx.Enabled)
	if len(settings) > 0 {
		obj["settings"] = settings
	}
	// Autobrr names a feed indexer itself, as <implementation>-<name>, so only an
	// IRC indexer's identifier can be compared.
	if implementation == autobrrclient.ImplementationIRC {
		obj["identifier"] = idx.Identifier
		obj["base_url"] = idx.BaseURL
		if idx.BaseURL == "" {
			urls, _ := def["urls"].([]any)
			if len(urls) > 0 {
				obj["base_url"] = urls[0]
			}
		}
	}
	switch ok, err := s.useProxy(ctx, obj, idx.ProxyName); {
	case err != nil:
		return err
	case !ok:
		s.RecordDrift("indexer", idx.Name)
		return nil
	}

	for _, e := range existing {
		if e["name"] != idx.Name {
			continue
		}
		id, ok := e["id"].(float64)
		if !ok {
			return fmt.Errorf("indexer %q has no valid id", idx.Name)
		}
		current, err := s.ac.GetIndexer(ctx, int(id))
		if err != nil {
			return fmt.Errorf("reading indexer %q: %w", idx.Name, err)
		}
		desired, err := jsonValue(obj)
		if err != nil {
			return err
		}
		out, err := reconciler.MergeDesired(current, obj)
		if err != nil {
			return err
		}
		key := s.secretKey("indexer", idx.Name)
		if autobrrCovers(current, desired) && !reconciler.SecretsDiffer(key, out.SecretDigest, s.Observe) {
			return nil
		}
		return s.Apply("indexer", idx.Name, func() error {
			// An update must carry every stored secret setting, redacted or not.
			if err := s.ac.UpdateIndexer(ctx, int(id), out.Merged); err != nil {
				return err
			}
			reconciler.RecordSecrets(key, out.SecretDigest)
			return nil
		})
	}
	return s.Apply("indexer", idx.Name, s.created("indexers", idx.Name, func() error {
		body := createBody(obj, map[string]any{"enabled": true})
		body["identifier"] = idx.Identifier
		if err := s.ac.CreateIndexer(ctx, body); err != nil {
			return err
		}
		s.rememberCreated(ctx, s.secretKey("indexer", idx.Name), idx.Name, obj, func(ctx context.Context) ([]map[string]any, error) {
			ids, err := s.indexerIDs(ctx)
			if err != nil {
				return nil, err
			}
			id, _, err := ids.Find(idx.Name)
			if err != nil {
				return nil, err
			}
			created, err := s.ac.GetIndexer(ctx, id)
			if err != nil {
				return nil, err
			}
			return []map[string]any{created}, nil
		})
		return nil
	}))
}

// definition returns the Autobrr indexer definition with the given identifier.
func (s *autobrrSync) definition(ctx context.Context, identifier string) (map[string]any, error) {
	if s.definitions == nil {
		defs, err := s.ac.IndexerDefinitions(ctx)
		if err != nil {
			return nil, fmt.Errorf("listing indexer definitions: %w", err)
		}
		s.definitions = defs
	}
	for _, d := range s.definitions {
		if d["identifier"] == identifier {
			return d, nil
		}
	}
	return nil, fmt.Errorf("Autobrr has no indexer definition %q", identifier)
}

func (s *autobrrSync) indexerSettings(ctx context.Context, idx automationv1alpha1.AutobrrIndexer) (map[string]any, error) {
	fields, err := ctrlcommon.ResolveConfigFields(ctx, s.c, s.namespace, "indexer "+idx.Name, idx.Settings)
	if err != nil {
		return nil, err
	}
	settings := make(map[string]any, len(fields))
	for _, f := range fields {
		if f.Value == nil {
			continue
		}
		value := f.Value.ToInterface()
		if text, ok := value.(string); ok {
			settings[f.Name] = text
		} else {
			settings[f.Name] = fmt.Sprint(value)
		}
	}
	return settings, nil
}

func (s *autobrrSync) ircNetwork(ctx context.Context, irc automationv1alpha1.AutobrrIRCNetwork) error {
	existing, err := s.ac.ListIRCNetworks(ctx)
	if err != nil {
		return fmt.Errorf("listing IRC networks: %w", err)
	}

	obj := map[string]any{
		"name":   irc.Name,
		"server": irc.Server,
		"port":   irc.Port,
		"nick":   irc.Nick,
	}
	setEnabled(obj, irc.Enabled)
	if irc.TLS != nil {
		obj["tls"] = *irc.TLS
	}
	// Autobrr keeps IRC authentication in a nested object.
	auth := map[string]any{}
	if irc.AuthMechanism != "" {
		auth["mechanism"] = autobrrIRCAuth(irc.AuthMechanism)
	}
	if irc.InviteCommand != "" {
		obj["invite_command"] = irc.InviteCommand
	}
	if irc.BouncerAddress != "" {
		obj["use_bouncer"], obj["bouncer_addr"] = true, irc.BouncerAddress
	}
	if irc.BotMode != nil {
		obj["bot_mode"] = *irc.BotMode
	}
	if irc.ServerPasswordSecretRef != nil {
		val, err := reconciler.ResolveSecretKeyRef(ctx, s.c, s.namespace, *irc.ServerPasswordSecretRef)
		if err != nil {
			return fmt.Errorf("resolving server password: %w", err)
		}
		obj["pass"] = val
	}
	switch ok, err := s.useProxy(ctx, obj, irc.ProxyName); {
	case err != nil:
		return err
	case !ok:
		s.RecordDrift("IRC network", irc.Name)
		return nil
	}

	if irc.AuthAccountSecretRef != nil {
		val, err := reconciler.ResolveSecretKeyRef(ctx, s.c, s.namespace, *irc.AuthAccountSecretRef)
		if err != nil {
			return fmt.Errorf("resolving auth account: %w", err)
		}
		auth["account"] = val
	}
	if irc.AuthPasswordSecretRef != nil {
		val, err := reconciler.ResolveSecretKeyRef(ctx, s.c, s.namespace, *irc.AuthPasswordSecretRef)
		if err != nil {
			return fmt.Errorf("resolving auth password: %w", err)
		}
		auth["password"] = val
	}
	if len(auth) > 0 {
		obj["auth"] = auth
	}

	if len(irc.Channels) > 0 {
		channels := make([]map[string]any, 0, len(irc.Channels))
		for _, ch := range irc.Channels {
			chObj := map[string]any{"name": ch.Name}
			if ch.PasswordSecretRef != nil {
				val, err := reconciler.ResolveSecretKeyRef(ctx, s.c, s.namespace, *ch.PasswordSecretRef)
				if err != nil {
					return fmt.Errorf("resolving channel %q password: %w", ch.Name, err)
				}
				chObj["password"] = val
			}
			channels = append(channels, chObj)
		}
		obj["channels"] = channels
	}

	update := func(id int, current map[string]any) error {
		body, err := mergedOver(current, obj)
		if err != nil {
			return err
		}
		// Autobrr replaces a network's channels with the ones sent.
		if _, declared := obj["channels"]; declared {
			body["channels"] = onlyNamed(body["channels"], irc.Channels)
		}
		return s.ac.UpdateIRCNetwork(ctx, id, body)
	}
	return s.upsert(ctx, "IRC network", irc.Name, existing, obj,
		s.created("ircNetworks", irc.Name, func() error {
			body := createBody(obj, map[string]any{"enabled": true})
			auth, _ := body["auth"].(map[string]any)
			if _, ok := auth["mechanism"]; !ok {
				body["auth"] = createBody(auth, map[string]any{"mechanism": autobrrIRCAuth("None")})
			}
			if _, proxied := obj["use_proxy"]; !proxied {
				return s.ac.CreateIRCNetwork(ctx, body)
			}
			// Autobrr before 1.86.0 ignores a new network's proxy, so the network
			// is created off, then turned on with its proxy: an enabled network
			// would connect without it.
			enabled := body["enabled"]
			body["enabled"] = false
			return createThenUpdate(ctx, "IRC network", body, s.ac.CreateIRCNetwork, s.ac.ListIRCNetworks,
				func(id int, current map[string]any) error {
					current["enabled"] = enabled
					return update(id, current)
				})
		}),
		update, s.ac.ListIRCNetworks)
}

// createThenUpdate creates an item, then updates it with what Autobrr leaves
// out of a create.
func createThenUpdate(
	ctx context.Context, kind string, body map[string]any,
	create func(context.Context, map[string]any) error,
	list func(context.Context) ([]map[string]any, error),
	update func(id int, current map[string]any) error,
) error {
	if err := create(ctx, body); err != nil {
		return err
	}
	items, err := list(ctx)
	if err != nil {
		return fmt.Errorf("listing %ss: %w", kind, err)
	}
	for _, item := range items {
		if item["name"] != body["name"] {
			continue
		}
		id, _ := item["id"].(float64)
		return update(int(id), item)
	}
	return fmt.Errorf("%s %q was not created", kind, body["name"])
}

func (s *autobrrSync) feed(ctx context.Context, feed automationv1alpha1.AutobrrFeed) error {
	existing, err := s.ac.ListFeeds(ctx)
	if err != nil {
		return fmt.Errorf("listing feeds: %w", err)
	}
	indexerIDs, err := s.indexerIDs(ctx)
	if err != nil {
		return err
	}
	indexerID, found, err := indexerIDs.Find(feed.IndexerName)
	switch {
	case err != nil:
		return fmt.Errorf("indexer: %w", err)
	case !found && s.planned("indexers", feed.IndexerName):
		s.RecordDrift("feed", feed.Name)
		return nil
	case !found:
		return fmt.Errorf("references indexer %q, which does not exist", feed.IndexerName)
	}

	obj := map[string]any{
		"name":    feed.Name,
		"type":    strings.ToUpper(feed.Type),
		"url":     feed.URL,
		"indexer": map[string]any{"id": indexerID},
	}
	setEnabled(obj, feed.Enabled)
	if feed.Interval != nil {
		obj["interval"] = *feed.Interval
	}
	if feed.Timeout != 0 {
		obj["timeout"] = feed.Timeout
	}
	if feed.MaxAge != nil {
		obj["max_age"] = *feed.MaxAge
	}
	if t, ok := autobrrclient.FeedDownloadTypes[feed.DownloadType]; ok {
		obj["settings"] = map[string]any{"download_type": t}
	}
	if feed.APIKeySecretRef != nil {
		val, err := reconciler.ResolveSecretKeyRef(ctx, s.c, s.namespace, *feed.APIKeySecretRef)
		if err != nil {
			return fmt.Errorf("resolving API key: %w", err)
		}
		obj["api_key"] = val
	}
	if feed.CookieSecretRef != nil {
		val, err := reconciler.ResolveSecretKeyRef(ctx, s.c, s.namespace, *feed.CookieSecretRef)
		if err != nil {
			return fmt.Errorf("resolving cookie: %w", err)
		}
		obj["cookie"] = val
	}
	// Autobrr links a feed to its indexer by indexer_id and reads it back as an
	// indexer object.
	withIndexer := func(body map[string]any) map[string]any {
		delete(body, "indexer")
		body["indexer_id"] = indexerID
		return body
	}

	update := func(id int, current map[string]any) error {
		body, err := mergedOver(current, obj)
		if err != nil {
			return err
		}
		return s.ac.UpdateFeed(ctx, id, withIndexer(body))
	}
	return s.upsert(ctx, "feed", feed.Name, existing, obj,
		s.created("feeds", feed.Name, func() error {
			body := withIndexer(createBody(obj, map[string]any{
				"enabled": true, "interval": defaultFeedInterval, "timeout": defaultFeedTimeout,
			}))
			// Autobrr leaves a feed's max age and cookie out of a create.
			_, maxAge := obj["max_age"]
			_, cookie := obj["cookie"]
			if !maxAge && !cookie {
				return s.ac.CreateFeed(ctx, body)
			}
			return createThenUpdate(ctx, "feed", body, s.ac.CreateFeed, s.ac.ListFeeds, update)
		}),
		update, s.ac.ListFeeds)
}

// useProxy points obj at the named proxy. It reports false for a proxy the
// spec declares but observe mode hasn't created.
func (s *autobrrSync) useProxy(ctx context.Context, obj map[string]any, name string) (bool, error) {
	if name == "" {
		return true, nil
	}
	proxies, err := s.ac.ListProxies(ctx)
	if err != nil {
		return false, fmt.Errorf("listing proxies: %w", err)
	}
	id, found, err := engine.NamedIDsOf(proxies, "name").Find(name)
	switch {
	case err != nil:
		return false, fmt.Errorf("proxy: %w", err)
	case !found && s.planned("proxies", name):
		return false, nil
	case !found:
		return false, fmt.Errorf("references proxy %q, which does not exist", name)
	}
	obj["use_proxy"] = true
	obj["proxy_id"] = id
	return true, nil
}

func (s *autobrrSync) proxy(ctx context.Context, p automationv1alpha1.AutobrrProxy) error {
	existing, err := s.ac.ListProxies(ctx)
	if err != nil {
		return fmt.Errorf("listing proxies: %w", err)
	}
	obj := map[string]any{"name": p.Name, "type": strings.ToUpper(p.Type), "addr": p.Addr}
	setEnabled(obj, p.Enabled)
	if p.Timeout != 0 {
		obj["timeout"] = p.Timeout
	}
	for key, ref := range map[string]*commonv1alpha1.SecretKeyRef{"user": p.UsernameSecretRef, "pass": p.PasswordSecretRef} {
		if ref == nil {
			continue
		}
		val, err := reconciler.ResolveSecretKeyRef(ctx, s.c, s.namespace, *ref)
		if err != nil {
			return fmt.Errorf("resolving %s: %w", key, err)
		}
		obj[key] = val
	}
	return s.upsert(ctx, "proxy", p.Name, existing, obj,
		s.created("proxies", p.Name, func() error {
			return s.ac.CreateProxy(ctx, createBody(obj, map[string]any{"enabled": true}))
		}),
		func(id int, current map[string]any) error {
			body, err := mergedOver(current, obj)
			if err != nil {
				return err
			}
			return s.ac.UpdateProxy(ctx, id, body)
		}, s.ac.ListProxies)
}

// titleList reconciles one list. A download client or filter it names that
// the spec declares but Autobrr doesn't have yet, in observe mode, is drift.
func (s *autobrrSync) titleList(ctx context.Context, l automationv1alpha1.AutobrrTitleList) error {
	existing, err := s.ac.ListLists(ctx)
	if err != nil {
		return fmt.Errorf("listing lists: %w", err)
	}
	obj := map[string]any{"name": l.Name, "type": strings.ToUpper(l.Type)}
	setEnabled(obj, l.Enabled)
	if l.URL != "" {
		obj["url"] = l.URL
	}
	if l.APIKeySecretRef != nil {
		val, err := reconciler.ResolveSecretKeyRef(ctx, s.c, s.namespace, *l.APIKeySecretRef)
		if err != nil {
			return fmt.Errorf("resolving API key: %w", err)
		}
		obj["api_key"] = val
	}
	if l.DownloadClientName != "" {
		downloaders, err := s.ac.ListDownloadClients(ctx)
		if err != nil {
			return fmt.Errorf("listing download clients: %w", err)
		}
		id, found, err := engine.NamedIDsOf(downloaders, "name").Find(l.DownloadClientName)
		switch {
		case err != nil:
			return fmt.Errorf("download client: %w", err)
		case !found && s.planned("downloadClients", l.DownloadClientName):
			s.RecordDrift("list", l.Name)
			return nil
		case !found:
			return fmt.Errorf("references download client %q, which does not exist", l.DownloadClientName)
		}
		obj["client_id"] = id
	}
	filters, err := s.ac.ListFilters(ctx)
	if err != nil {
		return fmt.Errorf("listing filters: %w", err)
	}
	ids := engine.NamedIDsOf(filters, "name")
	refs := make([]any, 0, len(l.Filters))
	for _, name := range l.Filters {
		id, found, err := ids.Find(name)
		switch {
		case err != nil:
			return fmt.Errorf("filter: %w", err)
		case !found && s.planned("filters", name):
			s.RecordDrift("list", l.Name)
			return nil
		case !found:
			return fmt.Errorf("references filter %q, which does not exist", name)
		}
		refs = append(refs, map[string]any{"id": id, "name": name})
	}
	obj["filters"] = refs
	for key, value := range map[string]*bool{
		"match_release": l.MatchRelease, "include_unmonitored": l.IncludeUnmonitored,
		"include_alternate_titles": l.IncludeAlternateTitles, "include_year": l.IncludeYear,
		"skip_clean_sanitize": l.SkipCleanSanitize,
	} {
		if value != nil {
			obj[key] = *value
		}
	}
	for key, tags := range map[string][]string{"tags_included": l.TagsIncluded, "tags_excluded": l.TagsExcluded} {
		if tags != nil {
			obj[key] = tags
		}
	}
	return s.upsert(ctx, "list", l.Name, existing, obj,
		s.created("lists", l.Name, func() error {
			return s.ac.CreateList(ctx, createBody(obj, map[string]any{
				"enabled": true, "tags_included": []any{}, "tags_excluded": []any{}, "headers": []any{},
			}))
		}),
		func(id int, current map[string]any) error {
			body, err := mergedOver(current, obj)
			if err != nil {
				return err
			}
			return s.ac.UpdateList(ctx, id, body)
		}, s.ac.ListLists)
}

func (s *autobrrSync) notification(ctx context.Context, n automationv1alpha1.AutobrrNotification) error {
	options, err := ctrlcommon.FieldValues(ctx, s.c, s.namespace, "notification "+n.Name, n.Options)
	if err != nil {
		return err
	}
	existing, err := s.ac.ListNotifications(ctx)
	if err != nil {
		return fmt.Errorf("listing notifications: %w", err)
	}
	obj := map[string]any{"name": n.Name, "type": strings.ToUpper(n.Type)}
	setEnabled(obj, n.Enabled)
	if n.Events != nil {
		names := make([]any, 0, len(n.Events))
		for _, event := range n.Events {
			names = append(names, autobrrclient.NotificationEvent(event))
		}
		obj["events"] = names
	}
	for key, value := range options {
		if !slices.Contains(autobrrclient.NotificationOptions, key) {
			return fmt.Errorf("option %q isn't an Autobrr notification setting", key)
		}
		obj[key] = value
	}
	return s.upsert(ctx, "notification", n.Name, existing, obj,
		s.created("notifications", n.Name, func() error {
			return s.ac.CreateNotification(ctx, createBody(obj, map[string]any{"enabled": true, "events": []any{}}))
		}),
		func(id int, current map[string]any) error {
			body, err := mergedOver(current, obj)
			if err != nil {
				return err
			}
			return s.ac.UpdateNotification(ctx, id, body)
		}, s.ac.ListNotifications)
}

// indexerIDs maps each Autobrr indexer's name to its id.
func (s *autobrrSync) indexerIDs(ctx context.Context) (engine.NamedIDs, error) {
	indexers, err := s.ac.ListIndexers(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing indexers: %w", err)
	}
	return engine.NamedIDsOf(indexers, "name"), nil
}

func (s *autobrrSync) filter(ctx context.Context, filter automationv1alpha1.AutobrrFilter) error {
	existing, err := s.ac.ListFilters(ctx)
	if err != nil {
		return fmt.Errorf("listing filters: %w", err)
	}
	downloaders, err := s.ac.ListDownloadClients(ctx)
	if err != nil {
		return fmt.Errorf("listing download clients: %w", err)
	}
	clientIDs := engine.NamedIDsOf(downloaders, "name")

	obj, err := autobrrFields(filter, autobrrclient.FilterFields)
	if err != nil {
		return err
	}
	obj["name"] = filter.Name
	setEnabled(obj, filter.Enabled)
	// Autobrr keeps tags as one comma-separated string.
	if len(filter.Tags) > 0 {
		obj["tags"] = strings.Join(filter.Tags, ",")
	}
	if len(filter.ExceptTags) > 0 {
		obj["except_tags"] = strings.Join(filter.ExceptTags, ",")
	}
	if len(filter.Indexers) > 0 {
		indexers, missing, err := s.filterIndexers(ctx, filter.Indexers)
		switch {
		case err != nil:
			return err
		case missing != "" && s.planned("indexers", missing):
			s.RecordDrift("filter", filter.Name)
			return nil
		case missing != "":
			return fmt.Errorf("references indexer %q, which does not exist", missing)
		}
		obj["indexers"] = indexers
	}

	// A new filter must carry these lists; Autobrr's columns reject null.
	create := createBody(obj, map[string]any{"enabled": true})
	addRequiredFilterLists(create)
	filterID, err := s.upsertFilter(ctx, filter.Name, existing, obj, create)
	if err != nil {
		return err
	}
	if filterID == 0 || len(filter.Actions) == 0 {
		return nil
	}
	return s.actions(ctx, filterID, filter, clientIDs)
}

// filterIndexers resolves indexer names to the objects Autobrr links a filter
// to, ordered by id as Autobrr returns them. It returns the first name with no
// indexer.
func (s *autobrrSync) filterIndexers(ctx context.Context, names []string) ([]any, string, error) {
	ids, err := s.indexerIDs(ctx)
	if err != nil {
		return nil, "", err
	}
	resolved := make([]int, 0, len(names))
	for _, name := range names {
		id, found, err := ids.Find(name)
		switch {
		case err != nil:
			return nil, "", fmt.Errorf("indexer: %w", err)
		case !found:
			return nil, name, nil
		}
		resolved = append(resolved, id)
	}
	slices.Sort(resolved)
	out := make([]any, 0, len(resolved))
	for _, id := range resolved {
		out = append(out, map[string]any{"id": id})
	}
	return out, "", nil
}

// actions creates or updates a filter's actions by name.
func (s *autobrrSync) actions(
	ctx context.Context, filterID int, filter automationv1alpha1.AutobrrFilter, clientIDs engine.NamedIDs,
) error {
	existing, err := s.ac.ListActions(ctx, filterID)
	if err != nil {
		return fmt.Errorf("listing actions of filter %q: %w", filter.Name, err)
	}
	for _, a := range filter.Actions {
		aObj, err := autobrrFields(a, autobrrclient.ActionFields)
		if err != nil {
			return fmt.Errorf("action %q: %w", a.Name, err)
		}
		aObj["name"] = a.Name
		aObj["type"] = autobrrType(a.Type)
		setEnabled(aObj, a.Enabled)
		if a.DownloadClientName != "" {
			// Autobrr links an action to a downloader by its numeric ID.
			id, found, err := clientIDs.Find(a.DownloadClientName)
			switch {
			case err != nil:
				return fmt.Errorf("action %q download client: %w", a.Name, err)
			case !found && s.planned("downloadClients", a.DownloadClientName):
				s.RecordDrift("action", a.Name)
				continue
			case !found:
				return fmt.Errorf("action %q references download client %q, which does not exist", a.Name, a.DownloadClientName)
			}
			aObj["client_id"] = id
		}
		// Actions read back without their filter_id, so it is sent but not compared.
		withFilter := func(body map[string]any) map[string]any {
			body["filter_id"] = filterID
			return body
		}
		if err := s.upsert(ctx, "action", filter.Name+"/"+a.Name, existing, aObj,
			s.created(actionsKind, filter.Name+"/"+a.Name, func() error {
				return s.ac.CreateAction(ctx, withFilter(createBody(aObj, map[string]any{"enabled": true})))
			}),
			func(id int, current map[string]any) error {
				body, err := mergedOver(current, aObj)
				if err != nil {
					return err
				}
				return s.ac.UpdateAction(ctx, id, withFilter(body))
			},
			func(ctx context.Context) ([]map[string]any, error) { return s.ac.ListActions(ctx, filterID) }); err != nil {
			return fmt.Errorf("action %q: %w", a.Name, err)
		}
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AutobrrReconciler) SetupWithManager(mgr ctrl.Manager) error {
	b := ctrl.NewControllerManagedBy(mgr).
		For(&automationv1alpha1.Autobrr{}, builder.WithPredicates(ctrlcommon.ConfigChangedPredicate())).
		WatchesMetadata(&corev1.Secret{}, ctrlcommon.EnqueueForSecretThroughRefs(r.Client, &automationv1alpha1.AutobrrList{}, autobrrAppRefs))
	b, err := ctrlcommon.WatchAppRefs(mgr, b, r.Apps, &automationv1alpha1.Autobrr{}, &automationv1alpha1.AutobrrList{}, autobrrAppRefs,
		"QBittorrent", "Sabnzbd", "Sonarr", "Radarr", "Lidarr", "Readarr")
	if err != nil {
		return err
	}
	return b.Named("autobrr").Complete(r)
}

func autobrrAppRefs(o client.Object) []commonv1alpha1.AppRef {
	var refs []commonv1alpha1.AppRef
	for _, dc := range o.(*automationv1alpha1.Autobrr).Spec.DownloadClients {
		if dc.AppRef != nil {
			refs = append(refs, *dc.AppRef)
		}
	}
	return refs
}

// autobrrFields renders the spec fields a table names under Autobrr's keys,
// leaving out the ones the spec leaves unset.
func autobrrFields(spec any, table map[string]string) (map[string]any, error) {
	raw, err := json.Marshal(spec)
	if err != nil {
		return nil, err
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, err
	}
	out := map[string]any{}
	for name, key := range table {
		if v, ok := fields[name]; ok {
			out[key] = v
		}
	}
	return out, nil
}

func autobrrType(name string) string {
	if t, ok := autobrrclient.Types[name]; ok {
		return t
	}
	return name
}

// upsert updates an existing resource only when Autobrr does not already hold
// every declared value; writing on every pass rewrote each one each time.
// upsert updates the existing item with obj's name or creates it. Autobrr reads
// secrets back redacted, so a rotated one is noticed through the digest of what
// was last written; list rereads the items once one is created to record it.
func (s *autobrrSync) upsert(ctx context.Context, kind, key string, existing []map[string]any, obj map[string]any,
	create func() error, update func(id int, current map[string]any) error,
	list func(context.Context) ([]map[string]any, error),
) error {
	name, _ := obj["name"].(string)
	secretKey := s.secretKey(kind, key)
	for _, e := range existing {
		if n, _ := e["name"].(string); n != name {
			continue
		}
		id, ok := e["id"].(float64)
		if !ok {
			return fmt.Errorf("%s %q has no valid id", kind, name)
		}
		obj["id"] = int(id)
		desired, err := jsonValue(obj)
		if err != nil {
			return err
		}
		out, err := reconciler.MergeDesired(e, obj)
		if err != nil {
			return err
		}
		if autobrrCovers(e, desired) && !reconciler.SecretsDiffer(secretKey, out.SecretDigest, s.Observe) {
			return nil
		}
		return s.Apply(kind, name, func() error {
			if err := update(int(id), e); err != nil {
				return err
			}
			reconciler.RecordSecrets(secretKey, out.SecretDigest)
			return nil
		})
	}
	return s.Apply(kind, name, func() error {
		if err := create(); err != nil {
			return err
		}
		s.rememberCreated(ctx, secretKey, name, obj, list)
		return nil
	})
}

// rememberCreated records the secrets a new item was created with, which it
// reads back redacted, so the next pass doesn't write them again.
func (s *autobrrSync) rememberCreated(
	ctx context.Context, secretKey, name string, obj map[string]any, list func(context.Context) ([]map[string]any, error),
) {
	items, err := list(ctx)
	if err != nil {
		return
	}
	for _, e := range items {
		if e["name"] != name {
			continue
		}
		if out, err := reconciler.MergeDesired(e, obj); err == nil {
			reconciler.RecordSecrets(secretKey, out.SecretDigest)
		}
		return
	}
}

// planned reports whether observe mode can't find an item because the spec
// declares it and observe mode creates nothing, so a reference to it is a
// difference rather than a failure.
func (s *autobrrSync) planned(kind, name string) bool {
	return s.Observe && slices.Contains(s.declared[kind], name)
}

func (s *autobrrSync) secretKey(kind, name string) string {
	return fmt.Sprintf("autobrr/%q/%q/%q", s.owner, kind, name)
}

func setEnabled(obj map[string]any, enable *bool) {
	if enable != nil {
		obj["enabled"] = *enable
	}
}

// createBody is obj with defaults beneath it, for a new item.
func createBody(obj, defaults map[string]any) map[string]any {
	body := maps.Clone(obj)
	if body == nil {
		body = map[string]any{}
	}
	for k, v := range defaults {
		if _, ok := body[k]; !ok {
			body[k] = v
		}
	}
	return body
}

// onlyNamed keeps the entries of a list whose name a channel declares.
func onlyNamed(list any, channels []automationv1alpha1.AutobrrIRCChannel) []any {
	items, _ := list.([]any)
	return slices.DeleteFunc(slices.Clone(items), func(item any) bool {
		name, _ := item.(map[string]any)["name"].(string)
		return !slices.ContainsFunc(channels, func(ch automationv1alpha1.AutobrrIRCChannel) bool { return ch.Name == name })
	})
}

// autobrrCovers compares as reconciler.Covers does, except that a declared
// false, 0 or empty value, at any depth, matches a key Autobrr left out: it
// omits zero values from what it returns.
func autobrrCovers(current map[string]any, desired any) bool {
	return reconciler.Covers(withZeros(current, desired), desired)
}

func withZeros(current, desired any) any {
	fields, ok := desired.(map[string]any)
	if !ok {
		return current
	}
	c, isMap := current.(map[string]any)
	if !isMap && current != nil {
		return current
	}
	filled := maps.Clone(c)
	if filled == nil {
		filled = map[string]any{}
	}
	for key, value := range fields {
		existing, present := filled[key]
		if _, nested := value.(map[string]any); nested {
			filled[key] = withZeros(existing, value)
			continue
		}
		if !present && isZero(value) {
			filled[key] = value
		}
	}
	return filled
}

func isZero(value any) bool {
	switch v := value.(type) {
	case bool:
		return !v
	case float64:
		return v == 0
	case string:
		return v == ""
	case []any:
		return len(v) == 0
	}
	return false
}

// upsertFilter compares against the full stored filter, since the list
// endpoint returns a summary. An update replaces the whole filter, so it sends
// the stored filter with the declared values over it. It returns the filter's
// id, or 0 when observe mode found it missing.
func (s *autobrrSync) upsertFilter(
	ctx context.Context, name string, existing []map[string]any, obj, create map[string]any,
) (int, error) {
	for _, e := range existing {
		if n, _ := e["name"].(string); n != name {
			continue
		}
		raw, ok := e["id"].(float64)
		if !ok {
			return 0, fmt.Errorf("filter %q has no valid id", name)
		}
		id := int(raw)
		full, err := s.ac.GetFilter(ctx, id)
		if err != nil {
			return 0, fmt.Errorf("reading filter %q: %w", name, err)
		}
		desired, err := jsonValue(obj)
		if err != nil {
			return 0, err
		}
		sortByID(full["indexers"])
		if autobrrCovers(full, desired) {
			return id, nil
		}
		merged, _, err := reconciler.MergeDesiredOverCurrent(full, obj)
		if err != nil {
			return 0, err
		}
		delete(merged, "actions")
		addRequiredFilterLists(merged)
		return id, s.Apply("filter", name, func() error { return s.ac.UpdateFilter(ctx, id, merged) })
	}
	var id int
	err := s.Apply("filter", name, func() error {
		created, err := s.ac.CreateFilter(ctx, create)
		if err != nil {
			return err
		}
		raw, _ := created["id"].(float64)
		id = int(raw)
		s.owned.Created("filters", name)
		// Autobrr links a filter to its indexers only on update.
		if _, ok := obj["indexers"]; !ok {
			return nil
		}
		full, err := s.ac.GetFilter(ctx, id)
		if err != nil {
			return fmt.Errorf("reading the new filter: %w", err)
		}
		linked, err := mergedOver(full, obj)
		if err != nil {
			return err
		}
		delete(linked, "actions")
		addRequiredFilterLists(linked)
		return s.ac.UpdateFilter(ctx, id, linked)
	})
	return id, err
}

// addRequiredFilterLists sets the lists Autobrr's filter columns reject as null
// and its reads omit when empty.
func addRequiredFilterLists(filter map[string]any) {
	for _, list := range []string{"codecs", "containers"} {
		if filter[list] == nil {
			filter[list] = []string{}
		}
	}
}

func jsonValue(v any) (any, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var out any
	return out, json.Unmarshal(raw, &out)
}

func autobrrIRCAuth(mechanism string) string {
	if m, ok := autobrrclient.IRCAuthMechanisms[mechanism]; ok {
		return m
	}
	return mechanism
}

// defaultFeedTimeout is the request timeout, in seconds, Autobrr's web UI gives a new feed.
const defaultFeedTimeout = 60

// defaultFeedInterval is the polling interval, in minutes, Autobrr's web UI gives a new feed.
const defaultFeedInterval = 15

// mergedOver returns the stored object with the declared values over it, so an
// update keeps what the resource doesn't manage. Autobrr keeps a stored secret
// sent back redacted.
func mergedOver(current, obj map[string]any) (map[string]any, error) {
	merged, _, err := reconciler.MergeDesiredOverCurrent(current, obj)
	return merged, err
}

func sortByID(list any) {
	items, _ := list.([]any)
	slices.SortFunc(items, func(a, b any) int {
		ai, _ := a.(map[string]any)["id"].(float64)
		bi, _ := b.(map[string]any)["id"].(float64)
		return int(ai - bi)
	})
}
