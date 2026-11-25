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

// SeerrSpec defines the desired state of Seerr.
// +kubebuilder:validation:XValidation:rule="!(has(self.jellyfinAuth) && has(self.plexAuth))",message="jellyfinAuth and plexAuth are mutually exclusive"
// +kubebuilder:validation:XValidation:rule="!has(self.libraries) || has(self.jellyfinAuth) || has(self.plexAuth)",message="libraries needs jellyfinAuth or plexAuth"
type SeerrSpec struct {
	// connection defines how to connect to the Seerr instance.
	// +required
	Connection SeerrConnection `json:"connection,omitzero"`

	// reconcile configures reconciliation behavior (e.g., sync interval).
	// +optional
	Reconcile *commonv1alpha1.ReconcileConfig `json:"reconcile,omitempty"`

	// jellyfinAuth defines Jellyfin authentication settings.
	// Set this for Jellyfin-based Seerr instances. Mutually exclusive with plexAuth.
	// +optional
	JellyfinAuth SeerrJellyfinAuth `json:"jellyfinAuth,omitzero"`

	// plexAuth defines Plex authentication settings.
	// Set this for Plex-based Seerr instances. Mutually exclusive with jellyfinAuth.
	// +optional
	PlexAuth SeerrPlexAuth `json:"plexAuth,omitzero"`

	// libraries names the media server's libraries Seerr shows and takes
	// requests for. Those are enabled and the rest disabled, on every pass.
	// Left out, first-time setup enables every library and later passes leave
	// them as they are.
	// +optional
	// +listType=set
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:items:MinLength=1
	Libraries []string `json:"libraries,omitempty"`

	// main configures general application settings.
	// +optional
	Main *SeerrMain `json:"main,omitempty"`

	// network configures how Seerr treats the network in front of it.
	// +optional
	Network SeerrNetwork `json:"network,omitzero"`

	// metadataProviders chooses where Seerr gets series and anime metadata.
	// Seerr checks it can reach the providers chosen before it saves them.
	// +optional
	MetadataProviders SeerrMetadataProviders `json:"metadataProviders,omitzero"`

	// jobSchedules sets when Seerr runs its scheduled jobs. Jobs left out keep
	// their schedules.
	// +optional
	// +listType=map
	// +listMapKey=id
	JobSchedules []SeerrJobSchedule `json:"jobSchedules,omitempty"`

	// sonarr lists the Sonarr servers Seerr sends TV requests to, matched by
	// name. Seerr uses the default server, and the default 4K server for 4K
	// requests.
	// +optional
	// +listType=map
	// +listMapKey=name
	// +kubebuilder:validation:XValidation:rule="self.all(s, !has(s.appRef) || s.appRef.kind == 'Sonarr')",message="a sonarr server refers to a Sonarr"
	Sonarr []SeerrServiceConnection `json:"sonarr,omitempty"`

	// radarr lists the Radarr servers Seerr sends movie requests to, matched by
	// name. Seerr uses the default server, and the default 4K server for 4K
	// requests.
	// +optional
	// +listType=map
	// +listMapKey=name
	// +kubebuilder:validation:XValidation:rule="self.all(s, !has(s.appRef) || s.appRef.kind == 'Radarr')",message="a radarr server refers to a Radarr"
	Radarr []SeerrServiceConnection `json:"radarr,omitempty"`

	// notifications configures notification agents (Discord, Slack, Telegram, etc.).
	// Each agent takes one configuration.
	// +optional
	// +listType=map
	// +listMapKey=agent
	Notifications []SeerrNotificationAgent `json:"notifications,omitempty"`
}

// SeerrNotificationAgent configures a single notification agent in Seerr.
type SeerrNotificationAgent struct {
	// agent is the notification agent type.
	// Supported values: discord, slack, telegram, pushover, email, webhook,
	// gotify, ntfy, pushbullet, webpush.
	// +required
	// +kubebuilder:validation:MinLength=1
	Agent string `json:"agent,omitempty"`

	// enabled enables this notification agent.
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// types are the events that send a notification. An empty list sends none.
	// +optional
	// +kubebuilder:validation:items:Enum=mediaPending;mediaApproved;mediaAutoApproved;mediaAutoRequested;mediaAvailable;mediaFailed;mediaDeclined;issueCreated;issueComment;issueResolved;issueReopened
	// +listType=set
	Types []string `json:"types,omitempty"`

	// options are the agent's settings, such as botUsername or chatId, each a
	// value or read from a Secret, such as webhookUrl or botAPI. Values keep
	// their type, so enableMentions: false is off.
	// +optional
	// +listType=map
	// +listMapKey=name
	Options []commonv1alpha1.ConfigField `json:"options,omitempty"`
}

