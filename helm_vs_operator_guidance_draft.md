# Deploy the Datadog Agent: Helm or the Datadog Operator

<!-- DRAFT for CONTP-1575. Dialect-agnostic (plain code fences + markdown tables).
     Adapt to the target's shortcodes ({% ... %} for docs site, {{< highlight >}} for the
     Operator repo single-sourced pages) when the publish location is fixed. Inline TODOs marked. -->

There are two supported ways to run the Datadog Agent on Kubernetes: the [`datadog` Helm chart][1]
and the [Datadog Operator][2]. Both deploy the same Agent components—the Node Agent, Cluster Agent,
and Cluster Checks Runner. They differ in *how* that configuration is delivered and managed.

Datadog recommends the Operator as the default path for new deployments. It reaches feature parity
with the Helm chart on the major cloud providers and adds capabilities the chart does not offer.
A few specific cases still call for Helm; those are listed in [Known gaps and limitations](#known-gaps-and-limitations),
along with how to work around them.

## Two ways to deploy the Agent

Both methods are declarative, but they operate at different layers:

- **Helm (`datadog` chart)** renders the Agent's Kubernetes objects from a `values.yaml` file at
  install and upgrade time. Helm's job is packaging and templated deployment.
- **The Datadog Operator** runs a controller in the cluster that reconciles a single `DatadogAgent`
  custom resource toward its desired state continuously—not only at install time. You install the
  Operator once (with Helm or through [Operator Lifecycle Manager][3]), then manage the Agent by
  editing one custom resource.

In short: Helm deploys the Agent; the Operator deploys *and continuously manages* it. Because the
`DatadogAgent` resource is a single source of truth reconciled by the controller, you get native
API-server validation of your configuration and live status through `kubectl get datadogagent`.

The following examples deploy an equivalent Agent—API key from an existing secret, with log
collection and APM enabled—each way.

Helm (`values.yaml`):

```yaml
datadog:
  apiKeyExistingSecret: datadog-secret
  logs:
    enabled: true
    containerCollectAll: true
  apm:
    portEnabled: true
```

Operator (`DatadogAgent`):

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
  features:
    logCollection:
      enabled: true
      containerCollectAll: true
    apm:
      enabled: true
      hostPortConfig:
        enabled: true
```

For the full `DatadogAgent` configuration reference, see the [configuration spec][4]. For the mapping
between Helm values and `DatadogAgent` fields, see [Migrating from Helm](#migrating-from-helm-to-the-operator).

## When to choose which

Use the **Datadog Operator** when:

- You are starting a new deployment. It is the recommended default.
- You run on a supported platform (see [Platform and provider support](#platform-and-provider-support)).
- You want per-node-group Agent configurations, automatic environment detection, or distribution
  through a platform catalog (see [What the Operator adds](#what-the-operator-adds-beyond-parity)).

Use the **`datadog` Helm chart** when:

- You run on a platform the Operator does not support yet, such as Talos or Flatcar.
- You run GKE on Google Distributed Cloud (GDC), which is Helm-only.
- You depend on a feature that is not yet exposed by the Operator and cannot be reproduced with an
  override (see [Known gaps and limitations](#known-gaps-and-limitations)).

These cases are narrow. For everything else, the Operator is the recommended choice.

## What the Operator adds beyond parity

Beyond matching the Helm chart on the major providers, the Operator offers capabilities the chart
does not, or offers only at additional cost and effort.

### Multiple Agent configurations from one resource

With [DatadogAgentProfiles][5], you apply different Agent configurations to different node groups
from a single `DatadogAgent`—for example, larger resource limits on GPU nodes, or a higher log level
on a specific node role. Achieving the same with Helm requires multiple `datadog` chart releases per
cluster, each with hand-written affinity rules and duplicated Cluster Agent settings.

### Remote management with Fleet Automation

The Operator can be managed remotely through Fleet Automation (private preview), letting you apply
and update Agent configuration from Datadog without editing manifests in each cluster. The Helm chart
has no equivalent.

### Automatic environment detection and adjustment

The Operator detects the cluster [provider][6] and applies the matching configuration automatically.
Today this includes control plane monitoring on Amazon EKS and Red Hat OpenShift, and AKS-specific
Agent settings, with more environments planned. With Helm, you set the corresponding `providers.*`
flags yourself.

### Native platform distribution and lifecycle

The Operator ships as a first-class artifact in the native catalogs of the major managed-Kubernetes
ecosystems, which the `datadog` Helm chart does not:

- **Red Hat OperatorHub and certified Marketplace**, installed and upgraded through
  [Operator Lifecycle Manager][3]. See [Install on OpenShift][7].
- **Amazon EKS add-on.** https://aws.amazon.com/marketplace/pp/prodview-wedp6r37fkufe
- **Google Cloud Marketplace.** https://console.cloud.google.com/marketplace/product/datadog-saas/datadog 

In these channels you get console-native discovery, catalog-managed upgrades, certified bundles, and
unified procurement. This is a distribution and lifecycle benefit rather than a monitoring capability,
and it is most relevant if your organization standardizes on one of these catalogs.

## Platform and provider support

The Operator supports the same major platforms as the Helm chart. On EKS, AKS, and OpenShift the
provider is detected automatically; on GKE it is declared. For how detection and the
`agent.datadoghq.com/cluster-provider` annotation work, see the [providers documentation][6].

| Platform | `datadog` Helm chart | Datadog Operator | Notes |
| -------- | -------------------- | ---------------- | ----- |
| Amazon EKS | Supported | Supported (auto-detected) | Enables [control plane monitoring][8]. Includes the EC2 file-based hostname variant. |
| Azure AKS | Supported | Supported (auto-detected) | Applies the required AKS admission controller selectors and AKS-specific Agent defaults. |
| Red Hat OpenShift | Supported | Supported (auto-detected) | Enables [control plane monitoring][8], including etcd. |
| GKE (Container-Optimized OS) | Supported | Supported (declared) | Declared with the `gke-cos` provider. |
| GKE Autopilot | Supported | Supported (declared) | Declared with the `gke-autopilot` provider. See [Datadog Operator on GKE Autopilot][9]. |
| Windows nodes | Supported | Supported | No dedicated Operator guide yet; see the example manifests. https://github.com/DataDog/datadog-operator/pull/3209 |

## Known gaps and limitations

The Operator does not yet cover every Helm option. The gaps below are the ones to be aware of; each
has a workaround or a clear recommendation.

### Feature gaps with workarounds

**Container Autodiscovery filtering.** The Helm chart exposes dedicated values such as
`datadog.containerExclude` and `datadog.containerInclude` to control which containers Autodiscovery
monitors. The Operator does not expose these as fields yet. Set the equivalent environment variables
on the Node Agent instead:

```yaml
apiVersion: datadoghq.com/v2alpha1
kind: DatadogAgent
metadata:
  name: datadog
spec:
  override:
    nodeAgent:
      env:
        - name: DD_CONTAINER_EXCLUDE
          value: "image:gcr.io/datadoghq/agent.*"
        - name: DD_CONTAINER_INCLUDE
          value: "name:my-app"
```

The same pattern applies to the log- and metric-specific variants (`DD_CONTAINER_EXCLUDE_LOGS`,
`DD_CONTAINER_INCLUDE_METRICS`, and so on), `DD_EXCLUDE_PAUSE_CONTAINER`, and `DD_IGNORE_AUTOCONF`.

**Kubernetes State Metrics Core advanced options.** The Helm chart exposes fine-grained options for
the KSM Core check—such as `labelsAsTags`, `annotationsAsTags`, static `tags`, and toggles for
collecting Secret, ConfigMap, and VPA metrics. The Operator exposes only a subset. Tag- and
check-level options can be supplied through a custom check configuration on the
`kubeStateMetricsCore` feature. Options that also change collection scope and RBAC—collecting Secret
or ConfigMap metrics, or restricting collection to specific namespaces—cannot be fully reproduced
through overrides; use the Helm chart if you require them.

### Not yet supported

The following platforms are supported by the Helm chart but not by the Operator. Use the `datadog`
Helm chart to deploy the Agent on them:

- **Talos**
- **Flatcar Container Linux**

### Helm-only

- **GKE on Google Distributed Cloud (GDC)** requires deployment-specific handling that the Operator
  does not implement. Use the [`datadog` Helm chart][1] for GDC.

## Migrating from Helm to the Operator

Existing Helm users can move to the Operator without changing what the Agent collects. The migration
is a translation from `values.yaml` to a `DatadogAgent` resource: global settings map to `spec.global`,
feature toggles to `spec.features`, and per-component customizations to `spec.override`. The Helm
`providers.*` flags map to the `agent.datadoghq.com/cluster-provider` annotation—see the
[providers documentation][6].

Before switching, check the [Known gaps and limitations](#known-gaps-and-limitations) for any Helm
values your deployment relies on, and apply the listed workarounds where needed. For the field-level
mapping and step-by-step instructions, see the [migration guide][10].

<!-- Reference-style links; renumber/convert to absolute https://docs.datadoghq.com/... when placing. -->
[1]: https://github.com/DataDog/helm-charts/tree/main/charts/datadog
[2]: https://github.com/DataDog/datadog-operator
[3]: https://olm.operatorframework.io/
[4]: https://github.com/DataDog/datadog-operator/blob/main/docs/configuration.v2alpha1.md
[5]: https://github.com/DataDog/datadog-operator/blob/main/docs/datadog_agent_profiles.md
[6]: https://github.com/DataDog/datadog-operator/blob/main/docs/providers.md
[7]: https://github.com/DataDog/datadog-operator/blob/main/docs/install-openshift.md
[8]: https://github.com/DataDog/datadog-operator/blob/main/docs/control_plane_monitoring.md
[9]: https://github.com/DataDog/datadog-operator/blob/main/docs/gke_autopilot/external.md
[10]: https://github.com/DataDog/datadog-operator/blob/main/docs/v2alpha1_migration.md
