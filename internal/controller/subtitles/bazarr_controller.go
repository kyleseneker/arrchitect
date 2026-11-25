package subtitles

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"reflect"
	"slices"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	subtitlesv1alpha1 "github.com/kyleseneker/arrchitect/api/subtitles/v1alpha1"
	bazarrclient "github.com/kyleseneker/arrchitect/internal/client/bazarr"
	ctrlcommon "github.com/kyleseneker/arrchitect/internal/controller/common"
	"github.com/kyleseneker/arrchitect/internal/engine"
	"github.com/kyleseneker/arrchitect/internal/reconciler"
)

// BazarrReconciler reconciles a Bazarr object.
type BazarrReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder events.EventRecorder
	Apps     ctrlcommon.Apps
}

// +kubebuilder:rbac:groups=arrchitect.dev,resources=bazarrs,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=arrchitect.dev,resources=bazarrs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=arrchitect.dev,resources=bazarrs/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=arrchitect.dev,resources=sonarrs;radarrs;jellyfins,verbs=get;list;watch

func (r *BazarrReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	integration := ctrlcommon.Integration[*subtitlesv1alpha1.Bazarr, *bazarrclient.Client]{
		App:     "bazarr",
		Connect: r.connect,
		Sync:    r.sync,
		Cleanup: cleanupBazarr,
		Prune:   true,
	}
	return integration.Reconcile(ctx, r.Client, r.Recorder, req, &subtitlesv1alpha1.Bazarr{})
}

func (r *BazarrReconciler) connect(ctx context.Context, config *subtitlesv1alpha1.Bazarr) (*bazarrclient.Client, error) {
	apiKey, err := reconciler.ResolveSecretKeyRef(ctx, r.Client, config.Namespace, config.Spec.Connection.APIKeySecretRef)
	if err != nil {
		return nil, err
	}
	tlsCfg, err := engine.ResolveTLSConfig(ctx, r.Client, config.Namespace, config.Spec.Connection.TLS)
	if err != nil {
		return nil, err
	}
	hc, err := engine.NewHTTPClient(config.Spec.Connection.URL, engine.AuthFormEncoded, engine.WithAPIKey(apiKey),
		engine.WithAPIKeyHeader("X-API-KEY"), engine.WithTLSConfig(tlsCfg), engine.WithAppLabel("bazarr"))
	if err != nil {
		return nil, err
	}
	bc := bazarrclient.NewClient(hc)
	return bc, bc.Ping(ctx)
}

func (r *BazarrReconciler) sync(ctx context.Context, bc *bazarrclient.Client, config *subtitlesv1alpha1.Bazarr, pass *engine.Pass) []string {
	desired, problems := r.bazarrDesired(ctx, config)
	settings, err := bc.GetSettings(ctx)
	if err != nil {
		return append(problems, fmt.Sprintf("reading settings: %v", err))
	}
	var profiles *bazarrProfiles
	if desired.Has(profilesField) {
		existing, err := bc.GetLanguageProfiles(ctx)
		if err != nil {
			return append(problems, fmt.Sprintf("reading language profiles: %v", err))
		}
		profiles, err = planBazarrProfiles(desired.Get(profilesField), existing, pass.PruneEnabled, pass.Owned)
		if err != nil {
			problems = append(problems, fmt.Sprintf("languages: %v", err))
			desired.Del(profilesField)
		} else {
			desired.Set(profilesField, profiles.send)
		}
	}
	var enabled []string
	if desired.Has(bazarrclient.EnabledLanguagesField) {
		if enabled, err = bc.EnabledLanguages(ctx); err != nil {
			return append(problems, fmt.Sprintf("reading languages: %v", err))
		}
	}
	if err := resolveBazarrDefaultProfiles(ctx, bc, config.Spec, desired); err != nil {
		problems = append(problems, fmt.Sprintf("general: %v", err))
	}
	if err := resolveBazarrJellyfinLibraries(ctx, bc, desired); err != nil {
		problems = append(problems, fmt.Sprintf("jellyfin: %v", err))
	}
	changes := bazarrChanges(desired, settings, profiles.stored(), enabled)
	if items, err := r.bazarrNotificationChanges(ctx, config.Namespace, config.Spec.Notifications, settings); err != nil {
		problems = append(problems, fmt.Sprintf("notifications: %v", err))
	} else if len(items) > 0 {
		changes[notificationsField] = items
	}

	// One request with only what differs: every save reconnects Bazarr to
	// Sonarr and Radarr, and enabling useSonarr in a request before the one
	// carrying the connection hangs while Bazarr retries 127.0.0.1.
	err = pass.ApplyBatch(bazarrChangeList(changes), func() error {
		if err := bc.PostForm(ctx, "/api/system/settings", changes); err != nil {
			return err
		}
		if changes.Has(profilesField) {
			profiles.saved(pass.Owned)
		}
		return nil
	})
	if err != nil {
		log.FromContext(ctx).Error(err, "failed to write Bazarr settings")
		problems = append(problems, fmt.Sprintf("settings: %v", err))
	}
	return problems
}

