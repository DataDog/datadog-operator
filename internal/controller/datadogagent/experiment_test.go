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
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	v2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

// pinExpectedSpecHash stamps the expected-spec-hash annotation Fleet writes
// atomically with a start signal, computed over the instance's live spec. Start
// signals without it are rejected as baseline_missing, so any test exercising a
// successful start must pin it the way the daemon does.
func pinExpectedSpecHash(t *testing.T, instance *v2alpha1.DatadogAgent) {
	t.Helper()
	hash, err := v2alpha1.ComputeSpecHash(instance.Spec, instance.GetAnnotations())
	require.NoError(t, err)
	instance.Annotations[v2alpha1.AnnotationExperimentExpectedSpecHash] = hash
}

// seedOwnedRevision creates a ControllerRevision named name that satisfies the
// full ownedByDDA contract for instance, so processStartSignal's rollback-target
// validation resolves it instead of aborting with baseline_not_found.
func seedOwnedRevision(t *testing.T, c client.Client, instance *v2alpha1.DatadogAgent, name string) *appsv1.ControllerRevision {
	t.Helper()
	rev := &appsv1.ControllerRevision{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: instance.GetNamespace(),
			Labels:    map[string]string{apicommon.DatadogAgentNameLabelKey: instance.GetName()},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "datadoghq.com/v2alpha1",
				Kind:       "DatadogAgent",
				Name:       instance.GetName(),
				UID:        instance.GetUID(),
				Controller: ptr.To(true),
			}},
		},
		Revision: 1,
	}
	require.NoError(t, c.Create(context.Background(), rev))
	return rev
}

func TestManageExperiment_AbortsOnManualChange(t *testing.T) {
	r, _ := newRevisionTestReconciler(t)

	// Checkpoint recorded when the experiment started with specB.
	experimentSpec := v2alpha1.DatadogAgentSpec{Global: &v2alpha1.GlobalConfig{}}
	expectedHash, err := v2alpha1.ComputeSpecHash(experimentSpec, nil)
	require.NoError(t, err)

	// Simulate a manual spec change: the live spec doesn't match the
	// checkpointed hash. StartedAt is recent so the timeout path doesn't fire.
	manualSite := "manual-change.example.com"
	instanceC := newRevisionTestOwner("test-dda", "default")
	instanceC.Spec = v2alpha1.DatadogAgentSpec{Global: &v2alpha1.GlobalConfig{Site: &manualSite}}
	startedAt := metav1.Now()
	instanceC.Status.Experiment = &v2alpha1.ExperimentStatus{
		Phase:     v2alpha1.ExperimentPhaseRunning,
		StartedAt: &startedAt,
		Checkpoint: &v2alpha1.ExperimentCheckpoint{
			RollbackTargetRevision: "rev-baseline",
			ExpectedSpecHash:       expectedHash,
		},
	}

	status := &v2alpha1.DatadogAgentStatus{
		Experiment: instanceC.Status.Experiment.DeepCopy(),
	}

	_, err = r.manageExperiment(context.Background(), instanceC, status, metav1.Now())
	require.NoError(t, err)
	require.NotNil(t, status.Experiment)
	assert.Equal(t, v2alpha1.ExperimentPhaseAborted, status.Experiment.Phase)
	assert.Equal(t, v2alpha1.ExperimentTerminationReasonManualSpecChange, status.Experiment.TerminationReason)
}

