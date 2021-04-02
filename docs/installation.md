# Installation

## Prerequisites

Using the Datadog Operator requires the following prerequisites:

- **Kubernetes Cluster version >= v1.14.X**: Tests were done on versions >= `1.14.0`. Still, it should work on versions `>= v1.11.0`. For earlier versions, due to limited CRD support, the operator may not work as expected.
- [`Helm`][1] for deploying the `Datadog-operator`.
- [`Kubectl` cli][2] for installing the `Datadog-agent`.

## Deploy the Datadog Operator

### With Helm

To use the Datadog Operator, deploy it in your Kubernetes cluster. Then create a `DatadogAgent` Kubernetes resource that contains the Datadog deployment configuration:

1. Download the [Datadog Operator project zip ball][5]. Source code can be found at [`DataDog/datadog-operator`][6].
2. Unzip the project, and go into the `./datadog-operator` folder.
3. Define your namespace and operator:

   ```shell
   DD_NAMESPACE="datadog"
   DD_NAMEOP="ddoperator"
   ```

4. Create the namespace:

   ```shell
   kubectl create ns $DD_NAMESPACE
   ```

5. Install the operator with Helm:

   - Helm v2:

   ```shell
   helm install --name $DD_NAMEOP -n $DD_NAMESPACE .
   ```

   - Helm v3:

   ```shell
   helm install $DD_NAMEOP -n $DD_NAMESPACE .
   ```

### With the Operator Lifecycle Manager

The Datadog Operator deployment with [Operator Lifecycle Manager][7] documentation is available at [operatorhub.io][8].

#### Override default Operator configuration

The [Operator Lifecycle Manager][7] framework allows overriding default Operator configuration. See the [Subscription Config][9] document for a list of the supported installation configuration parameters.

For example, the Datadog Operator's Pod resources are changed with the following [Operator Lifecycle Manager][7] `Subscription`:

```yaml
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: my-datadog-operator
  namespace: operators
spec:
  channel: alpha
  name: datadog-operator
  source: operatorhubio-catalog
  sourceNamespace: olm
  config:
    resources:
      requests:
        memory: "250Mi"
        cpu: "250m"
      limits:
        memory: "250Mi"
        cpu: "500m"
```

## Deploy the Datadog Agents with the operator

After deploying the Datadog Operator, create the `DatadogAgent` resource that triggers the Datadog Agent's deployment in your Kubernetes cluster. By creating this resource in the `Datadog-Operator` namespace, the Agent will be deployed as a `DaemonSet` on every `Node` of your cluster.

The following [`datadog-agent.yaml` file][10] is the simplest configuration for the Datadog Operator:

```yaml
apiVersion: datadoghq.com/v1alpha1
kind: DatadogAgent
metadata:
  name: datadog
spec:
  credentials:
    apiKey: "<DATADOG_API_KEY>"
    appKey: "<DATADOG_APP_KEY>"
  agent:
    image:
      name: "gcr.io/datadoghq/agent:latest"
```

Replace `<DATADOG_API_KEY>` and `<DATADOG_APP_KEY>` with your [Datadog API and application keys][11], then trigger the Agent installation with the following command:

```shell
$ kubectl apply -n $DD_NAMESPACE -f datadog-agent.yaml
datadogagent.datadoghq.com/datadog created
```

You can check the state of the `DatadogAgent` ressource with:

```shell
kubectl get -n $DD_NAMESPACE dd datadog
NAME            ACTIVE   AGENT             CLUSTER-AGENT   CLUSTER-CHECKS-RUNNER   AGE
datadog-agent   True     Running (2/2/2)                                           110m
```

In a 2-worker-nodes cluster, you should see the Agent pods created on each node.

```shell
$ kubectl get -n $DD_NAMESPACE daemonset
NAME            DESIRED   CURRENT   READY   UP-TO-DATE   AVAILABLE   NODE SELECTOR   AGE
datadog-agent   2         2         2       2            2           <none>          5m30s

$ kubectl get -n $DD_NAMESPACE pod -owide
NAME                                         READY   STATUS    RESTARTS   AGE     IP            NODE
agent-datadog-operator-d897fc9b-7wbsf        1/1     Running   0          1h      10.244.2.11   kind-worker
datadog-agent-k26tp                          1/1     Running   0          5m59s   10.244.2.13   kind-worker
datadog-agent-zcxx7                          1/1     Running   0          5m59s   10.244.1.7    kind-worker2
```

### Tolerations

Update your [`datadog-agent.yaml` file][12] with the following configuration to add the toleration in the `Daemonset.spec.template` of your `DaemonSet` :

```yaml
apiVersion: datadoghq.com/v1alpha1
kind: DatadogAgent
metadata:
  name: datadog
spec:
  credentials:
    apiKey: "<DATADOG_API_KEY>"
    appKey: "<DATADOG_APP_KEY>"
  agent:
    image:
      name: "gcr.io/datadoghq/agent:latest"
    config:
      tolerations:
       - operator: Exists
```

Apply this new configuration:

```shell
$ kubectl apply -f datadog-agent.yaml
datadogagent.datadoghq.com/datadog updated
```

The DaemonSet update can be validated by looking at the new desired pod value:

```shell
$ kubectl get -n $DD_NAMESPACE daemonset
NAME            DESIRED   CURRENT   READY   UP-TO-DATE   AVAILABLE   NODE SELECTOR   AGE
datadog-agent   3         3         3       3            3           <none>          7m31s

$ kubectl get -n $DD_NAMESPACE pod
NAME                                         READY   STATUS     RESTARTS   AGE
agent-datadog-operator-d897fc9b-7wbsf        1/1     Running    0          15h
datadog-agent-5ctrq                          1/1     Running    0          7m43s
datadog-agent-lkfqt                          0/1     Running    0          15s
datadog-agent-zvdbw                          1/1     Running    0          8m1s
```

## Install the kubectl plugin

[kubctl plugin doc](/docs/kubectl-plugin.md)

## Cleanup

The following command deletes all the Kubernetes resources created by the Datadog Operator and the linked `DatadogAgent` `datadog`.

```shell
$ kubectl delete -n $DD_NAMESPACE datadogagent datadog
datadogagent.datadoghq.com/datadog deleted
```

You can then remove the Datadog-Operator with the `helm delete` command:

```shell
helm delete $DD_NAMEOP -n $DD_NAMESPACE
```

[1]: https://helm.sh
[2]: https://kubernetes.io/docs/tasks/tools/install-kubectl/
[3]: https://www.openshift.com/learn/topics/operators
[4]: https://github.com/operator-framework/operator-sdk
[5]: https://github.com/DataDog/datadog-operator/releases/latest
[6]: https://github.com/DataDog/datadog-operator
[7]: https://olm.operatorframework.io/
[8]: https://operatorhub.io/operator/datadog-operator
[9]: https://github.com/operator-framework/operator-lifecycle-manager/blob/main/doc/design/subscription-config.md#subscription-config
[10]: https://github.com/DataDog/datadog-operator/blob/main/examples/datadog-agent.yaml
[11]: https://app.datadoghq.com/account/settings#api
[12]: https://github.com/DataDog/datadog-operator/blob/main/examples/datadog-agent-with-tolerations.yaml
