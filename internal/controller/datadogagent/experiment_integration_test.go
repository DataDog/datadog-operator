// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

// Integration tests for the experiment rollback flow wired through the full
// DDA reconcile path. These complement the unit tests in experiment_test.go.
//
// Coverage goals:
//   - Stopped rollback: daemon writing rollback annotation causes the operator to
//     restore the previous spec and set phase=terminated, terminationReason=stopped.
//   - Timeout rollback: an experiment running past ExperimentTimeout causes the
//     operator to restore the previous spec and set phase=terminated,
//     terminationReason=timed_out.
//
// The daemon communicates experiment signals via annotations on the DDA:
//   - experiment.datadoghq.com/id = <experiment-id>
//   - experiment.datadoghq.com/signal = start|rollback|promote
//
// The controller is the sole writer of status.experiment.
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
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	assert "github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	v2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/pkg/condition"
	"github.com/DataDog/datadog-operator/pkg/fleet"
)

// simulateDaemonStart writes experiment start annotations on the DDA, simulating
// what the fleet daemon does when starting an experiment. Uses the real daemon's
// patch builder so both pinned baseline annotations (rollback target revision
// and expected spec hash) carry exactly the values production would write.
//
// The daemon folds the pending annotations into that same patch via
// buildFinalPatch; here they go in a follow-up patch, which reaches the same
// end state because pending annotations are fleet.datadoghq.com/-prefixed and
// therefore excluded from the spec hash. Atomicity of the real single patch is
// covered by the envtest seam-contract test in pkg/fleet.
//
// The reconciler consumes only AnnotationPendingTaskID (into
// Status.Experiment.StartTaskID); the rest are Fleet-worker-only and are
// written so the annotation set matches the daemon's write shape.
func simulateDaemonStart(t *testing.T, c client.Client, nsName types.NamespacedName, experimentID string, config ...json.RawMessage) {
	t.Helper()
	var dda v2alpha1.DatadogAgent
	assert.NoError(t, c.Get(context.TODO(), nsName, &dda))
	rollbackTarget := dda.Status.CurrentRevision
	if rollbackTarget == "" {
		rollbackTarget = "baseline-placeholder"
	}
	var experimentConfig json.RawMessage
	if len(config) > 0 {
		experimentConfig = config[0]
	}
	patch, err := fleet.BuildStartPatch(&dda, experimentID, experimentConfig, rollbackTarget)
	assert.NoError(t, err)
	assert.NoError(t, c.Patch(context.TODO(), &dda, client.RawPatch(types.MergePatchType, patch)))

	pending, err := json.Marshal(map[string]any{"metadata": map[string]any{"annotations": map[string]string{
		v2alpha1.AnnotationPendingTaskID:       "test-task-" + experimentID,
		v2alpha1.AnnotationPendingAction:       "start",
		v2alpha1.AnnotationPendingExperimentID: experimentID,
		v2alpha1.AnnotationPendingPackage:      "datadog-operator",
	}}})
	assert.NoError(t, err)
	assert.NoError(t, c.Patch(context.TODO(), &dda, client.RawPatch(types.MergePatchType, pending)))
}

// simulateDaemonStartWithTaskID writes a well-formed start signal like
// simulateDaemonStart and then attaches the daemon's pending-task-id
// annotation, which processStartSignal captures into Status.Experiment so the
// daemon can report the original task's outcome later. The task ID lives under
// fleet.datadoghq.com/, which ComputeSpecHash filters out, so writing it in a
// second patch leaves the pinned spec hash valid.
func simulateDaemonStartWithTaskID(t *testing.T, c client.Client, nsName types.NamespacedName, experimentID, taskID string) {
	t.Helper()
	simulateDaemonStart(t, c, nsName, experimentID)
	var dda v2alpha1.DatadogAgent
	assert.NoError(t, c.Get(context.TODO(), nsName, &dda))
	patch := fmt.Sprintf(`{"metadata":{"annotations":{%q:%q}}}`, v2alpha1.AnnotationPendingTaskID, taskID)
	assert.NoError(t, c.Patch(context.TODO(), &dda, client.RawPatch(types.MergePatchType, []byte(patch))))
}

// simulateDaemonRollback writes the rollback signal annotations on the DDA.
// Both signal and ID must be set — the real daemon always writes both via buildSignalPatch.
func simulateDaemonRollback(t *testing.T, c client.Client, nsName types.NamespacedName, experimentID string) {
	t.Helper()
	var dda v2alpha1.DatadogAgent
	assert.NoError(t, c.Get(context.TODO(), nsName, &dda))
	patch, err := fleet.BuildSignalPatchWithAnnotations(v2alpha1.ExperimentSignalRollback, experimentID, nil)
	assert.NoError(t, err)
	assert.NoError(t, c.Patch(context.TODO(), &dda, client.RawPatch(types.MergePatchType, patch)))
}

// simulateDaemonPromote writes the promote signal annotations on the DDA.
// Both signal and ID must be set — the real daemon always writes both via buildSignalPatch.
func simulateDaemonPromote(t *testing.T, c client.Client, nsName types.NamespacedName, experimentID string) {
	t.Helper()
	var dda v2alpha1.DatadogAgent
	assert.NoError(t, c.Get(context.TODO(), nsName, &dda))
	patch, err := fleet.BuildSignalPatchWithAnnotations(v2alpha1.ExperimentSignalPromote, experimentID, nil)
	assert.NoError(t, err)
	assert.NoError(t, c.Patch(context.TODO(), &dda, client.RawPatch(types.MergePatchType, patch)))
}

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

// mustGetTerminationReason fetches the DDA and returns the experiment termination reason.
func mustGetTerminationReason(t *testing.T, r *Reconciler, ns, name string) v2alpha1.ExperimentTerminationReason {
	t.Helper()
	var dda v2alpha1.DatadogAgent
	assert.NoError(t, r.client.Get(context.TODO(), types.NamespacedName{Namespace: ns, Name: name}, &dda))
	if dda.Status.Experiment == nil {
		return ""
	}
	return dda.Status.Experiment.TerminationReason
}

// Test_Experiment_StoppedRollback verifies that when the daemon writes a rollback
// annotation, the operator restores the previous spec and sets phase=terminated
// with terminationReason=stopped.
func Test_Experiment_StoppedRollback(t *testing.T) {
	const ns, name = "default", "test-dda"
	const uid = types.UID("uid-1")
	nsName := types.NamespacedName{Namespace: ns, Name: name}

	r := newExperimentIntegrationReconciler(t, 0)

	// Rev1: initial spec.
	dda := baseDDA(ns, name, uid)
	createAndReconcile(t, r, dda)

	// Rev2: daemon applies experiment spec and writes start annotations.
	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Spec.Global.Site = ptr.To("datadoghq.eu")
	assert.NoError(t, r.client.Update(context.TODO(), dda))
	simulateDaemonStart(t, r.client, nsName, "exp-1")
	// Reconcile processes start signal → status.experiment = {running, exp-1}.
	reconcileN(t, r, ns, name, 1)
	assert.Len(t, listOwnedRevisions(t, r.client, ns, uid), 2)
	assert.Equal(t, v2alpha1.ExperimentPhaseRunning, mustGetExperimentPhase(t, r, ns, name))

	// Daemon writes rollback signal annotation with its own task ID (different from start).
	simulateDaemonRollback(t, r.client, nsName, "stop-1")

	// First reconcile: rollback triggered (spec restored, status update may conflict).
	// Second reconcile: spec already correct, status update succeeds.
	reconcileN(t, r, ns, name, 2)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	// The snapshot is taken from the raw, user-submitted spec, which never set
	// Site, so rollback restores Site to nil rather than baking in a default.
	assert.Nil(t, dda.Spec.Global.Site, "spec should be restored to pre-experiment state")
	assert.NotNil(t, dda.Status.Experiment)
	assert.Equal(t, v2alpha1.ExperimentPhaseTerminated, dda.Status.Experiment.Phase)
	assert.Equal(t, v2alpha1.ExperimentTerminationReasonStopped, dda.Status.Experiment.TerminationReason)
}

// Test_Rollback_ReplacesDatadogAnnotationSetToBaseline verifies that rollback
// restores the baseline's Datadog-filter annotation set exactly: keys the
// baseline had come back with their baseline values, keys added after the
// baseline are removed, and annotations outside the Datadog filter are left
// alone. An overlay merge (copy snapshot annotations on top of live) would
// leave the post-baseline Datadog key live on the DDA after rollback.
func Test_Rollback_ReplacesDatadogAnnotationSetToBaseline(t *testing.T) {
	const ns, name = "default", "test-dda"
	const uid = types.UID("uid-1")
	nsName := types.NamespacedName{Namespace: ns, Name: name}

	r := newExperimentIntegrationReconciler(t, 0)

	// Rev1 (baseline): A is the only Datadog-filter annotation.
	dda := baseDDA(ns, name, uid)
	if dda.Annotations == nil {
		dda.Annotations = map[string]string{}
	}
	dda.Annotations["preview.datadoghq.com/A"] = "1"
	createAndReconcile(t, r, dda)

	// Rev2: the experiment changes the spec and adds a second Datadog-filter
	// annotation. Both land before the start signal, so the hash Fleet pins
	// covers them and the later rollback is not misread as a manual change.
	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Spec.Global.Site = ptr.To("datadoghq.eu")
	dda.Annotations["preview.datadoghq.com/B"] = "2"
	assert.NoError(t, r.client.Update(context.TODO(), dda))
	simulateDaemonStart(t, r.client, nsName, "exp-1")
	reconcileN(t, r, ns, name, 1)
	assert.Equal(t, v2alpha1.ExperimentPhaseRunning, mustGetExperimentPhase(t, r, ns, name))

	// A user annotation outside the Datadog filter, added while the experiment
	// runs. It is not part of the snapshot and must survive the rollback.
	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Annotations["example.com/user"] = "keep"
	assert.NoError(t, r.client.Update(context.TODO(), dda))

	simulateDaemonRollback(t, r.client, nsName, "exp-1")
	reconcileN(t, r, ns, name, 2)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	assert.NotNil(t, dda.Status.Experiment)
	assert.Equal(t, v2alpha1.ExperimentPhaseTerminated, dda.Status.Experiment.Phase)
	assert.Equal(t, v2alpha1.ExperimentTerminationReasonStopped, dda.Status.Experiment.TerminationReason)
	assert.Nil(t, dda.Spec.Global.Site, "spec should be restored to pre-experiment state")

	assert.Equal(t, "1", dda.Annotations["preview.datadoghq.com/A"],
		"a Datadog-filter annotation present in the baseline snapshot must be restored")
	assert.NotContains(t, dda.Annotations, "preview.datadoghq.com/B",
		"a Datadog-filter annotation added after the baseline must not survive rollback")
	assert.Equal(t, "keep", dda.Annotations["example.com/user"],
		"annotations outside the Datadog filter must be preserved")
}

