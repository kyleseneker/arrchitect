package v1alpha1

import (
	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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
