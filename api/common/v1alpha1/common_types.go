// +kubebuilder:object:generate=true
package v1alpha1

import (
	"encoding/json"
	"maps"
	"slices"
)

// ReconcileConfig holds common reconciliation settings shared across all config CRDs.
// +kubebuilder:validation:XValidation:rule="!has(self.deletionPolicy) || self.deletionPolicy != 'delete' || !has(self.driftPolicy) || self.driftPolicy != 'observe'",message="deletionPolicy delete conflicts with driftPolicy observe"
type ReconcileConfig struct {
	// intervalSeconds is how often the operator reconciles, in seconds.
	// Defaults to 300, and is at least 30.
	// +optional
	// +kubebuilder:validation:Minimum=30
	IntervalSeconds int32 `json:"intervalSeconds,omitempty"`

	// prune deletes resources the operator created, tracked in
	// status.managedResources, once they are removed from this spec. Resources
	// it did not create are never deleted, and a resource type with nothing
	// declared is left alone. Root folders and tags are never pruned.
	// Not every integration supports prune; one that does not rejects it with
	// InvalidConfig rather than ignoring it.
	// Default: false (only create/update, never delete).
	// +optional
	Prune *bool `json:"prune,omitempty"`

	// driftPolicy controls whether drift is corrected or only recorded.
	// "enforce" rewrites a resource whose live state differs from this CR.
	// "observe" leaves the app untouched and only counts the difference, which
	// makes drift measurable before handing the operator write authority.
	// Default: enforce.
	// +kubebuilder:validation:Enum=enforce;observe
	// +optional
	DriftPolicy string `json:"driftPolicy,omitempty"`

	// deletionPolicy controls what happens to resources in the app when this CR
	// is deleted. "orphan" leaves them in place, "delete" removes the resources
	// the operator created.
	// Default: orphan.
	// +kubebuilder:validation:Enum=orphan;delete
	// +optional
	DeletionPolicy string `json:"deletionPolicy,omitempty"`
}

// SecretKeyRef references a key within a Kubernetes Secret.
type SecretKeyRef struct {
	// name is the name of the Secret.
	// +required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name,omitempty"`

	// key is the key within the Secret.
	// +required
	// +kubebuilder:validation:MinLength=1
	Key string `json:"key,omitempty"`
}

// TLSConfig configures TLS for connections to an app's API.
// Use this when the target app uses HTTPS with a self-signed certificate or a private CA.
type TLSConfig struct {
	// insecureSkipVerify disables TLS certificate verification.
	// WARNING: This makes the connection susceptible to machine-in-the-middle attacks.
	// Only use for testing or when the target app uses a self-signed certificate and
	// providing a CA is not practical.
	// +optional
	InsecureSkipVerify *bool `json:"insecureSkipVerify,omitempty"`

	// caSecretRef references a Secret containing a PEM-encoded CA certificate bundle
	// used to verify the target app's TLS certificate.
	// The Secret must contain the CA certificate under the specified key.
	// +optional
	CASecretRef SecretKeyRef `json:"caSecretRef,omitzero"`
}

// AppConnection defines how the operator connects to an app's API.
type AppConnection struct {
	// url is the base URL of the app (e.g., http://arr-sonarr.arr.svc.cluster.local:8989).
	// +required
	// +kubebuilder:validation:Pattern=`^https?://`
	// +kubebuilder:validation:MinLength=1
	URL string `json:"url,omitempty"`

	// apiKeySecretRef references a Secret containing the API key.
	// +required
	APIKeySecretRef SecretKeyRef `json:"apiKeySecretRef,omitzero"`

	// tls configures TLS settings for the connection.
	// Only needed when the app uses HTTPS with self-signed or private CA certificates.
	// +optional
	TLS *TLSConfig `json:"tls,omitempty"`
}

// ConfigField is a named field of a provider's configuration. It sets value
// or valueFrom, not both.
// +kubebuilder:validation:MaxProperties=2
type ConfigField struct {
	// name is the field name.
	// +required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name,omitempty"`

	// value is the field value: a string, number, boolean or list.
	// +optional
	Value *FieldValue `json:"value,omitempty"`

	// valueFrom reads the field value from a source.
	// +optional
	ValueFrom ValueSource `json:"valueFrom,omitzero"`
}

// ValueSource names where a value is read from.
type ValueSource struct {
	// secretKeyRef reads the value from a key of a Secret in the resource's namespace.
	// +required
	SecretKeyRef SecretKeyRef `json:"secretKeyRef,omitzero"`
}

// FieldValue wraps a value that can be a string, int, or bool.
// +kubebuilder:validation:Type=""
// +kubebuilder:pruning:PreserveUnknownFields
type FieldValue struct {
	Raw []byte `json:"-"`
}

