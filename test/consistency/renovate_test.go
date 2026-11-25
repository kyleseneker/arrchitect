package consistency

import (
	"encoding/json"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Renovate only updates Makefile tool pins its custom regex manager matches;
// a pin added without a "# renovate:" annotation silently stops updating.
func TestRenovateTracksEveryMakefileToolVersion(t *testing.T) {
	var config struct {
		CustomManagers []struct {
			ManagerFilePatterns []string `json:"managerFilePatterns"`
			MatchStrings        []string `json:"matchStrings"`
		} `json:"customManagers"`
	}
	raw, err := os.ReadFile("../../renovate.json")
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &config))

	var manager *regexp.Regexp
	for _, m := range config.CustomManagers {
		if slices.Contains(m.ManagerFilePatterns, "/^Makefile$/") {
			manager = regexp.MustCompile(m.MatchStrings[0])
		}
	}
	require.NotNil(t, manager, "renovate.json has no custom manager for the Makefile")

	makefile, err := os.ReadFile("../../Makefile")
	require.NoError(t, err)

	extracted := map[int]bool{} // offsets where Renovate reads a currentValue
	cv := manager.SubexpIndex("currentValue")
	for _, m := range manager.FindAllSubmatchIndex(makefile, -1) {
		extracted[m[2*cv]] = true
	}

	pins := regexp.MustCompile(`(?m)^(\w+_VERSION) \?= (v\S+)$`).FindAllSubmatchIndex(makefile, -1)
	require.NotEmpty(t, pins)
	for _, p := range pins {
		assert.True(t, extracted[p[4]], "%s has no # renovate: annotation", makefile[p[2]:p[3]])
	}
}

// The custom golangci-lint with kube-api-linter builds on the version make
// lint installs; Renovate bumps both in one PR.
func TestCustomGolangciLintMatchesTheMakefile(t *testing.T) {
	makefile, err := os.ReadFile("../../Makefile")
	require.NoError(t, err)
	custom, err := os.ReadFile("../../.custom-gcl.yml")
	require.NoError(t, err)

	local := regexp.MustCompile(`(?m)^GOLANGCI_LINT_VERSION \?= (\S+)$`).FindSubmatch(makefile)
	require.NotNil(t, local, "GOLANGCI_LINT_VERSION not found in Makefile")
	built := regexp.MustCompile(`(?m)^version: (\S+)$`).FindSubmatch(custom)
	require.NotNil(t, built, "version not found in .custom-gcl.yml")

	assert.Equal(t, string(local[1]), string(built[1]))
}

// config:recommended ignores every path under test/, which hid the compat
// images from Renovate; ignorePaths replaces that list rather than adding to it.
func TestRenovateTracksEveryCompatImage(t *testing.T) {
	var config struct {
		IgnorePaths    []string `json:"ignorePaths"`
		CustomManagers []struct {
			ManagerFilePatterns []string `json:"managerFilePatterns"`
			MatchStrings        []string `json:"matchStrings"`
		} `json:"customManagers"`
	}
	raw, err := os.ReadFile("../../renovate.json")
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &config))

	const file = "test/compat/images_test.go"
	require.NotNil(t, config.IgnorePaths, "without ignorePaths, config:recommended ignores %s", file)
	for _, pattern := range config.IgnorePaths {
		assert.False(t, globRegexp(pattern).MatchString(file), "ignorePaths entry %s hides %s from Renovate", pattern, file)
	}

	var manager *regexp.Regexp
	for _, m := range config.CustomManagers {
		for _, p := range m.ManagerFilePatterns {
			if regexp.MustCompile(strings.Trim(p, "/")).MatchString(file) {
				manager = regexp.MustCompile(m.MatchStrings[0])
			}
		}
	}
	require.NotNil(t, manager, "renovate.json has no custom manager for %s", file)

	images, err := os.ReadFile("../../" + file)
	require.NoError(t, err)
	extracted := map[string]bool{}
	for _, m := range manager.FindAllSubmatch(images, -1) {
		extracted[string(m[manager.SubexpIndex("depName")])] = true
	}
	pins := regexp.MustCompile(`(?m)^\t(\w+Image) += "([^:"]+):`).FindAllSubmatch(images, -1)
	require.NotEmpty(t, pins)
	for _, p := range pins {
		assert.True(t, extracted[string(p[2])], "%s has no // renovate: annotation", p[1])
	}
}

// The example stack shows the compat suite's newest pins. Renovate reads them
// with the compat versioning, so each bump joins the app's compat PR.
func TestRenovateTracksTheExampleStackImages(t *testing.T) {
	var config struct {
		CustomManagers []struct {
			ManagerFilePatterns []string `json:"managerFilePatterns"`
			MatchStrings        []string `json:"matchStrings"`
		} `json:"customManagers"`
		PackageRules []struct {
			Description    string   `json:"description"`
			MatchFileNames []string `json:"matchFileNames"`
			Versioning     string   `json:"versioning"`
		} `json:"packageRules"`
	}
	raw, err := os.ReadFile("../../renovate.json")
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &config))

	const compat, file = "test/compat/images_test.go", "docs/example-stack.md"
	var manager *regexp.Regexp
	for _, m := range config.CustomManagers {
		for _, p := range m.ManagerFilePatterns {
			if regexp.MustCompile(strings.Trim(p, "/")).MatchString(file) {
				manager = regexp.MustCompile(m.MatchStrings[0])
			}
		}
	}
	require.NotNil(t, manager, "renovate.json has no custom manager for %s", file)

	images, err := os.ReadFile("../../" + compat)
	require.NoError(t, err)
	newest := map[string]string{}
	for _, m := range regexp.MustCompile(`(?m)^\t\w+Image += "([^":]+):([^"]+)"$`).FindAllSubmatch(images, -1) {
		newest[string(m[1])] = string(m[2])
	}

	doc, err := os.ReadFile("../../" + file)
	require.NoError(t, err)
	extracted := map[string]bool{}
	for _, m := range manager.FindAllSubmatch(doc, -1) {
		extracted[string(m[manager.SubexpIndex("depName")])] = true
	}
	pins := regexp.MustCompile(`(?m)^\s+image: ([^\s:]+):(\S+)$`).FindAllSubmatch(doc, -1)
	require.NotEmpty(t, pins)
	for _, p := range pins {
		image := string(p[1])
		assert.True(t, extracted[image], "Renovate does not read the pin of %s", image)
		assert.Equal(t, newest[image], string(p[2]), "%s is not at the compat suite's newest pin", image)
	}

	for _, rule := range config.PackageRules {
		if rule.Versioning != "" && slices.Contains(rule.MatchFileNames, compat) {
			assert.Contains(t, rule.MatchFileNames, file, "%q does not cover %s", rule.Description, file)
		}
	}
}

// globRegexp turns a Renovate ignorePaths glob into a regular expression.
func globRegexp(glob string) *regexp.Regexp {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(glob); i++ {
		switch {
		case strings.HasPrefix(glob[i:], "**/"):
			b.WriteString("(.*/)?")
			i += 2
		case strings.HasPrefix(glob[i:], "**"):
			b.WriteString(".*")
			i++
		case glob[i] == '*':
			b.WriteString("[^/]*")
		default:
			b.WriteString(regexp.QuoteMeta(string(glob[i])))
		}
	}
	b.WriteString("$")
	return regexp.MustCompile(b.String())
}
