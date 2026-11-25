## What this changes

<!-- Brief description. Link the issue if there is one. -->

## Testing

<!--
Which app and version did you test against? Unit tests are good but these
integrations mostly break against real APIs, so say what you actually ran this
against if you could.
-->

## Checklist

- [ ] `make lint test test-chart verify-crds verify-api-docs` passes
- [ ] Ran `make manifests generate sync-crds api-docs` if I touched `api/`
- [ ] Added or updated a sample in `config/samples/` for new fields
- [ ] New CRD fields have comments explaining them to someone unfamiliar with the app's API
- [ ] If I added an app, its controller is in `cmd/operator/main.go` and its name in the chart's `values.yaml` and `values.schema.json`
