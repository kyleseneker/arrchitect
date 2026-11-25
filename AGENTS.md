# AGENTS.md

Guidance for coding agents working on Arrchitect. Read it with
[CONTRIBUTING.md](CONTRIBUTING.md), which has the full test table, the package
layout and the release process.

## What this is

Arrchitect is a Kubernetes operator, built with kubebuilder and
controller-runtime, that configures self-hosted media apps through their HTTP
APIs: Sonarr, Radarr, Lidarr, Readarr, Prowlarr, FlareSolverr, Bazarr,
qBittorrent, SABnzbd, Jellyfin, Plex, Seerr, Maintainerr, Tdarr and Autobrr. It
doesn't deploy the apps. Each app has a custom resource in
`arrchitect.dev/v1alpha1`, and its controller makes the app's settings match
the resource on every reconcile.

## Commands

- `make help` lists every target.
- Before you call work done, run
  `make lint test test-chart verify-crds verify-api-docs`. CI runs the same
  targets.
- After changing anything under `api/`, run
  `make manifests generate sync-crds api-docs` and commit what it regenerates.
  The two verify targets compare against git, so they fail until the
  regenerated files are committed.
- `make test-compat` and `make test-e2e` start containers and take several
  minutes. Don't run them unless asked. CI runs compat against each app's
  oldest and newest supported release.
- `go vet -tags compat ./test/compat/` checks that the compat suite compiles
  without running it.

## Layout

- `api/<group>/v1alpha1/`: the API types. `api/common/v1alpha1/` holds the
  types several apps share.
- `internal/controller/<group>/`: one controller per app.
  `internal/controller/common/` holds the shared status, appRef and finalizer
  helpers.
- `internal/client/<app>/`: a client for each app's HTTP API.
- `internal/engine/`: shared reconcile logic, the HTTP client, ownership
  tracking, and the status and event reason constants.
- `internal/export/`: `arrchitect export`, which writes a resource from a
  running app.
- `chart/arrchitect/`: the Helm chart, with `values.schema.json`.
- `config/samples/`: one readable sample per kind. `examples/`: complete setups
  users copy. envtest applies every file in both to a real API server.
- `test/compat/`: each app in a container, reconciled from the fixtures in
  `test/compat/testdata/`.
- `test/consistency/`: checks that the chart, CRDs, README, Renovate config and
  API docs agree with the code.

## Generated files

Never edit these by hand. Change the source and regenerate.

- `api/**/zz_generated.deepcopy.go`, from `make generate`
- `config/crd/bases/`, from `make manifests`
- `chart/arrchitect/files/crds/`, from `make sync-crds`
- `docs/api.md`, from `make api-docs`, built from the API doc comments

## Changing the API

- A field copies the app's own API key in camelCase. When the app's key is
  snake_case or lowercase, write initialisms as words: `useAutoTmm`,
  `transcodeCpu`. `test/consistency` rejects capitals in a row unless the name
  copies the app's key and is listed in `mirroredCapitals`.
- Use `enable` or `enabled`, whichever the app uses.
- A reference to another item in the same app ends in `Name`, such as
  `downloadClientName`. `Ref` is only for Kubernetes object references, such as
  `appRef` and `apiKeySecretRef`.
- Go field names use Go's initialisms (`BaseURL`, `UseSSL`, `ProfileID`). The
  json tag holds the name users write.
- Every field's doc comment starts with its json name, and equivalent fields
  read the same across apps. These comments become `docs/api.md` and the CRD
  descriptions.
- Keep sibling apps consistent. A capability added to Sonarr goes to Radarr,
  Lidarr and Readarr too, unless the app lacks it. Then `docs/guide.md` says
  which app lacks what.
- A new field needs the type with its kubebuilder markers, the controller
  mapping, export support, and a value in the compat fixture
  (`test/compat/testdata/<app>.yaml`) so CI tests it against the real app. Add
  it to the kind's sample when the sample shows similar fields. A field the app
  only has from some version on goes in that app's `newerFields` in the compat
  suite.
- Renaming a json field or tightening validation is a breaking change. Mark the
  commit with `!` and add an Upgrading line to the release notes.
- CEL validation rules run against a cost budget. A rule that walks a list of
  objects can exceed it, so check new rules with envtest.

## Code

- Tests use testify's `require` and `assert`, never `t.Fatal` or `t.Error`.
  Inside an `httptest` handler goroutine, use `assert`, since `require` can't
  stop the test from there.
- A test lives in the `_test.go` file that mirrors the source file it tests.
  Shared test helpers go in a package such as `internal/testnet`, not in
  topic-named test files.
- Write comments only for what the code can't say. Don't add comments that
  restate the next line.
- Errors spell app names as the apps do, through `engine.AppName`:
  `Maintainerr has no Radarr server named "M"`,
  `Tdarr has more than one node named "T"`. The lint config allows a capital
  first letter in an error string only for an app name.
- Report status through `ctrlcommon.UpdateSyncStatus` and the reason constants
  in `internal/engine/controller.go`. An appRef failure goes through
  `common.AppRefFailure`, which picks the reason and the requeue.
- Never put a credential in a spec, status, event or log line. Status messages
  strip every Secret value the operator has read.
- Prune and `deletionPolicy: delete` only delete items the operator created,
  which `status.managedResources` records.

## Things that have caught us before

- Sonarr, Radarr, Lidarr and Readarr leave null fields out of their responses.
  Check whether a key is present instead of reading it as null.
- App API keys often appear in code and tests as strings, such as Jellyfin's
  `"PublishedServerUriBySubnet"`. Renaming a Go identifier must not touch them.
  Several controllers map fields to app keys through the json tag (Jellyfin
  pascal-cases it, Tdarr sends it as is), so changing a json tag changes what
  the app receives.
- Changing a field from an object to a list breaks server-side apply of
  resources already stored. Avoid it, or put the one-off fix in the release
  notes.
- A scoped API key, as Autobrr allows, gets 403 on kinds the operator doesn't
  own. List only kinds that `engine.Ownership` says the operator owns. Compat
  can't catch this, because its key has every scope.
- `git grep -E` doesn't support `\b`. Try a search on a known hit before
  trusting an empty result.

## Docs and YAML

- Write short, plain sentences. Avoid colon-then-reveal sentences,
  "not X, but Y", em dashes, stacked clauses and cute phrasing. Taglines have
  no trailing period.
- Spell apps as they do: qBittorrent, SABnzbd, Seerr, FlareSolverr. Never write
  "Servarr". Name the apps, or say "PVR apps". Readarr is archived upstream,
  not deprecated.
- Lead with plain manifests or the apps' official charts, not a generic chart
  such as app-template.
- In YAML, use `http://<app>.media.svc.cluster.local:<port>`, namespace
  `media`, Secrets named `<thing>-credentials` with kebab-case keys such as
  `api-key`, media under `/data/media/<tv|movies|music|books>`, download
  categories `tv`, `movies`, `music` and `books`, and placeholders on
  `example.com`. Don't name real indexers, trackers or piracy sites.
- Keep links in `README.md` relative. The release copies it into the chart and
  points them at the tag.

## Commits

- Use Conventional Commits with the app or area as the scope:
  `fix(tdarr): ...`, `feat(chart)!: ...`. Write one short subject line and no
  body.
- `feat` and `fix` subjects should read as a change a user would notice. They
  feed the release notes and the Artifact Hub changelog.
- Don't add AI attribution, such as `Co-Authored-By` or "Generated with" lines.
