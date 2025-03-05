# Migrate DatadogAgent CRDs to v2alpha1

This page discusses how to convert your DatadogAgent Custom Resources Definitions (CRDs) from `v2alpha` to version `v2alpha1` used by the Datadog Operator v1.0.0.

## Prerequisites

* Completed Datadog Operator v1.0.0+ Helm chart migration. For details, see the [Migration Guide][1].
* Running `cert-manager` with `installCRDs` set to `true`:
   ```shell
   helm install \
     cert-manager jetstack/cert-manager \
     --version v1.11.0 \
     --set installCRDs=true
   ```
* Running Datadog Operator v1.0.0+ with the Conversion Webhook Server enabled:
   ```shell
   helm install \
     datadog-operator datadog/datadog-operator \
     --set image.tag=1.0.0 \
     --set datadogCRDs.migration.datadogAgents.version=v2alpha1 \
     --set datadogCRDs.migration.datadogAgents.useCertManager=true \
     --set datadogCRDs.migration.datadogAgents.conversionWebhook.enabled=true
   ```

## Convert DatadogAgent/v1alpha1 to DatadogAgent/v2alpha1

The Datadog Operator runs a reconciler for v2alpha1 objects and also starts a Conversion Webhook Server, exposed on port 9443. The API Server uses this server to convert v1alpha1 DatadogAgent CRDs to v2alpha1. 

1. Forward a local port to the Conversion Webhook Server exposed on port 9443:

   ```shell
   kubectl port-forward <DATADOG_OPERATOR_POD_NAME> 2345:9443
   ```

2. Save a `v1alpha1` DatadogAgent definition as JSON. You can use a tool like `yq`.

3. Run a `curl` command targeting the `/convert` endpoint with your DatadogAgent.v1alpha1 JSON:

   ``` shell
   curl -k https://localhost:2345/convert -X POST -d '{"request":{"uid":"123", "desiredAPIVersion":"datadoghq.com/v2alpha1", "objects":[{
     "apiVersion": "datadoghq.com/v1alpha1",
     "kind": "DatadogAgent",
     "metadata": {
       "name": "datadog"
     },
     "spec": {
       "credentials": {
         "apiKey": "DATADOG_API_KEY",
         "appKey": "DATADOG_APP_KEY"
       }
     }
   }]}}'
   ```

   This returns a response with your converted `v2alpha1` DatadogAgent definition:

   ```yaml
   kind: DatadogAgent
   apiVersion: datadoghq.com/v2alpha1
   metadata:
     name: datadog
     creationTimestamp: null
   spec:
     features: {}
     global:
       credentials:
         apiKey: <DATADOG_API_KEY>
         appKey: <DATADOG_APP_KEY>
   status:
     conditions: null
   ```

[1]: https://github.com/DataDog/helm-charts/blob/main/charts/datadog-operator/README.md#migrating-to-the-version-10-of-the-datadog-operator