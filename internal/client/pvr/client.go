package pvr

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	pvrv1alpha1 "github.com/kyleseneker/arrchitect/api/pvr/v1alpha1"
	"github.com/kyleseneker/arrchitect/internal/engine"
)

// App holds what differs between Sonarr, Radarr, Lidarr and Readarr.
type App struct {
	Name             string
	Kind             string
	APIVersion       string
	CategoryField    string
	ImportListFields ImportListFields
	MetadataProfiles bool
	// ReleaseProfiles marks apps whose release profiles have names (Sonarr,
	// Radarr), so the operator can match them.
	ReleaseProfiles bool
	// Choices are settings, by section and field, that the resource names and
	// the app stores as numbers.
	Choices map[string]map[string]map[string]int
	// Sections are the app's settings sections beyond those all four share.
	Sections []string
}

// sectionPaths are the endpoints, under the API prefix, of the settings
// sections only some of the apps have.
var sectionPaths = map[string]string{
	"metadataProvider": "/config/metadataprovider",
	"development":      "/config/development",
	"metadataConfig":   "/config/metadata",
	"importListConfig": "/config/importlist",
}

var bookAndMusicImportListFields = ImportListFields{
	AutomaticAdd: "enableAutomaticAdd", Monitor: "shouldMonitor", Search: "shouldSearch",
	MonitorNewItems: "monitorNewItems", MonitorExisting: "shouldMonitorExisting",
}

var bookAndMusicChoices = map[string]map[string]map[string]int{"naming": {
	"colonReplacementFormat": {"delete": 0, "dash": 1, "spaceDash": 2, "spaceDashSpace": 3, "smart": 4},
}}

var (
	Sonarr = App{
		Name: "sonarr", Kind: "Sonarr", APIVersion: "v3", CategoryField: "tvCategory", ReleaseProfiles: true,
		ImportListFields: ImportListFields{
			AutomaticAdd: "enableAutomaticAdd", Monitor: "shouldMonitor", Search: "searchForMissingEpisodes",
			MonitorNewItems: "monitorNewItems",
		},
		Sections: []string{"importListConfig"},
		Choices: map[string]map[string]map[string]int{"naming": {
			"colonReplacementFormat": {"delete": 0, "dash": 1, "spaceDash": 2, "spaceDashSpace": 3, "smart": 4, "custom": 5},
			"multiEpisodeStyle":      {"extend": 0, "duplicate": 1, "repeat": 2, "scene": 3, "range": 4, "prefixedRange": 5},
		}},
	}
	Radarr = App{
		Name: "radarr", Kind: "Radarr", APIVersion: "v3", CategoryField: "movieCategory", ReleaseProfiles: true,
		ImportListFields: ImportListFields{Enable: "enabled", AutomaticAdd: "enableAuto", Monitor: "monitor", Search: "searchOnAdd"},
		Sections:         []string{"metadataConfig", "importListConfig"},
	}
	Lidarr = App{
		Name: "lidarr", Kind: "Lidarr", APIVersion: "v1", CategoryField: "musicCategory", MetadataProfiles: true,
		ImportListFields: bookAndMusicImportListFields, Choices: bookAndMusicChoices, Sections: []string{"metadataProvider"},
	}
	Readarr = App{
		Name: "readarr", Kind: "Readarr", APIVersion: "v1", CategoryField: "bookCategory", MetadataProfiles: true,
		ImportListFields: bookAndMusicImportListFields, Choices: bookAndMusicChoices,
		Sections: []string{"metadataProvider", "development"},
	}
)

