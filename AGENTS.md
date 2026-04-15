# Datadog Operator - Agent Guide

Kubernetes Operator managing Datadog resources (Agent, Monitors, SLOs, Dashboards, etc.) via multiple controllers. The DatadogAgent controller is the most complex; other controllers are simpler and follow standard controller-runtime patterns.

## Build & Test Commands

```bash
# Build
make build                    # Build manager + kubectl plugin (includes generate + lint)
make managergobuild           # Build only Go binary (skip lint/generate)

# Test
go test ./...                 # Unit tests only
make ci-test                  # Unit + integration tests (no build/fmt/licenses)
make test                     # Full: build + fmt + licenses + unit + integration (slow)
make integration-tests        # Integration tests only (needs envtest)

# Run a single test
go test ./internal/controller/datadogagent/feature/apm/ -run TestMyTest

# Lint & Format
make fmt                      # gofmt + golangci-lint --fix
make lint                     # golangci-lint + go vet
make vet                      # go vet only

# Code Generation (run after changing API types or kubebuilder markers)
make generate                 # Generate deepcopy, openapi, docs
make manifests                # Generate CRDs, RBAC, webhooks
```

## Go Workspace

This repo uses `go.work` with two modules: root (`.`) and `api/`. Run `make sync` (`go work sync`) after dependency changes.

## Import Ordering

Enforced by `gci` via golangci-lint. Four groups separated by blank lines:
1. Standard library
2. Third-party
3. `github.com/DataDog/datadog-operator/...`
4. Blank (for side-effect imports)

## Architecture

### Reconciliation Flow

```
DatadogAgent CR
  -> Validate
  -> Apply global settings
  -> Feature handlers (each in its own package) configure components
  -> User overrides applied last (always win over feature-set values)
  -> Component managers build final manifests (NodeAgent DaemonSet, ClusterAgent Deployment, ClusterChecksRunner Deployment)
  -> Apply to cluster + update status
```

Priority: **global settings < feature defaults < user overrides** (overrides always win).

### Key Paths

| What | Where |
|------|-------|
| Primary CRD types (DatadogAgent) | `api/datadoghq/v2alpha1/datadogagent_types.go` |
| Other CRD types | `api/datadoghq/v1alpha1/` |
| DatadogAgent reconciler | `internal/controller/datadogagent/controller.go` |
| DatadogAgentInternal reconciler | `internal/controller/datadogagentinternal/controller.go` |
| Feature registration (blank imports) | `internal/controller/datadogagentinternal/controller.go` |
| Feature handlers | `internal/controller/datadogagent/feature/<name>/` |
| Component managers | `internal/controller/datadogagent/component/` |
| Config mergers | `internal/controller/datadogagent/merger/` |
| User overrides | `internal/controller/datadogagent/override/` |
| Constants (shared across controllers) | `pkg/constants/` |
| Entry point | `cmd/main.go` |

### Adding a Feature (CRD-based)

Standard path for stable features with typed configuration in the CRD:

1. Add config struct to `api/datadoghq/v2alpha1/datadogagent_types.go`
2. Run `make generate && make manifests`
3. Create package in `internal/controller/datadogagent/feature/<name>/`
4. Implement the `Feature` interface and self-register via `init()` + `feature.Register()`
5. Add blank import (`_ ".../<name>"`) in `internal/controller/datadogagentinternal/controller.go` to register the feature
6. Write tests in the same package

### Adding an Experimental Feature (annotation-based, no CRD change)

For features that need rapid iteration without CRD schema changes, configuration can live in annotations on the DatadogAgent object instead of typed CRD fields. See `privateactionrunner`, `flightrecorder`, or `hostprofiler` for examples.

The pattern:
1. Define annotation keys in `internal/controller/datadogagent/feature/utils/utils.go` (e.g. `agent.datadoghq.com/<name>-enabled`, `agent.datadoghq.com/<name>-configdata`)
2. In `Configure()`, read annotations via `featureutils.HasFeatureEnableAnnotation()` / `featureutils.GetFeatureConfigAnnotation()` instead of reading from `ddaSpec.Features`
3. Parse YAML config from annotation values into typed Go structs internally
4. Create ConfigMaps dynamically in `ManageDependencies()` to provide config files to containers
5. Everything else (registration, blank import, volumes, env vars) works the same as CRD-based features

### Modifying the CRD

1. Edit types in `api/datadoghq/v2alpha1/` (DatadogAgent) or `api/datadoghq/v1alpha1/` (other CRs)
2. Add kubebuilder validation markers as needed
3. Run `make generate && make manifests`
4. Update conversion webhooks if needed (`datadogagent_conversion.go`)

## Gotchas

- CRD types file (`api/datadoghq/v2alpha1/datadogagent_types.go`) is ~92KB. Read only the section you need
- `make test` is slow (builds + formats + licenses + all tests). For quick iteration use `go test ./...` or `make ci-test`
- Run `make lint` before committing to catch issues early

## API Versions

- **v2alpha1**: Current version for DatadogAgent
- **v1alpha1**: Used by other CRDs (DatadogMonitor, DatadogSLO, DatadogDashboard, DatadogAgentProfile, etc.)
