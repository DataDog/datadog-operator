This page lists commonly-used configuration parameters for the Datadog Operator. For all configuration parameters, see the [configuration spec][1] in the [`DataDog/datadog-operator`][2] repo.

### Example manifests

* [Manifest with logs, APM, process, and metrics collection enabled][3]
* [Manifest with logs, APM, and metrics collection enabled][4]
* [Manifest with APM and metrics collection enabled][5]
* [Manifest with Cluster Agent][6]
* [Manifest with tolerations][7]

## Global options

The table in this section lists configurable parameters for the `DatadogAgent` resource. To override parameters for individual components (Node Agent, Cluster Agent, or Cluster Checks Runner) see [override options](#override-options).

For example: the following manifest uses the `global.clusterName` parameter to set a custom cluster name:

{{< highlight yaml "hl_lines=7" >}}
{% collapse-content title="Parameters" level="h4" expanded=true id="global-options-list" %}}

`features.admissionController.agentCommunicationMode`n: AgentCommunicationMode corresponds to the mode used by the Datadog application libraries to communicate with the Agent. It can be "hostip", "service", or "socket".

`features.admissionController.agentSidecarInjection.clusterAgentCommunicationEnabled`n: ClusterAgentCommunicationEnabled enables communication between Agent sidecars and the Cluster Agent. Default : true

`features.admissionController.agentSidecarInjection.enabled`n: Enables Sidecar injections. Default: false

`features.admissionController.agentSidecarInjection.image.jmxEnabled`n: Define whether the Agent image should support JMX. To be used if the `Name` field does not correspond to a full image string.

`features.admissionController.agentSidecarInjection.image.name`n: Defines the Agent image name for the pod. You can provide this as: * `<NAME>` - Use `agent` for the Datadog Agent, `cluster-agent` for the Datadog Cluster Agent, or `dogstatsd` for DogStatsD. The full image string is derived from `global.registry`, `[key].image.tag`, and `[key].image.jmxEnabled`. * `<NAME>:<TAG>` - For example, `agent:latest`. The registry is derived from `global.registry`. `[key].image.tag` and `[key].image.jmxEnabled` are ignored. * `<REGISTRY>/<NAME>:<TAG>` - For example, `gcr.io/datadoghq/agent:latest`. If the full image string is specified   like this, then `global.registry`, `[key].image.tag`, and `[key].image.jmxEnabled` are ignored.

`features.admissionController.agentSidecarInjection.image.pullPolicy`n: The Kubernetes pull policy: Use `Always`, `Never`, or `IfNotPresent`.

`features.admissionController.agentSidecarInjection.image.pullSecrets`n: It is possible to specify Docker registry credentials. See https://kubernetes.io/docs/concepts/containers/images/#specifying-imagepullsecrets-on-a-pod

`features.admissionController.agentSidecarInjection.image.tag`n: Define the image tag to use. To be used if the `Name` field does not correspond to a full image string.

`features.admissionController.agentSidecarInjection.profiles`n: Define the sidecar configuration override. Only one profile is supported.

`features.admissionController.agentSidecarInjection.provider`n: Is used to add infrastructure provider-specific configurations to the Agent sidecar. Currently only "fargate" is supported. To use the feature in other environments (including local testing) omit the config. See also: https://docs.datadoghq.com/integrations/eks_fargate

`features.admissionController.agentSidecarInjection.registry`n: Overrides the default registry for the sidecar Agent.

`features.admissionController.agentSidecarInjection.selectors`n: Define the pod selector for sidecar injection. Only one rule is supported.

`features.admissionController.cwsInstrumentation.enabled`n: Enable the CWS Instrumentation admission controller endpoint. Default: false

`features.admissionController.cwsInstrumentation.mode`n: Defines the behavior of the CWS Instrumentation endpoint, and can be either "init_container" or "remote_copy". Default: "remote_copy"

`features.admissionController.enabled`n: Enables the Admission Controller. Default: true

`features.admissionController.failurePolicy`n: FailurePolicy determines how unrecognized and timeout errors are handled.

`features.admissionController.kubernetesAdmissionEvents.enabled`n: Enable the Kubernetes Admission Events feature. Default: false

`features.admissionController.mutateUnlabelled`n: MutateUnlabelled enables config injection without the need of pod label 'admission.datadoghq.com/enabled="true"'. Default: false

`features.admissionController.mutation.enabled`n: Enables the Admission Controller mutation webhook. Default: true

`features.admissionController.registry`n: Defines an image registry for the admission controller.

`features.admissionController.serviceName`n: ServiceName corresponds to the webhook service name.

`features.admissionController.validation.enabled`n: Enables the Admission Controller validation webhook. Default: true

`features.admissionController.webhookName`n: WebhookName is a custom name for the MutatingWebhookConfiguration. Default: "datadog-webhook"

