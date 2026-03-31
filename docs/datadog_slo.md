# Datadog SLOs
This feature is in Preview.

## Overview
The `DatadogSLO` Custom Resource Definition (CRD) allows users to create [SLOs][1] using the Operator and manage them as Kubernetes resources.

## Prerequisites

- Datadog Operator v1.9+
- [Helm][2], to deploy the Datadog Operator
- The [kubectl CLI][3], to install a `DatadogSLO`


## Adding a DatadogSLO

To deploy a `DatadogSLO` with the Datadog Operator, use the [`datadog-operator` Helm chart][4].

1. To install the [Datadog Operator][5], first add the Datadog Helm chart using the following command:

    ```shell
    helm repo add datadog https://helm.datadoghq.com
    ```

1. Choose one of the following options:

    * Run the install command, substituting your [Datadog API and application keys][6]:

        ```shell
        helm install my-datadog-operator datadog/datadog-operator --set apiKey=<DATADOG_API_KEY> --set appKey=<DATADOG_APP_KEY> --set datadogSLO.enabled=true
        ```

    * Create an override [`values.yaml`][7] file that includes your [Datadog API and application keys][6] and enables the `DatadogSLO` controller. Then run the install command:

        ```shell
        helm install my-datadog-operator datadog/datadog-operator -f values.yaml
        ```

2. Create a file with the spec of your `DatadogSLO` deployment configuration. The Datadog Operator supports three types of SLOs:

    **Metric SLO Example:**
    ```yaml
    apiVersion: datadoghq.com/v1alpha1
    kind: DatadogSLO
    metadata:
      name: example-slo
      namespace: system 
    spec:
      name: example-slo
      description: "This is an example metric SLO from datadog-operator"
      query:
        denominator: "sum:requests.total{service:example,env:prod}.as_count()"
        numerator: "sum:requests.success{service:example,env:prod}.as_count()"
      tags:
        - "service:example"
        - "env:prod"
      targetThreshold: "99.9"
      timeframe: "7d"
      type: "metric"
    ```

    **Monitor SLO Example:**
    ```yaml
    apiVersion: datadoghq.com/v1alpha1
    kind: DatadogSLO
    metadata:
      name: example-monitor-slo
      namespace: system
    spec:
      name: example-monitor-slo
      description: "This is an example monitor SLO"
      monitorIDs:
        - 12345678
      tags:
        - "service:example"
        - "env:prod"
      targetThreshold: "99.5"
      timeframe: "30d"
      type: "monitor"
    ```

    **Time-Slice SLO Example:**
    ```yaml
    apiVersion: datadoghq.com/v1alpha1
    kind: DatadogSLO
    metadata:
      name: example-time-slice-slo
      namespace: system
    spec:
      name: example-time-slice-slo
      description: "This is an example time-slice SLO"
      type: "time_slice"
      timeSliceSpec:
        timeSliceCondition:
          comparator: "<"
          threshold: "5.0"
        query:
          formulas:
            - formula: "query1"
          queries:
            - dataSource: "metrics"
              name: "query1"
              query: "avg:system.cpu.user{*}"
      targetThreshold: "99.0"
      timeframe: "7d"
      tags:
        - "team:infrastructure"
        - "env:prod"
    ```

3. Deploy the `DatadogSLO` with the above configuration file:

    ```shell
    kubectl apply -f /path/to/your/datadog-slo.yaml
    ```

    This automatically creates a new SLO in Datadog. You can find it on the [SLOs][8] page of your Datadog account.
    Datadog Operator occasionally reconciles and keeps SLOs in line with the given configuration. There is also a force 
    sync every hour, so if a user deletes an SLO in the Datadog UI, Datadog Operator restores it in under an hour.

## DatadogSLO Spec

| Parameter | Description |
| --------- | ----------- |
| name | Name of the SLO |
| description | Description of the SLO |
| tags | Tags to associate with the SLO |
| type | Type of the SLO. Can be `metric`, `monitor`, or `time_slice` |
| query | Query for `metric` SLOs |
| monitorIDs | Monitor IDs for `monitor` SLOs |
| groups | Monitor groups for `monitor` SLOs (only valid when one monitor ID is provided) |
| timeSliceSpec | Time-slice specification for `time_slice` SLOs |
| timeSliceSpec.timeSliceCondition.comparator | Comparator for the time-slice condition (e.g., `<`, `>`, `<=`, `>=`) |
| timeSliceSpec.timeSliceCondition.threshold | Threshold value for the time-slice condition |
| timeSliceSpec.query.formulas | List of formulas for the time-slice query |
| timeSliceSpec.query.queries | List of query definitions with dataSource, name, and query |
| targetThreshold | Target threshold for the SLO |
| warningThreshold | Warning threshold for the SLO |
| timeframe | Timeframe for the SLO. Can be `7d`, `30d`, or `90d` |
| controllerOptions.disableRequiredTags | Disables the automatic addition of required tags to SLOs |

## Cleanup

The following commands delete the SLO from your Datadog account as well as all of the Kubernetes resources created by the previous instructions:

```shell
kubectl delete datadogslo example-slo
helm delete my-datadog-operator
```


[1]: https://docs.datadoghq.com/service_management/service_level_objectives/
[2]: https://helm.sh
[3]: https://kubernetes.io/docs/tasks/tools/install-kubectl/
[4]: https://github.com/DataDog/helm-charts/tree/main/charts/datadog-operator
[5]: https://artifacthub.io/packages/helm/datadog/datadog-operator
[6]: https://app.datadoghq.com/account/settings#api
[7]: https://github.com/DataDog/helm-charts/blob/main/charts/datadog-operator/values.yaml
[8]: https://app.datadoghq.com/slo/manage
