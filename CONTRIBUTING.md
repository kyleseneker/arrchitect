# Contributing to Arrchitect

## Getting started

1. Fork and clone: `git clone https://github.com/<your-username>/arrchitect`
2. Branch: `git checkout -b my-feature`
3. Make your changes
4. `make lint test test-chart verify-crds verify-api-docs`
5. Push and open a PR

You'll need the Go version in `go.mod` and helm; the Makefile installs every
other tool into `bin/`. The app and end-to-end suites also need docker or
podman. Most of the interesting behavior only shows up against a real Sonarr or
Jellyfin, so it helps to have one running.

```bash
make help                     # every target
make manifests generate       # after editing anything in api/
make sync-crds                # copy the generated CRDs into the chart
make api-docs                 # regenerate docs/api.md from the API types
make run                      # run the operator against the cluster in KUBECONFIG, which must be set
make build                    # the operator and the CLI
make docker-build             # the operator's image; IMG= to tag, ARCH= to cross-build
```

## Tests

Each layer catches what the ones before it can't. CI runs each target as a job
of the same name.

| Target | What it covers | Needs |
| --- | --- | --- |
| `make test` | Unit tests against fakes, with the race detector; envtest tests against a real kube-apiserver for every sample, admission rule and controller; and `test/consistency`, which checks the chart, Renovate and CI agree with the code | Go, helm |
| `make test-chart` | The chart's templates, with the helm-unittest suites in `chart/arrchitect/tests`. CI also runs `ct lint` | helm |
| `make verify-crds` | That the chart carries the generated CRDs | |
| `make verify-api-docs` | That `docs/api.md` matches the API types | |
| `make verify-crd-compat` | crdify's report of CRD changes since the last release that existing resources could break on. In CI it shows as a warning, not a failure, while the API is v1alpha1 | git history |
| `make test-compat` | Each app running in a container, reconciled from its fixture in `test/compat/testdata`. `COMPAT_VERSIONS=oldest` runs each app's oldest supported version, `previous-major` the newest of the major before its newest | docker or podman |
| `make test-e2e` | The image and chart in a kind cluster, with the Chainsaw tests in `test/e2e`: install and upgrade from the last release, every controller, and a single-namespace install. `KIND_NODE_IMAGE=` picks the Kubernetes version; CI runs each one [the stability policy](docs/stability.md#kubernetes) supports | docker |

`config/samples` holds one readable example per kind for users; envtest checks
the API server accepts each one, and the end-to-end suite runs them. The compat
fixtures in `test/compat/testdata` are different: they set as many fields as the
app accepts and point at the test's containers.

None of the suites reads or changes your default kubeconfig: envtest and compat
start their own API server, and the end-to-end suite writes its kind cluster to
a kubeconfig in a temporary directory.

`test/compat/images_test.go` pins each app's current version, which Renovate
bumps, and the newest release of the line before it. When a current pin moves
to a new line, move its previous pin too.

## Layout

`cmd/operator` runs the controllers for every app, `cmd/cli` is the
`arrchitect export` command, and `chart/arrchitect` installs the operator with
the CRDs of the apps it runs. The apps
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
before `verify-crds` passes, and a crd-ref-docs PR needs `make api-docs`.

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
