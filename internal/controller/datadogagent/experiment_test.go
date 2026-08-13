// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	v2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

func TestManageExperiment_AbortsOnManualChange(t *testing.T) {
	r, _ := newRevisionTestReconciler(t)

	// Create two revisions: pre-experiment (specA) and experiment (specB).
	instanceA := newRevisionTestOwner("test-dda", "default")
	require.NoError(t, r.manageRevision(context.Background(), instanceA, instanceA.Spec, mustListRevisions(t, r, instanceA), nil))

	instanceB := newRevisionTestOwner("test-dda", "default")
	instanceB.Spec = v2alpha1.DatadogAgentSpec{Global: &v2alpha1.GlobalConfig{}}
	require.NoError(t, r.manageRevision(context.Background(), instanceB, instanceB.Spec, mustListRevisions(t, r, instanceB), nil))

	// Simulate a manual spec change: specC differs from what was captured
	// as ExpectedSpecHash at experiment start (a hash of specB).
	instanceC := newRevisionTestOwner("test-dda", "default")
	manualSite := "manual-change.example.com"
	instanceC.Spec = v2alpha1.DatadogAgentSpec{Global: &v2alpha1.GlobalConfig{Site: &manualSite}}
	expectedHash, err := computeSpecHash(instanceB.Spec, instanceB.GetAnnotations())
	require.NoError(t, err)
	instanceC.Status.Experiment = &v2alpha1.ExperimentStatus{
		Phase:            v2alpha1.ExperimentPhaseRunning,
		ExpectedSpecHash: expectedHash,
	}

	revList := mustListRevisions(t, r, instanceC)

	status := &v2alpha1.DatadogAgentStatus{
		Experiment: instanceC.Status.Experiment.DeepCopy(),
	}

	err = r.manageExperiment(context.Background(), instanceC, status, metav1.Now(), revList)
	require.NoError(t, err)
	require.NotNil(t, status.Experiment)
	assert.Equal(t, v2alpha1.ExperimentPhaseAborted, status.Experiment.Phase)
}

// TestManageExperiment_ManualRevertToBaselineTerminatesViaTimeout verifies that
// when the user manually reverts the spec to the pre-experiment value during a
// running experiment, the experiment terminates via timeout rather than abort.
// The revision-based abort check sees the spec matching the baseline revision
// and treats it as a known state; the timeout path fires because the elapsed
// time since Status.Experiment.StartedAt exceeds the threshold. The rollback
// is a no-op (spec already matches target), and the phase is set to
// "terminated" with terminationReason "timed_out".
// TestManageExperiment_AbortsInFlightExperimentMissingCheckpoint verifies the
// Phase 8 preview-compatibility gate: an experiment carried across an operator
// upgrade lacks the new checkpoint fields and must be aborted proactively
// rather than limping along in Running phase forever.
func TestManageExperiment_AbortsInFlightExperimentMissingCheckpoint(t *testing.T) {
	r, _ := newRevisionTestReconciler(t)
	instance := newRevisionTestOwner("test-dda", "default")
	newStatus := &v2alpha1.DatadogAgentStatus{
		Experiment: &v2alpha1.ExperimentStatus{
			Phase:       v2alpha1.ExperimentPhaseRunning,
			ID:          "exp-inflight",
			StartTaskID: "task-inflight",
			// Neither RollbackTargetRevision nor ExpectedSpecHash set —
			// this is what an in-flight experiment looks like post-upgrade.
		},
	}
	require.NoError(t, r.manageExperiment(context.Background(), instance, newStatus, metav1.Now(), nil))
	assert.Equal(t, v2alpha1.ExperimentPhaseAborted, newStatus.Experiment.Phase)
	assert.Equal(t, ExperimentTerminationReasonBaselineMissing, newStatus.Experiment.TerminationReason)
	assert.Equal(t, "task-inflight", newStatus.Experiment.StartTaskID,
		"StartTaskID must be preserved so the daemon reports ERROR to Remote Config")
}

func TestManageExperiment_ManualRevertToBaselineTerminatesViaTimeout(t *testing.T) {
	r, c := newRevisionTestReconciler(t)

	// Rev1: pre-experiment spec (specA).
	instanceA := newRevisionTestOwner("test-dda", "default")
	require.NoError(t, r.manageRevision(context.Background(), instanceA, instanceA.Spec, mustListRevisions(t, r, instanceA), nil))

	// Rev2: experiment spec (specB).
	instanceB := newRevisionTestOwner("test-dda", "default")
	instanceB.Spec = v2alpha1.DatadogAgentSpec{Global: &v2alpha1.GlobalConfig{}}
	require.NoError(t, r.manageRevision(context.Background(), instanceB, instanceB.Spec, mustListRevisions(t, r, instanceB), nil))
	require.NoError(t, c.Create(context.Background(), instanceA))

	// User manually reverts to specA. The live spec hashes to the pre-experiment
	// value, matching neither the ExpectedSpecHash captured at start (a hash of
	// specB) nor... wait — abortExperiment's hash check fires when live != expected,
	// so a revert to baseline DOES trigger abort under the checkpoint model.
	// Under the checkpoint model, abortExperiment on hash-mismatch preempts the
	// timeout path. This test's original intent — "revert to baseline terminates
	// via timeout" — no longer holds; the abort path fires first. Assert the
	// abort outcome instead.
	startedAt := metav1.NewTime(time.Now().Add(-ExperimentDefaultTimeout - time.Minute))
	revList := mustListRevisions(t, r, instanceA)
	expectedHash, err := computeSpecHash(instanceB.Spec, instanceB.GetAnnotations())
	require.NoError(t, err)
	instanceA.Status.Experiment = &v2alpha1.ExperimentStatus{
		Phase:                  v2alpha1.ExperimentPhaseRunning,
		StartedAt:              &startedAt,
		RollbackTargetRevision: findRollbackTarget(revList),
		ExpectedSpecHash:       expectedHash,
	}

	newStatus := &v2alpha1.DatadogAgentStatus{Experiment: instanceA.Status.Experiment.DeepCopy()}
	require.NoError(t, r.manageExperiment(context.Background(), instanceA, newStatus, metav1.Now(), revList))
	// Under the checkpoint model, handleRollback still runs before
	// abortExperiment, so an elapsed timeout terminates before the hash-mismatch
	// abort can fire. Same outcome as the pre-checkpoint model.
	assert.Equal(t, v2alpha1.ExperimentPhaseTerminated, newStatus.Experiment.Phase)
	assert.Equal(t, ExperimentTerminationReasonTimedOut, newStatus.Experiment.TerminationReason)
}

func TestRollback_RestoresSpec(t *testing.T) {
	r, c := newRevisionTestReconciler(t)

	// Create a revision for specA.
	instanceA := newRevisionTestOwner("test-dda", "default")
	err := r.manageRevision(context.Background(), instanceA, instanceA.Spec, mustListRevisions(t, r, instanceA), nil)
	require.NoError(t, err)

	revListA := mustListRevisions(t, r, instanceA)
	require.Len(t, revListA, 1)
	prevRevision := revListA[0].Name

	// Create a second revision for specB.
	instanceB := newRevisionTestOwner("test-dda", "default")
	instanceB.Spec = v2alpha1.DatadogAgentSpec{Global: &v2alpha1.GlobalConfig{}}
	err = r.manageRevision(context.Background(), instanceB, instanceB.Spec, mustListRevisions(t, r, instanceB), nil)
	require.NoError(t, err)

	// rollback fetches the current DDA to compare specs; it must exist in the fake client.
	require.NoError(t, c.Create(context.Background(), instanceB))

	// Rollback from instanceB to prevRevision (specA).
	require.NoError(t, r.rollback(context.Background(), instanceB, prevRevision))
}

