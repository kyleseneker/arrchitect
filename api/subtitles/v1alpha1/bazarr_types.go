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

// BazarrSpec defines the desired state of Bazarr.
type BazarrSpec struct {
	// connection defines how to connect to the Bazarr instance.
	// +required
	Connection commonv1alpha1.AppConnection `json:"connection,omitempty"`

	// reconcile configures reconciliation behavior (e.g., sync interval).
	// +optional
	Reconcile *commonv1alpha1.ReconcileConfig `json:"reconcile,omitempty"`

	// general configures general Bazarr settings.
	// +optional
	General *BazarrGeneral `json:"general,omitempty"`

	// sonarrConnection configures the connection to a Sonarr instance.
	// +optional
	// +kubebuilder:validation:XValidation:rule="!has(self.appRef) || self.appRef.kind == 'Sonarr'",message="sonarrConnection refers to a Sonarr"
	SonarrConnection BazarrAppConnection `json:"sonarrConnection,omitzero"`

	// radarrConnection configures the connection to a Radarr instance.
	// +optional
	// +kubebuilder:validation:XValidation:rule="!has(self.appRef) || self.appRef.kind == 'Radarr'",message="radarrConnection refers to a Radarr"
	RadarrConnection BazarrAppConnection `json:"radarrConnection,omitzero"`

	// jellyfin has Bazarr refresh a movie or episode in Jellyfin after its
	// subtitles change, so they show without waiting for a library scan.
	// Setting it turns Bazarr's Jellyfin integration on. Needs Bazarr 1.6.0 or
	// later.
	// +optional
	Jellyfin *BazarrJellyfin `json:"jellyfin,omitempty"`

	// subSync configures subtitle synchronization settings.
	// +optional
	SubSync *BazarrSubSync `json:"subSync,omitempty"`

	// notifications sets up Bazarr's notification providers, matched by name.
	// Bazarr lists every provider Apprise supports, and one left out keeps its
	// settings.
	// +optional
	// +listType=map
	// +listMapKey=name
	Notifications []BazarrNotification `json:"notifications,omitempty"`

	// providers defines subtitle provider configurations.
	// +optional
	// +listType=map
	// +listMapKey=name
	Providers []BazarrProvider `json:"providers,omitempty"`

	// languages configures language settings and profiles.
	// +optional
	Languages *BazarrLanguages `json:"languages,omitempty"`
}

