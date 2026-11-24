package envtest

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/yaml"
)

const group = "arrchitect.dev/v1alpha1"

func TestSamplesApply(t *testing.T) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Skip("KUBEBUILDER_ASSETS unset; run via `make test`")
	}

	env := &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}
	cfg, err := env.Start()
	require.NoError(t, err, "starting control plane")
	t.Cleanup(func() { _ = env.Stop() })

	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme), "building scheme")
	c, err := client.New(cfg, client.Options{Scheme: scheme})
	require.NoError(t, err, "building client")

	ctx := context.Background()

	dirs := []string{"config/samples", "examples"}
	patterns := make([]string, 0, len(dirs))
	for _, dir := range dirs {
		patterns = append(patterns, filepath.Join("..", "..", dir, "*.yaml"))
	}

	applied := 0
	for _, pattern := range patterns {
		paths, err := filepath.Glob(pattern)
		require.NoError(t, err, "globbing %s", pattern)
		for _, path := range paths {
			// Each file gets its own namespace. Samples reuse names across files
			// and span several namespaces; isolating them keeps a collision from
			// masking a genuine schema rejection.
			ns := namespaceFor(path)
			require.NoError(t, c.Create(ctx, &corev1.Namespace{Name: ns}), "creating namespace %s", ns)
			for _, obj := range decode(t, path) {
				if obj.GetAPIVersion() != group {
					continue
				}
				obj.SetNamespace(ns)
				name := obj.GetKind() + "/" + obj.GetName()
				if !assert.NoError(t, c.Create(ctx, obj), "%s: applying %s", filepath.Base(path), name) {
					continue
				}
				applied++
			}
		}
	}
	t.Logf("applied %d resources", applied)
	require.NotZero(t, applied, "no resources applied; sample discovery is broken")
}

// namespaceFor derives a DNS-1123 namespace name from a manifest path.
func namespaceFor(path string) string {
	base := strings.TrimSuffix(filepath.Base(path), ".yaml")
	base = strings.ToLower(strings.ReplaceAll(base, "_", "-"))
	if parent := filepath.Base(filepath.Dir(path)); parent != "" {
		base = parent + "-" + base
	}
	if len(base) > 63 {
		base = base[:63]
	}
	return strings.Trim(base, "-")
}

func decode(t *testing.T, path string) []*unstructured.Unstructured {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err, "reading %s", path)
	var out []*unstructured.Unstructured
	reader := utilyaml.NewYAMLReader(bufio.NewReader(bytes.NewReader(data)))
	for {
		doc, err := reader.Read()
		if err != nil {
			break
		}
		if len(strings.TrimSpace(string(doc))) == 0 {
			continue
		}
		obj := &unstructured.Unstructured{}
		require.NoError(t, yaml.Unmarshal(doc, obj), "parsing %s", path)
		if obj.GetKind() == "" {
			continue
		}
		out = append(out, obj)
	}
	return out
}

