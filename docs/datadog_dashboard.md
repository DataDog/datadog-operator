# Datadog Dashboards
This feature is in Preview.

## Overview
The `DatadogDashboard` Custom Resource Definition (CRD) allows users to create [dashboards][1] using the Operator and manage them as Kubernetes resources.

## Prerequisites

- Datadog Operator v1.9+
- [Helm][2], to deploy the Datadog Operator
- The [kubectl CLI][3], to install a `DatadogDashboard`

## Adding a DatadogDashboard

To deploy a `DatadogDashboard` with the Datadog Operator, use the [`datadog-operator` Helm chart][4].

1. To install the [Datadog Operator][5], first add the Datadog Helm chart using the following command:

    ```shell
    helm repo add datadog https://helm.datadoghq.com
    ```

1. Choose one of the following options:

    * Run the install command, substituting your [Datadog API and application keys][6]:

        ```shell
        helm install my-datadog-operator datadog/datadog-operator --set apiKey=<DATADOG_API_KEY> --set appKey=<DATADOG_APP_KEY> --set datadogDashboard.enabled=true
        ```

    * Create an override [`values.yaml`][7] file that includes your [Datadog API and application keys][6] and enables the `DatadogDashboard` controller. Then run the install command:

        ```shell
        helm install my-datadog-operator datadog/datadog-operator -f values.yaml
        ```

2. Create a file with the spec of your `DatadogDashboard` deployment configuration. An example configuration is:


    ```
    apiVersion: datadoghq.com/v1alpha1
    kind: DatadogDashboard
    metadata:
      name: example-dashboard
    spec:
      title: Test Dashboard
      layoutType: ordered
      tags:
        - "team:my_team"
      templateVariables:
        - availableValues:
            - host1
            - host2
            - host3
          name: first
          prefix: bar-foo
      notifyList:
        - foobar@example.com
      widgets: '[{
                "id": 2639892738901474,
                "definition": {
                    "title": "",
                    "title_size": "16",
                    "title_align": "left",
                    "show_legend": true,
                    "legend_layout": "auto",
                    "legend_columns": [
                        "avg",
                        "min",
                        "max",
                        "value",
                        "sum"
                    ],
                    "type": "timeseries",
                    "requests": [
                        {
                            "formulas": [
                                {
                                    "formula": "query1"
                                }
                            ],
                            "queries": [
                                {
                                    "name": "query1",
                                    "data_source": "metrics",
                                    "query": "avg:system.cpu.user{*} by {host}"
                                }
                            ],
                            "response_format": "timeseries",
                            "style": {
                                "palette": "dog_classic",
                                "order_by": "values",
                                "line_type": "solid",
                                "line_width": "normal"
                            },
                            "display_type": "line"
                        }
                    ]
                },
                "layout": {
                    "x": 0,
                    "y": 0,
                    "width": 4,
                    "height": 2
                }
             }]'
    ```

3. Deploy the `DatadogDashboard` with the above configuration file:

    ```shell
    kubectl apply -f /path/to/your/datadog-dashboard.yaml
    ```

    This automatically creates a new dashboard in Datadog. You can find it on the [Dashboards][8] page of your Datadog account.

By default, the Operator ensures that the API dashboard definition stays in sync with the DatadogDashboard resource every **60** minutes (per dashboard). This interval can be adjusted using the environment variable `DD_DASHBOARD_FORCE_SYNC_PERIOD`, which specifies the number of minutes. For example, setting this variable to `"30"` changes the interval to 30 minutes.


## Cleanup

The following commands delete the dashboard from your Datadog account as well as all of the Kubernetes resources created by the previous instructions:

```shell
kubectl delete datadogdashboard example-dashboard
helm delete my-datadog-operator
```


[1]: https://docs.datadoghq.com/dashboards/
[2]: https://helm.sh
[3]: https://kubernetes.io/docs/tasks/tools/install-kubectl/
[4]: https://github.com/DataDog/helm-charts/tree/main/charts/datadog-operator
[5]: https://artifacthub.io/packages/helm/datadog/datadog-operator
[6]: https://app.datadoghq.com/account/settings#api
[7]: https://github.com/DataDog/helm-charts/blob/main/charts/datadog-operator/values.yaml
[8]: https://app.datadoghq.com/dashboard/lists
