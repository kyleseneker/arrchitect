# Stability and support

What each part of Arrchitect promises, and what a release can change.

## Versions

Releases follow [semantic versioning](https://semver.org). The operator image,
the CLI and the chart share one version.

Arrchitect is before 1.0, so a minor release (0.x.0) can carry breaking
changes; a patch release (0.x.y) never does. Each breaking change is at the top
of the release notes with the steps to move to it. That covers the API, the
chart's values, the CLI's flags, metric names and status condition reasons.

Only the latest release gets fixes. See [SECURITY.md](../SECURITY.md).

## API

The API is `arrchitect.dev/v1alpha1`. At alpha, any kind, field, default or
validation rule can change in a minor release.

It moves to `v1beta1` once the resource model has gone two minor releases
without a breaking change and runs in real installations:

- `v1beta1` starts from the `v1alpha1` schema as it stands, so the API server
  serves both without a conversion webhook.
- `v1alpha1` stays served for at least two minor releases after that, and the
  release notes say when it goes.
- From `v1beta1` on, the
  [Kubernetes deprecation policy](https://kubernetes.io/docs/reference/using-api/deprecation-policy/)
  for beta APIs applies: a field or kind is deprecated before it's removed, and
  stays for at least 9 months or three minor releases, whichever is longer.

The [API reference](api.md) lists every kind and field.

## Apps

An app is supported at every stable release that was its newest at some time
in the 12 months before its range was last set, through its newest release.
The README's [supported apps](../README.md#supported-apps) table lists each
range. Apps are harder to upgrade than Arrchitect, so Arrchitect supports a
range of versions of each app.

Some ranges start later, at the oldest release Arrchitect works with, and
grow to 12 months as newer releases age:

- Seerr starts at 3.5.0, the first release whose setup the operator can run.
- Maintainerr starts at 3.30.0, which added the Sonarr and Radarr tag
  exclusions the operator manages.
- Tdarr starts at 3.00.00, the first release with the API keys the operator
  signs in with.

Readarr is archived upstream, and its only releases were development builds.
It is supported at its final ones.

How a range moves:

- A new release joins once CI passes against it. Renovate opens a pull request
  per app that moves its newest version.
- The oldest supported releases move up only in a minor release, which says so
  in its notes.
- Pre-releases and development builds, such as Sonarr's v5 builds, aren't
  supported.

CI reconciles a resource for every app at both ends of its range, each app
running in a container, on every change. While an app's newest release starts
a major version, CI also tests the newest release of the major before it.
Releases in between aren't tested one by one; apps rarely change their APIs
within a range, and a fix for one is a patch release.

A setting an older supported release doesn't have, where the operator knows of
it, is left out and named in the resource's status with the release that added
it. Jellyfin's `encoding.subtitleExtractionTimeoutMinutes`, added in 12.0, is
one.

## Kubernetes

The three newest Kubernetes minor releases that the pinned kind release ships
node images for, currently 1.35, 1.36 and 1.37. CI installs the chart on each,
upgrades it from the last release, and reconciles against a real Sonarr.
