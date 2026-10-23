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
| `AGENT_SKIP_NOT_READY_MAX_PCT` | `10` | The most % of the scheduled nodes that the check can skip. Above it, the not-Ready nodes are more likely an outage, and the check skips no node. `100` lets it skip every not-Ready node, even in a full outage. Values outside 0–100 are ignored. |
| `AGENT_SKIP_NOT_READY_MAX_NODE_LOOKUPS` | `300` | The most nodes with an out-of-date Agent pod that the check looks up. Above it, the check skips no node. Values of 0 or less are ignored. |

### Skipping not-Ready nodes

The Kubernetes node lifecycle controller adds the `node.kubernetes.io/not-ready:NoSchedule` or `node.kubernetes.io/unreachable:NoSchedule` taint to not-Ready nodes. If the Agent does not tolerate it (for example with `tolerations: [{operator: Exists}]`), the DaemonSet status already leaves the node out, and `AGENT_SKIP_NOT_READY_NODES` changes nothing.

The check counts nodes like the DaemonSet status does, so it skips a node only if the current Agent template still targets it (node selector, affinity and taints).

API calls: the check makes them only when skipping could make it pass. Then, on each poll, it lists the Agent DaemonSets and their out-of-date pods, gets each node that runs an out-of-date pod, and lists the pods of each not-Ready one. Pod lists are served from the API server cache.

Permissions: give the service account `list` on `pods` and `daemonsets` in the DatadogAgent namespace, and `get` on `nodes`. Nodes are cluster-scoped, so `get` on `nodes` needs a ClusterRole. If a permission is missing, the check logs the error and uses the plain threshold.
