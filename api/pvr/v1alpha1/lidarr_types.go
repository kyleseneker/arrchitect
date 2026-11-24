package v1alpha1

import (
	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// LidarrMetadataProfile defines a Lidarr metadata profile. Each list names
// what to allow, and Lidarr disallows the rest; a list left out keeps what the
// profile has, or Lidarr's defaults for a new profile.
type LidarrMetadataProfile struct {
	// name identifies the profile.
	// +required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name,omitempty"`

	// primaryAlbumTypes are the primary album types to allow, such as Album,
	// EP or Single.
	// +optional
	// +listType=set
	// +kubebuilder:validation:items:MinLength=1
	PrimaryAlbumTypes []string `json:"primaryAlbumTypes,omitempty"`

	// secondaryAlbumTypes are the secondary album types to allow, such as
	// Studio, Live or Compilation.
	// +optional
	// +listType=set
	// +kubebuilder:validation:items:MinLength=1
	SecondaryAlbumTypes []string `json:"secondaryAlbumTypes,omitempty"`

	// releaseStatuses are the release statuses to allow, such as Official.
	// +optional
	// +listType=set
	// +kubebuilder:validation:items:MinLength=1
	ReleaseStatuses []string `json:"releaseStatuses,omitempty"`
}

// LidarrSpec is a Lidarr instance's connection and app-wide settings. Indexers
// and download clients are resources of their own that refer to it.
type LidarrSpec struct {
	// connection defines how to connect to the Lidarr instance.
	// +required
	Connection commonv1alpha1.AppConnection `json:"connection,omitempty"`

	// reconcile configures reconciliation behavior (e.g., sync interval).
	// +optional
	Reconcile *commonv1alpha1.ReconcileConfig `json:"reconcile,omitempty"`

	// mediaManagement configures media management settings.
	// +optional
	MediaManagement *LidarrMediaManagement `json:"mediaManagement,omitempty"`

	// naming configures artist and track naming formats.
	// +optional
	Naming *LidarrNaming `json:"naming,omitempty"`

	// indexerConfig configures global indexer settings.
	// +optional
	IndexerConfig *LidarrIndexerConfig `json:"indexerConfig,omitempty"`

	// downloadClientConfig configures global download client behavior.
	// +optional
	DownloadClientConfig *LidarrDownloadClientConfig `json:"downloadClientConfig,omitempty"`

	// ui configures UI display settings.
	// +optional
	UI *LidarrUI `json:"ui,omitempty"`

	// metadataProvider configures the tags Lidarr writes into audio files and
	// the metadata server it uses.
	// +optional
	MetadataProvider *LidarrMetadataProvider `json:"metadataProvider,omitempty"`

	// rootFolders defines music library root folder paths.
	// +optional
	// +listType=map
	// +listMapKey=path
	// +kubebuilder:validation:XValidation:rule="self.all(f, has(f.defaultQualityProfileName) && has(f.defaultMetadataProfileName))",message="Lidarr root folders need defaultQualityProfileName and defaultMetadataProfileName"
	RootFolders []commonv1alpha1.RootFolder `json:"rootFolders,omitempty"`

	// remotePathMappings translates paths reported by remote download clients.
	// Entries are matched by host and remotePath. Only operator-created entries can be pruned.
	// +optional
	// +listType=map
	// +listMapKey=host
	// +listMapKey=remotePath
	RemotePathMappings []commonv1alpha1.RemotePathMapping `json:"remotePathMappings,omitempty"`

	// qualityProfiles defines quality profiles for controlling download quality preferences.
	// +optional
	// +listType=map
	// +listMapKey=name
	// +kubebuilder:validation:XValidation:rule="self.all(p, !has(p.language))",message="only Radarr quality profiles take a language"
	QualityProfiles []commonv1alpha1.QualityProfile `json:"qualityProfiles,omitempty"`

	// customFormats defines custom format scoring rules for release selection.
	// +optional
	// +listType=map
	// +listMapKey=name
	CustomFormats []commonv1alpha1.CustomFormat `json:"customFormats,omitempty"`

	// qualityDefinitions sets the size limits of the app's qualities. The app
	// has a fixed set of qualities, so none is created or pruned, and qualities
	// left out keep their limits.
	// +optional
	// +listType=map
	// +listMapKey=quality
	QualityDefinitions []commonv1alpha1.QualityDefinition `json:"qualityDefinitions,omitempty"`

	// metadataConsumers configures the app's metadata writers, which it keeps
	// one of per type: none is created or pruned, and consumers left out keep
	// their settings.
	// +optional
	// +listType=map
	// +listMapKey=implementation
	MetadataConsumers []commonv1alpha1.MetadataConsumer `json:"metadataConsumers,omitempty"`

	// delayProfiles defines delay profiles. The app checks tagged profiles in
	// the order listed, and the default profile, the one without tags, last.
	// Only operator-created profiles can be pruned, so the default never is.
	// +optional
	// +kubebuilder:validation:MaxItems=50
	// +kubebuilder:validation:XValidation:rule="self.filter(p, !has(p.tagLabels) || size(p.tagLabels) == 0).size() <= 1",message="only one delay profile can be without tags"
	// +listType=atomic
	DelayProfiles []commonv1alpha1.DelayProfile `json:"delayProfiles,omitempty"`

	// metadataProfiles defines metadata profiles, matched by name: which album
	// types and release statuses Lidarr considers for an artist. Root folders
	// and import lists can name a profile declared here.
	// +optional
	// +listType=map
	// +listMapKey=name
	MetadataProfiles []LidarrMetadataProfile `json:"metadataProfiles,omitempty"`

	// tags defines tags for linking resources together.
	// +optional
	// +listType=map
	// +listMapKey=label
	Tags []commonv1alpha1.Tag `json:"tags,omitempty"`

	// notifications defines notification/connection configurations.
	// +optional
	// +listType=map
	// +listMapKey=name
	Notifications []commonv1alpha1.Notification `json:"notifications,omitempty"`

	// importLists defines import list configurations for automatic content addition.
	// +optional
	// +listType=map
	// +listMapKey=name
	ImportLists []commonv1alpha1.ImportList `json:"importLists,omitempty"`
}

// LidarrMediaManagement configures how Lidarr handles media files.
type LidarrMediaManagement struct {
	// copyUsingHardlinks uses hardlinks instead of copies when importing.
	// +optional
	CopyUsingHardlinks *bool `json:"copyUsingHardlinks,omitempty"`

	// deleteEmptyFolders removes empty artist folders after cleanup.
	// +optional
	DeleteEmptyFolders *bool `json:"deleteEmptyFolders,omitempty"`

	// importExtraFiles imports additional files alongside media.
	// +optional
	ImportExtraFiles *bool `json:"importExtraFiles,omitempty"`

	// extraFileExtensions is a comma-separated list of extra file extensions to import.
	// +optional
	// +kubebuilder:validation:MinLength=1
	ExtraFileExtensions string `json:"extraFileExtensions,omitempty"`

	// downloadPropersAndRepacks controls how proper/repack releases are handled.
	// +optional
	// +kubebuilder:validation:Enum=preferAndUpgrade;doNotPrefer;doNotUpgrade
	DownloadPropersAndRepacks string `json:"downloadPropersAndRepacks,omitempty"`

	// recycleBin is the path to the recycling bin folder. An empty string
	// turns the recycling bin off.
	// +optional
	RecycleBin *string `json:"recycleBin,omitempty"`

	// recycleBinCleanupDays is how many days files stay in the recycling bin.
	// +optional
	RecycleBinCleanupDays *int32 `json:"recycleBinCleanupDays,omitempty"`

	// minimumFreeSpaceWhenImporting is the minimum free space in MB required.
	// +optional
	MinimumFreeSpaceWhenImporting *int32 `json:"minimumFreeSpaceWhenImporting,omitempty"`

	// rescanAfterRefresh controls when to rescan artist folders.
	// +optional
	// +kubebuilder:validation:Enum=always;afterManual;never
	RescanAfterRefresh string `json:"rescanAfterRefresh,omitempty"`

	// setPermissionsLinux enables setting file permissions on import.
	// +optional
	SetPermissionsLinux *bool `json:"setPermissionsLinux,omitempty"`

	// createEmptyArtistFolders creates folders for artists with no files.
	// +optional
	CreateEmptyArtistFolders *bool `json:"createEmptyArtistFolders,omitempty"`

	// fileDate sets an imported file's modified date: none keeps it,
	// albumReleaseDate uses the album's release date.
	// +optional
	// +kubebuilder:validation:Enum=none;albumReleaseDate
	FileDate string `json:"fileDate,omitempty"`

	// chmodFolder is the octal mode, such as 755, given to media folders
	// when setPermissionsLinux is on. Files get it without the execute bits.
	// +optional
	// +kubebuilder:validation:MinLength=3
	// +kubebuilder:validation:Pattern=`^[0-7]{3,4}$`
	ChmodFolder string `json:"chmodFolder,omitempty"`

	// chownGroup is the group name or gid given to imported files when
	// setPermissionsLinux is on. An empty string leaves the group alone.
	// +optional
	ChownGroup *string `json:"chownGroup,omitempty"`

	// autoUnmonitorPreviouslyDownloadedTracks unmonitors tracks deleted from
	// disk.
	// +optional
	AutoUnmonitorPreviouslyDownloadedTracks *bool `json:"autoUnmonitorPreviouslyDownloadedTracks,omitempty"`

	// watchLibraryForChanges rescans when files in a root folder change.
	// +optional
	WatchLibraryForChanges *bool `json:"watchLibraryForChanges,omitempty"`

	// allowFingerprinting matches tracks by audio fingerprint: never, for new
	// files, or for all files.
	// +optional
	// +kubebuilder:validation:Enum=never;newFiles;allFiles
	AllowFingerprinting string `json:"allowFingerprinting,omitempty"`

	// skipFreeSpaceCheckWhenImporting skips free space check during import.
	// +optional
	SkipFreeSpaceCheckWhenImporting *bool `json:"skipFreeSpaceCheckWhenImporting,omitempty"`

	// enableMediaInfo enables media info scanning.
	// +optional
	EnableMediaInfo *bool `json:"enableMediaInfo,omitempty"`

	// useScriptImport imports with the script at scriptImportPath instead of
	// copying or hardlinking.
	// +optional
	UseScriptImport *bool `json:"useScriptImport,omitempty"`

	// scriptImportPath is the script run for each import when useScriptImport
	// is on.
	// +optional
	// +kubebuilder:validation:MinLength=1
	ScriptImportPath string `json:"scriptImportPath,omitempty"`
}

// LidarrNaming configures artist and track naming conventions.
type LidarrNaming struct {
	// renameTracks enables automatic track file renaming.
	// +optional
	RenameTracks *bool `json:"renameTracks,omitempty"`

	// replaceIllegalCharacters replaces illegal characters in filenames.
	// +optional
	ReplaceIllegalCharacters *bool `json:"replaceIllegalCharacters,omitempty"`

	// standardTrackFormat is the naming format for standard tracks.
	// +optional
	// +kubebuilder:validation:MinLength=1
	StandardTrackFormat string `json:"standardTrackFormat,omitempty"`

	// multiDiscTrackFormat is the naming format for multi-disc tracks.
	// +optional
	// +kubebuilder:validation:MinLength=1
	MultiDiscTrackFormat string `json:"multiDiscTrackFormat,omitempty"`

	// artistFolderFormat is the naming format for artist folders.
	// +optional
	// +kubebuilder:validation:MinLength=1
	ArtistFolderFormat string `json:"artistFolderFormat,omitempty"`

	// colonReplacementFormat defines how colons are replaced in filenames.
	// +optional
	// +kubebuilder:validation:Enum=delete;dash;spaceDash;spaceDashSpace;smart
	ColonReplacementFormat string `json:"colonReplacementFormat,omitempty"`
}

// LidarrIndexerConfig configures global indexer settings.
type LidarrIndexerConfig struct {
	// minimumAge is the minimum age in minutes before downloading.
	// +optional
	MinimumAge *int32 `json:"minimumAge,omitempty"`

	// retention is the usenet retention in days.
	// +optional
	Retention *int32 `json:"retention,omitempty"`

	// maximumSize is the maximum release size in MB.
	// +optional
	MaximumSize *int32 `json:"maximumSize,omitempty"`

	// rssSyncInterval is the RSS sync interval in minutes.
	// +optional
	RssSyncInterval *int32 `json:"rssSyncInterval,omitempty"`
}

// LidarrDownloadClientConfig configures global download client behavior.
type LidarrDownloadClientConfig struct {
	// enableCompletedDownloadHandling auto-imports completed downloads.
	// +optional
	EnableCompletedDownloadHandling *bool `json:"enableCompletedDownloadHandling,omitempty"`

	// autoRedownloadFailed automatically searches when a download fails.
	// +optional
	AutoRedownloadFailed *bool `json:"autoRedownloadFailed,omitempty"`

	// autoRedownloadFailedFromInteractiveSearch auto-searches failed interactive searches.
	// +optional
	AutoRedownloadFailedFromInteractiveSearch *bool `json:"autoRedownloadFailedFromInteractiveSearch,omitempty"`

	// downloadClientWorkingFolders lists, separated by |, the folders download
	// clients unpack and fail into, which imports skip. The app's default is
	// _UNPACK_|_FAILED_.
	// +optional
	// +kubebuilder:validation:MinLength=1
	DownloadClientWorkingFolders string `json:"downloadClientWorkingFolders,omitempty"`
}

// LidarrUI configures UI display settings.
type LidarrUI struct {
	// firstDayOfWeek is the first day of the week. 0=Sunday, 1=Monday.
	// +optional
	FirstDayOfWeek *int32 `json:"firstDayOfWeek,omitempty"`

	// shortDateFormat is the short date display format.
	// +optional
	// +kubebuilder:validation:MinLength=1
	ShortDateFormat string `json:"shortDateFormat,omitempty"`

	// longDateFormat is the long date display format.
	// +optional
	// +kubebuilder:validation:MinLength=1
	LongDateFormat string `json:"longDateFormat,omitempty"`

	// timeFormat is the time display format.
	// +optional
	// +kubebuilder:validation:MinLength=1
	TimeFormat string `json:"timeFormat,omitempty"`

	// theme is the UI theme.
	// +optional
	// +kubebuilder:validation:Enum=auto;dark;light
	Theme string `json:"theme,omitempty"`

	// uiLanguage is the UI language ID.
	// +optional
	UILanguage *int32 `json:"uiLanguage,omitempty"`
}

// LidarrMetadataProvider configures Lidarr's metadata settings.
type LidarrMetadataProvider struct {
	// metadataSource is the URL of a metadata server Lidarr uses in place of
	// its own. Lidarr's UI doesn't show this setting.
	// +optional
	// +kubebuilder:validation:Pattern=`^https?://`
	// +kubebuilder:validation:MinLength=1
	MetadataSource string `json:"metadataSource,omitempty"`

	// writeAudioTags is when Lidarr tags audio files with metadata: no, newFiles
	// for new downloads only, allFiles when files are first imported (altering
	// existing files), or sync to keep every file in sync with MusicBrainz.
	// +optional
	// +kubebuilder:validation:Enum=no;newFiles;allFiles;sync
	WriteAudioTags string `json:"writeAudioTags,omitempty"`

	// scrubAudioTags removes existing tags from files, leaving only those
	// Lidarr adds.
	// +optional
	ScrubAudioTags *bool `json:"scrubAudioTags,omitempty"`

	// embedCoverArt embeds album art into audio files when Lidarr writes tags.
	// +optional
	EmbedCoverArt *bool `json:"embedCoverArt,omitempty"`
}

// LidarrStatus defines the observed state of Lidarr.
// +kubebuilder:validation:MinProperties=1
type LidarrStatus struct {
	// conditions represent the current state of the Lidarr resource.
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

// Lidarr configures a Lidarr instance the operator connects to.
type Lidarr struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is the standard object metadata.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec is the desired state of the Lidarr.
	// +required
	Spec LidarrSpec `json:"spec,omitzero"`

	// status is the observed state of the Lidarr.
	// +optional
	Status LidarrStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// LidarrList contains a list of Lidarr.
type LidarrList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []Lidarr `json:"items"`
}

func (c *Lidarr) GetConditions() *[]metav1.Condition                  { return &c.Status.Conditions }
func (c *Lidarr) GetObservedGeneration() *int64                       { return &c.Status.ObservedGeneration }
func (c *Lidarr) GetLastSyncTime() **metav1.Time                      { return &c.Status.LastSyncTime }
func (c *Lidarr) GetReconcileConfig() *commonv1alpha1.ReconcileConfig { return c.Spec.Reconcile }
func (c *Lidarr) GetManagedResources() map[string][]string {
	return commonv1alpha1.ManagedMap(c.Status.ManagedResources)
}

func (c *Lidarr) SetManagedResources(m map[string][]string) {
	c.Status.ManagedResources = commonv1alpha1.ManagedList(m)
}
