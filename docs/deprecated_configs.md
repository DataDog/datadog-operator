# Deprecated Configurations and Migration Guidelines

This document lists configuration options that are deprecated or will be deprecated in future versions of the Datadog Operator.

## Deprecated Configuration Table

| Feature | Deprecation Notice | Deprecation Version | Removal Version |
|---------|-------------------|-------------------|-------------------|
| `global.runProcessChecksInCoreAgent` | The `runProcessChecksInCoreAgent` configuration is deprecated in 1.19, and will be removed in v1.21. | v1.19 | v1.21 |

## Migration Guidelines

### runProcessChecksInCoreAgent

The `runProcessChecksInCoreAgent` field in the Global configuration has been removed. This field previously controlled whether the Process Agent or Core Agent collects process and container checks and features.

### Migration Path
Process checks are now run in the core Agent by default.

As of Agent 7.78, the `process_config.run_in_core_agent.enabled` config key has been removed from the Agent. On Linux, process checks always run in the core Agent — no configuration toggle is needed. The Operator no longer injects the `DD_PROCESS_CONFIG_RUN_IN_CORE_AGENT_ENABLED` environment variable.

If this field was set to `true`, it can be removed with no behavior change. If you are using Agent v7.60 or below, the Operator will still route process check configuration to the Process Agent container automatically.
