package secrets

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	"github.com/kyleseneker/arrchitect/internal/failure"
)

func newFakeClient(objs ...client.Object) client.Reader {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
}

func TestResolveSecretKeyRef_Success(t *testing.T) {
	secret := &corev1.Secret{
		Name: "my-secret", Namespace: "default",
		Data: map[string][]byte{"API_KEY": []byte("secret-value-123")},
	}
	fc := newFakeClient(secret)

	val, err := ResolveSecretKeyRef(context.Background(), fc, "default", commonv1alpha1.SecretKeyRef{
		Name: "my-secret", Key: "API_KEY",
	})
	require.NoError(t, err)
	assert.Equal(t, "secret-value-123", val)
}

func TestResolveSecretKeyRef_SecretNotFound(t *testing.T) {
	fc := newFakeClient()

	_, err := ResolveSecretKeyRef(context.Background(), fc, "default", commonv1alpha1.SecretKeyRef{
		Name: "nonexistent", Key: "key",
	})
	assert.EqualError(t, err, `secret "nonexistent" not found`)
	assert.Equal(t, failure.SecretNotFound, failure.KindOf(fmt.Errorf("wrapped: %w", err)))
}

func TestResolveSecretKeyRef_KeyNotFound(t *testing.T) {
	secret := &corev1.Secret{
		Name: "my-secret", Namespace: "default",
		Data: map[string][]byte{"OTHER_KEY": []byte("value")},
	}
	fc := newFakeClient(secret)

	_, err := ResolveSecretKeyRef(context.Background(), fc, "default", commonv1alpha1.SecretKeyRef{
		Name: "my-secret", Key: "MISSING_KEY",
	})
	assert.EqualError(t, err, `key "MISSING_KEY" not found in secret "my-secret"`)
	assert.Equal(t, failure.SecretNotFound, failure.KindOf(err))
}

// A Secret that can't be read for any other reason isn't missing: no watch
// event will come for it, so the failure backs off.
func TestResolveSecretKeyRef_ReadFailure(t *testing.T) {
	forbidden := apierrors.NewForbidden(schema.GroupResource{Resource: "secrets"}, "my-secret", errors.New("RBAC"))
	fc := fake.NewClientBuilder().WithInterceptorFuncs(interceptor.Funcs{
		Get: func(context.Context, client.WithWatch, client.ObjectKey, client.Object, ...client.GetOption) error {
			return forbidden
		},
	}).Build()

	_, err := ResolveSecretKeyRef(context.Background(), fc, "default", commonv1alpha1.SecretKeyRef{Name: "my-secret", Key: "k"})
	assert.Equal(t, failure.Transient, failure.KindOf(err))
	assert.ErrorIs(t, err, forbidden)
}
