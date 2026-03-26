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
	err := r.manageRevision(context.Background(), instanceA, mustListRevisions(t, r, instanceA))
	require.NoError(t, err)

	revListA := mustListRevisions(t, r, instanceA)
	require.Len(t, revListA, 1)
	prevRevision := revListA[0].Name

	// Create a second revision for specB.
	instanceB := newRevisionTestOwner("test-dda", "default")
	instanceB.Spec = v2alpha1.DatadogAgentSpec{Global: &v2alpha1.GlobalConfig{}}
	err = r.manageRevision(context.Background(), instanceB, mustListRevisions(t, r, instanceB))
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
	err := r.manageRevision(context.Background(), instanceA, mustListRevisions(t, r, instanceA))
	require.NoError(t, err)

	instanceB := newRevisionTestOwner("test-dda", "default")
	instanceB.Spec = v2alpha1.DatadogAgentSpec{Global: &v2alpha1.GlobalConfig{}}
	err = r.manageRevision(context.Background(), instanceB, mustListRevisions(t, r, instanceB))
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

func TestHandleRollback_Timeout(t *testing.T) {
	r, c := newRevisionTestReconciler(t)

	// Create two revisions so rollback has a target.
	instanceA := newRevisionTestOwner("test-dda", "default")
	require.NoError(t, r.manageRevision(context.Background(), instanceA, mustListRevisions(t, r, instanceA)))

	instanceB := newRevisionTestOwner("test-dda", "default")
	instanceB.Spec = v2alpha1.DatadogAgentSpec{Global: &v2alpha1.GlobalConfig{}}
	require.NoError(t, r.manageRevision(context.Background(), instanceB, mustListRevisions(t, r, instanceB)))

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
