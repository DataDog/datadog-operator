# Monitoring your cluster with Kubernetes State Metrics

## Introduction

You can easily monitor your cluster using [Kubernetes State Metrics][1].

The Datadog Operator has a feature that allows users to configure the new Kubernetes State Metrics check (v2.0.0) as a Cluster Level Check.
See the Further Reading section for more details on the v2.0.0 of the check.
You will need to have the [Datadog Cluster Agent][2] as well as the Cluster Level Check features enabled.

## Configuration 

To enable this feature, you will need to use the option `kubeStateMetricsCore.enabled: true`, the DatadogAgent spec should look like this:

```yaml
features:
    kubeStateMetricsCore:
      enabled: true
clusterAgent:
    image:
      name: "datadog/cluster-agent:1.10.0"
    config:
[...]
      clusterChecksEnabled: true
clusterChecksRunner:
    image:
      name: "datadog/agent:7.24.0"
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
features:
    kubeStateMetricsCore:
      enabled: true
      conf:
        configMap:
          name: custom-kubernetes-state-core-check
          fileKey: kubernetes_state_core.yaml
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
        telemetry: true
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
        telemetry: true
``` 

The above example will create 2 separate Cluster Level Checks, using different collectors and features (label joins, telemetry, remapping...).
Once you have created the ConfigMap (in the same namespace as the operator), make sure you reference the name in the DatadogAgent Spec, in this case:

You can also reference the configuration in the specification of the DatadogAgent spec as follows:

```yaml
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
            telemetry: true
```

The above will have the operator create and maintain a ConfigMap for you with this config. It will run a single Kubernetes State Metrics Core check with the pods and nodes collectors enabled.

NB: You can't use `configData` and `configMap` simultaneously.

## Further Reading

The v2 of the Kubernetes State Metrics check is embedded as a "core check" in the Datadog Agent.
When an agent (Cluster Check Runner) receives the instruction to schedule this check, the configured Kube State Metrics collectors will be started as separate routines in the container.

This means that the agent does not need to monitor independent instances of [Kube State Metrics][1] anymore. 

When activating this feature, the Datadog Node Agent will be instructed not to Autodiscover independent Kubernetes State Metrics instances (via their image name) in order to avoid overlap.
However if you have configured the v1 of the check via annotations, make sure you remove them to avoid any potential confusion. 

[1]: https://github.com/kubernetes/kube-state-metrics
[2]: https://github.com/DataDog/datadog-operator/blob/master/docs/cluster_agent_setup.md
