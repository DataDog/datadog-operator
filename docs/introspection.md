# Introspection (beta)

> [!CAUTION]
> Starting with Datadog Operator v1.27.0, `DatadogAgentInternal` (DDAI) is required and the legacy introspection implementation described on this page is no longer used. Enabling introspection is therefore a no-op: it does not create provider-specific DaemonSets or apply provider-specific configuration. If you used introspection with DDAI disabled, follow the [migration instructions](#migration-to-operator-v1270-and-later) to upgrade.

This beta feature was introduced in Operator v1.4.0. The behavior below applies only through Operator v1.26.x when DDAI is disabled. DDAI has been enabled by default since Operator v1.22.0 and cannot be disabled starting with Operator v1.27.0. See the [DDAI version timeline](datadog_agent_internal.md#version-timeline) for details.

## Overview

In its legacy implementation, introspection allows the Operator to detect a node's environment and automatically make configuration changes based on it. Each environment is referred to as a `provider`. Examples include GKE Container-Optimized OS (GKE COS), Azure Kubernetes Service (AKS), and Red Hat OpenShift. Depending on the node's provider, the Datadog Agent on that node may require certain configurations to run properly. Introspection creates a Datadog Agent deployment for each provider, including provider-specific configuration and node affinity.

Any node that does not have an associated provider will have a `default` provider applied to them. The `default` provider does not contain any special configuration.

Example:

In a mixed GKE cluster with Ubuntu and COS nodes, the legacy implementation creates two DaemonSets: one for the Ubuntu nodes and one for the COS nodes. A suffix is added to each DaemonSet name to identify the provider used to create the Agent configuration. In this example, `datadog-agent-gke-cos` applies to the GKE COS nodes and `datadog-agent-default` applies to the Ubuntu nodes.

```console
$ kubectl get ds
NAME                    DESIRED   CURRENT   READY   UP-TO-DATE   AVAILABLE   NODE SELECTOR   AGE
datadog-agent-default   2         2         2       2            2           <none>          3m21s
datadog-agent-gke-cos   2         2         2       2            2           <none>          3m21s
```

## Applicability

- Operator v1.4.0 through v1.26.x
- The legacy direct reconciliation path is in use: DDAI is unavailable before v1.16.0 or disabled from v1.16.0 through v1.26.x

This behavior was tested on Kubernetes v1.27.0 and later.

## Enabling Introspection

The following instructions apply only to the legacy implementation described above. Introspection is disabled by default. To enable introspection using the [Datadog Operator Helm chart](https://github.com/DataDog/helm-charts/tree/main/charts/datadog-operator), set `introspection.enabled=true` in your `values.yaml` file or pass `--set introspection.enabled=true` on the command line.

For OLM deployments where container arguments cannot be set, enable introspection using an environment variable in the `Subscription`:

```yaml
config:
  env:
    - name: DD_INTROSPECTION_ENABLED
      value: "true"
```

## Migration to Operator v1.27.0 and Later

> [!CAUTION]
> If introspection is enabled and DDAI is disabled, do not upgrade directly without following one of the migration methods below. Operator v1.27.0 removes the DDAI opt-out and no longer runs the legacy direct reconciliation path that implements introspection.

DDAI has been enabled by default since Operator v1.22.0, so this migration applies only if you explicitly disabled it while having introspection enabled.

### Preferred: Disable Introspection Before Upgrading

If you are still running Operator v1.26.x or earlier, disable introspection before upgrading. This allows the existing Operator to remove the provider-specific DaemonSets for you.

1. If you use GKE COS with OOM Kill or TCP Queue Length monitoring, first apply the [GKE COS override](#gke-cos-with-oom-kill-or-tcp-queue-length-monitoring).
2. Keep DDAI disabled and disable introspection in the Operator deployment. When using the Helm chart, set `introspection.enabled=false`.
3. Wait for the Operator to reconcile the `DatadogAgent`. Confirm that the replacement Agent DaemonSet has been created and the legacy `-default` and/or `-gke-cos` DaemonSets have been removed.
4. Upgrade to Operator v1.27.0 or later. No manual DaemonSet cleanup is required.

### Alternative: Clean Up After Upgrading

Use this method if you have already upgraded to Operator v1.27.0 or later. After upgrading, the Operator creates the DDAI-managed Agent DaemonSet, but it does not remove the legacy `-default` or `-gke-cos` DaemonSets. Leaving those DaemonSets in place will prevent new Agent pods from being scheduled due to the stale pods remaining. If you use GKE COS with OOM Kill or TCP Queue Length monitoring, apply the [GKE COS override](#gke-cos-with-oom-kill-or-tcp-queue-length-monitoring) before waiting for the new DaemonSet to become ready.

After the new DDAI-managed Agent DaemonSet is created:

1. List the legacy DaemonSets. Use the provider label and the `-default` or `-gke-cos` name suffix to identify them:

   ```bash
   kubectl get ds -l 'agent.datadoghq.com/provider!='
   ```

   The inequality selector can also match DaemonSets that do not have the provider label. Confirm the `-default` or `-gke-cos` suffix before deleting anything.

2. Delete each confirmed legacy DaemonSet manually:

   ```bash
   kubectl delete ds <legacy-daemonset-name>
   ```

### GKE COS with OOM Kill or TCP Queue Length Monitoring

This workaround applies only to GKE COS nodes where `features.oomKill` or `features.tcpQueueLength` is enabled and the Agent cannot be scheduled. These features normally create a `hostPath` volume for `/usr/src`, but `/usr/src` is unavailable on the read-only COS host filesystem. Legacy introspection avoided that `hostPath` on COS.

To reproduce that behavior, add the following override to the `DatadogAgent` spec. It replaces the `/usr/src` host volume with an `emptyDir` volume for the `system-probe` container:

```yaml
override:
  nodeAgent:
    volumes:
      - name: src
        emptyDir: {}
    containers:
      system-probe:
        volumeMounts:
          - name: src
            mountPath: /usr/src
```

## Migration from Operator Version < 1.4.0

### Operator v1.4.0 <= x < v1.6.0

1. Upgrade to Operator v1.4.0+ **without** enabling introspection. The Operator should label the existing node Agent DaemonSet or ExtendedDaemonSet with the label `agent.datadoghq.com/provider=""`.
2. Enable introspection in the Operator following the instructions above. The Operator should delete the unused node Agent DaemonSet or ExtendedDaemonSet.

### Operator v1.6.0+

1. Upgrade the Operator image and enable introspection in the same step.

## Supported Providers

| Provider | Operator Version |
| -------- | :--------------: |
| GKE Container-Optimized OS | v1.4.0 |
