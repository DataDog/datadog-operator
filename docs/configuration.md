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
  credentials:
    apiSecret:
      secretName: datadog-secret
      keyName: api-key
    appSecret:
      secretName: datadog-secret
      keyName: app-key
```

| Parameter | Description |
| --------- | ----------- |
| features | Features running on the Agent and Cluster Agent. |
| global.clusterName | Set a unique cluster name to allow scoping hosts and Cluster Checks Runner easily. |
| global.registry | Registry to use for all Agent images (default gcr.io/datadoghq). Use public.ecr.aws/datadog for AWS Use docker.io/datadog for DockerHub |
| global.site | The site of the Datadog intake to send Agent data to. Set to 'datadoghq.eu' to send data to the EU site. |
| override | Override the confiuration of the components |

[1]: https://github.com/DataDog/datadog-operator/blob/main/examples/datadogagent/datadog-agent-all.yaml
[2]: https://github.com/DataDog/datadog-operator/blob/main/examples/datadogagent/datadog-agent-logs-apm.yaml
[3]: https://github.com/DataDog/datadog-operator/blob/main/examples/datadogagent/datadog-agent-logs.yaml
[4]: https://github.com/DataDog/datadog-operator/blob/main/examples/datadogagent/datadog-agent-apm.yaml
[5]: https://github.com/DataDog/datadog-operator/blob/main/examples/datadogagent/datadog-agent-with-clusteragent.yaml
[6]: https://github.com/DataDog/datadog-operator/blob/main/examples/datadogagent/datadog-agent-with-tolerations.yaml
