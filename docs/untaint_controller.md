# Untaint Controller

This feature was introduced in Datadog Operator v1.28 and is currently in preview.

## Overview

The Untaint controller watches Kubernetes Nodes carrying the taint
`agent.datadoghq.com/not-ready=presence:NoSchedule` and removes it when
readiness criteria are met (see below), or after a configurable timeout. It is
intended to run alongside a separate mechanism (cluster-autoscaler hook, CCM,
admission webhook, etc.) that adds the taint to new nodes.

**With `--untaintControllerEnabled=true` only** (and without `--untaintControllerWaitForCSIDriver`):
the controller removes the taint once the **node Agent** pod
(`agent.datadoghq.com/component=agent`) on that node is `Ready`. Agent pods are
listed in the operator's agent watch namespaces (`WATCH_NAMESPACE` /
`DD_AGENT_WATCH_NAMESPACE`).

**With `--untaintControllerEnabled=true` and `--untaintControllerWaitForCSIDriver=true`:**
the controller waits until **both** the node Agent and **CSI
node-server** pod (`app=datadog-csi-driver-node-server`) on the node are
`Ready` before removing the taint. The taint stays until both are
satisfied or a timeout fires. The operator's Pod informer then watches the
**union** of `DD_AGENT_WATCH_NAMESPACE` and `DD_CSIDRIVER_WATCH_NAMESPACE` (all
pods in those namespaces—keep namespaces tight). Ensure CSI namespaces are
covered so the controller can list CSI pod status.

**`--datadogCSIDriverEnabled`** only controls whether the **DatadogCSIDriver**
controller runs; it does **not** by itself turn on dual-readiness untaint.
Enable `--untaintControllerWaitForCSIDriver` only when you actually deploy CSI
node-server pods on tainted nodes (for example via a `DatadogCSIDriver` CR with
the operator's CSI controller enabled, or another install path that produces
the same pod labels).

If a required pod never reaches Ready on a tainted node, a configurable timeout
policy ensures the node is never permanently unschedulable. Two clocks cover
the main failure modes:

- **Readiness timeout** — at least one Agent pod is on the node but the Agent
  is not Ready yet, **or** (with `--untaintControllerWaitForCSIDriver`) the Agent is Ready but a CSI
  node-server pod exists on the node and is not Ready. Clock: latest
  `pod.Status.StartTime` among **Agent** pods in the first case, and among **CSI
  node-server** pods only in the second (the Agent’s age does not shorten the
  wait for CSI). Pod recreation restarts the window; container restarts inside the
  same pod do not.
- **Scheduling timeout** — no Agent pod is on the node, **or** (with wait-for-CSI)
  the Agent is Ready but **no** CSI node-server pod is on the node
  yet. Clock: `node.metadata.creationTimestamp`. Covers DaemonSets that never
  schedule onto the node (taint not tolerated, missing labels, CSI still pulling,
  etc.).

A pod-recreation crash-loop faster than the readiness window can hold a node
tainted indefinitely; run with `policy=keep` and alert on
`untaint_taint_timeouts_total{policy="keep"}` to catch this.

The controller removes only this fixed taint and does not add it; both
timeouts are global and cannot be tuned per Node (Group), DDA, or DAP.

## Prerequisites

- Operator v1.x+
- Tested on Kubernetes 1.27.0+

## Enable the Untaint controller

The Untaint controller is disabled by default. Enable it on the operator
manager:

```yaml
args:
  - --untaintControllerEnabled=true
  # Optional: require CSI node-server Ready before untainting (see Overview).
  - --untaintControllerWaitForCSIDriver=true
```

| `--untaintControllerEnabled` | `--untaintControllerWaitForCSIDriver` | Behavior |
| ----------------------------- | ------------------------------------- | -------- |
| `false` | any | Untaint controller off; no Agent startup toleration for this feature. |
| `true` | `false` | Agent-only readiness; Agent DaemonSet startup toleration injected. |
| `true` | `true` | Wait for Agent **and** CSI node-server Ready; widened Pod cache (agent + `DD_CSIDRIVER_WATCH_NAMESPACE` namespaces); startup toleration on Agent and, when the DatadogCSIDriver controller is enabled, on the CSI node DaemonSet. |

`--untaintControllerWaitForCSIDriver` requires `--untaintControllerEnabled=true` (the operator exits on invalid combinations).

When `--untaintControllerEnabled` is enabled, the operator injects a toleration for
`agent.datadoghq.com/not-ready=presence:NoSchedule` into the node Agent
DaemonSet (or ExtendedDaemonSet) pod template, unless an equivalent toleration
is already present. When **`--untaintControllerWaitForCSIDriver`** is also true **and**
the DatadogCSIDriver controller is running (`--datadogCSIDriverEnabled=true`), the same
toleration is injected into the **Datadog CSI node-server** DaemonSet pod
template so the CSI workload can schedule on tainted nodes before the taint is
removed.

## Configuration

All other tuning knobs are environment variables on the operator pod. Values
use Go's `time.ParseDuration` format (`90s`, `5m`, `1h`, etc.). Invalid values
fail the controller startup with an ERROR log; other controllers continue to
start normally.


| Env var                                    | Default  | Description                                                                                                                                                                                                                                                                                         |
| ------------------------------------------ | -------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `DD_UNTAINT_CONTROLLER_TIMEOUT`            | `10m`    | Readiness timeout. Tune to the upper bound of legitimate agent startup on your nodes; 2–5m is often enough on clusters with cached images.                                                                                                                                                          |
| `DD_UNTAINT_CONTROLLER_SCHEDULING_TIMEOUT` | `5m`     | Scheduling timeout. Set larger than your scheduler retry window; raise it on clusters with large pending queues or aggressive autoscaling.                                                                                                                                                          |
| `DD_UNTAINT_CONTROLLER_TIMEOUT_POLICY`     | `remove` | Action when a timeout fires. `remove` untaints the node anyway (favors scheduling availability over telemetry; lowest operational risk). `keep` leaves the taint in place and emits a Warning event (favors telemetry; pair with an alert on the timeout counter to surface stuck nodes). |
| `DD_UNTAINT_CONTROLLER_EVENTS_ENABLED`     | `false`  | Emit Kubernetes Events on Nodes for taint removals and timeout decisions.                                                                                                                                                                                                                           |

## Observability

Metrics, under the `untaint` Prometheus subsystem:

- `untaint_taint_removals_total` — counter, every taint removal regardless of cause.
- `untaint_taint_removal_latency_seconds` — histogram, time between pod Ready and taint removal.
- `untaint_taint_timeouts_total{reason, policy}` — counter, timeout decisions. `reason` in {`readiness`, `scheduling`}; `policy` in {`remove`, `keep`}. Alert on `policy="keep"` to investigate stuck nodes.

Kubernetes Events (gated by `DD_UNTAINT_CONTROLLER_EVENTS_ENABLED=true`):

- `TaintRemoved` (Normal) — taint removed after the Agent became Ready, or (when
  `--untaintControllerWaitForCSIDriver` is enabled) after both the Agent and
  CSI node-server pods became Ready.
- `UntaintTimeout` — a timeout fired. Normal under `remove`, Warning under `keep`. Message carries the reason, elapsed time, and policy.

