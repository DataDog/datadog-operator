# Datadog Plugin for kubectl

Datadog provides a `kubectl` plugin with helper utilities that gives visibility into internal components. You can use the plugin with Operator installations or with the Datadog [Helm chart][1].

## Install the plugin

Run:
```shell
kubectl krew install datadog
```

This uses the [Krew plugin manager](https://krew.sigs.k8s.io/).

```console
$ kubectl krew install datadog
Installing plugin: datadog
Installed plugin: datadog
\
 | Use this plugin:
 | 	kubectl datadog
 | Documentation:
 | 	https://github.com/DataDog/datadog-operator
/
```

## Available commands

```console
$ kubectl datadog --help
Usage:
  datadog [command]

Available Commands:
  agent
  autoscaling  Manage autoscaling features
  clusteragent
  completion   Generate the autocompletion script for the specified shell
  flare        Collect a Datadog's Operator flare and send it to Datadog
  get          Get DatadogAgent deployment(s)
  helm2dda     Map Datadog Helm values to DatadogAgent CRD schema
  help         Help about any command
  metrics
  validate

```

### Agent sub-commands

```console
$ kubectl datadog agent --help
Usage:
  datadog agent [command]

Available Commands:
  check       Find check errors
  find        Find datadog agent pod monitoring a given pod
  upgrade     Upgrade the Datadog Agent version

```

### Cluster Agent sub-commands

```console
$ kubectl datadog clusteragent --help
Usage:
  datadog clusteragent [command]

Available Commands:
  leader      Get Datadog Cluster Agent leader
  upgrade     Upgrade the Datadog Cluster Agent version
```

### Validate sub-commands

```console
$ kubectl datadog validate ad --help
Usage:
  datadog validate ad [command]

Available Commands:
  pod         Validate the autodiscovery annotations for a pod
  service     Validate the autodiscovery annotations for a service
```

### Autoscaling sub-commands (Technical Preview)

> **Note:** The `autoscaling` commands are part of the Datadog Cluster Autoscaling feature, which is in **technical preview**. APIs and behaviors may change in future releases.

These commands install and configure [Karpenter](https://karpenter.sh/) on an EKS cluster so that Datadog can manage cluster autoscaling.

#### `autoscaling cluster install`

Installs Karpenter on an EKS cluster and configures it for use with Datadog Cluster Autoscaling. The command:

1. Creates the required AWS CloudFormation stacks.
2. Configures EKS authentication (aws-auth ConfigMap, EKS Pod Identity, or API-based access entries depending on the cluster).
3. Installs Karpenter via Helm from the OCI registry.
4. Optionally creates `EC2NodeClass` and `NodePool` Karpenter resources, inferred from existing cluster nodes or EKS node groups.

```console
$ kubectl datadog autoscaling cluster install --help
Install autoscaling on an EKS cluster

Usage:
  datadog autoscaling cluster install [flags]

Examples:

  # install autoscaling
  kubectl datadog autoscaling cluster install

Flags:
      --cluster-name string                                   Name of the EKS cluster
      --create-karpenter-resources CreateKarpenterResources   Which Karpenter resources to create: none, ec2nodeclass, all (default: all) (default all)
      --debug                                                 Enable debug logs
      --inference-method InferenceMethod                      Method to infer EC2NodeClass and NodePool properties: nodes, nodegroups (default nodegroups)
      --karpenter-namespace string                            Name of the Kubernetes namespace to deploy Karpenter into (default "dd-karpenter")
      --karpenter-version string                              Version of Karpenter to install (default to latest)
```

#### `autoscaling cluster uninstall`

Removes Karpenter and all associated resources from an EKS cluster. Deletes `NodePool` and `EC2NodeClass` resources, waits for the corresponding EC2 instances to terminate, uninstalls the Karpenter Helm release, cleans up IAM roles, and removes the CloudFormation stacks. Only resources originally created by `kubectl datadog` are affected.

```console
$ kubectl datadog autoscaling cluster uninstall --help
Uninstall autoscaling from an EKS cluster

Usage:
  datadog autoscaling cluster uninstall [flags]

Examples:

  # uninstall autoscaling
  kubectl datadog autoscaling cluster uninstall

Flags:
      --cluster-name string        Name of the EKS cluster
      --karpenter-namespace string  Name of the Kubernetes namespace where Karpenter is deployed (default "dd-karpenter")
      --yes                        Skip confirmation prompt
```

[1]: https://github.com/DataDog/helm-charts/tree/main/charts/datadog
