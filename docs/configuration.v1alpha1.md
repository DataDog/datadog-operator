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
| agent.additionalAnnotations | AdditionalAnnotations provide annotations that will be added to the Agent Pods. |
| agent.additionalLabels | AdditionalLabels provide labels that will be added to the Agent Pods. |
| agent.affinity.nodeAffinity.preferredDuringSchedulingIgnoredDuringExecution | The scheduler will prefer to schedule pods to nodes that satisfy the affinity expressions specified by this field, but it may choose a node that violates one or more of the expressions. The node that is most preferred is the one with the greatest sum of weights, i.e. for each node that meets all of the scheduling requirements (resource request, requiredDuringScheduling affinity expressions, etc.), compute a sum by iterating through the elements of this field and adding "weight" to the sum if the node matches the corresponding matchExpressions; the node(s) with the highest sum are the most preferred. |
| agent.affinity.nodeAffinity.requiredDuringSchedulingIgnoredDuringExecution.nodeSelectorTerms | Required. A list of node selector terms. The terms are ORed. |
| agent.affinity.podAffinity.preferredDuringSchedulingIgnoredDuringExecution | The scheduler will prefer to schedule pods to nodes that satisfy the affinity expressions specified by this field, but it may choose a node that violates one or more of the expressions. The node that is most preferred is the one with the greatest sum of weights, i.e. for each node that meets all of the scheduling requirements (resource request, requiredDuringScheduling affinity expressions, etc.), compute a sum by iterating through the elements of this field and adding "weight" to the sum if the node has pods which matches the corresponding podAffinityTerm; the node(s) with the highest sum are the most preferred. |
| agent.affinity.podAffinity.requiredDuringSchedulingIgnoredDuringExecution | If the affinity requirements specified by this field are not met at scheduling time, the pod will not be scheduled onto the node. If the affinity requirements specified by this field cease to be met at some point during pod execution (e.g. due to a pod label update), the system may or may not try to eventually evict the pod from its node. When there are multiple elements, the lists of nodes corresponding to each podAffinityTerm are intersected, i.e. all terms must be satisfied. |
| agent.affinity.podAntiAffinity.preferredDuringSchedulingIgnoredDuringExecution | The scheduler will prefer to schedule pods to nodes that satisfy the anti-affinity expressions specified by this field, but it may choose a node that violates one or more of the expressions. The node that is most preferred is the one with the greatest sum of weights, i.e. for each node that meets all of the scheduling requirements (resource request, requiredDuringScheduling anti-affinity expressions, etc.), compute a sum by iterating through the elements of this field and adding "weight" to the sum if the node has pods which matches the corresponding podAffinityTerm; the node(s) with the highest sum are the most preferred. |
| agent.affinity.podAntiAffinity.requiredDuringSchedulingIgnoredDuringExecution | If the anti-affinity requirements specified by this field are not met at scheduling time, the pod will not be scheduled onto the node. If the anti-affinity requirements specified by this field cease to be met at some point during pod execution (e.g. due to a pod label update), the system may or may not try to eventually evict the pod from its node. When there are multiple elements, the lists of nodes corresponding to each podAffinityTerm are intersected, i.e. all terms must be satisfied. |
| agent.apm.args | Args allows the specification of extra args to `Command` parameter |
| agent.apm.command | Command allows the specification of custom entrypoint for Trace Agent container |
| agent.apm.enabled | Enable this to enable APM and tracing, on port 8126. See also: https://github.com/DataDog/docker-dd-agent#tracing-from-the-host |
| agent.apm.env | The Datadog Agent supports many environment variables. See also: https://docs.datadoghq.com/agent/docker/?tab=standard#environment-variables |
| agent.apm.hostPort | Number of port to expose on the host. If specified, this must be a valid port number, 0 < x < 65536. If HostNetwork is specified, this must match ContainerPort. Most containers do not need this. |
| agent.apm.livenessProbe.exec.command | Command is the command line to execute inside the container, the working directory for the command  is root ('/') in the container's filesystem. The command is simply exec'd, it is not run inside a shell, so traditional shell instructions ('|', etc) won't work. To use a shell, you need to explicitly call out to that shell. Exit status of 0 is treated as live/healthy and non-zero is unhealthy. |
| agent.apm.livenessProbe.failureThreshold | Minimum consecutive failures for the probe to be considered failed after having succeeded. Defaults to 3. Minimum value is 1. |
| agent.apm.livenessProbe.grpc.port | Port number of the gRPC service. Number must be in the range 1 to 65535. |
| agent.apm.livenessProbe.grpc.service | Service is the name of the service to place in the gRPC HealthCheckRequest (see https://github.com/grpc/grpc/blob/master/doc/health-checking.md).  If this is not specified, the default behavior is defined by gRPC. |
| agent.apm.livenessProbe.httpGet.host | Host name to connect to, defaults to the pod IP. You probably want to set "Host" in httpHeaders instead. |
| agent.apm.livenessProbe.httpGet.httpHeaders | Custom headers to set in the request. HTTP allows repeated headers. |
| agent.apm.livenessProbe.httpGet.path | Path to access on the HTTP server. |
| agent.apm.livenessProbe.httpGet.port | Name or number of the port to access on the container. Number must be in the range 1 to 65535. Name must be an IANA_SVC_NAME. |
| agent.apm.livenessProbe.httpGet.scheme | Scheme to use for connecting to the host. Defaults to HTTP. |
| agent.apm.livenessProbe.initialDelaySeconds | Number of seconds after the container has started before liveness probes are initiated. More info: https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle#container-probes |
| agent.apm.livenessProbe.periodSeconds | How often (in seconds) to perform the probe. Default to 10 seconds. Minimum value is 1. |
| agent.apm.livenessProbe.successThreshold | Minimum consecutive successes for the probe to be considered successful after having failed. Defaults to 1. Must be 1 for liveness and startup. Minimum value is 1. |
| agent.apm.livenessProbe.tcpSocket.host | Optional: Host name to connect to, defaults to the pod IP. |
| agent.apm.livenessProbe.tcpSocket.port | Number or name of the port to access on the container. Number must be in the range 1 to 65535. Name must be an IANA_SVC_NAME. |
| agent.apm.livenessProbe.terminationGracePeriodSeconds | Optional duration in seconds the pod needs to terminate gracefully upon probe failure. The grace period is the duration in seconds after the processes running in the pod are sent a termination signal and the time when the processes are forcibly halted with a kill signal. Set this value longer than the expected cleanup time for your process. If this value is nil, the pod's terminationGracePeriodSeconds will be used. Otherwise, this value overrides the value provided by the pod spec. Value must be non-negative integer. The value zero indicates stop immediately via the kill signal (no opportunity to shut down). This is a beta field and requires enabling ProbeTerminationGracePeriod feature gate. Minimum value is 1. spec.terminationGracePeriodSeconds is used if unset. |
| agent.apm.livenessProbe.timeoutSeconds | Number of seconds after which the probe times out. Defaults to 1 second. Minimum value is 1. More info: https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle#container-probes |
| agent.apm.resources.limits | Limits describes the maximum amount of compute resources allowed. More info: https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/ |
| agent.apm.resources.requests | Requests describes the minimum amount of compute resources required. If Requests is omitted for a container, it defaults to Limits if that is explicitly specified, otherwise to an implementation-defined value. More info: https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/ |
| agent.apm.unixDomainSocket.enabled | Enable APM over Unix Domain Socket See also: https://docs.datadoghq.com/agent/kubernetes/apm/?tab=helm#agent-environment-variables |
| agent.apm.unixDomainSocket.hostFilepath | Define the host APM socket filepath used when APM over Unix Domain Socket is enabled. (default value: /var/run/datadog/apm.sock) See also: https://docs.datadoghq.com/agent/kubernetes/apm/?tab=helm#agent-environment-variables |
| agent.apm.volumeMounts | Specify additional volume mounts in the APM Agent container. |
| agent.config.args | Args allows the specification of extra args to `Command` parameter |
| agent.config.checksd.configMapName | ConfigMapName name of a ConfigMap used to mount a directory. |
| agent.config.checksd.items | items mapping between configMap data key and file path mount. |
| agent.config.collectEvents | Enables this to start event collection from the Kubernetes API. See also: https://docs.datadoghq.com/agent/kubernetes/event_collection/ |
| agent.config.command | Command allows the specification of custom entrypoint for the Agent container |
| agent.config.confd.configMapName | ConfigMapName name of a ConfigMap used to mount a directory. |
| agent.config.confd.items | items mapping between configMap data key and file path mount. |
| agent.config.criSocket.criSocketPath | Path to the container runtime socket (if different from Docker). This is supported starting from agent 6.6.0. |
| agent.config.criSocket.dockerSocketPath | Path to the docker runtime socket. |
| agent.config.ddUrl | The host of the Datadog intake server to send Agent data to, only set this option if you need the Agent to send data to a custom URL. Overrides the site setting defined in "site". |
| agent.config.dogstatsd.dogstatsdOriginDetection | Enable origin detection for container tagging. See also: https://docs.datadoghq.com/developers/dogstatsd/unix_socket/#using-origin-detection-for-container-tagging |
| agent.config.dogstatsd.mapperProfiles.configData | ConfigData corresponds to the configuration file content. |
| agent.config.dogstatsd.mapperProfiles.configMap.fileKey | FileKey corresponds to the key used in the ConfigMap.Data to store the configuration file content. |
| agent.config.dogstatsd.mapperProfiles.configMap.name | The name of source ConfigMap. |
| agent.config.dogstatsd.unixDomainSocket.enabled | Enable APM over Unix Domain Socket. See also: https://docs.datadoghq.com/developers/dogstatsd/unix_socket/ |
| agent.config.dogstatsd.unixDomainSocket.hostFilepath | Define the host APM socket filepath used when APM over Unix Domain Socket is enabled. (default value: /var/run/datadog/statsd.sock). See also: https://docs.datadoghq.com/developers/dogstatsd/unix_socket/ |
| agent.config.env | The Datadog Agent supports many environment variables. See also: https://docs.datadoghq.com/agent/docker/?tab=standard#environment-variables |
| agent.config.healthPort | HealthPort of the Agent container for internal liveness probe. Must be the same as the Liveness/Readiness probes. |
| agent.config.hostPort | Number of port to expose on the host. If specified, this must be a valid port number, 0 < x < 65536. If HostNetwork is specified, this must match ContainerPort. Most containers do not need this. |
| agent.config.kubelet.agentCAPath | AgentCAPath is the container path where the kubelet CA certificate is stored. Default: '/var/run/host-kubelet-ca.crt' if hostCAPath is set, else '/var/run/secrets/kubernetes.io/serviceaccount/ca.crt' |
| agent.config.kubelet.host.configMapKeyRef.key | The key to select. |
| agent.config.kubelet.host.configMapKeyRef.name | Name of the referent. More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names TODO: Add other useful fields. apiVersion, kind, uid? |
| agent.config.kubelet.host.configMapKeyRef.optional | Specify whether the ConfigMap or its key must be defined |
| agent.config.kubelet.host.fieldRef.apiVersion | Version of the schema the FieldPath is written in terms of, defaults to "v1". |
| agent.config.kubelet.host.fieldRef.fieldPath | Path of the field to select in the specified API version. |
| agent.config.kubelet.host.resourceFieldRef.containerName | Container name: required for volumes, optional for env vars |
| agent.config.kubelet.host.resourceFieldRef.divisor | Specifies the output format of the exposed resources, defaults to "1" |
| agent.config.kubelet.host.resourceFieldRef.resource | Required: resource to select |
| agent.config.kubelet.host.secretKeyRef.key | The key of the secret to select from.  Must be a valid secret key. |
| agent.config.kubelet.host.secretKeyRef.name | Name of the referent. More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names TODO: Add other useful fields. apiVersion, kind, uid? |
| agent.config.kubelet.host.secretKeyRef.optional | Specify whether the Secret or its key must be defined |
| agent.config.kubelet.hostCAPath | HostCAPath is the host path where the kubelet CA certificate is stored. |
| agent.config.kubelet.tlsVerify | TLSVerify toggles kubelet TLS verification. Default: true |
| agent.config.leaderElection | Enables leader election mechanism for event collection. |
| agent.config.livenessProbe.exec.command | Command is the command line to execute inside the container, the working directory for the command  is root ('/') in the container's filesystem. The command is simply exec'd, it is not run inside a shell, so traditional shell instructions ('|', etc) won't work. To use a shell, you need to explicitly call out to that shell. Exit status of 0 is treated as live/healthy and non-zero is unhealthy. |
| agent.config.livenessProbe.failureThreshold | Minimum consecutive failures for the probe to be considered failed after having succeeded. Defaults to 3. Minimum value is 1. |
| agent.config.livenessProbe.grpc.port | Port number of the gRPC service. Number must be in the range 1 to 65535. |
| agent.config.livenessProbe.grpc.service | Service is the name of the service to place in the gRPC HealthCheckRequest (see https://github.com/grpc/grpc/blob/master/doc/health-checking.md).  If this is not specified, the default behavior is defined by gRPC. |
| agent.config.livenessProbe.httpGet.host | Host name to connect to, defaults to the pod IP. You probably want to set "Host" in httpHeaders instead. |
| agent.config.livenessProbe.httpGet.httpHeaders | Custom headers to set in the request. HTTP allows repeated headers. |
| agent.config.livenessProbe.httpGet.path | Path to access on the HTTP server. |
| agent.config.livenessProbe.httpGet.port | Name or number of the port to access on the container. Number must be in the range 1 to 65535. Name must be an IANA_SVC_NAME. |
| agent.config.livenessProbe.httpGet.scheme | Scheme to use for connecting to the host. Defaults to HTTP. |
| agent.config.livenessProbe.initialDelaySeconds | Number of seconds after the container has started before liveness probes are initiated. More info: https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle#container-probes |
| agent.config.livenessProbe.periodSeconds | How often (in seconds) to perform the probe. Default to 10 seconds. Minimum value is 1. |
| agent.config.livenessProbe.successThreshold | Minimum consecutive successes for the probe to be considered successful after having failed. Defaults to 1. Must be 1 for liveness and startup. Minimum value is 1. |
| agent.config.livenessProbe.tcpSocket.host | Optional: Host name to connect to, defaults to the pod IP. |
| agent.config.livenessProbe.tcpSocket.port | Number or name of the port to access on the container. Number must be in the range 1 to 65535. Name must be an IANA_SVC_NAME. |
| agent.config.livenessProbe.terminationGracePeriodSeconds | Optional duration in seconds the pod needs to terminate gracefully upon probe failure. The grace period is the duration in seconds after the processes running in the pod are sent a termination signal and the time when the processes are forcibly halted with a kill signal. Set this value longer than the expected cleanup time for your process. If this value is nil, the pod's terminationGracePeriodSeconds will be used. Otherwise, this value overrides the value provided by the pod spec. Value must be non-negative integer. The value zero indicates stop immediately via the kill signal (no opportunity to shut down). This is a beta field and requires enabling ProbeTerminationGracePeriod feature gate. Minimum value is 1. spec.terminationGracePeriodSeconds is used if unset. |
| agent.config.livenessProbe.timeoutSeconds | Number of seconds after which the probe times out. Defaults to 1 second. Minimum value is 1. More info: https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle#container-probes |
| agent.config.logLevel | Set logging verbosity, valid log levels are: trace, debug, info, warn, error, critical, and off |
| agent.config.namespaceLabelsAsTags | Provide a mapping of Kubernetes Namespace Labels to Datadog Tags. <KUBERNETES_LABEL>: <DATADOG_TAG_KEY> |
| agent.config.nodeLabelsAsTags | Provide a mapping of Kubernetes Node Labels to Datadog Tags. <KUBERNETES_LABEL>: <DATADOG_TAG_KEY> |
| agent.config.podAnnotationsAsTags | Provide a mapping of Kubernetes Pod Annotations to Datadog Tags. <KUBERNETES_ANNOTATIONS>: <DATADOG_TAG_KEY> |
| agent.config.podLabelsAsTags | Provide a mapping of Kubernetes Pod Labels to Datadog Tags. <KUBERNETES_LABEL>: <DATADOG_TAG_KEY> |
| agent.config.readinessProbe.exec.command | Command is the command line to execute inside the container, the working directory for the command  is root ('/') in the container's filesystem. The command is simply exec'd, it is not run inside a shell, so traditional shell instructions ('|', etc) won't work. To use a shell, you need to explicitly call out to that shell. Exit status of 0 is treated as live/healthy and non-zero is unhealthy. |
| agent.config.readinessProbe.failureThreshold | Minimum consecutive failures for the probe to be considered failed after having succeeded. Defaults to 3. Minimum value is 1. |
| agent.config.readinessProbe.grpc.port | Port number of the gRPC service. Number must be in the range 1 to 65535. |
| agent.config.readinessProbe.grpc.service | Service is the name of the service to place in the gRPC HealthCheckRequest (see https://github.com/grpc/grpc/blob/master/doc/health-checking.md).  If this is not specified, the default behavior is defined by gRPC. |
| agent.config.readinessProbe.httpGet.host | Host name to connect to, defaults to the pod IP. You probably want to set "Host" in httpHeaders instead. |
| agent.config.readinessProbe.httpGet.httpHeaders | Custom headers to set in the request. HTTP allows repeated headers. |
| agent.config.readinessProbe.httpGet.path | Path to access on the HTTP server. |
| agent.config.readinessProbe.httpGet.port | Name or number of the port to access on the container. Number must be in the range 1 to 65535. Name must be an IANA_SVC_NAME. |
| agent.config.readinessProbe.httpGet.scheme | Scheme to use for connecting to the host. Defaults to HTTP. |
| agent.config.readinessProbe.initialDelaySeconds | Number of seconds after the container has started before liveness probes are initiated. More info: https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle#container-probes |
| agent.config.readinessProbe.periodSeconds | How often (in seconds) to perform the probe. Default to 10 seconds. Minimum value is 1. |
| agent.config.readinessProbe.successThreshold | Minimum consecutive successes for the probe to be considered successful after having failed. Defaults to 1. Must be 1 for liveness and startup. Minimum value is 1. |
| agent.config.readinessProbe.tcpSocket.host | Optional: Host name to connect to, defaults to the pod IP. |
| agent.config.readinessProbe.tcpSocket.port | Number or name of the port to access on the container. Number must be in the range 1 to 65535. Name must be an IANA_SVC_NAME. |
| agent.config.readinessProbe.terminationGracePeriodSeconds | Optional duration in seconds the pod needs to terminate gracefully upon probe failure. The grace period is the duration in seconds after the processes running in the pod are sent a termination signal and the time when the processes are forcibly halted with a kill signal. Set this value longer than the expected cleanup time for your process. If this value is nil, the pod's terminationGracePeriodSeconds will be used. Otherwise, this value overrides the value provided by the pod spec. Value must be non-negative integer. The value zero indicates stop immediately via the kill signal (no opportunity to shut down). This is a beta field and requires enabling ProbeTerminationGracePeriod feature gate. Minimum value is 1. spec.terminationGracePeriodSeconds is used if unset. |
| agent.config.readinessProbe.timeoutSeconds | Number of seconds after which the probe times out. Defaults to 1 second. Minimum value is 1. More info: https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle#container-probes |
| agent.config.resources.limits | Limits describes the maximum amount of compute resources allowed. More info: https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/ |
| agent.config.resources.requests | Requests describes the minimum amount of compute resources required. If Requests is omitted for a container, it defaults to Limits if that is explicitly specified, otherwise to an implementation-defined value. More info: https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/ |
| agent.config.securityContext.fsGroup | A special supplemental group that applies to all containers in a pod. Some volume types allow the Kubelet to change the ownership of that volume to be owned by the pod:  1. The owning GID will be the FSGroup 2. The setgid bit is set (new files created in the volume will be owned by FSGroup) 3. The permission bits are OR'd with rw-rw----  If unset, the Kubelet will not modify the ownership and permissions of any volume. Note that this field cannot be set when spec.os.name is windows. |
| agent.config.securityContext.fsGroupChangePolicy | fsGroupChangePolicy defines behavior of changing ownership and permission of the volume before being exposed inside Pod. This field will only apply to volume types which support fsGroup based ownership(and permissions). It will have no effect on ephemeral volume types such as: secret, configmaps and emptydir. Valid values are "OnRootMismatch" and "Always". If not specified, "Always" is used. Note that this field cannot be set when spec.os.name is windows. |
| agent.config.securityContext.runAsGroup | The GID to run the entrypoint of the container process. Uses runtime default if unset. May also be set in SecurityContext.  If set in both SecurityContext and PodSecurityContext, the value specified in SecurityContext takes precedence for that container. Note that this field cannot be set when spec.os.name is windows. |
| agent.config.securityContext.runAsNonRoot | Indicates that the container must run as a non-root user. If true, the Kubelet will validate the image at runtime to ensure that it does not run as UID 0 (root) and fail to start the container if it does. If unset or false, no such validation will be performed. May also be set in SecurityContext.  If set in both SecurityContext and PodSecurityContext, the value specified in SecurityContext takes precedence. |
| agent.config.securityContext.runAsUser | The UID to run the entrypoint of the container process. Defaults to user specified in image metadata if unspecified. May also be set in SecurityContext.  If set in both SecurityContext and PodSecurityContext, the value specified in SecurityContext takes precedence for that container. Note that this field cannot be set when spec.os.name is windows. |
| agent.config.securityContext.seLinuxOptions.level | Level is SELinux level label that applies to the container. |
| agent.config.securityContext.seLinuxOptions.role | Role is a SELinux role label that applies to the container. |
| agent.config.securityContext.seLinuxOptions.type | Type is a SELinux type label that applies to the container. |
| agent.config.securityContext.seLinuxOptions.user | User is a SELinux user label that applies to the container. |
| agent.config.securityContext.seccompProfile.localhostProfile | localhostProfile indicates a profile defined in a file on the node should be used. The profile must be preconfigured on the node to work. Must be a descending path, relative to the kubelet's configured seccomp profile location. Must only be set if type is "Localhost". |
| agent.config.securityContext.seccompProfile.type | type indicates which kind of seccomp profile will be applied. Valid options are:  Localhost - a profile defined in a file on the node should be used. RuntimeDefault - the container runtime default profile should be used. Unconfined - no profile should be applied. |
| agent.config.securityContext.supplementalGroups | A list of groups applied to the first process run in each container, in addition to the container's primary GID.  If unspecified, no groups will be added to any container. Note that this field cannot be set when spec.os.name is windows. |
| agent.config.securityContext.sysctls | Sysctls hold a list of namespaced sysctls used for the pod. Pods with unsupported sysctls (by the container runtime) might fail to launch. Note that this field cannot be set when spec.os.name is windows. |
| agent.config.securityContext.windowsOptions.gmsaCredentialSpec | GMSACredentialSpec is where the GMSA admission webhook (https://github.com/kubernetes-sigs/windows-gmsa) inlines the contents of the GMSA credential spec named by the GMSACredentialSpecName field. |
| agent.config.securityContext.windowsOptions.gmsaCredentialSpecName | GMSACredentialSpecName is the name of the GMSA credential spec to use. |
| agent.config.securityContext.windowsOptions.hostProcess | HostProcess determines if a container should be run as a 'Host Process' container. This field is alpha-level and will only be honored by components that enable the WindowsHostProcessContainers feature flag. Setting this field without the feature flag will result in errors when validating the Pod. All of a Pod's containers must have the same effective HostProcess value (it is not allowed to have a mix of HostProcess containers and non-HostProcess containers).  In addition, if HostProcess is true then HostNetwork must also be set to true. |
| agent.config.securityContext.windowsOptions.runAsUserName | The UserName in Windows to run the entrypoint of the container process. Defaults to the user specified in image metadata if unspecified. May also be set in PodSecurityContext. If set in both SecurityContext and PodSecurityContext, the value specified in SecurityContext takes precedence. |
| agent.config.tags | List of tags to attach to every metric, event and service check collected by this Agent. Learn more about tagging: https://docs.datadoghq.com/tagging/ |
| agent.config.tolerations | If specified, the Agent pod's tolerations. |
| agent.config.volumeMounts | Specify additional volume mounts in the Datadog Agent container. |
| agent.config.volumes | Specify additional volumes in the Datadog Agent container. |
| agent.customConfig.configData | ConfigData corresponds to the configuration file content. |
| agent.customConfig.configMap.fileKey | FileKey corresponds to the key used in the ConfigMap.Data to store the configuration file content. |
| agent.customConfig.configMap.name | The name of source ConfigMap. |
| agent.daemonsetName | Name of the Daemonset to create or migrate from. |
| agent.deploymentStrategy.canary.autoFail.canaryTimeout | CanaryTimeout defines the maximum duration of a Canary, after which the Canary deployment is autofailed. This is a safeguard against lengthy Canary pauses. There is no default value. |
| agent.deploymentStrategy.canary.autoFail.enabled | Enabled enables AutoFail. Default value is true. |
| agent.deploymentStrategy.canary.autoFail.maxRestarts | MaxRestarts defines the number of tolerable (per pod) Canary pod restarts after which the Canary deployment is autofailed. Default value is 5. |
| agent.deploymentStrategy.canary.autoFail.maxRestartsDuration | MaxRestartsDuration defines the maximum duration of tolerable Canary pod restarts after which the Canary deployment is autofailed. There is no default value. |
| agent.deploymentStrategy.canary.autoPause.enabled | Enabled enables AutoPause. Default value is true. |
| agent.deploymentStrategy.canary.autoPause.maxRestarts | MaxRestarts defines the number of tolerable (per pod) Canary pod restarts after which the Canary deployment is autopaused. Default value is 2. |
| agent.deploymentStrategy.canary.autoPause.maxSlowStartDuration | MaxSlowStartDuration defines the maximum slow start duration for a pod (stuck in Creating state) after which the Canary deployment is autopaused. There is no default value. |
| agent.deploymentStrategy.canary.duration |  |
| agent.deploymentStrategy.canary.noRestartsDuration | NoRestartsDuration defines min duration since last restart to end the canary phase. |
| agent.deploymentStrategy.canary.nodeAntiAffinityKeys |  |
| agent.deploymentStrategy.canary.nodeSelector.matchExpressions | matchExpressions is a list of label selector requirements. The requirements are ANDed. |
| agent.deploymentStrategy.canary.nodeSelector.matchLabels | matchLabels is a map of {key,value} pairs. A single {key,value} in the matchLabels map is equivalent to an element of matchExpressions, whose key field is "key", the operator is "In", and the values array contains only "value". The requirements are ANDed. |
| agent.deploymentStrategy.canary.replicas |  |
| agent.deploymentStrategy.canary.validationMode | ValidationMode used to configure how a canary deployment is validated. Possible values are 'auto' (default) and 'manual' |
| agent.deploymentStrategy.reconcileFrequency | The reconcile frequency of the ExtendDaemonSet. |
| agent.deploymentStrategy.rollingUpdate.maxParallelPodCreation | The maxium number of pods created in parallel. Default value is 250. |
| agent.deploymentStrategy.rollingUpdate.maxPodSchedulerFailure | MaxPodSchedulerFailure the maxinum number of not scheduled on its Node due to a scheduler failure: resource constraints. Value can be an absolute number (ex: 5) or a percentage of total number of DaemonSet pods at the start of the update (ex: 10%). Absolute |
| agent.deploymentStrategy.rollingUpdate.maxUnavailable | The maximum number of DaemonSet pods that can be unavailable during the update. Value can be an absolute number (ex: 5) or a percentage of total number of DaemonSet pods at the start of the update (ex: 10%). Absolute number is calculated from percentage by rounding up. This cannot be 0. Default value is 1. |
| agent.deploymentStrategy.rollingUpdate.slowStartAdditiveIncrease | SlowStartAdditiveIncrease Value can be an absolute number (ex: 5) or a percentage of total number of DaemonSet pods at the start of the update (ex: 10%). Default value is 5. |
| agent.deploymentStrategy.rollingUpdate.slowStartIntervalDuration | SlowStartIntervalDuration the duration between to 2 Default value is 1min. |
| agent.deploymentStrategy.updateStrategyType | The update strategy used for the DaemonSet. |
| agent.dnsConfig.nameservers | A list of DNS name server IP addresses. This will be appended to the base nameservers generated from DNSPolicy. Duplicated nameservers will be removed. |
| agent.dnsConfig.options | A list of DNS resolver options. This will be merged with the base options generated from DNSPolicy. Duplicated entries will be removed. Resolution options given in Options will override those that appear in the base DNSPolicy. |
| agent.dnsConfig.searches | A list of DNS search domains for host-name lookup. This will be appended to the base search paths generated from DNSPolicy. Duplicated search paths will be removed. |
| agent.dnsPolicy | Set DNS policy for the pod. Defaults to "ClusterFirst". Valid values are 'ClusterFirstWithHostNet', 'ClusterFirst', 'Default' or 'None'. DNS parameters given in DNSConfig will be merged with the policy selected with DNSPolicy. To have DNS options set along with hostNetwork, you have to specify DNS policy explicitly to 'ClusterFirstWithHostNet'. |
| agent.enabled | Enabled |
| agent.env | Environment variables for all Datadog Agents. See also: https://docs.datadoghq.com/agent/docker/?tab=standard#environment-variables |
| agent.hostNetwork | Host networking requested for this pod. Use the host's network namespace. If this option is set, the ports that will be used must be specified. Default to false. |
| agent.hostPID | Use the host's pid namespace. Optional: Default to false. |
| agent.image.jmxEnabled | Define whether the Agent image should support JMX. |
| agent.image.name | Define the image to use: Use "gcr.io/datadoghq/agent:latest" for Datadog Agent 7. Use "datadog/dogstatsd:latest" for standalone Datadog Agent DogStatsD 7. Use "gcr.io/datadoghq/cluster-agent:latest" for Datadog Cluster Agent. Use "agent" with the registry and tag configurations for <registry>/agent:<tag>. Use "cluster-agent" with the registry and tag configurations for <registry>/cluster-agent:<tag>. |
| agent.image.pullPolicy | The Kubernetes pull policy: Use Always, Never or IfNotPresent. |
| agent.image.pullSecrets | It is possible to specify Docker registry credentials. See https://kubernetes.io/docs/concepts/containers/images/#specifying-imagepullsecrets-on-a-pod |
| agent.image.tag | Define the image tag to use. To be used if the Name field does not correspond to a full image string. |
| agent.keepAnnotations | KeepAnnotations allows the specification of annotations not managed by the Operator that will be kept on Agent DaemonSet. All annotations containing 'datadoghq.com' are always included. This field uses glob syntax. |
| agent.keepLabels | KeepLabels allows the specification of labels not managed by the Operator that will be kept on Agent DaemonSet. All labels containing 'datadoghq.com' are always included. This field uses glob syntax. |
| agent.localService.forceLocalServiceEnable | Force the creation of the internal traffic policy service to target the agent running on the local node. By default, the internal traffic service is created only on Kubernetes 1.22+ where the feature became beta and enabled by default. This option allows to force the creation of the internal traffic service on kubernetes 1.21 where the feature was alpha and required a feature gate to be explicitly enabled. |
| agent.localService.overrideName | Name of the internal traffic service to target the agent running on the local node |
| agent.log.containerCollectUsingFiles | Collect logs from files in `/var/log/pods instead` of using the container runtime API. Collecting logs from files is usually the most efficient way of collecting logs. See also: https://docs.datadoghq.com/agent/basic_agent_usage/kubernetes/#log-collection-setup Default is true |
| agent.log.containerLogsPath | Allows log collection from the container log path. Set to a different path if you are not using the Docker runtime. See also: https://docs.datadoghq.com/agent/kubernetes/daemonset_setup/?tab=k8sfile#create-manifest Defaults to `/var/lib/docker/containers` |
| agent.log.containerSymlinksPath | Allows the log collection to use symbolic links in this directory to validate container ID -> pod. Defaults to `/var/log/containers` |
| agent.log.enabled | Enable this option to activate Datadog Agent log collection. See also: https://docs.datadoghq.com/agent/basic_agent_usage/kubernetes/#log-collection-setup |
| agent.log.logsConfigContainerCollectAll | Enable this option to allow log collection for all containers. See also: https://docs.datadoghq.com/agent/basic_agent_usage/kubernetes/#log-collection-setup |
| agent.log.openFilesLimit | Sets the maximum number of log files that the Datadog Agent tails. Increasing this limit can increase resource consumption of the Agent. See also: https://docs.datadoghq.com/agent/basic_agent_usage/kubernetes/#log-collection-setup Default is 100 |
| agent.log.podLogsPath | Allows log collection from pod log path. Defaults to `/var/log/pods`. |
| agent.log.tempStoragePath | This path (always mounted from the host) is used by Datadog Agent to store information about processed log files. If the Datadog Agent is restarted, it starts tailing the log files immediately. Default to `/var/lib/datadog-agent/logs` |
| agent.networkPolicy.create | If true, create a NetworkPolicy for the current agent. |
| agent.networkPolicy.dnsSelectorEndpoints | Cilium selector of the DNS server entity. |
| agent.networkPolicy.flavor | Which network policy to use. Can be `kubernetes` or `cilium`. |
| agent.priorityClassName | If specified, indicates the pod's priority. "system-node-critical" and "system-cluster-critical" are two special keywords which indicate the highest priorities with the former being the highest priority. Any other name must be defined by creating a PriorityClass object with that name. If not specified, the pod priority will be default or zero if there is no default. |
| agent.process.args | Args allows the specification of extra args to `Command` parameter |
| agent.process.command | Command allows the specification of custom entrypoint for Process Agent container |
| agent.process.enabled | Enable the Process Agent container. See also: https://docs.datadoghq.com/infrastructure/process/?tab=kubernetes#installation |
| agent.process.env | The Datadog Agent supports many environment variables. See also: https://docs.datadoghq.com/agent/docker/?tab=standard#environment-variables |
| agent.process.processCollectionEnabled | false (default): Only collect containers if available. true: collect process information as well. Note: If enabled, /etc/passwd is automatically mounted to allow username resolution. |
| agent.process.resources.limits | Limits describes the maximum amount of compute resources allowed. More info: https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/ |
| agent.process.resources.requests | Requests describes the minimum amount of compute resources required. If Requests is omitted for a container, it defaults to Limits if that is explicitly specified, otherwise to an implementation-defined value. More info: https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/ |
| agent.process.volumeMounts | Specify additional volume mounts in the Process Agent container. |
| agent.rbac.create | Used to configure RBAC resources creation. |
| agent.rbac.serviceAccountName | Used to set up the service account name to use. Ignored if the field Create is true. |
| agent.security.args | Args allows the specification of extra args to `Command` parameter |
| agent.security.command | Command allows the specification of custom entrypoint for Security Agent container |
| agent.security.compliance.checkInterval | Check interval. |
| agent.security.compliance.configDir.configMapName | ConfigMapName name of a ConfigMap used to mount a directory. |
| agent.security.compliance.configDir.items | items mapping between configMap data key and file path mount. |
| agent.security.compliance.enabled | Enables continuous compliance monitoring. |
| agent.security.env | The Datadog Security Agent supports many environment variables. See also: https://docs.datadoghq.com/agent/docker/?tab=standard#environment-variables |
| agent.security.resources.limits | Limits describes the maximum amount of compute resources allowed. More info: https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/ |
| agent.security.resources.requests | Requests describes the minimum amount of compute resources required. If Requests is omitted for a container, it defaults to Limits if that is explicitly specified, otherwise to an implementation-defined value. More info: https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/ |
| agent.security.runtime.enabled | Enables runtime security features. |
| agent.security.runtime.policiesDir.configMapName | ConfigMapName name of a ConfigMap used to mount a directory. |
| agent.security.runtime.policiesDir.items | items mapping between configMap data key and file path mount. |
| agent.security.runtime.syscallMonitor.enabled | Enabled enables syscall monitor |
| agent.security.volumeMounts | Specify additional volume mounts in the Security Agent container. |
| agent.systemProbe.appArmorProfileName | AppArmorProfileName specify a apparmor profile. |
| agent.systemProbe.args | Args allows the specification of extra args to `Command` parameter |
| agent.systemProbe.bpfDebugEnabled | BPFDebugEnabled logging for kernel debug. |
| agent.systemProbe.collectDNSStats | CollectDNSStats enables DNS stat collection. |
| agent.systemProbe.command | Command allows the specification of custom entrypoint for System Probe container |
| agent.systemProbe.conntrackEnabled | ConntrackEnabled enable the system-probe agent to connect to the netlink/conntrack subsystem to add NAT information to connection data. See also: http://conntrack-tools.netfilter.org/ |
| agent.systemProbe.customConfig.configData | ConfigData corresponds to the configuration file content. |
| agent.systemProbe.customConfig.configMap.fileKey | FileKey corresponds to the key used in the ConfigMap.Data to store the configuration file content. |
| agent.systemProbe.customConfig.configMap.name | The name of source ConfigMap. |
| agent.systemProbe.debugPort | DebugPort Specify the port to expose pprof and expvar for system-probe agent. |
| agent.systemProbe.enableOOMKill | EnableOOMKill enables the OOM kill eBPF-based check. |
| agent.systemProbe.enableTCPQueueLength | EnableTCPQueueLength enables the TCP queue length eBPF-based check. |
| agent.systemProbe.enabled | Enable this to activate live process monitoring. Note: /etc/passwd is automatically mounted to allow username resolution. See also: https://docs.datadoghq.com/infrastructure/process/?tab=kubernetes#installation |
| agent.systemProbe.env | The Datadog SystemProbe supports many environment variables. See also: https://docs.datadoghq.com/agent/docker/?tab=standard#environment-variables |
| agent.systemProbe.resources.limits | Limits describes the maximum amount of compute resources allowed. More info: https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/ |
| agent.systemProbe.resources.requests | Requests describes the minimum amount of compute resources required. If Requests is omitted for a container, it defaults to Limits if that is explicitly specified, otherwise to an implementation-defined value. More info: https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/ |
| agent.systemProbe.secCompCustomProfileConfigMap | SecCompCustomProfileConfigMap specify a pre-existing ConfigMap containing a custom SecComp profile. This ConfigMap must contain a file named system-probe-seccomp.json. |
| agent.systemProbe.secCompProfileName | SecCompProfileName specify a seccomp profile. |
| agent.systemProbe.secCompRootPath | SecCompRootPath specify the seccomp profile root directory. |
| agent.systemProbe.securityContext.allowPrivilegeEscalation | AllowPrivilegeEscalation controls whether a process can gain more privileges than its parent process. This bool directly controls if the no_new_privs flag will be set on the container process. AllowPrivilegeEscalation is true always when the container is: 1) run as Privileged 2) has CAP_SYS_ADMIN Note that this field cannot be set when spec.os.name is windows. |
| agent.systemProbe.securityContext.capabilities.add | Added capabilities |
| agent.systemProbe.securityContext.capabilities.drop | Removed capabilities |
| agent.systemProbe.securityContext.privileged | Run container in privileged mode. Processes in privileged containers are essentially equivalent to root on the host. Defaults to false. Note that this field cannot be set when spec.os.name is windows. |
| agent.systemProbe.securityContext.procMount | procMount denotes the type of proc mount to use for the containers. The default is DefaultProcMount which uses the container runtime defaults for readonly paths and masked paths. This requires the ProcMountType feature flag to be enabled. Note that this field cannot be set when spec.os.name is windows. |
| agent.systemProbe.securityContext.readOnlyRootFilesystem | Whether this container has a read-only root filesystem. Default is false. Note that this field cannot be set when spec.os.name is windows. |
| agent.systemProbe.securityContext.runAsGroup | The GID to run the entrypoint of the container process. Uses runtime default if unset. May also be set in PodSecurityContext.  If set in both SecurityContext and PodSecurityContext, the value specified in SecurityContext takes precedence. Note that this field cannot be set when spec.os.name is windows. |
| agent.systemProbe.securityContext.runAsNonRoot | Indicates that the container must run as a non-root user. If true, the Kubelet will validate the image at runtime to ensure that it does not run as UID 0 (root) and fail to start the container if it does. If unset or false, no such validation will be performed. May also be set in PodSecurityContext.  If set in both SecurityContext and PodSecurityContext, the value specified in SecurityContext takes precedence. |
| agent.systemProbe.securityContext.runAsUser | The UID to run the entrypoint of the container process. Defaults to user specified in image metadata if unspecified. May also be set in PodSecurityContext.  If set in both SecurityContext and PodSecurityContext, the value specified in SecurityContext takes precedence. Note that this field cannot be set when spec.os.name is windows. |
| agent.systemProbe.securityContext.seLinuxOptions.level | Level is SELinux level label that applies to the container. |
| agent.systemProbe.securityContext.seLinuxOptions.role | Role is a SELinux role label that applies to the container. |
| agent.systemProbe.securityContext.seLinuxOptions.type | Type is a SELinux type label that applies to the container. |
| agent.systemProbe.securityContext.seLinuxOptions.user | User is a SELinux user label that applies to the container. |
| agent.systemProbe.securityContext.seccompProfile.localhostProfile | localhostProfile indicates a profile defined in a file on the node should be used. The profile must be preconfigured on the node to work. Must be a descending path, relative to the kubelet's configured seccomp profile location. Must only be set if type is "Localhost". |
| agent.systemProbe.securityContext.seccompProfile.type | type indicates which kind of seccomp profile will be applied. Valid options are:  Localhost - a profile defined in a file on the node should be used. RuntimeDefault - the container runtime default profile should be used. Unconfined - no profile should be applied. |
| agent.systemProbe.securityContext.windowsOptions.gmsaCredentialSpec | GMSACredentialSpec is where the GMSA admission webhook (https://github.com/kubernetes-sigs/windows-gmsa) inlines the contents of the GMSA credential spec named by the GMSACredentialSpecName field. |
| agent.systemProbe.securityContext.windowsOptions.gmsaCredentialSpecName | GMSACredentialSpecName is the name of the GMSA credential spec to use. |
| agent.systemProbe.securityContext.windowsOptions.hostProcess | HostProcess determines if a container should be run as a 'Host Process' container. This field is alpha-level and will only be honored by components that enable the WindowsHostProcessContainers feature flag. Setting this field without the feature flag will result in errors when validating the Pod. All of a Pod's containers must have the same effective HostProcess value (it is not allowed to have a mix of HostProcess containers and non-HostProcess containers).  In addition, if HostProcess is true then HostNetwork must also be set to true. |
| agent.systemProbe.securityContext.windowsOptions.runAsUserName | The UserName in Windows to run the entrypoint of the container process. Defaults to the user specified in image metadata if unspecified. May also be set in PodSecurityContext. If set in both SecurityContext and PodSecurityContext, the value specified in SecurityContext takes precedence. |
| agent.systemProbe.volumeMounts | Specify additional volume mounts in the Security Agent container. |
| agent.useExtendedDaemonset | UseExtendedDaemonset use ExtendedDaemonset for Agent deployment. default value is false. |
| clusterAgent.additionalAnnotations | AdditionalAnnotations provide annotations that will be added to the Cluster Agent Pods. |
| clusterAgent.additionalLabels | AdditionalLabels provide labels that will be added to the Cluster Agent Pods. |
| clusterAgent.affinity.nodeAffinity.preferredDuringSchedulingIgnoredDuringExecution | The scheduler will prefer to schedule pods to nodes that satisfy the affinity expressions specified by this field, but it may choose a node that violates one or more of the expressions. The node that is most preferred is the one with the greatest sum of weights, i.e. for each node that meets all of the scheduling requirements (resource request, requiredDuringScheduling affinity expressions, etc.), compute a sum by iterating through the elements of this field and adding "weight" to the sum if the node matches the corresponding matchExpressions; the node(s) with the highest sum are the most preferred. |
| clusterAgent.affinity.nodeAffinity.requiredDuringSchedulingIgnoredDuringExecution.nodeSelectorTerms | Required. A list of node selector terms. The terms are ORed. |
| clusterAgent.affinity.podAffinity.preferredDuringSchedulingIgnoredDuringExecution | The scheduler will prefer to schedule pods to nodes that satisfy the affinity expressions specified by this field, but it may choose a node that violates one or more of the expressions. The node that is most preferred is the one with the greatest sum of weights, i.e. for each node that meets all of the scheduling requirements (resource request, requiredDuringScheduling affinity expressions, etc.), compute a sum by iterating through the elements of this field and adding "weight" to the sum if the node has pods which matches the corresponding podAffinityTerm; the node(s) with the highest sum are the most preferred. |
| clusterAgent.affinity.podAffinity.requiredDuringSchedulingIgnoredDuringExecution | If the affinity requirements specified by this field are not met at scheduling time, the pod will not be scheduled onto the node. If the affinity requirements specified by this field cease to be met at some point during pod execution (e.g. due to a pod label update), the system may or may not try to eventually evict the pod from its node. When there are multiple elements, the lists of nodes corresponding to each podAffinityTerm are intersected, i.e. all terms must be satisfied. |
| clusterAgent.affinity.podAntiAffinity.preferredDuringSchedulingIgnoredDuringExecution | The scheduler will prefer to schedule pods to nodes that satisfy the anti-affinity expressions specified by this field, but it may choose a node that violates one or more of the expressions. The node that is most preferred is the one with the greatest sum of weights, i.e. for each node that meets all of the scheduling requirements (resource request, requiredDuringScheduling anti-affinity expressions, etc.), compute a sum by iterating through the elements of this field and adding "weight" to the sum if the node has pods which matches the corresponding podAffinityTerm; the node(s) with the highest sum are the most preferred. |
| clusterAgent.affinity.podAntiAffinity.requiredDuringSchedulingIgnoredDuringExecution | If the anti-affinity requirements specified by this field are not met at scheduling time, the pod will not be scheduled onto the node. If the anti-affinity requirements specified by this field cease to be met at some point during pod execution (e.g. due to a pod label update), the system may or may not try to eventually evict the pod from its node. When there are multiple elements, the lists of nodes corresponding to each podAffinityTerm are intersected, i.e. all terms must be satisfied. |
| clusterAgent.config.admissionController.agentCommunicationMode | agentCommunicationMode corresponds to the mode used by the Datadog application libraries to communicate with the Agent. It can be "hostip", "service", or "socket". |
| clusterAgent.config.admissionController.enabled | Enable the admission controller to be able to inject APM/Dogstatsd config and standard tags (env, service, version) automatically into your pods. |
| clusterAgent.config.admissionController.mutateUnlabelled | MutateUnlabelled enables injecting config without having the pod label 'admission.datadoghq.com/enabled="true"'. |
| clusterAgent.config.admissionController.serviceName | ServiceName corresponds to the webhook service name. |
| clusterAgent.config.args | Args allows the specification of extra args to `Command` parameter |
| clusterAgent.config.clusterChecksEnabled | Enable the Cluster Checks and Endpoint Checks feature on both the Cluster Agent and the daemonset. See also: https://docs.datadoghq.com/agent/cluster_agent/clusterchecks/ https://docs.datadoghq.com/agent/cluster_agent/endpointschecks/ Autodiscovery via Kube Service annotations is automatically enabled. |
| clusterAgent.config.collectEvents | Enable this to start event collection from the kubernetes API. See also: https://docs.datadoghq.com/agent/cluster_agent/event_collection/ |
| clusterAgent.config.command | Command allows the specification of custom entrypoint for Cluster Agent container |
| clusterAgent.config.confd.configMapName | ConfigMapName name of a ConfigMap used to mount a directory. |
| clusterAgent.config.confd.items | items mapping between configMap data key and file path mount. |
| clusterAgent.config.env | The Datadog Agent supports many environment variables. See also: https://docs.datadoghq.com/agent/docker/?tab=standard#environment-variables |
| clusterAgent.config.externalMetrics.credentials.apiKey | APIKey Set this to your Datadog API key before the Agent runs. See also: https://app.datadoghq.com/account/settings#agent/kubernetes |
| clusterAgent.config.externalMetrics.credentials.apiKeyExistingSecret | APIKeyExistingSecret is DEPRECATED. In order to pass the API key through an existing secret, please consider "apiSecret" instead. If set, this parameter takes precedence over "apiKey". |
| clusterAgent.config.externalMetrics.credentials.apiSecret.keyName | KeyName is the key of the secret to use. |
| clusterAgent.config.externalMetrics.credentials.apiSecret.secretName | SecretName is the name of the secret. |
| clusterAgent.config.externalMetrics.credentials.appKey | If you are using clusterAgent.metricsProvider.enabled = true, you must set a Datadog application key for read access to your metrics. |
| clusterAgent.config.externalMetrics.credentials.appKeyExistingSecret | AppKeyExistingSecret is DEPRECATED. In order to pass the APP key through an existing secret, please consider "appSecret" instead. If set, this parameter takes precedence over "appKey". |
| clusterAgent.config.externalMetrics.credentials.appSecret.keyName | KeyName is the key of the secret to use. |
| clusterAgent.config.externalMetrics.credentials.appSecret.secretName | SecretName is the name of the secret. |
| clusterAgent.config.externalMetrics.enabled | Enable the metricsProvider to be able to scale based on metrics in Datadog. |
| clusterAgent.config.externalMetrics.endpoint | Override the API endpoint for the external metrics server. Defaults to .spec.agent.config.ddUrl or "https://app.datadoghq.com" if that's empty. |
| clusterAgent.config.externalMetrics.port | If specified configures the metricsProvider external metrics service port. |
| clusterAgent.config.externalMetrics.useDatadogMetrics | Enable usage of DatadogMetrics CRD (allow to scale on arbitrary queries). |
| clusterAgent.config.externalMetrics.wpaController | Enable informer and controller of the watermark pod autoscaler. NOTE: The WatermarkPodAutoscaler controller needs to be installed. See also: https://github.com/DataDog/watermarkpodautoscaler. |
| clusterAgent.config.healthPort | HealthPort of the Agent container for internal liveness probe. Must be the same as the Liveness/Readiness probes. |
| clusterAgent.config.logLevel | Set logging verbosity, valid log levels are: trace, debug, info, warn, error, critical, and off |
| clusterAgent.config.resources.limits | Limits describes the maximum amount of compute resources allowed. More info: https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/ |
| clusterAgent.config.resources.requests | Requests describes the minimum amount of compute resources required. If Requests is omitted for a container, it defaults to Limits if that is explicitly specified, otherwise to an implementation-defined value. More info: https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/ |
| clusterAgent.config.securityContext.fsGroup | A special supplemental group that applies to all containers in a pod. Some volume types allow the Kubelet to change the ownership of that volume to be owned by the pod:  1. The owning GID will be the FSGroup 2. The setgid bit is set (new files created in the volume will be owned by FSGroup) 3. The permission bits are OR'd with rw-rw----  If unset, the Kubelet will not modify the ownership and permissions of any volume. Note that this field cannot be set when spec.os.name is windows. |
| clusterAgent.config.securityContext.fsGroupChangePolicy | fsGroupChangePolicy defines behavior of changing ownership and permission of the volume before being exposed inside Pod. This field will only apply to volume types which support fsGroup based ownership(and permissions). It will have no effect on ephemeral volume types such as: secret, configmaps and emptydir. Valid values are "OnRootMismatch" and "Always". If not specified, "Always" is used. Note that this field cannot be set when spec.os.name is windows. |
| clusterAgent.config.securityContext.runAsGroup | The GID to run the entrypoint of the container process. Uses runtime default if unset. May also be set in SecurityContext.  If set in both SecurityContext and PodSecurityContext, the value specified in SecurityContext takes precedence for that container. Note that this field cannot be set when spec.os.name is windows. |
| clusterAgent.config.securityContext.runAsNonRoot | Indicates that the container must run as a non-root user. If true, the Kubelet will validate the image at runtime to ensure that it does not run as UID 0 (root) and fail to start the container if it does. If unset or false, no such validation will be performed. May also be set in SecurityContext.  If set in both SecurityContext and PodSecurityContext, the value specified in SecurityContext takes precedence. |
| clusterAgent.config.securityContext.runAsUser | The UID to run the entrypoint of the container process. Defaults to user specified in image metadata if unspecified. May also be set in SecurityContext.  If set in both SecurityContext and PodSecurityContext, the value specified in SecurityContext takes precedence for that container. Note that this field cannot be set when spec.os.name is windows. |
| clusterAgent.config.securityContext.seLinuxOptions.level | Level is SELinux level label that applies to the container. |
| clusterAgent.config.securityContext.seLinuxOptions.role | Role is a SELinux role label that applies to the container. |
| clusterAgent.config.securityContext.seLinuxOptions.type | Type is a SELinux type label that applies to the container. |
| clusterAgent.config.securityContext.seLinuxOptions.user | User is a SELinux user label that applies to the container. |
| clusterAgent.config.securityContext.seccompProfile.localhostProfile | localhostProfile indicates a profile defined in a file on the node should be used. The profile must be preconfigured on the node to work. Must be a descending path, relative to the kubelet's configured seccomp profile location. Must only be set if type is "Localhost". |
| clusterAgent.config.securityContext.seccompProfile.type | type indicates which kind of seccomp profile will be applied. Valid options are:  Localhost - a profile defined in a file on the node should be used. RuntimeDefault - the container runtime default profile should be used. Unconfined - no profile should be applied. |
| clusterAgent.config.securityContext.supplementalGroups | A list of groups applied to the first process run in each container, in addition to the container's primary GID.  If unspecified, no groups will be added to any container. Note that this field cannot be set when spec.os.name is windows. |
| clusterAgent.config.securityContext.sysctls | Sysctls hold a list of namespaced sysctls used for the pod. Pods with unsupported sysctls (by the container runtime) might fail to launch. Note that this field cannot be set when spec.os.name is windows. |
| clusterAgent.config.securityContext.windowsOptions.gmsaCredentialSpec | GMSACredentialSpec is where the GMSA admission webhook (https://github.com/kubernetes-sigs/windows-gmsa) inlines the contents of the GMSA credential spec named by the GMSACredentialSpecName field. |
| clusterAgent.config.securityContext.windowsOptions.gmsaCredentialSpecName | GMSACredentialSpecName is the name of the GMSA credential spec to use. |
| clusterAgent.config.securityContext.windowsOptions.hostProcess | HostProcess determines if a container should be run as a 'Host Process' container. This field is alpha-level and will only be honored by components that enable the WindowsHostProcessContainers feature flag. Setting this field without the feature flag will result in errors when validating the Pod. All of a Pod's containers must have the same effective HostProcess value (it is not allowed to have a mix of HostProcess containers and non-HostProcess containers).  In addition, if HostProcess is true then HostNetwork must also be set to true. |
| clusterAgent.config.securityContext.windowsOptions.runAsUserName | The UserName in Windows to run the entrypoint of the container process. Defaults to the user specified in image metadata if unspecified. May also be set in PodSecurityContext. If set in both SecurityContext and PodSecurityContext, the value specified in SecurityContext takes precedence. |
| clusterAgent.config.volumeMounts | Specify additional volume mounts in the Datadog Cluster Agent container. |
| clusterAgent.config.volumes | Specify additional volumes in the Datadog Cluster Agent container. |
| clusterAgent.customConfig.configData | ConfigData corresponds to the configuration file content. |
| clusterAgent.customConfig.configMap.fileKey | FileKey corresponds to the key used in the ConfigMap.Data to store the configuration file content. |
| clusterAgent.customConfig.configMap.name | The name of source ConfigMap. |
| clusterAgent.deploymentName | Name of the Cluster Agent Deployment to create or migrate from. |
| clusterAgent.enabled | Enabled |
| clusterAgent.image.jmxEnabled | Define whether the Agent image should support JMX. |
| clusterAgent.image.name | Define the image to use: Use "gcr.io/datadoghq/agent:latest" for Datadog Agent 7. Use "datadog/dogstatsd:latest" for standalone Datadog Agent DogStatsD 7. Use "gcr.io/datadoghq/cluster-agent:latest" for Datadog Cluster Agent. Use "agent" with the registry and tag configurations for <registry>/agent:<tag>. Use "cluster-agent" with the registry and tag configurations for <registry>/cluster-agent:<tag>. |
| clusterAgent.image.pullPolicy | The Kubernetes pull policy: Use Always, Never or IfNotPresent. |
| clusterAgent.image.pullSecrets | It is possible to specify Docker registry credentials. See https://kubernetes.io/docs/concepts/containers/images/#specifying-imagepullsecrets-on-a-pod |
| clusterAgent.image.tag | Define the image tag to use. To be used if the Name field does not correspond to a full image string. |
| clusterAgent.keepAnnotations | KeepAnnotations allows the specification of annotations not managed by the Operator that will be kept on ClusterAgent Deployment. All annotations containing 'datadoghq.com' are always included. This field uses glob syntax. |
| clusterAgent.keepLabels | KeepLabels allows the specification of labels not managed by the Operator that will be kept on ClusterAgent Deployment. All labels containing 'datadoghq.com' are always included. This field uses glob syntax. |
| clusterAgent.networkPolicy.create | If true, create a NetworkPolicy for the current agent. |
| clusterAgent.networkPolicy.dnsSelectorEndpoints | Cilium selector of the DNS server entity. |
| clusterAgent.networkPolicy.flavor | Which network policy to use. Can be `kubernetes` or `cilium`. |
| clusterAgent.nodeSelector | NodeSelector is a selector which must be true for the pod to fit on a node. Selector which must match a node's labels for the pod to be scheduled on that node. More info: https://kubernetes.io/docs/concepts/configuration/assign-pod-node/ |
| clusterAgent.priorityClassName | If specified, indicates the pod's priority. "system-node-critical" and "system-cluster-critical" are two special keywords which indicate the highest priorities with the former being the highest priority. Any other name must be defined by creating a PriorityClass object with that name. If not specified, the pod priority will be default or zero if there is no default. |
| clusterAgent.rbac.create | Used to configure RBAC resources creation. |
| clusterAgent.rbac.serviceAccountName | Used to set up the service account name to use. Ignored if the field Create is true. |
| clusterAgent.replicas | Number of the Cluster Agent replicas. |
| clusterAgent.tolerations | If specified, the Cluster-Agent pod's tolerations. |
| clusterChecksRunner.additionalAnnotations | AdditionalAnnotations provide annotations that will be added to the cluster checks runner Pods. |
| clusterChecksRunner.additionalLabels | AdditionalLabels provide labels that will be added to the cluster checks runner Pods. |
| clusterChecksRunner.affinity.nodeAffinity.preferredDuringSchedulingIgnoredDuringExecution | The scheduler will prefer to schedule pods to nodes that satisfy the affinity expressions specified by this field, but it may choose a node that violates one or more of the expressions. The node that is most preferred is the one with the greatest sum of weights, i.e. for each node that meets all of the scheduling requirements (resource request, requiredDuringScheduling affinity expressions, etc.), compute a sum by iterating through the elements of this field and adding "weight" to the sum if the node matches the corresponding matchExpressions; the node(s) with the highest sum are the most preferred. |
| clusterChecksRunner.affinity.nodeAffinity.requiredDuringSchedulingIgnoredDuringExecution.nodeSelectorTerms | Required. A list of node selector terms. The terms are ORed. |
| clusterChecksRunner.affinity.podAffinity.preferredDuringSchedulingIgnoredDuringExecution | The scheduler will prefer to schedule pods to nodes that satisfy the affinity expressions specified by this field, but it may choose a node that violates one or more of the expressions. The node that is most preferred is the one with the greatest sum of weights, i.e. for each node that meets all of the scheduling requirements (resource request, requiredDuringScheduling affinity expressions, etc.), compute a sum by iterating through the elements of this field and adding "weight" to the sum if the node has pods which matches the corresponding podAffinityTerm; the node(s) with the highest sum are the most preferred. |
| clusterChecksRunner.affinity.podAffinity.requiredDuringSchedulingIgnoredDuringExecution | If the affinity requirements specified by this field are not met at scheduling time, the pod will not be scheduled onto the node. If the affinity requirements specified by this field cease to be met at some point during pod execution (e.g. due to a pod label update), the system may or may not try to eventually evict the pod from its node. When there are multiple elements, the lists of nodes corresponding to each podAffinityTerm are intersected, i.e. all terms must be satisfied. |
| clusterChecksRunner.affinity.podAntiAffinity.preferredDuringSchedulingIgnoredDuringExecution | The scheduler will prefer to schedule pods to nodes that satisfy the anti-affinity expressions specified by this field, but it may choose a node that violates one or more of the expressions. The node that is most preferred is the one with the greatest sum of weights, i.e. for each node that meets all of the scheduling requirements (resource request, requiredDuringScheduling anti-affinity expressions, etc.), compute a sum by iterating through the elements of this field and adding "weight" to the sum if the node has pods which matches the corresponding podAffinityTerm; the node(s) with the highest sum are the most preferred. |
| clusterChecksRunner.affinity.podAntiAffinity.requiredDuringSchedulingIgnoredDuringExecution | If the anti-affinity requirements specified by this field are not met at scheduling time, the pod will not be scheduled onto the node. If the anti-affinity requirements specified by this field cease to be met at some point during pod execution (e.g. due to a pod label update), the system may or may not try to eventually evict the pod from its node. When there are multiple elements, the lists of nodes corresponding to each podAffinityTerm are intersected, i.e. all terms must be satisfied. |
| clusterChecksRunner.config.args | Args allows the specification of extra args to `Command` parameter |
| clusterChecksRunner.config.command | Command allows the specification of custom entrypoint for Cluster Checks Runner container |
| clusterChecksRunner.config.env | The Datadog Agent supports many environment variables. See also: https://docs.datadoghq.com/agent/docker/?tab=standard#environment-variables |
| clusterChecksRunner.config.healthPort | HealthPort of the Agent container for internal liveness probe. Must be the same as the Liveness/Readiness probes. |
| clusterChecksRunner.config.livenessProbe.exec.command | Command is the command line to execute inside the container, the working directory for the command  is root ('/') in the container's filesystem. The command is simply exec'd, it is not run inside a shell, so traditional shell instructions ('|', etc) won't work. To use a shell, you need to explicitly call out to that shell. Exit status of 0 is treated as live/healthy and non-zero is unhealthy. |
| clusterChecksRunner.config.livenessProbe.failureThreshold | Minimum consecutive failures for the probe to be considered failed after having succeeded. Defaults to 3. Minimum value is 1. |
| clusterChecksRunner.config.livenessProbe.grpc.port | Port number of the gRPC service. Number must be in the range 1 to 65535. |
| clusterChecksRunner.config.livenessProbe.grpc.service | Service is the name of the service to place in the gRPC HealthCheckRequest (see https://github.com/grpc/grpc/blob/master/doc/health-checking.md).  If this is not specified, the default behavior is defined by gRPC. |
| clusterChecksRunner.config.livenessProbe.httpGet.host | Host name to connect to, defaults to the pod IP. You probably want to set "Host" in httpHeaders instead. |
| clusterChecksRunner.config.livenessProbe.httpGet.httpHeaders | Custom headers to set in the request. HTTP allows repeated headers. |
| clusterChecksRunner.config.livenessProbe.httpGet.path | Path to access on the HTTP server. |
| clusterChecksRunner.config.livenessProbe.httpGet.port | Name or number of the port to access on the container. Number must be in the range 1 to 65535. Name must be an IANA_SVC_NAME. |
| clusterChecksRunner.config.livenessProbe.httpGet.scheme | Scheme to use for connecting to the host. Defaults to HTTP. |
| clusterChecksRunner.config.livenessProbe.initialDelaySeconds | Number of seconds after the container has started before liveness probes are initiated. More info: https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle#container-probes |
| clusterChecksRunner.config.livenessProbe.periodSeconds | How often (in seconds) to perform the probe. Default to 10 seconds. Minimum value is 1. |
| clusterChecksRunner.config.livenessProbe.successThreshold | Minimum consecutive successes for the probe to be considered successful after having failed. Defaults to 1. Must be 1 for liveness and startup. Minimum value is 1. |
| clusterChecksRunner.config.livenessProbe.tcpSocket.host | Optional: Host name to connect to, defaults to the pod IP. |
| clusterChecksRunner.config.livenessProbe.tcpSocket.port | Number or name of the port to access on the container. Number must be in the range 1 to 65535. Name must be an IANA_SVC_NAME. |
| clusterChecksRunner.config.livenessProbe.terminationGracePeriodSeconds | Optional duration in seconds the pod needs to terminate gracefully upon probe failure. The grace period is the duration in seconds after the processes running in the pod are sent a termination signal and the time when the processes are forcibly halted with a kill signal. Set this value longer than the expected cleanup time for your process. If this value is nil, the pod's terminationGracePeriodSeconds will be used. Otherwise, this value overrides the value provided by the pod spec. Value must be non-negative integer. The value zero indicates stop immediately via the kill signal (no opportunity to shut down). This is a beta field and requires enabling ProbeTerminationGracePeriod feature gate. Minimum value is 1. spec.terminationGracePeriodSeconds is used if unset. |
| clusterChecksRunner.config.livenessProbe.timeoutSeconds | Number of seconds after which the probe times out. Defaults to 1 second. Minimum value is 1. More info: https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle#container-probes |
| clusterChecksRunner.config.logLevel | Set logging verbosity, valid log levels are: trace, debug, info, warn, error, critical, and off |
| clusterChecksRunner.config.readinessProbe.exec.command | Command is the command line to execute inside the container, the working directory for the command  is root ('/') in the container's filesystem. The command is simply exec'd, it is not run inside a shell, so traditional shell instructions ('|', etc) won't work. To use a shell, you need to explicitly call out to that shell. Exit status of 0 is treated as live/healthy and non-zero is unhealthy. |
| clusterChecksRunner.config.readinessProbe.failureThreshold | Minimum consecutive failures for the probe to be considered failed after having succeeded. Defaults to 3. Minimum value is 1. |
| clusterChecksRunner.config.readinessProbe.grpc.port | Port number of the gRPC service. Number must be in the range 1 to 65535. |
| clusterChecksRunner.config.readinessProbe.grpc.service | Service is the name of the service to place in the gRPC HealthCheckRequest (see https://github.com/grpc/grpc/blob/master/doc/health-checking.md).  If this is not specified, the default behavior is defined by gRPC. |
| clusterChecksRunner.config.readinessProbe.httpGet.host | Host name to connect to, defaults to the pod IP. You probably want to set "Host" in httpHeaders instead. |
| clusterChecksRunner.config.readinessProbe.httpGet.httpHeaders | Custom headers to set in the request. HTTP allows repeated headers. |
| clusterChecksRunner.config.readinessProbe.httpGet.path | Path to access on the HTTP server. |
| clusterChecksRunner.config.readinessProbe.httpGet.port | Name or number of the port to access on the container. Number must be in the range 1 to 65535. Name must be an IANA_SVC_NAME. |
| clusterChecksRunner.config.readinessProbe.httpGet.scheme | Scheme to use for connecting to the host. Defaults to HTTP. |
| clusterChecksRunner.config.readinessProbe.initialDelaySeconds | Number of seconds after the container has started before liveness probes are initiated. More info: https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle#container-probes |
| clusterChecksRunner.config.readinessProbe.periodSeconds | How often (in seconds) to perform the probe. Default to 10 seconds. Minimum value is 1. |
| clusterChecksRunner.config.readinessProbe.successThreshold | Minimum consecutive successes for the probe to be considered successful after having failed. Defaults to 1. Must be 1 for liveness and startup. Minimum value is 1. |
| clusterChecksRunner.config.readinessProbe.tcpSocket.host | Optional: Host name to connect to, defaults to the pod IP. |
| clusterChecksRunner.config.readinessProbe.tcpSocket.port | Number or name of the port to access on the container. Number must be in the range 1 to 65535. Name must be an IANA_SVC_NAME. |
| clusterChecksRunner.config.readinessProbe.terminationGracePeriodSeconds | Optional duration in seconds the pod needs to terminate gracefully upon probe failure. The grace period is the duration in seconds after the processes running in the pod are sent a termination signal and the time when the processes are forcibly halted with a kill signal. Set this value longer than the expected cleanup time for your process. If this value is nil, the pod's terminationGracePeriodSeconds will be used. Otherwise, this value overrides the value provided by the pod spec. Value must be non-negative integer. The value zero indicates stop immediately via the kill signal (no opportunity to shut down). This is a beta field and requires enabling ProbeTerminationGracePeriod feature gate. Minimum value is 1. spec.terminationGracePeriodSeconds is used if unset. |
| clusterChecksRunner.config.readinessProbe.timeoutSeconds | Number of seconds after which the probe times out. Defaults to 1 second. Minimum value is 1. More info: https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle#container-probes |
| clusterChecksRunner.config.resources.limits | Limits describes the maximum amount of compute resources allowed. More info: https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/ |
| clusterChecksRunner.config.resources.requests | Requests describes the minimum amount of compute resources required. If Requests is omitted for a container, it defaults to Limits if that is explicitly specified, otherwise to an implementation-defined value. More info: https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/ |
| clusterChecksRunner.config.securityContext.fsGroup | A special supplemental group that applies to all containers in a pod. Some volume types allow the Kubelet to change the ownership of that volume to be owned by the pod:  1. The owning GID will be the FSGroup 2. The setgid bit is set (new files created in the volume will be owned by FSGroup) 3. The permission bits are OR'd with rw-rw----  If unset, the Kubelet will not modify the ownership and permissions of any volume. Note that this field cannot be set when spec.os.name is windows. |
| clusterChecksRunner.config.securityContext.fsGroupChangePolicy | fsGroupChangePolicy defines behavior of changing ownership and permission of the volume before being exposed inside Pod. This field will only apply to volume types which support fsGroup based ownership(and permissions). It will have no effect on ephemeral volume types such as: secret, configmaps and emptydir. Valid values are "OnRootMismatch" and "Always". If not specified, "Always" is used. Note that this field cannot be set when spec.os.name is windows. |
| clusterChecksRunner.config.securityContext.runAsGroup | The GID to run the entrypoint of the container process. Uses runtime default if unset. May also be set in SecurityContext.  If set in both SecurityContext and PodSecurityContext, the value specified in SecurityContext takes precedence for that container. Note that this field cannot be set when spec.os.name is windows. |
| clusterChecksRunner.config.securityContext.runAsNonRoot | Indicates that the container must run as a non-root user. If true, the Kubelet will validate the image at runtime to ensure that it does not run as UID 0 (root) and fail to start the container if it does. If unset or false, no such validation will be performed. May also be set in SecurityContext.  If set in both SecurityContext and PodSecurityContext, the value specified in SecurityContext takes precedence. |
| clusterChecksRunner.config.securityContext.runAsUser | The UID to run the entrypoint of the container process. Defaults to user specified in image metadata if unspecified. May also be set in SecurityContext.  If set in both SecurityContext and PodSecurityContext, the value specified in SecurityContext takes precedence for that container. Note that this field cannot be set when spec.os.name is windows. |
| clusterChecksRunner.config.securityContext.seLinuxOptions.level | Level is SELinux level label that applies to the container. |
| clusterChecksRunner.config.securityContext.seLinuxOptions.role | Role is a SELinux role label that applies to the container. |
| clusterChecksRunner.config.securityContext.seLinuxOptions.type | Type is a SELinux type label that applies to the container. |
| clusterChecksRunner.config.securityContext.seLinuxOptions.user | User is a SELinux user label that applies to the container. |
| clusterChecksRunner.config.securityContext.seccompProfile.localhostProfile | localhostProfile indicates a profile defined in a file on the node should be used. The profile must be preconfigured on the node to work. Must be a descending path, relative to the kubelet's configured seccomp profile location. Must only be set if type is "Localhost". |
| clusterChecksRunner.config.securityContext.seccompProfile.type | type indicates which kind of seccomp profile will be applied. Valid options are:  Localhost - a profile defined in a file on the node should be used. RuntimeDefault - the container runtime default profile should be used. Unconfined - no profile should be applied. |
| clusterChecksRunner.config.securityContext.supplementalGroups | A list of groups applied to the first process run in each container, in addition to the container's primary GID.  If unspecified, no groups will be added to any container. Note that this field cannot be set when spec.os.name is windows. |
| clusterChecksRunner.config.securityContext.sysctls | Sysctls hold a list of namespaced sysctls used for the pod. Pods with unsupported sysctls (by the container runtime) might fail to launch. Note that this field cannot be set when spec.os.name is windows. |
| clusterChecksRunner.config.securityContext.windowsOptions.gmsaCredentialSpec | GMSACredentialSpec is where the GMSA admission webhook (https://github.com/kubernetes-sigs/windows-gmsa) inlines the contents of the GMSA credential spec named by the GMSACredentialSpecName field. |
| clusterChecksRunner.config.securityContext.windowsOptions.gmsaCredentialSpecName | GMSACredentialSpecName is the name of the GMSA credential spec to use. |
| clusterChecksRunner.config.securityContext.windowsOptions.hostProcess | HostProcess determines if a container should be run as a 'Host Process' container. This field is alpha-level and will only be honored by components that enable the WindowsHostProcessContainers feature flag. Setting this field without the feature flag will result in errors when validating the Pod. All of a Pod's containers must have the same effective HostProcess value (it is not allowed to have a mix of HostProcess containers and non-HostProcess containers).  In addition, if HostProcess is true then HostNetwork must also be set to true. |
| clusterChecksRunner.config.securityContext.windowsOptions.runAsUserName | The UserName in Windows to run the entrypoint of the container process. Defaults to the user specified in image metadata if unspecified. May also be set in PodSecurityContext. If set in both SecurityContext and PodSecurityContext, the value specified in SecurityContext takes precedence. |
| clusterChecksRunner.config.volumeMounts | Specify additional volume mounts in the Datadog Cluster Check Runner container. |
| clusterChecksRunner.config.volumes | Specify additional volumes in the Datadog Cluster Check Runner container. |
| clusterChecksRunner.customConfig.configData | ConfigData corresponds to the configuration file content. |
| clusterChecksRunner.customConfig.configMap.fileKey | FileKey corresponds to the key used in the ConfigMap.Data to store the configuration file content. |
| clusterChecksRunner.customConfig.configMap.name | The name of source ConfigMap. |
| clusterChecksRunner.deploymentName | Name of the cluster checks deployment to create or migrate from. |
| clusterChecksRunner.enabled | Enabled |
| clusterChecksRunner.image.jmxEnabled | Define whether the Agent image should support JMX. |
| clusterChecksRunner.image.name | Define the image to use: Use "gcr.io/datadoghq/agent:latest" for Datadog Agent 7. Use "datadog/dogstatsd:latest" for standalone Datadog Agent DogStatsD 7. Use "gcr.io/datadoghq/cluster-agent:latest" for Datadog Cluster Agent. Use "agent" with the registry and tag configurations for <registry>/agent:<tag>. Use "cluster-agent" with the registry and tag configurations for <registry>/cluster-agent:<tag>. |
| clusterChecksRunner.image.pullPolicy | The Kubernetes pull policy: Use Always, Never or IfNotPresent. |
| clusterChecksRunner.image.pullSecrets | It is possible to specify Docker registry credentials. See https://kubernetes.io/docs/concepts/containers/images/#specifying-imagepullsecrets-on-a-pod |
| clusterChecksRunner.image.tag | Define the image tag to use. To be used if the Name field does not correspond to a full image string. |
| clusterChecksRunner.networkPolicy.create | If true, create a NetworkPolicy for the current agent. |
| clusterChecksRunner.networkPolicy.dnsSelectorEndpoints | Cilium selector of the DNS server entity. |
| clusterChecksRunner.networkPolicy.flavor | Which network policy to use. Can be `kubernetes` or `cilium`. |
| clusterChecksRunner.nodeSelector | NodeSelector is a selector which must be true for the pod to fit on a node. Selector which must match a node's labels for the pod to be scheduled on that node. More info: https://kubernetes.io/docs/concepts/configuration/assign-pod-node/ |
| clusterChecksRunner.priorityClassName | If specified, indicates the pod's priority. "system-node-critical" and "system-cluster-critical" are two special keywords which indicate the highest priorities with the former being the highest priority. Any other name must be defined by creating a PriorityClass object with that name. If not specified, the pod priority will be default or zero if there is no default. |
| clusterChecksRunner.rbac.create | Used to configure RBAC resources creation. |
| clusterChecksRunner.rbac.serviceAccountName | Used to set up the service account name to use. Ignored if the field Create is true. |
| clusterChecksRunner.replicas | Number of the Cluster Checks Runner replicas. |
| clusterChecksRunner.tolerations | If specified, the Cluster-Checks pod's tolerations. |
| clusterName | Set a unique cluster name to allow scoping hosts and Cluster Checks Runner easily. |
| credentials.apiKey | APIKey Set this to your Datadog API key before the Agent runs. See also: https://app.datadoghq.com/account/settings#agent/kubernetes |
| credentials.apiKeyExistingSecret | APIKeyExistingSecret is DEPRECATED. In order to pass the API key through an existing secret, please consider "apiSecret" instead. If set, this parameter takes precedence over "apiKey". |
| credentials.apiSecret.keyName | KeyName is the key of the secret to use. |
| credentials.apiSecret.secretName | SecretName is the name of the secret. |
| credentials.appKey | If you are using clusterAgent.metricsProvider.enabled = true, you must set a Datadog application key for read access to your metrics. |
| credentials.appKeyExistingSecret | AppKeyExistingSecret is DEPRECATED. In order to pass the APP key through an existing secret, please consider "appSecret" instead. If set, this parameter takes precedence over "appKey". |
| credentials.appSecret.keyName | KeyName is the key of the secret to use. |
| credentials.appSecret.secretName | SecretName is the name of the secret. |
| credentials.token | This needs to be at least 32 characters a-zA-z. It is a preshared key between the node agents and the cluster agent. |
| credentials.useSecretBackend | UseSecretBackend use the Agent secret backend feature for retreiving all credentials needed by the different components: Agent, Cluster, Cluster-Checks. default value is false. |
| features.kubeStateMetricsCore.clusterCheck | ClusterCheck configures the Kubernetes State Metrics Core check as a cluster check. |
| features.kubeStateMetricsCore.conf.configData | ConfigData corresponds to the configuration file content. |
| features.kubeStateMetricsCore.conf.configMap.fileKey | FileKey corresponds to the key used in the ConfigMap.Data to store the configuration file content. |
| features.kubeStateMetricsCore.conf.configMap.name | The name of source ConfigMap. |
| features.kubeStateMetricsCore.enabled | Enable this to start the Kubernetes State Metrics Core check. Refer to https://docs.datadoghq.com/integrations/kubernetes_state_core |
| features.logCollection.containerCollectUsingFiles | Collect logs from files in `/var/log/pods instead` of using the container runtime API. Collecting logs from files is usually the most efficient way of collecting logs. See also: https://docs.datadoghq.com/agent/basic_agent_usage/kubernetes/#log-collection-setup Default is true |
| features.logCollection.containerLogsPath | Allows log collection from the container log path. Set to a different path if you are not using the Docker runtime. See also: https://docs.datadoghq.com/agent/kubernetes/daemonset_setup/?tab=k8sfile#create-manifest Defaults to `/var/lib/docker/containers` |
| features.logCollection.containerSymlinksPath | Allows the log collection to use symbolic links in this directory to validate container ID -> pod. Defaults to `/var/log/containers` |
| features.logCollection.enabled | Enable this option to activate Datadog Agent log collection. See also: https://docs.datadoghq.com/agent/basic_agent_usage/kubernetes/#log-collection-setup |
| features.logCollection.logsConfigContainerCollectAll | Enable this option to allow log collection for all containers. See also: https://docs.datadoghq.com/agent/basic_agent_usage/kubernetes/#log-collection-setup |
| features.logCollection.openFilesLimit | Sets the maximum number of log files that the Datadog Agent tails. Increasing this limit can increase resource consumption of the Agent. See also: https://docs.datadoghq.com/agent/basic_agent_usage/kubernetes/#log-collection-setup Default is 100 |
| features.logCollection.podLogsPath | Allows log collection from pod log path. Defaults to `/var/log/pods`. |
| features.logCollection.tempStoragePath | This path (always mounted from the host) is used by Datadog Agent to store information about processed log files. If the Datadog Agent is restarted, it starts tailing the log files immediately. Default to `/var/lib/datadog-agent/logs` |
| features.networkMonitoring.enabled |  |
| features.orchestratorExplorer.additionalEndpoints | Additional endpoints for shipping the collected data as json in the form of {"https://process.agent.datadoghq.com": ["apikey1", ...], ...}'. |
| features.orchestratorExplorer.clusterCheck | ClusterCheck configures the Orchestrator Explorer check as a cluster check. |
| features.orchestratorExplorer.conf.configData | ConfigData corresponds to the configuration file content. |
| features.orchestratorExplorer.conf.configMap.fileKey | FileKey corresponds to the key used in the ConfigMap.Data to store the configuration file content. |
| features.orchestratorExplorer.conf.configMap.name | The name of source ConfigMap. |
| features.orchestratorExplorer.ddUrl | Set this for the Datadog endpoint for the orchestrator explorer |
| features.orchestratorExplorer.enabled | Enable this to activate live Kubernetes monitoring. See also: https://docs.datadoghq.com/infrastructure/livecontainers/#kubernetes-resources |
| features.orchestratorExplorer.extraTags | Additional tags for the collected data in the form of `a b c` Difference to DD_TAGS: this is a cluster agent option that is used to define custom cluster tags |
| features.orchestratorExplorer.scrubbing.containers | Deactivate this to stop the scrubbing of sensitive container data (passwords, tokens, etc. ). |
| features.prometheusScrape.additionalConfigs | AdditionalConfigs allows adding advanced prometheus check configurations with custom discovery rules. |
| features.prometheusScrape.enabled | Enable autodiscovering pods and services exposing prometheus metrics. |
| features.prometheusScrape.serviceEndpoints | ServiceEndpoints enables generating dedicated checks for service endpoints. |
| registry | Registry to use for all Agent images (default gcr.io/datadoghq). Use public.ecr.aws/datadog for AWS Use docker.io/datadog for DockerHub |
| site | The site of the Datadog intake to send Agent data to. Set to 'datadoghq.eu' to send data to the EU site. |

[1]: https://github.com/DataDog/datadog-operator/blob/main/examples/datadogagent/datadog-agent-all.yaml
[2]: https://github.com/DataDog/datadog-operator/blob/main/examples/datadogagent/datadog-agent-logs-apm.yaml
[3]: https://github.com/DataDog/datadog-operator/blob/main/examples/datadogagent/datadog-agent-logs.yaml
[4]: https://github.com/DataDog/datadog-operator/blob/main/examples/datadogagent/datadog-agent-apm.yaml
[5]: https://github.com/DataDog/datadog-operator/blob/main/examples/datadogagent/datadog-agent-with-clusteragent.yaml
[6]: https://github.com/DataDog/datadog-operator/blob/main/examples/datadogagent/datadog-agent-with-tolerations.yaml
