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
	require.NoError(t, r.manageRevision(context.Background(), instanceA, mustListRevisions(t, r, instanceA), nil))

	instanceB := newRevisionTestOwner("test-dda", "default")
	instanceB.Spec = v2alpha1.DatadogAgentSpec{Global: &v2alpha1.GlobalConfig{}}
	require.NoError(t, r.manageRevision(context.Background(), instanceB, mustListRevisions(t, r, instanceB), nil))

	// Simulate a manual spec change: specC doesn't match any revision.
	instanceC := newRevisionTestOwner("test-dda", "default")
	manualSite := "manual-change.example.com"
	instanceC.Spec = v2alpha1.DatadogAgentSpec{Global: &v2alpha1.GlobalConfig{Site: &manualSite}}
	instanceC.Status.Experiment = &v2alpha1.ExperimentStatus{
		Phase: v2alpha1.ExperimentPhaseRunning,
	}

	// Set recent timestamps so the timeout path in handleRollback does not fire.
	revList := mustListRevisions(t, r, instanceC)
	for i := range revList {
		revList[i].CreationTimestamp = metav1.Now()
	}

	status := &v2alpha1.DatadogAgentStatus{
		Experiment: instanceC.Status.Experiment.DeepCopy(),
	}

	err := r.manageExperiment(context.Background(), instanceC, status, metav1.Now(), revList)
	require.NoError(t, err)
	require.NotNil(t, status.Experiment)
	assert.Equal(t, v2alpha1.ExperimentPhaseAborted, status.Experiment.Phase)
}

// TestManageExperiment_ManualRevertToBaselineTerminatesViaTimeout verifies that
// when the user manually reverts the spec to the pre-experiment value during a
// running experiment, the experiment terminates via timeout rather than abort.
// The revision-based abort check sees the spec matching the baseline revision
// and treats it as a known state. The timeout path fires because the baseline
// revision's old timestamp exceeds the threshold. The rollback is a no-op
// (spec already matches target), and the phase is set to "timeout".
func TestManageExperiment_ManualRevertToBaselineTerminatesViaTimeout(t *testing.T) {
	r, c := newRevisionTestReconciler(t)

	// Rev1: pre-experiment spec (specA).
	instanceA := newRevisionTestOwner("test-dda", "default")
	require.NoError(t, r.manageRevision(context.Background(), instanceA, mustListRevisions(t, r, instanceA), nil))

	// Rev2: experiment spec (specB).
	instanceB := newRevisionTestOwner("test-dda", "default")
	instanceB.Spec = v2alpha1.DatadogAgentSpec{Global: &v2alpha1.GlobalConfig{}}
	require.NoError(t, r.manageRevision(context.Background(), instanceB, mustListRevisions(t, r, instanceB), nil))
	require.NoError(t, c.Create(context.Background(), instanceA))

	// User manually reverts to specA. The spec matches rev1 (the baseline),
	// so abortExperiment won't fire. Instead, handleRollback detects timeout
	// from rev1's old CreationTimestamp.
	instanceA.Status.Experiment = &v2alpha1.ExperimentStatus{
		Phase: v2alpha1.ExperimentPhaseRunning,
	}

	revList := mustListRevisions(t, r, instanceA)
	// Ensure the baseline revision's timestamp is old enough to trigger timeout.
	for i := range revList {
		revList[i].CreationTimestamp = metav1.NewTime(time.Now().Add(-ExperimentDefaultTimeout - time.Minute))
	}

	newStatus := &v2alpha1.DatadogAgentStatus{Experiment: instanceA.Status.Experiment.DeepCopy()}
	require.NoError(t, r.manageExperiment(context.Background(), instanceA, newStatus, metav1.Now(), revList))
	// Abort does not fire — spec matches a known revision. Timeout fires instead
	// because the matching revision's timestamp exceeds the threshold.
	assert.Equal(t, v2alpha1.ExperimentPhaseTimeout, newStatus.Experiment.Phase)
}

