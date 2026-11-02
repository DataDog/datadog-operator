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

The `kueue-enabled` annotation must equal `"true"`. The optional `kueue-config` annotation is a JSON object. It accepts `collectWorkloadEvents` (defaults to `true`), `metricsService.name` (defaults to `kueue-controller-manager-metrics-service`), `metricsService.namespace` (defaults to `kueue-system`), and `conf`. Both metrics Service identifiers must be DNS labels: lowercase letters, digits, and hyphens, with at most 63 characters. The `conf` object has the same `configData` or `configMap` format as other check configurations. Invalid configuration disables the integration and logs an error.

The Cluster Agent needs version 7.82.0 or newer. The Kueue check also requires cluster checks, a node Agent version 7.82.0 or newer, and a node Agent running on each Kueue controller node. Multiple Kueue controller replicas require Agent 7.86.0 or newer. If `global.disableNonResourceRules` is enabled, grant the node Agent service account `get` on the `/metrics` non-resource URL separately.
