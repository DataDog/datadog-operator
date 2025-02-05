// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v2alpha1

const (
	DDAPIKey      = "DD_API_KEY"
	DDAppKey      = "DD_APP_KEY"
	DDddURL       = "DD_DD_URL"
	DDURL         = "DD_URL"
	DDSite        = "DD_SITE"
	DDLogLevel    = "DD_LOG_LEVEL"
	DDClusterName = "DD_CLUSTER_NAME"

	DDAPMEnabled                           = "DD_APM_ENABLED"
	DDAPMInstrumentationInstallTime        = "DD_INSTRUMENTATION_INSTALL_TIME"
	DDAPMInstrumentationInstallId          = "DD_INSTRUMENTATION_INSTALL_ID"
	DDAPMInstrumentationInstallType        = "DD_INSTRUMENTATION_INSTALL_TYPE"
	DDAuthTokenFilePath                    = "DD_AUTH_TOKEN_FILE_PATH"
	DDChecksTagCardinality                 = "DD_CHECKS_TAG_CARDINALITY"
	DDClcRunnerEnabled                     = "DD_CLC_RUNNER_ENABLED"
	DDClcRunnerHost                        = "DD_CLC_RUNNER_HOST"
	DDClcRunnerID                          = "DD_CLC_RUNNER_ID"
	DDClusterAgentEnabled                  = "DD_CLUSTER_AGENT_ENABLED"
	DDClusterAgentKubeServiceName          = "DD_CLUSTER_AGENT_KUBERNETES_SERVICE_NAME"
	DDClusterAgentTokenName                = "DD_CLUSTER_AGENT_TOKEN_NAME"
	DDContainerCollectionEnabled           = "DD_PROCESS_CONFIG_CONTAINER_COLLECTION_ENABLED"
	DDContainerImageEnabled                = "DD_CONTAINER_IMAGE_ENABLED"
	DDCriSocketPath                        = "DD_CRI_SOCKET_PATH"
	DDDogstatsdEnabled                     = "DD_USE_DOGSTATSD"
	DDEnableMetadataCollection             = "DD_ENABLE_METADATA_COLLECTION"
	DDFIPSEnabled                          = "DD_FIPS_ENABLED"
	DDFIPSPortRangeStart                   = "DD_FIPS_PORT_RANGE_START"
	DDFIPSUseHTTPS                         = "DD_FIPS_HTTPS"
	DDFIPSLocalAddress                     = "DD_FIPS_LOCAL_ADDRESS"
	DDHealthPort                           = "DD_HEALTH_PORT"
	DDHostname                             = "DD_HOSTNAME"
	DDHostRootEnvVar                       = "HOST_ROOT"
	DDKubeletCAPath                        = "DD_KUBELET_CLIENT_CA"
	DDKubeletHost                          = "DD_KUBERNETES_KUBELET_HOST"
	DDKubeletTLSVerify                     = "DD_KUBELET_TLS_VERIFY"
	DDKubeResourcesNamespace               = "DD_KUBE_RESOURCES_NAMESPACE"
	DDKubernetesResourcesLabelsAsTags      = "DD_KUBERNETES_RESOURCES_LABELS_AS_TAGS"
	DDKubernetesResourcesAnnotationsAsTags = "DD_KUBERNETES_RESOURCES_ANNOTATIONS_AS_TAGS"
	DDKubernetesPodResourcesSocket         = "DD_KUBERNETES_KUBELET_PODRESOURCES_SOCKET"
	DDLeaderElection                       = "DD_LEADER_ELECTION"
	DDLogsEnabled                          = "DD_LOGS_ENABLED"
	DDNamespaceLabelsAsTags                = "DD_KUBERNETES_NAMESPACE_LABELS_AS_TAGS"
	DDNamespaceAnnotationsAsTags           = "DD_KUBERNETES_NAMESPACE_ANNOTATIONS_AS_TAGS"
	DDNodeLabelsAsTags                     = "DD_KUBERNETES_NODE_LABELS_AS_TAGS"
	DDOriginDetectionUnified               = "DD_ORIGIN_DETECTION_UNIFIED"
	DDPodAnnotationsAsTags                 = "DD_KUBERNETES_POD_ANNOTATIONS_AS_TAGS"
	DDPodLabelsAsTags                      = "DD_KUBERNETES_POD_LABELS_AS_TAGS"
	DDPodName                              = "DD_POD_NAME"
	DDProcessCollectionEnabled             = "DD_PROCESS_CONFIG_PROCESS_COLLECTION_ENABLED"
	DDProcessConfigRunInCoreAgent          = "DD_PROCESS_CONFIG_RUN_IN_CORE_AGENT_ENABLED"
	DDSecretBackendCommand                 = "DD_SECRET_BACKEND_COMMAND"
	DDSecretBackendArguments               = "DD_SECRET_BACKEND_ARGUMENTS"
	DDSecretBackendTimeout                 = "DD_SECRET_BACKEND_TIMEOUT"
	DDSystemProbeEnabled                   = "DD_SYSTEM_PROBE_ENABLED"
	DDSystemProbeExternal                  = "DD_SYSTEM_PROBE_EXTERNAL"
	DDSystemProbeSocket                    = "DD_SYSPROBE_SOCKET"
	DDTags                                 = "DD_TAGS"
	DDAgentIpcPort                         = "DD_AGENT_IPC_PORT"
	DDAgentIpcConfigRefreshInterval        = "DD_AGENT_IPC_CONFIG_REFRESH_INTERVAL"

	// otelcollector core agent configs
	DDOtelCollectorCoreConfigEnabled          = "DD_OTELCOLLECTOR_ENABLED"
	DDOtelCollectorCoreConfigExtensionURL     = "DD_OTELCOLLECTOR_EXTENSION_URL"
	DDOtelCollectorCoreConfigExtensionTimeout = "DD_OTELCOLLECTOR_EXTENSION_TIMEOUT"

	DockerHost = "DOCKER_HOST"
	// KubernetesEnvvarName Env var used by the Datadog Agent container entrypoint
	// to add kubelet config provider and listener
	KubernetesEnvVar = "KUBERNETES"
)