// TestManageExperiment_ManualRevertToBaselineTerminatesViaTimeout verifies that
// when the user manually reverts the spec to the pre-experiment value during a
// running experiment, the experiment terminates via timeout rather than abort.
// The live spec's hash no longer matches the checkpoint's ExpectedSpecHash, but
// its snapshot matches the validated rollback target, so rollbackBlockedByManualChange
// does not block it — the timeout path fires because the elapsed time since
// Status.Experiment.StartedAt exceeds the threshold. The rollback is a no-op
// (spec already matches target), and the phase is set to "terminated" with
// terminationReason "timed_out".
func TestManageExperiment_ManualRevertToBaselineTerminatesViaTimeout(t *testing.T) {
	r, c := newRevisionTestReconciler(t)

	// Baseline revision (pre-experiment spec).
	instanceA := newRevisionTestOwner("test-dda", "default")
	require.NoError(t, r.manageRevision(context.Background(), instanceA, instanceA.Spec, mustListRevisions(t, r, instanceA), nil))
	revListA := mustListRevisions(t, r, instanceA)
	require.Len(t, revListA, 1)
	baselineRevision := revListA[0].Name

	// Checkpoint recorded when the experiment started with a different spec.
	experimentSpec := v2alpha1.DatadogAgentSpec{Global: &v2alpha1.GlobalConfig{}}
	expectedHash, err := v2alpha1.ComputeSpecHash(experimentSpec, nil)
	require.NoError(t, err)

	// User manually reverts the live spec to the baseline. handleRollback
	// detects timeout from a StartedAt past the timeout threshold and
	// completes the rollback since the live spec matches the validated target.
	startedAt := metav1.NewTime(time.Now().Add(-ExperimentDefaultTimeout - time.Minute))
	instanceA.Status.Experiment = &v2alpha1.ExperimentStatus{
		Phase:     v2alpha1.ExperimentPhaseRunning,
		StartedAt: &startedAt,
		Checkpoint: &v2alpha1.ExperimentCheckpoint{
			RollbackTargetRevision: baselineRevision,
			ExpectedSpecHash:       expectedHash,
		},
	}
	require.NoError(t, c.Create(context.Background(), instanceA))

	newStatus := &v2alpha1.DatadogAgentStatus{Experiment: instanceA.Status.Experiment.DeepCopy()}
	_, err = r.manageExperiment(context.Background(), instanceA, newStatus, metav1.Now())
	require.NoError(t, err)
	assert.Equal(t, v2alpha1.ExperimentPhaseTerminated, newStatus.Experiment.Phase)
	assert.Equal(t, v2alpha1.ExperimentTerminationReasonTimedOut, newStatus.Experiment.TerminationReason)
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
	updated, err := r.rollback(context.Background(), instanceB, prevRevision)
	require.NoError(t, err)
	assert.True(t, updated)
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

	updated, err := r.rollback(context.Background(), current, target)
	require.NoError(t, err)
	assert.False(t, updated)

	got := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Namespace: "default", Name: "test-dda"}, got))
	assert.Equal(t, rvBefore, got.ResourceVersion,
		"rollback must skip the Update when the raw spec already matches the target revision")
}

func TestRollback_NoPreviousRevision(t *testing.T) {
	r, _ := newRevisionTestReconciler(t)
	instance := newRevisionTestOwner("test-dda", "default")

	updated, err := r.rollback(context.Background(), instance, "")
	require.NoError(t, err)
	assert.False(t, updated)
}

func TestRollback_NoOwnedRevision(t *testing.T) {
	r, _ := newRevisionTestReconciler(t)
	instance := newRevisionTestOwner("test-dda", "default")

	_, err := r.rollback(context.Background(), instance, "does-not-exist")
	require.ErrorIs(t, err, errBaselineNotFound)
}

func TestProcessExperimentSignal_RollbackSignalRollsBack(t *testing.T) {
	r, c := newRevisionTestReconciler(t)

	// Create two revisions so we have a previous to roll back to.
	instanceA := newRevisionTestOwner("test-dda", "default")
	require.NoError(t, r.manageRevision(context.Background(), instanceA, instanceA.Spec, mustListRevisions(t, r, instanceA), nil))
	revListA := mustListRevisions(t, r, instanceA)
	require.Len(t, revListA, 1)
	baselineRevision := revListA[0].Name

	instanceB := newRevisionTestOwner("test-dda", "default")
	instanceB.Spec = v2alpha1.DatadogAgentSpec{Global: &v2alpha1.GlobalConfig{}}
	require.NoError(t, r.manageRevision(context.Background(), instanceB, instanceB.Spec, mustListRevisions(t, r, instanceB), nil))

	expectedHash, err := v2alpha1.ComputeSpecHash(instanceB.Spec, instanceB.GetAnnotations())
	require.NoError(t, err)

	// Set annotations to signal rollback (different task ID), and status to running
	// with a checkpoint recorded when the experiment started.
	instanceB.Annotations = map[string]string{
		v2alpha1.AnnotationExperimentSignal: v2alpha1.ExperimentSignalRollback,
		v2alpha1.AnnotationExperimentID:     "stop-1",
	}
	instanceB.Status.Experiment = &v2alpha1.ExperimentStatus{
		Phase: v2alpha1.ExperimentPhaseRunning,
		ID:    "exp-1",
		Checkpoint: &v2alpha1.ExperimentCheckpoint{
			RollbackTargetRevision: baselineRevision,
			ExpectedSpecHash:       expectedHash,
		},
	}

	// rollback fetches the current DDA to compare specs; it must exist in the fake client.
	require.NoError(t, c.Create(context.Background(), instanceB))

	newStatus := &v2alpha1.DatadogAgentStatus{Experiment: instanceB.Status.Experiment.DeepCopy()}
	_, specUpdated, processErr := r.processExperimentSignal(context.Background(), instanceB, newStatus, metav1.Now())
	require.NoError(t, processErr)
	require.NotNil(t, newStatus.Experiment)
	assert.True(t, specUpdated)
	assert.Equal(t, v2alpha1.ExperimentPhaseTerminated, newStatus.Experiment.Phase, "rollback signal should trigger terminated phase")
	assert.Equal(t, v2alpha1.ExperimentTerminationReasonStopped, newStatus.Experiment.TerminationReason)
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

	_, err := r.rollback(context.Background(), instanceB, prevRevision)
	require.NoError(t, err)

	updated := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Namespace: "default", Name: "test-dda"}, updated))

	// Datadog annotation should be restored to the snapshot value.
	assert.Equal(t, "old-value", updated.Annotations["some.datadoghq.com/key"])
	// Non-Datadog annotation must be preserved.
	assert.Equal(t, "keep-me", updated.Annotations["user-tooling/key"])
}

