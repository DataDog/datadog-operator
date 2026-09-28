// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clusterchecks

const (
	DDClusterChecksEnabled   = "DD_CLUSTER_CHECKS_ENABLED"
	DDExtraConfigProviders   = "DD_EXTRA_CONFIG_PROVIDERS"
	DDExtraListeners         = "DD_EXTRA_LISTENERS"
	// DD_EXPERIMENTAL_* names: the agent-side config keys are experimental
	// (experimental.clc_runner_checks_*), not yet stable API.
	DDCLCRunnerChecksInclude = "DD_EXPERIMENTAL_CLC_RUNNER_CHECKS_INCLUDE"
	DDCLCRunnerChecksExclude = "DD_EXPERIMENTAL_CLC_RUNNER_CHECKS_EXCLUDE"
)
