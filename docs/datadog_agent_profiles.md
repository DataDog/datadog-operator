# DatadogAgentProfiles

This feature was introduced in Datadog Operator v1.5.0 and was made generally available in v1.24.0.

## Overview

DatadogAgentProfiles (DAPs), also known as profiles, can be created to override certain Operator settings that were set in a DatadogAgent (DDA) on a subset of nodes. The [Supported Settings](#supported-settings) table lists which settings can be overridden and the minimum Operator versions for each. While multiple DAPs can be applied to a cluster, each DAP must target a different subset of nodes so the DAPs do not conflict with each other. 

Example:

```yaml
apiVersion: datadoghq.com/v1alpha1
kind: DatadogAgentProfile
metadata:
  name: datadogagentprofile-sample
spec:
  profileAffinity:
    profileNodeAffinity:
      - key: kubernetes.io/os
        operator: In
        values:
          - linux
  config:
    override:
      nodeAgent:
        containers:
          agent:
            resources:
              requests:
                cpu: 256m
```

The DAP spec has two main sections:
* `profileAffinity` is used to target a subset of nodes. It accepts a list of [NodeSelectorRequirements](https://pkg.go.dev/k8s.io/api/core/v1#NodeSelectorRequirement).
* `config` defines the configuration to override in the DDA. It follows the configuration formatting of the Operator's [DatadogAgentSpec](https://github.com/DataDog/datadog-operator/blob/98276c56ad824f81be6f75128d230d2c4eda4c0b/apis/datadoghq/v2alpha1/datadogagent_types.go#L28).

When a DAP is applied, the Operator creates a new DaemonSet for that profile using the same name as the DAP. Even if the Operator is configured to use ExtendedDaemonSets, it will still create DaemonSets for any DAPs. It will also create a DaemonSet (or an ExtendedDaemonSet, if enabled) for a default profile. The default profile uses the same name as the DDA and applies to all nodes that are not targeted by a DAP.

```console
$ kubectl get ds
NAME                                                            DESIRED   CURRENT   READY   UP-TO-DATE   AVAILABLE   NODE SELECTOR   AGE
datadog-agent                                                   1         1         1       1            1           <none>          5m3s
datadogagentprofile-sample                                      1         1         1       1            1           <none>          44s
```

* `datadog-agent` is the DaemonSet created by the default profile
* `datadogagentprofile-sample` is the DaemonSet created by the profile `datadogagentprofile-sample`

## When to use DatadogAgentProfiles

Common scenarios:

* **Heterogeneous clusters with varying CPU core counts.** A single global resource limit set on the `DatadogAgent` is either too low for small control-plane nodes or too high for large GPU/bare-metal nodes. Use one DatadogAgentProfile (DAP) per node shape (selected via `profileAffinity` on a label such as `node.kubernetes.io/instance-type` or a custom node-role label) to apply appropriate `resources.limits.cpu` and `resources.limits.memory` to each group. On nodes with a high logical CPU count, an explicit CPU limit also prevents the Agent's Go runtime from sizing its scheduler to the host CPU count, which can otherwise lead to OOM kills.
* **Targeting specific node roles.** Run a different Agent configuration (for example, additional checks or a higher log level) on nodes labeled for a particular workload.

## Prerequisites

* Operator v1.5.0+
* Tests were performed on Kubernetes versions >= `1.27.0`
* The cluster must allow the Operator to `patch` nodes. Environments that block node modifications, such as GKE Autopilot, cannot use DAPs.

## Enabling DatadogAgentProfiles

DAP is disabled by default. To enable DAP using the [datadog-operator helm chart](https://github.com/DataDog/helm-charts/tree/main/charts/datadog-operator), set in your `values.yaml` or as a flag in the command line arguments using `--set`:
* `datadogAgentProfile.enabled=true`: this instructs the Operator deployment to start the `DatadogAgentProfile` controller.
* `datadogCRDs.crds.datadogAgentProfiles=true`: this installs the `DatadogAgentProfile` CRD.

For **OLM deployments** where container args cannot be set, enable the controller
via environment variable in the `Subscription`:
```yaml
config:
  env:
    - name: DD_AGENT_PROFILE_CONTROLLER_ENABLED
      value: "true"
```

> [!CAUTION]
> Enabling DAP will increase the resource usage of the Operator. Please ensure the operator pod has enough resources allocated to it prior to enabling DAP.

## Declaring a provider

A DAP can declare a [provider](providers.md) for the subset of nodes it targets by setting the `agent.datadoghq.com/cluster-provider` annotation on the profile. This is the supported way to apply provider-specific configuration (for example, a GKE COS node pool) to a subset of nodes.

This is safe **only if the profile's `profileAffinity` correctly selects the nodes that actually match the declared provider**. The Operator does not verify that the selected nodes match the annotation; if the selector is too broad, the provider configuration is applied to nodes it does not fit.

The node-scoped providers that make sense on a DAP are:

| Provider | Applies to | Notes |
| -------- | ---------- | ----- |
| `gke-cos` | GKE Container-Optimized OS node pools | Drops the `/usr/src` volume for the OOM Kill, TCP Queue Length, and GPU checks |
| `eks-ec2-use-hostname-from-file` | EKS EC2 node groups | Adds `DD_HOSTNAME_FILE` and the cloud-init instance-id mount |

See the [providers documentation](providers.md) for the full catalog, effects, and Helm mappings.

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
  config:
    override:
      nodeAgent:
        containers:
          agent:
            resources:
              requests:
                cpu: 256m
```

## Supported Settings

| Setting | Operator Version |
| -------- | :--------------: |
| override.[nodeAgent].containers.[\*].resources.\* | v1.5.0 |
| override.[nodeAgent].priorityClassName | v1.6.0 |
| override.[nodeAgent].containers.[\*].env | v1.8.0 |
| override.[nodeAgent].labels | v1.8.0 |
| override.[nodeAgent].updateStrategy | v1.9.0 |
| override.[nodeAgent].runtimeClassName | v1.12.0 |
| override.[nodeAgent].volumes | v1.29.0 |
| override.[nodeAgent].containers.[\*].volumeMounts | v1.29.0 |
