# Introspection (beta)

This feature was released in operator v1.4.0 and is currently in beta.

## Overview

Introspection allows the operator to detect a node's environment and automatically make configuration changes based on it. Each environment is referred to as a `provider`. Examples include GKE Container-Optimized OS (GKE COS), Azure Kubernetes Service (AKS), and Red Hat OpenShift. Depending on the node's provider, the Datadog Agent on that node may require certain configurations be set to run properly. Introspection creates a Datadog Agent deployment for each provider, which includes provider-specific configurations needed to run the Agent and a provider-specific node affinity.

Any node that does not have an associated provider will have a `default` provider applied to them. The `default` provider does not contain any special configuration.

Example:

In a mixed GKE cluster with Ubuntu and COS nodes, the operator will create 2 DaemonSets: one for the Ubuntu nodes and one for the COS nodes. A suffix is added to all DaemonSet names to identify which provider was used to create the Agent configuration. In this example, `datadog-agent-gke-cos` will apply to the GKE COS nodes and `datadog-agent-default` will apply to the nodes that are not GKE COS, i.e. the Ubuntu nodes.

```console
$ kubectl get ds
NAME                    DESIRED   CURRENT   READY   UP-TO-DATE   AVAILABLE   NODE SELECTOR   AGE
datadog-agent-default   2         2         2       2            2           <none>          3m21s
datadog-agent-gke-cos   2         2         2       2            2           <none>          3m21s
```

## Prerequisites

* Operator v1.4.0+

## Enabling Introspection

Introspection is disabled by default. To enable introspection using the [datadog-operator helm chart](https://github.com/DataDog/helm-charts/tree/main/charts/datadog-operator), set `introspection.enabled=true` in your `values.yaml` file or as a flag in the command line arguments `--set introspection.enabled=true`.

**Note:** Enabling introspection in an existing operator installation will create new DaemonSets or ExtendedDaemonSets without deleting the existing DaemonSet or ExtendedDaemonSet. The existing DaemonSet or ExtendedDaemonSet needs to be removed manually. This behavior will change in a future version of the operator to make the migration process smoother.

## Supported Providers

| Provider | Operator Version |
| -------- | :--------------: |
| GKE Container-Optimized OS | v1.4.0 |
