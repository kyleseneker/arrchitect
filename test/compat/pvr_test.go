//go:build compat

package compat

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	pvrv1alpha1 "github.com/kyleseneker/arrchitect/api/pvr/v1alpha1"
	pvrclient "github.com/kyleseneker/arrchitect/internal/client/pvr"
	ctrlcommon "github.com/kyleseneker/arrchitect/internal/controller/common"
	"github.com/kyleseneker/arrchitect/internal/controller/pvr"
	"github.com/kyleseneker/arrchitect/internal/engine"
)

func TestSonarr(t *testing.T) {
	t.Parallel()
	sonarr, key := startPVR(t, "sonarr", sonarrImage, 8989, "v3")
	sonarr.exec(t, "mkdir -p /data/media/tv && chown -R abc:abc /data")
	seedDelayProfile(t, directClient(t, sonarr.url(), key), "/api/v3", "hdr",
		map[string]any{"enableUsenet": true, "enableTorrent": true, "preferredProtocol": "usenet"})
	condition := func(name, implementation string, value any) map[string]any {
		return map[string]any{
			"name": name, "implementation": implementation,
			"fields": []any{map[string]any{"name": "value", "value": value}},
		}
	}
	_, err := directClient(t, sonarr.url(), key).Do(context.Background(), http.MethodPost, "/api/v3/customformat",
		map[string]any{
			"name": "WEBDL", "includeCustomFormatWhenRenaming": true,
			"specifications": []any{
				condition("WEBDL", "SourceSpecification", 3),
				condition("Made In UI", "ReleaseTitleSpecification", "ui"),
			},
		})
	require.NoError(t, err)
	qbit, qbitPassword := startQBittorrent(t)
	fakes := startFakes(t)
	proxy := newRecordingProxy(t, sonarr.url())

	checkCompat(t, compatCase{
		fixture: "sonarr.yaml",
		update: `[
			{"op": "replace", "path": "/spec/customFormats/0/includeCustomFormatWhenRenaming", "value": true},
			{"op": "replace", "path": "/spec/qualityProfiles/0/minUpgradeFormatScore", "value": 10},
			{"op": "replace", "path": "/spec/notifications/0/onRename", "value": false},
			{"op": "replace", "path": "/spec/importLists/0/seasonFolder", "value": false},
			{"op": "replace", "path": "/spec/delayProfiles/0/usenetDelay", "value": 90},
			{"op": "add", "path": "/spec/releaseProfiles/0/indexerName", "value": "Compat Torznab"},
			{"op": "replace", "path": "/spec/metadataConsumers/0/fields/1/value", "value": true},
			{"op": "replace", "path": "/spec/autoTags/0/specifications/2/fields/0/value", "value": ["Animation", "Anime"]}
		]`,
		export: "sonarr",
		vars: map[string]string{
			"URL": proxy.url(), "HOST": hostIP, "FAKES": fakes,
			"QBITTORRENT_PORT": strconv.Itoa(qbit.port),
		},
		secrets: map[string]string{
			"api-key": key, "qbittorrent-username": qbittorrentUser, "qbittorrent-password": qbitPassword,
			"jellyfin-api-key": "compat-jellyfin-key", "plex-token": "compat-plex-token",
		},
		newObj: func() ctrlcommon.ConfigResource { return &pvrv1alpha1.Sonarr{} },
		reconciler: func(c client.Client) reconcile.Reconciler {
			return &pvr.SonarrReconciler{Client: c, Recorder: events.NewFakeRecorder(100)}
		},
		objects: map[string]compatKind{
			"SonarrIndexer": {
				newObj: func() ctrlcommon.ConfigResource { return &pvrv1alpha1.SonarrIndexer{} },
				reconciler: func(c client.Client) reconcile.Reconciler {
					return pvr.NewObjectReconciler(pvr.SonarrIndexers, c, events.NewFakeRecorder(100), nil)
				},
			},
			"SonarrDownloadClient": {
				newObj: func() ctrlcommon.ConfigResource { return &pvrv1alpha1.SonarrDownloadClient{} },
				reconciler: func(c client.Client) reconcile.Reconciler {
					return pvr.NewObjectReconciler(pvr.SonarrDownloadClients, c, events.NewFakeRecorder(100), nil)
				},
			},
		},
		proxy:   proxy,
		observe: true,
		verify: func(t *testing.T) {
			hc := directClient(t, sonarr.url(), key)
			requireAutoTagReference(t, hc, "/api/v3", "Compat Anime", "Profile", "/qualityprofile", "name", "HD-1080p")
			requireAutoTagReference(t, hc, "/api/v3", "Compat Anime", "Anime", "/tag", "label", "anime")
			listConfig, err := hc.GetJSON(context.Background(), "/api/v3/config/importlist")
			require.NoError(t, err)
			assert.Equal(t, "keepAndTag", listConfig["listSyncLevel"])
			tags, err := hc.GetJSONList(context.Background(), "/api/v3/tag")
			require.NoError(t, err)
			hdr := slices.IndexFunc(tags, func(tag map[string]any) bool { return tag["label"] == "hdr" })
			require.GreaterOrEqual(t, hdr, 0)
			assert.Equal(t, tags[hdr]["id"], listConfig["listSyncTag"])
			requireMediaServerNotification(t, hc, "/api/v3/notification", "Compat Jellyfin", "MediaBrowser", "/jellyfin")
			requireMediaServerNotification(t, hc, "/api/v3/notification", "Compat Plex", "PlexServer", "/plex")
			naming, err := hc.GetJSON(context.Background(), "/api/v3/config/naming")
			require.NoError(t, err)
			assert.Equal(t, "Season {season:00}", naming["seasonFolderFormat"])
			requireNamed(t, hc, "/api/v3/downloadclient", "qBittorrent")
			requireNamed(t, hc, "/api/v3/indexer", "Compat Torznab")
			requireNamed(t, hc, "/api/v3/qualityprofile", "HD-1080p")
			requireNamed(t, hc, "/api/v3/notification", "Compat Webhook")
			lists, err := hc.GetJSONList(context.Background(), "/api/v3/importlist")
			require.NoError(t, err)
			require.Len(t, lists, 1)
			assert.Equal(t, "all", lists[0]["shouldMonitor"], "import list monitor did not reach Sonarr")
			requireMetadataConsumer(t, hc, "/api/v3/metadata", "XbmcMetadata",
				map[string]any{"seriesMetadataUrl": true, "episodeImages": false})

			webdl := requireNamed(t, hc, "/api/v3/customformat", "WEBDL")
			assert.Equal(t, true, webdl["includeCustomFormatWhenRenaming"], "a field the resource leaves out stays as it was")
			specs, _ := webdl["specifications"].([]any)
			require.Len(t, specs, 1, "a condition the resource doesn't list is removed")
			assert.Equal(t, "WEBDL", specs[0].(map[string]any)["name"])

			bluray := qualityDefinition(t, hc, "/api/v3", "Bluray-1080p")
			assert.Equal(t, "Blu-ray 1080p", bluray["title"])
			assert.InDelta(t, 50.4, bluray["minSize"], 0)
			assert.Nil(t, bluray["maxSize"], "0 should be stored as unlimited")
			assert.Nil(t, bluray["preferredSize"], "0 should be stored as unlimited")
			assert.InDelta(t, 67.5, qualityDefinition(t, hc, "/api/v3", "HDTV-720p")["maxSize"], 0)

			profiles, order := delayProfiles(t, hc, "/api/v3")
			assert.Equal(t, []string{"anime", "hdr"}, order, "the adopted hdr profile should have moved after anime")
			assert.Equal(t, "torrent", profiles["anime"]["preferredProtocol"])
			assert.InDelta(t, 60, profiles["anime"]["usenetDelay"], 0)
			assert.InDelta(t, 100, profiles["anime"]["minimumCustomFormatScore"], 0)
			assert.Equal(t, false, profiles["hdr"]["enableUsenet"])
			assert.InDelta(t, 30, profiles["default"]["usenetDelay"], 0)
			assert.InDelta(t, 10, profiles["default"]["torrentDelay"], 0)
		},
		cleanup: ownedGone(directClient(t, sonarr.url(), key), pvrclient.Definition(pvrclient.Sonarr)),
	})
}