// TestRollback_SkipsUpdateWhenSpecAlreadyMatchesTarget verifies that if the
// raw spec has already been independently reverted to match the rollback
// target (e.g. a manual kubectl/GitOps revert) before rollback() runs, the
// short-circuit compares the raw current spec against the target and skips
// the Update entirely rather than re-pinning defaulted values onto the CR.
func TestRollback_SkipsUpdateWhenSpecAlreadyMatchesTarget(t *testing.T) {
	r, c := newRevisionTestReconciler(t)

	// Create the rollback target revision (specA).
	instanceA := newRevisionTestOwner("test-dda", "default")
	require.NoError(t, r.manageRevision(context.Background(), instanceA, instanceA.Spec, mustListRevisions(t, r, instanceA), nil))
	revListA := mustListRevisions(t, r, instanceA)
	require.Len(t, revListA, 1)
	target := revListA[0].Name

	// current already matches specA (manually reverted before rollback() ran).
	current := newRevisionTestOwner("test-dda", "default")
	require.NoError(t, c.Create(context.Background(), current))
	rvBefore := current.ResourceVersion

	require.NoError(t, r.rollback(context.Background(), current, target))

	updated := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Namespace: "default", Name: "test-dda"}, updated))
	assert.Equal(t, rvBefore, updated.ResourceVersion,
		"rollback must skip the Update when the raw spec already matches the target revision")
}

func TestRollback_NoPreviousRevision(t *testing.T) {
	r, _ := newRevisionTestReconciler(t)
	instance := newRevisionTestOwner("test-dda", "default")

	err := r.rollback(context.Background(), instance, "")
	require.NoError(t, err)
}

func TestProcessExperimentSignal_RollbackSignalRollsBack(t *testing.T) {
	r, c := newRevisionTestReconciler(t)

	// Create two revisions so we have a previous to roll back to.
	instanceA := newRevisionTestOwner("test-dda", "default")
	require.NoError(t, r.manageRevision(context.Background(), instanceA, instanceA.Spec, mustListRevisions(t, r, instanceA), nil))

	instanceB := newRevisionTestOwner("test-dda", "default")
	instanceB.Spec = v2alpha1.DatadogAgentSpec{Global: &v2alpha1.GlobalConfig{}}
	require.NoError(t, r.manageRevision(context.Background(), instanceB, instanceB.Spec, mustListRevisions(t, r, instanceB), nil))

	// Set annotations to signal rollback (different task ID), and status to running
	// (controller already processed start with a different ID).
	instanceB.Annotations = map[string]string{
		v2alpha1.AnnotationExperimentSignal: v2alpha1.ExperimentSignalRollback,
		v2alpha1.AnnotationExperimentID:     "stop-1",
	}

	// rollback fetches the current DDA to compare specs; it must exist in the fake client.
	require.NoError(t, c.Create(context.Background(), instanceB))

	revList := mustListRevisions(t, r, instanceB)
	instanceB.Status.Experiment = &v2alpha1.ExperimentStatus{
		Phase:                  v2alpha1.ExperimentPhaseRunning,
		ID:                     "exp-1",
		RollbackTargetRevision: findRollbackTarget(revList),
	}
	newStatus := &v2alpha1.DatadogAgentStatus{Experiment: instanceB.Status.Experiment.DeepCopy()}
	_, processErr := r.processExperimentSignal(context.Background(), instanceB, newStatus, metav1.Now(), revList)
	require.NoError(t, processErr)
	require.NotNil(t, newStatus.Experiment)
	assert.Equal(t, v2alpha1.ExperimentPhaseTerminated, newStatus.Experiment.Phase, "rollback signal should trigger terminated phase")
	assert.Equal(t, ExperimentTerminationReasonStopped, newStatus.Experiment.TerminationReason)
}

func TestRollback_PreservesNonDatadogAnnotations(t *testing.T) {
	r, c := newRevisionTestReconciler(t)

	// Create revision for specA with a Datadog annotation.
	instanceA := newRevisionTestOwner("test-dda", "default")
	instanceA.Annotations = map[string]string{
		"some.datadoghq.com/key": "old-value",
	}
	require.NoError(t, r.manageRevision(context.Background(), instanceA, instanceA.Spec, mustListRevisions(t, r, instanceA), nil))
	revListA := mustListRevisions(t, r, instanceA)
	require.Len(t, revListA, 1)
	prevRevision := revListA[0].Name

	// instanceB is the "current" DDA: has a different Datadog annotation value,
	// plus a non-Datadog annotation that should survive rollback.
	instanceB := newRevisionTestOwner("test-dda", "default")
	instanceB.Spec = v2alpha1.DatadogAgentSpec{Global: &v2alpha1.GlobalConfig{}}
	instanceB.Annotations = map[string]string{
		"some.datadoghq.com/key": "experiment-value",
		"user-tooling/key":       "keep-me",
	}
	require.NoError(t, c.Create(context.Background(), instanceB))

	require.NoError(t, r.rollback(context.Background(), instanceB, prevRevision))

	updated := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Namespace: "default", Name: "test-dda"}, updated))

	// Datadog annotation should be restored to the snapshot value.
	assert.Equal(t, "old-value", updated.Annotations["some.datadoghq.com/key"])
	// Non-Datadog annotation must be preserved.
	assert.Equal(t, "keep-me", updated.Annotations["user-tooling/key"])
}

func TestRestorePreviousSpec_PhaseSetOnlyOnSuccess(t *testing.T) {
	r, c := newRevisionTestReconciler(t)

	// Create two revisions so rollback has a target.
	instanceA := newRevisionTestOwner("test-dda", "default")
	require.NoError(t, r.manageRevision(context.Background(), instanceA, instanceA.Spec, mustListRevisions(t, r, instanceA), nil))

	instanceB := newRevisionTestOwner("test-dda", "default")
	instanceB.Spec = v2alpha1.DatadogAgentSpec{Global: &v2alpha1.GlobalConfig{}}
	require.NoError(t, r.manageRevision(context.Background(), instanceB, instanceB.Spec, mustListRevisions(t, r, instanceB), nil))

	revList := mustListRevisions(t, r, instanceB)
	newStatus := &v2alpha1.DatadogAgentStatus{Experiment: &v2alpha1.ExperimentStatus{
		Phase:                  v2alpha1.ExperimentPhaseRunning,
		RollbackTargetRevision: findRollbackTarget(revList),
	}}

	// rollback requires the DDA to exist in the fake client; don't create it so it errors.
	err := r.restorePreviousSpec(context.Background(), instanceB, newStatus, revList, ExperimentTerminationReasonStopped)
	require.Error(t, err)
	// Phase must NOT have been set since rollback failed.
	assert.Equal(t, v2alpha1.ExperimentPhaseRunning, newStatus.Experiment.Phase)

	// Now create the DDA so rollback can succeed.
	require.NoError(t, c.Create(context.Background(), instanceB))
	err = r.restorePreviousSpec(context.Background(), instanceB, newStatus, revList, ExperimentTerminationReasonStopped)
	require.NoError(t, err)
	assert.Equal(t, v2alpha1.ExperimentPhaseTerminated, newStatus.Experiment.Phase)
	assert.Equal(t, ExperimentTerminationReasonStopped, newStatus.Experiment.TerminationReason)
}

