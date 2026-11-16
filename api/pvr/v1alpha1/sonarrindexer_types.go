package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
)

// SonarrIndexerSpec is one indexer in a Sonarr instance.
type SonarrIndexerSpec struct {
	// sonarrRef names the Sonarr this indexer belongs to, in the same namespace.
	// +required
	SonarrRef commonv1alpha1.LocalObjectReference `json:"sonarrRef"`

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
type SonarrIndexerStatus struct {
	// conditions represent the current state of the indexer.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// observedGeneration is the most recent generation observed.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// lastSyncTime is the timestamp of the last successful reconciliation.
	// +optional
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// id is the indexer's ID in Sonarr.
	// +optional
	ID int32 `json:"id,omitempty"`

	// managedResources records what the operator created for this resource,
	// so deletion only removes what it created.
	// +optional
	ManagedResources map[string][]string `json:"managedResources,omitempty"`
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
	Spec SonarrIndexerSpec `json:"spec"`

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

func (c *SonarrIndexer) GetConditions() *[]metav1.Condition        { return &c.Status.Conditions }
func (c *SonarrIndexer) GetObservedGeneration() *int64             { return &c.Status.ObservedGeneration }
func (c *SonarrIndexer) GetLastSyncTime() **metav1.Time            { return &c.Status.LastSyncTime }
func (c *SonarrIndexer) GetManagedResources() *map[string][]string { return &c.Status.ManagedResources }

// GetReconcileConfig carries only the deletion policy: the interval and drift
// policy come from the Sonarr the indexer belongs to.
func (c *SonarrIndexer) GetReconcileConfig() *commonv1alpha1.ReconcileConfig {
	return &commonv1alpha1.ReconcileConfig{DeletionPolicy: &c.Spec.DeletionPolicy}
}
