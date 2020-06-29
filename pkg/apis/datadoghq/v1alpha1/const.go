// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package v1alpha1

const (
	// AgentDeploymentNameLabelKey label key use to link a Resource to a DatadogAgent
	AgentDeploymentNameLabelKey = "agent.datadoghq.com/name"
	// AgentDeploymentComponentLabelKey label key use to know with component is it
	AgentDeploymentComponentLabelKey = "agent.datadoghq.com/component"
	// MD5AgentDeploymentAnnotationKey annotation key used on ExtendedDaemonSet in order to identify which AgentDeployment have been used to generate it.
	MD5AgentDeploymentAnnotationKey = "agent.datadoghq.com/agentspechash"

	// DefaultAgentResourceSuffix use as suffix for agent resource naming
	DefaultAgentResourceSuffix = "agent"
	// DefaultClusterAgentResourceSuffix use as suffix for cluster-agent resource naming
	DefaultClusterAgentResourceSuffix = "cluster-agent"
	// DefaultClusterChecksRunnerResourceSuffix use as suffix for cluster-checks-runner resource naming
	DefaultClusterChecksRunnerResourceSuffix = "cluster-checks-runner"
	// DefaultMetricsServerResourceSuffix use as suffix for cluster-agent metrics-server resource naming
	DefaultMetricsServerResourceSuffix = "cluster-agent-metrics-server"
	// DefaultAPPKeyKey default app-key key (use in secret for instance).
	DefaultAPPKeyKey = "app_key"
	// DefaultAPIKeyKey default api-key key (use in secret for instance).
	DefaultAPIKeyKey = "api_key"
	// DefaultTokenKey default token key (use in secret for instance).
	DefaultTokenKey = "token"
	// DefaultClusterAgentServicePort default cluster-agent service port
	DefaultClusterAgentServicePort = 5005
	// DefaultMetricsServerServicePort default metrics-server port
	DefaultMetricsServerServicePort = 443
	// DefaultMetricsServerTargetPort default metrics-server pod port
	DefaultMetricsServerTargetPort = int(defaultMetricsProviderPort)
	// DefaultAdmissionControllerServicePort default admission controller service port
	DefaultAdmissionControllerServicePort = 443
	// DefaultAdmissionControllerTargetPort default admission controller pod port
	DefaultAdmissionControllerTargetPort = 8000
	// DefaultDogstatsdPort default dogstatsd port
	DefaultDogstatsdPort = 8125
)