// MarshalJSON returns the raw JSON encoding of the value.
func (v FieldValue) MarshalJSON() ([]byte, error) {
	if v.Raw == nil {
		return []byte("null"), nil
	}
	return v.Raw, nil
}

// UnmarshalJSON stores the raw JSON value.
func (v *FieldValue) UnmarshalJSON(data []byte) error {
	if data == nil || string(data) == "null" {
		v.Raw = nil
		return nil
	}
	v.Raw = append(v.Raw[:0:0], data...)
	return nil
}

// ToInterface returns the underlying Go value (string, float64, bool, or nil).
func (v FieldValue) ToInterface() any {
	if v.Raw == nil {
		return nil
	}
	var val any
	if err := json.Unmarshal(v.Raw, &val); err != nil {
		return string(v.Raw)
	}
	return val
}

// RootFolder defines a media root folder path.
type RootFolder struct {
	// path is the filesystem path for the root folder inside the container.
	// +required
	// +kubebuilder:validation:MinLength=1
	Path string `json:"path,omitempty"`

	// name labels the root folder (Lidarr, Readarr). Defaults to the last
	// element of path.
	// +optional
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name,omitempty"`

	// defaultQualityProfileName is the quality profile for content added to
	// this folder (Lidarr, Readarr; required there). Resolved to an ID at
	// reconciliation time, after declared quality profiles are applied.
	// +optional
	// +kubebuilder:validation:MinLength=1
	DefaultQualityProfileName string `json:"defaultQualityProfileName,omitempty"`

	// defaultMetadataProfileName is the metadata profile for content added to
	// this folder (Lidarr, Readarr; required there). Resolved to an ID at
	// reconciliation time.
	// +optional
	// +kubebuilder:validation:MinLength=1
	DefaultMetadataProfileName string `json:"defaultMetadataProfileName,omitempty"`
}

// DownloadClient defines a download client connection.
// +kubebuilder:validation:XValidation:rule="has(self.appRef) ? !has(self.implementation) && !has(self.protocol) && !has(self.host) && !has(self.port) && !has(self.useSsl) && !has(self.urlBase) && !has(self.usernameSecretRef) && !has(self.passwordSecretRef) && !has(self.apiKeySecretRef) : has(self.implementation) && has(self.protocol) && has(self.host) && has(self.port)",message="set appRef, or implementation, protocol, host and port, not both"
type DownloadClient struct {
	// name is the display name of the download client.
	// +required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name,omitempty"`

	// appRef names the download client's resource, a QBittorrent or Sabnzbd,
	// whose kind and connection are used in place of implementation, protocol,
	// host, port, useSsl, urlBase and the credentials.
	// +optional
	// +kubebuilder:validation:XValidation:rule="self.kind in ['QBittorrent', 'Sabnzbd']",message="a download client refers to a QBittorrent or Sabnzbd"
	AppRef AppRef `json:"appRef,omitzero"`

	// enable controls whether this download client is active. A new download
	// client is enabled unless this is false.
	// +optional
	Enable *bool `json:"enable,omitempty"`

	// protocol is the download protocol.
	// +optional
	// +kubebuilder:validation:Enum=torrent;usenet
	Protocol string `json:"protocol,omitempty"`

	// implementation is the download client type (e.g., QBittorrent, Sabnzbd, NzbGet, Transmission, Deluge).
	// +optional
	// +kubebuilder:validation:MinLength=1
	Implementation string `json:"implementation,omitempty"`

	// configContract overrides the default config contract name.
	// Defaults to "<implementation>Settings" if not set.
	// +optional
	// +kubebuilder:validation:MinLength=1
	ConfigContract string `json:"configContract,omitempty"`

	// host is the hostname or IP of the download client.
	// +optional
	// +kubebuilder:validation:MinLength=1
	Host string `json:"host,omitempty"`

	// port is the port of the download client.
	// +optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port,omitempty"`

	// useSsl enables SSL/TLS for the download client connection.
	// +optional
	UseSsl *bool `json:"useSsl,omitempty"`

	// urlBase is the base URL path for the download client (e.g., "/qbittorrent").
	// +optional
	// +kubebuilder:validation:MinLength=1
	UrlBase string `json:"urlBase,omitempty"`

	// category is the download category to use.
	// +optional
	// +kubebuilder:validation:MinLength=1
	Category string `json:"category,omitempty"`

	// priority is the download client priority. Lower values are higher priority.
	// A new download client gets 1.
	// +optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=50
	Priority int32 `json:"priority,omitempty"`

	// removeCompletedDownloads controls whether completed downloads are removed.
	// A new download client removes them unless this is false.
	// +optional
	RemoveCompletedDownloads *bool `json:"removeCompletedDownloads,omitempty"`

	// removeFailedDownloads controls whether failed downloads are removed. A new
	// download client removes them unless this is false.
	// +optional
	RemoveFailedDownloads *bool `json:"removeFailedDownloads,omitempty"`

	// usernameSecretRef references a Secret containing the username for authentication.
	// +optional
	UsernameSecretRef SecretKeyRef `json:"usernameSecretRef,omitzero"`

	// passwordSecretRef references a Secret containing the password for authentication.
	// +optional
	PasswordSecretRef SecretKeyRef `json:"passwordSecretRef,omitzero"`

	// apiKeySecretRef references a Secret containing the API key (for Usenet clients like SABnzbd/NzbGet).
	// +optional
	APIKeySecretRef SecretKeyRef `json:"apiKeySecretRef,omitzero"`

	// tagLabels references tags by label, resolved against the app on each
	// reconciliation. A missing or ambiguous label blocks writes, and an empty
	// list clears the tags.
	// +optional
	// +listType=set
	TagLabels []string `json:"tagLabels,omitempty"`

	// fields defines additional implementation-specific configuration fields.
	// Use these for settings not covered by first-class fields (e.g., initialState, sequentialOrder).
	// +optional
	// +listType=map
	// +listMapKey=name
	Fields []ConfigField `json:"fields,omitempty"`
}