// SeerrConnection defines how the operator connects to Seerr.
type SeerrConnection struct {
	// url is the base URL of the Seerr instance (e.g., http://seerr.media.svc.cluster.local:5055).
	// +required
	// +kubebuilder:validation:Pattern=`^https?://`
	// +kubebuilder:validation:MinLength=1
	URL string `json:"url,omitempty"`

	// apiKeySecretRef references a Secret containing Seerr's API key. When set,
	// it is used instead of logging in through the media server.
	// +optional
	APIKeySecretRef *commonv1alpha1.SecretKeyRef `json:"apiKeySecretRef,omitempty"`

	// tls configures TLS settings for the connection.
	// Only needed when Seerr uses HTTPS with self-signed or private CA certificates.
	// +optional
	TLS *commonv1alpha1.TLSConfig `json:"tls,omitempty"`
}

// SeerrPlexAuth defines Plex authentication for Seerr setup.
// +kubebuilder:validation:MinProperties=1
// +kubebuilder:validation:XValidation:rule="has(self.appRef) ? !has(self.hostname) && !has(self.port) && !has(self.useSsl) && !has(self.tokenSecretRef) : has(self.hostname) && has(self.tokenSecretRef)",message="set appRef, or hostname and tokenSecretRef, not both"
type SeerrPlexAuth struct {
	// appRef names the Plex resource whose connection URL and token are used in
	// place of hostname, port, useSsl and tokenSecretRef.
	// +optional
	// +kubebuilder:validation:XValidation:rule="self.kind == 'Plex'",message="plexAuth refers to a Plex"
	AppRef *commonv1alpha1.AppRef `json:"appRef,omitempty"`

	// hostname is the Plex server hostname.
	// +optional
	// +kubebuilder:validation:MinLength=1
	Hostname string `json:"hostname,omitempty"`

	// port is the Plex server port. Defaults to 32400.
	// +optional
	Port *int32 `json:"port,omitempty"`

	// useSsl enables SSL for the Plex connection.
	// +optional
	UseSsl *bool `json:"useSsl,omitempty"`

	// tokenSecretRef references a Secret containing a Plex token.
	// +optional
	TokenSecretRef *commonv1alpha1.SecretKeyRef `json:"tokenSecretRef,omitempty"`
}

// SeerrJellyfinAuth defines the Jellyfin authentication configuration for Seerr.
// +kubebuilder:validation:MinProperties=1
// +kubebuilder:validation:XValidation:rule="has(self.appRef) ? !has(self.hostname) && !has(self.port) && !has(self.useSsl) && !has(self.urlBase) && !has(self.usernameSecretRef) && !has(self.passwordSecretRef) : has(self.hostname) && has(self.port) && has(self.usernameSecretRef) && has(self.passwordSecretRef)",message="set appRef, or hostname, port, usernameSecretRef and passwordSecretRef, not both"
type SeerrJellyfinAuth struct {
	// appRef names the Jellyfin resource whose connection URL and admin user are
	// used in place of hostname, port, useSsl, urlBase, usernameSecretRef and
	// passwordSecretRef.
	// +optional
	// +kubebuilder:validation:XValidation:rule="self.kind == 'Jellyfin'",message="jellyfinAuth refers to a Jellyfin"
	AppRef *commonv1alpha1.AppRef `json:"appRef,omitempty"`

	// hostname is the Jellyfin server hostname.
	// +optional
	// +kubebuilder:validation:MinLength=1
	Hostname string `json:"hostname,omitempty"`

	// port is the Jellyfin server port.
	// +optional
	Port *int32 `json:"port,omitempty"`

	// useSsl enables SSL for the Jellyfin connection.
	// +optional
	UseSsl *bool `json:"useSsl,omitempty"`

	// urlBase is the subpath Jellyfin is served under, if any.
	// +optional
	// +kubebuilder:validation:MinLength=1
	UrlBase string `json:"urlBase,omitempty"`

	// usernameSecretRef references a Secret containing the Jellyfin username.
	// +optional
	UsernameSecretRef *commonv1alpha1.SecretKeyRef `json:"usernameSecretRef,omitempty"`

	// passwordSecretRef references a Secret containing the Jellyfin password.
	// +optional
	PasswordSecretRef *commonv1alpha1.SecretKeyRef `json:"passwordSecretRef,omitempty"`
}

