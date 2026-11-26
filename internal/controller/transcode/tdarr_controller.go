package transcode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	transcodev1alpha1 "github.com/kyleseneker/arrchitect/api/transcode/v1alpha1"
	tdarrclient "github.com/kyleseneker/arrchitect/internal/client/tdarr"
	ctrlcommon "github.com/kyleseneker/arrchitect/internal/controller/common"
	"github.com/kyleseneker/arrchitect/internal/engine"
	"github.com/kyleseneker/arrchitect/internal/reconciler"
)

// TdarrReconciler reconciles a Tdarr object.
type TdarrReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder events.EventRecorder
}

// +kubebuilder:rbac:groups=arrchitect.dev,resources=tdarrs,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=arrchitect.dev,resources=tdarrs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=arrchitect.dev,resources=tdarrs/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

func (r *TdarrReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	integration := ctrlcommon.Integration[*transcodev1alpha1.Tdarr, *tdarrclient.Client]{
		App:     "tdarr",
		Connect: r.connect,
		Check: func(ctx context.Context, tc *tdarrclient.Client, _ *transcodev1alpha1.Tdarr) error {
			return tc.Ping(ctx)
		},
		Sync:    r.sync,
		Cleanup: cleanupTdarr,
		Prune:   true,
	}
	return integration.Reconcile(ctx, r.Client, r.Recorder, req, &transcodev1alpha1.Tdarr{})
}

func (r *TdarrReconciler) connect(ctx context.Context, config *transcodev1alpha1.Tdarr) (*tdarrclient.Client, error) {
	apiKey, err := reconciler.ResolveSecretKeyRef(ctx, r.Client, config.Namespace, config.Spec.Connection.APIKeySecretRef)
	if err != nil {
		return nil, err
	}
	tlsCfg, err := engine.ResolveTLSConfig(ctx, r.Client, config.Namespace, config.Spec.Connection.TLS)
	if err != nil {
		return nil, err
	}
	hc, err := engine.NewHTTPClient(config.Spec.Connection.URL, engine.AuthAPIKey, engine.WithAPIKey(apiKey),
		engine.WithAPIKeyHeader("x-api-key"), engine.WithTLSConfig(tlsCfg), engine.WithAppLabel("tdarr"),
		engine.WithTimeout(3*time.Minute))
	if err != nil {
		return nil, err
	}
	return tdarrclient.NewClient(hc), nil
}

func (r *TdarrReconciler) sync(ctx context.Context, tc *tdarrclient.Client, config *transcodev1alpha1.Tdarr, pass *engine.Pass) []string {
	s := &tdarrSync{Pass: pass, tc: tc}
	spec := config.Spec
	var unresolved []string
	if settings := spec.Settings; settings != nil && settings.NotificationsDiscordWebhookSecretRef != nil {
		webhook, err := reconciler.ResolveSecretKeyRef(ctx, r.Client, config.Namespace, *settings.NotificationsDiscordWebhookSecretRef)
		if err != nil {
			// The settings are left out of the pass; the rest still syncs.
			unresolved = append(unresolved, fmt.Sprintf("settings: %v", err))
			spec.Settings = nil
		} else {
			s.discordWebhook = &webhook
		}
	}
	return append(unresolved, s.run(ctx, &spec)...)
}

// tdarrSync applies one Tdarr. With observe set it only reads, and
// records what it would have written.
type tdarrSync struct {
	*engine.Pass
	tc             *tdarrclient.Client
	discordWebhook *string
}

// variablesResource is the status.managedResources key for library variables,
// each recorded as <library id>/<key>.
const variablesResource = "libraryVariables"

func (s *tdarrSync) run(ctx context.Context, spec *transcodev1alpha1.TdarrSpec) []string {
	logger := log.FromContext(ctx)
	var syncErrors []string
	for _, lib := range spec.Libraries {
		if err := s.library(ctx, lib); err != nil {
			logger.Error(err, "failed to reconcile library", "id", lib.ID)
			syncErrors = append(syncErrors, fmt.Sprintf("library(%s): %v", lib.ID, err))
		}
	}
	for _, flow := range spec.Flows {
		if err := s.flow(ctx, flow); err != nil {
			logger.Error(err, "failed to reconcile flow", "id", flow.ID)
			syncErrors = append(syncErrors, fmt.Sprintf("flow(%s): %v", flow.ID, err))
		}
	}
	if len(spec.Workers) > 0 {
		if err := s.workers(ctx, spec.Workers); err != nil {
			logger.Error(err, "failed to reconcile workers")
			syncErrors = append(syncErrors, fmt.Sprintf("workers: %v", err))
		}
	}
	if spec.Settings != nil {
		if err := s.settings(ctx, spec.Settings); err != nil {
			logger.Error(err, "failed to reconcile settings")
			syncErrors = append(syncErrors, fmt.Sprintf("settings: %v", err))
		}
	}
	return syncErrors
}

