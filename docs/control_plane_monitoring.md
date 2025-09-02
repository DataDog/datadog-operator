# Kubernetes Control Plane Monitoring

The Datadog Operator can automatically configure monitoring for Kubernetes control plane components including the API Server, etcd, Controller Manager, and Scheduler.

This feature was introduced in Datadog Operator v1.18.0 for Openshift and Amazon EKS clusters and is currently in Preview. 

## What Gets Monitored

- [Kubernetes API Server][1]
- [Kubernetes Controller Manager][2]
- [Kubernetes Scheduler][3]
- [etcd][4] (OpenShift only)

## Supported Platforms

| Platform | Operator Version | Notes |
|----------|:----------------:|-------|
| Amazon EKS | v1.18.0+ | |
| Red Hat OpenShift 4 | v1.18.0+ | `etcd` not supported on versions 4.0-4.13, requires Agent v7.69+ |

## Prerequisites

- Datadog Operator v1.18.0+
- For OpenShift: Datadog Agent v7.69+

## General Setup

Control plane monitoring is enabled by default, but requires [introspection](introspection.md) to be enabled.

You can enable introspection using the [datadog-operator Helm chart](https://github.com/DataDog/helm-charts/tree/main/charts/datadog-operator):

```yaml
# values.yaml
introspection:
  enabled: true
```

Or via command line:
```bash
helm install datadog-operator datadog/datadog-operator --set introspection.enabled=true
```

Since this feature is enabled by default, you can deploy a minimal DatadogAgent spec. 

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