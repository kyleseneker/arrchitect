package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
)

// RadarrIndexerSpec is one indexer in a Radarr instance.
type RadarrIndexerSpec struct {
	// radarrRef names the Radarr this indexer belongs to, in the same namespace.
	// +required
	RadarrRef commonv1alpha1.LocalObjectReference `json:"radarrRef"`

	// deletionPolicy controls what deleting this resource does in Radarr:
	// "delete" removes the indexer, "orphan" leaves it. One that existed before
	// the operator adopted it is never deleted.
	// +optional
	// +kubebuilder:default=delete
	// +kubebuilder:validation:Enum=delete;orphan
	DeletionPolicy string `json:"deletionPolicy,omitempty"`

	commonv1alpha1.Indexer `json:",inline"`
}

// RadarrIndexerStatus is the observed state of a RadarrIndexer.
type RadarrIndexerStatus struct {
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

	// id is the indexer's ID in Radarr.
	// +optional
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

// RadarrIndexer is one indexer in a Radarr instance.
type RadarrIndexer struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is the standard object metadata.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec is the desired state of the RadarrIndexer.
	// +required
	Spec RadarrIndexerSpec `json:"spec"`

	// status is the observed state of the RadarrIndexer.
	// +optional
	Status RadarrIndexerStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// RadarrIndexerList contains a list of RadarrIndexer.
type RadarrIndexerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []RadarrIndexer `json:"items"`
}

func (c *RadarrIndexer) GetConditions() *[]metav1.Condition { return &c.Status.Conditions }
func (c *RadarrIndexer) GetObservedGeneration() *int64      { return &c.Status.ObservedGeneration }
func (c *RadarrIndexer) GetLastSyncTime() **metav1.Time     { return &c.Status.LastSyncTime }
func (c *RadarrIndexer) GetManagedResources() map[string][]string {
	return commonv1alpha1.ManagedMap(c.Status.ManagedResources)
}

func (c *RadarrIndexer) SetManagedResources(m map[string][]string) {
	c.Status.ManagedResources = commonv1alpha1.ManagedList(m)
}

// GetReconcileConfig carries only the deletion policy: the interval and drift
// policy come from the Radarr the indexer belongs to.
func (c *RadarrIndexer) GetReconcileConfig() *commonv1alpha1.ReconcileConfig {
	return &commonv1alpha1.ReconcileConfig{DeletionPolicy: &c.Spec.DeletionPolicy}
}
