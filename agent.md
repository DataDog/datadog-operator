# Agent Guide: Datadog Operator Repository

**Last Updated:** 2026-01-04
**Repository:** github.com/DataDog/datadog-operator
**Purpose:** Kubernetes Operator for deploying and managing Datadog Agent

## Quick Reference

| Item | Value |
|------|-------|
| Language | Go 1.25.5 |
| Framework | controller-runtime v0.20.4 |
| Main Branch | `main` |
| API Version | v2alpha1 (current), v1alpha1 (deprecated) |
| Kubernetes Support | 1.16+ |
| Entry Point | `cmd/main.go` |

## Repository Purpose

The Datadog Operator is a **Kubernetes Operator** that provides declarative deployment and lifecycle management of the Datadog Agent on Kubernetes clusters. It offers advantages over Helm charts and manual DaemonSets:

- Built-in defaults based on Datadog best practices
- Configuration validation to prevent mistakes
- First-class Kubernetes API resource
- Included in Kubernetes reconciliation loop
- Support for 43+ monitoring features

## Directory Structure

```
datadog-operator/
├── api/                              # Kubernetes API definitions (CRDs)
│   ├── datadoghq/v2alpha1/          # Current API version (PRIMARY)
│   ├── datadoghq/v1alpha1/          # Deprecated API version
│   └── datadoghq/common/            # Shared types
├── cmd/                              # Executables
│   ├── main.go                      # Operator manager (MAIN ENTRY)
│   ├── kubectl-datadog/             # kubectl plugin
│   └── check-operator/              # Health checker
├── internal/controller/              # Controller implementations
│   ├── datadogagent/                # Primary controller (CORE)
│   │   ├── component/               # Agent components (node, cluster, checks)
│   │   ├── feature/                 # 43 feature handlers
│   │   ├── merger/                  # Config merging logic (31 mergers)
│   │   ├── override/                # Resource overrides
│   │   └── controller.go            # Main reconciliation logic
│   ├── datadogagentprofile/         # Agent profiles (beta)
│   ├── datadogmonitor/              # Monitor CRD
│   ├── datadogslo/                  # SLO CRD
│   ├── datadogdashboard/            # Dashboard CRD
│   └── setup.go                     # Controller registration
├── pkg/                              # Shared packages
│   ├── kubernetes/                  # K8s utilities, RBAC
│   ├── config/                      # Configuration management
│   ├── secrets/                     # Secret backends
│   ├── constants/                   # Global constants
│   └── controller/utils/            # Datadog API clients
├── config/                           # Kubernetes manifests (Kustomize)
│   ├── crd/                         # CRD definitions
│   ├── rbac/                        # RBAC configs
│   ├── manager/                     # Operator deployment
│   └── samples/                     # Example CRs
├── test/e2e/                         # End-to-end tests
├── examples/                         # Configuration examples
├── docs/                             # Documentation
├── Makefile                          # Build automation
├── Dockerfile                        # Container build
└── go.mod                            # Go dependencies
```

## Core Components

### 1. Custom Resource Definitions (CRDs)

| CRD | API Version | Status | Purpose |
|-----|-------------|--------|---------|
| **DatadogAgent** | v2alpha1 | Current | Primary resource for agent deployment |
| DatadogAgentInternal | v1alpha1 | Internal | Internal reconciliation state |
| DatadogAgentProfile | v1alpha1 | Beta | Advanced agent configuration profiles |
| DatadogMonitor | v1alpha1 | Stable | Datadog monitor management |
| DatadogSLO | v1alpha1 | Stable | Service Level Objective management |
| DatadogDashboard | v1alpha1 | Stable | Dashboard management |
| DatadogGenericResource | v1alpha1 | Stable | Generic resource creation |

**Location:** `api/datadoghq/v2alpha1/datadogagent_types.go` (primary)

### 2. Controllers

#### Primary Controller: DatadogAgent

**File:** `internal/controller/datadogagent/controller.go`

**Key Methods:**
- `Reconcile()`: Main reconciliation loop
- `reconcileV2()`: v2alpha1 reconciliation logic
- `handleFinalizer()`: Cleanup on deletion

**Reconciliation Flow:**
```
Reconcile()
  → Load DatadogAgent CR
  → Validate configuration
  → Load features (43 feature handlers)
  → Build component manifests (Agent, ClusterAgent, ClusterChecksRunner)
  → Apply/Update Kubernetes resources
  → Update status
```

