// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package oomkill

const (
	modulesVolumeName = "modules"
	// same path on host and container
	modulesVolumePath = "/lib/modules"
	srcVolumeName     = "src"
	// same path on host and container
	srcVolumePath = "/usr/src"

	// DDEnableOOMKillEnvVar is the env var that enables the OOM kill check
	DDEnableOOMKillEnvVar = "DD_SYSTEM_PROBE_CONFIG_ENABLE_OOM_KILL"
)
