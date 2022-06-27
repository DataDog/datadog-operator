# Getting Started

This procedure describes the simplest and fastest way to deploy the Datadog agent with the operator.
For a more complete description of a more versatile way to install the operator and configure the agent it deploys, please refer to the [Installation guide](installation.md).

## Prerequisites

Using the Datadog Operator requires the following prerequisites:

- **Kubernetes Cluster version >= v1.14.X**: Tests were done on versions >= `1.14.0`. Still, it should work on versions `>= v1.11.0`. For earlier versions, due to limited CRD support, the operator may not work as expected.
- [`Helm`][1] for deploying the `Datadog-operator`.
- [`Kubectl` cli][2] for installing the `Datadog-agent`.

## Deploy the Agent with the operator

To deploy the Datadog Agent with the operator in the minimum number of steps, see the [`datadog-operator`](https://github.com/DataDog/helm-charts/tree/main/charts/datadog-operator) helm chart.
Here are the steps:

1. Install the [Datadog Operator][3]:

   ```shell
   helm repo add datadog https://helm.datadoghq.com
   helm install my-datadog-operator datadog/datadog-operator
   ```

1. Create a Kubernetes secret with your API and APP keys

   ```shell
   kubectl create secret generic datadog-secret --from-literal api-key=<DATADOG_API_KEY> --from-literal app-key=<DATADOG_APP_KEY>
   ```
   Replace `<DATADOG_API_KEY>` and `<DATADOG_APP_KEY>` with your [Datadog API and application keys][4]

1. Create a file with the spec of your DatadogAgent deployment configuration. The simplest configuration is:

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

1. Refer to the ['Datadog Agent examples subfolder'](https://github.com/DataDog/datadog-operator/tree/main/examples/datadogagent)] and select an appropriate Datadog Agent configuration file.

1. Deploy the Datadog agent with the above configuration file:
   ```shell
   kubectl apply -f /path/to/your/datadog-agent.yaml
   ```

NOTE: If the Kubernetes cluster that you are deploying the Datadog Agent (as an Operator) to is behind a proxy server, the recommended Datadog Agent configuration file to use will be `datadog-agent-all-with-prox.yaml`

### Installation option

The [configuration][5] page lists all the Datadog Agent and Cluster Agent features and options that can be configured with the `DatadogAgent` resource.

#### Containers registry

The default registry ([gcr.io/datadoghq][6]) can be change to any other registry with the option `spec.registry`.

Use the [`datadog-agent-with-registry.yaml` example file][7] to configure the operator to use the [public.ecr.aws/datadog][8] registry.

```yaml
apiVersion: datadoghq.com/v1alpha1
kind: DatadogAgent
metadata:
  name: datadog
spec:
  registry: public.ecr.aws/datadog
  # ...
```

## Cleanup

The following command deletes all the Kubernetes resources created by the above instructions:

```shell
kubectl delete datadogagent datadog
helm delete my-datadog-operator
```

[1]: https://helm.sh
[2]: https://kubernetes.io/docs/tasks/tools/install-kubectl/
[3]: https://artifacthub.io/packages/helm/datadog/datadog-operator
[4]: https://app.datadoghq.com/account/settings#api
[5]: https://github.com/DataDog/datadog-operator/blob/main/docs/configuration.md
[6]: ttps://gcr.io/datadoghq
[7]: https://github.com/DataDog/datadog-operator/blob/main/examples/datadogagent/datadog-agent-with-registry.yaml
[8]: https://gallery.ecr.aws/datadog/