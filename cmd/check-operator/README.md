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
| `AGENT_SKIP_NOT_READY_NODES` | `false` | When `true` and the Agent threshold is not reached, the check does not count nodes that are not Ready (`Ready` condition `False` or `Unknown`) and whose oldest Agent pod is out of date. These nodes usually cannot run the new pod, so without this setting they hold the check until its timeout. The check skips no node if this would remove more than `AGENT_SKIP_NOT_READY_MAX_PCT` of the scheduled nodes, or if more than `AGENT_SKIP_NOT_READY_MAX_NODE_LOOKUPS` nodes run an out-of-date Agent pod. |
| `AGENT_SKIP_NOT_READY_MAX_NODE_LOOKUPS` | `300` | With `AGENT_SKIP_NOT_READY_NODES`, the most nodes with an out-of-date Agent pod that the check looks up (one `GET` each). Above it, the check skips no node. |
| `AGENT_SKIP_NOT_READY_MAX_PCT` | `10` | With `AGENT_SKIP_NOT_READY_NODES`, the most % of the scheduled nodes that the check can skip. Above it, the not-Ready nodes are more likely an outage, and the check skips no node. Values outside 0–100 are ignored. |

`AGENT_SKIP_NOT_READY_NODES` needs more permissions than the other settings. Give the service account `list` on `pods` and `daemonsets` in the DatadogAgent namespace, and `get` on `nodes`. If a permission is missing, the check logs the error and uses the plain threshold.
