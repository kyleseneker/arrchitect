package reconciler

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
)

func TestReconcileInterval(t *testing.T) {
	tests := []struct {
		name string
		rc   *commonv1alpha1.ReconcileConfig
		want time.Duration
	}{
		{"nil config", nil, DefaultReconcileInterval},
		{"nil interval", &commonv1alpha1.ReconcileConfig{}, DefaultReconcileInterval},
		{"30s", &commonv1alpha1.ReconcileConfig{IntervalSeconds: int32(30)}, 30 * time.Second},
		{"10m", &commonv1alpha1.ReconcileConfig{IntervalSeconds: int32(600)}, 10 * time.Minute},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, reconcileInterval(tt.rc))
		})
	}
}

func TestPruneEnabled(t *testing.T) {
	tests := []struct {
		name string
		rc   *commonv1alpha1.ReconcileConfig
		want bool
	}{
		{"nil config", nil, false},
		{"nil prune", &commonv1alpha1.ReconcileConfig{}, false},
		{"false", &commonv1alpha1.ReconcileConfig{Prune: new(false)}, false},
		{"true", &commonv1alpha1.ReconcileConfig{Prune: new(true)}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, pruneEnabled(tt.rc))
		})
	}
}

func TestNextSyncStretchesTheIntervalALittle(t *testing.T) {
	for range 50 {
		got := nextSync(nil)
		assert.GreaterOrEqual(t, got, DefaultReconcileInterval)
		assert.LessOrEqual(t, got, DefaultReconcileInterval+time.Duration(float64(DefaultReconcileInterval)*jitter))
	}
}
