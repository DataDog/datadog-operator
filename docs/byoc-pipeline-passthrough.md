# BYOC pipelines: optional and `pipelineConfig` passthrough

Dev-only note. Not linked from other docs.

## Why pipelines are optional

A `DatadogBYOCCluster` can come up before any pipeline exists: `components.pipelines`
has no `required` and no `minItems` (already true on main). Pipelines can be added later.

## Why `pipelineConfig` passes through

`components.pipelines[].pipelineConfig` is copied as-is to the child
`DatadogObservabilityPipelinesWorker`. The Worker runs that YAML locally, so a pipeline can be
pre-baked without Remote Config or the Observability Pipelines UI.

`pipelineID` is optional when `pipelineConfig` is set: the Worker reports its own UID instead.
With neither set, the cluster is invalid (`pipelines[i].pipelineID` is required).

## Example

```yaml
apiVersion: datadoghq.com/v1alpha1
kind: DatadogBYOCCluster
metadata:
  name: byoc
spec:
  signal: logs
  components:
    pipelines:
      - name: logs
        pipelineConfig: |
          sources:
            agent:
              type: datadog_agent
              address: 0.0.0.0:8282
          destinations:
            cloudprem:
              type: cloudprem
              inputs: [agent]
              endpoint_url: ${DD_OP_DESTINATION_CLOUDPREM_ENDPOINT_URL}
```

The destination must use the env var the operator injects into the Worker, never a hardcoded URL:

- `DD_OP_DESTINATION_CLOUDPREM_ENDPOINT_URL` for logs (this branch).
- `DD_OP_DESTINATION_DATADOG_BYOC_TRACES_ENDPOINT_URL` for `signal: traces`, once #3749 lands.

The YAML above is illustrative: the exact keys follow the Observability Pipelines config schema.

## Errors

The cluster does not validate `pipelineConfig` YAML (or reject multi-document YAML). Those errors
surface only on the child Worker status, not on the cluster.

## Open questions

- Should the cluster validate `pipelineConfig` itself? Blocked today by an import cycle
  (Worker validation imports cluster validation).
- Is the config snippet above the right minimal shape, and is `${ENV}` the right interpolation?
- Should a cluster-level condition mirror the Worker's config error?
