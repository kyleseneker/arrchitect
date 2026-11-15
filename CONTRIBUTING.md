# Contributing to media-operator

## Getting started

1. Fork and clone: `git clone https://github.com/<your-username>/media-operator`
2. Branch: `git checkout -b my-feature`
3. Make your changes
4. `make test lint verify-crds`
5. Push and open a PR

You'll need the Go version in `go.mod`, kubebuilder, a cluster (kind is fine),
and access to whichever app you're working on. Most of the interesting behavior
only shows up against a real Sonarr or Jellyfin, so it helps to have one
running.

```bash
make run                      # run the operator against your current kube context
make build                    # the operator and the CLI
make docker-build             # the operator's image for this machine; IMG= to tag, ARCH= to cross-build
make test
make manifests generate       # after editing anything in api/
make sync-crds                # copy generated CRDs into the chart
make lint
```

## Layout

One binary, `cmd/operator`, runs a controller per app, and one chart,
`chart/media-operator`, installs it with the CRDs of the apps it runs. The apps
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
the chart's `apps` and `media-operator.enabledApps`.

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

A release is an annotated tag on `main`:

```bash
git tag -a v0.37.0 -F- <<'EOF'
v0.37.0: one-line summary, used as the release title

Optional upgrade notes: what a user has to do or check before upgrading.
EOF
git push origin v0.37.0
```

Anything after the subject's blank line goes under Upgrade notes. Git strips
lines starting with `#` from tag messages, so use plain paragraphs or `-`
bullets, not headings. The notes also list `kubectl apply` commands for any CRD
that changed since the previous tag, because Helm never upgrades CRDs.
