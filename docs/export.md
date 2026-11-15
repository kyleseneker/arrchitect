# Exporting an existing app

`arrchitect export` reads a running app and prints a config resource that
matches it, plus a Secret for its credentials.

## Install

Download the `arrchitect-cli` archive for your platform from the
[releases page](https://github.com/kyleseneker/arrchitect/releases), or run
the `ghcr.io/kyleseneker/arrchitect-cli` image. Both are signed; see
[Verifying a release](guide.md#verifying-a-release).

## Usage

```bash
arrchitect export <app> --url <url> [flags] > config.yaml
```

From a workstation, through a port-forward:

```bash
kubectl -n media port-forward svc/sonarr 8989 &
arrchitect export sonarr \
  --url http://localhost:8989 --connection-url http://sonarr.media:8989 \
  --api-key "$SONARR_API_KEY" --namespace media > sonarr.yaml
```

From inside the cluster:

```bash
kubectl -n media run export --rm -i --quiet --restart=Never \
  --image ghcr.io/kyleseneker/arrchitect-cli:<version> \
  -- export sonarr --url http://sonarr:8989 --api-key "$SONARR_API_KEY" --namespace media > sonarr.yaml
```

| Flag | Description |
| --- | --- |
| `--url` | URL to read the app from |
| `--connection-url` | URL the operator should use, if different from `--url` |
| `--api-key` | API key, or `$ARRCHITECT_API_KEY` |
| `--username`, `--password` | Login for qBittorrent and Jellyfin, or `$ARRCHITECT_USERNAME` and `$ARRCHITECT_PASSWORD` |
| `--token` | Plex token, or `$ARRCHITECT_TOKEN` |
| `--name` | Name of the resource; defaults to the app |
| `--namespace` | Namespace of the resource and Secret |
| `--secret` | Name of the Secret; defaults to `<name>-credentials` |
| `--exclude` | Comma-separated spec fields to leave out |
| `--secret-file` | Write the Secret to this file instead of with the config |
| `--enforce` | Leave out observe mode |
| `--insecure-skip-verify` | Skip TLS certificate verification |

FlareSolverr and Maintainerr need no credentials. Every other app takes
`--api-key`, except as listed above.

## Output

Credentials the app returns are written to the Secret. Ones it masks are left
out of it and listed on stderr; add them before applying. Until then the
resource reports the missing key. Use `--secret-file` to keep the Secret out of
a file you commit.

Fields still at the app's defaults are omitted where the app reports its
defaults. The command also prints a warning for anything the resource can't
represent or the app doesn't report, such as a Plex preference Plex doesn't
list. Seerr notification options that look like credentials go to the Secret.

Use `--exclude` for parts managed by another tool, for example `indexers` when
Prowlarr syncs them, or `customFormats,qualityProfiles,qualityDefinitions` when
Recyclarr does.

## Applying the result

The resource is written in observe mode, `spec.reconcile.driftPolicy: observe`,
so applying it changes nothing in the app. It should report `Synced` with
reason `Observed`. If it reports `DriftDetected` instead, the condition's
message lists what differs; resolve those before removing observe mode.
