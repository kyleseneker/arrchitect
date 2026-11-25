# User guide

- [Reconciling](#reconciling)
- [Indexers and download clients](#indexers-and-download-clients)
- [Connecting apps](#connecting-apps)
- [Status](#status)
- [Observe mode](#observe-mode)
- [Delay profiles, quality definitions and metadata consumers](#delay-profiles-quality-definitions-and-metadata-consumers)
- [Prune](#prune)
- [Deletion](#deletion)
- [Credentials](#credentials)
- [References by name](#references-by-name)
- [Access](#access)
- [Metrics](#metrics)
- [Upgrading](#upgrading)
- [Verifying a release](#verifying-a-release)

## Reconciling

The operator reads each app's configuration through its API, compares it with
the resource, and writes what differs. Fields left out of the resource are not
managed. An item that already exists in the app keeps them, and a new item gets
the app's defaults. An empty list, such as `tagLabels: []`, is declared and
clears the item's tags. It reconciles every 5 minutes, when the resource
changes, and when a Secret the resource refers to changes.
`spec.reconcile.intervalSeconds` changes the period, down to a minimum of 30.

Download clients, indexers, notifications and other lists are matched by name.
Root folders are matched by path and created if missing. Lidarr and Readarr
root folders also have a name, default profiles and other settings, and the
operator keeps those as declared. Sonarr and Radarr root folders have only a
path.

Lidarr and Readarr have no import list sync, so only Sonarr and Radarr take
`importListConfig`. Readarr's media management has no `enableMediaInfo`,
`useScriptImport` or `scriptImportPath`.

Jellyfin and Plex libraries are matched by name and created if missing. An
existing library is kept as declared, including its folders. A folder the
resource doesn't list is removed from the library. Libraries are never
deleted. The operator doesn't change a library's type. To change it, recreate
the library. When two libraries share a name, the operator reports an error
and changes neither.

On a fresh install, the operator runs the Jellyfin and Seerr setup wizards.
Seerr's needs Seerr 3.5.0 or later, and when set up through Jellyfin it waits
until Jellyfin has a library. Seerr's setup enables every library of its media
server, unless `spec.libraries` names the ones to use. Those are then enabled,
and the rest disabled, on every pass; a name the media server doesn't have
fails the reconcile.

## Indexers and download clients

Sonarr, Radarr, Lidarr and Readarr indexers and download clients are resources
of their own, such as `SonarrIndexer` and `SonarrDownloadClient`. Each one
configures one item in one app. Its reference, such as `spec.sonarrRef`, names
that app's resource in the same namespace. Its other fields are the item's
settings:

```yaml
apiVersion: arrchitect.dev/v1alpha1
kind: SonarrDownloadClient
metadata:
  name: sonarr-qbittorrent
  namespace: media
spec:
  sonarrRef:
    name: sonarr
  name: qBittorrent
  protocol: torrent
  implementation: QBittorrent
  host: qbittorrent.media.svc.cluster.local
  port: 8080
  usernameSecretRef:
    name: qbittorrent-credentials
    key: username
  passwordSecretRef:
    name: qbittorrent-credentials
    key: password
  category: tv
```

- The app's connection, `driftPolicy` and `intervalSeconds` apply to it.
  Until the app's resource exists, it reports `ReferenceNotFound`.
- `status.id` is the item's ID in the app.
- An item that already exists is adopted. The operator configures it as
  declared and never deletes it.
- Renaming `spec.name` replaces the item the resource created.
- When two resources name the same item in the same app, the older one
  configures it and the other reports `NameConflict`.
- `spec.deletionPolicy` defaults to `delete`. Deleting the resource then
  deletes the item it created. `orphan` leaves it. If the app's resource is
  deleted first, nothing can be removed, and an `InstanceMissing` warning event
  says so.

## Connecting apps

Where one app connects to another, the connection can name that app's resource
with `appRef` instead of repeating its address and credentials. On each
reconcile the operator reads the resource and uses its `spec.connection`:

```yaml
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

| Connection | `appRef` kinds | In place of |
| --- | --- | --- |
| Prowlarr `applications` | `Sonarr`, `Radarr`, `Lidarr`, `Readarr` | `implementation`, `baseUrl`, `apiKeySecretRef`; `prowlarrUrl` defaults to the Prowlarr resource's URL |
| Seerr `sonarr`, `radarr` | `Sonarr` in `sonarr`, `Radarr` in `radarr` | `hostname`, `port`, `useSsl`, `baseUrl`, `apiKeySecretRef` |
| Seerr `jellyfinAuth` | `Jellyfin` | the address and the admin user |
| Seerr `plexAuth` | `Plex` | the address and the token |
| Bazarr `sonarrConnection`, `radarrConnection` | `Sonarr`, `Radarr` | `host`, `port`, `ssl`, `basePath`, `apiKeySecretRef` |
| Bazarr `jellyfin` | `Jellyfin` | `url`, `apiKeySecretRef`; the Jellyfin resource must set `connection.apiKeySecretRef` |
| Maintainerr `sonarr`, `radarr` | `Sonarr` in `sonarr`, `Radarr` in `radarr` | `url`, `apiKeySecretRef` |
| Maintainerr `seerr` | `Seerr` | `url`, `apiKeySecretRef`; the Seerr resource must set `connection.apiKeySecretRef` |
| Maintainerr `mediaServer.plex` | `Plex` | `hostname`, `port`, `ssl`, `tokenSecretRef` |
| Maintainerr `mediaServer.jellyfin` | `Jellyfin` | `url`, `apiKeySecretRef`; the Jellyfin resource must set `connection.apiKeySecretRef` |
| Sonarr, Radarr, Lidarr and Readarr download clients | `QBittorrent`, `Sabnzbd` | `implementation`, `protocol`, `host`, `port`, `useSsl`, `urlBase` and the credentials |
| Autobrr `downloadClients` | `QBittorrent`, `Sabnzbd`, `Sonarr`, `Radarr`, `Lidarr`, `Readarr` | `type`, `host`, `port`, `tls` and the credentials |
| Sonarr, Radarr and Lidarr `notifications` | `Jellyfin`, `Plex` | `implementation`, `configContract` and the `host`, `port`, `useSsl`, `urlBase` and `apiKey` or `authToken` fields; the Jellyfin resource must set `connection.apiKeySecretRef` |
| Readarr `notifications` | `Plex` | the same; Readarr has no Jellyfin notification |

- A connection sets `appRef` or the fields it stands for, not both, and the API
  server rejects a mix. A notification's `fields` beside its `appRef` are the
  exception. The operator reports those as a sync error instead. To have an
  app reach another at a different address from the operator's, set the
  fields.
- The resource must be in the same namespace. The operator only reads it, and
  copies nothing from it into the resource that refers to it.
- The referenced app must be turned on, as `apps.<app>` in the chart, since the
  operator reads and watches only the kinds of the apps it runs. A reference to
  an app that's off reports `SyncFailed`, saying the operator doesn't run that
  app's controller.
- Until the resource exists, that connection is neither written nor pruned and
  the `SyncFailed` message names the reference; the rest still syncs. A
  download client resource, and a Seerr that signs in through the media server
  it refers to, report `ReferenceNotFound` instead and write nothing.
- A change to the resource, or to a Secret it names, reconciles the resources
  that refer to it straight away.
- Once Jellyfin is set up, a Jellyfin resource with
  `connection.apiKeySecretRef` uses that key instead of signing in as
  `adminUser`. Signing in adds a Jellyfin session on every reconcile.
- `arrchitect export` writes the address and credentials. It can't tell which
  resource, if any, describes the app at a URL.

## Status

Each resource has a `Ready` condition, for whether the app and its credentials
are reachable, and a `Synced` condition, for whether the app matches. `kubectl
get` shows both, and `kubectl get arrchitect` lists every resource across
apps. The `Synced` reason is one of:

| Reason | Meaning |
| --- | --- |
| `Synced` | The app matches the resource. |
| `SyncFailed` | Some settings could not be applied; the message says which, and the reason the app gave. |
| `AppUnreachable` | The app did not respond, or rejected the credentials. |
| `SecretNotFound` | A Secret or key the connection needs does not exist. |
| `InvalidConfig` | The resource asks for something the operator does not support. |
| `ReferenceNotFound` | An indexer's or download client's app resource doesn't exist, or an `appRef` the resource needs names a missing resource. |
| `NameConflict` | An older resource already configures the item this one names. |
| `Observed` | Observe mode found no differences. |
| `DriftDetected` | Observe mode found differences. |

## Observe mode

`spec.reconcile.driftPolicy: observe` compares without writing to the app.
Differences set `Synced` to `False` with reason `DriftDetected`, are listed in
the condition's message (the first ten), and increment
`arrchitect_drift_observed_total`.

Some things can't be compared by reading:

- qBittorrent still signs in to read protected settings, and so do Jellyfin
  and Seerr without `connection.apiKeySecretRef`.
- An uninitialized Jellyfin or Seerr is reported as needing setup; the wizard
  doesn't run.
- An item that refers by name to something the same resource declares, such as
  a tag, a quality profile or an indexer, is reported as different while that
  thing doesn't exist yet. A name the resource doesn't declare and the app
  doesn't have fails, as it would when writing.
- A new Maintainerr reports rule groups as different until the media server,
  the Sonarr and Radarr servers and the notifications they use exist.
- Sonarr and Radarr root folders are compared by existence.
- Passwords and API keys that an app masks count as different only if the
  operator wrote them and the Secret has changed since.
- Plex preferences that Plex doesn't list are not compared.

## Delay profiles, quality definitions and metadata consumers

Delay profiles have no name. Each one in the resource is matched to the app's
profile that holds any of its tags, and the one without tags to the app's
default profile. A matching profile is updated, even one made in the app, and
changing a profile's tags updates it rather than replacing it. The app allows a
tag in only one profile, so a tag moving to another profile is taken off the
first one before it's added. The app checks tagged profiles in the order the
resource lists them.

Quality definitions are a fixed set. The operator updates the ones listed,
matched by `quality`, and leaves the rest alone. Sizes are in the app's units.
Sonarr and Radarr use megabytes per minute of runtime, and Lidarr uses
kilobits per second. A `maxSize` or `preferredSize` of 0 means unlimited.
Readarr has no preferred size.

Metadata consumers are a fixed set too. Sonarr, Radarr and Lidarr keep one of
each type, off until turned on. The operator updates the ones listed, matched
by `implementation`, and leaves the rest alone. Readarr has none.

## Prune

With `spec.reconcile.prune: true`, the operator deletes items it created once
they are removed from the resource. It tracks what it created in
`status.managedResources`; items it didn't create are never deleted. Without
prune, removing an item from the resource only stops managing it.

| App | Pruned |
|---|---|
| Sonarr, Radarr, Lidarr, Readarr | notifications, import lists, custom formats, quality profiles, remote path mappings, delay profiles; release profiles in Sonarr and Radarr; auto tags in Sonarr, Radarr and Lidarr; metadata profiles in Lidarr and Readarr |
| Prowlarr | applications, indexers, proxies, download clients, notifications |
| FlareSolverr | sessions |
| Tdarr | library variables |
| Autobrr | download clients, indexers, IRC networks, feeds, filters and their actions, notifications, lists, proxies |
| Seerr | Sonarr and Radarr servers |
| SABnzbd | servers, categories |
| qBittorrent | categories, RSS feeds and rules |
| Bazarr | language profiles, matched by `profileId` |
| Maintainerr | rule groups, Sonarr and Radarr servers |

The API server rejects `prune: true` on Jellyfin and Plex.

- A list with no declared items is not pruned.
- Settings, root folders, tags, quality definitions, metadata consumers,
  libraries and Prowlarr sync profiles are never pruned, and neither is the
  default delay profile.
- A pass that would delete more than 25 items of one kind deletes none of them.
- Observe mode never prunes. It reports each item prune would delete as a
  difference.
- Deleting a Maintainerr rule group also deletes its collection.
- Deleting a qBittorrent category leaves its torrents uncategorized.
- Delay profiles are pruned before other changes, so a new profile can take the
  tags of the one it replaces. `status.managedResources` records them by the
  app's profile ID.

## Deletion

Deleting an app's resource leaves the app unchanged unless
`spec.reconcile.deletionPolicy` is `delete`. Then the operator first deletes
the items it created, the same kinds prune covers, and keeps everything else.
Indexers and download clients default to `delete`; see
[Indexers and download clients](#indexers-and-download-clients).

Only resources set to `delete` carry the operator's finalizer. Removing the
operator never leaves the other resources stuck. Delete the ones with the
finalizer before uninstalling it.

`delete` is supported for the apps in the prune table. The API server rejects
it on Jellyfin and Plex, and alongside `driftPolicy: observe`. An item the app
won't delete because something still uses it is kept and named in a
`ResourceKept` warning event. A Lidarr or Readarr quality profile that a root
folder defaults to is one example. If cleanup keeps failing for another
reason, the operator gives up after 10 minutes. It then records a warning
event and removes the finalizer.

## Credentials

Credentials come from Secrets, referenced by name and key. Any kind of Secret
works, including Sealed and External Secrets. When a Secret the connection
needs is missing, `Ready` is `False` with reason `SecretNotFound` and nothing
is synced. When one an item needs is missing, that kind of item is neither
written nor pruned and is named in a `SyncFailed` message; the rest still
syncs. An indexer or download client reports `SecretNotFound` and writes
nothing.

Provider `fields`, Autobrr indexer and Bazarr provider `settings`, and Seerr
and Maintainerr notification `options` read a value from a Secret with
`valueFrom` instead of `value`:

```yaml
fields:
  - name: apiKey
    valueFrom:
      secretKeyRef:
        name: usenet-indexer-credentials
        key: api-key
```

For a private certificate authority, set `connection.tls.caSecretRef`.
`connection.tls.insecureSkipVerify` disables verification.

## References by name

The apps link resources by ID, and IDs change when an app is rebuilt. These
references can use names instead:

- `tagLabels` on Sonarr, Radarr, Lidarr and Readarr download clients, indexers,
  notifications, import lists and delay profiles, and on Prowlarr applications,
  indexers, proxies, download clients and notifications. Tags in `spec.tags`
  are created first.
- Custom formats in quality profiles, and quality profiles in import lists.
- `appProfileName` on Prowlarr indexers.
- `downloadClientName` on Sonarr, Radarr, Lidarr and Readarr indexers, which
  sends the indexer's releases to that download client.
- `indexerName` on Sonarr and Radarr release profiles, which limits the profile
  to that indexer.
- `importListConfig.listSyncTagLabel` in Sonarr.
- In Sonarr, Radarr and Lidarr auto tags: `tagLabels`, and
  `qualityProfileName`, `metadataProfileName` (Lidarr) and `tagLabel` on
  conditions, in place of the condition's `value` ID.
- `activeProfileName` on Seerr's Sonarr and Radarr connections, looked up from
  that Sonarr or Radarr.
- In Autobrr: `indexers` on filters, `downloadClientName` on actions,
  `indexerName` on feeds, `downloadClientName` and `filters` on lists, and
  `proxyName` on indexers and IRC networks.

A name that matches nothing, or more than one item, sets `Synced` to `False`
with reason `SyncFailed`, and the message names the reference. What uses the
name isn't written. A reconcile isn't a transaction, so other changes in the
same pass may still be made. See [Rebuilding apps](rebuilding.md) for more.

## Access

The operator reads its resources and the Secrets they name, and writes only
their status and finalizers; it never creates or deletes them. The chart adds
two ClusterRoles for people who manage these resources.
`<release>-view` grants read access and is part of Kubernetes' built-in `view`,
`edit` and `admin` roles; `<release>-edit` grants write access and is part of
`edit` and `admin`. A namespace's existing role bindings therefore cover the
resources of the apps the chart runs. Set `rbac.aggregateClusterRoles: false`
to leave them out.

The namespace is the trust boundary. Anyone who can create a resource in a
namespace can have the operator use the credentials of any other resource
there through an `appRef`, as naming that resource's Secret already allows.

## Metrics

Metrics are served on `:8443/metrics` over HTTPS and require a token with access
to them. Set `metrics.serviceMonitor.enabled` in the chart to create a
ServiceMonitor.

| Metric | Description |
| --- | --- |
| `arrchitect_config_synced` | 1 if the resource last reconciled successfully, otherwise 0 |
| `arrchitect_drift_corrected_total` | Items rewritten because the app differed |
| `arrchitect_drift_observed_total` | Differences found in observe mode |
| `arrchitect_resources_pruned_total` | Items deleted by prune |
| `arrchitect_managed_resources` | Items declared per resource type |
| `arrchitect_app_api_request_duration_seconds` | Request latency to each app |
| `arrchitect_app_api_errors_total` | Failed requests to each app |

`arrchitect_config_synced == 0` is the condition to alert on.

## Upgrading

The chart installs and upgrades the CRDs of the apps it runs, and uninstalling
it leaves them in place along with your resources. With `crds.enabled: false`,
apply them yourself from the release assets; the release notes list the ones
that changed. Breaking changes are listed at the top of the release notes; see
[Stability and support](stability.md) for what a release can change.

A CRD that a release removes stays installed, along with any resources of that
kind, and the operator no longer reconciles them. Delete it once you've moved
them to their replacement.

## Verifying a release

The image, the chart and the checksums file for the release archives are signed
with [cosign](https://docs.sigstore.dev/) by the release workflow. With
`VERSION` set to the release, without the `v`:

```bash
cosign verify ghcr.io/kyleseneker/arrchitect:$VERSION \
  --certificate-identity https://github.com/kyleseneker/arrchitect/.github/workflows/release.yml@refs/tags/v$VERSION \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

The chart verifies the same way, as `ghcr.io/kyleseneker/charts/arrchitect:$VERSION`.
The image includes an SBOM:

```bash
docker buildx imagetools inspect ghcr.io/kyleseneker/arrchitect:$VERSION --format '{{ json .SBOM }}'
```

Download the checksums file with its `.sigstore.json` bundle, verify it, then
check the archive:

```bash
cosign verify-blob arrchitect_${VERSION}_checksums.txt \
  --bundle arrchitect_${VERSION}_checksums.txt.sigstore.json \
  --certificate-identity https://github.com/kyleseneker/arrchitect/.github/workflows/release.yml@refs/tags/v$VERSION \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
sha256sum --ignore-missing -c arrchitect_${VERSION}_checksums.txt
```