`features.apm.enabled`n: Enables Application Performance Monitoring. Default: true

`features.apm.errorTrackingStandalone.enabled`n: Enables Error Tracking for backend services. Default: false

`features.apm.hostPortConfig.enabled`n: Enables host port configuration

`features.apm.hostPortConfig.hostPort`n: Port takes a port number (0 < x < 65536) to expose on the host. (Most containers do not need this.) If HostNetwork is enabled, this value must match the ContainerPort.

`features.apm.instrumentation.disabledNamespaces`n: DisabledNamespaces disables injecting the Datadog APM libraries into pods in specific namespaces.

`features.apm.instrumentation.enabled`n: Enables injecting the Datadog APM libraries into all pods in the cluster. Default: false

`features.apm.instrumentation.enabledNamespaces`n: EnabledNamespaces enables injecting the Datadog APM libraries into pods in specific namespaces.

`features.apm.instrumentation.injector.imageTag`n: Set the image tag to use for the APM Injector. (Requires Cluster Agent 7.57.0+)

`features.apm.instrumentation.languageDetection.enabled`n: Enables Language Detection to automatically detect languages of user workloads (beta). Requires SingleStepInstrumentation.Enabled to be true. Default: true

`features.apm.instrumentation.libVersions`n: LibVersions configures injection of specific tracing library versions with Single Step Instrumentation. <Library>: <Version> ex: "java": "v1.18.0"

`features.apm.instrumentation.targets`n: Is a list of targets to apply the auto instrumentation to. The first target that matches the pod will be used. If no target matches, the auto instrumentation will not be applied. (Requires Cluster Agent 7.64.0+)

`features.apm.unixDomainSocketConfig.enabled`n: Enables Unix Domain Socket. Default: true

`features.apm.unixDomainSocketConfig.path`n: Defines the socket path used when enabled.

`features.asm.iast.enabled`n: Enables Interactive Application Security Testing (IAST). Default: false

`features.asm.sca.enabled`n: Enables Software Composition Analysis (SCA). Default: false

`features.asm.threats.enabled`n: Enables ASM App & API Protection. Default: false

`features.autoscaling.workload.enabled`n: Enables the workload autoscaling product. Default: false

`features.clusterChecks.enabled`n: Enables Cluster Checks scheduling in the Cluster Agent. Default: true

`features.clusterChecks.useClusterChecksRunners`n: Enabled enables Cluster Checks Runners to run all Cluster Checks. Default: false

`features.controlPlaneMonitoring.enabled`n: Enables control plane monitoring checks in the cluster agent. Default: true

`features.cspm.checkInterval`n: CheckInterval defines the check interval.

`features.cspm.customBenchmarks.configData`n: ConfigData corresponds to the configuration file content.

`features.cspm.customBenchmarks.configMap.items`n: Maps a ConfigMap data `key` to a file `path` mount.

`features.cspm.customBenchmarks.configMap.name`n: Is the name of the ConfigMap.

`features.cspm.enabled`n: Enables Cloud Security Posture Management. Default: false

`features.cspm.hostBenchmarks.enabled`n: Enables host benchmarks. Default: true

`features.cws.customPolicies.configData`n: ConfigData corresponds to the configuration file content.

`features.cws.customPolicies.configMap.items`n: Maps a ConfigMap data `key` to a file `path` mount.

`features.cws.customPolicies.configMap.name`n: Is the name of the ConfigMap.

`features.cws.directSendFromSystemProbe`n: DirectSendFromSystemProbe configures CWS to send payloads directly from the system-probe, without using the security-agent. This is an experimental feature. Contact support before using. Default: false

`features.cws.enabled`n: Enables Cloud Workload Security. Default: false

`features.cws.network.enabled`n: Enables Cloud Workload Security Network detections. Default: true

`features.cws.remoteConfiguration.enabled`n: Enables Remote Configuration for Cloud Workload Security. Default: true

`features.cws.securityProfiles.enabled`n: Enables Security Profiles collection for Cloud Workload Security. Default: true

`features.cws.syscallMonitorEnabled`n: SyscallMonitorEnabled enables Syscall Monitoring (recommended for troubleshooting only). Default: false

`features.dogstatsd.hostPortConfig.enabled`n: Enables host port configuration

`features.dogstatsd.hostPortConfig.hostPort`n: Port takes a port number (0 < x < 65536) to expose on the host. (Most containers do not need this.) If HostNetwork is enabled, this value must match the ContainerPort.

`features.dogstatsd.mapperProfiles.configData`n: ConfigData corresponds to the configuration file content.

`features.dogstatsd.mapperProfiles.configMap.items`n: Maps a ConfigMap data `key` to a file `path` mount.