// bazarrDesired renders every declared setting as the form fields Bazarr's
// settings endpoint takes. Sections whose Secrets cannot be read are left out
// and reported, so the rest still reconcile.
func (r *BazarrReconciler) bazarrDesired(ctx context.Context, config *subtitlesv1alpha1.Bazarr) (url.Values, []string) {
	spec := config.Spec
	form := url.Values{}
	var problems []string
	merge := func(v url.Values) { maps.Copy(form, v) }

	if spec.General != nil {
		merge(bazarrclient.StructToFormData("general", spec.General))
		merge(bazarrPathMappings(spec.General))
	}
	if spec.SubSync != nil {
		merge(bazarrclient.StructToFormData("subsync", spec.SubSync))
	}
	for _, c := range []struct {
		section string
		conn    subtitlesv1alpha1.BazarrAppConnection
	}{{"sonarr", spec.SonarrConnection}, {"radarr", spec.RadarrConnection}} {
		if reflect.ValueOf(c.conn).IsZero() {
			continue
		}
		conn, err := r.resolveConnection(ctx, config.Namespace, c.conn)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%sConnection: %v", c.section, err))
			continue
		}
		key, err := reconciler.ResolveSecretKeyRef(ctx, r.Client, config.Namespace, *conn.APIKeySecretRef)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%sConnection: resolving apiKeySecretRef: %v", c.section, err))
			continue
		}
		merge(bazarrclient.StructToFormData(c.section, conn))
		if conn.AppRef != nil {
			form.Set(fmt.Sprintf("settings-%s-%s", c.section, bazarrclient.SettingName(c.section, "basePath")), conn.BasePath)
		}
		form.Set(fmt.Sprintf("settings-%s-apikey", c.section), key)
	}
	if spec.Jellyfin != nil {
		jellyfin, err := r.bazarrJellyfinForm(ctx, config.Namespace, spec.Jellyfin)
		if err != nil {
			problems = append(problems, fmt.Sprintf("jellyfin: %v", err))
		} else {
			merge(jellyfin)
		}
	}
	if len(spec.Providers) > 0 {
		providers, err := r.bazarrProviderForm(ctx, config.Namespace, spec.Providers)
		if err != nil {
			problems = append(problems, fmt.Sprintf("providers: %v", err))
		} else {
			merge(providers)
		}
	}
	if spec.Languages != nil {
		languages, err := bazarrclient.LanguagesForm(spec.Languages)
		if err != nil {
			problems = append(problems, fmt.Sprintf("languages: %v", err))
		} else {
			merge(languages)
		}
	}
	return form, problems
}

// resolveConnection returns conn with its appRef, if any, replaced by the
// referenced resource's address and API key Secret. A reference sets the whole
// address, so its caller sends the base path even when it is the root's "",
// which is how Bazarr stores "/" once it has used it.
func (r *BazarrReconciler) resolveConnection(
	ctx context.Context, namespace string, conn subtitlesv1alpha1.BazarrAppConnection,
) (subtitlesv1alpha1.BazarrAppConnection, error) {
	if conn.AppRef != nil {
		app, addr, err := ctrlcommon.ResolveAppAddress(ctx, r.Client, r.Apps, namespace, *conn.AppRef)
		if err != nil {
			return conn, fmt.Errorf("appRef: %w", err)
		}
		conn.Host, conn.Port, conn.SSL, conn.BasePath, conn.APIKeySecretRef = addr.Host, &addr.Port, &addr.TLS, addr.Base, app.APIKeySecretRef
	}
	if conn.APIKeySecretRef == nil {
		return conn, fmt.Errorf("no API key Secret")
	}
	return conn, nil
}

