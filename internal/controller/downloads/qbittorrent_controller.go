package downloads

import (
	"context"
	"fmt"
	"maps"
	"net/netip"
	"reflect"
	"slices"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	downloadsv1alpha1 "github.com/kyleseneker/arrchitect/api/downloads/v1alpha1"
	qbclient "github.com/kyleseneker/arrchitect/internal/client/qbittorrent"
	ctrlcommon "github.com/kyleseneker/arrchitect/internal/controller/common"
	"github.com/kyleseneker/arrchitect/internal/engine"
	"github.com/kyleseneker/arrchitect/internal/reconciler"
)

// QBittorrentReconciler reconciles a QBittorrent object.
type QBittorrentReconciler struct {
	client.Client
	Recorder events.EventRecorder
}

// +kubebuilder:rbac:groups=arrchitect.dev,resources=qbittorrents,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=arrchitect.dev,resources=qbittorrents/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=arrchitect.dev,resources=qbittorrents/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

func (r *QBittorrentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	integration := ctrlcommon.Integration[*downloadsv1alpha1.QBittorrent, *qbclient.Client]{
		App:     "qbittorrent",
		Connect: r.connect,
		Check: func(ctx context.Context, qb *qbclient.Client, _ *downloadsv1alpha1.QBittorrent) error {
			return qb.Ping(ctx)
		},
		Sync:    r.sync,
		Cleanup: cleanupQBittorrent,
		Prune:   true,
	}
	return integration.Reconcile(ctx, r.Client, r.Recorder, req, &downloadsv1alpha1.QBittorrent{})
}

func (r *QBittorrentReconciler) sync(ctx context.Context, qb *qbclient.Client, config *downloadsv1alpha1.QBittorrent, pass *engine.Pass) []string {
	secret := func(ref commonv1alpha1.SecretKeyRef) (string, error) {
		return reconciler.ResolveSecretKeyRef(ctx, r.Client, config.Namespace, ref)
	}
	var problems []string
	if config.Spec.Preferences != nil {
		if err := reconcileQBPreferences(ctx, qb, &pass.Writes, config.Spec.Preferences, secret); err != nil {
			log.FromContext(ctx).Error(err, "failed to reconcile preferences")
			problems = append(problems, fmt.Sprintf("preferences: %v", err))
		}
	}
	if len(config.Spec.Categories) > 0 {
		problems = append(problems, reconcileQBCategories(ctx, qb, pass, config.Spec.Categories)...)
	}
	if len(config.Spec.RSSFeeds) > 0 {
		problems = append(problems, reconcileQBRSSFeeds(ctx, qb, pass, config.Spec.RSSFeeds)...)
	}
	if len(config.Spec.RSSRules) > 0 {
		problems = append(problems, reconcileQBRSSRules(ctx, qb, pass, config.Spec.RSSRules)...)
	}
	return problems
}

// secretResolver reads a Secret key the resource refers to.
type secretResolver func(commonv1alpha1.SecretKeyRef) (string, error)

