# Arrchitect

[![CI](https://github.com/kyleseneker/arrchitect/actions/workflows/ci.yml/badge.svg)](https://github.com/kyleseneker/arrchitect/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/kyleseneker/arrchitect)](https://github.com/kyleseneker/arrchitect/releases)

Configure and wire Sonarr, Radarr, Prowlarr, Jellyfin, Plex and other
self-hosted media apps as Kubernetes resources. Arrchitect configures the apps
you already deploy, with app-template or any other chart, and connects them to
each other; it doesn't deploy them.

## Features

- Settings, download clients, indexers, quality profiles, notifications and other app configuration as YAML
- Drift correction on every reconcile, or an observe mode that only reports differences
- Credentials read from Kubernetes Secrets
- Optional pruning, limited to what the operator created
- `arrchitect export` to generate config from an app you already run
- Tested in CI against each app running in a container, at its current and previous version

## Supported apps

Every release is tested against each app's current version and the newest
release of the line before it.

| App | Resource | Tested versions |
| --- | --- | --- |
| Sonarr | `Sonarr`, `SonarrIndexer`, `SonarrDownloadClient` | 4.0.20, 4.0.19 |
| Radarr | `Radarr`, `RadarrIndexer`, `RadarrDownloadClient` | 6.4.4, 6.3.0 |
| Lidarr | `Lidarr`, `LidarrIndexer`, `LidarrDownloadClient` | 3.1.6, 3.0.1 |
| Readarr | `Readarr`, `ReadarrIndexer`, `ReadarrDownloadClient` | 0.4.18, 0.4.17 |
| Prowlarr | `ProwlarrConfig` | 2.7.0, 2.6.5 |
| FlareSolverr | `FlareSolverrConfig` | 3.5.2, 3.4.6 |
| Bazarr | `BazarrConfig` | 1.6.2, 1.5.6 |
| qBittorrent | `QBittorrentConfig` | 5.3.0, 5.2.4 |
| SABnzbd | `SabnzbdConfig` | 5.1.3, 5.0.4 |
| Jellyfin | `JellyfinConfig` | 12.4, 12.3 |
| Plex | `PlexConfig` | 1.43.5, 1.42.2 |
| Seerr | `SeerrConfig` | 3.5.1, 3.5.0 |
| Maintainerr | `MaintainerrConfig` | 3.32.0, 3.31.0 |
| Tdarr | `TdarrConfig` | 3.06.00, 3.05.00 |
| Autobrr | `AutobrrConfig` | 1.90.0, 1.89.0 |

## Getting started

Install the chart. It runs every app's controller unless you turn some off in
`apps`:

```bash
helm install arrchitect oci://ghcr.io/kyleseneker/charts/arrchitect \
  --namespace media --set apps.plex=false --set apps.autobrr=false
```

Then apply a config. [examples/minimal-sonarr.yaml](examples/minimal-sonarr.yaml)
adds a root folder to Sonarr:

```bash
kubectl apply -f examples/minimal-sonarr.yaml
kubectl get sonarr -n media
```

To start from an existing setup instead, see
[Exporting an existing app](docs/export.md).

## Documentation

- [User guide](docs/guide.md)
- [Exporting an existing app](docs/export.md)
- [Rebuilding apps](docs/rebuilding.md)
- [Examples](examples/) and [one sample per resource](config/samples/)

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Report security issues as described in
[SECURITY.md](SECURITY.md).

## License

[Apache 2.0](LICENSE)
