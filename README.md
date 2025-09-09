# Datadog Operator

![badge](https://github.com/DataDog/datadog-operator/actions/workflows/main.yml/badge.svg)
[![Go Report Card](https://goreportcard.com/badge/github.com/datadog/datadog-operator)](https://goreportcard.com/report/github.com/datadog/datadog-operator)
[![codecov](https://codecov.io/gh/datadog/datadog-operator/branch/main/graph/badge.svg)](https://codecov.io/gh/datadog/datadog-operator)

## Overview

> [!WARNING]
> Upcoming changes to Agent DaemonSet labels and selectors may affect your setup.
> 
> - In **Operator v1.18.0**, the `app.kubernetes.io/instance` label value was changed from `<dda-name>-agent` to `<dap-name>-agent` on Pods and DaemonSets managed by [DatadogAgentProfile][17] (beta).
> 
> - In **Operator v1.21.0**, the following changes will occur:
>   - DAP-managed DaemonSets will be renamed from `datadog-agent-with-profile-<dda-name>-<dap-name>` to `<dap-name>-agent`.
>   - All DaemonSets (default and DAP) will replace the `matchLabels` selector `agent.datadoghq.com/name: <dda-name>` with `agent.datadoghq.com/instance: <dda-name>-agent` or `<dap-name>-agent`.
> 
> ⚠️ If you rely on these labels or `matchLabels` (e.g., in NetworkPolicies, admission controllers, or automation), you may need to update those resources.
> 
> For a safe, zero-downtime migration path and full details, see the [migration guide][18].

The **Datadog Operator** aims to provide a new way of deploying the [Datadog Agent][1] on Kubernetes. Once deployed, the Datadog Operator provides:

- Agent configuration validation that limits configuration mistakes.
- Orchestration of creating/updating Datadog Agent resources.
- Reporting of Agent configuration status in its Kubernetes CRD resource.
- Optionally, use of an advanced `DaemonSet` deployment by leveraging the [ExtendedDaemonSet][2].
- Many other features to come :).

The **Datadog Operator** is [RedHat certified][10] and available on [operatorhub.io][11].

## Datadog Operator vs. Helm chart

You can also use official [Datadog Helm chart][3] or a DaemonSet to install the Datadog Agent on Kubernetes. However, using the Datadog Operator offers the following advantages:

* The Operator has built-in defaults based on Datadog best practices.
* Operator configuration is more flexible for future enhancements.
* As a [Kubernetes Operator][16], the Datadog Operator is treated as a first-class resource by the Kubernetes API.
* Unlike the Helm chart, the Operator is included in the Kubernetes reconciliation loop.

Datadog fully supports using a DaemonSet to deploy the Agent, but manual DaemonSet configuration leaves significant room for error. Therefore, using a DaemonSet is not highly recommended.

## Getting started

See the [Getting Started][5] dedicated documentation to learn how to deploy the Datadog operator and your first Agent, and [Configuration][12] to see examples, a list of all configuration keys, and default values.

### Migrating from `v1alpha1` to `v2alpha1`

Datadog Operator `v1.8.0+` does not support migrating from `DatadogAgent` CRD `v1alpha1` to `v2alpha1` or from Operator `v0.8.x` to `v1.x.x`.

Use the conversion webhook in `v1.7.0` to migrate, and then upgrade to a recent version.

### Default Enabled Features

- Cluster Agent
- Admission Controller
- Cluster Checks
- Kubernetes Event Collection
- Kubernetes State Core Check
- Live Container Collection
- Orchestrator Explorer
- UnixDomainSocket transport for DogStatsD (and APM if enabled)
- Process Discovery

## Functionalities

The Datadog operator also allows you to:

- [Configure and provide custom checks to the Agents][6].
- [Deploy the Datadog Cluster Agent with your node Agents][7].
- [Secrets Management with the Datadog Operator][8].

## How to contribute

See the [How to Contribute page][9].

[1]: https://github.com/DataDog/datadog-agent/
[2]: https://github.com/DataDog/extendeddaemonset
[3]: https://github.com/DataDog/helm-charts/tree/main/charts/datadog
[4]: https://github.com/DataDog/datadog-agent/tree/6.15.0/Dockerfiles/manifests
[5]: https://github.com/DataDog/datadog-operator/blob/main/docs/getting_started.md
[6]: https://github.com/DataDog/datadog-operator/blob/main/docs/custom_check.md
[7]: https://github.com/DataDog/datadog-operator/blob/main/docs/cluster_agent_setup.md
[8]: https://github.com/DataDog/datadog-operator/blob/main/docs/secret_management.md
[9]: https://github.com/DataDog/datadog-operator/tree/main/docs/how-to-contribute.md
[10]: https://catalog.redhat.com/software/operators/detail/5e9874986c5dcb34dfbb1a12
[11]: https://operatorhub.io/operator/datadog-operator
[12]: https://github.com/DataDog/datadog-operator/blob/main/docs/configuration.v2alpha1.md
[13]: https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definition-versioning/
[14]: https://docs.datadoghq.com/containers/guide/datadogoperator_migration/
[15]: https://github.com/DataDog/helm-charts/tree/main/charts/datadog-operator#migration
[16]: https://kubernetes.io/docs/concepts/extend-kubernetes/operator/
[17]: https://github.com/DataDog/datadog-operator/blob/main/docs/datadog_agent_profiles.md
[18]: https://github.com/DataDog/datadog-operator/blob/main/docs/agent_metadata_changes.md

## Release

Release process documentation is available [here](./RELEASING.md).
