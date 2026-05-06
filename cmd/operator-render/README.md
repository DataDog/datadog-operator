# operator-render

`operator-render` is a CLI tool that simulates the Datadog Operator's reconciliation loop offline. Given a `DatadogAgent` (DDA) and optional `DatadogAgentProfile` (DAP) manifests, it produces the complete set of Kubernetes resources the operator would create — without needing a running cluster.

This binary is a thin wrapper over the [`internal/controller/testutils/renderer`](../../internal/controller/testutils/renderer) package. Tests can import the package directly to render resources from in-memory DDA fixtures (see `renderer.Render(renderer.Options{...})`).

## How it works

The tool reuses the operator's own reconciler code via `controller-runtime`'s fake client:

1. **Builds a fake Kubernetes client** pre-populated with the provided DDA, DAPs, and the `DatadogAgentInternal` CRD loaded from `config/crd/bases/v1/`.
2. **Runs the DDA reconciler twice**: the first pass adds the finalizer (as it would in a real cluster); the second pass does the actual work and creates `DatadogAgentInternal` (DDAI) objects.
3. **Runs the DDAI reconciler** once for each DDAI, which generates all workload and dependency resources (DaemonSets, Deployments, RBAC, Services, Secrets, ConfigMaps, etc.).
4. **Collects and serializes** all resources written to the fake client, strips non-deterministic metadata fields (UID, resourceVersion, etc.), sorts by kind and name, and writes YAML or JSON.

Because it runs the real reconciler code, output changes whenever the operator logic changes — making it a reliable building block for golden-file regression tests.

## Build

```bash
make build-renderer   # just this binary
make build            # all binaries (manager, kubectl-datadog, operator-render)
```

The binary is written to `bin/<platform>/operator-render`.

> **Note**: the binary loads the DDAI CRD from `config/crd/bases/v1/` at runtime; the path is baked in at compile time via `runtime.Caller`. The binary therefore only works when run from a checkout of the source tree it was built from, which is fine for golden-file tests and dev use.

## CLI reference

```
Usage:
  operator-render --dda <file> [flags]

Flags:
  --dda <file>          Path to DatadogAgent YAML file (required)
  --dap <file>          Path to DatadogAgentProfile YAML file (repeatable)
  --profiles-enabled    Enable DatadogAgentProfile reconciliation (independent of --dap inputs)
  --output <file>       Write output to file instead of stdout
  --format yaml|json    Output format (default: yaml)
  --support-cilium      Emit CiliumNetworkPolicy resources in addition to NetworkPolicy
```

## Example usage

### Basic — render a DatadogAgent

```bash
operator-render --dda my-dda.yaml
```

### With agent profiles

```bash
operator-render --dda my-dda.yaml \
  --profiles-enabled \
  --dap profiles/linux.yaml \
  --dap profiles/windows.yaml
```

`--profiles-enabled` is required for DAPs to take effect — it mirrors the operator's own `--datadogAgentProfileEnabled` flag. Each DAP produces an additional DaemonSet scoped to the nodes matching the profile's affinity rules.

### Write to a file

```bash
operator-render --dda my-dda.yaml --output rendered.yaml
```

### JSON output

```bash
operator-render --dda my-dda.yaml --format json --output rendered.json
```

JSON output is a top-level array of objects with alphabetically sorted keys, suitable for `jq` and structured diffs.

### Cilium network policies

```bash
operator-render --dda my-dda.yaml --support-cilium
```

Adds `CiliumNetworkPolicy` resources alongside the standard `NetworkPolicy` objects.

### Golden-file regression test

```bash
# Generate baseline
operator-render --dda config/samples/datadoghq_v2alpha1_datadogagent.yaml \
  --output testdata/golden.yaml

# After an operator change, compare
operator-render --dda config/samples/datadoghq_v2alpha1_datadogagent.yaml | \
  diff testdata/golden.yaml -
```

## Output

Resources are emitted in dependency order: RBAC first, then config, then workloads.

| Order | Kinds |
|-------|-------|
| 1 | ServiceAccount |
| 2 | ClusterRole |
| 3 | ClusterRoleBinding |
| 4 | Role |
| 5 | RoleBinding |
| 6 | Secret |
| 7 | ConfigMap |
| 8 | Service |
| 9 | NetworkPolicy, CiliumNetworkPolicy |
| 10 | PodDisruptionBudget |
| 11 | APIService |
| 12 | MutatingWebhookConfiguration |
| 13 | ValidatingWebhookConfiguration |
| 14 | DatadogAgentInternal |
| 15 | DaemonSet |
| 16 | Deployment |

Within each kind, resources are sorted alphabetically by `namespace/name`.

The top-level `status` stanza is stripped from every resource (always server-side runtime state, never meaningful in a rendered manifest).

The following metadata fields are also stripped for stable output:

- `resourceVersion`
- `uid`
- `generation`
- `creationTimestamp`
- `managedFields`