func TestRollback_RestoresSpec(t *testing.T) {
	r, c := newRevisionTestReconciler(t)

	// Create a revision for specA.
	instanceA := newRevisionTestOwner("test-dda", "default")
	err := r.manageRevision(context.Background(), instanceA, mustListRevisions(t, r, instanceA), nil)
	require.NoError(t, err)

	revListA := mustListRevisions(t, r, instanceA)
	require.Len(t, revListA, 1)
	prevRevision := revListA[0].Name

	// Create a second revision for specB.
	instanceB := newRevisionTestOwner("test-dda", "default")
	instanceB.Spec = v2alpha1.DatadogAgentSpec{Global: &v2alpha1.GlobalConfig{}}
	err = r.manageRevision(context.Background(), instanceB, mustListRevisions(t, r, instanceB), nil)
	require.NoError(t, err)

	// rollback fetches the current DDA to compare specs; it must exist in the fake client.
	require.NoError(t, c.Create(context.Background(), instanceB))

	// Rollback from instanceB to prevRevision (specA).
	require.NoError(t, r.rollback(context.Background(), instanceB.ObjectMeta, prevRevision))
}

func TestRollback_NoPreviousRevision(t *testing.T) {
	r, _ := newRevisionTestReconciler(t)
	instance := newRevisionTestOwner("test-dda", "default")

	err := r.rollback(context.Background(), instance.ObjectMeta, "")
	require.NoError(t, err)
}

func TestHandleRollback_StoppedPhase(t *testing.T) {
	r, c := newRevisionTestReconciler(t)

	// Create two revisions so we have a previous to roll back to.
	instanceA := newRevisionTestOwner("test-dda", "default")
	err := r.manageRevision(context.Background(), instanceA, mustListRevisions(t, r, instanceA), nil)
	require.NoError(t, err)

	instanceB := newRevisionTestOwner("test-dda", "default")
	instanceB.Spec = v2alpha1.DatadogAgentSpec{Global: &v2alpha1.GlobalConfig{}}
	err = r.manageRevision(context.Background(), instanceB, mustListRevisions(t, r, instanceB), nil)
	require.NoError(t, err)

	instanceB.Status.Experiment = &v2alpha1.ExperimentStatus{
		Phase: v2alpha1.ExperimentPhaseStopped,
	}

	// rollback fetches the current DDA to compare specs; it must exist in the fake client.
	require.NoError(t, c.Create(context.Background(), instanceB))

	revList := mustListRevisions(t, r, instanceB)
	newStatus := &v2alpha1.DatadogAgentStatus{Experiment: instanceB.Status.Experiment.DeepCopy()}
	require.NoError(t, r.handleRollback(context.Background(), instanceB, newStatus, metav1.Now(), revList))
	require.NotNil(t, newStatus.Experiment)
	assert.Equal(t, v2alpha1.ExperimentPhaseRollback, newStatus.Experiment.Phase, "should ack stopped by setting phase to rollback")
}

func TestRollback_PreservesNonDatadogAnnotations(t *testing.T) {
	r, c := newRevisionTestReconciler(t)

	// Create revision for specA with a Datadog annotation.
	instanceA := newRevisionTestOwner("test-dda", "default")
	instanceA.Annotations = map[string]string{
		"some.datadoghq.com/key": "old-value",
	}
	require.NoError(t, r.manageRevision(context.Background(), instanceA, mustListRevisions(t, r, instanceA), nil))
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

	require.NoError(t, r.rollback(context.Background(), instanceB.ObjectMeta, prevRevision))

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
	require.NoError(t, r.manageRevision(context.Background(), instanceA, mustListRevisions(t, r, instanceA), nil))

	instanceB := newRevisionTestOwner("test-dda", "default")
	instanceB.Spec = v2alpha1.DatadogAgentSpec{Global: &v2alpha1.GlobalConfig{}}
	require.NoError(t, r.manageRevision(context.Background(), instanceB, mustListRevisions(t, r, instanceB), nil))

	newStatus := &v2alpha1.DatadogAgentStatus{Experiment: &v2alpha1.ExperimentStatus{Phase: v2alpha1.ExperimentPhaseStopped}}

	// rollback requires the DDA to exist in the fake client; don't create it so it errors.
	err := r.restorePreviousSpec(context.Background(), instanceB, newStatus, mustListRevisions(t, r, instanceB), v2alpha1.ExperimentPhaseRollback)
	require.Error(t, err)
	// Phase must NOT have been set since rollback failed.
	assert.Equal(t, v2alpha1.ExperimentPhaseStopped, newStatus.Experiment.Phase)

	// Now create the DDA so rollback can succeed.
	require.NoError(t, c.Create(context.Background(), instanceB))
	err = r.restorePreviousSpec(context.Background(), instanceB, newStatus, mustListRevisions(t, r, instanceB), v2alpha1.ExperimentPhaseRollback)
	require.NoError(t, err)
	assert.Equal(t, v2alpha1.ExperimentPhaseRollback, newStatus.Experiment.Phase)
}