func TestRestorePreviousSpec_PhaseSetOnlyOnSuccess(t *testing.T) {
	r, c := newRevisionTestReconciler(t)

	// Create a revision so rollback has a target.
	instanceA := newRevisionTestOwner("test-dda", "default")
	require.NoError(t, r.manageRevision(context.Background(), instanceA, instanceA.Spec, mustListRevisions(t, r, instanceA), nil))
	revListA := mustListRevisions(t, r, instanceA)
	require.Len(t, revListA, 1)
	target := revListA[0].Name

	instanceB := newRevisionTestOwner("test-dda", "default")
	instanceB.Spec = v2alpha1.DatadogAgentSpec{Global: &v2alpha1.GlobalConfig{}}

	newStatus := &v2alpha1.DatadogAgentStatus{Experiment: &v2alpha1.ExperimentStatus{
		Phase:      v2alpha1.ExperimentPhaseRunning,
		Checkpoint: &v2alpha1.ExperimentCheckpoint{RollbackTargetRevision: target},
	}}

	// rollback requires the DDA to exist in the fake client; don't create it so it errors.
	_, err := r.restorePreviousSpec(context.Background(), instanceB, newStatus, v2alpha1.ExperimentTerminationReasonStopped)
	require.Error(t, err)
	// Phase must NOT have been set since rollback failed.
	assert.Equal(t, v2alpha1.ExperimentPhaseRunning, newStatus.Experiment.Phase)

	// Now create the DDA so rollback can succeed.
	require.NoError(t, c.Create(context.Background(), instanceB))
	_, err = r.restorePreviousSpec(context.Background(), instanceB, newStatus, v2alpha1.ExperimentTerminationReasonStopped)
	require.NoError(t, err)
	assert.Equal(t, v2alpha1.ExperimentPhaseTerminated, newStatus.Experiment.Phase)
	assert.Equal(t, v2alpha1.ExperimentTerminationReasonStopped, newStatus.Experiment.TerminationReason)
}

// TestRestorePreviousSpec_BaselineNotFoundAborts verifies that when the
// checkpointed rollback target cannot be validated (absent or not owned by
// this DDA), restorePreviousSpec aborts with baseline_not_found instead of
// propagating a hard error that would just requeue.
func TestRestorePreviousSpec_BaselineNotFoundAborts(t *testing.T) {
	r, c := newRevisionTestReconciler(t)

	instance := newRevisionTestOwner("test-dda", "default")
	require.NoError(t, c.Create(context.Background(), instance))

	newStatus := &v2alpha1.DatadogAgentStatus{Experiment: &v2alpha1.ExperimentStatus{
		Phase:      v2alpha1.ExperimentPhaseRunning,
		Checkpoint: &v2alpha1.ExperimentCheckpoint{RollbackTargetRevision: "does-not-exist"},
	}}

	updated, err := r.restorePreviousSpec(context.Background(), instance, newStatus, v2alpha1.ExperimentTerminationReasonStopped)
	require.NoError(t, err)
	assert.False(t, updated)
	assert.Equal(t, v2alpha1.ExperimentPhaseAborted, newStatus.Experiment.Phase)
	assert.Equal(t, v2alpha1.ExperimentTerminationReasonBaselineNotFound, newStatus.Experiment.TerminationReason)
}

func TestManageExperiment_ManualChangeAbortsInsteadOfTimeout(t *testing.T) {
	r, c := newRevisionTestReconciler(t)

	// Baseline revision.
	instanceA := newRevisionTestOwner("test-dda", "default")
	require.NoError(t, r.manageRevision(context.Background(), instanceA, instanceA.Spec, mustListRevisions(t, r, instanceA), nil))
	revListA := mustListRevisions(t, r, instanceA)
	require.Len(t, revListA, 1)
	baselineRevision := revListA[0].Name

	expectedHash, err := v2alpha1.ComputeSpecHash(v2alpha1.DatadogAgentSpec{Global: &v2alpha1.GlobalConfig{}}, nil)
	require.NoError(t, err)

	// Live spec is neither the experiment spec (hash mismatch) nor the
	// baseline (content mismatch) — a genuine manual change. Timeout has
	// also elapsed, but the manual change must take precedence.
	manualSite := "manual-change.example.com"
	instanceD := newRevisionTestOwner("test-dda", "default")
	instanceD.Spec = v2alpha1.DatadogAgentSpec{Global: &v2alpha1.GlobalConfig{Site: &manualSite}}
	startedAt := metav1.NewTime(time.Now().Add(-ExperimentDefaultTimeout - time.Minute))
	instanceD.Status.Experiment = &v2alpha1.ExperimentStatus{
		Phase:     v2alpha1.ExperimentPhaseRunning,
		StartedAt: &startedAt,
		Checkpoint: &v2alpha1.ExperimentCheckpoint{
			RollbackTargetRevision: baselineRevision,
			ExpectedSpecHash:       expectedHash,
		},
	}
	require.NoError(t, c.Create(context.Background(), instanceD))

	newStatus := &v2alpha1.DatadogAgentStatus{Experiment: instanceD.Status.Experiment.DeepCopy()}
	_, err = r.manageExperiment(context.Background(), instanceD, newStatus, metav1.Now())
	require.NoError(t, err)
	assert.Equal(t, v2alpha1.ExperimentPhaseAborted, newStatus.Experiment.Phase)
	assert.Equal(t, v2alpha1.ExperimentTerminationReasonManualSpecChange, newStatus.Experiment.TerminationReason)
}

