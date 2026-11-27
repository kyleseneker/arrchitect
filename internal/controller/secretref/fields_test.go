package secretref

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	"github.com/kyleseneker/arrchitect/internal/failure"
)

func newSecretClient() *fake.ClientBuilder {
	return fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(&corev1.Secret{
		Name: "tracker", Namespace: "arr",
		Data: map[string][]byte{"passkey": []byte("s3cr3t")},
	})
}

func TestResolveConfigFieldsFromSecret(t *testing.T) {
	c := newSecretClient().Build()
	fields := []commonv1alpha1.ConfigField{
		{Name: "apiKey", ValueFrom: commonv1alpha1.ValueSource{SecretKeyRef: commonv1alpha1.SecretKeyRef{Name: "tracker", Key: "passkey"}}},
	}
	out, err := ResolveConfigFields(context.Background(), c, "arr", "indexer x", fields)
	require.NoError(t, err)
	assert.Zero(t, out[0].ValueFrom, "valueFrom should be cleared after resolution")
	b, _ := json.Marshal(out[0].Value)
	assert.Equal(t, `"s3cr3t"`, string(b), "expected quoted secret value")
	assert.Nil(t, fields[0].Value, "input slice was mutated")
}

func TestResolveConfigFieldsRejectsBothValueAndValueFrom(t *testing.T) {
	c := newSecretClient().Build()
	raw, _ := json.Marshal("literal")
	fields := []commonv1alpha1.ConfigField{{
		Name:      "apiKey",
		Value:     &commonv1alpha1.FieldValue{Raw: raw},
		ValueFrom: commonv1alpha1.ValueSource{SecretKeyRef: commonv1alpha1.SecretKeyRef{Name: "tracker", Key: "passkey"}},
	}}
	_, err := ResolveConfigFields(context.Background(), c, "arr", "indexer x", fields)
	require.Error(t, err, "expected an error when both value and valueFrom are set")
	assert.True(t, failure.IsConfigError(err), "only a spec change fixes it")
}

func TestResolveConfigFieldsMissingSecret(t *testing.T) {
	c := newSecretClient().Build()
	fields := []commonv1alpha1.ConfigField{
		{Name: "apiKey", ValueFrom: commonv1alpha1.ValueSource{SecretKeyRef: commonv1alpha1.SecretKeyRef{Name: "nope", Key: "passkey"}}},
	}
	_, err := ResolveConfigFields(context.Background(), c, "arr", "indexer x", fields)
	require.Error(t, err, "expected an error for a missing secret")
}
