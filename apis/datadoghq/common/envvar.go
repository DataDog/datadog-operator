// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

// Datadog env var names
const (
	DDIgnoreAutoConf                      = "DD_IGNORE_AUTOCONF"
	DDKubeStateMetricsCoreEnabled         = "DD_KUBE_STATE_METRICS_CORE_ENABLED"
	DDKubeStateMetricsCoreConfigMap       = "DD_KUBE_STATE_METRICS_CORE_CONFIGMAP_NAME"
	DDProcessAgentEnabled                 = "DD_PROCESS_AGENT_ENABLED"
	DDAPMEnabled                          = "DD_APM_ENABLED"
	DDSystemProbeNPMEnabledEnvVar         = "DD_SYSTEM_PROBE_NETWORK_ENABLED"
	DDSystemProbeEnabledEnvVar            = "DD_SYSTEM_PROBE_ENABLED"
	DDSystemProbeExternal                 = "DD_SYSTEM_PROBE_EXTERNAL"
	DDSystemProbeServiceMonitoringEnabled = "DD_SYSTEM_PROBE_SERVICE_MONITORING_ENABLED"
	DDSystemProbeSocket                   = "DD_SYSPROBE_SOCKET"
	DDComplianceEnabled                   = "DD_COMPLIANCE_CONFIG_ENABLED"
	DDComplianceCheckInterval             = "DD_COMPLIANCE_CONFIG_CHECK_INTERVAL"
	DDHostRootEnvVar                      = "HOST_ROOT"
	DDEnableOOMKillEnvVar                 = "DD_SYSTEM_PROBE_CONFIG_ENABLE_OOM_KILL"
	DDEnableTCPQueueLengthEnvVar          = "DD_SYSTEM_PROBE_CONFIG_ENABLE_TCP_QUEUE_LENGTH"
	DDLeaderElection                      = "DD_LEADER_ELECTION"
	DDClusterAgentKubeServiceName         = "DD_CLUSTER_AGENT_KUBERNETES_SERVICE_NAME"
	DDHealthPort                          = "DD_HEALTH_PORT"
	DDLogsEnabled                         = "DD_LOGS_ENABLED"
	DDLogsConfigContainerCollectAll       = "DD_LOGS_CONFIG_CONTAINER_COLLECT_ALL"
	DDLogsContainerCollectUsingFiles      = "DD_LOGS_CONFIG_K8S_CONTAINER_USE_FILE"
	DDLogsConfigOpenFilesLimit            = "DD_LOGS_CONFIG_OPEN_FILES_LIMIT"
	DDPrometheusScrapeEnabled             = "DD_PROMETHEUS_SCRAPE_ENABLED"
	DDPrometheusScrapeServiceEndpoints    = "DD_PROMETHEUS_SCRAPE_SERVICE_ENDPOINTS"
	DDPrometheusScrapeChecks              = "DD_PROMETHEUS_SCRAPE_CHECKS"
	DDCollectKubernetesEvents             = "DD_COLLECT_KUBERNETES_EVENTS"
	DDLeaderLeaseName                     = "DD_LEADER_LEASE_NAME"
	DDClusterAgentTokenName               = "DD_CLUSTER_AGENT_TOKEN_NAME"
	DDClusterAgentEnabled                 = "DD_CLUSTER_AGENT_ENABLED"
	DDClusterChecksEnabled                = "DD_CLUSTER_CHECKS_ENABLED"
	DDCLCRunnerEnabled                    = "DD_CLC_RUNNER_ENABLED"
	DDCLCRunnerHost                       = "DD_CLC_RUNNER_HOST"
	DDCLCRunnerID                         = "DD_CLC_RUNNER_ID"
	DDExtraConfigProviders                = "DD_EXTRA_CONFIG_PROVIDERS"
	DDEnableMetadataCollection            = "DD_ENABLE_METADATA_COLLECTION"
	DDDogstatsdEnabled                    = "DD_USE_DOGSTATSD"
	DDHostname                            = "DD_HOSTNAME"

	// KubernetesEnvvarName Env var used by the Datadog Agent container entrypoint
	// to add kubelet config provider and listener
	KubernetesEnvVar = "KUBERNETES"

	ClusterChecksConfigProvider = "clusterchecks"
)
