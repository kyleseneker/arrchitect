package provider

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
)

func rawValue(raw string) *commonv1alpha1.FieldValue {
	fv := &commonv1alpha1.FieldValue{}
	_ = fv.UnmarshalJSON([]byte(raw))
	return fv
}

func TestNotificationPayload(t *testing.T) {
	tests := []struct {
		name  string
		n     commonv1alpha1.Notification
		check func(t *testing.T, p map[string]any)
	}{
		{
			name: "minimal",
			n:    commonv1alpha1.Notification{Name: "Discord", Implementation: "Discord"},
			check: func(t *testing.T, p map[string]any) {
				assert.Equal(t, "Discord", p["name"])
				assert.Equal(t, "DiscordSettings", p["configContract"])
				_, hasOnGrab := p["onGrab"]
				assert.False(t, hasOnGrab, "nil bools should not be set")
				assert.NotContains(t, p, "fields")
				assert.NotContains(t, p, "tags")
			},
		},
		{
			name: "with triggers",
			n: commonv1alpha1.Notification{
				Name: "Slack", Implementation: "Slack",
				OnGrab: new(true), OnDownload: new(false),
				OnMovieAdded: new(true),
			},
			check: func(t *testing.T, p map[string]any) {
				assert.Equal(t, true, p["onGrab"])
				assert.Equal(t, false, p["onDownload"])
				assert.Equal(t, true, p["onMovieAdded"])
				_, hasOnRename := p["onRename"]
				assert.False(t, hasOnRename)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NotificationPayload(tt.n, nil)
			tt.check(t, p)
		})
	}
}

func TestFieldsPayload(t *testing.T) {
	tests := []struct {
		name   string
		fields []commonv1alpha1.ConfigField
		check  func(t *testing.T, result []map[string]any)
	}{
		{
			name:   "empty",
			fields: []commonv1alpha1.ConfigField{},
			check: func(t *testing.T, result []map[string]any) {
				assert.Empty(t, result)
			},
		},
		{
			name: "with values",
			fields: []commonv1alpha1.ConfigField{
				{Name: "baseUrl", Value: rawValue(`"http://example.com"`)},
				{Name: "apiKey", Value: rawValue(`"secret"`)},
			},
			check: func(t *testing.T, result []map[string]any) {
				require.Len(t, result, 2)
				assert.Equal(t, "baseUrl", result[0]["name"])
				assert.Equal(t, "http://example.com", result[0]["value"])
			},
		},
		{
			name: "nil value",
			fields: []commonv1alpha1.ConfigField{
				{Name: "optional"},
			},
			check: func(t *testing.T, result []map[string]any) {
				require.Len(t, result, 1)
				assert.Nil(t, result[0]["value"])
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FieldsPayload(tt.fields)
			tt.check(t, result)
		})
	}
}

func TestTagsPayload(t *testing.T) {
	tests := []struct {
		name string
		tags []int
		want []any
	}{
		{"empty", []int{}, []any{}},
		{"multiple", []int{1, 5, 10}, []any{1, 5, 10}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TagsPayload(tt.tags)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestSetOptionalBool(t *testing.T) {
	tests := []struct {
		name    string
		val     *bool
		wantSet bool
		want    bool
	}{
		{"nil not set", nil, false, false},
		{"true", new(true), true, true},
		{"false", new(false), true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := map[string]any{}
			SetOptionalBool(p, "key", tt.val)
			if tt.wantSet {
				assert.Equal(t, tt.want, p["key"])
			} else {
				_, exists := p["key"]
				assert.False(t, exists)
			}
		})
	}
}

// The schema listing names the fields each implementation takes. An
// implementation it doesn't describe can't be checked.
func TestSchemaFieldNames(t *testing.T) {
	schema := []map[string]any{
		{"implementation": "Newznab", "fields": []any{map[string]any{"name": "baseUrl"}, map[string]any{"name": "apiKey"}}},
		{"implementation": "Torznab"},
	}

	names, ok := SchemaFieldNames(schema, "newznab")
	require.True(t, ok, "the implementation's case doesn't matter")
	assert.Equal(t, map[string]bool{"baseUrl": true, "apiKey": true}, names)

	_, ok = SchemaFieldNames(schema, "Torznab")
	assert.False(t, ok, "a definition without fields describes nothing to check")
	_, ok = SchemaFieldNames(schema, "Missing")
	assert.False(t, ok)
}
