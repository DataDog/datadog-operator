# Getting Started

This page describes the simplest and fastest way to deploy the Datadog Agent with the Operator.
For more details on how to install the Operator and configure the Agent it deploys, refer to the [installation guide](installation.md).

## Prerequisites

Using the Datadog Operator requires the following prerequisites:

- **Kubernetes Cluster version >= v1.20.X**: Tests were done on versions >= `1.20.0`. Still, it should work on versions `>= v1.11.0`. For earlier versions, because of limited CRD support, the Operator may not work as expected.
- **[Helm][1]** for deploying the Datadog Operator
- **[`kubectl` CLI][2]** for installing the Datadog Agent

## Deploy the Agent with the Operator

To deploy the Datadog Agent with the Operator in the minimum number of steps, use the [`datadog-operator` Helm chart](https://github.com/DataDog/helm-charts/tree/main/charts/datadog-operator).

1. Install the [Datadog Operator][3]:

   ```shell
   helm repo add datadog https://helm.datadoghq.com
   helm install my-datadog-operator datadog/datadog-operator
   ```

1. Create a Kubernetes Secret with your API and app keys:

   ```shell
   kubectl create secret generic datadog-secret --from-literal api-key=<DATADOG_API_KEY> --from-literal app-key=<DATADOG_APP_KEY>
   ```
   Replace `<DATADOG_API_KEY>` and `<DATADOG_APP_KEY>` with your [Datadog API and application keys][4].

1. Create a file with the spec of your `DatadogAgent` deployment configuration. The simplest configuration is:

   ```yaml
   apiVersion: datadoghq.com/v2alpha1
   kind: DatadogAgent
   metadata:
     name: datadog
   spec:
     global:
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

### Installation options

The [configuration][5] page lists all the Datadog Agent and Cluster Agent features and options that can be configured with the `DatadogAgent` resource.

### Configure integrations
Visit the [Integrations Autodiscover][9] page for details about how to configure Agent Integrations when using the Datadog Operator

#### Containers registry

To change the default registry ([gcr.io/datadoghq][6]) to another registry, use the option `spec.registry`.

The example [`datadog-agent-with-registry.yaml` file][7] demonstrates how to configure the Operator to use the [public.ecr.aws/datadog][8] registry.

```yaml
apiVersion: datadoghq.com/v2alpha1
kind: DatadogAgent
metadata:
  name: datadog
spec:
  global:
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
[5]: https://github.com/DataDog/datadog-operator/blob/main/docs/configuration.v2alpha1.md
[6]: ttps://gcr.io/datadoghq
[7]: https://github.com/DataDog/datadog-operator/blob/main/examples/datadogagent/v2alpha1/datadog-agent-with-registry.yaml
[8]: https://gallery.ecr.aws/datadog/
[9]: /docs/integrations_autodiscovery.md