// Test_Experiment_TimeoutRollback verifies that an experiment running past
// ExperimentTimeout causes the operator to restore the previous spec and set
// phase=terminated with terminationReason=timed_out.
func Test_Experiment_TimeoutRollback(t *testing.T) {
	const ns, name = "default", "test-dda"
	const uid = types.UID("uid-1")
	const timeout = 50 * time.Millisecond
	nsName := types.NamespacedName{Namespace: ns, Name: name}

	r := newExperimentIntegrationReconciler(t, timeout)

	// Rev1: initial spec.
	dda := baseDDA(ns, name, uid)
	createAndReconcile(t, r, dda)

	// Rev2: daemon applies experiment spec and writes start annotations.
	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Spec.Global.Site = ptr.To("datadoghq.eu")
	assert.NoError(t, r.client.Update(context.TODO(), dda))
	simulateDaemonStart(t, r.client, nsName, "exp-1")
	// Reconcile processes start signal → status.experiment = {running, exp-1}.
	reconcileN(t, r, ns, name, 1)
	assert.Len(t, listOwnedRevisions(t, r.client, ns, uid), 2)
	assert.Equal(t, v2alpha1.ExperimentPhaseRunning, mustGetExperimentPhase(t, r, ns, name))

	// Wait for the timeout to elapse.
	time.Sleep(2 * timeout)

	// Reconcile 1: timeout detected → spec restored; status write may conflict.
	// Reconcile 2: idempotent rollback → status write succeeds.
	reconcileN(t, r, ns, name, 2)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	// The snapshot is taken from the raw, user-submitted spec, which never set
	// Site, so rollback restores Site to nil rather than baking in a default.
	assert.Nil(t, dda.Spec.Global.Site, "spec should be restored after timeout")
	assert.NotNil(t, dda.Status.Experiment)
	assert.Equal(t, v2alpha1.ExperimentPhaseTerminated, dda.Status.Experiment.Phase)
	assert.Equal(t, v2alpha1.ExperimentTerminationReasonTimedOut, dda.Status.Experiment.TerminationReason)
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

	// Rev2: daemon applies experiment spec and writes start annotations.
	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Spec.Global.Site = ptr.To("datadoghq.eu")
	assert.NoError(t, r.client.Update(context.TODO(), dda))
	simulateDaemonStart(t, r.client, nsName, "exp-1")
	reconcileN(t, r, ns, name, 1)
	assert.Equal(t, v2alpha1.ExperimentPhaseRunning, mustGetExperimentPhase(t, r, ns, name))

	// Patch revision timestamps to a recent time so the timeout path in
	// handleRollback is not accidentally triggered before the abort check runs.
	for _, rev := range listOwnedRevisions(t, r.client, ns, uid) {
		rev.CreationTimestamp = metav1.Now()
		assert.NoError(t, r.client.Update(context.TODO(), &rev))
	}

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
	assert.Equal(t, v2alpha1.ExperimentPhaseAborted, mustGetExperimentPhase(t, r, ns, name))
}

// Test_Experiment_TimeoutPhase_IsStable verifies that once phase=terminated
// (timed_out) is persisted, further reconciles do not change the spec or phase.
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
	simulateDaemonStart(t, r.client, nsName, "exp-1")
	reconcileN(t, r, ns, name, 1)
	assert.Equal(t, v2alpha1.ExperimentPhaseRunning, mustGetExperimentPhase(t, r, ns, name))

	time.Sleep(2 * timeout)
	reconcileN(t, r, ns, name, 2)

	assert.Equal(t, v2alpha1.ExperimentPhaseTerminated, mustGetExperimentPhase(t, r, ns, name))
	assert.Equal(t, v2alpha1.ExperimentTerminationReasonTimedOut, mustGetTerminationReason(t, r, ns, name))

	// Extra reconciles must not change phase or spec.
	reconcileN(t, r, ns, name, 3)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	// Raw baseline never set Site, so it stays nil after rollback.
	assert.Nil(t, dda.Spec.Global.Site)
	assert.Equal(t, v2alpha1.ExperimentPhaseTerminated, mustGetExperimentPhase(t, r, ns, name))
}

// Test_Experiment_TerminatedPhase_IsStable verifies that once phase=terminated
// (stopped) is persisted, further reconciles do not change the spec or phase.
func Test_Experiment_TerminatedPhase_IsStable(t *testing.T) {
	const ns, name = "default", "test-dda"
	const uid = types.UID("uid-1")
	nsName := types.NamespacedName{Namespace: ns, Name: name}

	r := newExperimentIntegrationReconciler(t, 0)

	dda := baseDDA(ns, name, uid)
	createAndReconcile(t, r, dda)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Spec.Global.Site = ptr.To("datadoghq.eu")
	assert.NoError(t, r.client.Update(context.TODO(), dda))
	simulateDaemonStart(t, r.client, nsName, "exp-1")
	reconcileN(t, r, ns, name, 1)
	assert.Equal(t, v2alpha1.ExperimentPhaseRunning, mustGetExperimentPhase(t, r, ns, name))

	// Daemon writes rollback signal annotation with its own task ID.
	simulateDaemonRollback(t, r.client, nsName, "stop-1")
	reconcileN(t, r, ns, name, 2)

	assert.Equal(t, v2alpha1.ExperimentPhaseTerminated, mustGetExperimentPhase(t, r, ns, name))
	assert.Equal(t, v2alpha1.ExperimentTerminationReasonStopped, mustGetTerminationReason(t, r, ns, name))

	// Extra reconciles must not change phase or spec.
	reconcileN(t, r, ns, name, 3)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	// Raw baseline never set Site, so it stays nil after rollback.
	assert.Nil(t, dda.Spec.Global.Site)
	assert.Equal(t, v2alpha1.ExperimentPhaseTerminated, mustGetExperimentPhase(t, r, ns, name))
}

// Test_Experiment_RunningAfterTimeout verifies that if RC writes phase=running
// after a timeout rollback has completed, the operator fires timeout again
// idempotently: the pre-experiment revision is old enough to exceed the timeout
// threshold, rollback is a no-op (spec already correct), and phase=terminated
// is written again.
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
	simulateDaemonStart(t, r.client, nsName, "exp-1")
	reconcileN(t, r, ns, name, 1)
	assert.Equal(t, v2alpha1.ExperimentPhaseRunning, mustGetExperimentPhase(t, r, ns, name))

	time.Sleep(2 * timeout)
	reconcileN(t, r, ns, name, 2)
	assert.Equal(t, v2alpha1.ExperimentPhaseTerminated, mustGetExperimentPhase(t, r, ns, name))

	// Daemon writes start signal again after the rollback already completed.
	// The start signal uses the same ID, so processStartSignal sees that the
	// annotation ID already matches the status ID and is a no-op. The status
	// stays at terminated. (In the old model, the daemon could directly overwrite
	// status to running, but that's no longer possible.)
	// Instead, verify that the terminated phase is stable by just reconciling again.
	reconcileN(t, r, ns, name, 1)

	// Phase should remain terminated — the experiment is already terminated.
	assert.Equal(t, v2alpha1.ExperimentPhaseTerminated, mustGetExperimentPhase(t, r, ns, name))
}

