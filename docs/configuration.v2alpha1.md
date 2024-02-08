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
resource. For example, if you wanted to set a custom cluster name, your
`DatadogAgent` resource would look like the following:

```yaml
apiVersion: datadoghq.com/v2alpha1
kind: DatadogAgent
metadata:
  name: datadog
spec:
  global:
    clusterName: my-test-cluster
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
| features.admissionController.agentCommunicationMode | AgentCommunicationMode corresponds to the mode used by the Datadog application libraries to communicate with the Agent. It can be "hostip", "service", or "socket". |
| features.admissionController.enabled | Enabled enables the Admission Controller. Default: true |
| features.admissionController.failurePolicy | FailurePolicy determines how unrecognized and timeout errors are handled. |
| features.admissionController.mutateUnlabelled | MutateUnlabelled enables config injection without the need of pod label 'admission.datadoghq.com/enabled="true"'. Default: false |
| features.admissionController.serviceName | ServiceName corresponds to the webhook service name. |
| features.admissionController.webhookName | WebhookName is a custom name for the MutatingWebhookConfiguration. Default: "datadog-webhook" |
| features.apm.enabled | Enabled enables Application Performance Monitoring. Default: true |
| features.apm.hostPortConfig.enabled | Enabled enables host port configuration Default: false |
| features.apm.hostPortConfig.hostPort | Port takes a port number (0 < x < 65536) to expose on the host. (Most containers do not need this.) If HostNetwork is enabled, this value must match the ContainerPort. |
| features.apm.unixDomainSocketConfig.enabled | Enabled enables Unix Domain Socket. Default: true |
| features.apm.unixDomainSocketConfig.path | Path defines the socket path used when enabled. |
| features.clusterChecks.enabled | Enables Cluster Checks scheduling in the Cluster Agent. Default: true |
| features.clusterChecks.useClusterChecksRunners | Enabled enables Cluster Checks Runners to run all Cluster Checks. Default: false |
| features.cspm.checkInterval | CheckInterval defines the check interval. |
| features.cspm.customBenchmarks.configData | ConfigData corresponds to the configuration file content. |
| features.cspm.customBenchmarks.configMap.items | Items maps a ConfigMap data `key` to a file `path` mount. |
| features.cspm.customBenchmarks.configMap.name | Name is the name of the ConfigMap. |
| features.cspm.enabled | Enabled enables Cloud Security Posture Management. Default: false |
| features.cspm.hostBenchmarks.enabled | Enabled enables host benchmarks. Default: false |
| features.cws.customPolicies.configData | ConfigData corresponds to the configuration file content. |
| features.cws.customPolicies.configMap.items | Items maps a ConfigMap data `key` to a file `path` mount. |
| features.cws.customPolicies.configMap.name | Name is the name of the ConfigMap. |
| features.cws.enabled | Enabled enables Cloud Workload Security. Default: false |
| features.cws.network.enabled | Enabled enables Cloud Workload Security Network detections. Default: true |
| features.cws.remoteConfiguration.enabled | Enabled enables Remote Configuration for Cloud Workload Security. Default: true |
| features.cws.securityProfiles.enabled | Enabled enables Security Profiles collection for Cloud Workload Security. Default: true |
| features.cws.syscallMonitorEnabled | SyscallMonitorEnabled enables Syscall Monitoring (recommended for troubleshooting only). Default: false |
| features.dogstatsd.hostPortConfig.enabled | Enabled enables host port configuration Default: false |
| features.dogstatsd.hostPortConfig.hostPort | Port takes a port number (0 < x < 65536) to expose on the host. (Most containers do not need this.) If HostNetwork is enabled, this value must match the ContainerPort. |
| features.dogstatsd.mapperProfiles.configData | ConfigData corresponds to the configuration file content. |
| features.dogstatsd.mapperProfiles.configMap.items | Items maps a ConfigMap data `key` to a file `path` mount. |
| features.dogstatsd.mapperProfiles.configMap.name | Name is the name of the ConfigMap. |
| features.dogstatsd.originDetectionEnabled | OriginDetectionEnabled enables origin detection for container tagging. See also: https://docs.datadoghq.com/developers/dogstatsd/unix_socket/#using-origin-detection-for-container-tagging |
| features.dogstatsd.tagCardinality | TagCardinality configures tag cardinality for the metrics collected using origin detection (`low`, `orchestrator` or `high`). See also: https://docs.datadoghq.com/getting_started/tagging/assigning_tags/?tab=containerizedenvironments#environment-variables Cardinality default: low |
| features.dogstatsd.unixDomainSocketConfig.enabled | Enabled enables Unix Domain Socket. Default: true |
| features.dogstatsd.unixDomainSocketConfig.path | Path defines the socket path used when enabled. |
| features.ebpfCheck.enabled | Enables the eBPF check. Default: false |
| features.eventCollection.collectKubernetesEvents | CollectKubernetesEvents enables Kubernetes event collection. Default: true |
| features.externalMetricsServer.enabled | Enabled enables the External Metrics Server. Default: false |
| features.externalMetricsServer.endpoint.credentials.apiKey | APIKey configures your Datadog API key. See also: https://app.datadoghq.com/account/settings#agent/kubernetes |
| features.externalMetricsServer.endpoint.credentials.apiSecret.keyName | KeyName is the key of the secret to use. |
| features.externalMetricsServer.endpoint.credentials.apiSecret.secretName | SecretName is the name of the secret. |
| features.externalMetricsServer.endpoint.credentials.appKey | AppKey configures your Datadog application key. If you are using features.externalMetricsServer.enabled = true, you must set a Datadog application key for read access to your metrics. |
| features.externalMetricsServer.endpoint.credentials.appSecret.keyName | KeyName is the key of the secret to use. |
| features.externalMetricsServer.endpoint.credentials.appSecret.secretName | SecretName is the name of the secret. |
| features.externalMetricsServer.endpoint.url | URL defines the endpoint URL. |
| features.externalMetricsServer.port | Port specifies the metricsProvider External Metrics Server service port. Default: 8443 |
| features.externalMetricsServer.registerAPIService | RegisterAPIService registers the External Metrics endpoint as an APIService Default: true |
| features.externalMetricsServer.useDatadogMetrics | UseDatadogMetrics enables usage of the DatadogMetrics CRD (allowing one to scale on arbitrary Datadog metric queries). Default: true |
| features.externalMetricsServer.wpaController | WPAController enables the informer and controller of the Watermark Pod Autoscaler. NOTE: The Watermark Pod Autoscaler controller needs to be installed. See also: https://github.com/DataDog/watermarkpodautoscaler. Default: false |
| features.kubeStateMetricsCore.conf.configData | ConfigData corresponds to the configuration file content. |
| features.kubeStateMetricsCore.conf.configMap.items | Items maps a ConfigMap data `key` to a file `path` mount. |
| features.kubeStateMetricsCore.conf.configMap.name | Name is the name of the ConfigMap. |
| features.kubeStateMetricsCore.enabled | Enabled enables Kube State Metrics Core. Default: true |
| features.liveContainerCollection.enabled | Enables container collection for the Live Container View. Default: true |
| features.liveProcessCollection.enabled | Enabled enables Process monitoring. Default: false |
| features.liveProcessCollection.scrubProcessArguments | ScrubProcessArguments enables scrubbing of sensitive data in process command-lines (passwords, tokens, etc. ). Default: true |
| features.liveProcessCollection.stripProcessArguments | StripProcessArguments enables stripping of all process arguments. Default: false |
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
| features.orchestratorExplorer.conf.configMap.items | Items maps a ConfigMap data `key` to a file `path` mount. |
| features.orchestratorExplorer.conf.configMap.name | Name is the name of the ConfigMap. |
| features.orchestratorExplorer.customResources | `CustomResources` defines custom resources for the orchestrator explorer to collect. Each item should follow the convention `group/version/kind`. For example, `datadoghq.com/v1alpha1/datadogmetrics`. |
| features.orchestratorExplorer.ddUrl | Override the API endpoint for the Orchestrator Explorer. URL Default: "https://orchestrator.datadoghq.com". |
| features.orchestratorExplorer.enabled | Enabled enables the Orchestrator Explorer. Default: true |
| features.orchestratorExplorer.extraTags | Additional tags to associate with the collected data in the form of `a b c`. This is a Cluster Agent option distinct from DD_TAGS that is used in the Orchestrator Explorer. |
| features.orchestratorExplorer.scrubContainers | ScrubContainers enables scrubbing of sensitive container data (passwords, tokens, etc. ). Default: true |
| features.otlp.receiver.protocols.grpc.enabled | Enable the OTLP/gRPC endpoint. |
| features.otlp.receiver.protocols.grpc.endpoint | Endpoint for OTLP/gRPC. gRPC supports several naming schemes: https://github.com/grpc/grpc/blob/master/doc/naming.md The Datadog Operator supports only 'host:port' (usually `0.0.0.0:port`). Default: `0.0.0.0:4317`. |
| features.otlp.receiver.protocols.http.enabled | Enable the OTLP/HTTP endpoint. |
| features.otlp.receiver.protocols.http.endpoint | Endpoint for OTLP/HTTP. Default: '0.0.0.0:4318'. |
| features.processDiscovery.enabled | Enabled enables the Process Discovery check in the Agent. Default: true |
| features.prometheusScrape.additionalConfigs | AdditionalConfigs allows adding advanced Prometheus check configurations with custom discovery rules. |
| features.prometheusScrape.enableServiceEndpoints | EnableServiceEndpoints enables generating dedicated checks for service endpoints. Default: false |
| features.prometheusScrape.enabled | Enable autodiscovery of pods and services exposing Prometheus metrics. Default: false |
| features.prometheusScrape.version | Version specifies the version of the OpenMetrics check. Default: 2 |
| features.remoteConfiguration.enabled | Enable this option to activate Remote Configuration. Default: true |
| features.sbom.containerImage.analyzers | Analyzers to use for SBOM collection. |
| features.sbom.containerImage.enabled | Enable this option to activate SBOM collection. Default: false |
| features.sbom.enabled | Enable this option to activate SBOM collection. Default: false |
| features.sbom.host.analyzers | Analyzers to use for SBOM collection. |
| features.sbom.host.enabled | Enable this option to activate SBOM collection. Default: false |
| features.tcpQueueLength.enabled | Enables the TCP queue length eBPF-based check. Default: false |
| features.usm.enabled | Enabled enables Universal Service Monitoring. Default: false |
| global.clusterAgentToken | ClusterAgentToken is the token for communication between the NodeAgent and ClusterAgent. |
| global.clusterAgentTokenSecret.keyName | KeyName is the key of the secret to use. |
| global.clusterAgentTokenSecret.secretName | SecretName is the name of the secret. |
| global.clusterName | ClusterName sets a unique cluster name for the deployment to easily scope monitoring data in the Datadog app. |
| global.containerStrategy | ContainerStrategy determines whether agents run in a single or multiple containers. Default: 'optimized' |
| global.credentials.apiKey | APIKey configures your Datadog API key. See also: https://app.datadoghq.com/account/settings#agent/kubernetes |
| global.credentials.apiSecret.keyName | KeyName is the key of the secret to use. |
| global.credentials.apiSecret.secretName | SecretName is the name of the secret. |
| global.credentials.appKey | AppKey configures your Datadog application key. If you are using features.externalMetricsServer.enabled = true, you must set a Datadog application key for read access to your metrics. |
| global.credentials.appSecret.keyName | KeyName is the key of the secret to use. |
| global.credentials.appSecret.secretName | SecretName is the name of the secret. |
| global.criSocketPath | Path to the container runtime socket (if different from Docker). |
| global.disableNonResourceRules | Set DisableNonResourceRules to exclude NonResourceURLs from default ClusterRoles. Required 'true' for Google Cloud Marketplace. |
| global.dockerSocketPath | Path to the docker runtime socket. |
| global.endpoint.credentials.apiKey | APIKey configures your Datadog API key. See also: https://app.datadoghq.com/account/settings#agent/kubernetes |
| global.endpoint.credentials.apiSecret.keyName | KeyName is the key of the secret to use. |
| global.endpoint.credentials.apiSecret.secretName | SecretName is the name of the secret. |
| global.endpoint.credentials.appKey | AppKey configures your Datadog application key. If you are using features.externalMetricsServer.enabled = true, you must set a Datadog application key for read access to your metrics. |
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
| global.namespaceLabelsAsTags | Provide a mapping of Kubernetes Namespace Labels to Datadog Tags. <KUBERNETES_NAMESPACE_LABEL>: <DATADOG_TAG_KEY> |
| global.networkPolicy.create | Create defines whether to create a NetworkPolicy for the current deployment. |
| global.networkPolicy.dnsSelectorEndpoints | DNSSelectorEndpoints defines the cilium selector of the DNS server entity. |
| global.networkPolicy.flavor | Flavor defines Which network policy to use. |
| global.nodeLabelsAsTags | Provide a mapping of Kubernetes Node Labels to Datadog Tags. <KUBERNETES_NODE_LABEL>: <DATADOG_TAG_KEY> |
| global.podAnnotationsAsTags | Provide a mapping of Kubernetes Annotations to Datadog Tags. <KUBERNETES_ANNOTATIONS>: <DATADOG_TAG_KEY> |
| global.podLabelsAsTags | Provide a mapping of Kubernetes Labels to Datadog Tags. <KUBERNETES_LABEL>: <DATADOG_TAG_KEY> |
| global.registry | Registry is the image registry to use for all Agent images. Use 'public.ecr.aws/datadog' for AWS ECR. Use 'docker.io/datadog' for DockerHub. Default: 'gcr.io/datadoghq' |
| global.site | Site is the Datadog intake site Agent data are sent to. Set to 'datadoghq.com' to send data to the US1 site (default). Set to 'datadoghq.eu' to send data to the EU site. Set to 'us3.datadoghq.com' to send data to the US3 site. Set to 'us5.datadoghq.com' to send data to the US5 site. Set to 'ddog-gov.com' to send data to the US1-FED site. Set to 'ap1.datadoghq.com' to send data to the AP1 site. Default: 'datadoghq.com' |
| global.tags | Tags contains a list of tags to attach to every metric, event and service check collected. Learn more about tagging: https://docs.datadoghq.com/tagging/ |
| override | Override the default configurations of the agents |
<br>

### Override

The table below lists parameters that can be used to override default or global settings. Maps and arrays have a type annotation in the table; properties that are configured as map values contain a `[key]` element which should be replaced by the actual map key. `override` itself is a map with the following possible keys: `nodeAgent`, `clusterAgent`, or `clusterChecksRunner`. Other keys can be added, but they do not have any effect.

For example, the manifest below can be used to override the node Agent image, tag, and the resource limits of the system probe container. 

```yaml
apiVersion: datadoghq.com/v2alpha1
kind: DatadogAgent
metadata:
  name: datadog