func TestRadarr(t *testing.T) {
	t.Parallel()
	radarr, key := startPVR(t, "radarr", radarrImage, 7878, "v3")
	radarr.exec(t, "mkdir -p /data/media/movies && chown -R abc:abc /data")
	qbit, qbitPassword := startQBittorrent(t)
	fakes := startFakes(t)
	proxy := newRecordingProxy(t, radarr.url())

	checkCompat(t, compatCase{
		fixture: "radarr.yaml",
		update: `[
			{"op": "replace", "path": "/spec/customFormats/0/includeCustomFormatWhenRenaming", "value": true},
			{"op": "replace", "path": "/spec/qualityProfiles/0/minUpgradeFormatScore", "value": 10},
			{"op": "replace", "path": "/spec/notifications/0/onRename", "value": false},
			{"op": "replace", "path": "/spec/importLists/0/minimumAvailability", "value": "announced"},
			{"op": "replace", "path": "/spec/delayProfiles/0/usenetDelay", "value": 180},
			{"op": "add", "path": "/spec/releaseProfiles/0/indexerName", "value": "Compat Torznab"}
		]`,
		export: "radarr",
		vars: map[string]string{
			"URL": proxy.url(), "HOST": hostIP, "FAKES": fakes,
			"QBITTORRENT_PORT": strconv.Itoa(qbit.port), "RADARR_LIST": "RadarrListImport",
		},
		secrets: map[string]string{
			"api-key": key, "qbittorrent-username": qbittorrentUser, "qbittorrent-password": qbitPassword,
			"jellyfin-api-key": "compat-jellyfin-key", "plex-token": "compat-plex-token",
		},
		newObj: func() ctrlcommon.ConfigResource { return &pvrv1alpha1.Radarr{} },
		reconciler: func(c client.Client) reconcile.Reconciler {
			return &pvr.RadarrReconciler{Client: c, Recorder: events.NewFakeRecorder(100)}
		},
		objects: map[string]compatKind{
			"RadarrIndexer": {
				newObj: func() ctrlcommon.ConfigResource { return &pvrv1alpha1.RadarrIndexer{} },
				reconciler: func(c client.Client) reconcile.Reconciler {
					return pvr.NewObjectReconciler(pvr.RadarrIndexers, c, events.NewFakeRecorder(100), nil)
				},
			},
			"RadarrDownloadClient": {
				newObj: func() ctrlcommon.ConfigResource { return &pvrv1alpha1.RadarrDownloadClient{} },
				reconciler: func(c client.Client) reconcile.Reconciler {
					return pvr.NewObjectReconciler(pvr.RadarrDownloadClients, c, events.NewFakeRecorder(100), nil)
				},
			},
		},
		proxy:   proxy,
		observe: true,
		verify: func(t *testing.T) {
			hc := directClient(t, radarr.url(), key)
			requireAutoTagReference(t, hc, "/api/v3", "Compat Kids", "Profile", "/qualityprofile", "name", "HD-1080p")
			requireMediaServerNotification(t, hc, "/api/v3/notification", "Compat Jellyfin", "MediaBrowser", "/jellyfin")
			requireMetadataConsumer(t, hc, "/api/v3/metadata", "XbmcMetadata",
				map[string]any{"movieMetadataURL": true, "movieImages": false})
			metadataConfig, err := hc.GetJSON(context.Background(), "/api/v3/config/metadata")
			require.NoError(t, err)
			assert.Equal(t, "gb", metadataConfig["certificationCountry"])
			listConfig, err := hc.GetJSON(context.Background(), "/api/v3/config/importlist")
			require.NoError(t, err)
			assert.Equal(t, "logOnly", listConfig["listSyncLevel"])
			requireMediaServerNotification(t, hc, "/api/v3/notification", "Compat Plex", "PlexServer", "/plex")
			naming, err := hc.GetJSON(context.Background(), "/api/v3/config/naming")
			require.NoError(t, err)
			assert.Equal(t, "smart", naming["colonReplacementFormat"])
			requireNamed(t, hc, "/api/v3/downloadclient", "qBittorrent")
			requireNamed(t, hc, "/api/v3/indexer", "Compat Torznab")
			requireNamed(t, hc, "/api/v3/qualityprofile", "HD-1080p")
			requireNamed(t, hc, "/api/v3/notification", "Compat Webhook")
			lists, err := hc.GetJSONList(context.Background(), "/api/v3/importlist")
			require.NoError(t, err)
			require.Len(t, lists, 1)
			assert.Equal(t, "movieOnly", lists[0]["monitor"], "import list monitor did not reach Radarr")
			assert.Equal(t, true, lists[0]["enabled"], "import list enabled did not reach Radarr")

			web := qualityDefinition(t, hc, "/api/v3", "WEBDL-1080p")
			assert.Equal(t, "WEB 1080p", web["title"])
			assert.InDelta(t, 12.5, web["minSize"], 0)
			assert.InDelta(t, 350, web["preferredSize"], 0)
			assert.Nil(t, qualityDefinition(t, hc, "/api/v3", "Bluray-1080p")["maxSize"])

			profiles, _ := delayProfiles(t, hc, "/api/v3")
			assert.Equal(t, "torrent", profiles["kids,uhd"]["preferredProtocol"])
			assert.InDelta(t, 120, profiles["kids,uhd"]["usenetDelay"], 0)
			assert.Equal(t, false, profiles["default"]["bypassIfHighestQuality"])
		},
		cleanup: ownedGone(directClient(t, radarr.url(), key), pvrclient.Definition(pvrclient.Radarr)),
	})
}

