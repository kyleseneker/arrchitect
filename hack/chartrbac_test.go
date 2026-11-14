package hack

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"
)

type rbacRules struct {
	Rules []struct {
		APIGroups []string `json:"apiGroups"`
		Resources []string `json:"resources"`
		Verbs     []string `json:"verbs"`
	} `json:"rules"`
}

func grants(t *testing.T, path string) map[string]bool {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	_, rules, found := strings.Cut(string(data), "\nrules:\n")
	require.True(t, found, "%s has no rules", path)
	kept := []string{"rules:"}
	for line := range strings.SplitSeq(rules, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "{{") {
			kept = append(kept, line)
		}
	}
	var role rbacRules
	require.NoError(t, yaml.Unmarshal([]byte(strings.Join(kept, "\n")), &role), "parsing %s", path)
	out := map[string]bool{}
	for _, r := range role.Rules {
		for _, g := range r.APIGroups {
			for _, res := range r.Resources {
				for _, v := range r.Verbs {
					out[g+" "+res+" "+v] = true
				}
			}
		}
	}
	return out
}

func TestChartClusterRolesGrantWhatTheControllersNeed(t *testing.T) {
	generated := grants(t, "../config/rbac/role.yaml")
	charts, err := filepath.Glob("../chart/*/templates/clusterrole.yaml")
	require.NoError(t, err)
	require.NotEmpty(t, charts)

	for _, path := range charts {
		chartDir := filepath.Dir(filepath.Dir(path))
		crds, err := filepath.Glob(filepath.Join(chartDir, "crds", "media-operator.dev_*.yaml"))
		require.NoError(t, err)
		plurals := map[string]bool{}
		for _, crd := range crds {
			plurals[strings.TrimSuffix(strings.TrimPrefix(filepath.Base(crd), "media-operator.dev_"), ".yaml")] = true
		}

		chart := grants(t, path)
		for grant := range generated {
			group, rest, _ := strings.Cut(grant, " ")
			resource, _, _ := strings.Cut(rest, " ")
			base, _, _ := strings.Cut(resource, "/")
			if group == "media-operator.dev" && !plurals[base] {
				continue
			}
			assert.True(t, chart[grant], "%s is missing %q from config/rbac/role.yaml", filepath.Base(chartDir), grant)
		}
	}
}