#### Feature System

**Location:** `internal/controller/datadogagent/feature/`

**Architecture:** Each feature is a self-contained package implementing the `Feature` interface.

**Examples:**
- `apm/` - Application Performance Monitoring
- `cspm/` - Cloud Security Posture Management
- `npm/` - Network Performance Monitoring
- `logcollection/` - Log collection
- `admissioncontroller/` - Admission controller

**Feature Registration:** Features self-register via `init()` functions.

**Interface:**
```go
type Feature interface {
    ID() string
    Configure(dda *DatadogAgent) error
    ManageDependencies(store *Store) error
    ManageClusterAgent(mgrInterface) error
    ManageNodeAgent(mgrInterface) error
}
```

### 3. Component Managers

**Location:** `internal/controller/datadogagent/component/`

**Components:**
- **agent/**: Node Agent (DaemonSet)
- **clusteragent/**: Cluster Agent (Deployment)
- **clusterchecksrunner/**: Cluster Checks Runner (Deployment)

Each component manager handles:
- Manifest generation
- Resource application
- Status updates
- RBAC creation

### 4. Configuration Merging

**Location:** `internal/controller/datadogagent/merger/`

**Purpose:** Merge user configuration with feature defaults

**Pattern:** 31 specialized merger handlers for different resource types
- Example: `merger/daemonset.go`, `merger/deployment.go`

**Flow:**
```
User Config → Feature Defaults → Global Config → Final Manifest
```

### 5. Key Packages

| Package | Purpose | Key Files |
|---------|---------|-----------|
| `pkg/kubernetes` | K8s platform detection, RBAC, object utils | `platform.go`, `rbac/` |
| `pkg/config` | Configuration management | `config.go` |
| `pkg/secrets` | Secret backend integration | `secrets.go` |
| `pkg/controller/utils` | Datadog API client, metrics forwarding | `datadog.go` |
| `pkg/constants` | Global constants | `constants.go` |
| `pkg/images` | Image configuration | `image.go` |

## Common Development Tasks

### Adding a New Feature

1. **Create feature package:**
   ```bash
   mkdir -p internal/controller/datadogagent/feature/myfeature
   ```

2. **Implement Feature interface:**
   ```go
   // myfeature/feature.go
   package myfeature

   import "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"

   func init() {
       feature.Register(feature.MyFeatureIDType, buildMyFeature)
   }

   type myFeature struct {
       // fields
   }

   func (f *myFeature) ID() string { return string(feature.MyFeatureIDType) }
   func (f *myFeature) Configure(dda *v2alpha1.DatadogAgent) error { /* ... */ }
   // Implement other interface methods
   ```

3. **Add feature to API types:**
   - Update `api/datadoghq/v2alpha1/datadogagent_types.go`
   - Add feature configuration struct

4. **Write tests:**
   ```go
   // myfeature/feature_test.go
   var _ = Describe("MyFeature", func() {
       // Ginkgo tests
   })
   ```

5. **Generate manifests:**
   ```bash
   make generate
   make manifests
   ```

### Modifying the DatadogAgent CRD

1. **Edit API types:** `api/datadoghq/v2alpha1/datadogagent_types.go`

2. **Add validation markers:**
   ```go
   // +kubebuilder:validation:Enum=value1;value2
   // +kubebuilder:validation:Optional
   ```

3. **Regenerate CRDs:**
   ```bash
   make generate
   make manifests
   ```

4. **Update conversion webhooks if needed:**
   - Location: `api/datadoghq/v2alpha1/datadogagent_conversion.go`

### Adding Controller Logic

**Main reconciliation:** `internal/controller/datadogagent/controller.go`

**Pattern:**
```go
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // 1. Fetch resource
    // 2. Handle deletion (finalizers)
    // 3. Validate configuration
    // 4. Process features
    // 5. Generate manifests
    // 6. Apply resources
    // 7. Update status
    // 8. Return result
}
```

### Working with Mergers

**Location:** `internal/controller/datadogagent/merger/`

**Example:** Merging DaemonSet configurations
```go
// merger/daemonset.go
func DaemonSetMerger(manager *merge.PodTemplateManager, spec *v2alpha1.DatadogAgentComponentOverride) {
    // Merge logic
}
```

**Key Merger Types:**
- `PodTemplateManager`: Pod-level merging
- `ContainerManager`: Container-level merging
- `VolumeManager`: Volume merging

## Testing

### Running Tests

```bash
# Unit tests
make test