// QualityProfile defines a quality profile that controls which qualities are acceptable
// and when upgrades should occur.
type QualityProfile struct {
	// name is the display name of the quality profile.
	// +required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name,omitempty"`

	// upgradeAllowed controls whether upgrades to better qualities are allowed.
	// +optional
	UpgradeAllowed *bool `json:"upgradeAllowed,omitempty"`

	// cutoff is the quality or group name at which quality upgrades stop.
	// Must match a quality name or group name in the items list.
	// +optional
	// +kubebuilder:validation:MinLength=1
	Cutoff string `json:"cutoff,omitempty"`

	// minFormatScore is the minimum custom format score required to grab a release.
	// +optional
	MinFormatScore *int32 `json:"minFormatScore,omitempty"`

	// cutoffFormatScore is the custom format score at which upgrades stop.
	// +optional
	CutoffFormatScore *int32 `json:"cutoffFormatScore,omitempty"`

	// minUpgradeFormatScore is the minimum score improvement required for a format-based upgrade.
	// +optional
	MinUpgradeFormatScore *int32 `json:"minUpgradeFormatScore,omitempty"`

	// items defines the quality ordering and allowed states.
	// Items at the end of the list are higher priority. Each item is either
	// an individual quality (with quality set) or a quality group (with name and items set).
	// +optional
	// +listType=atomic
	Items []QualityProfileItem `json:"items,omitempty"`

	// formatItems assigns custom format scores within this profile.
	// Reference custom formats by name; IDs are resolved automatically.
	// +optional
	// +listType=map
	// +listMapKey=name
	FormatItems []QualityProfileFormatItem `json:"formatItems,omitempty"`
}

// QualityProfileItem represents a quality or quality group in a profile.
// Set quality for an individual quality, or set name+items for a group.
// +kubebuilder:validation:ExactlyOneOf=name;quality
type QualityProfileItem struct {
	// name is the group name. Set only for quality groups, not individual qualities.
	// +optional
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name,omitempty"`

	// quality identifies a specific quality level.
	// Set for individual qualities, omit for groups.
	// +optional
	Quality QualityReference `json:"quality,omitzero"`

	// allowed controls whether this quality or group is enabled in the profile.
	// +optional
	Allowed *bool `json:"allowed,omitempty"`

	// items contains the child qualities when this is a group.
	// Only set for quality groups.
	// +optional
	// +listType=atomic
	Items []QualityProfileGroupItem `json:"items,omitempty"`
}

// QualityProfileGroupItem represents an individual quality within a quality group.
type QualityProfileGroupItem struct {
	// quality identifies the quality level.
	// +required
	Quality QualityReference `json:"quality,omitzero"`

	// allowed controls whether this quality is enabled.
	// +optional
	Allowed *bool `json:"allowed,omitempty"`
}

// LocalObjectReference names another resource of a known kind in the same
// namespace.
type LocalObjectReference struct {
	// name is the referenced resource's name.
	// +required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name,omitempty"`
}

// QualityReference identifies a specific quality level by ID and name.
type QualityReference struct {
	// id is the quality ID (e.g., 7 for Bluray-1080p). IDs are app-specific,
	// and 0 is Unknown.
	// +required
	// +kubebuilder:validation:Minimum=0
	ID *int32 `json:"id,omitempty"`

	// name is the quality display name (e.g., "Bluray-1080p").
	// +optional
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name,omitempty"`
}

