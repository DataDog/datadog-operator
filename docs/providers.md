# Providers

A *provider* identifies an environment or platform that needs a specific set of
customizations to the Agent configuration. A provider is warranted when the
environment imposes requirements or restrictions the defaults do not satisfy: a
managed Kubernetes service (`eks`, `aks`), a Kubernetes distribution
(`openshift`), a node OS without kernel sources or a host user/group database
(`gke-cos`, `talos`), a restricted managed environment with a workload
allowlist and a fixed node OS (`gke-autopilot`), or a platform-specific Agent
behavior (`eks-ec2-use-hostname-from-file`).

Setting a provider applies that set of customizations to the Agents it covers,
whether across the whole cluster or only the nodes a
[DatadogAgentProfile][4] targets.

> This page describes the supported provider model, which is distinct from
> the legacy [introspection][1] feature (one DaemonSet per node
> provider), which is deprecated and no longer active on the default
> reconciliation path.



## Provider scope

A provider applies at one of two scopes, both expressed through the single
`agent.datadoghq.com/cluster-provider` annotation:

- **Cluster scope**: set on (or detected for) the `DatadogAgent`. Applies to the
whole cluster. Used for cluster-wide providers such as `eks`, `openshift`,
`aks`, and `gke-autopilot` on an Autopilot cluster.
- **Node scope**: set on a [DatadogAgentProfile][4]
(DAP). Applies only to the subset of nodes the profile targets. Used for node
variations such as `gke-cos`.

The annotation key is the same in both cases; the scope is determined by the
object it is set on.

## Setting a provider

There are three ways a provider is established, in increasing order of user
intent.

### Automatic detection

For cluster-scope providers, the Operator inspects the node the Operator pod
runs on and applies the matching configuration automatically. No annotation is
required. Most providers are identified by node labels; `talos` is identified
from the node's `osImage` instead, because Talos Linux exposes no default stable
node label.


| Provider                       | Detected since | Detected from                                          |
| ------------------------------ | -------------- | ------------------------------------------------------ |
| `default`                      | v1.29.0+       | fallback when no row below matches                     |
| `aks`                          | v1.29.0+       | any `kubernetes.azure.com/*` label                     |
| `eks`                          | v1.29.0+       | any `eks.amazonaws.com/*` or `alpha.eksctl.io/*` label |
| `openshift` (`openshift-<os>`) | v1.29.0+       | `node.openshift.io/os_id` label                        |
| `talos`                        | v1.31.0+       | `status.nodeInfo.osImage` starting with `Talos (`      |


Detection always resolves to exactly one provider, and `default` is the result when
none of the other rows match. That is the normal outcome for an on-premises or
unrecognized cluster, not an error or an "undetected" state. The rows are listed
alphabetically after `default`, not in precedence order; the signals are mutually
exclusive in practice, but if a node did match more than one, `talos` wins over any
label, and among labels the order is `openshift`, then `eks`, then `aks`.

