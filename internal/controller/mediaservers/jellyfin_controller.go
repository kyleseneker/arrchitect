package mediaservers

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
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	mediaserversv1alpha1 "github.com/kyleseneker/arrchitect/api/mediaservers/v1alpha1"
	jellyfinclient "github.com/kyleseneker/arrchitect/internal/client/jellyfin"
	ctrlcommon "github.com/kyleseneker/arrchitect/internal/controller/common"
	"github.com/kyleseneker/arrchitect/internal/engine"
	"github.com/kyleseneker/arrchitect/internal/reconciler"
)

// JellyfinReconciler reconciles a Jellyfin object.
type JellyfinReconciler struct {
	client.Client
	Recorder events.EventRecorder
}

// +kubebuilder:rbac:groups=arrchitect.dev,resources=jellyfins,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=arrchitect.dev,resources=jellyfins/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=arrchitect.dev,resources=jellyfins/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

func (r *JellyfinReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	integration := ctrlcommon.Integration[*mediaserversv1alpha1.Jellyfin, *jellyfinConn]{
		App:     "jellyfin",
		Connect: r.connect,
		Sync:    r.sync,
	}
	return integration.Reconcile(ctx, r.Client, r.Recorder, req, &mediaserversv1alpha1.Jellyfin{})
}

// jellyfinAdmin is the admin user the setup wizard creates and the operator
// signs in as.
type jellyfinAdmin struct{ username, password string }

// jellyfinConn is a Jellyfin the operator has reached, signed in to once its
// setup is complete.
type jellyfinConn struct {
	jf            *jellyfinclient.Client
	admin         jellyfinAdmin
	setupComplete bool
}

func (r *JellyfinReconciler) connect(ctx context.Context, config *mediaserversv1alpha1.Jellyfin) (*jellyfinConn, error) {
	username, err := reconciler.ResolveSecretKeyRef(ctx, r.Client, config.Namespace, config.Spec.AdminUser.UsernameSecretRef)
	if err != nil {
		return nil, err
	}
	password, err := reconciler.ResolveSecretKeyRef(ctx, r.Client, config.Namespace, config.Spec.AdminUser.PasswordSecretRef)
	if err != nil {
		return nil, err
	}
	tlsCfg, err := engine.ResolveTLSConfig(ctx, r.Client, config.Namespace, config.Spec.Connection.TLS)
	if err != nil {
		return nil, err
	}
	hc, err := engine.NewHTTPClient(config.Spec.Connection.URL, engine.AuthMediaBrowser, engine.WithTLSConfig(tlsCfg), engine.WithAppLabel("jellyfin"))
	if err != nil {
		return nil, err
	}
	conn := &jellyfinConn{jf: jellyfinclient.NewClient(hc), admin: jellyfinAdmin{username: username, password: password}}
	if conn.setupComplete, err = conn.jf.IsSetupComplete(ctx); err != nil || !conn.setupComplete {
		return conn, err
	}
	if err := r.signIn(ctx, conn.jf, config, conn.admin); err != nil {
		return nil, fmt.Errorf("authentication: %w", err)
	}
	return conn, nil
}

func (r *JellyfinReconciler) sync(ctx context.Context, conn *jellyfinConn, config *mediaserversv1alpha1.Jellyfin, pass *engine.Pass) []error {
	logger := log.FromContext(ctx)
	jf := conn.jf
	if !conn.setupComplete {
		config.Status.Initialized = &conn.setupComplete
		err := pass.Apply("settings", "initialization", func() error {
			return runJellyfinSetup(ctx, jf, config.Spec.Server, conn.admin)
		})
		if err != nil {
			logger.Error(err, "failed to run setup wizard")
			pass.Retry = time.Minute
			return []error{fmt.Errorf("setup wizard: %w", err)}
		}
		if pass.Observe {
			return nil
		}
		logger.Info("Jellyfin setup wizard completed")
		if err := r.signIn(ctx, jf, config, conn.admin); err != nil {
			_, pass.Retry = ctrlcommon.Classify(err)
			return []error{fmt.Errorf("authentication: %w", err)}
		}
	}
	initialized := true
	config.Status.Initialized = &initialized

	var problems []error
	version := jellyfinVersion(ctx, jf)
	for _, section := range jellyfinSections(config.Spec) {
		if err := reconcileJellyfinSection(ctx, jf, &pass.Writes, section, version); err != nil {
			logger.Error(err, "failed to reconcile settings", "section", section.name)
			problems = append(problems, fmt.Errorf("%s: %w", section.name, err))
		}
	}
	problems = append(problems, reconcileJellyfinTasks(ctx, jf, config.Spec.ScheduledTasks, &pass.Writes)...)
	for _, lib := range config.Spec.Libraries {
		if err := reconcileJellyfinLibrary(ctx, jf, &pass.Writes, lib); err != nil {
			logger.Error(err, "failed to reconcile library", "name", lib.Name)
			problems = append(problems, fmt.Errorf("library(%s): %w", lib.Name, err))
		}
	}
	return problems
}

// runJellyfinSetup completes a new Jellyfin's setup wizard, creating the admin
// user the operator signs in as.
func runJellyfinSetup(ctx context.Context, jf *jellyfinclient.Client, server *mediaserversv1alpha1.JellyfinServer, admin jellyfinAdmin) error {
	serverName, metadataLang, countryCode := "", "en", "US"
	if server != nil {
		if server.ServerName != "" {
			serverName = server.ServerName
		}
		if server.PreferredMetadataLanguage != "" {
			metadataLang = server.PreferredMetadataLanguage
		}
		if server.MetadataCountryCode != "" {
			countryCode = server.MetadataCountryCode
		}
	}
	return jf.RunSetupWizard(ctx, admin.username, admin.password, serverName, metadataLang, countryCode)
}

// signIn authenticates with the connection's API key when it sets one, or as
// the admin user.
func (r *JellyfinReconciler) signIn(
	ctx context.Context, jf *jellyfinclient.Client, config *mediaserversv1alpha1.Jellyfin, admin jellyfinAdmin,
) error {
	ref := config.Spec.Connection.APIKeySecretRef
	if ref == nil {
		return jf.Authenticate(ctx, admin.username, admin.password)
	}
	key, err := reconciler.ResolveSecretKeyRef(ctx, r.Client, config.Namespace, *ref)
	if err != nil {
		return err
	}
	return jf.UseAPIKey(ctx, key)
}

// jellyfinSettings renders a spec section under Jellyfin's own PascalCase
// setting names. Jellyfin accepts camelCase on write but reads back PascalCase,
// so camelCase keys always looked missing and every pass rewrote the settings.
func jellyfinSettings(section any) (map[string]any, error) {
	raw, err := json.Marshal(section)
	if err != nil {
		return nil, err
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, err
	}
	return pascalKeys(fields), nil
}

func pascalKeys(fields map[string]any) map[string]any {
	out := make(map[string]any, len(fields))
	for k, v := range fields {
		if nested, ok := v.(map[string]any); ok {
			v = pascalKeys(nested)
		}
		out[strings.ToUpper(k[:1])+k[1:]] = v
	}
	return out
}

type jellyfinSection struct {
	name     string
	path     string
	settings any
}

func jellyfinSections(spec mediaserversv1alpha1.JellyfinSpec) []jellyfinSection {
	var sections []jellyfinSection
	if spec.Encoding != nil {
		sections = append(sections, jellyfinSection{"encoding", "/System/Configuration/encoding", spec.Encoding})
	}
	if spec.Server != nil {
		sections = append(sections, jellyfinSection{"server", "/System/Configuration", spec.Server})
	}
	if spec.Network != nil {
		sections = append(sections, jellyfinSection{"network", "/System/Configuration/network", spec.Network})
	}
	if spec.Branding != nil {
		sections = append(sections, jellyfinSection{"branding", "/System/Configuration/branding", spec.Branding})
	}
	return sections
}

func reconcileJellyfinSection(ctx context.Context, jf *jellyfinclient.Client, w *engine.Writes, section jellyfinSection, version string) error {
	current, err := jf.GetConfig(ctx, section.path)
	if err != nil {
		return fmt.Errorf("getting current config: %w", err)
	}
	desired, err := jellyfinSettings(section.settings)
	if err != nil {
		return err
	}
	missing := withoutNewerSettings(section.name, desired, version)
	merged, changed, err := reconciler.MergeDesiredOverCurrent(current, desired)
	if err != nil {
		return fmt.Errorf("merging config: %w", err)
	}
	if changed {
		if err := w.Apply("settings", section.path, func() error { return jf.PostConfig(ctx, section.path, merged) }); err != nil {
			return err
		}
	}
	if len(missing) > 0 {
		return errors.New(strings.Join(missing, "; "))
	}
	return nil
}