// Definition returns the AppDefinition for Sonarr, Radarr, Lidarr or Readarr.
func Definition(app App) engine.AppDefinition {
	prefix := "/api/" + app.APIVersion
	settings := make([]engine.SettingEndpoint, 0, 5+len(app.Sections))
	settings = append(settings,
		engine.SettingEndpoint{Name: "mediaManagement", Path: prefix + "/config/mediamanagement"},
		engine.SettingEndpoint{Name: "naming", Path: prefix + "/config/naming"},
		engine.SettingEndpoint{Name: "indexerConfig", Path: prefix + "/config/indexer"},
		engine.SettingEndpoint{Name: "downloadClientConfig", Path: prefix + "/config/downloadclient"},
		engine.SettingEndpoint{Name: "ui", Path: prefix + "/config/ui"},
	)
	for _, name := range app.Sections {
		settings = append(settings, engine.SettingEndpoint{Name: name, Path: prefix + sectionPaths[name]})
	}
	// Only Lidarr's and Readarr's root folders have settings to update.
	rootFolders := engine.CreateOnly
	if app.MetadataProfiles {
		rootFolders = engine.CreateOrUpdate
	}
	importListDefaults := map[string]any{"fields": []any{}, "tags": []any{}}
	if app.ImportListFields.Enable != "" {
		importListDefaults[app.ImportListFields.Enable] = true
	}
	return engine.AppDefinition{
		HealthPath: prefix + "/system/status",
		Settings:   settings,
		Resources: []engine.ResourceEndpoint{
			// Tags first, since other resources reference tag IDs. Not prunable, since
			// they may be shared.
			{Name: "tags", Path: prefix + "/tag", MatchField: "label", Policy: engine.CreateOrUpdate},
			// Metadata profiles before root folders and import lists, which name them.
			{Name: "metadataProfiles", Path: prefix + "/metadataprofile", MatchField: "name", Policy: engine.CreateOrUpdate, Prunable: true},
			// Root folders not prunable (destructive to remove).
			{Name: "rootFolders", Path: prefix + "/rootfolder", MatchField: "path", Policy: rootFolders},
			// Delay profiles reconcile in their own stage; this entry drives cleanup.
			{Name: delayProfilesKind, Path: prefix + "/delayprofile", MatchKey: delayProfileID, Policy: engine.CreateOrUpdate, Prunable: true},
			{Name: "remotePathMappings", Path: prefix + "/remotepathmapping", MatchKey: remotePathMappingKey, Policy: engine.CreateOrUpdate, Prunable: true},
			{
				Name: "downloadClients", Path: prefix + "/downloadclient", MatchField: "name", Policy: engine.CreateOrUpdate, Prunable: true,
				CreateDefaults: map[string]any{
					"enable": true, "priority": 1, "removeCompletedDownloads": true, "removeFailedDownloads": true, "tags": []any{},
				},
			},
			{
				Name: "indexers", Path: prefix + "/indexer", MatchField: "name", Policy: engine.CreateOrUpdate, Prunable: true,
				CreateDefaults: map[string]any{
					"enableRss": true, "enableAutomaticSearch": true, "enableInteractiveSearch": true, "fields": []any{}, "tags": []any{},
				},
			},
			// customFormats before qualityProfiles so IDs can be resolved for formatItems.
			{
				Name: "customFormats", Path: prefix + "/customformat", MatchField: "name", Policy: engine.CreateOrUpdate, Prunable: true,
				CreateDefaults: map[string]any{"includeCustomFormatWhenRenaming": false, "specifications": []any{}},
				ExactLists:     []string{"specifications"},
			},
			{Name: "qualityDefinitions", Path: prefix + "/qualitydefinition", MatchKey: qualityName, Policy: engine.UpdateOnly},
			{Name: "qualityProfiles", Path: prefix + "/qualityprofile", MatchField: "name", Policy: engine.CreateOrUpdate, Prunable: true},
			{
				Name: "notifications", Path: prefix + "/notification", MatchField: "name", Policy: engine.CreateOrUpdate, Prunable: true,
				CreateDefaults: map[string]any{"fields": []any{}, "tags": []any{}},
			},
			{
				Name: "releaseProfiles", Path: prefix + "/releaseprofile", MatchField: "name", Policy: engine.CreateOrUpdate, Prunable: true,
				CreateDefaults: map[string]any{"enabled": true, "required": []any{}, "ignored": []any{}, "indexerId": 0, "tags": []any{}},
			},
			// Auto tags after the tags and quality profiles their conditions name.
			{
				Name: "autoTags", Path: prefix + "/autotagging", MatchField: "name", Policy: engine.CreateOrUpdate, Prunable: true,
				CreateDefaults: map[string]any{"removeTagsAutomatically": false, "specifications": []any{}},
				ExactLists:     []string{"specifications"},
			},
			// The app keeps one metadata consumer per type, created at startup.
			{Name: "metadataConsumers", Path: prefix + "/metadata", MatchField: "implementation", Policy: engine.UpdateOnly},
			// Import lists last, since they may reference quality profiles by name.
			{
				Name: "importLists", Path: prefix + "/importlist", MatchField: "name", Policy: engine.CreateOrUpdate, Prunable: true,
				CreateDefaults: importListDefaults,
			},
		},
	}
}

