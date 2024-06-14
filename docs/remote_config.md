# Remote Configuration (beta)

This feature was introduced in Datadog Operator v1.7.0 and is currently in beta.

## Overview

Remote Configuration in the Datadog Operator allows users to enable features in a Kubernetes cluster from the Datadog web app.

From the [Fleet Automation page](link-to-be-added), a policy can be created to enable a feature on an eligible scope. The Datadog Operator will then update the agents with the necessary configuration.


## Prerequisites

* Datadog Operator v1.7.0+

## Enabling Remote Configuration

Remote Configuration is disabled by default. To enable it using the [datadog-operator helm chart](https://github.com/DataDog/helm-charts/tree/main/charts/datadog-operator), in your `values.yaml` file make the following changes:

1. Set `remoteConfiguration.enabled=true`.
2. Set a cluster name `clusterName`.
3. Set Datadog credentials, using `apiKey/apiKeyExistingSecret` and `appKey/appKeyExistingSecret`. (If using a secret, create the secret with `kubectl create secret generic datadog-secret --from-literal api-key=<DATADOG_API_KEY> --from-literal app-key=<DATADOG_APP_KEY>`.)

Then install the Datadog Operator:

```shell
helm install my-datadog-operator datadog/datadog-operator -f values.yaml
```
