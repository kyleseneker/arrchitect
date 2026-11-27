// Package secrets reads values from Kubernetes Secrets, keeps them out of
// errors and logs, and remembers the masked secrets the operator wrote.
package secrets

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	"github.com/kyleseneker/arrchitect/internal/failure"
)

// ResolveSecretKeyRef reads a value from a Kubernetes Secret. A missing Secret
// or key is a SecretNotFound failure, which a watch on the Secret retries. Any
// other failure to read it is Transient, which backs off.
func ResolveSecretKeyRef(ctx context.Context, c client.Reader, namespace string, ref commonv1alpha1.SecretKeyRef) (string, error) {
	secret := &corev1.Secret{}
	err := c.Get(ctx, client.ObjectKey{
		Namespace: namespace,
		Name:      ref.Name,
	}, secret)
	if apierrors.IsNotFound(err) {
		return "", failure.SecretNotFound.Errorf("secret %q not found", ref.Name)
	}
	if err != nil {
		return "", failure.Transient.Errorf("reading secret %q: %w", ref.Name, err)
	}

	val, ok := secret.Data[ref.Key]
	if !ok {
		return "", failure.SecretNotFound.Errorf("key %q not found in secret %q", ref.Key, ref.Name)
	}

	RememberSecretAs(namespace+"/"+ref.Name+"/"+ref.Key, string(val))
	return string(val), nil
}