`features.dogstatsd.mapperProfiles.configMap.name`n: Is the name of the ConfigMap.

`features.dogstatsd.nonLocalTraffic`n: NonLocalTraffic enables non-local traffic for Dogstatsd. Default: true

`features.dogstatsd.originDetectionEnabled`n: OriginDetectionEnabled enables origin detection for container tagging. See also: https://docs.datadoghq.com/developers/dogstatsd/unix_socket/#using-origin-detection-for-container-tagging

`features.dogstatsd.tagCardinality`n: TagCardinality configures tag cardinality for the metrics collected using origin detection (`low`, `orchestrator` or `high`). See also: https://docs.datadoghq.com/getting_started/tagging/assigning_tags/?tab=containerizedenvironments#environment-variables Cardinality default: low

`features.dogstatsd.unixDomainSocketConfig.enabled`n: Enables Unix Domain Socket. Default: true

`features.dogstatsd.unixDomainSocketConfig.path`n: Defines the socket path used when enabled.

`features.ebpfCheck.enabled`n: Enables the eBPF check. Default: false

`features.eventCollection.collectKubernetesEvents`n: CollectKubernetesEvents enables Kubernetes event collection. Default: true

`features.eventCollection.collectedEventTypes`n: CollectedEventTypes defines the list of events to collect when UnbundleEvents is enabled. Default: [ {"kind":"Pod","reasons":["Failed","BackOff","Unhealthy","FailedScheduling","FailedMount","FailedAttachVolume"]}, {"kind":"Node","reasons":["TerminatingEvictedPod","NodeNotReady","Rebooted","HostPortConflict"]}, {"kind":"CronJob","reasons":["SawCompletedJob"]} ]

`features.eventCollection.unbundleEvents`n: UnbundleEvents enables collection of Kubernetes events as individual events. Default: false

`features.externalMetricsServer.enabled`n: Enables the External Metrics Server. Default: false

`features.externalMetricsServer.endpoint.credentials.apiKey`n: APIKey configures your Datadog API key. See also: https://app.datadoghq.com/account/settings#agent/kubernetes

`features.externalMetricsServer.endpoint.credentials.apiSecret.keyName`n: KeyName is the key of the secret to use.

`features.externalMetricsServer.endpoint.credentials.apiSecret.secretName`n: SecretName is the name of the secret.

`features.externalMetricsServer.endpoint.credentials.appKey`n: AppKey configures your Datadog application key. If you are using features.externalMetricsServer.enabled = true, you must set a Datadog application key for read access to your metrics.

`features.externalMetricsServer.endpoint.credentials.appSecret.keyName`n: KeyName is the key of the secret to use.

`features.externalMetricsServer.endpoint.credentials.appSecret.secretName`n: SecretName is the name of the secret.

`features.externalMetricsServer.endpoint.url`n: URL defines the endpoint URL.

`features.externalMetricsServer.port`n: Specifies the metricsProvider External Metrics Server service port. Default: 8443

`features.externalMetricsServer.registerAPIService`n: RegisterAPIService registers the External Metrics endpoint as an APIService Default: true

`features.externalMetricsServer.useDatadogMetrics`n: UseDatadogMetrics enables usage of the DatadogMetrics CRD (allowing one to scale on arbitrary Datadog metric queries). Default: true

`features.externalMetricsServer.wpaController`n: WPAController enables the informer and controller of the Watermark Pod Autoscaler. NOTE: The Watermark Pod Autoscaler controller needs to be installed. See also: https://github.com/DataDog/watermarkpodautoscaler. Default: false

`features.gpu.enabled`n: Enables GPU monitoring core check. Default: false

`features.gpu.patchCgroupPermissions`n: PatchCgroupPermissions enables the patch of cgroup permissions for GPU monitoring, in case the container runtime is not properly configured and the Agent containers lose access to GPU devices. Default: false

`features.gpu.privilegedMode`n: PrivilegedMode enables GPU Probe module in System Probe. Default: false

`features.gpu.requiredRuntimeClassName`n: PodRuntimeClassName specifies the runtime class name required for the GPU monitoring feature. If the value is an empty string, the runtime class is not set. Default: nvidia

`features.helmCheck.collectEvents`n: CollectEvents set to `true` enables event collection in the Helm check (Requires Agent 7.36.0+ and Cluster Agent 1.20.0+) Default: false

`features.helmCheck.enabled`n: Enables the Helm check. Default: false

`features.helmCheck.valuesAsTags`n: ValuesAsTags collects Helm values from a release and uses them as tags (Requires Agent and Cluster Agent 7.40.0+). Default: {}

