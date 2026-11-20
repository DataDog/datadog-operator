# KAI Scheduler (experimental)

KAI Scheduler integration is configured through a `DatadogAgent` annotation while it is in preview. The annotation can change before the feature becomes stable.

```yaml
metadata:
  annotations:
    agent.datadoghq.com/kai-scheduler-enabled: "true"
```

The `kai-scheduler-enabled` annotation must equal `"true"`. It enables KAI PodGroup collection in the Cluster Agent, which tags pods with their KAI workload and queue, and grants the Cluster Agent `list` and `watch` on `podgroups.scheduling.run.ai`. When `features.gpu.enabled` is also set, the node Agent maps KAI fractional GPUs to the containers using them.

PodGroup collection requires Cluster Agent 7.86.0 or newer. Fractional GPU mapping requires Agent 7.86.0 or newer. The `kai_scheduler` check is not configured by this annotation; set it up as a cluster check.