func TestHandleRollback_NoTimeoutOnFirstReconcile(t *testing.T) {
	r, c := newRevisionTestReconciler(t)

	instanceB := newRevisionTestOwner("test-dda", "default")
	instanceB.Spec = v2alpha1.DatadogAgentSpec{Global: &v2alpha1.GlobalConfig{}}
	startedAt := metav1.Now()
	instanceB.Status.Experiment = &v2alpha1.ExperimentStatus{
		Phase:     v2alpha1.ExperimentPhaseRunning,
		StartedAt: &startedAt,
		Checkpoint: &v2alpha1.ExperimentCheckpoint{
			RollbackTargetRevision: "rev-baseline",
			ExpectedSpecHash:       "does-not-matter",
		},
	}
	require.NoError(t, c.Create(context.Background(), instanceB))

	newStatus := &v2alpha1.DatadogAgentStatus{Experiment: instanceB.Status.Experiment.DeepCopy()}
	_, err := r.handleRollback(context.Background(), instanceB, newStatus, metav1.Now())
	require.NoError(t, err)
	assert.Equal(t, v2alpha1.ExperimentPhaseRunning, newStatus.Experiment.Phase)
}

// TestHandleRollback_PostRollbackSetsTerminated verifies the reconcile-2 scenario:
// the spec has already been restored to the pre-experiment value (e.g. by a
// previous reconcile whose status write 409'd), so phase is still running.
// elapsed is large, the idempotent rollback fires, and
// phase=terminated with terminationReason=timed_out is set without a
// spec-update conflict.
func TestHandleRollback_PostRollbackSetsTerminated(t *testing.T) {
	r, c := newRevisionTestReconciler(t)

	// Baseline revision.
	instanceA := newRevisionTestOwner("test-dda", "default")
	require.NoError(t, r.manageRevision(context.Background(), instanceA, instanceA.Spec, mustListRevisions(t, r, instanceA), nil))
	revListA := mustListRevisions(t, r, instanceA)
	require.Len(t, revListA, 1)
	baselineRevision := revListA[0].Name

	expectedHash, err := v2alpha1.ComputeSpecHash(v2alpha1.DatadogAgentSpec{Global: &v2alpha1.GlobalConfig{}}, nil)
	require.NoError(t, err)

	// The DDA in the cluster already has the rolled-back spec (baseline),
	// as if a previous reconcile restored it but its status write 409'd.
	// StartedAt sits past the timeout threshold so handleRollback fires the
	// idempotent rollback.
	startedAt := metav1.NewTime(time.Now().Add(-ExperimentDefaultTimeout - time.Hour))
	instanceA.Status.Experiment = &v2alpha1.ExperimentStatus{
		Phase:     v2alpha1.ExperimentPhaseRunning,
		StartedAt: &startedAt,
		Checkpoint: &v2alpha1.ExperimentCheckpoint{
			RollbackTargetRevision: baselineRevision,
			ExpectedSpecHash:       expectedHash,
		},
	}
	require.NoError(t, c.Create(context.Background(), instanceA))

	newStatus := &v2alpha1.DatadogAgentStatus{Experiment: instanceA.Status.Experiment.DeepCopy()}
	updated, err := r.handleRollback(context.Background(), instanceA, newStatus, metav1.Now())
	require.NoError(t, err)
	assert.False(t, updated, "idempotent rollback should not perform an Update")
	assert.Equal(t, v2alpha1.ExperimentPhaseTerminated, newStatus.Experiment.Phase)
	assert.Equal(t, v2alpha1.ExperimentTerminationReasonTimedOut, newStatus.Experiment.TerminationReason)
}

