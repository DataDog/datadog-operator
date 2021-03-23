# Getting Started

This procedure describes the simplest and fastest way to deploy a `DatadogMonitor` with the Datadog Operator.

## Prerequisites

Using the Datadog Operator requires the following prerequisites:

- **Kubernetes Cluster version >= v1.14.X**: Tests were done on versions >= `1.14.0`. However, it should work on versions `>= v1.11.0`. For earlier versions, due to limited CRD support, the operator may not work as expected.
- [`Helm`][1] for deploying the `datadog-operator`.
- [`Kubectl` cli][2] for installing a `DatadogMonitor`.

## Using DatadogMonitor

To deploy a `DatadogMonitor` with the Datadog Operator, see the [`datadog-operator`][3] helm chart.
Here are the steps:

1. Update the [values.yaml][4] file of the Datadog Operator helm chart to include your [Datadog API and application keys][5]

1. Install the [Datadog Operator][6]:

   ```shell
   helm repo add datadog https://helm.datadoghq.com
   helm install my-datadog-operator datadog/datadog-operator -f values.yaml
   ```

1. Create a file with the spec of your `DatadogMonitor` deployment configuration. A simple example configuration is:

    ```yaml
    apiVersion: datadoghq.com/v1alpha1
    kind: DatadogMonitor
    metadata:
    name: datadog-monitor-test
    namespace: datadog
    spec:
      query: "avg(last_10m):avg:system.disk.in_use{*} by {host} > 0.5"
      type: "metric alert"
      name: "Test monitor made from DatadogMonitor"
      message: "We are running out of disk space!"
      tags:
        - "test:datadog"
    ```

    For additional examples, see [examples/datadog-monitor](../examples/datadogmonitor). Note that currently only metric alerts, query alerts, and service checks are supported.

1. Deploy the `DatadogMonitor` with the above configuration file:
    ```shell
    kubectl apply -f /path/to/your/datadog-monitor.yaml
    ```

    If configured properly this will result in the creation of a new monitor in your [Datadog account][7].

## Cleanup

The following command deletes all the Kubernetes resources created by the above instructions:

```shell
kubectl delete datadogmonitor datadog-monitor-test
helm delete datadog
```

[1]: https://helm.sh
[2]: https://kubernetes.io/docs/tasks/tools/install-kubectl/
[3]: https://github.com/DataDog/helm-charts/tree/master/charts/datadog-operator
[4]: https://github.com/DataDog/helm-charts/blob/master/charts/datadog-operator/values.yaml
[5]: https://app.datadoghq.com/account/settings#api
[6]: https://artifacthub.io/packages/helm/datadog/datadog-operator
[7]: https://app.datadoghq.com/monitors/manage
