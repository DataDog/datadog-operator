// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cws

const (
	cwsConfigVolumeName = "customruntimepoliciesdir"
	cwsConfFileName     = "custom_with_operator.policy"

	securityAgentRuntimeCustomPoliciesVolumePath = "/etc/datadog-agent-runtime-policies"
	securityAgentRuntimePoliciesDirVolumeName    = "runtimepoliciesdir"
	securityAgentRuntimePoliciesDirVolumePath    = "/etc/datadog-agent/runtime-security.d"

	tracefsVolumeName    = "tracefs"
	tracefsPath          = "/sys/kernel/tracing"
	securityfsVolumeName = "securityfs"
	securityfsVolumePath = "/sys/kernel/security"
	securityfsMountPath  = "/host/sys/kernel/security"
)
