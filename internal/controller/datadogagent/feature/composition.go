// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package feature

import (
	"fmt"

	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/composition"
)

// DroppedFeature is an enabled feature removed because the node Agent mode disallows what it requires.
type DroppedFeature struct {
	ID      IDType
	Reasons []string
}

// policyViolations lists what rc requires that the policy disallows.
// A component explicitly set to not required (IsRequired=false) is not a violation.
func policyViolations(p composition.Policy, rc RequiredComponents) []string {
	if !p.Composed() {
		return nil
	}

	var reasons []string
	for _, name := range rc.Agent.Containers {
		if !p.AllowsNodeAgentContainer(name) {
			reasons = append(reasons, fmt.Sprintf("node Agent container %q", name))
		}
	}
	// Requiring the node Agent without naming containers implies the core Agent.
	if rc.Agent.IsEnabled() && len(rc.Agent.Containers) == 0 {
		reasons = append(reasons, "node Agent core container")
	}
	if !p.ClusterAgent && requestsComponent(rc.ClusterAgent) {
		reasons = append(reasons, "Cluster Agent")
	}
	if !p.ClusterChecksRunner && requestsComponent(rc.ClusterChecksRunner) {
		reasons = append(reasons, "Cluster Checks Runner")
	}
	if !p.OtelAgentGateway && requestsComponent(rc.OtelAgentGateway) {
		reasons = append(reasons, "OTel Agent Gateway")
	}
	return reasons
}

func requestsComponent(rc RequiredComponent) bool {
	return rc.IsEnabled() || rc.IsConfigured()
}
