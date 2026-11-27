package common

import (
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
)

// ConfigResource is the common interface implemented by all arrchitect config CRDs.
// It provides access to the status fields that every config resource shares.
type ConfigResource interface {
	client.Object
	GetConfigStatus() *commonv1alpha1.ConfigStatus
	GetReconcileConfig() *commonv1alpha1.ReconcileConfig
}
