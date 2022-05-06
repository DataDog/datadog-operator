// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tcpqueuelength

const (
	// CELENE TODO MOVE THIS INTO A COMMON FILE (AND CLEANUP THE OLD ONES)
	modulesVolumeName = "modules"
	// same path on host and container
	modulesVolumePath = "/lib/modules"
	srcVolumeName     = "src"
	// same path on host and container
	srcVolumePath = "/usr/src"

	// DDEnableTCPQueueLengthEnvVar is the env var that enables the TCP queue legnth check
	DDEnableTCPQueueLengthEnvVar = "DD_SYSTEM_PROBE_CONFIG_ENABLE_TCP_QUEUE_LENGTH"
)
