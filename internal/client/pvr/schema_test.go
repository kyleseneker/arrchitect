package pvr

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	"github.com/kyleseneker/arrchitect/internal/httpapi"
)

func qbitSchema() []map[string]any {
	return []map[string]any{
		{
			"implementation": "QBittorrent",
			"fields": []any{
				map[string]any{"name": "host"},
				map[string]any{"name": "port"},
				map[string]any{"name": "tvCategory"},
				map[string]any{"name": "tvImportedCategory"},
			},
		},
		{
			"implementation": "Sabnzbd",
			"fields":         []any{map[string]any{"name": "apiKey"}},
		},
	}
}

func TestValidateFieldNamesAcceptsKnownFields(t *testing.T) {
	assert.NoError(t, validateFieldNames(qbitSchema(), "QBittorrent", []string{"host", "tvCategory"}), "valid fields rejected")
}

// The apps store an unknown field and answer 2xx, so a typo has to be caught here
// or it is silently inert.
func TestValidateFieldNamesRejectsTypos(t *testing.T) {
	err := validateFieldNames(qbitSchema(), "QBittorrent", []string{"tvCatagory"})
	require.Error(t, err, "a misspelled field was accepted")
	assert.ErrorContains(t, err, "tvCatagory", "error does not name the offending field")
	assert.ErrorContains(t, err, "tvCategory", "error does not list the accepted names, so it is not actionable")
}

func TestValidateFieldNamesIsCaseInsensitiveOnImplementation(t *testing.T) {
	assert.Error(t, validateFieldNames(qbitSchema(), "qbittorrent", []string{"nope"}),
		"implementation lookup should not be case sensitive")
}

// An unknown implementation or unreadable schema must not block a write, since the
// operator cannot tell a bad field from an app it does not have a schema for.
func TestValidateFieldNamesSkipsWhenSchemaUnknown(t *testing.T) {
	assert.NoError(t, validateFieldNames(qbitSchema(), "Deluge", []string{"anything"}), "unknown implementation should skip validation")
	assert.NoError(t, validateFieldNames(nil, "QBittorrent", []string{"anything"}), "absent schema should skip validation")
}

func TestSchemaConfigContractComesFromTheAppSchema(t *testing.T) {
	schema := []map[string]any{
		{"implementation": "CustomImport", "configContract": "CustomSettings"},
		{"implementation": "TraktPopularImport", "configContract": "TraktPopularSettings"},
	}
	assert.Equal(t, "CustomSettings", schemaConfigContract(schema, "customimport", ""),
		"the implementation+Settings guess is not what the app expects")
	assert.Equal(t, "Declared", schemaConfigContract(schema, "CustomImport", "Declared"), "a declared contract must win")
	assert.Empty(t, schemaConfigContract(nil, "CustomImport", ""), "an unreadable schema must leave the builder's fallback")
}

func TestSchemaProtocolUsesTheAppsSpelling(t *testing.T) {
	schema := []map[string]any{{"implementation": "QBittorrent", "protocol": "TorrentDownloadProtocol"}}
	assert.Equal(t, "TorrentDownloadProtocol", schemaProtocol(schema, "qbittorrent", "torrent"),
		"Lidarr reads torrent back as TorrentDownloadProtocol and rewrites it forever")
	assert.Equal(t, "torrent", schemaProtocol(nil, "QBittorrent", "torrent"), "an unreadable schema must keep the declared protocol")
}

func TestUnsupportedEventsAreTheOnesTheImplementationLacks(t *testing.T) {
	schema := []map[string]any{{"implementation": "Webhook", "onGrab": false, "onReleaseImport": false, "includeHealthWarnings": false}}
	payload := map[string]any{"name": "hook", "onGrab": true, "onDownload": true, "onHealthRestored": true, "includeHealthWarnings": true}
	got := unsupportedEvents(schema, "Webhook", payload)
	assert.Equal(t, []string{"onDownload", "onHealthRestored"}, got)
	assert.Nil(t, unsupportedEvents(nil, "Webhook", payload), "an unreadable schema must not reject events")
}

// schemaServer serves only the schema endpoints, so a request for anything else
// stands out as unexpected.
func schemaServer(t *testing.T) *httpapi.HTTPClient {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/downloadclient/schema") {
			_ = json.NewEncoder(w).Encode(qbitSchema())
			return
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{})
	}))
	t.Cleanup(srv.Close)

	hc, err := httpapi.NewHTTPClient(srv.URL, httpapi.AuthAPIKey,
		httpapi.WithAPIKey("k"), httpapi.WithAPIKeyHeader("X-Api-Key"),
		httpapi.WithTransport(&http.Transport{}))
	require.NoError(t, err, "building client")
	return hc
}

func TestValidateOptionFieldsReportsTypedFieldName(t *testing.T) {
	opts := Options{DownloadClients: []commonv1alpha1.DownloadClient{{
		Name: "qbit", Implementation: "QBittorrent",
		Fields: []commonv1alpha1.ConfigField{{Name: "tvCatagory"}},
	}}}

	problems := validateOptionFields(context.Background(), schemaServer(t), "v3", opts)
	require.Len(t, problems, 1, "expected one problem")
	assert.ErrorContains(t, problems[0], "qbit", "problem does not identify the resource")
	assert.ErrorContains(t, problems[0], "tvCatagory", "problem does not identify the field")
}

func TestValidateOptionFieldsPassesKnownFields(t *testing.T) {
	opts := Options{DownloadClients: []commonv1alpha1.DownloadClient{{
		Name: "qbit", Implementation: "QBittorrent",
		Fields: []commonv1alpha1.ConfigField{{Name: "tvCategory"}},
	}}}

	problems := validateOptionFields(context.Background(), schemaServer(t), "v3", opts)
	assert.Empty(t, problems, "valid field reported as a problem")
}

// An app that does not serve a schema must still reconcile.
func TestValidateOptionFieldsSkipsWithoutSchema(t *testing.T) {
	opts := Options{Indexers: []commonv1alpha1.Indexer{{
		Name: "nzb", Implementation: "Newznab",
		Fields: []commonv1alpha1.ConfigField{{Name: "whatever"}},
	}}}

	problems := validateOptionFields(context.Background(), schemaServer(t), "v3", opts)
	assert.Empty(t, problems, "missing schema should skip validation")
}