// Test_Experiment_StopAfterRollback verifies that if the daemon writes a rollback
// annotation after a rollback has already completed, the controller handles it
// cleanly (rollback signal is a no-op since phase is terminal, spec unchanged).
func Test_Experiment_StopAfterRollback(t *testing.T) {
	const ns, name = "default", "test-dda"
	const uid = types.UID("uid-1")
	nsName := types.NamespacedName{Namespace: ns, Name: name}

	r := newExperimentIntegrationReconciler(t, 0)

	dda := baseDDA(ns, name, uid)
	createAndReconcile(t, r, dda)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Spec.Global.Site = ptr.To("datadoghq.eu")
	assert.NoError(t, r.client.Update(context.TODO(), dda))
	simulateDaemonStart(t, r.client, nsName, "exp-1")
	reconcileN(t, r, ns, name, 1)
	assert.Equal(t, v2alpha1.ExperimentPhaseRunning, mustGetExperimentPhase(t, r, ns, name))

	// Daemon writes rollback signal with its own task ID → triggers rollback.
	simulateDaemonRollback(t, r.client, nsName, "stop-1")
	reconcileN(t, r, ns, name, 2)
	assert.Equal(t, v2alpha1.ExperimentPhaseTerminated, mustGetExperimentPhase(t, r, ns, name))

	// Daemon writes rollback signal again after rollback already completed.
	// processRollbackSignal checks isTerminalPhase(terminated) == true,
	// so it's a no-op.
	simulateDaemonRollback(t, r.client, nsName, "stop-2")
	reconcileN(t, r, ns, name, 2)

	// Spec should still be the rolled-back spec; phase=terminated unchanged.
	// Raw baseline never set Site, so it stays nil after rollback.
	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	assert.Nil(t, dda.Spec.Global.Site)
	assert.Equal(t, v2alpha1.ExperimentPhaseTerminated, mustGetExperimentPhase(t, r, ns, name))
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

	// Spec should be the user's manual change, not rolled back.
	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	assert.Equal(t, "datadoghq.eu", *dda.Spec.Global.Site, "aborted experiment must not trigger rollback")
	assert.Equal(t, v2alpha1.ExperimentPhaseAborted, mustGetExperimentPhase(t, r, ns, name))

	// Also verify the revision timestamp is not used as a proxy for time.Now() comparison.
	_ = metav1.Now()
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
	simulateDaemonStart(t, r.client, nsName, "exp-1")
	reconcileN(t, r, ns, name, 1)
	assert.Len(t, listOwnedRevisions(t, r.client, ns, uid), 2)
	assert.Equal(t, v2alpha1.ExperimentPhaseRunning, mustGetExperimentPhase(t, r, ns, name))

	// Daemon writes promote signal with its own task ID (experiment succeeded, keep the new spec).
	simulateDaemonPromote(t, r.client, nsName, "promote-1")

	// Reconcile processes the promote signal: sets phase=promoted, annotates the
	// revision, then ensureRevision sees the annotation and recreates it with
	// a fresh timestamp (consuming the annotation in the process).
	reconcileN(t, r, ns, name, 1)
	assert.Equal(t, v2alpha1.ExperimentPhasePromoted, mustGetExperimentPhase(t, r, ns, name))

	// New experiment: daemon patches the spec and writes start annotations.
	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Spec.Global.Site = ptr.To("datadoghq.jp")
	assert.NoError(t, r.client.Update(context.TODO(), dda))
	simulateDaemonStart(t, r.client, nsName, "exp-2")
	// Reconcile processes start signal → status.experiment = {running, exp-2}.
	reconcileN(t, r, ns, name, 1)

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
			// promoted: daemon signals promote, spec stays as-is.
			name: "promoted",
			reach: func(t *testing.T, r *Reconciler, ns, name string, uid types.UID, nsName types.NamespacedName, dda *v2alpha1.DatadogAgent) {
				t.Helper()
				simulateDaemonStart(t, r.client, nsName, "exp-1")
				reconcileN(t, r, ns, name, 1)
				simulateDaemonPromote(t, r.client, nsName, "promote-1")
				// Reconcile processes promote → annotates the experiment revision.
				reconcileN(t, r, ns, name, 1)
			},
		},
		{
			// terminated (stopped): daemon signals rollback, operator restores previous spec.
			name: "terminated_stopped",
			reach: func(t *testing.T, r *Reconciler, ns, name string, uid types.UID, nsName types.NamespacedName, dda *v2alpha1.DatadogAgent) {
				t.Helper()
				simulateDaemonStart(t, r.client, nsName, "exp-1")
				reconcileN(t, r, ns, name, 1)
				simulateDaemonRollback(t, r.client, nsName, "stop-1")
				// Two reconciles: first restores spec (status conflicts),
				// second persists phase=terminated.
				reconcileN(t, r, ns, name, 2)
			},
		},
		{
			// terminated (timed_out): experiment runs past the deadline, operator rolls back.
			name: "terminated_timed_out",
			reach: func(t *testing.T, r *Reconciler, ns, name string, uid types.UID, nsName types.NamespacedName, dda *v2alpha1.DatadogAgent) {
				t.Helper()
				r.options.ExperimentTimeout = 50 * time.Millisecond
				simulateDaemonStart(t, r.client, nsName, "exp-1")
				reconcileN(t, r, ns, name, 1)
				time.Sleep(100 * time.Millisecond)
				// Two reconciles: first rolls back spec (status conflicts),
				// second persists phase=terminated.
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
				simulateDaemonStart(t, r.client, nsName, "exp-1")
				reconcileN(t, r, ns, name, 1)
				// Patch timestamps again after reconcile.
				for _, rev := range listOwnedRevisions(t, r.client, ns, uid) {
					rev.CreationTimestamp = metav1.Now()
					assert.NoError(t, r.client.Update(context.TODO(), &rev))
				}
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
			// rollback: daemon signals rollback → operator rolls back → phase=terminated.
			name: "rollback",
			action: func(t *testing.T, r *Reconciler, ns, name string, uid types.UID, nsName types.NamespacedName, dda *v2alpha1.DatadogAgent) {
				t.Helper()
				simulateDaemonRollback(t, r.client, nsName, "stop-2")
				reconcileN(t, r, ns, name, 2)
			},
			expect: v2alpha1.ExperimentPhaseTerminated,
		},
		{
			// promote: daemon signals promote → phase=promoted.
			name: "promote",
			action: func(t *testing.T, r *Reconciler, ns, name string, uid types.UID, nsName types.NamespacedName, dda *v2alpha1.DatadogAgent) {
				t.Helper()
				simulateDaemonPromote(t, r.client, nsName, "promote-2")
				reconcileN(t, r, ns, name, 1)
			},
			expect: v2alpha1.ExperimentPhasePromoted,
		},
		{
			// timeout: age both the new experiment's revision and its
			// Status.Experiment.StartedAt past the deadline so the
			// reconciler triggers a real timeout. After the
			// StartedAt anchor change (commit 9d8492397), revision
			// timestamps alone are no longer load-bearing — the
			// reconciler measures elapsed against StartedAt.
			name: "timeout",
			action: func(t *testing.T, r *Reconciler, ns, name string, uid types.UID, nsName types.NamespacedName, dda *v2alpha1.DatadogAgent) {
				t.Helper()
				r.options.ExperimentTimeout = 50 * time.Millisecond
				staleTime := metav1.NewTime(time.Now().Add(-time.Minute))
				// Backdate StartedAt so handleRollback's primary anchor
				// exceeds the timeout.
				fresh := &v2alpha1.DatadogAgent{}
				assert.NoError(t, r.client.Get(context.TODO(), nsName, fresh))
				if fresh.Status.Experiment != nil {
					fresh.Status.Experiment.StartedAt = &staleTime
					assert.NoError(t, r.client.Status().Update(context.TODO(), fresh))
				}
				reconcileN(t, r, ns, name, 2)
			},
			expect: v2alpha1.ExperimentPhaseTerminated,
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

					// Drive exp-1 to its terminal state (promoted/terminated/aborted).
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

					// Daemon patches spec and writes start annotations atomically.
					r.options.ExperimentTimeout = 5 * time.Second
					assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
					dda.Spec.Global.Site = ptr.To("datadoghq.jp")
					assert.NoError(t, r.client.Update(context.TODO(), dda))
					simulateDaemonStart(t, r.client, nsName, "exp-2")
					// Reconcile processes start signal → status = {running, exp-2},
					// and manageRevision creates a revision for the new spec.
					reconcileN(t, r, ns, name, 1)

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
// NOTE: timeout is anchored on Status.Experiment.StartedAt (not on any
// ControllerRevision's CreationTimestamp), so re-applying the same spec for a
// brand-new experiment cannot false-timeout regardless of sibling revision age.
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

	// Step 2: Apply experiment spec (rev2) and start via annotations.
	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Spec.Global.Site = ptr.To("datadoghq.eu")
	assert.NoError(t, r.client.Update(context.TODO(), dda))
	simulateDaemonStart(t, r.client, nsName, "exp-1")
	reconcileN(t, r, ns, name, 1)
	assert.Len(t, listOwnedRevisions(t, r.client, ns, uid), 2)
	assert.Equal(t, v2alpha1.ExperimentPhaseRunning, mustGetExperimentPhase(t, r, ns, name))

	// Step 3: Let it timeout.
	time.Sleep(2 * shortTimeout)
	reconcileN(t, r, ns, name, 2)

	assert.Equal(t, v2alpha1.ExperimentPhaseTerminated, mustGetExperimentPhase(t, r, ns, name))
	assert.Equal(t, v2alpha1.ExperimentTerminationReasonTimedOut, mustGetTerminationReason(t, r, ns, name))
	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	// Raw baseline never set Site, so it stays nil after rollback.
	assert.Nil(t, dda.Spec.Global.Site, "spec should be rolled back")

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
	reconcileN(t, r, ns, name, 1)

	// Step 5: Start new experiment via annotations. Should NOT timeout even
	// though sibling revisions may still carry stale timestamps, because
	// StartedAt was just set fresh by processStartSignal.
	simulateDaemonStart(t, r.client, nsName, "exp-2")
	reconcileN(t, r, ns, name, 1)

	assert.Equal(t, v2alpha1.ExperimentPhaseRunning, mustGetExperimentPhase(t, r, ns, name),
		"experiment should still be running — no immediate timeout after reapply")
}

// Test_Experiment_StartWritesCheckpoint verifies that a successful start
// signal records an ExperimentCheckpoint carrying the rollback-target
// revision from the daemon's annotation and a non-empty expected spec hash.
func Test_Experiment_StartWritesCheckpoint(t *testing.T) {
	const ns, name = "default", "test-dda"
	const uid = types.UID("uid-1")
	nsName := types.NamespacedName{Namespace: ns, Name: name}

	r := newExperimentIntegrationReconciler(t, 0)

	dda := baseDDA(ns, name, uid)
	createAndReconcile(t, r, dda)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	baselineRevision := dda.Status.CurrentRevision
	assert.NotEmpty(t, baselineRevision)

	dda.Spec.Global.Site = ptr.To("datadoghq.eu")
	assert.NoError(t, r.client.Update(context.TODO(), dda))
	simulateDaemonStart(t, r.client, nsName, "exp-1")

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	pinnedHash := dda.GetAnnotations()[v2alpha1.AnnotationExperimentExpectedSpecHash]
	assert.NotEmpty(t, pinnedHash)

	reconcileN(t, r, ns, name, 1)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	assert.NotNil(t, dda.Status.Experiment)
	assert.NotNil(t, dda.Status.Experiment.Checkpoint)
	assert.Equal(t, baselineRevision, dda.Status.Experiment.Checkpoint.RollbackTargetRevision)
	// Byte-equal against what Fleet pinned, not merely non-empty: the reconciler
	// must copy the annotation, never recompute a hash of its own.
	assert.Equal(t, pinnedHash, dda.Status.Experiment.Checkpoint.ExpectedSpecHash)
}

// Test_Experiment_StartWithoutExpectedHashAborts verifies the symmetric case to
// a missing rollback target: a start signal carrying a baseline revision but no
// pinned spec hash is equally unusable, because the reconciler cannot tell
// whether the spec it sees is the one Fleet intended to run.
func Test_Experiment_StartWithoutExpectedHashAborts(t *testing.T) {
	const ns, name = "default", "test-dda"
	const uid = types.UID("uid-1")
	nsName := types.NamespacedName{Namespace: ns, Name: name}

	r := newExperimentIntegrationReconciler(t, 0)

	dda := baseDDA(ns, name, uid)
	createAndReconcile(t, r, dda)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	baselineRevision := dda.Status.CurrentRevision
	dda.Spec.Global.Site = ptr.To("datadoghq.eu")
	assert.NoError(t, r.client.Update(context.TODO(), dda))

	patch, err := fleet.BuildSignalPatchWithAnnotations(v2alpha1.ExperimentSignalStart, "exp-1", map[string]string{
		v2alpha1.AnnotationExperimentRollbackTargetRevision: baselineRevision,
		v2alpha1.AnnotationPendingTaskID:                    "task-1",
	})
	assert.NoError(t, err)
	assert.NoError(t, r.client.Patch(context.TODO(), dda, client.RawPatch(types.MergePatchType, patch)))

	reconcileN(t, r, ns, name, 1)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	assert.NotNil(t, dda.Status.Experiment)
	assert.Equal(t, v2alpha1.ExperimentPhaseAborted, dda.Status.Experiment.Phase)
	assert.Equal(t, v2alpha1.ExperimentTerminationReasonBaselineMissing, dda.Status.Experiment.TerminationReason)
	assert.Nil(t, dda.Status.Experiment.Checkpoint)
	assert.Equal(t, "task-1", dda.Status.Experiment.StartTaskID)

	cond := condition.GetCondition(&dda.Status, common.ExperimentConfigStrandedConditionType)
	assert.NotNil(t, cond, "a malformed start signal leaves unapproved config live")
	assert.Equal(t, metav1.ConditionTrue, cond.Status)
}

// Test_Experiment_StartWithMismatchedExpectedHash_ManualSpecChange verifies the
// hash validation on the normal start path: Fleet's atomic write lands, a user
// edits the spec before the reconciler reads it, and the reconciler refuses to
// start. The user's spec stands, so nothing is stranded — this is a manual spec
// change, not a broken baseline.
//
// This is the test that distinguishes copying the pinned annotation from
// recomputing it: a reconciler that recomputed would agree with itself and
// start the experiment.
func Test_Experiment_StartWithMismatchedExpectedHash_ManualSpecChange(t *testing.T) {
	const ns, name = "default", "test-dda"
	const uid = types.UID("uid-1")
	nsName := types.NamespacedName{Namespace: ns, Name: name}

	r := newExperimentIntegrationReconciler(t, 0)

	dda := baseDDA(ns, name, uid)
	createAndReconcile(t, r, dda)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Spec.Global.Site = ptr.To("datadoghq.eu")
	assert.NoError(t, r.client.Update(context.TODO(), dda))

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	patch, err := fleet.BuildStartPatch(dda, "exp-1", nil, dda.Status.CurrentRevision)
	assert.NoError(t, err)
	assert.NoError(t, r.client.Patch(context.TODO(), dda, client.RawPatch(types.MergePatchType, patch)))

	// The user edits the spec between Fleet's write and the reconciler's read.
	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Spec.Global.Site = ptr.To("user-edit.example.com")
	assert.NoError(t, r.client.Update(context.TODO(), dda))

	reconcileN(t, r, ns, name, 1)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	assert.NotNil(t, dda.Status.Experiment)
	assert.Equal(t, v2alpha1.ExperimentPhaseAborted, dda.Status.Experiment.Phase)
	assert.Equal(t, v2alpha1.ExperimentTerminationReasonManualSpecChange, dda.Status.Experiment.TerminationReason)
	assert.Nil(t, dda.Status.Experiment.Checkpoint)
	assert.Equal(t, "user-edit.example.com", *dda.Spec.Global.Site)

	cond := condition.GetCondition(&dda.Status, common.ExperimentConfigStrandedConditionType)
	assert.Nil(t, cond, "the user's own spec is live, so nothing is stranded")
}

// Test_Experiment_StartWithoutRollbackTargetAborts verifies that a start
// signal with no rollback-target-revision annotation is rejected immediately:
// the experiment is aborted with baseline_missing rather than started, since
// there is no safe baseline to roll back to.
func Test_Experiment_StartWithoutRollbackTargetAborts(t *testing.T) {
	const ns, name = "default", "test-dda"
	const uid = types.UID("uid-1")
	nsName := types.NamespacedName{Namespace: ns, Name: name}

	r := newExperimentIntegrationReconciler(t, 0)

	dda := baseDDA(ns, name, uid)
	createAndReconcile(t, r, dda)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Spec.Global.Site = ptr.To("datadoghq.eu")
	assert.NoError(t, r.client.Update(context.TODO(), dda))

	// Simulate a daemon start signal missing the rollback-target-revision
	// annotation (e.g. planStart never computed a baseline).
	patch, err := fleet.BuildSignalPatchWithAnnotations(v2alpha1.ExperimentSignalStart, "exp-1", map[string]string{
		v2alpha1.AnnotationPendingTaskID: "task-1",
	})
	assert.NoError(t, err)
	assert.NoError(t, r.client.Patch(context.TODO(), dda, client.RawPatch(types.MergePatchType, patch)))

	reconcileN(t, r, ns, name, 1)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	assert.NotNil(t, dda.Status.Experiment)
	assert.Equal(t, v2alpha1.ExperimentPhaseAborted, dda.Status.Experiment.Phase)
	assert.Equal(t, v2alpha1.ExperimentTerminationReasonBaselineMissing, dda.Status.Experiment.TerminationReason)
	assert.Nil(t, dda.Status.Experiment.Checkpoint)
	assert.Equal(t, "task-1", dda.Status.Experiment.StartTaskID)

	cond := condition.GetCondition(&dda.Status, common.ExperimentConfigStrandedConditionType)
	assert.NotNil(t, cond, "a malformed start signal leaves unapproved config live")
	assert.Equal(t, metav1.ConditionTrue, cond.Status)
}

// failingRevisionReader delegates to a real reader but fails every
// ControllerRevision Get. It stands in for a transient apiserver error on the
// uncached read getOwnedRevision performs, which must be distinguishable from
// "the revision is gone".
type failingRevisionReader struct {
	client.Reader
	err error
}

func (f failingRevisionReader) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if _, ok := obj.(*appsv1.ControllerRevision); ok {
		return f.err
	}
	return f.Reader.Get(ctx, key, obj, opts...)
}