func TestHandleRollback_Timeout(t *testing.T) {
	r, c := newRevisionTestReconciler(t)

	// Baseline revision.
	instanceA := newRevisionTestOwner("test-dda", "default")
	require.NoError(t, r.manageRevision(context.Background(), instanceA, instanceA.Spec, mustListRevisions(t, r, instanceA), nil))
	revListA := mustListRevisions(t, r, instanceA)
	require.Len(t, revListA, 1)
	baselineRevision := revListA[0].Name

	instanceB := newRevisionTestOwner("test-dda", "default")
	instanceB.Spec = v2alpha1.DatadogAgentSpec{Global: &v2alpha1.GlobalConfig{}}
	expectedHash, err := v2alpha1.ComputeSpecHash(instanceB.Spec, instanceB.GetAnnotations())
	require.NoError(t, err)

	// rollback fetches the current DDA to compare specs; it must exist in the fake client.
	require.NoError(t, c.Create(context.Background(), instanceB))

	// StartedAt past the timeout threshold triggers the rollback path. Live
	// spec still matches the experiment's checkpointed hash — no manual
	// change, so timeout rolls back normally.
	startedAt := metav1.NewTime(time.Now().Add(-ExperimentDefaultTimeout - time.Minute))
	instanceB.Status.Experiment = &v2alpha1.ExperimentStatus{
		Phase:     v2alpha1.ExperimentPhaseRunning,
		StartedAt: &startedAt,
		Checkpoint: &v2alpha1.ExperimentCheckpoint{
			RollbackTargetRevision: baselineRevision,
			ExpectedSpecHash:       expectedHash,
		},
	}

	newStatus := &v2alpha1.DatadogAgentStatus{Experiment: instanceB.Status.Experiment.DeepCopy()}
	updated, err := r.handleRollback(context.Background(), instanceB, newStatus, metav1.Now())
	require.NoError(t, err)
	assert.True(t, updated)
	require.NotNil(t, newStatus.Experiment)
	assert.Equal(t, v2alpha1.ExperimentPhaseTerminated, newStatus.Experiment.Phase)
	assert.Equal(t, v2alpha1.ExperimentTerminationReasonTimedOut, newStatus.Experiment.TerminationReason)
}

// --- processExperimentSignal tests ---

func TestProcessExperimentSignal_StartNewExperiment(t *testing.T) {
	r, c := newRevisionTestReconciler(t)
	instance := newRevisionTestOwner("test-dda", "default")
	instance.Annotations = map[string]string{
		v2alpha1.AnnotationExperimentSignal:                 v2alpha1.ExperimentSignalStart,
		v2alpha1.AnnotationExperimentID:                     "exp-new",
		v2alpha1.AnnotationExperimentRollbackTargetRevision: "rev-baseline",
	}
	pinExpectedSpecHash(t, instance)
	seedOwnedRevision(t, c, instance, "rev-baseline")

	newStatus := &v2alpha1.DatadogAgentStatus{}
	_, _, processErr := r.processExperimentSignal(context.Background(), instance, newStatus, metav1.Now())
	require.NoError(t, processErr)
	require.NotNil(t, newStatus.Experiment)
	assert.Equal(t, v2alpha1.ExperimentPhaseRunning, newStatus.Experiment.Phase)
	assert.Equal(t, "exp-new", newStatus.Experiment.ID)
	require.NotNil(t, newStatus.Experiment.Checkpoint)
	assert.Equal(t, "rev-baseline", newStatus.Experiment.Checkpoint.RollbackTargetRevision)
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
	_, _, processErr := r.processExperimentSignal(context.Background(), instance, newStatus, metav1.Now())
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
	_, _, processErr := r.processExperimentSignal(context.Background(), instance, newStatus, metav1.Now())
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

	expectedHash, err := v2alpha1.ComputeSpecHash(instance.Spec, instance.GetAnnotations())
	require.NoError(t, err)

	instance.Annotations = map[string]string{
		v2alpha1.AnnotationExperimentSignal: v2alpha1.ExperimentSignalRollback,
		v2alpha1.AnnotationExperimentID:     "stop-task-id",
	}
	instance.Status.Experiment = &v2alpha1.ExperimentStatus{
		Phase:      v2alpha1.ExperimentPhaseRunning,
		ID:         "exp-1",
		Checkpoint: &v2alpha1.ExperimentCheckpoint{ExpectedSpecHash: expectedHash},
	}

	newStatus := &v2alpha1.DatadogAgentStatus{Experiment: instance.Status.Experiment.DeepCopy()}
	_, _, processErr := r.processExperimentSignal(context.Background(), instance, newStatus, metav1.Now())
	require.NoError(t, processErr)
	// Rollback proceeds despite different annotation ID.
	assert.Equal(t, v2alpha1.ExperimentPhaseTerminated, newStatus.Experiment.Phase)
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
	_, _, processErr := r.processExperimentSignal(context.Background(), instance, newStatus, metav1.Now())
	require.NoError(t, processErr)
	// Already terminal — no-op.
	assert.Equal(t, v2alpha1.ExperimentPhaseTerminated, newStatus.Experiment.Phase)
}

