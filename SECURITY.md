# Security

## Reporting a vulnerability

Use [private vulnerability reporting](https://github.com/kyleseneker/arrchitect/security/advisories/new)
rather than opening a public issue.

This is a side project with one maintainer, so I'm not going to pretend to a
response SLA. I'll acknowledge what I can, as fast as I can.

## Supported versions

The latest release only. There are no backport branches.

## What this operator has access to

Worth understanding before you deploy it:

- **It holds every API key in your media stack.** Sonarr, Radarr, Plex tokens,
  download client passwords — anything you reference from a CR gets read out of a
  Secret and sent to the corresponding app. Its ServiceAccount can read Secrets in
  whatever namespaces it watches. Set `watchNamespace` to one or more namespaces,
  comma-separated, to narrow that down. It reads a Secret only when a resource
  uses it and doesn't keep Secret contents in memory.
- **It can delete things in those apps.** Only items the operator created, and
  only when you ask: `prune`, `deletionPolicy: delete` on an app's resource, or
  deleting an indexer or download client resource, whose `deletionPolicy`
  defaults to `delete`. They are real delete calls.
- **`insecureSkipVerify` does what it says.** It disables certificate
  verification for that app's connection. Fine on a LAN you trust, not fine over
  anything you don't. Prefer `caSecretRef` with your own CA.
- **Metrics are authenticated.** The `:8443/metrics` endpoint requires a
  TokenReview-validated bearer token, served over HTTPS with a self-signed cert.
  Resource names appear in `arrchitect_config_synced` labels; no credentials
  do.

Credentials are never written to CR specs or status. A status message carries
the reason an app gives for refusing a change, with every Secret value the
operator has read removed from it. If you find a credential leaking into a log
line or a status message, that's a bug worth reporting privately.
