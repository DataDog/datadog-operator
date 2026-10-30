# check-operator

`check-operator` is a CLI to run checks against the operator.
The main use case is to run it as a [Helm chart test](https://helm.sh/docs/topics/chart_tests/) to validate the rolling update of the Agent, based on the DatadogAgent status and the reconcile status of its associated DatadogAgentInternal resources.

## `upgrade` settings

`check-operator upgrade <DatadogAgent name>` polls every 30 seconds. It passes when the Agent, the Cluster Agent and the Cluster Checks Runner reach their thresholds and no reconcile error is set. Set these environment variables to change the thresholds:

| Variable | Default | Effect |
|---|---|---|
| `CHECK_TIMEOUT_MINUTES` | `120` | The check fails after this time. |
| `AGENT_COMPLETION_PCT` | `95` | The Agent passes when more than this % of the nodes scheduled with an Agent pod run an up-to-date pod. |
| `AGENT_COMPLETION_MIN` | `10` | The Agent also passes when this number of nodes with an out-of-date pod or fewer is left. |
| `DCA_MIN_UP_TO_DATE` | `1` | The Cluster Agent passes with this number of up-to-date replicas. |
| `CLC_MIN_UP_TO_DATE` | `2` | The Cluster Checks Runner passes with this number of up-to-date replicas. |
| `AGENT_SKIP_NOT_READY_NODES` | `false` | When `true` and the Agent threshold is not reached, the check does not count nodes that are not Ready (`Ready` condition `False`, `Unknown` or missing) and whose oldest Agent pod is out of date. These nodes usually cannot run the new pod, so without this setting they hold the check until its timeout. See [Skipping not-Ready nodes](#skipping-not-ready-nodes). |
| `AGENT_SKIP_NOT_READY_MAX_PCT` | `10` | The most % of the scheduled nodes that the check can skip. Above it, the not-Ready nodes are more likely an outage, and the check skips no node. The count rounds down, so at `10` a cluster with fewer than 10 Agent nodes skips none. `100` lets it skip every not-Ready node, even in a full outage. Values outside 0–100 are ignored. |
| `AGENT_SKIP_NOT_READY_MAX_NODE_LOOKUPS` | `300` | The most nodes with an out-of-date Agent pod that the check looks up. Above it, the check skips no node. Values of 0 or less are ignored. |

### Skipping not-Ready nodes

The setting only helps if the Agent tolerates the `node.kubernetes.io/not-ready:NoSchedule` and `node.kubernetes.io/unreachable:NoSchedule` taints, for example with `tolerations: [{operator: Exists}]`. The Kubernetes node lifecycle controller adds these taints to not-Ready nodes. If the Agent does not tolerate them, the DaemonSet status already leaves these nodes out, and the setting changes nothing.

The DatadogAgent status is copied from the Agent DaemonSets when the Operator reconciles, so it can lag behind them. When the check skips nodes, it uses the live status of the Agent DaemonSets instead, and skips none while a DaemonSet status does not match its spec yet.

The check counts nodes like the DaemonSet status does, so it skips a node only if the current Agent template still targets it (node selector, affinity and taints).

The skip adds to the `AGENT_COMPLETION_PCT` tolerance. With the defaults, the check can pass with up to about 14.5% of the Agent nodes out of date: up to 10% skipped as not Ready, plus up to 5% of the remaining nodes, which can be Ready.

API calls: the check makes them only when skipping could make it pass. Then, on each poll, it lists the Agent DaemonSets and their out-of-date pods, and gets each node that runs an out-of-date pod. If a DaemonSet has not-Ready candidates, it also lists one of its up-to-date pods to read the current revision hash. The out-of-date pod list is served from the API server cache.

Permissions: give the service account `list` on `pods` and `daemonsets` in the DatadogAgent namespace, and `get` on `nodes`. Nodes are cluster-scoped, so `get` on `nodes` needs a ClusterRole. If a permission is missing, the check logs the error and uses the plain threshold.
