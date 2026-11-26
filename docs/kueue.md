# Kueue (experimental)

Kueue integration is configured through `DatadogAgent` annotations while it is in preview. The annotations can change before the feature becomes stable.

```yaml
metadata:
  annotations:
    agent.datadoghq.com/kueue-enabled: "true"
    agent.datadoghq.com/kueue-config: >-
      {"collectWorkloadEvents":true,"metricsService":{"name":"kueue-controller-manager-metrics-service","namespace":"kueue-system"}}
spec:
  features:
    clusterChecks:
      enabled: true
```

The `kueue-enabled` annotation must equal `"true"`. The optional `kueue-config` annotation is a JSON object. It accepts `collectWorkloadEvents` (defaults to `true`), `clusterAgentEvents` (defaults to `false`), `metricsService.name` (defaults to `kueue-controller-manager-metrics-service`), `metricsService.namespace` (defaults to `kueue-system`), and `conf`. Both metrics Service identifiers must be DNS labels: lowercase letters, digits, and hyphens, with at most 63 characters. The `conf` object has the same `configData` or `configMap` format as other check configurations. Invalid configuration disables the integration and logs an error.

The Cluster Agent needs version 7.82.0 or newer. The Kueue check also requires cluster checks, a node Agent version 7.82.0 or newer, and a node Agent running on each Kueue controller node. Multiple Kueue controller replicas require Agent 7.86.0 or newer. If `global.disableNonResourceRules` is enabled, grant the node Agent service account `get` on the `/metrics` non-resource URL separately.

## Workload events from the Cluster Agent

Set `clusterAgentEvents` to `true` in `kueue-config` to send the Kueue Workload events from the Cluster Agent leader (the `job_scheduler_events` check) instead of from the node Agent check. The operator then sets `DD_CLUSTER_AGENT_KUEUE_EVENTS_ENABLED=true` on the Cluster Agent and `collect_workload_events: false` in the generated Kueue check configuration, and does not grant the node Agent access to Workloads. It overrides `collectWorkloadEvents`. The Cluster Agent already runs with leader election and has the permissions it needs.

This requires Cluster Agent 7.87.0 or newer. With an older or unparsable Cluster Agent version, `clusterAgentEvents` is ignored and the node Agent check keeps sending the events, so none are lost or duplicated. If you provide the check configuration with `conf`, set `collect_workload_events: false` in it yourself.
