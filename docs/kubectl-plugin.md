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
  dashboard    Show the health of the DatadogAgent, its profiles and its workloads
  flare        Collect a Datadog's Operator flare and send it to Datadog
  get          Get DatadogAgent deployment(s)
  helm2dda     Map Datadog Helm values to DatadogAgent CRD schema
  help         Help about any command
  metrics
  validate

```

### Dashboard

`kubectl datadog dashboard` (alias `dash`) shows the state of the DatadogAgent in one view:
- the DatadogAgent, its DatadogAgentProfiles and their DatadogAgentInternal objects;
- the Agent DaemonSet and the Cluster Agent, Cluster Checks Runner and OTel Agent Gateway Deployments, with ready, up-to-date and unavailable counts and rollout progress;
- deduplicated errors and warnings;
- a summary of the other Datadog resources (monitors, dashboards, SLOs, …).

**Modes.**
- **Live (default on a terminal):** a full-screen view that updates from watches until `q` or `Ctrl-C`. When it is taller than the terminal, scroll with `↑`/`↓` (`k`/`j`), `PgUp`/`PgDn` (`b`, `f` or space) and `Home`/`End` (`g`/`G`); the footer shows the visible lines.
- **Static:** with `--once`, with `-o json`, or when the output is not a terminal (for example a pipe), it prints one snapshot and exits.

**Requirements.**
- Datadog Operator 1.21 or later: the DatadogAgentInternal CRD is required. The DatadogAgentProfile CRD is optional.
- The command expects one DatadogAgent in scope. With several, it lists them and exits non-zero unless you pass a name.

**Permissions.** The command is read-only: it only uses `get`, `list` and `watch`.
- A missing permission degrades only the affected panel, which shows `no access`.
- Helm release history is read from the release Secrets when the DatadogAgent is Helm-managed. Only chart name, version, revision, status and time are used. Pass `--no-helm` to skip it.

**Reading the view.**
- **Badges:** `✓` healthy, `↻` progressing, `⚠` degraded, `✗` error, `?` unknown. A row's badge is the worst of its own conditions and its children.
- **Rollout bar:** `█` updated and ready, `▒` updated but not ready, `░` not updated. The percentage is updated-and-ready pods out of desired.
- **Rollout phase:**
  - `Rolling` while pods are being updated (`Stalled` with `--pods` when there's no progress for `--stall-after`).
  - `Complete` once every pod is updated.
  - When every pod is updated but more than `--max-unavailable` pods are unavailable, the row shows `Settling` for 5 minutes, then `Complete` with a Degraded badge and an `N unavailable (> T allowed)` warning.

```console
$ kubectl datadog dashboard --help
Show the health of the DatadogAgent, its profiles, internals and workloads, with rollout progress, errors and a summary of the other Datadog resources. On a terminal the view is live and updates from watches until q or Ctrl-C; with --once, -o or when the output is not a terminal it prints one snapshot. The command is read-only.

Usage:
  datadog dashboard [DatadogAgent name] [flags]

Aliases:
  dashboard, dash

Examples:

  # watch the DatadogAgent of the current namespace (live on a terminal)
  kubectl datadog dashboard

  # print a static snapshot and exit
  kubectl datadog dashboard --once

  # look for the DatadogAgent in all namespaces, and show failing agent pods
  kubectl datadog dashboard -A --pods

  # emit the dashboard as JSON
  kubectl datadog dashboard -n datadog -o json

  # hide the healthy profiles with no agent pods, largest profiles first
  kubectl datadog dashboard --hide-empty --sort=desired

  # also hide the profiles with no agent pods that carry stale warnings
  kubectl datadog dashboard --hide-empty-force

  # tolerate 1% unavailable agent pods per DaemonSet once a rollout converged
  kubectl datadog dashboard --max-unavailable=1%

Flags:
  -A, --all-namespaces           Look for the DatadogAgent in all namespaces
      --ascii                    Use ASCII characters only
  -h, --help                     help for dashboard
      --hide-empty               Hide the healthy profiles whose agent DaemonSet has 0 desired pods; profiles with an issue, a rollout or no DaemonSet stay shown
      --hide-empty-force         Like --hide-empty, but also hide the profiles with 0 desired pods that have warnings or errors, and their issues; profiles with a rollout in progress, no status, no DDAI or no DaemonSet stay shown
      --log-file string          Live view only: write the client logs and warnings to this file instead of discarding them
      --max-unavailable string   Unavailable agent pods tolerated per DaemonSet or Deployment once its rollout converged, as a count or a percentage of desired pods rounded down (e.g. 2 or 1%); more show Settling for 5 minutes after the rollout, then Degraded with a warning (default "0")
  -n, --namespace string         If present, the namespace scope for this CLI request
      --no-color                 Disable colors (also set by the NO_COLOR environment variable)
      --no-helm                  Skip the Helm release history lookup (no Secret access)
      --once                     Print a static snapshot and exit instead of the live view
  -o, --output string            Output format: "" (styled text) or "json"
      --poll-interval duration   Live view only: how often the polled sources (other Datadog resources, operator and its lease) refresh; watched resources update immediately (minimum 5s) (default 30s)
      --pods                     Read the agent pods: enables Stalled detection and shows failing pods and their nodes
      --sort string              Profile order: "name" or "desired" (agent DaemonSet desired pods, most first; profiles without a DaemonSet last) (default "name")
      --stall-after duration     Time without progress before a rollout is Stalled; only with --pods (default 10m0s)
```

The command also accepts the standard kubeconfig flags (`--kubeconfig`, `--context`, `--as`, …).

**Notes.**
- **Stale profiles:** on clusters with many unused profiles, `--hide-empty` hides healthy profiles with 0 desired pods. `--hide-empty-force` also hides those carrying warnings, together with their issues. Either way the view ends with a line counting what was hidden, and the totals and DatadogAgent health still include hidden profiles.
- **`--max-unavailable`** is a runtime health threshold, separate from the DaemonSet's `rollingUpdate.maxUnavailable` rollout budget. The default `0` reports any unavailable pod once the rollout has settled.
- **Refresh:** the DatadogAgent, profiles, internals and workloads are watched and update immediately. The other Datadog resources and the operator are polled every `--poll-interval` (default 30s), and the header counts down to the next poll.
- **`--pods`** adds a watch on the Agent pods (label-selected, in the DatadogAgent namespace). Leave it off on very large clusters if memory or API load matters.

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

[1]: https://github.com/DataDog/helm-charts/tree/main/charts/datadog