func TestLidarr(t *testing.T) {
	t.Parallel()
	lidarr, key := startPVR(t, "lidarr", lidarrImage, 8686, "v1")
	lidarr.exec(t, "mkdir -p /data/media/music && chown -R abc:abc /data")
	lidarrHC := directClient(t, lidarr.url(), key)
	seedDelayProfile(t, lidarrHC, "/api/v1", "live", lidarrDelayProfile(t, lidarrHC))
	qbit, qbitPassword := startQBittorrent(t)
	fakes := startFakes(t)
	proxy := newRecordingProxy(t, lidarr.url())

	checkCompat(t, compatCase{
		fixture: "lidarr.yaml",
		update: `[
			{"op": "replace", "path": "/spec/qualityProfiles/0/upgradeAllowed", "value": false},
			{"op": "replace", "path": "/spec/notifications/0/onRename", "value": false},
			{"op": "replace", "path": "/spec/importLists/0/monitor", "value": "none"},
			{"op": "replace", "path": "/spec/delayProfiles/0/torrentDelay", "value": 20},
			{"op": "replace", "path": "/spec/metadataProfiles/0/primaryAlbumTypes", "value": ["Album"]},
			{"op": "replace", "path": "/spec/rootFolders/0/defaultMonitorOption", "value": "missing"},
			{"op": "add", "path": "/spec/rootFolders/0/name", "value": "Compat Music"}
		]`,
		export: "lidarr",
		vars: map[string]string{
			"URL": proxy.url(), "HOST": hostIP, "FAKES": fakes, "QBITTORRENT_PORT": strconv.Itoa(qbit.port),
		},
		secrets: map[string]string{
			"api-key": key, "qbittorrent-username": qbittorrentUser, "qbittorrent-password": qbitPassword,
			"jellyfin-api-key": "compat-jellyfin-key", "plex-token": "compat-plex-token",
		},
		newObj: func() ctrlcommon.ConfigResource { return &pvrv1alpha1.Lidarr{} },
		reconciler: func(c client.Client) reconcile.Reconciler {
			return &pvr.LidarrReconciler{Client: c, Recorder: events.NewFakeRecorder(100)}
		},
		objects: map[string]compatKind{
			"LidarrIndexer": {
				newObj: func() ctrlcommon.ConfigResource { return &pvrv1alpha1.LidarrIndexer{} },
				reconciler: func(c client.Client) reconcile.Reconciler {
					return pvr.NewObjectReconciler(pvr.LidarrIndexers, c, events.NewFakeRecorder(100), nil)
				},
			},
			"LidarrDownloadClient": {
				newObj: func() ctrlcommon.ConfigResource { return &pvrv1alpha1.LidarrDownloadClient{} },
				reconciler: func(c client.Client) reconcile.Reconciler {
					return pvr.NewObjectReconciler(pvr.LidarrDownloadClients, c, events.NewFakeRecorder(100), nil)
				},
			},
		},
		proxy:   proxy,
		observe: true,
		verify: func(t *testing.T) {
			hc := directClient(t, lidarr.url(), key)
			requireAutoTagReference(t, hc, "/api/v1", "Compat Albums", "Albums", "/metadataprofile", "name", "Albums Only")
			requireAutoTagReference(t, hc, "/api/v1", "Compat Albums", "Favorites", "/tag", "label", "favorites")
			requireMediaServerNotification(t, hc, "/api/v1/notification", "Compat Jellyfin", "MediaBrowser", "/jellyfin")
			requireMetadataConsumer(t, hc, "/api/v1/metadata", "XbmcMetadata", map[string]any{"albumImages": false})
			metadata, err := hc.GetJSON(context.Background(), "/api/v1/config/metadataprovider")
			require.NoError(t, err)
			assert.Equal(t, fakes+"/musicinfo", metadata["metadataSource"])
			assert.Equal(t, "sync", metadata["writeAudioTags"])
			assert.Equal(t, true, metadata["embedCoverArt"])
			requireMediaServerNotification(t, hc, "/api/v1/notification", "Compat Plex", "PlexServer", "/plex")
			requireNamed(t, hc, "/api/v1/rootfolder", "music")
			requireNamed(t, hc, "/api/v1/downloadclient", "qBittorrent")
			requireNamed(t, hc, "/api/v1/indexer", "Compat Torznab")
			requireNamed(t, hc, "/api/v1/qualityprofile", "Music")
			requireNamed(t, hc, "/api/v1/notification", "Compat Webhook")
			lists, err := hc.GetJSONList(context.Background(), "/api/v1/importlist")
			require.NoError(t, err)
			require.Len(t, lists, 1)
			assert.Equal(t, "entireArtist", lists[0]["shouldMonitor"], "import list monitor did not reach Lidarr")

			flac := qualityDefinition(t, hc, "/api/v1", "FLAC")
			assert.Nil(t, flac["maxSize"])
			assert.InDelta(t, 900, flac["preferredSize"], 0)
			assert.Equal(t, "MP3 320", qualityDefinition(t, hc, "/api/v1", "MP3-320")["title"])

			profiles, order := delayProfiles(t, hc, "/api/v1")
			assert.Equal(t, []string{"favorites", "live"}, order, "the adopted live profile should have moved after favorites")
			if _, protocolItems := profiles["favorites"]["items"]; !protocolItems {
				assert.Equal(t, "torrent", profiles["favorites"]["preferredProtocol"])
				assert.Equal(t, false, profiles["favorites"]["enableUsenet"])
				assert.InDelta(t, 15, profiles["favorites"]["torrentDelay"], 0)
				assert.InDelta(t, 5, profiles["live"]["usenetDelay"], 0)
				assert.InDelta(t, 30, profiles["default"]["usenetDelay"], 0)
				return
			}
			items := list(profiles["favorites"]["items"])
			require.Len(t, items, 2)
			assert.Equal(t, "TorrentDownloadProtocol", items[0]["protocol"], "the preferred protocol goes first")
			assert.InDelta(t, 15, items[0]["delay"], 0)
			assert.Equal(t, false, items[1]["allowed"])
			assert.InDelta(t, 5, list(profiles["live"]["items"])[0]["delay"], 0)
			assert.InDelta(t, 30, list(profiles["default"]["items"])[0]["delay"], 0)
		},
		// The root folder defaults to this profile and is never deleted.
		// The root folder defaults to these, so the app keeps them.
		cleanup: ownedGone(directClient(t, lidarr.url(), key), pvrclient.Definition(pvrclient.Lidarr),
			"qualityProfiles/Music", "metadataProfiles/Albums Only"),
	})
}