func bazarrAppRefs(o client.Object) []commonv1alpha1.AppRef {
	spec := o.(*subtitlesv1alpha1.Bazarr).Spec
	var refs []commonv1alpha1.AppRef
	candidates := []*commonv1alpha1.AppRef{spec.SonarrConnection.AppRef, spec.RadarrConnection.AppRef}
	if spec.Jellyfin != nil {
		candidates = append(candidates, spec.Jellyfin.AppRef)
	}
	for _, ref := range candidates {
		if ref != nil {
			refs = append(refs, *ref)
		}
	}
	return refs
}

// bazarrJellyfinForm renders Bazarr's Jellyfin integration, turned on, with
// the declared library names; resolveBazarrJellyfinLibraries adds their ids.
func (r *BazarrReconciler) bazarrJellyfinForm(
	ctx context.Context, namespace string, jf *subtitlesv1alpha1.BazarrJellyfin,
) (url.Values, error) {
	if jf.AppRef != nil {
		app, err := ctrlcommon.ResolveAppRef(ctx, r.Client, r.Apps, namespace, *jf.AppRef)
		if err != nil {
			return nil, fmt.Errorf("appRef: %w", err)
		}
		if app.APIKeySecretRef == nil {
			return nil, ctrlcommon.InvalidAppRef("appRef: Jellyfin %q sets no API key Secret", jf.AppRef.Name)
		}
		resolved := *jf
		resolved.URL, resolved.APIKeySecretRef = app.URL, app.APIKeySecretRef
		jf = &resolved
	}
	if jf.APIKeySecretRef == nil {
		return nil, fmt.Errorf("no API key Secret")
	}
	key, err := reconciler.ResolveSecretKeyRef(ctx, r.Client, namespace, *jf.APIKeySecretRef)
	if err != nil {
		return nil, fmt.Errorf("resolving apiKeySecretRef: %w", err)
	}
	form := url.Values{
		"settings-general-use_jellyfin": {"true"},
		"settings-jellyfin-url":         {strings.TrimSuffix(jf.URL, "/")},
		"settings-jellyfin-apikey":      {key},
	}
	if jf.RefreshMethod != "" {
		form.Set("settings-jellyfin-refresh_method", jf.RefreshMethod)
	}
	for _, l := range []struct {
		kind  string
		names []string
	}{{"movie", jf.MovieLibraries}, {"series", jf.SeriesLibraries}} {
		if len(l.names) > 0 {
			form["settings-jellyfin-"+l.kind+"_library"] = slices.Clone(l.names)
			form.Set("settings-jellyfin-update_"+l.kind+"_library", "true")
		}
	}
	return form, nil
}

// bazarrJellyfinLibraryKinds pairs each kind of library Bazarr refreshes with
// the Jellyfin collection type it must be.
var bazarrJellyfinLibraryKinds = []struct{ kind, collection, label string }{
	{"movie", "movies", "movie"}, {"series", "tvshows", "TV"},
}

