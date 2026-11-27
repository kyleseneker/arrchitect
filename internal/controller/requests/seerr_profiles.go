package requests

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	requestsv1alpha1 "github.com/kyleseneker/arrchitect/api/requests/v1alpha1"
	ctrlcommon "github.com/kyleseneker/arrchitect/internal/controller/common"
	"github.com/kyleseneker/arrchitect/internal/engine"
	"github.com/kyleseneker/arrchitect/internal/httpapi"
)

// Names survive a database rebuild; numeric profile IDs do not. Resolve before
// writing Seerr settings, and leave its existing connection intact on failure.
// The lookup goes to the Sonarr or Radarr itself, with tlsSpec's settings.
func (r *SeerrReconciler) resolvedServicePayload(
	ctx context.Context, namespace, kind string, svc *requestsv1alpha1.SeerrServiceConnection,
	tlsSpec *commonv1alpha1.TLSConfig, apiKey string,
) (map[string]any, error) {
	payload := buildServicePayload(svc, apiKey)
	if svc.ActiveProfileName == "" && svc.ActiveAnimeProfileName == "" {
		return payload, nil // Preserve ID-only configurations.
	}
	if svc.Port == nil || *svc.Port < 1 || *svc.Port > 65535 {
		return nil, fmt.Errorf("a valid service port is required for profile name resolution")
	}
	scheme := "http"
	if svc.UseSSL != nil && *svc.UseSSL {
		scheme = "https"
	}
	endpoint := url.URL{Scheme: scheme, Host: net.JoinHostPort(svc.Hostname, strconv.Itoa(int(*svc.Port))), Path: "/" + strings.Trim(svc.BaseURL, "/")}
	tlsCfg, err := ctrlcommon.ResolveTLSConfig(ctx, r.Client, namespace, tlsSpec)
	if err != nil {
		return nil, err
	}
	hc, err := httpapi.NewHTTPClient(endpoint.String(), httpapi.AuthAPIKey, httpapi.WithAPIKey(apiKey),
		httpapi.WithTLSConfig(tlsCfg), httpapi.WithAppLabel(kind))
	if err != nil {
		return nil, err
	}
	profiles, err := engine.FetchNamedIDs(ctx, hc, "/api/v3/qualityprofile", "name")
	if err != nil {
		return nil, err
	}
	for field, name := range map[string]string{"activeProfile": svc.ActiveProfileName, "activeAnimeProfile": svc.ActiveAnimeProfileName} {
		if name == "" {
			continue
		}
		id, err := profiles.Resolve(name)
		if err != nil {
			return nil, fmt.Errorf("%sName: %w", field, err)
		}
		payload[field+"Id"] = id
	}
	return payload, nil
}
