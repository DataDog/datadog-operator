# Quick Start

To use the Datadog Operator, deploy it in your Kubernetes cluster. Then create a `DatadogAgentDeployment` Kubernetes resource that contains the Datadog deployment configuration.

## Deploy the Datadog Operator

Find the Datadog Operator Helm chart in `/chart/datadog-operator`:

```console
$ DD_NAMESPACE="datadog"
$ DD_NAMEOP="ddoperator"
$ kubectl create ns $DD_NAMESPACE
$ helm install --name $DD_NAMEOP -n $DD_NAMESPACE ./chart/datadog-operator
```

## Deploy the Datadog Agents with the operator

After deploying the Datadog Operator, create the `DatadogAgentDeployment` resource that will trigger the Datadog Agent's deployment in your Kubernetes cluster.

The following is the simplest configuration for the Datadog Operator:

```yaml
apiVersion: datadoghq.com/v1alpha1
kind: DatadogAgentDeployment
metadata:
  name: datadog-agent
spec:
  credentials:
    apiKey: <paste-your-api-key-here>
  agent:
    image:
      name: "datadog/agent:latest"
```

By creating this resource in the `Datadog-Operator` namespace, the Agent will be deployed as a `DaemonSet` on every `Node` of your cluster.

After adding your own Datadog API key in the `examples/datadog-agent.yaml` file, you can trigger the Agent installation with the following command:

```console
$ kubectl apply -n $DD_NAMESPACE -f examples/datadog-agent.yaml
datadogagentdeployment.datadoghq.com/datadog-agent created
```

You can check the state of the `DatadogAgentDeployment` `datadog` with:

```console
kubectl get -n $DD_NAMESPACE dad datadog-agent
NAME             ACTIVE   AGENT     CLUSTER-AGENT   AGE
datadog-agent    True     Running                   4m2s
```

In a 2-worker-nodes cluster, you should see the Agent pods created on each node.

```console
$ kubectl get -n $DD_NAMESPACE ds
NAME            DESIRED   CURRENT   READY   UP-TO-DATE   AVAILABLE   NODE SELECTOR   AGE
datadog-agent   2         2         2       2            2           <none>          5m30s

$ kubectl get -n $DD_NAMESPACE pod -owide
NAME                                         READY   STATUS    RESTARTS   AGE     IP            NODE
agent-datadog-operator-d897fc9b-7wbsf        1/1     Running   0          1h      10.244.2.11   kind-worker
datadog-agent-k26tp                          1/1     Running   0          5m59s   10.244.2.13   kind-worker
datadog-agent-zcxx7                          1/1     Running   0          5m59s   10.244.1.7    kind-worker2
```

If you now update the `DatadogAgentDeployment` `datadog-agent` by using `examples/datadog-agent-with-tolerations.yaml`, the `DaemonSet` will be updated in order to add the toleration in the `Daemonset.spec.template`. (Remember to also put your Datadog API key in this file.)

```console
diff examples/datadog-agent.yaml examples/datadog-agent-with-tolerations.yaml
10a11,13
>     config:
>       tolerations:
>        - operator: Exists

$ kubectl apply -f examples/datadog-agent-with-tolerations.yaml
datadogagentdeployment.datadoghq.com/datadog-agent updated
```

The DaemonSet update can be validated by looking at the new desired pod value:

```console
$ kubectl get -n $DD_NAMESPACE ds
NAME            DESIRED   CURRENT   READY   UP-TO-DATE   AVAILABLE   NODE SELECTOR   AGE
datadog-agent   3         3         3       3            3           <none>          7m31s

$ kubectl get -n $DD_NAMESPACE pod
NAME                                         READY   STATUS     RESTARTS   AGE
agent-datadog-operator-d897fc9b-7wbsf        1/1     Running    0          15h
datadog-agent-5ctrq                          1/1     Running    0          7m43s
datadog-agent-lkfqt                          0/1     Running    0          15s
datadog-agent-zvdbw                          1/1     Running    0          8m1s
```

The last experimentation step is to deploy the Cluster Agent along with the Agent. To do this, update the current `DatadogAgentDeployment` `datadog-agent` with the `examples/datadog-agent-with-clusteragent.yaml` file:

```yaml
apiVersion: datadoghq.com/v1alpha1
kind: DatadogAgentDeployment
metadata:
  name: datadog-agent
spec:
  credentials:
    apiKey: <paste-your-api-key-here>
    appKey: <paste-your-app-key-here>
    token: <paste-your-cluster-agent-token-here>
  agent:
    image:
      name: "datadog/agent:latest"
    config:
      tolerations:
      - operator: Exists
  clusterAgent:
    image:
      name: "datadog/cluster-agent:latest"
    config:
      metricsProviderEnabled: true
      clusterChecksEnabled: true
    replicas: 2
```

As you can see, a `clusterAgent` section has been added. You can compare this to the previous `datadog-agent` version to see what has been changed.

This `clusterAgent` section enables you to add several options:

```console
$ kubectl apply -n $DD_NAMESPACE -f examples/datadog-agent-with-clusteragent.yaml
datadogagentdeployment.datadoghq.com/datadog-agent configured

$ kubectl get -n $DD_NAMESPACE dad datadog-agent
NAME            ACTIVE   AGENT     CLUSTER-AGENT   AGE
datadog-agent   True     Running   Running         15m22s

$ kubectl get -n $DD_NAMESPACE deployment datadog-agent-cluster-agent
NAME                          READY   UP-TO-DATE   AVAILABLE   AGE
datadog-agent-cluster-agent   2/2     2            2           21s
```

The "datadog-agent" `DaemonSet` has also been updated to get the new configuration for using the `Cluster-Agent` deployment pods.

```console
$kubectl get pod
NAME                                         READY   STATUS    RESTARTS   AGE
datadog-operator-6f49889b99-vlscz            1/1     Running   0          15h
datadog-agent-22x44                          1/1     Running   0          40s
datadog-agent-665dr                          1/1     Running   0          50s
datadog-agent-cluster-agent-9f9c5c4c-2v9f7   1/1     Running   0          58s
datadog-agent-cluster-agent-9f9c5c4c-pmhqb   1/1     Running   0          58s
datadog-agent-hjlbg                          1/1     Running   0          33s
```

## Cleanup

The following command will delete all the Kubernetes resources created by the Datadog Operator and the linked `DatadogAgentDeployment` `datadog-agent`.

```console
$ kubectl delete -n $DD_NAMESPACE dad datadog-agent 
datadogagentdeployment.datadoghq.com/datadog-agent deleted
```

You can then remove the Datadog-Operator with the `helm delete` command:

```console
$ helm delete $DD_NAMEOP -n $DD_NAMESPACE
```
