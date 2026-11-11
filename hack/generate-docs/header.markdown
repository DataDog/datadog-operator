# Configuration

## Configuration inputs

The Agent configuration the Operator generates is determined by two inputs: the
`DatadogAgent` `spec` (the parameters documented on this page) and a small set of
metadata annotations on the `DatadogAgent`.

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

### Process passwd mounts

For eligible containers using Agent version 7.86.0 or later (including 7.86.0+ prereleases such as release candidates), live-process and process-discovery mount the host's `/etc/passwd` read-only at `/host/etc/passwd`, preserving image-local accounts in `/etc/passwd`. Like other version-gated features, the version comes from the node Agent image override (`spec.override.nodeAgent.image`), or the Operator's default Agent version when there is no override. Older versions, tags without a full version (such as `7`, `latest`, or `7.86`), custom tags, and digest-only images retain `/etc/passwd`. Experimental per-container image annotations and container renames are not considered, so they can make the mount disagree with the running image; the effect is limited to process usernames. The relocated mount still uses the `passwd` volume. Separately, the core agent and process-agent always mount host `/etc/os-release` read-only at `/host/etc/os-release` (volume `os-release-file`), like the Helm chart, because the Agent sets `HOST_ETC=/host/etc` whenever that directory exists and then reads the host OS from there. Existing Talos exclusions and security mounts are unchanged.

On GKE Autopilot, process collection keeps `/etc/passwd` for now: the new destination needs a published WorkloadAllowlist that permits it.

The Operator does not override `HOST_ETC` or `ignore_host_etc`. If you customize these settings, ensure `HOST_ETC=/host/etc` to resolve users through the new mount.

**Known limitation:** the [process resolver change](https://github.com/DataDog/datadog-agent/pull/55789) does not cover Gohai. Moving the core Agent mount can cause Gohai process metadata to report numeric UIDs or image-local usernames instead of host usernames.

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