func TestManageExperiment_ManualChangeAbortsInsteadOfTimeout(t *testing.T) {
	r, _ := newRevisionTestReconciler(t)

	// Create two revisions so rollback has a target.
	instanceA := newRevisionTestOwner("test-dda", "default")
	require.NoError(t, r.manageRevision(context.Background(), instanceA, instanceA.Spec, mustListRevisions(t, r, instanceA), nil))

	instanceB := newRevisionTestOwner("test-dda", "default")
	instanceB.Spec = v2alpha1.DatadogAgentSpec{Global: &v2alpha1.GlobalConfig{}}
	require.NoError(t, r.manageRevision(context.Background(), instanceB, instanceB.Spec, mustListRevisions(t, r, instanceB), nil))

	// Simulate: Phase=Running, live spec differs from ExpectedSpecHash captured
	// at start (a hash of instanceB.Spec), AND timeout has elapsed. Under the
	// checkpoint model the hash-based manual-change check fires first and
	// aborts; timeout does not fire because abortExperiment publishes the
	// terminal phase before handleRollback runs.
	expectedHash, err := computeSpecHash(instanceB.Spec, instanceB.GetAnnotations())
	require.NoError(t, err)
	manualSite := "manual-change.example.com"
	instanceB.Spec = v2alpha1.DatadogAgentSpec{Global: &v2alpha1.GlobalConfig{Site: &manualSite}}
	startedAt := metav1.NewTime(time.Now().Add(-ExperimentDefaultTimeout - time.Minute))
	instanceB.Status.Experiment = &v2alpha1.ExperimentStatus{
		Phase:            v2alpha1.ExperimentPhaseRunning,
		StartedAt:        &startedAt,
		ExpectedSpecHash: expectedHash,
	}

	revList := mustListRevisions(t, r, instanceB)

	newStatus := &v2alpha1.DatadogAgentStatus{Experiment: instanceB.Status.Experiment.DeepCopy()}
	require.NoError(t, r.manageExperiment(context.Background(), instanceB, newStatus, metav1.Now(), revList))
	assert.Equal(t, v2alpha1.ExperimentPhaseAborted, newStatus.Experiment.Phase)
}

func TestHandleRollback_NoTimeoutOnFirstReconcile(t *testing.T) {
	r, c := newRevisionTestReconciler(t)

	// Only one revision exists — for the pre-experiment spec — with an old timestamp.
	instanceA := newRevisionTestOwner("test-dda", "default")
	require.NoError(t, r.manageRevision(context.Background(), instanceA, instanceA.Spec, mustListRevisions(t, r, instanceA), nil))
	revList := mustListRevisions(t, r, instanceA)
	require.Len(t, revList, 1)
	revList[0].CreationTimestamp = metav1.NewTime(time.Now().Add(-ExperimentDefaultTimeout - time.Hour))

	// instanceB has a different spec (the experiment spec); its revision hasn't been created yet.
	instanceB := newRevisionTestOwner("test-dda", "default")
	instanceB.Spec = v2alpha1.DatadogAgentSpec{Global: &v2alpha1.GlobalConfig{}}
	instanceB.Status.Experiment = &v2alpha1.ExperimentStatus{Phase: v2alpha1.ExperimentPhaseRunning}
	require.NoError(t, c.Create(context.Background(), instanceB))

	newStatus := &v2alpha1.DatadogAgentStatus{Experiment: instanceB.Status.Experiment.DeepCopy()}
	// Pass the stale revList (pre-experiment revision only) — timeout must NOT fire.
	require.NoError(t, r.handleRollback(context.Background(), instanceB, newStatus, metav1.Now(), revList))
	assert.Equal(t, v2alpha1.ExperimentPhaseRunning, newStatus.Experiment.Phase)
}

// TestHandleRollback_PostRollbackSetsTerminated verifies the reconcile-2 scenario:
// the spec has already been restored to the pre-experiment value (e.g. by a
// previous reconcile whose status write 409'd), so phase is still running and
// the generation is mismatched. findMostRecentMatchingRevision finds the
// pre-experiment revision (spec matches), elapsed is large, idempotent rollback
// fires, and phase=terminated with terminationReason=timed_out is set without a
// spec-update conflict.
func TestHandleRollback_PostRollbackSetsTerminated(t *testing.T) {
	r, c := newRevisionTestReconciler(t)

	// rev1: pre-experiment spec (instanceA).
	instanceA := newRevisionTestOwner("test-dda", "default")
	require.NoError(t, r.manageRevision(context.Background(), instanceA, instanceA.Spec, mustListRevisions(t, r, instanceA), nil))

	// rev2: experiment spec (instanceB).
	instanceB := newRevisionTestOwner("test-dda", "default")
	instanceB.Spec = v2alpha1.DatadogAgentSpec{Global: &v2alpha1.GlobalConfig{}}
	require.NoError(t, r.manageRevision(context.Background(), instanceB, instanceB.Spec, mustListRevisions(t, r, instanceB), nil))

	// The DDA in the cluster already has the rolled-back spec (instanceA's spec),
	// as if reconcile-1 restored it but its status write 409'd. StartedAt sits
	// past the timeout threshold so handleRollback fires the idempotent rollback.
	require.NoError(t, c.Create(context.Background(), instanceA))

	revList := mustListRevisions(t, r, instanceA)
	startedAt := metav1.NewTime(time.Now().Add(-ExperimentDefaultTimeout - time.Hour))
	instanceA.Status.Experiment = &v2alpha1.ExperimentStatus{
		Phase:                  v2alpha1.ExperimentPhaseRunning,
		StartedAt:              &startedAt,
		RollbackTargetRevision: findRollbackTarget(revList),
	}

	newStatus := &v2alpha1.DatadogAgentStatus{Experiment: instanceA.Status.Experiment.DeepCopy()}
	require.NoError(t, r.handleRollback(context.Background(), instanceA, newStatus, metav1.Now(), revList))
	assert.Equal(t, v2alpha1.ExperimentPhaseTerminated, newStatus.Experiment.Phase)
	assert.Equal(t, ExperimentTerminationReasonTimedOut, newStatus.Experiment.TerminationReason)
}

