// Package consistency checks that the repository's files agree with each other:
// the chart with the controllers and their RBAC markers, and Renovate with the
// Makefile and CI. The chart's own behaviour is tested with helm-unittest in
// chart/arrchitect/tests.
package consistency

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"
)

const chartDir = "../../chart/arrchitect"

type manifest struct {
	Kind     string `json:"kind"`
	Metadata struct {
		Name string `json:"name"`
	} `json:"metadata"`
	Rules []struct {
		APIGroups []string `json:"apiGroups"`
		Resources []string `json:"resources"`
		Verbs     []string `json:"verbs"`
	} `json:"rules"`
}

func render(t *testing.T, args ...string) []manifest {
	t.Helper()
	helm, err := exec.LookPath("helm")
	if err != nil {
		t.Skip("helm is not installed")
	}
	out, err := exec.Command(helm, append([]string{"template", "test", chartDir}, args...)...).CombinedOutput()
	require.NoError(t, err, "%s", out)
	var manifests []manifest
	for doc := range bytes.SplitSeq(out, []byte("\n---")) {
		var m manifest
		require.NoError(t, yaml.Unmarshal(doc, &m))
		if m.Kind != "" {
			manifests = append(manifests, m)
		}
	}
	return manifests
}

func grants(manifests []manifest) map[string]bool {
	out := map[string]bool{}
	for _, m := range manifests {
		for _, r := range m.Rules {
			for _, g := range r.APIGroups {
				for _, res := range r.Resources {
					for _, v := range r.Verbs {
						out[g+" "+res+" "+v] = true
					}
				}
			}
		}
	}
	return out
}

func controllerNames(t *testing.T) []string {
	t.Helper()
	main, err := os.ReadFile("../../cmd/operator/main.go")
	require.NoError(t, err)
	matches := regexp.MustCompile(`operator\.Controller\{Name: "(\w+)"`).FindAllSubmatch(main, -1)
	names := make([]string, 0, len(matches))
	for _, m := range matches {
		names = append(names, string(m[1]))
	}
	require.NotEmpty(t, names)
	return names
}

func TestChartGrantsWhatTheControllersNeed(t *testing.T) {
	data, err := os.ReadFile("../../config/rbac/role.yaml")
	require.NoError(t, err)
	var generated manifest
	require.NoError(t, yaml.Unmarshal(data, &generated))
	var operator []manifest
	for _, m := range render(t) {
		if m.Kind == "ClusterRole" && m.Metadata.Name == "test" {
			operator = append(operator, m)
		}
	}
	require.Len(t, operator, 1, "the operator's ClusterRole is named after the release")
	rendered := grants(operator)

	for grant := range grants([]manifest{generated}) {
		assert.True(t, rendered[grant], "the chart's ClusterRole is missing %q from config/rbac/role.yaml", grant)
	}
	assert.False(t, rendered["coordination.k8s.io leases delete"], "leases belong in a Role in the release namespace")
}

func TestChartAppsMatchTheControllersAndCRDs(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(chartDir, "values.yaml"))
	require.NoError(t, err)
	var values struct {
		Apps map[string]bool `json:"apps"`
	}
	require.NoError(t, yaml.Unmarshal(data, &values))
	apps := make([]string, 0, len(values.Apps))
	for app := range values.Apps {
		apps = append(apps, app)
	}
	assert.ElementsMatch(t, controllerNames(t), apps, "values.yaml apps and cmd/operator controllers differ")

	var installed []string
	for _, m := range render(t) {
		if m.Kind == "CustomResourceDefinition" {
			installed = append(installed, "arrchitect.dev_"+strings.TrimSuffix(m.Metadata.Name, ".arrchitect.dev")+".yaml")
		}
	}
	files, err := filepath.Glob(filepath.Join(chartDir, "files/crds/*.yaml"))
	require.NoError(t, err)
	for i, f := range files {
		files[i] = filepath.Base(f)
	}
	assert.ElementsMatch(t, files, installed, "the chart's CRD files and the CRDs it installs with every app on differ")
}
