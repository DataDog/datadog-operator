# Remote Configuration (beta)

This feature was introduced in Datadog Operator v1.7.0 and is currently in beta.

## Overview

Remote Configuration in the Datadog Operator allows you to enable features in a Kubernetes cluster from Datadog.

You can create a policy on the Fleet Automation page to enable a feature for an eligible scope. The Datadog Operator then updates the Agents with the necessary configuration.

## Prerequisites

* Datadog Operator v1.7.0+

## Enabling Remote Configuration

Remote Configuration is disabled by default. It authenticates with an API key only; the API key must have Remote Configuration enabled. An application key is not required.

To enable it using the latest [datadog-operator Helm chart](https://github.com/DataDog/helm-charts/tree/main/charts/datadog-operator), create a Kubernetes Secret that contains your API key:

```shell
export DD_API_KEY=<YOUR_API_KEY>

kubectl create secret generic datadog-operator-secret --from-literal api-key=$DD_API_KEY
```

Then modify your `values.yaml` file with the following:

```yaml
clusterName: example-cluster-name
remoteConfiguration:
  enabled: true
apiKeyExistingSecret: datadog-operator-secret
```

**Note**: Earlier Operator versions also require an application key (`appKey` or `appKeyExistingSecret`) for Remote Configuration to start. Controllers that manage Datadog resources through the Datadog API (`DatadogMonitor`, `DatadogDashboard`, `DatadogSLO`, `DatadogGenericResource`) still require an application key.

Then install the Datadog Operator:

```shell
helm install my-datadog-operator datadog/datadog-operator -f values.yaml
```