// Sections builds the sections map from a Sonarr, Radarr, Lidarr or Readarr spec,
// turning the names in choices into the numbers the app stores.
// The spec may have the fields MediaManagement, Naming, IndexerConfig,
// DownloadClientConfig and UI, and the app's own sections.
func Sections(spec any, choices map[string]map[string]map[string]int) (map[string]any, error) {
	data, err := json.Marshal(spec)
	if err != nil {
		return nil, fmt.Errorf("marshaling spec to JSON: %w", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("unmarshaling spec from JSON: %w", err)
	}

	sections := make(map[string]any)
	names := append([]string{"mediaManagement", "naming", "indexerConfig", "downloadClientConfig", "ui"},
		slices.Collect(maps.Keys(sectionPaths))...)
	for _, name := range names {
		if v, ok := m[name]; ok && v != nil {
			sections[name] = v
		}
	}
	for section, fields := range choices {
		values, _ := sections[section].(map[string]any)
		for field, numbers := range fields {
			name, ok := values[field].(string)
			if !ok {
				continue
			}
			number, ok := numbers[name]
			if !ok {
				return nil, fmt.Errorf("%s.%s: unknown value %q", section, field, name)
			}
			values[field] = number
		}
	}
	return sections, nil
}

// DownloadClientResolvedSecrets holds resolved secret values for a download client.
type DownloadClientResolvedSecrets struct {
	Username string
	Password string
	APIKey   string
}

// Options holds all the inputs needed to reconcile Sonarr, Radarr, Lidarr or Readarr.
type Options struct {
	App                App
	RemotePathMappings []commonv1alpha1.RemotePathMapping
	RootFolders        []commonv1alpha1.RootFolder
	DownloadClients    []commonv1alpha1.DownloadClient
	DCSecrets          map[string]DownloadClientResolvedSecrets
	QualityProfiles    []commonv1alpha1.QualityProfile
	CustomFormats      []commonv1alpha1.CustomFormat
	Tags               []commonv1alpha1.Tag
	Indexers           []commonv1alpha1.Indexer
	Notifications      []commonv1alpha1.Notification
	ImportLists        []commonv1alpha1.ImportList
	ReleaseProfiles    []commonv1alpha1.ReleaseProfile
	// LidarrMetadataProfiles and ReadarrMetadataProfiles are each app's own
	// metadata profiles.
	LidarrMetadataProfiles  []pvrv1alpha1.LidarrMetadataProfile
	ReadarrMetadataProfiles []pvrv1alpha1.ReadarrMetadataProfile
	// QualityDefinitions and DelayProfiles change items the app already has as
	// well as adding to them.
	QualityDefinitions []commonv1alpha1.QualityDefinition
	DelayProfiles      []commonv1alpha1.DelayProfile
	MetadataConsumers  []commonv1alpha1.MetadataConsumer
	AutoTags           []commonv1alpha1.AutoTag

	// schemas caches each provider kind's /schema listing for one reconcile.
	schemas map[string][]map[string]any
	// planned names, by kind, what observe mode may find missing that the
	// resource declares, so references to them resolve; see engine.PlannedID.
	planned map[string][]string
}

// schema returns the cached listing for a provider kind, reading it when absent.
func (o Options) schema(ctx context.Context, client *engine.HTTPClient, apiVersion, kind string) []map[string]any {
	if s, ok := o.schemas[kind]; ok {
		return s
	}
	return fetchSchema(ctx, client, apiVersion, kind)
}

// stages are the kinds in the order they're written, after tags and the
// settings. Items name what earlier stages wrote, whose IDs only exist once it's
// written: indexers name download clients, release profiles name indexers,
// quality profiles name custom formats, auto tags name quality profiles and
// metadata profiles, root folders in Lidarr and Readarr name quality profiles,
// and import lists name root folders.
var stages = [][]string{
	{
		"metadataProfiles", "remotePathMappings", "downloadClients", "customFormats",
		"qualityDefinitions", "notifications", "metadataConsumers",
	},
	{"indexers"},
	{"releaseProfiles"},
	{"qualityProfiles"},
	{"autoTags"},
	{"rootFolders"},
	{"importLists"},
}

func (o Options) builders(ctx context.Context, client *engine.HTTPClient) map[string]engine.Builder {
	v := o.App.APIVersion
	return map[string]engine.Builder{
		"tags": {Count: len(o.Tags), Build: func(engine.NamedIDs) ([]map[string]any, error) {
			return tagResources(o.Tags), nil
		}},
		"metadataProfiles": {Count: len(o.LidarrMetadataProfiles) + len(o.ReadarrMetadataProfiles), Build: func(engine.NamedIDs) ([]map[string]any, error) {
			if len(o.LidarrMetadataProfiles) > 0 {
				return lidarrMetadataProfileResources(ctx, client, v, o.LidarrMetadataProfiles)
			}
			return readarrMetadataProfileResources(ctx, client, v, o.ReadarrMetadataProfiles)
		}},
		"remotePathMappings": {Count: len(o.RemotePathMappings), Build: func(engine.NamedIDs) ([]map[string]any, error) {
			return remotePathMappingResources(o.RemotePathMappings)
		}},
		"rootFolders": {
			Count:  len(o.RootFolders),
			Tagged: slices.ContainsFunc(o.RootFolders, func(rf commonv1alpha1.RootFolder) bool { return len(rf.DefaultTagLabels) > 0 }),
			Build: func(tags engine.NamedIDs) ([]map[string]any, error) {
				return rootFolderResources(ctx, client, v, o, tags)
			},
		},
		"downloadClients": {
			Count:  len(o.DownloadClients),
			Tagged: slices.ContainsFunc(o.DownloadClients, func(dc commonv1alpha1.DownloadClient) bool { return len(dc.TagLabels) > 0 }),
			Build: func(tags engine.NamedIDs) ([]map[string]any, error) {
				return downloadClientResources(ctx, client, v, o, tags)
			},
		},
		"indexers": {
			Count:  len(o.Indexers),
			Tagged: slices.ContainsFunc(o.Indexers, func(idx commonv1alpha1.Indexer) bool { return len(idx.TagLabels) > 0 }),
			Build: func(tags engine.NamedIDs) ([]map[string]any, error) {
				return indexerResources(ctx, client, v, o, tags)
			},
		},
		"customFormats": {Count: len(o.CustomFormats), Build: func(engine.NamedIDs) ([]map[string]any, error) {
			return customFormatResources(o.CustomFormats), nil
		}},
		"qualityDefinitions": {Count: len(o.QualityDefinitions), Build: func(engine.NamedIDs) ([]map[string]any, error) {
			return qualityDefinitionResources(ctx, client, v, o.QualityDefinitions)
		}},
		"qualityProfiles": {Count: len(o.QualityProfiles), Build: func(engine.NamedIDs) ([]map[string]any, error) {
			return qualityProfileResources(ctx, client, v, o.QualityProfiles, o.planned["customFormats"])
		}},
		"notifications": {
			Count:  len(o.Notifications),
			Tagged: slices.ContainsFunc(o.Notifications, func(n commonv1alpha1.Notification) bool { return len(n.TagLabels) > 0 }),
			Build: func(tags engine.NamedIDs) ([]map[string]any, error) {
				return notificationResources(ctx, client, v, o, tags)
			},
		},
		"importLists": {
			Count:  len(o.ImportLists),
			Tagged: slices.ContainsFunc(o.ImportLists, func(il commonv1alpha1.ImportList) bool { return len(il.TagLabels) > 0 }),
			Build: func(tags engine.NamedIDs) ([]map[string]any, error) {
				return importListResources(ctx, client, v, o, tags)
			},
		},
		"releaseProfiles": {
			Count:  len(o.ReleaseProfiles),
			Tagged: slices.ContainsFunc(o.ReleaseProfiles, func(rp commonv1alpha1.ReleaseProfile) bool { return len(rp.TagLabels) > 0 }),
			Build: func(tags engine.NamedIDs) ([]map[string]any, error) {
				return releaseProfileResources(ctx, client, v, o.ReleaseProfiles, tags, o.planned["indexers"])
			},
		},
		"autoTags": {Count: len(o.AutoTags), Tagged: true, Build: func(tags engine.NamedIDs) ([]map[string]any, error) {
			return autoTagResources(ctx, client, v, o, tags)
		}},
		"metadataConsumers": {Count: len(o.MetadataConsumers), Build: func(engine.NamedIDs) ([]map[string]any, error) {
			return metadataConsumerResources(ctx, client, v, o.MetadataConsumers)
		}},
	}
}

func tagResources(declared []commonv1alpha1.Tag) []map[string]any {
	tags := make([]map[string]any, 0, len(declared))
	for _, t := range declared {
		tags = append(tags, map[string]any{"label": engine.TagLabel(t.Label)})
	}
	return tags
}

func customFormatResources(declared []commonv1alpha1.CustomFormat) []map[string]any {
	cfs := make([]map[string]any, 0, len(declared))
	for _, cf := range declared {
		cfs = append(cfs, buildCustomFormatPayload(cf))
	}
	return cfs
}

func downloadClientResources(
	ctx context.Context, client *engine.HTTPClient, apiVersion string, opts Options, tagIDs engine.NamedIDs,
) ([]map[string]any, error) {
	schema := opts.schema(ctx, client, apiVersion, "downloadclient")
	dcs := make([]map[string]any, 0, len(opts.DownloadClients))
	for _, dc := range opts.DownloadClients {
		tags, err := tagIDs.ResolveTags(dc.TagLabels)
		if err != nil {
			return nil, fmt.Errorf("downloadClients(%s): %w", dc.Name, err)
		}
		dc.ConfigContract = schemaConfigContract(schema, dc.Implementation, dc.ConfigContract)
		dc.Protocol = schemaProtocol(schema, dc.Implementation, dc.Protocol)
		dcs = append(dcs, BuildDownloadClientPayload(dc, opts.DCSecrets[dc.Name], opts.App.CategoryField, tags))
	}
	return dcs, nil
}

func indexerResources(
	ctx context.Context, client *engine.HTTPClient, apiVersion string, opts Options, tagIDs engine.NamedIDs,
) ([]map[string]any, error) {
	schema := opts.schema(ctx, client, apiVersion, "indexer")
	var clients engine.NamedIDs
	idxs := make([]map[string]any, 0, len(opts.Indexers))
	for _, idx := range opts.Indexers {
		tags, err := tagIDs.ResolveTags(idx.TagLabels)
		if err != nil {
			return nil, fmt.Errorf("indexers(%s): %w", idx.Name, err)
		}
		idx.ConfigContract = schemaConfigContract(schema, idx.Implementation, idx.ConfigContract)
		idx.Protocol = schemaProtocol(schema, idx.Implementation, idx.Protocol)
		payload := BuildIndexerPayload(idx, tags)
		if idx.DownloadClientName != "" {
			if clients == nil {
				if clients, err = engine.FetchNamedIDs(ctx, client, fmt.Sprintf("/api/%s/downloadclient", apiVersion), "name"); err != nil {
					return nil, err
				}
				clients.Plan(opts.planned["downloadClients"]...)
			}
			id, err := clients.Resolve(idx.DownloadClientName)
			if err != nil {
				return nil, fmt.Errorf("indexers(%s): download client: %w", idx.Name, err)
			}
			payload["downloadClientId"] = id
		}
		idxs = append(idxs, payload)
	}
	return idxs, nil
}

func releaseProfileResources(
	ctx context.Context, client *engine.HTTPClient, apiVersion string, declared []commonv1alpha1.ReleaseProfile,
	tagIDs engine.NamedIDs, plannedIndexers []string,
) ([]map[string]any, error) {
	var indexers engine.NamedIDs
	profiles := make([]map[string]any, 0, len(declared))
	for _, rp := range declared {
		payload := map[string]any{"name": rp.Name}
		if rp.Enabled != nil {
			payload["enabled"] = *rp.Enabled
		}
		if rp.Required != nil {
			payload["required"] = rp.Required
		}
		if rp.Ignored != nil {
			payload["ignored"] = rp.Ignored
		}
		if rp.TagLabels != nil {
			tags, err := tagIDs.ResolveTags(rp.TagLabels)
			if err != nil {
				return nil, fmt.Errorf("releaseProfiles(%s): %w", rp.Name, err)
			}
			payload["tags"] = buildTagsPayload(tags)
		}
		if rp.IndexerName != "" {
			if indexers == nil {
				var err error
				if indexers, err = engine.FetchNamedIDs(ctx, client, fmt.Sprintf("/api/%s/indexer", apiVersion), "name"); err != nil {
					return nil, err
				}
				indexers.Plan(plannedIndexers...)
			}
			id, err := indexers.Resolve(rp.IndexerName)
			if err != nil {
				return nil, fmt.Errorf("releaseProfiles(%s): indexer: %w", rp.Name, err)
			}
			payload["indexerId"] = id
		}
		profiles = append(profiles, payload)
	}
	return profiles, nil
}

func notificationResources(
	ctx context.Context, client *engine.HTTPClient, apiVersion string, opts Options, tagIDs engine.NamedIDs,
) ([]map[string]any, error) {
	schema := opts.schema(ctx, client, apiVersion, "notification")
	notifs := make([]map[string]any, 0, len(opts.Notifications))
	for _, n := range opts.Notifications {
		tags, err := tagIDs.ResolveTags(n.TagLabels)
		if err != nil {
			return nil, fmt.Errorf("notifications(%s): %w", n.Name, err)
		}
		n.ConfigContract = schemaConfigContract(schema, n.Implementation, n.ConfigContract)
		payload := BuildNotificationPayload(n, tags)
		// Validation reports these; sending them would rewrite the notification forever.
		for _, event := range unsupportedEvents(schema, n.Implementation, payload) {
			delete(payload, event)
		}
		notifs = append(notifs, payload)
	}
	return notifs, nil
}

func importListResources(
	ctx context.Context, client *engine.HTTPClient, apiVersion string, opts Options, tagIDs engine.NamedIDs,
) ([]map[string]any, error) {
	profiles, err := engine.FetchNamedIDs(ctx, client, fmt.Sprintf("/api/%s/qualityprofile", apiVersion), "name")
	if err != nil {
		return nil, err
	}
	profiles.Plan(opts.planned["qualityProfiles"]...)
	var metadataProfiles engine.NamedIDs
	if opts.App.MetadataProfiles {
		if metadataProfiles, err = engine.FetchNamedIDs(ctx, client, fmt.Sprintf("/api/%s/metadataprofile", apiVersion), "name"); err != nil {
			return nil, err
		}
		metadataProfiles.Plan(opts.planned["metadataProfiles"]...)
	}
	schema := opts.schema(ctx, client, apiVersion, "importlist")
	ils := make([]map[string]any, 0, len(opts.ImportLists))
	for _, il := range opts.ImportLists {
		tags, err := tagIDs.ResolveTags(il.TagLabels)
		if err != nil {
			return nil, fmt.Errorf("importLists(%s): %w", il.Name, err)
		}
		il.ConfigContract = schemaConfigContract(schema, il.Implementation, il.ConfigContract)
		qpIDs := map[string]int{}
		if il.QualityProfileName != "" {
			id, err := profiles.Resolve(il.QualityProfileName)
			if err != nil {
				return nil, fmt.Errorf("importLists(%s): %w", il.Name, err)
			}
			qpIDs[il.QualityProfileName] = id
		}
		payload := buildImportListPayload(il, qpIDs, opts.App.ImportListFields, tags)
		if opts.App.MetadataProfiles && il.MetadataProfileName != "" {
			id, err := metadataProfiles.Resolve(il.MetadataProfileName)
			if err != nil {
				return nil, fmt.Errorf("importLists(%s): %w", il.Name, err)
			}
			payload["metadataProfileId"] = id
		}
		ils = append(ils, payload)
	}
	return ils, nil
}

// Reconcile runs the full reconciliation for Sonarr, Radarr, Lidarr or Readarr.
func Reconcile(ctx context.Context, client *engine.HTTPClient, spec any, opts Options, pass *engine.Pass) []string {
	apiVersion := opts.App.APIVersion
	def := Definition(opts.App)
	sections, err := Sections(spec, opts.App.Choices)
	if err != nil {
		return []string{fmt.Sprintf("building sections: %v", err)}
	}
	opts = withSchemas(ctx, client, apiVersion, opts)
	if pass.Observe {
		opts.planned = plannedNames(opts)
	}

	problems := validateOptionFields(ctx, client, apiVersion, opts)
	run := engine.Stages{
		Order: [][]string{{"tags"}}, Builders: opts.builders(ctx, client),
		TagPath: "/api/" + apiVersion + "/tag", PlannedTags: opts.planned["tags"],
	}
	problems = append(problems, run.Reconcile(ctx, client, def, pass)...)

	// The settings name tags, and go before the kinds they govern, so Radarr's
	// list sync level changes before its import lists do.
	if err := resolveListSyncTag(ctx, client, apiVersion, sections, opts.planned["tags"]); err != nil {
		delete(sections, "importListConfig")
		problems = append(problems, fmt.Sprintf("importListConfig: %v", err))
	}
	problems = append(problems, engine.ReconcileApp(ctx, client, def, sections, nil, pass)...)

	run.Order = stages
	problems = append(problems, run.Reconcile(ctx, client, def, pass)...)
	if len(opts.DelayProfiles) > 0 {
		problems = append(problems, reconcileDelayProfiles(ctx, client, apiVersion, opts.DelayProfiles, pass, opts.planned["tags"])...)
	}
	return problems
}

// resolveListSyncTag puts the ID of the tag Sonarr's listSyncTagLabel names in
// listSyncTag.
func resolveListSyncTag(ctx context.Context, client *engine.HTTPClient, apiVersion string, sections map[string]any, planned []string) error {
	config, _ := sections["importListConfig"].(map[string]any)
	label, _ := config["listSyncTagLabel"].(string)
	if label == "" {
		return nil
	}
	tags, err := engine.FetchNamedIDs(ctx, client, fmt.Sprintf("/api/%s/tag", apiVersion), "label")
	if err != nil {
		return err
	}
	tags.Plan(planned...)
	id, err := tags.Resolve(engine.TagLabel(label))
	if err != nil {
		return fmt.Errorf("listSyncTagLabel: %w", err)
	}
	delete(config, "listSyncTagLabel")
	config["listSyncTag"] = id
	return nil
}

// plannedNames lists what the resource declares that others in it may refer to
// by name.
func plannedNames(opts Options) map[string][]string {
	planned := map[string][]string{}
	for _, t := range opts.Tags {
		planned["tags"] = append(planned["tags"], engine.TagLabel(t.Label))
	}
	for _, qp := range opts.QualityProfiles {
		planned["qualityProfiles"] = append(planned["qualityProfiles"], qp.Name)
	}
	for _, cf := range opts.CustomFormats {
		planned["customFormats"] = append(planned["customFormats"], cf.Name)
	}
	for _, mp := range opts.LidarrMetadataProfiles {
		planned["metadataProfiles"] = append(planned["metadataProfiles"], mp.Name)
	}
	for _, mp := range opts.ReadarrMetadataProfiles {
		planned["metadataProfiles"] = append(planned["metadataProfiles"], mp.Name)
	}
	for _, dc := range opts.DownloadClients {
		planned["downloadClients"] = append(planned["downloadClients"], dc.Name)
	}
	for _, idx := range opts.Indexers {
		planned["indexers"] = append(planned["indexers"], idx.Name)
	}
	return planned
}

// These apps store directory paths with a trailing separator. Use the same form
// before matching so a fresh mapping is not recreated on every reconciliation.
func directoryPath(path string) string {
	if path == "" || strings.HasSuffix(path, "/") || strings.HasSuffix(path, `\`) {
		return path
	}
	if strings.Contains(path, `\`) || (len(path) >= 2 && path[1] == ':') {
		return path + `\`
	}
	return path + "/"
}

func remotePathMappingKey(item map[string]any) string {
	host, _ := item["host"].(string)
	path, _ := item["remotePath"].(string)
	if host == "" || path == "" {
		return ""
	}
	key, _ := json.Marshal([]string{host, directoryPath(path)})
	return string(key)
}

// autoTagResources builds the declared auto tags, resolving the tags they
// apply and the quality profiles, metadata profiles and tags their conditions
// name to the app's IDs.
func autoTagResources(
	ctx context.Context, client *engine.HTTPClient, apiVersion string, opts Options, tagIDs engine.NamedIDs,
) ([]map[string]any, error) {
	var profiles, metadataProfiles engine.NamedIDs
	for _, at := range opts.AutoTags {
		for _, spec := range at.Specifications {
			var err error
			if spec.QualityProfileName != "" && profiles == nil {
				if profiles, err = engine.FetchNamedIDs(ctx, client, fmt.Sprintf("/api/%s/qualityprofile", apiVersion), "name"); err != nil {
					return nil, err
				}
				profiles.Plan(opts.planned["qualityProfiles"]...)
			}
			if spec.MetadataProfileName != "" && metadataProfiles == nil {
				if metadataProfiles, err = engine.FetchNamedIDs(ctx, client, fmt.Sprintf("/api/%s/metadataprofile", apiVersion), "name"); err != nil {
					return nil, err
				}
				metadataProfiles.Plan(opts.planned["metadataProfiles"]...)
			}
		}
	}
	out := make([]map[string]any, 0, len(opts.AutoTags))
	for _, at := range opts.AutoTags {
		tags, err := tagIDs.ResolveTags(at.TagLabels)
		if err != nil {
			return nil, fmt.Errorf("autoTags(%s): %w", at.Name, err)
		}
		specs := make([]map[string]any, 0, len(at.Specifications))
		for _, spec := range at.Specifications {
			payload, err := autoTagSpecificationPayload(spec, profiles, metadataProfiles, tagIDs)
			if err != nil {
				return nil, fmt.Errorf("autoTags(%s): %s: %w", at.Name, spec.Name, err)
			}
			specs = append(specs, payload)
		}
		payload := map[string]any{"name": at.Name, "tags": buildTagsPayload(tags), "specifications": specs}
		SetOptionalBool(payload, "removeTagsAutomatically", at.RemoveTagsAutomatically)
		out = append(out, payload)
	}
	return out, nil
}

// autoTagSpecificationPayload builds one condition, putting the ID of the
// profile or tag it names in its value field.
func autoTagSpecificationPayload(
	spec commonv1alpha1.AutoTagSpecification, profiles, metadataProfiles, tagIDs engine.NamedIDs,
) (map[string]any, error) {
	fields := BuildFieldsPayload(spec.Fields)
	for _, ref := range []struct {
		field, name string
		ids         engine.NamedIDs
	}{
		{"qualityProfileName", spec.QualityProfileName, profiles},
		{"metadataProfileName", spec.MetadataProfileName, metadataProfiles},
		{"tagLabel", engine.TagLabel(spec.TagLabel), tagIDs},
	} {
		if ref.name == "" {
			continue
		}
		if slices.ContainsFunc(fields, func(f map[string]any) bool { return f["name"] == "value" }) {
			return nil, fmt.Errorf("%s fills in value, so leave that field out", ref.field)
		}
		id, err := ref.ids.Resolve(ref.name)
		if err != nil {
			return nil, err
		}
		fields = append(fields, map[string]any{"name": "value", "value": id})
	}
	payload := map[string]any{"name": spec.Name, "implementation": spec.Implementation, "fields": fields}
	SetOptionalBool(payload, "negate", spec.Negate)
	SetOptionalBool(payload, "required", spec.Required)
	return payload, nil
}

// metadataConsumerResources builds updates for the declared consumers,
// checking each type and field name against the consumers the app has.
func metadataConsumerResources(
	ctx context.Context, client *engine.HTTPClient, apiVersion string, declared []commonv1alpha1.MetadataConsumer,
) ([]map[string]any, error) {
	current, err := client.GetJSONList(ctx, fmt.Sprintf("/api/%s/metadata", apiVersion))
	if err != nil {
		return nil, err
	}
	byImplementation := make(map[string]map[string]any, len(current))
	for _, c := range current {
		impl, _ := c["implementation"].(string)
		byImplementation[impl] = c
	}
	out := make([]map[string]any, 0, len(declared))
	for _, mc := range declared {
		consumer, ok := byImplementation[mc.Implementation]
		if !ok {
			return nil, fmt.Errorf("metadataConsumers(%s): the app has no such consumer; it has %s",
				mc.Implementation, strings.Join(slices.Sorted(maps.Keys(byImplementation)), ", "))
		}
		payload := map[string]any{"implementation": mc.Implementation}
		SetOptionalBool(payload, "enable", mc.Enable)
		if len(mc.Fields) > 0 {
			names := make([]string, 0, len(mc.Fields))
			fields := make([]any, 0, len(mc.Fields))
			for _, f := range mc.Fields {
				names = append(names, f.Name)
				fields = append(fields, map[string]any{"name": f.Name, "value": f.Value.ToInterface()})
			}
			if err := validateFieldNames([]map[string]any{consumer}, mc.Implementation, names); err != nil {
				return nil, fmt.Errorf("metadataConsumers(%s): %w", mc.Implementation, err)
			}
			payload["fields"] = fields
		}
		out = append(out, payload)
	}
	return out, nil
}

// rootFolderResources builds root folders. Lidarr and Readarr reject a folder
// without a name and default quality and metadata profiles; those profiles are
// referenced by name so a rebuilt app's new IDs are picked up. A folder that
// exists keeps its name unless the spec sets one.
func rootFolderResources(
	ctx context.Context, client *engine.HTTPClient, apiVersion string, opts Options, tagIDs engine.NamedIDs,
) ([]map[string]any, error) {
	var qualities, metadata engine.NamedIDs
	existing := map[string]bool{}
	if opts.App.MetadataProfiles {
		folders, err := client.GetJSONList(ctx, fmt.Sprintf("/api/%s/rootfolder", apiVersion))
		if err != nil {
			return nil, err
		}
		for _, f := range folders {
			if p, ok := f["path"].(string); ok {
				existing[p] = true
			}
		}
		if qualities, err = engine.FetchNamedIDs(ctx, client, fmt.Sprintf("/api/%s/qualityprofile", apiVersion), "name"); err != nil {
			return nil, err
		}
		qualities.Plan(opts.planned["qualityProfiles"]...)
		if metadata, err = engine.FetchNamedIDs(ctx, client, fmt.Sprintf("/api/%s/metadataprofile", apiVersion), "name"); err != nil {
			return nil, err
		}
		metadata.Plan(opts.planned["metadataProfiles"]...)
	}
	rfs := make([]map[string]any, 0, len(opts.RootFolders))
	for _, rf := range opts.RootFolders {
		item := map[string]any{"path": rf.Path}
		if opts.App.MetadataProfiles {
			if rf.DefaultQualityProfileName == "" || rf.DefaultMetadataProfileName == "" {
				return nil, fmt.Errorf("rootFolders(%s): defaultQualityProfileName and defaultMetadataProfileName are required", rf.Path)
			}
			qualityID, err := qualities.Resolve(rf.DefaultQualityProfileName)
			if err != nil {
				return nil, fmt.Errorf("rootFolders(%s): %w", rf.Path, err)
			}
			metadataID, err := metadata.Resolve(rf.DefaultMetadataProfileName)
			if err != nil {
				return nil, fmt.Errorf("rootFolders(%s): %w", rf.Path, err)
			}
			switch {
			case rf.Name != "":
				item["name"] = rf.Name
			case !existing[rf.Path]:
				item["name"] = folderName(rf.Path)
			}
			item["defaultQualityProfileId"] = qualityID
			item["defaultMetadataProfileId"] = metadataID
			if rf.DefaultMonitorOption != "" {
				item["defaultMonitorOption"] = rf.DefaultMonitorOption
			}
			if rf.DefaultNewItemMonitorOption != "" {
				item["defaultNewItemMonitorOption"] = rf.DefaultNewItemMonitorOption
			}
			tags, err := tagIDs.ResolveTags(rf.DefaultTagLabels)
			if err != nil {
				return nil, fmt.Errorf("rootFolders(%s): %w", rf.Path, err)
			}
			if tags != nil {
				item["defaultTags"] = buildTagsPayload(tags)
			}
		}
		rfs = append(rfs, item)
	}
	return rfs, nil
}

// folderName is the last element of a container path, POSIX or Windows.
func folderName(dir string) string {
	trimmed := strings.TrimRight(dir, `/\`)
	return trimmed[strings.LastIndexAny(trimmed, `/\`)+1:]
}

func remotePathMappingResources(mappings []commonv1alpha1.RemotePathMapping) ([]map[string]any, error) {
	seen := map[string]bool{}
	resources := make([]map[string]any, 0, len(mappings))
	for _, mapping := range mappings {
		item := map[string]any{
			"host": mapping.Host, "remotePath": directoryPath(mapping.RemotePath), "localPath": directoryPath(mapping.LocalPath),
		}
		key := remotePathMappingKey(item)
		if key == "" || mapping.LocalPath == "" {
			return nil, fmt.Errorf("remotePathMappings requires host, remotePath and localPath")
		}
		if seen[key] {
			return nil, fmt.Errorf("remotePathMappings has duplicate identity %s", key)
		}
		seen[key] = true
		resources = append(resources, item)
	}
	return resources, nil
}

func num(v any) float64 {
	f, _ := v.(float64)
	return f
}

// same compares as the app reads values back, so an int written reads back
// equal to the float64 decoded from the app.
func same(a, b map[string]any) bool {
	x, errX := json.Marshal(a)
	y, errY := json.Marshal(b)
	return errX == nil && errY == nil && bytes.Equal(x, y)
}
