# Rebuilding apps

Arrchitect configures apps that are already running. Deploy their containers,
storage, networking and credentials separately. A reconcile doesn't restore
media, watch history, requests, download queues or anything else an app keeps
in its database.

## References by name

Apps link items by ID, and a rebuilt app hands out new IDs. Resources can
refer to those items by name instead, and the names are looked up on every
reconcile. [References by name](guide.md#references-by-name) lists each one.
[portable-references.yaml](../examples/portable-references.yaml) is a starting
point; replace its endpoints, paths and Secret references before applying it.

Declared items are created before references to them are looked up. Tags in
`spec.tags` come first. Prowlarr sync profiles come before indexers, custom
formats before quality profiles, and quality profiles before import lists. A
Prowlarr indexer's `appProfileName` takes precedence over its `appProfileId`.
Sync profiles are never pruned or deleted, since indexers the operator doesn't
manage may use them. Seerr's `activeProfileName` is looked up from the Sonarr
or Radarr it connects to. When another tool, such as Recyclarr, owns those
profiles, run it first.

When a name matches nothing, or more than one item, the items already created
stay recorded in `status.managedResources`. Observe mode creates nothing, so
it reports references to things that don't exist yet.

## Remote download paths

Sonarr, Radarr, Lidarr and Readarr take `remotePathMappings` with `host`,
`remotePath` and `localPath`. The host must match the download client's
configured hostname, and the local directory must already exist inside the
app's container.

A mapping is identified by `host` and `remotePath`. Changing `localPath`
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

CI covers part of this on every change. `make test-compat` starts every app
with empty configuration and a known API key, at the oldest and newest
versions pinned in [test/compat/images_test.go](../test/compat/images_test.go).
It reconciles each one, reads the settings back and checks that a second pass
writes nothing. It doesn't recreate the apps or break references, so steps 5
and 6 aren't covered.

## Limits

- Bazarr language profiles are identified by an explicit `profileId`, not
  looked up by name. After a rebuild, check the default profiles and existing
  media assignments.
- Users, history, media assignments and settings the resources don't cover
  need backups or separate provisioning.
