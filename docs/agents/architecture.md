# Architecture

The DatadogAgent controller is the most complex; other controllers are simpler and follow standard controller-runtime patterns.

## Reconciliation Flow

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

## Key Paths

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
