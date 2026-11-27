package downloads

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net"
	"net/url"
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
	sabnzbdclient "github.com/kyleseneker/arrchitect/internal/client/sabnzbd"
	ctrlcommon "github.com/kyleseneker/arrchitect/internal/controller/common"
	"github.com/kyleseneker/arrchitect/internal/engine"
	"github.com/kyleseneker/arrchitect/internal/reconciler"
)

type SabnzbdReconciler struct {
	client.Client
	Recorder events.EventRecorder
}

// +kubebuilder:rbac:groups=arrchitect.dev,resources=sabnzbds,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=arrchitect.dev,resources=sabnzbds/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=arrchitect.dev,resources=sabnzbds/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

func (r *SabnzbdReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	integration := ctrlcommon.Integration[*downloadsv1alpha1.Sabnzbd, *sabnzbdclient.Client]{
		App:     "sabnzbd",
		Connect: r.connect,
		Check:   r.check,
		Sync:    r.sync,
		Cleanup: cleanupSabnzbd,
		Prune:   true,
	}
	return integration.Reconcile(ctx, r.Client, r.Recorder, req, &downloadsv1alpha1.Sabnzbd{})
}

func (r *SabnzbdReconciler) connect(ctx context.Context, config *downloadsv1alpha1.Sabnzbd) (*sabnzbdclient.Client, error) {
	apiKey, err := reconciler.ResolveSecretKeyRef(ctx, r.Client, config.Namespace, config.Spec.Connection.APIKeySecretRef)
	if err != nil {
		return nil, err
	}
	tlsCfg, err := engine.ResolveTLSConfig(ctx, r.Client, config.Namespace, config.Spec.Connection.TLS)
	if err != nil {
		return nil, err
	}
	hc, err := engine.NewHTTPClient(config.Spec.Connection.URL, engine.AuthNone, engine.WithTLSConfig(tlsCfg), engine.WithAppLabel("sabnzbd"))
	if err != nil {
		return nil, err
	}
	return sabnzbdclient.NewClient(hc, apiKey), nil
}

// check refuses a host whitelist that would lock the operator out, then checks
// SABnzbd answers. Cleanup skips it, so a bad whitelist never blocks deletion.
func (r *SabnzbdReconciler) check(ctx context.Context, sc *sabnzbdclient.Client, config *downloadsv1alpha1.Sabnzbd) error {
	if config.Spec.General != nil {
		if err := sabnzbdHostAllowed(config.Spec.Connection.URL, config.Spec.General.HostWhitelist); err != nil {
			return engine.InvalidConfig(err)
		}
	}
	return sc.Ping(ctx)
}

func (r *SabnzbdReconciler) sync(ctx context.Context, sc *sabnzbdclient.Client, config *downloadsv1alpha1.Sabnzbd, pass *engine.Pass) []error {
	current, err := sc.GetConfig(ctx)
	if err != nil {
		return []error{fmt.Errorf("reading config: %w", err)}
	}
	servers, problems := r.resolveSabnzbdServers(ctx, config)
	apprise, err := r.sabnzbdAppriseValues(ctx, config)
	if err != nil {
		problems = append(problems, fmt.Errorf("apprise: %w", err))
	}
	login, err := r.sabnzbdLoginValues(ctx, config)
	if err != nil {
		problems = append(problems, fmt.Errorf("login: %w", err))
	}
	plan := func(current map[string]any) []sabWrite {
		writes := planSabnzbd(current, config.Spec, servers, pass.Owner)
		writes = append(writes, planKeywords(current, "apprise", apprise)...)
		return append(writes, planLogin(current, login, pass.Owner, pass.Observe)...)
	}
	writes := plan(current)
	problems = append(problems, applySabnzbd(ctx, sc, pass, writes)...)
	// A fresh SABnzbd creates its default categories on the first category
	// write and discards that write, so check once more after writing.
	if len(writes) > 0 && len(problems) == 0 && !pass.Observe {
		if current, err = sc.GetConfig(ctx); err != nil {
			problems = append(problems, fmt.Errorf("re-reading config: %w", err))
		} else {
			again, refused := sabnzbdRefused(writes, plan(current))
			problems = append(problems, refused...)
			problems = append(problems, applySabnzbd(ctx, sc, pass, again)...)
		}
	}
	if !pass.PruneEnabled || len(problems) > 0 {
		return problems
	}
	if !pass.Observe {
		if current, err = sc.GetConfig(ctx); err != nil {
			return append(problems, fmt.Errorf("reading config to prune: %w", err))
		}
	}
	return append(problems, pruneSabnzbd(ctx, sc, current, config.Spec, pass)...)
}

