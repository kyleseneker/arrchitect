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

// ProwlarrSpec defines the desired state of Prowlarr.
type ProwlarrSpec struct {
	// connection defines how to connect to the Prowlarr instance.
	// +required
	Connection commonv1alpha1.AppConnection `json:"connection,omitempty"`

	// reconcile configures reconciliation behavior (e.g., sync interval).
	// +optional
	Reconcile *commonv1alpha1.ReconcileConfig `json:"reconcile,omitempty"`

	// applications defines application connections managed by Prowlarr.
	// +optional
	// +listType=map
	// +listMapKey=name
	Applications []ProwlarrApplication `json:"applications,omitempty"`

	// appProfiles defines indexer sync profiles, applied before indexers.
	// Profiles are not pruned because indexers outside this CR may reference them.
	// +optional
	// +listType=map
	// +listMapKey=name
	AppProfiles []ProwlarrAppProfile `json:"appProfiles,omitempty"`

	// indexers defines indexer configurations managed by Prowlarr.
	// +optional
	// +listType=map
	// +listMapKey=name
	Indexers []ProwlarrIndexer `json:"indexers,omitempty"`

	// proxies defines proxy configurations for Prowlarr.
	// +optional
	// +listType=map
	// +listMapKey=name
	Proxies []ProwlarrProxy `json:"proxies,omitempty"`

	// tags defines tags for linking resources together.
	// +optional
	// +listType=map
	// +listMapKey=label
	Tags []commonv1alpha1.Tag `json:"tags,omitempty"`

	// downloadClients defines download client connections for manual grabs.
	// +optional
	// +listType=map
	// +listMapKey=name
	DownloadClients []ProwlarrDownloadClient `json:"downloadClients,omitempty"`

	// notifications defines notification/connection configurations.
	// +optional
	// +listType=map
	// +listMapKey=name
	// +kubebuilder:validation:XValidation:rule="self.all(n, !has(n.appRef))",message="Prowlarr has no media server notifications to refer to"
	Notifications []commonv1alpha1.Notification `json:"notifications,omitempty"`
}

// ProwlarrAppProfile configures which searches an indexer supports.
type ProwlarrAppProfile struct {
	// name uniquely identifies the sync profile.
	// +kubebuilder:validation:MinLength=1
	// +required
	Name string `json:"name,omitempty"`

	// enableRss enables RSS sync.
	// +optional
	EnableRss *bool `json:"enableRss,omitempty"`

	// enableAutomaticSearch enables automatic searches.
	// +optional
	EnableAutomaticSearch *bool `json:"enableAutomaticSearch,omitempty"`

	// enableInteractiveSearch enables interactive searches.
	// +optional
	EnableInteractiveSearch *bool `json:"enableInteractiveSearch,omitempty"`

	// minimumSeeders is the minimum number of seeders for torrent results.
	// +kubebuilder:validation:Minimum=0
	// +optional
	MinimumSeeders *int32 `json:"minimumSeeders,omitempty"`
}

