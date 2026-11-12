// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package componenthealthsuite

import (
	"github.com/DataDog/agent-payload/v5/healthplatform"

	"github.com/DataDog/datadog-operator/pkg/componenthealth"
)

// TestComponentHealthReporting drives the component-level issue lifecycle on the
// managed Cluster Agent and asserts the operator emits the matching HealthReport
// to the (fake) agenthealth intake:
//
//  1. inject an image-pull fault -> image_pull_failure reported ACTIVE (detect);
//  2. remediate (healthy DatadogAgent) -> image_pull_failure reported RESOLVED;
//  3. inject an unschedulable fault -> component_unschedulable reported ACTIVE.
//
// Only deterministic, credential-independent faults are asserted in CI; the
// Cluster Agent never needs to reach a running state for these to be observed.
func (s *componentHealthSuite) TestComponentHealthReporting() {
	// Delete the DatadogAgent before the Pulumi stack destroy so CRD teardown does
	// not deadlock on operator finalizers.
	s.registerDatadogResourceCleanup()

	s.Run("image_pull_failure is reported ACTIVE", func() {
		s.applyDatadogAgent(clusterAgentImagePullFault())
		s.waitForIssueState(componenthealth.IssueImagePullFailure, healthplatform.IssueState_ISSUE_STATE_ACTIVE,
			detectTimeout, "operator did not emit image_pull_failure as ACTIVE after injecting the bad image")
	})

	s.Run("image_pull_failure is reported RESOLVED after remediation", func() {
		s.applyDatadogAgent(nil) // healthy: drops the image override
		s.waitForIssueState(componenthealth.IssueImagePullFailure, healthplatform.IssueState_ISSUE_STATE_RESOLVED,
			resolveTimeout, "operator did not emit image_pull_failure as RESOLVED after remediation")
	})

	s.Run("unschedulable is reported ACTIVE", func() {
		s.applyDatadogAgent(clusterAgentUnschedulableFault())
		s.waitForIssueState(componenthealth.IssueUnschedulable, healthplatform.IssueState_ISSUE_STATE_ACTIVE,
			detectTimeout, "operator did not emit component_unschedulable as ACTIVE after pinning to an unschedulable nodeSelector")
	})
}
