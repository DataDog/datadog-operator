# Installation

## Prerequisites

- **Kubernetes Cluster version >= v1.20.X**: Tests were done on versions >= `1.20.0`. Still, it should work on versions `>= v1.11.0`. For earlier versions, because of limited CRD support, the Operator may not work as expected.
- **[Helm][1]** for deploying the Datadog Operator
- **[`kubectl` CLI][2]** for installing the Datadog Agent

## 1. Deploy the Datadog Operator

### With Helm

To use the Datadog Operator, deploy it in your cluster using the [Datadog Operator Helm chart][3]:

   ```shell
   helm repo add datadog https://helm.datadoghq.com
   helm install my-datadog-operator datadog/datadog-operator
   ```

### With the Operator Lifecycle Manager

The Datadog Operator deployment with [Operator Lifecycle Manager][4] documentation is available at [operatorhub.io][5].

#### Override default Operator configuration

The [Operator Lifecycle Manager][4] framework allows overriding default Operator configuration. See the [Subscription Config][6] document for a list of the supported installation configuration parameters.

For example, the Datadog Operator's Pod resources are changed with the following [Operator Lifecycle Manager][4] `Subscription`:

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

### 2. Add Datadog credentials to the Operator

The Datadog Operator requires access to your API and application keys to add a [Datadog monitor](https://docs.datadoghq.com/monitors/). 

1. Create a Kubernetes Secret that contains your API and application keys. In the example below, the secret keys are `api-key` and `app-key`.

```
export DD_API_KEY=<replace-by-your-api-key>
export DD_APP_KEY=<replace-by-your-app-key>

kubectl create secret generic datadog-secret --from-literal api-key=$DD_API_KEY --from-literal app-key=$DD_APP_KEY
```

2. Add references to the Secret in the Datadog Operator `Subscription` resource instance. 

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
    env:
      - name: DD_API_KEY
        valueFrom:
          secretKeyRef: 
             key: api-key
             name: datadog-secret
      - name: DD_APP_KEY
        valueFrom:
          secretKeyRef: 
            key: app-key
            name: datadog-secret
```

**Note**: You can add configuration overrides in the `spec.config` section. For example, override `env` and `resources`.

## 3. Deploy the Datadog Agents with the Operator

After deploying the Datadog Operator, create the `DatadogAgent` resource that triggers the Datadog Agent's deployment in your Kubernetes cluster. The creation of this resource deploys the Agent as a DaemonSet on every Node of your cluster.

1. Create a Kubernetes secret with your API and APP keys

   ```shell
   kubectl create secret generic datadog-secret --from-literal api-key=<DATADOG_API_KEY> --from-literal app-key=<DATADOG_APP_KEY>
   ```
   Replace `<DATADOG_API_KEY>` and `<DATADOG_APP_KEY>` with your [Datadog API and application keys][7]

1. Create a file with the spec of your `DatadogAgent` deployment configuration. The simplest configuration is:

   ```yaml
   apiVersion: datadoghq.com/v1alpha1
   kind: DatadogAgent
   metadata:
     name: datadog
   spec:
     credentials:
       apiSecret:
         secretName: datadog-secret
         keyName: api-key
       appSecret:
         secretName: datadog-secret
         keyName: app-key
   ```

1. Deploy the Datadog Agent with the above configuration file:
   ```shell
   kubectl apply -f agent_spec=/path/to/your/datadog-agent.yaml
   ```

In a 2-worker-nodes cluster, you should see the Agent Pods created on each node.

```shell
$ kubectl get daemonset
NAME            DESIRED   CURRENT   READY   UP-TO-DATE   AVAILABLE   NODE SELECTOR   AGE
datadog-agent   2         2         2       2            2           <none>          5m30s

$ kubectl get pod -owide
NAME                                         READY   STATUS    RESTARTS   AGE     IP            NODE
agent-datadog-operator-d897fc9b-7wbsf        1/1     Running   0          1h      10.244.2.11   kind-worker
datadog-agent-k26tp                          1/1     Running   0          5m59s   10.244.2.13   kind-worker
datadog-agent-zcxx7                          1/1     Running   0          5m59s   10.244.1.7    kind-worker2
```

### Tolerations

Update your [`datadog-agent.yaml` file][8] with the following configuration to add the toleration in the `Daemonset.spec.template` of your DaemonSet:

   ```yaml
   apiVersion: datadoghq.com/v1alpha1
   kind: DatadogAgent
   metadata:
     name: datadog
   spec:
     credentials:
       apiSecret:
         secretName: datadog-secret
         keyName: api-key
       appSecret:
         secretName: datadog-secret
         keyName: app-key
     agent:
       config:
         tolerations:
          - operator: Exists
   ```
Apply this new configuration:

```shell
$ kubectl apply -f datadog-agent.yaml
datadogagent.datadoghq.com/datadog updated
```

Validate the DaemonSet update by looking at the new `desired` Pod value:

```shell
$ kubectl get daemonset
NAME            DESIRED   CURRENT   READY   UP-TO-DATE   AVAILABLE   NODE SELECTOR   AGE
datadog-agent   3         3         3       3            3           <none>          7m31s

$ kubectl get pod
NAME                                         READY   STATUS     RESTARTS   AGE
agent-datadog-operator-d897fc9b-7wbsf        1/1     Running    0          15h
datadog-agent-5ctrq                          1/1     Running    0          7m43s
datadog-agent-lkfqt                          0/1     Running    0          15s
datadog-agent-zvdbw                          1/1     Running    0          8m1s
```

## Install the `kubectl` plugin

See the [`kubectl` plugin doc](/docs/kubectl-plugin.md)


## Use a custom Datadog Operator image

you can find the instruction to build a Datadog Operator custom container image based on an official release [custom-operator-image.md][9]

### with the helm chart

```console
helm install my-datadog-operator --set image.repository=<custom-image-repository> --set image.tag=<custom-image-tag> datadog/datadog-operator
```


## Cleanup

The following command deletes all the Kubernetes resources created by the Datadog Operator and the linked `DatadogAgent` `datadog`.

```shell
$ kubectl delete datadogagent datadog
datadogagent.datadoghq.com/datadog deleted
```

You can then remove the Datadog Operator with the `helm delete` command:

```shell
helm delete my-datadog-operator
```

[1]: https://helm.sh
[2]: https://kubernetes.io/docs/tasks/tools/install-kubectl/
[3]: https://artifacthub.io/packages/helm/datadog/datadog-operator
[4]: https://olm.operatorframework.io/
[5]: https://operatorhub.io/operator/datadog-operator
[6]: https://github.com/operator-framework/operator-lifecycle-manager/blob/master/doc/design/subscription-config.md
[7]: https://app.datadoghq.com/account/settings#api
[8]: https://github.com/DataDog/datadog-operator/blob/main/examples/datadogagent/datadog-agent-with-tolerations.yaml
[9]: https://github.com/DataDog/datadog-operator/blob/main/docs/custom-operator-image.md
