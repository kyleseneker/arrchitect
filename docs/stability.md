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

Each app is supported at its current release and the newest release of the line
before it, the versions in the README's table. CI reconciles a resource for
every app against both versions, each running in a container, on every change.
Other versions may work but aren't tested.

When an app releases a new version, Renovate opens a pull request that moves
the tests to it, and the table moves with it.

## Kubernetes

The three newest Kubernetes minor releases that the pinned kind release ships
node images for, currently 1.35, 1.36 and 1.37. CI installs the chart on each,
upgrades it from the last release, and reconciles against a real Sonarr.
