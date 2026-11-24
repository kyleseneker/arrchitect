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

// crd-ref-docs leaves out, without a word, any package with a marker its
// controller-tools can't parse, so docs/api.md can lose a whole app while
// verify-api-docs passes. Every kind with a CRD must be a resource type there.
func TestAPIDocsListEveryKind(t *testing.T) {
	docs, err := os.ReadFile("../../docs/api.md")
	require.NoError(t, err)
	block := regexp.MustCompile(`(?s)### Resource Types\n(.*?)\n\n`).FindSubmatch(docs)
	require.NotNil(t, block, "docs/api.md has no Resource Types list")
	entries := regexp.MustCompile(`(?m)^- \[(\w+)\]`).FindAllSubmatch(block[1], -1)
	listed := make([]string, 0, len(entries))
	for _, m := range entries {
		listed = append(listed, string(m[1]))
	}

	crds, err := filepath.Glob("../../config/crd/bases/*.yaml")
	require.NoError(t, err)
	require.NotEmpty(t, crds)
	for _, path := range crds {
		raw, err := os.ReadFile(path)
		require.NoError(t, err)
		var crd struct {
			Spec struct {
				Names struct {
					Kind string `json:"kind"`
				} `json:"names"`
			} `json:"spec"`
		}
		require.NoError(t, yaml.Unmarshal(raw, &crd), "reading %s", path)
		require.NotEmpty(t, crd.Spec.Names.Kind, "%s names no kind", path)
		assert.Contains(t, listed, crd.Spec.Names.Kind,
			"docs/api.md doesn't list it: check its package's markers parse with crd-ref-docs' controller-tools")
	}
}