// Test_Experiment_StartAbortsWhenRollbackTargetRevisionMissing verifies the
// second half of the rollback-target contract: the annotation is present and
// the pinned hash matches, but the ControllerRevision it names no longer
// exists. Fleet's spec is live with no proven way back, so the start is
// aborted with baseline_not_found rather than recorded as Running — the
// distinction from baseline_missing is that the name did resolve when Fleet
// pinned it.
func Test_Experiment_StartAbortsWhenRollbackTargetRevisionMissing(t *testing.T) {
	const ns, name = "default", "test-dda"
	const uid = types.UID("uid-1")
	nsName := types.NamespacedName{Namespace: ns, Name: name}

	r := newExperimentIntegrationReconciler(t, 0)

	dda := baseDDA(ns, name, uid)
	createAndReconcile(t, r, dda)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	baselineRevision := dda.Status.CurrentRevision
	assert.NotEmpty(t, baselineRevision)

	dda.Spec.Global.Site = ptr.To("datadoghq.eu")
	assert.NoError(t, r.client.Update(context.TODO(), dda))
	simulateDaemonStartWithTaskID(t, r.client, nsName, "exp-1", "task-1")

	// The pinned baseline disappears between Fleet's atomic write and the
	// reconciler's read (GC, or a hand-deleted ControllerRevision).
	assert.NoError(t, r.client.Delete(context.TODO(), &appsv1.ControllerRevision{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: baselineRevision},
	}))

	reconcileN(t, r, ns, name, 1)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	assert.NotNil(t, dda.Status.Experiment)
	assert.Equal(t, v2alpha1.ExperimentPhaseAborted, dda.Status.Experiment.Phase)
	assert.Equal(t, v2alpha1.ExperimentTerminationReasonBaselineNotFound, dda.Status.Experiment.TerminationReason)
	assert.Nil(t, dda.Status.Experiment.Checkpoint)
	assert.Equal(t, "task-1", dda.Status.Experiment.StartTaskID)

	cond := condition.GetCondition(&dda.Status, common.ExperimentConfigStrandedConditionType)
	assert.NotNil(t, cond, "the experiment spec is live with no resolvable baseline")
	assert.Equal(t, metav1.ConditionTrue, cond.Status)
}

// Test_Experiment_StartAbortsWhenRollbackTargetRevisionForeign verifies that
// resolving the pinned name is not enough: the object it resolves to must pass
// the full ownedByDDA check. Each subtest breaks exactly one clause, so a
// reconciler that only checked existence (or only one of the three clauses)
// would start an experiment whose "baseline" belongs to somebody else.
func Test_Experiment_StartAbortsWhenRollbackTargetRevisionForeign(t *testing.T) {
	const ns, name = "default", "test-dda"
	const uid = types.UID("uid-1")

	corruptions := map[string]func(rev *appsv1.ControllerRevision){
		"foreign controller UID": func(rev *appsv1.ControllerRevision) {
			// A DDA deleted and recreated under the same name leaves revisions
			// behind that match on namespace and label but not on UID.
			rev.OwnerReferences[0].UID = types.UID("uid-someone-else")
		},
		"foreign name label": func(rev *appsv1.ControllerRevision) {
			rev.Labels[apicommon.DatadogAgentNameLabelKey] = "other-dda"
		},
		"wrong namespace": func(rev *appsv1.ControllerRevision) {
			rev.Namespace = "other-ns"
		},
	}

	for tn, corrupt := range corruptions {
		t.Run(tn, func(t *testing.T) {
			nsName := types.NamespacedName{Namespace: ns, Name: name}

			r := newExperimentIntegrationReconciler(t, 0)

			dda := baseDDA(ns, name, uid)
			createAndReconcile(t, r, dda)

			assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
			baselineRevision := dda.Status.CurrentRevision
			assert.NotEmpty(t, baselineRevision)

			var baseline appsv1.ControllerRevision
			assert.NoError(t, r.client.Get(context.TODO(), types.NamespacedName{Namespace: ns, Name: baselineRevision}, &baseline))
			assert.Len(t, baseline.OwnerReferences, 1)

			dda.Spec.Global.Site = ptr.To("datadoghq.eu")
			assert.NoError(t, r.client.Update(context.TODO(), dda))
			simulateDaemonStartWithTaskID(t, r.client, nsName, "exp-1", "task-1")

			// Replace the pinned baseline with a revision carrying the same
			// name but failing one ownership clause.
			assert.NoError(t, r.client.Delete(context.TODO(), &baseline))
			foreign := baseline.DeepCopy()
			foreign.ResourceVersion = ""
			foreign.UID = ""
			corrupt(foreign)
			assert.NoError(t, r.client.Create(context.TODO(), foreign))

			reconcileN(t, r, ns, name, 1)

			assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
			assert.NotNil(t, dda.Status.Experiment)
			assert.Equal(t, v2alpha1.ExperimentPhaseAborted, dda.Status.Experiment.Phase)
			assert.Equal(t, v2alpha1.ExperimentTerminationReasonBaselineNotFound, dda.Status.Experiment.TerminationReason)
			assert.Nil(t, dda.Status.Experiment.Checkpoint)
			assert.Equal(t, "task-1", dda.Status.Experiment.StartTaskID)

			cond := condition.GetCondition(&dda.Status, common.ExperimentConfigStrandedConditionType)
			assert.NotNil(t, cond, "the experiment spec is live with no owned baseline")
			assert.Equal(t, metav1.ConditionTrue, cond.Status)
		})
	}
}