// QualityProfileFormatItem assigns a score to a custom format within a quality profile.
type QualityProfileFormatItem struct {
	// name is the custom format name. Must match an existing custom format.
	// +required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name,omitempty"`

	// score is the score to assign. Positive values prefer, negative values penalize.
	// +required
	Score *int32 `json:"score,omitempty"`
}

// QualityDefinition sets the size limits of one of the app's qualities. Sizes
// use the units of the app's quality settings: megabytes per minute of runtime
// in Sonarr and Radarr, kilobits per second in Lidarr.
type QualityDefinition struct {
	// quality is the quality's name in the app, such as WEBDL-1080p.
	// +required
	// +kubebuilder:validation:MinLength=1
	Quality string `json:"quality,omitempty"`

	// title is the name the app shows for the quality.
	// +optional
	// +kubebuilder:validation:MinLength=1
	Title string `json:"title,omitempty"`

	// minSize is the smallest release the app grabs at this quality.
	// +optional
	MinSize *Decimal `json:"minSize,omitempty"`

	// maxSize is the largest release the app grabs at this quality. 0 means unlimited.
	// +optional
	MaxSize *Decimal `json:"maxSize,omitempty"`

	// preferredSize is the size the app prefers among acceptable releases. 0
	// means unlimited, so the largest release wins. Readarr has no preferred size.
	// +optional
	PreferredSize *Decimal `json:"preferredSize,omitempty"`
}

// Decimal is a number that may have a fractional part, such as 17.1.
// +kubebuilder:validation:Type=number
// +kubebuilder:validation:Minimum=0
type Decimal string

// MarshalJSON writes the number as a JSON number.
func (d Decimal) MarshalJSON() ([]byte, error) {
	return json.Marshal(json.Number(d))
}

// UnmarshalJSON reads a JSON number.
func (d *Decimal) UnmarshalJSON(data []byte) error {
	var n json.Number
	if err := json.Unmarshal(data, &n); err != nil {
		return err
	}
	*d = Decimal(n)
	return nil
}

// DelayProfile holds a release back for a while, so a better one can turn up
// before the app grabs it. A profile is matched to the app's by its tags, and
// the app allows a tag in only one profile.
type DelayProfile struct {
	// tagLabels limits the profile to items with one of these tags. The profile
	// without tags is the app's default, which covers everything the others don't.
	// +optional
	// +listType=set
	TagLabels []string `json:"tagLabels,omitempty"`

	// enableUsenet allows Usenet releases.
	// +optional
	EnableUsenet *bool `json:"enableUsenet,omitempty"`

	// enableTorrent allows torrent releases.
	// +optional
	EnableTorrent *bool `json:"enableTorrent,omitempty"`

	// preferredProtocol is the protocol the app picks when a release is on both.
	// +optional
	// +kubebuilder:validation:Enum=usenet;torrent
	PreferredProtocol string `json:"preferredProtocol,omitempty"`

	// usenetDelay is how many minutes the app waits before grabbing a Usenet release.
	// +optional
	// +kubebuilder:validation:Minimum=0
	UsenetDelay *int32 `json:"usenetDelay,omitempty"`

	// torrentDelay is how many minutes the app waits before grabbing a torrent release.
	// +optional
	// +kubebuilder:validation:Minimum=0
	TorrentDelay *int32 `json:"torrentDelay,omitempty"`

	// bypassIfHighestQuality skips the delay for a release at the quality
	// profile's highest allowed quality.
	// +optional
	BypassIfHighestQuality *bool `json:"bypassIfHighestQuality,omitempty"`

	// bypassIfAboveCustomFormatScore skips the delay for a release whose custom
	// format score reaches minimumCustomFormatScore.
	// +optional
	BypassIfAboveCustomFormatScore *bool `json:"bypassIfAboveCustomFormatScore,omitempty"`

	// minimumCustomFormatScore is the score bypassIfAboveCustomFormatScore needs.
	// +optional
	MinimumCustomFormatScore *int32 `json:"minimumCustomFormatScore,omitempty"`
}

// CustomFormat defines a custom format with matching specifications.
// Custom formats score releases based on matching conditions and are used
// within quality profiles to guide upgrade decisions.
type CustomFormat struct {
	// name is the display name of the custom format.
	// +required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name,omitempty"`

	// includeCustomFormatWhenRenaming includes this format's name in renamed filenames
	// via the {Custom Formats} naming token.
	// +optional
	IncludeCustomFormatWhenRenaming *bool `json:"includeCustomFormatWhenRenaming,omitempty"`

	// specifications defines the matching conditions for this custom format.
	// A release matches if all required specs match and at least one non-required spec matches.
	// +optional
	// +listType=map
	// +listMapKey=name
	Specifications []CustomFormatSpecification `json:"specifications,omitempty"`
}

