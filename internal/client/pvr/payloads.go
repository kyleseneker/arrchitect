package pvr

import (
	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
)

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
	if dc.UseSSL != nil {
		fields = append(fields, map[string]any{"name": "useSsl", "value": *dc.UseSSL})
	}
	if dc.Category != "" {
		fields = append(fields, map[string]any{"name": categoryField, "value": dc.Category})
	}
	if dc.URLBase != "" || dc.AppRef != (commonv1alpha1.AppRef{}) {
		fields = append(fields, map[string]any{"name": "urlBase", "value": dc.URLBase})
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
		payload["tags"] = buildTagsPayload(tags)
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
		"enableRss":               idx.EnableRSS,
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
		payload["tags"] = buildTagsPayload(tags)
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
		payload["tags"] = buildTagsPayload(tags)
	}

	// Unset event switches are left out, so the merge keeps the app's defaults.
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

// buildImportListPayload builds the API payload for an import list.
// qpIDs maps quality profile names to their IDs.
func buildImportListPayload(il commonv1alpha1.ImportList, qpIDs map[string]int, names ImportListFields, tags []int) map[string]any {
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
		payload["tags"] = buildTagsPayload(tags)
	}

	if names.Enable != "" {
		SetOptionalBool(payload, names.Enable, il.Enabled)
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

// buildTagsPayload converts an int slice to an interface slice for JSON.
func buildTagsPayload(tagIDs []int) []any {
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

// buildCustomFormatPayload builds the API payload for a custom format.
func buildCustomFormatPayload(cf commonv1alpha1.CustomFormat) map[string]any {
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