func (r *SabnzbdReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&downloadsv1alpha1.Sabnzbd{}, builder.WithPredicates(ctrlcommon.ConfigChangedPredicate())).
		WatchesMetadata(&corev1.Secret{}, ctrlcommon.EnqueueForSecret(r.Client, &downloadsv1alpha1.SabnzbdList{})).
		Named("sabnzbd").
		Complete(r)
}

func buildSabnzbdFolderValues(f *downloadsv1alpha1.SabnzbdFolders) map[string]string {
	return sabnzbdValues(f)
}

// sabnzbdAgeSort is the queue order SABnzbd 4 and later use for the old
// auto_sort on setting; auto_sort now holds a field and a direction.
const sabnzbdAgeSort = "avg_age asc"

func buildSabnzbdGeneralValues(g *downloadsv1alpha1.SabnzbdGeneral) map[string]string {
	vals := sabnzbdValues(g)
	if g.AutoSort != nil {
		vals["auto_sort"] = ""
		if *g.AutoSort {
			vals["auto_sort"] = sabnzbdAgeSort
		}
	}
	return vals
}

// sabnzbdValues reads the settings in sabnzbdclient's tables from a spec
// section, as the text set_config takes.
func sabnzbdValues(section any) map[string]string {
	vals := map[string]string{}
	v := reflect.ValueOf(section).Elem()
	for i := range v.NumField() {
		name, _, _ := strings.Cut(v.Type().Field(i).Tag.Get("json"), ",")
		field := v.Field(i)
		if field.IsZero() {
			continue
		}
		if key, ok := sabnzbdclient.Folders[name]; ok {
			vals[key] = field.String()
		}
		if key, ok := sabnzbdclient.Switches[name]; ok {
			vals[key] = ctrlcommon.BoolTo01(field.Elem().Bool())
		}
		if key, ok := sabnzbdclient.Texts[name]; ok {
			vals[key] = field.String()
		}
		if key, ok := sabnzbdclient.Numbers[name]; ok {
			vals[key] = strconv.FormatInt(reflect.Indirect(field).Int(), 10)
		}
		if key, ok := sabnzbdclient.Lists[name]; ok {
			vals[key] = strings.Join(field.Interface().([]string), ",")
		}
		if choice, ok := sabnzbdclient.Choices[name]; ok {
			vals[choice.Key] = choice.Values[field.String()]
		}
	}
	return vals
}

// sabnzbdHostAllowed checks that hostWhitelist keeps the host the operator
// connects to. SABnzbd refuses any other host name, so leaving it out would
// lock the operator out.
func sabnzbdHostAllowed(connection string, whitelist []string) error {
	if len(whitelist) == 0 {
		return nil
	}
	u, err := url.Parse(connection)
	if err != nil {
		return err
	}
	host := strings.ToLower(u.Hostname())
	if host == "localhost" || net.ParseIP(host) != nil || strings.HasSuffix(strings.TrimSuffix(host, "."), ".local") ||
		slices.Contains(whitelist, host) {
		return nil
	}
	return fmt.Errorf("general.hostWhitelist must include %s, the host in connection.url; SABnzbd refuses host names it doesn't list", host)
}

func sabnzbdServerValues(srv downloadsv1alpha1.SabnzbdServer) map[string]string {
	vals := map[string]string{
		"host": srv.Host,
		"port": strconv.Itoa(int(srv.Port)),
	}
	for key, value := range map[string]*bool{"ssl": srv.SSL, "enable": srv.Enable, "optional": srv.Optional, "required": srv.Required} {
		if value != nil {
			vals[key] = ctrlcommon.BoolTo01(*value)
		}
	}
	for key, value := range map[string]*int32{
		"connections": srv.Connections, "priority": srv.Priority, "retention": srv.Retention,
	} {
		if value != nil {
			vals[key] = strconv.Itoa(int(*value))
		}
	}
	if srv.Timeout != 0 {
		vals["timeout"] = strconv.Itoa(int(srv.Timeout))
	}
	if srv.SSLVerify != "" {
		vals["ssl_verify"] = sabnzbdclient.SSLVerify[srv.SSLVerify]
	}
	if srv.SSLCiphers != "" {
		vals["ssl_ciphers"] = srv.SSLCiphers
	}
	return vals
}

