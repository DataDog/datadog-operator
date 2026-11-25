# Datadog GenericResource

## Overview

The `DatadogGenericResource` (DDGR) Custom Resource Definition allows users to create a variety of [Datadog resources](#supported-resources) using the Operator and manage them as Kubernetes resources.

DDGR is the preferred Custom Resource Definition for Datadog resources that are supported by both DDGR and older resource-specific CRDs, such as `DatadogMonitor`, `DatadogDashboard`, and `DatadogSLO`. Existing resource-specific CRDs remain supported, but new Datadog API features are expected to be easier to consume through DDGR because `jsonSpec` follows the Datadog API payload directly.

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
* `type`: one of the [supported resource types](#supported-resources) (e.g. `synthetics_browser_test`)
* `jsonSpec`: JSON description of the resource you want to create.

(Optional): To migrate existing `DatadogMonitor`, `DatadogDashboard`, or `DatadogSLO` resources to DDGR, see the [migration guide](./datadog_generic_resource_migration.md).

## Supported Resources

| Type                      | Operator Version | Json template                                                                         | Example manifest                                                                     |
|---------------------------|:----------------:|---------------------------------------------------------------------------------------|:------------------------------------------------------------------------------------:|
| `notebook`                | v1.12.0          | https://docs.datadoghq.com/api/latest/notebooks/#create-a-notebook                    | [Notebook manifest](../../examples/datadoggenericresource/notebook-sample.yaml)         |
| `synthetics_api_test`     | v1.12.0          | https://docs.datadoghq.com/api/latest/synthetics/#create-an-api-test                  | [API test manifest](../../examples/datadoggenericresource/api-test-sample.yaml)         |
| `synthetics_browser_test` | v1.12.0          | https://docs.datadoghq.com/api/latest/synthetics/#create-a-browser-test               | [Browser test manifest](../../examples/datadoggenericresource/browser-test-sample.yaml) |
| `monitor`                 | v1.13.0          | https://docs.datadoghq.com/api/latest/monitors/#create-a-monitor                      | [Monitor manifest](../../examples/datadoggenericresource/monitor-sample.yaml)           |
| `downtime`                | v1.22.0          | https://docs.datadoghq.com/api/latest/downtimes/#schedule-a-downtime                  | [Downtime manifest](../../examples/datadoggenericresource/downtime-sample.yaml)         |
| `dashboard`               | v1.27.0          | https://docs.datadoghq.com/api/latest/dashboards/#create-a-dashboard                  | [Dashboard manifest](../../examples/datadoggenericresource/dashboard-sample.yaml)       |
| `slo`                     | v1.28.0          | https://docs.datadoghq.com/api/latest/service-level-objectives/#create-an-slo-object  | [SLO manifest](../../examples/datadoggenericresource/slo-sample.yaml)                   |
| `monitor_notification_rule` | v1.30.0        | https://docs.datadoghq.com/api/latest/monitors/create-a-monitor-notification-rule/    | [Notification rule manifest](../../examples/datadoggenericresource/notification-rule-sample.yaml) |
| `slo_correction`          | v1.31.0          | https://docs.datadoghq.com/api/latest/service-level-objective-corrections/#create-an-slo-correction | [SLO correction manifest](../../examples/datadoggenericresource/slo-correction-sample.yaml) |
| `software_catalog_entity` | v1.32.0          | https://docs.datadoghq.com/api/latest/software-catalog/#create-or-update-entity      | [Software Catalog entity manifest](../../examples/datadoggenericresource/software-catalog-entity-sample.yaml) |

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

    For **OLM deployments**, enable the controller via environment variable in the `Subscription`:
    ```yaml
    config:
      env:
        - name: DD_GENERIC_RESOURCE_CONTROLLER_ENABLED
          value: "true"
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
    This creates a new notebook in Datadog: it is available on the [Notebooks][4] page.

5. (Cleanup) Delete the Kubernetes resource to remove the resource from your account:
    ```shell
    kubectl delete datadoggenericresource ddgr-notebook-sample
    ```

    To keep the resource in your account, add the `datadoghq.com/management-policy: orphan` annotation before deleting. See [Stop managing a Datadog resource](../management_policy.md).

Further example manifests are provided [in the supported resources table](#supported-resources).

## Adopting an existing Datadog resource

By default, a `DatadogGenericResource` creates a new Datadog object and stores its ID in `.status.id`. To manage an object that already exists instead, for example one created in the Datadog UI, with the API, with Terraform, or by a `DatadogMonitor`, `DatadogDashboard`, or `DatadogSLO`, set its ID in `spec.id`. The object keeps its ID, its history, and every reference to it, such as composite monitors, SLOs, and dashboard links.

```yaml
apiVersion: datadoghq.com/v1alpha1
kind: DatadogGenericResource
metadata:
  name: checkout-latency
spec:
  type: monitor
  id: "123456789"
  jsonSpec: |-
    {
      "name": "Checkout latency",
      "type": "query alert",
      "query": "avg(last_5m):avg:trace.http.request.duration{service:checkout} > 0.5",
      "message": "Checkout latency is high @slack-checkout",
      "tags": ["team:checkout"],
      "options": {"thresholds": {"critical": 0.5}}
    }
```

`jsonSpec` is the source of truth: the Operator reads the object to check that it exists, records the ID in `.status.id` with an `Adopted` condition and event, then writes `jsonSpec` to the object. Build `jsonSpec` from the current definition of the object, as described in the [migration guide](datadog_generic_resource_migration.md#export-the-datadog-definition), so that the first update doesn't change it unexpectedly.

```shell
kubectl apply -f checkout-latency.yaml
kubectl wait --for=condition=Adopted datadoggenericresource/checkout-latency
```

While `spec.id` is set, the Operator **never creates a replacement object**:

| Situation | Behavior |
| --- | --- |
| The object doesn't exist, or `spec.type` doesn't match it | Error condition with reason `AdoptionTargetNotFound`. Nothing is created. The Operator retries at the force sync period. |
| The API keys can't read the object (401, 403) | Error condition with reason `AdoptionForbidden`. Nothing is created. |
| `spec.id` has the wrong format for `spec.type` | Error condition with reason `AdoptionInvalidID`, and no Datadog API call until the spec changes. Monitor and notebook IDs are integers, dashboard and synthetic test IDs look like `abc-def-ghi`, downtime and monitor notification rule IDs are UUIDs. |
| `spec.type` is `software_catalog_entity` | Error condition with reason `AdoptionUnsupportedType`. Entities are upserted by kind, namespace and name, so a `DatadogGenericResource` without `spec.id` already updates an existing entity. |
| Another `DatadogGenericResource` in the cluster manages or adopts the same object | Error condition with reason `AdoptionConflict`, naming the other resource. A resource that already manages the object keeps it. Between two resources waiting to adopt it, the oldest one adopts it. |
| A `DatadogMonitor`, `DatadogDashboard`, or `DatadogSLO` in the cluster has the same ID in its `.status.id`, even if its controller is disabled or it is being deleted | Error condition with reason `AdoptionLegacyOwner`, naming the legacy resource. Nothing happens to the object. Annotate the legacy resource with `datadoghq.com/management-policy=orphan` and delete it: the adoption goes ahead on the next retry, within the requeue period (60 seconds by default). |
| The Operator can't list `datadogmonitors`, `datadogdashboards`, or `datadogslos` | Error condition with reason `AdoptionLegacyCheckError`, naming the missing permission. The adoption waits, because the Operator can't check that no legacy resource manages the object. |
| The adopted object is deleted in Datadog | Error condition with reason `AdoptedResourceNotFound`. The object is not recreated: restore it, or remove `spec.id` to let the Operator create a new one. |
| `spec.id` is changed after the adoption | Error condition with reason `IDMismatch`, and no Datadog API call. Revert the change, or delete and recreate the resource. |
| `spec.id` is removed after the adoption | The Operator keeps managing the same object, but a later deletion in Datadog recreates it with a new ID. |
| `spec.id` is added to a resource that already manages the object, with the ID in `.status.id` | Accepted with no Datadog API call. This lets you write the ID back into your manifests. |

Deleting an adopted resource deletes the Datadog object, like any other `DatadogGenericResource`. To keep it, use the [orphan management policy](../management_policy.md).

Things to know before adopting:

* The check against legacy resources uses a list of each legacy kind taken once per Operator start, when the first resource of that type is adopted. A legacy resource that only gets its ID afterwards isn't detected until the next restart.
* Only one manager should write to an object. Remove the object from the previous manager first: for Terraform, run `terraform state rm` or use a `removed` block with `lifecycle { destroy = false }`; for a `DatadogMonitor`, use [`kubectl datadog monitor migrate`](../kubectl-plugin.md#monitor-migrate), which does the whole migration; for a `DatadogDashboard` or `DatadogSLO`, delete it with the [orphan management policy](../management_policy.md) so that the object isn't deleted. Managers in other clusters or tools are not detected.
* Datadog UI edits are reverted at each force sync, as for any `DatadogGenericResource`.
* If you downgrade the Operator, finish adoptions first. An older CRD drops `spec.id`, so a resource that isn't adopted yet (empty `.status.id`) would create a new object.

## Controller tuning

The `DatadogGenericResource` controller exposes several tuning options for large installations or load tests.

| Option | Default | Description |
| --- | --- | --- |
| `DD_GENERIC_RESOURCE_FORCE_SYNC_PERIOD` | `60` minutes | Interval, in minutes, for checking that the Datadog API resource definition still matches the Kubernetes `DatadogGenericResource`. For example, `"30"` changes the interval to 30 minutes. `monitor` resources are only updated when the monitor in Datadog changed since the Operator last wrote it (tracked in `status.remoteDefinitionHash`). Monitor force syncs and state polls send the ETag of the last such read (`status.etag`) in `If-None-Match`, so Datadog answers `304 Not Modified` without a body while the monitor, including its state, is unchanged. This comparison covers the monitor definition and every top-level `jsonSpec` field, so keep response-only fields such as `state`, `overall_state`, or `matching_downtimes` out of `jsonSpec`: they change without any edit and cause unnecessary updates. Monitors are updated on every check when `jsonSpec` sets `restriction_policy` or a field that Datadog does not return. Other resource types are updated on every check. |
| `--datadogGenericResourceRequeuePeriod` / `DD_GENERIC_RESOURCE_REQUEUE_PERIOD` | `60s` | Scheduled requeue interval for each `DatadogGenericResource` after a successful reconcile. On idle requeues, the controller also polls Datadog-side live state for resource types that expose it, currently `monitor` and `slo`. Accepts Go duration strings such as `30s` or `5m`. |
| `DD_STATE_POLLING_ENABLED` | `true` | Polls live state for `monitor` and `slo` resources (also applies to `DatadogMonitor`). Polling can hit Datadog API rate limits and slow down other operations, so enable it only when you need `.status.state`. When `false`, `.status.state` is `PollingDisabled` and the `StateSynced` condition is `False`. Set it explicitly: the default will change to `false`. |
| `--datadogGenericResourceMaxConcurrentReconciles` / `DD_GENERIC_RESOURCE_MAX_CONCURRENT_RECONCILES` | `1` | Maximum number of `DatadogGenericResource` objects that the controller reconciles at the same time. |

Increasing `--datadogGenericResourceMaxConcurrentReconciles` can improve throughput when creating, updating, deleting, or periodically syncing many resources. The tradeoff is higher Operator CPU usage and more concurrent requests to the Datadog API. Setting this value too high can increase the likelihood of Datadog API rate limits, especially when many resources reconcile at once or when the requeue interval is short.

Lowering `DD_GENERIC_RESOURCE_REQUEUE_PERIOD` or `--datadogGenericResourceRequeuePeriod` makes all `DatadogGenericResource` objects reconcile more often. For `monitor` and `slo` resources, it also keeps `.status.state` fresher. The tradeoff is more Operator work and, for requeues that call the Datadog API, more API traffic. Raising the interval reduces polling overhead at the cost of slower periodic reconciliation and less frequent state updates.

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

The controller requeues every `DatadogGenericResource` roughly every 60 seconds by default. This interval is controlled by `DD_GENERIC_RESOURCE_REQUEUE_PERIOD` or the `--datadogGenericResourceRequeuePeriod` manager flag, with the CLI flag taking precedence when both are set. For `monitor` and `slo` resources, these idle requeues refresh `state`. For resource types without live state, the state fields remain empty. Status polling requeues have lower priority than normal create, update, and delete work, so Datadog-side state updates may be delayed when the controller queue is busy. This keeps management operations ahead of background state polling, but means `.status.state` is eventually consistent rather than immediate.

Failures are visible only through the `StateSynced` condition. They do not break the reconcile loop and the last-known `state` is retained until a subsequent refresh succeeds.

This information is currently surfaced for `monitor` and `slo` resources. Resource types that do not expose live Datadog-side state (e.g., `dashboard`, `notebook`) leave these fields empty.

## Comparison with existing CRDs

The Datadog Operator continues to support specific-resource CRDs:
* [`DatadogMonitor`](../datadog_monitor.md)
* [`DatadogDashboard`](../datadog_dashboard.md)
* [`DatadogSLO`](../datadog_slo.md)

All of these resources are also supported by the `DatadogGenericResource` controller with the same possible operations: create, update and delete.

At the expense of more verbose manifests, this new controller provides:
* Easier maintainability: when Datadog APIs evolve to support additional features (e.g. a new type of monitor), the resource-specific controller requires a Custom Resource Definition update with the mapping of the new fields. On the other hand, the DDGR controller only requires a version of the [underlying Go client][5] that supports these new fields.
* Expandability: adding support for a new type of resource requires little turnaround by following [the development steps](./datadog_generic_resource_dev.md).

[1]: https://kubernetes.io/docs/tasks/tools/#kubectl
[2]: https://docs.datadoghq.com/account_management/api-app-keys/
[3]: https://github.com/DataDog/helm-charts/blob/main/charts/datadog-operator/values.yaml
[4]: https://app.datadoghq.com/notebook/list
[5]: https://github.com/DataDog/datadog-api-client-go
