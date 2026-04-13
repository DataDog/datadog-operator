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

2. Create a file with the spec of your `DatadogSLO` deployment configuration. The operator supports three SLO types: `metric`, `monitor`, and `time_slice`. Example configurations are shown below.

    **Metric SLO** — measures the ratio of good events to total events using metric queries:

    ```yaml
    apiVersion: datadoghq.com/v1alpha1
    kind: DatadogSLO
    metadata:
      name: example-metric-slo
      namespace: system
    spec:
      name: example-metric-slo
      description: "This is an example metric SLO from datadog-operator"
      type: "metric"
      query:
        numerator: "sum:requests.success{service:example,env:prod}.as_count()"
        denominator: "sum:requests.total{service:example,env:prod}.as_count()"
      tags:
        - "service:example"
        - "env:prod"
      targetThreshold: "99.9"
      timeframe: "7d"
    ```

    **Monitor SLO** — calculates SLO from the uptime of one or more existing Datadog monitors:

    ```yaml
    apiVersion: datadoghq.com/v1alpha1
    kind: DatadogSLO
    metadata:
      name: example-monitor-slo
      namespace: system
    spec:
      name: example-monitor-slo
      description: "This is an example monitor SLO from datadog-operator"
      type: "monitor"
      monitorIDs:
        - 12345678
      tags:
        - "service:example"
        - "env:prod"
      targetThreshold: "99.9"
      timeframe: "7d"
    ```

    **Time-slice SLO** — evaluates a metric query against a threshold at each time interval to determine what fraction of time the service was in a good state. This type requires a `timeSlice` block with a `query`, a `comparator` (`>`, `>=`, `<`, `<=`), and a numeric `threshold`:

    ```yaml
    apiVersion: datadoghq.com/v1alpha1
    kind: DatadogSLO
    metadata:
      name: example-time-slice-slo
      namespace: system
    spec:
      name: example-time-slice-slo
      description: "This is an example time-slice SLO from datadog-operator"
      type: "time_slice"
      timeSlice:
        query: "trace.servlet.request{env:prod}"
        comparator: ">"
        threshold: "5"
      tags:
        - "service:example"
        - "env:prod"
      targetThreshold: "97"
      timeframe: "7d"
    ```

    The operator wraps the `timeSlice.query` into the formula and named-query structure required by the Datadog API automatically — no manual formula configuration is needed.

3. Deploy the `DatadogSLO` with the above configuration file:

    ```shell
    kubectl apply -f /path/to/your/datadog-slo.yaml
    ```

    This automatically creates a new SLO in Datadog. You can find it on the [SLOs][8] page of your Datadog account.
    Datadog Operator occasionally reconciles and keeps SLOs in line with the given configuration. There is also a force 
    sync every hour, so if a user deletes an SLO in the Datadog UI, Datadog Operator restores it in under an hour.

By default, the Operator ensures that the API SLO definition stays in sync with the DatadogSLO resource every **60** minutes (per SLO). This interval can be adjusted using the environment variable `DD_SLO_FORCE_SYNC_PERIOD`, which specifies the number of minutes. For example, setting this variable to `"30"` changes the interval to 30 minutes.

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
