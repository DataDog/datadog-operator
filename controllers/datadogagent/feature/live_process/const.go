// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package liveprocess

const (
	passwdVolumeName string = "passwd"
	passwdVolumePath string = "/etc/passwd"

	// DDProcessAgentEnabledEnvVar enables the process agent
	DDProcessAgentEnabledEnvVar string = "DD_PROCESS_AGENT_ENABLED"
)
