# RFC: Prepared per-node Agent rollouts

- Status: Draft
- Last updated: 2026-07-22
- Owners: Agent and Datadog Operator

## Decision

Prototype native DaemonSet surge with an Agent **Prepared** state and an
Operator-controlled, one-node-at-a-time handoff.

The replacement Pod is scheduled, its images and init containers complete, and
the real Agent processes start before the old Pod is terminated. Prepared
processes do not bind shared ports or UDS paths, start active collectors, mutate
shared log state, or acquire exclusive kernel resources. A failed pull, init, or
preparation therefore leaves the old Agent running unless the user-enabled
resource fallback has already deleted it.

This phase removes the dominant pull/init/process-start delay. It does not claim
strict zero-gap handoff: releasing an old listener or collector and activating
its replacement leaves a smaller residual interval. If strict endpoint
continuity is required, the preferred extension is a stable node-local endpoint
holder or socket file-descriptor handoff.

## Why this is needed

The current DaemonSet rollout deletes the old Agent before scheduling, pulling,
initializing, and starting its replacement. A slow or failed pull can leave a
node without an Agent indefinitely. A slow system-probe teardown can also keep
the whole old Pod Terminating after sibling containers exit.

Native `maxSurge` gives us create-before-delete, but production Agent Pods have
three overlap constraints:

- With `hostNetwork: true`, Kubernetes defaults every declared `containerPort`
  to the same `hostPort`; the scheduler rejects the second Pod.
- Both Pods mount the same UDS and host paths. A second active process can bind
  or unlink the socket, duplicate log/check collection, or contend for kernel
  resources.
- The scheduler accounts both Pods' CPU and memory requests.

The design keeps `hostNetwork: true`. The Operator removes all regular and init
container `ports:` declarations from an explicitly compatible template; Agent
Prepared mode prevents runtime binds until activation. Shared hostPath mounting
itself is not the UDS conflict—the bind/unlink behavior is.

## Proposed lifecycle

1. Enabling the experiment first renders an **arm** revision with `maxSurge: 0`.
   It installs the lock-aware Agent, state files, exec probes, and narrowed
   profile anti-affinity through a conventional rollout. This bootstrap is
   required because a legacy Agent does not hold the ownership lock.
2. Once the exact arm template is fully updated, Ready, and Available, the
   Operator renders a **standby** revision. It keeps `hostNetwork: true`, strips
   PodSpec port declarations, sets `maxUnavailable: 0`, and derives `maxSurge`
   from the user's existing `maxUnavailable` budget.
3. A replacement constructs the real process graph, writes `prepared` to its
   Pod-private state file, then waits on a per-component advisory `flock` in a
   stable node hostPath. Startup and liveness exec probes accept Prepared;
   readiness accepts only Active.
4. When every supported replacement container is Running and
   `ContainerStatus.Started=true`, the Operator persists a token on that Pod and
   UID-precondition deletes the old Pod. The old processes release their locks
   only after stopping.
5. The replacements acquire the locks, start listeners and collectors, write
   `active`, and become Ready. The token remains charged until that happens.

The handoff gate is necessary because native `maxSurge` bounds preparation, not
termination and activation. Kubernetes frees a surge slot as soon as old-Pod
deletion begins. Without Active acknowledgement, many nodes could enter a long
handoff concurrently even with a small `maxSurge`.

Prepared health must not use HTTP, TCP, or gRPC probes. Both host-network Pods use the
node IP, so a replacement probe can accidentally hit the old Agent and trigger
premature deletion. Exec probes must identify the process inside the container.

The first cluster experiment deliberately supports only the optimized Linux
core and trace containers plus the standard `init-volume` and `init-config`
init containers. It bypasses `trace-loader`, which otherwise binds APM
endpoints before the trace process can wait. Emissary, process/system/security
agents, OTel, host profiler, and other sidecars are rejected or disabled until
their pre-Prepared side effects are audited.

## Capacity policy

The correct default is honest double requests during overlap. Assigning Agent
requests to a different "port-holder" Pod is not request transfer: scheduling,
QoS, CPU shares, memory protection, and eviction accounting apply to the holder's
cgroup, leaving the Agent under-requested.