func TestManageExperiment_TimeoutWinsOverSpuriousAbort(t *testing.T) {
	r, c := newRevisionTestReconciler(t)

	// Create two revisions so rollback has a target.
	instanceA := newRevisionTestOwner("test-dda", "default")
	require.NoError(t, r.manageRevision(context.Background(), instanceA, mustListRevisions(t, r, instanceA), nil))

	instanceB := newRevisionTestOwner("test-dda", "default")
	instanceB.Spec = v2alpha1.DatadogAgentSpec{Global: &v2alpha1.GlobalConfig{}}
	require.NoError(t, r.manageRevision(context.Background(), instanceB, mustListRevisions(t, r, instanceB), nil))
	require.NoError(t, c.Create(context.Background(), instanceB))

	// Simulate a post-409 reconcile: phase=running, the spec was manually changed
	// so it doesn't match any revision, AND timeout has elapsed. abortExperiment
	// would fire for the revision mismatch, but handleRollback must win and
	// persist phase=timeout.
	manualSite := "manual-change.example.com"
	instanceB.Spec = v2alpha1.DatadogAgentSpec{Global: &v2alpha1.GlobalConfig{Site: &manualSite}}
	instanceB.Status.Experiment = &v2alpha1.ExperimentStatus{
		Phase: v2alpha1.ExperimentPhaseRunning,
	}

	revList := mustListRevisions(t, r, instanceB)
	for i := range revList {
		if revList[i].Revision == 2 {
			revList[i].CreationTimestamp = metav1.NewTime(time.Now().Add(-ExperimentDefaultTimeout - time.Minute))
		}
	}

	newStatus := &v2alpha1.DatadogAgentStatus{Experiment: instanceB.Status.Experiment.DeepCopy()}
	require.NoError(t, r.manageExperiment(context.Background(), instanceB, newStatus, metav1.Now(), revList))
	assert.Equal(t, v2alpha1.ExperimentPhaseTimeout, newStatus.Experiment.Phase)
}

