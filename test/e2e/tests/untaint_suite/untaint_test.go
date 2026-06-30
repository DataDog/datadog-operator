// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package untaintsuite

import (
	"time"
)

// TestAgentOnlyUntaintFlow validates the agent-only onboarding flow
// (--untaintControllerEnabled=true, without wait-for-CSI):
//
//  1. a workload pinned to the pre-tainted node stays Pending while no Agent is
//     present and the taint remains;
//  2. once the DatadogAgent is deployed and the node Agent becomes Ready, the
//     untaint controller removes the taint;
//  3. the workload then schedules and runs.
func (s *untaintSuite) TestAgentOnlyUntaintFlow() {
	if s.waitForCSI {
		s.T().Skip("suite configured for wait-for-CSI; skipping agent-only flow")
	}
	ctx := s.T().Context()

	// Deploy the workload at the parent-test level so its cleanup is scoped to the
	// whole test method. If created inside an s.Run subtest, s.T() is the subtest's
	// T and the cleanup (which deletes the workload) would fire when that subtest
	// ends — removing the workload before the next subtest can observe it.
	s.deployPendingWorkload(ctx)

	s.Run("Workload stays Pending while node is tainted and Agent is absent", func() {
		s.Require().Never(func() bool { return s.workloadRunning(ctx) },
			45*time.Second, 5*time.Second,
			"workload must stay Pending while the node is tainted and no Agent is running")
		s.Require().Truef(s.nodeHasAgentNotReadyTaint(s.taintedNode),
			"taint must remain on %s before the Agent is deployed", s.taintedNode)
	})

	s.Run("Taint removed and workload schedules once Agent is Ready", func() {
		s.applyDDA()
		s.waitForAgentReadyOnNode(ctx, s.taintedNode)

		s.Require().Eventuallyf(func() bool { return !s.nodeHasAgentNotReadyTaint(s.taintedNode) },
			5*time.Minute, 10*time.Second,
			"untaint controller should remove the taint from %s after the Agent is Ready", s.taintedNode)

		s.assertWorkloadEventuallyRunning(ctx)
	})
}

// TestWaitForCSIUntaintFlow validates the dual-readiness onboarding flow
// (--untaintControllerEnabled=true and --untaintControllerWaitForCSIDriver=true):
//
//  1. a workload pinned to the pre-tainted node stays Pending;
//  2. after the node Agent becomes Ready, the taint MUST still persist because
//     the CSI node-server is not Ready yet (this is the dual-readiness gate);
//  3. once the DatadogCSIDriver is deployed and the CSI node-server becomes
//     Ready, the controller removes the taint and the workload schedules.
func (s *untaintSuite) TestWaitForCSIUntaintFlow() {
	if !s.waitForCSI {
		s.T().Skip("suite configured for agent-only; skipping wait-for-CSI flow")
	}
	ctx := s.T().Context()

	// Deploy the workload at the parent-test level (see note in TestAgentOnlyUntaintFlow):
	// a subtest-scoped cleanup would delete it before later subtests observe it.
	s.deployPendingWorkload(ctx)

	s.Run("Workload stays Pending while node is tainted and Agent is absent", func() {
		s.Require().Never(func() bool { return s.workloadRunning(ctx) },
			45*time.Second, 5*time.Second,
			"workload must stay Pending while the node is tainted and no Agent is running")
	})

	s.Run("Taint persists while only the Agent is Ready (CSI not yet deployed)", func() {
		s.applyDDA()
		s.waitForAgentReadyOnNode(ctx, s.taintedNode)

		// Dual-readiness gate: Agent readiness alone must NOT remove the taint.
		s.Require().Never(func() bool { return !s.nodeHasAgentNotReadyTaint(s.taintedNode) },
			60*time.Second, 5*time.Second,
			"taint must persist on %s while the CSI node-server is not Ready", s.taintedNode)
		s.Require().Falsef(s.workloadRunning(ctx),
			"workload must stay Pending until the CSI node-server is also Ready")
	})

	s.Run("Taint removed and workload schedules once Agent and CSI are Ready", func() {
		s.applyCSIDriver(ctx)
		s.waitForCSINodeServerReadyOnNode(ctx, s.taintedNode)

		s.Require().Eventuallyf(func() bool { return !s.nodeHasAgentNotReadyTaint(s.taintedNode) },
			5*time.Minute, 10*time.Second,
			"untaint controller should remove the taint from %s after both Agent and CSI are Ready", s.taintedNode)

		s.assertWorkloadEventuallyRunning(ctx)
	})
}