For constrained nodes, retain the PoC's resource-fit classification behind the
separate `experimental.agent.datadoghq.com/resource-fallback` opt-in. The
Operator may delete an old Pod only when the
replacement is Pending solely for CPU or memory, the node is healthy, supported
scheduling constraints are revalidated, removing that exact old Pod appears to
make the replacement fit, and the Operator first persists a handoff token.

Normal approved-but-not-Active handoffs and fallback reserved/deleted-but-not-
Active nodes share one ledger and must not exceed the configured
`maxUnavailable`. A fallback token is reserved before the UID-preconditioned
delete and released only after the replacement reports Active.

This fallback is best effort, not proof. Another Pod can take the freed capacity
before the Agent schedules, leaving no old Agent and a still-Pending replacement.
The replacement can also encounter a later pull or init failure after the old
Agent is gone.
Clusters requiring deterministic headroom can run a separate low-priority
Agent-sized placeholder on each node, at the cost of reserving overlap capacity
continuously.

## Alternatives and trade-offs

| Option | Benefit | Main limitation | Position |
|---|---|---|---|
| Native surge + Prepared Agent + handoff gate | Normal path preserves old through pull/init/start; no permanent data-plane hop | Agent/Operator lifecycle work, temporary double requests, residual activation gap | Lead prototype |
| Stable endpoint-holder DaemonSet | Preserves public ports and UDS inode; can drain/buffer | Tier-0 proxy, protocol/origin fidelity, its own upgrade problem; does not fence logs/system-probe | Prototype if continuity is required |
| Holder also owns Agent requests | Appears to avoid double requests | Requests protect the wrong Pod/cgroup | Reject |
| Per-node request placeholder | Honest deterministic surge headroom | Permanently reserves a second Agent-sized slot; global priority can consume it | Optional capacity policy |
| OpenKruise Standard surge | Partition, pause, node selection, PreDelete hooks | Same hostPort, UDS, request, and handoff-budget problems as native surge | Focused comparison |
| OpenKruise `InPlaceIfPossible` | One Pod; preserves unaffected containers and avoids duplicate requests | Changed container stops before replacement starts; failed pull can leave it down; unsupported changes recreate Pod | Complementary optimization |
| CSI | Can provide per-Pod/shared mount paths | Does not own or transfer a live UDS socket, bind ports, or select an active Agent | Not a solution alone |
| Service/CNI/eBPF indirection | Can move TCP/UDP endpoints off host networking | Changes UDP source/origin semantics and does not solve UDS or collectors | Environment-specific |

OpenKruise should be evaluated as two separate experiments. Standard surge does
not remove the need for Prepared mode, and PreDelete alone does not hold a surge
slot through activation. In-place update is valuable for image-only or
single-container changes, but its image pre-download currently optimizes rather
than gates rollout, and Advanced DaemonSet has no fail-closed `InPlaceOnly` mode.
Single, non-comparable local v1.9.1 runs observed 7.845-second uncached and
4.511-second pre-pulled request gaps; a failed automatic pre-download still let
the selected Pod enter `ImagePullBackOff`.

## Current PoC and required work

The coordinated PoC now has three pieces under development: Agent process
locking/state, Operator arm-to-standby rendering and prepared handoff, and a
single-cluster ops override. The Operator fails closed on unsupported
containers, init containers, commands, lifecycle hooks, operating systems,
anti-affinity, and reserved volume paths. Ordinary native surge is unchanged
when the prepared-rollout annotation is absent, and resource fallback is
separately disabled by default.

Before choosing a public API, validate the two-container pilot with numbered
metrics and traces, failed/slow pulls, failed preparation and activation,
Operator restart, and resource pressure with fallback both off and on. Expand
the allowlist only after logs/process/system-probe side effects are gated; the
full validation still includes a real two-minute system-probe teardown. Then
repeat the leading result on Linux KindVM and an experimental cluster.

## Open decisions

- Exact Prepared boundary and ownership groups for each Agent component.
- Authenticated transport for Prepared, handoff-approved, and Active status.
- Whether the residual activation interval is acceptable or requires a stable
  endpoint holder.
- Whether constrained clusters prefer indefinite stall, explicit best-effort
  fallback, or permanently reserved placeholder capacity.

Detailed failure behavior, protocol notes, alternative analysis, validation
matrix, and primary sources are in the
[investigation appendix](agent_zero_gap_rollouts_rfc_appendix.md). The current
implementation notes are in the
[prepared host-network surge PoC](agent_host_network_surge_poc.md).
