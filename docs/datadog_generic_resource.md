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

| Type                      | Operator Version | Json template                                                                         | Example manifest                                                                     |
|---------------------------|:----------------:|---------------------------------------------------------------------------------------|:------------------------------------------------------------------------------------:|
| `notebook`                | v1.12.0          | https://docs.datadoghq.com/api/latest/notebooks/#create-a-notebook                    | [Notebook manifest](../examples/datadoggenericresource/notebook-sample.yaml)         |
| `synthetics_api_test`     | v1.12.0          | https://docs.datadoghq.com/api/latest/synthetics/#create-an-api-test                  | [API test manifest](../examples/datadoggenericresource/api-test-sample.yaml)         |
| `synthetics_browser_test` | v1.12.0          | https://docs.datadoghq.com/api/latest/synthetics/#create-a-browser-test               | [Browser test manifest](../examples/datadoggenericresource/browser-test-sample.yaml) |
| `monitor`                 | v1.13.0          | https://docs.datadoghq.com/api/latest/monitors/#create-a-monitor                      | [Monitor manifest](../examples/datadoggenericresource/monitor-sample.yaml)           |
| `downtime`                | v1.22.0          | https://docs.datadoghq.com/api/latest/downtimes/#schedule-a-downtime                  | [Downtime manifest](../examples/datadoggenericresource/downtime-sample.yaml)         |
| `dashboard`               | v1.27.0          | https://docs.datadoghq.com/api/latest/dashboards/#create-a-dashboard                  | [Dashboard manifest](../examples/datadoggenericresource/dashboard-sample.yaml)       |
| `slo`                     | v1.28.0          | https://docs.datadoghq.com/api/latest/service-level-objectives/#create-an-slo-object  | [SLO manifest](../examples/datadoggenericresource/slo-sample.yaml)                   |

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

2. The DDGR controller and its CRD are both disabled by default. The controller also requires an API and an application key. Choose one of the following options:
    * Override the default values by providing your [API and application keys][2], installing the CRD, and enabling the controller:
      ```shell
      helm install datadog-operator datadog/datadog-operator \
        --set apiKey=<DATADOG_API_KEY> \
        --set appKey=<DATADOG_APP_KEY> \
        --set datadogCRDs.crds.datadogGenericResources=true \
        --set datadogGenericResource.enabled=true
      ```
      Both flags are required: `datadogCRDs.crds.datadogGenericResources=true` installs the `DatadogGenericResource` CRD, and `datadogGenericResource.enabled=true` starts the controller that reconciles it.
    * Create an override [`values.yaml`][3] file with your [API and application keys][2], `datadogCRDs.crds.datadogGenericResources: true`, and `datadogGenericResource.enabled: true`. Then run the install command:
      ```shell
      helm install datadog-operator datadog/datadog-operator -f values.yaml
      ```

   By default, the Operator only watches its own namespace, so it will manage any `DatadogGenericResource` objects within its own namespace. To deploy `DatadogGenericResource` objects in other namespaces, configure the Operator [`watchNamespaces`][3] section with those namespaces. The DDGR controller can also be scoped independently with the `DD_GENERIC_RESOURCE_WATCH_NAMESPACE` environment variable, which takes a comma-separated list of namespaces and falls back to `WATCH_NAMESPACE` when unset.

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

## Controller tuning

The `DatadogGenericResource` controller exposes several tuning options for large installations or load tests.

| Option | Default | Description |
| --- | --- | --- |
| `DD_GENERIC_RESOURCE_FORCE_SYNC_PERIOD` | `60` minutes | Interval, in minutes, for checking that the Datadog API resource definition still matches the Kubernetes `DatadogGenericResource`. For example, `"30"` changes the interval to 30 minutes. |
| `DD_GENERIC_RESOURCE_REQUEUE_PERIOD` | `60s` | Scheduled requeue interval for each `DatadogGenericResource` after a successful reconcile. On these idle requeues, the controller also polls Datadog-side live state for resource types that expose it, currently `monitor` and `slo`. Accepts Go duration strings such as `30s`, `5m`, or a plain integer interpreted as seconds. The minimum value is `1s`. This can also be set with the `--datadogGenericResourceRequeuePeriod` manager flag. |
| `--datadogGenericResourceMaxConcurrentReconciles` | `1` | Maximum number of `DatadogGenericResource` objects the controller reconciles at the same time. |

Increasing `--datadogGenericResourceMaxConcurrentReconciles` can improve throughput when creating, updating, deleting, or periodically syncing many resources. The tradeoff is higher Operator CPU usage and more concurrent requests to the Datadog API. Setting this too high can make Datadog API rate limits more likely, especially when many resources reconcile at once or when the requeue interval is short.

Lowering `DD_GENERIC_RESOURCE_REQUEUE_PERIOD` makes all DDGR objects reconcile more often. For `monitor` and `slo` resources, it also makes `.status.state` fresher. The tradeoff is more Operator work and, for requeues that call the Datadog API, more API traffic. Raising it reduces polling overhead at the cost of slower periodic reconciliation and less frequent state updates.

## Datadog-side status

For resource types that expose a live state in the Datadog backend, the controller reflects that state into the `DatadogGenericResource` `.status` so it can be inspected directly from `kubectl` without leaving the cluster:

| Field | Description |
| --- | --- |
| `.status.state` | Live state as reported by Datadog. Values are resource-type dependent. For Monitors: `OK`, `Alert`, `Warn`, `No Data`, `Skipped`, `Ignored`, `Unknown`. For SLOs: `breached`, `warning`, `ok`, `no_data`. |
| `.status.stateLastUpdateTime` | Last time `state` was successfully refreshed from the Datadog API. |
| `.status.stateLastTransitionTime` | Last time `state` changed value. |
| `.status.conditions[type=StateSynced]` | `True` after a successful state refresh; `False` with `reason=GetError` when the most recent refresh failed (last-known `state` is preserved). |

Inspect quickly via:

```shell
kubectl get datadoggenericresource    # shows state and last state sync columns
kubectl wait --for=condition=StateSynced datadoggenericresource/<name>
```

The controller requeues every `DatadogGenericResource` roughly every 60 seconds by default. This interval is controlled by `DD_GENERIC_RESOURCE_REQUEUE_PERIOD` or the `--datadogGenericResourceRequeuePeriod` manager flag. For `monitor` and `slo` resources, these idle requeues refresh `state`. For resource types without live state, the state fields remain empty. Status polling requeues have lower priority than normal create, update, and delete work, so Datadog-side state updates may be delayed when the controller queue is busy. This keeps management operations ahead of background state polling, but means `.status.state` is eventually consistent rather than immediate.

Failures are visible only through the `StateSynced` condition. They do not break the reconcile loop and the last-known `state` is retained until a subsequent refresh succeeds.

This information is currently surfaced for `monitor` and `slo` resources. Resource types that do not expose live Datadog-side state (e.g., `dashboard`, `notebook`) leave these fields empty.

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