`features.kubeStateMetricsCore.collectCrMetrics`n: `CollectCrMetrics` defines custom resources for the kube-state-metrics core check to collect.  The datadog agent uses the same logic as upstream `kube-state-metrics`. So is its configuration. The exact structure and existing fields of each item in this list can be found in: https://github.com/kubernetes/kube-state-metrics/blob/main/docs/metrics/extend/customresourcestate-metrics.md

`features.kubeStateMetricsCore.conf.configData`n: ConfigData corresponds to the configuration file content.

`features.kubeStateMetricsCore.conf.configMap.items`n: Maps a ConfigMap data `key` to a file `path` mount.

`features.kubeStateMetricsCore.conf.configMap.name`n: Is the name of the ConfigMap.

`features.kubeStateMetricsCore.enabled`n: Enables Kube State Metrics Core. Default: true

`features.liveContainerCollection.enabled`n: Enables container collection for the Live Container View. Default: true

`features.liveProcessCollection.enabled`n: Enables Process monitoring. Default: false

`features.liveProcessCollection.scrubProcessArguments`n: ScrubProcessArguments enables scrubbing of sensitive data in process command-lines (passwords, tokens, etc. ). Default: true

`features.liveProcessCollection.stripProcessArguments`n: StripProcessArguments enables stripping of all process arguments. Default: false

`features.logCollection.autoMultiLineDetection`n: AutoMultiLineDetection allows the Agent to detect and aggregate common multi-line logs automatically. See also: https://docs.datadoghq.com/agent/logs/auto_multiline_detection/

`features.logCollection.containerCollectAll`n: ContainerCollectAll enables Log collection from all containers. Default: false

`features.logCollection.containerCollectUsingFiles`n: ContainerCollectUsingFiles enables log collection from files in `/var/log/pods instead` of using the container runtime API. Collecting logs from files is usually the most efficient way of collecting logs. See also: https://docs.datadoghq.com/agent/basic_agent_usage/kubernetes/#log-collection-setup Default: true

`features.logCollection.containerLogsPath`n: ContainerLogsPath allows log collection from the container log path. Set to a different path if you are not using the Docker runtime. See also: https://docs.datadoghq.com/agent/kubernetes/daemonset_setup/?tab=k8sfile#create-manifest Default: `/var/lib/docker/containers`

`features.logCollection.containerSymlinksPath`n: ContainerSymlinksPath allows log collection to use symbolic links in this directory to validate container ID -> pod. Default: `/var/log/containers`

`features.logCollection.enabled`n: Enables Log collection. Default: false

`features.logCollection.openFilesLimit`n: OpenFilesLimit sets the maximum number of log files that the Datadog Agent tails. Increasing this limit can increase resource consumption of the Agent. See also: https://docs.datadoghq.com/agent/basic_agent_usage/kubernetes/#log-collection-setup Default: 100

`features.logCollection.podLogsPath`n: PodLogsPath allows log collection from a pod log path. Default: `/var/log/pods`

`features.logCollection.tempStoragePath`n: TempStoragePath (always mounted from the host) is used by the Agent to store information about processed log files. If the Agent is restarted, it starts tailing the log files immediately. Default: `/var/lib/datadog-agent/logs`

`features.npm.collectDNSStats`n: CollectDNSStats enables DNS stat collection. Default: false

`features.npm.enableConntrack`n: EnableConntrack enables the system-probe agent to connect to the netlink/conntrack subsystem to add NAT information to connection data. See also: http://conntrack-tools.netfilter.org/ Default: false

`features.npm.enabled`n: Enables Network Performance Monitoring. Default: false

`features.oomKill.enabled`n: Enables the OOMKill eBPF-based check. Default: false

`features.orchestratorExplorer.conf.configData`n: ConfigData corresponds to the configuration file content.

`features.orchestratorExplorer.conf.configMap.items`n: Maps a ConfigMap data `key` to a file `path` mount.

`features.orchestratorExplorer.conf.configMap.name`n: Is the name of the ConfigMap.

`features.orchestratorExplorer.customResources`n: `CustomResources` defines custom resources for the orchestrator explorer to collect. Each item should follow the convention `group/version/kind`. For example, `datadoghq.com/v1alpha1/datadogmetrics`.

`features.orchestratorExplorer.ddUrl`n: Override the API endpoint for the Orchestrator Explorer. URL Default: "https://orchestrator.datadoghq.com".

`features.orchestratorExplorer.enabled`n: Enables the Orchestrator Explorer. Default: true

`features.orchestratorExplorer.extraTags`n: Additional tags to associate with the collected data in the form of `a b c`. This is a Cluster Agent option distinct from DD_TAGS that is used in the Orchestrator Explorer.

`features.orchestratorExplorer.scrubContainers`n: ScrubContainers enables scrubbing of sensitive container data (passwords, tokens, etc. ). Default: true

`features.otelCollector.conf.configData`n: ConfigData corresponds to the configuration file content.

