package pvr

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	pvrv1alpha1 "github.com/kyleseneker/arrchitect/api/pvr/v1alpha1"
	"github.com/kyleseneker/arrchitect/internal/engine"
	"github.com/kyleseneker/arrchitect/internal/metrics"
	"github.com/kyleseneker/arrchitect/internal/reconciler"
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
		Choices: map[string]map[string]map[string]int{"naming": {
			"colonReplacementFormat": {"delete": 0, "dash": 1, "spaceDash": 2, "spaceDashSpace": 3, "smart": 4, "custom": 5},
			"multiEpisodeStyle":      {"extend": 0, "duplicate": 1, "repeat": 2, "scene": 3, "range": 4, "prefixedRange": 5},
		}},
	}
	Radarr = App{
		Name: "radarr", Kind: "Radarr", APIVersion: "v3", CategoryField: "movieCategory", ReleaseProfiles: true,
		ImportListFields: ImportListFields{Enable: "enabled", AutomaticAdd: "enableAuto", Monitor: "monitor", Search: "searchOnAdd"},
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
// apiVersion is "v3" for Sonarr/Radarr, "v1" for Lidarr/Readarr; sections
// are the app's own settings sections (App.Sections).
func Definition(apiVersion string, sections ...string) engine.AppDefinition {
	prefix := fmt.Sprintf("/api/%s", apiVersion)
	settings := make([]engine.SettingEndpoint, 0, 5+len(sections))
	settings = append(settings,
		engine.SettingEndpoint{Name: "mediaManagement", Path: prefix + "/config/mediamanagement"},
		engine.SettingEndpoint{Name: "naming", Path: prefix + "/config/naming"},
		engine.SettingEndpoint{Name: "indexerConfig", Path: prefix + "/config/indexer"},
		engine.SettingEndpoint{Name: "downloadClientConfig", Path: prefix + "/config/downloadclient"},
		engine.SettingEndpoint{Name: "ui", Path: prefix + "/config/ui"},
	)
	for _, name := range sections {
		settings = append(settings, engine.SettingEndpoint{Name: name, Path: prefix + sectionPaths[name]})
	}
	return engine.AppDefinition{
		HealthPath: prefix + "/system/status",
		Settings:   settings,
		Resources: []engine.ResourceEndpoint{
			// Tags first — other resources reference tag IDs. Not prunable (may be shared).
			{Name: "tags", Path: prefix + "/tag", MatchField: "label", Policy: engine.CreateOrUpdate},
			// Metadata profiles before root folders and import lists, which name them.
			{Name: "metadataProfiles", Path: prefix + "/metadataprofile", MatchField: "name", Policy: engine.CreateOrUpdate, Prunable: true},
			// Root folders not prunable (destructive to remove); only Lidarr's and
			// Readarr's have settings to update.
			{Name: "rootFolders", Path: prefix + "/rootfolder", MatchField: "path", Policy: engine.CreateOnly},
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
			// The app keeps one metadata consumer per type, created at startup.
			{Name: "metadataConsumers", Path: prefix + "/metadata", MatchField: "implementation", Policy: engine.UpdateOnly},
			// importLists last — may reference quality profiles by name.
			{
				Name: "importLists", Path: prefix + "/importlist", MatchField: "name", Policy: engine.CreateOrUpdate, Prunable: true,
				CreateDefaults: map[string]any{"fields": []any{}, "tags": []any{}},
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
	RemotePathMappings []commonv1alpha1.RemotePathMapping
	RootFolders        []commonv1alpha1.RootFolder
	DownloadClients    []commonv1alpha1.DownloadClient
	DCSecrets          map[string]DownloadClientResolvedSecrets
	CategoryField      string
	ImportListFields   ImportListFields
	Choices            map[string]map[string]map[string]int
	// MetadataProfiles marks apps with metadata profiles (Lidarr, Readarr): their
	// root folders need a name and default profiles, and import lists a metadata profile.
	MetadataProfiles bool
	// Sections are the app's own settings sections (App.Sections).
	Sections        []string
	QualityProfiles []commonv1alpha1.QualityProfile
	CustomFormats   []commonv1alpha1.CustomFormat
	Tags            []commonv1alpha1.Tag
	Indexers        []commonv1alpha1.Indexer
	Notifications   []commonv1alpha1.Notification
	ImportLists     []commonv1alpha1.ImportList
	ReleaseProfiles []commonv1alpha1.ReleaseProfile
	// LidarrMetadataProfiles and ReadarrMetadataProfiles are each app's own
	// metadata profiles.
	LidarrMetadataProfiles  []pvrv1alpha1.LidarrMetadataProfile
	ReadarrMetadataProfiles []pvrv1alpha1.ReadarrMetadataProfile
	// QualityDefinitions and DelayProfiles change items the app already has as
	// well as adding to them.
	QualityDefinitions []commonv1alpha1.QualityDefinition
	DelayProfiles      []commonv1alpha1.DelayProfile
	MetadataConsumers  []commonv1alpha1.MetadataConsumer

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
	return FetchSchema(ctx, client, apiVersion, kind)
}

// Resources builds the resources map from the options. A kind that can't be
// built, such as one naming a tag that doesn't exist, is left out of the map,
// so it is neither written nor pruned, and reported in the error; the other
// kinds are still built.
func Resources(ctx context.Context, client *engine.HTTPClient, apiVersion string, opts Options) (map[string][]map[string]any, error) {
	var tagIDs engine.NamedIDs
	if hasTagLabels(opts) {
		var err error
		tagIDs, err = engine.FetchNamedIDs(ctx, client, fmt.Sprintf("/api/%s/tag", apiVersion), "label")
		if err != nil {
			return nil, err
		}
		tagIDs.Plan(opts.planned["tags"]...)
	}

	builders := []struct {
		name  string
		count int
		build func() ([]map[string]any, error)
	}{
		{"tags", len(opts.Tags), func() ([]map[string]any, error) { return tagResources(opts.Tags), nil }},
		{"metadataProfiles", len(opts.LidarrMetadataProfiles) + len(opts.ReadarrMetadataProfiles), func() ([]map[string]any, error) {
			if len(opts.LidarrMetadataProfiles) > 0 {
				return lidarrMetadataProfileResources(ctx, client, apiVersion, opts.LidarrMetadataProfiles)
			}
			return readarrMetadataProfileResources(ctx, client, apiVersion, opts.ReadarrMetadataProfiles)
		}},
		{"remotePathMappings", len(opts.RemotePathMappings), func() ([]map[string]any, error) {
			return remotePathMappingResources(opts.RemotePathMappings)
		}},
		{"rootFolders", len(opts.RootFolders), func() ([]map[string]any, error) {
			return rootFolderResources(ctx, client, apiVersion, opts, tagIDs)
		}},
		{"downloadClients", len(opts.DownloadClients), func() ([]map[string]any, error) {
			return downloadClientResources(ctx, client, apiVersion, opts, tagIDs)
		}},
		{"indexers", len(opts.Indexers), func() ([]map[string]any, error) {
			return indexerResources(ctx, client, apiVersion, opts, tagIDs)
		}},
		{"customFormats", len(opts.CustomFormats), func() ([]map[string]any, error) {
			return customFormatResources(opts.CustomFormats), nil
		}},
		{"qualityDefinitions", len(opts.QualityDefinitions), func() ([]map[string]any, error) {
			return qualityDefinitionResources(ctx, client, apiVersion, opts.QualityDefinitions)
		}},
		{"qualityProfiles", len(opts.QualityProfiles), func() ([]map[string]any, error) {
			return qualityProfileResources(ctx, client, apiVersion, opts.QualityProfiles, opts.planned["customFormats"])
		}},
		{"notifications", len(opts.Notifications), func() ([]map[string]any, error) {
			return notificationResources(ctx, client, apiVersion, opts, tagIDs)
		}},
		{"importLists", len(opts.ImportLists), func() ([]map[string]any, error) {
			return importListResources(ctx, client, apiVersion, opts, tagIDs)
		}},
		{"releaseProfiles", len(opts.ReleaseProfiles), func() ([]map[string]any, error) {
			return releaseProfileResources(ctx, client, apiVersion, opts.ReleaseProfiles, tagIDs)
		}},
		{"metadataConsumers", len(opts.MetadataConsumers), func() ([]map[string]any, error) {
			return metadataConsumerResources(ctx, client, apiVersion, opts.MetadataConsumers)
		}},
	}

	resources := make(map[string][]map[string]any)
	var problems []string
	for _, b := range builders {
		if b.count == 0 {
			continue
		}
		items, err := b.build()
		if err != nil {
			problems = append(problems, err.Error())
			continue
		}
		resources[b.name] = items
	}

	// Record managed resource counts for observability.
	app := client.AppLabel()
	for resourceType, items := range resources {
		metrics.ManagedResources.WithLabelValues(app, resourceType).Set(float64(len(items)))
	}

	if len(problems) > 0 {
		return resources, errors.New(strings.Join(problems, "; "))
	}
	return resources, nil
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
		cfs = append(cfs, BuildCustomFormatPayload(cf))
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
		dc.ConfigContract = SchemaConfigContract(schema, dc.Implementation, dc.ConfigContract)
		dc.Protocol = SchemaProtocol(schema, dc.Implementation, dc.Protocol)
		dcs = append(dcs, BuildDownloadClientPayload(dc, opts.DCSecrets[dc.Name], opts.CategoryField, tags))
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
		idx.ConfigContract = SchemaConfigContract(schema, idx.Implementation, idx.ConfigContract)
		idx.Protocol = SchemaProtocol(schema, idx.Implementation, idx.Protocol)
		payload := BuildIndexerPayload(idx, tags)
		if idx.DownloadClientName != "" {
			if clients == nil {
				if clients, err = engine.FetchNamedIDs(ctx, client, fmt.Sprintf("/api/%s/downloadclient", apiVersion), "name"); err != nil {
					return nil, err
				}
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
	ctx context.Context, client *engine.HTTPClient, apiVersion string, declared []commonv1alpha1.ReleaseProfile, tagIDs engine.NamedIDs,
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
			payload["tags"] = BuildTagsPayload(tags)
		}
		if rp.IndexerName != "" {
			if indexers == nil {
				var err error
				if indexers, err = engine.FetchNamedIDs(ctx, client, fmt.Sprintf("/api/%s/indexer", apiVersion), "name"); err != nil {
					return nil, err
				}
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
		n.ConfigContract = SchemaConfigContract(schema, n.Implementation, n.ConfigContract)
		payload := BuildNotificationPayload(n, tags)
		// Validation reports these; sending them would rewrite the notification forever.
		for _, event := range UnsupportedEvents(schema, n.Implementation, payload) {
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
	if opts.MetadataProfiles {
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
		il.ConfigContract = SchemaConfigContract(schema, il.Implementation, il.ConfigContract)
		qpIDs := map[string]int{}
		if il.QualityProfileName != "" {
			id, err := profiles.Resolve(il.QualityProfileName)
			if err != nil {
				return nil, fmt.Errorf("importLists(%s): %w", il.Name, err)
			}
			qpIDs[il.QualityProfileName] = id
		}
		payload := BuildImportListPayload(il, qpIDs, opts.ImportListFields, tags)
		if opts.MetadataProfiles && il.MetadataProfileName != "" {
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

// BuildDownloadClientPayload builds the API payload for a download client.
func BuildDownloadClientPayload(
	dc commonv1alpha1.DownloadClient, secrets DownloadClientResolvedSecrets, categoryField string, tags []int,
) map[string]any {
	configContract := dc.Implementation + "Settings"
	if dc.ConfigContract != "" {
		configContract = dc.ConfigContract
	}

	fields := []map[string]any{
		{"name": "host", "value": dc.Host},
		{"name": "port", "value": dc.Port},
	}
	if dc.UseSsl != nil {
		fields = append(fields, map[string]any{"name": "useSsl", "value": *dc.UseSsl})
	}
	if dc.Category != "" {
		fields = append(fields, map[string]any{"name": categoryField, "value": dc.Category})
	}
	if dc.UrlBase != "" || dc.AppRef != (commonv1alpha1.AppRef{}) {
		fields = append(fields, map[string]any{"name": "urlBase", "value": dc.UrlBase})
	}
	if secrets.Username != "" {
		fields = append(fields, map[string]any{"name": "username", "value": secrets.Username})
	}
	if secrets.Password != "" {
		fields = append(fields, map[string]any{"name": "password", "value": secrets.Password})
	}
	if secrets.APIKey != "" {
		fields = append(fields, map[string]any{"name": "apiKey", "value": secrets.APIKey})
	}

	knownFields := map[string]bool{
		"host": true, "port": true, "useSsl": true, "urlBase": true,
		"username": true, "password": true, "apiKey": true,
		categoryField: true,
	}
	for _, f := range dc.Fields {
		if knownFields[f.Name] {
			continue
		}
		var val any
		if f.Value != nil {
			val = f.Value.ToInterface()
		}
		fields = append(fields, map[string]any{"name": f.Name, "value": val})
	}

	payload := map[string]any{
		"name":           dc.Name,
		"protocol":       dc.Protocol,
		"implementation": dc.Implementation,
		"configContract": configContract,
		"fields":         fields,
	}
	SetOptionalBool(payload, "enable", dc.Enable)
	SetOptionalBool(payload, "removeCompletedDownloads", dc.RemoveCompletedDownloads)
	SetOptionalBool(payload, "removeFailedDownloads", dc.RemoveFailedDownloads)
	if dc.Priority != 0 {
		payload["priority"] = dc.Priority
	}
	if tags != nil {
		payload["tags"] = BuildTagsPayload(tags)
	}
	return payload
}

// BuildIndexerPayload builds the API payload for an indexer.
func BuildIndexerPayload(idx commonv1alpha1.Indexer, tags []int) map[string]any {
	configContract := idx.Implementation + "Settings"
	if idx.ConfigContract != "" {
		configContract = idx.ConfigContract
	}

	payload := map[string]any{
		"name":           idx.Name,
		"protocol":       idx.Protocol,
		"implementation": idx.Implementation,
		"configContract": configContract,
	}

	// The apps have no on/off switch for an indexer, only the three uses of it,
	// so enable gates those instead of being sent and dropped as drift.
	uses := map[string]*bool{
		"enableRss":               idx.EnableRss,
		"enableAutomaticSearch":   idx.EnableAutomaticSearch,
		"enableInteractiveSearch": idx.EnableInteractiveSearch,
	}
	for key, use := range uses {
		switch {
		case idx.Enable != nil && !*idx.Enable:
			payload[key] = false
		case use != nil:
			payload[key] = *use
		case idx.Enable != nil:
			payload[key] = true
		}
	}

	if idx.Fields != nil {
		payload["fields"] = BuildFieldsPayload(idx.Fields)
	}
	if tags != nil {
		payload["tags"] = BuildTagsPayload(tags)
	}
	if idx.Priority != 0 {
		payload["priority"] = idx.Priority
	}

	return payload
}

// BuildNotificationPayload builds the API payload for a notification.
func BuildNotificationPayload(n commonv1alpha1.Notification, tags []int) map[string]any {
	configContract := n.Implementation + "Settings"
	if n.ConfigContract != "" {
		configContract = n.ConfigContract
	}

	payload := map[string]any{
		"name":           n.Name,
		"implementation": n.Implementation,
		"configContract": configContract,
	}
	if n.Fields != nil {
		payload["fields"] = BuildFieldsPayload(n.Fields)
	}
	if tags != nil {
		payload["tags"] = BuildTagsPayload(tags)
	}

	// Add trigger booleans — only set non-nil values so the merge preserves API defaults.
	SetOptionalBool(payload, "onGrab", n.OnGrab)
	SetOptionalBool(payload, "onDownload", n.OnDownload)
	SetOptionalBool(payload, "onUpgrade", n.OnUpgrade)
	SetOptionalBool(payload, "onRename", n.OnRename)
	SetOptionalBool(payload, "onHealthIssue", n.OnHealthIssue)
	SetOptionalBool(payload, "onHealthRestored", n.OnHealthRestored)
	SetOptionalBool(payload, "onApplicationUpdate", n.OnApplicationUpdate)
	SetOptionalBool(payload, "onManualInteractionRequired", n.OnManualInteractionRequired)
	SetOptionalBool(payload, "includeHealthWarnings", n.IncludeHealthWarnings)
	SetOptionalBool(payload, "includeManualGrabs", n.IncludeManualGrabs)
	// Sonarr-specific
	SetOptionalBool(payload, "onSeriesAdd", n.OnSeriesAdd)
	SetOptionalBool(payload, "onSeriesDelete", n.OnSeriesDelete)
	SetOptionalBool(payload, "onEpisodeFileDelete", n.OnEpisodeFileDelete)
	SetOptionalBool(payload, "onEpisodeFileDeleteForUpgrade", n.OnEpisodeFileDeleteForUpgrade)
	// Radarr-specific
	SetOptionalBool(payload, "onMovieAdded", n.OnMovieAdded)
	SetOptionalBool(payload, "onMovieDelete", n.OnMovieDelete)
	SetOptionalBool(payload, "onMovieFileDelete", n.OnMovieFileDelete)
	SetOptionalBool(payload, "onMovieFileDeleteForUpgrade", n.OnMovieFileDeleteForUpgrade)
	SetOptionalBool(payload, "onImportComplete", n.OnImportComplete)
	// Lidarr and Readarr
	SetOptionalBool(payload, "onReleaseImport", n.OnReleaseImport)
	SetOptionalBool(payload, "onDownloadFailure", n.OnDownloadFailure)
	SetOptionalBool(payload, "onImportFailure", n.OnImportFailure)
	// Lidarr-specific
	SetOptionalBool(payload, "onArtistAdd", n.OnArtistAdd)
	SetOptionalBool(payload, "onArtistDelete", n.OnArtistDelete)
	SetOptionalBool(payload, "onAlbumDelete", n.OnAlbumDelete)
	SetOptionalBool(payload, "onTrackRetag", n.OnTrackRetag)
	// Readarr-specific
	SetOptionalBool(payload, "onAuthorAdded", n.OnAuthorAdded)
	SetOptionalBool(payload, "onAuthorDelete", n.OnAuthorDelete)
	SetOptionalBool(payload, "onBookDelete", n.OnBookDelete)
	SetOptionalBool(payload, "onBookFileDelete", n.OnBookFileDelete)
	SetOptionalBool(payload, "onBookFileDeleteForUpgrade", n.OnBookFileDeleteForUpgrade)
	SetOptionalBool(payload, "onBookRetag", n.OnBookRetag)

	return payload
}

// ImportListFields names an app's import list properties, which the PVR apps
// spell differently. An empty name means the app has no such property, so it is
// not sent: a property the app drops reads back missing and is rewritten forever.
type ImportListFields struct {
	// Enable switches the list on (Radarr: enabled).
	Enable string
	// AutomaticAdd adds found items without review (Radarr: enableAuto).
	AutomaticAdd string
	// Monitor takes the app's monitor enum (Radarr: monitor; others: shouldMonitor).
	Monitor string
	// Search searches for what a list adds (Sonarr: searchForMissingEpisodes;
	// Radarr: searchOnAdd; Lidarr, Readarr: shouldSearch).
	Search string
	// MonitorNewItems monitors what is added to a series or artist later.
	MonitorNewItems string
	// MonitorExisting also applies monitor to items already in the library.
	MonitorExisting string
}

// BuildImportListPayload builds the API payload for an import list.
// qpIDs maps quality profile names to their IDs.
func BuildImportListPayload(il commonv1alpha1.ImportList, qpIDs map[string]int, names ImportListFields, tags []int) map[string]any {
	configContract := il.Implementation + "Settings"
	if il.ConfigContract != "" {
		configContract = il.ConfigContract
	}

	payload := map[string]any{
		"name":           il.Name,
		"implementation": il.Implementation,
		"configContract": configContract,
	}
	if il.Fields != nil {
		payload["fields"] = BuildFieldsPayload(il.Fields)
	}
	if tags != nil {
		payload["tags"] = BuildTagsPayload(tags)
	}

	if names.Enable != "" {
		SetOptionalBool(payload, names.Enable, il.Enable)
	}
	if names.AutomaticAdd != "" {
		SetOptionalBool(payload, names.AutomaticAdd, il.EnableAutomaticAdd)
	}
	if names.Search != "" {
		SetOptionalBool(payload, names.Search, il.SearchOnAdd)
	}
	if names.MonitorExisting != "" {
		SetOptionalBool(payload, names.MonitorExisting, il.MonitorExisting)
	}
	if il.MonitorNewItems != "" && names.MonitorNewItems != "" {
		payload[names.MonitorNewItems] = il.MonitorNewItems
	}
	SetOptionalBool(payload, "seasonFolder", il.SeasonFolder)

	if value := il.Monitor; value != "" && names.Monitor != "" {
		payload[names.Monitor] = value
	}
	if il.RootFolderPath != "" {
		payload["rootFolderPath"] = il.RootFolderPath
	}
	if il.SeriesType != "" {
		payload["seriesType"] = il.SeriesType
	}
	if il.MinimumAvailability != "" {
		payload["minimumAvailability"] = il.MinimumAvailability
	}
	if il.ListOrder != nil {
		payload["listOrder"] = *il.ListOrder
	}

	// Resolve quality profile name → ID
	if il.QualityProfileName != "" {
		if id, ok := qpIDs[il.QualityProfileName]; ok {
			payload["qualityProfileId"] = id
		}
	}

	return payload
}

// BuildFieldsPayload converts ConfigField slice to API fields format.
func BuildFieldsPayload(dcFields []commonv1alpha1.ConfigField) []map[string]any {
	fields := make([]map[string]any, len(dcFields))
	for i, f := range dcFields {
		var val any
		if f.Value != nil {
			val = f.Value.ToInterface()
		}
		fields[i] = map[string]any{
			"name":  f.Name,
			"value": val,
		}
	}
	return fields
}

// BuildTagsPayload converts an int slice to an interface slice for JSON.
func BuildTagsPayload(tagIDs []int) []any {
	tags := make([]any, len(tagIDs))
	for i, t := range tagIDs {
		tags[i] = t
	}
	return tags
}

// SetOptionalBool sets a key in the payload only if the pointer is non-nil.
func SetOptionalBool(payload map[string]any, key string, val *bool) {
	if val != nil {
		payload[key] = *val
	}
}

// BuildCustomFormatPayload builds the API payload for a custom format.
func BuildCustomFormatPayload(cf commonv1alpha1.CustomFormat) map[string]any {
	payload := map[string]any{"name": cf.Name}
	SetOptionalBool(payload, "includeCustomFormatWhenRenaming", cf.IncludeCustomFormatWhenRenaming)
	if cf.Specifications == nil {
		return payload
	}

	specs := make([]map[string]any, len(cf.Specifications))
	for i, s := range cf.Specifications {
		fields := make([]map[string]any, len(s.Fields))
		for j, f := range s.Fields {
			var val any
			if f.Value != nil {
				val = f.Value.ToInterface()
			}
			fields[j] = map[string]any{
				"name":  f.Name,
				"value": val,
			}
		}

		specs[i] = map[string]any{
			"name":           s.Name,
			"implementation": s.Implementation,
			"fields":         fields,
		}
		SetOptionalBool(specs[i], "negate", s.Negate)
		SetOptionalBool(specs[i], "required", s.Required)
	}
	payload["specifications"] = specs
	return payload
}

// BuildQualityProfilePayload builds the API payload for a quality profile.
// cfIDs maps custom format names to their IDs for resolving formatItems references.
func BuildQualityProfilePayload(qp commonv1alpha1.QualityProfile, cfIDs map[string]int) map[string]any {
	payload := map[string]any{
		"name": qp.Name,
	}

	if qp.UpgradeAllowed != nil {
		payload["upgradeAllowed"] = *qp.UpgradeAllowed
	}
	if qp.MinFormatScore != nil {
		payload["minFormatScore"] = *qp.MinFormatScore
	}
	if qp.CutoffFormatScore != nil {
		payload["cutoffFormatScore"] = *qp.CutoffFormatScore
	}
	if qp.MinUpgradeFormatScore != nil {
		payload["minUpgradeFormatScore"] = *qp.MinUpgradeFormatScore
	}

	if len(qp.Items) > 0 {
		items := BuildQualityItems(qp.Items)
		payload["items"] = items

		// Resolve cutoff name to ID
		if qp.Cutoff != "" {
			if id, ok := FindCutoffID(qp.Items, qp.Cutoff); ok {
				payload["cutoff"] = id
			}
		}
	}

	if len(qp.FormatItems) > 0 {
		formatItems := make([]map[string]any, 0, len(qp.FormatItems))
		for _, fi := range qp.FormatItems {
			if id, ok := cfIDs[fi.Name]; ok {
				formatItems = append(formatItems, map[string]any{
					"format": id,
					"name":   fi.Name,
					"score":  scoreOf(fi.Score),
				})
			}
		}
		payload["formatItems"] = formatItems
	}

	return payload
}

// BuildQualityItems converts CRD quality profile items to API payload format.
func BuildQualityItems(items []commonv1alpha1.QualityProfileItem) []map[string]any {
	result := make([]map[string]any, len(items))
	for i, item := range items {
		m := map[string]any{}

		if item.Quality != (commonv1alpha1.QualityReference{}) {
			// Individual quality
			m["quality"] = map[string]any{
				"id":   qualityID(item.Quality.ID),
				"name": item.Quality.Name,
			}
			m["items"] = []any{}
		} else {
			// Quality group. The PVR apps treat an item without a positive id as a
			// single quality, so a group must carry its 1000+index id; FindCutoffID
			// resolves a group cutoff to the same id.
			m["id"] = 1000 + i
			m["name"] = item.Name
			children := make([]map[string]any, len(item.Items))
			for j, child := range item.Items {
				childAllowed := child.Allowed == nil || *child.Allowed
				children[j] = map[string]any{
					"quality": map[string]any{
						"id":   qualityID(child.Quality.ID),
						"name": child.Quality.Name,
					},
					"items":   []any{},
					"allowed": childAllowed,
				}
			}
			m["items"] = children
		}

		if item.Allowed != nil {
			m["allowed"] = *item.Allowed
		} else {
			m["allowed"] = true
		}

		result[i] = m
	}
	return result
}

// FindCutoffID resolves a cutoff name (quality name or group name) to the corresponding ID.
// For individual qualities, returns quality.id. For groups, returns the group item index + 1000
// (the IDs these apps give quality groups).
func FindCutoffID(items []commonv1alpha1.QualityProfileItem, cutoffName string) (int, bool) {
	for i, item := range items {
		if item.Quality != (commonv1alpha1.QualityReference{}) && item.Quality.Name == cutoffName {
			return qualityID(item.Quality.ID), true
		}
		if item.Quality == (commonv1alpha1.QualityReference{}) && item.Name == cutoffName {
			// Group — use 1000+index as the group ID (how these apps number groups)
			return 1000 + i, true
		}
	}
	return 0, false
}

// Reconcile runs the full reconciliation for Sonarr, Radarr, Lidarr or Readarr.
func Reconcile(ctx context.Context, client *engine.HTTPClient, apiVersion string, spec any, opts Options, policy engine.SyncPolicy, managed map[string][]string) (engine.ReconcileResult, error) {
	def := Definition(apiVersion, opts.Sections...)
	if opts.MetadataProfiles {
		for i := range def.Resources {
			if def.Resources[i].Name == "rootFolders" {
				def.Resources[i].Policy = engine.CreateOrUpdate
			}
		}
	}
	if opts.ImportListFields.Enable != "" {
		for i := range def.Resources {
			if def.Resources[i].Name == "importLists" {
				def.Resources[i].CreateDefaults[opts.ImportListFields.Enable] = true
			}
		}
	}
	sections, err := Sections(spec, opts.Choices)
	if err != nil {
		return engine.ReconcileResult{}, fmt.Errorf("building sections: %w", err)
	}
	// Quality profiles reference custom formats by ID, and import lists reference
	// quality profiles by ID. Those IDs only exist once the referenced resources
	// have been written, so each dependent stage resolves after the prior apply
	// rather than all at once up front.
	opts = withSchemas(ctx, client, apiVersion, opts)
	fieldProblems := ValidateOptionFields(ctx, client, apiVersion, opts)
	if policy.Observe {
		opts.planned = plannedNames(opts)
	}

	result := engine.ReconcileResult{Managed: managed}
	if len(opts.Tags) > 0 {
		tags, _ := Resources(ctx, client, apiVersion, Options{Tags: opts.Tags})
		result = engine.ReconcileApp(ctx, client, def, nil, tags, policy, result.Managed)
	}

	base := opts
	base.Tags = nil
	base.QualityProfiles = nil
	base.RootFolders = nil
	base.ImportLists = nil
	base.DelayProfiles = nil
	resources, err := Resources(ctx, client, apiVersion, base)
	if err != nil {
		result.Errors = append(result.Errors, err.Error())
	}
	mergeResult(&result, engine.ReconcileApp(ctx, client, def, sections, resources, policy, result.Managed))
	result.Errors = append(result.Errors, fieldProblems...)

	if len(opts.DelayProfiles) > 0 {
		mergeResult(&result, reconcileDelayProfiles(ctx, client, apiVersion, opts.DelayProfiles, policy, result.Managed, opts.planned["tags"]))
	}

	if len(opts.QualityProfiles) > 0 {
		stage := Options{QualityProfiles: opts.QualityProfiles, planned: opts.planned}
		qpRes, qerr := Resources(ctx, client, apiVersion, stage)
		if qerr != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("qualityProfiles: %v", qerr))
		} else {
			mergeResult(&result, engine.ReconcileApp(ctx, client, def, nil, qpRes, policy, result.Managed))
		}
	}

	// Root folders in Lidarr and Readarr reference quality profiles, and import
	// lists reference root folders, so they apply between the two.
	if len(opts.RootFolders) > 0 {
		stage := Options{RootFolders: opts.RootFolders, MetadataProfiles: opts.MetadataProfiles, planned: opts.planned}
		rfRes, rerr := Resources(ctx, client, apiVersion, stage)
		if rerr != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("rootFolders: %v", rerr))
		} else {
			mergeResult(&result, engine.ReconcileApp(ctx, client, def, nil, rfRes, policy, result.Managed))
		}
	}

	if len(opts.ImportLists) > 0 {
		stage := Options{
			ImportLists: opts.ImportLists, ImportListFields: opts.ImportListFields,
			MetadataProfiles: opts.MetadataProfiles, schemas: opts.schemas, planned: opts.planned,
		}
		ilRes, ierr := Resources(ctx, client, apiVersion, stage)
		if ierr != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("importLists: %v", ierr))
		} else {
			mergeResult(&result, engine.ReconcileApp(ctx, client, def, nil, ilRes, policy, result.Managed))
		}
	}

	return result, nil
}

// plannedNames lists what the resource declares that others in it may refer to
// by name: tags, quality profiles and custom formats.
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
	return planned
}

func mergeResult(dst *engine.ReconcileResult, src engine.ReconcileResult) {
	dst.Synced = append(dst.Synced, src.Synced...)
	dst.Errors = append(dst.Errors, src.Errors...)
	dst.Drift = append(dst.Drift, src.Drift...)
	dst.Pruned = append(dst.Pruned, src.Pruned...)
	// Each stage receives the preceding snapshot, including successful deletions.
	// Appending snapshots duplicates ownership and resurrects pruned entries.
	dst.Managed = src.Managed
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

func qualityProfileResources(
	ctx context.Context, client *engine.HTTPClient, apiVersion string, profiles []commonv1alpha1.QualityProfile, plannedFormats []string,
) ([]map[string]any, error) {
	// Resolve custom format name → ID mapping for formatItems references.
	formats, err := engine.FetchNamedIDs(ctx, client, fmt.Sprintf("/api/%s/customformat", apiVersion), "name")
	if err != nil {
		return nil, err
	}
	formats.Plan(plannedFormats...)
	definitions, err := client.GetJSONList(ctx, fmt.Sprintf("/api/%s/qualitydefinition", apiVersion))
	if err != nil {
		return nil, err
	}
	current, err := client.GetJSONList(ctx, fmt.Sprintf("/api/%s/qualityprofile", apiVersion))
	if err != nil {
		return nil, err
	}
	existing := map[string]map[string]any{}
	for _, profile := range current {
		name, _ := profile["name"].(string)
		existing[name] = profile
	}
	languages, err := profileLanguages(ctx, client, apiVersion, profiles)
	if err != nil {
		return nil, err
	}
	var qps []map[string]any
	for _, qp := range profiles {
		cfIDs := map[string]int{}
		for _, format := range qp.FormatItems {
			id, err := formats.Resolve(format.Name)
			if err != nil {
				return nil, fmt.Errorf("qualityProfiles(%s): %w", qp.Name, err)
			}
			cfIDs[format.Name] = id
		}
		payload := BuildQualityProfilePayload(qp, cfIDs)
		if items, ok := payload["items"].([]map[string]any); ok {
			if profile, ok := existing[qp.Name]; ok {
				KeepGroupIDs(payload, profile)
			}
			payload["items"] = CompleteQualityItems(items, qp.Items, definitions)
		}
		if _, ok := existing[qp.Name]; !ok {
			declared, _ := payload["formatItems"].([]map[string]any)
			payload["formatItems"] = CompleteFormatItems(declared, formats)
		}
		if qp.Language != "" {
			id, ok := languages[qp.Language]
			if !ok {
				return nil, fmt.Errorf("qualityProfiles(%s): language %q does not exist", qp.Name, qp.Language)
			}
			payload["language"] = map[string]any{"id": id, "name": qp.Language}
		}
		qps = append(qps, payload)
	}
	return qps, nil
}

// profileLanguages maps the app's language names to their IDs, read only when
// a profile names a language.
func profileLanguages(
	ctx context.Context, client *engine.HTTPClient, apiVersion string, profiles []commonv1alpha1.QualityProfile,
) (map[string]any, error) {
	if !slices.ContainsFunc(profiles, func(qp commonv1alpha1.QualityProfile) bool { return qp.Language != "" }) {
		return nil, nil
	}
	list, err := client.GetJSONList(ctx, fmt.Sprintf("/api/%s/language", apiVersion))
	if err != nil {
		return nil, err
	}
	languages := make(map[string]any, len(list))
	for _, l := range list {
		if name, ok := l["name"].(string); ok {
			languages[name] = l["id"]
		}
	}
	return languages, nil
}

func qualityName(item map[string]any) string {
	quality, _ := item["quality"].(map[string]any)
	name, _ := quality["name"].(string)
	return name
}

func qualityDefinitionResources(
	ctx context.Context, client *engine.HTTPClient, apiVersion string, declared []commonv1alpha1.QualityDefinition,
) ([]map[string]any, error) {
	current, err := client.GetJSONList(ctx, fmt.Sprintf("/api/%s/qualitydefinition", apiVersion))
	if err != nil {
		return nil, err
	}
	byName := make(map[string]map[string]any, len(current))
	preferred := false
	for _, definition := range current {
		byName[qualityName(definition)] = definition
		_, has := definition["preferredSize"]
		preferred = preferred || has
	}
	out := make([]map[string]any, 0, len(declared))
	for _, qd := range declared {
		definition, ok := byName[qd.Quality]
		if !ok {
			return nil, fmt.Errorf("qualityDefinitions(%s): the app has no such quality", qd.Quality)
		}
		payload, err := qualityDefinitionPayload(qd, definition, preferred)
		if err != nil {
			return nil, fmt.Errorf("qualityDefinitions(%s): %w", qd.Quality, err)
		}
		out = append(out, payload)
	}
	return out, nil
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
			if err := ValidateFieldNames([]map[string]any{consumer}, mc.Implementation, names); err != nil {
				return nil, fmt.Errorf("metadataConsumers(%s): %w", mc.Implementation, err)
			}
			payload["fields"] = fields
		}
		out = append(out, payload)
	}
	return out, nil
}

// qualityDefinitionPayload builds an update for one definition. Readarr has no
// preferred size; the other apps leave it out only where it's unlimited.
func qualityDefinitionPayload(qd commonv1alpha1.QualityDefinition, current map[string]any, preferred bool) (map[string]any, error) {
	payload := map[string]any{"quality": map[string]any{"name": qd.Quality}}
	if qd.Title != "" {
		payload["title"] = qd.Title
	}
	if qd.MinSize != nil {
		payload["minSize"] = json.Number(*qd.MinSize)
	}
	if qd.MaxSize != nil {
		if err := setSize(payload, current, "maxSize", *qd.MaxSize); err != nil {
			return nil, err
		}
	}
	if qd.PreferredSize != nil {
		if !preferred {
			return nil, fmt.Errorf("this app has no preferred size")
		}
		if err := setSize(payload, current, "preferredSize", *qd.PreferredSize); err != nil {
			return nil, err
		}
	}
	return payload, nil
}

// setSize writes 0 as the app's unlimited, null. The apps leave a null out of
// what they return and already treat a maximum of 0 as unlimited, so a size
// the app has as unlimited is left out.
func setSize(payload, current map[string]any, key string, size commonv1alpha1.Decimal) error {
	n, err := strconv.ParseFloat(string(size), 64)
	if err != nil {
		return fmt.Errorf("%s: %w", key, err)
	}
	if n != 0 {
		payload[key] = json.Number(size)
		return nil
	}
	have, set := current[key].(float64)
	if set && (have != 0 || key == "preferredSize") {
		payload[key] = nil
	}
	return nil
}

// KeepGroupIDs gives each declared quality group the id it already has in the
// app's profile of the same name, and new groups the lowest free ids. The apps
// number groups themselves, so ids from a group's position would rewrite every
// profile the app created.
func KeepGroupIDs(payload, profile map[string]any) {
	existing := map[string]int{}
	rawItems, _ := profile["items"].([]any)
	for _, raw := range rawItems {
		item, _ := raw.(map[string]any)
		if _, single := item["quality"].(map[string]any); single {
			continue
		}
		name, _ := item["name"].(string)
		if id, ok := item["id"].(float64); ok {
			existing[name] = int(id)
		}
	}
	items, _ := payload["items"].([]map[string]any)
	used := map[int]bool{}
	remap := map[int]int{}
	var unmatched []map[string]any
	for _, item := range items {
		if _, single := item["quality"]; single {
			continue
		}
		name, _ := item["name"].(string)
		if id, ok := existing[name]; ok && !used[id] {
			remap[item["id"].(int)] = id
			item["id"] = id
			used[id] = true
			continue
		}
		unmatched = append(unmatched, item)
	}
	next := 1000
	for _, item := range unmatched {
		for used[next] {
			next++
		}
		remap[item["id"].(int)] = next
		item["id"] = next
		used[next] = true
	}
	if cutoff, ok := payload["cutoff"].(int); ok {
		if id, ok := remap[cutoff]; ok {
			payload["cutoff"] = id
		}
	}
}

// CompleteFormatItems lists every custom format the app has, scoring the
// undeclared ones 0. A new profile must name each custom format exactly once;
// an existing one already does, because the apps add each new format to every
// profile, so updates send only the declared scores and leave others' scores be.
func CompleteFormatItems(declared []map[string]any, formats engine.NamedIDs) []map[string]any {
	out := append([]map[string]any{}, declared...)
	seen := map[string]bool{}
	for _, item := range declared {
		name, _ := item["name"].(string)
		seen[name] = true
	}
	names := make([]string, 0, len(formats))
	for name := range formats {
		if !seen[name] {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		for _, id := range formats[name] {
			out = append(out, map[string]any{"format": id, "name": name, "score": 0})
		}
	}
	return out
}

// CompleteQualityItems sends each quality as the app defines it, and adds every
// quality the profile does not mention as a disallowed item ahead of the
// declared ones. The PVR apps reject a profile that omits any quality, and read
// qualities back with app-specific keys (source, resolution, modifier) that a
// bare id and name would strip on every write.
func CompleteQualityItems(items []map[string]any, declared []commonv1alpha1.QualityProfileItem, definitions []map[string]any) []map[string]any {
	byID := map[int]map[string]any{}
	var order []int
	for _, definition := range definitions {
		quality, _ := definition["quality"].(map[string]any)
		raw, ok := quality["id"].(float64)
		if !ok {
			continue
		}
		byID[int(raw)] = quality
		order = append(order, int(raw))
	}
	var resolve func(item map[string]any)
	resolve = func(item map[string]any) {
		if quality, ok := item["quality"].(map[string]any); ok {
			if id, ok := quality["id"].(int); ok && byID[id] != nil {
				item["quality"] = maps.Clone(byID[id])
			}
		}
		if children, ok := item["items"].([]map[string]any); ok {
			for _, child := range children {
				resolve(child)
			}
		}
	}
	for _, item := range items {
		resolve(item)
	}

	mentioned := map[int]bool{}
	for _, item := range declared {
		if item.Quality != (commonv1alpha1.QualityReference{}) {
			mentioned[qualityID(item.Quality.ID)] = true
		}
		for _, child := range item.Items {
			mentioned[qualityID(child.Quality.ID)] = true
		}
	}
	var missing []map[string]any
	for _, id := range order {
		if mentioned[id] {
			continue
		}
		mentioned[id] = true
		missing = append(missing, map[string]any{
			"quality": maps.Clone(byID[id]),
			"items":   []any{},
			"allowed": false,
		})
	}
	return append(missing, items...)
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
	if opts.MetadataProfiles {
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
		if opts.MetadataProfiles {
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
				item["defaultTags"] = BuildTagsPayload(tags)
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

const delayProfilesKind = "delayProfiles"

// Lidarr lists protocols as items, preferred first, instead of the fields the
// other apps have.
var delayProtocols = []struct{ key, protocol, name string }{
	{"usenet", "UsenetDownloadProtocol", "Usenet"},
	{"torrent", "TorrentDownloadProtocol", "Torrent"},
}

// Delay profiles have no name. Ownership is by the app's id, which a change of
// tags keeps.
func delayProfileID(item map[string]any) string {
	id, _ := item["id"].(float64)
	if id <= 0 {
		return ""
	}
	return strconv.Itoa(int(id))
}

type delayPlan struct {
	name    string
	tags    []int
	spec    commonv1alpha1.DelayProfile
	current map[string]any
}

type delayProfileStage struct {
	client *engine.HTTPClient
	path   string
	labels map[int]string
	owned  *engine.Ownership
	writes *engine.Writes
	prune  bool
	synced []string
}

// reconcileDelayProfiles matches each declared profile to the app's profile
// holding any of its tags, or to the default profile when it has none. The app
// allows a tag in only one profile, so pruned profiles go first and tags leave
// a profile before any joins another.
func reconcileDelayProfiles(
	ctx context.Context, client *engine.HTTPClient, apiVersion string, declared []commonv1alpha1.DelayProfile,
	policy engine.SyncPolicy, managed map[string][]string, plannedTags []string,
) engine.ReconcileResult {
	s := &delayProfileStage{
		client: client,
		path:   fmt.Sprintf("/api/%s/delayprofile", apiVersion),
		owned:  engine.NewOwnership(client.AppLabel(), managed),
		writes: &engine.Writes{App: client.AppLabel(), Observe: policy.Observe},
		prune:  policy.Prune,
	}
	errs := s.run(ctx, fmt.Sprintf("/api/%s/tag", apiVersion), declared, plannedTags)
	return engine.ReconcileResult{
		Synced:  s.synced,
		Errors:  errs,
		Drift:   s.writes.Drift(),
		Pruned:  s.owned.Pruned(),
		Managed: s.owned.Managed(),
	}
}

func (s *delayProfileStage) run(ctx context.Context, tagPath string, declared []commonv1alpha1.DelayProfile, plannedTags []string) []string {
	tags, err := engine.FetchNamedIDs(ctx, s.client, tagPath, "label")
	if err != nil {
		return []string{fmt.Sprintf("%s: %v", delayProfilesKind, err)}
	}
	tags.Plan(plannedTags...)
	s.labels = map[int]string{}
	for label, ids := range tags {
		for _, id := range ids {
			s.labels[id] = label
		}
	}
	existing, err := s.client.GetJSONList(ctx, s.path)
	if err != nil {
		return []string{fmt.Sprintf("%s: listing: %v", delayProfilesKind, err)}
	}
	plans, err := planDelayProfiles(declared, tags, existing)
	if err != nil {
		return []string{err.Error()}
	}

	if err := s.pruneUnmatched(ctx, plans, existing); err != nil {
		return []string{fmt.Sprintf("%s(prune): %v", delayProfilesKind, err)}
	}
	var errs []string
	if !s.writes.Observe {
		for _, p := range plans {
			if err := s.releaseTags(ctx, p); err != nil {
				return []string{fmt.Sprintf("%s(%s): %v", delayProfilesKind, p.name, err)}
			}
		}
	}
	var template map[string]any
	for _, p := range plans {
		if p.current == nil && template == nil && hasProtocolItems(existing) {
			if template, err = s.client.GetJSON(ctx, s.path+"/schema"); err != nil {
				errs = append(errs, fmt.Sprintf("%s(%s): reading the schema: %v", delayProfilesKind, p.name, err))
				continue
			}
		}
		if err := s.apply(ctx, p, template, hasProtocolItems(existing)); err != nil {
			errs = append(errs, fmt.Sprintf("%s(%s): %v", delayProfilesKind, p.name, err))
		}
	}
	if len(errs) > 0 {
		return errs
	}
	if err := s.order(ctx, plans); err != nil {
		return []string{fmt.Sprintf("%s(order): %v", delayProfilesKind, err)}
	}
	return nil
}

func planDelayProfiles(declared []commonv1alpha1.DelayProfile, tags engine.NamedIDs, existing []map[string]any) ([]*delayPlan, error) {
	var fallback map[string]any
	var tagged []map[string]any
	for _, e := range existing {
		if len(profileTags(e)) == 0 {
			fallback = e
		} else {
			tagged = append(tagged, e)
		}
	}

	plans := make([]*delayPlan, 0, len(declared))
	claimed := map[int]string{}
	matched := map[string]bool{}
	for _, d := range declared {
		p := &delayPlan{name: "default", spec: d}
		if len(d.TagLabels) > 0 {
			p.name = strings.Join(d.TagLabels, ",")
		}
		fail := func(format string, args ...any) error {
			return fmt.Errorf("%s(%s): %s", delayProfilesKind, p.name, fmt.Sprintf(format, args...))
		}
		ids, err := tags.ResolveTags(d.TagLabels)
		if err != nil {
			return nil, fail("%v", err)
		}
		slices.Sort(ids)
		p.tags = ids
		if len(ids) == 0 {
			switch {
			case fallback == nil:
				return nil, fail("the app has no default profile")
			case matched[delayProfileID(fallback)]:
				return nil, fail("only one profile can be without tags")
			}
			matched[delayProfileID(fallback)] = true
			p.current = fallback
			plans = append(plans, p)
			continue
		}
		for _, id := range ids {
			if other, ok := claimed[id]; ok {
				return nil, fail("shares a tag with %s, and the app allows a tag in only one profile", other)
			}
			claimed[id] = p.name
		}
		var best map[string]any
		bestOverlap := 0
		for _, e := range tagged {
			if matched[delayProfileID(e)] {
				continue
			}
			n := overlap(profileTags(e), ids)
			if n > bestOverlap || (n > 0 && n == bestOverlap && num(e["order"]) < num(best["order"])) {
				best, bestOverlap = e, n
			}
		}
		if best != nil {
			matched[delayProfileID(best)] = true
			p.current = best
		}
		plans = append(plans, p)
	}
	return plans, nil
}

// pruneUnmatched deletes owned profiles no declared profile matched, before
// any write that could need their tags.
func (s *delayProfileStage) pruneUnmatched(ctx context.Context, plans []*delayPlan, existing []map[string]any) error {
	if !s.prune {
		return nil
	}
	matched := map[string]bool{}
	for _, p := range plans {
		if p.current != nil {
			matched[delayProfileID(p.current)] = true
		}
	}
	names := map[string]string{}
	var unmatched []string
	for _, e := range existing {
		if id := delayProfileID(e); id != "" && !matched[id] {
			unmatched = append(unmatched, id)
			names[id] = s.describe(e)
		}
	}
	if s.writes.Observe {
		for _, id := range unmatched {
			if slices.Contains(s.owned.Managed()[delayProfilesKind], id) {
				s.writes.RecordDrift(delayProfilesKind, names[id])
			}
		}
		return nil
	}
	return s.owned.Cleanup(delayProfilesKind, unmatched, func(id string) error {
		err := s.client.DeleteJSON(ctx, s.path+"/"+id)
		if reason, refused := engine.Refusal(err); refused {
			return fmt.Errorf("the app refused: %s", reason)
		}
		return err
	})
}

// releaseTags drops the tags a matched profile is losing, so another declared
// profile can take them.
func (s *delayProfileStage) releaseTags(ctx context.Context, p *delayPlan) error {
	if p.current == nil || len(p.tags) == 0 {
		return nil
	}
	current := profileTags(p.current)
	kept := slices.DeleteFunc(slices.Clone(current), func(id int) bool { return !slices.Contains(p.tags, id) })
	if len(kept) == len(current) {
		return nil
	}
	tags := make([]any, 0, len(kept))
	for _, id := range kept {
		tags = append(tags, float64(id))
	}
	body := maps.Clone(p.current)
	body["tags"] = tags
	return s.writes.Apply(delayProfilesKind, p.name, func() error {
		if err := s.client.PutJSON(ctx, s.path+"/"+delayProfileID(p.current), body); err != nil {
			return err
		}
		p.current = body
		return nil
	})
}

func (s *delayProfileStage) apply(ctx context.Context, p *delayPlan, template map[string]any, protocolItems bool) error {
	if p.current == nil {
		body := map[string]any{"enableUsenet": true, "enableTorrent": true, "preferredProtocol": "usenet"}
		if protocolItems {
			body = map[string]any{"items": template["items"]}
		}
		desired, err := delayProfileBody(body, p, protocolItems)
		if err != nil {
			return err
		}
		return s.writes.Apply(delayProfilesKind, p.name, func() error {
			created, err := s.client.Do(ctx, http.MethodPost, s.path, desired)
			if err != nil {
				return err
			}
			var stored map[string]any
			if err := json.Unmarshal(created, &stored); err != nil || delayProfileID(stored) == "" {
				return fmt.Errorf("the app did not return the created profile")
			}
			p.current = stored
			s.owned.Created(delayProfilesKind, delayProfileID(stored))
			s.synced = append(s.synced, delayProfilesKind+"("+p.name+")")
			return nil
		})
	}

	desired, err := delayProfileBody(p.current, p, protocolItems)
	if err != nil {
		return err
	}
	if same(p.current, desired) {
		s.synced = append(s.synced, delayProfilesKind+"("+p.name+")")
		return nil
	}
	return s.writes.Apply(delayProfilesKind, p.name, func() error {
		if err := s.client.PutJSON(ctx, s.path+"/"+delayProfileID(p.current), desired); err != nil {
			return err
		}
		s.synced = append(s.synced, delayProfilesKind+"("+p.name+")")
		return nil
	})
}

// delayProfileBody overlays the declared settings on base, the app's profile or
// a new one.
func delayProfileBody(base map[string]any, p *delayPlan, protocolItems bool) (map[string]any, error) {
	d := p.spec
	set := map[string]any{}
	for key, value := range map[string]*bool{
		"bypassIfHighestQuality":         d.BypassIfHighestQuality,
		"bypassIfAboveCustomFormatScore": d.BypassIfAboveCustomFormatScore,
	} {
		if value != nil {
			set[key] = *value
		}
	}
	if d.MinimumCustomFormatScore != nil {
		set["minimumCustomFormatScore"] = *d.MinimumCustomFormatScore
	}
	if len(p.tags) > 0 {
		set["tags"] = p.tags
		if current := profileTags(base); len(current) == len(p.tags) && overlap(current, p.tags) == len(p.tags) {
			set["tags"] = current
		}
	}
	if !protocolItems {
		for key, value := range map[string]*bool{"enableUsenet": d.EnableUsenet, "enableTorrent": d.EnableTorrent} {
			if value != nil {
				set[key] = *value
			}
		}
		for key, value := range map[string]*int32{"usenetDelay": d.UsenetDelay, "torrentDelay": d.TorrentDelay} {
			if value != nil {
				set[key] = *value
			}
		}
		if d.PreferredProtocol != "" {
			set["preferredProtocol"] = d.PreferredProtocol
		}
	}
	out, err := reconciler.MergeDesired(base, set)
	if err != nil {
		return nil, err
	}
	if protocolItems {
		out.Merged["items"] = protocolItemsFor(base["items"], d)
	}
	return out.Merged, nil
}

func protocolItemsFor(current any, d commonv1alpha1.DelayProfile) []any {
	raw, _ := current.([]any)
	items := make([]any, 0, len(raw)+len(delayProtocols))
	seen := map[string]bool{}
	for _, it := range raw {
		item, ok := it.(map[string]any)
		if !ok {
			continue
		}
		item = maps.Clone(item)
		protocol, _ := item["protocol"].(string)
		seen[protocol] = true
		items = append(items, item)
	}
	for _, p := range delayProtocols {
		if !seen[p.protocol] {
			items = append(items, map[string]any{"name": p.name, "protocol": p.protocol, "allowed": false, "delay": 0})
		}
	}
	settings := map[string]struct {
		allowed *bool
		delay   *int32
	}{
		delayProtocols[0].protocol: {d.EnableUsenet, d.UsenetDelay},
		delayProtocols[1].protocol: {d.EnableTorrent, d.TorrentDelay},
	}
	for _, it := range items {
		item, _ := it.(map[string]any)
		protocol, _ := item["protocol"].(string)
		if setting, ok := settings[protocol]; ok {
			if setting.allowed != nil {
				item["allowed"] = *setting.allowed
			}
			if setting.delay != nil {
				item["delay"] = *setting.delay
			}
		}
	}
	preferred := ""
	for _, p := range delayProtocols {
		if p.key == d.PreferredProtocol {
			preferred = p.protocol
		}
	}
	if preferred != "" {
		slices.SortStableFunc(items, func(a, b any) int {
			pa, _ := a.(map[string]any)["protocol"].(string)
			pb, _ := b.(map[string]any)["protocol"].(string)
			switch {
			case pa == preferred && pb != preferred:
				return -1
			case pb == preferred && pa != preferred:
				return 1
			}
			return 0
		})
	}
	return items
}

// order moves declared tagged profiles the app checks out of turn, keeping
// where it has the others. Sonarr and Radarr read the profile to follow from
// after, Lidarr and Readarr from afterId.
func (s *delayProfileStage) order(ctx context.Context, plans []*delayPlan) error {
	var ids []string
	for _, p := range plans {
		if len(p.tags) > 0 && p.current != nil {
			ids = append(ids, delayProfileID(p.current))
		}
	}
	if len(ids) < 2 {
		return nil
	}
	profiles, err := s.client.GetJSONList(ctx, s.path)
	if err != nil {
		return err
	}
	for i := 1; i < len(ids); i++ {
		positions := map[string]float64{}
		for _, profile := range profiles {
			positions[delayProfileID(profile)] = num(profile["order"])
		}
		if positions[ids[i]] > positions[ids[i-1]] {
			continue
		}
		err := s.writes.Apply(delayProfilesKind, "order", func() error {
			body, err := s.client.Do(ctx, http.MethodPut, fmt.Sprintf("%s/reorder/%s?after=%s&afterId=%s", s.path, ids[i], ids[i-1], ids[i-1]), nil)
			if err != nil {
				return err
			}
			profiles = nil
			return json.Unmarshal(body, &profiles)
		})
		if err != nil || s.writes.Observe {
			return err
		}
	}
	return nil
}

func (s *delayProfileStage) describe(profile map[string]any) string {
	ids := profileTags(profile)
	if len(ids) == 0 {
		return "default"
	}
	labels := make([]string, 0, len(ids))
	for _, id := range ids {
		if label, ok := s.labels[id]; ok {
			labels = append(labels, label)
		} else {
			labels = append(labels, strconv.Itoa(id))
		}
	}
	return strings.Join(labels, ",")
}

func hasProtocolItems(existing []map[string]any) bool {
	for _, e := range existing {
		if _, ok := e["items"]; ok {
			return true
		}
	}
	return false
}

func profileTags(profile map[string]any) []int {
	raw, _ := profile["tags"].([]any)
	ids := make([]int, 0, len(raw))
	for _, v := range raw {
		if id, ok := v.(float64); ok {
			ids = append(ids, int(id))
		}
	}
	return ids
}

func overlap(a, b []int) int {
	n := 0
	for _, id := range a {
		if slices.Contains(b, id) {
			n++
		}
	}
	return n
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

// qualityID is a declared quality's ID; the API server requires one.
func qualityID(id *int32) int {
	if id == nil {
		return 0
	}
	return int(*id)
}

// scoreOf is a declared format score; the API server requires one.
func scoreOf(score *int32) int32 {
	if score == nil {
		return 0
	}
	return *score
}