func sabnzbdQuotaValues(q downloadsv1alpha1.SabnzbdQuota) map[string]string {
	vals := map[string]string{}
	if q.Enable != nil && !*q.Enable {
		vals["quota_size"] = ""
	} else if q.Size != "" {
		vals["quota_size"] = q.Size
	}
	if q.Period != "" {
		vals["quota_period"] = sabnzbdclient.QuotaPeriods[q.Period]
	}
	// SABnzbd reads quota_day as "<day> <hh:mm>", and a lone hh:mm as a day.
	if q.ResetDay != 0 || q.ResetTime != "" {
		vals["quota_day"] = strings.TrimSpace(fmt.Sprintf("%d %s", max(q.ResetDay, 1), q.ResetTime))
	}
	if q.Resume != nil {
		vals["quota_resume"] = ctrlcommon.BoolTo01(*q.Resume)
	}
	return vals
}

// sabnzbdRefused splits the second pass's writes into those to retry and the
// settings SABnzbd didn't keep. SABnzbd answers a setting it refuses, such as a
// script it can't find, as if it took it, so a setting that still differs
// after being written is reported instead of rewritten every pass. Servers and
// categories are retried: a fresh SABnzbd drops the first category write.
func sabnzbdRefused(first, again []sabWrite) ([]sabWrite, []error) {
	written := map[string]string{}
	for _, w := range first {
		if w.section == "misc" || w.section == "apprise" {
			written[w.section+"."+w.name] = w.values["value"]
		}
	}
	var retry []sabWrite
	var refused []error
	for _, w := range again {
		if v, ok := written[w.section+"."+w.name]; ok && v == w.values["value"] {
			refused = append(refused, fmt.Errorf("%s(%s.%s): SABnzbd didn't keep the value written", w.resourceType, w.section, w.name))
			continue
		}
		retry = append(retry, w)
	}
	return retry, refused
}

// applySabnzbd performs the planned writes and returns the failures.
func applySabnzbd(ctx context.Context, sc *sabnzbdclient.Client, pass *engine.Pass, writes []sabWrite) []error {
	var problems []error
	for _, w := range writes {
		err := pass.Apply(w.resourceType, w.name, func() error {
			var err error
			if w.section == "misc" || w.section == "apprise" {
				err = sc.SetConfig(ctx, w.section, w.name, w.values["value"])
			} else {
				err = sc.SetNamed(ctx, w.section, w.name, w.values)
			}
			if err != nil {
				return err
			}
			pass.RecordSecrets(w.secretKey, w.secretDigest)
			if w.create {
				pass.Owned.Created(w.section, w.name)
			}
			return nil
		})
		if err != nil {
			log.FromContext(ctx).Error(err, "failed to write SABnzbd config", "section", w.section, "name", w.name)
			problems = append(problems, fmt.Errorf("%s(%s): %w", w.resourceType, w.name, err))
		}
	}
	return problems
}

// pruneSabnzbd deletes the servers and categories the operator created that the
// spec no longer declares.
func pruneSabnzbd(ctx context.Context, sc *sabnzbdclient.Client, current map[string]any, spec downloadsv1alpha1.SabnzbdSpec, pass *engine.Pass) []error {
	declared := map[string][]string{}
	for _, s := range spec.Servers {
		declared["servers"] = append(declared["servers"], s.Name)
	}
	for _, c := range spec.Categories {
		declared["categories"] = append(declared["categories"], c.Name)
	}
	var problems []error
	for _, section := range []string{"servers", "categories"} {
		err := pass.Prune(pass.Owned, section, declared[section], sabnzbdNames(current, section), func(name string) error {
			return sc.DeleteNamed(ctx, section, name)
		})
		if err != nil {
			problems = append(problems, fmt.Errorf("%s(prune): %w", section, err))
		}
	}
	return problems
}

// cleanupSabnzbd deletes the servers and categories the operator created, for
// deletionPolicy delete.
func cleanupSabnzbd(ctx context.Context, sc *sabnzbdclient.Client, owned *engine.Ownership) error {
	current, err := sc.GetConfig(ctx)
	if err != nil {
		return err
	}
	var errs []error
	for _, section := range []string{"servers", "categories"} {
		errs = append(errs, owned.Cleanup(section, sabnzbdNames(current, section), func(name string) error {
			return sc.DeleteNamed(ctx, section, name)
		}))
	}
	return errors.Join(errs...)
}

func sabnzbdNames(current map[string]any, section string) []string {
	entries, _ := current[section].([]any)
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		entry, _ := e.(map[string]any)
		if name, _ := entry["name"].(string); name != "" {
			names = append(names, name)
		}
	}
	return names
}

// sabWrite is one set_config call the reconcile needs.
type sabWrite struct {
	section      string // misc, servers or categories
	name         string // the misc keyword, or the server or category name
	create       bool
	values       map[string]string
	resourceType string
	secretKey    string
	secretDigest string
}

