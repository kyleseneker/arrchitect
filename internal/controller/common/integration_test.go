package common

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	"github.com/kyleseneker/arrchitect/internal/engine"
	"github.com/kyleseneker/arrchitect/internal/reconciler"
)

func TestClassify(t *testing.T) {
	_, missingSecret := reconciler.ResolveSecretKeyRef(context.Background(), fake.NewClientBuilder().Build(), "media",
		commonv1alpha1.SecretKeyRef{Name: "sonarr-credentials", Key: "api-key"})

	for _, tc := range []struct {
		name   string
		err    error
		reason string
		after  time.Duration
	}{
		{"missing appRef target", fmt.Errorf("Sonarr %q %w", "sonarr", errAppRefNotFound), engine.ReasonReferenceNotFound, 30 * time.Second},
		{"appRef that can't be followed", InvalidAppRef("Plex %q: unsupported kind", "plex"), engine.ReasonInvalidConfig, 0},
		{"spec problem", engine.InvalidConfig(errors.New("bad URL")), engine.ReasonInvalidConfig, 0},
		{"missing Secret", fmt.Errorf("authentication: %w", missingSecret), engine.ReasonSecretNotFound, 30 * time.Second},
		{"anything else", errors.New("connection refused"), engine.ReasonAppUnreachable, 60 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reason, after := Classify(tc.err)
			assert.Equal(t, tc.reason, reason)
			assert.Equal(t, tc.after, after)
		})
	}
}
