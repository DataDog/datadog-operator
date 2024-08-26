// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package merger

import commonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"

// AllAgentContainers is a map of all agent containers
var AllAgentContainers = map[commonv1.AgentContainerName]struct{}{
	// Node agent containers
	commonv1.CoreAgentContainerName:      {},
	commonv1.TraceAgentContainerName:     {},
	commonv1.ProcessAgentContainerName:   {},
	commonv1.SecurityAgentContainerName:  {},
	commonv1.SystemProbeContainerName:    {},
	commonv1.OtelAgent:                   {},
	commonv1.AgentDataPlaneContainerName: {},
	// DCA containers
	commonv1.ClusterAgentContainerName: {},
	// CCR container name is equivalent to core agent container name
	// Single Agent container
	commonv1.UnprivilegedSingleAgentContainerName: {},
}