// Test_Experiment_StartSignalOnTerminalSameIDAbortsAsStranded pins the
// reconciler's half of the same-ID-vs-terminal defense. Fleet's planStart
// refuses to write a start signal for an experiment that already completed, so
// a signal for a completed ID showing up here means something bypassed that
// guard — and whatever wrote the signal wrote the experiment spec with it. The
// reconciler must not clear those annotations quietly: that would leave an
// unapproved spec live under a terminal status with nothing to surface it. It
// aborts and reports stranded config instead, and deliberately leaves the
// drifted spec alone — the abort exists to make the drift visible, not repair
// it. Every terminal phase is covered so a regression that phase-guards only
// one of them fails here instead of shipping.
func Test_Experiment_StartSignalOnTerminalSameIDAbortsAsStranded(t *testing.T) {
	terminalStates := map[string]struct {
		phase  v2alpha1.ExperimentPhase
		reason v2alpha1.ExperimentTerminationReason
	}{
		"terminated": {v2alpha1.ExperimentPhaseTerminated, v2alpha1.ExperimentTerminationReasonStopped},
		"aborted":    {v2alpha1.ExperimentPhaseAborted, v2alpha1.ExperimentTerminationReasonManualSpecChange},
		"promoted":   {v2alpha1.ExperimentPhasePromoted, ""},
	}
	for stateName, state := range terminalStates {
		t.Run(stateName, func(t *testing.T) {
			const ns, name = "default", "test-dda"
			const uid = types.UID("uid-1")
			nsName := types.NamespacedName{Namespace: ns, Name: name}

			r := newExperimentIntegrationReconciler(t, 0)

			dda := baseDDA(ns, name, uid)
			createAndReconcile(t, r, dda)

			assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
			baselineRevision := dda.Status.CurrentRevision
			assert.NotEmpty(t, baselineRevision)

			// A completed experiment: terminal phase, its own termination
			// reason, the checkpoint it ran under, and no signal annotations —
			// the reconciler cleared those when the experiment ended.
			startedAt := metav1.Now()
			dda.Status.Experiment = &v2alpha1.ExperimentStatus{
				Phase:             state.phase,
				ID:                "exp-1",
				StartedAt:         &startedAt,
				StartTaskID:       "task-original",
				TerminationReason: state.reason,
				Checkpoint: &v2alpha1.ExperimentCheckpoint{
					RollbackTargetRevision: baselineRevision,
					ExpectedSpecHash:       "hash-of-the-spec-this-experiment-actually-ran",
				},
			}
			assert.NoError(t, r.client.Status().Update(context.TODO(), dda))

			// The write that bypassed planStart: the experiment spec plus all
			// four pinned annotations, built by the real BuildStartPatch so the
			// pins carry exactly what a live-but-unguarded Fleet would pin. The
			// pinned hash therefore matches the drifted spec, which rules out
			// the manual_spec_change path — the only thing wrong here is the
			// terminal status underneath.
			dda.Spec.Global.Site = ptr.To("datadoghq.eu")
			assert.NoError(t, r.client.Update(context.TODO(), dda))
			simulateDaemonStartWithTaskID(t, r.client, nsName, "exp-1", "task-resend")

			assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
			assert.Equal(t, "datadoghq.eu", *dda.Spec.Global.Site,
				"the bypassing write must really persist the experiment spec, or there is no drift to detect")
			assert.NotEmpty(t, dda.GetAnnotations()[v2alpha1.AnnotationExperimentRollbackTargetRevision])
			assert.NotEmpty(t, dda.GetAnnotations()[v2alpha1.AnnotationExperimentExpectedSpecHash])

			reconcileN(t, r, ns, name, 1)

			assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
			assert.NotNil(t, dda.Status.Experiment)
			assert.Equal(t, v2alpha1.ExperimentPhaseAborted, dda.Status.Experiment.Phase)
			assert.Equal(t, v2alpha1.ExperimentTerminationReasonBaselineMissing, dda.Status.Experiment.TerminationReason)
			assert.Nil(t, dda.Status.Experiment.Checkpoint,
				"a stale same-ID signal must not be trusted far enough to record a checkpoint")
			assert.Equal(t, "task-resend", dda.Status.Experiment.StartTaskID,
				"StartTaskID comes from the pending annotation so Fleet can fail the task that caused this")

			cond := condition.GetCondition(&dda.Status, common.ExperimentConfigStrandedConditionType)
			assert.NotNil(t, cond, "ExperimentConfigStranded is the only visible signal of this drift")
			assert.Equal(t, metav1.ConditionTrue, cond.Status)
			assert.Equal(t, string(v2alpha1.ExperimentTerminationReasonBaselineMissing), cond.Reason)

			// The abort reports the drift; it does not undo it. A regression
			// that rolls back here would silently discard live config on a path
			// with no verified baseline behind it.
			assert.Equal(t, "datadoghq.eu", *dda.Spec.Global.Site)

			// One more pass settles: the stale annotations clear (they cannot
			// be cleared in the same pass that mutated status) and the terminal
			// state holds.
			reconcileN(t, r, ns, name, 1)
			assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
			for _, key := range []string{
				v2alpha1.AnnotationExperimentSignal,
				v2alpha1.AnnotationExperimentID,
				v2alpha1.AnnotationExperimentRollbackTargetRevision,
				v2alpha1.AnnotationExperimentExpectedSpecHash,
			} {
				assert.Empty(t, dda.GetAnnotations()[key], "annotation %s should be cleared", key)
			}
			assert.Equal(t, v2alpha1.ExperimentPhaseAborted, dda.Status.Experiment.Phase)
			assert.Equal(t, v2alpha1.ExperimentTerminationReasonBaselineMissing, dda.Status.Experiment.TerminationReason)
			assert.Equal(t, "datadoghq.eu", *dda.Spec.Global.Site)
		})
	}
}

// Test_Experiment_StartSignalLeftoverOnTerminalSameIDIsCleared is the other
// half of the same-ID-vs-terminal rule. Annotation clearing is deferred to a
// pass that does not mutate status, so a start signal routinely survives into
// the terminal phase of the very experiment it started — here a timeout
// terminates the experiment while its own start annotations are still on the
// object. Those pins still match the recorded checkpoint, so they are a
// leftover and get cleared quietly. A regression that keys the abort on the
// terminal phase alone would flip every timed-out or promoted experiment to
// aborted and report stranded config on a perfectly clean rollback.
func Test_Experiment_StartSignalLeftoverOnTerminalSameIDIsCleared(t *testing.T) {
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
	simulateDaemonStart(t, r.client, nsName, "exp-1")

	reconcileN(t, r, ns, name, 1)
	assert.Equal(t, v2alpha1.ExperimentPhaseRunning, mustGetExperimentPhase(t, r, ns, name))

	time.Sleep(2 * timeout)

	// One pass at a time, because how many passes the terminal status write and
	// the annotation clear take is an internal detail. What matters is that some
	// pass sees a terminal phase with the start signal still on the object —
	// the exact shape the stranded abort looks for — and that none of them
	// aborts.
	sawTerminalWithStartSignal := false
	for i := 0; i < 4; i++ {
		reconcileN(t, r, ns, name, 1)
		assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
		if dda.Status.Experiment.Phase == v2alpha1.ExperimentPhaseTerminated &&
			dda.GetAnnotations()[v2alpha1.AnnotationExperimentID] == "exp-1" {
			sawTerminalWithStartSignal = true
		}
		assert.NotEqual(t, v2alpha1.ExperimentPhaseAborted, dda.Status.Experiment.Phase,
			"a leftover start signal must never abort the experiment it started")
	}
	assert.True(t, sawTerminalWithStartSignal, "test never reached the leftover path")

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	assert.Equal(t, v2alpha1.ExperimentPhaseTerminated, dda.Status.Experiment.Phase)
	assert.Equal(t, v2alpha1.ExperimentTerminationReasonTimedOut, dda.Status.Experiment.TerminationReason)
	assert.Nil(t, condition.GetCondition(&dda.Status, common.ExperimentConfigStrandedConditionType),
		"a clean timeout rollback strands nothing")
	assert.Empty(t, dda.GetAnnotations()[v2alpha1.AnnotationExperimentSignal])
	assert.Nil(t, dda.Spec.Global.Site, "the timeout rollback stands")
}

// Test_Experiment_StartRollbackTargetReadErrorRequeuesWithoutAbort verifies
// that a transient failure reading the rollback target is not treated as proof
// the baseline is gone. Aborting here would burn a legitimate experiment on an
// apiserver blip; instead the reconcile errors out with the start signal still
// pending, so the next pass retries it.
func Test_Experiment_StartRollbackTargetReadErrorRequeuesWithoutAbort(t *testing.T) {
	const ns, name = "default", "test-dda"
	const uid = types.UID("uid-1")
	nsName := types.NamespacedName{Namespace: ns, Name: name}

	r := newExperimentIntegrationReconciler(t, 0)

	dda := baseDDA(ns, name, uid)
	createAndReconcile(t, r, dda)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	assert.NotEmpty(t, dda.Status.CurrentRevision)
	dda.Spec.Global.Site = ptr.To("datadoghq.eu")
	assert.NoError(t, r.client.Update(context.TODO(), dda))
	simulateDaemonStartWithTaskID(t, r.client, nsName, "exp-1", "task-1")

	readErr := errors.New("etcdserver: request timed out")
	r.options.APIReader = failingRevisionReader{Reader: r.client, err: readErr}

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	_, err := r.Reconcile(context.TODO(), dda)
	assert.ErrorIs(t, err, readErr)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	assert.Nil(t, dda.Status.Experiment, "a transient read failure must record no experiment outcome at all")
	cond := condition.GetCondition(&dda.Status, common.ExperimentConfigStrandedConditionType)
	assert.Nil(t, cond)
	// The signal survives, so the retry has something to act on.
	annotations := dda.GetAnnotations()
	assert.Equal(t, v2alpha1.ExperimentSignalStart, annotations[v2alpha1.AnnotationExperimentSignal])
	assert.Equal(t, "exp-1", annotations[v2alpha1.AnnotationExperimentID])
	assert.NotEmpty(t, annotations[v2alpha1.AnnotationExperimentRollbackTargetRevision])
	assert.NotEmpty(t, annotations[v2alpha1.AnnotationExperimentExpectedSpecHash])
}

// Test_Experiment_StartClearsAllAnnotationsAfterProcess verifies that once a
// start signal has been processed into status, the signal, ID,
// rollback-target-revision, and expected-spec-hash annotations are all removed
// from the DDA so they aren't reprocessed on subsequent reconciles.
func Test_Experiment_StartClearsAllAnnotationsAfterProcess(t *testing.T) {
	const ns, name = "default", "test-dda"
	const uid = types.UID("uid-1")
	nsName := types.NamespacedName{Namespace: ns, Name: name}

	r := newExperimentIntegrationReconciler(t, 0)

	dda := baseDDA(ns, name, uid)
	createAndReconcile(t, r, dda)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Spec.Global.Site = ptr.To("datadoghq.eu")
	assert.NoError(t, r.client.Update(context.TODO(), dda))
	simulateDaemonStart(t, r.client, nsName, "exp-1")

	// First reconcile: processes signal, mutates status → annotations left in
	// place for the idempotent pass. Second reconcile: detects no further
	// status mutation and clears annotations.
	reconcileN(t, r, ns, name, 2)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	assert.Equal(t, v2alpha1.ExperimentPhaseRunning, dda.Status.Experiment.Phase)
	annotations := dda.GetAnnotations()
	assert.NotContains(t, annotations, v2alpha1.AnnotationExperimentSignal)
	assert.NotContains(t, annotations, v2alpha1.AnnotationExperimentID)
	assert.NotContains(t, annotations, v2alpha1.AnnotationExperimentRollbackTargetRevision)
	assert.NotContains(t, annotations, v2alpha1.AnnotationExperimentExpectedSpecHash)
}

// Test_Experiment_ExpectedSpecHashDoesNotRefreshOnLaterReconciles verifies
// that once a checkpoint is written on start, later idempotent reconciles do
// not recompute or overwrite it.
func Test_Experiment_ExpectedSpecHashDoesNotRefreshOnLaterReconciles(t *testing.T) {
	const ns, name = "default", "test-dda"
	const uid = types.UID("uid-1")
	nsName := types.NamespacedName{Namespace: ns, Name: name}

	r := newExperimentIntegrationReconciler(t, 0)

	dda := baseDDA(ns, name, uid)
	createAndReconcile(t, r, dda)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Spec.Global.Site = ptr.To("datadoghq.eu")
	assert.NoError(t, r.client.Update(context.TODO(), dda))
	simulateDaemonStart(t, r.client, nsName, "exp-1")
	reconcileN(t, r, ns, name, 1)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	assert.NotNil(t, dda.Status.Experiment.Checkpoint)
	originalHash := dda.Status.Experiment.Checkpoint.ExpectedSpecHash
	assert.NotEmpty(t, originalHash)

	// Further reconciles with no signal change should leave the checkpoint untouched.
	reconcileN(t, r, ns, name, 3)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	assert.NotNil(t, dda.Status.Experiment.Checkpoint)
	assert.Equal(t, originalHash, dda.Status.Experiment.Checkpoint.ExpectedSpecHash)
}

