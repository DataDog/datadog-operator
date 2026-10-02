// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clusterchecks

const (
	DDClusterChecksEnabled = "DD_CLUSTER_CHECKS_ENABLED"
	DDExtraConfigProviders = "DD_EXTRA_CONFIG_PROVIDERS"
	DDExtraListeners       = "DD_EXTRA_LISTENERS"
	// DDCLCRunnerGroup is a runner group pod's group name (clc_runner_group).
	DDCLCRunnerGroup = "DD_CLC_RUNNER_GROUP"
	// DDClusterChecksRunnerGroups is the Cluster Agent's JSON map of each
	// runner group to the checks it claims (cluster_checks.runner_groups).
	DDClusterChecksRunnerGroups = "DD_CLUSTER_CHECKS_RUNNER_GROUPS"
)
