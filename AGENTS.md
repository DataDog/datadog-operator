# Datadog Operator

Kubernetes Operator managing Datadog resources (Agent, Monitors, SLOs, Dashboards, etc.) via multiple controllers.

## Build & Test

```bash
make build                    # Build manager + kubectl plugin (includes generate + lint)
make managergobuild           # Build only Go binary (skip lint/generate)
go test ./...                 # Unit tests only
make ci-test                  # Unit + integration tests (no build/fmt/licenses)
make lint                     # golangci-lint + go vet — run before committing
go test ./path/to/pkg/ -run TestName  # Single test
```

## Code Generation

Run after changing API types or kubebuilder markers:

```bash
make generate && make manifests
```

## Gotchas

- Run `make sync` after dependency changes (Go workspace: root `.` + `api/`)
- CRD types file (`api/datadoghq/v2alpha1/datadogagent_types.go`) is very large — read only the section you need
- `make test` is slow (builds + formats + licenses + all tests). Use `go test ./...` or `make ci-test` for quick iteration

## Deep Dives

- [DatadogAgent architecture & reconciliation flow](docs/agents/architecture.md)
- [DatadogAgent feature development playbooks](docs/agents/feature-development.md)
