# Deprecated Configurations and Migration Guidelines

This document lists configuration options that are deprecated or will be deprecated in future versions of the Datadog Operator.

## Deprecated Configuration Table

| Feature | Deprecation Notice | Deprecation Version | Removal Version |
|---------|-------------------|-------------------|-------------------|
| `global.runProcessChecksInCoreAgent` | The `runProcessChecksInCoreAgent` configuration is deprecated in 1.19, and will be removed in v1.21. | v1.19 | v1.21 |
| `features.serviceDiscovery.networkStats` | The `networkStats` configuration is deprecated in v1.26 and removed in v1.28. | v1.26 | v1.28 |
| `appsec.* annotations` | The `appsec.*` annotations are deprecated in v1.30 in favor of `spec.features.appsec.injector`. | v1.30 | TBD |

## Migration Guidelines

### runProcessChecksInCoreAgent

The `runProcessChecksInCoreAgent` field in the Global configuration has been removed. This field previously controlled whether the Process Agent or Core Agent collects process and container checks and features.

#### Migration Path
Process checks are now run in the core Agent by default. 

As of Agent 7.78, the `process_config.run_in_core_agent.enabled` config key has been removed from the Agent. On Linux, process checks always run in the core Agent — no configuration toggle is needed.

If this field was set to `true`, it can be removed with no behavior change. If you are using Agent v7.60 or below, you can use environment variable overrides or upgrade your Agent version.
If this field was set to `false`, use the environment variable override (`DD_PROCESS_CONFIG_RUN_IN_CORE_AGENT_ENABLED=false`) to disable this functionality.

### serviceDiscovery.networkStats

The `networkStats` field in the ServiceDiscovery feature configuration was removed in v1.28. Network stats collection is no longer configurable through the operator.

#### Migration Path

This field can be removed from your `DatadogAgent` spec with no behavior change.

### appsec.* annotations

The `agent.datadoghq.com/appsec.*` annotations are deprecated in `v1.30` and have been migrated to the `DatadogAgent` CRD spec under `spec.features.appsec.injector`.

#### Migration Path

Migrate your Kubernetes annotations to the `spec.features.appsec.injector` configuration in your `DatadogAgent` spec:

| Annotation | CRD Path |
|------------|----------|
| `agent.datadoghq.com/appsec.injector.enabled` | `spec.features.appsec.injector.enabled` |
| `agent.datadoghq.com/appsec.injector.autoDetect` | `spec.features.appsec.injector.autoDetect` |
| `agent.datadoghq.com/appsec.injector.proxies` | `spec.features.appsec.injector.proxies` |
| `agent.datadoghq.com/appsec.injector.processor.address` | `spec.features.appsec.injector.processor.address` |
| `agent.datadoghq.com/appsec.injector.processor.port` | `spec.features.appsec.injector.processor.port` |
| `agent.datadoghq.com/appsec.injector.processor.service.name` | `spec.features.appsec.injector.processor.service.name` |
| `agent.datadoghq.com/appsec.injector.processor.service.namespace` | `spec.features.appsec.injector.processor.service.namespace` |
| `agent.datadoghq.com/appsec.injector.mode` | `spec.features.appsec.injector.mode` |
| `agent.datadoghq.com/appsec.sidecar.image` | `spec.features.appsec.injector.sidecar.image` |
| `agent.datadoghq.com/appsec.sidecar.image_tag` | `spec.features.appsec.injector.sidecar.imageTag` |
| `agent.datadoghq.com/appsec.sidecar.port` | `spec.features.appsec.injector.sidecar.port` |
| `agent.datadoghq.com/appsec.sidecar.health_port` | `spec.features.appsec.injector.sidecar.healthPort` |
| `agent.datadoghq.com/appsec.sidecar.resources.requests.cpu` | `spec.features.appsec.injector.sidecar.resources.requests.cpu` |
| `agent.datadoghq.com/appsec.sidecar.resources.requests.memory` | `spec.features.appsec.injector.sidecar.resources.requests.memory` |
| `agent.datadoghq.com/appsec.sidecar.resources.limits.cpu` | `spec.features.appsec.injector.sidecar.resources.limits.cpu` |
| `agent.datadoghq.com/appsec.sidecar.resources.limits.memory` | `spec.features.appsec.injector.sidecar.resources.limits.memory` |
| `agent.datadoghq.com/appsec.sidecar.body_parsing_size_limit` | `spec.features.appsec.injector.sidecar.bodyParsingSizeLimit` |
| `agent.datadoghq.com/appsec.nginx.module_mount_path` | `spec.features.appsec.injector.nginx.moduleMountPath` |
