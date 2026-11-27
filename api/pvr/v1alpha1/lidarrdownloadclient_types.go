package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
)

// LidarrDownloadClientSpec is one download client in a Lidarr instance.
type LidarrDownloadClientSpec struct {
	// lidarrRef names the Lidarr this download client belongs to, in the same namespace.
	// +required
	LidarrRef commonv1alpha1.LocalObjectReference `json:"lidarrRef,omitempty"`

	// deletionPolicy controls what deleting this resource does in Lidarr:
	// "delete" removes the download client, "orphan" leaves it. One that existed before
	// the operator adopted it is never deleted.
	// +optional
	// +kubebuilder:default=delete
	// +kubebuilder:validation:Enum=delete;orphan
	DeletionPolicy string `json:"deletionPolicy,omitempty"`

	commonv1alpha1.DownloadClient `json:",inline"`
}

// LidarrDownloadClientStatus is the observed state of a LidarrDownloadClient.
// +kubebuilder:validation:MinProperties=1
type LidarrDownloadClientStatus struct {
	commonv1alpha1.ConfigStatus `json:",inline"`

	// id is the download client's ID in Lidarr.
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

// LidarrDownloadClient is one download client in a Lidarr instance.
type LidarrDownloadClient struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is the standard object metadata.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec is the desired state of the LidarrDownloadClient.
	// +required
	Spec LidarrDownloadClientSpec `json:"spec,omitzero"`

	// status is the observed state of the LidarrDownloadClient.
	// +optional
	Status LidarrDownloadClientStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// LidarrDownloadClientList contains a list of LidarrDownloadClient.
type LidarrDownloadClientList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []LidarrDownloadClient `json:"items"`
}

// GetReconcileConfig carries only the deletion policy: the interval and drift
// policy come from the Lidarr the download client belongs to.
func (c *LidarrDownloadClient) GetConfigStatus() *commonv1alpha1.ConfigStatus {
	return &c.Status.ConfigStatus
}
func (c *LidarrDownloadClient) GetReconcileConfig() *commonv1alpha1.ReconcileConfig {
	return &commonv1alpha1.ReconcileConfig{DeletionPolicy: c.Spec.DeletionPolicy}
}

// InstanceName names the Lidarr the download client belongs to.
func (c *LidarrDownloadClient) InstanceName() string { return c.Spec.LidarrRef.Name }

// ItemName is the download client's name in Lidarr.
func (c *LidarrDownloadClient) ItemName() string { return c.Spec.Name }

// SetItemID records the download client's ID in Lidarr.
func (c *LidarrDownloadClient) SetItemID(id int32) { c.Status.ID = id }

// Item is the download client as Lidarr stores it.
func (c *LidarrDownloadClient) Item() commonv1alpha1.DownloadClient { return c.Spec.DownloadClient }
