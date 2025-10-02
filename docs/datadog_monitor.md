# Getting Started

This page describes the simplest and fastest way to deploy a [Datadog monitor](https://docs.datadoghq.com/monitors/) with the Datadog Operator.

## Prerequisites

- Datadog Operator v0.6+
- **Kubernetes Cluster version >= v1.20.X**: Tests were done on versions >= `1.20.0`. Still, it should work on versions `>= v1.11.0`. For earlier versions, because of limited CRD support, the Operator may not work as expected.
- **[Helm][1]** for deploying the Datadog Operator
- **[`kubectl` CLI][2]** for installing a `DatadogMonitor`

## Adding a DatadogMonitor

To deploy a `DatadogMonitor` with the Datadog Operator, use the [`datadog-operator` Helm chart][3].

1. Install the [Datadog Operator][4]:

   First, add the Datadog Helm chart with

    ```shell
    helm repo add datadog https://helm.datadoghq.com
    ```

    1. Run the following command, substituting your [Datadog API and application keys][5]:

        ```shell
        helm install my-datadog-operator datadog/datadog-operator --set apiKey=<DATADOG_API_KEY> --set appKey=<DATADOG_APP_KEY> --set datadogMonitor.enabled=true
        ```

    1. Alternatively, update the [`values.yaml`][6] file of the Datadog Operator Helm chart to include your [Datadog API and application keys][5] and enable `DatadogMonitor`.
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

   By default, the Operator only watches its own namespace, so it will manage any `DatadogMonitor` objects within its own namespace. Therefore, you should deploy your Datadog objects in the same namespace as the Operator. If you'd like to deploy your DatadogMonitors in different namespaces, then you will need to configure the Operator [`watchNamespaces`][6] section with those additional namespaces:

   ```yaml
      #(...)
      watchNamespaces:
        - datadog
        - <NAMESPACE_1>
        - <NAMESPACE_2>
        - <NAMESPACE_3>
   ```
   *Note:* Adding namespaces increases number of resources the Operator watches. You may need to adjust the memory limits for these addition of namespaces.

1. Deploy the `DatadogMonitor` with the above configuration file:

    ```shell
    kubectl apply -f /path/to/your/datadog-monitor.yaml
    ```

    This automatically creates a new monitor in Datadog. You can find it on the [Manage Monitors][7] page of your Datadog account.

    *Note*: All monitors created from `DatadogMonitor` are automatically tagged with `generated:kubernetes`.

By default, the Operator ensures that the API monitor definition stays in sync with the DatadogMonitor resource every **60** minutes (per monitor). This interval can be adjusted using the environment variable `DD_MONITOR_FORCE_SYNC_PERIOD`, which specifies the number of minutes. For example, setting this variable to `"30"` changes the interval to 30 minutes.

## Cleanup

The following commands delete the monitor from your Datadog account and all the Kubernetes resources created by the above instructions:

```shell
kubectl delete datadogmonitor datadog-monitor-test
helm delete my-datadog-operator
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
$ kubectl describe datadogmonitor datadog-event-v2-alert-test

Name:         datadog-event-v2-alert-test
Namespace:    system
Labels:       <none>
Annotations:  <none>
API Version:  datadoghq.com/v1alpha1
Kind:         DatadogMonitor
Metadata:
  Creation Timestamp:  2025-09-22T19:10:03Z
  Finalizers:
    finalizer.monitor.datadoghq.com
  Generation:        5
  Resource Version:  3193
  UID:               60c8071f-518f-4b75-a445-caaecea92061
Spec:
  Controller Options:
  Message:  1-2-3 testing
  Name:     Test event v2 alert made from DatadogMonitor
  Options:
    Evaluation Delay:   300
    Include Tags:       true
    Locked:             false
    No Data Timeframe:  30
    Notify No Data:     true
    Renotify Interval:  1440
  Priority:             5
  Query:                events("sources:nagios status:(error OR warning) priority:normal").rollup("count").last("1h") > 10
  Tags:
    test:datadog
    generated:kubernetes
  Type:  event-v2 alert
Status:
  Conditions:
    Last Transition Time:  2025-09-22T19:10:21Z
    Last Update Time:      2025-09-22T19:10:21Z
    Status:                False
    Type:                  Error
    Last Transition Time:  2025-09-22T19:10:21Z
    Last Update Time:      2025-09-22T19:10:21Z
    Message:               DatadogMonitor Created
    Status:                True
    Type:                  Created
    Last Transition Time:  2025-09-22T19:10:21Z
    Last Update Time:      2025-09-22T19:10:21Z
    Message:               DatadogMonitor ready
    Status:                True
    Type:                  Active
    Last Transition Time:  2025-09-22T19:10:26Z
    Last Update Time:      2025-09-22T19:10:26Z
    Message:               DatadogMonitor Updated
    Status:                True
    Type:                  Updated
  Created:                 2025-09-22T19:10:21Z
  Creator:                 eman.okyere@datadoghq.com
  Current Hash:            009148d415bf6e16d2f920e69d03a5f3
  Downtime Status:
  Id:                                  217179025
  Monitor Last Force Sync Time:        2025-09-22T19:10:26Z
  Monitor State:                       OK
  Monitor State Last Transition Time:  2025-09-22T19:11:27Z
  Monitor State Last Update Time:      2025-09-22T19:32:31Z
  Monitor State Sync Status:           OK
  Primary:                             true
Events:
  Type    Reason                 Age   From            Message
  ----    ------                 ----  ----            -------
  Normal  Create DatadogMonitor  22m   DatadogMonitor  system/datadog-event-v2-alert-test
```
Unlike dashboards and SLOs, monitor state is synced every minute to ensure that K8s contains up-to-date state changes. 
This means that if a monitor's state transitions from OK to Warn, the CR's state gets updated to Warn in a minute. 
It also means that a user deletes a dashboard in the Datadog UI, Datadog Operator restores it in under an hour.

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
