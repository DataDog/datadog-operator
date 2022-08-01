# Configuration

## Manifest Templates

* [Manifest with Logs, APM, process, and metrics collection enabled.][1]
* [Manifest with Logs, APM, and metrics collection enabled.][2]
* [Manifest with Logs and metrics collection enabled.][3]
* [Manifest with APM and metrics collection enabled.][4]
* [Manifest with Cluster Agent.][5]
* [Manifest with tolerations.][6]

## All configuration options

The following table lists the configurable parameters for the `DatadogAgent`
resource. For example, if you wanted to set a value for `agent.image.name`,
your `DatadogAgent` resource would look like the following:

```yaml
apiVersion: datadoghq.com/v1alpha1
kind: DatadogAgent
metadata:
  name: datadog
spec:
  credentials:
    apiSecret:
      secretName: datadog-secret
      keyName: api-key
    appSecret:
      secretName: datadog-secret
      keyName: app-key
```

| Parameter | Description |
| --------- | ----------- |
| features.admissionController.agentCommunicationMode | agentCommunicationMode corresponds to the mode used by the Datadog application libraries to communicate with the Agent. It can be "hostip", "service", or "socket". |
| features.admissionController.enabled | Enabled enables the Admission Controller. Default: false |
| features.admissionController.mutateUnlabelled | MutateUnlabelled enables config injection without the need of pod label 'admission.datadoghq.com/enabled="true"'. Default: false |
| features.admissionController.serviceName | ServiceName corresponds to the webhook service name. |
| features.apm.enabled | Enabled enables Application Performance Monitoring. Default: false |
| features.apm.hostPortConfig.enabled | Enabled enables host port configuration Default: false |
| features.apm.hostPortConfig.hostPort | Port takes a port number (0 < x < 65536) to expose on the host. (Most containers do not need this.) If HostNetwork is enabled, this value must match the ContainerPort. |
| features.apm.unixDomainSocketConfig.enabled | Enabled enables Unix Domain Socket. Default: true |
| features.apm.unixDomainSocketConfig.path | Path defines the socket path used when enabled. |
| features.clusterChecks.enabled | Enables Cluster Checks scheduling in the Cluster Agent. Default: true |
| features.clusterChecks.useClusterChecksRunners | Enabled enables Cluster Checks Runners to run all Cluster Checks. Default: false |
| features.cspm.checkInterval | CheckInterval defines the check interval. |
| features.cspm.customBenchmarks.items | Items maps a ConfigMap data key to a file path mount. |
| features.cspm.customBenchmarks.name | Name is the name of the ConfigMap. |
| features.cspm.enabled | Enabled enables Cloud Security Posture Management. Default: false |
| features.cws.customPolicies.items | Items maps a ConfigMap data key to a file path mount. |
| features.cws.customPolicies.name | Name is the name of the ConfigMap. |
| features.cws.enabled | Enabled enables Cloud Workload Security. Default: false |
| features.cws.syscallMonitorEnabled | SyscallMonitorEnabled enables Syscall Monitoring (recommended for troubleshooting only). Default: false |
| features.datadogMonitor.enabled | Enabled enables Datadog Monitors. Default: false |
| features.dogstatsd.hostPortConfig.enabled | Enabled enables host port configuration Default: false |
| features.dogstatsd.hostPortConfig.hostPort | Port takes a port number (0 < x < 65536) to expose on the host. (Most containers do not need this.) If HostNetwork is enabled, this value must match the ContainerPort. |
| features.dogstatsd.mapperProfiles.configData | ConfigData corresponds to the configuration file content. |
| features.dogstatsd.mapperProfiles.configMap.items | Items maps a ConfigMap data key to a file path mount. |
| features.dogstatsd.mapperProfiles.configMap.name | Name is the name of the ConfigMap. |
| features.dogstatsd.originDetectionEnabled | OriginDetectionEnabled enables origin detection for container tagging. See also: https://docs.datadoghq.com/developers/dogstatsd/unix_socket/#using-origin-detection-for-container-tagging |
| features.dogstatsd.unixDomainSocketConfig.enabled | Enabled enables Unix Domain Socket. Default: true |
| features.dogstatsd.unixDomainSocketConfig.path | Path defines the socket path used when enabled. |
| features.eventCollection.collectKubernetesEvents | CollectKubernetesEvents enables Kubernetes event collection. Default: true |
| features.externalMetricsServer.enabled | Enabled enables the External Metrics Server. Default: false |
| features.externalMetricsServer.endpoint.credentials.apiKey | APIKey configures your Datadog API key. See also: https://app.datadoghq.com/account/settings#agent/kubernetes |
| features.externalMetricsServer.endpoint.credentials.apiSecret.keyName | KeyName is the key of the secret to use. |
| features.externalMetricsServer.endpoint.credentials.apiSecret.secretName | SecretName is the name of the secret. |
| features.externalMetricsServer.endpoint.credentials.appKey | AppKey configures your Datadog application key. If you are using clusterAgent.metricsProvider.enabled = true, you must set a Datadog application key for read access to your metrics. |
| features.externalMetricsServer.endpoint.credentials.appSecret.keyName | KeyName is the key of the secret to use. |
| features.externalMetricsServer.endpoint.credentials.appSecret.secretName | SecretName is the name of the secret. |
| features.externalMetricsServer.endpoint.url | URL defines the endpoint URL. |
| features.externalMetricsServer.port | Port specifies the metricsProvider External Metrics Server service port. Default: 8443 |
| features.externalMetricsServer.useDatadogMetrics | UseDatadogMetrics enables usage of the DatadogMetrics CRD (allowing one to scale on arbitrary Datadog metric queries). Default: true |
| features.externalMetricsServer.wpaController | WPAController enables the informer and controller of the Watermark Pod Autoscaler. NOTE: The Watermark Pod Autoscaler controller needs to be installed. See also: https://github.com/DataDog/watermarkpodautoscaler. Default: false |
| features.kubeStateMetricsCore.conf.configData | ConfigData corresponds to the configuration file content. |
| features.kubeStateMetricsCore.conf.configMap.items | Items maps a ConfigMap data key to a file path mount. |
| features.kubeStateMetricsCore.conf.configMap.name | Name is the name of the ConfigMap. |
| features.kubeStateMetricsCore.enabled | Enabled enables Kube State Metrics Core. Default: true |
| features.liveContainerCollection.enabled | Enables container collection for the Live Container View. Default: true |
| features.liveProcessCollection.enabled | Enabled enables Process monitoring. Default: false |
| features.liveProcessCollection.scrubProcessArguments | ScrubProcessArguments enables scrubbing of sensitive data (passwords, tokens, etc. ). Default: true |
| features.logCollection.containerCollectAll | ContainerCollectAll enables Log collection from all containers. Default: false |
| features.logCollection.containerCollectUsingFiles | ContainerCollectUsingFiles enables log collection from files in `/var/log/pods instead` of using the container runtime API. Collecting logs from files is usually the most efficient way of collecting logs. See also: https://docs.datadoghq.com/agent/basic_agent_usage/kubernetes/#log-collection-setup Default: true |
| features.logCollection.containerLogsPath | ContainerLogsPath allows log collection from the container log path. Set to a different path if you are not using the Docker runtime. See also: https://docs.datadoghq.com/agent/kubernetes/daemonset_setup/?tab=k8sfile#create-manifest Default: `/var/lib/docker/containers` |
| features.logCollection.containerSymlinksPath | ContainerSymlinksPath allows log collection to use symbolic links in this directory to validate container ID -> pod. Default: `/var/log/containers` |
| features.logCollection.enabled | Enabled enables Log collection. Default: false |
| features.logCollection.openFilesLimit | OpenFilesLimit sets the maximum number of log files that the Datadog Agent tails. Increasing this limit can increase resource consumption of the Agent. See also: https://docs.datadoghq.com/agent/basic_agent_usage/kubernetes/#log-collection-setup Default: 100 |
| features.logCollection.podLogsPath | PodLogsPath allows log collection from a pod log path. Default: `/var/log/pods` |
| features.logCollection.tempStoragePath | TempStoragePath (always mounted from the host) is used by the Agent to store information about processed log files. If the Agent is restarted, it starts tailing the log files immediately. Default: `/var/lib/datadog-agent/logs` |
| features.npm.collectDNSStats | CollectDNSStats enables DNS stat collection. Default: false |
| features.npm.enableConntrack | EnableConntrack enables the system-probe agent to connect to the netlink/conntrack subsystem to add NAT information to connection data. See also: http://conntrack-tools.netfilter.org/ Default: false |
| features.npm.enabled | Enabled enables Network Performance Monitoring. Default: false |
| features.oomKill.enabled | Enables the OOMKill eBPF-based check. Default: false |
| features.orchestratorExplorer.conf.configData | ConfigData corresponds to the configuration file content. |
| features.orchestratorExplorer.conf.configMap.items | Items maps a ConfigMap data key to a file path mount. |
| features.orchestratorExplorer.conf.configMap.name | Name is the name of the ConfigMap. |
| features.orchestratorExplorer.ddUrl | Override the API endpoint for the Orchestrator Explorer. URL Default: "https://orchestrator.datadoghq.com". |
| features.orchestratorExplorer.enabled | Enabled enables the Orchestrator Explorer. Default: true |
| features.orchestratorExplorer.extraTags | Additional tags to associate with the collected data in the form of `a b c`. This is a Cluster Agent option distinct from DD_TAGS that is used in the Orchestrator Explorer. |
| features.orchestratorExplorer.scrubContainers | ScrubContainers enables scrubbing of sensitive container data (passwords, tokens, etc. ). Default: true |
| features.prometheusScrape.additionalConfigs | AdditionalConfigs allows adding advanced Prometheus check configurations with custom discovery rules. |
| features.prometheusScrape.enableServiceEndpoints | EnableServiceEndpoints enables generating dedicated checks for service endpoints. Default: false |
| features.prometheusScrape.enabled | Enable autodiscovery of pods and services exposing Prometheus metrics. Default: false |
| features.tcpQueueLength.enabled | Enables the TCP queue length eBPF-based check. Default: false |
| features.usm.enabled | Enabled enables Universal Service Monitoring. Default: false |
| global.clusterAgentToken | ClusterAgentToken is the token for communication between the NodeAgent and ClusterAgent |
| global.clusterName | ClusterName sets a unique cluster name for the deployment to easily scope monitoring data in the Datadog app. |
| global.credentials.apiKey | APIKey configures your Datadog API key. See also: https://app.datadoghq.com/account/settings#agent/kubernetes |
| global.credentials.apiSecret.keyName | KeyName is the key of the secret to use. |
| global.credentials.apiSecret.secretName | SecretName is the name of the secret. |
| global.credentials.appKey | AppKey configures your Datadog application key. If you are using clusterAgent.metricsProvider.enabled = true, you must set a Datadog application key for read access to your metrics. |
| global.credentials.appSecret.keyName | KeyName is the key of the secret to use. |
| global.credentials.appSecret.secretName | SecretName is the name of the secret. |
| global.criSocketPath | Path to the container runtime socket (if different from Docker). |
| global.dockerSocketPath | Path to the docker runtime socket. |
| global.endpoint.credentials.apiKey | APIKey configures your Datadog API key. See also: https://app.datadoghq.com/account/settings#agent/kubernetes |
| global.endpoint.credentials.apiSecret.keyName | KeyName is the key of the secret to use. |
| global.endpoint.credentials.apiSecret.secretName | SecretName is the name of the secret. |
| global.endpoint.credentials.appKey | AppKey configures your Datadog application key. If you are using clusterAgent.metricsProvider.enabled = true, you must set a Datadog application key for read access to your metrics. |
| global.endpoint.credentials.appSecret.keyName | KeyName is the key of the secret to use. |
| global.endpoint.credentials.appSecret.secretName | SecretName is the name of the secret. |
| global.endpoint.url | URL defines the endpoint URL. |
| global.kubelet.agentCAPath | AgentCAPath is the container path where the kubelet CA certificate is stored. Default: '/var/run/host-kubelet-ca.crt' if hostCAPath is set, else '/var/run/secrets/kubernetes.io/serviceaccount/ca.crt' |
| global.kubelet.host.configMapKeyRef.key | The key to select. |
| global.kubelet.host.configMapKeyRef.name | Name of the referent. More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names TODO: Add other useful fields. apiVersion, kind, uid? |
| global.kubelet.host.configMapKeyRef.optional | Specify whether the ConfigMap or its key must be defined |
| global.kubelet.host.fieldRef.apiVersion | Version of the schema the FieldPath is written in terms of, defaults to "v1". |
| global.kubelet.host.fieldRef.fieldPath | Path of the field to select in the specified API version. |
| global.kubelet.host.resourceFieldRef.containerName | Container name: required for volumes, optional for env vars |
| global.kubelet.host.resourceFieldRef.divisor | Specifies the output format of the exposed resources, defaults to "1" |
| global.kubelet.host.resourceFieldRef.resource | Required: resource to select |
| global.kubelet.host.secretKeyRef.key | The key of the secret to select from.  Must be a valid secret key. |
| global.kubelet.host.secretKeyRef.name | Name of the referent. More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names TODO: Add other useful fields. apiVersion, kind, uid? |
| global.kubelet.host.secretKeyRef.optional | Specify whether the Secret or its key must be defined |
| global.kubelet.hostCAPath | HostCAPath is the host path where the kubelet CA certificate is stored. |
| global.kubelet.tlsVerify | TLSVerify toggles kubelet TLS verification. Default: true |
| global.localService.forceEnableLocalService | ForceEnableLocalService forces the creation of the internal traffic policy service to target the agent running on the local node. This parameter only applies to Kubernetes 1.21, where the feature is in alpha and is disabled by default. (On Kubernetes 1.22+, the feature entered beta and the internal traffic service is created by default, so this parameter is ignored.) Default: false |
| global.localService.nameOverride | NameOverride defines the name of the internal traffic service to target the agent running on the local node. |
| global.logLevel | LogLevel sets logging verbosity. This can be overridden by container. Valid log levels are: trace, debug, info, warn, error, critical, and off. Default: 'info' |
| global.networkPolicy.create | Create defines whether to create a NetworkPolicy for the current deployment. |
| global.networkPolicy.dnsSelectorEndpoints | DNSSelectorEndpoints defines the cilium selector of the DNS server entity. |
| global.networkPolicy.flavor | Flavor defines Which network policy to use. |
| global.podAnnotationsAsTags | Provide a mapping of Kubernetes Annotations to Datadog Tags. <KUBERNETES_ANNOTATIONS>: <DATADOG_TAG_KEY> |
| global.podLabelsAsTags | Provide a mapping of Kubernetes Labels to Datadog Tags. <KUBERNETES_LABEL>: <DATADOG_TAG_KEY> |
| global.registry | Registry is the image registry to use for all Agent images. Use 'public.ecr.aws/datadog' for AWS ECR. Use 'docker.io/datadog' for DockerHub. Default: 'gcr.io/datadoghq' |
| global.site | Site is the Datadog intake site Agent data are sent to. Set to 'datadoghq.eu' to send data to the EU site. Default: 'datadoghq.com' |
| global.tags | Tags contains a list of tags to attach to every metric, event and service check collected. Learn more about tagging: https://docs.datadoghq.com/tagging/ |
| override | Override the default configurations of the agents |

[1]: https://github.com/DataDog/datadog-operator/blob/main/examples/datadogagent/datadog-agent-all.yaml
[2]: https://github.com/DataDog/datadog-operator/blob/main/examples/datadogagent/datadog-agent-logs-apm.yaml
[3]: https://github.com/DataDog/datadog-operator/blob/main/examples/datadogagent/datadog-agent-logs.yaml
[4]: https://github.com/DataDog/datadog-operator/blob/main/examples/datadogagent/datadog-agent-apm.yaml
[5]: https://github.com/DataDog/datadog-operator/blob/main/examples/datadogagent/datadog-agent-with-clusteragent.yaml
[6]: https://github.com/DataDog/datadog-operator/blob/main/examples/datadogagent/datadog-agent-with-tolerations.yaml
