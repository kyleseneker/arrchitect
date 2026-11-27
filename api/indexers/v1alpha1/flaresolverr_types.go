package v1alpha1

import (
	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// FlareSolverrSpec defines the desired state of FlareSolverr.
type FlareSolverrSpec struct {
	// connection defines how to connect to the FlareSolverr instance.
	// FlareSolverr does not require authentication by default.
	// +required
	Connection FlareSolverrConnection `json:"connection,omitzero"`

	// reconcile configures reconciliation behavior (e.g., sync interval).
	// +optional
	Reconcile *commonv1alpha1.ReconcileConfig `json:"reconcile,omitempty"`

	// sessions defines persistent browser sessions to maintain.
	// Named sessions are reused across requests and survive between solves,
	// improving performance for repeated requests to the same site.
	// +optional
	// +listType=map
	// +listMapKey=name
	Sessions []FlareSolverrSession `json:"sessions,omitempty"`
}

// FlareSolverrConnection defines how the operator connects to a FlareSolverr instance.
// FlareSolverr typically has no authentication; it is expected to be network-isolated.
type FlareSolverrConnection struct {
	// url is the base URL of the FlareSolverr instance (e.g., http://flaresolverr.media.svc.cluster.local:8191).
	// +required
	// +kubebuilder:validation:Pattern=`^https?://`
	// +kubebuilder:validation:MinLength=1
	URL string `json:"url,omitempty"`

	// tls configures TLS settings for the connection.
	// Only needed when FlareSolverr uses HTTPS with self-signed or private CA certificates.
	// +optional
	TLS *commonv1alpha1.TLSConfig `json:"tls,omitempty"`
}

// FlareSolverrSession defines a named browser session to maintain in FlareSolverr.
type FlareSolverrSession struct {
	// name is the session identifier. Must be unique across sessions. A client
	// reuses the session by sending this name with its requests; Prowlarr's
	// FlareSolverr proxy doesn't.
	// +required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name,omitempty"`
}

// FlareSolverrStatus defines the observed state of FlareSolverr.
// +kubebuilder:validation:MinProperties=1
type FlareSolverrStatus struct {
	commonv1alpha1.ConfigStatus `json:",inline"`

	// activeSessions is the number of active sessions managed by this resource.
	// +optional
	// +kubebuilder:validation:Minimum=1
	ActiveSessions int32 `json:"activeSessions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=arrchitect
// +kubebuilder:printcolumn:name="Synced",type="string",JSONPath=`.status.conditions[?(@.type=="Synced")].status`
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Sessions",type="integer",JSONPath=`.status.activeSessions`
// +kubebuilder:printcolumn:name="Last Sync",type="date",JSONPath=`.status.lastSyncTime`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=`.metadata.creationTimestamp`

// FlareSolverr configures a FlareSolverr instance the operator connects to.
type FlareSolverr struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is the standard object metadata.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec is the desired state of the FlareSolverr.
	// +required
	Spec FlareSolverrSpec `json:"spec,omitzero"`

	// status is the observed state of the FlareSolverr.
	// +optional
	Status FlareSolverrStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// FlareSolverrList contains a list of FlareSolverr.
type FlareSolverrList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []FlareSolverr `json:"items"`
}

func (c *FlareSolverr) GetConfigStatus() *commonv1alpha1.ConfigStatus { return &c.Status.ConfigStatus }
func (c *FlareSolverr) GetReconcileConfig() *commonv1alpha1.ReconcileConfig {
	return c.Spec.Reconcile
}
