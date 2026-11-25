# Security

## Reporting a vulnerability

Use [private vulnerability reporting](https://github.com/kyleseneker/arrchitect/security/advisories/new)
rather than opening a public issue.

This is a side project with one maintainer, so there is no response SLA.
Reports are acknowledged as soon as possible.

## Supported versions

The latest release only. There are no backport branches.

## What this operator has access to

- **It can read every credential in your media stack.** Any credential a
  resource references, such as a Sonarr API key, a Plex token or a download
  client password, is read from a Secret and sent to that app. Its
  ServiceAccount can read Secrets in whatever namespaces it watches. Set
  `watchNamespace` to one or more namespaces, comma-separated, to narrow that
  down. It reads a Secret only when a resource uses it and doesn't cache
  Secrets. It does hold the current value of each key it has read, so it can
  strip them from status messages, events and logs.
- **It can delete things in those apps.** Only items the operator created, and
  only when you ask: `prune`, `deletionPolicy: delete` on an app's resource, or
  deleting an indexer or download client resource, whose `deletionPolicy`
  defaults to `delete`. They are real delete calls.
- **`insecureSkipVerify` turns off certificate checks** for that app's
  connection. Use it only on a network you trust, and prefer `caSecretRef` with
  your own CA.
- **Metrics are authenticated.** The `:8443/metrics` endpoint requires a
  TokenReview-validated bearer token, served over HTTPS with a self-signed cert.
  Resource names appear in `arrchitect_config_synced` labels; no credentials
  do.

Credentials are never written to resource specs or status. A status message
carries the reason an app gives for refusing a change, with every Secret value
the operator has read removed from it. If you find a credential leaking into a
log line or a status message, report it privately.
