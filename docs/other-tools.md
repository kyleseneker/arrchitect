# Sharing apps with other tools

Recyclarr, Configarr, Profilarr and Prowlarr also write configuration into
Sonarr, Radarr and the other apps. Each of them can run beside Arrchitect, as
long as every setting has one owner. Two tools that both manage a setting
rewrite each other's value on every pass.

## What Arrchitect touches

- Only what a resource declares. A field left out keeps whatever value it has,
  whoever set it.
- List items are matched by name. An item another tool created is adopted if a
  resource declares the same name, and from then on matches the resource.
- Prune and `deletionPolicy: delete` remove only items the operator created,
  recorded in `status.managedResources`. Another tool's items are never deleted.
- On an existing quality profile, only the format scores the resource lists are
  set, and other formats keep their scores. A profile the operator creates
  scores every unlisted format 0, since the app requires a score for each.

So leave out of the resource whatever another tool owns. `arrchitect export
--exclude` writes a resource without those sections; see
[Exporting an existing app](export.md).

## Who writes what

| | Recyclarr | Configarr | Profilarr | Prowlarr |
| --- | --- | --- | --- | --- |
| Custom formats | Yes | Yes | Yes | |
| Quality profiles | Yes | Yes | Yes | |
| Quality definitions | Yes | Yes | Yes | |
| Naming | Yes | Yes | Yes | |
| Media management | Propers and repacks | Yes | Yes | |
| Delay profiles | | Yes | The default one | |
| Root folders, remote path mappings, release profiles | | Yes | | |
| Download clients, import lists, notifications, tags | | Yes | | |
| Indexers | | Edits existing ones | | Yes |
| Apps | Sonarr, Radarr | Sonarr, Radarr; others experimental | Sonarr, Radarr | Sonarr, Radarr, Lidarr, Readarr |

## Recyclarr

[Recyclarr](https://recyclarr.dev/guide/sync-behavior/) syncs custom formats,
quality profiles, quality definition sizes and naming from the TRaSH guides,
plus the propers and repacks setting under media management.

- Leave `customFormats`, `qualityProfiles` and `qualityDefinitions` out of the
  resource, along with the `naming` fields and the
  `mediaManagement.downloadPropersAndRepacks` it sets. Other settings in those
  sections can stay.
- Since 8.4.0, Recyclarr takes over an existing custom format or quality profile
  with the same name as one in its config.
- `delete_old_custom_formats` deletes only custom formats Recyclarr has synced,
  including ones it took over.
- `reset_unmatched_scores` sets to 0 the score of every format a profile's
  config doesn't list. Keep the scores of a profile in one tool.
- Items that name a Recyclarr profile, such as an import list's
  `qualityProfileName` or a Seerr server's `activeProfileName`, fail until the
  profile exists and then sync on the next pass. On a rebuild, run Recyclarr
  first.

## Configarr

[Configarr](https://configarr.de/docs/configuration/config-file/) syncs the same
things as Recyclarr and can also manage most other settings, matching items by
name without tracking what it created.

- Some of its keys describe the whole list once they're present: `root_folders`,
  `delay_profiles`, `release_profiles` and `remote_paths`. Items they don't
  list are deleted, including ones Arrchitect created. Give each of those kinds
  wholly to one tool.
- Its `delete_unmanaged_*` options, and `delete_unmanaged` on download clients,
  import lists and notifications, delete every item of that kind Configarr
  doesn't manage, whoever created it. Leave them off for any kind Arrchitect
  manages items of.
- Its Prowlarr support, still experimental, overlaps the `Prowlarr` resource.
  Pick one.

## Profilarr

[Profilarr](https://github.com/Dictionarry-Hub/profilarr) syncs custom formats,
quality profiles, naming, media management and quality definitions, and
overwrites the default delay profile.

- Leave those out of the resource, along with the delay profile without tags,
  which is the app's default.
- In every profile it syncs, it sets to 0 the score of each format it doesn't
  manage.
- Its Arr Cleanup job, off by default, deletes every custom format and quality
  profile it doesn't have selected, including Arrchitect's.

## Prowlarr

A Prowlarr application adds Prowlarr's indexers to the app, named
`{indexer} (Prowlarr)`, and tracks them by ID in its own database. Declare them
once, in the `Prowlarr` resource's `indexers` and `applications`, and let
Prowlarr sync them.

- Don't declare a synced indexer as a `SonarrIndexer` or similar. The operator
  would adopt it by name, and with `syncLevel: fullSync` the two would rewrite
  its settings on every pass.
- Prowlarr also takes over any Newznab or Torznab indexer in the app whose URL
  starts with Prowlarr's URL or whose API key is Prowlarr's. Point indexer
  resources at the indexer itself, not at Prowlarr.
- Synced indexers are never pruned, since the operator didn't create them.
- `syncLevel: addOnly` adds and removes indexers but never updates them, so an
  edit made in the app stays. `fullSync` also overwrites the
  [settings Prowlarr compares](https://wiki.servarr.com/prowlarr/faq#what-arr-indexer-settings-are-compared-for-app-full-sync).
- Prowlarr doesn't sync download clients, so `SonarrDownloadClient` and the
  others don't overlap with it.

## Finding overlaps

When two tools manage the same setting, `arrchitect_drift_corrected_total`
keeps rising for that resource, since every pass corrects the other tool's
write. To see what overlaps, set `spec.reconcile.driftPolicy: observe` after
the other tool has run: `Synced` turns `False` with reason `DriftDetected`, and
its message lists what differs. Remove those settings from one of the two.