// jellyfinSettingsSince lists settings older Jellyfin releases don't have, by
// the release that added them. Jellyfin leaves unset settings out of what it
// returns, so a missing one can't tell an older release apart.
var jellyfinSettingsSince = map[string]map[string]string{
	"encoding": {"SubtitleExtractionTimeoutMinutes": "12.0"},
}

// jellyfinVersion is the server's version, or "" when it can't be read, which
// leaves every setting in.
func jellyfinVersion(ctx context.Context, jf *jellyfinclient.Client) string {
	version, err := jf.Version(ctx)
	if err != nil {
		log.FromContext(ctx).V(1).Info("reading the Jellyfin version", "error", err.Error())
	}
	return version
}

// withoutNewerSettings removes from desired the settings version doesn't have
// and describes each one.
func withoutNewerSettings(section string, desired map[string]any, version string) []string {
	var missing []string
	for name, since := range jellyfinSettingsSince[section] {
		if _, ok := desired[name]; !ok || version == "" || !versionBefore(version, since) {
			continue
		}
		delete(desired, name)
		missing = append(missing, fmt.Sprintf("%s.%s needs Jellyfin %s or later, and this server runs %s",
			section, strings.ToLower(name[:1])+name[1:], since, version))
	}
	slices.Sort(missing)
	return missing
}

// versionBefore reports whether dotted version a is older than b.
func versionBefore(a, b string) bool {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := range max(len(as), len(bs)) {
		var x, y int
		if i < len(as) {
			x, _ = strconv.Atoi(as[i])
		}
		if i < len(bs) {
			y, _ = strconv.Atoi(bs[i])
		}
		if x != y {
			return x < y
		}
	}
	return false
}

// reconcileJellyfinLibrary creates a library, or keeps an existing one's
// folders and options as declared. Folders are added before any is removed.
func reconcileJellyfinLibrary(ctx context.Context, jf *jellyfinclient.Client, w *engine.Writes, lib mediaserversv1alpha1.JellyfinLibrary) error {
	current, err := findJellyfinLibrary(ctx, jf, lib.Name)
	if err != nil {
		return err
	}
	if current == nil {
		options, err := jellyfinLibraryDesired(ctx, jf, lib, nil)
		if err != nil {
			return err
		}
		options["PathInfos"] = buildPathInfos(lib.Paths)
		return w.Apply("libraries", lib.Name, func() error { return jf.CreateLibrary(ctx, lib.Name, lib.CollectionType, options) })
	}
	if have := jellyfinCollectionType(current); have != lib.CollectionType {
		return fmt.Errorf("the library holds %s in Jellyfin; changing its type means recreating it", have)
	}

	have := jellyfinLocations(current)
	want := cleanPaths(lib.Paths)
	changed := false
	for _, path := range want {
		if !slices.Contains(have, path) {
			if err := w.Apply("libraries", lib.Name, func() error { return jf.AddLibraryPath(ctx, lib.Name, path) }); err != nil {
				return fmt.Errorf("adding %s: %w", path, err)
			}
			changed = !w.Observe
		}
	}
	for _, path := range have {
		if !slices.Contains(want, path) {
			if err := w.Apply("libraries", lib.Name, func() error { return jf.RemoveLibraryPath(ctx, lib.Name, path) }); err != nil {
				return fmt.Errorf("removing %s: %w", path, err)
			}
			changed = !w.Observe
		}
	}
	// The options include the folders, so they are saved from a fresh read.
	if changed {
		if current, err = findJellyfinLibrary(ctx, jf, lib.Name); err != nil {
			return err
		}
		if current == nil {
			return fmt.Errorf("the library disappeared while its folders were changed")
		}
	}
	options, _ := current["LibraryOptions"].(map[string]any)
	declared, err := jellyfinLibraryDesired(ctx, jf, lib, options)
	if err != nil || len(declared) == 0 {
		return err
	}
	merged, differs, err := reconciler.MergeDesiredOverCurrent(options, declared)
	if err != nil || !differs {
		return err
	}
	id, _ := current["ItemId"].(string)
	return w.Apply("libraries", lib.Name, func() error { return jf.UpdateLibraryOptions(ctx, id, merged) })
}

