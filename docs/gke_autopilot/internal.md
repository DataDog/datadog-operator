# GKE Autopilot Technical Dive

This document describes how GKE Autopilot support is implemented in the Datadog Operator.

## Entry point

Autopilot mode is enabled through a `DatadogAgent` annotation:

```yaml
metadata:
  annotations:
    experimental.agent.datadoghq.com/autopilot: "true"
```

The constants live in `internal/controller/datadogagent/experimental/const.go`:

- `ExperimentalAnnotationPrefix = "experimental.agent.datadoghq.com"`
- `ExperimentalAutopilotSubkey = "autopilot"`
- `ExperimentalAutopilotAllowlistVersionSubkey = "autopilot-allowlist-version"`

`experimental.IsAutopilotEnabled(obj)` reads the annotation and returns true when its value equals `true`, case-insensitively.

## Reconcile flow

The public `DatadogAgent` controller does not build the final Agent DaemonSet directly. Its flow is:

```text
DatadogAgent
  -> validate and default spec
  -> manage DatadogAgent-owned dependencies
  -> generate DatadogAgentInternal
  -> copy annotations from DatadogAgent to DatadogAgentInternal
  -> create or update DatadogAgentInternal
```

Relevant code:

- `internal/controller/datadogagent/controller_reconcile_v2.go`
- `internal/controller/datadogagent/ddai.go`

`generateObjMetaFromDDA` copies annotations from the `DatadogAgent` to the generated `DatadogAgentInternal`, excluding only `kubectl.kubernetes.io/last-applied-configuration`. This is what carries the Autopilot annotation into the internal reconciler.

The `DatadogAgentInternal` controller then builds the actual workloads:

```text
DatadogAgentInternal
  -> default spec
  -> build configured/enabled features and required components
  -> manage global and feature dependencies
  -> reconcile Deployment-backed components
  -> reconcile Node Agent DaemonSet or ExtendedDaemonSet
  -> cleanup stale resources
```

Relevant code:

- `internal/controller/datadogagentinternal/controller_reconcile_v2.go`
- `internal/controller/datadogagentinternal/controller_reconcile_agent.go`
- `internal/controller/datadogagentinternal/component_reconciler.go`

## Where Autopilot mutates the Node Agent

The Node Agent path in `reconcileV2Agent` is the main pod-template integration point.

For a DaemonSet, the order is:

```text
NewDefaultAgentDaemonset
  -> global provider capabilities
  -> global Node Agent settings
  -> feature ManageNodeAgent or ManageSingleContainerNodeAgent
  -> feature provider capabilities
  -> spec.override.nodeAgent
  -> experimental.ApplyExperimentalOverrides
  -> untaint startup toleration
  -> create or update DaemonSet
```

`experimental.ApplyExperimentalOverrides` calls:

1. `applyExperimentalImageOverrides`
2. `applyExperimentalAutopilotOverrides`

The Autopilot mutation therefore runs after `spec.override.nodeAgent` for the Node Agent workload. User overrides still apply to fields that Autopilot does not subsequently rewrite or remove, such as container resource requirements and `priorityClassName`. Overrides that re-add blocked volumes or mounts are stripped again by the Autopilot pass.

Deployment-backed components (`clusterAgent`, `clusterChecksRunner`, `otelAgentGateway`) go through `datadogagentinternal/component_reconciler.go`. That path applies global settings, features, and component overrides, but it does not call `experimental.ApplyExperimentalOverrides`.

## Autopilot pod-template changes

The implementation is in `internal/controller/datadogagent/experimental/autopilot.go`.

When enabled, it applies these Node Agent changes:

- Creates or updates the GKE `AllowlistSynchronizer`.
- Adds `DD_KUBELET_USE_API_SERVER=true` to use the API server instead of the kubelet for pod discovery.
- Adds `DD_CLOUD_PROVIDER_METADATA=["gcp"]`.
- Adds pod label `admission.datadoghq.com/enabled: "false"` so the Datadog admission controller does not mutate the Agent.
- Changes the `init-volume` container args to `cp -r /etc/datadog-agent /opt`.
- Removes disallowed Agent volumes, including auth, CRI socket, DogStatsD socket, and APM socket volumes.
- Rewrites the run path hostPath to `/var/autopilot/addon/datadog/logs` when the run path is hostPath-backed.
- Removes `DD_AUTH_TOKEN_FILE_PATH` from init containers and containers.
- Removes disallowed volume mounts per container.
- Marks the seccomp security mount read-only on init containers.
- Replaces the `trace-agent` command with `trace-agent -config=/etc/datadog-agent/datadog.yaml`.
- Replaces the `process-agent` command with `process-agent -config=/etc/datadog-agent/datadog.yaml`.

The forbidden mount lists are container-specific. For example, `trace-agent` drops auth, CRI socket, DogStatsD socket, proc, cgroups, and APM socket mounts, while `system-probe` drops auth, CRI socket, and DogStatsD socket mounts.

NPM-related volumes and mounts on `system-probe` are intentionally preserved. The tests assert that `proc`, `cgroups`, `debugfs`, `system-probe-socket`, and `HostPID` survive the Autopilot pass because the WorkloadAllowlist grants the required exemptions.

## AllowlistSynchronizer

The GKE `AllowlistSynchronizer` integration lives in `pkg/allowlistsynchronizer`.

`CreateAllowlistSynchronizer(version, partOfLabel)`:

- Resolves the requested WorkloadAllowlist version.
- Falls back to `DefaultWorkloadAllowlistVersion` when the annotation is empty or malformed.
- Builds a direct controller-runtime client from the current kubeconfig.
- Server-side applies an `auto.gke.io/v1` `AllowlistSynchronizer` named `datadog-synchronizer`.

The generated object points at:

```text
Datadog/datadog/datadog-datadog-daemonset-exemption-<version>.yaml
```

The default version is defined by `DefaultWorkloadAllowlistVersion` in `pkg/allowlistsynchronizer/allowlistsynchronizer.go`.

## Feature-specific Autopilot behavior

Several features check `experimental.IsAutopilotEnabled` during `Configure`.

APM, in `internal/controller/datadogagent/feature/apm/feature.go`:

- Enables host port mode.
- Disables Unix domain socket mode.

DogStatsD, in `internal/controller/datadogagent/feature/dogstatsd/feature.go`:

- Enables host port mode.
- Uses the configured host port when present, otherwise the default DogStatsD port.
- Disables Unix domain socket mode.

Admission Controller, in `internal/controller/datadogagent/feature/admissioncontroller/feature.go`:

- Defaults Agent communication mode to `hostip` when the user has not set `agentCommunicationMode`.

These feature defaults happen before Node Agent pod-template overrides and before the final Autopilot mutation pass.

## RBAC behavior

Node Agent RBAC is adjusted in `internal/controller/datadogagent/global/dependencies.go`.

When Autopilot is enabled:

- Fine-grained kubelet authorization is enabled by default.
- The Agent ClusterRole includes the API-server pod permissions required by `DD_KUBELET_USE_API_SERVER=true`.

The lower-level rule construction is in `internal/controller/datadogagent/component/agent/rbac.go`.

## Testing

Focused tests:

- `internal/controller/datadogagent/experimental/autopilot_test.go`
- `internal/controller/datadogagent/controller_v2_test.go`, `Test_AutopilotOverrides`

Add tests when changing:

- Annotation parsing or constants.
- The WorkloadAllowlist version resolver.
- Any forbidden volume or mount list.
- The relative ordering of overrides and Autopilot mutations.
- Feature defaults that branch on `IsAutopilotEnabled`.