func TestProcessExperimentSignal_PromoteRunning(t *testing.T) {
	r, _ := newRevisionTestReconciler(t)

	instance := newRevisionTestOwner("test-dda", "default")
	expectedHash, err := v2alpha1.ComputeSpecHash(instance.Spec, instance.GetAnnotations())
	require.NoError(t, err)

	// Promote signal has its own task ID, different from the start experiment ID.
	instance.Annotations = map[string]string{
		v2alpha1.AnnotationExperimentSignal: v2alpha1.ExperimentSignalPromote,
		v2alpha1.AnnotationExperimentID:     "promote-1",
	}
	instance.Status.Experiment = &v2alpha1.ExperimentStatus{
		Phase:      v2alpha1.ExperimentPhaseRunning,
		ID:         "exp-1",
		Checkpoint: &v2alpha1.ExperimentCheckpoint{ExpectedSpecHash: expectedHash},
	}

	newStatus := &v2alpha1.DatadogAgentStatus{Experiment: instance.Status.Experiment.DeepCopy()}
	_, _, processErr := r.processExperimentSignal(context.Background(), instance, newStatus, metav1.Now())
	require.NoError(t, processErr)
	assert.Equal(t, v2alpha1.ExperimentPhasePromoted, newStatus.Experiment.Phase)
}

func TestProcessExperimentSignal_PromoteBeatsTimeout(t *testing.T) {
	r, c := newRevisionTestReconciler(t)

	instanceB := newRevisionTestOwner("test-dda", "default")
	instanceB.Spec = v2alpha1.DatadogAgentSpec{Global: &v2alpha1.GlobalConfig{}}
	expectedHash, err := v2alpha1.ComputeSpecHash(instanceB.Spec, instanceB.GetAnnotations())
	require.NoError(t, err)
	require.NoError(t, c.Create(context.Background(), instanceB))

	// Set promote annotation (different task ID) and running phase with timeout elapsed.
	instanceB.Annotations = map[string]string{
		v2alpha1.AnnotationExperimentSignal: v2alpha1.ExperimentSignalPromote,
		v2alpha1.AnnotationExperimentID:     "promote-1",
	}
	startedAt := metav1.NewTime(time.Now().Add(-ExperimentDefaultTimeout - time.Minute))
	instanceB.Status.Experiment = &v2alpha1.ExperimentStatus{
		Phase:      v2alpha1.ExperimentPhaseRunning,
		ID:         "exp-1",
		StartedAt:  &startedAt,
		Checkpoint: &v2alpha1.ExperimentCheckpoint{ExpectedSpecHash: expectedHash},
	}

	newStatus := &v2alpha1.DatadogAgentStatus{Experiment: instanceB.Status.Experiment.DeepCopy()}
	// Run the full manageExperiment flow — processExperimentSignal runs first,
	// sets promoted, then handleRollback sees the phase change and skips.
	_, err = r.manageExperiment(context.Background(), instanceB, newStatus, metav1.Now())
	require.NoError(t, err)
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
	_, _, processErr := r.processExperimentSignal(context.Background(), instance, newStatus, metav1.Now())
	require.NoError(t, processErr)
	// No annotations — no change.
	assert.Equal(t, v2alpha1.ExperimentPhaseRunning, newStatus.Experiment.Phase)
}