// qbPreferencePayload maps spec fields to the keys qBittorrent's API expects.
func qbPreferencePayload(prefs *downloadsv1alpha1.QBittorrentPreferences, secret secretResolver) (map[string]any, error) {
	out := map[string]any{}
	v := reflect.ValueOf(prefs).Elem()
	for i := range v.NumField() {
		name, _, _ := strings.Cut(v.Type().Field(i).Tag.Get("json"), ",")
		field := v.Field(i)
		key, plain := qbclient.Preferences[name]
		bytes, speed := qbclient.SpeedLimits[name]
		switch {
		case field.IsZero():
		case plain:
			out[key] = reflect.Indirect(field).Interface()
		case speed:
			out[bytes] = field.Elem().Int() * 1024
		}
	}

	if prefs.SavePath != "" {
		out["save_path"] = qbPath(prefs.SavePath)
	}
	if prefs.TempPath != "" {
		out["temp_path"] = qbPath(prefs.TempPath)
	}
	if len(prefs.ExcludedFileNames) > 0 {
		out["excluded_file_names"] = strings.Join(prefs.ExcludedFileNames, "\n")
	}
	if len(prefs.WebUIDomainList) > 0 {
		out["web_ui_domain_list"] = strings.Join(prefs.WebUIDomainList, ";")
	}
	if len(prefs.WebUIReverseProxiesList) > 0 {
		out["web_ui_reverse_proxies_list"] = strings.Join(prefs.WebUIReverseProxiesList, ";")
	}
	if len(prefs.BannedIPs) > 0 {
		ips, err := qbBannedIPs(prefs.BannedIPs)
		if err != nil {
			return nil, err
		}
		out["banned_IPs"] = ips
	}
	if len(prefs.AddTrackers) > 0 {
		out["add_trackers"] = strings.Join(prefs.AddTrackers, "\n\n")
	}
	if len(prefs.RSSSmartEpisodeFilters) > 0 {
		out["rss_smart_episode_filters"] = strings.Join(prefs.RSSSmartEpisodeFilters, "\n")
	}
	for key, ref := range map[string]*commonv1alpha1.SecretKeyRef{
		"proxy_username": prefs.ProxyUsernameSecretRef, "proxy_password": prefs.ProxyPasswordSecretRef,
	} {
		if ref == nil {
			continue
		}
		if secret == nil {
			return nil, fmt.Errorf("%s refers to a Secret, which can't be read here", key)
		}
		value, err := secret(*ref)
		if err != nil {
			return nil, fmt.Errorf("resolving %s: %w", key, err)
		}
		out[key] = value
	}
	if prefs.SchedulerDays != "" {
		days, ok := qbclient.SchedulerDays[prefs.SchedulerDays]
		if !ok {
			return nil, fmt.Errorf("unknown schedulerDays %q", prefs.SchedulerDays)
		}
		out["scheduler_days"] = days
	}
	if prefs.MaxRatio != nil {
		ratio, err := strconv.ParseFloat(*prefs.MaxRatio, 64)
		if err != nil {
			return nil, fmt.Errorf("parsing maxRatio %q: %w", *prefs.MaxRatio, err)
		}
		out["max_ratio"] = ratio
	}
	if prefs.Encryption != "" {
		mode, ok := qbclient.Encryptions[prefs.Encryption]
		if !ok {
			return nil, fmt.Errorf("unknown encryption %q", prefs.Encryption)
		}
		out["encryption"] = mode
	}
	if prefs.MaxRatioAction != "" {
		act, ok := qbclient.MaxRatioActions[prefs.MaxRatioAction]
		if !ok {
			return nil, fmt.Errorf("unknown maxRatioAction %q", prefs.MaxRatioAction)
		}
		out["max_ratio_act"] = act
	}
	// A disabled limit is stored as -1, and qBittorrent ignores a limit sent with
	// it, so comparing the declared limit would differ on every pass.
	for _, pair := range qbLimitPairs {
		if out[pair.enabled] == false {
			delete(out, pair.limit)
		}
	}

	return out, nil
}

// qbBannedIPs is the banned addresses as qBittorrent stores them: in RFC 5952
// form, sorted, once each, a line apiece.
func qbBannedIPs(ips []string) (string, error) {
	out := make([]string, 0, len(ips))
	for _, ip := range ips {
		addr, err := netip.ParseAddr(ip)
		if err != nil {
			return "", fmt.Errorf("bannedIps: %q isn't an IP address", ip)
		}
		out = append(out, addr.String())
	}
	slices.Sort(out)
	return strings.Join(slices.Compact(out), "\n"), nil
}

// qbLimitPairs are the share limits qBittorrent reads as a pair: it applies a
// limit only when its switch comes with it, and a switch turned on without its
// limit sets the limit to 0, which finishes every torrent at once.
var qbLimitPairs = []struct{ enabled, limit, field string }{
	{"max_ratio_enabled", "max_ratio", "maxRatioEnabled"},
	{"max_seeding_time_enabled", "max_seeding_time", "maxSeedingTimeEnabled"},
	{"max_inactive_seeding_time_enabled", "max_inactive_seeding_time", "maxInactiveSeedingTimeEnabled"},
}

// completeLimitPairs sends the half of a limit pair the spec leaves out as
// qBittorrent has it. A limit whose switch is off in qBittorrent isn't applied.
func completeLimitPairs(payload, current map[string]any) error {
	for _, pair := range qbLimitPairs {
		on, declaredOn := payload[pair.enabled].(bool)
		_, declaredLimit := payload[pair.limit]
		switch {
		case declaredOn && on && !declaredLimit:
			stored, ok := current[pair.limit].(float64)
			if !ok || stored < 0 {
				return fmt.Errorf("%s is true, but the limit isn't set here or in qBittorrent", pair.field)
			}
			payload[pair.limit] = stored
		case declaredLimit && !declaredOn:
			if storedOn, _ := current[pair.enabled].(bool); storedOn {
				payload[pair.enabled] = true
			} else {
				delete(payload, pair.limit)
			}
		}
	}
	return nil
}