// resolveSabnzbdServers reads each server's credentials. A server whose Secret
// cannot be read is skipped and reported, so the rest still reconcile.
func (r *SabnzbdReconciler) resolveSabnzbdServers(
	ctx context.Context, config *downloadsv1alpha1.Sabnzbd,
) (map[string]map[string]string, []error) {
	servers := map[string]map[string]string{}
	var problems []error
	for _, srv := range config.Spec.Servers {
		vals := sabnzbdServerValues(srv)
		ok := true
		for field, ref := range map[string]*commonv1alpha1.SecretKeyRef{"username": srv.UsernameSecretRef, "password": srv.PasswordSecretRef} {
			if ref == nil {
				continue
			}
			value, err := reconciler.ResolveSecretKeyRef(ctx, r.Client, config.Namespace, *ref)
			if err != nil {
				problems = append(problems, fmt.Errorf("server(%s).%s: %w", srv.Name, field, err))
				ok = false
				continue
			}
			vals[field] = value
		}
		if ok {
			servers[srv.Name] = vals
		}
	}
	return servers, problems
}

// planKeywords returns a write for each keyword of a section whose value
// differs from get_config's.
func planKeywords(current map[string]any, section string, values map[string]string) []sabWrite {
	var writes []sabWrite
	stored, _ := current[section].(map[string]any)
	for _, key := range slices.Sorted(maps.Keys(values)) {
		// A switch reads back as 0/1 until written and as true/false after, and
		// a list as an array of what was written comma-separated.
		value := stored[key]
		switch v := value.(type) {
		case bool:
			value = ctrlcommon.BoolTo01(v)
		case []any:
			items := make([]string, 0, len(v))
			for _, item := range v {
				items = append(items, fmt.Sprint(item))
			}
			value = strings.Join(items, ",")
		}
		out, err := reconciler.MergeDesired(map[string]any{key: value}, map[string]any{key: values[key]})
		if err == nil && !out.Changed {
			continue
		}
		writes = append(writes, sabWrite{section: section, name: key, values: map[string]string{"value": values[key]}, resourceType: "settings"})
	}
	return writes
}

// sabnzbdAppriseValues is the apprise section's declared keywords, with the
// URLs read from their Secret.
func (r *SabnzbdReconciler) sabnzbdAppriseValues(ctx context.Context, config *downloadsv1alpha1.Sabnzbd) (map[string]string, error) {
	a := config.Spec.Apprise
	values := map[string]string{}
	if a.Enable != nil {
		values["apprise_enable"] = ctrlcommon.BoolTo01(*a.Enable)
	}
	if a.Categories != nil {
		values["apprise_cats"] = strings.Join(a.Categories, ",")
	}
	if a.Events != nil {
		for event, name := range sabnzbdclient.AppriseEvents {
			values["apprise_target_"+name+"_enable"] = ctrlcommon.BoolTo01(slices.Contains(a.Events, event))
		}
	}
	if a.URLsSecretRef != nil {
		urls, err := reconciler.ResolveSecretKeyRef(ctx, r.Client, config.Namespace, *a.URLsSecretRef)
		if err != nil {
			return nil, fmt.Errorf("urls: %w", err)
		}
		values["apprise_urls"] = urls
	}
	return values, nil
}

// sabnzbdLoginValues reads the web interface login from its Secrets.
func (r *SabnzbdReconciler) sabnzbdLoginValues(ctx context.Context, config *downloadsv1alpha1.Sabnzbd) (map[string]string, error) {
	l := config.Spec.Login
	if l == (downloadsv1alpha1.SabnzbdLogin{}) {
		return nil, nil
	}
	values := map[string]string{}
	for field, ref := range map[string]commonv1alpha1.SecretKeyRef{"username": l.UsernameSecretRef, "password": l.PasswordSecretRef} {
		value, err := reconciler.ResolveSecretKeyRef(ctx, r.Client, config.Namespace, ref)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", field, err)
		}
		values[field] = value
	}
	return values, nil
}

// planLogin plans the web interface login. The password reads back masked
// once set, so a change is noticed through the remembered digest.
func planLogin(current map[string]any, login map[string]string, owner string, observe bool) []sabWrite {
	if login == nil {
		return nil
	}
	writes := planKeywords(current, "misc", map[string]string{"username": login["username"]})
	misc, _ := current["misc"].(map[string]any)
	stored, _ := misc["password"].(string)
	password := login["password"]
	w := sabWrite{section: "misc", name: "password", values: map[string]string{"value": password}, resourceType: "settings"}
	if password == "" {
		if stored != "" {
			writes = append(writes, w)
		}
		return writes
	}
	w.secretKey = "misc/password"
	w.secretDigest = maskedDigest("password", password)
	if !reconciler.IsMasked(stored) || reconciler.SecretsDiffer(owner, w.secretKey, w.secretDigest, observe) {
		writes = append(writes, w)
	}
	return writes
}

