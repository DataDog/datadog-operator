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
  agent:
    image:
      name: "gcr.io/datadoghq/agent:latest"
```

| Parameter | Description |
| --------- | ----------- |
| agent.additionalAnnotations | AdditionalAnnotations provide annotations that will be added to the Agent Pods. |
| agent.additionalLabels | AdditionalLabels provide labels that will be added to the cluster checks runner Pods. |
| agent.apm.enabled | Enable this to enable APM and tracing, on port 8126 ref: https://github.com/DataDog/docker-dd-agent#tracing-from-the-host |
| agent.apm.env | The Datadog Agent supports many environment variables Ref: https://docs.datadoghq.com/agent/docker/?tab=standard#environment-variables |
| agent.apm.hostPort | Number of port to expose on the host. If specified, this must be a valid port number, 0 < x < 65536. If HostNetwork is specified, this must match ContainerPort. Most containers do not need this. |
| agent.apm.resources.limits | Limits describes the maximum amount of compute resources allowed. More info: https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/ |
| agent.apm.resources.requests | Requests describes the minimum amount of compute resources required. If Requests is omitted for a container, it defaults to Limits if that is explicitly specified, otherwise to an implementation-defined value. More info: https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/ |
| agent.apm.unixDomainSocket.enabled | Enable APM over Unix Domain Socket ref: https://docs.datadoghq.com/agent/kubernetes/apm/?tab=helm#agent-environment-variables |
| agent.apm.unixDomainSocket.hostFilepath | Define the host APM socket filepath used when APM over Unix Domain Socket is enabled (default value: /var/run/datadog/apm.sock) ref: https://docs.datadoghq.com/agent/kubernetes/apm/?tab=helm#agent-environment-variables |
| agent.apm.volumeMounts | Specify additional volume mounts in the APM Agent container |
| agent.config.checksd.configMapName | ConfigMapName name of a ConfigMap used to mount a directory |
| agent.config.collectEvents | Enables this to start event collection from the Kubernetes API ref: https://docs.datadoghq.com/agent/kubernetes/event_collection/ |
| agent.config.confd.configMapName | ConfigMapName name of a ConfigMap used to mount a directory |
| agent.config.criSocket.criSocketPath | Path to the container runtime socket (if different from Docker) This is supported starting from agent 6.6.0 |
| agent.config.criSocket.dockerSocketPath | Path to the docker runtime socket |
| agent.config.ddUrl | The host of the Datadog intake server to send Agent data to, only set this option if you need the Agent to send data to a custom URL. Overrides the site setting defined in "site". |
| agent.config.dogstatsd.dogstatsdOriginDetection | Enable origin detection for container tagging ref: https://docs.datadoghq.com/developers/dogstatsd/unix_socket/#using-origin-detection-for-container-tagging |
| agent.config.dogstatsd.mapperProfiles.configData | ConfigData corresponds to the configuration file content |
| agent.config.dogstatsd.mapperProfiles.configMap.fileKey | FileKey corresponds to the key used in the ConfigMap.Data to store the configuration file content |
| agent.config.dogstatsd.mapperProfiles.configMap.name | Name the ConfigMap name |
| agent.config.dogstatsd.unixDomainSocket.enabled | Enable APM over Unix Domain Socket ref: https://docs.datadoghq.com/developers/dogstatsd/unix_socket/ |
| agent.config.dogstatsd.unixDomainSocket.hostFilepath | Define the host APM socket filepath used when APM over Unix Domain Socket is enabled (default value: /var/run/datadog/statsd.sock) ref: https://docs.datadoghq.com/developers/dogstatsd/unix_socket/ |
| agent.config.env | The Datadog Agent supports many environment variables Ref: https://docs.datadoghq.com/agent/docker/?tab=standard#environment-variables |
| agent.config.hostPort | Number of port to expose on the host. If specified, this must be a valid port number, 0 < x < 65536. If HostNetwork is specified, this must match ContainerPort. Most containers do not need this. |
| agent.config.leaderElection | Enables leader election mechanism for event collection. |
| agent.config.logLevel | Set logging verbosity, valid log levels are: trace, debug, info, warn, error, critical, and off |
| agent.config.podAnnotationsAsTags | Provide a mapping of Kubernetes Annotations to Datadog Tags. <KUBERNETES_ANNOTATIONS>: <DATADOG_TAG_KEY> |
| agent.config.podLabelsAsTags | Provide a mapping of Kubernetes Labels to Datadog Tags. <KUBERNETES_LABEL>: <DATADOG_TAG_KEY> |
| agent.config.resources.limits | Limits describes the maximum amount of compute resources allowed. More info: https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/ |
| agent.config.resources.requests | Requests describes the minimum amount of compute resources required. If Requests is omitted for a container, it defaults to Limits if that is explicitly specified, otherwise to an implementation-defined value. More info: https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/ |
| agent.config.securityContext.fsGroup | A special supplemental group that applies to all containers in a pod. Some volume types allow the Kubelet to change the ownership of that volume to be owned by the pod:  1. The owning GID will be the FSGroup 2. The setgid bit is set (new files created in the volume will be owned by FSGroup) 3. The permission bits are OR'd with rw-rw----  If unset, the Kubelet will not modify the ownership and permissions of any volume. |
| agent.config.securityContext.fsGroupChangePolicy | fsGroupChangePolicy defines behavior of changing ownership and permission of the volume before being exposed inside Pod. This field will only apply to volume types which support fsGroup based ownership(and permissions). It will have no effect on ephemeral volume types such as: secret, configmaps and emptydir. Valid values are "OnRootMismatch" and "Always". If not specified, "Always" is used. |
| agent.config.securityContext.runAsGroup | The GID to run the entrypoint of the container process. Uses runtime default if unset. May also be set in SecurityContext.  If set in both SecurityContext and PodSecurityContext, the value specified in SecurityContext takes precedence for that container. |
| agent.config.securityContext.runAsNonRoot | Indicates that the container must run as a non-root user. If true, the Kubelet will validate the image at runtime to ensure that it does not run as UID 0 (root) and fail to start the container if it does. If unset or false, no such validation will be performed. May also be set in SecurityContext.  If set in both SecurityContext and PodSecurityContext, the value specified in SecurityContext takes precedence. |
| agent.config.securityContext.runAsUser | The UID to run the entrypoint of the container process. Defaults to user specified in image metadata if unspecified. May also be set in SecurityContext.  If set in both SecurityContext and PodSecurityContext, the value specified in SecurityContext takes precedence for that container. |
| agent.config.securityContext.seLinuxOptions.level | Level is SELinux level label that applies to the container. |
| agent.config.securityContext.seLinuxOptions.role | Role is a SELinux role label that applies to the container. |
| agent.config.securityContext.seLinuxOptions.type | Type is a SELinux type label that applies to the container. |
| agent.config.securityContext.seLinuxOptions.user | User is a SELinux user label that applies to the container. |
| agent.config.securityContext.seccompProfile.localhostProfile | localhostProfile indicates a profile defined in a file on the node should be used. The profile must be preconfigured on the node to work. Must be a descending path, relative to the kubelet's configured seccomp profile location. Must only be set if type is "Localhost". |
| agent.config.securityContext.seccompProfile.type | type indicates which kind of seccomp profile will be applied. Valid options are:  Localhost - a profile defined in a file on the node should be used. RuntimeDefault - the container runtime default profile should be used. Unconfined - no profile should be applied. |
| agent.config.securityContext.supplementalGroups | A list of groups applied to the first process run in each container, in addition to the container's primary GID.  If unspecified, no groups will be added to any container. |
| agent.config.securityContext.sysctls | Sysctls hold a list of namespaced sysctls used for the pod. Pods with unsupported sysctls (by the container runtime) might fail to launch. |
| agent.config.securityContext.windowsOptions.gmsaCredentialSpec | GMSACredentialSpec is where the GMSA admission webhook (https://github.com/kubernetes-sigs/windows-gmsa) inlines the contents of the GMSA credential spec named by the GMSACredentialSpecName field. |
| agent.config.securityContext.windowsOptions.gmsaCredentialSpecName | GMSACredentialSpecName is the name of the GMSA credential spec to use. |
| agent.config.securityContext.windowsOptions.runAsUserName | The UserName in Windows to run the entrypoint of the container process. Defaults to the user specified in image metadata if unspecified. May also be set in PodSecurityContext. If set in both SecurityContext and PodSecurityContext, the value specified in SecurityContext takes precedence. |
| agent.config.tags | List of tags to attach to every metric, event and service check collected by this Agent. Learn more about tagging: https://docs.datadoghq.com/tagging/ |
| agent.config.tolerations | If specified, the Agent pod's tolerations. |
| agent.config.volumeMounts | Specify additional volume mounts in the Datadog Agent container |
| agent.config.volumes | Specify additional volumes in the Datadog Agent container |
| agent.customConfig.configData | ConfigData corresponds to the configuration file content |
| agent.customConfig.configMap.fileKey | FileKey corresponds to the key used in the ConfigMap.Data to store the configuration file content |
| agent.customConfig.configMap.name | Name the ConfigMap name |
| agent.daemonsetName | Name of the Daemonset to create or migrate from |
| agent.deploymentStrategy.canary.autoFail.enabled |  |
| agent.deploymentStrategy.canary.autoFail.maxRestarts | MaxRestarts defines the number of tolerable (per pod) Canary pod restarts after which the Canary deployment is autofailed. |
| agent.deploymentStrategy.canary.autoFail.maxRestartsDuration | MaxRestartsDuration defines the maximum duration of tolerable Canary pod restarts after which the Canary deployment is autofailed. |
| agent.deploymentStrategy.canary.autoPause.enabled |  |
| agent.deploymentStrategy.canary.autoPause.maxRestarts | MaxRestarts defines the number of tolerable (per pod) Canary pod restarts after which the Canary deployment is autopaused. |
| agent.deploymentStrategy.canary.autoPause.maxSlowStartDuration | MaxSlowStartDuration defines the maximum slow start duration for a pod (stuck in Creating state) after which the. Canary deployment is autopaused |
| agent.deploymentStrategy.canary.duration |  |
| agent.deploymentStrategy.canary.noRestartsDuration | NoRestartsDuration defines min duration since last restart to end the canary phase. |
| agent.deploymentStrategy.canary.nodeAntiAffinityKeys |  |
| agent.deploymentStrategy.canary.nodeSelector.matchExpressions | matchExpressions is a list of label selector requirements. The requirements are ANDed. |
| agent.deploymentStrategy.canary.nodeSelector.matchLabels | matchLabels is a map of {key,value} pairs. A single {key,value} in the matchLabels map is equivalent to an element of matchExpressions, whose key field is "key", the operator is "In", and the values array contains only "value". The requirements are ANDed. |
| agent.deploymentStrategy.canary.replicas |  |
| agent.deploymentStrategy.reconcileFrequency | The reconcile frequency of the ExtendDaemonSet |
| agent.deploymentStrategy.rollingUpdate.maxParallelPodCreation | The maxium number of pods created in parallel. Default value is 250. |
| agent.deploymentStrategy.rollingUpdate.maxPodSchedulerFailure | MaxPodSchedulerFailure the maxinum number of not scheduled on its Node due to a scheduler failure: resource constraints. Value can be an absolute number (ex: 5) or a percentage of total number of DaemonSet pods at the start of the update (ex: 10%). Absolute |
| agent.deploymentStrategy.rollingUpdate.maxUnavailable | The maximum number of DaemonSet pods that can be unavailable during the update. Value can be an absolute number (ex: 5) or a percentage of total number of DaemonSet pods at the start of the update (ex: 10%). Absolute number is calculated from percentage by rounding up. This cannot be 0. Default value is 1. |
| agent.deploymentStrategy.rollingUpdate.slowStartAdditiveIncrease | SlowStartAdditiveIncrease Value can be an absolute number (ex: 5) or a percentage of total number of DaemonSet pods at the start of the update (ex: 10%). Default value is 5. |
| agent.deploymentStrategy.rollingUpdate.slowStartIntervalDuration | SlowStartIntervalDuration the duration between to 2 Default value is 1min. |
| agent.deploymentStrategy.updateStrategyType | The update strategy used for the DaemonSet |
| agent.dnsConfig.nameservers | A list of DNS name server IP addresses. This will be appended to the base nameservers generated from DNSPolicy. Duplicated nameservers will be removed. |
| agent.dnsConfig.options | A list of DNS resolver options. This will be merged with the base options generated from DNSPolicy. Duplicated entries will be removed. Resolution options given in Options will override those that appear in the base DNSPolicy. |
| agent.dnsConfig.searches | A list of DNS search domains for host-name lookup. This will be appended to the base search paths generated from DNSPolicy. Duplicated search paths will be removed. |
| agent.dnsPolicy | Set DNS policy for the pod. Defaults to "ClusterFirst". Valid values are 'ClusterFirstWithHostNet', 'ClusterFirst', 'Default' or 'None'. DNS parameters given in DNSConfig will be merged with the policy selected with DNSPolicy. To have DNS options set along with hostNetwork, you have to specify DNS policy explicitly to 'ClusterFirstWithHostNet'. |
| agent.env | Environment variables for all Datadog Agents Ref: https://docs.datadoghq.com/agent/docker/?tab=standard#environment-variables |
| agent.hostNetwork | Host networking requested for this pod. Use the host's network namespace. If this option is set, the ports that will be used must be specified. Default to false. |
| agent.hostPID | Use the host's pid namespace. Optional: Default to false. |
| agent.image.name | Define the image to use Use "gcr.io/datadoghq/agent:latest" for Datadog Agent 6 Use "datadog/dogstatsd:latest" for Standalone Datadog Agent DogStatsD6 Use "gcr.io/datadoghq/cluster-agent:latest" for Datadog Cluster Agent |
| agent.image.pullPolicy | The Kubernetes pull policy Use Always, Never or IfNotPresent |
| agent.image.pullSecrets | It is possible to specify docker registry credentials See https://kubernetes.io/docs/concepts/containers/images/#specifying-imagepullsecrets-on-a-pod |
| agent.log.containerCollectUsingFiles | Collect logs from files in /var/log/pods instead of using container runtime API. It's usually the most efficient way of collecting logs. ref: https://docs.datadoghq.com/agent/basic_agent_usage/kubernetes/#log-collection-setup Default: true |
| agent.log.containerLogsPath | This to allow log collection from container log path. Set to a different path if not using docker runtime. ref: https://docs.datadoghq.com/agent/kubernetes/daemonset_setup/?tab=k8sfile#create-manifest Default to `/var/lib/docker/containers` |
| agent.log.enabled | Enables this to activate Datadog Agent log collection. ref: https://docs.datadoghq.com/agent/basic_agent_usage/kubernetes/#log-collection-setup |
| agent.log.logsConfigContainerCollectAll | Enable this to allow log collection for all containers. ref: https://docs.datadoghq.com/agent/basic_agent_usage/kubernetes/#log-collection-setup |
| agent.log.openFilesLimit | Set the maximum number of logs files that the Datadog Agent will tail up to. Increasing this limit can increase resource consumption of the Agent. ref: https://docs.datadoghq.com/agent/basic_agent_usage/kubernetes/#log-collection-setup Default to 100 |
| agent.log.podLogsPath | This to allow log collection from pod log path. Default to `/var/log/pods` |
| agent.log.tempStoragePath | This path (always mounted from the host) is used by Datadog Agent to store information about processed log files. If the Datadog Agent is restarted, it allows to start tailing the log files from the right offset Default to `/var/lib/datadog-agent/logs` |
| agent.networkPolicy.create | If true, create a NetworkPolicy for the current agent |
| agent.priorityClassName | If specified, indicates the pod's priority. "system-node-critical" and "system-cluster-critical" are two special keywords which indicate the highest priorities with the former being the highest priority. Any other name must be defined by creating a PriorityClass object with that name. If not specified, the pod priority will be default or zero if there is no default. |
| agent.process.enabled | Note: /etc/passwd is automatically mounted to allow username resolution. ref: https://docs.datadoghq.com/graphing/infrastructure/process/#kubernetes-daemonset |
| agent.process.env | The Datadog Agent supports many environment variables Ref: https://docs.datadoghq.com/agent/docker/?tab=standard#environment-variables |
| agent.process.processCollectionEnabled | false (default): Only collect containers if available. true: collect process information as well |
| agent.process.resources.limits | Limits describes the maximum amount of compute resources allowed. More info: https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/ |
| agent.process.resources.requests | Requests describes the minimum amount of compute resources required. If Requests is omitted for a container, it defaults to Limits if that is explicitly specified, otherwise to an implementation-defined value. More info: https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/ |
| agent.process.volumeMounts | Specify additional volume mounts in the Process Agent container |
| agent.rbac.create | Used to configure RBAC resources creation |
| agent.rbac.serviceAccountName | Used to set up the service account name to use Ignored if the field Create is true |
| agent.security.compliance.checkInterval | Check interval |
| agent.security.compliance.configDir.configMapName | ConfigMapName name of a ConfigMap used to mount a directory |
| agent.security.compliance.enabled | Enables continuous compliance monitoring |
| agent.security.env | The Datadog Security Agent supports many environment variables Ref: https://docs.datadoghq.com/agent/docker/?tab=standard#environment-variables |
| agent.security.resources.limits | Limits describes the maximum amount of compute resources allowed. More info: https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/ |
| agent.security.resources.requests | Requests describes the minimum amount of compute resources required. If Requests is omitted for a container, it defaults to Limits if that is explicitly specified, otherwise to an implementation-defined value. More info: https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/ |
| agent.security.runtime.enabled | Enables runtime security features |
| agent.security.runtime.policiesDir.configMapName | ConfigMapName name of a ConfigMap used to mount a directory |
| agent.security.runtime.syscallMonitor.enabled | Enabled enables syscall monitor |
| agent.security.volumeMounts | Specify additional volume mounts in the Security Agent container |
| agent.systemProbe.appArmorProfileName | AppArmorProfileName specify a apparmor profile |
| agent.systemProbe.bpfDebugEnabled | BPFDebugEnabled logging for kernel debug |
| agent.systemProbe.collectDNSStats | CollectDNSStats enables DNS stat collection |
| agent.systemProbe.conntrackEnabled | ConntrackEnabled enable the system-probe agent to connect to the netlink/conntrack subsystem to add NAT information to connection data Ref: http://conntrack-tools.netfilter.org/ |
| agent.systemProbe.debugPort | DebugPort Specify the port to expose pprof and expvar for system-probe agent |
| agent.systemProbe.enableOOMKill | EnableOOMKill enables the OOM kill eBPF-based check |
| agent.systemProbe.enableTCPQueueLength | EnableTCPQueueLength enables the TCP queue length eBPF-based check |
| agent.systemProbe.enabled | Enable this to activate live process monitoring. Note: /etc/passwd is automatically mounted to allow username resolution. ref: https://docs.datadoghq.com/graphing/infrastructure/process/#kubernetes-daemonset |
| agent.systemProbe.env | The Datadog SystemProbe supports many environment variables Ref: https://docs.datadoghq.com/agent/docker/?tab=standard#environment-variables |
| agent.systemProbe.resources.limits | Limits describes the maximum amount of compute resources allowed. More info: https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/ |
| agent.systemProbe.resources.requests | Requests describes the minimum amount of compute resources required. If Requests is omitted for a container, it defaults to Limits if that is explicitly specified, otherwise to an implementation-defined value. More info: https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/ |
| agent.systemProbe.secCompCustomProfileConfigMap | SecCompCustomProfileConfigMap specify a pre-existing ConfigMap containing a custom SecComp profile |
| agent.systemProbe.secCompProfileName | SecCompProfileName specify a seccomp profile |
| agent.systemProbe.secCompRootPath | SecCompRootPath specify the seccomp profile root directory |
| agent.systemProbe.securityContext.allowPrivilegeEscalation | AllowPrivilegeEscalation controls whether a process can gain more privileges than its parent process. This bool directly controls if the no_new_privs flag will be set on the container process. AllowPrivilegeEscalation is true always when the container is: 1) run as Privileged 2) has CAP_SYS_ADMIN |
| agent.systemProbe.securityContext.capabilities.add | Added capabilities |
| agent.systemProbe.securityContext.capabilities.drop | Removed capabilities |
| agent.systemProbe.securityContext.privileged | Run container in privileged mode. Processes in privileged containers are essentially equivalent to root on the host. Defaults to false. |
| agent.systemProbe.securityContext.procMount | procMount denotes the type of proc mount to use for the containers. The default is DefaultProcMount which uses the container runtime defaults for readonly paths and masked paths. This requires the ProcMountType feature flag to be enabled. |
| agent.systemProbe.securityContext.readOnlyRootFilesystem | Whether this container has a read-only root filesystem. Default is false. |
| agent.systemProbe.securityContext.runAsGroup | The GID to run the entrypoint of the container process. Uses runtime default if unset. May also be set in PodSecurityContext.  If set in both SecurityContext and PodSecurityContext, the value specified in SecurityContext takes precedence. |
| agent.systemProbe.securityContext.runAsNonRoot | Indicates that the container must run as a non-root user. If true, the Kubelet will validate the image at runtime to ensure that it does not run as UID 0 (root) and fail to start the container if it does. If unset or false, no such validation will be performed. May also be set in PodSecurityContext.  If set in both SecurityContext and PodSecurityContext, the value specified in SecurityContext takes precedence. |
| agent.systemProbe.securityContext.runAsUser | The UID to run the entrypoint of the container process. Defaults to user specified in image metadata if unspecified. May also be set in PodSecurityContext.  If set in both SecurityContext and PodSecurityContext, the value specified in SecurityContext takes precedence. |
| agent.systemProbe.securityContext.seLinuxOptions.level | Level is SELinux level label that applies to the container. |
| agent.systemProbe.securityContext.seLinuxOptions.role | Role is a SELinux role label that applies to the container. |
| agent.systemProbe.securityContext.seLinuxOptions.type | Type is a SELinux type label that applies to the container. |
| agent.systemProbe.securityContext.seLinuxOptions.user | User is a SELinux user label that applies to the container. |
| agent.systemProbe.securityContext.seccompProfile.localhostProfile | localhostProfile indicates a profile defined in a file on the node should be used. The profile must be preconfigured on the node to work. Must be a descending path, relative to the kubelet's configured seccomp profile location. Must only be set if type is "Localhost". |
| agent.systemProbe.securityContext.seccompProfile.type | type indicates which kind of seccomp profile will be applied. Valid options are:  Localhost - a profile defined in a file on the node should be used. RuntimeDefault - the container runtime default profile should be used. Unconfined - no profile should be applied. |
| agent.systemProbe.securityContext.windowsOptions.gmsaCredentialSpec | GMSACredentialSpec is where the GMSA admission webhook (https://github.com/kubernetes-sigs/windows-gmsa) inlines the contents of the GMSA credential spec named by the GMSACredentialSpecName field. |
| agent.systemProbe.securityContext.windowsOptions.gmsaCredentialSpecName | GMSACredentialSpecName is the name of the GMSA credential spec to use. |
| agent.systemProbe.securityContext.windowsOptions.runAsUserName | The UserName in Windows to run the entrypoint of the container process. Defaults to the user specified in image metadata if unspecified. May also be set in PodSecurityContext. If set in both SecurityContext and PodSecurityContext, the value specified in SecurityContext takes precedence. |
| agent.useExtendedDaemonset | UseExtendedDaemonset use ExtendedDaemonset for Agent deployment. default value is false. |
| clusterAgent.additionalAnnotations | AdditionalAnnotations provide annotations that will be added to the cluster-agent Pods. |
| clusterAgent.additionalLabels | AdditionalLabels provide labels that will be added to the cluster checks runner Pods. |
| clusterAgent.affinity.nodeAffinity.preferredDuringSchedulingIgnoredDuringExecution | The scheduler will prefer to schedule pods to nodes that satisfy the affinity expressions specified by this field, but it may choose a node that violates one or more of the expressions. The node that is most preferred is the one with the greatest sum of weights, i.e. for each node that meets all of the scheduling requirements (resource request, requiredDuringScheduling affinity expressions, etc.), compute a sum by iterating through the elements of this field and adding "weight" to the sum if the node matches the corresponding matchExpressions; the node(s) with the highest sum are the most preferred. |
| clusterAgent.affinity.nodeAffinity.requiredDuringSchedulingIgnoredDuringExecution.nodeSelectorTerms | Required. A list of node selector terms. The terms are ORed. |
| clusterAgent.affinity.podAffinity.preferredDuringSchedulingIgnoredDuringExecution | The scheduler will prefer to schedule pods to nodes that satisfy the affinity expressions specified by this field, but it may choose a node that violates one or more of the expressions. The node that is most preferred is the one with the greatest sum of weights, i.e. for each node that meets all of the scheduling requirements (resource request, requiredDuringScheduling affinity expressions, etc.), compute a sum by iterating through the elements of this field and adding "weight" to the sum if the node has pods which matches the corresponding podAffinityTerm; the node(s) with the highest sum are the most preferred. |
| clusterAgent.affinity.podAffinity.requiredDuringSchedulingIgnoredDuringExecution | If the affinity requirements specified by this field are not met at scheduling time, the pod will not be scheduled onto the node. If the affinity requirements specified by this field cease to be met at some point during pod execution (e.g. due to a pod label update), the system may or may not try to eventually evict the pod from its node. When there are multiple elements, the lists of nodes corresponding to each podAffinityTerm are intersected, i.e. all terms must be satisfied. |
| clusterAgent.affinity.podAntiAffinity.preferredDuringSchedulingIgnoredDuringExecution | The scheduler will prefer to schedule pods to nodes that satisfy the anti-affinity expressions specified by this field, but it may choose a node that violates one or more of the expressions. The node that is most preferred is the one with the greatest sum of weights, i.e. for each node that meets all of the scheduling requirements (resource request, requiredDuringScheduling anti-affinity expressions, etc.), compute a sum by iterating through the elements of this field and adding "weight" to the sum if the node has pods which matches the corresponding podAffinityTerm; the node(s) with the highest sum are the most preferred. |
| clusterAgent.affinity.podAntiAffinity.requiredDuringSchedulingIgnoredDuringExecution | If the anti-affinity requirements specified by this field are not met at scheduling time, the pod will not be scheduled onto the node. If the anti-affinity requirements specified by this field cease to be met at some point during pod execution (e.g. due to a pod label update), the system may or may not try to eventually evict the pod from its node. When there are multiple elements, the lists of nodes corresponding to each podAffinityTerm are intersected, i.e. all terms must be satisfied. |
| clusterAgent.config.admissionController.enabled | Enable the admission controller to be able to inject APM/Dogstatsd config and standard tags (env, service, version) automatically into your pods |
| clusterAgent.config.admissionController.mutateUnlabelled | MutateUnlabelled enables injecting config without having the pod label 'admission.datadoghq.com/enabled="true"' |
| clusterAgent.config.admissionController.serviceName | ServiceName corresponds to the webhook service name |
| clusterAgent.config.clusterChecksEnabled | Enable the Cluster Checks and Endpoint Checks feature on both the cluster-agents and the daemonset ref: https://docs.datadoghq.com/agent/cluster_agent/clusterchecks/ https://docs.datadoghq.com/agent/cluster_agent/endpointschecks/ Autodiscovery via Kube Service annotations is automatically enabled |
| clusterAgent.config.collectEvents | Enable this to start event collection from the kubernetes API ref: https://docs.datadoghq.com/agent/cluster_agent/event_collection/ |
| clusterAgent.config.confd.configMapName | ConfigMapName name of a ConfigMap used to mount a directory |
| clusterAgent.config.env | The Datadog Agent supports many environment variables Ref: https://docs.datadoghq.com/agent/docker/?tab=standard#environment-variables |
| clusterAgent.config.externalMetrics.credentials.apiKey | APIKey Set this to your Datadog API key before the Agent runs. ref: https://app.datadoghq.com/account/settings#agent/kubernetes |
| clusterAgent.config.externalMetrics.credentials.apiKeyExistingSecret | APIKeyExistingSecret is DEPRECATED. In order to pass the API key through an existing secret, please consider "apiSecret" instead. If set, this parameter takes precedence over "apiKey". |
| clusterAgent.config.externalMetrics.credentials.apiSecret.keyName | KeyName is the key of the secret to use |
| clusterAgent.config.externalMetrics.credentials.apiSecret.secretName | SecretName is the name of the secret |
| clusterAgent.config.externalMetrics.credentials.appKey | If you are using clusterAgent.metricsProvider.enabled = true, you must set a Datadog application key for read access to your metrics. |
| clusterAgent.config.externalMetrics.credentials.appKeyExistingSecret | AppKeyExistingSecret is DEPRECATED. In order to pass the APP key through an existing secret, please consider "appSecret" instead. If set, this parameter takes precedence over "appKey". |
| clusterAgent.config.externalMetrics.credentials.appSecret.keyName | KeyName is the key of the secret to use |
| clusterAgent.config.externalMetrics.credentials.appSecret.secretName | SecretName is the name of the secret |
| clusterAgent.config.externalMetrics.enabled | Enable the metricsProvider to be able to scale based on metrics in Datadog |
| clusterAgent.config.externalMetrics.endpoint | Override the API endpoint for the external metrics server. Defaults to .spec.agent.config.ddUrl or "https://app.datadoghq.com" if that's empty. |
| clusterAgent.config.externalMetrics.port | If specified configures the metricsProvider external metrics service port |
| clusterAgent.config.externalMetrics.useDatadogMetrics | Enable usage of DatadogMetrics CRD (allow to scale on arbitrary queries) |
| clusterAgent.config.externalMetrics.wpaController | Enable informer and controller of the watermark pod autoscaler NOTE: The WatermarkPodAutoscaler controller needs to be installed see https://github.com/DataDog/watermarkpodautoscaler for more details. |
| clusterAgent.config.logLevel | Set logging verbosity, valid log levels are: trace, debug, info, warn, error, critical, and off |
| clusterAgent.config.resources.limits | Limits describes the maximum amount of compute resources allowed. More info: https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/ |
| clusterAgent.config.resources.requests | Requests describes the minimum amount of compute resources required. If Requests is omitted for a container, it defaults to Limits if that is explicitly specified, otherwise to an implementation-defined value. More info: https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/ |
| clusterAgent.config.volumeMounts | Specify additional volume mounts in the Datadog Cluster Agent container |
| clusterAgent.config.volumes | Specify additional volumes in the Datadog Cluster Agent container |
| clusterAgent.customConfig.configData | ConfigData corresponds to the configuration file content |
| clusterAgent.customConfig.configMap.fileKey | FileKey corresponds to the key used in the ConfigMap.Data to store the configuration file content |
| clusterAgent.customConfig.configMap.name | Name the ConfigMap name |
| clusterAgent.deploymentName | Name of the Cluster Agent Deployment to create or migrate from |
| clusterAgent.image.name | Define the image to use Use "gcr.io/datadoghq/agent:latest" for Datadog Agent 6 Use "datadog/dogstatsd:latest" for Standalone Datadog Agent DogStatsD6 Use "gcr.io/datadoghq/cluster-agent:latest" for Datadog Cluster Agent |
| clusterAgent.image.pullPolicy | The Kubernetes pull policy Use Always, Never or IfNotPresent |
| clusterAgent.image.pullSecrets | It is possible to specify docker registry credentials See https://kubernetes.io/docs/concepts/containers/images/#specifying-imagepullsecrets-on-a-pod |
| clusterAgent.networkPolicy.create | If true, create a NetworkPolicy for the current agent |
| clusterAgent.nodeSelector | NodeSelector is a selector which must be true for the pod to fit on a node. Selector which must match a node's labels for the pod to be scheduled on that node. More info: https://kubernetes.io/docs/concepts/configuration/assign-pod-node/ |
| clusterAgent.priorityClassName | If specified, indicates the pod's priority. "system-node-critical" and "system-cluster-critical" are two special keywords which indicate the highest priorities with the former being the highest priority. Any other name must be defined by creating a PriorityClass object with that name. If not specified, the pod priority will be default or zero if there is no default. |
| clusterAgent.rbac.create | Used to configure RBAC resources creation |
| clusterAgent.rbac.serviceAccountName | Used to set up the service account name to use Ignored if the field Create is true |
| clusterAgent.replicas | Number of the Cluster Agent replicas |
| clusterAgent.tolerations | If specified, the Cluster-Agent pod's tolerations. |
| clusterChecksRunner.additionalAnnotations | AdditionalAnnotations provide annotations that will be added to the cluster checks runner Pods. |
| clusterChecksRunner.additionalLabels | AdditionalLabels provide labels that will be added to the cluster checks runner Pods. |
| clusterChecksRunner.affinity.nodeAffinity.preferredDuringSchedulingIgnoredDuringExecution | The scheduler will prefer to schedule pods to nodes that satisfy the affinity expressions specified by this field, but it may choose a node that violates one or more of the expressions. The node that is most preferred is the one with the greatest sum of weights, i.e. for each node that meets all of the scheduling requirements (resource request, requiredDuringScheduling affinity expressions, etc.), compute a sum by iterating through the elements of this field and adding "weight" to the sum if the node matches the corresponding matchExpressions; the node(s) with the highest sum are the most preferred. |
| clusterChecksRunner.affinity.nodeAffinity.requiredDuringSchedulingIgnoredDuringExecution.nodeSelectorTerms | Required. A list of node selector terms. The terms are ORed. |
| clusterChecksRunner.affinity.podAffinity.preferredDuringSchedulingIgnoredDuringExecution | The scheduler will prefer to schedule pods to nodes that satisfy the affinity expressions specified by this field, but it may choose a node that violates one or more of the expressions. The node that is most preferred is the one with the greatest sum of weights, i.e. for each node that meets all of the scheduling requirements (resource request, requiredDuringScheduling affinity expressions, etc.), compute a sum by iterating through the elements of this field and adding "weight" to the sum if the node has pods which matches the corresponding podAffinityTerm; the node(s) with the highest sum are the most preferred. |
| clusterChecksRunner.affinity.podAffinity.requiredDuringSchedulingIgnoredDuringExecution | If the affinity requirements specified by this field are not met at scheduling time, the pod will not be scheduled onto the node. If the affinity requirements specified by this field cease to be met at some point during pod execution (e.g. due to a pod label update), the system may or may not try to eventually evict the pod from its node. When there are multiple elements, the lists of nodes corresponding to each podAffinityTerm are intersected, i.e. all terms must be satisfied. |
| clusterChecksRunner.affinity.podAntiAffinity.preferredDuringSchedulingIgnoredDuringExecution | The scheduler will prefer to schedule pods to nodes that satisfy the anti-affinity expressions specified by this field, but it may choose a node that violates one or more of the expressions. The node that is most preferred is the one with the greatest sum of weights, i.e. for each node that meets all of the scheduling requirements (resource request, requiredDuringScheduling anti-affinity expressions, etc.), compute a sum by iterating through the elements of this field and adding "weight" to the sum if the node has pods which matches the corresponding podAffinityTerm; the node(s) with the highest sum are the most preferred. |
| clusterChecksRunner.affinity.podAntiAffinity.requiredDuringSchedulingIgnoredDuringExecution | If the anti-affinity requirements specified by this field are not met at scheduling time, the pod will not be scheduled onto the node. If the anti-affinity requirements specified by this field cease to be met at some point during pod execution (e.g. due to a pod label update), the system may or may not try to eventually evict the pod from its node. When there are multiple elements, the lists of nodes corresponding to each podAffinityTerm are intersected, i.e. all terms must be satisfied. |
| clusterChecksRunner.config.env | The Datadog Agent supports many environment variables Ref: https://docs.datadoghq.com/agent/docker/?tab=standard#environment-variables |
| clusterChecksRunner.config.logLevel | Set logging verbosity, valid log levels are: trace, debug, info, warn, error, critical, and off |
| clusterChecksRunner.config.resources.limits | Limits describes the maximum amount of compute resources allowed. More info: https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/ |
| clusterChecksRunner.config.resources.requests | Requests describes the minimum amount of compute resources required. If Requests is omitted for a container, it defaults to Limits if that is explicitly specified, otherwise to an implementation-defined value. More info: https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/ |
| clusterChecksRunner.config.volumeMounts | Specify additional volume mounts in the Datadog Cluster Check Runner container |
| clusterChecksRunner.config.volumes | Specify additional volumes in the Datadog Cluster Check Runner container |
| clusterChecksRunner.customConfig.configData | ConfigData corresponds to the configuration file content |
| clusterChecksRunner.customConfig.configMap.fileKey | FileKey corresponds to the key used in the ConfigMap.Data to store the configuration file content |
| clusterChecksRunner.customConfig.configMap.name | Name the ConfigMap name |
| clusterChecksRunner.deploymentName | Name of the cluster checks deployment to create or migrate from |
| clusterChecksRunner.image.name | Define the image to use Use "gcr.io/datadoghq/agent:latest" for Datadog Agent 6 Use "datadog/dogstatsd:latest" for Standalone Datadog Agent DogStatsD6 Use "gcr.io/datadoghq/cluster-agent:latest" for Datadog Cluster Agent |
| clusterChecksRunner.image.pullPolicy | The Kubernetes pull policy Use Always, Never or IfNotPresent |
| clusterChecksRunner.image.pullSecrets | It is possible to specify docker registry credentials See https://kubernetes.io/docs/concepts/containers/images/#specifying-imagepullsecrets-on-a-pod |
| clusterChecksRunner.networkPolicy.create | If true, create a NetworkPolicy for the current agent |
| clusterChecksRunner.nodeSelector | NodeSelector is a selector which must be true for the pod to fit on a node. Selector which must match a node's labels for the pod to be scheduled on that node. More info: https://kubernetes.io/docs/concepts/configuration/assign-pod-node/ |
| clusterChecksRunner.priorityClassName | If specified, indicates the pod's priority. "system-node-critical" and "system-cluster-critical" are two special keywords which indicate the highest priorities with the former being the highest priority. Any other name must be defined by creating a PriorityClass object with that name. If not specified, the pod priority will be default or zero if there is no default. |
| clusterChecksRunner.rbac.create | Used to configure RBAC resources creation |
| clusterChecksRunner.rbac.serviceAccountName | Used to set up the service account name to use Ignored if the field Create is true |
| clusterChecksRunner.replicas | Number of the Cluster Agent replicas |
| clusterChecksRunner.tolerations | If specified, the Cluster-Checks pod's tolerations. |
| clusterName | Set a unique cluster name to allow scoping hosts and Cluster Checks Runner easily |
| credentials.apiKey | APIKey Set this to your Datadog API key before the Agent runs. ref: https://app.datadoghq.com/account/settings#agent/kubernetes |
| credentials.apiKeyExistingSecret | APIKeyExistingSecret is DEPRECATED. In order to pass the API key through an existing secret, please consider "apiSecret" instead. If set, this parameter takes precedence over "apiKey". |
| credentials.apiSecret.keyName | KeyName is the key of the secret to use |
| credentials.apiSecret.secretName | SecretName is the name of the secret |
| credentials.appKey | If you are using clusterAgent.metricsProvider.enabled = true, you must set a Datadog application key for read access to your metrics. |
| credentials.appKeyExistingSecret | AppKeyExistingSecret is DEPRECATED. In order to pass the APP key through an existing secret, please consider "appSecret" instead. If set, this parameter takes precedence over "appKey". |
| credentials.appSecret.keyName | KeyName is the key of the secret to use |
| credentials.appSecret.secretName | SecretName is the name of the secret |
| credentials.token | This needs to be at least 32 characters a-zA-z It is a preshared key between the node agents and the cluster agent |
| credentials.useSecretBackend | UseSecretBackend use the Agent secret backend feature for retreiving all credentials needed by the different components: Agent, Cluster, Cluster-Checks. If `useSecretBackend: true`, other credential parameters will be ignored. default value is false. |
| features.kubeStateMetricsCore.conf.configData | ConfigData corresponds to the configuration file content |
| features.kubeStateMetricsCore.conf.configMap.fileKey | FileKey corresponds to the key used in the ConfigMap.Data to store the configuration file content |
| features.kubeStateMetricsCore.conf.configMap.name | Name the ConfigMap name |
| features.kubeStateMetricsCore.enabled | Enable this to start the Kubernetes State Metrics Core check. Refer to https://github.com/DataDog/datadog-operator/blob/master/docs/kubernetes_state_metrics.md |
| features.orchestratorExplorer.additionalEndpoints | Additional endpoints for shipping the collected data as json in the form of {"https://process.agent.datadoghq.com": ["apikey1", ...], ...}'. |
| features.orchestratorExplorer.ddUrl | Set this for the Datadog endpoint for the orchestrator explorer |
| features.orchestratorExplorer.enabled | Enable this to activate live Kubernetes monitoring. ref: https://docs.datadoghq.com/infrastructure/livecontainers/#kubernetes-resources |
| features.orchestratorExplorer.extraTags | Additional tags for the collected data in the form of `a b c` Difference to DD_TAGS: this is a cluster agent option that is used to define custom cluster tags |
| features.orchestratorExplorer.scrubbing.containers | Deactivate this to stop the scrubbing of sensitive container data (passwords, tokens, etc. ). |
| features.prometheusScrape.additionalConfigs | AdditionalConfigs allows adding advanced prometheus check configurations with custom discovery rules. |
| features.prometheusScrape.enabled | Enable autodiscovering pods and services exposing prometheus metrics. |
| features.prometheusScrape.serviceEndpoints | ServiceEndpoints enables generating dedicated checks for service endpoints. |
| site | The site of the Datadog intake to send Agent data to. Set to 'datadoghq.eu' to send data to the EU site. |

[1]: https://github.com/DataDog/datadog-operator/blob/master/examples/datadog-agent-all.yaml
[2]: https://github.com/DataDog/datadog-operator/blob/master/examples/datadog-agent-logs-apm.yaml
[3]: https://github.com/DataDog/datadog-operator/blob/master/examples/datadog-agent-logs.yaml
[4]: https://github.com/DataDog/datadog-operator/blob/master/examples/datadog-agent-apm.yaml
[5]: https://github.com/DataDog/datadog-operator/blob/master/examples/datadog-agent-with-clusteragent.yaml
[6]: https://github.com/DataDog/datadog-operator/blob/master/examples/datadog-agent-with-tolerations.yaml