// BazarrGeneral configures general Bazarr settings.
type BazarrGeneral struct {
	// useSonarr enables the Sonarr integration.
	// +optional
	UseSonarr *bool `json:"useSonarr,omitempty"`

	// useRadarr enables the Radarr integration.
	// +optional
	UseRadarr *bool `json:"useRadarr,omitempty"`

	// minimumScore is the minimum subtitle match score for series.
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	MinimumScore *int32 `json:"minimumScore,omitempty"`

	// minimumScoreMovie is the minimum subtitle match score for movies.
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	MinimumScoreMovie *int32 `json:"minimumScoreMovie,omitempty"`

	// useScenename uses scene names for subtitle searching.
	// +optional
	UseScenename *bool `json:"useScenename,omitempty"`

	// usePostprocessing enables post-processing of downloaded subtitles.
	// +optional
	UsePostprocessing *bool `json:"usePostprocessing,omitempty"`

	// adaptiveSearching enables adaptive searching for subtitles.
	// +optional
	AdaptiveSearching *bool `json:"adaptiveSearching,omitempty"`

	// multithreading enables multithreaded subtitle downloading.
	// +optional
	Multithreading *bool `json:"multithreading,omitempty"`

	// chmodEnabled enables setting file permissions on subtitle files.
	// +optional
	ChmodEnabled *bool `json:"chmodEnabled,omitempty"`

	// upgradeSubs enables automatic subtitle upgrades when better ones are found.
	// +optional
	UpgradeSubs *bool `json:"upgradeSubs,omitempty"`

	// upgradeFrequency is how often, in hours, Bazarr looks for better
	// subtitles. 876000, a hundred years, is how Bazarr turns it off.
	// +optional
	// +kubebuilder:validation:Enum=6;12;24;168;876000
	UpgradeFrequency *int32 `json:"upgradeFrequency,omitempty"`

	// daysToUpgradeSubs is how many days back Bazarr looks for subtitles to upgrade.
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=30
	DaysToUpgradeSubs *int32 `json:"daysToUpgradeSubs,omitempty"`

	// upgradeManual also upgrades subtitles that were downloaded or translated by hand.
	// +optional
	UpgradeManual *bool `json:"upgradeManual,omitempty"`

	// wantedSearchFrequency is how often, in hours, Bazarr searches for missing
	// episode subtitles. 876000, a hundred years, is how Bazarr turns it off.
	// +optional
	// +kubebuilder:validation:Enum=6;12;24;168;876000
	WantedSearchFrequency *int32 `json:"wantedSearchFrequency,omitempty"`

	// wantedSearchFrequencyMovie is how often, in hours, Bazarr searches for
	// missing movie subtitles. 876000, a hundred years, is how Bazarr turns it off.
	// +optional
	// +kubebuilder:validation:Enum=6;12;24;168;876000
	WantedSearchFrequencyMovie *int32 `json:"wantedSearchFrequencyMovie,omitempty"`

	// adaptiveSearchingDelay is how long after the first search adaptiveSearching
	// starts searching less often.
	// +optional
	// +kubebuilder:validation:Enum="1w";"2w";"3w";"4w"
	AdaptiveSearchingDelay string `json:"adaptiveSearchingDelay,omitempty"`

	// adaptiveSearchingDelta is how often adaptiveSearching searches once it
	// searches less often.
	// +optional
	// +kubebuilder:validation:Enum="3d";"1w";"2w";"3w";"4w"
	AdaptiveSearchingDelta string `json:"adaptiveSearchingDelta,omitempty"`

	// seriesTagEnabled gives a series the language profile whose tag matches
	// one of its Sonarr tags.
	// +optional
	SeriesTagEnabled *bool `json:"seriesTagEnabled,omitempty"`

	// movieTagEnabled gives a movie the language profile whose tag matches one
	// of its Radarr tags.
	// +optional
	MovieTagEnabled *bool `json:"movieTagEnabled,omitempty"`

	// removeProfileTags lists Sonarr and Radarr tags that take a series' or
	// movie's language profile away when seriesTagEnabled or movieTagEnabled is on.
	// +optional
	// +listType=set
	// +kubebuilder:validation:items:Pattern=`^[a-z0-9_-]+$`
	// +kubebuilder:validation:items:MaxLength=64
	RemoveProfileTags []string `json:"removeProfileTags,omitempty"`

	// utf8Encode encodes subtitles to UTF-8.
	// +optional
	Utf8Encode *bool `json:"utf8Encode,omitempty"`

	// ignorePgsSubs ignores PGS (image-based) subtitles.
	// +optional
	IgnorePgsSubs *bool `json:"ignorePgsSubs,omitempty"`

	// ignoreVobsubSubs ignores VobSub (image-based) subtitles.
	// +optional
	IgnoreVobsubSubs *bool `json:"ignoreVobsubSubs,omitempty"`

	// ignoreAssSubs ignores ASS/SSA subtitles.
	// +optional
	IgnoreAssSubs *bool `json:"ignoreAssSubs,omitempty"`

	// embeddedSubtitlesParser is the parser used for embedded subtitles.
	// +optional
	// +kubebuilder:validation:Enum=ffprobe;mediainfo
	EmbeddedSubtitlesParser string `json:"embeddedSubtitlesParser,omitempty"`

	// hiExtension is the file extension used for hearing-impaired subtitle files.
	// +optional
	// +kubebuilder:validation:Enum=hi;cc;sdh
	HiExtension string `json:"hiExtension,omitempty"`

	// pathMappings translate Sonarr's paths into the paths Bazarr sees the same
	// files at. Bazarr separates the two with a comma, so neither may have one.
	// +optional
	// +listType=map
	// +listMapKey=remotePath
	PathMappings []BazarrPathMapping `json:"pathMappings,omitempty"`

	// pathMappingsMovie translate Radarr's paths into the paths Bazarr sees the
	// same files at.
	// +optional
	// +listType=map
	// +listMapKey=remotePath
	PathMappingsMovie []BazarrPathMapping `json:"pathMappingsMovie,omitempty"`

	// seriesDefaultProfile is the language profile, by name, that new series
	// get. Setting it turns the default on.
	// +optional
	// +kubebuilder:validation:MinLength=1
	SeriesDefaultProfile string `json:"seriesDefaultProfile,omitempty"`

	// movieDefaultProfile is the language profile, by name, that new movies
	// get. Setting it turns the default on.
	// +optional
	// +kubebuilder:validation:MinLength=1
	MovieDefaultProfile string `json:"movieDefaultProfile,omitempty"`

	// useEmbeddedSubs counts subtitles embedded in the media file as present.
	// +optional
	UseEmbeddedSubs *bool `json:"useEmbeddedSubs,omitempty"`

	// subfolder is where subtitles are saved: next to the media file, in a
	// folder relative to it, or in an absolute folder.
	// +optional
	// +kubebuilder:validation:Enum=current;relative;absolute
	Subfolder string `json:"subfolder,omitempty"`

	// subfolderCustom is the folder subfolder relative or absolute saves to.
	// +optional
	// +kubebuilder:validation:MinLength=1
	SubfolderCustom string `json:"subfolderCustom,omitempty"`

	// chmod is the octal permissions chmodEnabled sets on subtitle files, such as 0640.
	// +optional
	// +kubebuilder:validation:Pattern=`^0?[0-7]{3}$`
	// +kubebuilder:validation:MinLength=1
	Chmod string `json:"chmod,omitempty"`

	// postprocessingCmd is the command usePostprocessing runs after a subtitle
	// is downloaded.
	// +optional
	// +kubebuilder:validation:MinLength=1
	PostprocessingCmd string `json:"postprocessingCmd,omitempty"`

	// usePostprocessingThreshold runs postprocessingCmd for an episode subtitle
	// only when its score is below postprocessingThreshold.
	// +optional
	UsePostprocessingThreshold *bool `json:"usePostprocessingThreshold,omitempty"`

	// postprocessingThreshold is the episode subtitle score, in percent, below
	// which postprocessingCmd runs.
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	PostprocessingThreshold *int32 `json:"postprocessingThreshold,omitempty"`

	// usePostprocessingThresholdMovie runs postprocessingCmd for a movie
	// subtitle only when its score is below postprocessingThresholdMovie.
	// +optional
	UsePostprocessingThresholdMovie *bool `json:"usePostprocessingThresholdMovie,omitempty"`

	// postprocessingThresholdMovie is the movie subtitle score, in percent,
	// below which postprocessingCmd runs.
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	PostprocessingThresholdMovie *int32 `json:"postprocessingThresholdMovie,omitempty"`
}

