# AGENTS.md

Guidance for coding agents working on Arrchitect. It covers what you need to
change code safely here. [CONTRIBUTING.md](CONTRIBUTING.md) has the full test
table, the package layout and the release process, and this file links to it
instead of repeating it.

Arrchitect is a Kubernetes operator, built with kubebuilder and
controller-runtime, that configures self-hosted media apps through their HTTP
APIs: Sonarr, Radarr, Lidarr, Readarr, Prowlarr, FlareSolverr, Bazarr,
qBittorrent, SABnzbd, Jellyfin, Plex, Seerr, Maintainerr, Tdarr and Autobrr. It
doesn't deploy the apps. Each app has a custom resource in
`arrchitect.dev/v1alpha1`, and its controller makes the app's settings match
the resource on every reconcile.

## Agree on these first

Open an issue, or ask in the pull request, before you:

- Make a breaking API change: rename or remove a json field, change a field's
  type, or tighten validation. These break existing resources, so they're
  planned into a minor release and documented in its notes.
- Change an app's supported version range. Ranges follow
  [docs/stability.md](docs/stability.md#apps), and floors only move in a minor
  release.
- Add an app. Start with the "Support a new app" issue form, which asks for the
  app's API docs and the settings people want managed.

Maintainers cut releases, so leave tags and release notes alone. Renovate opens
routine dependency and tool updates, so a pull request only adds or bumps a
dependency when the change needs it.

## Commands

```bash
make help                                          # every target
go test ./internal/controller/pvr/ -run TestName   # one package or test while you work
make lint                                          # golangci-lint, including kube-api-linter
make test                                          # unit, envtest and consistency tests
make test-chart                                    # the chart's helm-unittest suites
make manifests generate sync-crds api-docs         # regenerate after any change under api/
make verify-crds verify-api-docs                   # generated files match, once committed
make test-compat COMPAT_RUN=TestSonarr             # one app in a container, against the real app
make test-e2e                                      # the image and chart in a kind cluster
```

`test/envtest` needs `KUBEBUILDER_ASSETS`, which `make test` sets. To run it on
its own, copy the variable from the `test` target in the Makefile.

`make test-compat` needs Docker, or Podman with `CONTAINER_TOOL=podman`, and
`make test-e2e` needs Docker. Both take several minutes. CI runs them on every
pull request, but running compat for the apps you changed catches API mismatches
sooner. `COMPAT_VERSIONS=oldest` tests each app's oldest supported release.
Where containers aren't available, `go vet -tags compat ./test/compat/` at least
checks that the compat suite compiles.

## Before you call it done

Run what matches the change, then the full gate:
`make lint test test-chart verify-crds verify-api-docs`. CI runs the same
targets, plus compat and e2e.

| You changed | Also run or check |
| --- | --- |
| `api/` | `make manifests generate sync-crds api-docs` and commit the output. The verify targets compare against git, so they fail until it's committed. |
| A controller, client or `internal/export/` | The package's tests with `go test`. A new or changed field also needs a compat fixture value (see below), and `make test-compat COMPAT_RUN=Test<App>` if you can run containers. |
| `chart/` | `make test-chart` and `helm lint chart/arrchitect`. A new value goes in `values.yaml` and `values.schema.json`. |
| `config/samples/`, `examples/` or YAML in docs | `make test`. envtest applies every sample and example to a real API server. |
| `test/compat/` | `make test-compat COMPAT_RUN=Test<App>` for the case you changed, or at least `go vet -tags compat ./test/compat/` |
| `README.md` | `go test ./test/consistency/...`, which checks the supported-apps table. Keep links relative. |

## Where things are

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
- `chart/arrchitect/`: the Helm chart and its `values.schema.json`.
- `config/samples/`: one readable sample per kind. `examples/`: complete setups
  users copy.
- `test/compat/`: each app in a container, reconciled from the fixtures in
  `test/compat/testdata/`. `test/envtest/`: the CRDs, samples and admission
  rules against a real API server. `test/consistency/`: checks that the chart,
  CRDs, README, Renovate config and API docs agree with the code.

Copy these when you add something similar:

- A controller's client setup: `tdarrClient` in
  `internal/controller/transcode/tdarr_controller.go`. It returns the client, a
  reason and an error, and cleanup reuses it.
- Status and events: `UpdateSyncStatus` in
  `internal/controller/common/helpers.go`, with the reasons in
  `internal/engine/controller.go`.
- appRef resolution: `internal/controller/common/appref.go`. A failure goes
  through `AppRefFailure`, which picks the reason and the requeue.
- A compat case: `compatCase` in `test/compat/main_test.go`. A case has a
  fixture, an `update` patch that changes what the first reconcile created,
  `newerFields` for fields older releases lack, and an `export` check.
- A CEL validation rule's test: `TestAdmissionRules` in
  `test/envtest/apply_test.go`.

## Generated files

Never edit these by hand. Change the source, then regenerate.

| File | Source | Command |
| --- | --- | --- |
| `api/**/zz_generated.deepcopy.go` | the API types | `make generate` |
| `config/crd/bases/*.yaml` | the API types and markers | `make manifests` |
| `chart/arrchitect/files/crds/*.yaml` | `config/crd/bases/` | `make sync-crds` |
| `docs/api.md` | the API doc comments | `make api-docs` |

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
  it to the kind's sample when the sample shows similar fields. If only newer
  releases of the app have it, list it in the case's `newerFields`.
- A breaking change gets `!` in the commit and an Upgrading line in the release
  notes. The API is `v1alpha1`, so a minor release may break it, but prefer
  additive changes.
- CEL validation rules run against a cost budget. A rule that walks a list of
  objects can exceed it, so test new rules in `TestAdmissionRules`.

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
- Keep the existing requeue policy: 30 seconds for a missing Secret, 60 when an
  app is unreachable, none for `InvalidConfig`. The shared helpers apply it.

## Safety

- Never put a credential in a spec, status, event or log line. Read Secrets
  through `reconciler.ResolveSecretKeyRef`, which registers each value for
  redaction, and pass any text an app sends back, such as an error body,
  through `reconciler.Redact` before it goes into an error.
- TLS verification stays on unless the user sets `insecureSkipVerify`. Every
  client built for an app, including the target of an `appRef`, passes that
  connection's TLS settings and its app name: `engine.WithTLSConfig` and
  `engine.WithAppLabel`.
- The engine's HTTP client refuses loopback, link-local, unspecified and cloud
  metadata addresses, follows redirects only on the app's own host and caps
  response bodies. Don't weaken that. Tests serve on `testnet.RoutableAddr(t)`
  or pass `engine.WithTransport`.
- Prune and `deletionPolicy: delete` only delete items the operator created,
  which `status.managedResources` records. Never delete an item the operator
  didn't create.

## Things that have caught us before

- Sonarr, Radarr, Lidarr and Readarr leave null fields out of their responses.
  Check whether a key is present instead of reading it as null.
- App API keys often appear in code and tests as strings, such as Jellyfin's
  `"PublishedServerUriBySubnet"`. Renaming a Go identifier must not touch them.
  Several controllers map fields to app keys through the json tag (Jellyfin
  pascal-cases it, Tdarr sends it as is), so changing a json tag changes what
  the app receives.
- The PVR clients take a provider's config contract from the app's schema.
  Prowlarr's guess `<implementation>Settings`.
- Changing a field from an object to a list breaks server-side apply of
  resources already stored. Avoid it, or put the one-off fix in the release
  notes.
- A scoped API key, as Autobrr allows, gets 403 on kinds the operator doesn't
  own. List only kinds that `engine.Ownership` says the operator owns. Compat
  can't catch this, because its key has every scope.
- qBittorrent 5.3 answers a bad password and a Host header that doesn't match
  its port with the same bare 401.
- Seerr's CSRF protection blocks API writes, so the operator leaves that
  setting alone on purpose.

## Docs and YAML

- Write short, plain sentences. Avoid colon-then-reveal sentences,
  "not X, but Y", em dashes, stacked clauses and cute phrasing. Taglines have
  no trailing period.
- Spell apps as they do: qBittorrent, SABnzbd, Seerr, FlareSolverr. Never write
  "Servarr". Name the apps, or say "PVR apps". Readarr is archived upstream,
  not deprecated, and supported at its final releases.
- Lead with plain manifests or the apps' official charts, not a generic chart
  such as app-template.
- In YAML, use `http://<app>.media.svc.cluster.local:<port>`, namespace
  `media`, Secrets named `<thing>-credentials` with kebab-case keys such as
  `api-key`, media under `/data/media/<tv|movies|music|books>`, download
  categories `tv`, `movies`, `music` and `books`, and placeholders on
  `example.com`. Don't name real indexers, trackers or piracy sites.
- Keep links in `README.md` relative. The release copies it into the chart and
  points them at the tag.

## Pull requests

- Pull requests are squash merged, and the title becomes the commit subject.
  Write the title as a Conventional Commit with the app or area as the scope:
  `fix(tdarr): ...`, `feat(chart)!: ...`. A `!` marks a change that needs
  action on upgrade.
- `feat` and `fix` titles should read as a change a user would notice. They
  feed the release notes and the Artifact Hub changelog. `docs`, `test`, `ci`,
  `refactor` and most `chore` changes are left out of both.
- Fill in the pull request template, including which app versions you tested
  against. These integrations mostly break against real APIs, so say what you
  actually ran.
- Keep a pull request to one change.

## Keeping this file current

When an agent makes the same mistake twice, or a reviewer corrects the same
thing twice, add a line here. When a command, path or rule changes, change it
here in the same commit. A stale instruction is worse than none.
