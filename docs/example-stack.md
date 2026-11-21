# Example stack

Arrchitect configures apps you deploy yourself. This page deploys Sonarr,
Prowlarr and qBittorrent with the
[bjw-s app-template](https://bjw-s-labs.github.io/helm-charts/docs/app-template/)
chart in a `media` namespace, then wires them together with Arrchitect
resources. Other charts work the same way: what matters is that each app gets
a known API key and a Service the operator can reach.

## API keys

Sonarr, Radarr, Lidarr, Readarr and Prowlarr read their API key from an
environment variable, so one Secret can feed both the app and the operator.
Generate each key the way the apps do, as 32 hex characters, in the Secrets
the [examples](../examples/secrets.yaml) use:

```bash
for app in sonarr prowlarr; do
  kubectl -n media create secret generic "$app-credentials" --from-literal=api-key="$(openssl rand -hex 16)"
done
```

The app reads the variable at startup, so restart it after changing the
Secret.

## Deploying the apps

`sonarr.yaml`, for app-template 5.3.0:

```yaml
controllers:
  sonarr:
    strategy: Recreate
    containers:
      app:
        image:
          repository: lscr.io/linuxserver/sonarr
          tag: 4.0.20.3014-ls328
        env:
          SONARR__SERVER__PORT: &port 8989
          SONARR__AUTH__APIKEY:
            valueFrom:
              secretKeyRef:
                name: sonarr-credentials
                key: api-key
        probes:
          liveness:
            enabled: true
            type: HTTP
            path: /ping
          readiness:
            enabled: true
            type: HTTP
            path: /ping
service:
  app:
    controller: sonarr
    ports:
      http:
        port: *port
persistence:
  config:
    type: persistentVolumeClaim
    accessMode: ReadWriteOnce
    size: 5Gi
    globalMounts:
      - path: /config
  data:
    existingClaim: data
    globalMounts:
      - path: /data
```

```bash
helm install sonarr oci://ghcr.io/bjw-s-labs/helm/app-template --version 5.3.0 \
  --namespace media --values sonarr.yaml
```

- With one Service, app-template names it after the release, so Sonarr is at
  `http://sonarr.media.svc.cluster.local:8989`.
- Prowlarr takes the same values with `prowlarr` in place of `sonarr`, port
  9696, the `PROWLARR__` prefix and no `data` volume. Radarr uses port 7878,
  Lidarr 8686 and Readarr 8787, each with its own prefix.
- Mount one shared volume at `/data` in every app that handles files, with
  downloads under `/data/torrents` and the library under `/data/media`. A path
  then means the same thing to qBittorrent and Sonarr, no remote path mapping
  is needed, and imports can hardlink rather than copy, which needs both on
  one mount.
- The API key is all the operator needs. Set the login for the web UI with the
  app's other variables; see Sonarr's
  [environment variables](https://wiki.servarr.com/sonarr/environment-variables).
- To run the linuxserver images as non-root or with a read-only root
  filesystem, see their [non-root](https://docs.linuxserver.io/misc/non-root/)
  and [read-only](https://docs.linuxserver.io/misc/read-only/) notes.

qBittorrent can't take its credentials from the environment. Deploy it with
the shared volume at `/data` and its web UI on port 8080, read the temporary
password it logs on first start, set your own in the web UI, and store it:

```bash
kubectl -n media create secret generic qbittorrent-credentials \
  --from-literal=username=admin --from-literal=password='<the password you set>'
```

## Wiring them up

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
kind: QBittorrent
metadata:
  name: qbittorrent
  namespace: media
spec:
  connection:
    url: http://qbittorrent.media.svc.cluster.local:8080
    usernameSecretRef:
      name: qbittorrent-credentials
      key: username
    passwordSecretRef:
      name: qbittorrent-credentials
      key: password
  categories:
    - name: tv-sonarr
      savePath: /data/torrents/tv
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
  category: tv-sonarr
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
      syncLevel: fullSync
```

- The download client and Prowlarr's application name the other app's resource
  with `appRef`, so its address and credentials are written once; see
  [Connecting apps](guide.md#connecting-apps).
- `/data/media/tv` must exist before Sonarr accepts it as a root folder.
- Each resource reports `AppUnreachable` until its app answers, then syncs.
- Indexers added to Prowlarr are synced to Sonarr by Prowlarr itself; see
  [Sharing apps with other tools](other-tools.md#prowlarr).
- [jellyfin-stack.yaml](../examples/jellyfin-stack.yaml) and
  [plex-stack.yaml](../examples/plex-stack.yaml) wire a fuller stack the same
  way, with Radarr, a media server and, for Plex, Seerr.

## Other apps

How each app's credentials get into a Secret:

| App | Known at deploy time | Otherwise |
| --- | --- | --- |
| Sonarr, Radarr, Lidarr, Readarr, Prowlarr | `<APP>__AUTH__APIKEY` | |
| Seerr | `API_KEY` | |
| Tdarr | [`seededApiKey`](https://docs.tdarr.io/docs/other/authentication), starting with `tapi_` | |
| Jellyfin | The admin user, which the operator creates when it runs the setup wizard | An API key, created in the dashboard |
| Bazarr | | The key under Settings, General, Security |
| SABnzbd | | The key under Config, General |
| Autobrr | | A key created under Settings, API keys |
| qBittorrent | | The password you set after first start |
| Plex | | The token Plex stores after `PLEX_CLAIM`; see [finding a token](https://support.plex.tv/articles/204059436-finding-an-authentication-token-x-plex-token/) |
| Maintainerr | No key needed | |
