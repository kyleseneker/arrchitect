package consistency

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"
)

// mirroredCapitals are field names copied from an app's own key, which
// spells the initialism in capitals.
var mirroredCapitals = map[string]bool{
	"enableIPv6":                true, // Jellyfin
	"enableLUFSScan":            true, // Jellyfin
	"enableUPnP":                true, // Jellyfin
	"isRemoteIPFilterBlacklist": true, // Jellyfin
	"remoteIPFilter":            true, // Jellyfin
	"jobHistorySizeLimitGB":     true, // Tdarr
}

var capitalRun = regexp.MustCompile(`[A-Z]{2,}`)

func TestAPIFieldNamesWriteInitialismsAsWords(t *testing.T) {
	files, err := filepath.Glob("../../config/crd/bases/*.yaml")
	require.NoError(t, err)
	require.NotEmpty(t, files)

	seen := map[string]bool{}
	for _, f := range files {
		data, err := os.ReadFile(f)
		require.NoError(t, err)
		var crd struct {
			Spec struct {
				Versions []struct {
					Schema struct {
						OpenAPIV3Schema map[string]any `json:"openAPIV3Schema"`
					} `json:"schema"`
				} `json:"versions"`
			} `json:"spec"`
		}
		require.NoError(t, yaml.Unmarshal(data, &crd))
		for _, v := range crd.Spec.Versions {
			walkProperties(v.Schema.OpenAPIV3Schema, func(name string) {
				if !capitalRun.MatchString(name) {
					return
				}
				seen[name] = true
				assert.True(t, mirroredCapitals[name],
					"%s: %q spells an initialism in capitals. Write it as a word (Url, Ssl, Cpu), or add it to mirroredCapitals if the app's own key is spelled that way",
					filepath.Base(f), name)
			})
		}
	}
	for name := range mirroredCapitals {
		assert.True(t, seen[name], "mirroredCapitals lists %q, which no CRD has", name)
	}
}

func walkProperties(schema map[string]any, visit func(string)) {
	if props, ok := schema["properties"].(map[string]any); ok {
		for name, sub := range props {
			visit(name)
			if m, ok := sub.(map[string]any); ok {
				walkProperties(m, visit)
			}
		}
	}
	for _, key := range []string{"items", "additionalProperties"} {
		if m, ok := schema[key].(map[string]any); ok {
			walkProperties(m, visit)
		}
	}
}
