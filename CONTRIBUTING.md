# Contributing to Arrchitect

## Getting started

1. Fork and clone: `git clone https://github.com/<your-username>/arrchitect`
2. Branch: `git checkout -b my-feature`
3. Make your changes
4. `make test lint verify-crds`
5. Push and open a PR

You'll need the Go version in `go.mod`, kubebuilder, a cluster to point
`KUBECONFIG` at, and access to whichever app you're working on. Most of the interesting behavior
only shows up against a real Sonarr or Jellyfin, so it helps to have one
running.

```bash
make run                      # run the operator against the cluster in KUBECONFIG, which must be set
make build                    # the operator and the CLI
make docker-build             # the operator's image for this machine; IMG= to tag, ARCH= to cross-build
make test
make manifests generate       # after editing anything in api/
make sync-crds                # copy generated CRDs into the chart
make lint
```

## Tests

| Command | What it runs |
| --- | --- |
| `make test` | Unit tests |
| `make test-envtest` | Every sample and admission rule against a real kube-apiserver, and the controllers' Kubernetes side |
| `make verify-crd-compat` | crdify's report of CRD changes since the last release that existing resources could break on |
| `make test-compat` | Each app in a container, reconciled from its fixture. `COMPAT_VERSIONS=previous` runs the previous versions; `CONTAINER_TOOL=podman` uses podman |
| `make test-e2e` | The chart installed into a throwaway kind cluster, upgraded from the last release, against a real Sonarr. Needs docker |

The envtest and compat suites start their own API server, and the e2e
suite writes its cluster to a kubeconfig in a temporary directory: none of them
reads or changes your default kubeconfig.

`test/compat/images_test.go` pins each app's current version, which Renovate
bumps, and the newest release of the line before it. When a current pin moves
to a new line, move its previous pin too.

## Layout

One binary, `cmd/operator`, runs a controller per app, and one chart,
`chart/arrchitect`, installs it with the CRDs of the apps it runs. The apps
are grouped by what they do, each group with an API package and a controller
package named the same way.

| Group | Apps | API package |
|-------|------|-------------|
| `pvr` | Sonarr, Radarr, Lidarr, Readarr | `api/pvr/v1alpha1/` |
| `indexers` | Prowlarr, FlareSolverr | `api/indexers/v1alpha1/` |
| `subtitles` | Bazarr | `api/subtitles/v1alpha1/` |
| `downloads` | qBittorrent, SABnzbd | `api/downloads/v1alpha1/` |
| `mediaservers` | Jellyfin, Plex | `api/mediaservers/v1alpha1/` |
| `requests` | Seerr/Jellyseerr | `api/requests/v1alpha1/` |
| `curation` | Maintainerr | `api/curation/v1alpha1/` |
| `transcode` | Tdarr | `api/transcode/v1alpha1/` |
| `automation` | Autobrr | `api/automation/v1alpha1/` |

A new app also needs its controller in `cmd/operator/main.go` and its name in
the chart's `apps` and `arrchitect.enabledApps`.

## Dependency updates

Renovate (`renovate.json`) opens PRs on Saturday mornings; none automerge. The
Kubernetes libraries and controller-runtime arrive as one PR. A controller-tools
PR needs `make manifests generate sync-crds` run and committed on its branch
before `verify-crds` passes.

## Commit messages and releases

Commits use [Conventional Commits](https://www.conventionalcommits.org/) with
the app as the scope: `fix(tdarr): ...`, `feat(prowlarr): ...`. Release notes
are generated from them, so `feat` and `fix` subjects should read as a change a
user would notice. `chore(deps)` commits are listed as dependencies. `test`,
`docs`, `refactor`, `style`, `ci` and other `chore` commits are left out. Mark
anything that needs action on upgrade with `!` (`feat(maintainerr)!: ...`) and
it is listed under Breaking changes.

A release is a tag on `main`:

```bash
git tag v0.2.0
git push origin v0.2.0
```

The release workflow publishes the image, CLI and chart, and drafts the notes
from the commits, which are then edited by hand. The draft lists `kubectl
apply` commands for any CRD that changed since the previous tag, for installs
that set `crds.enabled: false`.
