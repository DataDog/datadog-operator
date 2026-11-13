# check-operator

`check-operator` is a CLI to run checks against the operator.
The main use case is to run it as a [Helm chart test](https://helm.sh/docs/topics/chart_tests/) to validate the rolling update of the Agent, based on the DatadogAgent status and the reconcile status of its associated DatadogAgentInternal resources.

## Optional node Agent startup timings

The `upgrade` command can log pod status and completed init-container durations:

```sh
check-operator upgrade datadog-agent -n datadog \
  --collect-startup-timings \
  --startup-experiment=init-removal \
  --startup-variant=baseline
```

Collection is disabled by default. When enabled, it runs immediately and every
30 seconds while the existing upgrade check runs. It uses a five-second timeout
for each API request, paginates pod lists, and checks pod owner UIDs against the
DaemonSets named in the DatadogAgent and its DatadogAgentInternal statuses.
This includes profile-specific node Agent DaemonSets, not deployment pods.

Collection runs separately from rollout checking. API errors, missing permissions,
and slow telemetry output do not change the upgrade result or make it wait for
collection. The job can exit before collection finishes, so missing samples must
not be treated as successful or zero-duration starts.

JSON records are written to stderr, separately from the existing progress output:

- `agent_startup_snapshot`: pod identity, DaemonSet, node, creation/start times,
  conditions, and current/previous init and regular container states. Container
  statuses include images, image IDs, readiness, and restart counts. Pod specs,
  environment variables, configuration contents, mounts, resources, user/group
  details, container IDs, and full owner references are not logged. Conditions
  include only type, status, and last transition time.
- `agent_startup_measurement` with `phase:init_execution`: one retained completed init attempt, with source
  timestamps, `duration_seconds`, `restart_count`, exit code, and termination reason.
  `phase:init_execution` means `finishedAt - startedAt`; it does not include image
  pulls or waits before the container starts. Failed completed attempts are included.
- `agent_startup_measurement` with `phase:creation_to_scheduled`: scheduled time
  minus pod creation time.
- `agent_startup_measurement` with `phase:creation_to_initialized`: the current
  `Initialized=True` transition minus pod creation time, including preparation
  and waits before/between init containers.
- `agent_startup_measurement` with `phase:creation_to_latest_ready`: the current
  `Ready=True` transition time minus pod creation time. A later return to readiness
  produces a new sample; this is not guaranteed first readiness.
- `agent_startup_collection_error`: a collection error, independent of check success.

Unchanged snapshots and repeated completed attempts are deduplicated within one
invocation. Different invocations can observe the same pod again; use a separate
experiment identifier or deduplicate by pod UID, container, restart count, and
start/finish timestamps when comparing runs. A snapshot cannot recover all retries
or prove that the current readiness transition is the first. Missing or reversed
init timestamps produce no duration. Zero-second timestamp differences are retained; Kubernetes status timestamps
have second resolution, so zero does not mean no execution time.

The service account needs its existing DatadogAgent/DatadogAgentInternal permissions,
plus `get` on `apps/daemonsets` and `list` on core `pods` in the target namespace.
No additional write permissions are needed. The deployment job must explicitly
pass the flags above; this change does not enable collection on any cluster.

For later logs-to-metrics setup, use a distribution named
`contp.agent_startup.init_execution`, filtering
`@event:agent_startup_measurement @phase:init_execution` and measuring
`@duration_seconds`. Group by `container_name`, `experiment_id`, `variant`, and
cluster; retain pod UIDs in logs rather than metric tags. Separate failures and
retries from successful first attempts using `exit_code` and `restart_count`.
This metric is not created by the command.

Agent initialization tracing is separate: set `DD_FX_TRACING_ENABLED=true` on
Agent containers under test, not on this check job. Verify traces under service
`dd-agent-fx-init` before comparing runs. First-readiness collection is deferred;
the upgrade collector reports only the latest readiness transition.

Build a custom image from this repository for an internal test deployment:

```sh
docker build -f check-operator.Dockerfile \
  --build-arg GOARCH=amd64 -t operator-check:startup-telemetry .
```

Use `GOARCH=arm64` with an arm64 builder/target. Publishing the image and enabling
it in an internal cluster are separate deployment steps.

## Derived pod timing metrics

| Proposed metric | `phase` filter | Measure |
| --- | --- | --- |
| `contp.agent_startup.creation_to_scheduled` | `creation_to_scheduled` | `@duration_seconds` |
| `contp.agent_startup.creation_to_initialized` | `creation_to_initialized` | `@duration_seconds` |
| `contp.agent_startup.creation_to_latest_ready` | `creation_to_latest_ready` | `@duration_seconds` |

All filters also require `@event:agent_startup_measurement`. Latest-readiness
samples can include recoveries, so they cannot establish exact time to first
readiness. These log-based metrics still need to be created in Datadog.

Create these distributions in Logs > Configuration > Generate Metrics using
`source:datadog-check-operator kube_cluster_name:porygon @event:agent_startup_measurement`
plus the appropriate `@phase` filter. Select `@duration_seconds`, enable
percentiles, and group by `@experiment_id`, `@variant`, `@daemonset`, and
`kube_cluster_name`. Add `@container_name` for init execution. Do not include
an index filter: these rules run on ingestion, not stored logs. Create the rules
before the measurement run; they do not backfill existing logs.

Repeated checker jobs can measure the same startup again. Separate experiment
IDs make runs distinguishable but do not deduplicate them. For comparisons,
collect each cohort once and use disjoint pod creation windows, or deduplicate
the retained logs before computing statistics. The collector does not currently
enforce a creation window or deduplicate across jobs.
