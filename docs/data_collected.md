# Data collected

The Datadog Operator sends metrics and events to Datadog to monitor the Datadog Agent components deployment in the cluster.

## Metrics

| Metric name                                              | Metric type | Description                                                                                                                         |
| -------------------------------------------------------- | ----------- | ----------------------------------------------------------------------------------------------------------------------------------- |
| `datadog.operator.agent.deployment.success`              | gauge       | `1` if the desired number of Agent replicas equals the number of available Agent pods, `0` otherwise.                               |
| `datadog.operator.clusteragent.deployment.success`       | gauge       | `1` if the desired number of Cluster Agent replicas equals the number of available Cluster Agent pods, `0` otherwise.               |
| `datadog.operator.clustercheckrunner.deployment.success` | gauge       | `1` if the desired number of Cluster Check Runner replicas equals the number of available Cluster Check Runner pods, `0` otherwise. |
| `datadog.operator.reconcile.success`                     | gauge       | `1` if the last recorded reconcile error is null, `0` otherwise. The `reconcile_err` tag describes the last recorded error.         |

**Note:** The [Datadog API and app keys][1] are required to forward metrics to Datadog. They must be provided in the `credentials` field in the Custom Resource definition.

The Datadog Operator exposes Golang and Controller metrics in OpenMetrics format. For now they can be collected using the [OpenMetrics integration][2]. A Datadog integration will be available in the future.

The OpenMetrics check is activated by default via [Autodiscovery annotations][3] and is scheduled by the Agent running on the same node as the Datadog Operator Pod.

## Events

- Detect/Delete Custom Resource <Namespace/Name>
- Create/Update/Delete Service <Namespace/Name>
- Create/Update/Delete ConfigMap <Namespace/Name>
- Create/Update/Delete DaemonSet <Namespace/Name>
- Create/Update/Delete ExtendedDaemonSet <Namespace/Name>
- Create/Update/Delete Deployment <Namespace/Name>
- Create/Update/Delete ClusterRole </Name>
- Create/Update/Delete Role <Namespace/Name>
- Create/Update/Delete ClusterRoleBinding </Name>
- Create/Update/Delete RoleBinding <Namespace/Name>
- Create/Update/Delete Secret <Namespace/Name>
- Create/Update/Delete PDB <Namespace/Name>
- Create/Delete ServiceAccount <Namespace/Name>

[1]: https://docs.datadoghq.com/account_management/api-app-keys/
[2]: https://docs.datadoghq.com/integrations/openmetrics/
[3]: ./chart/datadog-operator/templates/deployment.yaml
