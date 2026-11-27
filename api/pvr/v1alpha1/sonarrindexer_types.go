package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
)

// SonarrIndexerSpec is one indexer in a Sonarr instance.
type SonarrIndexerSpec struct {
	// sonarrRef names the Sonarr this indexer belongs to, in the same namespace.
	// +required
	SonarrRef commonv1alpha1.LocalObjectReference `json:"sonarrRef,omitempty"`

	// deletionPolicy controls what deleting this resource does in Sonarr:
	// "delete" removes the indexer, "orphan" leaves it. One that existed before
	// the operator adopted it is never deleted.
	// +optional
	// +kubebuilder:default=delete
	// +kubebuilder:validation:Enum=delete;orphan
	DeletionPolicy string `json:"deletionPolicy,omitempty"`

	commonv1alpha1.Indexer `json:",inline"`
}

// SonarrIndexerStatus is the observed state of a SonarrIndexer.
// +kubebuilder:validation:MinProperties=1
type SonarrIndexerStatus struct {
	commonv1alpha1.ConfigStatus `json:",inline"`

	// id is the indexer's ID in Sonarr.
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

// SonarrIndexer is one indexer in a Sonarr instance.
type SonarrIndexer struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is the standard object metadata.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec is the desired state of the SonarrIndexer.
	// +required
	Spec SonarrIndexerSpec `json:"spec,omitzero"`

	// status is the observed state of the SonarrIndexer.
	// +optional
	Status SonarrIndexerStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// SonarrIndexerList contains a list of SonarrIndexer.
type SonarrIndexerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []SonarrIndexer `json:"items"`
}

// GetReconcileConfig carries only the deletion policy: the interval and drift
// policy come from the Sonarr the indexer belongs to.
func (c *SonarrIndexer) GetConfigStatus() *commonv1alpha1.ConfigStatus { return &c.Status.ConfigStatus }
func (c *SonarrIndexer) GetReconcileConfig() *commonv1alpha1.ReconcileConfig {
	return &commonv1alpha1.ReconcileConfig{DeletionPolicy: c.Spec.DeletionPolicy}
}

// InstanceName names the Sonarr the indexer belongs to.
func (c *SonarrIndexer) InstanceName() string { return c.Spec.SonarrRef.Name }

// ItemName is the indexer's name in Sonarr.
func (c *SonarrIndexer) ItemName() string { return c.Spec.Name }

// SetItemID records the indexer's ID in Sonarr.
func (c *SonarrIndexer) SetItemID(id int32) { c.Status.ID = id }

// Item is the indexer as Sonarr stores it.
func (c *SonarrIndexer) Item() commonv1alpha1.Indexer { return c.Spec.Indexer }
