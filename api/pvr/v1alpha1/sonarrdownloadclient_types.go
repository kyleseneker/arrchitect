package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
)

// SonarrDownloadClientSpec is one download client in a Sonarr instance.
type SonarrDownloadClientSpec struct {
	// sonarrRef names the Sonarr this download client belongs to, in the same namespace.
	// +required
	SonarrRef commonv1alpha1.LocalObjectReference `json:"sonarrRef,omitempty"`

	// deletionPolicy controls what deleting this resource does in Sonarr:
	// "delete" removes the download client, "orphan" leaves it. One that existed before
	// the operator adopted it is never deleted.
	// +optional
	// +kubebuilder:default=delete
	// +kubebuilder:validation:Enum=delete;orphan
	DeletionPolicy string `json:"deletionPolicy,omitempty"`

	commonv1alpha1.DownloadClient `json:",inline"`
}

// SonarrDownloadClientStatus is the observed state of a SonarrDownloadClient.
// +kubebuilder:validation:MinProperties=1
type SonarrDownloadClientStatus struct {
	// conditions represent the current state of the download client.
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

	// id is the download client's ID in Sonarr.
	// +optional
	// +kubebuilder:validation:Minimum=1
	ID int32 `json:"id,omitempty"`

	// managedResources records what the operator created for this resource,
	// so deletion only removes what it created.
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
// +kubebuilder:printcolumn:name="ID",type="integer",JSONPath=`.status.id`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=`.metadata.creationTimestamp`

// SonarrDownloadClient is one download client in a Sonarr instance.
type SonarrDownloadClient struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is the standard object metadata.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec is the desired state of the SonarrDownloadClient.
	// +required
	Spec SonarrDownloadClientSpec `json:"spec,omitzero"`

	// status is the observed state of the SonarrDownloadClient.
	// +optional
	Status SonarrDownloadClientStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// SonarrDownloadClientList contains a list of SonarrDownloadClient.
type SonarrDownloadClientList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []SonarrDownloadClient `json:"items"`
}

func (c *SonarrDownloadClient) GetConditions() *[]metav1.Condition { return &c.Status.Conditions }
func (c *SonarrDownloadClient) GetObservedGeneration() *int64      { return &c.Status.ObservedGeneration }
func (c *SonarrDownloadClient) GetLastSyncTime() **metav1.Time     { return &c.Status.LastSyncTime }
func (c *SonarrDownloadClient) GetManagedResources() map[string][]string {
	return commonv1alpha1.ManagedMap(c.Status.ManagedResources)
}

func (c *SonarrDownloadClient) SetManagedResources(m map[string][]string) {
	c.Status.ManagedResources = commonv1alpha1.ManagedList(m)
}

// GetReconcileConfig carries only the deletion policy: the interval and drift
// policy come from the Sonarr the download client belongs to.
func (c *SonarrDownloadClient) GetReconcileConfig() *commonv1alpha1.ReconcileConfig {
	return &commonv1alpha1.ReconcileConfig{DeletionPolicy: c.Spec.DeletionPolicy}
}

// InstanceName names the Sonarr the download client belongs to.
func (c *SonarrDownloadClient) InstanceName() string { return c.Spec.SonarrRef.Name }

// ItemName is the download client's name in Sonarr.
func (c *SonarrDownloadClient) ItemName() string { return c.Spec.Name }

// SetItemID records the download client's ID in Sonarr.
func (c *SonarrDownloadClient) SetItemID(id int32) { c.Status.ID = id }

// Item is the download client as Sonarr stores it.
func (c *SonarrDownloadClient) Item() commonv1alpha1.DownloadClient { return c.Spec.DownloadClient }
