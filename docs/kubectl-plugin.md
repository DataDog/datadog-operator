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

```console
$ kubectl datadog autoscaling cluster --help
Manage cluster autoscaling

Usage:
  datadog autoscaling cluster [command]

Available Commands:
  evict-legacy-nodes Drain workloads from non-Datadog node groups onto Datadog-managed Karpenter NodePools
  install            Install autoscaling on an EKS cluster
  uninstall          Uninstall autoscaling from an EKS cluster
  update             Update an existing autoscaling installation on an EKS cluster
```

#### `autoscaling cluster install`

Installs Karpenter on an EKS cluster and configures it for use with Datadog Cluster Autoscaling. The command:

1. Creates two AWS CloudFormation stacks: one carrying the node role, the controller IAM policies and the interruption-handling resources Karpenter needs, and one carrying the mode-specific authentication resources described below.
2. Configures how the Karpenter controller authenticates and where it runs, depending on `--install-mode`:
   - `fargate` (default): registers the cluster OIDC provider, creates an IRSA role for the controller, and creates a Fargate profile on the cluster private subnets (auto-discovered unless `--fargate-subnets` is set), so that Karpenter never runs on the nodes it manages.
   - `existing-nodes`: ensures the EKS Pod Identity agent addon is available and configures a Pod Identity association for the controller.

   In both modes, an EKS access entry is created for the Karpenter node role on clusters whose authentication mode supports access entries (`API` or `API_AND_CONFIG_MAP`), and that role is also mapped in the `aws-auth` ConfigMap when that ConfigMap is present.
3. Installs Karpenter via Helm from the OCI registry.
4. Optionally creates `EC2NodeClass` and `NodePool` Karpenter resources, inferred from existing cluster nodes or EKS node groups.

The command installs nothing and exits successfully with an explanatory message when EKS auto-mode is active, or when a Karpenter installation it did not create, or one in a different namespace, is already present on the cluster.

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
      --fargate-subnets strings                               Override auto-discovery of private subnets for the Fargate profile (comma-separated subnet IDs). Only used when --install-mode=fargate.
      --inference-method InferenceMethod                      Method to infer EC2NodeClass and NodePool properties: nodes, nodegroups (default nodegroups)
      --install-mode InstallMode                              How to run the Karpenter controller: fargate (on dedicated Fargate nodes, default) or existing-nodes (on existing cluster nodes) (default fargate)
      --karpenter-namespace string                            Name of the Kubernetes namespace to deploy Karpenter into (default "dd-karpenter")
      --karpenter-version string                              Version of Karpenter to install (default to latest)
```

#### `autoscaling cluster update`

Refreshes an autoscaling installation previously created by `kubectl datadog`: it updates the CloudFormation stacks and upgrades the Karpenter Helm release in place. The command refuses to touch a Karpenter installation it did not create.

The parameters that cannot change after the initial install — Karpenter namespace, install mode, and Fargate subnets — are read back from the CloudFormation stack `install` created, and are therefore not exposed as flags.

Unlike `install`, `--create-karpenter-resources` defaults to `none`, so that manual edits to the `EC2NodeClass` and `NodePool` resources survive an update. Pass `ec2nodeclass` to regenerate the `EC2NodeClass` alone, or `all` to regenerate both.

```console
$ kubectl datadog autoscaling cluster update --help
Update an existing autoscaling installation on an EKS cluster

Usage:
  datadog autoscaling cluster update [flags]

Examples:

  # update a previously installed kubectl-datadog Karpenter deployment
  kubectl datadog autoscaling cluster update

Flags:
      --cluster-name string                                   Name of the EKS cluster
      --create-karpenter-resources CreateKarpenterResources   Which Karpenter resources to (re-)create: none (default), ec2nodeclass, all (default none)
      --debug                                                 Enable debug logs
      --inference-method InferenceMethod                      Method to infer EC2NodeClass and NodePool properties when --create-karpenter-resources is set: nodes, nodegroups (default nodegroups)
      --karpenter-version string                              Version of Karpenter to upgrade to (default to latest)