// resolveBazarrJellyfinLibraries adds the ids of the declared Jellyfin
// libraries, which Bazarr finds items by. It asks Bazarr, so the lookup takes
// the path Bazarr's own refreshes will. A kind whose libraries aren't all
// found keeps its stored settings.
func resolveBazarrJellyfinLibraries(ctx context.Context, bc *bazarrclient.Client, form url.Values) error {
	var libraries []bazarrclient.JellyfinLibrary
	listed := false
	var errs []error
	for _, k := range bazarrJellyfinLibraryKinds {
		key := "settings-jellyfin-" + k.kind + "_library"
		names := form[key]
		if len(names) == 0 {
			continue
		}
		if !listed {
			var err error
			libraries, err = bc.JellyfinLibraries(ctx, form.Get("settings-jellyfin-url"), form.Get("settings-jellyfin-apikey"))
			if err != nil {
				for _, other := range bazarrJellyfinLibraryKinds {
					dropBazarrJellyfinLibraries(form, other.kind)
				}
				return fmt.Errorf("listing Jellyfin's libraries, which needs Bazarr 1.6.0 or later: %w", err)
			}
			listed = true
		}
		ids, err := jellyfinLibraryIDs(libraries, names, k.collection, k.label)
		if err != nil {
			dropBazarrJellyfinLibraries(form, k.kind)
			errs = append(errs, fmt.Errorf("%sLibraries: %w", k.kind, err))
			continue
		}
		form[key+"_ids"] = ids
	}
	return errors.Join(errs...)
}

func dropBazarrJellyfinLibraries(form url.Values, kind string) {
	key := "settings-jellyfin-" + kind + "_library"
	form.Del(key)
	form.Del(key + "_ids")
	form.Del("settings-jellyfin-update_" + kind + "_library")
}

// jellyfinLibraryIDs looks each name up among the libraries of one collection
// type.
func jellyfinLibraryIDs(libraries []bazarrclient.JellyfinLibrary, names []string, collection, label string) ([]string, error) {
	if len(libraries) == 0 {
		return nil, errors.New("no Jellyfin libraries came back from Bazarr; check that it reaches Jellyfin with this URL and API key")
	}
	ids := make([]string, 0, len(names))
	for _, name := range names {
		i := slices.IndexFunc(libraries, func(l bazarrclient.JellyfinLibrary) bool {
			return l.Name == name && l.Type == collection
		})
		if i < 0 {
			var have []string
			for _, l := range libraries {
				if l.Type == collection {
					have = append(have, strconv.Quote(l.Name))
				}
			}
			return nil, fmt.Errorf("no Jellyfin %s library named %q; Bazarr finds [%s]", label, name, strings.Join(have, ", "))
		}
		ids = append(ids, libraries[i].ID)
	}
	return ids, nil
}

// bazarrPathMappings sends each mapping as Bazarr's form does, "from,to".
func bazarrPathMappings(g *subtitlesv1alpha1.BazarrGeneral) url.Values {
	form := url.Values{}
	for name, mappings := range map[string][]subtitlesv1alpha1.BazarrPathMapping{
		"pathMappings": g.PathMappings, "pathMappingsMovie": g.PathMappingsMovie,
	} {
		if mappings == nil {
			continue
		}
		values := []string{""}
		if len(mappings) > 0 {
			values = values[:0]
		}
		for _, m := range mappings {
			values = append(values, m.RemotePath+","+m.LocalPath)
		}
		form["settings-general-"+bazarrclient.SettingName("general", name)] = values
	}
	return form
}

// resolveBazarrDefaultProfiles turns the declared default profile names into
// the ids Bazarr stores, looking in the declared profiles before Bazarr's, and
// turns each default on. An unknown name drops the setting.
func resolveBazarrDefaultProfiles(ctx context.Context, bc *bazarrclient.Client, spec subtitlesv1alpha1.BazarrSpec, form url.Values) error {
	if spec.General == nil || (spec.General.SeriesDefaultProfile == "" && spec.General.MovieDefaultProfile == "") {
		return nil
	}
	declared := engine.NamedIDs{}
	if spec.Languages != nil {
		for _, p := range spec.Languages.Profiles {
			if p.ProfileID != nil {
				declared[p.Name] = append(declared[p.Name], int(*p.ProfileID))
			}
		}
	}
	var stored engine.NamedIDs
	var errs []error
	for _, d := range []struct{ field, name, enabled string }{
		{"seriesDefaultProfile", spec.General.SeriesDefaultProfile, "serie_default_enabled"},
		{"movieDefaultProfile", spec.General.MovieDefaultProfile, "movie_default_enabled"},
	} {
		if d.name == "" {
			continue
		}
		key := "settings-general-" + bazarrclient.SettingName("general", d.field)
		id, found, err := declared.Find(d.name)
		if err == nil && !found {
			if stored == nil {
				existing, err := bc.GetLanguageProfiles(ctx)
				if err != nil {
					form.Del(key)
					errs = append(errs, fmt.Errorf("%s: reading language profiles: %w", d.field, err))
					continue
				}
				stored = engine.NamedIDs{}
				for _, p := range existing {
					if profileID, ok := p["profileId"].(float64); ok {
						name, _ := p["name"].(string)
						stored[name] = append(stored[name], int(profileID))
					}
				}
			}
			id, found, err = stored.Find(d.name)
		}
		switch {
		case err != nil:
			form.Del(key)
			errs = append(errs, fmt.Errorf("%s: Bazarr has more than one language profile named %q", d.field, d.name))
			continue
		case !found:
			form.Del(key)
			errs = append(errs, fmt.Errorf("%s: Bazarr has no language profile named %q", d.field, d.name))
			continue
		}
		form.Set(key, strconv.Itoa(id))
		form.Set("settings-general-"+d.enabled, "true")
	}
	return errors.Join(errs...)
}

