# Datadog Operator

![badge](https://action-badges.now.sh/datadog/datadog-operator)
[![Go Report Card](https://goreportcard.com/badge/github.com/datadog/datadog-operator)](https://goreportcard.com/report/github.com/datadog/datadog-operator)
[![codecov](https://codecov.io/gh/datadog/datadog-operator/branch/master/graph/badge.svg)](https://codecov.io/gh/datadog/datadog-operator)

> **!!! Alpha version !!!**
>
> Please don't use it yet in production.

## Overview

The **Datadog Operator** aims to provide a new way of deploying the [Datadog Agent](https://github.com/DataDog/datadog-agent/) on Kubernetes.

Once deployed, the Datadog Operator provides:

* Agent configuration validation that limits configuration mistakes.
* Orchestration of creating/updating Datadog Agent resources.
* Reporting of Agent configuration status in its Kubernetes CRD resource.
* Optionally, use of on an advanced `DaemonSet` deployment by leveraging the [ExtendedDaemonSet](https://github.com/DataDog/extendeddaemonset).
* Many other features to come :).

## Datadog Operator vs. Helm chart

The official [Datadog Helm chart](https://github.com/helm/charts/tree/master/stable/datadog) is still the recommended way to setup Datadog in a Kubernetes cluster, as it has most supported configuration options readily accessible. It also makes using more advanced features easier than rolling your own deployments.
The **Datadog Operator** aims to improve the user experience around deploying Datadog. It does this by reporting deployment status, health, and errors in its Custom Resource status, and by limiting the risk of misconfiguration thanks to higher-level configuration options. However, the Datadog Operator is still in alpha, so it is not yet a recommended way of installing the Agent in production. Our end goal is to support both the Helm chart and the operator (as well as providing basic deployment files in the Agent [repository](https://github.com/DataDog/datadog-agent/tree/6.15.0/Dockerfiles/manifests) when CRDs and Helm are not an option) as official ways of installing Datadog.

## Prerequisites

Using the Datadog Operator requires the following prerequisites:

* **Kubernetes Cluster version >= v1.14.X**: Tests were done on versions >= `1.14.0`. Still, it should work on versions `>= v1.11.0`. For earlier versions, due to limited CRD support, the operator may not work as expected.
* [`Helm`](https://helm.sh) for deploying the `Datadog-operator`.
* [`Kubectl` cli](https://kubernetes.io/docs/tasks/tools/install-kubectl/) for installing the `Datadog-agent`.

> We plan to provide Openshift support with its [operators-framework](https://www.openshift.com/learn/topics/operators) ecosystem but it is not yet released (for information the `Datadog-Operator` is based on the [`operator-sdk`](https://github.com/operator-framework/operator-sdk)).

## Quick start

See the [Quick Start tutorial](docs/quick-start.md).

## Metrics and Events

The Datadog Operator sends metrics and events to Datadog to monitor the Datadog Agent components deployment in the cluster.

### Metrics

|Metric name   |Metric type   |Description   |
|---|---|---|
| `datadog.operator.agent.deployment.success`   | gauge   | `1` if the desired number of Agent replicas equals the number of available Agent pods, `0` otherwise.   |
| `datadog.operator.clusteragent.deployment.success`   | gauge   | `1` if the desired number of Cluster Agent replicas equals the number of available Cluster Agent pods, `0` otherwise.   |
| `datadog.operator.clustercheckrunner.deployment.success`   | gauge   | `1` if the desired number of Cluster Check Runner replicas equals the number of available Cluster Check Runner pods, `0` otherwise.   |
| `datadog.operator.reconcile.success`   | gauge   | `1` if the last recorded reconcile error is null, `0` otherwise. The `reconcile_err` tag describes the last recorded error. |

**Note:** The [Datadog API and APP keys](https://docs.datadoghq.com/account_management/api-app-keys/) are required to forward metrics to Datadog, they must be provided in the `credentials` field in the Custom Resource definiton.

The Datadog Operator exposes Golang and Controller metrics in OpenMetrics format. For now they can be collected using the [OpenMetrics integration](https://docs.datadoghq.com/integrations/openmetrics/). A Datadog integration will be available in the future.

The OpenMetrics check is activated by default via [autodiscovery annotations](./chart/datadog-operator/templates/deployment.yaml) and is scheduled by the Agent running on the same node as the Datadog Operator Pod.

### Events

- Detect/Delete Custom Resource <Namespace/Name>
- Create/Update/Delete Service <Namespace/Name>
- Create/Update/Delete ConfigMap <Namespace/Name>
- Create/Update/Delete DaemonSet <Namespace/Name>
- Create/Update/Delete ExtendedDaemonSet <Namespace/Name>
- Create/Update/Delete Deployment <Namespace/Name>
- Create/Update/Delete ClusterRole </Name>
- Create/Update/Delete Role <Namespace/Name>
- Create/Update/Delete ClusterRoleBinding </Name>
- Create/Update/Delete RoleBinding <Namespace/Name>
- Create/Update/Delete Secret <Namespace/Name>
- Create/Update/Delete PDB <Namespace/Name>
- Create/Delete ServiceAccount <Namespace/Name>

## How to contribute

See the [How to Contribute page](docs/how-to-contribute.md).