// TestAdmissionRules checks what the API server accepts and rejects for the
// rules the CRDs carry: list keys, CEL rules, patterns and property limits.
func TestAdmissionRules(t *testing.T) {
	c := startAPIServerWith(t)
	const conn = `
  connection:
    url: http://app:8080
    apiKeySecretRef: {name: app, key: api-key}`
	for i, tt := range []struct {
		name, kind, spec string
		rejected         string
	}{
		{name: "one delay profile without tags", kind: "Sonarr", spec: conn + `
  delayProfiles: [{tagLabels: [anime]}, {usenetDelay: 30}]`},
		{name: "two delay profiles without tags", kind: "Sonarr", spec: conn + `
  delayProfiles: [{usenetDelay: 30}, {tagLabels: []}]`,
			rejected: "only one delay profile can be without tags"},
		{name: "duplicate notification names", kind: "Radarr", spec: conn + `
  notifications: [{name: Hook, implementation: Webhook}, {name: Hook, implementation: Discord}]`,
			rejected: "Duplicate value"},
		{name: "duplicate field names", kind: "SonarrIndexer", spec: `
  sonarrRef: {name: tv}
  name: x
  protocol: torrent
  implementation: Torznab
  fields: [{name: baseUrl, value: a}, {name: baseUrl, value: b}]`,
			rejected: "Duplicate value"},
		{name: "duplicate tag labels", kind: "SonarrIndexer", spec: `
  sonarrRef: {name: tv}
  name: x
  protocol: torrent
  implementation: Torznab
  tagLabels: [anime, anime]`,
			rejected: "Duplicate value"},
		{name: "a field with both value and valueFrom", kind: "SonarrIndexer", spec: `
  sonarrRef: {name: tv}
  name: x
  protocol: torrent
  implementation: Torznab
  fields: [{name: apiKey, value: a, valueFrom: {secretKeyRef: {name: s, key: k}}}]`,
			rejected: "must have at most 2 items"},
		{name: "an indexer that names no Sonarr", kind: "SonarrIndexer", spec: `
  name: x
  protocol: torrent
  implementation: Torznab`,
			rejected: "sonarrRef: Required value"},
		{name: "an unknown deletion policy", kind: "SonarrDownloadClient", spec: `
  sonarrRef: {name: tv}
  deletionPolicy: keep
  name: x
  protocol: torrent
  implementation: QBittorrent
  host: qbittorrent
  port: 8080`,
			rejected: "Unsupported value"},
		{name: "an interval under 30s", kind: "Sonarr", spec: conn + `
  reconcile: {intervalSeconds: 10}`,
			rejected: "should be greater than or equal to 30"},
		{name: "delete in observe mode", kind: "Sonarr", spec: conn + `
  reconcile: {deletionPolicy: delete, driftPolicy: observe}`,
			rejected: "deletionPolicy delete conflicts with driftPolicy observe"},
		{name: "a URL without a scheme", kind: "Sonarr", spec: `
  connection:
    url: sonarr:8989
    apiKeySecretRef: {name: app, key: api-key}`,
			rejected: "should match '^https?://'"},
		{name: "a Radarr quality profile language", kind: "Radarr", spec: conn + `
  qualityProfiles: [{name: HD, language: English}]`},
		{name: "a Sonarr quality profile language", kind: "Sonarr", spec: conn + `
  qualityProfiles: [{name: HD, language: English}]`,
			rejected: "only Radarr quality profiles take a language"},
		{name: "a symbolic folder mode", kind: "Sonarr", spec: conn + `
  mediaManagement: {chmodFolder: rwxr-xr-x}`,
			rejected: "should match '^[0-7]{3,4}$'"},
		{name: "a Lidarr root folder without profiles", kind: "Lidarr", spec: conn + `
  rootFolders: [{path: /music}]`,
			rejected: "Lidarr root folders need defaultQualityProfileName and defaultMetadataProfileName"},
		{name: "prune on Jellyfin", kind: "Jellyfin", spec: `
  connection: {url: http://jellyfin:8096}
  adminUser:
    usernameSecretRef: {name: app, key: username}
    passwordSecretRef: {name: app, key: password}
  reconcile: {prune: true}`,
			rejected: "Jellyfin doesn't support prune"},
		{name: "Jellyfin fetchers for the types a library holds", kind: "Jellyfin", spec: `
  connection: {url: http://jellyfin:8096}
  adminUser:
    usernameSecretRef: {name: app, key: username}
    passwordSecretRef: {name: app, key: password}
  libraries:
    - name: Shows
      collectionType: tvshows
      paths: [/tv]
      subtitleFetchers: [Open Subtitles]
      typeOptions:
        - {type: Series, metadataFetchers: [TheTVDB, TheMovieDb]}
        - {type: Episode, imageFetchers: [Screen Grabber]}
    - name: Everything
      collectionType: mixed
      paths: [/other]
      typeOptions: [{type: Movie, imageFetchers: [TheMovieDb]}]`},
		{name: "Jellyfin movie fetchers in a TV library", kind: "Jellyfin", spec: `
  connection: {url: http://jellyfin:8096}
  adminUser:
    usernameSecretRef: {name: app, key: username}
    passwordSecretRef: {name: app, key: password}
  libraries:
    - name: Shows
      collectionType: tvshows
      paths: [/tv]
      typeOptions: [{type: Movie, metadataFetchers: [TheMovieDb]}]`,
			rejected: "typeOptions names a type the library's collectionType doesn't hold"},
		{name: "Jellyfin type options without fetchers", kind: "Jellyfin", spec: `
  connection: {url: http://jellyfin:8096}
  adminUser:
    usernameSecretRef: {name: app, key: username}
    passwordSecretRef: {name: app, key: password}
  libraries: [{name: Movies, collectionType: movies, paths: [/movies], typeOptions: [{type: Movie}]}]`,
			rejected: "set metadataFetchers, imageFetchers or both"},
		{name: "qBittorrent banned addresses", kind: "QBittorrent", spec: `
  connection:
    url: http://qbittorrent:8080
    usernameSecretRef: {name: app, key: username}
    passwordSecretRef: {name: app, key: password}
  preferences: {bannedIps: [10.0.0.2, "2001:db8::1"], proxyType: SOCKS5, proxyPort: 1080}`},
		{name: "a qBittorrent banned subnet", kind: "QBittorrent", spec: `
  connection:
    url: http://qbittorrent:8080
    usernameSecretRef: {name: app, key: username}
    passwordSecretRef: {name: app, key: password}
  preferences: {bannedIps: [10.0.0.0/8]}`,
			rejected: "bannedIps holds IP addresses"},
		{name: "Bazarr search tuning and profile filters", kind: "Bazarr", spec: conn + `
  general: {adaptiveSearchingDelay: 2w, adaptiveSearchingDelta: 3d, daysToUpgradeSubs: 30, upgradeFrequency: 876000,
    seriesTagEnabled: true, removeProfileTags: [no-subs]}
  languages:
    profiles:
      - profileId: 1
        name: English
        items: [{language: en}, {language: es, audioOnlyInclude: true}]
        cutoff: "en:hi"
        mustContain: [WEB]
        tag: english`},
		{name: "a Bazarr upgrade frequency Bazarr refuses", kind: "Bazarr", spec: conn + `
  general: {upgradeFrequency: 8}`,
			rejected: "Unsupported value"},
		{name: "a Bazarr profile item both excluded and included by audio", kind: "Bazarr", spec: conn + `
  languages:
    profiles: [{profileId: 1, name: English, items: [{language: en, audioExclude: true, audioOnlyInclude: true}]}]`,
			rejected: "audioExclude and audioOnlyInclude can't both be true"},
		{name: "a Bazarr profile tag with capitals", kind: "Bazarr", spec: conn + `
  languages:
    profiles: [{profileId: 1, name: English, items: [{language: en}], tag: English}]`,
			rejected: "should match"},
		{name: "SABnzbd login, quota and duplicate handling", kind: "Sabnzbd", spec: conn + `
  general: {duplicateDetection: tag, smartDuplicateDetection: pause, preQueueScript: check.sh}
  servers: [{name: Primary, host: news.example.com, port: 563, timeout: 20, sslCiphers: AES128}]
  login: {usernameSecretRef: {name: app, key: username}, passwordSecretRef: {name: app, key: password}}
  quota: {size: 1.5T, period: week, resetDay: 7, resetTime: "3:00", resume: true}`},
		{name: "a SABnzbd quota without a size", kind: "Sabnzbd", spec: conn + `
  quota: {period: month}`,
			rejected: "size is required unless enabled is false"},
		{name: "a turned-off SABnzbd quota", kind: "Sabnzbd", spec: conn + `
  quota: {enabled: false}`},
		{name: "a weekly SABnzbd quota on day 8", kind: "Sabnzbd", spec: conn + `
  quota: {size: 500G, period: week, resetDay: 8}`,
			rejected: "a weekly quota resets on day 1 (Monday) to 7"},
		{name: "a SABnzbd quota size SABnzbd reads as nothing", kind: "Sabnzbd", spec: conn + `
  quota: {size: 500GB}`,
			rejected: "should match"},
		{name: "a SABnzbd server timeout SABnzbd raises to 20", kind: "Sabnzbd", spec: conn + `
  servers: [{name: Primary, host: news.example.com, port: 563, timeout: 10}]`,
			rejected: "should be greater than or equal to 20"},
		{name: "deletionPolicy delete on Plex", kind: "Plex", spec: `
  connection:
    url: http://plex:32400
    tokenSecretRef: {name: app, key: token}
  reconcile: {deletionPolicy: delete}`,
			rejected: "Plex doesn't support deletionPolicy delete"},
		{name: "Seerr with both auths", kind: "Seerr", spec: `
  connection:
    url: http://seerr:5055
    apiKeySecretRef: {name: app, key: api-key}
  jellyfinAuth:
    hostname: jellyfin
    port: 8096
    usernameSecretRef: {name: app, key: username}
    passwordSecretRef: {name: app, key: password}
  plexAuth:
    hostname: plex
    tokenSecretRef: {name: app, key: token}`,
			rejected: "jellyfinAuth and plexAuth are mutually exclusive"},
		{name: "a Prowlarr application by reference", kind: "Prowlarr", spec: conn + `
  applications: [{name: Sonarr, appRef: {kind: Sonarr, name: sonarr}}]`},
		{name: "a Prowlarr application by reference and address", kind: "Prowlarr", spec: conn + `
  applications: [{name: Sonarr, appRef: {kind: Sonarr, name: sonarr}, baseUrl: http://sonarr:8989}]`,
			rejected: "set appRef, or implementation, baseUrl and apiKeySecretRef, not both"},
		{name: "a Prowlarr application with neither", kind: "Prowlarr", spec: conn + `
  applications: [{name: Sonarr, implementation: Sonarr}]`,
			rejected: "set appRef, or implementation, baseUrl and apiKeySecretRef, not both"},
		{name: "anime categories on a Sonarr application", kind: "Prowlarr", spec: conn + `
  applications:
    - {name: Sonarr, appRef: {kind: Sonarr, name: sonarr}, animeSyncCategories: [5070]}
    - name: Anime
      implementation: Sonarr
      baseUrl: "http://anime:8989"
      apiKeySecretRef: {name: app, key: k}
      syncAnimeStandardFormatSearch: false`},
		{name: "anime categories on a Radarr application", kind: "Prowlarr", spec: conn + `
  applications: [{name: Radarr, appRef: {kind: Radarr, name: radarr}, animeSyncCategories: [5070]}]`,
			rejected: "animeSyncCategories and syncAnimeStandardFormatSearch are for Sonarr applications"},
		{name: "notifications to Jellyfin and Plex by reference", kind: "Sonarr", spec: conn + `
  notifications:
    - name: Jellyfin
      appRef: {kind: Jellyfin, name: jellyfin}
      onDownload: true
      fields: [{name: updateLibrary, value: true}]
    - {name: Plex, appRef: {kind: Plex, name: plex}}`},
		{name: "a notification by reference and implementation", kind: "Radarr", spec: conn + `
  notifications: [{name: Jellyfin, appRef: {kind: Jellyfin, name: jellyfin}, implementation: MediaBrowser}]`,
			rejected: "set appRef, or implementation, not both"},
		{name: "a notification with neither", kind: "Lidarr", spec: conn + `
  notifications: [{name: Discord}]`,
			rejected: "set appRef, or implementation, not both"},
		{name: "a notification referring to Sonarr", kind: "Radarr", spec: conn + `
  notifications: [{name: Sonarr, appRef: {kind: Sonarr, name: sonarr}}]`,
			rejected: "a notification refers to a Jellyfin or Plex"},
		{name: "a Readarr notification to Plex by reference", kind: "Readarr", spec: conn + `
  notifications: [{name: Plex, appRef: {kind: Plex, name: plex}}]`},
		{name: "a Readarr notification to Jellyfin by reference", kind: "Readarr", spec: conn + `
  notifications: [{name: Jellyfin, appRef: {kind: Jellyfin, name: jellyfin}}]`,
			rejected: "Readarr has no Jellyfin notification"},
		{name: "a Prowlarr notification by reference", kind: "Prowlarr", spec: conn + `
  notifications: [{name: Plex, appRef: {kind: Plex, name: plex}}]`,
			rejected: "Prowlarr has no media server notifications to refer to"},
		{name: "a Prowlarr application referring to Seerr", kind: "Prowlarr", spec: conn + `
  applications: [{name: Seerr, appRef: {kind: Seerr, name: seerr}}]`,
			rejected: "an application refers to a Sonarr, Radarr, Lidarr or Readarr"},
		{name: "a reference to an unknown kind", kind: "Prowlarr", spec: conn + `
  applications: [{name: Whisparr, appRef: {kind: Whisparr, name: whisparr}}]`,
			rejected: "Unsupported value"},
		{name: "a reference to another group", kind: "Prowlarr", spec: conn + `
  applications: [{name: Sonarr, appRef: {group: example.com, kind: Sonarr, name: sonarr}}]`,
			rejected: "Unsupported value"},
		{name: "a reference by an invalid name", kind: "Prowlarr", spec: conn + `
  applications: [{name: Sonarr, appRef: {kind: Sonarr, name: Not_A_Name}}]`,
			rejected: "should match"},
		{name: "Seerr servers and media server by reference", kind: "Seerr", spec: `
  connection: {url: http://seerr:5055}
  jellyfinAuth: {appRef: {kind: Jellyfin, name: jellyfin}}
  sonarr: [{name: Sonarr, appRef: {kind: Sonarr, name: sonarr}}]
  radarr: [{name: Radarr, appRef: {kind: Radarr, name: radarr}}]`},
		{name: "a Seerr sonarr server referring to Radarr", kind: "Seerr", spec: `
  connection: {url: http://seerr:5055}
  sonarr: [{name: Sonarr, appRef: {kind: Radarr, name: radarr}}]`,
			rejected: "a sonarr server refers to a Sonarr"},
		{name: "an empty Seerr jellyfinAuth", kind: "Seerr", spec: `
  connection: {url: http://seerr:5055}
  jellyfinAuth: {}`,
			rejected: "should have at least 1 properties"},
		{name: "Seerr discovery, blocklist and quota settings", kind: "Seerr", spec: `
  connection: {url: http://seerr:5055}
  main:
    discoverRegion: US
    originalLanguages: [en, ja]
    blocklistedTags: [210024]
    blocklistedTagsLimit: 250
    defaultQuotas: {movie: {limit: 0, days: 7}, tv: {limit: 10}}`},
		{name: "a lowercase Seerr discover region", kind: "Seerr", spec: `
  connection: {url: http://seerr:5055}
  main: {discoverRegion: us}`,
			rejected: "should match"},
		{name: "an empty Seerr movie quota", kind: "Seerr", spec: `
  connection: {url: http://seerr:5055}
  main: {defaultQuotas: {movie: {}}}`,
			rejected: "should have at least 1 properties"},
		{name: "a Seerr plexAuth by reference and hostname", kind: "Seerr", spec: `
  connection: {url: http://seerr:5055}
  plexAuth: {appRef: {kind: Plex, name: plex}, hostname: plex}`,
			rejected: "set appRef, or hostname and tokenSecretRef, not both"},
		{name: "Bazarr connections by reference", kind: "Bazarr", spec: conn + `
  sonarrConnection: {appRef: {kind: Sonarr, name: sonarr}}
  radarrConnection: {appRef: {kind: Radarr, name: radarr}, syncInterval: 60}`},
		{name: "a Bazarr sonarrConnection referring to Radarr", kind: "Bazarr", spec: conn + `
  sonarrConnection: {appRef: {kind: Radarr, name: radarr}}`,
			rejected: "sonarrConnection refers to a Sonarr"},
		{name: "Bazarr Jellyfin by reference", kind: "Bazarr", spec: conn + `
  jellyfin:
    appRef: {kind: Jellyfin, name: jellyfin}
    movieLibraries: [Movies]
    seriesLibraries: [TV Shows]
    refreshMethod: async`},
		{name: "Bazarr Jellyfin by URL", kind: "Bazarr", spec: conn + `
  jellyfin: {url: "http://jellyfin:8096", apiKeySecretRef: {name: jellyfin, key: api-key}}`},
		{name: "Bazarr Jellyfin by reference and URL", kind: "Bazarr", spec: conn + `
  jellyfin: {appRef: {kind: Jellyfin, name: jellyfin}, url: "http://jellyfin:8096"}`,
			rejected: "set appRef, or url and apiKeySecretRef, not both"},
		{name: "Bazarr Jellyfin by URL without a key", kind: "Bazarr", spec: conn + `
  jellyfin: {url: "http://jellyfin:8096"}`,
			rejected: "set appRef, or url and apiKeySecretRef, not both"},
		{name: "a Bazarr jellyfin referring to Plex", kind: "Bazarr", spec: conn + `
  jellyfin: {appRef: {kind: Plex, name: plex}}`,
			rejected: "jellyfin refers to a Jellyfin"},
		{name: "Maintainerr connections by reference", kind: "Maintainerr", spec: `
  connection: {url: http://maintainerr:6246}
  mediaServer: {plex: {appRef: {kind: Plex, name: plex}}}
  radarr: [{name: Radarr, appRef: {kind: Radarr, name: radarr}}]
  seerr: {appRef: {kind: Seerr, name: seerr}}`},
		{name: "Maintainerr's Jellyfin by reference", kind: "Maintainerr", spec: `
  connection: {url: http://maintainerr:6246}
  mediaServer: {jellyfin: {appRef: {kind: Jellyfin, name: jellyfin}, userId: abc}}`},
		{name: "Maintainerr's Jellyfin referring to Plex", kind: "Maintainerr", spec: `
  connection: {url: http://maintainerr:6246}
  mediaServer: {jellyfin: {appRef: {kind: Plex, name: plex}}}`,
			rejected: "jellyfin refers to a Jellyfin"},
		{name: "Maintainerr's Jellyfin by reference and URL", kind: "Maintainerr", spec: `
  connection: {url: http://maintainerr:6246}
  mediaServer: {jellyfin: {appRef: {kind: Jellyfin, name: jellyfin}, url: http://jellyfin:8096}}`,
			rejected: "set appRef, or url and apiKeySecretRef, not both"},
		{name: "a Jellyfin with an API key", kind: "Jellyfin", spec: `
  connection:
    url: http://jellyfin:8096
    apiKeySecretRef: {name: app, key: jellyfin-api-key}
  adminUser:
    usernameSecretRef: {name: app, key: username}
    passwordSecretRef: {name: app, key: password}`},
		{name: "a Maintainerr radarr server referring to Sonarr", kind: "Maintainerr", spec: `
  connection: {url: http://maintainerr:6246}
  radarr: [{name: Radarr, appRef: {kind: Sonarr, name: sonarr}}]`,
			rejected: "a radarr server refers to a Radarr"},
		{name: "a download client by reference", kind: "SonarrDownloadClient", spec: `
  sonarrRef: {name: tv}
  name: qBittorrent
  category: tv
  appRef: {kind: QBittorrent, name: qbittorrent}`},
		{name: "a download client by reference and host", kind: "SonarrDownloadClient", spec: `
  sonarrRef: {name: tv}
  name: qBittorrent
  appRef: {kind: QBittorrent, name: qbittorrent}
  host: qbittorrent`,
			rejected: "set appRef, or implementation, protocol, host and port, not both"},
		{name: "a download client referring to Sonarr", kind: "SonarrDownloadClient", spec: `
  sonarrRef: {name: tv}
  name: Sonarr
  appRef: {kind: Sonarr, name: sonarr}`,
			rejected: "a download client refers to a QBittorrent or Sabnzbd"},
		{name: "an Autobrr download client by reference", kind: "Autobrr", spec: conn + `
  downloadClients: [{name: Radarr, appRef: {kind: Radarr, name: radarr}}]`},
		{name: "an Autobrr download client referring to Plex", kind: "Autobrr", spec: conn + `
  downloadClients: [{name: Plex, appRef: {kind: Plex, name: plex}}]`,
			rejected: "a download client refers to a QBittorrent, Sabnzbd, Sonarr, Radarr, Lidarr or Readarr"},
		{name: "Autobrr lists", kind: "Autobrr", spec: conn + `
  lists:
    - {name: Shows, type: Sonarr, downloadClientName: Sonarr, filters: [TV]}
    - {name: Picks, type: Trakt, url: "https://trakt.example/list", filters: [TV]}`},
		{name: "an Autobrr Sonarr list without a download client", kind: "Autobrr", spec: conn + `
  lists: [{name: Shows, type: Sonarr, filters: [TV]}]`,
			rejected: "a Sonarr, Radarr, Lidarr, Readarr or Whisparr list needs downloadClientName"},
		{name: "an Autobrr Trakt list without a URL", kind: "Autobrr", spec: conn + `
  lists: [{name: Picks, type: Trakt, filters: [TV]}]`,
			rejected: "an MDBList, Metacritic, Plaintext, Trakt, Steam or AniList list needs url"},
		{name: "an Autobrr list without filters", kind: "Autobrr", spec: conn + `
  lists: [{name: Picks, type: Plaintext, url: "http://list", filters: []}]`,
			rejected: "should have at least 1 items"},
		{name: "Autobrr proxies", kind: "Autobrr", spec: conn + `
  proxies:
    - {name: VPN, type: Socks5, addr: "socks5h://vpn:1080"}
    - {name: Web, type: HTTP, addr: "https://proxy:8443"}`},
		{name: "an Autobrr Socks5 proxy with an HTTP address", kind: "Autobrr", spec: conn + `
  proxies: [{name: VPN, type: Socks5, addr: "http://vpn:8080"}]`,
			rejected: "a Socks5 proxy's addr starts with socks5:// or socks5h://"},
		{name: "Autobrr download client rules and HTTP auth", kind: "Autobrr", spec: conn + `
  downloadClients:
    - name: qBittorrent
      appRef: {kind: QBittorrent, name: qbittorrent}
      rules: {maxActiveDownloads: 3, ignoreSlowTorrents: true, ignoreSlowTorrentsCondition: Always}
      httpAuth: {usernameSecretRef: {name: s, key: u}, passwordSecretRef: {name: s, key: p}}
    - {name: Deluge, type: Deluge, host: deluge, rules: {maxActiveDownloads: 2}}
    - name: rTorrent
      type: rTorrent
      host: "http://rtorrent/RPC2"
      httpAuth: {type: Digest, usernameSecretRef: {name: s, key: u}, passwordSecretRef: {name: s, key: p}}
    - {name: Sonarr, appRef: {kind: Sonarr, name: sonarr}, externalDownloadClient: qBittorrent}`},
		{name: "Autobrr rules on a SABnzbd client", kind: "Autobrr", spec: conn + `
  downloadClients: [{name: SABnzbd, appRef: {kind: Sabnzbd, name: sabnzbd}, rules: {maxActiveDownloads: 3}}]`,
			rejected: "only qBittorrent, Deluge and Transmission clients take rules"},
		{name: "Autobrr slow torrent rules on a Deluge client", kind: "Autobrr", spec: conn + `
  downloadClients: [{name: Deluge, type: Deluge, host: deluge, rules: {downloadSpeedThreshold: 100}}]`,
			rejected: "only qBittorrent clients take the slow torrent rules"},
		{name: "Autobrr slow torrent rules without a condition", kind: "Autobrr", spec: conn + `
  downloadClients: [{name: qBit, type: qBittorrent, host: qbit, rules: {ignoreSlowTorrents: true}}]`,
			rejected: "ignoreSlowTorrents needs ignoreSlowTorrentsCondition"},
		{name: "Autobrr HTTP auth on a Transmission client", kind: "Autobrr", spec: conn + `
  downloadClients:
    - name: Transmission
      type: Transmission
      host: transmission
      httpAuth: {usernameSecretRef: {name: s, key: u}, passwordSecretRef: {name: s, key: p}}`,
			rejected: "Deluge and Transmission clients don't take httpAuth"},
		{name: "Autobrr Digest auth on a qBittorrent client", kind: "Autobrr", spec: conn + `
  downloadClients:
    - name: qBit
      type: qBittorrent
      host: qbit
      httpAuth: {type: Digest, usernameSecretRef: {name: s, key: u}, passwordSecretRef: {name: s, key: p}}`,
			rejected: "only rTorrent clients take Digest authentication"},
		{name: "Autobrr external download client on a qBittorrent client", kind: "Autobrr", spec: conn + `
  downloadClients: [{name: qBit, type: qBittorrent, host: qbit, externalDownloadClient: qBittorrent}]`,
			rejected: "only Sonarr, Radarr, Lidarr, Readarr and Whisparr clients take externalDownloadClient"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			obj := &unstructured.Unstructured{}
			require.NoError(t, yaml.Unmarshal([]byte(fmt.Sprintf(`
apiVersion: arrchitect.dev/v1alpha1
kind: %s
metadata:
  name: rule-%d
  namespace: default
spec:%s`, tt.kind, i, tt.spec)), &obj.Object))
			err := c.Create(context.Background(), obj)
			if tt.rejected == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.rejected)
		})
	}
}

// An object deletes what it created along with itself unless told otherwise,
// as a Kubernetes object owning an external one does.
func TestObjectKindsDefaultToDeletingWhatTheyCreated(t *testing.T) {
	c := startAPIServerWith(t)
	obj := &unstructured.Unstructured{}
	require.NoError(t, yaml.Unmarshal([]byte(`
apiVersion: arrchitect.dev/v1alpha1
kind: SonarrIndexer
metadata:
  name: defaulted
  namespace: default
spec:
  sonarrRef: {name: tv}
  name: x
  protocol: torrent
  implementation: Torznab`), &obj.Object))
	require.NoError(t, c.Create(context.Background(), obj))
	policy, _, err := unstructured.NestedString(obj.Object, "spec", "deletionPolicy")
	require.NoError(t, err)
	assert.Equal(t, "delete", policy)
}
