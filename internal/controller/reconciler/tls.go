package reconciler

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	"github.com/kyleseneker/arrchitect/internal/failure"
	"github.com/kyleseneker/arrchitect/internal/secrets"
)

// resolveTLSConfig builds a *tls.Config from a TLSConfig CRD spec.
// Returns nil if tlsCfg is nil and no TLS customization is needed.
func resolveTLSConfig(ctx context.Context, c client.Reader, namespace string, tlsCfg *commonv1alpha1.TLSConfig) (*tls.Config, error) {
	if tlsCfg == nil {
		return nil, nil
	}

	cfg := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	if tlsCfg.InsecureSkipVerify != nil && *tlsCfg.InsecureSkipVerify {
		cfg.InsecureSkipVerify = true
	}

	if tlsCfg.CASecretRef != (commonv1alpha1.SecretKeyRef{}) {
		caPEM, err := secrets.ResolveSecretKeyRef(ctx, c, namespace, tlsCfg.CASecretRef)
		if err != nil {
			return nil, fmt.Errorf("resolving CA certificate secret: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM([]byte(caPEM)) {
			return nil, failure.InvalidConfig.Errorf("CA certificate secret %s/%s key %q contains no valid PEM certificates", namespace, tlsCfg.CASecretRef.Name, tlsCfg.CASecretRef.Key)
		}
		cfg.RootCAs = pool
	}

	return cfg, nil
}
