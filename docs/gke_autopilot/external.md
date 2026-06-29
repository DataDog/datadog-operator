# Datadog Operator on GKE Autopilot

This guide explains how to deploy the Datadog Operator and a `DatadogAgent` on a GKE Autopilot cluster.

GKE Autopilot applies stricter workload restrictions than standard GKE clusters. The Operator's Autopilot mode adjusts the generated Agent workload so it can run within those restrictions.

## Prerequisites

- A GKE Autopilot cluster.
- `kubectl` configured for the cluster.
- `helm` for installing the Operator.
- A Datadog API key stored in a Kubernetes Secret.

See [Installing the Datadog Operator](../installation.md) for the general installation flow.

## Install the Operator

Add the Datadog Helm repository and install the Operator:

```shell
helm repo add datadog https://helm.datadoghq.com
helm install datadog-operator datadog/datadog-operator
```

*Note: GKE Autopilot is supported with the Operator version `1.27.0+`.*

## Create the Datadog credentials Secret

Create a Secret in the namespace where you plan to create the `DatadogAgent`:

```shell
kubectl create secret generic datadog-secret \
  --from-literal api-key=<DATADOG_API_KEY>
```

Add `app-key` if you enable features that require a Datadog application key.

## Enable Autopilot mode

Enable Autopilot mode by adding the experimental annotation to the `DatadogAgent` metadata:

```yaml
apiVersion: datadoghq.com/v2alpha1
kind: DatadogAgent
metadata:
  name: datadog
  annotations:
    experimental.agent.datadoghq.com/autopilot: "true"
spec:
  global:
    credentials:
      apiSecret:
        secretName: datadog-secret
        keyName: api-key
```

Apply the manifest:

```shell
kubectl apply -f datadog-agent.yaml
```

The annotation is case-insensitive for the value `true`, but use the quoted string `"true"` in manifests.

## Optional: select the WorkloadAllowlist version

When Autopilot mode is enabled, the Operator applies a GKE `AllowlistSynchronizer` named `datadog-synchronizer` that points GKE to the Datadog WorkloadAllowlist.

By default, the Operator uses the built-in default allowlist version. To pin another allowlist version, set:

```yaml
metadata:
  annotations:
    experimental.agent.datadoghq.com/autopilot: "true"
    experimental.agent.datadoghq.com/autopilot-allowlist-version: "v1.0.5"
```

The value must use the `vX.Y.Z` format. Malformed values are ignored and the default version is used.

## Customize resources

Use the `spec.override.nodeAgent.containers` override to set container resources. For example, the following configuration sets resources on the core Agent container:

```yaml
apiVersion: datadoghq.com/v2alpha1
kind: DatadogAgent
metadata:
  name: datadog
  annotations:
    experimental.agent.datadoghq.com/autopilot: "true"
spec:
  global:
    credentials:
      apiSecret:
        secretName: datadog-secret
        keyName: api-key
  override:
    nodeAgent:
      containers:
        agent:
          resources:
            requests:
              cpu: "200m"
              memory: "256Mi"
            limits:
              cpu: "500m"
              memory: "512Mi"
```

Add entries for other enabled Node Agent containers as needed. For example, if APM is enabled, you can also set resources for `trace-agent`:

```yaml
spec:
  override:
    nodeAgent:
      containers:
        trace-agent:
          resources:
            requests:
              cpu: "100m"
              memory: "128Mi"
            limits:
              cpu: "300m"
              memory: "256Mi"
```

Common Node Agent container names include:

| Container | Purpose |
|-----------|---------|
| `agent` | Core Agent container |
| `trace-agent` | APM trace intake |
| `process-agent` | Process and live process collection |
| `system-probe` | System Probe, NPM, and related features |
| `init-config` | Agent configuration init container |
| `init-volume` | Agent volume init container |
| `seccomp-setup` | Seccomp setup init container |

Only override containers that are enabled by your feature configuration.

## Set a PriorityClass

Create a `PriorityClass` if you do not want to use a built-in Kubernetes priority class:

```yaml
apiVersion: scheduling.k8s.io/v1
kind: PriorityClass
metadata:
  name: datadog-agent-priority
value: 1000000
globalDefault: false
description: "Priority class for Datadog Agent pods."
```

Then reference it from the Node Agent override:

```yaml
apiVersion: datadoghq.com/v2alpha1
kind: DatadogAgent
metadata:
  name: datadog
  annotations:
    experimental.agent.datadoghq.com/autopilot: "true"
spec:
  global:
    credentials:
      apiSecret:
        secretName: datadog-secret
        keyName: api-key
  override:
    nodeAgent:
      priorityClassName: datadog-agent-priority
```

You can also set `priorityClassName` on `clusterAgent`, `clusterChecksRunner`, or `otelAgentGateway` if those components are enabled.

## Enable APM or DogStatsD

On Autopilot, the Operator uses host ports for APM and DogStatsD instead of Unix domain sockets because host socket paths are not available in the same way as on standard clusters.

Example:

```yaml
apiVersion: datadoghq.com/v2alpha1
kind: DatadogAgent
metadata:
  name: datadog
  annotations:
    experimental.agent.datadoghq.com/autopilot: "true"
spec:
  global:
    credentials:
      apiSecret:
        secretName: datadog-secret
        keyName: api-key
  features:
    apm:
      enabled: true
    dogstatsd:
      originDetectionEnabled: true
```

The Operator keeps the configured host port values if you set them in the feature configuration.

## Operational notes

- Do not use overrides to re-add host paths, auth-token mounts, CRI socket mounts, DogStatsD socket mounts, or APM socket mounts that Autopilot mode removes.
- Autopilot mode rewrites the `trace-agent` and `process-agent` commands; avoid overriding those commands unless the Autopilot implementation is updated to support the change.
- If log collection uses a hostPath-backed run path, Autopilot mode rewrites it to `/var/autopilot/addon/datadog/logs`, the path allowed by the Datadog WorkloadAllowlist.
- For the full override schema, see [configuration.v2alpha1.md](../configuration.v2alpha1.md).
