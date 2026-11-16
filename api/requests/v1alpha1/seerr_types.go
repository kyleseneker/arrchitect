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

	// main configures general application settings.
	// +optional
	Main *SeerrMain `json:"main,omitempty"`

	// sonarr lists the Sonarr servers Seerr sends TV requests to, matched by
	// name. Seerr uses the default server, and the default 4K server for 4K
	// requests.
	// +optional
	// +listType=map
	// +listMapKey=name
	Sonarr []SeerrServiceConnection `json:"sonarr,omitempty"`

	// radarr lists the Radarr servers Seerr sends movie requests to, matched by
	// name. Seerr uses the default server, and the default 4K server for 4K
	// requests.
	// +optional
	// +listType=map
	// +listMapKey=name
	Radarr []SeerrServiceConnection `json:"radarr,omitempty"`

	// notifications configures notification agents (Discord, Slack, Telegram, etc.).
	// Each agent is a singleton — only one configuration per agent type.
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
	// url is the base URL of the Seerr instance (e.g., http://jellyseerr.media.svc.cluster.local:5055).
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
type SeerrPlexAuth struct {
	// hostname is the Plex server hostname.
	// +required
	// +kubebuilder:validation:MinLength=1
	Hostname string `json:"hostname,omitempty"`

	// port is the Plex server port.
	// +optional
	// +kubebuilder:default=32400
	Port *int32 `json:"port,omitempty"`

	// useSsl enables SSL for the Plex connection.
	// +optional
	UseSsl *bool `json:"useSsl,omitempty"`

	// tokenSecretRef references a Secret containing the Plex auth token.
	// +required
	TokenSecretRef commonv1alpha1.SecretKeyRef `json:"tokenSecretRef,omitempty"`
}

// SeerrJellyfinAuth defines the Jellyfin authentication configuration for Seerr.
type SeerrJellyfinAuth struct {
	// hostname is the Jellyfin server hostname.
	// +required
	// +kubebuilder:validation:MinLength=1
	Hostname string `json:"hostname,omitempty"`

	// port is the Jellyfin server port.
	// +required
	Port *int32 `json:"port,omitempty"`

	// useSsl enables SSL for the Jellyfin connection.
	// +optional
	UseSsl *bool `json:"useSsl,omitempty"`

	// urlBase is the subpath Jellyfin is served under, if any.
	// +optional
	// +kubebuilder:validation:MinLength=1
	UrlBase string `json:"urlBase,omitempty"`

	// usernameSecretRef references a Secret containing the Jellyfin username.
	// +required
	UsernameSecretRef commonv1alpha1.SecretKeyRef `json:"usernameSecretRef,omitempty"`

	// passwordSecretRef references a Secret containing the Jellyfin password.
	// +required
	PasswordSecretRef commonv1alpha1.SecretKeyRef `json:"passwordSecretRef,omitempty"`
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
}

// SeerrServiceConnection defines a connection to a Sonarr or Radarr instance.
type SeerrServiceConnection struct {
	// name is the display name for this service connection.
	// +required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name,omitempty"`

	// hostname is the service hostname.
	// +required
	// +kubebuilder:validation:MinLength=1
	Hostname string `json:"hostname,omitempty"`

	// port is the service port.
	// +required
	Port *int32 `json:"port,omitempty"`

	// apiKeySecretRef references a Secret containing the service API key.
	// +required
	APIKeySecretRef commonv1alpha1.SecretKeyRef `json:"apiKeySecretRef,omitempty"`

	// useSsl enables SSL for the service connection.
	// +optional
	UseSsl *bool `json:"useSsl,omitempty"`

	// baseUrl is the URL base path for the service.
	// +optional
	// +kubebuilder:validation:MinLength=1
	BaseUrl string `json:"baseUrl,omitempty"`

	// activeProfileId is used when activeProfileName is omitted.
	// +optional
	ActiveProfileId *int32 `json:"activeProfileId,omitempty"`

	// activeProfileName is resolved against the service on each reconciliation.
	// When set, it takes precedence over activeProfileId; exactly one match is required.
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
