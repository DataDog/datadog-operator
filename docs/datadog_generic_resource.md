# Datadog GenericResource

This feature was introduced in Datadog Operator v1.12.0 and is currently in Preview.

## Overview

The `DatadogGenericResource` (DDGR) Custom Resource Definition allows users to create a variety of [Datadog resources](#supported-resources) using the Operator and manage them as Kubernetes resources. 

Example:
```yaml
apiVersion: datadoghq.com/v1alpha1
kind: DatadogGenericResource
metadata:
  name: browser-test-example
spec:
  type: synthetics_browser_test
  jsonSpec: |-
    {
      "config": {
        [...]
      },
      "locations": [
        "aws:us-east-2"
      ],
      "message": "lorem ipsum",
      "name": "Example Browser test (DatadogGenericResource)",
      "options": {
        [...]
      },
      "tags": [
        "foo:bar"
      ],
      "type": "browser",
      "steps": [
        [...]
      ]
    }
```

A `DatadogGenericResource` object has two fields:
* `type`: one of the [supported resources types](#supported-resources) (e.g. `synthetics_browser_test`)
* `jsonSpec`: JSON description of the resource you want to create.

## Supported Resources

| Type                      | Operator Version | Json template                                                           | Example manifest                                                                     |
|---------------------------|:----------------:|------------------------------------------------------------------------ |:------------------------------------------------------------------------------------:|
| `downtime`                | v1.14.0          | https://docs.datadoghq.com/api/latest/downtimes/#schedule-a-downtime    | [Downtime manifest](../examples/datadoggenericresource/downtime-sample.yaml)         |
| `notebook`                | v1.12.0          | https://docs.datadoghq.com/api/latest/notebooks/#create-a-notebook      | [Notebook manifest](../examples/datadoggenericresource/notebook-sample.yaml)         |
| `synthetics_api_test`     | v1.12.0          | https://docs.datadoghq.com/api/latest/synthetics/#create-an-api-test    | [API test manifest](../examples/datadoggenericresource/api-test-sample.yaml)         |
| `synthetics_browser_test` | v1.12.0          | https://docs.datadoghq.com/api/latest/synthetics/#create-a-browser-test | [Browser test manifest](../examples/datadoggenericresource/browser-test-sample.yaml) |
| `monitor`                 | v1.13.0          | https://docs.datadoghq.com/api/latest/monitors/#create-a-monitor        | [Monitor manifest](../examples/datadoggenericresource/monitor-sample.yaml)           |

## Prerequisites

* Datadog Operator v1.12.0+
* The [kubectl CLI][1], to install a `DatadogGenericResource`
* [API and application keys][2] (with the necessary scope) to create the desired resource

## Creating a DatadogGenericResource

To deploy a `DatadogGenericResource` with the Datadog Operator, follow the steps below:

1. Add the Datadog Helm chart repository using:
    ```shell
    helm repo add datadog https://helm.datadoghq.com
    ```

2. DDGR controller is disabled by default. It also requires an API and an application key. Choose one of the following options:
    * Override the default values by providing your [API and application keys][2] and enabling the controller:
      ```shell
      helm install datadog-operator datadog/datadog-operator --set apiKey=<DATADOG_API_KEY> --set appKey=<DATADOG_APP_KEY> --set datadogGenericResource.enabled=true
      ```
    * Create an override [`values.yaml`][3] file with your [API and application keys][2] and the `DatadogGenericResource` controller. Then run the install command:
      ```shell
      helm install datadog-operator datadog/datadog-operator -f values.yaml
      ```

3. Create a file with the spec of your `DatadogGenericResource` configuration. An example configuration is:
    ```yaml
    apiVersion: datadoghq.com/v1alpha1
    kind: DatadogGenericResource
    metadata:
      name: ddgr-notebook-sample
      namespace: <datadog-operator-namespace>
    spec:
      type: notebook
      jsonSpec: |-
        {
          "data": {
            "attributes": {
              "cells": [
                {
                  "attributes": {
                    "definition": {
                      "text": "## Some test markdown\n\n```js\nvar x, y;\nx = 5;\ny = 6;\n```",
                      "type": "markdown"
                    }
                  },
                  "type": "notebook_cells"
                },
                {
                  "attributes": {
                    "definition": {
                      "requests": [
                        {
                          "display_type": "line",
                          "q": "avg:system.load.1{*}",
                          "style": {
                            "line_type": "solid",
                            "line_width": "normal",
                            "palette": "dog_classic"
                          }
                        }
                      ],
                      "show_legend": true,
                      "type": "timeseries",
                      "yaxis": {
                        "scale": "linear"
                      }
                    },
                    "graph_size": "m",
                    "split_by": {
                      "keys": [],
                      "tags": []
                    },
                    "time": null
                  },
                  "type": "notebook_cells"
                }
              ],
              "name": "Example-Notebook",
              "status": "published",
              "time": {
                "live_span": "1h"
              }
            },
            "type": "notebooks"
          }
        }
    ```

4. Deploy the `DatadogGenericResource`:
    ```shell
    kubectl apply -f /path/to/your/datadog-generic-resource.yaml
    ```
    This creates a new notebook in Datadog: it is available on the [Notebooks](4) page.

5. (Cleanup) Delete the Kubernetes resource to remove the resource from your account:
    ```shell
    kubectl delete datadoggenericresource ddgr-notebook-sample
    ```

Further example manifests are provided [in the supported resources table](#supported-resources).


## Comparison with existing CRDs

The Datadog Operator continues to support specific-resource CRDs:
* [`DatadogMonitor`](./datadog_monitor.md)
* [`DatadogDashboard`](./datadog_dashboard.md)
* `DatadogSLO`

Some of these resources are (or will be) also supported by the `DatadogGenericResource` controller with the same possible operations: create, update and delete.

At the expense of more verbose manifests, this new controller provides:
* Easier maintability: when Datadog APIs evolve to support additional features (e.g. a new type of monitor), the resource-specific controller requires a Custom Resource Definition update with the mapping of the new fields. On the other hand, the DDGR controller only requires a version of the [underlying Go client][5] that supports these new fields.
* Expandability: adding support for a new type of resource requires little turnaround by following [the development steps](./datadog_generic_resource_dev.md).

[1]: https://kubernetes.io/docs/tasks/tools/#kubectl
[2]: https://docs.datadoghq.com/account_management/api-app-keys/
[3]: https://github.com/DataDog/helm-charts/blob/main/charts/datadog-operator/values.yaml
[4]: https://app.datadoghq.com/notebook/list
[5]: https://github.com/DataDog/datadog-api-client-go
