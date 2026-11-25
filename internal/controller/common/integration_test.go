package common

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	pvrv1alpha1 "github.com/kyleseneker/arrchitect/api/pvr/v1alpha1"
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

func TestCleanupSkipsCheck(t *testing.T) {
	obj := newConfig(DeletionPolicyDelete, time.Minute)
	c := testClient(obj).WithStatusSubresource(obj).Build()
	cleaned := false
	integration := Integration[*pvrv1alpha1.Sonarr, string]{
		App:     "sonarr",
		Connect: func(context.Context, *pvrv1alpha1.Sonarr) (string, error) { return "conn", nil },
		Check: func(context.Context, string, *pvrv1alpha1.Sonarr) error {
			return engine.InvalidConfig(errors.New("the spec's whitelist would lock the operator out"))
		},
		Sync: func(context.Context, string, *pvrv1alpha1.Sonarr, *engine.Pass) []string { return nil },
		Cleanup: func(_ context.Context, conn string, owned *engine.Ownership) error {
			cleaned = conn == "conn" && owned.Owns("indexers")
			return nil
		},
	}

	_, err := integration.Reconcile(context.Background(), c, nil, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(obj)}, &pvrv1alpha1.Sonarr{})

	require.NoError(t, err)
	assert.True(t, cleaned, "a spec problem Check finds must not block deleting what the operator created")
}

func TestDeletionWithNothingDeletableNeedsNoConnection(t *testing.T) {
	obj := newConfig(DeletionPolicyDelete, time.Minute)
	c := testClient(obj).WithStatusSubresource(obj).Build()
	integration := Integration[*pvrv1alpha1.Sonarr, string]{
		App: "sonarr",
		Connect: func(context.Context, *pvrv1alpha1.Sonarr) (string, error) {
			return "", errors.New("the API key Secret is already gone")
		},
		Sync:      func(context.Context, string, *pvrv1alpha1.Sonarr, *engine.Pass) []string { return nil },
		Cleanup:   func(context.Context, string, *engine.Ownership) error { return nil },
		Deletable: func(*engine.Ownership) bool { return false },
	}

	_, err := integration.Reconcile(context.Background(), c, nil, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(obj)}, &pvrv1alpha1.Sonarr{})

	require.NoError(t, err)
	err = c.Get(context.Background(), client.ObjectKeyFromObject(obj), &pvrv1alpha1.Sonarr{})
	assert.True(t, apierrors.IsNotFound(err), "the finalizer is released without reading credentials, so the resource is gone")
}
