# Kubernetes Control Plane Monitoring

The Datadog Operator can automatically configure monitoring for Kubernetes control plane components including the API Server, etcd, Controller Manager, and Scheduler.

This feature supports Red Hat OpenShift and Amazon EKS clusters and is currently in Preview. Since Datadog Operator v1.29.0 it is applied automatically based on the detected [provider](providers.md).

## What Gets Monitored

- [Kubernetes API Server][1]
- [Kubernetes Controller Manager][2]
- [Kubernetes Scheduler][3]
- [etcd][4] (OpenShift only)

## Supported Platforms

| Platform | Provider | Operator Version | Notes |
|----------|----------|:----------------:|-------|
| Amazon EKS | `eks` | v1.29.0+ | |
| Red Hat OpenShift 4 | `openshift` | v1.29.0+ | `etcd` not supported on versions 4.0-4.13, requires Agent v7.69+ |

## Prerequisites

- Datadog Operator v1.29.0+ (for automatic provider detection)
- For OpenShift: Datadog Agent v7.69+

## General Setup

Control plane monitoring is applied automatically for clusters whose
[provider](providers.md) resolves to `eks` or `openshift`. Starting with Operator
v1.29.0, the provider is [auto-detected](providers.md#automatic-detection) by
default, so a minimal `DatadogAgent` spec is sufficient and no additional
configuration is required.

If the provider is not detected — for example, the Operator's node does not carry
the expected labels — set it explicitly with the
`agent.datadoghq.com/cluster-provider` annotation on the `DatadogAgent`:

```yaml
apiVersion: datadoghq.com/v2alpha1
kind: DatadogAgent
metadata:
  name: datadog
  annotations:
    agent.datadoghq.com/cluster-provider: eks   # or "openshift"
```

See the [providers documentation](providers.md) for how the provider is resolved.

### OpenShift-specific Setup
Enable `features.ClusterChecks.clusterCheckRunners: true` to schedule checks there; otherwise, control plane checks will run on the Node Agent. 

For OpenShift 4.14 and higher, etcd monitoring requires copying certificates. Check the operator logs for the exact command. See the following example (adjust namespace as needed):

```bash
oc get secret etcd-metric-client -n openshift-etcd-operator -o yaml | \
  sed 's/namespace: openshift-etcd-operator/namespace: datadog/' | \
  oc apply -f -
```

## Validation

Check that checks are running:
```bash
kubectl exec -it <cluster-agent-pod> -- agent clusterchecks
```

Look for:
- `kube_apiserver_metrics`
- `kube_controller_manager` 
- `kube_scheduler`
- `etcd` (OpenShift only)

You should see control plane metrics in Datadog like:
- `kube_apiserver.*`
- `kube_controller_manager.*`
- `kube_scheduler.*`
- `etcd.*` (OpenShift only)

[1]: https://docs.datadoghq.com/integrations/kube_apiserver_metrics/
[2]: https://docs.datadoghq.com/integrations/kube_controller_manager/
[3]: https://docs.datadoghq.com/integrations/kube_scheduler/
[4]: https://docs.datadoghq.com/integrations/etcd/