// ProwlarrApplication defines an application connection managed by Prowlarr.
// +kubebuilder:validation:XValidation:rule="has(self.appRef) ? !has(self.implementation) && !has(self.baseUrl) && !has(self.apiKeySecretRef) : has(self.implementation) && has(self.baseUrl) && has(self.apiKeySecretRef)",message="set appRef, or implementation, baseUrl and apiKeySecretRef, not both"
// +kubebuilder:validation:XValidation:rule="!has(self.animeSyncCategories) && !has(self.syncAnimeStandardFormatSearch) || (has(self.appRef) ? self.appRef.kind == 'Sonarr' : has(self.implementation) && self.implementation == 'Sonarr')",message="animeSyncCategories and syncAnimeStandardFormatSearch are for Sonarr applications"
type ProwlarrApplication struct {
	// name is the display name of the application.
	// +required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name,omitempty"`

	// syncLevel controls how indexers are synced to this application.
	// +optional
	// +kubebuilder:validation:Enum=disabled;addOnly;fullSync
	SyncLevel string `json:"syncLevel,omitempty"`

	// appRef names the application's resource, a Sonarr, Radarr, Lidarr or
	// Readarr, whose kind and connection are used in place of implementation,
	// baseUrl and apiKeySecretRef. prowlarrUrl then defaults to this resource's
	// connection URL.
	// +optional
	// +kubebuilder:validation:XValidation:rule="self.kind in ['Sonarr', 'Radarr', 'Lidarr', 'Readarr']",message="an application refers to a Sonarr, Radarr, Lidarr or Readarr"
	AppRef *commonv1alpha1.AppRef `json:"appRef,omitempty"`

	// implementation is the application type (e.g., Sonarr, Radarr).
	// +optional
	// +kubebuilder:validation:MinLength=1
	Implementation string `json:"implementation,omitempty"`

	// configContract is the configuration contract name for the implementation.
	// Defaults to "<implementation>Settings" if not set.
	// +optional
	// +kubebuilder:validation:MinLength=1
	ConfigContract string `json:"configContract,omitempty"`

	// prowlarrUrl is the URL Prowlarr uses to reach itself (for callbacks).
	// +optional
	// +kubebuilder:validation:MinLength=1
	ProwlarrUrl string `json:"prowlarrUrl,omitempty"`

	// baseUrl is the base URL of the application.
	// +optional
	// +kubebuilder:validation:MinLength=1
	BaseUrl string `json:"baseUrl,omitempty"`

	// apiKeySecretRef references a Secret containing the application's API key.
	// +optional
	APIKeySecretRef *commonv1alpha1.SecretKeyRef `json:"apiKeySecretRef,omitempty"`

	// authUsernameSecretRef references a Secret containing the username for
	// HTTP basic auth in front of the application, such as at a reverse proxy.
	// Needs Prowlarr 2.4.0 or later.
	// +optional
	AuthUsernameSecretRef *commonv1alpha1.SecretKeyRef `json:"authUsernameSecretRef,omitempty"`

	// authPasswordSecretRef references a Secret containing the password for
	// HTTP basic auth in front of the application. Needs Prowlarr 2.4.0 or
	// later.
	// +optional
	AuthPasswordSecretRef *commonv1alpha1.SecretKeyRef `json:"authPasswordSecretRef,omitempty"`

	// syncCategories is the list of category IDs to sync to this application.
	// +optional
	// +listType=set
	SyncCategories []int32 `json:"syncCategories,omitempty"`

	// animeSyncCategories is the list of category IDs to sync as anime to a
	// Sonarr application.
	// +optional
	// +listType=set
	AnimeSyncCategories []int32 `json:"animeSyncCategories,omitempty"`

	// syncAnimeStandardFormatSearch lets a Sonarr application also search for
	// anime by standard numbering.
	// +optional
	SyncAnimeStandardFormatSearch *bool `json:"syncAnimeStandardFormatSearch,omitempty"`

	// syncRejectBlocklistedTorrentHashesWhileGrabbing rejects a torrent the
	// application blocklisted by hash after it is grabbed, for indexers that
	// don't reject it during RSS or search.
	// +optional
	SyncRejectBlocklistedTorrentHashesWhileGrabbing *bool `json:"syncRejectBlocklistedTorrentHashesWhileGrabbing,omitempty"`

	// tagLabels references tags by label, resolved against Prowlarr on each
	// reconciliation. A missing or ambiguous label blocks writes, and an empty
	// list clears the tags.
	// +optional
	// +listType=set
	TagLabels []string `json:"tagLabels,omitempty"`
}

// ProwlarrIndexer defines an indexer configuration in Prowlarr.
type ProwlarrIndexer struct {
	// name is the display name of the indexer.
	// +required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name,omitempty"`

	// implementation is the indexer type (e.g., Newznab, Torznab).
	// +required
	// +kubebuilder:validation:MinLength=1
	Implementation string `json:"implementation,omitempty"`

	// configContract is the configuration contract name.
	// Defaults to "<implementation>Settings" if not set.
	// +optional
	// +kubebuilder:validation:MinLength=1
	ConfigContract string `json:"configContract,omitempty"`

	// enable controls whether this indexer is active. A new indexer is enabled
	// unless this is false.
	// +optional
	Enable *bool `json:"enable,omitempty"`

	// appProfileId is the sync profile's ID, used when appProfileName is not
	// set.
	// +optional
	AppProfileId *int32 `json:"appProfileId,omitempty"`

	// appProfileName is the sync profile's name, resolved against Prowlarr on
	// each reconciliation. It takes precedence over appProfileId. A missing or
	// ambiguous name blocks writes.
	// +optional
	// +kubebuilder:validation:MinLength=1
	AppProfileName string `json:"appProfileName,omitempty"`

	// priority is the indexer priority. Lower values are higher priority.
	// +optional
	Priority *int32 `json:"priority,omitempty"`

	// fields defines configuration fields for the indexer.
	// +optional
	// +listType=map
	// +listMapKey=name
	Fields []commonv1alpha1.ConfigField `json:"fields,omitempty"`

	// tagLabels references tags by label, resolved against Prowlarr on each
	// reconciliation. A missing or ambiguous label blocks writes, and an empty
	// list clears the tags.
	// +optional
	// +listType=set
	TagLabels []string `json:"tagLabels,omitempty"`
}

