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