// CustomFormatSpecification defines a single matching condition within a custom format.
type CustomFormatSpecification struct {
	// name is a human-readable label for this condition.
	// +required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name,omitempty"`

	// implementation is the specification type.
	// Common values: ReleaseTitleSpecification, SourceSpecification, ResolutionSpecification,
	// QualityModifierSpecification, LanguageSpecification, ReleaseGroupSpecification,
	// EditionSpecification (Radarr), ReleaseTypeSpecification (Sonarr), SizeSpecification,
	// IndexerFlagSpecification.
	// +required
	// +kubebuilder:validation:MinLength=1
	Implementation string `json:"implementation,omitempty"`

	// negate inverts the match logic. If true, the spec matches when the condition is NOT met.
	// +optional
	Negate *bool `json:"negate,omitempty"`

	// required makes this spec mandatory. If true, the custom format only matches when
	// this spec matches. If false, this spec participates in OR logic with other non-required specs.
	// +optional
	Required *bool `json:"required,omitempty"`

	// fields defines the specification's configuration values.
	// Most specs have a single field named "value" (e.g., a regex pattern or enum int).
	// SizeSpecification has "min" and "max" fields (floats in GB).
	// +optional
	// +listType=map
	// +listMapKey=name
	Fields []ConfigField `json:"fields,omitempty"`
}

// Tag defines a tag that can be used to link resources together
// (download clients, indexers, notifications, etc.).
type Tag struct {
	// label is the tag name.
	// +required
	// +kubebuilder:validation:MinLength=1
	Label string `json:"label,omitempty"`
}

// Indexer defines an indexer in Sonarr, Radarr, Lidarr or Readarr.
type Indexer struct {
	// name is the display name of the indexer.
	// +required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name,omitempty"`

	// enable controls whether this indexer is active. False turns off RSS,
	// automatic and interactive search; true turns on those not set below.
	// +optional
	Enable *bool `json:"enable,omitempty"`

	// protocol is the indexer protocol.
	// +required
	// +kubebuilder:validation:Enum=torrent;usenet
	Protocol string `json:"protocol,omitempty"`

	// implementation is the indexer type (e.g., Newznab, Torznab).
	// +required
	// +kubebuilder:validation:MinLength=1
	Implementation string `json:"implementation,omitempty"`

	// configContract is the configuration contract name for the implementation.
	// Defaults to "<implementation>Settings" if not set.
	// +optional
	// +kubebuilder:validation:MinLength=1
	ConfigContract string `json:"configContract,omitempty"`

	// priority is the indexer priority. Lower values are higher priority.
	// +optional
	// +kubebuilder:validation:Minimum=1
	Priority int32 `json:"priority,omitempty"`

	// enableRss enables RSS feed polling for this indexer. A new indexer
	// polls unless this or enable is false.
	// +optional
	EnableRss *bool `json:"enableRss,omitempty"`

	// enableAutomaticSearch enables automatic search for this indexer. A new
	// indexer searches unless this or enable is false.
	// +optional
	EnableAutomaticSearch *bool `json:"enableAutomaticSearch,omitempty"`

	// enableInteractiveSearch enables interactive (manual) search for this
	// indexer. A new indexer allows it unless this or enable is false.
	// +optional
	EnableInteractiveSearch *bool `json:"enableInteractiveSearch,omitempty"`

	// downloadClientName names the download client that gets this indexer's
	// releases, resolved against the app on each reconciliation. Left out, the
	// app picks a client for the release's protocol.
	// +optional
	// +kubebuilder:validation:MinLength=1
	DownloadClientName string `json:"downloadClientName,omitempty"`

	// tagLabels references tags by label, resolved against the app on each
	// reconciliation. A missing or ambiguous label blocks writes, and an empty
	// list clears the tags.
	// +optional
	// +listType=set
	TagLabels []string `json:"tagLabels,omitempty"`

	// fields defines the indexer's configuration fields (e.g., baseUrl, apiKey, categories).
	// +optional
	// +listType=map
	// +listMapKey=name
	Fields []ConfigField `json:"fields,omitempty"`
}

