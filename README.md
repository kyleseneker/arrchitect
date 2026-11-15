# media-operator

[![CI](https://github.com/kyleseneker/media-operator/actions/workflows/ci.yml/badge.svg)](https://github.com/kyleseneker/media-operator/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/kyleseneker/media-operator)](https://github.com/kyleseneker/media-operator/releases)

Kubernetes operators that configure Sonarr, Radarr, Prowlarr, Jellyfin, Plex
and other self-hosted media apps from custom resources.

## Features

- Settings, download clients, indexers, quality profiles, notifications and other app configuration as YAML
- Drift correction on every reconcile, or an observe mode that only reports differences
- Credentials read from Kubernetes Secrets
- Optional pruning, limited to what the operator created
- `media-operator export` to generate config from an app you already run
- Tested in CI against each app running in a container

## Supported apps

| App | Resource | Tested version |
| --- | --- | --- |
| Sonarr | `SonarrConfig` | 4.0.20 |
| Radarr | `RadarrConfig` | 6.4.4 |
| Lidarr | `LidarrConfig` | 3.1.6 |
| Readarr | `ReadarrConfig` | 0.4.18 |
| Prowlarr | `ProwlarrConfig` | 2.7.0 |
| FlareSolverr | `FlareSolverrConfig` | 3.5.2 |
| Bazarr | `BazarrConfig` | 1.6.2 |
| qBittorrent | `QBittorrentConfig` | 5.3.0 |
| SABnzbd | `SabnzbdConfig` | 5.1.3 |
| Jellyfin | `JellyfinConfig` | 12.4 |
| Plex | `PlexConfig` | 1.43.5 |
| Seerr | `SeerrConfig` | 3.5.1 |
| Maintainerr | `MaintainerrConfig` | 3.32.0 |
| Tdarr | `TdarrConfig` | 3.06.00 |
| Autobrr | `AutobrrConfig` | 1.90.0 |

## Getting started

Install the chart. It runs every app's controller unless you turn some off in
`apps`:

```bash
helm install media-operator oci://ghcr.io/kyleseneker/charts/media-operator \
  --namespace media --set apps.plex=false --set apps.autobrr=false
```

Then apply a config. [examples/minimal-sonarr.yaml](examples/minimal-sonarr.yaml)
adds a root folder to Sonarr:

```bash
kubectl apply -f examples/minimal-sonarr.yaml
kubectl get sonarrconfig -n media
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