# Unit tests with coverage
make test-coverage

# Integration tests
make integration-tests

# E2E tests
cd test/e2e
make e2e-tests
```

### Test Framework

**Framework:** Ginkgo + Gomega

**Example:**
```go
var _ = Describe("DatadogAgent Controller", func() {
    Context("When reconciling a resource", func() {
        It("Should create DaemonSet", func() {
            // Test logic
            Expect(err).NotTo(HaveOccurred())
        })
    })
})
```

### Test Utilities

**Location:** `internal/controller/datadogagent/testutils/`

**Available helpers:**
- `NewDatadogAgent()`: Create test DDA
- `NewDeployment()`: Create test Deployment
- Mock clients and interfaces

## Build and Deployment

### Local Development

```bash
# Install dependencies
go mod download

# Format code
make fmt

# Run linter
make vet

# Build operator
make build

# Run locally (requires kubeconfig)
make run
```

### Building Container Image

```bash
# Build image
make docker-build IMG=<your-registry>/datadog-operator:tag

# Push image
make docker-push IMG=<your-registry>/datadog-operator:tag
```

### Deploying to Cluster

```bash
# Install CRDs
make install

# Deploy operator
make deploy IMG=<your-registry>/datadog-operator:tag

# Uninstall
make undeploy
```

### Creating a Release

See `RELEASING.md` for full release process.

## Important Patterns and Conventions

### 1. Feature-Driven Architecture

All monitoring capabilities are implemented as **pluggable features** that self-register at initialization.

### 2. Component-Based Design

Three main components, each with dedicated managers:
- **NodeAgent**: DaemonSet for node-level monitoring
- **ClusterAgent**: Deployment for cluster-level operations
- **ClusterChecksRunner**: Deployment for distributed checks

### 3. Configuration Override System

Users can override generated configurations at multiple levels:
```yaml
apiVersion: datadoghq.com/v2alpha1
kind: DatadogAgent
spec:
  override:
    nodeAgent:
      containers:
        agent:
          env:
            - name: MY_VAR
              value: my_value
```

### 4. Status Reporting

Status updates use structured conditions and sub-resource status to report:
- Deployment progress
- Configuration errors
- Component health

### 5. RBAC Generation

RBAC is dynamically generated based on enabled features:
- Location: `pkg/kubernetes/rbac/`
- Generated at reconciliation time

### 6. Secret Management

Pluggable secret backend system:
- Support for external secret providers
- Command-based secret resolution
- Location: `pkg/secrets/`

## Navigation Tips

### Finding Functionality

| What | Where |
|------|-------|
| API definitions | `api/datadoghq/v2alpha1/` |
| Controller logic | `internal/controller/datadogagent/controller.go` |
| Feature implementations | `internal/controller/datadogagent/feature/<feature-name>/` |
| Configuration merging | `internal/controller/datadogagent/merger/` |
| RBAC generation | `pkg/kubernetes/rbac/` |
| Constants/defaults | `pkg/constants/` |
| Utilities | `pkg/controller/utils/` |
| CRD manifests | `config/crd/bases/` |
| Example configs | `config/samples/` or `examples/` |
| Documentation | `docs/` |

### Key Files to Know

| File | Purpose |
|------|---------|
| `cmd/main.go` | Operator entry point, flag parsing, manager setup |
| `internal/controller/setup.go` | Controller registration |
| `internal/controller/datadogagent/controller.go` | Main reconciliation logic |
| `api/datadoghq/v2alpha1/datadogagent_types.go` | Primary CRD definition (~92KB) |
| `pkg/constants/constants.go` | Global constants |
| `Makefile` | Build targets and automation |

### Searching the Codebase

**Common search patterns:**
```bash
# Find feature implementations
find internal/controller/datadogagent/feature -name "feature.go"

# Find CRD types
grep -r "type Datadog" api/

# Find controller reconcilers
grep -r "func (r \*.*Reconciler) Reconcile" internal/

# Find merger implementations
ls internal/controller/datadogagent/merger/

