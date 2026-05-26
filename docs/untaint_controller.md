# Untaint Controller

This feature was introduced in Datadog Operator v1.x and is currently in beta.

## Overview

The Untaint controller watches Kubernetes Nodes carrying the taint
`agent.datadoghq.com/not-ready=presence:NoSchedule` and removes it once the
Datadog Agent pod on that node is `Ready`. It is intended to run alongside a
separate mechanism (cluster-autoscaler hook, CCM, admission webhook, etc.)
that adds the taint to new nodes. The use case is keeping workloads off a
node until the Datadog Agent is Ready, and recovering gracefully if the Agent never
becomes Ready.

Agent pods are matched by the label `agent.datadoghq.com/component=agent` in
the operator's watched namespaces (`WATCH_NAMESPACE` /
`DD_AGENT_WATCH_NAMESPACE`).

If the Agent pod never reaches Ready on a tainted node, a configurable timeout
policy ensures the node is never permanently unschedulable. Two clocks cover
the two failure modes:

- **Readiness timeout** — the Agent pod is on the node but not Ready. Clock:
  `pod.Status.StartTime`. Pod recreation restarts the window; container
  restarts inside the same pod do not.
- **Scheduling timeout** — no Agent pod is on the node. Clock:
  `node.metadata.creationTimestamp`. The expected path when a DaemonSet never
  schedules a pod onto the node (taint not tolerated, missing labels, etc.).

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
```

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

- `TaintRemoved` (Normal) — taint removed because the Agent pod became Ready.
- `UntaintTimeout` — a timeout fired. Normal under `remove`, Warning under `keep`. Message carries the reason, elapsed time, and policy.