// BazarrPathMapping translates a path Sonarr or Radarr reports into the path
// Bazarr reads the same file at.
type BazarrPathMapping struct {
	// remotePath is the path as Sonarr or Radarr reports it.
	// +required
	// +kubebuilder:validation:Pattern=`^[^,]+$`
	// +kubebuilder:validation:MinLength=1
	RemotePath string `json:"remotePath,omitempty"`

	// localPath is the path inside the Bazarr container.
	// +required
	// +kubebuilder:validation:Pattern=`^[^,]+$`
	// +kubebuilder:validation:MinLength=1
	LocalPath string `json:"localPath,omitempty"`
}

// BazarrAppConnection defines a connection to a Sonarr or Radarr instance from Bazarr.
// +kubebuilder:validation:MinProperties=1
// +kubebuilder:validation:XValidation:rule="has(self.appRef) ? !has(self.host) && !has(self.port) && !has(self.basePath) && !has(self.ssl) && !has(self.apiKeySecretRef) : has(self.host) && has(self.port) && has(self.apiKeySecretRef)",message="set appRef, or host, port and apiKeySecretRef, not both"
type BazarrAppConnection struct {
	// appRef names the application's resource, whose connection is used in
	// place of host, port, basePath, ssl and apiKeySecretRef.
	// +optional
	AppRef *commonv1alpha1.AppRef `json:"appRef,omitempty"`

	// host is the hostname or IP of the application.
	// +optional
	// +kubebuilder:validation:MinLength=1
	Host string `json:"host,omitempty"`

	// port is the port of the application.
	// +optional
	Port *int32 `json:"port,omitempty"`

	// basePath is the URL base path of the application.
	// +optional
	// +kubebuilder:validation:MinLength=1
	BasePath string `json:"basePath,omitempty"`

	// ssl enables HTTPS connections to the application.
	// +optional
	SSL *bool `json:"ssl,omitempty"`

	// apiKeySecretRef references a Secret containing the application's API key.
	// +optional
	APIKeySecretRef *commonv1alpha1.SecretKeyRef `json:"apiKeySecretRef,omitempty"`

	// httpTimeout is the HTTP request timeout in seconds.
	// +optional
	// +kubebuilder:validation:Enum=60;120;180;240;300;600
	HttpTimeout *int32 `json:"httpTimeout,omitempty"`

	// fullUpdate controls how often a full library sync is performed.
	// +optional
	// +kubebuilder:validation:Enum=Daily;Weekly;Manually
	FullUpdate string `json:"fullUpdate,omitempty"`

	// onlyMonitored limits subtitle searches to monitored items only.
	// +optional
	OnlyMonitored *bool `json:"onlyMonitored,omitempty"`

	// syncInterval is how often, in minutes, Bazarr syncs the library.
	// +optional
	// +kubebuilder:validation:Enum=15;60;180;360;720;1440;10080
	SyncInterval *int32 `json:"syncInterval,omitempty"`

	// excludedTags leaves out items with any of these tags.
	// +optional
	// +listType=set
	// +kubebuilder:validation:items:Pattern=`^[a-z0-9_-]+$`
	// +kubebuilder:validation:items:MaxLength=64
	ExcludedTags []string `json:"excludedTags,omitempty"`

	// excludedSeriesTypes leaves out series of these types. Only applicable
	// when used with Sonarr.
	// +optional
	// +kubebuilder:validation:items:Enum=standard;anime;daily
	// +listType=set
	ExcludedSeriesTypes []string `json:"excludedSeriesTypes,omitempty"`

	// excludeSeasonZero leaves out specials. Only applicable when used with Sonarr.
	// +optional
	ExcludeSeasonZero *bool `json:"excludeSeasonZero,omitempty"`
}