The detected provider is recorded in `status.clusterProvider` on the
`DatadogAgent` (see [Effective provider resolution](#effective-provider-resolution)).

OpenShift is detected as `openshift-<os_id>` (for example `openshift-rhcos`). The
`<os_id>` value itself carries no meaning — `openshift-rhcos` and `openshift-rhel`
behave identically — but the suffix must be present for [control plane
monitoring][2] to be enabled. Prefer the detected value, or an explicit
`openshift-<os_id>`, over a bare `openshift`.

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
[DatadogAgentProfile][5]. The value
is propagated to the DaemonSet the profile generates and applies only to the
nodes the profile's `profileAffinity` selects.

This is safe **only if** `profileAffinity` **correctly selects the nodes that match
the declared provider**. The Operator does not verify that the selected nodes
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


| Provider                         | Available in | Scope                       | Resolution              | Effect                                                                                                                                                                                                                                                                                                                         | Helm equivalent                              |
| -------------------------------- | ------------ | --------------------------- | ----------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | -------------------------------------------- |
| `gke-cos`                        | v1.29.0+     | Cluster (DDA) or Node (DAP) | Annotation only         | Drops the `/usr/src` volume from the OOM Kill, TCP Queue Length, and GPU checks (node OS has no kernel sources)                                                                                                                                                                                                                | `providers.gke.cos`                          |
| `eks-ec2-use-hostname-from-file` | v1.29.0+     | Cluster (DDA) or Node (DAP) | Annotation only         | Adds `DD_HOSTNAME_FILE` and a host mount of the cloud-init instance-id file so the Agent derives a stable hostname                                                                                                                                                                                                             | `providers.eks.ec2.useHostnameFromFile`      |
| `eks`                            | v1.29.0+     | Cluster (DDA)               | Detection or annotation | Enables [control plane monitoring][2]: API Server, Controller Manager, Scheduler                                                                                                                                                                                                                                               | `providers.eks.controlPlaneMonitoring`       |
| `openshift` (`openshift-<os>`)   | v1.29.0+     | Cluster (DDA)               | Detection or annotation | Enables [control plane monitoring][2]: API Server, Controller Manager, Scheduler, and etcd. Operator v1.31.0+ also configures the node Agent for OpenShift: the `datadog-agent-scc` ServiceAccount (when it is authorized to use the `privileged` SCC), SELinux type `spc_t`, `master` and `infra` tolerations, and `kubelet.tlsVerify: false`. See [Install on OpenShift][6]                                                                                                                                                                                                                                     | `providers.openshift.controlPlaneMonitoring` |
| `aks`                            | v1.29.0+     | Cluster (DDA)               | Detection or annotation | Sets the mandatory `DD_ADMISSION_CONTROLLER_ADD_AKS_SELECTORS=true` environment variable on the Cluster Agent                                                                                                                                                                                                                  | `providers.aks.enabled`                      |
| `gke-autopilot`                  | v1.29.0+     | Cluster (DDA)               | Annotation only         | Full GKE Autopilot workload adaptation (volume, env var, path, image, and PriorityClass changes). See [Datadog Operator on GKE Autopilot][3]                                                                                                                                                                                   | `providers.gke.autopilot`                    |
| `windows`                        | v1.30.0+     | Node (DAP)                  | Annotation only         | Builds a Windows-compatible node Agent DaemonSet on the targeted Windows nodes: Linux-only containers, mounts, and security context are stripped, and a Windows base image and init config are applied                                                                                                                         | None                                         |
| `talos`                          | v1.31.0+     | Cluster (DDA)[^talos-scope] | Detection or annotation | Drops host volumes that don't exist on Talos Linux nodes: `/usr/src` and `/lib/modules` (OOM Kill, TCP Queue Length), and `/etc/passwd`/`/etc/group` (Live Process Collection, Process Discovery, CWS, CSPM). Adds a writable `/sys/kernel/tracing` mount to `system-probe` so eBPF features can attach probes[^talos-tracefs] | `providers.talos.enabled`                    |


`Available in` is the first Operator release that honors the provider value. Individual effects can be added in later releases; where that matters it is noted in a footnote. For when *detection* of a provider became available, see the `Detected since` column in [Automatic detection](#automatic-detection) — the two can differ, since a provider can be annotation-only before it gains detection.

Cluster scope applies the provider to every node, so use it only when all nodes match the provider (for example, a cluster where every node runs Container-Optimized OS). Otherwise, set the provider on a DAP that targets the matching nodes.

[^talos-tracefs]: Attaching an eBPF probe writes to `kprobe_events`, which lives in tracefs. Mainline kernels auto-mount tracefs under debugfs at `/sys/kernel/debug/tracing`, so the `/sys/kernel/debug` host mount these features already use is enough there. Talos exposes tracefs only as a standalone mount at `/sys/kernel/tracing`, so without this mount probe attachment fails while the pod still reports `Running`/`Ready`. Applies to NPM, USM, CWS, OOM Kill, TCP Queue Length, eBPF Check, Dynamic Instrumentation, SBOM, and GPU (privileged mode only).

[^talos-scope]: Talos detection assumes the normal Talos deployment model: the cluster is uniform and every Kubernetes node runs Talos Linux. If detection sees a Talos `osImage` on a single node, it treats the cluster as Talos. Opt out with `agent.datadoghq.com/cluster-provider: default` on the `DatadogAgent`. On mixed clusters, target the Talos nodes with a [DatadogAgentProfile][4] instead; features that mount absent Talos host paths such as `/etc/passwd`, `/etc/group`, `/usr/src`, or `/lib/modules` can otherwise fail to start on those nodes.

**OpenShift and Unix Domain Sockets**: the provider does not change the APM or
DogStatsD transport. If either uses a Unix Domain Socket, the Admission Controller
injects a `hostPath` volume into instrumented application pods, which the default
`restricted-v2` SCC forbids — those pods are rejected at admission, while the Agent
itself keeps running. Set
`features.apm.unixDomainSocketConfig.enabled: false` with
`features.apm.hostPortConfig.enabled: true`, and
`features.dogstatsd.unixDomainSocketConfig.enabled: false`.

## Examples

- **Cluster-wide, auto-detected**: an EKS cluster gets `eks` from detection and
enables [control plane monitoring][2] with no user
configuration.
- **Cluster-wide, declared**: set `agent.datadoghq.com/cluster-provider: aks` on
the `DatadogAgent` to apply the required AKS admission controller selectors.
- **Node OS variation**: set `agent.datadoghq.com/cluster-provider: gke-cos` on a
DAP that targets the COS node pool.
- **Granular behavior**: set
`agent.datadoghq.com/cluster-provider: eks-ec2-use-hostname-from-file` on a DAP
targeting EC2 nodes that need file-based hostname resolution.
- **Restricted managed environment**: set
`agent.datadoghq.com/cluster-provider: gke-autopilot` on the `DatadogAgent` for
an Autopilot cluster (see the [GKE Autopilot guide][3]).


[1]: https://github.com/DataDog/datadog-operator/blob/main/docs/introspection.md
[2]: https://docs.datadoghq.com/containers/kubernetes/control_plane/?tab=datadogoperator#EKS
[3]: https://docs.datadoghq.com/containers/kubernetes/distributions/?tab=datadogoperator#autopilot
[4]: https://docs.datadoghq.com/containers/datadog_operator/datadog_agent_profiles
[5]: https://docs.datadoghq.com/containers/datadog_operator/datadog_agent_profiles#declaring-a-provider
[6]: https://github.com/DataDog/datadog-operator/blob/main/docs/install-openshift.md