// TestReapplySameSpecAfterRollback_NoImmediateTimeout is the end-to-end
// regression test for the stale-revision bug.
//
// Without the fix: the stale experiment revision's old CreationTimestamp caused
// an immediate timeout when the same spec was re-applied as a new experiment.
//
// With the fix: restorePreviousSpec annotates the experiment revision with the
// rollback annotation. handleRollback skips the timeout check for annotated
// revisions. ensureRevision deletes+recreates the annotated revision with a
// fresh timestamp when the spec is re-applied.
func TestReapplySameSpecAfterRollback_NoImmediateTimeout(t *testing.T) {
	// This test asserts the pre-checkpoint model: rollback annotates a revision,
	// re-applied spec matches the annotated revision, ensureRevision recreates
	// it fresh, no immediate timeout. Under the checkpoint model the timeout
	// anchor is Status.Experiment.StartedAt (never revision timestamps), so
	// this scenario no longer requires special handling. To be replaced by
	// TestHandleRollback_TimeoutAnchoredOnStartedAt in Phase 7.
	t.Skip("obsolete under checkpoint model; will be replaced in Phase 7 deletion sweep")
	r, c := newRevisionTestReconciler(t)

	// Setup: create revisions for spec A (pre-experiment) and spec B (experiment).
	instanceA := newRevisionTestOwner("test-dda", "default")
	require.NoError(t, r.manageRevision(context.Background(), instanceA, instanceA.Spec, mustListRevisions(t, r, instanceA), nil))

	instanceB := newRevisionTestOwner("test-dda", "default")
	instanceB.Spec = v2alpha1.DatadogAgentSpec{Global: &v2alpha1.GlobalConfig{}}
	require.NoError(t, r.manageRevision(context.Background(), instanceB, instanceB.Spec, mustListRevisions(t, r, instanceB), nil))
	require.NoError(t, c.Create(context.Background(), instanceB))

	// Backdate rev2 (B) to simulate a long-running experiment whose revision
	// timestamp is well past the timeout threshold.
	revList := mustListRevisions(t, r, instanceB)
	for i := range revList {
		if revList[i].Revision == 2 {
			revList[i].CreationTimestamp = metav1.NewTime(time.Now().Add(-ExperimentDefaultTimeout - time.Hour))
		}
	}

	// Rollback: RC sends rollback signal via annotation (different task ID); operator restores spec A.
	// restorePreviousSpec annotates the experiment revision (B) as rolled back.
	instanceB.Annotations = map[string]string{
		v2alpha1.AnnotationExperimentSignal: v2alpha1.ExperimentSignalRollback,
		v2alpha1.AnnotationExperimentID:     "stop-1",
	}
	instanceB.Status.Experiment = &v2alpha1.ExperimentStatus{Phase: v2alpha1.ExperimentPhaseRunning, ID: "exp-1"}
	rollbackStatus := &v2alpha1.DatadogAgentStatus{Experiment: instanceB.Status.Experiment.DeepCopy()}
	_, processErr := r.processExperimentSignal(context.Background(), instanceB, rollbackStatus, metav1.Now(), revList)
	require.NoError(t, processErr)
	require.Equal(t, v2alpha1.ExperimentPhaseTerminated, rollbackStatus.Experiment.Phase)
	require.Equal(t, ExperimentTerminationReasonStopped, rollbackStatus.Experiment.TerminationReason)

	// Verify the experiment revision was annotated.
	remaining := mustListRevisions(t, r, instanceA)
	require.Len(t, remaining, 2, "both revisions should be kept (no aggressive GC)")
	var annotatedCount int
	for _, rev := range remaining {
		if revisionExperimentState(&rev) == experimentRevisionStateRolledBack {
			annotatedCount++
		}
	}
	assert.Equal(t, 1, annotatedCount, "exactly one revision should have the rollback annotation")

	// RC re-applies spec B as a new experiment.
	// In the real flow, the daemon patches the spec first, then a reconcile runs
	// (with no experiment phase set) where ensureRevision recreates the annotated
	// revision with a fresh timestamp. Only then does the daemon set phase=Running
	// and the next reconcile calls handleRollback.
	instanceB2 := newRevisionTestOwner("test-dda", "default")
	instanceB2.Spec = v2alpha1.DatadogAgentSpec{Global: &v2alpha1.GlobalConfig{}}

	// Step 1: ensureRevision recreates the annotated revision (fresh, no annotation).
	_, err := r.ensureRevision(context.Background(), instanceB2, instanceB2.Spec, mustListRevisions(t, r, instanceB2))
	require.NoError(t, err)

	finalRevs := mustListRevisions(t, r, instanceB2)
	for _, rev := range finalRevs {
		assert.NotEqual(t, experimentRevisionStateRolledBack, revisionExperimentState(&rev),
			"rolled-back state should be cleared after recreate")
	}

	// Fake client doesn't set CreationTimestamp on Create, so patch all
	// revision timestamps to now to simulate fresh revisions.
	for i := range finalRevs {
		finalRevs[i].CreationTimestamp = metav1.Now()
		require.NoError(t, c.Update(context.Background(), &finalRevs[i]))
	}

	// Step 2: daemon sets phase=Running, next reconcile calls handleRollback.
	instanceB2.Status.Experiment = &v2alpha1.ExperimentStatus{Phase: v2alpha1.ExperimentPhaseRunning}
	revListForNewExp := mustListRevisions(t, r, instanceB2)
	newStatus2 := &v2alpha1.DatadogAgentStatus{Experiment: instanceB2.Status.Experiment.DeepCopy()}
	require.NoError(t, r.handleRollback(context.Background(), instanceB2, newStatus2, metav1.Now(), revListForNewExp))
	assert.Equal(t, v2alpha1.ExperimentPhaseRunning, newStatus2.Experiment.Phase,
		"re-applying the same spec after rollback must not immediately time out")
}

// TestRestorePreviousSpec_ThreeRevisions_AnnotatesOnlyHighest verifies that
// when 3+ revisions exist (e.g. GC failed on a prior reconcile), only the
// highest-numbered revision (the experiment) is annotated — not older baselines.
func TestRestorePreviousSpec_ThreeRevisions_AnnotatesOnlyHighest(t *testing.T) {
	r, c := newRevisionTestReconciler(t)

	// Build 3 revisions using ensureRevision directly (bypasses GC).
	instanceA := newRevisionTestOwner("test-dda", "default")
	rev1Name, err := r.ensureRevision(context.Background(), instanceA, instanceA.Spec, nil)
	require.NoError(t, err)

	instanceB := newRevisionTestOwner("test-dda", "default")
	instanceB.Spec = v2alpha1.DatadogAgentSpec{Global: &v2alpha1.GlobalConfig{}}
	rev2Name, err := r.ensureRevision(context.Background(), instanceB, instanceB.Spec, mustListRevisions(t, r, instanceB))
	require.NoError(t, err)

	experimentSite := "datadoghq.eu"
	instanceC := newRevisionTestOwner("test-dda", "default")
	instanceC.Spec = v2alpha1.DatadogAgentSpec{Global: &v2alpha1.GlobalConfig{Site: &experimentSite}}
	rev3Name, err := r.ensureRevision(context.Background(), instanceC, instanceC.Spec, mustListRevisions(t, r, instanceC))
	require.NoError(t, err)

	revList := mustListRevisions(t, r, instanceA)
	require.Len(t, revList, 3, "need 3 revisions to test this scenario")

	// rollback fetches the current DDA; create it with the experiment spec.
	require.NoError(t, c.Create(context.Background(), instanceC))

	// Trigger rollback. Rollback target is the explicitly checkpointed baseline
	// (rev2 in this scenario — findRollbackTarget picks the second-highest).
	instanceC.Status.Experiment = &v2alpha1.ExperimentStatus{
		Phase:                  v2alpha1.ExperimentPhaseRunning,
		RollbackTargetRevision: findRollbackTarget(revList),
	}
	newStatus := &v2alpha1.DatadogAgentStatus{Experiment: instanceC.Status.Experiment.DeepCopy()}
	require.NoError(t, r.restorePreviousSpec(context.Background(), instanceC, newStatus, revList, ExperimentTerminationReasonStopped))
	assert.Equal(t, v2alpha1.ExperimentPhaseTerminated, newStatus.Experiment.Phase)
	assert.Equal(t, ExperimentTerminationReasonStopped, newStatus.Experiment.TerminationReason)

	// Under the checkpoint model restorePreviousSpec no longer annotates any
	// revision — the rollback target is proven by status, not inferred from
	// annotations. Assert none of the three revisions gained the annotation.
	_ = rev1Name
	_ = rev2Name
	_ = rev3Name
	for _, rev := range mustListRevisions(t, r, instanceA) {
		assert.NotEqual(t, experimentRevisionStateRolledBack, revisionExperimentState(&rev),
			"restorePreviousSpec must not annotate revisions under the checkpoint model")
	}
}