func TestReadarr(t *testing.T) {
	t.Parallel()
	readarr, key := startPVR(t, "readarr", readarrImage, 8787, "v1")
	readarr.exec(t, "mkdir -p /data/media/books && chown -R abc:abc /data")
	fakes := startFakes(t)
	proxy := newRecordingProxy(t, readarr.url())

	checkCompat(t, compatCase{
		fixture: "readarr.yaml",
		update: `[
			{"op": "replace", "path": "/spec/qualityProfiles/0/upgradeAllowed", "value": false},
			{"op": "replace", "path": "/spec/notifications/0/onRename", "value": false},
			{"op": "replace", "path": "/spec/delayProfiles/0/torrentDelay", "value": 45},
			{"op": "replace", "path": "/spec/metadataProfiles/0/minPages", "value": 30},
			{"op": "replace", "path": "/spec/rootFolders/0/defaultTagLabels", "value": []},
			{"op": "replace", "path": "/spec/metadataProvider/writeBookTags", "value": "newFiles"}
		]`,
		export:  "readarr",
		vars:    map[string]string{"URL": proxy.url(), "HOST": hostIP, "FAKES": fakes},
		secrets: map[string]string{"api-key": key, "plex-token": "compat-plex-token"},
		newObj:  func() ctrlcommon.ConfigResource { return &pvrv1alpha1.Readarr{} },
		reconciler: func(c client.Client) reconcile.Reconciler {
			return &pvr.ReadarrReconciler{Client: c, Recorder: events.NewFakeRecorder(100)}
		},
		objects: map[string]compatKind{
			"ReadarrIndexer": {
				newObj: func() ctrlcommon.ConfigResource { return &pvrv1alpha1.ReadarrIndexer{} },
				reconciler: func(c client.Client) reconcile.Reconciler {
					return pvr.NewObjectReconciler(pvr.ReadarrIndexers, c, events.NewFakeRecorder(100), nil)
				},
			},
			"ReadarrDownloadClient": {
				newObj: func() ctrlcommon.ConfigResource { return &pvrv1alpha1.ReadarrDownloadClient{} },
				reconciler: func(c client.Client) reconcile.Reconciler {
					return pvr.NewObjectReconciler(pvr.ReadarrDownloadClients, c, events.NewFakeRecorder(100), nil)
				},
			},
		},
		proxy:   proxy,
		observe: true,
		verify: func(t *testing.T) {
			hc := directClient(t, readarr.url(), key)
			requireMediaServerNotification(t, hc, "/api/v1/notification", "Compat Plex", "PlexServer", "/plex")
			metadata, err := hc.GetJSON(context.Background(), "/api/v1/config/metadataprovider")
			require.NoError(t, err)
			assert.Equal(t, "sync", metadata["writeBookTags"])
			assert.Equal(t, true, metadata["embedMetadata"])
			development, err := hc.GetJSON(context.Background(), "/api/v1/config/development")
			require.NoError(t, err)
			assert.Equal(t, fakes+"/bookinfo", development["metadataSource"])
			requireNamed(t, hc, "/api/v1/rootfolder", "books")
			requireNamed(t, hc, "/api/v1/indexer", "Compat Torznab")
			requireNamed(t, hc, "/api/v1/qualityprofile", "Books")
			requireNamed(t, hc, "/api/v1/notification", "Compat Webhook")

			epub := qualityDefinition(t, hc, "/api/v1", "EPUB")
			assert.Equal(t, "ePub", epub["title"])
			assert.InDelta(t, 100, epub["maxSize"], 0)
			assert.Nil(t, qualityDefinition(t, hc, "/api/v1", "MP3")["maxSize"])

			profiles, _ := delayProfiles(t, hc, "/api/v1")
			assert.Equal(t, "torrent", profiles["favorites"]["preferredProtocol"])
			assert.InDelta(t, 30, profiles["favorites"]["torrentDelay"], 0)
			assert.InDelta(t, 15, profiles["default"]["usenetDelay"], 0)
		},
		// The root folder defaults to this profile and is never deleted.
		// The root folder defaults to these, so the app keeps them.
		cleanup: ownedGone(directClient(t, readarr.url(), key), pvrclient.Definition(pvrclient.Readarr),
			"qualityProfiles/Books", "metadataProfiles/English Books"),
	})
}