// qbPath is a path as qBittorrent stores it: without a trailing separator.
// Declaring one would otherwise read back as a change on every pass.
func qbPath(p string) string {
	trimmed := strings.TrimRight(p, `/\`)
	if trimmed == "" {
		return p
	}
	return trimmed
}

// reconcileQBPreferences writes only when qBittorrent's preferences differ;
// setPreferences on every pass would re-apply listen and WebUI settings.
func reconcileQBPreferences(
	ctx context.Context, qb *qbclient.Client, w *engine.Writes, prefs *downloadsv1alpha1.QBittorrentPreferences, secret secretResolver,
) error {
	payload, err := qbPreferencePayload(prefs, secret)
	if err != nil {
		return err
	}
	if len(payload) == 0 {
		return nil
	}
	current, err := qb.GetPreferences(ctx)
	if err != nil {
		return fmt.Errorf("reading preferences: %w", err)
	}
	if err := completeLimitPairs(payload, current); err != nil {
		return err
	}
	return w.Sync("settings", "preferences", current, payload, func(map[string]any) error {
		return qb.SetPreferences(ctx, payload)
	})
}

func (r *QBittorrentReconciler) connect(ctx context.Context, config *downloadsv1alpha1.QBittorrent) (*qbclient.Client, error) {
	username, err := reconciler.ResolveSecretKeyRef(ctx, r.Client, config.Namespace, config.Spec.Connection.UsernameSecretRef)
	if err != nil {
		return nil, err
	}
	password, err := reconciler.ResolveSecretKeyRef(ctx, r.Client, config.Namespace, config.Spec.Connection.PasswordSecretRef)
	if err != nil {
		return nil, err
	}
	tlsCfg, err := engine.ResolveTLSConfig(ctx, r.Client, config.Namespace, config.Spec.Connection.TLS)
	if err != nil {
		return nil, err
	}
	hc, err := engine.NewHTTPClient(config.Spec.Connection.URL, engine.AuthCookie, engine.WithTLSConfig(tlsCfg),
		engine.WithDisableRedirect(), engine.WithAppLabel("qbittorrent"))
	if err != nil {
		return nil, err
	}
	qb := qbclient.NewClient(hc, username, password)
	return qb, qb.Login(ctx)
}

func reconcileQBCategories(ctx context.Context, qb *qbclient.Client, pass *engine.Pass, categories []downloadsv1alpha1.QBittorrentCategory) []string {
	existing, err := qb.ListCategories(ctx)
	if err != nil {
		return []string{fmt.Sprintf("listing categories: %v", err)}
	}
	var errs []string
	for _, cat := range categories {
		if err := reconcileQBCategory(ctx, qb, pass, existing, cat); err != nil {
			log.FromContext(ctx).Error(err, "failed to reconcile category", "name", cat.Name)
			errs = append(errs, fmt.Sprintf("category(%s): %v", cat.Name, err))
		}
	}
	if !pass.PruneEnabled || len(errs) > 0 {
		return errs
	}
	err = pass.Prune(pass.Owned, "categories", qbCategoryNames(categories), slices.Collect(maps.Keys(existing)), func(name string) error {
		return qb.RemoveCategory(ctx, name)
	})
	if err != nil {
		return []string{fmt.Sprintf("categories(prune): %v", err)}
	}
	return nil
}

func qbCategoryNames(categories []downloadsv1alpha1.QBittorrentCategory) []string {
	names := make([]string, 0, len(categories))
	for _, c := range categories {
		names = append(names, c.Name)
	}
	return names
}

// cleanupQBittorrent deletes the RSS rules and feeds and the categories the
// operator created, for deletionPolicy delete.
func cleanupQBittorrent(ctx context.Context, qb *qbclient.Client, owned *engine.Ownership) error {
	if err := cleanupQBRSS(ctx, qb, owned); err != nil {
		return err
	}
	existing, err := qb.ListCategories(ctx)
	if err != nil {
		return err
	}
	return owned.Cleanup("categories", slices.Collect(maps.Keys(existing)), func(name string) error {
		return qb.RemoveCategory(ctx, name)
	})
}

func reconcileQBCategory(
	ctx context.Context, qb *qbclient.Client, pass *engine.Pass, existing map[string]any, cat downloadsv1alpha1.QBittorrentCategory,
) error {
	current, exists := existing[cat.Name].(map[string]any)
	if !exists {
		return pass.Apply("categories", cat.Name, func() error {
			if err := qb.CreateCategory(ctx, cat.Name, qbPath(cat.SavePath)); err != nil {
				return err
			}
			pass.Owned.Created("categories", cat.Name)
			return nil
		})
	}
	if cat.SavePath == "" {
		return nil
	}
	return pass.Sync("categories", cat.Name, current, map[string]any{"savePath": qbPath(cat.SavePath)}, func(map[string]any) error {
		return qb.EditCategory(ctx, cat.Name, qbPath(cat.SavePath), current)
	})
}

// SetupWithManager sets up the controller with the Manager.
func (r *QBittorrentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&downloadsv1alpha1.QBittorrent{}, builder.WithPredicates(ctrlcommon.ConfigChangedPredicate())).
		WatchesMetadata(&corev1.Secret{}, ctrlcommon.EnqueueForSecret(r.Client, &downloadsv1alpha1.QBittorrentList{})).
		Named("qbittorrent").
		Complete(r)
}
