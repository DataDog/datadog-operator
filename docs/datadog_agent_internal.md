# DatadogAgentInternal

`DatadogAgentInternal` (DDAI) is an internal Custom Resource introduced in Datadog Operator v1.16.0.

It is automatically created and managed by the Operator.
You should continue to create and manage **DatadogAgent** resources as usual.

---

## Overview

Starting in v1.16.0, the Operator separates configuration from workload creation. This became the default in v1.22.

Instead of creating Kubernetes workloads directly from a `DatadogAgent`, the Operator now creates one or more `DatadogAgentInternal` resources, which in turn manage the underlying DaemonSets, Deployments, and other Agent components (RBAC, ConfigMaps, etc.).

**Before (< v1.16.0)**

```
DatadogAgent → DaemonSets / Deployments
```

**After (>= v1.16.0)**

```
DatadogAgent → DatadogAgentInternal(s) → DaemonSets / Deployments
```

This separation enables:

* Better separation of responsibilities inside the Operator
* Cleaner support for [DatadogAgentProfiles](datadog_agent_profiles.md)

---

## What You Need to Know

* You do not need to create `DatadogAgentInternal` resources. They are automatically created and owned by the corresponding `DatadogAgent`.
* Manual modification is not supported and may lead to undefined behavior.
* Deleting a `DatadogAgent` automatically deletes its `DatadogAgentInternal` resources.

---

## Behavior Without Profiles

When not using `DatadogAgentProfiles`, a `DatadogAgent` creates a single `DatadogAgentInternal` with the same name and namespace.

### Example

```console
$ kubectl get datadogagent -n datadog
NAME      AGE
datadog   5m

$ kubectl get ddai -n datadog
NAME      AGENT     CLUSTER-AGENT   CLUSTER-CHECKS-RUNNER   AGE
datadog   Running   Running         Running                 5m
```

The `DatadogAgentInternal`:

* Mirrors the `DatadogAgent` configuration
* Includes defaults resolved by the Operator
* Manages the actual Kubernetes workloads

---

## Behavior With Profiles

When using [DatadogAgentProfiles](datadog_agent_profiles.md), the Operator creates:

* One **default** `DatadogAgentInternal`
* One additional `DatadogAgentInternal` per profile

### Example

```console
$ kubectl get datadogagent -n datadog
NAME      AGE
datadog   10m

$ kubectl get datadogagentprofile -n datadog
NAME                AGE
high-memory-nodes   2m

$ kubectl get ddai -n datadog
NAME                AGENT     CLUSTER-AGENT   CLUSTER-CHECKS-RUNNER   AGE
datadog             Running   Running         Running                 10m
high-memory-nodes   Running                                           2m
```

* `datadog` → default profile (nodes not matched by any profile)
* `high-memory-nodes` → profile-specific configuration

Profile-specific DDAIs run only the node Agent.
Cluster Agent and Cluster Checks Runner run in the default DDAI.

For details, see [DatadogAgentProfiles](datadog_agent_profiles.md).

---

## Version Timeline

| Operator Version  | Status                                |
| ----------------- | ------------------------------------- |
| < v1.16.0         | DDAI not available                    |
| v1.16.0 – v1.21.x | Opt-in                                |
| v1.22.0 – v1.26.x | Enabled by default (opt-out possible) |
| >= v1.27.0        | Required, no opt-out                  |

As of v1.27.0, the legacy direct reconciliation path has been removed.

---

## Inspecting DatadogAgentInternal

You can view DDAIs using standard `kubectl` commands:

```bash
# List
kubectl get datadogagentinternal
kubectl get ddai

# Describe
kubectl describe ddai <name>

# View YAML
kubectl get ddai <name> -o yaml
```

---

## See Also

* [DatadogAgentProfiles](datadog_agent_profiles.md)
* [Configuration Reference](configuration.v2alpha1.md)
* [Datadog Operator Helm Chart](https://github.com/DataDog/helm-charts/tree/main/charts/datadog-operator)
