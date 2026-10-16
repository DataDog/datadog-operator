# Composable node Agent DaemonSet POC: architecture options

Two designs were drafted; both share the same core and differ in how much structure they add.

## Shared core (both designs)

- **Annotation.** `experimental.agent.datadoghq.com/node-agent-mode: standalone` is read from the object passed to `Configure`, which is the DDAI. Any other value means default mode.
- **Mode-aware features adapt in `Configure`:**
  - `enabledefault`: in default mode it now explicitly contributes `agent` (core) and keeps requiring the DCA. In standalone mode it contributes no containers and no DCA.
  - `otelcollector` requests `[otel-agent]` and `hostprofiler` requests `[host-profiler]`. Each gets a `standalone` flag and skips the core IPC / core-config env in `ManageNodeAgent`. That is the seam for the v1 runtime config.
- **Per-feature enforcement in `BuildFeatures`, before merge.** A feature is dropped when its requirements still ask for:
  - an agent container other than otel-agent or host-profiler;
  - the DCA or CLC;
  - the Agent with no containers (an implicit core request).

  A dropped feature is not added to the enabled/configured lists and is not merged. As a result it gets no `ManageDependencies` and no `Manage*` calls. Enabled drops are reported; configured-only drops (e.g. `remoteconfig`) are silent.
- **After the merge in standalone mode:** `ClusterAgent` and `ClusterChecksRunner` are left not-required, so the registry cleans them up. The single-container rewrite is skipped.
- **Builder.** The core Agent is rendered only when it is in `Containers`, always first, so default output stays byte-identical. Without the core Agent:
  - `init-config` is not added;
  - `init-volume` uses the ddot image;
  - volumes are only logs, config, auth (emptyDir), tmp and run;
  - otel-agent and host-profiler use standalone variants with no `--core-config`/`--sync-delay`, no `DD_CLUSTER_AGENT_*` env, and auth mounted RW.
- **Override fix.** `override/podtemplatespec.go:84-92` would otherwise give a ddot `init-volume` an agent or `-jmx` image. Ddot-based init containers get the same JMX-stripped override as the otel-agent container.
- **DDAI condition.** A new condition, `NodeAgentModeFeaturesDropped`, lists the dropped enabled features, and a warning event is emitted. Reconciliation is not blocked.
- **Empty standalone pod.** The DaemonSet is deleted, an error is logged and an event is emitted. No condition is set, because `reconcile.go` overwrites `AgentReconcile` with True afterwards.
- **DDA controller.** In standalone mode it skips the DCA Service and the per-DDAI shared local Services. It keeps credentials, install-info, the DCA token and APM telemetry. Store cleanup removes the stale Services, and switching back recreates them.

## Option A: pragmatic

- **Mode helper.** `feature/nodeagentmode.go` holds `NodeAgentModeFromObject`, `IsStandalone` and `AllowsNodeAgentContainer`. The policy is an `if standalone` check.
- **`BuildFeatures` signature unchanged.** A new `BuildFeaturesWithReport` returns a `BuildResult` struct, and `BuildFeatures` becomes a wrapper around it.
- **Builder.** A `hasCore` flag is passed to `agentOptimizedContainers`, `initContainers` and `volumesForAgent`. The standalone path returns a fixed short volume list. The existing `switch` stays.
- **`global/agent.go`.** Unchanged. In standalone, the kubelet CA and runtime-socket volumes are added but not mounted, which is cosmetic.
- **DDA controller** calls `feature.NodeAgentModeFromObject(dda)`.
- **Pros:** fewest lines and the smallest risk to byte-identity.
- **Cons:** each future mode needs another `if`, and the mode semantics are spread across the factory, the builder and the controllers.

## Option B: clean

- **New leaf package `datadogagent/composition`** holding a **policy table**:
  ```go
  type Policy struct {
      Mode                NodeAgentMode
      nodeContainers      set[AgentContainerName] // nil = unrestricted
      ClusterAgent, ClusterChecksRunner, OtelAgentGateway, SharedLocalServices bool
  }
  var policies = map[NodeAgentMode]Policy{ ModeDefault: {...all true}, ModeStandalone: {otel-agent, host-profiler; gateway only} }
  func PolicyOf(obj metav1.Object) Policy
  func (p Policy) NodeContainers(ideal ...AgentContainerName) []AgentContainerName // order-preserving trim
  ```
  - Features declare their ideal list, e.g. `policy.NodeContainers(core, otel-agent)`, and the policy trims it.
  - Controllers read capabilities such as `policy.ClusterAgent` and `policy.SharedLocalServices` instead of switching on mode names.
  - The package is importable from `feature`, `global`, the DDA and DDAI controllers and the builder without import cycles.
- **`BuildFeatures` gets a 5th return value, `[]DroppedFeature{ID, Reasons}`.** There are only 3 call sites.
- **Container registry in the builder** (`component/agent/registry.go`):
  ```go
  type containerSpec struct {
      build             func(*podBuild) corev1.Container
      initContainers    func(*podBuild) []corev1.Container
      volumes           func(*podBuild) []corev1.Volume
      needsCoreScaffold bool // false only for otel-agent, host-profiler
  }
  ```
  - It replaces the `switch` and the inline system-probe init and volume special cases.
  - Ordering is deterministic: core first, then the required list. Volumes are de-duplicated by name, first one wins.
- **`global/agent.go`.** The kubelet CA and runtime-socket volumes are added only when core is present.
- **`global/global.go`.** The Helm-migration `DD_CLUSTER_AGENT_URL` is gated on `policy.ClusterAgent`.
- **Pros:**
  - A new mode or combination is one row in the table.
  - The DAP extension is just "the profile DDAI carries the annotation", because the shared-services check is already per DDAI.
  - Containers own their scaffolding.
- **Cons:**
  - More churn: the builder refactor touches system-probe paths.
  - More test fixtures need `core` added explicitly.
  - Byte-identity must be proven at each step.

## Comparison

| | A pragmatic | B clean |
|---|---|---|
| Lines and churn | small | medium |
| Adding a future mode (e.g. host-profiler-only variants, a DAP-scoped mode) | another `if` in each of 4 places | one table row |
| Byte-identity risk | low | medium; mitigated by doing the registry refactor before any behavior change and checking goldens |
| Builder | `hasCore` flag plus a `switch` | container registry |
| Cosmetic leftovers in standalone | unmounted kubelet CA and runtime volumes | none |

## Recommendation: B's policy layer with a lighter builder

Take **Option B's `composition` policy table, the 5th `BuildFeatures` return value, and the `global` / `override` / DDA-controller gating.** This is the part that makes the design "correct" and ready for DAP: mode semantics live in one place, and every consumer asks the policy for a capability.

For the **builder, use a light version of the registry**:
- Each container gets a `containerSpec{build, needsCoreScaffold}`, and the `switch` goes away.
- Core scaffolding (init-config and the base volumes) is used only when core is present.
- The system-probe init and volume special cases stay as they are for now.

That keeps the byte-identity risk close to Option A's while still making the container set data-driven. The full per-container volumes/init registry can come once host-profiler or system-probe composition actually needs it.

Implementation order:
1. The `composition` package.
2. `BuildFeatures` enforcement and the condition.
3. The mode-aware features. Check the goldens.
4. The builder refactor with no behavior change. Check the goldens.
5. The standalone builder variants and the override fix.
6. The `global` gating.
7. The DDAI empty-pod handling.
8. The DDA controller gating.
9. The standalone goldens: DDOT only, host-profiler only, and both. Then `make lint` and the tests.

> Answer (A / B / recommended hybrid / changes): B