spec:
  override:
    nodeAgent:
      image:
        name: agent
        tag: 7.41.0-rc.5
      containers:
        system-probe:
          resources:
            limits:
              cpu: "2"
              memory: 1Gi
```
In the table, `spec.override.nodeAgent.image.name` and `spec.override.nodeAgent.containers.system-probe.resources.limits` appear as `[key].image.name` and `[key].containers.[key].resources.limits`, respectively.


| Parameter | Description |
| --------- | ----------- |
| [key].affinity.nodeAffinity.preferredDuringSchedulingIgnoredDuringExecution | The scheduler will prefer to schedule pods to nodes that satisfy the affinity expressions specified by this field, but it may choose a node that violates one or more of the expressions. The node that is most preferred is the one with the greatest sum of weights, i.e. for each node that meets all of the scheduling requirements (resource request, requiredDuringScheduling affinity expressions, etc.), compute a sum by iterating through the elements of this field and adding "weight" to the sum if the node matches the corresponding matchExpressions; the node(s) with the highest sum are the most preferred. |
| [key].affinity.nodeAffinity.requiredDuringSchedulingIgnoredDuringExecution.nodeSelectorTerms | Required. A list of node selector terms. The terms are ORed. |
| [key].affinity.podAffinity.preferredDuringSchedulingIgnoredDuringExecution | The scheduler will prefer to schedule pods to nodes that satisfy the affinity expressions specified by this field, but it may choose a node that violates one or more of the expressions. The node that is most preferred is the one with the greatest sum of weights, i.e. for each node that meets all of the scheduling requirements (resource request, requiredDuringScheduling affinity expressions, etc.), compute a sum by iterating through the elements of this field and adding "weight" to the sum if the node has pods which matches the corresponding podAffinityTerm; the node(s) with the highest sum are the most preferred. |
| [key].affinity.podAffinity.requiredDuringSchedulingIgnoredDuringExecution | If the affinity requirements specified by this field are not met at scheduling time, the pod will not be scheduled onto the node. If the affinity requirements specified by this field cease to be met at some point during pod execution (e.g. due to a pod label update), the system may or may not try to eventually evict the pod from its node. When there are multiple elements, the lists of nodes corresponding to each podAffinityTerm are intersected, i.e. all terms must be satisfied. |
| [key].affinity.podAntiAffinity.preferredDuringSchedulingIgnoredDuringExecution | The scheduler will prefer to schedule pods to nodes that satisfy the anti-affinity expressions specified by this field, but it may choose a node that violates one or more of the expressions. The node that is most preferred is the one with the greatest sum of weights, i.e. for each node that meets all of the scheduling requirements (resource request, requiredDuringScheduling anti-affinity expressions, etc.), compute a sum by iterating through the elements of this field and adding "weight" to the sum if the node has pods which matches the corresponding podAffinityTerm; the node(s) with the highest sum are the most preferred. |
| [key].affinity.podAntiAffinity.requiredDuringSchedulingIgnoredDuringExecution | If the anti-affinity requirements specified by this field are not met at scheduling time, the pod will not be scheduled onto the node. If the anti-affinity requirements specified by this field cease to be met at some point during pod execution (e.g. due to a pod label update), the system may or may not try to eventually evict the pod from its node. When there are multiple elements, the lists of nodes corresponding to each podAffinityTerm are intersected, i.e. all terms must be satisfied. |
| [key].annotations `map[string]string` | Annotations provide annotations that are added to the different component (Datadog Agent, Cluster Agent, Cluster Check Runner) pods. |
| [key].containers `map[string]object` | Configure the basic configurations for each Agent container. Valid Agent container names are: `agent`, `cluster-agent`, `init-config`, `init-volume`, `process-agent`, `seccomp-setup`, `security-agent`, `system-probe`, `trace-agent`, and `all`. Configuration under `all` applies to all configured containers. |
| [key].containers.[key].appArmorProfileName | AppArmorProfileName specifies an apparmor profile. |
| [key].containers.[key].args `[]string` | Args allows the specification of extra args to the `Command` parameter |
| [key].containers.[key].command `[]string` | Command allows the specification of a custom entrypoint for container |
| [key].containers.[key].env `[]object` | Specify additional environment variables in the container. See also: https://docs.datadoghq.com/agent/kubernetes/?tab=helm#environment-variables |
| [key].containers.[key].healthPort | HealthPort of the container for the internal liveness probe. Must be the same as the Liveness/Readiness probes. |
| [key].containers.[key].livenessProbe.exec.command | Command is the command line to execute inside the container, the working directory for the command  is root ('/') in the container's filesystem. The command is simply exec'd, it is not run inside a shell, so traditional shell instructions ('|', etc) won't work. To use a shell, you need to explicitly call out to that shell. Exit status of 0 is treated as live/healthy and non-zero is unhealthy. |
| [key].containers.[key].livenessProbe.failureThreshold | Minimum consecutive failures for the probe to be considered failed after having succeeded. Defaults to 3. Minimum value is 1. |
| [key].containers.[key].livenessProbe.grpc.port | Port number of the gRPC service. Number must be in the range 1 to 65535. |
| [key].containers.[key].livenessProbe.grpc.service | Service is the name of the service to place in the gRPC HealthCheckRequest (see https://github.com/grpc/grpc/blob/master/doc/health-checking.md).  If this is not specified, the default behavior is defined by gRPC. |
| [key].containers.[key].livenessProbe.httpGet.host | Host name to connect to, defaults to the pod IP. You probably want to set "Host" in httpHeaders instead. |
| [key].containers.[key].livenessProbe.httpGet.httpHeaders | Custom headers to set in the request. HTTP allows repeated headers. |
| [key].containers.[key].livenessProbe.httpGet.path | Path to access on the HTTP server. |
| [key].containers.[key].livenessProbe.httpGet.port | Name or number of the port to access on the container. Number must be in the range 1 to 65535. Name must be an IANA_SVC_NAME. |
| [key].containers.[key].livenessProbe.httpGet.scheme | Scheme to use for connecting to the host. Defaults to HTTP. |
| [key].containers.[key].livenessProbe.initialDelaySeconds | Number of seconds after the container has started before liveness probes are initiated. More info: https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle#container-probes |
| [key].containers.[key].livenessProbe.periodSeconds | How often (in seconds) to perform the probe. Default to 10 seconds. Minimum value is 1. |
| [key].containers.[key].livenessProbe.successThreshold | Minimum consecutive successes for the probe to be considered successful after having failed. Defaults to 1. Must be 1 for liveness and startup. Minimum value is 1. |
| [key].containers.[key].livenessProbe.tcpSocket.host | Optional: Host name to connect to, defaults to the pod IP. |
| [key].containers.[key].livenessProbe.tcpSocket.port | Number or name of the port to access on the container. Number must be in the range 1 to 65535. Name must be an IANA_SVC_NAME. |
| [key].containers.[key].livenessProbe.terminationGracePeriodSeconds | Optional duration in seconds the pod needs to terminate gracefully upon probe failure. The grace period is the duration in seconds after the processes running in the pod are sent a termination signal and the time when the processes are forcibly halted with a kill signal. Set this value longer than the expected cleanup time for your process. If this value is nil, the pod's terminationGracePeriodSeconds will be used. Otherwise, this value overrides the value provided by the pod spec. Value must be non-negative integer. The value zero indicates stop immediately via the kill signal (no opportunity to shut down). This is a beta field and requires enabling ProbeTerminationGracePeriod feature gate. Minimum value is 1. spec.terminationGracePeriodSeconds is used if unset. |
| [key].containers.[key].livenessProbe.timeoutSeconds | Number of seconds after which the probe times out. Defaults to 1 second. Minimum value is 1. More info: https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle#container-probes |
| [key].containers.[key].logLevel | LogLevel sets logging verbosity (overrides global setting). Valid log levels are: trace, debug, info, warn, error, critical, and off. Default: 'info' |
| [key].containers.[key].name | Name of the container that is overridden |
| [key].containers.[key].readinessProbe.exec.command | Command is the command line to execute inside the container, the working directory for the command  is root ('/') in the container's filesystem. The command is simply exec'd, it is not run inside a shell, so traditional shell instructions ('|', etc) won't work. To use a shell, you need to explicitly call out to that shell. Exit status of 0 is treated as live/healthy and non-zero is unhealthy. |
| [key].containers.[key].readinessProbe.failureThreshold | Minimum consecutive failures for the probe to be considered failed after having succeeded. Defaults to 3. Minimum value is 1. |
| [key].containers.[key].readinessProbe.grpc.port | Port number of the gRPC service. Number must be in the range 1 to 65535. |
| [key].containers.[key].readinessProbe.grpc.service | Service is the name of the service to place in the gRPC HealthCheckRequest (see https://github.com/grpc/grpc/blob/master/doc/health-checking.md).  If this is not specified, the default behavior is defined by gRPC. |
| [key].containers.[key].readinessProbe.httpGet.host | Host name to connect to, defaults to the pod IP. You probably want to set "Host" in httpHeaders instead. |
| [key].containers.[key].readinessProbe.httpGet.httpHeaders | Custom headers to set in the request. HTTP allows repeated headers. |
| [key].containers.[key].readinessProbe.httpGet.path | Path to access on the HTTP server. |
| [key].containers.[key].readinessProbe.httpGet.port | Name or number of the port to access on the container. Number must be in the range 1 to 65535. Name must be an IANA_SVC_NAME. |
| [key].containers.[key].readinessProbe.httpGet.scheme | Scheme to use for connecting to the host. Defaults to HTTP. |
| [key].containers.[key].readinessProbe.initialDelaySeconds | Number of seconds after the container has started before liveness probes are initiated. More info: https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle#container-probes |
| [key].containers.[key].readinessProbe.periodSeconds | How often (in seconds) to perform the probe. Default to 10 seconds. Minimum value is 1. |
| [key].containers.[key].readinessProbe.successThreshold | Minimum consecutive successes for the probe to be considered successful after having failed. Defaults to 1. Must be 1 for liveness and startup. Minimum value is 1. |
| [key].containers.[key].readinessProbe.tcpSocket.host | Optional: Host name to connect to, defaults to the pod IP. |
| [key].containers.[key].readinessProbe.tcpSocket.port | Number or name of the port to access on the container. Number must be in the range 1 to 65535. Name must be an IANA_SVC_NAME. |
| [key].containers.[key].readinessProbe.terminationGracePeriodSeconds | Optional duration in seconds the pod needs to terminate gracefully upon probe failure. The grace period is the duration in seconds after the processes running in the pod are sent a termination signal and the time when the processes are forcibly halted with a kill signal. Set this value longer than the expected cleanup time for your process. If this value is nil, the pod's terminationGracePeriodSeconds will be used. Otherwise, this value overrides the value provided by the pod spec. Value must be non-negative integer. The value zero indicates stop immediately via the kill signal (no opportunity to shut down). This is a beta field and requires enabling ProbeTerminationGracePeriod feature gate. Minimum value is 1. spec.terminationGracePeriodSeconds is used if unset. |
| [key].containers.[key].readinessProbe.timeoutSeconds | Number of seconds after which the probe times out. Defaults to 1 second. Minimum value is 1. More info: https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle#container-probes |
| [key].containers.[key].resources.limits | Limits describes the maximum amount of compute resources allowed. More info: https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/ |
| [key].containers.[key].resources.requests | Requests describes the minimum amount of compute resources required. If Requests is omitted for a container, it defaults to Limits if that is explicitly specified, otherwise to an implementation-defined value. More info: https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/ |
| [key].containers.[key].seccompConfig.customProfile.configData | ConfigData corresponds to the configuration file content. |
| [key].containers.[key].seccompConfig.customProfile.configMap.items | Items maps a ConfigMap data `key` to a file `path` mount. |
| [key].containers.[key].seccompConfig.customProfile.configMap.name | Name is the name of the ConfigMap. |
| [key].containers.[key].seccompConfig.customRootPath | CustomRootPath specifies a custom Seccomp Profile root location. |
| [key].containers.[key].securityContext.allowPrivilegeEscalation | AllowPrivilegeEscalation controls whether a process can gain more privileges than its parent process. This bool directly controls if the no_new_privs flag will be set on the container process. AllowPrivilegeEscalation is true always when the container is: 1) run as Privileged 2) has CAP_SYS_ADMIN Note that this field cannot be set when spec.os.name is windows. |
| [key].containers.[key].securityContext.capabilities.add | Added capabilities |
| [key].containers.[key].securityContext.capabilities.drop | Removed capabilities |
| [key].containers.[key].securityContext.privileged | Run container in privileged mode. Processes in privileged containers are essentially equivalent to root on the host. Defaults to false. Note that this field cannot be set when spec.os.name is windows. |
| [key].containers.[key].securityContext.procMount | procMount denotes the type of proc mount to use for the containers. The default is DefaultProcMount which uses the container runtime defaults for readonly paths and masked paths. This requires the ProcMountType feature flag to be enabled. Note that this field cannot be set when spec.os.name is windows. |
| [key].containers.[key].securityContext.readOnlyRootFilesystem | Whether this container has a read-only root filesystem. Default is false. Note that this field cannot be set when spec.os.name is windows. |
| [key].containers.[key].securityContext.runAsGroup | The GID to run the entrypoint of the container process. Uses runtime default if unset. May also be set in PodSecurityContext.  If set in both SecurityContext and PodSecurityContext, the value specified in SecurityContext takes precedence. Note that this field cannot be set when spec.os.name is windows. |
| [key].containers.[key].securityContext.runAsNonRoot | Indicates that the container must run as a non-root user. If true, the Kubelet will validate the image at runtime to ensure that it does not run as UID 0 (root) and fail to start the container if it does. If unset or false, no such validation will be performed. May also be set in PodSecurityContext.  If set in both SecurityContext and PodSecurityContext, the value specified in SecurityContext takes precedence. |
| [key].containers.[key].securityContext.runAsUser | The UID to run the entrypoint of the container process. Defaults to user specified in image metadata if unspecified. May also be set in PodSecurityContext.  If set in both SecurityContext and PodSecurityContext, the value specified in SecurityContext takes precedence. Note that this field cannot be set when spec.os.name is windows. |
| [key].containers.[key].securityContext.seLinuxOptions.level | Level is SELinux level label that applies to the container. |
| [key].containers.[key].securityContext.seLinuxOptions.role | Role is a SELinux role label that applies to the container. |
| [key].containers.[key].securityContext.seLinuxOptions.type | Type is a SELinux type label that applies to the container. |
| [key].containers.[key].securityContext.seLinuxOptions.user | User is a SELinux user label that applies to the container. |
| [key].containers.[key].securityContext.seccompProfile.localhostProfile | localhostProfile indicates a profile defined in a file on the node should be used. The profile must be preconfigured on the node to work. Must be a descending path, relative to the kubelet's configured seccomp profile location. Must only be set if type is "Localhost". |
| [key].containers.[key].securityContext.seccompProfile.type | type indicates which kind of seccomp profile will be applied. Valid options are:  Localhost - a profile defined in a file on the node should be used. RuntimeDefault - the container runtime default profile should be used. Unconfined - no profile should be applied. |
| [key].containers.[key].securityContext.windowsOptions.gmsaCredentialSpec | GMSACredentialSpec is where the GMSA admission webhook (https://github.com/kubernetes-sigs/windows-gmsa) inlines the contents of the GMSA credential spec named by the GMSACredentialSpecName field. |
| [key].containers.[key].securityContext.windowsOptions.gmsaCredentialSpecName | GMSACredentialSpecName is the name of the GMSA credential spec to use. |
| [key].containers.[key].securityContext.windowsOptions.hostProcess | HostProcess determines if a container should be run as a 'Host Process' container. This field is alpha-level and will only be honored by components that enable the WindowsHostProcessContainers feature flag. Setting this field without the feature flag will result in errors when validating the Pod. All of a Pod's containers must have the same effective HostProcess value (it is not allowed to have a mix of HostProcess containers and non-HostProcess containers).  In addition, if HostProcess is true then HostNetwork must also be set to true. |
| [key].containers.[key].securityContext.windowsOptions.runAsUserName | The UserName in Windows to run the entrypoint of the container process. Defaults to the user specified in image metadata if unspecified. May also be set in PodSecurityContext. If set in both SecurityContext and PodSecurityContext, the value specified in SecurityContext takes precedence. |
| [key].containers.[key].volumeMounts `[]object` | Specify additional volume mounts in the container. |
| [key].createRbac | Set CreateRbac to false to prevent automatic creation of Role/ClusterRole for this component |
| [key].customConfigurations `map[string]object` | CustomConfiguration allows to specify custom configuration files for `datadog.yaml`, `datadog-cluster.yaml`, `security-agent.yaml`, and `system-probe.yaml`. The content is merged with configuration generated by the Datadog Operator, with priority given to custom configuration. WARNING: It is possible to override values set in the `DatadogAgent`. |
| [key].customConfigurations.[key].configData | ConfigData corresponds to the configuration file content. |
| [key].customConfigurations.[key].configMap.items | Items maps a ConfigMap data `key` to a file `path` mount. |
| [key].customConfigurations.[key].configMap.name | Name is the name of the ConfigMap. |
| [key].disabled | Disabled force disables a component. |
| [key].env `[]object` | Specify additional environment variables for all containers in this component Priority is Container > Component. See also: https://docs.datadoghq.com/agent/kubernetes/?tab=helm#environment-variables |
| [key].extraChecksd.configDataMap | ConfigDataMap corresponds to the content of the configuration files. The key should be the filename the contents get mounted to; for instance check.py or check.yaml. |
| [key].extraChecksd.configMap.items | Items maps a ConfigMap data `key` to a file `path` mount. |
| [key].extraChecksd.configMap.name | Name is the name of the ConfigMap. |
| [key].extraConfd.configDataMap | ConfigDataMap corresponds to the content of the configuration files. The key should be the filename the contents get mounted to; for instance check.py or check.yaml. |
| [key].extraConfd.configMap.items | Items maps a ConfigMap data `key` to a file `path` mount. |
| [key].extraConfd.configMap.name | Name is the name of the ConfigMap. |
| [key].hostNetwork | Host networking requested for this pod. Use the host's network namespace. |
| [key].hostPID | Use the host's PID namespace. |
| [key].image.jmxEnabled | Define whether the Agent image should support JMX. To be used if the Name field does not correspond to a full image string. |
| [key].image.name | Define the image to use: Use "gcr.io/datadoghq/agent:latest" for Datadog Agent 7. Use "datadog/dogstatsd:latest" for standalone Datadog Agent DogStatsD 7. Use "gcr.io/datadoghq/cluster-agent:latest" for Datadog Cluster Agent. Use "agent" with the registry and tag configurations for <registry>/agent:<tag>. Use "cluster-agent" with the registry and tag configurations for <registry>/cluster-agent:<tag>. If the name is the full image string—`<name>:<tag>` or `<registry>/<name>:<tag>`, then `tag`, `jmxEnabled`, and `global.registry` values are ignored. Otherwise, image string is created by overriding default settings with supplied `name`, `tag`, and `jmxEnabled` values; image string is created using default registry unless `global.registry` is configured. |
| [key].image.pullPolicy | The Kubernetes pull policy: Use Always, Never, or IfNotPresent. |
| [key].image.pullSecrets | It is possible to specify Docker registry credentials. See https://kubernetes.io/docs/concepts/containers/images/#specifying-imagepullsecrets-on-a-pod |
| [key].image.tag | Define the image tag to use. To be used if the Name field does not correspond to a full image string. |
| [key].labels `map[string]string` | AdditionalLabels provide labels that are added to the different component (Datadog Agent, Cluster Agent, Cluster Check Runner) pods. |
| [key].name | Name overrides the default name for the resource |
| [key].nodeSelector `map[string]string` | NodeSelector is a selector which must be true for the pod to fit on a node. Selector which must match a node's labels for the pod to be scheduled on that node. More info: https://kubernetes.io/docs/concepts/configuration/assign-pod-node/ |
| [key].priorityClassName | If specified, indicates the pod's priority. "system-node-critical" and "system-cluster-critical" are two special keywords which indicate the highest priorities with the former being the highest priority. Any other name must be defined by creating a PriorityClass object with that name. If not specified, the pod priority is default, or zero if there is no default. |
| [key].replicas | Number of the replicas. Not applicable for a DaemonSet/ExtendedDaemonSet deployment |
| [key].securityContext.fsGroup | A special supplemental group that applies to all containers in a pod. Some volume types allow the Kubelet to change the ownership of that volume to be owned by the pod:  1. The owning GID will be the FSGroup 2. The setgid bit is set (new files created in the volume will be owned by FSGroup) 3. The permission bits are OR'd with rw-rw----  If unset, the Kubelet will not modify the ownership and permissions of any volume. Note that this field cannot be set when spec.os.name is windows. |
| [key].securityContext.fsGroupChangePolicy | fsGroupChangePolicy defines behavior of changing ownership and permission of the volume before being exposed inside Pod. This field will only apply to volume types which support fsGroup based ownership(and permissions). It will have no effect on ephemeral volume types such as: secret, configmaps and emptydir. Valid values are "OnRootMismatch" and "Always". If not specified, "Always" is used. Note that this field cannot be set when spec.os.name is windows. |
| [key].securityContext.runAsGroup | The GID to run the entrypoint of the container process. Uses runtime default if unset. May also be set in SecurityContext.  If set in both SecurityContext and PodSecurityContext, the value specified in SecurityContext takes precedence for that container. Note that this field cannot be set when spec.os.name is windows. |
| [key].securityContext.runAsNonRoot | Indicates that the container must run as a non-root user. If true, the Kubelet will validate the image at runtime to ensure that it does not run as UID 0 (root) and fail to start the container if it does. If unset or false, no such validation will be performed. May also be set in SecurityContext.  If set in both SecurityContext and PodSecurityContext, the value specified in SecurityContext takes precedence. |
| [key].securityContext.runAsUser | The UID to run the entrypoint of the container process. Defaults to user specified in image metadata if unspecified. May also be set in SecurityContext.  If set in both SecurityContext and PodSecurityContext, the value specified in SecurityContext takes precedence for that container. Note that this field cannot be set when spec.os.name is windows. |
| [key].securityContext.seLinuxOptions.level | Level is SELinux level label that applies to the container. |
| [key].securityContext.seLinuxOptions.role | Role is a SELinux role label that applies to the container. |
| [key].securityContext.seLinuxOptions.type | Type is a SELinux type label that applies to the container. |
| [key].securityContext.seLinuxOptions.user | User is a SELinux user label that applies to the container. |
| [key].securityContext.seccompProfile.localhostProfile | localhostProfile indicates a profile defined in a file on the node should be used. The profile must be preconfigured on the node to work. Must be a descending path, relative to the kubelet's configured seccomp profile location. Must only be set if type is "Localhost". |
| [key].securityContext.seccompProfile.type | type indicates which kind of seccomp profile will be applied. Valid options are:  Localhost - a profile defined in a file on the node should be used. RuntimeDefault - the container runtime default profile should be used. Unconfined - no profile should be applied. |
| [key].securityContext.supplementalGroups | A list of groups applied to the first process run in each container, in addition to the container's primary GID.  If unspecified, no groups will be added to any container. Note that this field cannot be set when spec.os.name is windows. |
| [key].securityContext.sysctls | Sysctls hold a list of namespaced sysctls used for the pod. Pods with unsupported sysctls (by the container runtime) might fail to launch. Note that this field cannot be set when spec.os.name is windows. |
| [key].securityContext.windowsOptions.gmsaCredentialSpec | GMSACredentialSpec is where the GMSA admission webhook (https://github.com/kubernetes-sigs/windows-gmsa) inlines the contents of the GMSA credential spec named by the GMSACredentialSpecName field. |
| [key].securityContext.windowsOptions.gmsaCredentialSpecName | GMSACredentialSpecName is the name of the GMSA credential spec to use. |
| [key].securityContext.windowsOptions.hostProcess | HostProcess determines if a container should be run as a 'Host Process' container. This field is alpha-level and will only be honored by components that enable the WindowsHostProcessContainers feature flag. Setting this field without the feature flag will result in errors when validating the Pod. All of a Pod's containers must have the same effective HostProcess value (it is not allowed to have a mix of HostProcess containers and non-HostProcess containers).  In addition, if HostProcess is true then HostNetwork must also be set to true. |
| [key].securityContext.windowsOptions.runAsUserName | The UserName in Windows to run the entrypoint of the container process. Defaults to the user specified in image metadata if unspecified. May also be set in PodSecurityContext. If set in both SecurityContext and PodSecurityContext, the value specified in SecurityContext takes precedence. |
| [key].serviceAccountName | Sets the ServiceAccount used by this component. Ignored if the field CreateRbac is true. |
| [key].tolerations `[]object` | Configure the component tolerations. |
| [key].volumes `[]object` | Specify additional volumes in the different components (Datadog Agent, Cluster Agent, Cluster Check Runner). |

[1]: https://github.com/DataDog/datadog-operator/blob/main/examples/datadogagent/v2alpha1/datadog-agent-all.yaml
[2]: https://github.com/DataDog/datadog-operator/blob/main/examples/datadogagent/v2alpha1/datadog-agent-logs-apm.yaml
[3]: https://github.com/DataDog/datadog-operator/blob/main/examples/datadogagent/v2alpha1/datadog-agent-logs.yaml
[4]: https://github.com/DataDog/datadog-operator/blob/main/examples/datadogagent/v2alpha1/datadog-agent-apm.yaml
[5]: https://github.com/DataDog/datadog-operator/blob/main/examples/datadogagent/v2alpha1/datadog-agent-with-clusteragent.yaml
[6]: https://github.com/DataDog/datadog-operator/blob/main/examples/datadogagent/v2alpha1/datadog-agent-with-tolerations.yaml