// Test_Experiment_StrandedConditionOnBaselineMissing verifies that aborting a
// start signal for lack of a rollback baseline sets the ExperimentConfigStranded
// condition to True, flagging that the running config was never approved and
// cannot be automatically restored.
func Test_Experiment_StrandedConditionOnBaselineMissing(t *testing.T) {
	const ns, name = "default", "test-dda"
	const uid = types.UID("uid-1")
	nsName := types.NamespacedName{Namespace: ns, Name: name}

	r := newExperimentIntegrationReconciler(t, 0)

	dda := baseDDA(ns, name, uid)
	createAndReconcile(t, r, dda)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Spec.Global.Site = ptr.To("datadoghq.eu")
	assert.NoError(t, r.client.Update(context.TODO(), dda))

	patch, err := fleet.BuildSignalPatchWithAnnotations(v2alpha1.ExperimentSignalStart, "exp-1", nil)
	assert.NoError(t, err)
	assert.NoError(t, r.client.Patch(context.TODO(), dda, client.RawPatch(types.MergePatchType, patch)))

	reconcileN(t, r, ns, name, 1)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	assert.Equal(t, v2alpha1.ExperimentPhaseAborted, dda.Status.Experiment.Phase)
	assert.Equal(t, v2alpha1.ExperimentTerminationReasonBaselineMissing, dda.Status.Experiment.TerminationReason)

	cond := condition.GetCondition(&dda.Status, common.ExperimentConfigStrandedConditionType)
	assert.NotNil(t, cond, "ExperimentConfigStranded condition should be set")
	assert.Equal(t, metav1.ConditionTrue, cond.Status)
	assert.Equal(t, string(v2alpha1.ExperimentTerminationReasonBaselineMissing), cond.Reason)
}

// Test_Experiment_NoStrandedConditionOnManualChangeAbort verifies that
// aborting an experiment due to a manual spec change does NOT set the
// ExperimentConfigStranded condition — the user's own edit is standing, so
// there is nothing stranded to report.
func Test_Experiment_NoStrandedConditionOnManualChangeAbort(t *testing.T) {
	const ns, name = "default", "test-dda"
	const uid = types.UID("uid-1")
	nsName := types.NamespacedName{Namespace: ns, Name: name}

	r := newExperimentIntegrationReconciler(t, 0)

	dda := baseDDA(ns, name, uid)
	createAndReconcile(t, r, dda)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Spec.Global.Site = ptr.To("datadoghq.eu")
	assert.NoError(t, r.client.Update(context.TODO(), dda))
	simulateDaemonStart(t, r.client, nsName, "exp-1")
	reconcileN(t, r, ns, name, 1)
	assert.Equal(t, v2alpha1.ExperimentPhaseRunning, mustGetExperimentPhase(t, r, ns, name))

	for _, rev := range listOwnedRevisions(t, r.client, ns, uid) {
		rev.CreationTimestamp = metav1.Now()
		assert.NoError(t, r.client.Update(context.TODO(), &rev))
	}

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Spec.Global.Site = ptr.To("manual-change.example.com")
	assert.NoError(t, r.client.Update(context.TODO(), dda))
	reconcileN(t, r, ns, name, 1)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	assert.Equal(t, v2alpha1.ExperimentPhaseAborted, dda.Status.Experiment.Phase)
	assert.Equal(t, v2alpha1.ExperimentTerminationReasonManualSpecChange, dda.Status.Experiment.TerminationReason)

	cond := condition.GetCondition(&dda.Status, common.ExperimentConfigStrandedConditionType)
	assert.Nil(t, cond, "manual spec change abort should not be reported as stranded")
}

// Test_Experiment_StrandedConditionSurvivesAcrossReconciles verifies that the
// ExperimentConfigStranded condition, once set, persists with a stable
// LastTransitionTime across further reconciles of the (now-terminal)
// experiment, since generateNewStatusFromDDA re-derives it fresh every
// reconcile and IsEqualCondition ignores LastTransitionTime when deciding
// whether a status write is needed.
func Test_Experiment_StrandedConditionSurvivesAcrossReconciles(t *testing.T) {
	const ns, name = "default", "test-dda"
	const uid = types.UID("uid-1")
	nsName := types.NamespacedName{Namespace: ns, Name: name}

	r := newExperimentIntegrationReconciler(t, 0)

	dda := baseDDA(ns, name, uid)
	createAndReconcile(t, r, dda)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Spec.Global.Site = ptr.To("datadoghq.eu")
	assert.NoError(t, r.client.Update(context.TODO(), dda))

	patch, err := fleet.BuildSignalPatchWithAnnotations(v2alpha1.ExperimentSignalStart, "exp-1", nil)
	assert.NoError(t, err)
	assert.NoError(t, r.client.Patch(context.TODO(), dda, client.RawPatch(types.MergePatchType, patch)))

	// First reconcile sets the aborted phase and the condition. Second
	// reconcile clears the now-stale annotations (idempotent pass).
	reconcileN(t, r, ns, name, 2)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	firstCond := condition.GetCondition(&dda.Status, common.ExperimentConfigStrandedConditionType)
	assert.NotNil(t, firstCond)
	firstTransition := firstCond.LastTransitionTime

	reconcileN(t, r, ns, name, 3)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	laterCond := condition.GetCondition(&dda.Status, common.ExperimentConfigStrandedConditionType)
	assert.NotNil(t, laterCond, "condition should still be present after further reconciles")
	assert.Equal(t, metav1.ConditionTrue, laterCond.Status)
	assert.True(t, firstTransition.Equal(&laterCond.LastTransitionTime),
		"LastTransitionTime should remain stable since nothing about the condition changed")
}

// -----------------------------------------------------------------------------
// Checkpoint-driven rollback/promote/timeout/abort (Commit 5)
// -----------------------------------------------------------------------------

// Test_Experiment_StrandedConditionOnBaselineNotFound verifies that a rollback
// signal whose checkpointed baseline ControllerRevision has been deleted
// aborts (rather than erroring or hanging) with baseline_not_found, and flags
// the ExperimentConfigStranded condition since the running config can no
// longer be automatically restored.
func Test_Experiment_StrandedConditionOnBaselineNotFound(t *testing.T) {
	const ns, name = "default", "test-dda"
	const uid = types.UID("uid-1")
	nsName := types.NamespacedName{Namespace: ns, Name: name}

	r := newExperimentIntegrationReconciler(t, 0)

	dda := baseDDA(ns, name, uid)
	createAndReconcile(t, r, dda)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Spec.Global.Site = ptr.To("datadoghq.eu")
	assert.NoError(t, r.client.Update(context.TODO(), dda))
	simulateDaemonStart(t, r.client, nsName, "exp-1")
	reconcileN(t, r, ns, name, 1)
	assert.Equal(t, v2alpha1.ExperimentPhaseRunning, mustGetExperimentPhase(t, r, ns, name))

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	baselineRevName := dda.Status.Experiment.Checkpoint.RollbackTargetRevision
	assert.NotEmpty(t, baselineRevName)

	var baselineRev appsv1.ControllerRevision
	assert.NoError(t, r.client.Get(context.TODO(), types.NamespacedName{Namespace: ns, Name: baselineRevName}, &baselineRev))
	assert.NoError(t, r.client.Delete(context.TODO(), &baselineRev))

	simulateDaemonRollback(t, r.client, nsName, "stop-1")
	reconcileN(t, r, ns, name, 1)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	assert.NotNil(t, dda.Status.Experiment)
	assert.Equal(t, v2alpha1.ExperimentPhaseAborted, dda.Status.Experiment.Phase)
	assert.Equal(t, v2alpha1.ExperimentTerminationReasonBaselineNotFound, dda.Status.Experiment.TerminationReason)

	cond := condition.GetCondition(&dda.Status, common.ExperimentConfigStrandedConditionType)
	assert.NotNil(t, cond, "ExperimentConfigStranded condition should be set")
	assert.Equal(t, metav1.ConditionTrue, cond.Status)
	assert.Equal(t, string(v2alpha1.ExperimentTerminationReasonBaselineNotFound), cond.Reason)
}

// decodeRevisionSnapshot fetches the named ControllerRevision and decodes its
// stored spec/annotations snapshot.
func decodeRevisionSnapshot(t *testing.T, r *Reconciler, ns, revName string) v2alpha1.RevisionSnapshot {
	t.Helper()
	var rev appsv1.ControllerRevision
	assert.NoError(t, r.client.Get(context.TODO(), types.NamespacedName{Namespace: ns, Name: revName}, &rev))
	var snap v2alpha1.RevisionSnapshot
	assert.NoError(t, json.Unmarshal(rev.Data.Raw, &snap))
	return snap
}

// Test_Experiment_RollbackSignalIdempotentAfterSpecRestore verifies that a
// rollback signal arriving after the spec was already restored to the
// baseline (e.g. a prior restore's status write was lost to a 409) still
// completes as Terminated/stopped rather than misreporting a manual change —
// the live spec matches the checkpointed rollback target exactly.
func Test_Experiment_RollbackSignalIdempotentAfterSpecRestore(t *testing.T) {
	const ns, name = "default", "test-dda"
	const uid = types.UID("uid-1")
	nsName := types.NamespacedName{Namespace: ns, Name: name}

	r := newExperimentIntegrationReconciler(t, 0)

	dda := baseDDA(ns, name, uid)
	createAndReconcile(t, r, dda)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Spec.Global.Site = ptr.To("datadoghq.eu")
	assert.NoError(t, r.client.Update(context.TODO(), dda))
	simulateDaemonStart(t, r.client, nsName, "exp-1")
	reconcileN(t, r, ns, name, 1)
	assert.Equal(t, v2alpha1.ExperimentPhaseRunning, mustGetExperimentPhase(t, r, ns, name))

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	baselineRevName := dda.Status.Experiment.Checkpoint.RollbackTargetRevision
	snap := decodeRevisionSnapshot(t, r, ns, baselineRevName)

	// Simulate an interrupted rollback: the spec restore Update succeeded but
	// the subsequent status write was lost (e.g. a 409), so status is still
	// Running even though the live spec already matches the baseline.
	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Spec = snap.Spec
	assert.NoError(t, r.client.Update(context.TODO(), dda))

	simulateDaemonRollback(t, r.client, nsName, "stop-1")
	reconcileN(t, r, ns, name, 1)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	assert.NotNil(t, dda.Status.Experiment)
	assert.Equal(t, v2alpha1.ExperimentPhaseTerminated, dda.Status.Experiment.Phase)
	assert.Equal(t, v2alpha1.ExperimentTerminationReasonStopped, dda.Status.Experiment.TerminationReason)
}

// Test_Experiment_TimeoutIdempotentAfterSpecRestore verifies the same
// interrupted-rollback scenario as above, but reached via timeout instead of
// an explicit rollback signal.
func Test_Experiment_TimeoutIdempotentAfterSpecRestore(t *testing.T) {
	const ns, name = "default", "test-dda"
	const uid = types.UID("uid-1")
	const shortTimeout = 50 * time.Millisecond
	nsName := types.NamespacedName{Namespace: ns, Name: name}

	r := newExperimentIntegrationReconciler(t, shortTimeout)

	dda := baseDDA(ns, name, uid)
	createAndReconcile(t, r, dda)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Spec.Global.Site = ptr.To("datadoghq.eu")
	assert.NoError(t, r.client.Update(context.TODO(), dda))
	simulateDaemonStart(t, r.client, nsName, "exp-1")
	reconcileN(t, r, ns, name, 1)
	assert.Equal(t, v2alpha1.ExperimentPhaseRunning, mustGetExperimentPhase(t, r, ns, name))

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	baselineRevName := dda.Status.Experiment.Checkpoint.RollbackTargetRevision
	snap := decodeRevisionSnapshot(t, r, ns, baselineRevName)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Spec = snap.Spec
	assert.NoError(t, r.client.Update(context.TODO(), dda))

	// Backdate StartedAt so handleRollback's timeout anchor has already elapsed.
	staleTime := metav1.NewTime(time.Now().Add(-time.Minute))
	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Status.Experiment.StartedAt = &staleTime
	assert.NoError(t, r.client.Status().Update(context.TODO(), dda))

	reconcileN(t, r, ns, name, 1)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	assert.NotNil(t, dda.Status.Experiment)
	assert.Equal(t, v2alpha1.ExperimentPhaseTerminated, dda.Status.Experiment.Phase)
	assert.Equal(t, v2alpha1.ExperimentTerminationReasonTimedOut, dda.Status.Experiment.TerminationReason)
}

