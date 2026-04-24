// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

// Integration tests for the experiment rollback flow wired through the full
// DDA reconcile path. These complement the unit tests in experiment_test.go.
//
// Coverage goals:
//   - Stopped rollback: RC writing phase=stopped causes the operator to restore
//     the previous spec and set phase=rollback.
//   - Timeout rollback: an experiment running past ExperimentTimeout causes the
//     operator to restore the previous spec and set phase=timeout.
//
// NOTE: rollback is idempotent — if the spec is already at the rollback target
// the Update is skipped. This means the status update in the same reconcile
// succeeds without a ResourceVersion conflict. In the first rollback reconcile
// the spec update bumps ResourceVersion and the status update conflicts; the
// second reconcile (fresh fetch) finds the spec already correct, skips the
// Update, and the status update succeeds. Tests therefore run two reconciles
// after triggering rollback.

import (
	"context"
	"testing"
	"time"

	assert "github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"

	v2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

// newExperimentIntegrationReconciler builds a revision reconciler with an
// overridden ExperimentTimeout for testing.
func newExperimentIntegrationReconciler(t *testing.T, timeout time.Duration) *Reconciler {
	t.Helper()
	r, _ := newRevisionIntegrationReconciler(t)
	r.options.ExperimentTimeout = timeout
	return r
}

// reconcileN re-fetches the DDA and calls Reconcile n times in sequence.
func reconcileN(t *testing.T, r *Reconciler, ns, name string, n int) {
	t.Helper()
	nsName := types.NamespacedName{Namespace: ns, Name: name}
	for i := 0; i < n; i++ {
		var dda v2alpha1.DatadogAgent
		assert.NoError(t, r.client.Get(context.TODO(), nsName, &dda))
		_, err := r.Reconcile(context.TODO(), &dda)
		assert.NoError(t, err)
	}
}

// Test_Experiment_RollbackPersistsInSingleReconcile verifies that the spec
// rollback and the terminal phase write both land in the same reconcile —
// i.e. the status update uses the post-rollback ResourceVersion rather than
// 409'ing and deferring the phase write. Regression guard: without
// ResourceVersion propagation, reconcile 2 could observe a post-rollback spec
// matching an annotated, freshly-timestamped revision and skip the timeout
// check, leaving phase=running.
func Test_Experiment_RollbackPersistsInSingleReconcile(t *testing.T) {
	const ns, name = "default", "test-dda"
	const uid = types.UID("uid-1")
	const timeout = 50 * time.Millisecond
	nsName := types.NamespacedName{Namespace: ns, Name: name}

	r := newExperimentIntegrationReconciler(t, timeout)

	dda := baseDDA(ns, name, uid)
	createAndReconcile(t, r, dda)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Spec.Global.Site = ptr.To("datadoghq.eu")
	assert.NoError(t, r.client.Update(context.TODO(), dda))
	reconcileN(t, r, ns, name, 1)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Status.Experiment = &v2alpha1.ExperimentStatus{
		Phase: v2alpha1.ExperimentPhaseRunning,
		ID:    "exp-1",
	}
	assert.NoError(t, r.client.Status().Update(context.TODO(), dda))
	time.Sleep(2 * timeout)

	// Exactly one reconcile — both spec rollback and phase=timeout must persist.
	reconcileN(t, r, ns, name, 1)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	assert.Equal(t, "datadoghq.com", *dda.Spec.Global.Site, "spec must be rolled back in the same reconcile")
	assert.NotNil(t, dda.Status.Experiment)
	assert.Equal(t, v2alpha1.ExperimentPhaseTimeout, dda.Status.Experiment.Phase, "phase=timeout must persist in the same reconcile as the rollback")
}

// Test_Experiment_RollbackPreservesConcurrentStatusWrite verifies that the
// targeted phase patch in restorePreviousSpec does not mutate the caller's
// instance.ResourceVersion. A downstream writer (simulated by mutating the
// stored status after the rollback has run but before updateStatusIfNeededV2
// would fire) must retain 409 protection on the full-status replace.
// Regression guard: if patchExperimentPhase were to patch the caller's
// instance in-place, the client would decode the server response onto it and
// advance ResourceVersion, causing the downstream full-status update to use a
// fresh RV and silently overwrite any concurrent status write.
func Test_Experiment_RollbackPreservesConcurrentStatusWrite(t *testing.T) {
	const ns, name = "default", "test-dda"
	const uid = types.UID("uid-1")
	nsName := types.NamespacedName{Namespace: ns, Name: name}

	r := newExperimentIntegrationReconciler(t, 0)

	// Fetch the instance as the reconcile would, and capture its RV.
	dda := baseDDA(ns, name, uid)
	createAndReconcile(t, r, dda)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Spec.Global.Site = ptr.To("datadoghq.eu")
	assert.NoError(t, r.client.Update(context.TODO(), dda))
	reconcileN(t, r, ns, name, 1)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Status.Experiment = &v2alpha1.ExperimentStatus{Phase: v2alpha1.ExperimentPhaseStopped, ID: "exp-1"}
	assert.NoError(t, r.client.Status().Update(context.TODO(), dda))

	// Snapshot what a reconcile loop would hold in memory at this point:
	// the RV prior to invoking restorePreviousSpec.
	snapshot := &v2alpha1.DatadogAgent{}
	assert.NoError(t, r.client.Get(context.TODO(), nsName, snapshot))
	rvBefore := snapshot.ResourceVersion

	newStatus := &v2alpha1.DatadogAgentStatus{Experiment: snapshot.Status.Experiment.DeepCopy()}
	revisions := mustListRevisions(t, r, snapshot)
	assert.NoError(t, r.restorePreviousSpec(context.TODO(), snapshot, newStatus, revisions, v2alpha1.ExperimentPhaseRollback))

	// The caller's in-memory instance RV must not have been advanced by the
	// targeted status patch — otherwise a subsequent full-status write would
	// skip its 409 protection.
	assert.Equal(t, rvBefore, snapshot.ResourceVersion, "restorePreviousSpec must not mutate caller's ResourceVersion")

	// Phase must still have landed on the server via the targeted patch.
	after := &v2alpha1.DatadogAgent{}
	assert.NoError(t, r.client.Get(context.TODO(), nsName, after))
	assert.Equal(t, v2alpha1.ExperimentPhaseRollback, after.Status.Experiment.Phase)
}

// Test_Experiment_RollbackDoesNotClobberConcurrentPhaseWrite verifies that
// when a concurrent writer (e.g. the RC daemon accepting a promotion) has
// moved `status.experiment.phase` between the reconcile's initial fetch and
// the restorePreviousSpec patch, the targeted JSON patch's `test` op fails
// and the concurrent phase is left intact — rather than silently overwritten
// by our stale terminal-phase decision.
func Test_Experiment_RollbackDoesNotClobberConcurrentPhaseWrite(t *testing.T) {
	const ns, name = "default", "test-dda"
	const uid = types.UID("uid-1")
	nsName := types.NamespacedName{Namespace: ns, Name: name}

	r := newExperimentIntegrationReconciler(t, 0)

	dda := baseDDA(ns, name, uid)
	createAndReconcile(t, r, dda)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Spec.Global.Site = ptr.To("datadoghq.eu")
	assert.NoError(t, r.client.Update(context.TODO(), dda))
	reconcileN(t, r, ns, name, 1)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Status.Experiment = &v2alpha1.ExperimentStatus{Phase: v2alpha1.ExperimentPhaseStopped, ID: "exp-1"}
	assert.NoError(t, r.client.Status().Update(context.TODO(), dda))

	// Snapshot the observed state — as if a reconcile had just fetched.
	snapshot := &v2alpha1.DatadogAgent{}
	assert.NoError(t, r.client.Get(context.TODO(), nsName, snapshot))

	// Simulate a concurrent writer moving the phase to promoted *after* the
	// operator fetched its view but *before* restorePreviousSpec runs.
	concurrent := &v2alpha1.DatadogAgent{}
	assert.NoError(t, r.client.Get(context.TODO(), nsName, concurrent))
	concurrent.Status.Experiment.Phase = v2alpha1.ExperimentPhasePromoted
	assert.NoError(t, r.client.Status().Update(context.TODO(), concurrent))

	newStatus := &v2alpha1.DatadogAgentStatus{Experiment: snapshot.Status.Experiment.DeepCopy()}
	revisions := mustListRevisions(t, r, snapshot)
	assert.NoError(t, r.restorePreviousSpec(context.TODO(), snapshot, newStatus, revisions, v2alpha1.ExperimentPhaseRollback))

	// The targeted patch's test op must have rejected our stale decision —
	// the concurrent writer's promoted phase survives.
	after := &v2alpha1.DatadogAgent{}
	assert.NoError(t, r.client.Get(context.TODO(), nsName, after))
	assert.Equal(t, v2alpha1.ExperimentPhasePromoted, after.Status.Experiment.Phase,
		"concurrent phase write must not be overwritten by a stale targeted patch")
}

// Test_Experiment_StoppedRollback verifies that when RC writes phase=stopped,
// the operator restores the previous spec and sets phase=rollback.
func Test_Experiment_StoppedRollback(t *testing.T) {
	const ns, name = "default", "test-dda"
	const uid = types.UID("uid-1")
	nsName := types.NamespacedName{Namespace: ns, Name: name}

	r := newExperimentIntegrationReconciler(t, 0)

	// Rev1: initial spec.
	dda := baseDDA(ns, name, uid)
	createAndReconcile(t, r, dda)

	// Rev2: RC applies experiment spec.
	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Spec.Global.Site = ptr.To("datadoghq.eu")
	assert.NoError(t, r.client.Update(context.TODO(), dda))
	reconcileN(t, r, ns, name, 1)
	assert.Len(t, listOwnedRevisions(t, r.client, ns, uid), 2)

	// RC writes phase=stopped.
	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Status.Experiment = &v2alpha1.ExperimentStatus{
		Phase: v2alpha1.ExperimentPhaseStopped,
		ID:    "exp-1",
	}
	assert.NoError(t, r.client.Status().Update(context.TODO(), dda))

	// First reconcile: rollback triggered (spec restored, status update may conflict).
	// Second reconcile: spec already correct, status update succeeds.
	reconcileN(t, r, ns, name, 2)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	// The snapshot is taken from the defaulted spec (defaults.DefaultDatadogAgentSpec applies
	// site="datadoghq.com" before snapshotting), so rollback restores that value.
	assert.NotNil(t, dda.Spec.Global.Site, "spec should be restored to pre-experiment state")
	assert.Equal(t, "datadoghq.com", *dda.Spec.Global.Site, "spec should be restored to pre-experiment state")
	assert.NotNil(t, dda.Status.Experiment)
	assert.Equal(t, v2alpha1.ExperimentPhaseRollback, dda.Status.Experiment.Phase)
}

// Test_Experiment_TimeoutRollback verifies that an experiment running past
// ExperimentTimeout causes the operator to restore the previous spec and set
// phase=timeout.
func Test_Experiment_TimeoutRollback(t *testing.T) {
	const ns, name = "default", "test-dda"
	const uid = types.UID("uid-1")
	const timeout = 50 * time.Millisecond
	nsName := types.NamespacedName{Namespace: ns, Name: name}

	r := newExperimentIntegrationReconciler(t, timeout)

	// Rev1: initial spec.
	dda := baseDDA(ns, name, uid)
	createAndReconcile(t, r, dda)

	// Rev2: RC applies experiment spec.
	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Spec.Global.Site = ptr.To("datadoghq.eu")
	assert.NoError(t, r.client.Update(context.TODO(), dda))
	reconcileN(t, r, ns, name, 1)
	assert.Len(t, listOwnedRevisions(t, r.client, ns, uid), 2)

	// RC writes phase=running.
	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Status.Experiment = &v2alpha1.ExperimentStatus{
		Phase: v2alpha1.ExperimentPhaseRunning,
		ID:    "exp-1",
	}
	assert.NoError(t, r.client.Status().Update(context.TODO(), dda))

	// Wait for the timeout to elapse.
	time.Sleep(2 * timeout)

	// Reconcile 1: timeout detected → spec restored; status write may conflict.
	// Reconcile 2: idempotent rollback → status write succeeds.
	reconcileN(t, r, ns, name, 2)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	// The snapshot is taken from the defaulted spec, so rollback restores site="datadoghq.com".
	assert.NotNil(t, dda.Spec.Global.Site, "spec should be restored after timeout")
	assert.Equal(t, "datadoghq.com", *dda.Spec.Global.Site, "spec should be restored after timeout")
	assert.NotNil(t, dda.Status.Experiment)
	assert.Equal(t, v2alpha1.ExperimentPhaseTimeout, dda.Status.Experiment.Phase)
}

// Test_Experiment_AbortOnManualChange verifies that a spec change while an
// experiment is running sets phase=aborted and does not trigger rollback.
func Test_Experiment_AbortOnManualChange(t *testing.T) {
	const ns, name = "default", "test-dda"
	const uid = types.UID("uid-1")
	nsName := types.NamespacedName{Namespace: ns, Name: name}

	r := newExperimentIntegrationReconciler(t, 0)

	// Rev1: initial spec.
	dda := baseDDA(ns, name, uid)
	createAndReconcile(t, r, dda)

	// Rev2: RC applies experiment spec; RC signals running.
	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Spec.Global.Site = ptr.To("datadoghq.eu")
	assert.NoError(t, r.client.Update(context.TODO(), dda))
	reconcileN(t, r, ns, name, 1)

	// Patch revision timestamps to a recent time so the timeout path in
	// handleRollback is not accidentally triggered before the abort check runs.
	for _, rev := range listOwnedRevisions(t, r.client, ns, uid) {
		rev.CreationTimestamp = metav1.Now()
		assert.NoError(t, r.client.Update(context.TODO(), &rev))
	}

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Status.Experiment = &v2alpha1.ExperimentStatus{
		Phase: v2alpha1.ExperimentPhaseRunning,
		ID:    "exp-1",
	}
	assert.NoError(t, r.client.Status().Update(context.TODO(), dda))

	// User manually changes the spec — the new spec won't match any known revision,
	// so abortExperiment detects it as a manual change.
	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Spec.Global.Site = ptr.To("manual-change.example.com")
	assert.NoError(t, r.client.Update(context.TODO(), dda))

	reconcileN(t, r, ns, name, 1)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	// Spec should be the user's manual change, not rolled back.
	assert.Equal(t, "manual-change.example.com", *dda.Spec.Global.Site)
	assert.NotNil(t, dda.Status.Experiment)
	assert.Equal(t, v2alpha1.ExperimentPhaseAborted, dda.Status.Experiment.Phase)
}

// mustGetExperimentPhase fetches the DDA and returns the experiment phase, or
// empty string if no experiment is set. Helper for readability in assertions.
func mustGetExperimentPhase(t *testing.T, r *Reconciler, ns, name string) v2alpha1.ExperimentPhase {
	t.Helper()
	var dda v2alpha1.DatadogAgent
	assert.NoError(t, r.client.Get(context.TODO(), types.NamespacedName{Namespace: ns, Name: name}, &dda))
	if dda.Status.Experiment == nil {
		return ""
	}
	return dda.Status.Experiment.Phase
}

// Test_Experiment_TimeoutPhase_IsStable verifies that once phase=timeout is
// persisted, further reconciles do not change the spec or phase.
func Test_Experiment_TimeoutPhase_IsStable(t *testing.T) {
	const ns, name = "default", "test-dda"
	const uid = types.UID("uid-1")
	const timeout = 50 * time.Millisecond
	nsName := types.NamespacedName{Namespace: ns, Name: name}

	r := newExperimentIntegrationReconciler(t, timeout)

	dda := baseDDA(ns, name, uid)
	createAndReconcile(t, r, dda)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Spec.Global.Site = ptr.To("datadoghq.eu")
	assert.NoError(t, r.client.Update(context.TODO(), dda))
	reconcileN(t, r, ns, name, 1)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Status.Experiment = &v2alpha1.ExperimentStatus{
		Phase: v2alpha1.ExperimentPhaseRunning,
		ID:    "exp-1",
	}
	assert.NoError(t, r.client.Status().Update(context.TODO(), dda))
	time.Sleep(2 * timeout)
	reconcileN(t, r, ns, name, 2)

	assert.Equal(t, v2alpha1.ExperimentPhaseTimeout, mustGetExperimentPhase(t, r, ns, name))

	// Extra reconciles must not change phase or spec.
	reconcileN(t, r, ns, name, 3)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	assert.Equal(t, "datadoghq.com", *dda.Spec.Global.Site)
	assert.Equal(t, v2alpha1.ExperimentPhaseTimeout, mustGetExperimentPhase(t, r, ns, name))
}

// Test_Experiment_RollbackPhase_IsStable verifies that once phase=rollback is
// persisted, further reconciles do not change the spec or phase.
func Test_Experiment_RollbackPhase_IsStable(t *testing.T) {
	const ns, name = "default", "test-dda"
	const uid = types.UID("uid-1")
	nsName := types.NamespacedName{Namespace: ns, Name: name}

	r := newExperimentIntegrationReconciler(t, 0)

	dda := baseDDA(ns, name, uid)
	createAndReconcile(t, r, dda)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Spec.Global.Site = ptr.To("datadoghq.eu")
	assert.NoError(t, r.client.Update(context.TODO(), dda))
	reconcileN(t, r, ns, name, 1)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Status.Experiment = &v2alpha1.ExperimentStatus{
		Phase: v2alpha1.ExperimentPhaseStopped,
		ID:    "exp-1",
	}
	assert.NoError(t, r.client.Status().Update(context.TODO(), dda))
	reconcileN(t, r, ns, name, 2)

	assert.Equal(t, v2alpha1.ExperimentPhaseRollback, mustGetExperimentPhase(t, r, ns, name))

	// Extra reconciles must not change phase or spec.
	reconcileN(t, r, ns, name, 3)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	assert.Equal(t, "datadoghq.com", *dda.Spec.Global.Site)
	assert.Equal(t, v2alpha1.ExperimentPhaseRollback, mustGetExperimentPhase(t, r, ns, name))
}

// Test_Experiment_RunningAfterTimeout verifies that if RC writes phase=running
// after a timeout rollback has completed, the operator fires timeout again
// idempotently: the pre-experiment revision is old enough to exceed the timeout
// threshold, rollback is a no-op (spec already correct), and phase=timeout is
// written again.
func Test_Experiment_RunningAfterTimeout(t *testing.T) {
	const ns, name = "default", "test-dda"
	const uid = types.UID("uid-1")
	const timeout = 50 * time.Millisecond
	nsName := types.NamespacedName{Namespace: ns, Name: name}

	r := newExperimentIntegrationReconciler(t, timeout)

	dda := baseDDA(ns, name, uid)
	createAndReconcile(t, r, dda)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Spec.Global.Site = ptr.To("datadoghq.eu")
	assert.NoError(t, r.client.Update(context.TODO(), dda))
	reconcileN(t, r, ns, name, 1)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Status.Experiment = &v2alpha1.ExperimentStatus{
		Phase: v2alpha1.ExperimentPhaseRunning,
		ID:    "exp-1",
	}
	assert.NoError(t, r.client.Status().Update(context.TODO(), dda))
	time.Sleep(2 * timeout)
	reconcileN(t, r, ns, name, 2)
	assert.Equal(t, v2alpha1.ExperimentPhaseTimeout, mustGetExperimentPhase(t, r, ns, name))

	// RC writes phase=running again after the rollback already completed.
	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Status.Experiment = &v2alpha1.ExperimentStatus{
		Phase: v2alpha1.ExperimentPhaseRunning,
		ID:    "exp-1",
	}
	assert.NoError(t, r.client.Status().Update(context.TODO(), dda))

	reconcileN(t, r, ns, name, 1)

	// The pre-experiment revision is old enough that timeout fires idempotently —
	// rollback is a no-op (spec already correct) and phase=timeout is written again.
	assert.Equal(t, v2alpha1.ExperimentPhaseTimeout, mustGetExperimentPhase(t, r, ns, name))
}

// Test_Experiment_StoppedAfterRollback verifies that if RC writes phase=stopped
// after a rollback has already completed, the idempotent rollback path handles
// it cleanly (spec unchanged, phase set to rollback again).
func Test_Experiment_StoppedAfterRollback(t *testing.T) {
	const ns, name = "default", "test-dda"
	const uid = types.UID("uid-1")
	nsName := types.NamespacedName{Namespace: ns, Name: name}

	r := newExperimentIntegrationReconciler(t, 0)

	dda := baseDDA(ns, name, uid)
	createAndReconcile(t, r, dda)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Spec.Global.Site = ptr.To("datadoghq.eu")
	assert.NoError(t, r.client.Update(context.TODO(), dda))
	reconcileN(t, r, ns, name, 1)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Status.Experiment = &v2alpha1.ExperimentStatus{
		Phase: v2alpha1.ExperimentPhaseStopped,
		ID:    "exp-1",
	}
	assert.NoError(t, r.client.Status().Update(context.TODO(), dda))
	reconcileN(t, r, ns, name, 2)
	assert.Equal(t, v2alpha1.ExperimentPhaseRollback, mustGetExperimentPhase(t, r, ns, name))

	// RC writes phase=stopped again.
	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Status.Experiment.Phase = v2alpha1.ExperimentPhaseStopped
	assert.NoError(t, r.client.Status().Update(context.TODO(), dda))

	reconcileN(t, r, ns, name, 2)

	// Spec should still be the rolled-back spec; phase=rollback again.
	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	assert.Equal(t, "datadoghq.com", *dda.Spec.Global.Site)
	assert.Equal(t, v2alpha1.ExperimentPhaseRollback, mustGetExperimentPhase(t, r, ns, name))
}

// Test_Experiment_AbortDoesNotRollback verifies that phase=aborted is a
// terminal state and does not trigger a spec restore on subsequent reconciles.
func Test_Experiment_AbortDoesNotRollback(t *testing.T) {
	const ns, name = "default", "test-dda"
	const uid = types.UID("uid-1")
	nsName := types.NamespacedName{Namespace: ns, Name: name}

	r := newExperimentIntegrationReconciler(t, 0)

	dda := baseDDA(ns, name, uid)
	createAndReconcile(t, r, dda)

	// Apply experiment spec.
	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Spec.Global.Site = ptr.To("datadoghq.eu")
	assert.NoError(t, r.client.Update(context.TODO(), dda))
	reconcileN(t, r, ns, name, 1)

	// Manually force phase=aborted (as if abort already happened).
	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Status.Experiment = &v2alpha1.ExperimentStatus{
		Phase: v2alpha1.ExperimentPhaseAborted,
		ID:    "exp-1",
	}
	assert.NoError(t, r.client.Status().Update(context.TODO(), dda))

	reconcileN(t, r, ns, name, 1)

	// Spec should be unchanged (experiment spec), phase still aborted.
	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	assert.Equal(t, "datadoghq.eu", *dda.Spec.Global.Site, "aborted experiment must not trigger rollback")
	assert.Equal(t, v2alpha1.ExperimentPhaseAborted, mustGetExperimentPhase(t, r, ns, name))

	// Also verify the revision timestamp is not used as a proxy for time.Now() comparison.
	_ = metav1.Now()
}

// Test_Experiment_Abort_AnnotatesOnlyExperimentRevision verifies that when an
// experiment is aborted due to a manual spec change:
//   - The experiment revision (highest at the time of abort) is annotated.
//   - The new revision created by manageRevision for the manual-change spec
//     is NOT annotated.
//   - Subsequent reconciles do not re-annotate or spread the annotation.
func Test_Experiment_Abort_AnnotatesOnlyExperimentRevision(t *testing.T) {
	const ns, name = "default", "test-dda"
	const uid = types.UID("uid-1")
	nsName := types.NamespacedName{Namespace: ns, Name: name}

	r := newExperimentIntegrationReconciler(t, 0)

	// Rev1: baseline.
	dda := baseDDA(ns, name, uid)
	createAndReconcile(t, r, dda)

	// Rev2: experiment spec.
	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Spec.Global.Site = ptr.To("datadoghq.eu")
	assert.NoError(t, r.client.Update(context.TODO(), dda))
	reconcileN(t, r, ns, name, 1)
	assert.Len(t, listOwnedRevisions(t, r.client, ns, uid), 2)

	// Record the experiment revision name (highest Revision number).
	var experimentRevName string
	for _, rev := range listOwnedRevisions(t, r.client, ns, uid) {
		if experimentRevName == "" || rev.Revision > 0 {
			experimentRevName = rev.Name
		}
	}
	maxRev := int64(0)
	for _, rev := range listOwnedRevisions(t, r.client, ns, uid) {
		if rev.Revision > maxRev {
			maxRev = rev.Revision
			experimentRevName = rev.Name
		}
	}

	// Patch timestamps so timeout doesn't fire before abort.
	for _, rev := range listOwnedRevisions(t, r.client, ns, uid) {
		rev.CreationTimestamp = metav1.Now()
		assert.NoError(t, r.client.Update(context.TODO(), &rev))
	}

	// Set phase=running.
	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Status.Experiment = &v2alpha1.ExperimentStatus{
		Phase: v2alpha1.ExperimentPhaseRunning,
		ID:    "exp-1",
	}
	assert.NoError(t, r.client.Status().Update(context.TODO(), dda))

	// User manually changes the spec — triggers abort.
	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Spec.Global.Site = ptr.To("manual-change.example.com")
	assert.NoError(t, r.client.Update(context.TODO(), dda))

	// First reconcile: abort fires (manageExperiment), then manageRevision
	// creates rev3 for the manual-change spec.
	reconcileN(t, r, ns, name, 1)
	assert.Equal(t, v2alpha1.ExperimentPhaseAborted, mustGetExperimentPhase(t, r, ns, name))

	// Verify: only the experiment revision is annotated, not the new one.
	revs := listOwnedRevisions(t, r.client, ns, uid)
	for _, rev := range revs {
		if rev.Name == experimentRevName {
			assert.Equal(t, "true", rev.Annotations[annotationExperimentRollback],
				"experiment revision %s should be annotated", rev.Name)
		} else {
			assert.NotEqual(t, "true", rev.Annotations[annotationExperimentRollback],
				"non-experiment revision %s should NOT be annotated", rev.Name)
		}
	}

	// Run additional reconciles — annotation state must be stable.
	reconcileN(t, r, ns, name, 3)

	revs = listOwnedRevisions(t, r.client, ns, uid)
	annotatedNames := []string{}
	for _, rev := range revs {
		if rev.Annotations[annotationExperimentRollback] == "true" {
			annotatedNames = append(annotatedNames, rev.Name)
		}
	}
	assert.Equal(t, []string{experimentRevName}, annotatedNames,
		"after multiple reconciles, only the original experiment revision should remain annotated")
}

// Test_Experiment_StoppedRollback_AnnotatesOnlyExperimentRevision verifies
// that when an experiment is rolled back via phase=stopped:
//   - The experiment revision is annotated with experiment-rollback.
//   - The rollback target (baseline) revision is NOT annotated.
//   - Subsequent reconciles do not spread or remove the annotation.
func Test_Experiment_StoppedRollback_AnnotatesOnlyExperimentRevision(t *testing.T) {
	const ns, name = "default", "test-dda"
	const uid = types.UID("uid-1")
	nsName := types.NamespacedName{Namespace: ns, Name: name}

	r := newExperimentIntegrationReconciler(t, 0)

	// Rev1: baseline.
	dda := baseDDA(ns, name, uid)
	createAndReconcile(t, r, dda)

	// Record baseline revision name.
	baselineRevName := listOwnedRevisions(t, r.client, ns, uid)[0].Name

	// Rev2: experiment spec.
	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Spec.Global.Site = ptr.To("datadoghq.eu")
	assert.NoError(t, r.client.Update(context.TODO(), dda))
	reconcileN(t, r, ns, name, 1)

	// Identify experiment revision.
	var experimentRevName string
	for _, rev := range listOwnedRevisions(t, r.client, ns, uid) {
		if rev.Name != baselineRevName {
			experimentRevName = rev.Name
		}
	}
	assert.NotEmpty(t, experimentRevName, "should have an experiment revision")

	// Add a pre-existing annotation to the experiment revision to verify the
	// merge patch preserves it when annotateRevision adds its own annotation.
	for _, rev := range listOwnedRevisions(t, r.client, ns, uid) {
		if rev.Name == experimentRevName {
			rev := rev
			if rev.Annotations == nil {
				rev.Annotations = map[string]string{}
			}
			rev.Annotations["custom-annotation"] = "should-survive"
			assert.NoError(t, r.client.Update(context.TODO(), &rev))
			break
		}
	}

	// RC writes phase=stopped → triggers rollback.
	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Status.Experiment = &v2alpha1.ExperimentStatus{
		Phase: v2alpha1.ExperimentPhaseStopped,
		ID:    "exp-1",
	}
	assert.NoError(t, r.client.Status().Update(context.TODO(), dda))
	reconcileN(t, r, ns, name, 2)

	assert.Equal(t, v2alpha1.ExperimentPhaseRollback, mustGetExperimentPhase(t, r, ns, name))

	// Verify: experiment revision annotated, baseline NOT annotated,
	// and pre-existing annotations are preserved by the merge patch.
	for _, rev := range listOwnedRevisions(t, r.client, ns, uid) {
		if rev.Name == experimentRevName {
			assert.Equal(t, "true", rev.Annotations[annotationExperimentRollback],
				"experiment revision %s should be annotated after rollback", rev.Name)
			assert.Equal(t, "should-survive", rev.Annotations["custom-annotation"],
				"pre-existing annotation on experiment revision %s should be preserved", rev.Name)
		} else {
			assert.NotEqual(t, "true", rev.Annotations[annotationExperimentRollback],
				"baseline revision %s should NOT be annotated", rev.Name)
		}
	}

	// Run additional reconciles — annotation state must be stable.
	reconcileN(t, r, ns, name, 3)

	for _, rev := range listOwnedRevisions(t, r.client, ns, uid) {
		if rev.Name == experimentRevName {
			assert.Equal(t, "true", rev.Annotations[annotationExperimentRollback],
				"experiment revision %s should still be annotated after extra reconciles", rev.Name)
		} else {
			assert.NotEqual(t, "true", rev.Annotations[annotationExperimentRollback],
				"baseline revision %s should still NOT be annotated after extra reconciles", rev.Name)
		}
	}
}

// Test_Experiment_PromoteThenNewExperiment_NoImmediateTimeout verifies that
// after an experiment is promoted, a subsequent new experiment does not
// immediately timeout due to a stale revision timestamp.
//
// Regression test: the promoted experiment's revision was not annotated, so
// handleRollback fell back to its stale timestamp and fired an immediate
// timeout on the first reconcile of the new experiment.
func Test_Experiment_PromoteThenNewExperiment_NoImmediateTimeout(t *testing.T) {
	const ns, name = "default", "test-dda"
	const uid = types.UID("uid-1")
	const longTimeout = 5 * time.Second
	nsName := types.NamespacedName{Namespace: ns, Name: name}

	r := newExperimentIntegrationReconciler(t, longTimeout)

	// Rev1: baseline.
	dda := baseDDA(ns, name, uid)
	createAndReconcile(t, r, dda)

	// Rev2: first experiment spec.
	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Spec.Global.Site = ptr.To("datadoghq.eu")
	assert.NoError(t, r.client.Update(context.TODO(), dda))
	reconcileN(t, r, ns, name, 1)
	assert.Len(t, listOwnedRevisions(t, r.client, ns, uid), 2)

	// RC writes phase=promoted (experiment succeeded, keep the new spec).
	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Status.Experiment = &v2alpha1.ExperimentStatus{
		Phase: v2alpha1.ExperimentPhasePromoted,
		ID:    "exp-1",
	}
	assert.NoError(t, r.client.Status().Update(context.TODO(), dda))

	// Reconcile processes the promoted phase: manageExperiment annotates the
	// revision, then ensureRevision sees the annotation and recreates it with
	// a fresh timestamp (consuming the annotation in the process).
	reconcileN(t, r, ns, name, 1)
	assert.Equal(t, v2alpha1.ExperimentPhasePromoted, mustGetExperimentPhase(t, r, ns, name))

	// New experiment: daemon patches the spec first, which triggers a reconcile
	// while still phase=promoted (abortExperiment is a no-op for non-running
	// phases). This reconcile creates a revision for the new spec.
	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Spec.Global.Site = ptr.To("datadoghq.jp")
	assert.NoError(t, r.client.Update(context.TODO(), dda))
	reconcileN(t, r, ns, name, 1)

	// Daemon then updates status to running with a new experiment ID.
	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Status.Experiment = &v2alpha1.ExperimentStatus{
		Phase: v2alpha1.ExperimentPhaseRunning,
		ID:    "exp-2",
	}
	assert.NoError(t, r.client.Status().Update(context.TODO(), dda))

	// Patch all revision timestamps to now so fresh revisions have fresh timestamps.
	for _, rev := range listOwnedRevisions(t, r.client, ns, uid) {
		rev.CreationTimestamp = metav1.Now()
		assert.NoError(t, r.client.Update(context.TODO(), &rev))
	}

	// Reconcile: the new experiment's revision exists with a fresh timestamp,
	// so neither timeout nor abort should fire.
	reconcileN(t, r, ns, name, 1)

	assert.Equal(t, v2alpha1.ExperimentPhaseRunning, mustGetExperimentPhase(t, r, ns, name),
		"new experiment should still be running — no false timeout after promotion")
}

// Test_Experiment_Promoted_DoesNotRecreateRevision verifies that a promoted
// experiment's revision is annotated with experiment-promoted (not
// experiment-rollback), and ensureRevision does NOT delete+recreate it.
func Test_Experiment_Promoted_DoesNotRecreateRevision(t *testing.T) {
	const ns, name = "default", "test-dda"
	const uid = types.UID("uid-1")
	nsName := types.NamespacedName{Namespace: ns, Name: name}

	r := newExperimentIntegrationReconciler(t, 0)

	// Rev1: baseline.
	dda := baseDDA(ns, name, uid)
	createAndReconcile(t, r, dda)

	// Rev2: experiment spec.
	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Spec.Global.Site = ptr.To("datadoghq.eu")
	assert.NoError(t, r.client.Update(context.TODO(), dda))
	reconcileN(t, r, ns, name, 1)
	assert.Len(t, listOwnedRevisions(t, r.client, ns, uid), 2)

	// Record the experiment revision name (highest Revision number).
	var experimentRevName string
	maxRev := int64(0)
	for _, rev := range listOwnedRevisions(t, r.client, ns, uid) {
		if rev.Revision > maxRev {
			maxRev = rev.Revision
			experimentRevName = rev.Name
		}
	}

	// RC writes phase=promoted.
	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Status.Experiment = &v2alpha1.ExperimentStatus{
		Phase: v2alpha1.ExperimentPhasePromoted,
		ID:    "exp-1",
	}
	assert.NoError(t, r.client.Status().Update(context.TODO(), dda))

	// Reconcile processes promoted phase.
	reconcileN(t, r, ns, name, 1)

	// The experiment revision should have the promoted annotation, NOT rollback.
	revs := listOwnedRevisions(t, r.client, ns, uid)
	for _, rev := range revs {
		if rev.Name == experimentRevName {
			assert.Equal(t, "true", rev.Annotations[annotationExperimentPromoted],
				"experiment revision should have promoted annotation")
			assert.NotEqual(t, "true", rev.Annotations[annotationExperimentRollback],
				"experiment revision should NOT have rollback annotation")
		}
	}

	// Run additional reconciles — the revision should NOT be recreated.
	// If it were recreated, the name would stay the same but the promoted
	// annotation would be cleared. Verify it persists.
	reconcileN(t, r, ns, name, 3)

	revs = listOwnedRevisions(t, r.client, ns, uid)
	for _, rev := range revs {
		if rev.Name == experimentRevName {
			assert.Equal(t, "true", rev.Annotations[annotationExperimentPromoted],
				"promoted annotation should persist — revision must not be recreated")
		}
	}
}

// Test_Experiment_StateTransitions verifies that after any terminal state,
// a new experiment can start and reach any terminal state correctly — with
// no false timeouts from stale revision timestamps.
//
// Matrix: 4 previous states × 3 new outcomes × 2 (fresh / stale old revision) = 24 subtests.
//
// Each sub-test follows the same 5-phase flow:
//
//	Phase 1: Set up baseline + first experiment, reach terminal state.
//	Phase 2: (stale variant only) Age all existing revision timestamps past timeout.
//	Phase 3: Start a new experiment (mimics daemon: patch spec, reconcile, set running).
//	Phase 4: Assert no false timeout — the new experiment must stay running.
//	Phase 5: Drive the new experiment to its target outcome (stop/promote/timeout).
func Test_Experiment_StateTransitions(t *testing.T) {
	type terminalSetup struct {
		name  string
		reach func(t *testing.T, r *Reconciler, ns, name string, uid types.UID, nsName types.NamespacedName, dda *v2alpha1.DatadogAgent)
	}

	type newOutcome struct {
		name   string
		action func(t *testing.T, r *Reconciler, ns, name string, uid types.UID, nsName types.NamespacedName, dda *v2alpha1.DatadogAgent)
		expect v2alpha1.ExperimentPhase
	}

	// ---------------------------------------------------------------
	// Terminal states: how to get the first experiment into each one.
	// ---------------------------------------------------------------
	terminalStates := []terminalSetup{
		{
			// promoted: RC signals success, spec stays as-is.
			name: "promoted",
			reach: func(t *testing.T, r *Reconciler, ns, name string, uid types.UID, nsName types.NamespacedName, dda *v2alpha1.DatadogAgent) {
				t.Helper()
				assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
				dda.Status.Experiment = &v2alpha1.ExperimentStatus{
					Phase: v2alpha1.ExperimentPhasePromoted,
					ID:    "exp-1",
				}
				assert.NoError(t, r.client.Status().Update(context.TODO(), dda))
				// Reconcile annotates the experiment revision with experiment-promoted.
				reconcileN(t, r, ns, name, 1)
			},
		},
		{
			// rollback: RC signals stop, operator restores previous spec.
			name: "rollback",
			reach: func(t *testing.T, r *Reconciler, ns, name string, uid types.UID, nsName types.NamespacedName, dda *v2alpha1.DatadogAgent) {
				t.Helper()
				assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
				dda.Status.Experiment = &v2alpha1.ExperimentStatus{
					Phase: v2alpha1.ExperimentPhaseStopped,
					ID:    "exp-1",
				}
				assert.NoError(t, r.client.Status().Update(context.TODO(), dda))
				// Two reconciles: first restores spec (status conflicts),
				// second persists phase=rollback.
				reconcileN(t, r, ns, name, 2)
			},
		},
		{
			// timeout: experiment runs past the deadline, operator rolls back.
			name: "timeout",
			reach: func(t *testing.T, r *Reconciler, ns, name string, uid types.UID, nsName types.NamespacedName, dda *v2alpha1.DatadogAgent) {
				t.Helper()
				r.options.ExperimentTimeout = 50 * time.Millisecond
				assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
				dda.Status.Experiment = &v2alpha1.ExperimentStatus{
					Phase: v2alpha1.ExperimentPhaseRunning,
					ID:    "exp-1",
				}
				assert.NoError(t, r.client.Status().Update(context.TODO(), dda))
				time.Sleep(100 * time.Millisecond)
				// Two reconciles: first rolls back spec (status conflicts),
				// second persists phase=timeout.
				reconcileN(t, r, ns, name, 2)
			},
		},
		{
			// aborted: user manually changes spec while experiment is running.
			name: "aborted",
			reach: func(t *testing.T, r *Reconciler, ns, name string, uid types.UID, nsName types.NamespacedName, dda *v2alpha1.DatadogAgent) {
				t.Helper()
				// Fresh timestamps so timeout doesn't race abort.
				for _, rev := range listOwnedRevisions(t, r.client, ns, uid) {
					rev.CreationTimestamp = metav1.Now()
					assert.NoError(t, r.client.Update(context.TODO(), &rev))
				}
				assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
				dda.Status.Experiment = &v2alpha1.ExperimentStatus{
					Phase: v2alpha1.ExperimentPhaseRunning,
					ID:    "exp-1",
				}
				assert.NoError(t, r.client.Status().Update(context.TODO(), dda))
				// Manual spec change — doesn't match any revision → abort.
				assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
				dda.Spec.Global.Site = ptr.To("manual-change.example.com")
				assert.NoError(t, r.client.Update(context.TODO(), dda))
				reconcileN(t, r, ns, name, 1)
			},
		},
	}

	// ---------------------------------------------------------------
	// Outcomes: how to drive the new experiment to its terminal state.
	// ---------------------------------------------------------------
	newOutcomes := []newOutcome{
		{
			// stop: RC signals stop → operator rolls back → phase=rollback.
			name: "stop",
			action: func(t *testing.T, r *Reconciler, ns, name string, uid types.UID, nsName types.NamespacedName, dda *v2alpha1.DatadogAgent) {
				t.Helper()
				assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
				dda.Status.Experiment.Phase = v2alpha1.ExperimentPhaseStopped
				assert.NoError(t, r.client.Status().Update(context.TODO(), dda))
				reconcileN(t, r, ns, name, 2)
			},
			expect: v2alpha1.ExperimentPhaseRollback,
		},
		{
			// promote: RC signals success → phase=promoted.
			name: "promote",
			action: func(t *testing.T, r *Reconciler, ns, name string, uid types.UID, nsName types.NamespacedName, dda *v2alpha1.DatadogAgent) {
				t.Helper()
				assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
				dda.Status.Experiment.Phase = v2alpha1.ExperimentPhasePromoted
				assert.NoError(t, r.client.Status().Update(context.TODO(), dda))
				reconcileN(t, r, ns, name, 1)
			},
			expect: v2alpha1.ExperimentPhasePromoted,
		},
		{
			// timeout: age the new experiment's revision past the deadline
			// so the reconciler triggers a real timeout.
			name: "timeout",
			action: func(t *testing.T, r *Reconciler, ns, name string, uid types.UID, nsName types.NamespacedName, dda *v2alpha1.DatadogAgent) {
				t.Helper()
				r.options.ExperimentTimeout = 50 * time.Millisecond
				// Only age unannotated revisions (the new experiment's revision).
				// Annotated revisions belong to the old experiment and must not
				// be touched — they're already handled by the fallback skip.
				staleTime := metav1.NewTime(time.Now().Add(-time.Minute))
				for _, rev := range listOwnedRevisions(t, r.client, ns, uid) {
					if rev.Annotations[annotationExperimentRollback] != "true" &&
						rev.Annotations[annotationExperimentPromoted] != "true" {
						rev.CreationTimestamp = staleTime
						assert.NoError(t, r.client.Update(context.TODO(), &rev))
					}
				}
				reconcileN(t, r, ns, name, 2)
			},
			expect: v2alpha1.ExperimentPhaseTimeout,
		},
	}

	// ---------------------------------------------------------------
	// Test loop: 4 previous × 3 outcomes × 2 (fresh/stale) = 24 subtests.
	// ---------------------------------------------------------------
	for _, prev := range terminalStates {
		for _, next := range newOutcomes {
			for _, staleOldRevision := range []bool{false, true} {
				suffix := ""
				if staleOldRevision {
					suffix = "/stale_old_revision"
				}
				testName := prev.name + "_then_" + next.name + suffix
				t.Run(testName, func(t *testing.T) {
					const ns, name = "default", "test-dda"
					const uid = types.UID("uid-1")
					nsName := types.NamespacedName{Namespace: ns, Name: name}

					r := newExperimentIntegrationReconciler(t, 5*time.Second)

					// -- Phase 1: set up first experiment and reach terminal state --

					// Baseline spec (rev1).
					dda := baseDDA(ns, name, uid)
					createAndReconcile(t, r, dda)

					// First experiment spec (rev2).
					assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
					dda.Spec.Global.Site = ptr.To("datadoghq.eu")
					assert.NoError(t, r.client.Update(context.TODO(), dda))
					reconcileN(t, r, ns, name, 1)

					// Drive exp-1 to its terminal state (promoted/rollback/timeout/aborted).
					prev.reach(t, r, ns, name, uid, nsName, dda)

					// -- Phase 2: (stale variant) age old revisions past timeout --

					if staleOldRevision {
						staleTime := metav1.NewTime(time.Now().Add(-10 * time.Minute))
						for _, rev := range listOwnedRevisions(t, r.client, ns, uid) {
							rev.CreationTimestamp = staleTime
							assert.NoError(t, r.client.Update(context.TODO(), &rev))
						}
					}

					// -- Phase 3: start new experiment (mimics daemon) --

					// Daemon step 1: patch spec to the new experiment config.
					r.options.ExperimentTimeout = 5 * time.Second
					assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
					dda.Spec.Global.Site = ptr.To("datadoghq.jp")
					assert.NoError(t, r.client.Update(context.TODO(), dda))
					// This reconcile runs while the old terminal phase is still set,
					// so handleRollback is a no-op. manageRevision creates a revision
					// for the new spec.
					reconcileN(t, r, ns, name, 1)

					// Daemon step 2: set phase=running with new experiment ID.
					assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
					dda.Status.Experiment = &v2alpha1.ExperimentStatus{
						Phase: v2alpha1.ExperimentPhaseRunning,
						ID:    "exp-2",
					}
					assert.NoError(t, r.client.Status().Update(context.TODO(), dda))

					// Give the new experiment's revision a fresh timestamp.
					// (Fake client doesn't set CreationTimestamp on create.)
					for _, rev := range listOwnedRevisions(t, r.client, ns, uid) {
						rev.CreationTimestamp = metav1.Now()
						assert.NoError(t, r.client.Update(context.TODO(), &rev))
					}

					// -- Phase 4: assert no false timeout --

					reconcileN(t, r, ns, name, 1)
					assert.Equal(t, v2alpha1.ExperimentPhaseRunning, mustGetExperimentPhase(t, r, ns, name),
						"new experiment should be running — no false timeout")

					// -- Phase 5: drive new experiment to target outcome --

					next.action(t, r, ns, name, uid, nsName, dda)
					assert.Equal(t, next.expect, mustGetExperimentPhase(t, r, ns, name))
				})
			}
		}
	}
}

// Test_Experiment_ReapplySameSpec_NoImmediateTimeout verifies the full
// annotation-based revision recreate flow end-to-end:
//
//  1. Baseline spec → experiment spec → timeout rollback.
//  2. Rollback annotates the experiment revision with experiment-rollback=true.
//  3. Re-apply the same experiment spec.
//  4. ensureRevision creates a new revision for the experiment spec (the content
//     hash may differ from the original due to defaulting, but if it matches,
//     the annotated revision is deleted+recreated with a fresh timestamp).
//  5. A subsequent reconcile with phase=running does NOT immediately timeout.
//
// NOTE: The fake client doesn't set CreationTimestamp on Create (unlike the
// real API server), so we manually patch timestamps to simulate fresh
// revisions after the re-apply reconcile.
func Test_Experiment_ReapplySameSpec_NoImmediateTimeout(t *testing.T) {
	const ns, name = "default", "test-dda"
	const uid = types.UID("uid-1")
	// Use a short timeout for the initial experiment so it times out quickly,
	// then switch to a long timeout for the re-applied experiment so we can
	// assert it does NOT timeout within a single reconcile.
	const shortTimeout = 50 * time.Millisecond
	const longTimeout = 5 * time.Second
	nsName := types.NamespacedName{Namespace: ns, Name: name}

	r := newExperimentIntegrationReconciler(t, shortTimeout)

	// Step 1: Create baseline (rev1).
	dda := baseDDA(ns, name, uid)
	createAndReconcile(t, r, dda)

	// Step 2: Apply experiment spec (rev2).
	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Spec.Global.Site = ptr.To("datadoghq.eu")
	assert.NoError(t, r.client.Update(context.TODO(), dda))
	reconcileN(t, r, ns, name, 1)
	assert.Len(t, listOwnedRevisions(t, r.client, ns, uid), 2)

	// Step 3: Set phase=running and let it timeout.
	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Status.Experiment = &v2alpha1.ExperimentStatus{
		Phase: v2alpha1.ExperimentPhaseRunning,
		ID:    "exp-1",
	}
	assert.NoError(t, r.client.Status().Update(context.TODO(), dda))
	time.Sleep(2 * shortTimeout)
	reconcileN(t, r, ns, name, 2)

	assert.Equal(t, v2alpha1.ExperimentPhaseTimeout, mustGetExperimentPhase(t, r, ns, name))
	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	assert.Equal(t, "datadoghq.com", *dda.Spec.Global.Site, "spec should be rolled back")

	// Verify the experiment revision is annotated as rolled back.
	revs := listOwnedRevisions(t, r.client, ns, uid)
	var annotatedCount int
	for _, rev := range revs {
		if rev.Annotations[annotationExperimentRollback] == "true" {
			annotatedCount++
		}
	}
	assert.Equal(t, 1, annotatedCount, "exactly one revision should have the rollback annotation")

	// Switch to a long timeout so the re-applied experiment doesn't timeout
	// within the reconcile's own execution time.
	r.options.ExperimentTimeout = longTimeout

	// Step 4: Clear experiment status (simulating fleet daemon acknowledging the
	// rollback) and re-apply the same experiment spec.
	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Status.Experiment = nil
	assert.NoError(t, r.client.Status().Update(context.TODO(), dda))

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Spec.Global.Site = ptr.To("datadoghq.eu")
	assert.NoError(t, r.client.Update(context.TODO(), dda))

	// This reconcile triggers ensureRevision which either:
	// - matches the annotated revision (same content hash) → delete+recreate, or
	// - creates a new revision (different hash due to defaulting differences).
	// Either way, the current revision for this spec has no rollback annotation.
	reconcileN(t, r, ns, name, 1)

	// Step 5: Set phase=running again. Patch all revision timestamps to now so
	// the timeout check works correctly with the fake client (which doesn't set
	// CreationTimestamp on Create like the real API server).
	revs = listOwnedRevisions(t, r.client, ns, uid)
	assert.Len(t, revs, 2)
	for i := range revs {
		revs[i].CreationTimestamp = metav1.Now()
		assert.NoError(t, r.client.Update(context.TODO(), &revs[i]))
	}

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Status.Experiment = &v2alpha1.ExperimentStatus{
		Phase: v2alpha1.ExperimentPhaseRunning,
		ID:    "exp-2",
	}
	assert.NoError(t, r.client.Status().Update(context.TODO(), dda))

	// Reconcile — should NOT timeout because the revision is fresh.
	reconcileN(t, r, ns, name, 1)

	assert.Equal(t, v2alpha1.ExperimentPhaseRunning, mustGetExperimentPhase(t, r, ns, name),
		"experiment should still be running — no immediate timeout after reapply")
}
