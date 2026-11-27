package reconciler

import (
	"context"
	"slices"

	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	"github.com/kyleseneker/arrchitect/internal/httpapi"
	"github.com/kyleseneker/arrchitect/internal/secrets"
)

// Endpoint is where an app is and how its client signs requests.
type Endpoint struct {
	// App labels the client's requests in metrics, such as "sonarr".
	App  string
	URL  string
	TLS  *commonv1alpha1.TLSConfig
	Auth httpapi.AuthType
	// Key names the Secret holding the API key, or Plex token, that Auth
	// sends. Nil when the client signs in another way.
	Key *commonv1alpha1.SecretKeyRef
	// Options are the app's own client options, such as a header or timeout.
	Options []httpapi.HTTPClientOption
}

// Dial builds the client for e, reading its key and TLS Secrets from
// namespace.
func Dial(ctx context.Context, c client.Reader, namespace string, e Endpoint) (*httpapi.HTTPClient, error) {
	opts := slices.Clone(e.Options)
	if e.Key != nil {
		key, err := secrets.ResolveSecretKeyRef(ctx, c, namespace, *e.Key)
		if err != nil {
			return nil, err
		}
		if e.Auth == httpapi.AuthPlexToken {
			opts = append(opts, httpapi.WithPlexToken(key))
		} else {
			opts = append(opts, httpapi.WithAPIKey(key))
		}
	}
	tlsCfg, err := resolveTLSConfig(ctx, c, namespace, e.TLS)
	if err != nil {
		return nil, err
	}
	return httpapi.NewHTTPClient(e.URL, e.Auth, append(opts, httpapi.WithTLSConfig(tlsCfg), httpapi.WithAppLabel(e.App))...)
}
