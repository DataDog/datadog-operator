# Prepared host-network Agent surge PoC

See [RFC: Prepared per-node Agent rollouts](agent_zero_gap_rollouts_rfc.md) for
the lifecycle proposal, trade-offs, alternatives, and validation plan.

This PoC keeps `override.nodeAgent.hostNetwork: true` while allowing a native
DaemonSet surge Pod to be scheduled beside the old Agent Pod.

It is explicitly enabled with:

```yaml
metadata:
  annotations:
    experimental.agent.datadoghq.com/host-network-surge-prepared: "true"
spec:
  override:
    nodeAgent:
      hostNetwork: true
      updateStrategy:
        type: RollingUpdate
        rollingUpdate:
          maxSurge: 1
          maxUnavailable: 1
```

The Operator first performs an `arm` rollout with `maxSurge: 0`. Once that
exact revision is fully Available it emits a `standby` template, removes
container port declarations, and changes the native DaemonSet strategy to
`maxUnavailable: 0`. Removing the declarations is necessary because Kubernetes
defaults every declared `containerPort` to the same `hostPort` for a
host-network Pod. The processes can still bind the node ports without PodSpec
port declarations.

For DatadogAgentProfiles, the PoC narrows the standard Pod anti-affinity only
enough to let old and new revisions of the same DDA and profile overlap. Other
profiles and other DDA installations remain excluded.

## Prepared-mode contract

The first pilot fails closed unless the rendered Pod has exactly:

- optimized Linux `agent` and `trace-agent` containers;
- standard `init-volume` and `init-config` init containers;
- `hostNetwork: true` and a RollingUpdate strategy; and
- no custom lifecycle hooks, reserved rollout paths, unsupported anti-affinity,
  or custom commands.

The Operator injects per-component node lock paths and Pod-private state paths,
bypasses `trace-loader`, and replaces network probes with state-file exec
probes. Startup accepts Prepared, waiting liveness accepts Prepared, and
post-activation liveness/readiness delegate to `agent health` or the local APM
listener. Once both containers have `Started=true`, the Operator annotates the
replacement with the old Pod UID and deletes that exact UID within the
`maxUnavailable` budget. The node locks keep the new processes asleep until the
old processes finish stopping.

Emissary and all additional Agent containers must be disabled for this pilot.
CPU/memory fallback is independent and remains off unless
`experimental.agent.datadoghq.com/resource-fallback: "true"` is also set.