// tdarrGlobalSettings is the document holding Tdarr's server-wide options.
const tdarrGlobalSettings = "globalsettings"

// settings writes the declared options to Tdarr's global settings document,
// whose keys the spec's field names match.
func (s *tdarrSync) settings(ctx context.Context, settings *transcodev1alpha1.TdarrSettings) error {
	raw, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return err
	}
	delete(obj, "notificationsDiscordWebhookSecretRef")
	if s.discordWebhook != nil {
		obj["notificationsDiscordWebhook"] = *s.discordWebhook
	}
	if len(obj) == 0 {
		return nil
	}
	return s.upsertDoc(ctx, "SettingsGlobalJSONDB", tdarrGlobalSettings, obj, nil)
}

func (s *tdarrSync) library(ctx context.Context, lib transcodev1alpha1.TdarrLibrary) error {
	// A new library processes every hour; a schedule set in Tdarr afterwards is kept.
	onCreate := map[string]any{"schedule": alwaysOnSchedule()}
	// Tdarr's scanner splits foldersToIgnore, so a new library can't go without it.
	if lib.FoldersToIgnore == "" {
		onCreate["foldersToIgnore"] = ""
	}
	if err := s.upsertDoc(ctx, "LibrarySettingsJSONDB", lib.ID, buildTdarrLibraryDoc(lib), onCreate); err != nil {
		return err
	}
	return s.libraryVariables(ctx, lib)
}

// variablesCollection holds Tdarr's user variables. Library variables are
// documents here rather than fields on the library itself.
const variablesCollection = "VariablesJSONDB"

// libraryVariableType is the type tag Tdarr stores on a library-scoped variable.
// Flows read these as args.userVariables.library.<key>.
func libraryVariableType(libraryID string) string {
	return "library:" + libraryID
}

// libraryVariables converges the VariablesJSONDB documents for a library. Each
// variable is its own document rather than a field on the library, so they are
// matched by key within the library's type tag. A variable that leaves the spec
// is deleted only under prune, and only if the operator created it.
func (s *tdarrSync) libraryVariables(ctx context.Context, lib transcodev1alpha1.TdarrLibrary) error {
	varType := libraryVariableType(lib.ID)

	docs, err := s.tc.GetAll(ctx, variablesCollection)
	if err != nil {
		return fmt.Errorf("listing variables: %w", err)
	}

	existing := make(map[string]map[string]any, len(docs))
	for _, doc := range docs {
		if t, _ := doc["type"].(string); t != varType {
			continue
		}
		if key, _ := doc["key"].(string); key != "" {
			existing[key] = doc
		}
	}

	keys := make([]string, 0, len(lib.Variables))
	for k := range lib.Variables {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		value := lib.Variables[key]
		doc, found := existing[key]
		if !found {
			obj := map[string]any{
				"key":   key,
				"value": value,
				"type":  varType,
				"date":  time.Now().UnixMilli(),
			}
			err := s.Apply(variablesCollection, lib.ID+"/"+key, func() error {
				if err := s.tc.Insert(ctx, variablesCollection, "", obj); err != nil {
					return err
				}
				s.Owned.Created(variablesResource, lib.ID+"/"+key)
				return nil
			})
			if err != nil {
				return fmt.Errorf("inserting variable %s: %w", key, err)
			}
			continue
		}
		if current, _ := doc["value"].(string); current == value {
			continue
		}
		id, _ := doc["_id"].(string)
		obj := map[string]any{
			"_id":   id,
			"key":   key,
			"value": value,
			"type":  varType,
			"date":  time.Now().UnixMilli(),
		}
		err := s.Apply(variablesCollection, lib.ID+"/"+key, func() error {
			return s.tc.Update(ctx, variablesCollection, id, obj)
		})
		if err != nil {
			return fmt.Errorf("updating variable %s: %w", key, err)
		}
	}

	return s.pruneLibraryVariables(ctx, lib, existing)
}

