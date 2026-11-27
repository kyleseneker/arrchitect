package prowlarr

import (
	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	indexersv1alpha1 "github.com/kyleseneker/arrchitect/api/indexers/v1alpha1"
	pvrclient "github.com/kyleseneker/arrchitect/internal/client/pvr"
	"github.com/kyleseneker/arrchitect/internal/engine"
)

// Definition returns the AppDefinition for Prowlarr.
func Definition() engine.AppDefinition {
	return engine.AppDefinition{
		HealthPath: "/api/v1/system/status",
		Resources: []engine.ResourceEndpoint{
			// Tags first, since other resources reference tag IDs.
			{Name: "tags", Path: "/api/v1/tag", MatchField: "label", Policy: engine.CreateOrUpdate},
			{Name: "appProfiles", Path: "/api/v1/appprofile", MatchField: "name", Policy: engine.CreateOrUpdate},
			{
				Name: "applications", Path: "/api/v1/applications", MatchField: "name", Policy: engine.CreateOrUpdate, Prunable: true,
				CreateDefaults: map[string]any{"fields": []any{}, "tags": []any{}},
			},
			{
				Name: "indexers", Path: "/api/v1/indexer", MatchField: "name", Policy: engine.CreateOrUpdate, Prunable: true,
				CreateDefaults: map[string]any{"enable": true, "fields": []any{}, "tags": []any{}},
			},
			{
				Name: "proxies", Path: "/api/v1/indexerproxy", MatchField: "name", Policy: engine.CreateOrUpdate, Prunable: true,
				CreateDefaults: map[string]any{"tags": []any{}},
			},
			{
				Name: "downloadClients", Path: "/api/v1/downloadclient", MatchField: "name", Policy: engine.CreateOrUpdate, Prunable: true,
				CreateDefaults: map[string]any{"enable": true, "fields": []any{}, "categories": []any{}, "tags": []any{}},
			},
			{
				Name: "notifications", Path: "/api/v1/notification", MatchField: "name", Policy: engine.CreateOrUpdate, Prunable: true,
				CreateDefaults: map[string]any{"fields": []any{}, "tags": []any{}},
			},
		},
	}
}

// Options holds all the inputs needed to reconcile Prowlarr.
type Options struct {
	AppProfiles     []indexersv1alpha1.ProwlarrAppProfile
	Tags            []commonv1alpha1.Tag
	Applications    []map[string]any
	Indexers        []indexersv1alpha1.ProwlarrIndexer
	Proxies         []map[string]any
	DownloadClients []indexersv1alpha1.ProwlarrDownloadClient
	Notifications   []commonv1alpha1.Notification
}

// Resources builds the resources map from the options.
func Resources(opts Options, tagIDs map[string]int) map[string][]map[string]any {
	resources := make(map[string][]map[string]any)
	for _, profile := range opts.AppProfiles {
		payload := map[string]any{"name": profile.Name}
		if profile.EnableRSS != nil {
			payload["enableRss"] = *profile.EnableRSS
		}
		if profile.EnableAutomaticSearch != nil {
			payload["enableAutomaticSearch"] = *profile.EnableAutomaticSearch
		}
		if profile.EnableInteractiveSearch != nil {
			payload["enableInteractiveSearch"] = *profile.EnableInteractiveSearch
		}
		if profile.MinimumSeeders != nil {
			payload["minimumSeeders"] = *profile.MinimumSeeders
		}
		resources["appProfiles"] = append(resources["appProfiles"], payload)
	}

	if len(opts.Tags) > 0 {
		tags := make([]map[string]any, 0, len(opts.Tags))
		for _, t := range opts.Tags {
			tags = append(tags, map[string]any{"label": engine.TagLabel(t.Label)})
		}
		resources["tags"] = tags
	}

	// Applications are pre-built by the controller (require secret resolution).
	if len(opts.Applications) > 0 {
		resources["applications"] = opts.Applications
	}

	if len(opts.Indexers) > 0 {
		idxs := make([]map[string]any, 0, len(opts.Indexers))
		for _, idx := range opts.Indexers {
			idxs = append(idxs, BuildIndexerPayload(idx, tagIDs))
		}
		resources["indexers"] = idxs
	}

	// Proxies are pre-built by the controller, like applications.
	if len(opts.Proxies) > 0 {
		resources["proxies"] = opts.Proxies
	}

	if len(opts.DownloadClients) > 0 {
		dcs := make([]map[string]any, 0, len(opts.DownloadClients))
		for _, dc := range opts.DownloadClients {
			dcs = append(dcs, BuildDownloadClientPayload(dc, tagIDs))
		}
		resources["downloadClients"] = dcs
	}

	if len(opts.Notifications) > 0 {
		notifs := make([]map[string]any, 0, len(opts.Notifications))
		for _, n := range opts.Notifications {
			payload := pvrclient.BuildNotificationPayload(n, nil)
			setTags(payload, n.TagLabels, tagIDs)
			notifs = append(notifs, payload)
		}
		resources["notifications"] = notifs
	}

	return resources
}