// Datadog env var names
const (
	DatadogHost                           = "DATADOG_HOST"
	DDAPIKey                              = "DD_API_KEY"
	DDClusterName                         = "DD_CLUSTER_NAME"
	DDSite                                = "DD_SITE"
	DDddURL                               = "DD_DD_URL"
	DDHealthPort                          = "DD_HEALTH_PORT"
	DDLogLevel                            = "DD_LOG_LEVEL"
	DDPodLabelsAsTags                     = "DD_KUBERNETES_POD_LABELS_AS_TAGS"
	DDPodAnnotationsAsTags                = "DD_KUBERNETES_POD_ANNOTATIONS_AS_TAGS"
	DDTags                                = "DD_TAGS"
	DDCollectKubeEvents                   = "DD_COLLECT_KUBERNETES_EVENTS"
	DDLeaderElection                      = "DD_LEADER_ELECTION"
	DDLogsEnabled                         = "DD_LOGS_ENABLED"
	DDLogsConfigContainerCollectAll       = "DD_LOGS_CONFIG_CONTAINER_COLLECT_ALL"
	DDLogsContainerCollectUsingFiles      = "DD_LOGS_CONFIG_K8S_CONTAINER_USE_FILE"
	DDDogstatsdOriginDetection            = "DD_DOGSTATSD_ORIGIN_DETECTION"
	DDDogstatsdPort                       = "DD_DOGSTATSD_PORT"
	DDClusterAgentEnabled                 = "DD_CLUSTER_AGENT_ENABLED"
	DDClusterAgentKubeServiceName         = "DD_CLUSTER_AGENT_KUBERNETES_SERVICE_NAME"
	DDClusterAgentAuthToken               = "DD_CLUSTER_AGENT_AUTH_TOKEN"
	DDMetricsProviderEnabled              = "DD_EXTERNAL_METRICS_PROVIDER_ENABLED"
	DDMetricsProviderPort                 = "DD_EXTERNAL_METRICS_PROVIDER_PORT"
	DDMetricsProviderUseDatadogMetric     = "DD_EXTERNAL_METRICS_PROVIDER_USE_DATADOGMETRIC_CRD"
	DDAppKey                              = "DD_APP_KEY"
	DDClusterChecksEnabled                = "DD_CLUSTER_CHECKS_ENABLED"
	DDClcRunnerEnabled                    = "DD_CLC_RUNNER_ENABLED"
	DDClcRunnerHost                       = "DD_CLC_RUNNER_HOST"
	DDExtraConfigProviders                = "DD_EXTRA_CONFIG_PROVIDERS"
	DDExtraListeners                      = "DD_EXTRA_LISTENERS"
	DDHostname                            = "DD_HOSTNAME"
	DDAPMEnabled                          = "DD_APM_ENABLED"
	DDProcessAgentEnabled                 = "DD_PROCESS_AGENT_ENABLED"
	DDSystemProbeAgentEnabled             = "DD_SYSTEM_PROBE_ENABLED"
	DDEnableMetadataCollection            = "DD_ENABLE_METADATA_COLLECTION"
	DDKubeletHost                         = "DD_KUBERNETES_KUBELET_HOST"
	DDCriSocketPath                       = "DD_CRI_SOCKET_PATH"
	DockerHost                            = "DOCKER_HOST"
	DDAdmissionControllerEnabled          = "DD_ADMISSION_CONTROLLER_ENABLED"
	DDAdmissionControllerMutateUnlabelled = "DD_ADMISSION_CONTROLLER_MUTATE_UNLABELLED"
	DDAdmissionControllerInjectConfig     = "DD_ADMISSION_CONTROLLER_INJECT_CONFIG_ENABLED"
	DDAdmissionControllerInjectTags       = "DD_ADMISSION_CONTROLLER_INJECT_TAGS_ENABLED"
	DDAdmissionControllerServiceName      = "DD_ADMISSION_CONTROLLER_SERVICE_NAME"

	// KubernetesEnvvarName Env var used by the Datadog Agent container entrypoint
	// to add kubelet config provider and listener
	KubernetesEnvvarName = "KUBERNETES"

	// Datadog volume names and mount paths

	ConfdVolumeName                    = "confd"
	ConfdVolumePath                    = "/conf.d"
	ChecksdVolumeName                  = "checksd"
	ChecksdVolumePath                  = "/checks.d"
	ConfigVolumeName                   = "config"
	ConfigVolumePath                   = "/etc/datadog-agent"
	ProcVolumeName                     = "procdir"
	ProcVolumePath                     = "/host/proc"
	ProcVolumeReadOnly                 = true
	PasswdVolumeName                   = "passwd"
	PasswdVolumePath                   = "/etc/passwd"
	CgroupsVolumeName                  = "cgroups"
	CgroupsVolumePath                  = "/host/sys/fs/cgroup"
	CgroupsVolumeReadOnly              = true
	SystemProbeSocketVolumeName        = "sysprobe-socket-dir"
	SystemProbeSocketVolumePath        = "/opt/datadog-agent/run"
	CriSocketVolumeName                = "runtimesocketdir"
	CriSocketVolumeReadOnly            = true
	DogstatsdSockerVolumeName          = "dsdsocket"
	DogstatsdSockerVolumePath          = "/var/run/datadog"
	PointerVolumeName                  = "pointerdir"
	PointerVolumePath                  = "/opt/datadog-agent/run"
	LogPodVolumeName                   = "logpodpath"
	LogPodVolumePath                   = "/var/log/pods"
	LogPodVolumeReadOnly               = true
	LogContainerVolumeName             = "logcontainerpath"
	LogContainerVolumeReadOnly         = true
	SystemProbeDebugfsVolumeName       = "debugfs"
	SystemProbeDebugfsVolumePath       = "/sys/kernel/debug"
	SystemProbeConfigVolumeName        = "system-probe-config"
	SystemProbeConfigVolumePath        = "/etc/datadog-agent/system-probe.yaml"
	SystemProbeConfigVolumeSubPath     = "system-probe.yaml"
	SystemProbeAgentSecurityVolumeName = "datadog-agent-security"
	SystemProbeAgentSecurityVolumePath = "/etc/config"
	SystemProbeSecCompRootVolumeName   = "seccomp-root"
	SystemProbeSecCompRootVolumePath   = "/host/var/lib/kubelet/seccomp"
	AgentCustomConfigVolumeName        = "custom-datadog-yaml"
	AgentCustomConfigVolumePath        = "/etc/datadog-agent/datadog.yaml"
	AgentCustomConfigVolumeSubPath     = "datadog.yaml"
	HostCriSocketPathPrefix            = "/host"

	ClusterAgentCustomConfigVolumeName    = "custom-datadog-yaml"
	ClusterAgentCustomConfigVolumePath    = "/etc/datadog-agent/datadog-cluster.yaml"
	ClusterAgentCustomConfigVolumeSubPath = "datadog-cluster.yaml"

	DefaultSystemProbeSecCompRootPath = "/var/lib/kubelet/seccomp"
	DefaultAppArmorProfileName        = "unconfined"
	DefaultSeccompProfileName         = "localhost/system-probe"
	SysteProbeAppArmorAnnotationKey   = "container.apparmor.security.beta.kubernetes.io/system-probe"
	SysteProbeSeccompAnnotationKey    = "container.seccomp.security.alpha.kubernetes.io/system-probe"

	// Extra config provider names

	KubeServicesConfigProvider              = "kube_services"
	KubeEndpointsConfigProvider             = "kube_endpoints"
	KubeServicesAndEndpointsConfigProviders = "kube_services kube_endpoints"
	ClusterChecksConfigProvider             = "clusterchecks"
	EndpointsChecksConfigProvider           = "endpointschecks"
	ClusterAndEndpointsConfigPoviders       = "clusterchecks endpointschecks"

	// Extra listeners

	KubeServicesListener              = "kube_services"
	KubeEndpointsListener             = "kube_endpoints"
	KubeServicesAndEndpointsListeners = "kube_services kube_endpoints"

	// Liveness probe default config

	DefaultLivenessProbeInitialDelaySeconds int32 = 15
	DefaultLivenessProbePeriodSeconds       int32 = 15
	DefaultLivenessProbeTimeoutSeconds      int32 = 5
	DefaultLivenessProbeSuccessThreshold    int32 = 1
	DefaultLivenessProbeFailureThreshold    int32 = 6
	DefaultAgentHealthPort                  int32 = 5555
	DefaultLivenessProbeHTTPPath                  = "/live"

	// Readiness probe default config

	DefaultReadinessProbeInitialDelaySeconds int32 = 15
	DefaultReadinessProbePeriodSeconds       int32 = 15
	DefaultReadinessProbeTimeoutSeconds      int32 = 5
	DefaultReadinessProbeSuccessThreshold    int32 = 1
	DefaultReadinessProbeFailureThreshold    int32 = 6
	DefaultReadinessProbeHTTPPath                  = "/ready"

	// APM default values

	DefaultAPMAgentTCPPort int32 = 8126

	// Consts used to setup Rbac config
	// API Groups

	CoreAPIGroup           = ""
	OpenShiftQuotaAPIGroup = "quota.openshift.io"
	RbacAPIGroup           = "rbac.authorization.k8s.io"
	AutoscalingAPIGroup    = "autoscaling"
	DatadogAPIGroup        = "datadoghq.com"
	AdmissionAPIGroup      = "admissionregistration.k8s.io"
	AppsAPIGroup           = "apps"
	BatchAPIGroup          = "batch"

	// Resources

	ServicesResource                 = "services"
	EventsResource                   = "events"
	EndpointsResource                = "endpoints"
	PodsResource                     = "pods"
	NodesResource                    = "nodes"
	ComponentStatusesResource        = "componentstatuses"
	ConfigMapsResource               = "configmaps"
	ClusterResourceQuotasResource    = "clusterresourcequotas"
	NodeMetricsResource              = "nodes/metrics"
	NodeSpecResource                 = "nodes/spec"
	NodeProxyResource                = "nodes/proxy"
	NodeStats                        = "nodes/stats"
	HorizontalPodAutoscalersRecource = "horizontalpodautoscalers"
	DatadogMetricsResource           = "datadogmetrics"
	DatadogMetricsStatusResource     = "datadogmetrics/status"
	MutatingConfigResource           = "mutatingwebhookconfigurations"
	SecretsResource                  = "secrets"
	ReplicasetsResource              = "replicasets"
	DeploymentsResource              = "deployments"
	StatefulsetsResource             = "statefulsets"
	JobsResource                     = "jobs"
	CronjobsResource                 = "cronjobs"

	// Resource names

	DatadogTokenResourceName           = "datadogtoken"
	DatadogLeaderElectionResourceName  = "datadog-leader-election"
	DatadogCustomMetricsResourceName   = "datadog-custom-metrics"
	ExtensionAPIServerAuthResourceName = "extension-apiserver-authentication"

	// Non resource URLs

	VersionURL = "/version"
	HealthzURL = "/healthz"
	MetricsURL = "/metrics"

	// Verbs

	GetVerb    = "get"
	ListVerb   = "list"
	WatchVerb  = "watch"
	UpdateVerb = "update"
	CreateVerb = "create"
	DeleteVerb = "delete"

	// Rbac resource kinds

	ClusterRoleKind    = "ClusterRole"
	RoleKind           = "Role"
	ServiceAccountKind = "ServiceAccount"
)