// pruneLibraryVariables deletes the variables the operator created that the
// library no longer declares.
func (s *tdarrSync) pruneLibraryVariables(ctx context.Context, lib transcodev1alpha1.TdarrLibrary, existing map[string]map[string]any) error {
	if !s.PruneEnabled {
		return nil
	}
	ids := map[string]string{}
	var present, declared []string
	for key, doc := range existing {
		if id, _ := doc["_id"].(string); id != "" {
			ids[lib.ID+"/"+key] = id
			present = append(present, lib.ID+"/"+key)
		}
	}
	for key := range lib.Variables {
		declared = append(declared, lib.ID+"/"+key)
	}
	return s.Prune(s.Owned, variablesResource, declared, present, func(name string) error {
		return s.tc.Remove(ctx, variablesCollection, ids[name])
	})
}

// cleanupTdarr deletes the library variables the operator created, for
// deletionPolicy delete. Libraries are kept.
func cleanupTdarr(ctx context.Context, tc *tdarrclient.Client, owned *engine.Ownership) error {
	docs, err := tc.GetAll(ctx, variablesCollection)
	if err != nil {
		return err
	}
	ids := map[string]string{}
	present := make([]string, 0, len(docs))
	for _, doc := range docs {
		varType, _ := doc["type"].(string)
		key, _ := doc["key"].(string)
		id, _ := doc["_id"].(string)
		libraryID, ok := strings.CutPrefix(varType, libraryVariableType(""))
		if !ok || key == "" || id == "" {
			continue
		}
		ids[libraryID+"/"+key] = id
		present = append(present, libraryID+"/"+key)
	}
	return owned.Cleanup(variablesResource, present, func(name string) error {
		return tc.Remove(ctx, variablesCollection, ids[name])
	})
}

// alwaysOnSchedule renders a schedule with every hour of the week enabled,
// each hour named as Tdarr names it, from "Sun:00-01" to "Sat:23-00".
func alwaysOnSchedule() []any {
	schedule := make([]any, 0, 7*24)
	for _, day := range []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"} {
		for hour := range 24 {
			id := fmt.Sprintf("%s:%02d-%02d", day, hour, (hour+1)%24)
			schedule = append(schedule, map[string]any{"_id": id, "checked": true})
		}
	}
	return schedule
}

// tdarrPluginIDs renders a classic plugin stack as Tdarr's pluginIDs, community
// plugins in the declared order.
func tdarrPluginIDs(plugins []string) []any {
	out := make([]any, 0, len(plugins))
	for i, id := range plugins {
		out = append(out, map[string]any{
			"_id": fmt.Sprintf("plugin%d", i+1), "id": id, "checked": true, "source": "Community", "priority": i, "InputsDB": map[string]any{},
		})
	}
	return out
}

