// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

// This file tracks common constants used across API versions

// // Labels
const (
	// AgentDeploymentNameLabelKey label key use to link a Resource to a DatadogAgent
	AgentDeploymentNameLabelKey = "agent.datadoghq.com/name"
	// AgentDeploymentComponentLabelKey label key use to know with component is it
	AgentDeploymentComponentLabelKey = "agent.datadoghq.com/component"
	// DatadogAgentNameLabelKey is used to know the name of the DatadogAgent
	DatadogAgentNameLabelKey = "agent.datadoghq.com/datadogagent"
	// UpdateSelectorAnnotationKey is used when the workload selector should be updated
	UpdateSelectorAnnotationKey = "agent.datadoghq.com/update-selector"
)
