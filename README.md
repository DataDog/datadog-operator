# Datadog Operator

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

## How to contribute

See the [How to Contribute page](docs/how-to-contribute.md).
