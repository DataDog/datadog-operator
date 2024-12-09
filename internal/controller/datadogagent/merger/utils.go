// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package merger

import "github.com/DataDog/datadog-operator/api/datadoghq/common"

// AllAgentContainers is a map of all agent containers
var AllAgentContainers = map[common.AgentContainerName]struct{}{
	// Node agent containers
	common.CoreAgentContainerName:      {},
	common.TraceAgentContainerName:     {},
	common.ProcessAgentContainerName:   {},
	common.SecurityAgentContainerName:  {},
	common.SystemProbeContainerName:    {},
	common.OtelAgent:                   {},
	common.AgentDataPlaneContainerName: {},
	// DCA containers
	common.ClusterAgentContainerName: {},
	// CCR container name is equivalent to core agent container name
	// Single Agent container
	common.UnprivilegedSingleAgentContainerName: {},
}
