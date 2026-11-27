package prowlarr

import (
	"context"
	"fmt"
	"slices"

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

// Application is an application to sync indexers to, with the Secret values
// its payload carries: apiKey, authUsername and authPassword, by field name.
type Application struct {
	indexersv1alpha1.ProwlarrApplication
	Secrets map[string]string
}

// Proxy is an indexer proxy with its username and password, by field name.
type Proxy struct {
	indexersv1alpha1.ProwlarrProxy
	Credentials map[string]string
}

// Options holds all the inputs needed to reconcile Prowlarr.
type Options struct {
	Tags            []commonv1alpha1.Tag
	AppProfiles     []indexersv1alpha1.ProwlarrAppProfile
	Applications    []Application
	Indexers        []indexersv1alpha1.ProwlarrIndexer
	Proxies         []Proxy
	DownloadClients []indexersv1alpha1.ProwlarrDownloadClient
	Notifications   []commonv1alpha1.Notification
}

// stages are the kinds in the order they're written: every kind but app
// profiles names tags, and indexers name app profiles.
var stages = [][]string{
	{"tags", "appProfiles"},
	{"applications", "indexers", "proxies", "downloadClients", "notifications"},
}

// Reconcile writes the options to Prowlarr. A kind that names a tag or app
// profile Prowlarr lacks is left out and reported; the rest still syncs.
func Reconcile(ctx context.Context, client *engine.HTTPClient, opts Options, pass *engine.Pass) []string {
	var plannedTags []string
	if pass.Observe {
		for _, t := range opts.Tags {
			plannedTags = append(plannedTags, t.Label)
		}
	}
	return engine.Stages{
		Order: stages, Builders: opts.builders(ctx, client, pass.Observe),
		TagPath: "/api/v1/tag", PlannedTags: plannedTags,
	}.Reconcile(ctx, client, Definition(), pass)
}

func (o Options) builders(ctx context.Context, client *engine.HTTPClient, observe bool) map[string]engine.Builder {
	return map[string]engine.Builder{
		"tags": {Count: len(o.Tags), Build: func(engine.NamedIDs) ([]map[string]any, error) {
			tags := make([]map[string]any, 0, len(o.Tags))
			for _, t := range o.Tags {
				tags = append(tags, map[string]any{"label": engine.TagLabel(t.Label)})
			}
			return tags, nil
		}},
		"appProfiles": {Count: len(o.AppProfiles), Build: func(engine.NamedIDs) ([]map[string]any, error) {
			profiles := make([]map[string]any, 0, len(o.AppProfiles))
			for _, p := range o.AppProfiles {
				profiles = append(profiles, appProfilePayload(p))
			}
			return profiles, nil
		}},
		"applications": taggedKind("applications", o.Applications,
			func(a Application) (string, []string) { return a.Name, a.TagLabels },
			func(a Application, tags []int) (map[string]any, error) {
				return buildApplicationPayload(a.ProwlarrApplication, a.Secrets, tags), nil
			}),
		"indexers": taggedKind("indexers", o.Indexers,
			func(i indexersv1alpha1.ProwlarrIndexer) (string, []string) { return i.Name, i.TagLabels },
			o.indexerPayload(ctx, client, observe)),
		"proxies": taggedKind("proxies", o.Proxies,
			func(p Proxy) (string, []string) { return p.Name, p.TagLabels },
			func(p Proxy, tags []int) (map[string]any, error) {
				return buildProxyPayload(p.ProwlarrProxy, p.Credentials, tags), nil
			}),
		"downloadClients": taggedKind("downloadClients", o.DownloadClients,
			func(dc indexersv1alpha1.ProwlarrDownloadClient) (string, []string) { return dc.Name, dc.TagLabels },
			func(dc indexersv1alpha1.ProwlarrDownloadClient, tags []int) (map[string]any, error) {
				return buildDownloadClientPayload(dc, tags), nil
			}),
		"notifications": taggedKind("notifications", o.Notifications,
			func(n commonv1alpha1.Notification) (string, []string) { return n.Name, n.TagLabels },
			func(n commonv1alpha1.Notification, tags []int) (map[string]any, error) {
				return pvrclient.BuildNotificationPayload(n, tags), nil
			}),
	}
}

// taggedKind is the Builder for a kind whose items name tags. ident returns an
// item's name and tag labels, and build its payload given the tags' IDs.
func taggedKind[T any](
	kind string, items []T, ident func(T) (string, []string), build func(T, []int) (map[string]any, error),
) engine.Builder {
	return engine.Builder{
		Count: len(items),
		Tagged: slices.ContainsFunc(items, func(item T) bool {
			_, labels := ident(item)
			return len(labels) > 0
		}),
		Build: func(tags engine.NamedIDs) ([]map[string]any, error) {
			payloads := make([]map[string]any, 0, len(items))
			for _, item := range items {
				name, labels := ident(item)
				ids, err := tags.ResolveTags(labels)
				if err != nil {
					return nil, fmt.Errorf("%s(%s): %w", kind, name, err)
				}
				payload, err := build(item, ids)
				if err != nil {
					return nil, fmt.Errorf("%s(%s): %w", kind, name, err)
				}
				payloads = append(payloads, payload)
			}
			return payloads, nil
		},
	}
}

// indexerPayload builds an indexer's payload with the ID of the app profile it
// names, reading Prowlarr's profiles the first time one does. In observe mode,
// profiles the resource declares resolve even when Prowlarr lacks them; see
// engine.PlannedID.
func (o Options) indexerPayload(
	ctx context.Context, client *engine.HTTPClient, observe bool,
) func(indexersv1alpha1.ProwlarrIndexer, []int) (map[string]any, error) {
	var profiles engine.NamedIDs
	return func(idx indexersv1alpha1.ProwlarrIndexer, tags []int) (map[string]any, error) {
		if idx.AppProfileName != "" {
			if profiles == nil {
				var err error
				if profiles, err = engine.FetchNamedIDs(ctx, client, "/api/v1/appprofile", "name"); err != nil {
					return nil, err
				}
				if observe {
					for _, p := range o.AppProfiles {
						profiles.Plan(p.Name)
					}
				}
			}
			id, err := profiles.Resolve(idx.AppProfileName)
			if err != nil {
				return nil, fmt.Errorf("appProfileName: %w", err)
			}
			idx.AppProfileID = new(int32(id))
		}
		return buildIndexerPayload(idx, tags), nil
	}
}

func appProfilePayload(profile indexersv1alpha1.ProwlarrAppProfile) map[string]any {
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
	return payload
}

// buildApplicationPayload builds the API payload for a Prowlarr application.
// secrets holds the resolved apiKey, authUsername and authPassword, by field name.
func buildApplicationPayload(app indexersv1alpha1.ProwlarrApplication, secrets map[string]string, tags []int) map[string]any {
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
	setTags(payload, tags)
	return payload
}

// buildIndexerPayload builds the API payload for a Prowlarr indexer.
func buildIndexerPayload(idx indexersv1alpha1.ProwlarrIndexer, tags []int) map[string]any {
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
	setTags(desired, tags)
	if idx.AppProfileID != nil {
		desired["appProfileId"] = *idx.AppProfileID
	}
	if idx.Priority != nil {
		desired["priority"] = *idx.Priority
	}
	return desired
}

// buildProxyPayload builds the API payload for a Prowlarr proxy.
// credentials holds the resolved username and password, by field name.
func buildProxyPayload(proxy indexersv1alpha1.ProwlarrProxy, credentials map[string]string, tags []int) map[string]any {
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
	setTags(payload, tags)
	return payload
}

// buildDownloadClientPayload builds the API payload for a Prowlarr download client.
func buildDownloadClientPayload(dc indexersv1alpha1.ProwlarrDownloadClient, tags []int) map[string]any {
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
	setTags(desired, tags)

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

// setTags sets the payload's tags when the resource declares them; an empty
// list clears them.
func setTags(payload map[string]any, ids []int) {
	if ids == nil {
		return
	}
	tags := make([]any, len(ids))
	for i, id := range ids {
		tags[i] = id
	}
	payload["tags"] = tags
}