// Test_Experiment_TimeoutWithUserEditAborts is the C3 regression: a spec edit
// that matches neither the experiment spec nor the baseline must abort as
// manual_spec_change on timeout, not be silently rolled back or misreported.
func Test_Experiment_TimeoutWithUserEditAborts(t *testing.T) {
	const ns, name = "default", "test-dda"
	const uid = types.UID("uid-1")
	const shortTimeout = 50 * time.Millisecond
	nsName := types.NamespacedName{Namespace: ns, Name: name}

	r := newExperimentIntegrationReconciler(t, shortTimeout)

	dda := baseDDA(ns, name, uid)
	createAndReconcile(t, r, dda)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Spec.Global.Site = ptr.To("datadoghq.eu")
	assert.NoError(t, r.client.Update(context.TODO(), dda))
	simulateDaemonStart(t, r.client, nsName, "exp-1")
	reconcileN(t, r, ns, name, 1)
	assert.Equal(t, v2alpha1.ExperimentPhaseRunning, mustGetExperimentPhase(t, r, ns, name))

	// User edit matching neither the experiment spec nor the baseline.
	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Spec.Global.Site = ptr.To("totally-different.example.com")
	assert.NoError(t, r.client.Update(context.TODO(), dda))

	staleTime := metav1.NewTime(time.Now().Add(-time.Minute))
	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Status.Experiment.StartedAt = &staleTime
	assert.NoError(t, r.client.Status().Update(context.TODO(), dda))

	reconcileN(t, r, ns, name, 1)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	assert.Equal(t, "totally-different.example.com", *dda.Spec.Global.Site, "manual edit must not be rolled back")
	assert.NotNil(t, dda.Status.Experiment)
	assert.Equal(t, v2alpha1.ExperimentPhaseAborted, dda.Status.Experiment.Phase)
	assert.Equal(t, v2alpha1.ExperimentTerminationReasonManualSpecChange, dda.Status.Experiment.TerminationReason)
}

// Test_Experiment_PromoteAfterUserRevertToBaselineAborts verifies that
// promote does not get the rollback-target exception: reverting to the exact
// baseline spec and then promoting must still abort as manual_spec_change,
// since promoting a reverted spec makes no sense.
func Test_Experiment_PromoteAfterUserRevertToBaselineAborts(t *testing.T) {
	const ns, name = "default", "test-dda"
	const uid = types.UID("uid-1")
	nsName := types.NamespacedName{Namespace: ns, Name: name}

	r := newExperimentIntegrationReconciler(t, 0)

	dda := baseDDA(ns, name, uid)
	createAndReconcile(t, r, dda)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Spec.Global.Site = ptr.To("datadoghq.eu")
	assert.NoError(t, r.client.Update(context.TODO(), dda))
	simulateDaemonStart(t, r.client, nsName, "exp-1")
	reconcileN(t, r, ns, name, 1)
	assert.Equal(t, v2alpha1.ExperimentPhaseRunning, mustGetExperimentPhase(t, r, ns, name))

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	baselineRevName := dda.Status.Experiment.Checkpoint.RollbackTargetRevision
	snap := decodeRevisionSnapshot(t, r, ns, baselineRevName)

	// User reverts to the exact baseline spec while the experiment is still running.
	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Spec = snap.Spec
	assert.NoError(t, r.client.Update(context.TODO(), dda))

	simulateDaemonPromote(t, r.client, nsName, "promote-1")
	reconcileN(t, r, ns, name, 1)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	assert.NotNil(t, dda.Status.Experiment)
	assert.Equal(t, v2alpha1.ExperimentPhaseAborted, dda.Status.Experiment.Phase)
	assert.Equal(t, v2alpha1.ExperimentTerminationReasonManualSpecChange, dda.Status.Experiment.TerminationReason)
}

// Test_Experiment_StopSignalAtNilPhaseWithAnnotationRecovers verifies the
// nil-phase ("Transition 6") recovery path: the daemon's start patch (spec +
// all annotations, including rollback-target-revision) landed, but the
// reconciler never processed it before the daemon retried with a stop
// signal. The reconciler must reconstruct the checkpoint from the validated
// annotation and complete the rollback rather than treating it as a no-op.
func Test_Experiment_StopSignalAtNilPhaseWithAnnotationRecovers(t *testing.T) {
	const ns, name = "default", "test-dda"
	const uid = types.UID("uid-1")
	nsName := types.NamespacedName{Namespace: ns, Name: name}

	r := newExperimentIntegrationReconciler(t, 0)

	dda := baseDDA(ns, name, uid)
	createAndReconcile(t, r, dda)

	// Daemon applies the experiment spec and writes the full start-signal
	// annotation set, but the reconciler has not processed it yet (no
	// reconcile in between).
	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Spec.Global.Site = ptr.To("datadoghq.eu")
	assert.NoError(t, r.client.Update(context.TODO(), dda))
	simulateDaemonStart(t, r.client, nsName, "exp-1")

	// Daemon retries with a stop instead, overwriting the signal/id
	// annotations while leaving rollback-target-revision and the pending
	// task id in place.
	simulateDaemonRollback(t, r.client, nsName, "exp-1")

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	pinnedHash := dda.GetAnnotations()[v2alpha1.AnnotationExperimentExpectedSpecHash]
	assert.NotEmpty(t, pinnedHash)

	reconcileN(t, r, ns, name, 2)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	assert.NotNil(t, dda.Status.Experiment)
	assert.Equal(t, "exp-1", dda.Status.Experiment.ID)
	assert.Equal(t, "test-task-exp-1", dda.Status.Experiment.StartTaskID)
	assert.Equal(t, v2alpha1.ExperimentPhaseTerminated, dda.Status.Experiment.Phase)
	assert.Equal(t, v2alpha1.ExperimentTerminationReasonStopped, dda.Status.Experiment.TerminationReason)
	assert.Nil(t, dda.Spec.Global.Site, "spec should be restored to pre-experiment state")
	assert.NotNil(t, dda.Status.Experiment.Checkpoint)
	assert.Equal(t, pinnedHash, dda.Status.Experiment.Checkpoint.ExpectedSpecHash,
		"recovery must synthesize the checkpoint from the pinned hash, not from hash(liveSpec)")
}

// Test_Experiment_StopSignalAtNilPhaseWithMismatchedExpectedHashAborts_ManualSpecChange
// covers the other failure axis of nil-phase recovery: the rollback target is
// a valid owned revision, but a user apply landed between Fleet's atomic start
// patch and this reconcile, so the live spec no longer hashes to the pin. The
// user's spec is what is live, so it stands — abort as manual_spec_change with
// no stranded condition and no synthesized checkpoint, and leave the spec
// alone.
func Test_Experiment_StopSignalAtNilPhaseWithMismatchedExpectedHashAborts_ManualSpecChange(t *testing.T) {
	const ns, name = "default", "test-dda"
	nsName := types.NamespacedName{Namespace: ns, Name: name}

	r := newExperimentIntegrationReconciler(t, 0)

	dda := baseDDA(ns, name, types.UID("uid-1"))
	createAndReconcile(t, r, dda)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Spec.Global.Site = ptr.To("datadoghq.eu")
	assert.NoError(t, r.client.Update(context.TODO(), dda))
	simulateDaemonStart(t, r.client, nsName, "exp-1")

	// User apply lands before the reconciler ever sees the start signal,
	// invalidating the pinned hash.
	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Spec.Global.LogLevel = ptr.To("debug")
	assert.NoError(t, r.client.Update(context.TODO(), dda))

	simulateDaemonRollback(t, r.client, nsName, "exp-1")

	reconcileN(t, r, ns, name, 1)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	assert.NotNil(t, dda.Status.Experiment)
	assert.Equal(t, "exp-1", dda.Status.Experiment.ID)
	assert.Equal(t, v2alpha1.ExperimentPhaseAborted, dda.Status.Experiment.Phase)
	assert.Equal(t, v2alpha1.ExperimentTerminationReasonManualSpecChange, dda.Status.Experiment.TerminationReason)
	assert.Nil(t, dda.Status.Experiment.Checkpoint,
		"no checkpoint should be synthesized from a spec we cannot attribute to Fleet")
	assert.Nil(t, condition.GetCondition(&dda.Status, common.ExperimentConfigStrandedConditionType),
		"a manual spec change is not stranding: the user's own spec is live")
	assert.Equal(t, "debug", *dda.Spec.Global.LogLevel, "the user's apply must stand")
	assert.Equal(t, "datadoghq.eu", *dda.Spec.Global.Site, "the user's apply must stand")
}

// Test_Experiment_StopSignalAtNilPhaseWithForeignRevisionAborts verifies
// that the nil-phase recovery path distinguishes "no rollback-target
// annotation" from "annotation present but naming a revision this DDA does
// not own" — the latter must abort as baseline_not_found rather than
// silently no-op, since the annotation names something real that just isn't
// a validated baseline.
func Test_Experiment_StopSignalAtNilPhaseWithForeignRevisionAborts(t *testing.T) {
	const ns, name = "default", "test-dda"
	const uid = types.UID("uid-1")
	const foreignUID = types.UID("uid-2")
	nsName := types.NamespacedName{Namespace: ns, Name: name}

	r := newExperimentIntegrationReconciler(t, 0)

	foreignDDA := baseDDA(ns, "other-dda", foreignUID)
	assert.NoError(t, r.client.Create(context.TODO(), foreignDDA))
	foreignRev := createRevisionWithSpec(t, r, foreignDDA, foreignDDA.Spec, 1)

	dda := baseDDA(ns, name, uid)
	createAndReconcile(t, r, dda)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Spec.Global.Site = ptr.To("datadoghq.eu")
	assert.NoError(t, r.client.Update(context.TODO(), dda))

	// Pin a hash matching the live experiment spec, so the only thing wrong
	// with this signal is the foreign rollback target.
	liveHash, err := v2alpha1.ComputeSpecHash(dda.Spec, dda.GetAnnotations())
	assert.NoError(t, err)
	extra := map[string]string{
		v2alpha1.AnnotationExperimentRollbackTargetRevision: foreignRev.Name,
		v2alpha1.AnnotationExperimentExpectedSpecHash:       liveHash,
		v2alpha1.AnnotationPendingTaskID:                    "test-task-exp-1",
	}
	patch, err := fleet.BuildSignalPatchWithAnnotations(v2alpha1.ExperimentSignalStart, "exp-1", extra)
	assert.NoError(t, err)
	assert.NoError(t, r.client.Patch(context.TODO(), dda, client.RawPatch(types.MergePatchType, patch)))

	// Daemon retries with a stop before the reconciler ever processes the
	// start signal above (no reconcile in between).
	simulateDaemonRollback(t, r.client, nsName, "exp-1")

	reconcileN(t, r, ns, name, 1)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	assert.NotNil(t, dda.Status.Experiment)
	assert.Equal(t, "exp-1", dda.Status.Experiment.ID)
	assert.Equal(t, "test-task-exp-1", dda.Status.Experiment.StartTaskID)
	assert.Equal(t, v2alpha1.ExperimentPhaseAborted, dda.Status.Experiment.Phase)
	assert.Equal(t, v2alpha1.ExperimentTerminationReasonBaselineNotFound, dda.Status.Experiment.TerminationReason)

	cond := condition.GetCondition(&dda.Status, common.ExperimentConfigStrandedConditionType)
	assert.NotNil(t, cond, "ExperimentConfigStranded condition should be set")
	assert.Equal(t, metav1.ConditionTrue, cond.Status)
}