// requireAutoTagReference checks that an auto tag's condition holds the ID of
// the item listed at refPath under refField.
func requireAutoTagReference(t *testing.T, hc interface {
	GetJSONList(context.Context, string) ([]map[string]any, error)
}, prefix, autoTag, condition, refPath, refField, refName string) {
	t.Helper()
	at := requireNamed(t, hc, prefix+"/autotagging", autoTag)
	refs, err := hc.GetJSONList(context.Background(), prefix+refPath)
	require.NoError(t, err)
	i := slices.IndexFunc(refs, func(r map[string]any) bool { return r[refField] == refName })
	require.GreaterOrEqual(t, i, 0, "no %s %q", refPath, refName)
	specs, _ := at["specifications"].([]any)
	j := slices.IndexFunc(specs, func(s any) bool { return s.(map[string]any)["name"] == condition })
	require.GreaterOrEqual(t, j, 0, "%s has no condition %q", autoTag, condition)
	fields, _ := specs[j].(map[string]any)["fields"].([]any)
	require.NotEmpty(t, fields)
	assert.Equal(t, refs[i]["id"], fields[0].(map[string]any)["value"])
}

// requireMetadataConsumer checks that the app's consumer of a type is on, with
// the given field values.
func requireMetadataConsumer(t *testing.T, hc interface {
	GetJSONList(context.Context, string) ([]map[string]any, error)
}, path, implementation string, want map[string]any) {
	t.Helper()
	consumers, err := hc.GetJSONList(context.Background(), path)
	require.NoError(t, err)
	i := slices.IndexFunc(consumers, func(c map[string]any) bool { return c["implementation"] == implementation })
	require.GreaterOrEqual(t, i, 0, "no %s consumer", implementation)
	assert.Equal(t, true, consumers[i]["enable"], "%s is off", implementation)
	got := map[string]any{}
	fields, _ := consumers[i]["fields"].([]any)
	for _, f := range fields {
		field, _ := f.(map[string]any)
		if name, _ := field["name"].(string); want[name] != nil {
			got[name] = field["value"]
		}
	}
	assert.Equal(t, want, got)
}

