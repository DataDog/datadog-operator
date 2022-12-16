# Installation

## Prerequisites

The Datadog Operator requires the following:

- Kubernetes v1.14.0+
- [Helm][1] for deploying the Datadog Operator
- [`kubectl` CLI][2] for installing the `DatadogAgent`

## Deploy the Datadog Operator

You can deploy the Datadog Operator with [Helm](#helm) or with the [Operator Lifecycle Manager (OLM)](#operator-lifecycle-manager).

### Helm

To use the Datadog Operator, deploy it in your cluster using the [Datadog Operator Helm chart][3]:

   ```shell
   helm repo add datadog https://helm.datadoghq.com
   helm install my-datadog-operator datadog/datadog-operator
   ```

### Operator Lifecycle Manager

1. Install [Operator Lifecycle Manager (OLM)][4].
  ```shell
  curl -sL https://github.com/operator-framework/operator-lifecycle-manager/releases/download/v0.21.1/install.sh | bash -s v0.21.1
  ```

2. Install the Datadog Operator. The following command installs the Operator in the `operators` namespace; the Operator will be usable from all namespaces in the cluster.

  ```shell
  kubectl create -f https://operatorhub.io/install/datadog-operator.yaml
  ```

3. After install, you can watch your Operator come up:

  ```shell
  kubectl create -f https://operatorhub.io/install/datadog-operator.yaml
  ```

Datadog Operator deployment with OLM documentation is also available at [operatorhub.io][5].

#### Override default Operator configuration

The OLM framework allows overriding default Operator configuration. See the [OLM Subscription Config][6] documentation for a list of supported installation configuration parameters.

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

### Add Datadog credentials to the Operator

The Datadog Operator requires access to your API and application keys to add a `DatadogMonitor`. 

1. Create a secret that contains both keys. In the example below, the secret keys are `api-key` and `app-key`.

```
export DD_API_KEY=<replace-by-your-api-key>
export DD_APP_KEY=<replace-by-your-app-key>

kubectl create secret generic datadog-secret --from-literal api-key=$DD_API_KEY --from-literal app-key=$DD_APP_KEY
```

2. Add references to the secret in the Datadog-Operator `Subscription` resource instance. 

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

## Deploy the Datadog Agents with the Operator

After deploying the Datadog Operator, create the `DatadogAgent` resource that triggers the Datadog Agent's deployment in your Kubernetes cluster. By creating this resource, the Agent will be deployed as a `DaemonSet` on every `Node` of your cluster.

1. Create a Kubernetes secret with your API and app keys.

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

In a cluster with two worker Nodes, you should see the Agent Pods created on each Node.

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

Update your [`datadog-agent.yaml` file][8] with the following configuration to add tolerations in the `Daemonset.spec.template` of your `DaemonSet` :

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

Validate the DaemonSet update by looking at the new desired Pod value:

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

## Install the kubectl plugin

[kubectl plugin doc](/docs/kubectl-plugin.md)

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