// SeerrNetwork configures Seerr's network settings. CSRF protection is left
// out: Seerr then blocks API writes from other applications, the operator's
// included, and requires HTTPS.
// +kubebuilder:validation:MinProperties=1
type SeerrNetwork struct {
	// trustProxy has Seerr take client addresses from the proxy in front of it,
	// such as an ingress controller. Seerr applies it when it restarts.
	// +optional
	TrustProxy *bool `json:"trustProxy,omitempty"`
}

// SeerrMetadataProviders chooses Seerr's metadata providers.
// +kubebuilder:validation:MinProperties=1
type SeerrMetadataProviders struct {
	// tv is the provider for series: tmdb or tvdb.
	// +optional
	// +kubebuilder:validation:Enum=tmdb;tvdb
	TV string `json:"tv,omitempty"`

	// anime is the provider for anime series: tmdb or tvdb.
	// +optional
	// +kubebuilder:validation:Enum=tmdb;tvdb
	Anime string `json:"anime,omitempty"`
}

// SeerrJobSchedule sets when Seerr runs one of its jobs.
type SeerrJobSchedule struct {
	// id is the job. Seerr runs the plex- jobs only with Plex, and the
	// jellyfin- jobs only with Jellyfin or Emby.
	// +required
	// +kubebuilder:validation:Enum=plex-recently-added-scan;plex-full-scan;plex-watchlist-sync;plex-refresh-token;radarr-scan;sonarr-scan;download-sync;download-sync-reset;jellyfin-recently-added-scan;jellyfin-full-scan;image-cache-cleanup;availability-sync;process-blocklisted-tags
	ID string `json:"id,omitempty"`

	// schedule is a cron expression with a leading seconds field, such as
	// "0 */5 * * * *" for every five minutes.
	// +required
	// +kubebuilder:validation:MinLength=1
	Schedule string `json:"schedule,omitempty"`
}

// SeerrMain configures general Seerr application settings.
type SeerrMain struct {
	// applicationTitle is the application display title.
	// +optional
	// +kubebuilder:validation:MinLength=1
	ApplicationTitle string `json:"applicationTitle,omitempty"`

	// applicationUrl is the external URL for the application.
	// +optional
	// +kubebuilder:validation:MinLength=1
	ApplicationUrl string `json:"applicationUrl,omitempty"`

	// hideAvailable hides already-available media from discovery.
	// +optional
	HideAvailable *bool `json:"hideAvailable,omitempty"`

	// localLogin enables local user login.
	// +optional
	LocalLogin *bool `json:"localLogin,omitempty"`

	// mediaServerLogin enables login via the media server (Jellyfin).
	// +optional
	MediaServerLogin *bool `json:"mediaServerLogin,omitempty"`

	// defaultPermissions is the default permission bitmask for new users.
	// +optional
	DefaultPermissions *int32 `json:"defaultPermissions,omitempty"`

	// partialRequestsEnabled enables partial season/episode requests.
	// +optional
	PartialRequestsEnabled *bool `json:"partialRequestsEnabled,omitempty"`

	// locale is the application locale (e.g., "en").
	// +optional
	// +kubebuilder:validation:MinLength=1
	Locale string `json:"locale,omitempty"`

	// newMediaServerLogin lets media server users sign in before they're
	// imported, creating their Seerr user.
	// +optional
	NewMediaServerLogin *bool `json:"newMediaServerLogin,omitempty"`

	// defaultQuotas limits how much users without a quota of their own can
	// request.
	// +optional
	DefaultQuotas SeerrDefaultQuotas `json:"defaultQuotas,omitzero"`

	// hideRequested hides requested media from discovery.
	// +optional
	HideRequested *bool `json:"hideRequested,omitempty"`

	// hideBlocklisted hides blocklisted media from discovery for users who
	// manage the blocklist.
	// +optional
	HideBlocklisted *bool `json:"hideBlocklisted,omitempty"`

	// enableSpecialEpisodes lets users request a series' specials.
	// +optional
	EnableSpecialEpisodes *bool `json:"enableSpecialEpisodes,omitempty"`

	// cacheImages caches images from TMDB and other sources, which takes a lot
	// of disk space.
	// +optional
	CacheImages *bool `json:"cacheImages,omitempty"`

	// versionCheck checks GitHub for new Seerr releases.
	// +optional
	VersionCheck *bool `json:"versionCheck,omitempty"`

	// youtubeUrl is the base URL trailers play from, for a self-hosted
	// YouTube frontend.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Pattern=`^https?://`
	YoutubeUrl string `json:"youtubeUrl,omitempty"`

	// discoverRegion limits discovery to what's released in this ISO 3166-1
	// region, such as US.
	// +optional
	// +kubebuilder:validation:MinLength=2
	// +kubebuilder:validation:MaxLength=2
	// +kubebuilder:validation:Pattern=`^[A-Z]{2}$`
	DiscoverRegion string `json:"discoverRegion,omitempty"`

	// streamingRegion is the ISO 3166-1 region whose streaming services Seerr
	// shows.
	// +optional
	// +kubebuilder:validation:MinLength=2
	// +kubebuilder:validation:MaxLength=2
	// +kubebuilder:validation:Pattern=`^[A-Z]{2}$`
	StreamingRegion string `json:"streamingRegion,omitempty"`

	// originalLanguages limits discovery to media originally in these ISO
	// 639-1 languages, such as en.
	// +optional
	// +listType=set
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:items:Pattern=`^[a-z]{2}$`
	OriginalLanguages []string `json:"originalLanguages,omitempty"`

	// blocklistedTags are TMDB keyword IDs whose media Seerr's Process
	// Blocklisted Tags job adds to the blocklist. The job drops an ID TMDB
	// doesn't know, and the operator puts it back.
	// +optional
	// +listType=set
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:items:Minimum=1
	BlocklistedTags []int32 `json:"blocklistedTags,omitempty"`

	// blocklistedTagsLimit is how many pages of results the job blocklists
	// per tag and sort order. A new Seerr takes 50.
	// +optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=250
	BlocklistedTagsLimit int32 `json:"blocklistedTagsLimit,omitempty"`

	// blocklistRegion is the ISO 3166-1 region the job searches, apart from
	// discoverRegion.
	// +optional
	// +kubebuilder:validation:MinLength=2
	// +kubebuilder:validation:MaxLength=2
	// +kubebuilder:validation:Pattern=`^[A-Z]{2}$`
	BlocklistRegion string `json:"blocklistRegion,omitempty"`

	// blocklistLanguages limits the job to media originally in these ISO
	// 639-1 languages, apart from originalLanguages.
	// +optional
	// +listType=set
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:items:Pattern=`^[a-z]{2}$`
	BlocklistLanguages []string `json:"blocklistLanguages,omitempty"`
}