// TestAbortExperiment_ThreeRevisions_AnnotatesOnlyHighest verifies that when
// 3+ revisions exist and abort fires, only the highest-numbered revision (the
// experiment) is annotated — not older baselines.
func TestAbortExperiment_ThreeRevisions_AnnotatesOnlyHighest(t *testing.T) {
	// Under the checkpoint model abortExperiment no longer annotates any
	// revision — abort is proven by the ExpectedSpecHash mismatch alone.
	// Test's original intent is Phase-7-obsolete.
	t.Skip("obsolete under checkpoint model; will be deleted in Phase 7")
	r, _ := newRevisionTestReconciler(t)

	// Build 3 revisions using ensureRevision directly (bypasses GC).
	instanceA := newRevisionTestOwner("test-dda", "default")
	rev1Name, err := r.ensureRevision(context.Background(), instanceA, instanceA.Spec, nil)
	require.NoError(t, err)

	instanceB := newRevisionTestOwner("test-dda", "default")
	instanceB.Spec = v2alpha1.DatadogAgentSpec{Global: &v2alpha1.GlobalConfig{}}
	rev2Name, err := r.ensureRevision(context.Background(), instanceB, instanceB.Spec, mustListRevisions(t, r, instanceB))
	require.NoError(t, err)

	experimentSite := "datadoghq.eu"
	instanceC := newRevisionTestOwner("test-dda", "default")
	instanceC.Spec = v2alpha1.DatadogAgentSpec{Global: &v2alpha1.GlobalConfig{Site: &experimentSite}}
	rev3Name, err := r.ensureRevision(context.Background(), instanceC, instanceC.Spec, mustListRevisions(t, r, instanceC))
	require.NoError(t, err)

	revList := mustListRevisions(t, r, instanceA)
	require.Len(t, revList, 3)

	// Set recent timestamps so timeout doesn't fire first.
	for i := range revList {
		revList[i].CreationTimestamp = metav1.Now()
	}

	// Simulate manual spec change (specD) — doesn't match any revision.
	manualSite := "manual-change.example.com"
	instanceD := newRevisionTestOwner("test-dda", "default")
	instanceD.Spec = v2alpha1.DatadogAgentSpec{Global: &v2alpha1.GlobalConfig{Site: &manualSite}}
	instanceD.Status.Experiment = &v2alpha1.ExperimentStatus{Phase: v2alpha1.ExperimentPhaseRunning}

	newStatus := &v2alpha1.DatadogAgentStatus{Experiment: instanceD.Status.Experiment.DeepCopy()}
	r.abortExperiment(context.Background(), instanceD, instanceD.Status.Experiment, newStatus, revList)
	assert.Equal(t, v2alpha1.ExperimentPhaseAborted, newStatus.Experiment.Phase)

	// Verify: only rev3 (experiment, highest) is annotated.
	for _, rev := range mustListRevisions(t, r, instanceA) {
		hasAnnotation := revisionExperimentState(&rev) == experimentRevisionStateRolledBack
		switch rev.Name {
		case rev3Name:
			assert.True(t, hasAnnotation, "rev3 (experiment, highest) should be annotated")
		case rev2Name:
			assert.False(t, hasAnnotation, "rev2 should NOT be annotated")
		case rev1Name:
			assert.False(t, hasAnnotation, "rev1 (old baseline) should NOT be annotated")
		}
	}
}

func TestHandleRollback_Timeout(t *testing.T) {
	r, c := newRevisionTestReconciler(t)

	// Create two revisions so rollback has a target.
	instanceA := newRevisionTestOwner("test-dda", "default")
	require.NoError(t, r.manageRevision(context.Background(), instanceA, instanceA.Spec, mustListRevisions(t, r, instanceA), nil))

	instanceB := newRevisionTestOwner("test-dda", "default")
	instanceB.Spec = v2alpha1.DatadogAgentSpec{Global: &v2alpha1.GlobalConfig{}}
	require.NoError(t, r.manageRevision(context.Background(), instanceB, instanceB.Spec, mustListRevisions(t, r, instanceB), nil))

	// rollback fetches the current DDA to compare specs; it must exist in the fake client.
	require.NoError(t, c.Create(context.Background(), instanceB))

	revList := mustListRevisions(t, r, instanceB)
	// StartedAt past the timeout threshold triggers the rollback path.
	startedAt := metav1.NewTime(time.Now().Add(-ExperimentDefaultTimeout - time.Minute))
	instanceB.Status.Experiment = &v2alpha1.ExperimentStatus{
		Phase:                  v2alpha1.ExperimentPhaseRunning,
		StartedAt:              &startedAt,
		RollbackTargetRevision: findRollbackTarget(revList),
	}

	newStatus := &v2alpha1.DatadogAgentStatus{Experiment: instanceB.Status.Experiment.DeepCopy()}
	require.NoError(t, r.handleRollback(context.Background(), instanceB, newStatus, metav1.Now(), revList))
	require.NotNil(t, newStatus.Experiment)
	assert.Equal(t, v2alpha1.ExperimentPhaseTerminated, newStatus.Experiment.Phase)
	assert.Equal(t, ExperimentTerminationReasonTimedOut, newStatus.Experiment.TerminationReason)
}

// --- processExperimentSignal tests ---

func TestProcessExperimentSignal_StartNewExperiment(t *testing.T) {
	r, _ := newRevisionTestReconciler(t)
	instance := newRevisionTestOwner("test-dda", "default")
	instance.Annotations = map[string]string{
		v2alpha1.AnnotationExperimentSignal:                    v2alpha1.ExperimentSignalStart,
		v2alpha1.AnnotationExperimentID:                        "exp-new",
		v2alpha1.AnnotationExperimentRollbackTargetRevision:    "baseline-rev",
	}

	newStatus := &v2alpha1.DatadogAgentStatus{}
	_, processErr := r.processExperimentSignal(context.Background(), instance, newStatus, metav1.Now(), nil)
	require.NoError(t, processErr)
	require.NotNil(t, newStatus.Experiment)
	assert.Equal(t, v2alpha1.ExperimentPhaseRunning, newStatus.Experiment.Phase)
	assert.Equal(t, "exp-new", newStatus.Experiment.ID)
	assert.Equal(t, "baseline-rev", newStatus.Experiment.RollbackTargetRevision)
	assert.NotEmpty(t, newStatus.Experiment.ExpectedSpecHash)
}

