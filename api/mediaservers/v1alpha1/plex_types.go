package v1alpha1

import (
	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PlexSpec defines the desired state of Plex.
// +kubebuilder:validation:XValidation:rule="!has(self.reconcile) || !has(self.reconcile.prune) || !self.reconcile.prune",message="Plex doesn't support prune"
// +kubebuilder:validation:XValidation:rule="!has(self.reconcile) || !has(self.reconcile.deletionPolicy) || self.reconcile.deletionPolicy != 'delete'",message="Plex doesn't support deletionPolicy delete"
type PlexSpec struct {
	// connection defines how to connect to the Plex instance.
	// +required
	Connection PlexConnection `json:"connection,omitzero"`

	// reconcile configures reconciliation behavior (e.g., sync interval).
	// +optional
	Reconcile *commonv1alpha1.ReconcileConfig `json:"reconcile,omitempty"`

	// server configures Plex server settings.
	// +optional
	Server *PlexServer `json:"server,omitempty"`

	// transcoder configures transcoding and hardware acceleration.
	// +optional
	Transcoder *PlexTranscoder `json:"transcoder,omitempty"`

	// libraries defines media libraries, matched by name. An existing library
	// keeps the declared agent, scanner, language and folders: folders it
	// doesn't list are removed. Libraries are never deleted, and a library's
	// type can't change in place.
	// +optional
	// +listType=map
	// +listMapKey=name
	Libraries []PlexLibrary `json:"libraries,omitempty"`

	// network configures network settings.
	// +optional
	Network *PlexNetwork `json:"network,omitempty"`
}

// PlexConnection defines how the operator connects to Plex.
type PlexConnection struct {
	// url is the base URL of the Plex server (e.g., http://plex:32400).
	// +required
	// +kubebuilder:validation:Pattern=`^https?://`
	// +kubebuilder:validation:MinLength=1
	URL string `json:"url,omitempty"`

	// tokenSecretRef references a Secret containing the X-Plex-Token.
	// +required
	TokenSecretRef commonv1alpha1.SecretKeyRef `json:"tokenSecretRef,omitempty"`

	// tls configures TLS settings for the connection.
	// Only needed when Plex uses HTTPS with self-signed or private CA certificates.
	// +optional
	TLS *commonv1alpha1.TLSConfig `json:"tls,omitempty"`
}

// PlexServer configures Plex server settings.
type PlexServer struct {
	// friendlyName is the server's display name.
	// +optional
	// +kubebuilder:validation:MinLength=1
	FriendlyName string `json:"friendlyName,omitempty"`

	// enableRemoteAccess allows access from outside the local network.
	// +optional
	EnableRemoteAccess *bool `json:"enableRemoteAccess,omitempty"`

	// logDebug enables debug logging.
	// +optional
	LogDebug *bool `json:"logDebug,omitempty"`

	// autoEmptyTrash automatically empties the trash.
	// +optional
	AutoEmptyTrash *bool `json:"autoEmptyTrash,omitempty"`

	// scanMyLibraryAutomatically enables automatic library scanning.
	// +optional
	ScanMyLibraryAutomatically *bool `json:"scanMyLibraryAutomatically,omitempty"`

	// scanMyLibraryPeriodically enables periodic library scanning.
	// +optional
	ScanMyLibraryPeriodically *bool `json:"scanMyLibraryPeriodically,omitempty"`

	// scanMyLibraryPartially scans only the folders that changed when changes
	// are detected.
	// +optional
	ScanMyLibraryPartially *bool `json:"scanMyLibraryPartially,omitempty"`

	// libraryScanInterval is how often, in seconds, scanMyLibraryPeriodically scans.
	// +optional
	// +kubebuilder:validation:Enum=900;1800;3600;7200;21600;43200;86400
	LibraryScanInterval *int32 `json:"libraryScanInterval,omitempty"`

	// scannerLowPriority runs scans at a lower priority.
	// +optional
	ScannerLowPriority *bool `json:"scannerLowPriority,omitempty"`

	// allowMediaDeletion lets the server owner delete media from Plex apps.
	// +optional
	AllowMediaDeletion *bool `json:"allowMediaDeletion,omitempty"`

	// videoPlayedThreshold is how far, in percent, a video must play to count as watched.
	// +optional
	// +kubebuilder:validation:Enum=70;75;80;85;90;95
	VideoPlayedThreshold *int32 `json:"videoPlayedThreshold,omitempty"`

	// generateVideoPreviewThumbnails is when Plex makes the thumbnails shown
	// while seeking: never, as a scheduled task, or also as media is added.
	// +optional
	// +kubebuilder:validation:Enum=never;scheduled;asap
	GenerateVideoPreviewThumbnails string `json:"generateVideoPreviewThumbnails,omitempty"`

	// generateChapterThumbnails is when Plex makes chapter thumbnails.
	// +optional
	// +kubebuilder:validation:Enum=never;scheduled;asap
	GenerateChapterThumbnails string `json:"generateChapterThumbnails,omitempty"`

	// loudnessAnalysis is when Plex analyzes audio loudness.
	// +optional
	// +kubebuilder:validation:Enum=never;scheduled;asap
	LoudnessAnalysis string `json:"loudnessAnalysis,omitempty"`

	// butlerStartHour is the hour scheduled maintenance tasks may start.
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=23
	ButlerStartHour *int32 `json:"butlerStartHour,omitempty"`

	// butlerEndHour is the hour scheduled maintenance tasks must stop.
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=23
	ButlerEndHour *int32 `json:"butlerEndHour,omitempty"`

	// butlerTaskBackupDatabase backs up the database every three days.
	// +optional
	ButlerTaskBackupDatabase *bool `json:"butlerTaskBackupDatabase,omitempty"`

	// butlerDatabaseBackupPath is where database backups go.
	// +optional
	// +kubebuilder:validation:MinLength=1
	ButlerDatabaseBackupPath string `json:"butlerDatabaseBackupPath,omitempty"`

	// butlerTaskOptimizeDatabase optimizes the database every week.
	// +optional
	ButlerTaskOptimizeDatabase *bool `json:"butlerTaskOptimizeDatabase,omitempty"`

	// butlerTaskCleanOldBundles removes old metadata bundles every week.
	// +optional
	ButlerTaskCleanOldBundles *bool `json:"butlerTaskCleanOldBundles,omitempty"`

	// butlerTaskCleanOldCacheFiles removes old cache files every week.
	// +optional
	ButlerTaskCleanOldCacheFiles *bool `json:"butlerTaskCleanOldCacheFiles,omitempty"`

	// butlerTaskRefreshLocalMedia refreshes local metadata every three days.
	// +optional
	ButlerTaskRefreshLocalMedia *bool `json:"butlerTaskRefreshLocalMedia,omitempty"`

	// butlerTaskRefreshLibraries scans the libraries during maintenance.
	// +optional
	ButlerTaskRefreshLibraries *bool `json:"butlerTaskRefreshLibraries,omitempty"`

	// butlerTaskUpgradeMediaAnalysis upgrades media analysis during maintenance.
	// +optional
	ButlerTaskUpgradeMediaAnalysis *bool `json:"butlerTaskUpgradeMediaAnalysis,omitempty"`

	// butlerTaskRefreshPeriodicMetadata refreshes metadata periodically.
	// +optional
	ButlerTaskRefreshPeriodicMetadata *bool `json:"butlerTaskRefreshPeriodicMetadata,omitempty"`

	// butlerTaskDeepMediaAnalysis analyzes media in depth during maintenance.
	// +optional
	ButlerTaskDeepMediaAnalysis *bool `json:"butlerTaskDeepMediaAnalysis,omitempty"`

	// dlnaEnabled runs the DLNA server.
	// +optional
	DlnaEnabled *bool `json:"dlnaEnabled,omitempty"`

	// sendCrashReports sends crash reports to Plex.
	// +optional
	SendCrashReports *bool `json:"sendCrashReports,omitempty"`

	// logVerbose enables verbose logging.
	// +optional
	LogVerbose *bool `json:"logVerbose,omitempty"`
}

// PlexTranscoder configures transcoding settings.
type PlexTranscoder struct {
	// transcodeHwRequested enables hardware-accelerated transcoding.
	// +optional
	TranscodeHwRequested *bool `json:"transcodeHwRequested,omitempty"`

	// hardwareAccelerationType sets the HW acceleration type.
	// +optional
	// +kubebuilder:validation:MinLength=1
	HardwareAccelerationType string `json:"hardwareAccelerationType,omitempty"`

	// maxSimultaneousVideoTranscodes limits concurrent transcodes. 0 = unlimited.
	// +optional
	MaxSimultaneousVideoTranscodes *int32 `json:"maxSimultaneousVideoTranscodes,omitempty"`

	// transcodeHwEncodingEnabled enables hardware encoding.
	// +optional
	TranscodeHwEncodingEnabled *bool `json:"transcodeHwEncodingEnabled,omitempty"`

	// transcoderTempDirectory sets the transcoder temporary directory.
	// +optional
	// +kubebuilder:validation:MinLength=1
	TranscoderTempDirectory string `json:"transcoderTempDirectory,omitempty"`

	// transcoderQuality trades encoding speed for quality. maximumQuality is
	// what Plex calls "make my CPU hurt".
	// +optional
	// +kubebuilder:validation:Enum=automatic;preferSpeed;preferQuality;maximumQuality
	TranscoderQuality string `json:"transcoderQuality,omitempty"`

	// transcoderThrottleBuffer is how many seconds ahead of playback the
	// transcoder works before slowing down.
	// +optional
	// +kubebuilder:validation:Minimum=0
	TranscoderThrottleBuffer *int32 `json:"transcoderThrottleBuffer,omitempty"`

	// transcoderH264BackgroundPreset is the encoder preset for background
	// transcodes such as optimizing and downloads.
	// +optional
	// +kubebuilder:validation:Enum=ultrafast;superfast;veryfast;faster;fast;medium;slow;slower;veryslow
	TranscoderH264BackgroundPreset string `json:"transcoderH264BackgroundPreset,omitempty"`

	// transcoderHevcEncoding allows HEVC encoding.
	// +optional
	TranscoderHevcEncoding *bool `json:"transcoderHevcEncoding,omitempty"`

	// downloadsTempDirectory is where transcodes for downloads are kept.
	// +optional
	// +kubebuilder:validation:MinLength=1
	DownloadsTempDirectory string `json:"downloadsTempDirectory,omitempty"`
}

// PlexLibrary defines a Plex media library.
type PlexLibrary struct {
	// name is the display name of the library.
	// +required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name,omitempty"`

	// type is the library type.
	// +required
	// +kubebuilder:validation:Enum=movie;show;artist;photo
	Type string `json:"type,omitempty"`

	// paths is the list of media folder paths.
	// +required
	// +listType=set
	Paths []string `json:"paths,omitempty"`

	// language is the library language (ISO 639-1).
	// +optional
	// +kubebuilder:validation:MinLength=1
	Language string `json:"language,omitempty"`

	// scanner is the scanner type.
	// +optional
	// +kubebuilder:validation:MinLength=1
	Scanner string `json:"scanner,omitempty"`

	// agent is the metadata agent to use.
	// +optional
	// +kubebuilder:validation:MinLength=1
	Agent string `json:"agent,omitempty"`
}

// PlexNetwork configures Plex network settings.
type PlexNetwork struct {
	// secureConnections controls whether clients must, should or can't use
	// secure connections.
	// +optional
	// +kubebuilder:validation:Enum=required;preferred;disabled
	SecureConnections string `json:"secureConnections,omitempty"`

	// customServerAccessUrls is a comma-separated list of custom access URLs.
	// +optional
	// +kubebuilder:validation:MinLength=1
	CustomServerAccessUrls string `json:"customServerAccessUrls,omitempty"`

	// allowedNetworks is a comma-separated list of networks allowed without auth.
	// +optional
	// +kubebuilder:validation:MinLength=1
	AllowedNetworks string `json:"allowedNetworks,omitempty"`

	// enableIPv6 enables IPv6 support.
	// +optional
	EnableIPv6 *bool `json:"enableIPv6,omitempty"`

	// manualPortMappingMode uses manualPortMappingPort as the public port for
	// remote access instead of finding one.
	// +optional
	ManualPortMappingMode *bool `json:"manualPortMappingMode,omitempty"`

	// manualPortMappingPort is the public port for remote access.
	// +optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	ManualPortMappingPort int32 `json:"manualPortMappingPort,omitempty"`

	// relayEnabled lets clients that can't connect directly go through Plex's relay.
	// +optional
	RelayEnabled *bool `json:"relayEnabled,omitempty"`

	// gdmEnabled announces the server to clients on the local network.
	// +optional
	GdmEnabled *bool `json:"gdmEnabled,omitempty"`

	// webhooksEnabled sends events to the webhooks set up in the Plex account.
	// +optional
	WebhooksEnabled *bool `json:"webhooksEnabled,omitempty"`

	// preferredNetworkInterface is the interface Plex uses, such as eth0. An
	// empty string means any.
	// +optional
	PreferredNetworkInterface *string `json:"preferredNetworkInterface,omitempty"`
}

// PlexStatus defines the observed state of Plex.
// +kubebuilder:validation:MinProperties=1
type PlexStatus struct {
	// conditions represent the current state of the Plex resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// observedGeneration is the most recent generation observed.
	// +optional
	// +kubebuilder:validation:Minimum=1
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// lastSyncTime is the timestamp of the last successful reconciliation.
	// +optional
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// managedResources records the resources this operator created, keyed by
	// resource type. Prune only removes entries listed here, so resources
	// created outside the operator are never deleted.
	// +optional
	// +listType=map
	// +listMapKey=type
	ManagedResources []commonv1alpha1.ManagedResource `json:"managedResources,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=arrchitect
// +kubebuilder:printcolumn:name="Synced",type="string",JSONPath=`.status.conditions[?(@.type=="Synced")].status`
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Last Sync",type="date",JSONPath=`.status.lastSyncTime`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=`.metadata.creationTimestamp`

// Plex configures a Plex instance the operator connects to.
type Plex struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is the standard object metadata.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec is the desired state of the Plex.
	// +required
	Spec PlexSpec `json:"spec,omitzero"`

	// status is the observed state of the Plex.
	// +optional
	Status PlexStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// PlexList contains a list of Plex.
type PlexList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []Plex `json:"items"`
}

func (c *Plex) GetConditions() *[]metav1.Condition                  { return &c.Status.Conditions }
func (c *Plex) GetObservedGeneration() *int64                       { return &c.Status.ObservedGeneration }
func (c *Plex) GetLastSyncTime() **metav1.Time                      { return &c.Status.LastSyncTime }
func (c *Plex) GetReconcileConfig() *commonv1alpha1.ReconcileConfig { return c.Spec.Reconcile }
func (c *Plex) GetManagedResources() map[string][]string {
	return commonv1alpha1.ManagedMap(c.Status.ManagedResources)
}

func (c *Plex) SetManagedResources(m map[string][]string) {
	c.Status.ManagedResources = commonv1alpha1.ManagedList(m)
}
