# Datadog Operator Kubernetes permissions

This document explains how and why the Operator requires RBAC permissions for Kubernetes clusters. Kubernetes security requires balancing least privilege with the practical needs of critical components such as the Datadog Agent, which is managed by the Datadog Operator.

## Quick Summary

- The Datadog Operator requires permissions to manage both its own resources and the Datadog Agent components it deploys
- Minimal permissions are needed for most controllers (leases and custom resources)
- The `DatadogAgent` controller requires extensive permissions to support various customer scenarios
- In practice, only a subset of permissions are used based on your specific configuration

## Generation of Datadog Operator role

The [Datadog Operator ClusterRole](../config/rbac/role.yaml) is automatically generated using [kubebuilder markers](https://book.kubebuilder.io/reference/markers/rbac) listed in `_controller` files: [datadogagent_controller.go](../internal/controller/datadogagent_controller.go), for example. For a given controller, a controller can only grant a RBAC permission if the Operator itself already has that permission. This means that in order for the `DatadogAgent` controller to create the Agent DaemonSet, the Operator itself needs to be granted this permission.

## Minimal set of permissions needed by the Datadog Operator

The Datadog Operator requires a minimal set of permissions to run without errors:
* Manage `leases` in the API group `coordination.k8s.io`: this is needed to run leader election. Only a single Operator replica takes actions when running multiple replicas for availability.
* Manage (`create`, `get`, `list`, `watch`, `update`, `delete`, `patch`) Datadog custom resources (`datadoghq.com` API group) depending on which controller is enabled. For example, if the `DatadogDashboard` controller is enabled, it needs these permissions to manage the lifecycle of `DatadogDashboard` resources, such as when a `DatadogDashboard` is created, the Operator needs to know and reconcile accordingly.
    * The exact set of resources is listed in `_controller` files within [controller](../internal/controller).
    * If the custom resource manages native Kubernetes resources (for example, a `DaemonSet` is created by the `DatadogAgent` controller), managing it is also needed.

This set of permissions (lease and Datadog-resources lifecycle) is sufficient for the following controllers:
* `DatadogDashboard`
* `DatadogGenericResource`
* `DatadogMonitor`
* `DatadogSLO`

## Additional permissions required by the `DatadogAgent` controller

This section covers more details on the permissions required by the `DatadogAgent` controller, as this controller is responsible for the vast majority of permissions in the Operator role. To sum it up, the Operator has a wide range of permissions to support a multitude of customer scenarios. However, for a given scenario, the Operator only uses a limited set of permissions to perform the required operations.

### Direct and indirect permissions

We first need to distinguish between the direct permissions needed by the Operator itself and those indirectly needed by the Datadog Agent to run in Kubernetes clusters. As mentioned previously, the Operator manages the lifecycle of Datadog custom resources. In the case of `DatadogAgent`, this means managing the lifecycle of native Kubernetes resources, such as DaemonSet required by the Datadog Agent. More details on the Agent architecture on Kubernetes can be found [in this blog post](https://www.datadoghq.com/architecture/efficient-kubernetes-monitoring-with-the-datadog-cluster-agent/). This is an example of a permission the Operator needs directly, but which the Agent itself does not need. On the other hand, if we take the resource `nodes` where the Operator role includes `get,list,patch,watch` permissions, it is not directly needed by the Operator, but it is needed by the Agent DaemonSet to access kubelet endpoints on each node to provide observability.

#### Examples of direct vs indirect permissions

| Permission Type | Resource/Verbs (not exhaustive) | Needed By | Purpose |
|-----------------|----------------|-----------|---------|
| Direct | `daemonsets` (create, update, delete) | Operator | Managing Agent deployment lifecycle |
| Direct | `serviceaccounts` (create, update) | Operator | Creating Agent ServiceAccounts with appropriate permissions |
| Indirect | `nodes` (get, list, watch) | Agent DaemonSet | Accessing kubelet endpoints for metrics collection |
| Direct & Indirect | `secrets` (create, get, list) | Operator & Agent components | Storing user-provided credentials in Secrets & Secrets management feature (when enabled) |

### Balancing user experience and principle of least privilege

The `DatadogAgent` Custom Resource Definition (CRD) offers a lot of options to users to smoothen the onboarding experience of the Datadog Agent. For instance, the option `global.secretBackend.enableGlobalPermissions` (defaulting to `false`) allows the Agent components (DaemonSet, Deployment) read-only access to Kubernetes `secrets` in all namespaces, in order to easily use the Secrets Management Agent [feature](https://docs.datadoghq.com/agent/configuration/secrets-management/?tab=jsonfilebackend#example-reading-a-kubernetes-secret-across-namespaces): avoid storing credentials such as a database password in plain text within configuration files. Therefore, the Operator role has a broad range of permissions to permit all the use-cases in the `DatadogAgent` CRD.

### ClusterRole vs Role usage

The Operator uses a `ClusterRole` instead of a `Role` for its permissions as it is needed to do cross-namespace (meaning cluster-wide) operations. For instance, the Operator can be installed in a namespace `operators` and configured to watch over a different namespace `datadog-agent`. Moreover, some Agent features require creating `ClusterRole` and `ClusterRoleBinding` such as [Kubernetes State Core](https://docs.datadoghq.com/integrations/kubernetes_state_core/?tab=operator). Finally, because managed resources are named dynamically (e.g., `<datadogagent-name>-cluster-agent`, `<datadogagent-name>-agent`), the Operator ClusterRole cannot use specific resource names and must use broader permissions.

## Common deployment scenarios and their permissions

Understanding which permissions are used in different configurations can help clarify why the Operator requires its broad ClusterRole.

| Scenario | Required permissions (not exhaustive) | Notes |
|---|---|---|
| Basic Agent deployment (default configuration) | `daemonsets`, `deployments`; `configmaps`, `secrets`; `services`; `serviceaccounts`, `roles`, `rolebindings`; `nodes`, `pods` | Create/manage Agent components; store Agent configuration (namespace-scoped by default); expose Agent endpoints; create ServiceAccounts with appropriate permissions; gather cluster metrics and metadata |
| APM with Admission Controller enabled | All Basic, plus: `mutatingwebhookconfigurations`; `certificatesigningrequests` | Create a mutating webhook to inject Datadog config into pods; generate certificates for the webhook server |
| Kubernetes State Metrics Core | All Basic, plus: `clusterroles`, `clusterrolebindings`; extended permissions on various resources | Create cluster-wide permissions for metrics collection; broader read access for comprehensive metrics gathering |

## Security considerations

While the Operator ClusterRole includes extensive permissions, several security measures are in place:

- **Separation of Concerns**: The Operator creates separate ServiceAccounts and (Cluster)Roles for Agent components, each with only the permissions they need. The Operator's broad permissions are used solely to create these more restricted roles, not for direct data access.
- **Feature-Gated Permissions**: Many extensive permissions (like cross-namespace secret access) are only activated when specific features are explicitly enabled in your `DatadogAgent` configuration.
- **Audit Trail**: All actions taken by the Operator are logged and can be audited through Kubernetes audit logs, providing visibility into permission usage.
- **Read-Only by Default**: Most indirect permissions granted to Agent components are read-only (get, list, watch), with write permissions limited to specific use cases.
- **No Privilege Escalation**: The Operator does not perform privilege escalation; it only grants permissions it already possesses.

## Troubleshooting permission issues

If you encounter permission-related errors:

1. **Check Operator logs** for error messages indicating missing permissions:
   ```bash
   kubectl logs -n <operator-namespace> deployment/datadog-operator
   ```

2. **Verify ServiceAccount bindings** are correctly configured:
   ```bash
   kubectl get clusterrolebinding | grep datadog-operator
   ```

3. **Confirm feature flags** match your intended configuration—some features require additional permissions that may not be granted by default.

4. **Review DatadogAgent status** to see reconciliation state:
   ```bash
   kubectl get datadogagent -o yaml
   ```

When reviewing Operator logs, look for messages with the string "is attempting to grant RBAC permissions not currently held":
```
clusterroles.rbac.authorization.k8s.io "datadog-agent-cluster-agent-autoscaling" is forbidden: user "system:serviceaccount:datadog-agent:datadog-operator" (groups=["system:serviceaccounts" "system:serviceaccounts:datadog-agent" "system:authenticated"]) is attempting to grant RBAC permissions not currently held:
{APIGroups:["argoproj.io"], Resources:["rollouts"], Verbs:["patch"]}
```

The key information is in the permissions section:
```
{APIGroups:["argoproj.io"], Resources:["rollouts"], Verbs:["patch"]}
```

This indicates the specific permission that needs to be added to the Operator ClusterRole.

## Going further

⚠️ **Advanced users Only - Not recommended**

Manually managing RBAC permissions significantly increases operational complexity and the risk of deployment failures. This approach requires deep Kubernetes expertise and ongoing maintenance as new Operator/Agent features are released.

When installing the Helm chart of the Datadog Operator, you can use the [option](https://github.com/DataDog/helm-charts/blob/be378fa68489230924c2f4118eed2a20da4c8937/charts/datadog-operator/values.yaml#L111) `rbac.create` set to `false` in your `values.yaml` file (or using the `--set rbac.create=false` CLI argument) to disable the creation of the Operator `ClusterRole`. In that case, you must manually provide the necessary RBACs depending on the features you are enabling. These are explained in the [Minimal set of permissions needed by the Datadog Operator](#minimal-set-of-permissions-needed-by-the-datadog-operator). For the `DatadogAgent` controller, this will involve trial and error to identify the exact set of permissions the Operator needs to be able to grant them accordingly to the Agent components. They can be identified by reviewing the Operator logs as described in the troubleshooting section above.