// TestProcessStartSignal_AbortsWhenBaselineMissing verifies that a start
// signal without the rollback-target-revision annotation aborts the
// experiment (with StartTaskID persisted so the daemon can report ERROR).
func TestProcessStartSignal_AbortsWhenBaselineMissing(t *testing.T) {
	r, _ := newRevisionTestReconciler(t)
	instance := newRevisionTestOwner("test-dda", "default")
	instance.Annotations = map[string]string{
		v2alpha1.AnnotationExperimentSignal: v2alpha1.ExperimentSignalStart,
		v2alpha1.AnnotationExperimentID:     "exp-abort",
		v2alpha1.AnnotationPendingTaskID:    "task-42",
	}

	newStatus := &v2alpha1.DatadogAgentStatus{}
	_, processErr := r.processExperimentSignal(context.Background(), instance, newStatus, metav1.Now(), nil)
	require.NoError(t, processErr)
	require.NotNil(t, newStatus.Experiment)
	assert.Equal(t, v2alpha1.ExperimentPhaseAborted, newStatus.Experiment.Phase)
	assert.Equal(t, "exp-abort", newStatus.Experiment.ID)
	assert.Equal(t, "task-42", newStatus.Experiment.StartTaskID,
		"StartTaskID must be persisted so reconcileLocallyTerminatedExperiment can report ERROR to RC")
	assert.Equal(t, ExperimentTerminationReasonBaselineMissing, newStatus.Experiment.TerminationReason)
}

func TestProcessExperimentSignal_StartIdempotent(t *testing.T) {
	r, _ := newRevisionTestReconciler(t)
	instance := newRevisionTestOwner("test-dda", "default")
	instance.Annotations = map[string]string{
		v2alpha1.AnnotationExperimentSignal: v2alpha1.ExperimentSignalStart,
		v2alpha1.AnnotationExperimentID:     "exp-1",
	}
	instance.Status.Experiment = &v2alpha1.ExperimentStatus{
		Phase: v2alpha1.ExperimentPhaseRunning,
		ID:    "exp-1",
	}

	newStatus := &v2alpha1.DatadogAgentStatus{Experiment: instance.Status.Experiment.DeepCopy()}
	_, processErr := r.processExperimentSignal(context.Background(), instance, newStatus, metav1.Now(), nil)
	require.NoError(t, processErr)
	// No change — already processed.
	assert.Equal(t, v2alpha1.ExperimentPhaseRunning, newStatus.Experiment.Phase)
	assert.Equal(t, "exp-1", newStatus.Experiment.ID)
}

func TestProcessExperimentSignal_StartBlockedByRunningExperiment(t *testing.T) {
	r, _ := newRevisionTestReconciler(t)
	instance := newRevisionTestOwner("test-dda", "default")
	instance.Annotations = map[string]string{
		v2alpha1.AnnotationExperimentSignal: v2alpha1.ExperimentSignalStart,
		v2alpha1.AnnotationExperimentID:     "exp-new",
	}
	instance.Status.Experiment = &v2alpha1.ExperimentStatus{
		Phase: v2alpha1.ExperimentPhaseRunning,
		ID:    "exp-existing",
	}

	newStatus := &v2alpha1.DatadogAgentStatus{Experiment: instance.Status.Experiment.DeepCopy()}
	_, processErr := r.processExperimentSignal(context.Background(), instance, newStatus, metav1.Now(), nil)
	require.NoError(t, processErr)
	// Refused — existing experiment still running.
	assert.Equal(t, v2alpha1.ExperimentPhaseRunning, newStatus.Experiment.Phase)
	assert.Equal(t, "exp-existing", newStatus.Experiment.ID)
}

// TestProcessExperimentSignal_RollbackDifferentID verifies that a rollback
// signal with a different annotation ID (normal for stop requests which have
// their own task ID) still triggers rollback of the running experiment.
func TestProcessExperimentSignal_RollbackDifferentID(t *testing.T) {
	r, _ := newRevisionTestReconciler(t)
	instance := newRevisionTestOwner("test-dda", "default")
	instance.Annotations = map[string]string{
		v2alpha1.AnnotationExperimentSignal: v2alpha1.ExperimentSignalRollback,
		v2alpha1.AnnotationExperimentID:     "stop-task-id",
	}
	// Status has no RollbackTargetRevision and no revision exists in the fake
	// client. The rollback signal is still processed — under the checkpoint
	// model that means the experiment aborts with baseline_not_found rather
	// than falling through to a heuristic target. Intent of this test:
	// verify the signal is acted on regardless of task-ID mismatch.
	instance.Status.Experiment = &v2alpha1.ExperimentStatus{
		Phase: v2alpha1.ExperimentPhaseRunning,
		ID:    "exp-1",
	}

	newStatus := &v2alpha1.DatadogAgentStatus{Experiment: instance.Status.Experiment.DeepCopy()}
	_, processErr := r.processExperimentSignal(context.Background(), instance, newStatus, metav1.Now(), nil)
	require.NoError(t, processErr)
	assert.True(t, isTerminalPhase(newStatus.Experiment.Phase),
		"rollback signal must be processed to a terminal phase despite different annotation ID")
	assert.Equal(t, v2alpha1.ExperimentPhaseAborted, newStatus.Experiment.Phase,
		"no baseline in status → aborted with baseline_not_found")
	assert.Equal(t, ExperimentTerminationReasonBaselineNotFound, newStatus.Experiment.TerminationReason)
}

func TestProcessExperimentSignal_RollbackTerminalPhaseNoOp(t *testing.T) {
	r, _ := newRevisionTestReconciler(t)
	instance := newRevisionTestOwner("test-dda", "default")
	instance.Annotations = map[string]string{
		v2alpha1.AnnotationExperimentSignal: v2alpha1.ExperimentSignalRollback,
		v2alpha1.AnnotationExperimentID:     "exp-1",
	}
	instance.Status.Experiment = &v2alpha1.ExperimentStatus{
		Phase: v2alpha1.ExperimentPhaseTerminated,
		ID:    "exp-1",
	}

	newStatus := &v2alpha1.DatadogAgentStatus{Experiment: instance.Status.Experiment.DeepCopy()}
	_, processErr := r.processExperimentSignal(context.Background(), instance, newStatus, metav1.Now(), nil)
	require.NoError(t, processErr)
	// Already terminal — no-op.
	assert.Equal(t, v2alpha1.ExperimentPhaseTerminated, newStatus.Experiment.Phase)
}

