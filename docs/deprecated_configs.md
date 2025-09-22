# Deprecated Configurations and Migration Guidelines

This document lists configuration options that are deprecated or will be deprecated in future versions of the Datadog Operator.

## Deprecated Configuration Table

| Feature | Deprecation Notice | Deprecation Version | Removal Version |
|---------|-------------------|-------------------|-------------------|
| `global.runProcessChecksInCoreAgent` | The `runProcessChecksInCoreAgent` configuration is deprecated in 1.19, and will be removed in v1.21. | v1.19 | v1.21 |

## Migration Guidelines

### runProcessChecksInCoreAgent

The `runProcessChecksInCoreAgent` field in the Global configuration is being deprecated. This field previously controlled whether the Process Agent or Core Agent collects process and container checks and featurres.

### Migration Path
Process checks are now run in the core Agent by default. 

If this field was set to `true`, it can be removed with no behavior change. If you are using Agent v7.60 or below, you can use environment variable overrides or upgrade your Agent version.

If this field was set to `false`, use the environment variable override (`DD_PROCESS_CONFIG_RUN_IN_CORE_AGENT_ENABLED=false`) to disable this functionality.