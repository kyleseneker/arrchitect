package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
)

// RadarrDownloadClientSpec is one download client in a Radarr instance.
type RadarrDownloadClientSpec struct {
	// radarrRef names the Radarr this download client belongs to, in the same namespace.
	// +required
	RadarrRef commonv1alpha1.LocalObjectReference `json:"radarrRef,omitempty"`

	// deletionPolicy controls what deleting this resource does in Radarr:
	// "delete" removes the download client, "orphan" leaves it. One that existed before
	// the operator adopted it is never deleted.
	// +optional
	// +kubebuilder:default=delete
	// +kubebuilder:validation:Enum=delete;orphan
	DeletionPolicy string `json:"deletionPolicy,omitempty"`

	commonv1alpha1.DownloadClient `json:",inline"`
}

// RadarrDownloadClientStatus is the observed state of a RadarrDownloadClient.
// +kubebuilder:validation:MinProperties=1
type RadarrDownloadClientStatus struct {
	commonv1alpha1.ConfigStatus `json:",inline"`

	// id is the download client's ID in Radarr.
	// +optional
	// +kubebuilder:validation:Minimum=1
	ID int32 `json:"id,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=arrchitect
// +kubebuilder:printcolumn:name="Synced",type="string",JSONPath=`.status.conditions[?(@.type=="Synced")].status`
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="ID",type="integer",JSONPath=`.status.id`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=`.metadata.creationTimestamp`

// RadarrDownloadClient is one download client in a Radarr instance.
type RadarrDownloadClient struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is the standard object metadata.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec is the desired state of the RadarrDownloadClient.
	// +required
	Spec RadarrDownloadClientSpec `json:"spec,omitzero"`

	// status is the observed state of the RadarrDownloadClient.
	// +optional
	Status RadarrDownloadClientStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// RadarrDownloadClientList contains a list of RadarrDownloadClient.
type RadarrDownloadClientList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []RadarrDownloadClient `json:"items"`
}

// GetReconcileConfig carries only the deletion policy: the interval and drift
// policy come from the Radarr the download client belongs to.
func (c *RadarrDownloadClient) GetConfigStatus() *commonv1alpha1.ConfigStatus {
	return &c.Status.ConfigStatus
}
func (c *RadarrDownloadClient) GetReconcileConfig() *commonv1alpha1.ReconcileConfig {
	return &commonv1alpha1.ReconcileConfig{DeletionPolicy: c.Spec.DeletionPolicy}
}

// InstanceName names the Radarr the download client belongs to.
func (c *RadarrDownloadClient) InstanceName() string { return c.Spec.RadarrRef.Name }

// ItemName is the download client's name in Radarr.
func (c *RadarrDownloadClient) ItemName() string { return c.Spec.Name }

// SetItemID records the download client's ID in Radarr.
func (c *RadarrDownloadClient) SetItemID(id int32) { c.Status.ID = id }

// Item is the download client as Radarr stores it.
func (c *RadarrDownloadClient) Item() commonv1alpha1.DownloadClient { return c.Spec.DownloadClient }
