// Package provider builds the payloads of the providers the PVR apps and
// Prowlarr share, such as notifications and the free-form fields of indexers
// and download clients, and reads the schema listing those apps serve for
// each provider kind.
package provider

import (
	"strings"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
)

// NotificationPayload builds the API payload for a notification.
func NotificationPayload(n commonv1alpha1.Notification, tags []int) map[string]any {
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
		payload["fields"] = FieldsPayload(n.Fields)
	}
	if tags != nil {
		payload["tags"] = TagsPayload(tags)
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

// FieldsPayload converts ConfigField slice to API fields format.
func FieldsPayload(dcFields []commonv1alpha1.ConfigField) []map[string]any {
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

// TagsPayload converts an int slice to an interface slice for JSON.
func TagsPayload(tagIDs []int) []any {
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

// SchemaFieldNames returns the field names an implementation accepts, taken from
// the app's own /schema listing. The second return is false when the schema does
// not describe the implementation, in which case callers must not validate.
func SchemaFieldNames(schema []map[string]any, implementation string) (map[string]bool, bool) {
	for _, def := range schema {
		impl, _ := def["implementation"].(string)
		if !strings.EqualFold(impl, implementation) {
			continue
		}
		raw, ok := def["fields"].([]any)
		if !ok {
			return nil, false
		}
		names := make(map[string]bool, len(raw))
		for _, f := range raw {
			fm, ok := f.(map[string]any)
			if !ok {
				continue
			}
			if n, ok := fm["name"].(string); ok {
				names[n] = true
			}
		}
		return names, len(names) > 0
	}
	return nil, false
}
