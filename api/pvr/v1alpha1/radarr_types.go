/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RadarrSpec is a Radarr instance's connection and app-wide settings. Indexers
// and download clients are resources of their own that refer to it.
type RadarrSpec struct {
	// connection defines how to connect to the Radarr instance.
	// +required
	Connection commonv1alpha1.AppConnection `json:"connection,omitempty"`

	// reconcile configures reconciliation behavior (e.g., sync interval).
	// +optional
	Reconcile *commonv1alpha1.ReconcileConfig `json:"reconcile,omitempty"`

	// mediaManagement configures media management settings.
	// +optional
	MediaManagement *RadarrMediaManagement `json:"mediaManagement,omitempty"`

	// naming configures movie and folder naming formats.
	// +optional
	Naming *RadarrNaming `json:"naming,omitempty"`

	// indexerConfig configures global indexer settings.
	// +optional
	IndexerConfig *RadarrIndexerConfig `json:"indexerConfig,omitempty"`

	// downloadClientConfig configures global download client behavior.
	// +optional
	DownloadClientConfig *RadarrDownloadClientConfig `json:"downloadClientConfig,omitempty"`

	// ui configures UI display settings.
	// +optional
	UI *RadarrUI `json:"ui,omitempty"`

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

// RadarrMediaManagement configures how Radarr handles media files.
type RadarrMediaManagement struct {
	// autoUnmonitorPreviouslyDownloadedMovies unmonitors movies that have been downloaded.
	// +optional
	AutoUnmonitorPreviouslyDownloadedMovies *bool `json:"autoUnmonitorPreviouslyDownloadedMovies,omitempty"`

	// createEmptyMovieFolders creates folders for movies with no files.
	// +optional
	CreateEmptyMovieFolders *bool `json:"createEmptyMovieFolders,omitempty"`

	// autoRenameFolders automatically renames movie folders when the movie title changes.
	// +optional
	AutoRenameFolders *bool `json:"autoRenameFolders,omitempty"`

	// pathsDefaultStatic prevents Radarr from updating paths when the root folder changes.
	// +optional
	PathsDefaultStatic *bool `json:"pathsDefaultStatic,omitempty"`

	// copyUsingHardlinks uses hardlinks instead of copies when importing.
	// +optional
	CopyUsingHardlinks *bool `json:"copyUsingHardlinks,omitempty"`

	// deleteEmptyFolders removes empty movie folders after cleanup.
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

	// rescanAfterRefresh controls when to rescan movie folders.
	// +optional
	// +kubebuilder:validation:Enum=always;afterManual;never
	RescanAfterRefresh string `json:"rescanAfterRefresh,omitempty"`

	// setPermissionsLinux enables setting file permissions on import.
	// +optional
	SetPermissionsLinux *bool `json:"setPermissionsLinux,omitempty"`

	// skipFreeSpaceCheckWhenImporting skips free space check during import.
	// +optional
	SkipFreeSpaceCheckWhenImporting *bool `json:"skipFreeSpaceCheckWhenImporting,omitempty"`
}

// RadarrNaming configures movie and folder naming conventions.
type RadarrNaming struct {
	// renameMovies enables automatic movie file renaming.
	// +optional
	RenameMovies *bool `json:"renameMovies,omitempty"`

	// replaceIllegalCharacters replaces illegal characters in filenames.
	// +optional
	ReplaceIllegalCharacters *bool `json:"replaceIllegalCharacters,omitempty"`

	// colonReplacementFormat defines how colons are replaced in filenames.
	// +optional
	// +kubebuilder:validation:Enum=delete;dash;spaceDash;spaceDashSpace;smart
	ColonReplacementFormat string `json:"colonReplacementFormat,omitempty"`

	// standardMovieFormat is the naming format for movies.
	// +optional
	// +kubebuilder:validation:MinLength=1
	StandardMovieFormat string `json:"standardMovieFormat,omitempty"`

	// movieFolderFormat is the naming format for movie folders.
	// +optional
	// +kubebuilder:validation:MinLength=1
	MovieFolderFormat string `json:"movieFolderFormat,omitempty"`
}

// RadarrIndexerConfig configures global indexer settings.
type RadarrIndexerConfig struct {
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

	// preferIndexerFlags prefers releases with indexer flags.
	// +optional
	PreferIndexerFlags *bool `json:"preferIndexerFlags,omitempty"`

	// availabilityDelay is the number of days to wait after a movie is available before searching.
	// +optional
	AvailabilityDelay *int32 `json:"availabilityDelay,omitempty"`

	// allowHardcodedSubs allows importing releases with hardcoded subtitles.
	// +optional
	AllowHardcodedSubs *bool `json:"allowHardcodedSubs,omitempty"`

	// whitelistedHardcodedSubs is a comma-separated list of subtitle languages to allow when hardcoded.
	// +optional
	// +kubebuilder:validation:MinLength=1
	WhitelistedHardcodedSubs string `json:"whitelistedHardcodedSubs,omitempty"`
}

// RadarrDownloadClientConfig configures global download client behavior.
type RadarrDownloadClientConfig struct {
	// enableCompletedDownloadHandling auto-imports completed downloads.
	// +optional
	EnableCompletedDownloadHandling *bool `json:"enableCompletedDownloadHandling,omitempty"`

	// autoRedownloadFailed automatically searches when a download fails.
	// +optional
	AutoRedownloadFailed *bool `json:"autoRedownloadFailed,omitempty"`

	// autoRedownloadFailedFromInteractiveSearch auto-searches failed interactive searches.
	// +optional
	AutoRedownloadFailedFromInteractiveSearch *bool `json:"autoRedownloadFailedFromInteractiveSearch,omitempty"`

	// checkForFinishedDownloadInterval is the interval in minutes to check for finished downloads.
	// +optional
	CheckForFinishedDownloadInterval *int32 `json:"checkForFinishedDownloadInterval,omitempty"`
}

// RadarrUI configures UI display settings.
type RadarrUI struct {
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

	// movieRuntimeFormat controls how movie runtimes are displayed.
	// +optional
	// +kubebuilder:validation:Enum=hoursMinutes;minutes
	MovieRuntimeFormat string `json:"movieRuntimeFormat,omitempty"`

	// movieInfoLanguage is the language ID for movie information. -1=Original.
	// +optional
	MovieInfoLanguage *int32 `json:"movieInfoLanguage,omitempty"`
}

// RadarrStatus defines the observed state of Radarr.
// +kubebuilder:validation:MinProperties=1
type RadarrStatus struct {
	// conditions represent the current state of the Radarr resource.
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

// Radarr configures a Radarr instance the operator connects to.
type Radarr struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is the standard object metadata.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec is the desired state of the Radarr.
	// +required
	Spec RadarrSpec `json:"spec,omitzero"`

	// status is the observed state of the Radarr.
	// +optional
	Status RadarrStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// RadarrList contains a list of Radarr.
type RadarrList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []Radarr `json:"items"`
}

func (c *Radarr) GetConditions() *[]metav1.Condition                  { return &c.Status.Conditions }
func (c *Radarr) GetObservedGeneration() *int64                       { return &c.Status.ObservedGeneration }
func (c *Radarr) GetLastSyncTime() **metav1.Time                      { return &c.Status.LastSyncTime }
func (c *Radarr) GetReconcileConfig() *commonv1alpha1.ReconcileConfig { return c.Spec.Reconcile }
func (c *Radarr) GetManagedResources() map[string][]string {
	return commonv1alpha1.ManagedMap(c.Status.ManagedResources)
}

func (c *Radarr) SetManagedResources(m map[string][]string) {
	c.Status.ManagedResources = commonv1alpha1.ManagedList(m)
}
