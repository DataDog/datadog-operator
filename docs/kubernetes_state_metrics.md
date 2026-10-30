# Monitoring your cluster with Kubernetes State Metrics

## Introduction

You can easily monitor your cluster using [Kubernetes State Metrics][1].

The Datadog Operator has a feature that allows users to configure the new Kubernetes State Metrics check (v2.0.0) as a Cluster Level Check.
See the Further Reading section for more details on the v2.0.0 of the check.
You will need to have the [Datadog Cluster Agent][2] as well as the Cluster Level Check features enabled.

## Configuration 

To enable this feature, you will need to use the option `kubeStateMetricsCore.enabled: true`, the DatadogAgent spec should look like this:

```yaml
spec:
  features:
      kubeStateMetricsCore:
        enabled: true
      clusterChecks:
        enabled: true
        useClusterChecksRunners: true
```

By default the following collectors will be activated:
  - pods
  - replicationcontrollers
  - statefulsets
  - nodes
  - cronjobs
  - jobs
  - replicasets
  - deployments
  - configmaps
  - services
  - endpoints
  - endpointslices
  - daemonsets
  - horizontalpodautoscalers
  - limitranges
  - resourcequotas
  - secrets
  - namespaces
  - persistentvolumeclaims
  - persistentvolumes

This will be done through a single Cluster Level Check.
You can also customize the configuration of this check with a ConfigMap.
If you want to maintain the ConfigMap yourself, you will need to use the field `features.kubeStateMetricsCore.conf.configMap: <name_of_your_CM>` as follows:

```yaml
spec:
  features:
    kubeStateMetricsCore:
      enabled: true
      conf:
        configMap:
          name: cluster-agent-ksm-conf
          items:
            - key: kubernetes_state_core.yaml
              path: kubernetes_state_core.yaml
```

For instance, in a large cluster where you might want to take advantage of the label join features and split the collectors so several Cluster Check Runners process them, your configuration could look like this:

```yaml
kind: ConfigMap
apiVersion: v1
metadata:
  name: custom-kubernetes-state-core-check
  namespace: datadog-operator-system
data:
  kubernetes_state_core.yaml: |
    cluster_check: true
    init_config:
    instances:
      - collectors:
          - pods
          - nodes
          - cronjobs
          - jobs
        label_joins:
          kube_job_labels:
              labels_to_match:
                - job_name
                - namespace
              labels_to_get:
                - label_chart_name
                - label_chart_version
                - label_team
        labels_mapper:
          label_chart_name: chart_name
          label_chart_version: chart_version
          label_team: team
      - collectors:
          - deployments
          - daemonsets
          - horizontalpodautoscalers
          - secrets
          - namespaces
        label_joins:
          kube_deployment_labels:
              labels_to_match:
                - deployment
                - namespace
              labels_to_get:
                - label_service
                - label_app
          kube_daemonset_labels:
              labels_to_match:
                - daemonset
                - namespace
              labels_to_get:
                - label_service
                - label_app
        labels_mapper:
          label_service: service
          label_app: app
``` 

The above example will create 2 separate Cluster Level Checks, using different collectors and features (label joins, remapping...).
Once you have created the ConfigMap (in the same namespace as the operator), make sure you reference the name in the DatadogAgent Spec, in this case:

You can also reference the configuration in the specification of the DatadogAgent spec as follows:

```yaml
spec:
  features:
    kubeStateMetricsCore:
      enabled: true
      conf: 
        configData: |
            cluster_check: true
            init_config:
            instances:
              - collectors:
                  - pods
                  - nodes
```

The above will have the operator create and maintain a ConfigMap for you with this config. It will run a single Kubernetes State Metrics Core check with the pods and nodes collectors enabled.

NB: You can't use `configData` and `configMap` simultaneously.

### Dynamic Resource Allocation (DRA) metrics

To collect metrics about Kubernetes [Dynamic Resource Allocation][dra] objects, add the `resourceclaims`, `resourceslices`, and `devicetaintrules` collectors to a custom check configuration (`conf.configData` or `conf.configMap`). A custom `collectors` list replaces the default one, so include the default collectors you want to keep:

```yaml
apiVersion: datadoghq.com/v2alpha1
kind: DatadogAgent
metadata:
  name: datadog
spec:
  features:
    kubeStateMetricsCore:
      enabled: true
      conf:
        configData: |-
          cluster_check: true
          init_config:
          instances:
            - skip_leader_election: true
              collectors:
              - pods
              - replicationcontrollers
              - statefulsets
              - nodes
              - cronjobs
              - jobs
              - replicasets
              - deployments
              - configmaps
              - services
              - endpoints
              - endpointslices
              - daemonsets
              - horizontalpodautoscalers
              - poddisruptionbudgets
              - limitranges
              - resourcequotas
              - secrets
              - namespaces
              - persistentvolumeclaims
              - persistentvolumes
              - ingresses
              - storageclasses
              - volumeattachments
              - apiservices
              - controllerrevisions
              - customresourcedefinitions
              - resourceclaims
              - resourceslices
              - devicetaintrules
```

This example runs the check on Cluster Checks Runners; set `cluster_check` and `skip_leader_election` to `false` when it runs on the Cluster Agent.

The DRA collectors require Cluster Agent 7.85.0 or later (or Cluster Checks Runner 7.85.0 or later when the check runs on Cluster Checks Runners). Older versions fail the whole `kubernetes_state_core` check when these collectors are listed on a cluster that serves the DRA API. When the component running the check is 7.85.0 or later, the operator automatically grants `list` and `watch` on `resourceclaims`, `resourceslices`, and `devicetaintrules` in the `resource.k8s.io` API group. Resource claims and resource slices require Kubernetes 1.34 or later, and device taint rules require Kubernetes 1.37 or later; the Agent skips collectors whose API the cluster does not serve.

For a complete setup that also enables GPU monitoring and pins the Agent and Cluster Agent to 7.85.0, see [datadog-agent-with-dra.yaml][dra-example].

[dra]: https://kubernetes.io/docs/concepts/scheduling-eviction/dynamic-resource-allocation/
[dra-example]: https://github.com/DataDog/datadog-operator/blob/main/examples/datadogagent/datadog-agent-with-dra.yaml

## Further Reading

The v2 of the Kubernetes State Metrics check is embedded as a "core check" in the Datadog Agent.
When an agent (Cluster Check Runner) receives the instruction to schedule this check, the configured Kube State Metrics collectors will be started as separate routines in the container.

This means that the agent does not need to monitor independent instances of [Kube State Metrics][1] anymore. 

When activating this feature, the Datadog Node Agent will be instructed not to Autodiscover independent Kubernetes State Metrics instances (via their image name) in order to avoid overlap.
However if you have configured the v1 of the check via annotations, make sure you remove them to avoid any potential confusion. 

[1]: https://github.com/kubernetes/kube-state-metrics
[2]: https://github.com/DataDog/datadog-operator/blob/main/docs/cluster_agent_setup.md
