# DatadogAgentProfiles (beta)

This feature was introduced in Datadog Operator v1.5.0 and is currently in beta.

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

When a DAP is applied, the Operator creates a new DaemonSet for that profile using the name format `datadog-agent-with-profile-<namespace>-<name>`. Even if the Operator is configured to use ExtendedDaemonSets, it will still create DaemonSets for any DAPs. It will also create a DaemonSet (or an ExtendedDaemonSet, if enabled) for a default profile. The default profile uses the same naming pattern that the DDA uses for node agents and applies to all nodes that are not targeted by a DAP.

```console
$ kubectl get ds
NAME                                                            DESIRED   CURRENT   READY   UP-TO-DATE   AVAILABLE   NODE SELECTOR   AGE
datadog-agent                                                   1         1         1       1            1           <none>          5m3s
datadog-agent-with-profile-default-datadogagentprofile-sample   1         1         1       1            1           <none>          44s
```

* `datadog-agent` is the DaemonSet created by the default profile
* `datadog-agent-with-profile-default-datadogagentprofile-sample` is the DaemonSet created by the profile `datadogagentprofile-sample`

## Prerequisites

* Operator v1.5.0+
* Tests were performed on Kubernetes versions >= `1.27.0`

## Enabling DatadogAgentProfiles

DAP is disabled by default. To enable DAP using the [datadog-operator helm chart](https://github.com/DataDog/helm-charts/tree/main/charts/datadog-operator), set `datadogAgentProfile.enabled=true` in your `values.yaml` file or as a flag in the command line arguments `--set datadogAgentProfile.enabled=true`.

> [!CAUTION]
> Enabling DAP will increase the resource usage of the Operator. Please ensure the operator pod has enough resources allocated to it prior to enabling DAP.

## Supported Settings

| Setting | Operator Version |
| -------- | :--------------: |
| override.[nodeAgent].containers.[\*].resources.\* | v1.5.0 |
| override.[nodeAgent].priorityClassName | v1.6.0 |
| override.[nodeAgent].containers.[\*].env | v1.8.0 |
| override.[nodeAgent].labels | v1.8.0 |
| override.[nodeAgent].updateStrategy | v1.9.0 |
| override.[nodeAgent].runtimeClassName | v1.12.0 |
