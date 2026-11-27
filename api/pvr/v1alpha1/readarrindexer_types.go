package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
)

// ReadarrIndexerSpec is one indexer in a Readarr instance.
type ReadarrIndexerSpec struct {
	// readarrRef names the Readarr this indexer belongs to, in the same namespace.
	// +required
	ReadarrRef commonv1alpha1.LocalObjectReference `json:"readarrRef,omitempty"`

	// deletionPolicy controls what deleting this resource does in Readarr:
	// "delete" removes the indexer, "orphan" leaves it. One that existed before
	// the operator adopted it is never deleted.
	// +optional
	// +kubebuilder:default=delete
	// +kubebuilder:validation:Enum=delete;orphan
	DeletionPolicy string `json:"deletionPolicy,omitempty"`

	commonv1alpha1.Indexer `json:",inline"`
}

// ReadarrIndexerStatus is the observed state of a ReadarrIndexer.
// +kubebuilder:validation:MinProperties=1
type ReadarrIndexerStatus struct {
	commonv1alpha1.ConfigStatus `json:",inline"`

	// id is the indexer's ID in Readarr.
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

// ReadarrIndexer is one indexer in a Readarr instance.
type ReadarrIndexer struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is the standard object metadata.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec is the desired state of the ReadarrIndexer.
	// +required
	Spec ReadarrIndexerSpec `json:"spec,omitzero"`

	// status is the observed state of the ReadarrIndexer.
	// +optional
	Status ReadarrIndexerStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// ReadarrIndexerList contains a list of ReadarrIndexer.
type ReadarrIndexerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []ReadarrIndexer `json:"items"`
}

// GetReconcileConfig carries only the deletion policy: the interval and drift
// policy come from the Readarr the indexer belongs to.
func (c *ReadarrIndexer) GetConfigStatus() *commonv1alpha1.ConfigStatus {
	return &c.Status.ConfigStatus
}
func (c *ReadarrIndexer) GetReconcileConfig() *commonv1alpha1.ReconcileConfig {
	return &commonv1alpha1.ReconcileConfig{DeletionPolicy: c.Spec.DeletionPolicy}
}

// InstanceName names the Readarr the indexer belongs to.
func (c *ReadarrIndexer) InstanceName() string { return c.Spec.ReadarrRef.Name }

// ItemName is the indexer's name in Readarr.
func (c *ReadarrIndexer) ItemName() string { return c.Spec.Name }

// SetItemID records the indexer's ID in Readarr.
func (c *ReadarrIndexer) SetItemID(id int32) { c.Status.ID = id }

// Item is the indexer as Readarr stores it.
func (c *ReadarrIndexer) Item() commonv1alpha1.Indexer { return c.Spec.Indexer }
