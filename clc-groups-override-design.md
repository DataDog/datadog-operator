# CLC groups in `spec.override`: design

Decision: keep `DatadogAgentComponentOverride` as the map value. Move its fields into an embedded `DatadogAgentComponentOverrideConfig` and add `groups` next to them. The CRD stays backward compatible. The Go API change is limited to struct literals.

## Type

```go
type DatadogAgentSpec struct {
    // +kubebuilder:validation:XValidation:rule="!has(self.nodeAgent) || !has(self.nodeAgent.groups)",message="groups is only supported for clusterChecksRunner"
    // (same rule for clusterAgent and otelAgentGateway)
    Override map[ComponentName]*DatadogAgentComponentOverride `json:"override,omitempty"`  // unchanged
}

type DatadogAgentComponentOverride struct {
    DatadogAgentComponentOverrideConfig `json:",inline"`
    // +kubebuilder:validation:MaxProperties=10
    Groups map[string]*DatadogAgentComponentOverrideConfig `json:"groups,omitempty"`
}

type DatadogAgentComponentOverrideConfig struct { /* former DatadogAgentComponentOverride fields */ }
```

Go impact: map types, map reads (`o.Image` is promoted) and `func(*DatadogAgentComponentOverride)` signatures are unchanged. Struct literals that set fields must wrap them:
`&DatadogAgentComponentOverride{DatadogAgentComponentOverrideConfig: DatadogAgentComponentOverrideConfig{Image: ...}}` (rewritten in 51 files, mostly tests).
Reflection over the override (DAP allowlist validation and its test) now walks into the embedded struct.

YAML: see `examples/datadogagent/datadog-agent-with-clusterchecksrunner-groups.yaml`.

## Verified (kind, k8s v1.37, controller-gen v0.17.3)

| Check | Result |
|---|---|
| Existing `override` schema (DDA, DDAI, DAP) with descriptions stripped | identical |
| Rest of each CRD | identical |
| `groups` value schema == `DatadogAgentComponentOverride` schema | identical |
| Install current CRD, create legacy CR (`clusterChecksRunner` + `nodeAgent`), upgrade CRD, update the CR | accepted, round-trips unchanged |
| CR with `clusterChecksRunner.groups` | accepted, stored as written |
| `nodeAgent.groups` | rejected: "groups is only supported for clusterChecksRunner" |
| 11 groups | rejected by `maxProperties` |
| repo build, `go vet ./...`, `go test ./...` (root + api), `make lint` | pass |

Findings that shaped the markers:
- `self.all(k, ...)` over `override` **is over the CEL cost budget (1.6×)**. The `override` map has no `maxProperties`, and adding one would tighten the existing CRD. Fixed-key rules are constant cost and are accepted.
- A group-name regex on `groups` **is over budget (15.9×)** because it is nested in the unbounded `override` map. Group names (DNS label, length) must be validated in Go (`datadogagent_validation.go`, reported as a condition). `Bad_Name` is currently accepted by the API server.
- An older CRD with server-side apply rejects `groups` ("field not declared in schema"). Client-side apply prunes it silently. This matters for operator/CRD downgrades.
- CRD size: DDA goes from 442 KB to 632 KB (compact JSON) because the override schema is duplicated. That is under the ~1.5 MiB etcd limit, but every future field on `DatadogAgentComponentOverride` now costs twice.

## Semantics

- **Default CLC**: `clusterChecksRunner` without groups behaves exactly as today.
- **Effective group override** = merge(base, group). Not inherited: `name` (a group gets `<dda>-cluster-checks-runner-<group>` unless it sets `name`).
- **`disabled`**: inherited like any other field. Base `disabled: true` disables the default deployment and all groups. A group can re-enable itself with `disabled: false`. A dedicated knob can be added later without a break.
- **Status**: keep `status.clusterChecksRunner` and add `status.clusterChecksRunnerGroups map[string]*DeploymentStatus` (additive).

## Remaining decision: merge semantics for lists

| | JSON merge patch (RFC 7386) | Typed merge using CRD list markers (same as DAP profiles via `ssaMergeCRD`) |
|---|---|---|
| maps (`labels`, `nodeSelector`, `containers`) | merged by key | merged by key |
| `env`, `volumes`, `topologySpreadConstraints` (`listType=map`) | **replaced** | merged by key |
| `tolerations`, `args` (`listType=atomic`) | replaced | replaced |
| cost | trivial, in-process | dry-run API call per group, or in-process structured-merge-diff with the CRD schema |

Recommendation: typed merge, for consistency with profiles. With JSON merge, setting one env var on a group would silently drop the common env vars.
A:

## Implementation checklist

1. Types + markers (patch), `make generate manifests`, docs regen.
2. ~~Mechanical Go migration~~ done (struct literal wrapping).
3. Go validation: group names, `name` collisions. `groups` is already rejected in DAP because it is not on the allowlist.
4. DDAI CLC component: reconcile the default deployment plus one deployment per group from the effective override. Delete orphaned group deployments.
5. Per-group objects: SA/RBAC, PDB, NetworkPolicy (`component/objects/network.go`), DCA token checksum annotation (`ddai_utils.go:50`), the `clusterchecks` / `kubernetesstatecore` features.
6. Status map, plus e2e/kind test of the upgrade path.
