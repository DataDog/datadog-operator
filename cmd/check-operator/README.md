# check-operator

`check-operator` is a CLI to run checks against the operator.
The main use case is to run it as a [Helm chart test](https://helm.sh/docs/topics/chart_tests/) to validate the rolling update of the Agent, based on the DatadogAgent status and the reconcile status of its associated DatadogAgentInternal resources.

## `upgrade` settings

`check-operator upgrade <DatadogAgent name>` polls every 30 seconds. It passes when the Agent, the Cluster Agent and the Cluster Checks Runner reach their thresholds and no reconcile error is set. Set these environment variables to change the thresholds:

| Variable | Default | Effect |
|---|---|---|
| `CHECK_TIMEOUT_MINUTES` | `120` | The check fails after this time. |
| `AGENT_COMPLETION_PCT` | `95` | The Agent passes when more than this % of scheduled Agent pods are up to date. |
| `AGENT_COMPLETION_MIN` | `10` | The Agent also passes when this number of out-of-date pods or fewer is left. |
| `DCA_MIN_UP_TO_DATE` | `1` | The Cluster Agent passes with this number of up-to-date replicas. |
| `CLC_MIN_UP_TO_DATE` | `2` | The Cluster Checks Runner passes with this number of up-to-date replicas. |
| `AGENT_SKIP_NOT_READY_NODES` | `false` | When `true` and the Agent threshold is not reached, the check does not count nodes that are not Ready and whose oldest Agent pod is out of date. Kubernetes cannot replace these pods, so without this setting they hold the check until its timeout. The check skips no node if this would remove more than 10% of the scheduled nodes. |

`AGENT_SKIP_NOT_READY_NODES` needs more permissions than the other settings. Give the service account `list` on `pods` and `daemonsets` in the DatadogAgent namespace, and `get` on `nodes`. If a permission is missing, the check logs the error and uses the plain threshold.