// BuildApplicationPayload builds the API payload for a Prowlarr application.
// secrets holds the resolved apiKey, authUsername and authPassword, by field name.
func BuildApplicationPayload(app indexersv1alpha1.ProwlarrApplication, secrets map[string]string, tagIDs map[string]int) map[string]any {
	fields := []map[string]any{
		{"name": "baseUrl", "value": app.BaseURL},
		{"name": "apiKey", "value": secrets["apiKey"]},
	}
	for _, name := range []string{"authUsername", "authPassword"} {
		if value, ok := secrets[name]; ok {
			fields = append(fields, map[string]any{"name": name, "value": value})
		}
	}
	if app.ProwlarrURL != "" {
		fields = append(fields, map[string]any{"name": "prowlarrUrl", "value": app.ProwlarrURL})
	}
	if app.SyncCategories != nil {
		fields = append(fields, map[string]any{"name": "syncCategories", "value": intSliceToInterface(app.SyncCategories)})
	}
	if app.AnimeSyncCategories != nil {
		fields = append(fields, map[string]any{"name": "animeSyncCategories", "value": intSliceToInterface(app.AnimeSyncCategories)})
	}
	for name, value := range map[string]*bool{
		"syncAnimeStandardFormatSearch":                   app.SyncAnimeStandardFormatSearch,
		"syncRejectBlocklistedTorrentHashesWhileGrabbing": app.SyncRejectBlocklistedTorrentHashesWhileGrabbing,
	} {
		if value != nil {
			fields = append(fields, map[string]any{"name": name, "value": *value})
		}
	}
	configContract := app.ConfigContract
	if configContract == "" {
		configContract = app.Implementation + "Settings"
	}
	payload := map[string]any{
		"name": app.Name, "implementation": app.Implementation, "configContract": configContract,
		"fields": fields,
	}
	if app.SyncLevel != "" {
		payload["syncLevel"] = app.SyncLevel
	}
	setTags(payload, app.TagLabels, tagIDs)
	return payload
}

// BuildIndexerPayload builds the API payload for a Prowlarr indexer.
func BuildIndexerPayload(idx indexersv1alpha1.ProwlarrIndexer, tagIDs map[string]int) map[string]any {
	// Prowlarr's indexer schema lists every tracker definition, too large to read
	// each pass; Torznab, Newznab and Cardigann all follow this naming.
	configContract := idx.Implementation + "Settings"
	if idx.ConfigContract != "" {
		configContract = idx.ConfigContract
	}
	desired := map[string]any{
		"name": idx.Name, "implementation": idx.Implementation, "configContract": configContract,
	}
	pvrclient.SetOptionalBool(desired, "enable", idx.Enable)
	if idx.Fields != nil {
		desired["fields"] = pvrclient.BuildFieldsPayload(idx.Fields)
	}
	setTags(desired, idx.TagLabels, tagIDs)
	if idx.AppProfileID != nil {
		desired["appProfileId"] = *idx.AppProfileID
	}
	if idx.Priority != nil {
		desired["priority"] = *idx.Priority
	}
	return desired
}

// BuildProxyPayload builds the API payload for a Prowlarr proxy.
// credentials holds the resolved username and password, by field name.
func BuildProxyPayload(proxy indexersv1alpha1.ProwlarrProxy, credentials map[string]string, tagIDs map[string]int) map[string]any {
	fields := []map[string]any{{"name": "host", "value": proxy.Host}}
	if proxy.Port != 0 {
		fields = append(fields, map[string]any{"name": "port", "value": proxy.Port})
	}
	for _, name := range []string{"username", "password"} {
		if value, ok := credentials[name]; ok {
			fields = append(fields, map[string]any{"name": name, "value": value})
		}
	}
	if proxy.RequestTimeout != nil {
		fields = append(fields, map[string]any{"name": "requestTimeout", "value": *proxy.RequestTimeout})
	}
	payload := map[string]any{
		"name": proxy.Name, "implementation": proxy.Implementation,
		"configContract": proxy.ConfigContract, "fields": fields,
	}
	setTags(payload, proxy.TagLabels, tagIDs)
	return payload
}

// BuildDownloadClientPayload builds the API payload for a Prowlarr download client.
func BuildDownloadClientPayload(dc indexersv1alpha1.ProwlarrDownloadClient, tagIDs map[string]int) map[string]any {
	configContract := dc.Implementation + "Settings"
	if dc.ConfigContract != "" {
		configContract = dc.ConfigContract
	}

	desired := map[string]any{
		"name":           dc.Name,
		"protocol":       dc.Protocol,
		"implementation": dc.Implementation,
		"configContract": configContract,
	}
	pvrclient.SetOptionalBool(desired, "enable", dc.Enable)
	if dc.Fields != nil {
		desired["fields"] = pvrclient.BuildFieldsPayload(dc.Fields)
	}
	if dc.Categories != nil {
		categories := make([]map[string]any, 0, len(dc.Categories))
		for _, cat := range dc.Categories {
			categories = append(categories, map[string]any{
				"clientCategory": cat.ClientCategory,
				"categories":     intSliceToInterface(cat.Categories),
			})
		}
		desired["categories"] = categories
	}
	setTags(desired, dc.TagLabels, tagIDs)

	if dc.Priority != nil {
		desired["priority"] = *dc.Priority
	}

	return desired
}

// intSliceToInterface converts an int slice to an interface slice for JSON serialization.
func intSliceToInterface(s []int32) []any {
	r := make([]any, len(s))
	for i, v := range s {
		r[i] = int(v)
	}
	return r
}

// setTags maps declared tag labels to the IDs Prowlarr assigned them.
// ResolveReferences rejects unknown labels before payloads are built; any that
// remain are dropped rather than sent as-is, since Prowlarr expects integers.
func setTags(payload map[string]any, labels []string, tagIDs map[string]int) {
	if labels == nil {
		return
	}
	out := make([]any, 0, len(labels))
	for _, l := range labels {
		if id, ok := tagIDs[l]; ok {
			out = append(out, id)
		}
	}
	payload["tags"] = out
}