func (r *BazarrReconciler) bazarrProviderForm(ctx context.Context, namespace string, providers []subtitlesv1alpha1.BazarrProvider) (url.Values, error) {
	form := url.Values{}
	for _, p := range providers {
		if p.Enabled == nil || *p.Enabled {
			form.Add("settings-general-enabled_providers", p.Name)
		}
		values, err := ctrlcommon.FieldValues(ctx, r.Client, namespace, "provider "+p.Name, p.Settings)
		if err != nil {
			return nil, err
		}
		for k, v := range values {
			text, ok := v.(string)
			if !ok {
				text = fmt.Sprint(v)
			}
			form.Set(fmt.Sprintf("settings-%s-%s", p.Name, k), text)
		}
	}
	return form, nil
}

// bazarrChanges returns the desired form fields whose value differs from what
// Bazarr reports, so a reconcile writes nothing when nothing changed.
// notificationsField carries each notification provider to change, as JSON.
const notificationsField = "notifications-providers"

// bazarrNotificationChanges is each declared notification provider whose
// enabled flag or URL differs, as the JSON Bazarr's settings form takes. Bazarr
// sets both from the item, so an unset one keeps its stored value.
func (r *BazarrReconciler) bazarrNotificationChanges(
	ctx context.Context, namespace string, declared []subtitlesv1alpha1.BazarrNotification, settings map[string]any,
) ([]string, error) {
	notifications, _ := settings["notifications"].(map[string]any)
	providers, _ := notifications["providers"].([]any)
	current := map[string]map[string]any{}
	for _, p := range providers {
		if provider, ok := p.(map[string]any); ok {
			name, _ := provider["name"].(string)
			current[name] = provider
		}
	}
	var items []string
	for _, n := range declared {
		have, ok := current[n.Name]
		if !ok {
			return nil, fmt.Errorf("Bazarr has no notification provider named %q", n.Name)
		}
		want := map[string]any{"name": n.Name, "enabled": have["enabled"], "url": have["url"]}
		if n.Enabled != nil {
			want["enabled"] = *n.Enabled
		}
		if n.URLSecretRef != nil {
			u, err := reconciler.ResolveSecretKeyRef(ctx, r.Client, namespace, *n.URLSecretRef)
			if err != nil {
				return nil, fmt.Errorf("%s url: %w", n.Name, err)
			}
			want["url"] = u
		}
		if want["enabled"] == have["enabled"] && want["url"] == have["url"] {
			continue
		}
		raw, err := json.Marshal(want)
		if err != nil {
			return nil, err
		}
		items = append(items, string(raw))
	}
	return items, nil
}

func bazarrChanges(desired url.Values, settings map[string]any, profiles []map[string]any, enabled []string) url.Values {
	changes := url.Values{}
	for key, values := range desired {
		switch key {
		case profilesField:
			if !sameBazarrProfiles(profiles, values[0]) {
				changes[key] = values
			}
			continue
		case bazarrclient.EnabledLanguagesField:
			if !slices.Equal(slices.Sorted(slices.Values(values)), slices.Sorted(slices.Values(enabled))) {
				changes[key] = values
			}
			continue
		}
		section, name, _ := strings.Cut(strings.TrimPrefix(key, "settings-"), "-")
		current, _ := settings[section].(map[string]any)
		if !bazarrSettingMatches(current[name], values) {
			changes[key] = values
		}
	}
	return changes
}

