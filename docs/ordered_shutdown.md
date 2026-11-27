# Ordered shutdown of the node Agent pod

This feature is experimental. It is gated by an experimental annotation and may
change or be removed.

## Overview

When a node Agent pod is replaced (rollout, drain, eviction), every container
receives `SIGTERM` at the same time. The core Agent, DogStatsD and the
trace-agent stop collecting within seconds, but the replacement pod can't start
until the slowest container, often `system-probe`, has finished its teardown,
which can take 30 seconds or more. A slow `system-probe` shutdown therefore
lengthens the collection gap of every other container.

With ordered shutdown, `system-probe` stops first, and the other containers keep
running until it has exited (or a fixed budget runs out). Only then do they
receive `SIGTERM`. The per-node collection gap of the core Agent becomes roughly
its own shutdown plus the new pod's startup, however long `system-probe` takes,
up to the budget.

## Enabling

Add the annotation to the `DatadogAgent`:

```yaml
apiVersion: datadoghq.com/v2alpha1
kind: DatadogAgent
metadata:
  name: datadog
  annotations:
    experimental.agent.datadoghq.com/ordered-shutdown: "true"
```

Any other value, or no annotation, leaves the node Agent DaemonSet unchanged.
Enabling or disabling it changes the pod template, so it triggers one rolling
restart of the node Agents.

## What changes in the pod

It applies to Linux node Agent pods that run a separate `system-probe`
container (any feature that needs `system-probe`, such as NPM, USM, CWS or
service discovery).

- `system-probe` runs through a small shell wrapper that holds an exclusive
  `flock` on `/var/run/sysprobe/.running.lock` (in the `sysprobe-socket-dir`
  volume) for as long as it runs. `system-probe` stays PID 1 and receives
  signals as before.
- Every other container gets a `preStop` hook that waits for a shared lock on
  the same file, which it only gets once `system-probe` has exited. The kubelet
  doesn't send `SIGTERM` to a container until its `preStop` hook returns. Each
  of these containers also mounts `sysprobe-socket-dir` read-only, if it doesn't
  already mount it.
- `terminationGracePeriodSeconds` is raised to at least 75 seconds: up to 60
  seconds to wait for `system-probe`, plus 15 seconds for the other containers
  to shut down. A longer grace period that is already set is kept.

It fails open: a missing lock file, a timeout, or a killed `system-probe` all let
`SIGTERM` through, as without the annotation.

## When it doesn't apply

The pod template is left untouched, and the Operator logs why at verbosity 1,
when:

- the pod has no `system-probe` container (no feature needs it, or the
  single-container strategy is in use);
- the `system-probe` container's `command` or `args` are overridden in
  `spec.override.nodeAgent.containers.system-probe`;
- the `system-probe` container mounts something other than `sysprobe-socket-dir`
  read-write at `/var/run/sysprobe`;
- the node Agent targets Windows nodes through a `DatadogAgentProfile`;
- GKE Autopilot handling is enabled, because its WorkloadAllowlist pins the
  container commands.

A container that already has its own `preStop` hook keeps it and doesn't wait
for `system-probe`.
