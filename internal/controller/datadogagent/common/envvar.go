// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package common

const (
	DDAPMEnabled                        = "DD_APM_ENABLED"
	DDAPMInstrumentationInstallTime     = "DD_INSTRUMENTATION_INSTALL_TIME"
	DDAPMInstrumentationInstallId       = "DD_INSTRUMENTATION_INSTALL_ID"
	DDAPMInstrumentationInstallType     = "DD_INSTRUMENTATION_INSTALL_TYPE"
	DDAPMErrorTrackingStandaloneEnabled = "DD_APM_ERROR_TRACKING_STANDALONE_ENABLED"
	DDClusterAgentEnabled               = "DD_CLUSTER_AGENT_ENABLED"
	DDClusterAgentKubeServiceName       = "DD_CLUSTER_AGENT_KUBERNETES_SERVICE_NAME"
	DDClusterAgentURL                   = "DD_CLUSTER_AGENT_URL"
	DDClusterAgentTokenName             = "DD_CLUSTER_AGENT_TOKEN_NAME"
	DDAuthTokenFilePath                 = "DD_AUTH_TOKEN_FILE_PATH"
	DDContainerCollectionEnabled        = "DD_PROCESS_CONFIG_CONTAINER_COLLECTION_ENABLED"
	DDDogstatsdEnabled                  = "DD_USE_DOGSTATSD"
	DDHealthPort                        = "DD_HEALTH_PORT"
	DDHostRootEnvVar                    = "HOST_ROOT"
	DDKubeletHost                       = "DD_KUBERNETES_KUBELET_HOST"
	DDLeaderElection                    = "DD_LEADER_ELECTION"
	DDLogsEnabled                       = "DD_LOGS_ENABLED"
	DDProcessCollectionEnabled          = "DD_PROCESS_CONFIG_PROCESS_COLLECTION_ENABLED"
	DDProcessConfigRunInCoreAgent       = "DD_PROCESS_CONFIG_RUN_IN_CORE_AGENT_ENABLED"
	DDSystemProbeEnabled                = "DD_SYSTEM_PROBE_ENABLED"
	DDSystemProbeExternal               = "DD_SYSTEM_PROBE_EXTERNAL"
	DDSystemProbeSocket                 = "DD_SYSPROBE_SOCKET"
	DDADPEnabled                        = "DD_ADP_ENABLED"
	DDKubernetesPodResourcesSocket      = "DD_KUBERNETES_KUBELET_PODRESOURCES_SOCKET"
	DDCELWorkloadExclude                = "DD_CEL_WORKLOAD_EXCLUDE"

	// KubernetesEnvvarName Env var used by the Datadog Agent container entrypoint
	// to add kubelet config provider and listener
	KubernetesEnvVar = "KUBERNETES" //common
)