// bazarrChangeList names each change by section and setting.
func bazarrChangeList(changes url.Values) []engine.Change {
	out := make([]engine.Change, 0, len(changes))
	for key := range changes {
		_, name, _ := strings.Cut(strings.TrimPrefix(key, "settings-"), "-")
		if key == profilesField {
			name = "profiles"
		}
		out = append(out, engine.Change{Kind: bazarrSettingSection(key), Name: name})
	}
	slices.SortFunc(out, func(a, b engine.Change) int {
		return strings.Compare(a.Kind+"("+a.Name+")", b.Kind+"("+b.Name+")")
	})
	return out
}

func bazarrSettingSection(key string) string {
	switch key {
	case profilesField:
		return "languages"
	case notificationsField:
		return "notifications"
	}
	section, _, _ := strings.Cut(strings.TrimPrefix(key, "settings-"), "-")
	return section
}

// bazarrSettingMatches compares a stored setting with its form value(s). Lists
// (enabled providers and languages) compare as sets.
func bazarrSettingMatches(current any, values []string) bool {
	if list, ok := current.([]any); ok {
		want := values
		if len(values) == 1 && strings.HasPrefix(values[0], "[") {
			if err := json.Unmarshal([]byte(values[0]), &want); err != nil {
				return false
			}
		}
		want = slices.DeleteFunc(slices.Clone(want), func(v string) bool { return v == "" })
		have := make([]string, 0, len(list))
		for _, v := range list {
			if pair, ok := v.([]any); ok {
				parts := make([]string, 0, len(pair))
				for _, part := range pair {
					parts = append(parts, fmt.Sprint(part))
				}
				have = append(have, strings.Join(parts, ","))
				continue
			}
			have = append(have, fmt.Sprint(v))
		}
		return slices.Equal(slices.Sorted(slices.Values(have)), slices.Sorted(slices.Values(want)))
	}
	return len(values) == 1 && bazarrScalarText(current) == values[0]
}

func bazarrScalarText(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	default:
		return fmt.Sprint(x)
	}
}

// sameBazarrProfiles compares only the keys the operator sends. Bazarr reports
// originalFormat as 0/1, which Covers allows.
func sameBazarrProfiles(current []map[string]any, desiredJSON string) bool {
	var want []any
	if err := json.Unmarshal([]byte(desiredJSON), &want); err != nil {
		return false
	}
	have := make([]any, len(current))
	for i, p := range current {
		have[i] = p
	}
	byID := func(list []any) {
		slices.SortStableFunc(list, func(a, b any) int {
			ai, _ := a.(map[string]any)["profileId"].(float64)
			bi, _ := b.(map[string]any)["profileId"].(float64)
			return int(ai - bi)
		})
	}
	if len(have) != len(want) {
		return false
	}
	byID(have)
	byID(want)
	return reconciler.Covers(any(have), any(want))
}

// SetupWithManager sets up the controller with the Manager.
func (r *BazarrReconciler) SetupWithManager(mgr ctrl.Manager) error {
	b := ctrl.NewControllerManagedBy(mgr).
		For(&subtitlesv1alpha1.Bazarr{}, builder.WithPredicates(ctrlcommon.ConfigChangedPredicate())).
		WatchesMetadata(&corev1.Secret{}, ctrlcommon.EnqueueForSecretThroughRefs(r.Client, r.Apps, &subtitlesv1alpha1.BazarrList{}, bazarrAppRefs))
	b, err := ctrlcommon.WatchAppRefs(mgr, b, r.Apps, &subtitlesv1alpha1.Bazarr{}, &subtitlesv1alpha1.BazarrList{}, bazarrAppRefs, "Sonarr", "Radarr", "Jellyfin")
	if err != nil {
		return err
	}
	return b.
		Named("bazarr").
		Complete(r)
}
