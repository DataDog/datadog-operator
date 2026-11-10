// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package common

import corev1 "k8s.io/api/core/v1"

const (
	DDAPMInstrumentationInstallTime       = "DD_INSTRUMENTATION_INSTALL_TIME"
	DDAPMInstrumentationInstallID         = "DD_INSTRUMENTATION_INSTALL_ID"
	DDAPMInstrumentationInstallType       = "DD_INSTRUMENTATION_INSTALL_TYPE"
	DDAPMErrorTrackingStandaloneEnabled   = "DD_APM_ERROR_TRACKING_STANDALONE_ENABLED"
	DDClusterAgentEnabled                 = "DD_CLUSTER_AGENT_ENABLED"
	DDClusterAgentKubeServiceName         = "DD_CLUSTER_AGENT_KUBERNETES_SERVICE_NAME"
	DDClusterAgentURL                     = "DD_CLUSTER_AGENT_URL"
	DDClusterAgentTokenName               = "DD_CLUSTER_AGENT_TOKEN_NAME"
	DDAuthTokenFilePath                   = "DD_AUTH_TOKEN_FILE_PATH"
	DDContainerCollectionEnabled          = "DD_PROCESS_CONFIG_CONTAINER_COLLECTION_ENABLED"
	DDDogstatsdEnabled                    = "DD_USE_DOGSTATSD"
	DDHealthPlatformEnabled               = "DD_HEALTH_PLATFORM_ENABLED"
	DDHealthPort                          = "DD_HEALTH_PORT"
	DDHostRootEnvVar                      = "HOST_ROOT"
	DDJMXUseContainerSupport              = "DD_JMX_USE_CONTAINER_SUPPORT"
	DDKubeletHost                         = "DD_KUBERNETES_KUBELET_HOST"
	DDLeaderElection                      = "DD_LEADER_ELECTION"
	DDLogsEnabled                         = "DD_LOGS_ENABLED"
	DDProcessCollectionEnabled            = "DD_PROCESS_CONFIG_PROCESS_COLLECTION_ENABLED"
	DDProcessConfigRunInCoreAgent         = "DD_PROCESS_CONFIG_RUN_IN_CORE_AGENT_ENABLED"
	DDServiceDiscoveryEnabled             = "DD_DISCOVERY_ENABLED"
	DDSystemProbeEnabled                  = "DD_SYSTEM_PROBE_ENABLED"
	DDSystemProbeExternal                 = "DD_SYSTEM_PROBE_EXTERNAL"
	DDSystemProbeSocket                   = "DD_SYSPROBE_SOCKET"
	DDADPEnabled                          = "DD_ADP_ENABLED" // Deprecated: Use DDDataPlaneEnabled instead
	DDDataPlaneEnabled                    = "DD_DATA_PLANE_ENABLED"
	DDDataPlaneDogstatsdEnabled           = "DD_DATA_PLANE_DOGSTATSD_ENABLED"
	DDDataPlaneOTLPEnabled                = "DD_DATA_PLANE_OTLP_ENABLED"
	DDDataPlaneRemoteAgentEnabled         = "DD_DATA_PLANE_REMOTE_AGENT_ENABLED"
	DDDataPlaneUseNewConfigStreamEndpoint = "DD_DATA_PLANE_USE_NEW_CONFIG_STREAM_ENDPOINT"
	DDDataPlaneChecksEnabled              = "DD_DATA_PLANE_CHECKS_ENABLED"
	DDKubernetesPodResourcesSocket        = "DD_KUBERNETES_KUBELET_PODRESOURCES_SOCKET"
	DDKubernetesUseEndpointSlices         = "DD_KUBERNETES_USE_ENDPOINT_SLICES"
	DDCELWorkloadExclude                  = "DD_CEL_WORKLOAD_EXCLUDE"
	DDAgentIpcPort                        = "DD_AGENT_IPC_PORT"
	DDAgentIpcConfigRefreshInterval       = "DD_AGENT_IPC_CONFIG_REFRESH_INTERVAL"

	// KubernetesEnvvarName Env var used by the Datadog Agent container entrypoint
	// to add kubelet config provider and listener
	KubernetesEnvVar = "KUBERNETES" //common
)

// HealthPlatformDisabledEnvVar turns off the Agent Health Platform (health_platform.enabled).
// It is set by default on the containers that run Autodiscovery (node Agent, Cluster Agent and
// Cluster Checks Runner) as a temporary mitigation: Agents 7.79.0+ report Autodiscovery annotation
// issues to it synchronously, which can stall Autodiscovery long enough that short-lived pods never
// get their checks or log sources scheduled. Users opt back in by setting
// DD_HEALTH_PLATFORM_ENABLED=true in the component's env override, which takes precedence.
// Agents older than 7.74.0 do not know the setting and ignore it.
func HealthPlatformDisabledEnvVar() corev1.EnvVar {
	return corev1.EnvVar{Name: DDHealthPlatformEnabled, Value: "false"}
}
