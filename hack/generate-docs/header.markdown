# Configuration

## Manifest Templates

* [Manifest with Logs, APM, process, and metrics collection enabled.][1]
* [Manifest with Logs, APM, and metrics collection enabled.][2]
* [Manifest with Logs and metrics collection enabled.][3]
* [Manifest with APM and metrics collection enabled.][4]
* [Manifest with Cluster Agent.][5]
* [Manifest with tolerations.][6]

## All configuration options

The following table lists the configurable parameters for the `DatadogAgent`
resource. For example, if you wanted to set a value for `agent.image.name`,
your `DatadogAgent` resource would look like the following:

```yaml
apiVersion: datadoghq.com/v1alpha1
kind: DatadogAgent
metadata:
  name: datadog
spec:
  agent:
    image:
      name: "gcr.io/datadoghq/agent:latest"
```

