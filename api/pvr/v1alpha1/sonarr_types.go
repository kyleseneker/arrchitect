package v1alpha1

import (
	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SonarrSpec is a Sonarr instance's connection and app-wide settings. Indexers
// and download clients are resources of their own that refer to it.
type SonarrSpec struct {
	// connection defines how to connect to the Sonarr instance.
	// +required
	Connection commonv1alpha1.AppConnection `json:"connection,omitempty"`

	// reconcile configures reconciliation behavior (e.g., sync interval).
	// +optional
	Reconcile *commonv1alpha1.ReconcileConfig `json:"reconcile,omitempty"`

	// mediaManagement configures media management settings.
	// +optional
	MediaManagement *SonarrMediaManagement `json:"mediaManagement,omitempty"`

	// naming configures episode and series naming formats.
	// +optional
	Naming *SonarrNaming `json:"naming,omitempty"`

	// indexerConfig configures global indexer settings.
	// +optional
	IndexerConfig *SonarrIndexerConfig `json:"indexerConfig,omitempty"`

	// downloadClientConfig configures global download client behavior.
	// +optional
	DownloadClientConfig *SonarrDownloadClientConfig `json:"downloadClientConfig,omitempty"`

	// ui configures UI display settings.
	// +optional
	UI *SonarrUI `json:"ui,omitempty"`

	// rootFolders defines media library root folder paths.
	// +optional
	// +listType=map
	// +listMapKey=path
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

	// releaseProfiles defines release profiles: terms a release must or must not
	// contain, matched by name.
	// +optional
	// +listType=map
	// +listMapKey=name
	ReleaseProfiles []commonv1alpha1.ReleaseProfile `json:"releaseProfiles,omitempty"`

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

// SonarrMediaManagement configures how Sonarr handles media files.
type SonarrMediaManagement struct {
	// copyUsingHardlinks uses hardlinks instead of copies when importing.
	// +optional
	CopyUsingHardlinks *bool `json:"copyUsingHardlinks,omitempty"`

	// deleteEmptyFolders removes empty series folders after cleanup.
	// +optional
	DeleteEmptyFolders *bool `json:"deleteEmptyFolders,omitempty"`

	// importExtraFiles imports additional files (subtitles, etc.) alongside media.
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

	// minimumFreeSpaceWhenImporting is the minimum free space in MB required to import.
	// +optional
	MinimumFreeSpaceWhenImporting *int32 `json:"minimumFreeSpaceWhenImporting,omitempty"`

	// enableMediaInfo enables media info scanning.
	// +optional
	EnableMediaInfo *bool `json:"enableMediaInfo,omitempty"`

	// rescanAfterRefresh controls when to rescan series folders.
	// +optional
	// +kubebuilder:validation:Enum=always;afterManual;never
	RescanAfterRefresh string `json:"rescanAfterRefresh,omitempty"`

	// setPermissionsLinux enables setting file permissions on import.
	// +optional
	SetPermissionsLinux *bool `json:"setPermissionsLinux,omitempty"`

	// episodeTitleRequired controls when episode titles are required.
	// +optional
	// +kubebuilder:validation:Enum=always;bulkSeasonReleases;never
	EpisodeTitleRequired string `json:"episodeTitleRequired,omitempty"`

	// skipFreeSpaceCheckWhenImporting skips free space check during import.
	// +optional
	SkipFreeSpaceCheckWhenImporting *bool `json:"skipFreeSpaceCheckWhenImporting,omitempty"`

	// createEmptySeriesFolders creates folders for series with no files.
	// +optional
	CreateEmptySeriesFolders *bool `json:"createEmptySeriesFolders,omitempty"`

	// fileDate sets an imported file's modified date: none keeps it,
	// localAirDate and utcAirDate use the episode's air date.
	// +optional
	// +kubebuilder:validation:Enum=none;localAirDate;utcAirDate
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

	// autoUnmonitorPreviouslyDownloadedEpisodes unmonitors episodes deleted from
	// disk.
	// +optional
	AutoUnmonitorPreviouslyDownloadedEpisodes *bool `json:"autoUnmonitorPreviouslyDownloadedEpisodes,omitempty"`

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

// SonarrNaming configures episode and folder naming conventions.
type SonarrNaming struct {
	// renameEpisodes enables automatic episode file renaming.
	// +optional
	RenameEpisodes *bool `json:"renameEpisodes,omitempty"`

	// replaceIllegalCharacters replaces illegal characters in filenames.
	// +optional
	ReplaceIllegalCharacters *bool `json:"replaceIllegalCharacters,omitempty"`

	// colonReplacementFormat defines how colons are replaced in filenames:
	// custom uses customColonReplacementFormat.
	// +optional
	// +kubebuilder:validation:Enum=delete;dash;spaceDash;spaceDashSpace;smart;custom
	ColonReplacementFormat string `json:"colonReplacementFormat,omitempty"`

	// customColonReplacementFormat replaces each colon when
	// colonReplacementFormat is custom.
	// +optional
	// +kubebuilder:validation:MinLength=1
	CustomColonReplacementFormat string `json:"customColonReplacementFormat,omitempty"`

	// multiEpisodeStyle defines how multi-episode files are named: extend
	// (S01E01-02-03), duplicate (S01E01.S01E02), repeat (S01E01E02E03), scene
	// (S01E01-E02-E03), range (S01E01-03) or prefixedRange (S01E01-E03).
	// +optional
	// +kubebuilder:validation:Enum=extend;duplicate;repeat;scene;range;prefixedRange
	MultiEpisodeStyle string `json:"multiEpisodeStyle,omitempty"`

	// standardEpisodeFormat is the naming format for standard episodes.
	// +optional
	// +kubebuilder:validation:MinLength=1
	StandardEpisodeFormat string `json:"standardEpisodeFormat,omitempty"`

	// dailyEpisodeFormat is the naming format for daily episodes.
	// +optional
	// +kubebuilder:validation:MinLength=1
	DailyEpisodeFormat string `json:"dailyEpisodeFormat,omitempty"`

	// animeEpisodeFormat is the naming format for anime episodes.
	// +optional
	// +kubebuilder:validation:MinLength=1
	AnimeEpisodeFormat string `json:"animeEpisodeFormat,omitempty"`

	// seriesFolderFormat is the naming format for series folders.
	// +optional
	// +kubebuilder:validation:MinLength=1
	SeriesFolderFormat string `json:"seriesFolderFormat,omitempty"`

	// seasonFolderFormat is the naming format for season folders.
	// +optional
	// +kubebuilder:validation:MinLength=1
	SeasonFolderFormat string `json:"seasonFolderFormat,omitempty"`

	// specialsFolderFormat is the naming format for specials folders.
	// +optional
	// +kubebuilder:validation:MinLength=1
	SpecialsFolderFormat string `json:"specialsFolderFormat,omitempty"`
}

// SonarrIndexerConfig configures global indexer settings.
type SonarrIndexerConfig struct {
	// minimumAge is the minimum age in minutes before downloading.
	// +optional
	MinimumAge *int32 `json:"minimumAge,omitempty"`

	// retention is the usenet retention in days. 0 means unlimited.
	// +optional
	Retention *int32 `json:"retention,omitempty"`

	// maximumSize is the maximum release size in MB. 0 means unlimited.
	// +optional
	MaximumSize *int32 `json:"maximumSize,omitempty"`

	// rssSyncInterval is the RSS sync interval in minutes. 0 disables RSS.
	// +optional
	RssSyncInterval *int32 `json:"rssSyncInterval,omitempty"`
}

// SonarrDownloadClientConfig configures global download client behavior.
type SonarrDownloadClientConfig struct {
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

// SonarrUI configures UI display settings.
type SonarrUI struct {
	// firstDayOfWeek is the first day of the week. 0=Sunday, 1=Monday.
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=1
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

	// showRelativeDates shows relative dates (today, yesterday).
	// +optional
	ShowRelativeDates *bool `json:"showRelativeDates,omitempty"`

	// theme is the UI theme.
	// +optional
	// +kubebuilder:validation:Enum=auto;dark;light
	Theme string `json:"theme,omitempty"`

	// enableColorImpairedMode enables color-impaired accessibility mode.
	// +optional
	EnableColorImpairedMode *bool `json:"enableColorImpairedMode,omitempty"`

	// uiLanguage is the UI language ID. 1=English.
	// +optional
	UILanguage *int32 `json:"uiLanguage,omitempty"`
}

// SonarrStatus defines the observed state of Sonarr.
// +kubebuilder:validation:MinProperties=1
type SonarrStatus struct {
	// conditions represent the current state of the Sonarr resource.
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

// Sonarr configures a Sonarr instance the operator connects to.
type Sonarr struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is the standard object metadata.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec is the desired state of the Sonarr.
	// +required
	Spec SonarrSpec `json:"spec,omitzero"`

	// status is the observed state of the Sonarr.
	// +optional
	Status SonarrStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// SonarrList contains a list of Sonarr.
type SonarrList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []Sonarr `json:"items"`
}

func (c *Sonarr) GetConditions() *[]metav1.Condition                  { return &c.Status.Conditions }
func (c *Sonarr) GetObservedGeneration() *int64                       { return &c.Status.ObservedGeneration }
func (c *Sonarr) GetLastSyncTime() **metav1.Time                      { return &c.Status.LastSyncTime }
func (c *Sonarr) GetReconcileConfig() *commonv1alpha1.ReconcileConfig { return c.Spec.Reconcile }
func (c *Sonarr) GetManagedResources() map[string][]string {
	return commonv1alpha1.ManagedMap(c.Status.ManagedResources)
}

func (c *Sonarr) SetManagedResources(m map[string][]string) {
	c.Status.ManagedResources = commonv1alpha1.ManagedList(m)
}
