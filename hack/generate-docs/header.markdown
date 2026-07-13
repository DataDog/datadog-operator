# Configuration

## Configuration inputs

The Agent configuration the Operator generates is determined by two inputs:

1. The `DatadogAgent` `spec`—the parameters documented on this page.
2. A small set of metadata annotations on the `DatadogAgent`.

### Provider

A *provider* identifies an environment or platform that needs a specific set of
customizations to the Agent configuration. The Operator detects the cluster
provider automatically, or you can declare it with the
`agent.datadoghq.com/cluster-provider` annotation (mirroring the Helm chart's
`providers.*` configuration):

```yaml
metadata:
  annotations:
    agent.datadoghq.com/cluster-provider: eks
```

For what a provider is, how it is resolved, the full list of values, and their
Helm mappings, see the
[providers documentation](https://github.com/DataDog/datadog-operator/blob/main/docs/providers.md).

### Experimental annotations

Some features are gated by *experimental* annotations under the
`experimental.agent.datadoghq.com/` prefix rather than by the `spec` or a
provider—for example, `experimental.agent.datadoghq.com/autopilot: "true"`
enables GKE Autopilot handling. Experimental annotations are unstable and may
change or be removed; prefer the stable equivalent where one exists (for GKE
Autopilot, the `agent.datadoghq.com/cluster-provider: gke-autopilot` provider).
They are documented with the features they control.

## Manifest Templates

* [Manifest with Logs, APM, process, and metrics collection enabled.][1]
* [Manifest with Logs, APM, and metrics collection enabled.][2]
* [Manifest with APM and metrics collection enabled.][3]
* [Manifest with Cluster Agent.][4]
* [Manifest with tolerations.][5]

## All configuration options

The following table lists the configurable parameters for the `DatadogAgent`
resource. For example, if you wanted to set a custom cluster name, your
`DatadogAgent` resource would look like the following:
