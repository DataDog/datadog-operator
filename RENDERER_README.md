# Render Subcommand

The `render` subcommand generates the Kubernetes manifests that the operator would create for a given `DatadogAgent` resource, without connecting to a cluster.

## 1. Running Offline

### From a compiled binary

```bash
# Build the manager binary
make build

# Render manifests for a DatadogAgent spec file
./bin/darwin-arm64/manager render -f path/to/dda.yaml

# Output to a file
./bin/darwin-arm64/manager render -f path/to/dda.yaml > manifests.yaml
```

### From Docker

```bash
docker run --rm \
  -v $(pwd)/dda.yaml:/tmp/dda.yaml \
  gcr.io/datadoghq/operator:latest \
  render -f /tmp/dda.yaml
```

### Available flags

| Flag | Default | Description |
|------|---------|-------------|
| `-f` | (required) | Path to the DatadogAgent YAML file |
| `-datadogAgentInternalEnabled` | `true` | Use the v3/DDAI reconciliation path (matches operator default) |
| `-datadogAgentProfileEnabled` | `false` | Enable DatadogAgentProfile support |
| `-supportExtendedDaemonset` | `false` | Use ExtendedDaemonSet instead of DaemonSet |
| `-supportCilium` | `false` | Enable Cilium network policy support |

### Example: minimal DDA

```bash
cat <<'EOF' > dda.yaml
apiVersion: datadoghq.com/v2alpha1
kind: DatadogAgent
metadata:
  name: datadog
  namespace: datadog
spec:
  global:
    clusterName: my-cluster
    credentials:
      apiKey: "0123456789abcdef0123456789abcdef"
EOF

./bin/darwin-arm64/manager render -f dda.yaml
```

The output is a multi-document YAML stream containing all generated resources: DaemonSet, Deployments, ServiceAccounts, ClusterRoles, and more.

---

## 2. Collecting Golden Files from a Live Cluster

Golden files capture what the operator actually creates on a running cluster. They are used by `TestRenderManifests_MatchesCluster` to verify that render output matches real operator behavior.

### Prerequisites

- A running Kubernetes cluster (e.g., kind) with the operator deployed
- `kubectl` configured to point at that cluster

### Steps

**1. Deploy the operator**

```bash
make deploy IMG=<your-image>
```

**2. Apply a DatadogAgent spec**

```bash
kubectl apply -f internal/controller/datadogagent/render/testdata/inputs/minimal.yaml
# or
kubectl apply -f internal/controller/datadogagent/render/testdata/inputs/all-features.yaml
```

**3. Wait for reconciliation**

```bash
kubectl get datadogagent -n datadog -w
# Wait until STATUS shows "Running" or similar
```

**4. Collect resources into golden files**

```bash
# For minimal
go run ./internal/controller/datadogagent/render/cmd/collect/ \
  -dda-name datadog \
  -namespace datadog \
  -output-dir internal/controller/datadogagent/render/testdata/golden/minimal

# For all-features
go run ./internal/controller/datadogagent/render/cmd/collect/ \
  -dda-name datadog \
  -namespace datadog \
  -output-dir internal/controller/datadogagent/render/testdata/golden/all-features
```

### What gets collected

The tool collects all resources matching the label `app.kubernetes.io/managed-by=datadog-operator`:

- **Namespaced:** DaemonSets, Deployments, ServiceAccounts, Services, Secrets, ConfigMaps, Roles, RoleBindings, NetworkPolicies, PodDisruptionBudgets
- **Cluster-scoped** (also filtered by `app.kubernetes.io/part-of=<dda-name>`): ClusterRoles, ClusterRoleBindings, APIServices, MutatingWebhookConfigurations, ValidatingWebhookConfigurations
- **DatadogAgentInternal** objects owned by the DDA

### Normalization applied

Before writing golden files, the tool strips fields that are runtime-specific and not produced by render:

- `resourceVersion`, `uid`, `creationTimestamp`, `managedFields`, `generation`, `ownerReferences`
- `kubectl.kubernetes.io/last-applied-configuration` annotation
- `status` field

### Collect tool flags

| Flag | Default | Description |
|------|---------|-------------|
| `-dda-name` | (required) | Name of the DatadogAgent resource |
| `-output-dir` | (required) | Directory to write golden YAML files |
| `-namespace` | `datadog` | Namespace where the DDA lives |
| `-kubeconfig` | `~/.kube/config` | Path to kubeconfig |

### Updating golden files

Re-run the collect command to overwrite existing golden files. Commit the updated files alongside any operator changes that affect rendered output.

---

## 3. Running the Diff Test

Once golden files are populated, the comparison test verifies that render output matches what the operator creates on a real cluster.

### Run the test

```bash
go test ./internal/controller/datadogagent/render/ \
  -run TestRenderManifests_MatchesCluster \
  -v
```

### What the test does

For each test case (`minimal`, `all-features`):

1. Loads the DDA spec from `testdata/inputs/`
2. Calls `RenderManifests` with the same options the operator uses (v3/DDAI path enabled)
3. Loads all golden files from `testdata/golden/<name>/`
4. Compares each golden file against the corresponding rendered object using a Kubernetes-aware diff (handles environment variable and volume ordering)

The check is **bidirectional**:
- Every golden file must have a matching rendered object (catches resources the renderer stopped producing)
- Every rendered object must have a golden file (catches new resources the renderer produces that weren't validated against a cluster)

### Skipping behavior

If the golden directory is empty or missing, the test skips gracefully with a message indicating that collection is needed. This allows the test to run in CI without requiring a live cluster.

```
--- SKIP: TestRenderManifests_MatchesCluster/minimal
    render_golden_test.go:51: no golden files at testdata/golden/minimal; run collection script first
```

### Workflow for operator changes

When a change affects rendered manifests:

1. Make the code change
2. Deploy to a kind cluster and reconcile
3. Re-collect golden files with the `collect` tool
4. Run the diff test to confirm render matches cluster
5. Commit both the code change and updated golden files