func findJellyfinLibrary(ctx context.Context, jf *jellyfinclient.Client, name string) (map[string]any, error) {
	libraries, err := jf.ListLibraries(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing libraries: %w", err)
	}
	return singleLibrary(libraries, "Jellyfin", "Name", name)
}

// singleLibrary returns the library of app whose field holds name, or nil when
// none does. Libraries are kept as declared, folders included, so two with one
// name is an error rather than a guess that could empty the wrong one.
func singleLibrary(libraries []map[string]any, app, field, name string) (map[string]any, error) {
	var found map[string]any
	for _, l := range libraries {
		if l[field] != name {
			continue
		}
		if found != nil {
			return nil, fmt.Errorf("%s has more than one library named %q", app, name)
		}
		found = l
	}
	return found, nil
}

// jellyfinLibraryOptions is a library's declared options under Jellyfin's
// names: every field but its name, type, folders and fetchers.
func jellyfinLibraryOptions(lib mediaserversv1alpha1.JellyfinLibrary) map[string]any {
	options, err := jellyfinSettings(lib)
	if err != nil {
		return map[string]any{}
	}
	for _, key := range []string{
		"Name", "CollectionType", "Paths", "TypeOptions", "SubtitleFetchers", "LyricFetchers", "MediaSegmentProviders", "LocalMetadataReaders",
	} {
		delete(options, key)
	}
	return options
}

// jellyfinLibraryDesired is the options a library is to have: those declared,
// and the fetchers, worked out against the options it has and the fetchers
// Jellyfin offers.
func jellyfinLibraryDesired(
	ctx context.Context, jf *jellyfinclient.Client, lib mediaserversv1alpha1.JellyfinLibrary, current map[string]any,
) (map[string]any, error) {
	options := jellyfinLibraryOptions(lib)
	if len(lib.TypeOptions) == 0 && len(lib.SubtitleFetchers) == 0 && len(lib.LyricFetchers) == 0 &&
		len(lib.MediaSegmentProviders) == 0 && len(lib.LocalMetadataReaders) == 0 {
		return options, nil
	}
	available, err := jf.AvailableLibraryOptions(ctx, lib.CollectionType)
	if err != nil {
		return nil, fmt.Errorf("listing the library's fetchers: %w", err)
	}
	maps.Copy(options, jellyfinFetchers(lib, current, available))
	return options, nil
}

// jellyfinFetchers turns the declared fetchers, each list the ones used in
// priority order, into what Jellyfin stores. A library keeps the fetchers it
// disables and an order; a type option keeps the ones it enables.
func jellyfinFetchers(lib mediaserversv1alpha1.JellyfinLibrary, current, available map[string]any) map[string]any {
	out := map[string]any{}
	for _, f := range []struct {
		declared                   []string
		disabled, order, available string
	}{
		{lib.SubtitleFetchers, "DisabledSubtitleFetchers", "SubtitleFetcherOrder", "SubtitleFetchers"},
		{lib.LyricFetchers, "DisabledLyricFetchers", "LyricFetcherOrder", "LyricFetchers"},
		{lib.MediaSegmentProviders, "DisabledMediaSegmentProviders", "MediaSegmentProviderOrder", "MediaSegmentProviders"},
		{lib.LocalMetadataReaders, "DisabledLocalMetadataReaders", "LocalMetadataReaderOrder", "MetadataReaders"},
	} {
		if len(f.declared) == 0 {
			continue
		}
		known := optionNames(available[f.available], false)
		order := stringList(current[f.order])
		out[f.order] = ranked(f.declared, order, known)
		disabled := slices.DeleteFunc(ranked(nil, known, order, stringList(current[f.disabled])), func(name string) bool {
			return slices.Contains(f.declared, name)
		})
		out[f.disabled] = sameSetOr(stringList(current[f.disabled]), disabled)
	}
	if len(lib.TypeOptions) > 0 {
		out["TypeOptions"] = jellyfinTypeOptions(lib.TypeOptions, current["TypeOptions"], available["TypeOptions"])
	}
	return out
}