func TestHandleRollback_NoTimeoutOnFirstReconcile(t *testing.T) {
	r, c := newRevisionTestReconciler(t)

	// Only one revision exists — for the pre-experiment spec — with an old timestamp.
	instanceA := newRevisionTestOwner("test-dda", "default")
	require.NoError(t, r.manageRevision(context.Background(), instanceA, mustListRevisions(t, r, instanceA), nil))
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

// TestHandleRollback_PostRollbackSetsTimeout verifies the reconcile-2 scenario:
// the spec has already been restored to the pre-experiment value (e.g. by a
// previous reconcile whose status write 409'd), so phase is still running and
// the generation is mismatched. findMostRecentMatchingRevision finds the
// pre-experiment revision (spec matches), elapsed is large, idempotent rollback
// fires, and phase=timeout is set without a spec-update conflict.
func TestHandleRollback_PostRollbackSetsTimeout(t *testing.T) {
	r, c := newRevisionTestReconciler(t)

	// rev1: pre-experiment spec (instanceA).
	instanceA := newRevisionTestOwner("test-dda", "default")
	require.NoError(t, r.manageRevision(context.Background(), instanceA, mustListRevisions(t, r, instanceA), nil))

	// rev2: experiment spec (instanceB).
	instanceB := newRevisionTestOwner("test-dda", "default")
	instanceB.Spec = v2alpha1.DatadogAgentSpec{Global: &v2alpha1.GlobalConfig{}}
	require.NoError(t, r.manageRevision(context.Background(), instanceB, mustListRevisions(t, r, instanceB), nil))

	// The DDA in the cluster already has the rolled-back spec (instanceA's spec),
	// as if reconcile-1 restored it but its status write 409'd.
	instanceA.Status.Experiment = &v2alpha1.ExperimentStatus{
		Phase: v2alpha1.ExperimentPhaseRunning,
	}
	require.NoError(t, c.Create(context.Background(), instanceA))

	// rev1 has an old timestamp — it was created well before the experiment started.
	revList := mustListRevisions(t, r, instanceA)
	for i := range revList {
		if revList[i].Revision == 1 {
			revList[i].CreationTimestamp = metav1.NewTime(time.Now().Add(-ExperimentDefaultTimeout - time.Hour))
		}
	}

	newStatus := &v2alpha1.DatadogAgentStatus{Experiment: instanceA.Status.Experiment.DeepCopy()}
	require.NoError(t, r.handleRollback(context.Background(), instanceA, newStatus, metav1.Now(), revList))
	assert.Equal(t, v2alpha1.ExperimentPhaseTimeout, newStatus.Experiment.Phase)
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
	r, c := newRevisionTestReconciler(t)

	// Setup: create revisions for spec A (pre-experiment) and spec B (experiment).
	instanceA := newRevisionTestOwner("test-dda", "default")
	require.NoError(t, r.manageRevision(context.Background(), instanceA, mustListRevisions(t, r, instanceA), nil))

	instanceB := newRevisionTestOwner("test-dda", "default")
	instanceB.Spec = v2alpha1.DatadogAgentSpec{Global: &v2alpha1.GlobalConfig{}}
	require.NoError(t, r.manageRevision(context.Background(), instanceB, mustListRevisions(t, r, instanceB), nil))
	require.NoError(t, c.Create(context.Background(), instanceB))

	// Backdate rev2 (B) to simulate a long-running experiment whose revision
	// timestamp is well past the timeout threshold.
	revList := mustListRevisions(t, r, instanceB)
	for i := range revList {
		if revList[i].Revision == 2 {
			revList[i].CreationTimestamp = metav1.NewTime(time.Now().Add(-ExperimentDefaultTimeout - time.Hour))
		}
	}

	// Rollback: RC sets phase=Stopped; operator restores spec A.
	// restorePreviousSpec annotates the experiment revision (B) as rolled back.
	instanceB.Status.Experiment = &v2alpha1.ExperimentStatus{Phase: v2alpha1.ExperimentPhaseStopped}
	rollbackStatus := &v2alpha1.DatadogAgentStatus{Experiment: instanceB.Status.Experiment.DeepCopy()}
	require.NoError(t, r.handleRollback(context.Background(), instanceB, rollbackStatus, metav1.Now(), revList))
	require.Equal(t, v2alpha1.ExperimentPhaseRollback, rollbackStatus.Experiment.Phase)

	// Verify the experiment revision was annotated.
	remaining := mustListRevisions(t, r, instanceA)
	require.Len(t, remaining, 2, "both revisions should be kept (no aggressive GC)")
	var annotatedCount int
	for _, rev := range remaining {
		if rev.Annotations[annotationExperimentRollback] == "true" {
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
	_, err := r.ensureRevision(context.Background(), instanceB2, mustListRevisions(t, r, instanceB2), false)
	require.NoError(t, err)

	finalRevs := mustListRevisions(t, r, instanceB2)
	for _, rev := range finalRevs {
		assert.Empty(t, rev.Annotations[annotationExperimentRollback],
			"rollback annotation should be cleared after recreate")
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
	rev1Name, err := r.ensureRevision(context.Background(), instanceA, nil, false)
	require.NoError(t, err)

	instanceB := newRevisionTestOwner("test-dda", "default")
	instanceB.Spec = v2alpha1.DatadogAgentSpec{Global: &v2alpha1.GlobalConfig{}}
	rev2Name, err := r.ensureRevision(context.Background(), instanceB, mustListRevisions(t, r, instanceB), false)
	require.NoError(t, err)

	experimentSite := "datadoghq.eu"
	instanceC := newRevisionTestOwner("test-dda", "default")
	instanceC.Spec = v2alpha1.DatadogAgentSpec{Global: &v2alpha1.GlobalConfig{Site: &experimentSite}}
	rev3Name, err := r.ensureRevision(context.Background(), instanceC, mustListRevisions(t, r, instanceC), false)
	require.NoError(t, err)

	revList := mustListRevisions(t, r, instanceA)
	require.Len(t, revList, 3, "need 3 revisions to test this scenario")

	// rollback fetches the current DDA; create it with the experiment spec.
	require.NoError(t, c.Create(context.Background(), instanceC))

	// Trigger rollback via phase=stopped.
	instanceC.Status.Experiment = &v2alpha1.ExperimentStatus{Phase: v2alpha1.ExperimentPhaseStopped}
	newStatus := &v2alpha1.DatadogAgentStatus{Experiment: instanceC.Status.Experiment.DeepCopy()}
	require.NoError(t, r.restorePreviousSpec(context.Background(), instanceC, newStatus, revList, v2alpha1.ExperimentPhaseRollback))
	assert.Equal(t, v2alpha1.ExperimentPhaseRollback, newStatus.Experiment.Phase)

	// Verify: only rev3 (experiment, highest) is annotated.
	// Rev1 (old baseline) and rev2 (rollback target) must NOT be annotated.
	for _, rev := range mustListRevisions(t, r, instanceA) {
		hasAnnotation := rev.Annotations[annotationExperimentRollback] == "true"
		switch rev.Name {
		case rev3Name:
			assert.True(t, hasAnnotation, "rev3 (experiment, highest) should be annotated")
		case rev2Name:
			assert.False(t, hasAnnotation, "rev2 (rollback target) should NOT be annotated")
		case rev1Name:
			assert.False(t, hasAnnotation, "rev1 (old baseline) should NOT be annotated")
		}
	}
}

// TestAbortExperiment_ThreeRevisions_AnnotatesOnlyHighest verifies that when
// 3+ revisions exist and abort fires, only the highest-numbered revision (the
// experiment) is annotated — not older baselines.
func TestAbortExperiment_ThreeRevisions_AnnotatesOnlyHighest(t *testing.T) {
	r, _ := newRevisionTestReconciler(t)

	// Build 3 revisions using ensureRevision directly (bypasses GC).
	instanceA := newRevisionTestOwner("test-dda", "default")
	rev1Name, err := r.ensureRevision(context.Background(), instanceA, nil, false)
	require.NoError(t, err)

	instanceB := newRevisionTestOwner("test-dda", "default")
	instanceB.Spec = v2alpha1.DatadogAgentSpec{Global: &v2alpha1.GlobalConfig{}}
	rev2Name, err := r.ensureRevision(context.Background(), instanceB, mustListRevisions(t, r, instanceB), false)
	require.NoError(t, err)

	experimentSite := "datadoghq.eu"
	instanceC := newRevisionTestOwner("test-dda", "default")
	instanceC.Spec = v2alpha1.DatadogAgentSpec{Global: &v2alpha1.GlobalConfig{Site: &experimentSite}}
	rev3Name, err := r.ensureRevision(context.Background(), instanceC, mustListRevisions(t, r, instanceC), false)
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
		hasAnnotation := rev.Annotations[annotationExperimentRollback] == "true"
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
	require.NoError(t, r.manageRevision(context.Background(), instanceA, mustListRevisions(t, r, instanceA), nil))

	instanceB := newRevisionTestOwner("test-dda", "default")
	instanceB.Spec = v2alpha1.DatadogAgentSpec{Global: &v2alpha1.GlobalConfig{}}
	require.NoError(t, r.manageRevision(context.Background(), instanceB, mustListRevisions(t, r, instanceB), nil))

	// rollback fetches the current DDA to compare specs; it must exist in the fake client.
	require.NoError(t, c.Create(context.Background(), instanceB))

	instanceB.Status.Experiment = &v2alpha1.ExperimentStatus{
		Phase: v2alpha1.ExperimentPhaseRunning,
	}

	// Simulate the most recent revision having a creation timestamp past the timeout threshold
	// by modifying the in-memory revList before passing it to handleRollback.
	revList := mustListRevisions(t, r, instanceB)
	for i := range revList {
		if revList[i].Revision == 2 {
			revList[i].CreationTimestamp = metav1.NewTime(time.Now().Add(-ExperimentDefaultTimeout - time.Minute))
		}
	}

	newStatus := &v2alpha1.DatadogAgentStatus{Experiment: instanceB.Status.Experiment.DeepCopy()}
	require.NoError(t, r.handleRollback(context.Background(), instanceB, newStatus, metav1.Now(), revList))
	require.NotNil(t, newStatus.Experiment)
	assert.Equal(t, v2alpha1.ExperimentPhaseTimeout, newStatus.Experiment.Phase)
}