// BazarrJellyfin connects Bazarr to Jellyfin and names the libraries it
// refreshes items in.
// +kubebuilder:validation:XValidation:rule="has(self.appRef) ? !has(self.url) && !has(self.apiKeySecretRef) : has(self.url) && has(self.apiKeySecretRef)",message="set appRef, or url and apiKeySecretRef, not both"
type BazarrJellyfin struct {
	// appRef names the Jellyfin resource whose connection URL and API key are
	// used in place of url and apiKeySecretRef. That resource must set its
	// connection's apiKeySecretRef.
	// +optional
	// +kubebuilder:validation:XValidation:rule="self.kind == 'Jellyfin'",message="jellyfin refers to a Jellyfin"
	AppRef *commonv1alpha1.AppRef `json:"appRef,omitempty"`

	// url is the Jellyfin URL as Bazarr reaches it (e.g., http://jellyfin.media.svc.cluster.local:8096).
	// +optional
	// +kubebuilder:validation:Pattern=`^https?://`
	// +kubebuilder:validation:MinLength=1
	URL string `json:"url,omitempty"`

	// apiKeySecretRef references a Secret containing a Jellyfin API key.
	// +optional
	APIKeySecretRef *commonv1alpha1.SecretKeyRef `json:"apiKeySecretRef,omitempty"`

	// movieLibraries names the Jellyfin movie libraries Bazarr finds a movie
	// in, and turns on refreshing a movie whose subtitles change. Left out,
	// Bazarr keeps its movie library settings.
	// +optional
	// +listType=set
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:items:MinLength=1
	MovieLibraries []string `json:"movieLibraries,omitempty"`

	// seriesLibraries names the Jellyfin TV libraries Bazarr finds a series in,
	// and turns on refreshing an episode whose subtitles change. Left out,
	// Bazarr keeps its series library settings.
	// +optional
	// +listType=set
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:items:MinLength=1
	SeriesLibraries []string `json:"seriesLibraries,omitempty"`

	// refreshMethod is how Bazarr tells Jellyfin: immediate refreshes the item
	// at once, and async reports the changed file for Jellyfin to pick up with
	// other changes.
	// +optional
	// +kubebuilder:validation:Enum=immediate;async
	RefreshMethod string `json:"refreshMethod,omitempty"`
}