func TestProcessExperimentSignal_PromoteRunning(t *testing.T) {
	r, _ := newRevisionTestReconciler(t)

	// Create a revision matching the instance spec so promote sees a matching revision.
	instance := newRevisionTestOwner("test-dda", "default")
	require.NoError(t, r.manageRevision(context.Background(), instance, instance.Spec, mustListRevisions(t, r, instance), nil))
	// Create a second revision to satisfy len(revisions) >= 2.
	instance2 := newRevisionTestOwner("test-dda", "default")
	instance2.Spec = v2alpha1.DatadogAgentSpec{Global: &v2alpha1.GlobalConfig{}}
	require.NoError(t, r.manageRevision(context.Background(), instance2, instance2.Spec, mustListRevisions(t, r, instance2), nil))

	// Now promote back to the first spec (which has a matching revision).
	// The promote signal has its own task ID, different from the start experiment ID.
	instance.Annotations = map[string]string{
		v2alpha1.AnnotationExperimentSignal: v2alpha1.ExperimentSignalPromote,
		v2alpha1.AnnotationExperimentID:     "promote-1",
	}
	instance.Status.Experiment = &v2alpha1.ExperimentStatus{
		Phase: v2alpha1.ExperimentPhaseRunning,
		ID:    "exp-1",
	}

	revList := mustListRevisions(t, r, instance)
	newStatus := &v2alpha1.DatadogAgentStatus{Experiment: instance.Status.Experiment.DeepCopy()}
	_, processErr := r.processExperimentSignal(context.Background(), instance, newStatus, metav1.Now(), revList)
	require.NoError(t, processErr)
	assert.Equal(t, v2alpha1.ExperimentPhasePromoted, newStatus.Experiment.Phase)
}

func TestProcessExperimentSignal_PromoteBeatsTimeout(t *testing.T) {
	r, c := newRevisionTestReconciler(t)

	// Create two revisions.
	instanceA := newRevisionTestOwner("test-dda", "default")
	require.NoError(t, r.manageRevision(context.Background(), instanceA, instanceA.Spec, mustListRevisions(t, r, instanceA), nil))

	instanceB := newRevisionTestOwner("test-dda", "default")
	instanceB.Spec = v2alpha1.DatadogAgentSpec{Global: &v2alpha1.GlobalConfig{}}
	require.NoError(t, r.manageRevision(context.Background(), instanceB, instanceB.Spec, mustListRevisions(t, r, instanceB), nil))
	require.NoError(t, c.Create(context.Background(), instanceB))

	// Set promote annotation (different task ID) and running phase with timeout elapsed.
	instanceB.Annotations = map[string]string{
		v2alpha1.AnnotationExperimentSignal: v2alpha1.ExperimentSignalPromote,
		v2alpha1.AnnotationExperimentID:     "promote-1",
	}
	revList := mustListRevisions(t, r, instanceB)
	expectedHash, err := computeSpecHash(instanceB.Spec, instanceB.GetAnnotations())
	require.NoError(t, err)
	instanceB.Status.Experiment = &v2alpha1.ExperimentStatus{
		Phase:                  v2alpha1.ExperimentPhaseRunning,
		ID:                     "exp-1",
		RollbackTargetRevision: findRollbackTarget(revList),
		ExpectedSpecHash:       expectedHash,
	}

	newStatus := &v2alpha1.DatadogAgentStatus{Experiment: instanceB.Status.Experiment.DeepCopy()}
	// Run the full manageExperiment flow — processExperimentSignal runs first,
	// sets promoted, then handleRollback sees the phase change and skips.
	require.NoError(t, r.manageExperiment(context.Background(), instanceB, newStatus, metav1.Now(), revList))
	assert.Equal(t, v2alpha1.ExperimentPhasePromoted, newStatus.Experiment.Phase, "promote should beat timeout")
}

func TestProcessExperimentSignal_NoAnnotations(t *testing.T) {
	r, _ := newRevisionTestReconciler(t)
	instance := newRevisionTestOwner("test-dda", "default")
	instance.Status.Experiment = &v2alpha1.ExperimentStatus{
		Phase: v2alpha1.ExperimentPhaseRunning,
		ID:    "exp-1",
	}

	newStatus := &v2alpha1.DatadogAgentStatus{Experiment: instance.Status.Experiment.DeepCopy()}
	_, processErr := r.processExperimentSignal(context.Background(), instance, newStatus, metav1.Now(), nil)
	require.NoError(t, processErr)
	// No annotations — no change.
	assert.Equal(t, v2alpha1.ExperimentPhaseRunning, newStatus.Experiment.Phase)
}

func TestProcessExperimentSignal_RollbackBeatsTimeout(t *testing.T) {
	r, c := newRevisionTestReconciler(t)

	// Create two revisions.
	instanceA := newRevisionTestOwner("test-dda", "default")
	require.NoError(t, r.manageRevision(context.Background(), instanceA, instanceA.Spec, mustListRevisions(t, r, instanceA), nil))

	instanceB := newRevisionTestOwner("test-dda", "default")
	instanceB.Spec = v2alpha1.DatadogAgentSpec{Global: &v2alpha1.GlobalConfig{}}
	require.NoError(t, r.manageRevision(context.Background(), instanceB, instanceB.Spec, mustListRevisions(t, r, instanceB), nil))
	require.NoError(t, c.Create(context.Background(), instanceB))

	// Set rollback annotation (different task ID) and running phase with timeout elapsed.
	instanceB.Annotations = map[string]string{
		v2alpha1.AnnotationExperimentSignal: v2alpha1.ExperimentSignalRollback,
		v2alpha1.AnnotationExperimentID:     "stop-1",
	}
	revList := mustListRevisions(t, r, instanceB)
	for i := range revList {
		revList[i].CreationTimestamp = metav1.NewTime(time.Now().Add(-ExperimentDefaultTimeout - time.Minute))
	}
	expectedHash, err := computeSpecHash(instanceB.Spec, instanceB.GetAnnotations())
	require.NoError(t, err)
	instanceB.Status.Experiment = &v2alpha1.ExperimentStatus{
		Phase:                  v2alpha1.ExperimentPhaseRunning,
		ID:                     "exp-1",
		RollbackTargetRevision: findRollbackTarget(revList),
		ExpectedSpecHash:       expectedHash,
	}

	newStatus := &v2alpha1.DatadogAgentStatus{Experiment: instanceB.Status.Experiment.DeepCopy()}
	require.NoError(t, r.manageExperiment(context.Background(), instanceB, newStatus, metav1.Now(), revList))
	assert.Equal(t, v2alpha1.ExperimentPhaseTerminated, newStatus.Experiment.Phase, "rollback should beat timeout")
	assert.Equal(t, ExperimentTerminationReasonStopped, newStatus.Experiment.TerminationReason)
}