# Find test files
find . -name "*_test.go" | head -20
```

## Configuration

### Operator Flags

**Location:** `cmd/main.go`

| Flag | Default | Purpose |
|------|---------|---------|
| `metrics-addr` | `:8080` | Metrics endpoint |
| `enable-leader-election` | `true` | HA support |
| `loglevel` | `info` | Log level |
| `datadogAgentEnabled` | `true` | Enable DatadogAgent controller |
| `datadogMonitorEnabled` | `false` | Enable Monitor controller |
| `datadogAgentProfileEnabled` | `false` | Enable Profile controller (beta) |
| `supportExtendedDaemonset` | `false` | Use ExtendedDaemonSet |
| `secretBackendCommand` | `""` | Secret backend executable |

### Environment Variables

| Variable | Purpose |
|----------|---------|
| `DD_API_KEY` | Datadog API key |
| `DD_APP_KEY` | Datadog application key |
| `DD_SITE` | Datadog site (datadoghq.com, datadoghq.eu, etc.) |
| `WATCH_NAMESPACE` | Namespace to watch (empty = all) |

## Default Features

The following features are **enabled by default** when a DatadogAgent resource is created:

- Cluster Agent
- Admission Controller
- Cluster Checks
- Kubernetes Event Collection
- Kubernetes State Core Check
- Live Container Collection
- Orchestrator Explorer
- UnixDomainSocket transport
- Process Discovery
- Control Plane Monitoring

## Troubleshooting

### Common Issues

**Issue:** CRD not found
**Solution:** Run `make install` to install CRDs

**Issue:** Webhook validation errors
**Solution:** Ensure webhook certificates are valid and webhook service is running

**Issue:** Feature not enabled
**Solution:** Check feature configuration in DatadogAgent spec, verify feature is registered in `init()`

**Issue:** RBAC errors
**Solution:** Verify ServiceAccount has correct ClusterRole bindings

### Debug Flags

```bash
# Run with debug logging
go run ./cmd/main.go --loglevel=debug

# Enable profiling
go run ./cmd/main.go --profiling-enabled=true
```

### Useful Commands

```bash
# Check operator logs
kubectl logs -n datadog deployment/datadog-operator-controller-manager

# Get DatadogAgent status
kubectl get datadogagent -o yaml

# Check generated resources
kubectl get daemonset,deployment -l app.kubernetes.io/managed-by=datadog-operator

# Validate CRD
kubectl explain datadogagent.spec.features
```

## Contributing

### Code Style

- Follow standard Go conventions
- Use `gofmt` for formatting
- Run `make vet` before committing
- Add tests for new features
- Update documentation

### Pull Request Process

1. Fork and create feature branch
2. Make changes with tests
3. Run `make test` and `make vet`
4. Update documentation if needed
5. Submit PR against `main` branch
6. Reference issues using `#<issue-number>`

### Documentation

**Main docs:** `docs/`

Key documentation files:
- `docs/getting_started.md` - Setup guide
- `docs/configuration.v2alpha1.md` - Configuration reference
- `docs/how-to-contribute.md` - Contribution guide
- `docs/deprecated_configs.md` - Deprecation notices

## External Resources

- **Documentation:** https://github.com/DataDog/datadog-operator/tree/main/docs
- **OperatorHub:** https://operatorhub.io/operator/datadog-operator
- **RedHat Certification:** https://catalog.redhat.com/software/operators/detail/5e9874986c5dcb34dfbb1a12
- **Datadog Agent:** https://github.com/DataDog/datadog-agent
- **ExtendedDaemonSet:** https://github.com/DataDog/extendeddaemonset
- **Helm Chart:** https://github.com/DataDog/helm-charts/tree/main/charts/datadog

## API Versions and Migration

### Version History

- **v1alpha1**: Original API (deprecated in v1.8.0+)
- **v2alpha1**: Current stable API

### Migration Notes

- Operator v1.8.0+ does not support direct migration from v1alpha1 to v2alpha1
- Use conversion webhook in v1.7.0 for migration
- See `docs/deprecated_configs.md` for deprecation notices

## License

Licensed under Apache License 2.0. See `LICENSE` file.

## Support

- **Issues:** https://github.com/DataDog/datadog-operator/issues
- **Discussions:** GitHub Discussions
- **Slack:** Datadog Community Slack

---

**Generated for AI Agents and Developers**
This guide provides a comprehensive overview for navigating and contributing to the Datadog Operator codebase.