// BazarrNotification sets up one of Bazarr's notification providers.
// +kubebuilder:validation:MinProperties=2
type BazarrNotification struct {
	// name is the provider, as Bazarr lists it, such as Discord or Telegram.
	// +required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name,omitempty"`

	// enabled turns the provider on.
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// urlSecretRef references a Secret containing the provider's Apprise URL,
	// such as discord://webhook_id/webhook_token.
	// +optional
	URLSecretRef *commonv1alpha1.SecretKeyRef `json:"urlSecretRef,omitempty"`
}

// BazarrSubSync configures subtitle synchronization settings.
type BazarrSubSync struct {
	// useSubsync enables automatic subtitle synchronization.
	// +optional
	UseSubsync *bool `json:"useSubsync,omitempty"`

	// useSubsyncThreshold enables a score threshold for series subtitle sync.
	// +optional
	UseSubsyncThreshold *bool `json:"useSubsyncThreshold,omitempty"`

	// subsyncThreshold is the minimum score threshold for series subtitle sync.
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	SubsyncThreshold *int32 `json:"subsyncThreshold,omitempty"`

	// useSubsyncMovieThreshold enables a score threshold for movie subtitle sync.
	// +optional
	UseSubsyncMovieThreshold *bool `json:"useSubsyncMovieThreshold,omitempty"`

	// subsyncMovieThreshold is the minimum score threshold for movie subtitle sync.
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	SubsyncMovieThreshold *int32 `json:"subsyncMovieThreshold,omitempty"`
}

// BazarrProvider defines a subtitle provider configuration.
type BazarrProvider struct {
	// name is the provider key (e.g., opensubtitlescom, addic7ed).
	// +required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name,omitempty"`

	// enabled controls whether this provider is active.
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// settings are the provider's settings, each a text value or read from a
	// Secret in the resource's namespace, such as a password or API key.
	// +optional
	// +listType=map
	// +listMapKey=name
	Settings []commonv1alpha1.ConfigField `json:"settings,omitempty"`
}

// BazarrLanguages configures language settings and profiles.
type BazarrLanguages struct {
	// enabled is the list of enabled subtitle languages as ISO 639-1 codes.
	// +optional
	// +listType=set
	Enabled []string `json:"enabled,omitempty"`

	// profiles defines language profiles for subtitle selection.
	// +optional
	// +listType=map
	// +listMapKey=name
	Profiles []BazarrLanguageProfile `json:"profiles,omitempty"`
}

