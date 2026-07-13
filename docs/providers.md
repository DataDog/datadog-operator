# Providers

A *provider* identifies an environment or platform that needs a specific set of
customizations to the Agent configuration. A provider is warranted when the
environment imposes requirements or restrictions the defaults do not satisfy: a
managed Kubernetes service (`eks`, `aks`), a Kubernetes distribution
(`openshift`), a node OS without kernel sources (`gke-cos`), a restricted managed
environment with a workload allowlist and a fixed node OS (`gke-autopilot`), or a
platform-specific Agent behavior (`eks-ec2-use-hostname-from-file`).

Setting a provider applies that set of customizations to the Agents it covers,
whether across the whole cluster or only the nodes a
[DatadogAgentProfile](datadog_agent_profiles.md) targets.

> This page describes the current, supported provider model. It is distinct from
> the legacy [introspection](introspection.md) feature (one DaemonSet per node
> provider), which is deprecated and no longer active on the default
> reconciliation path.

## Provider scope

A provider applies at one of two scopes, both expressed through the single
`agent.datadoghq.com/cluster-provider` annotation:

- **Cluster scope**—set on (or detected for) the `DatadogAgent`. Applies to the
  whole cluster. Used for cluster-wide providers such as `eks`, `openshift`,
  `aks`, and `gke-autopilot` on an Autopilot cluster.
