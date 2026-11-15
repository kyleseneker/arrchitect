package hack

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"
)

const chartDir = "../chart/media-operator"

type manifest struct {
	Kind     string `json:"kind"`
	Metadata struct {
		Name        string            `json:"name"`
		Annotations map[string]string `json:"annotations"`
	} `json:"metadata"`
	Rules []struct {
		APIGroups []string `json:"apiGroups"`
		Resources []string `json:"resources"`
		Verbs     []string `json:"verbs"`
	} `json:"rules"`
	Spec struct {
		Template struct {
			Spec struct {
				Containers []struct {
					Args []string `json:"args"`
				} `json:"containers"`
			} `json:"spec"`
		} `json:"template"`
	} `json:"spec"`
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

func grantsOf(manifests []manifest, kinds ...string) map[string]bool {
	out := map[string]bool{}
	for _, m := range manifests {
		if !slices.Contains(kinds, m.Kind) {
			continue
		}
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
	main, err := os.ReadFile("../cmd/operator/main.go")
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
	data, err := os.ReadFile("../config/rbac/role.yaml")
	require.NoError(t, err)
	var generated manifest
	require.NoError(t, yaml.Unmarshal(data, &generated))
	rendered := grantsOf(render(t), "ClusterRole")

	for grant := range grantsOf([]manifest{generated}, "ClusterRole") {
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
		assert.FileExists(t, filepath.Join(chartDir, "files/crds/media-operator.dev_"+app+"configs.yaml"))
	}
	assert.ElementsMatch(t, controllerNames(t), apps, "values.yaml apps and cmd/operator controllers differ")
}

func TestChartLeavesOutWhatAnAppTurnedOffNeeds(t *testing.T) {
	manifests := render(t, "--set", "apps.tdarr=false")

	var crds, args []string
	for _, m := range manifests {
		switch m.Kind {
		case "CustomResourceDefinition":
			crds = append(crds, m.Metadata.Name)
			assert.Equal(t, "keep", m.Metadata.Annotations["helm.sh/resource-policy"],
				"uninstalling must not delete %s", m.Metadata.Name)
		case "Deployment":
			args = m.Spec.Template.Spec.Containers[0].Args
		}
	}
	assert.Len(t, crds, len(controllerNames(t))-1)
	assert.NotContains(t, crds, "tdarrconfigs.media-operator.dev")
	assert.NotContains(t, grantsOf(manifests, "ClusterRole"), "media-operator.dev tdarrconfigs get")
	enabled := slices.IndexFunc(args, func(a string) bool { return strings.HasPrefix(a, "--enable-controllers=") })
	require.GreaterOrEqual(t, enabled, 0)
	assert.NotContains(t, strings.Split(strings.TrimPrefix(args[enabled], "--enable-controllers="), ","), "tdarr")
}