// ReleaseProfile defines a release profile in Sonarr or Radarr: terms a
// release must or must not contain.
// +kubebuilder:validation:XValidation:rule="has(self.required) || has(self.ignored)",message="set required, ignored or both"
type ReleaseProfile struct {
	// name identifies the profile.
	// +required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name,omitempty"`

	// enabled turns the profile on. A new profile is enabled unless this is
	// false.
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// required are terms a release must contain at least one of. A term between
	// slashes is a regular expression.
	// +optional
	// +listType=set
	// +kubebuilder:validation:items:MinLength=1
	Required []string `json:"required,omitempty"`

	// ignored are terms a release must not contain. A term between slashes is a
	// regular expression.
	// +optional
	// +listType=set
	// +kubebuilder:validation:items:MinLength=1
	Ignored []string `json:"ignored,omitempty"`

	// indexerName limits the profile to releases from one indexer, resolved by
	// name against the app on each reconciliation. Left out, the profile applies
	// to every indexer.
	// +optional
	// +kubebuilder:validation:MinLength=1
	IndexerName string `json:"indexerName,omitempty"`

	// tagLabels references tags by label, resolved against the app on each
	// reconciliation; the profile applies to series or movies with any of them.
	// A missing or ambiguous label blocks writes, and an empty list clears the
	// tags.
	// +optional
	// +listType=set
	TagLabels []string `json:"tagLabels,omitempty"`
}

// Notification defines a notification (a "Connect" entry) in Sonarr, Radarr, Lidarr, Readarr or Prowlarr.
type Notification struct {
	// name is the display name of the notification.
	// +required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name,omitempty"`

	// implementation is the notification type (e.g., Discord, Slack, Webhook, Email).
	// +required
	// +kubebuilder:validation:MinLength=1
	Implementation string `json:"implementation,omitempty"`

	// configContract is the configuration contract name for the implementation.
	// Defaults to "<implementation>Settings" if not set.
	// +optional
	// +kubebuilder:validation:MinLength=1
	ConfigContract string `json:"configContract,omitempty"`

	// tagLabels references tags by label, resolved against the app on each
	// reconciliation. A missing or ambiguous label blocks writes, and an empty
	// list clears the tags.
	// +optional
	// +listType=set
	TagLabels []string `json:"tagLabels,omitempty"`

	// fields defines the notification's configuration fields (e.g., webhook URL, API token).
	// +optional
	// +listType=map
	// +listMapKey=name
	Fields []ConfigField `json:"fields,omitempty"`

	// Trigger conditions — set to true to fire on these events.
	// Available triggers vary by app; unknown triggers are ignored.

	// onGrab fires when a release is grabbed (sent to download client).
	// +optional
	OnGrab *bool `json:"onGrab,omitempty"`

	// onDownload fires when a download is imported.
	// +optional
	OnDownload *bool `json:"onDownload,omitempty"`

	// onUpgrade fires when an existing file is upgraded.
	// +optional
	OnUpgrade *bool `json:"onUpgrade,omitempty"`

	// onRename fires when files are renamed.
	// +optional
	OnRename *bool `json:"onRename,omitempty"`

	// onHealthIssue fires when a health check fails.
	// +optional
	OnHealthIssue *bool `json:"onHealthIssue,omitempty"`

	// onHealthRestored fires when a health check recovers.
	// +optional
	OnHealthRestored *bool `json:"onHealthRestored,omitempty"`

	// onApplicationUpdate fires when the application is updated.
	// +optional
	OnApplicationUpdate *bool `json:"onApplicationUpdate,omitempty"`

	// onManualInteractionRequired fires when manual interaction is needed.
	// +optional
	OnManualInteractionRequired *bool `json:"onManualInteractionRequired,omitempty"`

	// includeHealthWarnings includes health warnings (not just errors) in notifications.
	// +optional
	IncludeHealthWarnings *bool `json:"includeHealthWarnings,omitempty"`

	// Sonarr-specific triggers

	// onSeriesAdd fires when a new series is added (Sonarr).
	// +optional
	OnSeriesAdd *bool `json:"onSeriesAdd,omitempty"`

	// onSeriesDelete fires when a series is deleted (Sonarr).
	// +optional
	OnSeriesDelete *bool `json:"onSeriesDelete,omitempty"`

	// onEpisodeFileDelete fires when an episode file is deleted (Sonarr).
	// +optional
	OnEpisodeFileDelete *bool `json:"onEpisodeFileDelete,omitempty"`

	// onEpisodeFileDeleteForUpgrade fires when an episode file is deleted for upgrade (Sonarr).
	// +optional
	OnEpisodeFileDeleteForUpgrade *bool `json:"onEpisodeFileDeleteForUpgrade,omitempty"`

	// Radarr-specific triggers

	// onMovieAdded fires when a new movie is added (Radarr).
	// +optional
	OnMovieAdded *bool `json:"onMovieAdded,omitempty"`

	// onMovieDelete fires when a movie is deleted (Radarr).
	// +optional
	OnMovieDelete *bool `json:"onMovieDelete,omitempty"`

	// onMovieFileDelete fires when a movie file is deleted (Radarr).
	// +optional
	OnMovieFileDelete *bool `json:"onMovieFileDelete,omitempty"`

	// onMovieFileDeleteForUpgrade fires when a movie file is deleted for upgrade (Radarr).
	// +optional
	OnMovieFileDeleteForUpgrade *bool `json:"onMovieFileDeleteForUpgrade,omitempty"`

	// onImportComplete fires when every file of a download has been imported (Sonarr).
	// +optional
	OnImportComplete *bool `json:"onImportComplete,omitempty"`

	// onReleaseImport fires when a release is imported (Lidarr, Readarr; Sonarr
	// and Radarr call this onDownload).
	// +optional
	OnReleaseImport *bool `json:"onReleaseImport,omitempty"`

	// onDownloadFailure fires when a download fails (Lidarr, Readarr).
	// +optional
	OnDownloadFailure *bool `json:"onDownloadFailure,omitempty"`

	// onImportFailure fires when an import fails (Lidarr, Readarr).
	// +optional
	OnImportFailure *bool `json:"onImportFailure,omitempty"`

	// onArtistAdd fires when an artist is added (Lidarr).
	// +optional
	OnArtistAdd *bool `json:"onArtistAdd,omitempty"`

	// onArtistDelete fires when an artist is deleted (Lidarr).
	// +optional
	OnArtistDelete *bool `json:"onArtistDelete,omitempty"`

	// onAlbumDelete fires when an album is deleted (Lidarr).
	// +optional
	OnAlbumDelete *bool `json:"onAlbumDelete,omitempty"`

	// onTrackRetag fires when track tags are rewritten (Lidarr).
	// +optional
	OnTrackRetag *bool `json:"onTrackRetag,omitempty"`

	// onAuthorAdded fires when an author is added (Readarr).
	// +optional
	OnAuthorAdded *bool `json:"onAuthorAdded,omitempty"`

	// onAuthorDelete fires when an author is deleted (Readarr).
	// +optional
	OnAuthorDelete *bool `json:"onAuthorDelete,omitempty"`

	// onBookDelete fires when a book is deleted (Readarr).
	// +optional
	OnBookDelete *bool `json:"onBookDelete,omitempty"`

	// onBookFileDelete fires when a book file is deleted (Readarr).
	// +optional
	OnBookFileDelete *bool `json:"onBookFileDelete,omitempty"`

	// onBookFileDeleteForUpgrade fires when a book file is deleted for an upgrade (Readarr).
	// +optional
	OnBookFileDeleteForUpgrade *bool `json:"onBookFileDeleteForUpgrade,omitempty"`

	// onBookRetag fires when book file tags are rewritten (Readarr).
	// +optional
	OnBookRetag *bool `json:"onBookRetag,omitempty"`
}

