# Getting Started

This procedure describes the simplest and fastest way to deploy a `DatadogMonitor` with the Datadog Operator. **Note: Operator version 0.6+ is required.**

## Prerequisites

The following prerequisites are required to use `DatadogMonitor`:

- **Kubernetes Cluster version >= v1.14.X**: Tests were done on versions >= `1.14.0`. However, it should work on versions `>= v1.11.0`. For earlier versions, due to limited CRD support, the Operator may not work as expected.
- [`Helm`][1] for deploying the `datadog-operator`.
- [`Kubectl` cli][2] for installing a `DatadogMonitor`.

## Adding a DatadogMonitor

To deploy a `DatadogMonitor` with the Datadog Operator, see the [`datadog-operator`][3] Helm chart.
Here are the steps:

1. Install the [Datadog Operator][4]:

   First, add the Datadog Helm chart with

    ```shell
    helm repo add datadog https://helm.datadoghq.com
    ```

    1. Run the following command, substituting your [Datadog API and application keys][5]:

        ```shell
        helm install my-datadog-operator datadog/datadog-operator --set apiKey=<DATADOG_API_KEY> --set appKey=<DATADOG_APP_KEY> --set datadogMonitor.enabled=true
        ```

    1. Alternatively, update the [values.yaml][6] file of the Datadog Operator Helm chart to include your [Datadog API and application keys][5] and enable `DatadogMonitor`.
       Then, run

        ```shell
        helm install my-datadog-operator datadog/datadog-operator -f values.yaml
        ```

1. Create a file with the spec of your `DatadogMonitor` deployment configuration. A simple example configuration is:

    ```yaml
    apiVersion: datadoghq.com/v1alpha1
    kind: DatadogMonitor
    metadata:
    name: datadog-monitor-test
    spec:
      query: "avg(last_10m):avg:system.disk.in_use{*} by {host} > 0.5"
      type: "metric alert"
      name: "Test monitor made from DatadogMonitor"
      message: "We are running out of disk space!"
      tags:
        - "test:datadog"
    ```

    For additional examples, see [examples/datadog-monitor](../examples/datadogmonitor). Note that only metric alerts, query alerts, and service checks are supported.

1. Deploy the `DatadogMonitor` with the above configuration file:

    ```shell
    kubectl apply -f /path/to/your/datadog-monitor.yaml
    ```

    This results in the automatic creation of a new monitor in Datadog. The monitor can be found on the [Manage Monitors][7] page of your Datadog account.
    *Note*: All monitors created from `DatadogMonitor` are automatically tagged with `generated:kubernetes`.

## Cleanup

The following commands delete the monitor from your Datadog account and all the Kubernetes resources created by the above instructions:

```shell
kubectl delete datadogmonitor datadog-monitor-test
helm delete datadog
```

## Usage and Troubleshooting

To verify monitor creation and check the monitor state, run

```shell
$ kubectl get datadogmonitor datadog-monitor-test

NAME                     ID         MONITOR STATE   LAST TRANSITION        LAST SYNC              SYNC STATUS                AGE
datadog-monitor-test     1234       Alert           2021-03-29T17:32:47Z   2021-03-30T12:52:47Z   OK                         19h
```

To view details about the monitor, including monitor groups that are currently in an alerting state, run

```shell
$ kubectl describe datadogmonitor datadog-monitor-test

Name:         datadog-monitor-test
Namespace:    datadog
Labels:       <none>
Annotations:  <none>
API Version:  datadoghq.com/v1alpha1
Kind:         DatadogMonitor
Metadata:
  Creation Timestamp:  2021-03-20T13:17:03Z
  ...
Spec:
  Message:  1-2-3 testing
  Name:     Test monitor made from DatadogMonitor
  Options:
  Query:  avg(last_10m):avg:system.disk.in_use{*} by {host} > 0.5
  Tags:
    test:datadog
    generated:kubernetes
  Type:  metric alert
Status:
  Conditions:
    Last Transition Time:  2021-03-29T17:32:47Z
    Last Update Time:      2021-03-30T12:52:47Z
    Message:               DatadogMonitor ready
    Status:                True
    Type:                  Active
  Current Hash:            b30484c5976d3709b623e5e081e6ce18
  Downtime Status:
Events:  <none>
```

To investigate any issues, view the Operator logs (of the leader pod, if more than one):

```shell
kubectl logs <my-datadog-operator-pod-name>
```


[1]: https://helm.sh
[2]: https://kubernetes.io/docs/tasks/tools/install-kubectl/
[3]: https://github.com/DataDog/helm-charts/tree/master/charts/datadog-operator
[4]: https://artifacthub.io/packages/helm/datadog/datadog-operator
[5]: https://app.datadoghq.com/account/settings#api
[6]: https://github.com/DataDog/helm-charts/blob/master/charts/datadog-operator/values.yaml
[7]: https://app.datadoghq.com/monitors/manage?q=tag%3A"generated%3Akubernetes"
