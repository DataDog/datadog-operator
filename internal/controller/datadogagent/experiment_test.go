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

	instance := newRevisionTestOwner("test-dda", "default")
	instance.Generation = 3
	instance.Spec = v2alpha1.DatadogAgentSpec{Global: &v2alpha1.GlobalConfig{}}
	instance.Status.Experiment = &v2alpha1.ExperimentStatus{
		Phase:      v2alpha1.ExperimentPhaseRunning,
		Generation: 2,
	}

	status := &v2alpha1.DatadogAgentStatus{
		Experiment: instance.Status.Experiment.DeepCopy(),
	}

	err := r.manageExperiment(context.Background(), instance, status, metav1.Now(), mustListRevisions(t, r, instance))
	require.NoError(t, err)
	require.NotNil(t, status.Experiment)
	assert.Equal(t, v2alpha1.ExperimentPhaseAborted, status.Experiment.Phase)
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
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no previous revision")
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
	err := r.restorePreviousSpec(context.Background(), instanceB.ObjectMeta, newStatus, mustListRevisions(t, r, instanceB), v2alpha1.ExperimentPhaseRollback)
	require.Error(t, err)
	// Phase must NOT have been set since rollback failed.
	assert.Equal(t, v2alpha1.ExperimentPhaseStopped, newStatus.Experiment.Phase)

	// Now create the DDA so rollback can succeed.
	require.NoError(t, c.Create(context.Background(), instanceB))
	err = r.restorePreviousSpec(context.Background(), instanceB.ObjectMeta, newStatus, mustListRevisions(t, r, instanceB), v2alpha1.ExperimentPhaseRollback)
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

	// Simulate a post-409 reconcile: phase=running, but generation was bumped by the
	// rollback spec update (instanceB.Generation != experiment.Generation), AND timeout
	// has elapsed. abortExperiment would fire for the generation mismatch, but
	// handleRollback must win and persist phase=timeout.
	instanceB.Generation = 99
	instanceB.Status.Experiment = &v2alpha1.ExperimentStatus{
		Phase:      v2alpha1.ExperimentPhaseRunning,
		Generation: 1,
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
	// Generation is set to a realistic non-zero value: RC would have recorded
	// instanceA.Generation when the experiment started.
	instanceA.Status.Experiment = &v2alpha1.ExperimentStatus{
		Phase:      v2alpha1.ExperimentPhaseRunning,
		Generation: instanceA.Generation,
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