// buildTdarrLibraryDoc renders the library document Tdarr stores.
func buildTdarrLibraryDoc(lib transcodev1alpha1.TdarrLibrary) map[string]any {
	obj := map[string]any{
		"_id":  lib.ID,
		"name": lib.Name,
	}
	if lib.FoldersToIgnore != "" {
		obj["foldersToIgnore"] = lib.FoldersToIgnore
	}
	if lib.Folder != "" {
		obj["folder"] = lib.Folder
	}
	if lib.Cache != "" {
		obj["cache"] = lib.Cache
	}
	if lib.Container != "" {
		obj["container"] = lib.Container
	}
	if lib.ContainerFilter != "" {
		obj["containerFilter"] = lib.ContainerFilter
	}
	if lib.FolderWatching != nil {
		obj["folderWatching"] = *lib.FolderWatching
	}
	for key, value := range map[string]string{
		"filterResolutionsSkip": lib.FilterResolutionsSkip, "filterCodecsSkip": lib.FilterCodecsSkip,
		"filterContainersSkip": lib.FilterContainersSkip,
	} {
		if value != "" {
			obj[key] = value
		}
	}
	for key, value := range map[string]*bool{
		"foldersToIgnoreCaseInsensitive": lib.FoldersToIgnoreCaseInsensitive, "filterHardlinked": lib.FilterHardlinked,
		"useFsEvents": lib.UseFsEvents,
	} {
		if value != nil {
			obj[key] = *value
		}
	}
	if lib.FolderWatchScanInterval != 0 {
		obj["folderWatchScanInterval"] = lib.FolderWatchScanInterval
	}
	if lib.ProcessLibrary != nil {
		obj["processLibrary"] = *lib.ProcessLibrary
	}
	if lib.ProcessTranscodes != nil {
		obj["processTranscodes"] = *lib.ProcessTranscodes
	}
	if lib.ProcessHealthChecks != nil {
		obj["processHealthChecks"] = *lib.ProcessHealthChecks
	}
	if lib.HealthCheckMode != "" {
		obj["handbrakescan"] = lib.HealthCheckMode == "quick"
		obj["ffmpegscan"] = lib.HealthCheckMode == "thorough"
	}
	if lib.DecisionMode != "" {
		obj["decisionMaker"] = map[string]any{
			"settingsFlows":      lib.DecisionMode == "flows",
			"settingsPlugin":     lib.DecisionMode == "plugins",
			"settingsVideo":      false,
			"settingsAudio":      false,
			"videoExcludeSwitch": false,
			"audioExcludeSwitch": false,
		}
	}
	if lib.ScanOnStart != nil {
		obj["scanOnStart"] = *lib.ScanOnStart
	}
	if lib.ScheduledScanFindNew != nil {
		obj["scheduledScanFindNew"] = *lib.ScheduledScanFindNew
	}
	if lib.ScannerThreadCount != nil {
		obj["scannerThreadCount"] = *lib.ScannerThreadCount
	}
	if lib.FFmpeg != nil {
		obj["ffmpeg"] = *lib.FFmpeg
	}
	if lib.Priority != nil {
		obj["priority"] = *lib.Priority
	}
	if lib.FlowID != "" {
		obj["flowId"] = lib.FlowID
	}
	if len(lib.PluginStack) > 0 {
		obj["pluginIDs"] = tdarrPluginIDs(lib.PluginStack)
	}
	if len(lib.Variables) > 0 {
		user := make(map[string]any, len(lib.Variables))
		for k, v := range lib.Variables {
			user[k] = v
		}
		obj["variables"] = map[string]any{"user": user}
	}

	return obj
}

func (s *tdarrSync) flow(ctx context.Context, flow transcodev1alpha1.TdarrFlow) error {
	obj := map[string]any{
		"_id":  flow.ID,
		"name": flow.Name,
	}
	if flow.Description != "" {
		obj["description"] = flow.Description
	}

	plugins, err := decodeRawList(flow.FlowPlugins)
	if err != nil {
		return fmt.Errorf("unmarshaling flowPlugins: %w", err)
	}
	obj["flowPlugins"] = plugins

	edges, err := decodeRawList(flow.FlowEdges)
	if err != nil {
		return fmt.Errorf("unmarshaling flowEdges: %w", err)
	}
	obj["flowEdges"] = edges

	return s.upsertDoc(ctx, "FlowsJSONDB", flow.ID, obj, nil)
}

// upsertDoc writes a library or flow only when Tdarr's stored document does not
// already hold every declared value. Updating on every pass rewrote each library
// and flow every few minutes, and Tdarr rescans on a library save.
func (s *tdarrSync) upsertDoc(ctx context.Context, collection, id string, obj, onCreate map[string]any) error {
	existing, err := s.tc.GetByID(ctx, collection, id)
	if err != nil {
		return fmt.Errorf("looking up %s/%s: %w", collection, id, err)
	}
	if len(existing) == 0 {
		doc := maps.Clone(obj)
		maps.Copy(doc, onCreate)
		return s.Apply(collection, id, func() error { return s.tc.Insert(ctx, collection, id, doc) })
	}
	// Compare in JSON terms: the stored document decodes to float64s and []any.
	raw, err := json.Marshal(obj)
	if err != nil {
		return err
	}
	var desired any
	if err := json.Unmarshal(raw, &desired); err != nil {
		return err
	}
	if reconciler.Covers(existing, desired) {
		return nil
	}
	return s.Apply(collection, id, func() error { return s.tc.Update(ctx, collection, id, obj) })
}

