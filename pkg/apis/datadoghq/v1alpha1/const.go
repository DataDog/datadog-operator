// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package v1alpha1

const (
	// AgentDeploymentNameLabelKey label key use to link a Resource to a DatadogAgentDeployment
	AgentDeploymentNameLabelKey = "agentdeployment.datadoghq.com/name"
	// AgentDeploymentComponentLabelKey label key use to know with component is it
	AgentDeploymentComponentLabelKey = "agentdeployment.datadoghq.com/component"
	// MD5AgentDeploymentAnnotationKey annotation key used on ExtendedDaemonSet in order to identify which AgentDeployment have been used to generate it.
	MD5AgentDeploymentAnnotationKey = "agentdeployment.datadoghq.com/agentspechash"

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

	// Datadog env var names
	DDAPIKey                        = "DD_API_KEY"
	DDClusterName                   = "DD_CLUSTER_NAME"
	DDSite                          = "DD_SITE"
	DDddURL                         = "DD_DD_URL"
	DDHealthPort                    = "DD_HEALTH_PORT"
	DDLogLevel                      = "DD_LOG_LEVEL"
	DDPodLabelsAsTags               = "DD_KUBERNETES_POD_LABELS_AS_TAGS"
	DDPodAnnotationsAsTags          = "DD_KUBERNETES_POD_ANNOTATIONS_AS_TAGS"
	DDTags                          = "DD_TAGS"
	DDCollectKubeEvents             = "DD_COLLECT_KUBERNETES_EVENTS"
	DDLeaderElection                = "DD_LEADER_ELECTION"
	DDLogsEnabled                   = "DD_LOGS_ENABLED"
	DDLogsConfigContainerCollectAll = "DD_LOGS_CONFIG_CONTAINER_COLLECT_ALL"
	DDDogstatsdOriginDetection      = "DD_DOGSTATSD_ORIGIN_DETECTION"
	DDClusterAgentEnabled           = "DD_CLUSTER_AGENT_ENABLED"
	DDClusterAgentKubeServiceName   = "DD_CLUSTER_AGENT_KUBERNETES_SERVICE_NAME"
	DDClusterAgentAuthToken         = "DD_CLUSTER_AGENT_AUTH_TOKEN"
	DDMetricsProviderEnabled        = "DD_EXTERNAL_METRICS_PROVIDER_ENABLED"
	DDAppKey                        = "DD_APP_KEY"
	DDClusterChecksRunnerEnabled    = "DD_CLUSTER_CHECKS_ENABLED"
	DDExtraConfigProviders          = "DD_EXTRA_CONFIG_PROVIDERS"
	DDExtraListeners                = "DD_EXTRA_LISTENERS"
	DDHostname                      = "DD_HOSTNAME"
	DDAPMEnabled                    = "DD_APM_ENABLED"
	DDProcessAgentEnabled           = "DD_PROCESS_AGENT_ENABLED"
	DDEnableMetadataCollection      = "DD_ENABLE_METADATA_COLLECTION"

	// Env var used by the Datadog Agent container entrypoint
	// to add kubelet config provider and listener
	KubernetesEnvvarName = "KUBERNETES"

	// Datadog volume names and mount paths
	ConfdVolumeName           = "confd"
	ConfdVolumePath           = "/conf.d"
	ConfigVolumeName          = "config"
	ConfigVolumePath          = "/etc/datadog-agent"
	ProcVolumeName            = "procdir"
	ProcVolumePath            = "/host/proc"
	ProcVolumeReadOnly        = true
	CgroupsVolumeName         = "cgroups"
	CgroupsVolumePath         = "/host/sys/fs/cgroup"
	CgroupsVolumeReadOnly     = true
	CriSockerVolumeName       = "runtimesocket"
	CriSockerVolumeReadOnly   = true
	DogstatsdSockerVolumeName = "dsdsocket"
	DogstatsdSockerVolumePath = "/var/run/datadog"
	PointerVolumeName         = "pointerdir"
	PointerVolumePath         = "/opt/datadog-agent/run"
	LogPodVolumeName          = "logpodpath"
	LogPodVolumePath          = "/var/log/pods"
	LogPodVolumeReadOnly      = true
	LogContainerVolumeName    = "logcontainerpath"
	LogContainerolumeReadOnly = true
	// Extra config provider names
	KubeServicesConfigProvider    = "kube_services"
	KubeEndpointsConfigProvider   = "kube_endpoints"
	ClusterChecksConfigProvider   = "clusterchecks"
	EndpointsChecksConfigProvider = "endpointschecks"
	// Extra listeners
	KubeServicesListener  = "kube_services"
	KubeEndpointsListener = "kube_endpoints"
	// Liveness probe default config
	DefaultLivenessProveInitialDelaySeconds int32 = 15
	DefaultLivenessProvePeriodSeconds       int32 = 15
	DefaultLivenessProveTimeoutSeconds      int32 = 5
	DefaultLivenessProveSuccessThreshold    int32 = 1
	DefaultLivenessProveFailureThreshold    int32 = 6
	DefaultAgentHealthPort                  int32 = 5555
	DefaultLivenessProveHTTPPath                  = "/health"

	// Consts used to setup Rbac config
	// API Groups
	CoreAPIGroup           = ""
	OpenShiftQuotaAPIGroup = "quota.openshift.io"
	RbacAPIGroup           = "rbac.authorization.k8s.io"
	AutoscalingAPIGroup    = "autoscaling"
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
	HorizontalPodAutoscalersRecource = "horizontalpodautoscalers"
	// Resource names
	DatadogTokenResourceName           = "datadogtoken"
	DatadogLeaderElectionResourceName  = "datadog-leader-election"
	DatadogCustomMetricsResourceName   = "datadog-custom-metrics"
	ExtensionApiServerAuthResourceName = "extension-apiserver-authentication"
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
	// Rbac resource kinds
	ClusterRoleKind    = "ClusterRole"
	RoleKind           = "Role"
	ServiceAccountKind = "ServiceAccount"
)