- **Node scope**—set on a [DatadogAgentProfile](datadog_agent_profiles.md)
  (DAP). Applies only to the subset of nodes the profile targets. Used for node
  variations such as `gke-cos`, or `gke-autopilot` on
  [Autopilot-mode node pools](https://docs.cloud.google.com/kubernetes-engine/docs/concepts/about-autopilot-mode-standard-clusters)
  in a Standard cluster.

The annotation key is the same in both cases; the scope is determined by the
object it is set on.

## Setting a provider

There are three ways a provider is established, in increasing order of user
intent.

### Automatic detection

For cluster-scope providers, the Operator detects the provider from the labels of
the node the Operator pod runs on and applies the matching configuration
automatically. No annotation is required. Available in Operator v1.29.0+.

| Provider | Detected from node label |
| -------- | ------------------------ |
| `eks` | any `eks.amazonaws.com/*` or `alpha.eksctl.io/*` label |
| `aks` | any `kubernetes.azure.com/*` label |
| `openshift` (`openshift-<os>`) | `node.openshift.io/os_id` |
| `default` | none of the above |

The detected provider is recorded in `status.clusterProvider` on the
`DatadogAgent` (see [Effective provider resolution](#effective-provider-resolution)).

`gke-cos`, `eks-ec2-use-hostname-from-file`, and `gke-autopilot` are **not**
auto-detected; they must be declared explicitly.

### On a DatadogAgent

Declare or override the cluster provider with the
`agent.datadoghq.com/cluster-provider` annotation on the `DatadogAgent`. This
mirrors the Helm chart's `providers.*` configuration and is also the correction
mechanism when auto-detection cannot determine the provider.

```yaml
apiVersion: datadoghq.com/v2alpha1
kind: DatadogAgent
metadata:
  name: datadog
  annotations:
    agent.datadoghq.com/cluster-provider: eks
spec:
  global:
    credentials:
      apiSecret:
        secretName: datadog-secret
        keyName: api-key
```

### On a DatadogAgentProfile

Declare a node-scope provider by setting the same annotation on a
[DatadogAgentProfile](datadog_agent_profiles.md#declaring-a-provider). The value
is propagated to the DaemonSet the profile generates and applies only to the
nodes the profile's `profileAffinity` selects.

This is safe **only if `profileAffinity` correctly selects the nodes that match
the declared provider**—the Operator does not verify that the selected nodes
match the annotation.

```yaml
apiVersion: datadoghq.com/v1alpha1
kind: DatadogAgentProfile
metadata:
  name: gke-cos-profile
  annotations:
    agent.datadoghq.com/cluster-provider: gke-cos
spec:
  profileAffinity:
    profileNodeAffinity:
      - key: cloud.google.com/gke-os-distribution
        operator: In
        values:
          - cos
```

## Effective provider resolution

When more than one source could supply a provider, the Operator resolves the
effective value by source:

1. A user-specified value (annotation on the `DatadogAgent`, or on a
   `DatadogAgentProfile` for its node subset) always wins.
2. Otherwise the auto-detected value is used.

The resolved value is recorded in the `DatadogAgent` status:

```yaml
status:
  clusterProvider: eks
  conditions:
  - type: ClusterProviderDetected
    status: "True"
    reason: ProviderDetected      # or "UserSpecified" when set via annotation
    message: Cluster provider detected as "eks".
```

## Supported providers

The following is the exhaustive list of provider values the Operator acts on. All
values are the value of the `agent.datadoghq.com/cluster-provider` annotation.

| Provider | Scope | Resolution | Effect | Helm equivalent |
| -------- | ----- | ---------- | ------ | --------------- |
| `gke-cos` | Node (DAP) | Annotation only | Drops the `/usr/src` volume from the OOM Kill, TCP Queue Length, and GPU checks (node OS has no kernel sources) | `providers.gke.cos` |
| `eks-ec2-use-hostname-from-file` | Node (DAP) | Annotation only | Adds `DD_HOSTNAME_FILE` and a host mount of the cloud-init instance-id file so the Agent derives a stable hostname | `providers.eks.ec2.useHostnameFromFile` |
| `eks` | Cluster (DDA) | Detection or annotation | Enables [control plane monitoring](control_plane_monitoring.md): API Server, Controller Manager, Scheduler | `providers.eks.controlPlaneMonitoring` |
| `openshift` (`openshift-<os>`) | Cluster (DDA) | Detection or annotation | Enables [control plane monitoring](control_plane_monitoring.md): API Server, Controller Manager, Scheduler, and etcd | `providers.openshift.controlPlaneMonitoring` |
| `aks` | Cluster (DDA) | Detection or annotation | Sets the mandatory `DD_ADMISSION_CONTROLLER_ADD_AKS_SELECTORS=true` environment variable on the Cluster Agent | `providers.aks.enabled` |
| `gke-autopilot` | Cluster (DDA) or Node (DAP) \* | Annotation only | Full GKE Autopilot workload adaptation (volume, env var, path, image, and PriorityClass changes). See [Datadog Operator on GKE Autopilot](gke_autopilot/external.md) | `providers.gke.autopilot` |

\* `gke-autopilot` is cluster-scoped on an Autopilot cluster. Use node scope (a
DAP) for [Autopilot-mode node pools](https://docs.cloud.google.com/kubernetes-engine/docs/concepts/about-autopilot-mode-standard-clusters)
in a Standard cluster; this is expected to work through the same mechanism but has
not been explicitly tested with DAPs.

## Examples

- **Cluster-wide, auto-detected**—an EKS cluster gets `eks` from detection and
  enables [control plane monitoring](control_plane_monitoring.md) with no user
  configuration.
- **Cluster-wide, declared**—set `agent.datadoghq.com/cluster-provider: aks` on
  the `DatadogAgent` to apply the required AKS admission controller selectors.
- **Node OS variation**—set `agent.datadoghq.com/cluster-provider: gke-cos` on a
  DAP that targets the COS node pool.
- **Granular behavior**—set
  `agent.datadoghq.com/cluster-provider: eks-ec2-use-hostname-from-file` on a DAP
  targeting EC2 nodes that need file-based hostname resolution.
- **Restricted managed environment**—set
  `agent.datadoghq.com/cluster-provider: gke-autopilot` on the `DatadogAgent` for
  an Autopilot cluster, or on a DAP targeting Autopilot-mode node pools in a
  Standard cluster (see the [GKE Autopilot guide](gke_autopilot/external.md)).