```

#### `autoscaling cluster evict-legacy-nodes`

Drains the node groups that Datadog does not manage, with the goal of having their workloads rescheduled onto the Datadog-managed Karpenter NodePools. It covers EC2 Auto Scaling groups, EKS managed node groups, user-managed Karpenter NodePools and standalone EC2 instances. Fargate profiles, and nodes whose provider cannot be identified, are out of scope and are left in place.

Before anything destructive, the command verifies its preconditions — Karpenter is installed, the cluster does not run EKS auto-mode, and at least one Datadog-managed `NodePool` exists to receive the evicted pods — then displays the eviction plan and asks for confirmation. It also warns when a user `NodePool` has a `spec.weight` greater than or equal to the Datadog ones, since evicted pods could land back on it.

Audit your PodDisruptionBudgets first: a workload whose own PDB never allows a disruption (`maxUnavailable: 0`, or `minAvailable` equal to its replica count) cannot be evicted, and its node will only fail once the timeouts have elapsed.

Once confirmed:

1. The `cluster-autoscaler` Deployment, when the cluster runs one, is scaled down to 0 replicas, so that it cannot provision new legacy nodes mid-migration (skip with `--skip-cluster-autoscaler`). The command waits for it to reach 0 replicas and aborts before any drain if it does not. It is never scaled back up — re-enabling it afterwards is manual.
2. Temporary PodDisruptionBudgets (`maxUnavailable: 1`) are created for the Deployments, StatefulSets and standalone ReplicaSets running on the targeted nodes that no PDB already covers, and removed at the end (enabled by default, disable with `--ensure-pdbs=false`). Pods owned by anything else — Jobs, CronJobs, DaemonSets, custom controllers — or by nothing at all get none.
3. Targets are processed one at a time. Each target's nodes are cordoned and drained through the Eviction API, then its capacity is retired:
   - **EC2 Auto Scaling group**: `AZRebalance` is suspended and `MinSize` set to 0 up front, each drained instance is terminated individually, and the group is finally locked at `min=max=desired=0`.
   - **EKS managed node group**: scaled to `min=desired=0`, `max` preserved, then the command waits for the group's nodes to disappear from the cluster. Raise `--node-timeout` for large node groups: it bounds that wait as a whole.
   - **Standalone EC2 instance**: terminated.
   - **User Karpenter NodePool**: its nodes are drained, but the NodePool itself is left untouched. The command does not disable it, so Karpenter may provision fresh nodes from it for the pods just evicted — lower its `spec.weight` or set its limits yourself to retire it for good.

None of this is reversible by the plugin: the previous scaling configuration is not recorded, so restoring legacy capacity is manual.

Exactly one of `--all` or `--target` is required. The command is re-runnable rather than transactional: a node that fails to drain keeps its workloads and is never terminated, and its node group is not scaled to zero — but everything already applied stays applied, so a later run resumes from there.

```console
$ kubectl datadog autoscaling cluster evict-legacy-nodes --help
Drain workloads from non-Datadog node groups onto Datadog-managed Karpenter NodePools

Usage:
  datadog autoscaling cluster evict-legacy-nodes [flags]

Examples:

  # evict every node group that is not Datadog-managed (cluster-autoscaler ASGs,
  # EKS managed node groups, user Karpenter NodePools, standalone EC2)
  kubectl datadog autoscaling cluster evict-legacy-nodes --all

  # evict a single ASG by name
  kubectl datadog autoscaling cluster evict-legacy-nodes --target=asg/my-legacy-asg

  # preview the actions without performing them
  kubectl datadog autoscaling cluster evict-legacy-nodes --all --dry-run

Flags:
      --all                          Evict every node group that is not managed by Datadog
      --cluster-name string          Name of the EKS cluster
      --debug                        Enable debug logs
      --dry-run                      Log the actions that would be taken without performing them
      --ensure-pdbs                  Create temporary PodDisruptionBudgets (maxUnavailable: 1) for workloads without one, and remove them at the end (default true)
      --eviction-timeout duration    Time budget per pod for the Eviction API to succeed before giving up (PDB-blocked pods) (default 5m0s)
      --karpenter-namespace string   Namespace where Karpenter is deployed (auto-detected when empty)
      --node-timeout duration        Time budget per node for it to become empty after pods have been evicted (default 15m0s)
      --skip-cluster-autoscaler      Do not scale the cluster-autoscaler Deployment to 0 replicas as step 1
      --target standalone            Target a specific node group: <manager>/<name>, with <manager> one of asg, eksManagedNodeGroup, karpenter. Use standalone (no name) for standalone EC2 instances. Repeatable. Mutually exclusive with --all.
      --yes                          Skip the confirmation prompt
```

#### `autoscaling cluster uninstall`

Removes the `kubectl-datadog`-managed autoscaling resources from an EKS cluster. The command:

1. Deletes the `NodePool` and `EC2NodeClass` resources it created, then waits for the corresponding EC2 instances to terminate. Every node Karpenter provisioned from them is drained and terminated, so make sure the cluster has other capacity first — in particular after `evict-legacy-nodes`, which leaves the legacy node groups scaled to zero. NodePools created by other Datadog products, or by hand, are left in place.
2. Uninstalls the Helm release named `karpenter`.
3. Removes the Karpenter node role mapping from the `aws-auth` ConfigMap when that ConfigMap is present, and deletes the instance profiles attached to that role.
4. Deletes the CloudFormation stacks, and with them the IAM roles.

Each step is independent and best-effort, so that a run interrupted halfway can simply be re-run: `uninstall` deliberately skips the ownership pre-checks `install` and `update` perform, since the resources they key off may already be gone. Point `--karpenter-namespace` at the installation to remove. The cluster OIDC provider registered by a Fargate-mode install is left in place.

```console
$ kubectl datadog autoscaling cluster uninstall --help
Uninstall autoscaling from an EKS cluster

Usage:
  datadog autoscaling cluster uninstall [flags]

Examples:

  # uninstall autoscaling
  kubectl datadog autoscaling cluster uninstall

Flags:
      --cluster-name string          Name of the EKS cluster
      --karpenter-namespace string   Name of the Kubernetes namespace where Karpenter is deployed (default "dd-karpenter")
      --yes                          Skip confirmation prompt
```

[1]: https://github.com/DataDog/helm-charts/tree/main/charts/datadog