`features.otelCollector.conf.configMap.items`n: Maps a ConfigMap data `key` to a file `path` mount.

`features.otelCollector.conf.configMap.name`n: Is the name of the ConfigMap.

`features.otelCollector.coreConfig.enabled`n: Marks otelcollector as enabled in core agent.

`features.otelCollector.coreConfig.extensionTimeout`n: Extension URL provides the timout of the ddflareextension to the core agent.

`features.otelCollector.coreConfig.extensionURL`n: Extension URL provides the URL of the ddflareextension to the core agent.

`features.otelCollector.enabled`n: Enables the OTel Agent. Default: false

`features.otelCollector.ports`n: Contains the ports for the otel-agent. Defaults: otel-grpc:4317 / otel-http:4318. Note: setting 4317 or 4318 manually is *only* supported if name match default names (otel-grpc, otel-http). If not, this will lead to a port conflict. This limitation will be lifted once annotations support is removed.

`features.otlp.receiver.protocols.grpc.enabled`n: Enable the OTLP/gRPC endpoint. Host port is enabled by default and can be disabled.

`features.otlp.receiver.protocols.grpc.endpoint`n: For OTLP/gRPC. gRPC supports several naming schemes: https://github.com/grpc/grpc/blob/master/doc/naming.md The Datadog Operator supports only 'host:port' (usually `0.0.0.0:port`). Default: `0.0.0.0:4317`.

`features.otlp.receiver.protocols.grpc.hostPortConfig.enabled`n: Enables host port configuration

`features.otlp.receiver.protocols.grpc.hostPortConfig.hostPort`n: Port takes a port number (0 < x < 65536) to expose on the host. (Most containers do not need this.) If HostNetwork is enabled, this value must match the ContainerPort.

`features.otlp.receiver.protocols.http.enabled`n: Enable the OTLP/HTTP endpoint. Host port is enabled by default and can be disabled.

`features.otlp.receiver.protocols.http.endpoint`n: For OTLP/HTTP. Default: '0.0.0.0:4318'.

`features.otlp.receiver.protocols.http.hostPortConfig.enabled`n: Enables host port configuration

`features.otlp.receiver.protocols.http.hostPortConfig.hostPort`n: Port takes a port number (0 < x < 65536) to expose on the host. (Most containers do not need this.) If HostNetwork is enabled, this value must match the ContainerPort.

`features.processDiscovery.enabled`n: Enables the Process Discovery check in the Agent. Default: true

`features.prometheusScrape.additionalConfigs`n: AdditionalConfigs allows adding advanced Prometheus check configurations with custom discovery rules.

`features.prometheusScrape.enableServiceEndpoints`n: EnableServiceEndpoints enables generating dedicated checks for service endpoints. Default: false

`features.prometheusScrape.enabled`n: Enable autodiscovery of pods and services exposing Prometheus metrics. Default: false

`features.prometheusScrape.version`n: Specifies the version of the OpenMetrics check. Default: 2

`features.remoteConfiguration.enabled`n: Enable this option to activate Remote Configuration. Default: true

`features.sbom.containerImage.analyzers`n: To use for SBOM collection.

`features.sbom.containerImage.enabled`n: Enable this option to activate SBOM collection. Default: false

`features.sbom.containerImage.overlayFSDirectScan`n: Enable this option to enable experimental overlayFS direct scan. Default: false

`features.sbom.containerImage.uncompressedLayersSupport`n: Enable this option to enable support for uncompressed layers. Default: false

`features.sbom.enabled`n: Enable this option to activate SBOM collection. Default: false

`features.sbom.host.analyzers`n: To use for SBOM collection.

`features.sbom.host.enabled`n: Enable this option to activate SBOM collection. Default: false

`features.serviceDiscovery.enabled`n: Enables the service discovery check. Default: false

`features.serviceDiscovery.networkStats.enabled`n: Enables the Service Discovery Network Stats feature. Default: true

`features.tcpQueueLength.enabled`n: Enables the TCP queue length eBPF-based check. Default: false

`features.usm.enabled`n: Enables Universal Service Monitoring. Default: false

`global.checksTagCardinality`n: ChecksTagCardinality configures tag cardinality for the metrics collected by integrations (`low`, `orchestrator` or `high`). See also: https://docs.datadoghq.com/getting_started/tagging/assigning_tags/?tab=containerizedenvironments#tags-cardinality. Not set by default to avoid overriding existing DD_CHECKS_TAG_CARDINALITY configurations, the default value in the Agent is low. Ref: https://github.com/DataDog/datadog-agent/blob/856cf4a66142ce91fd4f8a278149436eb971184a/pkg/config/setup/config.go#L625.

`global.clusterAgentToken`n: ClusterAgentToken is the token for communication between the NodeAgent and ClusterAgent.

