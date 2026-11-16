# User guide

- [Reconciling](#reconciling)
- [Indexers and download clients](#indexers-and-download-clients)
- [Status](#status)
- [Observe mode](#observe-mode)
- [Prune](#prune)
- [Deletion](#deletion)
- [Credentials](#credentials)
- [References by name](#references-by-name)
- [Metrics](#metrics)
- [Upgrading](#upgrading)
- [Verifying a release](#verifying-a-release)

## Reconciling

The operator reads each app's configuration through its API, compares it with
the resource, and writes what differs. Fields left out of the resource are not
managed: an item that already exists in the app keeps them, and a new item gets
the app's defaults. An empty list, such as `tagLabels: []`, is declared and
clears the item's tags. It reconciles every 5 minutes, when the resource changes, and when a
Secret the resource refers to changes. `spec.reconcile.interval` changes the
period, down to a minimum of 30s.

Download clients, indexers, notifications and other lists are matched by name.
Root folders are created if missing and otherwise left alone. Jellyfin and
Plex libraries are created if missing and otherwise kept as declared, folders
included: a folder the resource doesn't list is removed from the library.
Libraries are never deleted, changing a library's type means recreating it,
and a name two libraries share is refused rather than guessed.

On a fresh install, the operator runs the Jellyfin and Seerr setup wizards.
Seerr's needs Seerr 3.5.0 or later, and when set up through Jellyfin it waits
until Jellyfin has a library.

## Indexers and download clients

Sonarr's indexers and download clients are resources of their own,
`SonarrIndexer` and `SonarrDownloadClient`. Each one configures one item in the
Sonarr that `spec.sonarrRef` names, in the same namespace, and its other fields
are the item's:

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
  host: qbittorrent
  port: 8080
  category: tv
```

- The Sonarr's connection, `driftPolicy` and `interval` apply to it. Until the
  Sonarr exists, it reports `ReferenceNotFound`.
- `status.id` is the item's ID in Sonarr.
- An item that already exists is adopted: configured as declared, but never
  deleted.
- Renaming `spec.name` replaces the item the resource created.
- When two resources name the same item in the same Sonarr, the older one
  configures it and the other reports `NameConflict`.
- `spec.deletionPolicy` defaults to `delete`: deleting the resource deletes the
  item it created. `orphan` leaves it. If the Sonarr is deleted first, nothing
  can be removed, and an `InstanceMissing` warning event says so.

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
| `Observed` | Observe mode found no differences. |
| `DriftDetected` | Observe mode found differences. |

## Observe mode

`spec.reconcile.driftPolicy: observe` compares without writing to the app.
Differences set `Synced` to `False` with reason `DriftDetected`, are listed in
the condition's message (the first ten), and increment
`arrchitect_drift_observed_total`.

Some things can't be compared by reading:

- qBittorrent, Jellyfin and Seerr still sign in to read protected settings.
- An uninitialized Jellyfin or Seerr is reported as needing setup; the wizard
  doesn't run.
- An item that refers by name to something the same resource declares, such as
  a tag, a quality profile or an indexer, is reported as different while that
  thing doesn't exist yet. A name the resource doesn't declare and the app
  doesn't have fails, as it would when writing.
- A new Maintainerr reports rule groups as different until the media server,
  the Sonarr and Radarr servers and the notifications they use exist.
- Root folders are compared by existence.
- Passwords and API keys that an app masks count as different only if the
  operator wrote them and the Secret has changed since.
- Plex preferences that Plex doesn't list are not compared.

## Delay profiles and quality definitions

Delay profiles have no name. Each one in the resource is matched to the app's
profile that holds any of its tags, and the one without tags to the app's
default profile. A matching profile is updated, even one made in the app, and
changing a profile's tags updates it rather than replacing it. The app allows a
tag in only one profile, so a tag moving to another profile is taken off the
first one before it's added. The app checks tagged profiles in the order the
resource lists them.

Quality definitions are a fixed set. The operator updates the ones listed,
matched by `quality`, and leaves the rest alone. Sizes are in the app's units:
megabytes per minute of runtime in Sonarr and Radarr, kilobits per second in
Lidarr. A `maxSize` or `preferredSize` of 0 means unlimited. Readarr has no
preferred size.

## Prune

With `spec.reconcile.prune: true`, the operator deletes items it created once
they are removed from the resource. It tracks what it created in
`status.managedResources`; items it didn't create are never deleted. Without
prune, removing an item from the resource only stops managing it.

| App | Pruned |
|---|---|
| Sonarr | notifications, import lists, custom formats, quality profiles, remote path mappings, delay profiles |
| Radarr, Lidarr, Readarr | download clients, indexers, notifications, import lists, custom formats, quality profiles, remote path mappings, delay profiles |
| Prowlarr | applications, indexers, proxies, download clients, notifications |
| FlareSolverr | sessions |
| Tdarr | library variables |
| Autobrr | download clients, indexers, IRC networks, feeds, filters and their actions |
| Seerr | Sonarr and Radarr servers |
| SABnzbd | servers, categories |
| qBittorrent | categories |
| Bazarr | language profiles, matched by `profileId` |
| Maintainerr | rule groups, Sonarr and Radarr servers |

The API server rejects `prune: true` on Jellyfin and Plex.

- A list with no declared items is not pruned.
- Settings, root folders, tags, quality definitions, libraries and Prowlarr
  sync profiles are never pruned, and neither is the default delay profile.
- A pass that would delete more than 25 items of one kind deletes none of them.
- Observe mode never prunes. It reports each item prune would delete as a
  difference.
- Deleting a Maintainerr rule group also deletes its collection.
- Deleting a qBittorrent category leaves its torrents uncategorized.
- Delay profiles are pruned before other changes, so a new profile can take the
  tags of the one it replaces. `status.managedResources` records them by the
  app's profile ID.

## Deletion

Deleting a resource leaves the app unchanged. With
`spec.reconcile.deletionPolicy: delete`, the operator first deletes the items it
created, the same kinds prune covers. Everything else is kept. Only resources
with `delete` carry the operator's finalizer, so removing the operator never
leaves the others stuck deleting. Indexers and download clients default to
`delete`: delete them before uninstalling the operator, or remove the
finalizer from each one after.

`delete` is supported for the apps in the prune table, and not in observe mode;
the API server rejects it on Jellyfin and Plex and alongside `driftPolicy: observe`. An item the app won't delete
because something still uses it, such as a Lidarr or Readarr quality profile a
root folder defaults to, is kept and named in a `ResourceKept` warning event. If
cleanup keeps failing for another reason, the operator gives up after 10
minutes, records a warning event and removes the finalizer.

## Credentials

Credentials come from Secrets, referenced by name and key. Any kind of Secret
works, including Sealed and External Secrets. When a Secret the connection
needs is missing, `Ready` is `False` with reason `SecretNotFound` and nothing
is synced. When one an item needs is missing, that kind of item is neither
written nor pruned and is named in a `SyncFailed` message; the rest still
syncs.

Provider `fields`, and Autobrr indexer `settings`, read a value from a Secret
with `valueFrom` instead of `value`:

```yaml
fields:
  - name: apiKey
    valueFrom:
      secretKeyRef:
        name: indexer-credentials
        key: api-key
```

Bazarr providers put settings read from Secrets in `secretSettings`:

```yaml
providers:
  - name: opensubtitlescom
    settings:
      use_hash: "true"
    secretSettings:
      username: {name: opensubtitles, key: username}
      password: {name: opensubtitles, key: password}
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
- `activeProfileName` on Seerr's Sonarr and Radarr connections, looked up from
  that Sonarr or Radarr.

A name that matches nothing, or more than one item, fails the reconcile. See
[Rebuilding apps](rebuilding.md) for more.

## Access

The chart adds two ClusterRoles for people who manage these resources.
`<release>-view` grants read access and is part of Kubernetes' built-in `view`,
`edit` and `admin` roles; `<release>-edit` grants write access and is part of
`edit` and `admin`. A namespace's existing role bindings therefore cover the
resources of the apps the chart runs. Set `rbac.aggregateClusterRoles: false`
to leave them out.

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
that changed. Breaking changes are listed at the top of the release notes.

Versions before 0.34.2 also recorded pre-existing items in
`status.managedResources`. Review that list before enabling prune on a resource
created by one of those versions.

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
