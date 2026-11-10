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

## Log level and flares from Fleet Automation

When Remote Configuration is enabled, the Datadog Operator also receives these Fleet Automation requests. They don't need any other setting.

* **Log level**: a log level set from Fleet Automation (for example the temporary `debug` level set before a flare) changes the Operator log level at runtime. The Operator restores the level it started with (`-loglevel`) when the setting is removed.
* **Flare requests**: the Operator collects a flare of its own view of the cluster and sends it to the support case. Only the leader replica handles a request, and each request runs once. Fleet Automation shows the request as done once the flare was sent, or as failed with the error.

### What an Operator flare contains

The Operator collects the flare in-process, with its own Kubernetes client:

* **Operator**: version and build information, command line flags and the `DD_*`, `POD_*`, `WATCH_NAMESPACE` and proxy environment variables, enabled controllers and features, leader election and Remote Configuration client state, the logs of the Operator pods in its own namespace (current and previous container), a snapshot of its metrics (from the configured `-metrics-addr`, or read in-process when the metrics endpoint is secure), and its health and readiness probes.
* **Datadog resources**: every `DatadogAgent`, `DatadogAgentInternal` and `DatadogAgentProfile` with its spec and status, a status summary of `DatadogMonitor`, `DatadogSLO`, `DatadogDashboard` and `DatadogGenericResource` resources, and the Kubernetes events of these resources.
* **Managed workloads**: the node Agent DaemonSets (or ExtendedDaemonSets), and the Cluster Agent and Cluster Checks Runner Deployments: manifests, rollout status, their pods (phase, readiness, restarts) and events. The logs of these pods are not collected: the Agent and Cluster Agent flares collect them.
* **Configuration**: the ConfigMaps, RBAC and webhook configurations the Operator manages, the RBAC of the Operator service account, the installed Datadog CRD versions, and the Operator Deployment with its Helm chart metadata. Helm keeps the release values in a Secret, so they are not collected.
* **Cluster**: Kubernetes version and distribution, node count, OS and architecture mix, cluster name and ID.

A `manifest.json` file at the root of the archive lists what was collected and which collectors failed.

### Privacy and limits

* Secrets are never read. API and App keys, tokens, passwords, certificates and credentials in URLs are scrubbed from every file before it's added to the archive, with the Datadog Agent scrubber. Values of sensitive keys (such as `apiKey`, `clusterAgentToken` or `caBundle`) and of sensitive environment variables are replaced.
* A flare is capped at about 50 MB of files and 2 minutes of collection. Each container log is tailed to its last 10,000 lines and 10 MB. When a collector fails, or a limit is reached, the Operator still sends the partial flare and the manifest says what's missing.
* The flare is sent with the Operator API key to the flare intake of the configured site (`DD_SITE`, or `DD_DD_URL`/`DD_URL`), with the hostname `datadog-operator.<cluster ID>`, where the cluster ID is the UID of the `kube-system` namespace.

### Permissions

The Operator reads what the flare contains with its existing permissions, plus `get` on `pods/log` in its own namespace (the `flare-role` Role) and `get`/`list` on `extendeddaemonsets`. If you manage the Operator RBAC yourself, grant these permissions so that flares include the Operator logs.