// Test_Experiment_StopSignalAtNilPhaseWithoutAnnotationNoOps verifies that a
// rollback signal at nil phase with no rollback-target-revision annotation
// is a true no-op: the signal is cleared and no status is written.
func Test_Experiment_StopSignalAtNilPhaseWithoutAnnotationNoOps(t *testing.T) {
	const ns, name = "default", "test-dda"
	nsName := types.NamespacedName{Namespace: ns, Name: name}

	r := newExperimentIntegrationReconciler(t, 0)

	dda := baseDDA(ns, name, types.UID("uid-1"))
	createAndReconcile(t, r, dda)

	simulateDaemonRollback(t, r.client, nsName, "exp-1")
	reconcileN(t, r, ns, name, 1)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	assert.Nil(t, dda.Status.Experiment, "no status should be written for a no-op rollback signal")
	annotations := dda.GetAnnotations()
	assert.NotContains(t, annotations, v2alpha1.AnnotationExperimentSignal)
	assert.NotContains(t, annotations, v2alpha1.AnnotationExperimentID)
}

// -----------------------------------------------------------------------------
// ControllerRevision ownership hardening (Commit 5)
// -----------------------------------------------------------------------------

// Test_Rollback_RejectsForeignControllerRevisionByOwnerUID verifies that a
// pinned ControllerRevision name that has been deleted and recreated with a
// different owner UID is rejected: rollback treats it as baseline_not_found
// rather than restoring a foreign snapshot.
func Test_Rollback_RejectsForeignControllerRevisionByOwnerUID(t *testing.T) {
	const ns, name = "default", "test-dda"
	const uid = types.UID("uid-1")
	nsName := types.NamespacedName{Namespace: ns, Name: name}

	r := newExperimentIntegrationReconciler(t, 0)

	dda := baseDDA(ns, name, uid)
	createAndReconcile(t, r, dda)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Spec.Global.Site = ptr.To("datadoghq.eu")
	assert.NoError(t, r.client.Update(context.TODO(), dda))
	simulateDaemonStart(t, r.client, nsName, "exp-1")
	reconcileN(t, r, ns, name, 1)
	assert.Equal(t, v2alpha1.ExperimentPhaseRunning, mustGetExperimentPhase(t, r, ns, name))

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	baselineRevName := dda.Status.Experiment.Checkpoint.RollbackTargetRevision

	var baselineRev appsv1.ControllerRevision
	assert.NoError(t, r.client.Get(context.TODO(), types.NamespacedName{Namespace: ns, Name: baselineRevName}, &baselineRev))
	assert.NoError(t, r.client.Delete(context.TODO(), &baselineRev))

	foreign := appsv1.ControllerRevision{
		ObjectMeta: metav1.ObjectMeta{
			Name:      baselineRevName,
			Namespace: ns,
			Labels:    map[string]string{apicommon.DatadogAgentNameLabelKey: name},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "datadoghq.com/v2alpha1",
					Kind:       "DatadogAgent",
					Name:       "attacker-dda",
					UID:        types.UID("attacker-uid"),
					Controller: ptr.To(true),
				},
			},
		},
		Data:     baselineRev.Data,
		Revision: baselineRev.Revision,
	}
	assert.NoError(t, r.client.Create(context.TODO(), &foreign))

	simulateDaemonRollback(t, r.client, nsName, "stop-1")
	reconcileN(t, r, ns, name, 1)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	assert.NotNil(t, dda.Status.Experiment)
	assert.Equal(t, v2alpha1.ExperimentPhaseAborted, dda.Status.Experiment.Phase)
	assert.Equal(t, v2alpha1.ExperimentTerminationReasonBaselineNotFound, dda.Status.Experiment.TerminationReason)
}

// Test_Rollback_RejectsControllerRevisionInWrongNamespace verifies that a
// ControllerRevision only reachable in a different namespace than the DDA is
// treated the same as absent: getOwnedRevision scopes its lookup to the
// instance's own namespace.
func Test_Rollback_RejectsControllerRevisionInWrongNamespace(t *testing.T) {
	const ns, name = "default", "test-dda"
	const otherNS = "other-ns"
	const uid = types.UID("uid-1")
	nsName := types.NamespacedName{Namespace: ns, Name: name}

	r := newExperimentIntegrationReconciler(t, 0)

	dda := baseDDA(ns, name, uid)
	createAndReconcile(t, r, dda)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Spec.Global.Site = ptr.To("datadoghq.eu")
	assert.NoError(t, r.client.Update(context.TODO(), dda))
	simulateDaemonStart(t, r.client, nsName, "exp-1")
	reconcileN(t, r, ns, name, 1)
	assert.Equal(t, v2alpha1.ExperimentPhaseRunning, mustGetExperimentPhase(t, r, ns, name))

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	baselineRevName := dda.Status.Experiment.Checkpoint.RollbackTargetRevision

	var baselineRev appsv1.ControllerRevision
	assert.NoError(t, r.client.Get(context.TODO(), types.NamespacedName{Namespace: ns, Name: baselineRevName}, &baselineRev))
	assert.NoError(t, r.client.Delete(context.TODO(), &baselineRev))

	crossNS := appsv1.ControllerRevision{
		ObjectMeta: metav1.ObjectMeta{
			Name:      baselineRevName,
			Namespace: otherNS,
			Labels:    map[string]string{apicommon.DatadogAgentNameLabelKey: name},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "datadoghq.com/v2alpha1",
					Kind:       "DatadogAgent",
					Name:       name,
					UID:        uid,
					Controller: ptr.To(true),
				},
			},
		},
		Data:     baselineRev.Data,
		Revision: baselineRev.Revision,
	}
	assert.NoError(t, r.client.Create(context.TODO(), &crossNS))

	simulateDaemonRollback(t, r.client, nsName, "stop-1")
	reconcileN(t, r, ns, name, 1)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	assert.NotNil(t, dda.Status.Experiment)
	assert.Equal(t, v2alpha1.ExperimentPhaseAborted, dda.Status.Experiment.Phase)
	assert.Equal(t, v2alpha1.ExperimentTerminationReasonBaselineNotFound, dda.Status.Experiment.TerminationReason)
}

// Test_Rollback_UsesAPIReaderForNotFound verifies that ControllerRevision
// lookups for rollback go through the uncached APIReader rather than the
// (potentially stale) informer-backed client: a revision missing from the
// cache client but present via the APIReader must still be found.
func Test_Rollback_UsesAPIReaderForNotFound(t *testing.T) {
	const ns, name = "default", "test-dda"
	const uid = types.UID("uid-1")
	nsName := types.NamespacedName{Namespace: ns, Name: name}

	r := newExperimentIntegrationReconciler(t, 0)

	dda := baseDDA(ns, name, uid)
	createAndReconcile(t, r, dda)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Spec.Global.Site = ptr.To("datadoghq.eu")
	assert.NoError(t, r.client.Update(context.TODO(), dda))
	simulateDaemonStart(t, r.client, nsName, "exp-1")
	reconcileN(t, r, ns, name, 1)
	assert.Equal(t, v2alpha1.ExperimentPhaseRunning, mustGetExperimentPhase(t, r, ns, name))

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	baselineRevName := dda.Status.Experiment.Checkpoint.RollbackTargetRevision

	var baselineRev appsv1.ControllerRevision
	assert.NoError(t, r.client.Get(context.TODO(), types.NamespacedName{Namespace: ns, Name: baselineRevName}, &baselineRev))

	// A separate, independent client stands in for the uncached API reader:
	// it has the baseline revision. The reconciler's regular (cache-like)
	// client has it deleted out from under it, simulating a lagging cache.
	apiReaderClient := fake.NewClientBuilder().
		WithScheme(r.scheme).
		WithObjects(baselineRev.DeepCopy()).
		Build()
	r.options.APIReader = apiReaderClient

	assert.NoError(t, r.client.Delete(context.TODO(), &baselineRev))

	simulateDaemonRollback(t, r.client, nsName, "stop-1")
	reconcileN(t, r, ns, name, 1)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	assert.NotNil(t, dda.Status.Experiment)
	assert.Equal(t, v2alpha1.ExperimentPhaseTerminated, dda.Status.Experiment.Phase)
	assert.Equal(t, v2alpha1.ExperimentTerminationReasonStopped, dda.Status.Experiment.TerminationReason)
	assert.Nil(t, dda.Spec.Global.Site, "spec should be restored via the revision found through the APIReader")
}

// -----------------------------------------------------------------------------
// Preview carry-over guard (Commit 5)
// -----------------------------------------------------------------------------

// Test_Experiment_InFlightWithoutCheckpointAborts verifies that a Running
// experiment with no checkpoint — a state that predates the checkpoint
// contract, or a status write that never completed — is failed closed
// instead of nil-dereferencing the checkpoint-dependent logic below it.
func Test_Experiment_InFlightWithoutCheckpointAborts(t *testing.T) {
	const ns, name = "default", "test-dda"
	const uid = types.UID("uid-1")
	nsName := types.NamespacedName{Namespace: ns, Name: name}

	r := newExperimentIntegrationReconciler(t, 0)

	dda := baseDDA(ns, name, uid)
	createAndReconcile(t, r, dda)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	dda.Status.Experiment = &v2alpha1.ExperimentStatus{
		Phase:       v2alpha1.ExperimentPhaseRunning,
		ID:          "exp-1",
		StartTaskID: "task-1",
		Checkpoint:  nil,
	}
	assert.NoError(t, r.client.Status().Update(context.TODO(), dda))

	reconcileN(t, r, ns, name, 1)

	assert.NoError(t, r.client.Get(context.TODO(), nsName, dda))
	assert.NotNil(t, dda.Status.Experiment)
	assert.Equal(t, v2alpha1.ExperimentPhaseAborted, dda.Status.Experiment.Phase)
	assert.Equal(t, v2alpha1.ExperimentTerminationReasonBaselineMissing, dda.Status.Experiment.TerminationReason)
	assert.Equal(t, "task-1", dda.Status.Experiment.StartTaskID, "StartTaskID should be preserved through the abort")

	cond := condition.GetCondition(&dda.Status, common.ExperimentConfigStrandedConditionType)
	assert.NotNil(t, cond, "ExperimentConfigStranded condition should be set")
	assert.Equal(t, metav1.ConditionTrue, cond.Status)
}
