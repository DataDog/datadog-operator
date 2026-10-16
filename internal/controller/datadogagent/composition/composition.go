// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package composition defines which components and node Agent containers each
// node Agent mode allows. It is a leaf package so features, the pod builders and
// the controllers can all read the same policy.
package composition

import (
	"slices"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
)

// ModeAnnotationKey selects the node Agent mode. It shares the experimental annotation prefix.
const ModeAnnotationKey = "experimental.agent.datadoghq.com/node-agent-mode"

// Mode is a node Agent composition mode.
type Mode string

const (
	// ModeDefault is today's behavior: the core Agent is always present.
	ModeDefault Mode = ""
	// ModeStandalone runs the node Agent without the core Agent, Cluster Agent or Cluster Checks Runner.
	// Containers come from enabled standalone-capable features (DDOT, host profiler).
	ModeStandalone Mode = "standalone"
)

// Policy describes what a mode allows.
type Policy struct {
	Mode Mode
	// nodeAgentContainers is the node Agent container allowlist; nil means unrestricted.
	nodeAgentContainers []apicommon.AgentContainerName
	ClusterAgent        bool
	ClusterChecksRunner bool
	OtelAgentGateway    bool
}

var policies = map[Mode]Policy{
	ModeDefault: {
		Mode:                ModeDefault,
		ClusterAgent:        true,
		ClusterChecksRunner: true,
		OtelAgentGateway:    true,
	},
	ModeStandalone: {
		Mode:                ModeStandalone,
		nodeAgentContainers: []apicommon.AgentContainerName{apicommon.OtelAgent, apicommon.HostProfiler},
		OtelAgentGateway:    true,
	},
}

// ModeOf returns the mode set on obj. Unknown or missing values resolve to ModeDefault.
func ModeOf(obj metav1.Object) Mode {
	mode := Mode(obj.GetAnnotations()[ModeAnnotationKey])
	if _, ok := policies[mode]; !ok {
		return ModeDefault
	}
	return mode
}

// PolicyFor returns the policy of mode, falling back to the default policy.
func PolicyFor(mode Mode) Policy {
	if p, ok := policies[mode]; ok {
		return p
	}
	return policies[ModeDefault]
}

// PolicyOf returns the policy of the mode set on obj.
func PolicyOf(obj metav1.Object) Policy {
	return PolicyFor(ModeOf(obj))
}

// Composed returns true when the policy restricts the default node Agent composition.
func (p Policy) Composed() bool {
	return p.Mode != ModeDefault
}

// AllowsNodeAgentContainer returns true if the node Agent pod may run the container.
func (p Policy) AllowsNodeAgentContainer(name apicommon.AgentContainerName) bool {
	return p.nodeAgentContainers == nil || slices.Contains(p.nodeAgentContainers, name)
}

// NodeAgentContainers filters the containers a feature would like, keeping their order.
func (p Policy) NodeAgentContainers(wanted ...apicommon.AgentContainerName) []apicommon.AgentContainerName {
	out := make([]apicommon.AgentContainerName, 0, len(wanted))
	for _, name := range wanted {
		if p.AllowsNodeAgentContainer(name) {
			out = append(out, name)
		}
	}
	return out
}