`global.clusterAgentTokenSecret.keyName`n: KeyName is the key of the secret to use.

`global.clusterAgentTokenSecret.secretName`n: SecretName is the name of the secret.

`global.clusterName`n: ClusterName sets a unique cluster name for the deployment to easily scope monitoring data in the Datadog app.

`global.containerStrategy`n: ContainerStrategy determines whether agents run in a single or multiple containers. Default: 'optimized'

`global.credentials.apiKey`n: APIKey configures your Datadog API key. See also: https://app.datadoghq.com/account/settings#agent/kubernetes

`global.credentials.apiSecret.keyName`n: KeyName is the key of the secret to use.

`global.credentials.apiSecret.secretName`n: SecretName is the name of the secret.

`global.credentials.appKey`n: AppKey configures your Datadog application key. If you are using features.externalMetricsServer.enabled = true, you must set a Datadog application key for read access to your metrics.

`global.credentials.appSecret.keyName`n: KeyName is the key of the secret to use.

`global.credentials.appSecret.secretName`n: SecretName is the name of the secret.

`global.criSocketPath`n: Path to the container runtime socket (if different from Docker).

`global.csi.enabled`n: Enables the usage of CSI driver in Datadog Agent. Requires installation of Datadog CSI Driver https://github.com/DataDog/helm-charts/tree/main/charts/datadog-csi-driver Default: false

`global.disableNonResourceRules`n: Set DisableNonResourceRules to exclude NonResourceURLs from default ClusterRoles. Required 'true' for Google Cloud Marketplace.

`global.dockerSocketPath`n: Path to the docker runtime socket.

`global.endpoint.credentials.apiKey`n: APIKey configures your Datadog API key. See also: https://app.datadoghq.com/account/settings#agent/kubernetes

`global.endpoint.credentials.apiSecret.keyName`n: KeyName is the key of the secret to use.

`global.endpoint.credentials.apiSecret.secretName`n: SecretName is the name of the secret.

`global.endpoint.credentials.appKey`n: AppKey configures your Datadog application key. If you are using features.externalMetricsServer.enabled = true, you must set a Datadog application key for read access to your metrics.

`global.endpoint.credentials.appSecret.keyName`n: KeyName is the key of the secret to use.

`global.endpoint.credentials.appSecret.secretName`n: SecretName is the name of the secret.

`global.endpoint.url`n: URL defines the endpoint URL.

`global.env`n: Contains a list of environment variables that are set for all Agents.

`global.fips.customFIPSConfig.configData`n: ConfigData corresponds to the configuration file content.

`global.fips.customFIPSConfig.configMap.items`n: Maps a ConfigMap data `key` to a file `path` mount.

`global.fips.customFIPSConfig.configMap.name`n: Is the name of the ConfigMap.

`global.fips.enabled`n: Enable FIPS sidecar.

`global.fips.image.jmxEnabled`n: Define whether the Agent image should support JMX. To be used if the `Name` field does not correspond to a full image string.

`global.fips.image.name`n: Defines the Agent image name for the pod. You can provide this as: * `<NAME>` - Use `agent` for the Datadog Agent, `cluster-agent` for the Datadog Cluster Agent, or `dogstatsd` for DogStatsD. The full image string is derived from `global.registry`, `[key].image.tag`, and `[key].image.jmxEnabled`. * `<NAME>:<TAG>` - For example, `agent:latest`. The registry is derived from `global.registry`. `[key].image.tag` and `[key].image.jmxEnabled` are ignored. * `<REGISTRY>/<NAME>:<TAG>` - For example, `gcr.io/datadoghq/agent:latest`. If the full image string is specified   like this, then `global.registry`, `[key].image.tag`, and `[key].image.jmxEnabled` are ignored.

`global.fips.image.pullPolicy`n: The Kubernetes pull policy for the FIPS sidecar image. Values: Always, Never, IfNotPresent.