// SeerrDefaultQuotas sets the request quotas users get unless they have their own.
// +kubebuilder:validation:MinProperties=1
type SeerrDefaultQuotas struct {
	// movie limits movie requests.
	// +optional
	Movie SeerrQuota `json:"movie,omitzero"`

	// tv limits series requests, counted in seasons.
	// +optional
	TV SeerrQuota `json:"tv,omitzero"`
}

// SeerrQuota limits requests over a rolling window.
// +kubebuilder:validation:MinProperties=1
type SeerrQuota struct {
	// limit is how many requests count before the user is stopped. 0 is
	// unlimited.
	// +optional
	// +kubebuilder:validation:Minimum=0
	Limit *int32 `json:"limit,omitempty"`

	// days is the window, in days, that requests count toward limit. 0 counts
	// every request ever made.
	// +optional
	// +kubebuilder:validation:Minimum=0
	Days *int32 `json:"days,omitempty"`
}

// SeerrServiceConnection defines a connection to a Sonarr or Radarr instance.
// +kubebuilder:validation:XValidation:rule="has(self.appRef) ? !has(self.hostname) && !has(self.port) && !has(self.useSsl) && !has(self.baseUrl) && !has(self.apiKeySecretRef) : has(self.hostname) && has(self.port) && has(self.apiKeySecretRef)",message="set appRef, or hostname, port and apiKeySecretRef, not both"
type SeerrServiceConnection struct {
	// name is the display name for this service connection.
	// +required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name,omitempty"`

	// appRef names the server's resource, a Sonarr in sonarr or a Radarr in
	// radarr, whose connection is used in place of hostname, port, useSsl,
	// baseUrl and apiKeySecretRef.
	// +optional
	AppRef *commonv1alpha1.AppRef `json:"appRef,omitempty"`

	// hostname is the service hostname.
	// +optional
	// +kubebuilder:validation:MinLength=1
	Hostname string `json:"hostname,omitempty"`

	// port is the service port.
	// +optional
	Port *int32 `json:"port,omitempty"`

	// apiKeySecretRef references a Secret containing the service API key.
	// +optional
	APIKeySecretRef *commonv1alpha1.SecretKeyRef `json:"apiKeySecretRef,omitempty"`

	// useSsl enables SSL for the service connection.
	// +optional
	UseSsl *bool `json:"useSsl,omitempty"`

	// baseUrl is the URL base path for the service.
	// +optional
	// +kubebuilder:validation:MinLength=1
	BaseUrl string `json:"baseUrl,omitempty"`

	// activeProfileId is the quality profile's ID, used when activeProfileName
	// is not set.
	// +optional
	ActiveProfileId *int32 `json:"activeProfileId,omitempty"`

	// activeProfileName is the quality profile's name, resolved against the
	// server on each reconciliation. It takes precedence over activeProfileId.
	// A missing or ambiguous name blocks writes.
	// +optional
	// +kubebuilder:validation:MinLength=1
	ActiveProfileName string `json:"activeProfileName,omitempty"`

	// activeDirectory is the root folder path for media.
	// +optional
	// +kubebuilder:validation:MinLength=1
	ActiveDirectory string `json:"activeDirectory,omitempty"`

	// is4k indicates this is a 4K instance.
	// +optional
	Is4k *bool `json:"is4k,omitempty"`

	// isDefault indicates this is the default service for its type.
	// +optional
	IsDefault *bool `json:"isDefault,omitempty"`

	// seriesType is the series type for Sonarr (e.g., "standard", "anime").
	// Only applicable when used with Sonarr.
	// +optional
	// +kubebuilder:validation:MinLength=1
	SeriesType string `json:"seriesType,omitempty"`

	// minimumAvailability is the minimum availability for Radarr.
	// Only applicable when used with Radarr.
	// +optional
	// +kubebuilder:validation:Enum=announced;inCinemas;released
	MinimumAvailability string `json:"minimumAvailability,omitempty"`

	// enableSeasonFolders enables season folder organization in Sonarr.
	// Only applicable when used with Sonarr.
	// +optional
	EnableSeasonFolders *bool `json:"enableSeasonFolders,omitempty"`

	// externalUrl is the address users open the server at, for links in Seerr.
	// +optional
	// +kubebuilder:validation:MinLength=1
	ExternalUrl string `json:"externalUrl,omitempty"`

	// syncEnabled scans the server for media and request status.
	// +optional
	SyncEnabled *bool `json:"syncEnabled,omitempty"`

	// preventSearch stops the server from searching as soon as a request is approved.
	// +optional
	PreventSearch *bool `json:"preventSearch,omitempty"`

	// tagRequests tags each request with the requesting user.
	// +optional
	TagRequests *bool `json:"tagRequests,omitempty"`

	// activeAnimeProfileName is the quality profile for anime, resolved against
	// the server on each reconciliation. Only applicable when used with Sonarr.
	// +optional
	// +kubebuilder:validation:MinLength=1
	ActiveAnimeProfileName string `json:"activeAnimeProfileName,omitempty"`

	// activeAnimeDirectory is the root folder for anime. Only applicable when
	// used with Sonarr.
	// +optional
	// +kubebuilder:validation:MinLength=1
	ActiveAnimeDirectory string `json:"activeAnimeDirectory,omitempty"`

	// animeSeriesType is the series type for anime. Only applicable when used
	// with Sonarr.
	// +optional
	// +kubebuilder:validation:Enum=standard;daily;anime
	AnimeSeriesType string `json:"animeSeriesType,omitempty"`

	// monitorNewItems decides whether seasons added to a series later are
	// monitored. Only applicable when used with Sonarr.
	// +optional
	// +kubebuilder:validation:Enum=all;none
	MonitorNewItems string `json:"monitorNewItems,omitempty"`
}