// decodeRawList turns a list of raw JSON nodes into plain values. Tdarr stores
// flowPlugins and flowEdges as arrays, so each element decodes independently.
func decodeRawList(items []runtime.RawExtension) ([]any, error) {
	out := make([]any, 0, len(items))
	for i, it := range items {
		var v any
		if err := json.Unmarshal(it.Raw, &v); err != nil {
			return nil, fmt.Errorf("element %d: %w", i, err)
		}
		out = append(out, v)
	}
	return out, nil
}

func (s *tdarrSync) workers(ctx context.Context, workers []transcodev1alpha1.TdarrWorkers) error {
	nodes, err := s.tc.GetNodes(ctx)
	if err != nil {
		return fmt.Errorf("getting nodes: %w", err)
	}
	var errs []error
	for _, w := range workers {
		if err := s.nodeWorkers(ctx, nodes, w); err != nil {
			errs = append(errs, fmt.Errorf("node %s: %w", w.NodeName, err))
		}
	}
	return errors.Join(errs...)
}

func (s *tdarrSync) nodeWorkers(ctx context.Context, nodes map[string]any, workers transcodev1alpha1.TdarrWorkers) error {
	nodeID, err := workerNode(nodes, workers.NodeName)
	if err != nil {
		return err
	}

	workerLimits := []struct {
		workerType string
		limit      *int32
	}{
		{"transcodegpu", workers.TranscodeGPU},
		{"transcodecpu", workers.TranscodeCPU},
		{"healthcheckgpu", workers.HealthcheckGPU},
		{"healthcheckcpu", workers.HealthcheckCPU},
	}

	current := currentWorkerLimits(nodes[nodeID])
	for _, wl := range workerLimits {
		if wl.limit == nil {
			continue
		}
		if err := s.convergeWorkerLimit(ctx, nodeID, wl.workerType, current[wl.workerType], int(*wl.limit)); err != nil {
			return fmt.Errorf("setting %s limit: %w", wl.workerType, err)
		}
	}

	return nil
}

// currentWorkerLimits reads a node's per-type worker counts. Tdarr keys them in
// lower case.
func currentWorkerLimits(node any) map[string]int {
	out := map[string]int{}
	n, ok := node.(map[string]any)
	if !ok {
		return out
	}
	limits, ok := n["workerLimits"].(map[string]any)
	if !ok {
		return out
	}
	for k, v := range limits {
		if f, ok := v.(float64); ok {
			out[strings.ToLower(k)] = int(f)
		}
	}
	return out
}

// workerNode finds the node with the given name.
func workerNode(nodes map[string]any, name string) (string, error) {
	names := make([]string, 0, len(nodes))
	var matches []string
	for id, node := range nodes {
		n, _ := node.(map[string]any)
		nodeName, _ := n["nodeName"].(string)
		names = append(names, nodeName)
		if nodeName == name {
			matches = append(matches, id)
		}
	}
	slices.Sort(names)
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return "", fmt.Errorf("Tdarr has no node named %q (nodes: %s)", name, strings.Join(names, ", "))
	default:
		return "", fmt.Errorf("Tdarr has more than one node named %q", name)
	}
}

// convergeWorkerLimit steps a worker count toward target. Tdarr's endpoint only
// increases or decreases by one, so reaching a target takes repeated calls.
func (s *tdarrSync) convergeWorkerLimit(ctx context.Context, nodeID, workerType string, current, target int) error {
	if target < 0 {
		return fmt.Errorf("negative worker count %d", target)
	}
	process := "increase"
	steps := target - current
	if steps < 0 {
		process, steps = "decrease", -steps
	}
	if steps == 0 {
		return nil
	}
	return s.Apply("workers", workerType, func() error {
		for range steps {
			if err := s.tc.StepWorkerLimit(ctx, nodeID, workerType, process); err != nil {
				return err
			}
		}
		return nil
	})
}

// SetupWithManager sets up the controller with the Manager.
func (r *TdarrReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&transcodev1alpha1.Tdarr{}, builder.WithPredicates(ctrlcommon.ConfigChangedPredicate())).
		WatchesMetadata(&corev1.Secret{}, ctrlcommon.EnqueueForSecret(r.Client, &transcodev1alpha1.TdarrList{})).
		Named("tdarr").
		Complete(r)
}