// ProwlarrProxy defines a proxy configuration for Prowlarr.
type ProwlarrProxy struct {
	// name is the display name of the proxy.
	// +required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name,omitempty"`

	// implementation is the proxy type (e.g., Http, Socks4, Socks5).
	// +required
	// +kubebuilder:validation:MinLength=1
	Implementation string `json:"implementation,omitempty"`

	// configContract is the configuration contract name for the implementation.
	// +required
	// +kubebuilder:validation:MinLength=1
	ConfigContract string `json:"configContract,omitempty"`

	// host is the hostname or IP of the proxy, or FlareSolverr's URL.
	// +required
	// +kubebuilder:validation:MinLength=1
	Host string `json:"host,omitempty"`

	// port is the port of an Http, Socks4 or Socks5 proxy.
	// +optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port,omitempty"`

	// usernameSecretRef references a Secret containing an Http, Socks4 or
	// Socks5 proxy's username.
	// +optional
	UsernameSecretRef *commonv1alpha1.SecretKeyRef `json:"usernameSecretRef,omitempty"`

	// passwordSecretRef references a Secret containing an Http, Socks4 or
	// Socks5 proxy's password.
	// +optional
	PasswordSecretRef *commonv1alpha1.SecretKeyRef `json:"passwordSecretRef,omitempty"`

	// requestTimeout is FlareSolverr's request timeout in seconds.
	// +optional
	RequestTimeout *int32 `json:"requestTimeout,omitempty"`

	// tagLabels references tags by label, resolved against Prowlarr on each
	// reconciliation. A missing or ambiguous label blocks writes, and an empty
	// list clears the tags.
	// +optional
	// +listType=set
	TagLabels []string `json:"tagLabels,omitempty"`
}

// ProwlarrDownloadClient defines a download client for Prowlarr manual grabs.
type ProwlarrDownloadClient struct {
	// name is the display name of the download client.
	// +required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name,omitempty"`

	// enable controls whether this download client is active. A new download
	// client is enabled unless this is false.
	// +optional
	Enable *bool `json:"enable,omitempty"`

	// protocol is the download protocol.
	// +required
	// +kubebuilder:validation:Enum=torrent;usenet
	Protocol string `json:"protocol,omitempty"`

	// implementation is the download client type (e.g., QBittorrent, Sabnzbd).
	// +required
	// +kubebuilder:validation:MinLength=1
	Implementation string `json:"implementation,omitempty"`

	// configContract is the configuration contract name.
	// Defaults to "<implementation>Settings" if not set.
	// +optional
	// +kubebuilder:validation:MinLength=1
	ConfigContract string `json:"configContract,omitempty"`

	// priority is the download client priority. Lower values are higher priority.
	// +optional
	Priority *int32 `json:"priority,omitempty"`

	// categories defines the category mappings for this download client.
	// +optional
	// +listType=map
	// +listMapKey=clientCategory
	Categories []ProwlarrDownloadClientCategory `json:"categories,omitempty"`

	// tagLabels references tags by label, resolved against Prowlarr on each
	// reconciliation. A missing or ambiguous label blocks writes, and an empty
	// list clears the tags.
	// +optional
	// +listType=set
	TagLabels []string `json:"tagLabels,omitempty"`

	// fields defines the download client's configuration fields (e.g., host, port, apiKey).
	// +optional
	// +listType=map
	// +listMapKey=name
	Fields []commonv1alpha1.ConfigField `json:"fields,omitempty"`
}

// ProwlarrDownloadClientCategory maps Prowlarr categories to download client categories.
type ProwlarrDownloadClientCategory struct {
	// clientCategory is the category name in the download client.
	// +required
	// +kubebuilder:validation:MinLength=1
	ClientCategory string `json:"clientCategory,omitempty"`

	// categories is the list of Prowlarr category IDs to map.
	// +optional
	// +listType=set
	Categories []int32 `json:"categories,omitempty"`
}

// ProwlarrStatus defines the observed state of Prowlarr.
// +kubebuilder:validation:MinProperties=1
type ProwlarrStatus struct {
	// conditions represent the current state of the Prowlarr resource.
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

// Prowlarr configures a Prowlarr instance the operator connects to.
type Prowlarr struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is the standard object metadata.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec is the desired state of the Prowlarr.
	// +required
	Spec ProwlarrSpec `json:"spec,omitzero"`

	// status is the observed state of the Prowlarr.
	// +optional
	Status ProwlarrStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// ProwlarrList contains a list of Prowlarr.
type ProwlarrList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []Prowlarr `json:"items"`
}

func (c *Prowlarr) GetConditions() *[]metav1.Condition { return &c.Status.Conditions }
func (c *Prowlarr) GetObservedGeneration() *int64      { return &c.Status.ObservedGeneration }
func (c *Prowlarr) GetLastSyncTime() **metav1.Time     { return &c.Status.LastSyncTime }
func (c *Prowlarr) GetReconcileConfig() *commonv1alpha1.ReconcileConfig {
	return c.Spec.Reconcile
}
func (c *Prowlarr) GetManagedResources() map[string][]string {
	return commonv1alpha1.ManagedMap(c.Status.ManagedResources)
}

func (c *Prowlarr) SetManagedResources(m map[string][]string) {
	c.Status.ManagedResources = commonv1alpha1.ManagedList(m)
}