`global.fips.image.pullSecrets`n: Specifies Docker registry credentials (https://kubernetes.io/docs/concepts/containers/images/#specifying-imagepullsecrets-on-a-pod) for the FIPS sidecar.

`global.fips.image.tag`n: Defines the tag of the FIPS sidecar image. This parameter is used if global.fips.image.name does not correspond to a full image string.

`global.fips.localAddress`n: The local IP address of the FIPS sidecar. Default: 127.0.0.1.

`global.fips.port`n: Specifies which port is used by the containers to communicate to the FIPS sidecar. Default: 9803

`global.fips.portRange`n: The number of ports used by the containers to communicate to the FIPS sidecar. Default: 15

`global.fips.resources.claims`n: Lists the names of resources, defined in spec.resourceClaims, that are used by this container.  This is an alpha field and requires enabling the DynamicResourceAllocation feature gate.  This field is immutable. It can only be set for containers.

`global.fips.resources.limits`n: Resource limits for the FIPS sidecar. See https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/#resource-requests-and-limits-of-pod-and-container .

`global.fips.resources.requests`n: Resource requests for the FIPS sidecar. If undefined, defaults to global.fips.resources.limits (if set), then to an implementation-defined value. See https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/#resource-requests-and-limits-of-pod-and-container .

`global.fips.useHTTPS`n: If true, enables HTTPS on the FIPS sidecar. Default: false

`global.kubelet.agentCAPath`n: AgentCAPath is the container path where the kubelet CA certificate is stored. Default: '/var/run/host-kubelet-ca.crt' if hostCAPath is set, else '/var/run/secrets/kubernetes.io/serviceaccount/ca.crt'

`global.kubelet.host.configMapKeyRef.key`n: The key to select.

`global.kubelet.host.configMapKeyRef.name`n: Of the referent. This field is effectively required, but due to backwards compatibility is allowed to be empty. Instances of this type with an empty value here are almost certainly wrong. More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names

`global.kubelet.host.configMapKeyRef.optional`n: Specify whether the ConfigMap or its key must be defined

`global.kubelet.host.fieldRef.apiVersion`n: Version of the schema the FieldPath is written in terms of, defaults to "v1".

`global.kubelet.host.fieldRef.fieldPath`n: Path of the field to select in the specified API version.

`global.kubelet.host.resourceFieldRef.containerName`n: Container name: required for volumes, optional for env vars

`global.kubelet.host.resourceFieldRef.divisor`n: Specifies the output format of the exposed resources, defaults to "1"

`global.kubelet.host.resourceFieldRef.resource`n: Required: resource to select

`global.kubelet.host.secretKeyRef.key`n: The key of the secret to select from.  Must be a valid secret key.

`global.kubelet.host.secretKeyRef.name`n: Of the referent. This field is effectively required, but due to backwards compatibility is allowed to be empty. Instances of this type with an empty value here are almost certainly wrong. More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names

`global.kubelet.host.secretKeyRef.optional`n: Specify whether the Secret or its key must be defined

`global.kubelet.hostCAPath`n: HostCAPath is the host path where the kubelet CA certificate is stored.

`global.kubelet.podResourcesSocketPath`n: PodResourcesSocketPath is the host path where the pod resources socket is stored. Default: `/var/lib/kubelet/pod-resources/`

`global.kubelet.tlsVerify`n: TLSVerify toggles kubelet TLS verification. Default: true

`global.kubernetesResourcesAnnotationsAsTags`n: Provide a mapping of Kubernetes Resource Groups to annotations mapping to Datadog Tags. <KUBERNETES_RESOURCE_GROUP>: 		<KUBERNETES_ANNOTATION>: <DATADOG_TAG_KEY> KUBERNETES_RESOURCE_GROUP should be in the form `{resource}.{group}` or `{resource}` (example: deployments.apps, pods)

`global.kubernetesResourcesLabelsAsTags`n: Provide a mapping of Kubernetes Resource Groups to labels mapping to Datadog Tags. <KUBERNETES_RESOURCE_GROUP>: 		<KUBERNETES_LABEL>: <DATADOG_TAG_KEY> KUBERNETES_RESOURCE_GROUP should be in the form `{resource}.{group}` or `{resource}` (example: deployments.apps, pods)

`global.localService.forceEnableLocalService`n: ForceEnableLocalService forces the creation of the internal traffic policy service to target the agent running on the local node. This parameter only applies to Kubernetes 1.21, where the feature is in alpha and is disabled by default. (On Kubernetes 1.22+, the feature entered beta and the internal traffic service is created by default, so this parameter is ignored.) Default: false

`global.localService.nameOverride`n: NameOverride defines the name of the internal traffic service to target the agent running on the local node.

`global.logLevel`n: LogLevel sets logging verbosity. This can be overridden by container. Valid log levels are: trace, debug, info, warn, error, critical, and off. Default: 'info'

`global.namespaceAnnotationsAsTags`n: Provide a mapping of Kubernetes Namespace Annotations to Datadog Tags. <KUBERNETES_LABEL>: <DATADOG_TAG_KEY>

`global.namespaceLabelsAsTags`n: Provide a mapping of Kubernetes Namespace Labels to Datadog Tags. <KUBERNETES_NAMESPACE_LABEL>: <DATADOG_TAG_KEY>

`global.networkPolicy.create`n: Defines whether to create a NetworkPolicy for the current deployment.

`global.networkPolicy.dnsSelectorEndpoints`n: DNSSelectorEndpoints defines the cilium selector of the DNS server entity.

`global.networkPolicy.flavor`n: Defines Which network policy to use.

`global.nodeLabelsAsTags`n: Provide a mapping of Kubernetes Node Labels to Datadog Tags. <KUBERNETES_NODE_LABEL>: <DATADOG_TAG_KEY>

`global.originDetectionUnified.enabled`n: Enables unified mechanism for origin detection. Default: false

`global.podAnnotationsAsTags`n: Provide a mapping of Kubernetes Annotations to Datadog Tags. <KUBERNETES_ANNOTATIONS>: <DATADOG_TAG_KEY>

`global.podLabelsAsTags`n: Provide a mapping of Kubernetes Labels to Datadog Tags. <KUBERNETES_LABEL>: <DATADOG_TAG_KEY>

`global.registry`n: Is the image registry to use for all Agent images. Use 'public.ecr.aws/datadog' for AWS ECR. Use 'datadoghq.azurecr.io' for Azure Container Registry. Use 'gcr.io/datadoghq' for Google Container Registry. Use 'eu.gcr.io/datadoghq' for Google Container Registry in the EU region. Use 'asia.gcr.io/datadoghq' for Google Container Registry in the Asia region. Use 'docker.io/datadog' for DockerHub. Default: 'gcr.io/datadoghq'

`global.runProcessChecksInCoreAgent`n: Configure whether the Process Agent or core Agent collects process and/or container information (Linux only). If no other checks are running, the Process Agent container will not initialize. (Requires Agent 7.60.0+) Default: 'true' Deprecated: Functionality now handled automatically. Use env var `DD_PROCESS_CONFIG_RUN_IN_CORE_AGENT_ENABLED` to override.

`global.secretBackend.args`n: List of arguments to pass to the command (space-separated strings).

`global.secretBackend.command`n: The secret backend command to use. Datadog provides a pre-defined binary `/readsecret_multiple_providers.sh`. Read more about `/readsecret_multiple_providers.sh` at https://docs.datadoghq.com/agent/configuration/secrets-management/?tab=linux#script-for-reading-from-multiple-secret-providers.

`global.secretBackend.enableGlobalPermissions`n: Whether to create a global permission allowing Datadog agents to read all Kubernetes secrets. Default: `false`.

`global.secretBackend.refreshInterval`n: The refresh interval for secrets (0 disables refreshing). Default: `0`.

`global.secretBackend.roles`n: For Datadog to read the specified secrets, replacing `enableGlobalPermissions`. They are defined as a list of namespace/secrets. Each defined namespace needs to be present in the DatadogAgent controller using `WATCH_NAMESPACE` or `DD_AGENT_WATCH_NAMESPACE`. See also: https://github.com/DataDog/datadog-operator/blob/main/docs/secret_management.md#how-to-deploy-the-agent-components-using-the-secret-backend-feature-with-datadogagent.

`global.secretBackend.timeout`n: The command timeout in seconds. Default: `30`.

`global.site`n: Is the Datadog intake site Agent data are sent to. Set to 'datadoghq.com' to send data to the US1 site (default). Set to 'datadoghq.eu' to send data to the EU site. Set to 'us3.datadoghq.com' to send data to the US3 site. Set to 'us5.datadoghq.com' to send data to the US5 site. Set to 'ddog-gov.com' to send data to the US1-FED site. Set to 'ap1.datadoghq.com' to send data to the AP1 site. Default: 'datadoghq.com'

`global.tags`n: Contains a list of tags to attach to every metric, event and service check collected. Learn more about tagging: https://docs.datadoghq.com/tagging/

`global.useFIPSAgent`n: UseFIPSAgent enables the FIPS flavor of the Agent. If 'true', the FIPS proxy will always be disabled. Default: 'false'

`override`n: The default configurations of the agents

For a complete list of parameters, see the [Operator configuration spec][9].

[1]: https://github.com/DataDog/datadog-operator/blob/main/docs/configuration.v2alpha1.md
[2]: https://github.com/DataDog/datadog-operator/
[3]: https://github.com/DataDog/datadog-operator/blob/main/examples/datadogagent/datadog-agent-all.yaml
[4]: https://github.com/DataDog/datadog-operator/blob/main/examples/datadogagent/datadog-agent-with-logs-apm.yaml
[5]: https://github.com/DataDog/datadog-operator/blob/main/examples/datadogagent/datadog-agent-with-apm-hostport.yaml
[6]: https://github.com/DataDog/datadog-operator/blob/main/examples/datadogagent/datadog-agent-with-clusteragent.yaml
[7]: https://github.com/DataDog/datadog-operator/blob/main/examples/datadogagent/datadog-agent-with-tolerations.yaml
[8]: https://github.com/DataDog/datadog-operator/blob/main/docs/configuration.v2alpha1.md#all-configuration-options
[9]: https://github.com/DataDog/datadog-operator/blob/main/docs/configuration.v2alpha1.md#override
