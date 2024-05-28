// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import "fmt"

const (
	// ADPrefix prefix used for AD annotations
	ADPrefix = "ad.datadoghq.com/"
	// ADPrefixRegex used for matching AD annotations
	ADPrefixRegex = "ad\\.datadoghq\\.com/"
	// AgentLabelValue label value to define the Agent
	AgentLabelValue = "agent"
	// ComponentLabelKey label key used to define the datadog agent component
	ComponentLabelKey = "agent.datadoghq.com/component"
	// ClcRunnerLabelValue label value to define the Cluster Checks Runner
	ClcRunnerLabelValue = "cluster-checks-runner"
)

var (
	// AgentLabel can be used as a LabelSelector for the Agent
	AgentLabel = fmt.Sprintf("%s=%s", ComponentLabelKey, AgentLabelValue)
	// ClcRunnerLabel can be used as a LabelSelector for the Cluster Checks Runner
	ClcRunnerLabel = fmt.Sprintf("%s=%s", ComponentLabelKey, ClcRunnerLabelValue)
)
