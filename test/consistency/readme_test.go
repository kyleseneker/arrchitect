package consistency

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The README's supported apps table states each app's range; the compat suite
// tests its two ends. A pin moved without the table, or the other way round,
// fails here.
func TestReadmeListsTheTestedRanges(t *testing.T) {
	images, err := os.ReadFile("../../test/compat/images_test.go")
	require.NoError(t, err)
	newest := pinnedVersions(regexp.MustCompile(`(?m)^\t(\w+Image) += "[^":]+:([^"]+)"$`), images)
	block := regexp.MustCompile(`(?s)var oldestImages = map\[string\]string\{\n(.*?)\n\}`).FindSubmatch(images)
	require.NotNil(t, block, "no oldestImages map")
	oldest := pinnedVersions(regexp.MustCompile(`(?m)^\t(\w+Image): +"[^":]+:([^"]+)",$`), block[1])
	require.NotEmpty(t, newest)
	require.Len(t, oldest, len(newest), "every newest pin needs an oldest pin")

	readme, err := os.ReadFile("../../README.md")
	require.NoError(t, err)
	rows := regexp.MustCompile(`(?m)^\| ([^|]+?) \| [^|]+ \| (\S+) to (\S+) \|$`).FindAllSubmatch(readme, -1)
	listed := map[string]bool{}
	for _, row := range rows {
		app := string(row[1])
		pin := strings.ToLower(strings.ReplaceAll(app, " ", "")) + "Image"
		listed[pin] = true
		assert.Equal(t, oldest[pin], string(row[2]), "%s's oldest supported version", app)
		assert.Equal(t, newest[pin], string(row[3]), "%s's newest supported version", app)
	}
	for pin := range newest {
		assert.True(t, listed[pin], "the README's table has no row for %s", pin)
	}
}

// pinnedVersions maps each pin to the app version its image tag names.
func pinnedVersions(pattern *regexp.Regexp, images []byte) map[string]string {
	versions := map[string]string{}
	for _, m := range pattern.FindAllSubmatch(images, -1) {
		versions[string(m[1])] = appVersion(string(m[2]))
	}
	return versions
}

// appVersion is the version an image tag names, without the build number or
// the image's own suffixes: 4.0.20.3014-ls2 is 4.0.20, v1.6.2-ls371 is 1.6.2.
func appVersion(tag string) string {
	tag = strings.TrimPrefix(strings.TrimPrefix(tag, "develop-"), "v")
	parts := strings.Split(regexp.MustCompile(`^\d+(\.\d+)*`).FindString(tag), ".")
	if len(parts) > 3 {
		parts = parts[:3]
	}
	return strings.Join(parts, ".")
}
