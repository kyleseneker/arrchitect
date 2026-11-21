//go:build compat

package compat

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// The newest release of each app, which the suite runs against by default;
// README's supported apps table lists the same versions. Renovate updates
// each pin, and CI proves the new version before it merges.
const (
	// renovate: datasource=docker
	sonarrImage = "lscr.io/linuxserver/sonarr:4.0.20.3014-ls2"
	// renovate: datasource=docker
	radarrImage = "lscr.io/linuxserver/radarr:7.0.3.10918-ls328"
	// renovate: datasource=docker
	lidarrImage = "lscr.io/linuxserver/lidarr:3.1.6.5079-ls47"
	// renovate: datasource=docker
	readarrImage = "lscr.io/linuxserver/readarr:develop-0.4.18.2805-ls157"
	// renovate: datasource=docker
	prowlarrImage = "lscr.io/linuxserver/prowlarr:3.0.2.5780-ls169"
	// renovate: datasource=docker
	flaresolverrImage = "ghcr.io/flaresolverr/flaresolverr:v3.5.2"
	// renovate: datasource=docker
	autobrrImage = "ghcr.io/autobrr/autobrr:v1.91.1"
	// renovate: datasource=docker
	bazarrImage = "lscr.io/linuxserver/bazarr:v1.6.2-ls371"
	// renovate: datasource=docker
	jellyfinImage = "docker.io/jellyfin/jellyfin:12.4"
	// renovate: datasource=docker
	seerrImage = "ghcr.io/seerr-team/seerr:v3.5.2"
	// renovate: datasource=docker
	maintainerrImage = "ghcr.io/maintainerr/maintainerr:3.33.0"
	// renovate: datasource=docker
	tdarrImage = "ghcr.io/haveagitgat/tdarr:3.08.00"
	// renovate: datasource=docker
	sabnzbdImage = "lscr.io/linuxserver/sabnzbd:5.1.3-ls280"
	// renovate: datasource=docker
	plexImage = "docker.io/plexinc/pms-docker:1.43.5.11079-b3b13e506"
	// renovate: datasource=docker
	qbittorrentImage = "lscr.io/linuxserver/qbittorrent:5.3.0_v2.1.2-ls486"
)

// oldestImages are the oldest release each app is supported at, run with
// COMPAT_VERSIONS=oldest: the release that was current 12 months before the
// pins were last moved, or the first Arrchitect works with where that is
// later. Move them by hand in a minor release, and say so in its notes.
var oldestImages = map[string]string{
	sonarrImage:       "lscr.io/linuxserver/sonarr:4.0.16.2944-ls304",
	radarrImage:       "lscr.io/linuxserver/radarr:6.0.4.10291-ls296",
	lidarrImage:       "lscr.io/linuxserver/lidarr:3.1.0.4875-ls45",
	readarrImage:      "lscr.io/linuxserver/readarr:develop-0.4.17.2801-ls156",
	prowlarrImage:     "lscr.io/linuxserver/prowlarr:2.3.0.5236-ls140",
	flaresolverrImage: "ghcr.io/flaresolverr/flaresolverr:v3.4.5",
	autobrrImage:      "ghcr.io/autobrr/autobrr:v1.69.0",
	bazarrImage:       "lscr.io/linuxserver/bazarr:v1.5.3-ls329",
	jellyfinImage:     "docker.io/jellyfin/jellyfin:10.11.3",
	// Seerr's first-time setup needs 3.5.0 or later.
	seerrImage:       "ghcr.io/seerr-team/seerr:v3.5.0",
	maintainerrImage: "ghcr.io/maintainerr/maintainerr:3.30.0",
	tdarrImage:       "ghcr.io/haveagitgat/tdarr:3.00.00",
	sabnzbdImage:     "lscr.io/linuxserver/sabnzbd:4.5.5-ls250",
	plexImage:        "docker.io/plexinc/pms-docker:1.42.2.10156-f737b826c",
	qbittorrentImage: "lscr.io/linuxserver/qbittorrent:5.1.4-r3-ls453",
}

// previousMajorImages are, for an app whose newest pin starts a major line,
// the newest release of the line before it, run with
// COMPAT_VERSIONS=previous-major while that line is supported. Every other
// app runs at its newest pin in that run.
var previousMajorImages = map[string]string{
	radarrImage:   "lscr.io/linuxserver/radarr:6.4.4.10685-ls325",
	prowlarrImage: "lscr.io/linuxserver/prowlarr:2.7.0.5723-ls167",
	jellyfinImage: "docker.io/jellyfin/jellyfin:10.11.11",
}

// pinnedImage returns the image COMPAT_VERSIONS asks for in place of an
// app's newest pin.
func pinnedImage(t *testing.T, newest string) string {
	t.Helper()
	switch versions := os.Getenv("COMPAT_VERSIONS"); versions {
	case "", "newest":
		return newest
	case "oldest":
		oldest, ok := oldestImages[newest]
		require.True(t, ok, "no oldest pin for %s", newest)
		return oldest
	case "previous-major":
		if previous, ok := previousMajorImages[newest]; ok {
			return previous
		}
		return newest
	default:
		require.Failf(t, "unknown COMPAT_VERSIONS", "%q: use newest, oldest or previous-major", versions)
		return ""
	}
}

// atNewest reports whether the run uses every app's newest pin.
func atNewest() bool {
	versions := os.Getenv("COMPAT_VERSIONS")
	return versions == "" || versions == "newest"
}
