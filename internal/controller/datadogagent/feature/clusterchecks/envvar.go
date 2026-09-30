// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clusterchecks

const (
	DDClusterChecksEnabled = "DD_CLUSTER_CHECKS_ENABLED"
	DDExtraConfigProviders = "DD_EXTRA_CONFIG_PROVIDERS"
	DDExtraListeners       = "DD_EXTRA_LISTENERS"
	// DD_EXPERIMENTAL_* names: the agent-side config keys are experimental
	// (experimental.clc_runner_group[s]), not yet stable API.
	// DDCLCRunnerGroup is a runner group pod's group name.
	DDCLCRunnerGroup = "DD_EXPERIMENTAL_CLC_RUNNER_GROUP"
	// DDCLCRunnerGroups is the Cluster Agent's JSON map of group -> claimed checks.
	DDCLCRunnerGroups = "DD_EXPERIMENTAL_CLC_RUNNER_GROUPS"
)
