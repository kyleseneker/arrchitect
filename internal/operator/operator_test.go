package operator

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
			assert.Equal(t, tt.want, resolveEnabledControllers(controllers, tt.enable, tt.disable))
		})
	}
}
