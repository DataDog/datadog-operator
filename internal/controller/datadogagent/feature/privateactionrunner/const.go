// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package privateactionrunner

const (
	PrivateActionRunnerConfigPath = "/etc/datadog-agent/privateactionrunner.yaml"

	privateActionRunnerSplitMinVersion = "7.84.0-0"
	privateActionRunnerEntrypoint      = "/opt/entrypoints/privateactionrunner"
	privateActionRunnerProbe           = "/par-probe.sh"
	privateActionRunnerRunPath         = "/opt/datadog-agent/run"
	privateActionRunnerSocketPath      = privateActionRunnerRunPath + "/dd-procmgrd.sock"
	privateActionRunnerRunVolumeName   = "private-action-runner-run"
	privateActionRunnerGracePeriod     = int64(190)

	privateActionRunnerVolumeNameSuffix = "privateactionrunner-config"
	privateActionRunnerFileName         = "privateactionrunner.yaml"
	privateActionRunnerSuffix           = "private-action-runner"

	hostVarLogVolumeName = "host-varlog"
	hostVarLogHostPath   = "/var/log"
	hostVarLogMountPath  = "/host/var/log"
)
