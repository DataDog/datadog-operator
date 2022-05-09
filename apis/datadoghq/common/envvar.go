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

	DDLogsEnabled                    = "DD_LOGS_ENABLED"
	DDLogsConfigContainerCollectAll  = "DD_LOGS_CONFIG_CONTAINER_COLLECT_ALL"
	DDLogsContainerCollectUsingFiles = "DD_LOGS_CONFIG_K8S_CONTAINER_USE_FILE"
	DDLogsConfigOpenFilesLimit       = "DD_LOGS_CONFIG_OPEN_FILES_LIMIT"
)
