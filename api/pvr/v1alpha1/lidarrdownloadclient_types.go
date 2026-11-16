package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
)

// LidarrDownloadClientSpec is one download client in a Lidarr instance.
type LidarrDownloadClientSpec struct {
	// lidarrRef names the Lidarr this download client belongs to, in the same namespace.
	// +required
	LidarrRef commonv1alpha1.LocalObjectReference `json:"lidarrRef"`

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
type LidarrDownloadClientStatus struct {
	// conditions represent the current state of the download client.
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

	// id is the download client's ID in Lidarr.
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

// LidarrDownloadClient is one download client in a Lidarr instance.
type LidarrDownloadClient struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is the standard object metadata.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec is the desired state of the LidarrDownloadClient.
	// +required
	Spec LidarrDownloadClientSpec `json:"spec"`

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

func (c *LidarrDownloadClient) GetConditions() *[]metav1.Condition { return &c.Status.Conditions }
func (c *LidarrDownloadClient) GetObservedGeneration() *int64      { return &c.Status.ObservedGeneration }
func (c *LidarrDownloadClient) GetLastSyncTime() **metav1.Time     { return &c.Status.LastSyncTime }
func (c *LidarrDownloadClient) GetManagedResources() *map[string][]string {
	return &c.Status.ManagedResources
}

// GetReconcileConfig carries only the deletion policy: the interval and drift
// policy come from the Lidarr the download client belongs to.
func (c *LidarrDownloadClient) GetReconcileConfig() *commonv1alpha1.ReconcileConfig {
	return &commonv1alpha1.ReconcileConfig{DeletionPolicy: &c.Spec.DeletionPolicy}
}