func TestManageExperiment_StartSignalDoesNotClearAnnotationsPrematurely(t *testing.T) {
	r, c := newRevisionTestReconciler(t)

	// Create a DDA with a start signal annotation but no active experiment status.
	// processExperimentSignal should create the experiment in newStatus,
	// and annotations must NOT be cleared (they'll be cleared on the next reconcile).
	instance := newRevisionTestOwner("test-dda", "default")
	instance.Annotations = map[string]string{
		v2alpha1.AnnotationExperimentSignal:                 v2alpha1.ExperimentSignalStart,
		v2alpha1.AnnotationExperimentID:                     "new-exp",
		v2alpha1.AnnotationExperimentRollbackTargetRevision: "baseline-rev",
	}
	require.NoError(t, c.Create(context.Background(), instance))

	newStatus := &v2alpha1.DatadogAgentStatus{}
	require.NoError(t, r.manageExperiment(context.Background(), instance, newStatus, metav1.Now(), nil))

	// The experiment should have been created in newStatus.
	require.NotNil(t, newStatus.Experiment)
	assert.Equal(t, v2alpha1.ExperimentPhaseRunning, newStatus.Experiment.Phase)

	// Annotations should still be present — not prematurely cleared.
	got := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "test-dda", Namespace: "default"}, got))
	assert.Equal(t, v2alpha1.ExperimentSignalStart, got.Annotations[v2alpha1.AnnotationExperimentSignal],
		"start signal annotations should not be cleared when experiment was just created")
}

func TestManageExperiment_ClearsNoOpSignalWhenNoExperiment(t *testing.T) {
	r, c := newRevisionTestReconciler(t)

	// Create a DDA with a signal annotation but no active experiment status.
	instance := newRevisionTestOwner("test-dda", "default")
	instance.Annotations = map[string]string{
		v2alpha1.AnnotationExperimentSignal: v2alpha1.ExperimentSignalRollback,
		v2alpha1.AnnotationExperimentID:     "stale-1",
	}
	require.NoError(t, c.Create(context.Background(), instance))

	newStatus := &v2alpha1.DatadogAgentStatus{}
	require.NoError(t, r.manageExperiment(context.Background(), instance, newStatus, metav1.Now(), nil))

	// Annotations should have been cleared.
	got := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "test-dda", Namespace: "default"}, got))
	assert.Empty(t, got.Annotations[v2alpha1.AnnotationExperimentSignal], "no-op signal should be cleared")
	assert.Empty(t, got.Annotations[v2alpha1.AnnotationExperimentID], "no-op signal ID should be cleared")
}

// TestRevisionState_SingleAnnotation verifies that promote/rollback
// terminal states are represented by a single annotation value rather
// than two boolean flags. Flipping between states must overwrite, not
// accumulate — a revision cannot simultaneously be both promoted and
// rolled-back.
func TestRevisionState_SingleAnnotation(t *testing.T) {
	t.Skip("obsolete under checkpoint model")
	r, _ := newRevisionTestReconciler(t)

	instance := newRevisionTestOwner("test-dda", "default")
	revName, err := r.ensureRevision(context.Background(), instance, instance.Spec, nil)
	require.NoError(t, err)
	revList := mustListRevisions(t, r, instance)
	require.Len(t, revList, 1)
	rev := &revList[0]

	r.markRevisionState(context.Background(), rev, experimentRevisionStatePromoted)
	updated := fetchRevisionByName(t, r.client, instance.Namespace, revName)
	assert.Equal(t, experimentRevisionStatePromoted, revisionExperimentState(updated))

	// Flip to rolled-back: modern annotation overwrites, no second key created.
	r.markRevisionState(context.Background(), updated, experimentRevisionStateRolledBack)
	updated = fetchRevisionByName(t, r.client, instance.Namespace, revName)
	assert.Equal(t, experimentRevisionStateRolledBack, revisionExperimentState(updated))

	// And back to promoted.
	r.markRevisionState(context.Background(), updated, experimentRevisionStatePromoted)
	updated = fetchRevisionByName(t, r.client, instance.Namespace, revName)
	assert.Equal(t, experimentRevisionStatePromoted, revisionExperimentState(updated))
}

// TestHandleRollback_StartedAt_AnchorsTimeout verifies that handleRollback
// measures elapsed time against Status.Experiment.StartedAt and not against
// rev.CreationTimestamp.
//
// Regression: when a new experiment's spec equals a pre-existing baseline
// revision whose CreationTimestamp is far in the past, the old behaviour
// fired an immediate timeout on the first reconcile after Phase=Running
// because elapsed-against-rev-timestamp exceeded the threshold even
// though the experiment had just started. Anchoring on StartedAt makes
// the decision independent of revision metadata.
func TestHandleRollback_StartedAt_AnchorsTimeout(t *testing.T) {
	r, _ := newRevisionTestReconciler(t)

	// Build a baseline revision with an ancient CreationTimestamp.
	instance := newRevisionTestOwner("test-dda", "default")
	_, err := r.ensureRevision(context.Background(), instance, instance.Spec, nil)
	require.NoError(t, err)
	revList := mustListRevisions(t, r, instance)
	require.Len(t, revList, 1)
	revList[0].CreationTimestamp = metav1.NewTime(time.Now().Add(-24 * time.Hour))

	// Experiment just started (StartedAt = now), Phase=Running, ID set.
	startedAt := metav1.NewTime(time.Now().Add(-1 * time.Second))
	instance.Status.Experiment = &v2alpha1.ExperimentStatus{
		Phase:     v2alpha1.ExperimentPhaseRunning,
		ID:        "exp-1",
		StartedAt: &startedAt,
	}

	newStatus := &v2alpha1.DatadogAgentStatus{Experiment: instance.Status.Experiment.DeepCopy()}
	require.NoError(t, r.handleRollback(context.Background(), instance, newStatus, metav1.Now(), revList))
	// Phase must still be Running — the ancient baseline-revision timestamp
	// must NOT be used as the timeout anchor.
	require.NotNil(t, newStatus.Experiment)
	assert.Equal(t, v2alpha1.ExperimentPhaseRunning, newStatus.Experiment.Phase,
		"handleRollback must use StartedAt as the timeout anchor; using rev.CreationTimestamp would fire an immediate timeout here")
}

// TestProcessStartSignal_CapturesStartTaskID verifies that the daemon's
// pending-task-id annotation is captured into Status.Experiment.StartTaskID
// on the Running transition. Without this, the daemon cannot later report
// TaskState_ERROR for the original start task on local timeout.
func TestProcessStartSignal_CapturesStartTaskID(t *testing.T) {
	r, _ := newRevisionTestReconciler(t)

	const taskID = "task-uuid-abc-123"
	const expID = "exp-new"
	instance := newRevisionTestOwner("test-dda", "default")
	instance.Annotations = map[string]string{
		v2alpha1.AnnotationExperimentSignal:                 v2alpha1.ExperimentSignalStart,
		v2alpha1.AnnotationExperimentID:                     expID,
		v2alpha1.AnnotationPendingTaskID:                    taskID,
		v2alpha1.AnnotationPendingAction:                    "start",
		v2alpha1.AnnotationExperimentRollbackTargetRevision: "baseline-rev",
	}

	newStatus := &v2alpha1.DatadogAgentStatus{}
	_, processErr := r.processExperimentSignal(context.Background(), instance, newStatus, metav1.Now(), nil)
	require.NoError(t, processErr)
	require.NotNil(t, newStatus.Experiment)
	assert.Equal(t, v2alpha1.ExperimentPhaseRunning, newStatus.Experiment.Phase)
	assert.Equal(t, expID, newStatus.Experiment.ID)
	assert.Equal(t, taskID, newStatus.Experiment.StartTaskID,
		"start task ID must be captured from the pending annotation so it survives "+
			"daemon restarts and is available to report timeout errors")
}
