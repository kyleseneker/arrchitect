package seerr

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/kyleseneker/arrchitect/internal/engine"
)

// mediaServerTypeJellyfin is Seerr's MediaServerType.JELLYFIN.
const mediaServerTypeJellyfin = 2

// NotificationTypes maps notification type names to the bits Seerr combines
// into an agent's types.
var NotificationTypes = map[string]int{
	"mediaPending": 2, "mediaApproved": 4, "mediaAvailable": 8, "mediaFailed": 16, "mediaDeclined": 64,
	"mediaAutoApproved": 128, "issueCreated": 256, "issueComment": 512, "issueResolved": 1024,
	"issueReopened": 2048, "mediaAutoRequested": 4096,
}

// Client reaches the Seerr API v1 through an engine.HTTPClient configured with
// AuthSession. Each method names the endpoint it uses.
type Client struct {
	hc *engine.HTTPClient
}

// NewClient creates a new Seerr API client wrapping the given engine.HTTPClient.
func NewClient(hc *engine.HTTPClient) *Client {
	return &Client{hc: hc}
}

// IsInitialized checks whether Seerr has been set up by reading the public settings.
func (c *Client) IsInitialized(ctx context.Context) (bool, error) {
	data, err := c.hc.Do(ctx, http.MethodGet, "/api/v1/settings/public", nil)
	if err != nil {
		return false, err
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return false, fmt.Errorf("unmarshaling public settings: %w", err)
	}
	initialized, ok := result["initialized"].(bool)
	if !ok {
		return false, nil
	}
	return initialized, nil
}

// AuthenticatePlex logs in via the Plex auth provider.
// The session cookie is stored in the engine client's cookie jar.
func (c *Client) AuthenticatePlex(ctx context.Context, plexToken string) error {
	payload := map[string]any{"authToken": plexToken}
	_, err := c.hc.Do(ctx, http.MethodPost, "/api/v1/auth/plex", payload)
	return err
}

// mediaServerTypeNotConfigured is what Seerr reports before its first sign-in.
const mediaServerTypeNotConfigured = 4

