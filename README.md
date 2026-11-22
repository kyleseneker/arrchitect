# Arrchitect

[![CI](https://github.com/kyleseneker/arrchitect/actions/workflows/ci.yml/badge.svg)](https://github.com/kyleseneker/arrchitect/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/kyleseneker/arrchitect)](https://github.com/kyleseneker/arrchitect/releases)

Configure and wire Sonarr, Radarr, Prowlarr, Jellyfin, Plex and other
self-hosted media apps as Kubernetes resources. Arrchitect configures the apps
you already deploy, however you deploy them, and connects them to each other;
it doesn't deploy them.

## Features

- Settings, download clients, indexers, quality profiles, notifications and other app configuration as YAML
- Drift correction on every reconcile, or an observe mode that only reports differences
- Credentials read from Kubernetes Secrets
- Optional pruning, limited to what the operator created
- `arrchitect export` to generate config from an app you already run
- Tested in CI against each app running in a container, at its current and previous version

## Supported apps

Each app is supported at every release from the oldest to the newest in its
range, and every Arrchitect release is tested against both ends. A range
covers the releases of the last 12 months; see the
[stability policy](docs/stability.md#apps) for how it moves and the
exceptions.

| App | Resource | Supported versions |
| --- | --- | --- |
| Sonarr | `Sonarr`, `SonarrIndexer`, `SonarrDownloadClient` | 4.0.16 to 4.0.20 |
| Radarr | `Radarr`, `RadarrIndexer`, `RadarrDownloadClient` | 6.0.4 to 7.0.3 |
| Lidarr | `Lidarr`, `LidarrIndexer`, `LidarrDownloadClient` | 3.1.0 to 3.1.6 |
| Readarr | `Readarr`, `ReadarrIndexer`, `ReadarrDownloadClient` | 0.4.17 to 0.4.18 |
| Prowlarr | `Prowlarr` | 2.3.0 to 3.0.2 |
| FlareSolverr | `FlareSolverr` | 3.4.5 to 3.5.2 |
| Bazarr | `Bazarr` | 1.5.3 to 1.6.2 |
| qBittorrent | `QBittorrent` | 5.1.4 to 5.3.0 |
| SABnzbd | `Sabnzbd` | 4.5.5 to 5.1.3 |
| Jellyfin | `Jellyfin` | 10.11.3 to 12.4 |
| Plex | `Plex` | 1.42.2 to 1.43.5 |
| Seerr | `Seerr` | 3.5.0 to 3.5.2 |
| Maintainerr | `Maintainerr` | 3.30.0 to 3.33.0 |
| Tdarr | `Tdarr` | 3.00.00 to 3.08.00 |
| Autobrr | `Autobrr` | 1.69.0 to 1.91.1 |

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
- [Example stack](docs/example-stack.md)
- [Sharing apps with other tools](docs/other-tools.md) such as Recyclarr and Prowlarr
- [API reference](docs/api.md)
- [Stability and support](docs/stability.md)
- [Exporting an existing app](docs/export.md)
- [Rebuilding apps](docs/rebuilding.md)
- [Examples](examples/) and [one sample per resource](config/samples/)

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Report security issues as described in
[SECURITY.md](SECURITY.md).

## License

[Apache 2.0](LICENSE)