// SeerrStatus defines the observed state of Seerr.
// +kubebuilder:validation:MinProperties=1
type SeerrStatus struct {
	// conditions represent the current state of the Seerr resource.
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

	// initialized indicates whether the Seerr initial setup has been completed.
	// +optional
	Initialized *bool `json:"initialized,omitempty"`

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

// Seerr configures a Seerr instance the operator connects to.
type Seerr struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is the standard object metadata.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec is the desired state of the Seerr.
	// +required
	Spec SeerrSpec `json:"spec,omitzero"`

	// status is the observed state of the Seerr.
	// +optional
	Status SeerrStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// SeerrList contains a list of Seerr.
type SeerrList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []Seerr `json:"items"`
}

func (c *Seerr) GetConditions() *[]metav1.Condition                  { return &c.Status.Conditions }
func (c *Seerr) GetObservedGeneration() *int64                       { return &c.Status.ObservedGeneration }
func (c *Seerr) GetLastSyncTime() **metav1.Time                      { return &c.Status.LastSyncTime }
func (c *Seerr) GetReconcileConfig() *commonv1alpha1.ReconcileConfig { return c.Spec.Reconcile }
func (c *Seerr) GetManagedResources() map[string][]string {
	return commonv1alpha1.ManagedMap(c.Status.ManagedResources)
}

func (c *Seerr) SetManagedResources(m map[string][]string) {
	c.Status.ManagedResources = commonv1alpha1.ManagedList(m)
}
