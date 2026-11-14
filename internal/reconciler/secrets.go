package reconciler

import (
	"context"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1alpha1 "github.com/kyleseneker/media-operator/api/common/v1alpha1"
)

// SecretError is a value that couldn't be read from a Secret: the Secret or
// its key is missing, or reading it failed.
type SecretError struct {
	msg string
	err error
}

func (e *SecretError) Error() string { return e.msg }
func (e *SecretError) Unwrap() error { return e.err }

// IsSecretError reports whether err came from reading a Secret rather than
// from the app.
func IsSecretError(err error) bool {
	var s *SecretError
	return errors.As(err, &s)
}

// ResolveSecretKeyRef reads a value from a Kubernetes Secret.
func ResolveSecretKeyRef(ctx context.Context, c client.Reader, namespace string, ref commonv1alpha1.SecretKeyRef) (string, error) {
	secret := &corev1.Secret{}
	err := c.Get(ctx, client.ObjectKey{
		Namespace: namespace,
		Name:      ref.Name,
	}, secret)
	if err != nil {
		return "", &SecretError{msg: fmt.Sprintf("secret %q not found: %v", ref.Name, err), err: err}
	}

	val, ok := secret.Data[ref.Key]
	if !ok {
		return "", &SecretError{msg: fmt.Sprintf("key %q not found in secret %q", ref.Key, ref.Name)}
	}

	return string(val), nil
}
