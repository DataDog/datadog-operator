# Remote Configuration (beta)

This feature was introduced in Datadog Operator v1.7.0 and is currently in beta.

## Overview

Remote Configuration in the Datadog Operator allows you to enable features in a Kubernetes cluster from Datadog.

You can create a policy on the [Fleet Automation page](link-to-be-added) page to enable a feature for an eligible scope. The Datadog Operator then updates the Agents with the necessary configuration.

## Prerequisites

* Datadog Operator v1.7.0+

## Enabling Remote Configuration

Remote Configuration is disabled by default. To enable it using the [datadog-operator Helm chart](https://github.com/DataDog/helm-charts/tree/main/charts/datadog-operator), in your `values.yaml` file make the following changes:

1. Set `remoteConfiguration.enabled=true`.
2. Set a cluster name `clusterName`.
3. Set Datadog credentials, using `apiKey/apiKeyExistingSecret` and `appKey/appKeyExistingSecret`. (If using a secret, create the secret with `kubectl create secret generic datadog-secret --from-literal api-key=<DATADOG_API_KEY> --from-literal app-key=<DATADOG_APP_KEY>`.)

Then install the Datadog Operator:

```shell
helm install my-datadog-operator datadog/datadog-operator -f values.yaml
```
