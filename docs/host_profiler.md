# Host Profiler

Host Profiler is an experimental, annotation-based feature in Preview. It can
be enabled and configured through annotations on a `DatadogAgent`.

## Annotations

| Annotation | Default | Description |
| --------- | ------- | ----------- |
| `agent.datadoghq.com/host-profiler-enabled` | `false` | Enables Host Profiler when set to `"true"`. |
| `agent.datadoghq.com/host-profiler-seccomp-enabled` | `true` | Enables the Host Profiler seccomp profile and its setup init container. Set to `"false"` to disable both. |
| `agent.datadoghq.com/host-profiler-logging-seccomp-enabled` | `false` | Enables the logging seccomp profile. Has no effect when the Host Profiler seccomp profile is disabled. |
| `agent.datadoghq.com/host-profiler-non-root-enabled` | `false` | Runs the Host Profiler container as UID/GID `100` with `runAsNonRoot: true` when set to `"true"`. |
| `agent.datadoghq.com/host-profiler-selinux-type` | `spc_t` | Sets the SELinux type for the Host Profiler container and its setup init container. |

## Example

```yaml
apiVersion: datadoghq.com/v2alpha1
kind: DatadogAgent
metadata:
  name: datadog
  annotations:
    agent.datadoghq.com/host-profiler-enabled: "true"
    agent.datadoghq.com/host-profiler-non-root-enabled: "true"
    agent.datadoghq.com/host-profiler-selinux-type: "custom_t"
```
