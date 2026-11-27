package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
)

// LidarrIndexerSpec is one indexer in a Lidarr instance.
type LidarrIndexerSpec struct {
	// lidarrRef names the Lidarr this indexer belongs to, in the same namespace.
	// +required
	LidarrRef commonv1alpha1.LocalObjectReference `json:"lidarrRef,omitempty"`

	// deletionPolicy controls what deleting this resource does in Lidarr:
	// "delete" removes the indexer, "orphan" leaves it. One that existed before
	// the operator adopted it is never deleted.
	// +optional
	// +kubebuilder:default=delete
	// +kubebuilder:validation:Enum=delete;orphan
	DeletionPolicy string `json:"deletionPolicy,omitempty"`

	commonv1alpha1.Indexer `json:",inline"`
}

// LidarrIndexerStatus is the observed state of a LidarrIndexer.
// +kubebuilder:validation:MinProperties=1
type LidarrIndexerStatus struct {
	commonv1alpha1.ConfigStatus `json:",inline"`

	// id is the indexer's ID in Lidarr.
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

// LidarrIndexer is one indexer in a Lidarr instance.
type LidarrIndexer struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is the standard object metadata.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec is the desired state of the LidarrIndexer.
	// +required
	Spec LidarrIndexerSpec `json:"spec,omitzero"`

	// status is the observed state of the LidarrIndexer.
	// +optional
	Status LidarrIndexerStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// LidarrIndexerList contains a list of LidarrIndexer.
type LidarrIndexerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []LidarrIndexer `json:"items"`
}

// GetReconcileConfig carries only the deletion policy: the interval and drift
// policy come from the Lidarr the indexer belongs to.
func (c *LidarrIndexer) GetConfigStatus() *commonv1alpha1.ConfigStatus { return &c.Status.ConfigStatus }
func (c *LidarrIndexer) GetReconcileConfig() *commonv1alpha1.ReconcileConfig {
	return &commonv1alpha1.ReconcileConfig{DeletionPolicy: c.Spec.DeletionPolicy}
}

// InstanceName names the Lidarr the indexer belongs to.
func (c *LidarrIndexer) InstanceName() string { return c.Spec.LidarrRef.Name }

// ItemName is the indexer's name in Lidarr.
func (c *LidarrIndexer) ItemName() string { return c.Spec.Name }

// SetItemID records the indexer's ID in Lidarr.
func (c *LidarrIndexer) SetItemID(id int32) { c.Status.ID = id }

// Item is the indexer as Lidarr stores it.
func (c *LidarrIndexer) Item() commonv1alpha1.Indexer { return c.Spec.Indexer }