// ImportList defines an import list that adds content to Sonarr, Radarr, Lidarr or Readarr.
type ImportList struct {
	// name is the display name of the import list.
	// +required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name,omitempty"`

	// enable controls whether this import list is active. A new import list is
	// enabled unless this is false.
	// +optional
	Enable *bool `json:"enable,omitempty"`

	// enableAutomaticAdd automatically adds items from this list.
	// +optional
	EnableAutomaticAdd *bool `json:"enableAutomaticAdd,omitempty"`

	// implementation is the import list type (e.g., TraktListImport, IMDbListImport, SonarrImport).
	// +required
	// +kubebuilder:validation:MinLength=1
	Implementation string `json:"implementation,omitempty"`

	// configContract is the configuration contract name for the implementation.
	// Defaults to "<implementation>Settings" if not set.
	// +optional
	// +kubebuilder:validation:MinLength=1
	ConfigContract string `json:"configContract,omitempty"`

	// monitor controls how imported items are monitored.
	// Values vary by app (Sonarr: all, future, missing, existing, firstSeason, lastSeason, latestSeason,
	// pilot, recent, monitorSpecials, unmonitorSpecials, none; Radarr: movieOnly, movieAndCollection, none;
	// Lidarr: entireArtist, specificAlbum, none; Readarr: entireAuthor, specificBook, none).
	// +optional
	// +kubebuilder:validation:MinLength=1
	Monitor string `json:"monitor,omitempty"`

	// qualityProfileName is the name of the quality profile to assign to imported items.
	// Must match an existing quality profile. Resolved to an ID at reconciliation time.
	// +optional
	// +kubebuilder:validation:MinLength=1
	QualityProfileName string `json:"qualityProfileName,omitempty"`

	// metadataProfileName is the metadata profile for imported items (Lidarr,
	// Readarr). Resolved to an ID at reconciliation time.
	// +optional
	// +kubebuilder:validation:MinLength=1
	MetadataProfileName string `json:"metadataProfileName,omitempty"`

	// rootFolderPath is the root folder path for imported items.
	// +optional
	// +kubebuilder:validation:MinLength=1
	RootFolderPath string `json:"rootFolderPath,omitempty"`

	// seriesType is the series type for imported items (Sonarr: standard, daily, anime).
	// +optional
	// +kubebuilder:validation:MinLength=1
	SeriesType string `json:"seriesType,omitempty"`

	// seasonFolder enables season folders for imported items (Sonarr).
	// +optional
	SeasonFolder *bool `json:"seasonFolder,omitempty"`

	// minimumAvailability controls when imported movies are considered available (Radarr: announced, inCinemas, released).
	// +optional
	// +kubebuilder:validation:MinLength=1
	MinimumAvailability string `json:"minimumAvailability,omitempty"`

	// searchOnAdd searches for what the list adds (Sonarr's
	// searchForMissingEpisodes, Lidarr's and Readarr's shouldSearch).
	// +optional
	SearchOnAdd *bool `json:"searchOnAdd,omitempty"`
	// monitorNewItems monitors seasons, albums or books added later to what the
	// list added: all or none, or in Lidarr and Readarr new. Sonarr, Lidarr and
	// Readarr only.
	// +optional
	// +kubebuilder:validation:Enum=all;none;new
	MonitorNewItems string `json:"monitorNewItems,omitempty"`
	// monitorExisting also applies monitor to artists or authors already in the
	// library (Lidarr's and Readarr's shouldMonitorExisting).
	// +optional
	MonitorExisting *bool `json:"monitorExisting,omitempty"`

	// listOrder controls the display order of this list.
	// +optional
	ListOrder *int32 `json:"listOrder,omitempty"`

	// tagLabels references tags by label, resolved against the app on each
	// reconciliation. A missing or ambiguous label blocks writes, and an empty
	// list clears the tags.
	// +optional
	// +listType=set
	TagLabels []string `json:"tagLabels,omitempty"`

	// fields defines the import list's configuration fields (e.g., API URL, access token, list ID).
	// +optional
	// +listType=map
	// +listMapKey=name
	Fields []ConfigField `json:"fields,omitempty"`
}

