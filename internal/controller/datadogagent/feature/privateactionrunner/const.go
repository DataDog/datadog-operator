// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package privateactionrunner

const (
	PrivateActionRunnerConfigPath = "/etc/datadog-agent/privateactionrunner.yaml"

	privateActionRunnerVolumeNameSuffix           = "privateactionrunner-config"
	privateActionRunnerFileName                   = "privateactionrunner.yaml"
	privateActionRunnerProcessManagerPath         = "/opt/datadog-agent/embedded/bin/dd-procmgrd"
	privateActionRunnerProcessManagerSocketPath   = "/opt/datadog-agent/run/dd-procmgrd.sock"
	privateActionRunnerProcessConfigPath          = "/opt/datadog-agent/privateactionrunner/processes.d"
	privateActionRunnerProcessConfigPathEnvVar    = "DD_PM_CONFIG_DIR"
	privateActionRunnerProcessManagerSocketEnvVar = "DD_PM_SOCKET_PATH"
	privateActionRunnerSuffix                     = "private-action-runner"

	hostVarLogVolumeName = "host-varlog"
	hostVarLogHostPath   = "/var/log"
	hostVarLogMountPath  = "/host/var/log"
)
