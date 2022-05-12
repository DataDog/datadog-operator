// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

// Datadog env var names
const (
	DDIgnoreAutoConf                 = "DD_IGNORE_AUTOCONF"
	DDKubeStateMetricsCoreEnabled    = "DD_KUBE_STATE_METRICS_CORE_ENABLED"
	DDKubeStateMetricsCoreConfigMap  = "DD_KUBE_STATE_METRICS_CORE_CONFIGMAP_NAME"
	DDSystemProbeNPMEnabledEnvVar    = "DD_SYSTEM_PROBE_NETWORK_ENABLED"
	DDSystemProbeEnabledEnvVar       = "DD_SYSTEM_PROBE_ENABLED"
	DDProcessAgentEnabledEnvVar      = "DD_PROCESS_AGENT_ENABLED"
	DDSystemProbeSocketEnvVar        = "DD_SYSPROBE_SOCKET"
	DDEnableOOMKillEnvVar            = "DD_SYSTEM_PROBE_CONFIG_ENABLE_OOM_KILL"
	DDEnableTCPQueueLengthEnvVar     = "DD_SYSTEM_PROBE_CONFIG_ENABLE_TCP_QUEUE_LENGTH"
	DDLeaderElection                 = "DD_LEADER_ELECTION"
	DDClusterAgentKubeServiceName    = "DD_CLUSTER_AGENT_KUBERNETES_SERVICE_NAME"
	DDHealthPort                     = "DD_HEALTH_PORT"
	DDLogsEnabled                    = "DD_LOGS_ENABLED"
	DDLogsConfigContainerCollectAll  = "DD_LOGS_CONFIG_CONTAINER_COLLECT_ALL"
	DDLogsContainerCollectUsingFiles = "DD_LOGS_CONFIG_K8S_CONTAINER_USE_FILE"
	DDLogsConfigOpenFilesLimit       = "DD_LOGS_CONFIG_OPEN_FILES_LIMIT"
	DDDogStatsDNonLocalTraffic       = "DD_DOGSTATSD_NON_LOCAL_TRAFFIC"
	DDDogStatsDSocket                = "DD_DOGSTATSD_SOCKET"
	DDDogstatsdOriginDetection       = "DD_DOGSTATSD_ORIGIN_DETECTION"
	DDDogstatsdMapperProfiles        = "DD_DOGSTATSD_MAPPER_PROFILES"
)
