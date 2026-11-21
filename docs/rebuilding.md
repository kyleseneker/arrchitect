# Rebuilding apps

Arrchitect configures apps that are already running. Deploy their containers,
storage, networking and credentials separately. A reconcile doesn't restore
media, watch history, requests, download queues or anything else an app keeps
in its database.

## References by name

Apps link items by ID, and a rebuilt app hands out new IDs. Resources refer to
those items by name instead, and the names are looked up on every reconcile.
[portable-references.yaml](../examples/portable-references.yaml) is a starting
point; replace its endpoints, paths and Secret references before applying it.

- Tags, by label with `tagLabels`. Tags declared in `spec.tags` are created
  before references are looked up, and a label that matches no tag blocks
  writes.
- Prowlarr sync profiles. `appProfiles` are created or updated before indexers,
  and `indexers[].appProfileName` looks up a profile's current ID, taking
  precedence over `appProfileId`. Sync profiles are never pruned or deleted,
  since indexers the operator doesn't manage may use them.
- Custom formats in quality profiles, looked up after formats are applied, and
  quality profiles in import lists, looked up after profiles are applied.
- Seerr's `activeProfileName`, looked up from the Sonarr or Radarr it connects
  to. When another tool, such as Recyclarr, owns those profiles, run it first.

A name that matches nothing, or more than one item, fails that stage and says
so in status; items already created stay recorded. A reconcile isn't a
transaction, so other stages may still have made changes. Observe mode creates
nothing, so it reports references to things that don't exist yet.

## Remote download paths

Sonarr, Radarr, Lidarr and Readarr take `remotePathMappings` with `host`,
`remotePath` and `localPath`. The host must match the download client's
configured hostname, and the local directory must already exist inside the
app's container.

A mapping is identified by `host` and `remotePath`: changing `localPath`
updates it in place, and changing either of the others makes a new one.
Directory paths get the trailing separator these apps store, Windows paths
included. To remove a mapping the operator created, use prune with at least
one mapping still declared; leaving the list out, or empty, leaves existing
mappings alone. Mappings made in the app are never deleted, and
`deletionPolicy: delete` removes the ones the operator created.

## Rehearsing a rebuild

Use disposable apps with empty configuration volumes and test media paths,
never a production database.

1. Note each app's image and version. Start the apps with empty configuration,
   then install the chart and create the Secrets the apps and the operator
   need.
2. Set up authentication. Either give each app a known API key through its
   deployment, or copy the key it generates into a Secret; see
   [Example stack](example-stack.md#other-apps) for which apps take one. The
   operator doesn't discover keys, and runs only the Jellyfin and Seerr setup
   wizards.
3. Apply the resources, with one owner per setting. For example, let Recyclarr
   own quality profiles, custom formats, quality definitions and naming, and
   Prowlarr the indexers it syncs, and leave those out of Arrchitect's
   resources; see [Sharing apps with other tools](other-tools.md).
4. Check each resource's status and read the settings back in each app.
   Associations should use the new IDs, and a second reconcile should write
   nothing.
5. Recreate the apps so they hand out different IDs, apply the same manifests
   again, and repeat the checks without editing any IDs.
6. Break one reference and stop one dependency. The failure should show in
   status, existing associations should stay, and the first reconcile after the
   fix should succeed.

CI checks the same on every change: `make test-compat` reconciles every app
against fresh containers of the versions pinned in
[test/compat/images_test.go](../test/compat/images_test.go), the oldest and
newest each app is supported at, reads the settings back and checks that a
second pass writes nothing.

## Limits

- Bazarr language profiles are identified by an explicit `profileId`, not
  looked up by name. After a rebuild, check the default profiles and existing
  media assignments.
- Seerr's first-time setup enables Jellyfin libraries only. With Plex it
  neither configures the Plex server nor enables its libraries, so finish that
  in Seerr.
- Users, history, media assignments and settings the resources don't cover
  need backups or separate provisioning.
