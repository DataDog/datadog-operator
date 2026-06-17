# Migrating to DatadogGenericResource

This guide explains how to migrate resources managed by `DatadogMonitor`, `DatadogDashboard`, or `DatadogSLO` to `DatadogGenericResource` (DDGR).

DDGR uses the JSON payload accepted by the Datadog API for each resource type. The Operator stores the Datadog object ID in `.status.id` after creation, so do not put an existing Datadog ID in `spec.jsonSpec`. When migrating an existing resource, export its Datadog API definition, remove response-only fields such as IDs and timestamps, and apply the remaining create/update payload as DDGR `jsonSpec`.

## Before you migrate

* Enable the DDGR CRD and controller with `datadogCRDs.crds.datadogGenericResources=true` and `datadogGenericResource.enabled=true`.
* Keep the original Kubernetes resource until you have validated the new DDGR-managed Datadog resource.
* Expect DDGR to create a new Datadog object with a new Datadog ID. DDGR does not adopt an existing object by copying its ID into the manifest.
* Plan any external references that use Datadog IDs. For example, composite monitors and SLOs that reference monitor IDs must be updated if the migration creates replacement monitors.

## Export the Datadog definition

Use the Datadog API to retrieve the existing object definition. The examples below assume `jq` is installed.

```shell
# https://docs.datadoghq.com/getting_started/site/
export DD_SITE=datadoghq.com
export DD_API_KEY=<DATADOG_API_KEY>
export DD_APP_KEY=<DATADOG_APP_KEY>
```

For monitors:

```shell
export MONITOR_ID=<MONITOR_ID>

curl -sS \
  -H "DD-API-KEY: ${DD_API_KEY}" \
  -H "DD-APPLICATION-KEY: ${DD_APP_KEY}" \
  "https://api.${DD_SITE}/api/v1/monitor/${MONITOR_ID}" \
  | jq 'del(.id, .created, .created_at, .draft_status, .overall_state_modified, .org_id, .creator, .deleted, .matching_downtimes, .modified, .multi, .overall_state, .state)'
```

For dashboards:

```shell
export DASHBOARD_ID=<DASHBOARD_ID>

curl -sS \
  -H "DD-API-KEY: ${DD_API_KEY}" \
  -H "DD-APPLICATION-KEY: ${DD_APP_KEY}" \
  "https://api.${DD_SITE}/api/v1/dashboard/${DASHBOARD_ID}" \
  | jq 'del(.id, .author_handle, .author_name, .created_at, .modified_at, .url)'
```

For SLOs:

```shell
export SLO_ID=<SLO_ID>

curl -sS \
  -H "DD-API-KEY: ${DD_API_KEY}" \
  -H "DD-APPLICATION-KEY: ${DD_APP_KEY}" \
  "https://api.${DD_SITE}/api/v1/slo/${SLO_ID}" \
  | jq '.data | del(.id, .configured_alert_ids, .created_at, .creator, .modified_at, .monitor_tags)'
```

Review the generated JSON before applying it. Remove any other fields that are returned by the API only for IDs, status, history, authorship, fields you do not desire to set. Keep configuration fields that are part of the create/update API payload, such as monitor `query`, `type`, `message`, `options`, and `tags`; dashboard `title`, `layout_type`, `widgets`, and template variables; and SLO `name`, `type`, `thresholds`, `query`, `sli_specification`, `monitor_ids`, `groups`, and `tags`.

## Create the DDGR manifest

Embed the cleaned JSON as `spec.jsonSpec` and set `spec.type` to the matching DDGR resource type.

```yaml
apiVersion: datadoghq.com/v1alpha1
kind: DatadogGenericResource
metadata:
  name: example-monitor
  namespace: <datadog-operator-namespace>
spec:
  type: monitor
  jsonSpec: |-
    {
      "name": "Example Monitor",
      "type": "metric alert",
      "query": "avg(last_10m):avg:system.cpu.user{*} > 80",
      "message": "CPU usage is high",
      "tags": [
        "team:example"
      ],
      "options": {
        "notify_no_data": false
      }
    }
```

Use `type: dashboard` for dashboards and `type: slo` for SLOs. Additional examples are available in [`examples/datadoggenericresource`](../examples/datadoggenericresource).

## Apply and verify

Apply the new DDGR manifest:

```shell
kubectl apply -f /path/to/datadog-generic-resource.yaml
```

Wait for the Operator to create and sync the resource:

```shell
kubectl get datadoggenericresource
kubectl wait --for=condition=Created datadoggenericresource/<name>
```

Compare the new Datadog resource with the original one in Datadog. For monitors and SLOs, DDGR also refreshes `.status.state` so you can verify the live Datadog-side state from Kubernetes:

```shell
kubectl get datadoggenericresource <name>
```

## Retire the old resource

After the DDGR-managed resource is validated, remove the old Kubernetes resource. Deleting a `DatadogMonitor`, `DatadogDashboard`, or `DatadogSLO` normally deletes the Datadog object tracked by that resource's status ID. Do this only after confirming that the new DDGR-managed object is the one you want to keep.

```shell
kubectl delete datadogmonitor <old-name>
kubectl delete datadogdashboard <old-name>
kubectl delete datadogslo <old-name>
```

If other Datadog objects referenced the old object's ID, update those references to the new DDGR-managed Datadog ID shown in `.status.id`.