// MediaServerConfigured reports whether Seerr already has a media server. The
// first sign-in configures one, before setup as a whole is complete.
func (c *Client) MediaServerConfigured(ctx context.Context) (bool, error) {
	data, err := c.hc.Do(ctx, http.MethodGet, "/api/v1/settings/public", nil)
	if err != nil {
		return false, err
	}
	var result struct {
		MediaServerType *int `json:"mediaServerType"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return false, fmt.Errorf("unmarshaling public settings: %w", err)
	}
	return result.MediaServerType != nil && *result.MediaServerType != mediaServerTypeNotConfigured, nil
}

// AuthenticateJellyfin logs in via the Jellyfin auth provider.
// The session cookie is stored in the engine client's cookie jar. Only the
// first sign-in carries the server: once one is configured, Seerr rejects a
// sign-in that names it again ("Jellyfin hostname already configured").
func (c *Client) AuthenticateJellyfin(ctx context.Context, username, password, jellyfinHost string, jellyfinPort int, urlBase string, useSsl, configured bool) error {
	payload := map[string]any{
		"username": username,
		"password": password,
	}
	if !configured {
		payload["hostname"] = jellyfinHost
		payload["port"] = jellyfinPort
		payload["urlBase"] = urlBase
		payload["useSsl"] = useSsl
		payload["serverType"] = mediaServerTypeJellyfin
	}
	_, err := c.hc.Do(ctx, http.MethodPost, "/api/v1/auth/jellyfin", payload)
	return err
}

// GetAPIKey fetches the main settings and returns the API key.
// This requires an active session (cookie auth).
func (c *Client) GetAPIKey(ctx context.Context) (string, error) {
	result, err := c.Settings(ctx, "main")
	if err != nil {
		return "", err
	}
	key, ok := result["apiKey"].(string)
	if !ok || key == "" {
		return "", fmt.Errorf("no apiKey in main settings")
	}
	return key, nil
}

// SetAPIKey stores an API key for use in subsequent requests.
func (c *Client) SetAPIKey(apiKey string) {
	c.hc.SetAPIKey(apiKey)
}

// Settings reads a settings section: "main", "network", "jellyfin" or
// "plex".
func (c *Client) Settings(ctx context.Context, section string) (map[string]any, error) {
	return c.hc.GetJSON(ctx, "/api/v1/settings/"+section)
}

// SaveSettings writes a settings section that Settings reads.
func (c *Client) SaveSettings(ctx context.Context, section string, body map[string]any) error {
	_, err := c.hc.Do(ctx, http.MethodPost, "/api/v1/settings/"+section, body)
	return err
}

// MetadataSettings reads the metadata providers.
func (c *Client) MetadataSettings(ctx context.Context) (map[string]any, error) {
	return c.hc.GetJSON(ctx, "/api/v1/settings/metadatas")
}

// SaveMetadataSettings writes the metadata providers.
func (c *Client) SaveMetadataSettings(ctx context.Context, body map[string]any) error {
	return c.hc.PutJSON(ctx, "/api/v1/settings/metadatas", body)
}

// NotificationSettings reads a notification agent's settings.
func (c *Client) NotificationSettings(ctx context.Context, agent string) (map[string]any, error) {
	return c.hc.GetJSON(ctx, "/api/v1/settings/notifications/"+agent)
}

// SaveNotificationSettings writes a notification agent's settings.
func (c *Client) SaveNotificationSettings(ctx context.Context, agent string, body map[string]any) error {
	_, err := c.hc.Do(ctx, http.MethodPost, "/api/v1/settings/notifications/"+agent, body)
	return err
}

// Initialize marks setup complete.
func (c *Client) Initialize(ctx context.Context) error {
	_, err := c.hc.Do(ctx, http.MethodPost, "/api/v1/settings/initialize", map[string]any{})
	return err
}

// Services lists the connections of a kind, "sonarr" or "radarr".
func (c *Client) Services(ctx context.Context, kind string) ([]map[string]any, error) {
	return c.hc.GetJSONList(ctx, "/api/v1/settings/"+kind)
}

// CreateService adds a connection of a kind.
func (c *Client) CreateService(ctx context.Context, kind string, body map[string]any) error {
	_, err := c.hc.Do(ctx, http.MethodPost, "/api/v1/settings/"+kind, body)
	return err
}

// UpdateService replaces the connection of a kind with id.
func (c *Client) UpdateService(ctx context.Context, kind string, id int, body map[string]any) error {
	return c.hc.PutJSON(ctx, fmt.Sprintf("/api/v1/settings/%s/%d", kind, id), body)
}

// DeleteService removes the connection of a kind with id.
func (c *Client) DeleteService(ctx context.Context, kind string, id int) error {
	return c.hc.Delete(ctx, fmt.Sprintf("/api/v1/settings/%s/%d", kind, id))
}

// Libraries lists the libraries Seerr knows of a media server, "jellyfin" or
// "plex". The list is a cache that SyncLibraries refreshes.
func (c *Client) Libraries(ctx context.Context, server string) ([]map[string]any, error) {
	return c.hc.GetJSONList(ctx, "/api/v1/settings/"+server+"/library")
}

// SyncLibraries refreshes the libraries from the media server and returns
// them.
func (c *Client) SyncLibraries(ctx context.Context, server string) ([]map[string]any, error) {
	data, err := c.hc.Do(ctx, http.MethodPost, "/api/v1/settings/"+server+"/library/sync", map[string]any{})
	if err != nil {
		return nil, err
	}
	var libraries []map[string]any
	if err := json.Unmarshal(data, &libraries); err != nil {
		return nil, fmt.Errorf("unmarshaling libraries: %w", err)
	}
	return libraries, nil
}

// EnableLibrary turns a media server library with id on or off.
func (c *Client) EnableLibrary(ctx context.Context, server, id string, enabled bool) error {
	return c.hc.PutJSON(ctx, "/api/v1/settings/"+server+"/library/"+id, map[string]any{"enabled": enabled})
}

// Jobs lists the scheduled jobs.
func (c *Client) Jobs(ctx context.Context) ([]map[string]any, error) {
	return c.hc.GetJSONList(ctx, "/api/v1/settings/jobs")
}

// ScheduleJob sets the cron schedule of the job with id.
func (c *Client) ScheduleJob(ctx context.Context, id, schedule string) error {
	_, err := c.hc.Do(ctx, http.MethodPost, "/api/v1/settings/jobs/"+id+"/schedule", map[string]any{"schedule": schedule})
	return err
}

// Ping checks if Seerr is reachable.
func (c *Client) Ping(ctx context.Context) error {
	return c.hc.Ping(ctx, "/api/v1/settings/public")
}
