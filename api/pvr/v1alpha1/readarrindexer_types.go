package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
)

// ReadarrIndexerSpec is one indexer in a Readarr instance.
type ReadarrIndexerSpec struct {
	// readarrRef names the Readarr this indexer belongs to, in the same namespace.
	// +required
	ReadarrRef commonv1alpha1.LocalObjectReference `json:"readarrRef"`

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
type ReadarrIndexerStatus struct {
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

	// id is the indexer's ID in Readarr.
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

// ReadarrIndexer is one indexer in a Readarr instance.
type ReadarrIndexer struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is the standard object metadata.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec is the desired state of the ReadarrIndexer.
	// +required
	Spec ReadarrIndexerSpec `json:"spec"`

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

func (c *ReadarrIndexer) GetConditions() *[]metav1.Condition { return &c.Status.Conditions }
func (c *ReadarrIndexer) GetObservedGeneration() *int64      { return &c.Status.ObservedGeneration }
func (c *ReadarrIndexer) GetLastSyncTime() **metav1.Time     { return &c.Status.LastSyncTime }
func (c *ReadarrIndexer) GetManagedResources() *map[string][]string {
	return &c.Status.ManagedResources
}

// GetReconcileConfig carries only the deletion policy: the interval and drift
// policy come from the Readarr the indexer belongs to.
func (c *ReadarrIndexer) GetReconcileConfig() *commonv1alpha1.ReconcileConfig {
	return &commonv1alpha1.ReconcileConfig{DeletionPolicy: &c.Spec.DeletionPolicy}
}
