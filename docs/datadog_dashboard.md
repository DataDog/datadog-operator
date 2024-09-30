# Datadog Dashboards
This feature is in preview.

## Overview
The DatadogDashboard Custom Resource Definition (CRD) allows users to create [dashboards][1] using the Operator and manage them as Kubernetes resources.

## Prerequisites

- Datadog Operator v1.9+
- **[Helm][2]** for deploying the Datadog Operator
- **[`kubectl` CLI][3]** for installing a `DatadogDashboard`

Tests were performed on Kubernetes versions >= `v1.30.0`

## Adding a DatadogDashboard

To deploy a `DatadogDashboard` with the Datadog Operator, use the [`datadog-operator` Helm chart][4].

1. Install the [Datadog Operator][5]:

   First, add the Datadog Helm chart with

    ```shell
    helm repo add datadog https://helm.datadoghq.com
    ```

    1. Run the following command, substituting your [Datadog API and application keys][6]:

        ```shell
        helm install my-datadog-operator datadog/datadog-operator --set apiKey=<DATADOG_API_KEY> --set appKey=<DATADOG_APP_KEY> --set datadogDashboard.enabled=true
        ```

    1. Alternatively, update the [`values.yaml`][7] file of the Datadog Operator Helm chart to include your [Datadog API and application keys][6] and enable `DatadogDashboard`.
       Then, run

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


## Cleanup

The following commands delete the dashboard from your Datadog account and all the Kubernetes resources created by the above instructions:

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