// jellyfinTypeOptions is the library's type options with the declared ones
// applied. A type the library has no options for starts from the fetchers
// Jellyfin enables without them, so a list left out changes nothing.
func jellyfinTypeOptions(declared []mediaserversv1alpha1.JellyfinTypeOptions, current, available any) []any {
	var entries []any
	for _, e := range listOf(current) {
		entries = append(entries, maps.Clone(e))
	}
	offered := listOf(available)
	for _, t := range declared {
		var avail map[string]any
		if i := slices.IndexFunc(offered, func(o map[string]any) bool { return strings.EqualFold(fmt.Sprint(o["Type"]), t.Type) }); i >= 0 {
			avail = offered[i]
		}
		var entry map[string]any
		if i := slices.IndexFunc(entries, func(e any) bool {
			m, _ := e.(map[string]any)
			return strings.EqualFold(fmt.Sprint(m["Type"]), t.Type)
		}); i >= 0 {
			entry, _ = entries[i].(map[string]any)
		} else {
			entry = map[string]any{"Type": t.Type}
			for _, kind := range []string{"Metadata", "Image"} {
				entry[kind+"Fetchers"] = optionNames(avail[kind+"Fetchers"], true)
				entry[kind+"FetcherOrder"] = optionNames(avail[kind+"Fetchers"], false)
			}
			entries = append(entries, entry)
		}
		for kind, fetchers := range map[string][]string{"Metadata": t.MetadataFetchers, "Image": t.ImageFetchers} {
			if len(fetchers) == 0 {
				continue
			}
			entry[kind+"Fetchers"] = sameSetOr(stringList(entry[kind+"Fetchers"]), fetchers)
			entry[kind+"FetcherOrder"] = ranked(fetchers, stringList(entry[kind+"FetcherOrder"]), optionNames(avail[kind+"Fetchers"], false))
		}
	}
	return entries
}

// ranked is first, then the names of rest not already in it, in order.
func ranked(first []string, rest ...[]string) []string {
	out := append([]string{}, first...)
	for _, names := range rest {
		for _, name := range names {
			if !slices.Contains(out, name) {
				out = append(out, name)
			}
		}
	}
	return out
}

// sameSetOr keeps current when it holds the same names as want, so a list
// whose order means nothing isn't rewritten for its order.
func sameSetOr(current, want []string) []string {
	if len(current) == len(want) && !slices.ContainsFunc(want, func(name string) bool { return !slices.Contains(current, name) }) {
		return current
	}
	return want
}

// optionNames is the names of the options Jellyfin offers, or with enabled
// set, those it enables by default.
func optionNames(v any, enabled bool) []string {
	names := []string{}
	for _, o := range listOf(v) {
		if on, _ := o["DefaultEnabled"].(bool); enabled && !on {
			continue
		}
		if name, ok := o["Name"].(string); ok {
			names = append(names, name)
		}
	}
	return names
}

func stringList(v any) []string {
	raw, _ := v.([]any)
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func listOf(v any) []map[string]any {
	raw, _ := v.([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// jellyfinCollectionType is a library's type; Jellyfin leaves a mixed
// library's out.
func jellyfinCollectionType(library map[string]any) string {
	if t, _ := library["CollectionType"].(string); t != "" {
		return t
	}
	return "mixed"
}

func jellyfinLocations(library map[string]any) []string {
	raw, _ := library["Locations"].([]any)
	paths := make([]string, 0, len(raw))
	for _, p := range raw {
		if s, ok := p.(string); ok {
			paths = append(paths, s)
		}
	}
	return cleanPaths(paths)
}

// cleanPaths drops trailing separators, which a media server doesn't keep.
func cleanPaths(paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if trimmed := strings.TrimRight(p, "/"); trimmed != "" {
			p = trimmed
		}
		out = append(out, p)
	}
	return out
}

func buildPathInfos(paths []string) []map[string]any {
	infos := make([]map[string]any, len(paths))
	for i, p := range paths {
		infos[i] = map[string]any{"Path": p}
	}
	return infos
}

// SetupWithManager sets up the controller with the Manager.
func (r *JellyfinReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&mediaserversv1alpha1.Jellyfin{}, builder.WithPredicates(ctrlcommon.ConfigChangedPredicate())).
		WatchesMetadata(&corev1.Secret{}, ctrlcommon.EnqueueForSecret(r.Client, &mediaserversv1alpha1.JellyfinList{})).
		Named("jellyfin").
		Complete(r)
}
