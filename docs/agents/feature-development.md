# Feature Development Playbooks

## Adding a Feature (CRD-based)

Standard path for stable features with typed configuration in the CRD:

1. Add config struct to `api/datadoghq/v2alpha1/datadogagent_types.go`
2. Run `make generate && make manifests`
3. Create package in `internal/controller/datadogagent/feature/<name>/`
4. Implement the `Feature` interface and self-register via `init()` + `feature.Register()`
5. Add blank import (`_ ".../<name>"`) in `internal/controller/datadogagentinternal/controller.go` to register the feature
6. Write tests in the same package

## Adding an Experimental Feature (annotation-based, no CRD change)

For features that need rapid iteration without CRD schema changes. See `privateactionrunner`, `flightrecorder`, or `hostprofiler` for examples (valid for Operator 1.26 in April 2026).

1. Define annotation keys in `internal/controller/datadogagent/feature/utils/utils.go` (e.g. `agent.datadoghq.com/<name>-enabled`, `agent.datadoghq.com/<name>-configdata`)
2. In `Configure()`, read annotations via `featureutils.HasFeatureEnableAnnotation()` / `featureutils.GetFeatureConfigAnnotation()` instead of reading from `ddaSpec.Features`
3. Parse YAML config from annotation values into typed Go structs internally
4. Everything else (registration, blank import, volumes, env vars) works the same as CRD-based features

## Modifying the CRD

1. Edit types in `api/datadoghq/v2alpha1/` (DatadogAgent) or `api/datadoghq/v1alpha1/` (other CRs)
2. Add kubebuilder markers as needed
3. Run `make generate && make manifests`