// RemotePathMapping translates a download client's path into a path visible to the app.
// Identity is the pair (host, remotePath); localPath can be updated in place.
type RemotePathMapping struct {
	// host must match the download client's configured hostname.
	// +required
	// +kubebuilder:validation:MinLength=1
	Host string `json:"host,omitempty"`

	// remotePath is the directory reported by the download client.
	// +required
	// +kubebuilder:validation:MinLength=1
	RemotePath string `json:"remotePath,omitempty"`

	// localPath is the existing directory as seen by the app.
	// +required
	// +kubebuilder:validation:MinLength=1
	LocalPath string `json:"localPath,omitempty"`
}

// ManagedResource lists the items of one type the operator created.
type ManagedResource struct {
	// type is the kind of item, such as downloadClients.
	// +required
	// +kubebuilder:validation:MinLength=1
	Type string `json:"type,omitempty"`

	// names are the items of that type the operator created.
	// +required
	// +listType=set
	Names []string `json:"names,omitempty"`
}

// ManagedMap returns managed resources keyed by type.
func ManagedMap(list []ManagedResource) map[string][]string {
	if len(list) == 0 {
		return nil
	}
	out := make(map[string][]string, len(list))
	for _, r := range list {
		out[r.Type] = slices.Clone(r.Names)
	}
	return out
}

// ManagedList returns managed resources as a list ordered by type, leaving out
// types with no names.
func ManagedList(m map[string][]string) []ManagedResource {
	var out []ManagedResource
	for _, t := range slices.Sorted(maps.Keys(m)) {
		if len(m[t]) > 0 {
			out = append(out, ManagedResource{Type: t, Names: slices.Clone(m[t])})
		}
	}
	return out
}

// AppRef names another app's resource in the same namespace, whose connection
// is used in place of an address and credentials.
type AppRef struct {
	// group is the resource's API group.
	// +optional
	// +kubebuilder:default=arrchitect.dev
	// +kubebuilder:validation:Enum=arrchitect.dev
	Group string `json:"group,omitempty"`

	// kind is the resource's kind, such as Sonarr.
	// +required
	// +kubebuilder:validation:Enum=Sonarr;Radarr;Lidarr;Readarr;Prowlarr;Seerr;Jellyfin;Plex;QBittorrent;Sabnzbd
	Kind string `json:"kind,omitempty"`

	// name is the resource's name.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`
	Name string `json:"name,omitempty"`
}
