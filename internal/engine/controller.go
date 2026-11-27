package engine

import "time"

const (
	DefaultReconcileInterval = 5 * time.Minute
	ConditionSynced          = "Synced"
	ConditionReady           = "Ready"
	ConditionStalled         = "Stalled"
	ReasonSynced             = "Synced"
	ReasonSyncFailed         = "SyncFailed"
	ReasonAppUnreachable     = "AppUnreachable"
	ReasonUnauthorized       = "Unauthorized"
	ReasonSecretNotFound     = "SecretNotFound"
	ReasonInvalidConfig      = "InvalidConfig"
	ReasonReferenceNotFound  = "ReferenceNotFound"
	ReasonNameConflict       = "NameConflict"
	ReasonDriftDetected      = "DriftDetected"
	ReasonObserved           = "Observed"
)

const (
	ReasonResourcePruned    = "ResourcePruned"
	ReasonResourceKept      = "ResourceKept"
	ReasonCleanupFailed     = "CleanupFailed"
	ReasonFinalizerReleased = "FinalizerReleased"
	ReasonInstanceMissing   = "InstanceMissing"
)