func TestProcessExperimentSignal_RollbackBeatsTimeout(t *testing.T) {
	r, c := newRevisionTestReconciler(t)

	// Baseline revision.
	instanceA := newRevisionTestOwner("test-dda", "default")
	require.NoError(t, r.manageRevision(context.Background(), instanceA, instanceA.Spec, mustListRevisions(t, r, instanceA), nil))
	revListA := mustListRevisions(t, r, instanceA)
	require.Len(t, revListA, 1)
	baselineRevision := revListA[0].Name

	instanceB := newRevisionTestOwner("test-dda", "default")
	instanceB.Spec = v2alpha1.DatadogAgentSpec{Global: &v2alpha1.GlobalConfig{}}
	expectedHash, err := v2alpha1.ComputeSpecHash(instanceB.Spec, instanceB.GetAnnotations())
	require.NoError(t, err)
	require.NoError(t, c.Create(context.Background(), instanceB))

	// Set rollback annotation (different task ID) and running phase with timeout elapsed.
	instanceB.Annotations = map[string]string{
		v2alpha1.AnnotationExperimentSignal: v2alpha1.ExperimentSignalRollback,
		v2alpha1.AnnotationExperimentID:     "stop-1",
	}
	startedAt := metav1.NewTime(time.Now().Add(-ExperimentDefaultTimeout - time.Minute))
	instanceB.Status.Experiment = &v2alpha1.ExperimentStatus{
		Phase:     v2alpha1.ExperimentPhaseRunning,
		ID:        "exp-1",
		StartedAt: &startedAt,
		Checkpoint: &v2alpha1.ExperimentCheckpoint{
			RollbackTargetRevision: baselineRevision,
			ExpectedSpecHash:       expectedHash,
		},
	}

	newStatus := &v2alpha1.DatadogAgentStatus{Experiment: instanceB.Status.Experiment.DeepCopy()}
	_, err = r.manageExperiment(context.Background(), instanceB, newStatus, metav1.Now())
	require.NoError(t, err)
	assert.Equal(t, v2alpha1.ExperimentPhaseTerminated, newStatus.Experiment.Phase, "rollback should beat timeout")
	assert.Equal(t, v2alpha1.ExperimentTerminationReasonStopped, newStatus.Experiment.TerminationReason)
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
		v2alpha1.AnnotationExperimentRollbackTargetRevision: "rev-baseline",
	}
	pinExpectedSpecHash(t, instance)
	seedOwnedRevision(t, c, instance, "rev-baseline")
	require.NoError(t, c.Create(context.Background(), instance))

	newStatus := &v2alpha1.DatadogAgentStatus{}
	_, err := r.manageExperiment(context.Background(), instance, newStatus, metav1.Now())
	require.NoError(t, err)

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
	_, err := r.manageExperiment(context.Background(), instance, newStatus, metav1.Now())
	require.NoError(t, err)

	// Annotations should have been cleared.
	got := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "test-dda", Namespace: "default"}, got))
	assert.Empty(t, got.Annotations[v2alpha1.AnnotationExperimentSignal], "no-op signal should be cleared")
	assert.Empty(t, got.Annotations[v2alpha1.AnnotationExperimentID], "no-op signal ID should be cleared")
}

// TestManageExperiment_InFlightWithoutCheckpointAborts verifies that a running
// experiment with a nil checkpoint (carryover state predating the checkpoint
// contract, or a status write that never completed) is aborted immediately
// with baseline_missing before any signal processing, rather than letting the
// checkpoint-dependent logic downstream nil-dereference.
func TestManageExperiment_InFlightWithoutCheckpointAborts(t *testing.T) {
	r, _ := newRevisionTestReconciler(t)
	instance := newRevisionTestOwner("test-dda", "default")
	instance.Status.Experiment = &v2alpha1.ExperimentStatus{
		Phase: v2alpha1.ExperimentPhaseRunning,
		ID:    "exp-1",
	}

	newStatus := &v2alpha1.DatadogAgentStatus{Experiment: instance.Status.Experiment.DeepCopy()}
	specUpdated, err := r.manageExperiment(context.Background(), instance, newStatus, metav1.Now())
	require.NoError(t, err)
	assert.False(t, specUpdated)
	require.NotNil(t, newStatus.Experiment)
	assert.Equal(t, v2alpha1.ExperimentPhaseAborted, newStatus.Experiment.Phase)
	assert.Equal(t, v2alpha1.ExperimentTerminationReasonBaselineMissing, newStatus.Experiment.TerminationReason)
}

// TestHandleRollback_StartedAt_AnchorsTimeout verifies that handleRollback
// measures elapsed time against Status.Experiment.StartedAt, independent of
// any ControllerRevision metadata.
func TestHandleRollback_StartedAt_AnchorsTimeout(t *testing.T) {
	r, _ := newRevisionTestReconciler(t)

	// Build a baseline revision.
	instance := newRevisionTestOwner("test-dda", "default")
	require.NoError(t, r.manageRevision(context.Background(), instance, instance.Spec, mustListRevisions(t, r, instance), nil))
	revList := mustListRevisions(t, r, instance)
	require.Len(t, revList, 1)
	baselineRevision := revList[0].Name

	expectedHash, err := v2alpha1.ComputeSpecHash(instance.Spec, instance.GetAnnotations())
	require.NoError(t, err)

	// Experiment just started (StartedAt = now), Phase=Running, ID set.
	startedAt := metav1.NewTime(time.Now().Add(-1 * time.Second))
	instance.Status.Experiment = &v2alpha1.ExperimentStatus{
		Phase:     v2alpha1.ExperimentPhaseRunning,
		ID:        "exp-1",
		StartedAt: &startedAt,
		Checkpoint: &v2alpha1.ExperimentCheckpoint{
			RollbackTargetRevision: baselineRevision,
			ExpectedSpecHash:       expectedHash,
		},
	}

	newStatus := &v2alpha1.DatadogAgentStatus{Experiment: instance.Status.Experiment.DeepCopy()}
	_, err = r.handleRollback(context.Background(), instance, newStatus, metav1.Now())
	require.NoError(t, err)
	// Phase must still be Running — the experiment only just started.
	require.NotNil(t, newStatus.Experiment)
	assert.Equal(t, v2alpha1.ExperimentPhaseRunning, newStatus.Experiment.Phase)
}