// BazarrLanguageProfile defines a language profile for subtitle selection.
type BazarrLanguageProfile struct {
	// profileId is the unique identifier for this profile.
	// +required
	ProfileId *int32 `json:"profileId,omitempty"`

	// name is the display name of the language profile.
	// +required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name,omitempty"`

	// items defines the language items in this profile.
	// +optional
	// +listType=atomic
	Items []BazarrLanguageItem `json:"items,omitempty"`

	// cutoff is the item whose subtitle ends the search for the others, named
	// as Bazarr shows it: the language code, with :hi or :forced for such an
	// item, such as en or en:hi. any ends it at whichever item is found first,
	// and none searches for every item.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=32
	Cutoff string `json:"cutoff,omitempty"`

	// mustContain lists words or regular expressions one of which a
	// subtitle's release information must contain.
	// +optional
	// +listType=set
	// +kubebuilder:validation:items:MinLength=1
	MustContain []string `json:"mustContain,omitempty"`

	// mustNotContain lists words or regular expressions that rule out a
	// subtitle whose release information contains one, ignoring case.
	// +optional
	// +listType=set
	// +kubebuilder:validation:items:MinLength=1
	MustNotContain []string `json:"mustNotContain,omitempty"`

	// originalFormat keeps downloaded subtitles in their own format instead
	// of converting them.
	// +optional
	OriginalFormat *bool `json:"originalFormat,omitempty"`

	// tag is the Sonarr or Radarr tag that gives a series or movie this
	// profile when general.seriesTagEnabled or movieTagEnabled is on.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=64
	// +kubebuilder:validation:Pattern=`^[a-z0-9_-]+$`
	Tag string `json:"tag,omitempty"`
}

// BazarrLanguageItem defines a language entry within a language profile.
// +kubebuilder:validation:XValidation:rule="!(has(self.audioExclude) && self.audioExclude && has(self.audioOnlyInclude) && self.audioOnlyInclude)",message="audioExclude and audioOnlyInclude can't both be true"
type BazarrLanguageItem struct {
	// language is the ISO 639-1 language code.
	// +required
	// +kubebuilder:validation:MinLength=1
	Language string `json:"language,omitempty"`

	// hi indicates whether hearing-impaired subtitles are preferred.
	// +optional
	HI *bool `json:"hi,omitempty"`

	// forced indicates whether forced subtitles are preferred.
	// +optional
	Forced *bool `json:"forced,omitempty"`

	// audioExclude excludes this language if audio already matches.
	// +optional
	AudioExclude *bool `json:"audioExclude,omitempty"`

	// audioOnlyInclude wants this language only when the media has an audio
	// track in it.
	// +optional
	AudioOnlyInclude *bool `json:"audioOnlyInclude,omitempty"`
}

// BazarrStatus defines the observed state of Bazarr.
// +kubebuilder:validation:MinProperties=1
type BazarrStatus struct {
	// conditions represent the current state of the Bazarr resource.
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

// Bazarr configures a Bazarr instance the operator connects to.
type Bazarr struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is the standard object metadata.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec is the desired state of the Bazarr.
	// +required
	Spec BazarrSpec `json:"spec,omitzero"`

	// status is the observed state of the Bazarr.
	// +optional
	Status BazarrStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// BazarrList contains a list of Bazarr.
type BazarrList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []Bazarr `json:"items"`
}

func (c *Bazarr) GetConditions() *[]metav1.Condition                  { return &c.Status.Conditions }
func (c *Bazarr) GetObservedGeneration() *int64                       { return &c.Status.ObservedGeneration }
func (c *Bazarr) GetLastSyncTime() **metav1.Time                      { return &c.Status.LastSyncTime }
func (c *Bazarr) GetReconcileConfig() *commonv1alpha1.ReconcileConfig { return c.Spec.Reconcile }
func (c *Bazarr) GetManagedResources() map[string][]string {
	return commonv1alpha1.ManagedMap(c.Status.ManagedResources)
}

func (c *Bazarr) SetManagedResources(m map[string][]string) {
	c.Status.ManagedResources = commonv1alpha1.ManagedList(m)
}
