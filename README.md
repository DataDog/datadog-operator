# Datadog Operator

![badge](https://action-badges.now.sh/datadog/datadog-operator)
[![Go Report Card](https://goreportcard.com/badge/github.com/datadog/datadog-operator)](https://goreportcard.com/report/github.com/datadog/datadog-operator)
[![codecov](https://codecov.io/gh/datadog/datadog-operator/branch/main/graph/badge.svg)](https://codecov.io/gh/datadog/datadog-operator)

> **This project is currently in Beta**

## Overview

The **Datadog Operator** aims to provide a new way of deploying the [Datadog Agent][1] on Kubernetes. Once deployed, the Datadog Operator provides:

- Agent configuration validation that limits configuration mistakes.
- Orchestration of creating/updating Datadog Agent resources.
- Reporting of Agent configuration status in its Kubernetes CRD resource.
- Optionally, use of an advanced `DaemonSet` deployment by leveraging the [ExtendedDaemonSet][2].
- Many other features to come :).

The **Datadog Operator** is [RedHat certified][10] and available on [operatorhub.io][11].

## Datadog Operator vs. Helm chart

The official [Datadog Helm chart][3] is still the recommended way to setup Datadog in a Kubernetes cluster, as it has most supported configuration options readily accessible. It also makes using more advanced features easier than rolling your own deployments.

The **Datadog Operator** aims to improve the user experience around deploying Datadog. It does this by reporting deployment status, health, and errors in its Custom Resource status, and by limiting the risk of misconfiguration thanks to higher-level configuration options.

However, the Datadog Operator is still in beta, so it is not yet a recommended way of installing the Agent in production. Datadog's end goal is to support both the Helm chart and the operator (as well as providing basic deployment files in the Agent [repository][4] when CRDs and Helm are not an option) as official ways of installing Datadog.

## Getting started

See the [Getting Started][5] dedicated documentation to learn how to deploy the Datadog operator and your first Agent, and [Configuration][12] to see examples and a list of all configuration keys.

## Functionalities

The Datadog operator also allows you to:

- [Configure and provide custom checks to the Agents][6].
- [Deploy the Datadog Cluster Agent with your node Agents][7].
- [Secrets Management with the Datadog Operator][8].

## How to contribute

See the [How to Contribute page][9].

[1]: https://github.com/DataDog/datadog-agent/
[2]: https://github.com/DataDog/extendeddaemonset
[3]: https://github.com/helm/charts/tree/main/stable/datadog
[4]: https://github.com/DataDog/datadog-agent/tree/6.15.0/Dockerfiles/manifests
[5]: https://github.com/DataDog/datadog-operator/blob/main/docs/getting_started.md
[6]: https://github.com/DataDog/datadog-operator/blob/main/docs/custom_check.md
[7]: https://github.com/DataDog/datadog-operator/blob/main/docs/cluster_agent_setup.md
[8]: https://github.com/DataDog/datadog-operator/blob/main/docs/secret_management.md
[9]: https://github.com/DataDog/datadog-operator/tree/main/docs/how-to-contribute.md
[10]: https://catalog.redhat.com/software/operators/detail/5e845a42ecb5246c09fe90b6
[11]: https://operatorhub.io/operator/datadog-operator
[12]: https://github.com/DataDog/datadog-operator/blob/main/docs/configuration.md

## Release

Release process documentation is available [here](hack/release/README.md).
