package operator

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/cache"
)

func TestResolveEnabledControllers(t *testing.T) {
	controllers := []Controller{{Name: "sonarr"}, {Name: "radarr"}, {Name: "lidarr"}}
	tests := []struct {
		name, enable, disable string
		want                  map[string]bool
	}{
		{name: "all", enable: "*", want: map[string]bool{"sonarr": true, "radarr": true, "lidarr": true}},
		{name: "some", enable: "sonarr, radarr", want: map[string]bool{"sonarr": true, "radarr": true, "lidarr": false}},
		{name: "all but one", enable: "*", disable: "lidarr", want: map[string]bool{"sonarr": true, "radarr": true, "lidarr": false}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveEnabledControllers(controllers, tt.enable, tt.disable)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}

	_, err := resolveEnabledControllers(controllers, "sonar", "")
	assert.ErrorContains(t, err, `unknown controller "sonar"; the controllers are sonarr, radarr, lidarr`)
	_, err = resolveEnabledControllers(controllers, "*", "prowlarr")
	assert.ErrorContains(t, err, `unknown controller "prowlarr"`)
}

func TestWatchedNamespaces(t *testing.T) {
	assert.Nil(t, watchedNamespaces(""), "empty watches every namespace")
	assert.Equal(t, map[string]cache.Config{"media": {}, "arr": {}}, watchedNamespaces("media, arr,"))
}