// requireMediaServerNotification checks that a notification by reference
// reached the app as the media server's type, at its host and URL base.
func requireMediaServerNotification(t *testing.T, hc interface {
	GetJSONList(context.Context, string) ([]map[string]any, error)
}, path, name, implementation, urlBase string) {
	t.Helper()
	n := requireNamed(t, hc, path, name)
	assert.Equal(t, implementation, n["implementation"])
	fields := map[string]any{}
	list, _ := n["fields"].([]any)
	for _, f := range list {
		if m, ok := f.(map[string]any); ok {
			fields[fmt.Sprint(m["name"])] = m["value"]
		}
	}
	assert.Equal(t, urlBase, fields["urlBase"], "%s urlBase", name)
	assert.Equal(t, hostIP, fields["host"], "%s host", name)
	assert.Equal(t, true, fields["updateLibrary"], "%s keeps its own fields", name)
}

func requireNamed(t *testing.T, hc interface {
	GetJSONList(context.Context, string) ([]map[string]any, error)
}, path, name string) map[string]any {
	t.Helper()
	items, err := hc.GetJSONList(context.Background(), path)
	require.NoError(t, err)
	names := make([]any, 0, len(items))
	for _, item := range items {
		if item["name"] == name {
			return item
		}
		names = append(names, item["name"])
	}
	require.FailNow(t, "missing item", "%s has no %q among %v", path, name, names)
	return nil
}

