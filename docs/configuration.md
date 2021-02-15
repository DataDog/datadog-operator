# Configuration

> Note this document is generated from code comments. When contributing a change to this document please do so by changing the code comments.`

## Manifest Templates

* [Manifest with Logs, APM, process, and metrics collection enabled.][1]
* [Manifest with Logs, APM, and metrics collection enabled.][2]
* [Manifest with Logs and metrics collection enabled.][3]
* [Manifest with APM and metrics collection enabled.][4]
* [Manifest with Cluster Agent.][5]
* [Manifest with tolerations.][6]

## All configuration options

The following section lists the configurable parameters for the `DatadogAgent`
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

### Custom Resources

* [DatadogAgent](#datadogagent)
* [DatadogMetric](#datadogmetric)

### Sub Resources

* [APMSpec](#apmspec)
* [APMUnixDomainSocketSpec](#apmunixdomainsocketspec)
* [AdmissionControllerConfig](#admissioncontrollerconfig)
* [AgentCredentials](#agentcredentials)
* [CRISocketConfig](#crisocketconfig)
* [ClusterAgentConfig](#clusteragentconfig)
* [ClusterChecksRunnerConfig](#clusterchecksrunnerconfig)
* [ComplianceSpec](#compliancespec)
* [ConfigDirSpec](#configdirspec)
* [ConfigFileConfigMapSpec](#configfileconfigmapspec)
* [CustomConfigSpec](#customconfigspec)
* [DSDUnixDomainSocketSpec](#dsdunixdomainsocketspec)
* [DaemonSetDeploymentStrategy](#daemonsetdeploymentstrategy)
* [DaemonSetRollingUpdateSpec](#daemonsetrollingupdatespec)
* [DaemonSetStatus](#daemonsetstatus)
* [DatadogAgentCondition](#datadogagentcondition)
* [DatadogAgentList](#datadogagentlist)
* [DatadogAgentSpec](#datadogagentspec)
* [DatadogAgentSpecAgentSpec](#datadogagentspecagentspec)
* [DatadogAgentSpecClusterAgentSpec](#datadogagentspecclusteragentspec)
* [DatadogAgentSpecClusterChecksRunnerSpec](#datadogagentspecclusterchecksrunnerspec)
* [DatadogAgentStatus](#datadogagentstatus)
* [DatadogFeatures](#datadogfeatures)
* [DeploymentStatus](#deploymentstatus)
* [DogstatsdConfig](#dogstatsdconfig)
* [ExternalMetricsConfig](#externalmetricsconfig)
* [ImageConfig](#imageconfig)
* [KubeStateMetricsCore](#kubestatemetricscore)
* [LogSpec](#logspec)
* [NetworkPolicySpec](#networkpolicyspec)
* [NodeAgentConfig](#nodeagentconfig)
* [OrchestratorExplorerConfig](#orchestratorexplorerconfig)
* [ProcessSpec](#processspec)
* [RbacConfig](#rbacconfig)
* [RuntimeSecuritySpec](#runtimesecurityspec)
* [Scrubbing](#scrubbing)
* [Secret](#secret)
* [SecuritySpec](#securityspec)
* [SyscallMonitorSpec](#syscallmonitorspec)
* [SystemProbeSpec](#systemprobespec)
* [DatadogMetricCondition](#datadogmetriccondition)
* [DatadogMetricList](#datadogmetriclist)
* [DatadogMetricSpec](#datadogmetricspec)
* [DatadogMetricStatus](#datadogmetricstatus)

#### APMSpec

APMSpec contains the Trace Agent configuration

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| enabled | Enable this to enable APM and tracing, on port 8126 ref: https://github.com/DataDog/docker-dd-agent#tracing-from-the-host | *bool | false |
| hostPort | Number of port to expose on the host. If specified, this must be a valid port number, 0 < x < 65536. If HostNetwork is specified, this must match ContainerPort. Most containers do not need this. | *int32 | false |
| unixDomainSocket | UnixDomainSocket socket configuration ref: https://docs.datadoghq.com/agent/kubernetes/apm/?tab=helm#agent-environment-variables | *[APMUnixDomainSocketSpec](#apmunixdomainsocketspec) | false |
| env | The Datadog Agent supports many environment variables Ref: https://docs.datadoghq.com/agent/docker/?tab=standard#environment-variables | []corev1.EnvVar | false |
| resources | Datadog APM Agent resource requests and limits Make sure to keep requests and limits equal to keep the pods in the Guaranteed QoS class Ref: http://kubernetes.io/docs/user-guide/compute-resources/ | *corev1.ResourceRequirements | false |

[Back to Custom Resources](#custom-resources)

#### APMUnixDomainSocketSpec

APMUnixDomainSocketSpec contains the APM Unix Domain Socket configuration

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| enabled | Enable APM over Unix Domain Socket ref: https://docs.datadoghq.com/agent/kubernetes/apm/?tab=helm#agent-environment-variables | *bool | false |
| hostFilepath | Define the host APM socket filepath used when APM over Unix Domain Socket is enabled (default value: /var/run/datadog/apm.sock) ref: https://docs.datadoghq.com/agent/kubernetes/apm/?tab=helm#agent-environment-variables | *string | false |

[Back to Custom Resources](#custom-resources)

#### AdmissionControllerConfig

AdmissionControllerConfig contains the configuration of the admission controller in Cluster Agent

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| enabled | Enable the admission controller to be able to inject APM/Dogstatsd config and standard tags (env, service, version) automatically into your pods | bool | false |
| mutateUnlabelled | MutateUnlabelled enables injecting config without having the pod label 'admission.datadoghq.com/enabled=\"true\"' | *bool | false |
| serviceName | ServiceName corresponds to the webhook service name | *string | false |

[Back to Custom Resources](#custom-resources)

#### AgentCredentials

AgentCredentials contains credentials values to configure the Agent

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| apiKey | APIKey Set this to your Datadog API key before the Agent runs. ref: https://app.datadoghq.com/account/settings#agent/kubernetes | string | false |
| apiKeyExistingSecret | APIKeyExistingSecret is DEPRECATED. In order to pass the API key through an existing secret, please consider \"apiSecret\" instead. If set, this parameter takes precedence over \"apiKey\". | string | false |
| apiSecret | APISecret Use existing Secret which stores API key instead of creating a new one. If set, this parameter takes precedence over \"apiKey\" and \"apiKeyExistingSecret\". | *[Secret](#secret) | false |
| appKey | If you are using clusterAgent.metricsProvider.enabled = true, you must set a Datadog application key for read access to your metrics. | string | false |
| appKeyExistingSecret | AppKeyExistingSecret is DEPRECATED. In order to pass the APP key through an existing secret, please consider \"appSecret\" instead. If set, this parameter takes precedence over \"appKey\". | string | false |
| appSecret | APPSecret Use existing Secret which stores API key instead of creating a new one. If set, this parameter takes precedence over \"apiKey\" and \"appKeyExistingSecret\". | *[Secret](#secret) | false |
| token | This needs to be at least 32 characters a-zA-z It is a preshared key between the node agents and the cluster agent | string | false |
| useSecretBackend | UseSecretBackend use the Agent secret backend feature for retreiving all credentials needed by the different components: Agent, Cluster, Cluster-Checks. If `useSecretBackend: true`, other credential parameters will be ignored. default value is false. | *bool | false |

[Back to Custom Resources](#custom-resources)

#### CRISocketConfig

CRISocketConfig contains the CRI socket configuration parameters

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| dockerSocketPath | Path to the docker runtime socket | *string | false |
| criSocketPath | Path to the container runtime socket (if different from Docker) This is supported starting from agent 6.6.0 | *string | false |

[Back to Custom Resources](#custom-resources)

#### ClusterAgentConfig

ClusterAgentConfig contains the configuration of the Cluster Agent

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| externalMetrics |  | *[ExternalMetricsConfig](#externalmetricsconfig) | false |
| admissionController | Configure the Admission Controller | *[AdmissionControllerConfig](#admissioncontrollerconfig) | false |
| clusterChecksEnabled | Enable the Cluster Checks and Endpoint Checks feature on both the cluster-agents and the daemonset ref: https://docs.datadoghq.com/agent/cluster_agent/clusterchecks/ https://docs.datadoghq.com/agent/cluster_agent/endpointschecks/ Autodiscovery via Kube Service annotations is automatically enabled | *bool | false |
| collectEvents | Enable this to start event collection from the kubernetes API ref: https://docs.datadoghq.com/agent/cluster_agent/event_collection/ | *bool | false |
| logLevel | Set logging verbosity, valid log levels are: trace, debug, info, warn, error, critical, and off | *string | false |
| resources | Datadog cluster-agent resource requests and limits | *corev1.ResourceRequirements | false |
| confd | Confd Provide additional cluster check configurations. Each key will become a file in /conf.d see https://docs.datadoghq.com/agent/autodiscovery/ for more details. | *[ConfigDirSpec](#configdirspec) | false |
| env | The Datadog Agent supports many environment variables Ref: https://docs.datadoghq.com/agent/docker/?tab=standard#environment-variables | []corev1.EnvVar | false |
| volumeMounts | Specify additional volume mounts in the Datadog Cluster Agent container | []corev1.VolumeMount | false |
| volumes | Specify additional volumes in the Datadog Cluster Agent container | []corev1.Volume | false |

[Back to Custom Resources](#custom-resources)

#### ClusterChecksRunnerConfig

ClusterChecksRunnerConfig contains the configuration of the Cluster Checks Runner

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| resources | Datadog Cluster Checks Runner resource requests and limits | *corev1.ResourceRequirements | false |
| logLevel | Set logging verbosity, valid log levels are: trace, debug, info, warn, error, critical, and off | *string | false |
| env | The Datadog Agent supports many environment variables Ref: https://docs.datadoghq.com/agent/docker/?tab=standard#environment-variables | []corev1.EnvVar | false |
| volumeMounts | Specify additional volume mounts in the Datadog Cluster Check Runner container | []corev1.VolumeMount | false |
| volumes | Specify additional volumes in the Datadog Cluster Check Runner container | []corev1.Volume | false |

[Back to Custom Resources](#custom-resources)

#### ComplianceSpec

ComplianceSpec contains configuration for continuous compliance

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| enabled | Enables continuous compliance monitoring | *bool | false |
| checkInterval | Check interval | *metav1.Duration | false |
| configDir | Config dir containing compliance benchmarks | *[ConfigDirSpec](#configdirspec) | false |

[Back to Custom Resources](#custom-resources)

#### ConfigDirSpec

ConfigDirSpec contains config file directory configuration

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| configMapName | ConfigMapName name of a ConfigMap used to mount a directory | string | false |

[Back to Custom Resources](#custom-resources)

#### ConfigFileConfigMapSpec

ConfigFileConfigMapSpec contains configMap information used to store a config file

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| name | Name the ConfigMap name | string | false |
| fileKey | FileKey corresponds to the key used in the ConfigMap.Data to store the configuration file content | string | false |

[Back to Custom Resources](#custom-resources)

#### CustomConfigSpec

CustomConfigSpec Allow to put custom configuration for the agent, corresponding to the datadog-cluster.yaml or datadog.yaml config file the configuration can be provided in the 'configData' field as raw data, or in a configmap thanks to `configMap` field. Important: `configData` and `configMap` can't be set together.

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| configData | ConfigData corresponds to the configuration file content | *string | false |
| configMap | ConfigMap name of a ConfigMap used to mount the configuration file | *[ConfigFileConfigMapSpec](#configfileconfigmapspec) | false |

[Back to Custom Resources](#custom-resources)

#### DSDUnixDomainSocketSpec

DSDUnixDomainSocketSpec contains the Dogstatsd Unix Domain Socket configuration

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| enabled | Enable APM over Unix Domain Socket ref: https://docs.datadoghq.com/developers/dogstatsd/unix_socket/ | *bool | false |
| hostFilepath | Define the host APM socket filepath used when APM over Unix Domain Socket is enabled (default value: /var/run/datadog/statsd.sock) ref: https://docs.datadoghq.com/developers/dogstatsd/unix_socket/ | *string | false |

[Back to Custom Resources](#custom-resources)

#### DaemonSetDeploymentStrategy

DaemonSetDeploymentStrategy contains the node Agent deployment configuration

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| updateStrategyType | The update strategy used for the DaemonSet | *appsv1.DaemonSetUpdateStrategyType | false |
| rollingUpdate | Configure the rolling updater strategy of the DaemonSet or the ExtendedDaemonSet | [DaemonSetRollingUpdateSpec](#daemonsetrollingupdatespec) | false |
| canary | Configure the canary deployment configuration using ExtendedDaemonSet | *edsdatadoghqv1alpha1.ExtendedDaemonSetSpecStrategyCanary | false |
| reconcileFrequency | The reconcile frequency of the ExtendDaemonSet | *metav1.Duration | false |

[Back to Custom Resources](#custom-resources)

#### DaemonSetRollingUpdateSpec

DaemonSetRollingUpdateSpec contains configuration fields of the rolling update strategy The configuration is shared between DaemonSet and ExtendedDaemonSet

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| maxUnavailable | The maximum number of DaemonSet pods that can be unavailable during the update. Value can be an absolute number (ex: 5) or a percentage of total number of DaemonSet pods at the start of the update (ex: 10%). Absolute number is calculated from percentage by rounding up. This cannot be 0. Default value is 1. | *intstr.IntOrString | false |
| maxPodSchedulerFailure | MaxPodSchedulerFailure the maxinum number of not scheduled on its Node due to a scheduler failure: resource constraints. Value can be an absolute number (ex: 5) or a percentage of total number of DaemonSet pods at the start of the update (ex: 10%). Absolute | *intstr.IntOrString | false |
| maxParallelPodCreation | The maxium number of pods created in parallel. Default value is 250. | *int32 | false |
| slowStartIntervalDuration | SlowStartIntervalDuration the duration between to 2 Default value is 1min. | *metav1.Duration | false |
| slowStartAdditiveIncrease | SlowStartAdditiveIncrease Value can be an absolute number (ex: 5) or a percentage of total number of DaemonSet pods at the start of the update (ex: 10%). Default value is 5. | *intstr.IntOrString | false |

[Back to Custom Resources](#custom-resources)

#### DaemonSetStatus

DaemonSetStatus defines the observed state of Agent running as DaemonSet

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| desired |  | int32 | true |
| current |  | int32 | true |
| ready |  | int32 | true |
| available |  | int32 | true |
| upToDate |  | int32 | true |
| status |  | string | false |
| state |  | string | false |
| lastUpdate |  | *metav1.Time | false |
| currentHash |  | string | false |
| daemonsetName | DaemonsetName corresponds to the name of the created DaemonSet | string | false |

[Back to Custom Resources](#custom-resources)

#### DatadogAgent

DatadogAgent Deployment with Datadog Operator

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| metadata |  | [metav1.ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.18/#objectmeta-v1-meta) | false |
| spec |  | [DatadogAgentSpec](#datadogagentspec) | false |
| status |  | [DatadogAgentStatus](#datadogagentstatus) | false |

[Back to Custom Resources](#custom-resources)

#### DatadogAgentCondition

DatadogAgentCondition describes the state of a DatadogAgent at a certain point.

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| type | Type of DatadogAgent condition. | DatadogAgentConditionType | true |
| status | Status of the condition, one of True, False, Unknown. | corev1.ConditionStatus | true |
| lastTransitionTime | Last time the condition transitioned from one status to another. | metav1.Time | false |
| lastUpdateTime | Last time the condition was updated. | metav1.Time | false |
| reason | The reason for the condition's last transition. | string | false |
| message | A human readable message indicating details about the transition. | string | false |

[Back to Custom Resources](#custom-resources)

#### DatadogAgentList

DatadogAgentList contains a list of DatadogAgent

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| metadata |  | [metav1.ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.18/#listmeta-v1-meta) | false |
| items |  | [][DatadogAgent](#datadogagent) | true |

[Back to Custom Resources](#custom-resources)

#### DatadogAgentSpec

DatadogAgentSpec defines the desired state of DatadogAgent

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| credentials | Configure the credentials needed to run Agents. If not set, then the credentials set in the DatadogOperator will be used | [AgentCredentials](#agentcredentials) | false |
| features | Features running on the Agent and Cluster Agent | *[DatadogFeatures](#datadogfeatures) | false |
| agent | The desired state of the Agent as an extended daemonset Contains the Node Agent configuration and deployment strategy | *[DatadogAgentSpecAgentSpec](#datadogagentspecagentspec) | false |
| clusterAgent | The desired state of the Cluster Agent as a deployment | *[DatadogAgentSpecClusterAgentSpec](#datadogagentspecclusteragentspec) | false |
| clusterChecksRunner | The desired state of the Cluster Checks Runner as a deployment | *[DatadogAgentSpecClusterChecksRunnerSpec](#datadogagentspecclusterchecksrunnerspec) | false |
| clusterName | Set a unique cluster name to allow scoping hosts and Cluster Checks Runner easily | string | false |
| site | The site of the Datadog intake to send Agent data to. Set to 'datadoghq.eu' to send data to the EU site. | string | false |

[Back to Custom Resources](#custom-resources)

#### DatadogAgentSpecAgentSpec

DatadogAgentSpecAgentSpec defines the desired state of the node Agent

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| useExtendedDaemonset | UseExtendedDaemonset use ExtendedDaemonset for Agent deployment. default value is false. | *bool | false |
| image | The container image of the Datadog Agent | [ImageConfig](#imageconfig) | true |
| daemonsetName | Name of the Daemonset to create or migrate from | string | false |
| config | Agent configuration | [NodeAgentConfig](#nodeagentconfig) | false |
| rbac | RBAC configuration of the Agent | [RbacConfig](#rbacconfig) | false |
| deploymentStrategy | Update strategy configuration for the DaemonSet | *[DaemonSetDeploymentStrategy](#daemonsetdeploymentstrategy) | false |
| additionalAnnotations | AdditionalAnnotations provide annotations that will be added to the Agent Pods. | map[string]string | false |
| additionalLabels | AdditionalLabels provide labels that will be added to the cluster checks runner Pods. | map[string]string | false |
| priorityClassName | If specified, indicates the pod's priority. \"system-node-critical\" and \"system-cluster-critical\" are two special keywords which indicate the highest priorities with the former being the highest priority. Any other name must be defined by creating a PriorityClass object with that name. If not specified, the pod priority will be default or zero if there is no default. | string | false |
| dnsPolicy | Set DNS policy for the pod. Defaults to \"ClusterFirst\". Valid values are 'ClusterFirstWithHostNet', 'ClusterFirst', 'Default' or 'None'. DNS parameters given in DNSConfig will be merged with the policy selected with DNSPolicy. To have DNS options set along with hostNetwork, you have to specify DNS policy explicitly to 'ClusterFirstWithHostNet'. | corev1.DNSPolicy | false |
| dnsConfig | Specifies the DNS parameters of a pod. Parameters specified here will be merged to the generated DNS configuration based on DNSPolicy. | *corev1.PodDNSConfig | false |
| hostNetwork | Host networking requested for this pod. Use the host's network namespace. If this option is set, the ports that will be used must be specified. Default to false. | bool | false |
| hostPID | Use the host's pid namespace. Optional: Default to false. | bool | false |
| env | Environment variables for all Datadog Agents Ref: https://docs.datadoghq.com/agent/docker/?tab=standard#environment-variables | []corev1.EnvVar | false |
| apm | Trace Agent configuration | [APMSpec](#apmspec) | false |
| log | Log Agent configuration | [LogSpec](#logspec) | false |
| process | Process Agent configuration | [ProcessSpec](#processspec) | false |
| systemProbe | SystemProbe configuration | [SystemProbeSpec](#systemprobespec) | false |
| security | Security Agent configuration | [SecuritySpec](#securityspec) | false |
| customConfig | Allow to put custom configuration for the agent, corresponding to the datadog.yaml config file See https://docs.datadoghq.com/agent/guide/agent-configuration-files/?tab=agentv6 for more details. | *[CustomConfigSpec](#customconfigspec) | false |
| networkPolicy | Provide Agent Network Policy configuration | [NetworkPolicySpec](#networkpolicyspec) | false |

[Back to Custom Resources](#custom-resources)

#### DatadogAgentSpecClusterAgentSpec

DatadogAgentSpecClusterAgentSpec defines the desired state of the cluster Agent

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| image | The container image of the Datadog Cluster Agent | [ImageConfig](#imageconfig) | true |
| deploymentName | Name of the Cluster Agent Deployment to create or migrate from | string | false |
| config | Cluster Agent configuration | [ClusterAgentConfig](#clusteragentconfig) | false |
| customConfig | Allow to put custom configuration for the agent, corresponding to the datadog-cluster.yaml config file | *[CustomConfigSpec](#customconfigspec) | false |
| rbac | RBAC configuration of the Datadog Cluster Agent | [RbacConfig](#rbacconfig) | false |
| replicas | Number of the Cluster Agent replicas | *int32 | false |
| additionalAnnotations | AdditionalAnnotations provide annotations that will be added to the cluster-agent Pods. | map[string]string | false |
| additionalLabels | AdditionalLabels provide labels that will be added to the cluster checks runner Pods. | map[string]string | false |
| priorityClassName | If specified, indicates the pod's priority. \"system-node-critical\" and \"system-cluster-critical\" are two special keywords which indicate the highest priorities with the former being the highest priority. Any other name must be defined by creating a PriorityClass object with that name. If not specified, the pod priority will be default or zero if there is no default. | string | false |
| affinity | If specified, the pod's scheduling constraints | *corev1.Affinity | false |
| tolerations | If specified, the Cluster-Agent pod's tolerations. | []corev1.Toleration | false |
| nodeSelector | NodeSelector is a selector which must be true for the pod to fit on a node. Selector which must match a node's labels for the pod to be scheduled on that node. More info: https://kubernetes.io/docs/concepts/configuration/assign-pod-node/ | map[string]string | false |
| networkPolicy | Provide Cluster Agent Network Policy configuration | [NetworkPolicySpec](#networkpolicyspec) | false |

[Back to Custom Resources](#custom-resources)

#### DatadogAgentSpecClusterChecksRunnerSpec

DatadogAgentSpecClusterChecksRunnerSpec defines the desired state of the Cluster Checks Runner

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| image | The container image of the Datadog Cluster Checks Runner | [ImageConfig](#imageconfig) | true |
| deploymentName | Name of the cluster checks deployment to create or migrate from | string | false |
| config | Agent configuration | [ClusterChecksRunnerConfig](#clusterchecksrunnerconfig) | false |
| customConfig | Allow to put custom configuration for the agent, corresponding to the datadog.yaml config file See https://docs.datadoghq.com/agent/guide/agent-configuration-files/?tab=agentv6 for more details. | *[CustomConfigSpec](#customconfigspec) | false |
| rbac | RBAC configuration of the Datadog Cluster Checks Runner | [RbacConfig](#rbacconfig) | false |
| replicas | Number of the Cluster Agent replicas | *int32 | false |
| additionalAnnotations | AdditionalAnnotations provide annotations that will be added to the cluster checks runner Pods. | map[string]string | false |
| additionalLabels | AdditionalLabels provide labels that will be added to the cluster checks runner Pods. | map[string]string | false |
| priorityClassName | If specified, indicates the pod's priority. \"system-node-critical\" and \"system-cluster-critical\" are two special keywords which indicate the highest priorities with the former being the highest priority. Any other name must be defined by creating a PriorityClass object with that name. If not specified, the pod priority will be default or zero if there is no default. | string | false |
| affinity | If specified, the pod's scheduling constraints | *corev1.Affinity | false |
| tolerations | If specified, the Cluster-Checks pod's tolerations. | []corev1.Toleration | false |
| nodeSelector | NodeSelector is a selector which must be true for the pod to fit on a node. Selector which must match a node's labels for the pod to be scheduled on that node. More info: https://kubernetes.io/docs/concepts/configuration/assign-pod-node/ | map[string]string | false |
| networkPolicy | Provide Cluster Checks Runner Network Policy configuration | [NetworkPolicySpec](#networkpolicyspec) | false |

[Back to Custom Resources](#custom-resources)

#### DatadogAgentStatus

DatadogAgentStatus defines the observed state of DatadogAgent

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| agent | The actual state of the Agent as an extended daemonset | *[DaemonSetStatus](#daemonsetstatus) | false |
| clusterAgent | The actual state of the Cluster Agent as a deployment | *[DeploymentStatus](#deploymentstatus) | false |
| clusterChecksRunner | The actual state of the Cluster Checks Runner as a deployment | *[DeploymentStatus](#deploymentstatus) | false |
| conditions | Conditions Represents the latest available observations of a DatadogAgent's current state. | [][DatadogAgentCondition](#datadogagentcondition) | false |

[Back to Custom Resources](#custom-resources)

#### DatadogFeatures

DatadogFeatures are Features running on the Agent and Cluster Agent.

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| orchestratorExplorer | OrchestratorExplorer configuration | *[OrchestratorExplorerConfig](#orchestratorexplorerconfig) | false |
| kubeStateMetricsCore | KubeStateMetricsCore configuration | *[KubeStateMetricsCore](#kubestatemetricscore) | false |

[Back to Custom Resources](#custom-resources)

#### DeploymentStatus

DeploymentStatus type representing the Cluster Agent Deployment status

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| replicas | Total number of non-terminated pods targeted by this deployment (their labels match the selector). | int32 | false |
| updatedReplicas | Total number of non-terminated pods targeted by this deployment that have the desired template spec. | int32 | false |
| readyReplicas | Total number of ready pods targeted by this deployment. | int32 | false |
| availableReplicas | Total number of available pods (ready for at least minReadySeconds) targeted by this deployment. | int32 | false |
| unavailableReplicas | Total number of unavailable pods targeted by this deployment. This is the total number of pods that are still required for the deployment to have 100% available capacity. They may either be pods that are running but not yet available or pods that still have not been created. | int32 | false |
| lastUpdate |  | *metav1.Time | false |
| currentHash |  | string | false |
| generatedToken | GeneratedToken corresponds to the generated token if any token was provided in the Credential configuration when ClusterAgent is enabled | string | false |
| status | Status corresponds to the ClusterAgent deployment computed status | string | false |
| state | State corresponds to the ClusterAgent deployment state | string | false |
| deploymentName | DeploymentName corresponds to the name of the Cluster Agent Deployment | string | false |

[Back to Custom Resources](#custom-resources)

#### DogstatsdConfig

DogstatsdConfig contains the Dogstatsd configuration parameters

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| dogstatsdOriginDetection | Enable origin detection for container tagging ref: https://docs.datadoghq.com/developers/dogstatsd/unix_socket/#using-origin-detection-for-container-tagging | *bool | false |
| unixDomainSocket | Configure the Dogstatsd Unix Domain Socket ref: https://docs.datadoghq.com/developers/dogstatsd/unix_socket/ | *[DSDUnixDomainSocketSpec](#dsdunixdomainsocketspec) | false |

[Back to Custom Resources](#custom-resources)

#### ExternalMetricsConfig

ExternalMetricsConfig contains the configuration of the external metrics provider in Cluster Agent

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| enabled | Enable the metricsProvider to be able to scale based on metrics in Datadog | bool | false |
| wpaController | Enable informer and controller of the watermark pod autoscaler NOTE: The WatermarkPodAutoscaler controller needs to be installed see https://github.com/DataDog/watermarkpodautoscaler for more details. | bool | false |
| useDatadogMetrics | Enable usage of DatadogMetrics CRD (allow to scale on arbitrary queries) | bool | false |
| port | If specified configures the metricsProvider external metrics service port | *int32 | false |
| endpoint | Override the API endpoint for the external metrics server. Defaults to .spec.agent.config.ddUrl or \"https://app.datadoghq.com\" if that's empty. | *string | false |

[Back to Custom Resources](#custom-resources)

#### ImageConfig

ImageConfig Datadog agent container image config

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| name | Define the image to use Use \"gcr.io/datadoghq/agent:latest\" for Datadog Agent 6 Use \"datadog/dogstatsd:latest\" for Standalone Datadog Agent DogStatsD6 Use \"gcr.io/datadoghq/cluster-agent:latest\" for Datadog Cluster Agent | string | true |
| pullPolicy | The Kubernetes pull policy Use Always, Never or IfNotPresent | *corev1.PullPolicy | false |
| pullSecrets | It is possible to specify docker registry credentials See https://kubernetes.io/docs/concepts/containers/images/#specifying-imagepullsecrets-on-a-pod | *[]corev1.LocalObjectReference | false |

[Back to Custom Resources](#custom-resources)

#### KubeStateMetricsCore

KubeStateMetricsCore contains the required parameters to enable and override the configuration of the Kubernetes State Metrics Core (aka v2.0.0) of the check.

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| enabled | Enable this to start the Kubernetes State Metrics Core check. Refer to https://github.com/DataDog/datadog-operator/blob/master/docs/kubernetes_state_metrics.md | *bool | false |
| conf | To override the configuration for the default Kubernetes State Metrics Core check. Must point to a ConfigMap containing a valid cluster check configuration. | *[CustomConfigSpec](#customconfigspec) | false |

[Back to Custom Resources](#custom-resources)

#### LogSpec

LogSpec contains the Log Agent configuration

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| enabled | Enables this to activate Datadog Agent log collection. ref: https://docs.datadoghq.com/agent/basic_agent_usage/kubernetes/#log-collection-setup | *bool | false |
| logsConfigContainerCollectAll | Enable this to allow log collection for all containers. ref: https://docs.datadoghq.com/agent/basic_agent_usage/kubernetes/#log-collection-setup | *bool | false |
| containerCollectUsingFiles | Collect logs from files in /var/log/pods instead of using container runtime API. It's usually the most efficient way of collecting logs. ref: https://docs.datadoghq.com/agent/basic_agent_usage/kubernetes/#log-collection-setup Default: true | *bool | false |
| containerLogsPath | This to allow log collection from container log path. Set to a different path if not using docker runtime. ref: https://docs.datadoghq.com/agent/kubernetes/daemonset_setup/?tab=k8sfile#create-manifest Default to `/var/lib/docker/containers` | *string | false |
| podLogsPath | This to allow log collection from pod log path. Default to `/var/log/pods` | *string | false |
| tempStoragePath | This path (always mounted from the host) is used by Datadog Agent to store information about processed log files. If the Datadog Agent is restarted, it allows to start tailing the log files from the right offset Default to `/var/lib/datadog-agent/logs` | *string | false |
| openFilesLimit | Set the maximum number of logs files that the Datadog Agent will tail up to. Increasing this limit can increase resource consumption of the Agent. ref: https://docs.datadoghq.com/agent/basic_agent_usage/kubernetes/#log-collection-setup Default to 100 | *int32 | false |

[Back to Custom Resources](#custom-resources)

#### NetworkPolicySpec

NetworkPolicySpec provides Network Policy configuration for the agents

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| create | If true, create a NetworkPolicy for the current agent | *bool | false |

[Back to Custom Resources](#custom-resources)

#### NodeAgentConfig

NodeAgentConfig contains the configuration of the Node Agent

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| securityContext | Pod-level SecurityContext | *corev1.PodSecurityContext | false |
| ddUrl | The host of the Datadog intake server to send Agent data to, only set this option if you need the Agent to send data to a custom URL. Overrides the site setting defined in \"site\". | *string | false |
| logLevel | Set logging verbosity, valid log levels are: trace, debug, info, warn, error, critical, and off | *string | false |
| confd | Confd configuration allowing to specify config files for custom checks placed under /etc/datadog-agent/conf.d/. See https://docs.datadoghq.com/agent/guide/agent-configuration-files/?tab=agentv6 for more details. | *[ConfigDirSpec](#configdirspec) | false |
| checksd | Checksd configuration allowing to specify custom checks placed under /etc/datadog-agent/checks.d/ See https://docs.datadoghq.com/agent/guide/agent-configuration-files/?tab=agentv6 for more details. | *[ConfigDirSpec](#configdirspec) | false |
| podLabelsAsTags | Provide a mapping of Kubernetes Labels to Datadog Tags. <KUBERNETES_LABEL>: <DATADOG_TAG_KEY> | map[string]string | false |
| podAnnotationsAsTags | Provide a mapping of Kubernetes Annotations to Datadog Tags. <KUBERNETES_ANNOTATIONS>: <DATADOG_TAG_KEY> | map[string]string | false |
| tags | List of tags to attach to every metric, event and service check collected by this Agent. Learn more about tagging: https://docs.datadoghq.com/tagging/ | []string | false |
| collectEvents | Enables this to start event collection from the Kubernetes API ref: https://docs.datadoghq.com/agent/kubernetes/event_collection/ | *bool | false |
| leaderElection | Enables leader election mechanism for event collection. | *bool | false |
| env | The Datadog Agent supports many environment variables Ref: https://docs.datadoghq.com/agent/docker/?tab=standard#environment-variables | []corev1.EnvVar | false |
| volumeMounts | Specify additional volume mounts in the Datadog Agent container | []corev1.VolumeMount | false |
| volumes | Specify additional volumes in the Datadog Agent container | []corev1.Volume | false |
| resources | Datadog Agent resource requests and limits Make sure to keep requests and limits equal to keep the pods in the Guaranteed QoS class Ref: http://kubernetes.io/docs/user-guide/compute-resources/ | *corev1.ResourceRequirements | false |
| criSocket | Configure the CRI Socket | *[CRISocketConfig](#crisocketconfig) | false |
| dogstatsd | Configure Dogstatsd | *[DogstatsdConfig](#dogstatsdconfig) | false |
| tolerations | If specified, the Agent pod's tolerations. | []corev1.Toleration | false |
| hostPort | Number of port to expose on the host. If specified, this must be a valid port number, 0 < x < 65536. If HostNetwork is specified, this must match ContainerPort. Most containers do not need this. | *int32 | false |

[Back to Custom Resources](#custom-resources)

#### OrchestratorExplorerConfig

OrchestratorExplorerConfig contains the orchestrator explorer configuration. The orchestratorExplorer runs in the process-agent and DCA.

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| enabled | Enable this to activate live Kubernetes monitoring. ref: https://docs.datadoghq.com/infrastructure/livecontainers/#kubernetes-resources | *bool | false |
| scrubbing | Option to disable scrubbing of sensitive container data (passwords, tokens, etc. ). | *[Scrubbing](#scrubbing) | false |
| additionalEndpoints | Additional endpoints for shipping the collected data as json in the form of {\"https://process.agent.datadoghq.com\": [\"apikey1\", ...], ...}'. | *string | false |
| ddUrl | Set this for the Datadog endpoint for the orchestrator explorer | *string | false |
| extraTags | Additional tags for the collected data in the form of `a b c` Difference to DD_TAGS: this is a cluster agent option that is used to define custom cluster tags | []string | false |

[Back to Custom Resources](#custom-resources)

#### ProcessSpec

ProcessSpec contains the Process Agent configuration

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| enabled | Note: /etc/passwd is automatically mounted to allow username resolution. ref: https://docs.datadoghq.com/graphing/infrastructure/process/#kubernetes-daemonset | *bool | false |
| processCollectionEnabled | false (default): Only collect containers if available. true: collect process information as well | *bool | false |
| env | The Datadog Agent supports many environment variables Ref: https://docs.datadoghq.com/agent/docker/?tab=standard#environment-variables | []corev1.EnvVar | false |
| resources | Datadog Process Agent resource requests and limits Make sure to keep requests and limits equal to keep the pods in the Guaranteed QoS class Ref: http://kubernetes.io/docs/user-guide/compute-resources/ | *corev1.ResourceRequirements | false |

[Back to Custom Resources](#custom-resources)

#### RbacConfig

RbacConfig contains RBAC configuration

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| create | Used to configure RBAC resources creation | *bool | false |
| serviceAccountName | Used to set up the service account name to use Ignored if the field Create is true | *string | false |

[Back to Custom Resources](#custom-resources)

#### RuntimeSecuritySpec

RuntimeSecuritySpec contains configuration for runtime security features

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| enabled | Enables runtime security features | *bool | false |
| policiesDir | ConfigDir containing security policies | *[ConfigDirSpec](#configdirspec) | false |
| syscallMonitor | Syscall monitor configuration | *[SyscallMonitorSpec](#syscallmonitorspec) | false |

[Back to Custom Resources](#custom-resources)

#### Scrubbing

Scrubbing contains configuration to enable or disable scrubbing options

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| containers | Deactivate this to stop the scrubbing of sensitive container data (passwords, tokens, etc. ). | *bool | false |

[Back to Custom Resources](#custom-resources)

#### Secret

Secret contains a secret name and an included key

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| secretName | SecretName is the name of the secret | string | true |
| keyName | KeyName is the key of the secret to use | string | false |

[Back to Custom Resources](#custom-resources)

#### SecuritySpec

SecuritySpec contains the Security Agent configuration

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| compliance | Compliance configuration | [ComplianceSpec](#compliancespec) | false |
| runtime | Runtime security configuration | [RuntimeSecuritySpec](#runtimesecurityspec) | false |
| env | The Datadog Security Agent supports many environment variables Ref: https://docs.datadoghq.com/agent/docker/?tab=standard#environment-variables | []corev1.EnvVar | false |
| resources | Datadog Security Agent resource requests and limits Make sure to keep requests and limits equal to keep the pods in the Guaranteed QoS class Ref: http://kubernetes.io/docs/user-guide/compute-resources/ | *corev1.ResourceRequirements | false |

[Back to Custom Resources](#custom-resources)

#### SyscallMonitorSpec

SyscallMonitorSpec contains configuration for syscall monitor

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| enabled | Enabled enables syscall monitor | *bool | false |

[Back to Custom Resources](#custom-resources)

#### SystemProbeSpec

SystemProbeSpec contains the SystemProbe Agent configuration

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| enabled | Enable this to activate live process monitoring. Note: /etc/passwd is automatically mounted to allow username resolution. ref: https://docs.datadoghq.com/graphing/infrastructure/process/#kubernetes-daemonset | *bool | false |
| secCompRootPath | SecCompRootPath specify the seccomp profile root directory | string | false |
| secCompCustomProfileConfigMap | SecCompCustomProfileConfigMap specify a pre-existing ConfigMap containing a custom SecComp profile | string | false |
| secCompProfileName | SecCompProfileName specify a seccomp profile | string | false |
| appArmorProfileName | AppArmorProfileName specify a apparmor profile | string | false |
| conntrackEnabled | ConntrackEnabled enable the system-probe agent to connect to the netlink/conntrack subsystem to add NAT information to connection data Ref: http://conntrack-tools.netfilter.org/ | *bool | false |
| bpfDebugEnabled | BPFDebugEnabled logging for kernel debug | *bool | false |
| debugPort | DebugPort Specify the port to expose pprof and expvar for system-probe agent | int32 | false |
| enableTCPQueueLength | EnableTCPQueueLength enables the TCP queue length eBPF-based check | *bool | false |
| enableOOMKill | EnableOOMKill enables the OOM kill eBPF-based check | *bool | false |
| collectDNSStats | CollectDNSStats enables DNS stat collection | *bool | false |
| env | The Datadog SystemProbe supports many environment variables Ref: https://docs.datadoghq.com/agent/docker/?tab=standard#environment-variables | []corev1.EnvVar | false |
| resources | Datadog SystemProbe resource requests and limits Make sure to keep requests and limits equal to keep the pods in the Guaranteed QoS class Ref: http://kubernetes.io/docs/user-guide/compute-resources/ | *corev1.ResourceRequirements | false |
| securityContext | You can modify the security context used to run the containers by modifying the label type | *corev1.SecurityContext | false |

[Back to Custom Resources](#custom-resources)

#### DatadogMetric

DatadogMetric allows autoscaling on arbitrary Datadog query

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| metadata |  | [metav1.ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.18/#objectmeta-v1-meta) | false |
| spec |  | [DatadogMetricSpec](#datadogmetricspec) | false |
| status |  | [DatadogMetricStatus](#datadogmetricstatus) | false |

[Back to Custom Resources](#custom-resources)

#### DatadogMetricCondition

DatadogMetricCondition describes the state of a DatadogMetric at a certain point.

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| type | Type of DatadogMetric condition. | DatadogMetricConditionType | true |
| status | Status of the condition, one of True, False, Unknown. | corev1.ConditionStatus | true |
| lastTransitionTime | Last time the condition transitioned from one status to another. | metav1.Time | false |
| lastUpdateTime | Last time the condition was updated. | metav1.Time | false |
| reason | The reason for the condition's last transition. | string | false |
| message | A human readable message indicating details about the transition. | string | false |

[Back to Custom Resources](#custom-resources)

#### DatadogMetricList

DatadogMetricList contains a list of DatadogMetric

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| metadata |  | [metav1.ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.18/#listmeta-v1-meta) | false |
| items |  | [][DatadogMetric](#datadogmetric) | true |

[Back to Custom Resources](#custom-resources)

#### DatadogMetricSpec

DatadogMetricSpec defines the desired state of DatadogMetric

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| query | Query is the raw datadog query | string | false |
| externalMetricName | ExternalMetricName is reversed for internal use | string | false |
| maxAge | MaxAge provides the max age for the metric query (overrides the default setting `external_metrics_provider.max_age`) | metav1.Duration | false |

[Back to Custom Resources](#custom-resources)

#### DatadogMetricStatus

DatadogMetricStatus defines the observed state of DatadogMetric

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| conditions | Conditions Represents the latest available observations of a DatadogMetric's current state. | [][DatadogMetricCondition](#datadogmetriccondition) | false |
| currentValue | Value is the latest value of the metric | string | true |
| autoscalerReferences | List of autoscalers currently using this DatadogMetric | string | false |

[Back to Custom Resources](#custom-resources)
## Links

[1]: https://github.com/DataDog/datadog-operator/blob/master/examples/datadog-agent-all.yaml
[2]: https://github.com/DataDog/datadog-operator/blob/master/examples/datadog-agent-logs-apm.yaml
[3]: https://github.com/DataDog/datadog-operator/blob/master/examples/datadog-agent-logs.yaml
[4]: https://github.com/DataDog/datadog-operator/blob/master/examples/datadog-agent-apm.yaml
[5]: https://github.com/DataDog/datadog-operator/blob/master/examples/datadog-agent-with-clusteragent.yaml
[6]: https://github.com/DataDog/datadog-operator/blob/master/examples/datadog-agent-with-tolerations.yaml