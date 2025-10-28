This page lists commonly-used configuration parameters for the Datadog Operator. For all configuration parameters, see the [configuration spec][1] in the [`DataDog/datadog-operator`][2] repo.

### Example manifests

* [Manifest with logs, APM, process, and metrics collection enabled][3]
* [Manifest with logs, APM, and metrics collection enabled][4]
* [Manifest with APM and metrics collection enabled][5]
* [Manifest with Cluster Agent][6]
* [Manifest with tolerations][7]

## Global options

The table in this section lists configurable parameters for the `DatadogAgent` resource. To override parameters for individual components (Node Agent, Cluster Agent, or Cluster Checks Runner) see [override options](#override-options).

For example: the following manifest uses the `global.clusterName` parameter to set a custom cluster name:

{{< highlight yaml "hl_lines=7" >}}
apiVersion: datadoghq.com/v2alpha1
kind: DatadogAgent
metadata:
  name: datadog
spec:
  global:
    clusterName: my-test-cluster
    credentials:
      apiSecret:
        secretName: datadog-secret
        keyName: api-key
      appSecret:
        secretName: datadog-secret
        keyName: app-key
{{< /highlight >}}