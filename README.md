<p align="center">
  <img src="docs/assets/logo.svg" width="128" height="128" alt="">
</p>

<h1 align="center">Arrchitect</h1>

<p align="center">The keystone of your arr stack</p>

<p align="center">
  <a href="https://github.com/kyleseneker/arrchitect/actions/workflows/ci.yml"><img src="https://github.com/kyleseneker/arrchitect/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://github.com/kyleseneker/arrchitect/releases"><img src="https://img.shields.io/github/v/release/kyleseneker/arrchitect" alt="Release"></a>
  <a href="https://artifacthub.io/packages/helm/arrchitect/arrchitect"><img src="https://img.shields.io/endpoint?url=https://artifacthub.io/badge/repository/arrchitect" alt="Artifact Hub"></a>
</p>

Configure and wire Sonarr, Radarr, Prowlarr, Jellyfin, Plex and other
self-hosted media apps as Kubernetes resources. Arrchitect doesn't deploy the
apps. It configures the ones you already run, however you deploy them, and
connects them to each other.

Your app settings live in git with the rest of your cluster, instead of in a
dozen web UIs. Apps find each other by reference, so an address or API key is
written once. A rebuilt app is configured again on the next reconcile, and a
change made in an app's web UI is put back or reported.

Arrchitect is alpha. The API is `v1alpha1` and can change in any minor release.
See [stability and support](docs/stability.md) for details.

## What it looks like

Each app is a resource. To connect two apps, point `appRef` at the other app's
resource instead of repeating its address and credentials. This sets up Sonarr
with a root folder and qBittorrent as its download client, and has Prowlarr
sync its indexers to Sonarr:

```yaml
apiVersion: arrchitect.dev/v1alpha1
kind: Sonarr
metadata:
  name: sonarr
  namespace: media
spec:
  connection:
    url: http://sonarr.media.svc.cluster.local:8989
    apiKeySecretRef:
      name: sonarr-credentials
      key: api-key
  rootFolders:
    - path: /data/media/tv
---
apiVersion: arrchitect.dev/v1alpha1
kind: SonarrDownloadClient
metadata:
  name: sonarr-qbittorrent
  namespace: media
spec:
  sonarrRef:
    name: sonarr
  name: qBittorrent
  appRef:
    kind: QBittorrent
    name: qbittorrent
  category: tv
---
apiVersion: arrchitect.dev/v1alpha1
kind: Prowlarr
metadata:
  name: prowlarr
  namespace: media
spec:
  connection:
    url: http://prowlarr.media.svc.cluster.local:9696
    apiKeySecretRef:
      name: prowlarr-credentials
      key: api-key
  applications:
    - name: Sonarr
      appRef:
        kind: Sonarr
        name: sonarr
```

If an app's address or credentials change, you only update its own resource.
See [examples/jellyfin-stack.yaml](examples/jellyfin-stack.yaml) for a full
stack, and [connecting apps](docs/guide.md#connecting-apps) for every
connection that supports `appRef`.

## Getting started

Install the chart in a namespace of its own. It runs a controller for every
supported app unless you turn some off under `apps`:

```bash
helm install arrchitect oci://ghcr.io/kyleseneker/charts/arrchitect \
  --namespace arrchitect --create-namespace \
  --set apps.plex=false --set apps.autobrr=false
```

Then try it on one app. Download
[examples/minimal-sonarr.yaml](examples/minimal-sonarr.yaml), which adds a root
folder to Sonarr, and change three things to match your Sonarr:

- the API key in the Secret, which Sonarr shows under Settings, General
- the URL, if Sonarr isn't at `sonarr.media.svc.cluster.local:8989`
- the root folder, to a path Sonarr can see

```bash
kubectl apply -f minimal-sonarr.yaml
kubectl get sonarr -n media
```

```text
NAME     SYNCED   READY   LAST SYNC   AGE
sonarr   True     True    12s         15s
```

`kubectl get arrchitect -A` lists every Arrchitect resource in the cluster. To
start from the apps you already run, see
[Exporting an existing app](docs/export.md).

## Features

- Settings, download clients, indexers, quality profiles, notifications and other app configuration as YAML
- Connections between apps by reference, with `appRef`
- References to other items by name instead of ID, so a rebuilt app gets the same configuration
- Drift correction on every reconcile, or an observe mode that only reports differences
- Credentials read from Kubernetes Secrets
- Optional pruning, limited to what the operator created
- `arrchitect export` to generate config from an app you already run
- Status conditions on every resource, and Prometheus metrics
- Tested in CI against real app containers, at the oldest and newest supported version of each app

## Supported apps

Each app is supported at every release from the oldest to the newest in its
range, and every Arrchitect release is tested against both ends. A range
starts at the release that was newest 12 months before the range was last set;
see the [stability policy](docs/stability.md#apps) for how it moves and the
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

## Other tools

Recyclarr, Configarr and Profilarr sync quality profiles and custom formats,
often from the TRaSH Guides, and Prowlarr syncs indexers into the apps. Each one
can run beside Arrchitect as long as every setting has one owner. Arrchitect
only touches what a resource declares, so leave out whatever another tool
manages. [Sharing apps with other tools](docs/other-tools.md) lists who writes
what.

## Documentation

- [Example stack](docs/example-stack.md): deploying the apps and wiring them together
- [User guide](docs/guide.md): how resources reconcile, connect apps and report status
- [Exporting an existing app](docs/export.md): generating resources from apps you already run
- [Rebuilding apps](docs/rebuilding.md): getting a rebuilt app configured again
- [Sharing apps with other tools](docs/other-tools.md): Recyclarr, Configarr, Profilarr and Prowlarr
- [API reference](docs/api.md): every kind and field
- [Stability and support](docs/stability.md): versioning, and the app and Kubernetes versions supported
- [Examples](examples/) and [one sample per resource](config/samples/)

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) and the
[code of conduct](CODE_OF_CONDUCT.md). Report security issues as described in
[SECURITY.md](SECURITY.md).

## License

[Apache 2.0](LICENSE)
