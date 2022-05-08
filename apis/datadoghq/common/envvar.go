// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

// Datadog env var names
const (
	DDIgnoreAutoConf                = "DD_IGNORE_AUTOCONF"
	DDKubeStateMetricsCoreEnabled   = "DD_KUBE_STATE_METRICS_CORE_ENABLED"
	DDKubeStateMetricsCoreConfigMap = "DD_KUBE_STATE_METRICS_CORE_CONFIGMAP_NAME"
	DDSystemProbeNPMEnabledEnvVar   = "DD_SYSTEM_PROBE_NETWORK_ENABLED"
	DDSystemProbeEnabledEnvVar      = "DD_SYSTEM_PROBE_ENABLED"
	DDProcessAgentEnabledEnvVar     = "DD_PROCESS_AGENT_ENABLED"
	DDSystemProbeSocketEnvVar       = "DD_SYSPROBE_SOCKET"
)
