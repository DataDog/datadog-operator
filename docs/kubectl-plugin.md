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

The `autoscaling cluster` commands install and configure [Karpenter](https://karpenter.sh/) on an EKS cluster so that Datadog can manage cluster autoscaling. The `autoscaling dpa` commands operate `DatadogPodAutoscaler` objects during an incident; see [`autoscaling dpa`](#autoscaling-dpa).

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

1. Creates two AWS CloudFormation stacks holding the IAM roles, permissions and AWS-side plumbing Karpenter needs.
2. Installs Karpenter via Helm from the OCI registry. By default the controller runs on dedicated Fargate nodes, so that it never runs on the nodes it manages; pass `--install-mode=existing-nodes` to run it on the cluster's own nodes instead.
3. Optionally creates `EC2NodeClass` and `NodePool` Karpenter resources, inferred from existing cluster nodes or EKS node groups.

If something goes wrong, those CloudFormation stacks and that Helm release are the two places to look. The command installs nothing and exits with an explanatory message when EKS auto-mode is active, or when the cluster already runs a Karpenter installation `kubectl-datadog` does not manage in the requested namespace. Re-running it over its own installation is not a no-op: with the default `--create-karpenter-resources=all` it re-creates the `EC2NodeClass` and `NodePool`, discarding manual edits — use `update` for that.

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

Migrates a cluster off the node groups Datadog does not manage — EC2 Auto Scaling groups, EKS managed node groups, user Karpenter NodePools and standalone EC2 instances — so that their workloads can reschedule onto the Datadog-managed Karpenter NodePools. It scales down the `cluster-autoscaler` if there is one, then cordons and drains each target's nodes and scales the target down to zero, one target at a time.

Select the targets with `--all` or with one or more `--target`, and preview a run with `--dry-run`. The command displays its plan and asks for confirmation before draining anything. It is re-runnable: a node that fails to drain keeps its workloads and its instance is never terminated, so a later run can pick up where this one stopped.

The migration is one-way — the plugin does not restore the previous capacity, nor scale the `cluster-autoscaler` back up. Note also that a user Karpenter NodePool is only drained, not disabled, so Karpenter may provision new nodes from it unless you retire it yourself.

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

Removes Karpenter and the associated resources from an EKS cluster. Deletes the `NodePool` and `EC2NodeClass` resources it created, waits for the corresponding EC2 instances to terminate, uninstalls the Karpenter Helm release, cleans up IAM, and removes the CloudFormation stacks.

The nodes provisioned from the `NodePool` resources it created are drained and terminated in the process, so make sure the cluster has other capacity first; hand-created and third-party NodePools are left alone. Each step is independent and best-effort, so an interrupted run can simply be re-run.

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

#### `autoscaling dpa`

Break-glass overrides for `DatadogPodAutoscaler` (DPA) objects. They set operational annotations rather than editing the object `spec`, so they do not fight GitOps or autoscaling profiles. They require Cluster Agent 7.85.0 or later; an older one ignores them.

```console
$ kubectl datadog autoscaling dpa --help
Break-glass overrides for DatadogPodAutoscaler objects, through operational
annotations. The object spec is never modified.

Requires Cluster Agent 7.85.0 or later; an older one ignores the annotations.

Usage:
  datadog autoscaling dpa [command]

Available Commands:
  force-fallback  Force or stop forcing the local fallback of DatadogPodAutoscalers
  force-replicas  Force or stop forcing the replica count of DatadogPodAutoscalers
  force-resources Force or stop forcing container resources of DatadogPodAutoscalers
  pause           Pause or resume DatadogPodAutoscalers
```

| Command | Annotation | Effect |
|---|---|---|
| `pause enable` / `disable` | `autoscaling.datadoghq.com/pause` | Recommendations are still computed, but nothing is applied: no horizontal scaling, no vertical rollout or in-place resize, no POD patching by the admission controller. Takes precedence over every force-* override. |
| `force-fallback enable` / `disable` | `autoscaling.datadoghq.com/force-fallback` | Use the in-cluster local recommender as if recommendations from Datadog were stale, even when they are fresh. Has no effect when `spec.fallback.horizontal.enabled` is `false`. `disable` only stops *forcing* it; it does **not** disable the local fallback, which remains `spec.fallback.horizontal.enabled`. |
| `force-replicas set --replicas N` / `unset` | `autoscaling.datadoghq.com/force-replicas` | Force the replica count to `N` (at least 1), ignoring recommendations, even without any. Not clamped by `spec.constraints`, reached in one step without the scaling rules and stabilization windows. |
| `force-resources set -c <container> --requests … --limits …` / `unset [-c <container>]` | `autoscaling.datadoghq.com/force-resources` | Force cpu/memory requests and limits of the listed containers, overlaid on the vertical recommendation; everything else keeps following it. Without a vertical recommendation, nothing is forced. Bounded by `spec.constraints`. Pods may be evicted or rolled out unless in-place resize is enabled. |

Turning an override on writes its value (`"true"`, the replica count, or the resources as a JSON list of container resources in the DPA format, for example `[{"name":"app","limits":{"cpu":"4"},"requests":{"cpu":"2"}}]`); turning it off removes the annotation. `pause` and `applyPolicy.mode: Preview` take precedence over every force-* override.

`force-resources set` merges into the current annotation, so several containers can be forced one after the other: containers and fields not passed are kept. `unset -c <container>` removes only that container; `unset` without `-c` removes the whole annotation. Positional arguments are always DatadogPodAutoscaler names, containers are only named with `-c`:

```console
$ kubectl datadog autoscaling dpa force-resources set -c app --requests cpu=2 --limits cpu=4 checkout-dpa
$ kubectl datadog autoscaling dpa force-resources set -c sidecar --requests memory=512Mi checkout-dpa
$ kubectl datadog autoscaling dpa force-resources unset -c sidecar checkout-dpa
$ kubectl datadog autoscaling dpa force-resources unset checkout-dpa
```

Only `metadata.annotations` is patched; the `spec` is never touched. Objects already in the requested state are left untouched.

Each override is a `<feature> <action>` command: `enable|disable` for `pause` and `force-fallback`, `set|unset` for the overrides that carry a value (`force-replicas`, `force-resources`).

Objects are selected by name, with `-l/--label-selector`, or with `--all` (the whole namespace). `-A/--all-namespaces` only widens a selection: combined with names, it finds them in every namespace; combined with `-l` or `--all`, it widens them to the cluster (`--all -A` selects every DatadogPodAutoscaler in the cluster). There is no default selection, and `-A` alone is rejected. When more than one object is selected, the command lists the ones that will change and asks for confirmation unless `--yes` is passed, even when the others are already in the requested state. `--dry-run` prints the objects that would change, one per line, as `namespace/name: current → new`.

`pause`, `force-fallback enable`, `force-replicas` and `force-resources` wait (`--timeout`, default 30s) for the Cluster Agent to report, or stop reporting, the override in the status, and fail if it does not (`force-fallback enable` only warns): either the Cluster Agent is older than 7.85.0, or it has not reconciled the objects yet. The Cluster Agent reports:

- `pause` with the `Active` condition set to `False`, reason `LocallyPaused`;
- `force-fallback` with no condition: `enable` waits for `status.horizontal.target.source` to be `Local`, which needs fresh values from the local recommender, so an unconfirmed `enable` is reported as a warning and does not fail the command. `disable` does not wait: the source stays `Local` while recommendations from Datadog are stale;
- `force-replicas` with the `HorizontalScalingLimited` condition, message `replica count pinned to N by the … annotation`, only while the count is applied: not while paused, in `Preview` mode, or with both scaling directions disabled;
- `force-resources` with the `VerticalScalingLimited` condition, reason `ForcedByAnnotation`, listing the forced containers, only while the DatadogPodAutoscaler has a vertical recommendation.

The Cluster Agent ignores invalid values silently, so the command validates them before patching. When the status already reported the override before the change (for example, changing the forced resources of the same containers), it cannot confirm the new value and the command says so instead of waiting. Pass `--wait=false` to skip the wait, for example when changing many DatadogPodAutoscalers at once.

`pause disable` resumes every selected object, including those that were paused on purpose before an incident. Use `--dry-run` to check the selection first.

[1]: https://github.com/DataDog/helm-charts/tree/main/charts/datadog