// seedDelayProfile adds a tag and a delay profile holding it, as if made in the
// app, so the first reconcile has a profile to adopt and move into place.
func seedDelayProfile(t *testing.T, hc *engine.HTTPClient, prefix, tag string, profile map[string]any) {
	t.Helper()
	ctx := context.Background()
	body, err := hc.Do(ctx, http.MethodPost, prefix+"/tag", map[string]any{"label": tag})
	require.NoError(t, err)
	var created map[string]any
	require.NoError(t, json.Unmarshal(body, &created))
	profile["tags"] = []any{created["id"]}
	_, err = hc.Do(ctx, http.MethodPost, prefix+"/delayprofile", profile)
	require.NoError(t, err)
}

// lidarrDelayProfile returns a delay profile in the shape the running Lidarr
// takes: Lidarr 3.1 lists protocols as items, earlier versions have flags.
func lidarrDelayProfile(t *testing.T, hc *engine.HTTPClient) map[string]any {
	t.Helper()
	profiles, err := hc.GetJSONList(context.Background(), "/api/v1/delayprofile")
	require.NoError(t, err)
	if len(profiles) > 0 {
		if _, ok := profiles[0]["items"]; ok {
			return map[string]any{"items": []any{
				map[string]any{"name": "Usenet", "protocol": "UsenetDownloadProtocol", "allowed": true, "delay": 0},
				map[string]any{"name": "Torrent", "protocol": "TorrentDownloadProtocol", "allowed": true, "delay": 0},
			}}
		}
	}
	return map[string]any{
		"enableUsenet": true, "enableTorrent": true, "preferredProtocol": "usenet", "usenetDelay": 0, "torrentDelay": 0,
	}
}

