# Installing the Datadog Operator

This document contains detailed information about installing the Datadog Operator. For basic installation instructions for the Datadog Agent on Kubernetes, see [Install the Datadog Agent on Kubernetes][10].

## Prerequisites

- **Kubernetes Cluster version >= v1.20.X**: Tests were performed on Kubernetes versions >= `1.20.0`. It is expected to work on versions `>= v1.11.0`, but for earlier versions the Operator may not work as expected because of limited CRD support.
- **[Helm][1]** for deploying the Datadog Operator
- **[`kubectl` CLI][2]** for installing the Datadog Agent


## Install the Datadog Operator with Helm

You can deploy the Datadog Operator in your cluster using the [Datadog Operator Helm chart][3]:

```shell
helm repo add datadog https://helm.datadoghq.com
helm install my-datadog-operator datadog/datadog-operator
```

To customize the Operator configuration, create a `values.yaml` file that can override the default Helm chart values.

For instance:

```yaml
image:
  tag: 1.2.0
clusterName: my-cluster
datadogMonitor:
  enabled: true
```

Then, to update the Helm release, run:

```shell
helm upgrade my-datadog-operator datadog/datadog-operator -f values.yaml
```

### Add credentials

1. Create a Kubernetes Secret that contains your API and application keys.

   ```
   export DD_API_KEY=<YOUR_API_KEY>
   export DD_APP_KEY=<YOUR_APP_KEY>

   kubectl create secret generic datadog-operator-secret --from-literal api-key=$DD_API_KEY --from-literal app-key=$DD_APP_KEY
   ```

2. Reference this Secret in your `values.yaml` file.

   ```yaml
   apiKeyExistingSecret: datadog-operator-secret
   appKeyExistingSecret: datadog-operator-secret
   image:
     tag: 1.2.0
   datadogMonitor:
     enabled: true
   ```

3. Update the Helm release.

   ```shell
   helm upgrade my-datadog-operator datadog/datadog-operator -f values.yaml
   ```

### Configure cluster name (optional)

Setting a cluster name is optional but recommended. The cluster name can be configured in the following ways:

- **Helm chart `clusterName` value** (sets `DD_CLUSTER_NAME` environment variable on the Operator):
   ```yaml
   clusterName: my-cluster
   ```

- **DatadogAgent CRD `spec.global.clusterName`**:
   ```yaml
   apiVersion: datadoghq.com/v2alpha1
   kind: DatadogAgent
   metadata:
     name: datadog
   spec:
     global:
       clusterName: my-cluster
   ```

## Install the Datadog Operator with Operator Lifecycle Manager

Instructions for deploying the Datadog Operator with [Operator Lifecycle Manager][4] (OLM) are available at [operatorhub.io][5].

### Override the default Operator configuration with OLM

The [Operator Lifecycle Manager][4] framework allows overriding the default Operator configuration. See [Subscription Config][6] for a list of the supported installation configuration parameters.

For example, the following [Operator Lifecycle Manager][4] `Subscription` changes the Datadog Operator's Pod resources:

```yaml
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: my-datadog-operator
  namespace: operators
spec:
  channel: stable
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

### Add credentials

1. Create a Kubernetes Secret that contains your API and application keys.

   ```
   export DD_API_KEY=<YOUR_API_KEY>
   export DD_APP_KEY=<YOUR_APP_KEY>

   kubectl create secret generic datadog-operator-secret --from-literal api-key=$DD_API_KEY --from-literal app-key=$DD_APP_KEY
   ```

2. Add references to the Secret in the Datadog Operator `Subscription` resource instance. 

   ```yaml
   apiVersion: operators.coreos.com/v1alpha1
   kind: Subscription
   metadata:
     name: my-datadog-operator
     namespace: operators
   spec:
     channel: stable
     name: datadog-operator
     source: operatorhubio-catalog
     sourceNamespace: olm
     config:
       env:
         - name: DD_API_KEY
           valueFrom:
             secretKeyRef: 
                key: api-key
                name: datadog-operator-secret
         - name: DD_APP_KEY
           valueFrom:
             secretKeyRef: 
               key: app-key
               name: datadog-operator-secret
   ```


### Enable optional controllers with OLM

Each optional controller can be toggled via a CLI flag (highest precedence) or
an environment variable (middle precedence). If neither is set, the compiled
default applies.

| Controller              | CLI flag                          | Env var                                  | Default |
|-------------------------|-----------------------------------|------------------------------------------|---------|
| DatadogAgent            | `--datadogAgentEnabled`           | `DD_AGENT_CONTROLLER_ENABLED`            | `true`  |
| DatadogMonitor          | `--datadogMonitorEnabled`         | `DD_MONITOR_CONTROLLER_ENABLED`          | `false` |
| DatadogSLO              | `--datadogSLOEnabled`             | `DD_SLO_CONTROLLER_ENABLED`              | `false` |
| DatadogDashboard        | `--datadogDashboardEnabled`       | `DD_DASHBOARD_CONTROLLER_ENABLED`        | `false` |
| DatadogGenericResource  | `--datadogGenericResourceEnabled` | `DD_GENERIC_RESOURCE_CONTROLLER_ENABLED` | `false` |
| DatadogCSIDriver        | `--datadogCSIDriverEnabled`       | `DD_CSI_DRIVER_CONTROLLER_ENABLED`       | `false` |
| DatadogAgentProfile     | `--datadogAgentProfileEnabled`    | `DD_AGENT_PROFILE_CONTROLLER_ENABLED`    | `false` |
| Introspection           | `--introspectionEnabled`          | `DD_INTROSPECTION_ENABLED`               | `false` |
| RemoteConfig            | `--remoteConfigEnabled`           | `DD_REMOTE_CONFIG_ENABLED`               | `false` |
| RemoteUpdates           | `--remoteUpdatesEnabled`          | `DD_REMOTE_UPDATES_ENABLED`              | `false` |
| EKSAddOnManagedAgentInstallation       | `--eksManagedAgentInstallationEnabled`      | `DD_EKS_MANAGED_AGENT_INSTALLATION_ENABLED`         | `false` |
| OperatorMetrics         | `--operatorMetricsEnabled`        | `DD_OPERATOR_METRICS_ENABLED`            | `true`  |
| UntaintController       | `--untaintControllerEnabled`      | `DD_UNTAINT_CONTROLLER_ENABLED`          | `false` |
| UntaintWaitForCSIDriver | `--untaintControllerWaitForCSIDriver` | `DD_UNTAINT_CONTROLLER_WAIT_FOR_CSI_DRIVER` | `false` |

Other operator startup options can also be configured via environment variable:

| Option                     | CLI flag                             | Env var                               | Default |
|----------------------------|--------------------------------------|---------------------------------------|---------|
| Metrics address            | `--metrics-addr`                     | `DD_METRICS_ADDR`                     | `:8080` |
| Secure metrics             | `--metrics-secure`                   | `DD_METRICS_SECURE`                   | `false` |
| Profiling                  | `--profiling-enabled`                | `DD_PROFILING_ENABLED`                | `false` |
| Leader election lease      | `--leader-election-lease-duration`   | `DD_LEADER_ELECTION_LEASE_DURATION`   | `60s`   |
| Cilium network policies    | `--supportCilium`                    | `DD_SUPPORT_CILIUM`                   | `false` |
| Maximum goroutines         | `--maximumGoroutines`                | `DD_MAXIMUM_GOROUTINES`               | `400`   |
| DDGR max concurrent reconciles | `--datadogGenericResourceMaxConcurrentReconciles` | `DD_GENERIC_RESOURCE_MAX_CONCURRENT_RECONCILES` | `1` |
| DDGR requeue period        | `--datadogGenericResourceRequeuePeriod` | `DD_GENERIC_RESOURCE_REQUEUE_PERIOD` | `60s`   |
| Controller revisions       | `--createControllerRevisions`        | `DD_CREATE_CONTROLLER_REVISIONS`      | `false` |

ExtendedDaemonset options (`--supportExtendedDaemonset` and `--eds*`),
the leader election toggle (`--enable-leader-election`), pprof (`--pprof`),
log options (`--loglevel`, `--logEncoder`), secret backend options
(`--secretBackend*`, `--secretRefreshInterval`), and `--version` are only
configurable using CLI flags in the shipped manifests.

Boolean values follow Go's [`strconv.ParseBool`](https://pkg.go.dev/strconv#ParseBool):
`true`, `True`, `TRUE`, `1` or `false`, `False`, `FALSE`, `0`. The strings
`yes` and `no` are **not** accepted and are logged as errors, leaving the
default in effect.

Duration values follow Go's [`time.ParseDuration`](https://pkg.go.dev/time#ParseDuration);
for example, `30s`, `5m`, or `1h`. Integer values use base 10.

For example, to enable the DatadogMonitor controller in an OLM deployment:

```yaml
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: my-datadog-operator
  namespace: operators
spec:
  channel: stable
  name: datadog-operator
  source: operatorhubio-catalog
  sourceNamespace: olm
  config:
    env:
      - name: DD_MONITOR_CONTROLLER_ENABLED
        value: "true"
```

## Deploy the DatadogAgent custom resource managed by the Operator

After deploying the Datadog Operator, create the `DatadogAgent` resource that triggers the deployment of the Datadog Agent, Cluster Agent, and Cluster Checks Runners (if used) in your Kubernetes cluster. The Datadog Agent is deployed as a DaemonSet, running a pod on every node of your cluster.

1. Create a Kubernetes secret with your API and application keys.

   ```
   export DD_API_KEY=<YOUR_API_KEY>
   export DD_APP_KEY=<YOUR_APP_KEY>

   kubectl create secret generic datadog-secret --from-literal api-key=<DATADOG_API_KEY> --from-literal app-key=<DATADOG_APP_KEY>
   ```
  
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
   kubectl apply -f /path/to/your/datadog-agent.yaml
   ```

In a cluster with two worker nodes, you should see the Agent Pods created on each node.

```console
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

Update your [`datadog-agent.yaml` file][8] with the following configuration to add tolerations in the `Daemonset.spec.template` of your DaemonSet:

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

```console
$ kubectl apply -f datadog-agent.yaml
datadogagent.datadoghq.com/datadog updated
```

Validate the DaemonSet update by looking at the new `desired` Pod value:

```console
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

## Configuration

For a full list of configuration options, see the [configuration spec][12].

## Install the kubectl plugin

See the [`kubectl` plugin documentation][11].

## Use a custom Datadog Operator image

See instructions to build a Datadog Operator custom container image based on an official release in [Custom Operator container images][9].

### Datadog Operator images with Helm charts

To install a custom Datadog Operator image using the Helm chart, run the following:

```shell
helm install my-datadog-operator --set image.repository=<custom-image-repository> --set image.tag=<custom-image-tag> datadog/datadog-operator
```

## Cleanup

The following command deletes all the Kubernetes resources created by the Datadog Operator and the linked `DatadogAgent` `datadog`.

```shell
kubectl delete datadogagent datadog
```

This command outputs `datadogagent.datadoghq.com/datadog deleted`.

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
[10]: https://docs.datadoghq.com/containers/kubernetes/installation
[11]: https://github.com/DataDog/datadog-operator/blob/main/docs/kubectl-plugin.md
[12]: https://github.com/DataDog/datadog-operator/blob/main/docs/configuration.v2alpha1.md
