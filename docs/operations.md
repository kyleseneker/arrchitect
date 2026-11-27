# Running the operator

- [Health in Argo CD and Flux](#health-in-argo-cd-and-flux)
- [Troubleshooting](#troubleshooting)
- [Retries](#retries)
- [Alerting](#alerting)
- [Tuning](#tuning)

## Health in Argo CD and Flux

Every resource reports `Ready`, `Synced` and, for a failure no retry fixes,
`Stalled`. The [status section of the guide](guide.md#status) lists the
reasons.

### Argo CD

Argo CD has no built-in health check for `arrchitect.dev`, so it shows every
resource as Healthy. Add this to `argocd-cm`. It marks a resource Progressing
while the operator hasn't caught up or its app is briefly out of reach, and
Degraded when it's stalled or a sync failed. The wildcard works only in the
`resource.customizations` key, not in the per-kind
`resource.customizations.health.*` keys.

```yaml
data:
  resource.customizations: |
    arrchitect.dev/*:
      health.lua: |
        local hs = {status = "Progressing", message = "Waiting for the operator"}
        if obj.status == nil or obj.status.conditions == nil then return hs end
        if obj.status.observedGeneration ~= obj.metadata.generation then return hs end
        local ready, stalled
        for _, c in ipairs(obj.status.conditions) do
          if c.type == "Ready" then ready = c end
          if c.type == "Stalled" and c.status == "True" then stalled = c end
        end
        if ready == nil then return hs end
        if ready.status == "True" then return {status = "Healthy", message = ready.message} end
        if stalled ~= nil then return {status = "Degraded", message = stalled.reason .. ": " .. stalled.message} end
        local passing = {AppUnreachable = true, SecretNotFound = true, ReferenceNotFound = true}
        if passing[ready.reason] then return {status = "Progressing", message = ready.reason .. ": " .. ready.message} end
        return {status = "Degraded", message = ready.reason .. ": " .. ready.message}
```

A resource in observe mode that finds differences reports `DriftDetected`, so
it shows as Degraded too. Add `DriftDetected = true` to `passing` if you'd
rather see it as Progressing.

### Flux

Flux's health checks read `Ready`, `Stalled` and `observedGeneration`, so they
work without configuration. A `Stalled` resource fails the Kustomization at
once. Any other failure keeps it waiting until its `timeout`. To fail at once on
a sync the app refused as well, add a check for each kind you use. A custom
check replaces Flux's own for that kind, so it covers `Stalled` too:

```yaml
spec:
  wait: true
  healthCheckExprs:
    - apiVersion: arrchitect.dev/v1alpha1
      kind: Sonarr
      inProgress: status.observedGeneration != metadata.generation
      failed: >-
        status.conditions.exists(e, (e.type == 'Stalled' && e.status == 'True') ||
        (e.type == 'Ready' && e.status == 'False' && e.reason == 'SyncFailed'))
      current: status.conditions.exists(e, e.type == 'Ready' && e.status == 'True')
```

### kubectl

`kubectl wait --for=condition=Ready sonarr/sonarr` waits until the app matches
the resource.

## Troubleshooting

`kubectl describe` shows a resource's conditions and its events. A Warning
event is recorded each time `Ready` turns `False` or its reason changes, and a
Normal event when it turns `True` again. An event says what changed in a fixed
sentence, since events can reach people who can't read the resource. The
`Ready` condition holds the details.

| Reason | What to check |
| --- | --- |
| `AppUnreachable` | The app is running, and the connection URL resolves and answers from the operator's namespace. A NetworkPolicy may block it. |
| `Unauthorized` | The API key, token or password in the Secret matches the app's. For qBittorrent, also that the URL uses qBittorrent's own port. |
| `SecretNotFound` | The Secret and key the message names exist in the resource's namespace. |
| `ReferenceNotFound` | The app resource or `appRef` the message names exists in the same namespace. |
| `InvalidConfig` | The message names the setting. The resource waits for you to change it. |
| `NameConflict` | Another resource already configures the same item. The older one keeps it. |
| `SyncFailed` | The message lists each setting the app refused and the reason it gave. |

Set the chart's `logLevel` to `debug` to log the full error each app sends
back.

Changing any annotation reconciles a resource at once:

```bash
kubectl annotate sonarr/sonarr arrchitect.dev/reconcile-requested-at="$(date +%s)" --overwrite
```

## Retries

How soon a resource that failed tries again depends on why:

| Reason | Next try |
| --- | --- |
| `AppUnreachable` | After 30 seconds, doubling with each failure in a row, up to 10 minutes |
| `SyncFailed` from reading a Secret or an appRef's resource | The same |
| `SecretNotFound`, `ReferenceNotFound` | When the Secret or resource appears, or after the interval |
| `Unauthorized`, `InvalidConfig` | When the resource or a Secret it uses changes, or after the interval |
| `NameConflict`, any other `SyncFailed` | After the interval |

The interval is `spec.reconcile.intervalSeconds`, 5 minutes by default. An
indexer or download client takes its app resource's. A resource that synced
waits for the interval too, stretched by up to 10% at random so resources
created together don't all reach their app at once. Changing the resource, or
a Secret it uses, reconciles it at once.

Refused credentials aren't retried before the interval unless the resource or
a Secret changes, so an app that locks out failed sign-ins sees one attempt per
interval. A short interval for such an app makes a lockout come sooner.

A failure to read the resource itself, or the app resource an indexer or
download client belongs to, backs off without changing its status. A retry
that backs off counts in `controller_runtime_reconcile_errors_total`.

## Alerting

Set `metrics.prometheusRule.enabled` in the chart for these alerts:

| Alert | Fires when |
| --- | --- |
| `ArrchitectResourceNotReady` | A resource hasn't synced for 15 minutes. |
| `ArrchitectDriftNeverSettles` | The operator corrected the same item more than six times in two hours. Something else keeps changing it, or the app accepts the write and ignores it. |
| `ArrchitectDown` | Prometheus hasn't scraped the operator for 10 minutes. Needs `metrics.serviceMonitor.enabled`. |

The [metrics section of the guide](guide.md#metrics) lists the operator's own
metrics. controller-runtime adds `controller_runtime_reconcile_time_seconds`,
`workqueue_depth` and `workqueue_longest_running_processor_seconds`, which show
a controller falling behind.

## Tuning

| Chart value | Default | What it does |
| --- | --- | --- |
| `maxConcurrentReconciles` | `2` | How many resources of one kind reconcile at once. Raise it when one slow app holds up others of the same kind. |
| `reconcileTimeout` | `10m` | How long one reconcile may run before it's cancelled. |
| `watchNamespace` | every namespace | The namespaces whose resources the operator reconciles. Its access to resources and Secrets then comes from a Role in each of them instead of a ClusterRole. |

`spec.reconcile.intervalSeconds` on each resource sets how often it's
compared with its app.
