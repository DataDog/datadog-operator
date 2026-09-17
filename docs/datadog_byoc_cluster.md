# DatadogBYOCCluster image overrides

By default, `spec.release` selects an OCI release artifact containing the BYOC and Observability Pipelines Worker images. Use `spec.imageOverrides` to mirror images into private registries or deploy compatible hotfix images without publishing a new release artifact.

The examples below are partial specifications; retain your existing component and other cluster settings. See the [complete sample](../config/samples/datadoghq_v1alpha1_datadogbyoccluster.yaml).

## Mirror images while retaining release versions

```yaml
spec:
  release:
    tag: "0.1.32"
  imageOverrides:
    byoc:
      repository: registry.example.com/datadog/byoc
      imagePullSecrets:
        - name: workload-registry-credentials
    observabilityPipelinesWorker:
      repository: registry.example.com/datadog/observability-pipelines-worker
      imagePullSecrets:
        - name: workload-registry-credentials
```

A repository-only override retains the tag and digest selected by the release. When the release provides a digest, it takes precedence over its tag. The mirror must serve the image at that same digest; merely copying a tag to a different manifest digest is not sufficient.

## Fully specify images without resolving a release

```yaml
spec:
  imageOverrides:
    byoc:
      repository: registry.example.com/datadog/byoc
      tag: hotfix-123
      imagePullSecrets:
        - name: workload-registry-credentials
    observabilityPipelinesWorker:
      repository: registry.example.com/datadog/observability-pipelines-worker
      digest: sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
      imagePullSecrets:
        - name: workload-registry-credentials
```

When **both** images specify a repository and either a tag or digest, the Operator does not fetch the release artifact. `spec.release` may be omitted or left in place for later recovery. If present, its fields must still pass CRD validation. This mode works even when the release repository is unavailable.

Both logical images must be fully specified to skip release resolution, regardless of which workloads are currently enabled. If either image needs values from a release, `spec.release` is required and must resolve successfully. There is no implicit fallback on resolution failure.

## Override rules

Each image override supports:

| Field | Behavior |
| --- | --- |
| `repository` | Replaces the image repository. If omitted, the release image repository is retained. |
| `tag` | Replaces the release version and clears its digest. If `repository` is omitted, the release repository is retained. Mutually exclusive with `digest`. |
| `digest` | Replaces the release version and clears its tag. If `repository` is omitted, the release repository is retained. Mutually exclusive with `tag`. |
| `imagePullSecrets` | Names of Secrets in the DatadogBYOCCluster namespace, added to Pods that use this workload image. |

An empty override object is a no-op. A secrets-only override is allowed and retains the release-selected image reference. Secret entries are unique by name. Workload pull secrets are not used for fetching the release artifact; `spec.release.imagePullSecrets` is not supported.

The BYOC override applies to all enabled BYOC components, including the optional read-only Metastore and Compactor. It does not replace user-specified init container images. Pull secrets are Kubernetes Pod-level settings, so they can also be used for init containers in those Pods.

The Observability Pipelines Worker override participates in image resolution and the release-skip decision. The Operator does not currently generate a worker workload, so this setting does not change any running container. Worker pull secrets are not added to BYOC Pods.

## Status and recovery

`status.conditions` reports image resolution, including overrides, through the existing `ReleaseResolved` condition:

| Status / Reason | Meaning |
| --- | --- |
| `True / Resolved` | The workload images were resolved successfully, from the release artifact, overrides, or both. |
| `False / ResolutionFailed` | A required release artifact could not be resolved. |

There is no separate `ImageOverridesActive` condition. Image resolution does not imply that the workload rollout has completed. The generated `IMAGE_NAME` and `IMAGE_TAG` telemetry environment variables use the effective BYOC image values, rather than stale release values. A digest override has no tag.

Overrides persist when `spec.release` changes. To return to release-managed images, remove the overrides and ensure a valid `spec.release` is present. Removing an override also removes its explicitly configured Pod pull secrets. Restore the intended release version before removing overrides if the release reference changed during the incident.

Image changes use the existing Deployment and StatefulSet rollout strategies; they do not guarantee zero downtime or safe rollback across incompatible data-format changes. Hotfix images must support the existing arguments, configuration, probes, and security settings. Use immutable digests or unique tags: the pull policy remains `IfNotPresent`, and pushing a new image under an unchanged tag does not itself trigger a rollout.

## Indexer split store sizing

The split store is a local cache. With PVC storage, the Operator generates
`indexer.split_store_max_num_bytes` as 70% of the requested PVC capacity minus
the Operator-calculated default `ingest_api.max_queue_disk_usage`, in integer
bytes. Overrides in `spec.nodeConfig` are passed through without parsing their
byte-size values; overriding the queue does not recalculate the split store
size. If the result is zero or negative, the Operator sets the cache capacity to
zero. This prevents split caching on that PVC; it does not reduce the queue size.

The default PVC capacity is 30Gi. With the default Indexer memory limit of 16Gi,
the queue uses approximately 9.6Gi, leaving approximately 11.4Gi for the split
store. With a 3Gi memory limit, the queue uses approximately 1.8Gi and the split
store receives approximately 19.2Gi.

With `emptyDir` storage, intended for testing, the Operator omits
`split_store_max_num_bytes` regardless of `sizeLimit`, leaving Pomsky's default of
100GiB in effect. The Operator keeps `split_store_max_num_splits` at 10000 unless
overridden; it does not disable the split store for `emptyDir`.

An explicit `spec.nodeConfig.indexer.split_store_max_num_bytes` takes precedence
for either storage type and overrides the automatically calculated value. For example:

```yaml
spec:
  nodeConfig:
    indexer:
      split_store_max_num_bytes: 10GiB
```