// TestProcessStartSignal_CapturesStartTaskID verifies that the daemon's
// pending-task-id annotation is captured into Status.Experiment.StartTaskID
// on the Running transition. Without this, the daemon cannot later report
// TaskState_ERROR for the original start task on local timeout.
func TestProcessStartSignal_CapturesStartTaskID(t *testing.T) {
	r, c := newRevisionTestReconciler(t)

	const taskID = "task-uuid-abc-123"
	const expID = "exp-new"
	instance := newRevisionTestOwner("test-dda", "default")
	instance.Annotations = map[string]string{
		v2alpha1.AnnotationExperimentSignal:                 v2alpha1.ExperimentSignalStart,
		v2alpha1.AnnotationExperimentID:                     expID,
		v2alpha1.AnnotationPendingTaskID:                    taskID,
		v2alpha1.AnnotationPendingAction:                    "start",
		v2alpha1.AnnotationExperimentRollbackTargetRevision: "rev-baseline",
	}
	pinExpectedSpecHash(t, instance)
	seedOwnedRevision(t, c, instance, "rev-baseline")

	newStatus := &v2alpha1.DatadogAgentStatus{}
	_, _, processErr := r.processExperimentSignal(context.Background(), instance, newStatus, metav1.Now())
	require.NoError(t, processErr)
	require.NotNil(t, newStatus.Experiment)
	assert.Equal(t, v2alpha1.ExperimentPhaseRunning, newStatus.Experiment.Phase)
	assert.Equal(t, expID, newStatus.Experiment.ID)
	assert.Equal(t, taskID, newStatus.Experiment.StartTaskID,
		"start task ID must be captured from the pending annotation so it survives "+
			"daemon restarts and is available to report timeout errors")
}

// TestProcessRollbackSignal_NilPhaseWithAnnotationRecovers verifies Transition
// 6: a rollback signal arrives while status.experiment.phase=="" (a start
// signal whose spec patch landed but whose status write never completed).
// processRollbackSignal reconstructs the checkpoint from the validated
// rollback-target annotation and restores the baseline.
func TestProcessRollbackSignal_NilPhaseWithAnnotationRecovers(t *testing.T) {
	r, c := newRevisionTestReconciler(t)

	// Baseline revision.
	instanceA := newRevisionTestOwner("test-dda", "default")
	require.NoError(t, r.manageRevision(context.Background(), instanceA, instanceA.Spec, mustListRevisions(t, r, instanceA), nil))
	revListA := mustListRevisions(t, r, instanceA)
	require.Len(t, revListA, 1)
	baselineRevision := revListA[0].Name

	// Live DDA has the experiment spec applied, with the rollback-target
	// annotation still present (from the interrupted start) and a rollback
	// signal now arriving, but status.experiment is nil (phase=="").
	instance := newRevisionTestOwner("test-dda", "default")
	instance.Spec = v2alpha1.DatadogAgentSpec{Global: &v2alpha1.GlobalConfig{}}
	instance.Annotations = map[string]string{
		v2alpha1.AnnotationExperimentSignal:                 v2alpha1.ExperimentSignalRollback,
		v2alpha1.AnnotationExperimentID:                     "stop-1",
		v2alpha1.AnnotationExperimentRollbackTargetRevision: baselineRevision,
	}
	pinExpectedSpecHash(t, instance)
	require.NoError(t, c.Create(context.Background(), instance))

	newStatus := &v2alpha1.DatadogAgentStatus{}
	_, specUpdated, err := r.processExperimentSignal(context.Background(), instance, newStatus, metav1.Now())
	require.NoError(t, err)
	require.NotNil(t, newStatus.Experiment)
	assert.True(t, specUpdated)
	assert.Equal(t, v2alpha1.ExperimentPhaseTerminated, newStatus.Experiment.Phase)
	assert.Equal(t, v2alpha1.ExperimentTerminationReasonStopped, newStatus.Experiment.TerminationReason)
}

// TestProcessRollbackSignal_NilPhaseWithoutAnnotationNoOps verifies that a
// rollback signal arriving at phase=="" with no rollback-target-revision
// annotation (nothing to validate, nothing to roll back to) is a clean no-op
// rather than an error.
func TestProcessRollbackSignal_NilPhaseWithoutAnnotationNoOps(t *testing.T) {
	r, _ := newRevisionTestReconciler(t)
	instance := newRevisionTestOwner("test-dda", "default")
	instance.Annotations = map[string]string{
		v2alpha1.AnnotationExperimentSignal: v2alpha1.ExperimentSignalRollback,
		v2alpha1.AnnotationExperimentID:     "stop-1",
	}

	newStatus := &v2alpha1.DatadogAgentStatus{}
	_, specUpdated, err := r.processExperimentSignal(context.Background(), instance, newStatus, metav1.Now())
	require.NoError(t, err)
	assert.False(t, specUpdated)
	assert.Nil(t, newStatus.Experiment)
}
