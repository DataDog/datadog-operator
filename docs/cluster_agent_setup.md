# Cluster Agent

By default, the Cluster Agent is deployed along with the Agent.

The Cluster Agent configuration is defined in the `spec.override.clusterAgent` section, for example: [`datadog-agent-with-clusteragent.yaml` file][1]:

```yaml
apiVersion: datadoghq.com/v2alpha1
kind: DatadogAgent
metadata:
  name: datadog
spec:
  # Credentials to communicate between:
  #  * Agents and Datadog (API/APP key)
  #  * Node Agent and Cluster Agent (Token)
  global:
    credentials:
      apiKey: "<DATADOG_API_KEY>"
      appKey: "<DATADOG_APP_KEY>"
    clusterAgentToken: "<DATADOG_CLUSTER_AGENT_TOKEN>"

  override:
    # Node Agent configuration
    nodeAgent:
      tolerations:
        - operator: Exists

    # Cluster Agent configuration
    clusterAgent:
      replicas: 2

  # Enable Features
  features:
    externalMetricsServer:
      enabled: true
      useDatadogMetrics: false
    clusterChecks:
      enabled: true
  ```

**Note**: `<DATADOG_CLUSTER_AGENT_TOKEN>` is a custom 32 characters long token that you can define. If it is omitted, a random one is generated automatically.

Then apply it with:

```shell
$ kubectl apply -n $DD_NAMESPACE -f datadog-agent-with-clusteragent.yaml
datadogagent.datadoghq.com/datadog configured
```

Verify that the Agent and Cluster Agent are correctly running:

```shell
$ kubectl get -n $DD_NAMESPACE dd datadog
NAME            ACTIVE   AGENT             CLUSTER-AGENT     CLUSTER-CHECKS-RUNNER   AGE
datadog-agent   True     Running (2/2/2)   Running (2/2/2)                           6m

$ kubectl get -n $DD_NAMESPACE deployment datadog-cluster-agent
NAME                          READY   UP-TO-DATE   AVAILABLE   AGE
datadog-cluster-agent   2/2     2            2           21s
```

The "datadog-agent" `DaemonSet` has also been updated to get the new configuration for using the `Cluster-Agent` deployment pods.

```shell
$ kubectl get pod
NAME                                         READY   STATUS    RESTARTS   AGE
datadog-operator-6f49889b99-vlscz            1/1     Running   0          15h
datadog-agent-22x44                          1/1     Running   0          40s
datadog-agent-665dr                          1/1     Running   0          50s
datadog-cluster-agent-9f9c5c4c-2v9f7         1/1     Running   0          58s
datadog-cluster-agent-9f9c5c4c-pmhqb         1/1     Running   0          58s
datadog-agent-hjlbg                          1/1     Running   0          33s
```

[1]: https://github.com/DataDog/datadog-operator/blob/main/examples/datadogagent/datadog-agent-with-clusteragent.yaml