func qualityDefinition(t *testing.T, hc *engine.HTTPClient, prefix, quality string) map[string]any {
	t.Helper()
	definitions, err := hc.GetJSONList(context.Background(), prefix+"/qualitydefinition")
	require.NoError(t, err)
	for _, d := range definitions {
		if q, _ := d["quality"].(map[string]any); q["name"] == quality {
			return d
		}
	}
	require.FailNow(t, "no quality definition", quality)
	return nil
}

// delayProfiles keys the app's delay profiles by their sorted tag labels, or
// "default", and lists the tagged ones in the order the app checks them.
func delayProfiles(t *testing.T, hc *engine.HTTPClient, prefix string) (map[string]map[string]any, []string) {
	t.Helper()
	ctx := context.Background()
	tags, err := hc.GetJSONList(ctx, prefix+"/tag")
	require.NoError(t, err)
	labels := map[float64]string{}
	for _, tag := range tags {
		labels[tag["id"].(float64)], _ = tag["label"].(string)
	}
	profiles, err := hc.GetJSONList(ctx, prefix+"/delayprofile")
	require.NoError(t, err)
	slices.SortFunc(profiles, func(a, b map[string]any) int {
		return cmp.Compare(a["order"].(float64), b["order"].(float64))
	})
	byTags := map[string]map[string]any{}
	var order []string
	for _, p := range profiles {
		var names []string
		for _, id := range p["tags"].([]any) {
			names = append(names, labels[id.(float64)])
		}
		slices.Sort(names)
		key := strings.Join(names, ",")
		if key == "" {
			key = "default"
		} else {
			order = append(order, key)
		}
		byTags[key] = p
	}
	return byTags, order
}

func list(v any) []map[string]any {
	raw, _ := v.([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}