// maskedDigest is the digest of a secret SABnzbd reads back masked.
func maskedDigest(field, value string) string {
	out, err := reconciler.MergeDesired(map[string]any{field: "**********"}, map[string]any{field: value})
	if err != nil {
		return ""
	}
	return out.SecretDigest
}

// planSabnzbd compares the declared settings with get_config and returns only
// the writes needed. Writing every setting on every pass made each reconcile a
// burst of set_config calls and hid which settings actually drifted.
func planSabnzbd(
	current map[string]any, spec downloadsv1alpha1.SabnzbdSpec, servers map[string]map[string]string, owner string,
) []sabWrite {
	var writes []sabWrite
	observe := ctrlcommon.ObserveOnly(spec.Reconcile)

	// Folders go first: SABnzbd takes a script only once script_dir holds it.
	if spec.Folders != nil {
		writes = append(writes, planKeywords(current, "misc", buildSabnzbdFolderValues(spec.Folders))...)
	}
	misc := sabnzbdQuotaValues(spec.Quota)
	if spec.General != nil {
		maps.Copy(misc, buildSabnzbdGeneralValues(spec.General))
	}
	writes = append(writes, planKeywords(current, "misc", misc)...)

	for _, srv := range spec.Servers {
		vals, ok := servers[srv.Name]
		if !ok {
			continue
		}
		vals = withServerQuota(current, srv.Name, vals)
		key := fmt.Sprintf("server/%q", srv.Name)
		if w, needed := planNamed(current, "servers", srv.Name, vals, owner, key, observe); needed {
			w.resourceType = "servers"
			writes = append(writes, w)
		}
	}

	for _, cat := range spec.Categories {
		if w, needed := planNamed(current, "categories", cat.Name, sabnzbdCategoryValues(cat), owner, "", observe); needed {
			w.resourceType = "categories"
			writes = append(writes, w)
		}
	}
	return writes
}

// planNamed decides whether a server or category needs writing. Passwords read
// back masked, so a change there is noticed through the remembered digest.
func planNamed(current map[string]any, section, name string, vals map[string]string, owner, secretKey string, observe bool) (sabWrite, bool) {
	w := sabWrite{section: section, name: name, values: vals, secretKey: secretKey}
	entries, _ := current[section].([]any)
	for _, e := range entries {
		entry, _ := e.(map[string]any)
		if entry["name"] != name {
			continue
		}
		desired := make(map[string]any, len(vals))
		for k, v := range vals {
			desired[k] = v
		}
		out, err := reconciler.MergeDesired(entry, desired)
		if err != nil {
			return w, true
		}
		w.secretDigest = out.SecretDigest
		return w, out.Changed || reconciler.SecretsDiffer(owner, secretKey, out.SecretDigest, observe)
	}
	w.create = true
	// A new entry reads back with its password masked; remember what is
	// written now, or the next pass rewrites it once.
	if password, ok := vals["password"]; ok {
		w.secretDigest = maskedDigest("password", password)
	}
	return w, true
}

// withServerQuota adds the server's stored quota to a write. SABnzbd restarts
// a server's quota count whenever a write's quota differs from the stored one,
// and a write without a quota counts as one that differs.
func withServerQuota(current map[string]any, name string, vals map[string]string) map[string]string {
	entries, _ := current["servers"].([]any)
	for _, e := range entries {
		entry, _ := e.(map[string]any)
		if quota, _ := entry["quota"].(string); entry["name"] == name && quota != "" {
			vals = maps.Clone(vals)
			vals["quota"] = quota
		}
	}
	return vals
}

func sabnzbdCategoryValues(cat downloadsv1alpha1.SabnzbdCategory) map[string]string {
	vals := map[string]string{}
	if cat.Dir != "" {
		vals["dir"] = cat.Dir
	}
	if cat.Script != "" {
		vals["script"] = cat.Script
	}
	if cat.Priority != "" {
		vals["priority"] = sabnzbdclient.Priorities[cat.Priority]
	}
	if cat.PostProcessing != "" {
		vals["pp"] = sabnzbdclient.PostProcessing[cat.PostProcessing]
	}
	return vals
}
