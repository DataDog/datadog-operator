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

2. Create a file with the spec of your `DatadogSLO` deployment configuration. An example configuration is:


    ```
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

3. Deploy the `DatadogSLO` with the above configuration file:

    ```shell
    kubectl apply -f /path/to/your/datadog-slo.yaml
    ```

    This automatically creates a new SLO in Datadog. You can find it on the [SLOs][8] page of your Datadog account.
    Datadog Operator will occasionally reconcile and keep SLOs in line with the given configuration. There is also a force 
    sync every hour, so if a SLO is deleted in the Datadog UI, it will be back up in under an hour.

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
