# Datadog Dashboards

This feature is in Preview.

## Overview

The `DatadogDashboard` Custom Resource Definition (CRD) allows users to create [dashboards][1] using the Operator and manage them as Kubernetes resources.

## API Versions

The DatadogDashboard CRD supports two API versions:

- **v1alpha1**: Legacy version using JSON string format for widgets (deprecated)
- **v1alpha2**: Current version using native YAML array format for widgets (recommended)

### Key Differences

| Feature | v1alpha1 | v1alpha2 |
|---------|----------|----------|
| Widget Format | JSON string | Native YAML array |
| Readability | Limited | High |
| Validation | Basic | Enhanced |
| Maintainability | Difficult | Easy |

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

    - Run the install command, substituting your [Datadog API and application keys][6]:

        ```shell
        helm install my-datadog-operator datadog/datadog-operator --set apiKey=<DATADOG_API_KEY> --set appKey=<DATADOG_APP_KEY> --set datadogDashboard.enabled=true
        ```

    - Create an override [`values.yaml`][7] file that includes your [Datadog API and application keys][6] and enables the `DatadogDashboard` controller. Then run the install command:

        ```shell
        helm install my-datadog-operator datadog/datadog-operator -f values.yaml
        ```

## v1alpha2 Configuration (Recommended)

Create a file with the spec of your `DatadogDashboard` deployment configuration using the v1alpha2 API with native YAML widgets:

```yaml
apiVersion: datadoghq.com/v1alpha2
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
  widgets:
    - definition:
        type: timeseries
        title: "CPU Usage"
        title_size: "16"
        title_align: left
        show_legend: true
        legend_layout: auto
        legend_columns:
          - avg
          - min
          - max
          - value
          - sum
        requests:
          - formulas:
              - formula: query1
            queries:
              - name: query1
                data_source: metrics
                query: "avg:system.cpu.user{*} by {host}"
            response_format: timeseries
            style:
              palette: dog_classic
              order_by: values
              line_type: solid
              line_width: normal
            display_type: line
      layout:
        x: 0
        y: 0
        width: 4
        height: 2
```

## v1alpha1 Configuration (Legacy)

For backward compatibility, the v1alpha1 API is still supported but deprecated:

```yaml
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

## Migration from v1alpha1 to v1alpha2

To migrate from v1alpha1 to v1alpha2:

1. **Change the API version**:

   ```yaml
   apiVersion: datadoghq.com/v1alpha2  # Changed from v1alpha1
   ```

2. **Convert JSON string widgets to YAML array**:
   - Remove the JSON string format from the `widgets` field
   - Convert each widget object to native YAML format
   - Ensure proper indentation and structure

3. **Benefits of migration**:
   - **Improved readability**: Native YAML is easier to read and understand
   - **Better validation**: Enhanced error messages and validation
   - **Easier maintenance**: Standard YAML editing tools and syntax highlighting
   - **Version control friendly**: Better diff visualization in Git

4. **Example conversion**:

   **Before (v1alpha1)**:

   ```yaml
   widgets: '[{"definition": {"type": "timeseries", "title": "CPU"}}]'
   ```

   **After (v1alpha2)**:

   ```yaml
   widgets:
     - definition:
         type: timeseries
         title: CPU
   ```

## Deployment

Deploy the `DatadogDashboard` with the configuration file:

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
