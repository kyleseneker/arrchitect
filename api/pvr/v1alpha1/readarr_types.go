package v1alpha1

import (
	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ReadarrMetadataProfile defines a Readarr metadata profile. A setting left
// out keeps what the profile has, or Readarr's default for a new profile.
type ReadarrMetadataProfile struct {
	// name identifies the profile.
	// +required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name,omitempty"`

	// minPopularity skips books whose popularity, ratings times rating count,
	// is below it. Readarr's own None profile sets 10000000000.
	// +optional
	// +kubebuilder:validation:Minimum=0
	MinPopularity *int64 `json:"minPopularity,omitempty"`

	// skipMissingDate skips books without a release date.
	// +optional
	SkipMissingDate *bool `json:"skipMissingDate,omitempty"`

	// skipMissingIsbn skips books without an ISBN or ASIN.
	// +optional
	SkipMissingIsbn *bool `json:"skipMissingIsbn,omitempty"`

	// skipPartsAndSets skips books that are parts of a book or sets of books.
	// +optional
	SkipPartsAndSets *bool `json:"skipPartsAndSets,omitempty"`

	// skipSeriesSecondary skips secondary books in a series, such as companions.
	// +optional
	SkipSeriesSecondary *bool `json:"skipSeriesSecondary,omitempty"`

	// allowedLanguages are the ISO 639-3 codes of the editions to allow, such
	// as eng; "null" allows editions without a language. An empty list allows
	// every language.
	// +optional
	// +listType=set
	// +kubebuilder:validation:items:MinLength=1
	AllowedLanguages []string `json:"allowedLanguages,omitempty"`

	// minPages skips editions with fewer pages.
	// +optional
	// +kubebuilder:validation:Minimum=0
	MinPages *int32 `json:"minPages,omitempty"`

	// ignored are terms that skip a book whose title contains one.
	// +optional
	// +listType=set
	// +kubebuilder:validation:items:MinLength=1
	Ignored []string `json:"ignored,omitempty"`
}

// ReadarrSpec is a Readarr instance's connection and app-wide settings. Indexers
// and download clients are resources of their own that refer to it.
type ReadarrSpec struct {
	// connection defines how to connect to the Readarr instance.
	// +required
	Connection commonv1alpha1.AppConnection `json:"connection,omitempty"`

	// reconcile configures reconciliation behavior (e.g., sync interval).
	// +optional
	Reconcile *commonv1alpha1.ReconcileConfig `json:"reconcile,omitempty"`

	// mediaManagement configures media management settings.
	// +optional
	MediaManagement *ReadarrMediaManagement `json:"mediaManagement,omitempty"`

	// naming configures author and book naming formats.
	// +optional
	Naming *ReadarrNaming `json:"naming,omitempty"`

	// indexerConfig configures global indexer settings.
	// +optional
	IndexerConfig *ReadarrIndexerConfig `json:"indexerConfig,omitempty"`

	// downloadClientConfig configures global download client behavior.
	// +optional
	DownloadClientConfig *ReadarrDownloadClientConfig `json:"downloadClientConfig,omitempty"`

	// ui configures UI display settings.
	// +optional
	UI *ReadarrUI `json:"ui,omitempty"`

	// metadataProvider configures the metadata Readarr sends to Calibre and
	// writes into audio files.
	// +optional
	MetadataProvider *ReadarrMetadataProvider `json:"metadataProvider,omitempty"`

	// development holds the settings on Readarr's hidden development page.
	// +optional
	Development ReadarrDevelopment `json:"development,omitzero"`

	// rootFolders defines book library root folder paths.
	// +optional
	// +listType=map
	// +listMapKey=path
	// +kubebuilder:validation:XValidation:rule="self.all(f, has(f.defaultQualityProfileName) && has(f.defaultMetadataProfileName))",message="Readarr root folders need defaultQualityProfileName and defaultMetadataProfileName"
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

	// delayProfiles defines delay profiles. The app checks tagged profiles in
	// the order listed, and the default profile, the one without tags, last.
	// Only operator-created profiles can be pruned, so the default never is.
	// +optional
	// +kubebuilder:validation:MaxItems=50
	// +kubebuilder:validation:XValidation:rule="self.filter(p, !has(p.tagLabels) || size(p.tagLabels) == 0).size() <= 1",message="only one delay profile can be without tags"
	// +listType=atomic
	DelayProfiles []commonv1alpha1.DelayProfile `json:"delayProfiles,omitempty"`

	// metadataProfiles defines metadata profiles, matched by name: which books
	// Readarr considers for an author. Root folders and import lists can name a
	// profile declared here.
	// +optional
	// +listType=map
	// +listMapKey=name
	MetadataProfiles []ReadarrMetadataProfile `json:"metadataProfiles,omitempty"`

	// tags defines tags for linking resources together.
	// +optional
	// +listType=map
	// +listMapKey=label
	Tags []commonv1alpha1.Tag `json:"tags,omitempty"`

	// notifications defines notification/connection configurations.
	// +optional
	// +listType=map
	// +listMapKey=name
	// +kubebuilder:validation:XValidation:rule="self.all(n, !has(n.appRef) || n.appRef.kind == 'Plex')",message="Readarr has no Jellyfin notification"
	Notifications []commonv1alpha1.Notification `json:"notifications,omitempty"`

	// importLists defines import list configurations for automatic content addition.
	// +optional
	// +listType=map
	// +listMapKey=name
	ImportLists []commonv1alpha1.ImportList `json:"importLists,omitempty"`
}

// ReadarrMediaManagement configures how Readarr handles media files.
type ReadarrMediaManagement struct {
	// copyUsingHardlinks uses hardlinks instead of copies when importing.
	// +optional
	CopyUsingHardlinks *bool `json:"copyUsingHardlinks,omitempty"`

	// deleteEmptyFolders removes empty author folders after cleanup.
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

	// rescanAfterRefresh controls when to rescan author folders.
	// +optional
	// +kubebuilder:validation:Enum=always;afterManual;never
	RescanAfterRefresh string `json:"rescanAfterRefresh,omitempty"`

	// setPermissionsLinux enables setting file permissions on import.
	// +optional
	SetPermissionsLinux *bool `json:"setPermissionsLinux,omitempty"`

	// createEmptyAuthorFolders creates folders for authors with no files.
	// +optional
	CreateEmptyAuthorFolders *bool `json:"createEmptyAuthorFolders,omitempty"`

	// fileDate sets an imported file's modified date: none keeps it,
	// bookReleaseDate uses the book's release date.
	// +optional
	// +kubebuilder:validation:Enum=none;bookReleaseDate
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

	// autoUnmonitorPreviouslyDownloadedBooks unmonitors books deleted from
	// disk.
	// +optional
	AutoUnmonitorPreviouslyDownloadedBooks *bool `json:"autoUnmonitorPreviouslyDownloadedBooks,omitempty"`

	// watchLibraryForChanges rescans when files in a root folder change.
	// +optional
	WatchLibraryForChanges *bool `json:"watchLibraryForChanges,omitempty"`

	// allowFingerprinting matches books by fingerprint: never, for new
	// files, or for all files.
	// +optional
	// +kubebuilder:validation:Enum=never;newFiles;allFiles
	AllowFingerprinting string `json:"allowFingerprinting,omitempty"`

	// skipFreeSpaceCheckWhenImporting skips free space check during import.
	// +optional
	SkipFreeSpaceCheckWhenImporting *bool `json:"skipFreeSpaceCheckWhenImporting,omitempty"`
}

// ReadarrNaming configures author and book naming conventions.
type ReadarrNaming struct {
	// renameBooks enables automatic book file renaming.
	// +optional
	RenameBooks *bool `json:"renameBooks,omitempty"`

	// replaceIllegalCharacters replaces illegal characters in filenames.
	// +optional
	ReplaceIllegalCharacters *bool `json:"replaceIllegalCharacters,omitempty"`

	// standardBookFormat is the naming format for book files.
	// +optional
	// +kubebuilder:validation:MinLength=1
	StandardBookFormat string `json:"standardBookFormat,omitempty"`

	// authorFolderFormat is the naming format for author folders.
	// +optional
	// +kubebuilder:validation:MinLength=1
	AuthorFolderFormat string `json:"authorFolderFormat,omitempty"`

	// colonReplacementFormat defines how colons are replaced in filenames.
	// +optional
	// +kubebuilder:validation:Enum=delete;dash;spaceDash;spaceDashSpace;smart
	ColonReplacementFormat string `json:"colonReplacementFormat,omitempty"`
}

// ReadarrIndexerConfig configures global indexer settings.
type ReadarrIndexerConfig struct {
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

// ReadarrDownloadClientConfig configures global download client behavior.
type ReadarrDownloadClientConfig struct {
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

// ReadarrUI configures UI display settings.
type ReadarrUI struct {
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

// ReadarrMetadataProvider configures Readarr's metadata settings.
type ReadarrMetadataProvider struct {
	// writeBookTags is when Readarr sends metadata to Calibre: newFiles for new
	// downloads only, allFiles when files are first imported (altering existing
	// files), or sync to keep every file in sync.
	// +optional
	// +kubebuilder:validation:Enum=newFiles;allFiles;sync
	WriteBookTags string `json:"writeBookTags,omitempty"`

	// updateCovers sets book covers in Calibre to match Readarr's.
	// +optional
	UpdateCovers *bool `json:"updateCovers,omitempty"`

	// embedMetadata has Calibre write metadata into the book files themselves.
	// +optional
	EmbedMetadata *bool `json:"embedMetadata,omitempty"`

	// writeAudioTags is when Readarr tags audiobook files with metadata: no,
	// newFiles for new downloads only, allFiles when files are first imported
	// (altering existing files), or sync to keep every file in sync.
	// +optional
	// +kubebuilder:validation:Enum=no;newFiles;allFiles;sync
	WriteAudioTags string `json:"writeAudioTags,omitempty"`

	// scrubAudioTags removes existing tags from audiobook files, leaving only
	// those Readarr adds.
	// +optional
	ScrubAudioTags *bool `json:"scrubAudioTags,omitempty"`
}

// ReadarrDevelopment configures the settings on Readarr's hidden development
// page.
type ReadarrDevelopment struct {
	// metadataSource is the URL of a metadata server Readarr uses in place of
	// its own, such as a replacement for the retired official one.
	// +required
	// +kubebuilder:validation:Pattern=`^https?://`
	// +kubebuilder:validation:MinLength=1
	MetadataSource string `json:"metadataSource,omitempty"`
}

// ReadarrStatus defines the observed state of Readarr.
// +kubebuilder:validation:MinProperties=1
type ReadarrStatus struct {
	// conditions represent the current state of the Readarr resource.
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

// Readarr configures a Readarr instance the operator connects to.
type Readarr struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is the standard object metadata.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec is the desired state of the Readarr.
	// +required
	Spec ReadarrSpec `json:"spec,omitzero"`

	// status is the observed state of the Readarr.
	// +optional
	Status ReadarrStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// ReadarrList contains a list of Readarr.
type ReadarrList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []Readarr `json:"items"`
}

func (c *Readarr) GetConditions() *[]metav1.Condition                  { return &c.Status.Conditions }
func (c *Readarr) GetObservedGeneration() *int64                       { return &c.Status.ObservedGeneration }
func (c *Readarr) GetLastSyncTime() **metav1.Time                      { return &c.Status.LastSyncTime }
func (c *Readarr) GetReconcileConfig() *commonv1alpha1.ReconcileConfig { return c.Spec.Reconcile }
func (c *Readarr) GetManagedResources() map[string][]string {
	return commonv1alpha1.ManagedMap(c.Status.ManagedResources)
}

func (c *Readarr) SetManagedResources(m map[string][]string) {
	c.Status.ManagedResources = commonv1alpha1.ManagedList(m)
}
