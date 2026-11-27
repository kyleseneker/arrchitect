package jellyfin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/kyleseneker/arrchitect/internal/engine"
)

// Client is an HTTP client for the Jellyfin API.
// It delegates all HTTP and auth handling to engine.HTTPClient.
type Client struct {
	hc *engine.HTTPClient
}

// NewClient creates a new Jellyfin API client backed by an engine.HTTPClient.
func NewClient(hc *engine.HTTPClient) *Client {
	return &Client{hc: hc}
}

// IsSetupComplete checks whether the Jellyfin startup wizard has been completed.
func (c *Client) IsSetupComplete(ctx context.Context) (bool, error) {
	result, err := c.hc.GetJSON(ctx, "/System/Info/Public")
	if err != nil {
		return false, err
	}
	completed, ok := result["StartupWizardCompleted"].(bool)
	if !ok {
		return false, nil
	}
	return completed, nil
}

// Version is the server's version, as Jellyfin reports it.
func (c *Client) Version(ctx context.Context) (string, error) {
	result, err := c.hc.GetJSON(ctx, "/System/Info/Public")
	if err != nil {
		return "", err
	}
	version, _ := result["Version"].(string)
	return version, nil
}

// RunSetupWizard drives the Jellyfin first-time setup wizard to completion.
func (c *Client) RunSetupWizard(ctx context.Context, username, password, serverName, metadataLang, countryCode string) error {
	// Step 1: Set initial user. Reading it first is what creates it: Jellyfin 12
	// answers 404 to the update otherwise, and its own web wizard reads first.
	if _, err := c.hc.GetJSON(ctx, "/Startup/User"); err != nil {
		return fmt.Errorf("reading startup user: %w", err)
	}
	userPayload := map[string]any{
		"Name":     username,
		"Password": password,
	}
	if err := c.hc.PostJSON(ctx, "/Startup/User", userPayload); err != nil {
		return fmt.Errorf("setting startup user: %w", err)
	}

	// Step 2: Set server configuration.
	configPayload := map[string]any{
		"MetadataCountryCode":       countryCode,
		"PreferredMetadataLanguage": metadataLang,
		"UICulture":                 metadataLang,
	}
	if err := c.hc.PostJSON(ctx, "/Startup/Configuration", configPayload); err != nil {
		return fmt.Errorf("setting startup configuration: %w", err)
	}

	// Step 3: Configure remote access.
	remotePayload := map[string]any{
		"EnableRemoteAccess":         true,
		"EnableAutomaticPortMapping": false,
	}
	if err := c.hc.PostJSON(ctx, "/Startup/RemoteAccess", remotePayload); err != nil {
		return fmt.Errorf("setting remote access: %w", err)
	}

	// Step 4: Complete the wizard.
	if err := c.hc.PostJSON(ctx, "/Startup/Complete", nil); err != nil {
		return fmt.Errorf("completing startup wizard: %w", err)
	}

	return nil
}

// Authenticate logs in with username/password and stores the access token.
func (c *Client) Authenticate(ctx context.Context, username, password string) error {
	payload := map[string]any{
		"Username": username,
		"Pw":       password,
	}
	data, err := c.hc.Do(ctx, http.MethodPost, "/Users/AuthenticateByName", payload)
	if err != nil {
		return fmt.Errorf("authenticating: %w", err)
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return fmt.Errorf("unmarshaling auth response: %w", err)
	}
	token, ok := result["AccessToken"].(string)
	if !ok || token == "" {
		return fmt.Errorf("no access token in auth response")
	}
	c.hc.SetMediaToken(token)
	return nil
}

// UseAPIKey authenticates with an API key, which Jellyfin takes as a token,
// and checks it.
func (c *Client) UseAPIKey(ctx context.Context, key string) error {
	c.hc.SetMediaToken(key)
	if _, err := c.hc.Do(ctx, http.MethodGet, "/System/Info", nil); err != nil {
		return fmt.Errorf("checking the API key: %w", err)
	}
	return nil
}

// GetConfig fetches a configuration resource by path.
func (c *Client) GetConfig(ctx context.Context, path string) (map[string]any, error) {
	return c.hc.GetJSON(ctx, path)
}

// PostConfig sends a configuration payload to the given path.
func (c *Client) PostConfig(ctx context.Context, path string, payload map[string]any) error {
	return c.hc.PostJSON(ctx, path, payload)
}

// ListLibraries returns all virtual folder (library) entries.
func (c *Client) ListLibraries(ctx context.Context) ([]map[string]any, error) {
	return c.hc.GetJSONList(ctx, "/Library/VirtualFolders")
}

// CreateLibrary adds a new virtual folder (library).
func (c *Client) CreateLibrary(ctx context.Context, name, collectionType string, libraryOptions map[string]any) error {
	params := url.Values{}
	params.Set("name", name)
	params.Set("collectionType", collectionType)
	params.Set("refreshLibrary", "false")

	path := "/Library/VirtualFolders?" + params.Encode()
	return c.hc.PostJSON(ctx, path, map[string]any{"LibraryOptions": libraryOptions})
}

// AddLibraryPath adds a folder to a library without rescanning it.
func (c *Client) AddLibraryPath(ctx context.Context, library, path string) error {
	return c.hc.PostJSON(ctx, "/Library/VirtualFolders/Paths?refreshLibrary=false",
		map[string]any{"Name": library, "PathInfo": map[string]any{"Path": path}})
}

// RemoveLibraryPath removes a folder from a library without rescanning it.
func (c *Client) RemoveLibraryPath(ctx context.Context, library, path string) error {
	params := url.Values{"name": {library}, "path": {path}, "refreshLibrary": {"false"}}
	return c.hc.Delete(ctx, "/Library/VirtualFolders/Paths?"+params.Encode())
}

// AvailableLibraryOptions lists the fetchers, readers and providers a library
// of the collection type can use, with the ones Jellyfin enables when a
// library has no say of its own. A mixed library has no collection type.
func (c *Client) AvailableLibraryOptions(ctx context.Context, collectionType string) (map[string]any, error) {
	params := url.Values{"isNewLibrary": {"false"}}
	if collectionType != "" && collectionType != "mixed" {
		params.Set("libraryContentType", collectionType)
	}
	return c.hc.GetJSON(ctx, "/Libraries/AvailableOptions?"+params.Encode())
}

// UpdateLibraryOptions replaces a library's options, which include its folders.
func (c *Client) UpdateLibraryOptions(ctx context.Context, id string, options map[string]any) error {
	return c.hc.PostJSON(ctx, "/Library/VirtualFolders/LibraryOptions", map[string]any{"Id": id, "LibraryOptions": options})
}

// ScheduledTasks lists Jellyfin's scheduled tasks with their triggers.
func (c *Client) ScheduledTasks(ctx context.Context) ([]map[string]any, error) {
	return c.hc.GetJSONList(ctx, "/ScheduledTasks")
}

// SetTaskTriggers replaces the triggers of the task with id.
func (c *Client) SetTaskTriggers(ctx context.Context, id string, triggers []map[string]any) error {
	_, err := c.hc.Do(ctx, http.MethodPost, "/ScheduledTasks/"+url.PathEscape(id)+"/Triggers", triggers)
	return err
}
