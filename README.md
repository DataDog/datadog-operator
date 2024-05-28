# Datadog Operator

![badge](https://github.com/DataDog/datadog-operator/actions/workflows/main.yml/badge.svg)
[![Go Report Card](https://goreportcard.com/badge/github.com/datadog/datadog-operator)](https://goreportcard.com/report/github.com/datadog/datadog-operator)
[![codecov](https://codecov.io/gh/datadog/datadog-operator/branch/main/graph/badge.svg)](https://codecov.io/gh/datadog/datadog-operator)

## Overview

> [!WARNING]
> Operator v1.7.0 removes support for DatadogAgent `v1alpha1` reconciliation (`v2APIEnabled` flag). v1.8.0 will remove the conversion webhook as well, and users will not be able to apply the DatadogAgent `v1alpha1` manifest.
> 
> Operator v1.8.0 will deprecate custom resource definitions using `apiextensions.k8s.io/v1beta1`. They will be kept in the repository but will not be updated. They will be removed in v1.10.0.  

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

### Migrating from `0.8.x` to `1.0.0`

Operator `1.0.0` contains several changes users need to be aware of:
* `DatadogAgent` CRD has two versions, `v1alpha1` and `v2alpha1`. They are used as a stored version by Operator `0.8.x` and `1.0.0` respectively. See this [Kubernetes documentation page][13] for more details about CRD versioning.
* `v1alpha1` and `v2alpha1` are not backward or forward compatible. The Datadog Operator `1.0.0` implements a Conversion Webhook to migrate, though it only supports migrating from `v1alpha1` to `v2alpha1`.
* With the Conversion Webhook enabled, users can run `1.0.0` but continue applying a `v1alpha1` manifest. However, they won't be able to retrieve the `DatadogAgent` manifest as a `v1alpha1` object (see the previous item).
*  The Conversion Webhook requires a cert manager. See the migration guide in the [public][14] or [helm chart][15] documentation for more details.
* `0.8.x` managed `PodDisruptionBudget` for Cluster Agent and Cluster Checks Worker deployments. `1.0.0` doesn't, however this is on our roadmap.

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

## Release

Release process documentation is available [here](./RELEASING.md).
