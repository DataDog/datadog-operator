// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package experiment

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

// HandleExperimentLifecycle is the main hook called from the reconciler.
// It manages ControllerRevision creation, revision pointer updates, and experiment phase transitions.
// Returns (shouldReturn, result, err). shouldReturn=true means the reconciler should return early.
func HandleExperimentLifecycle(ctx context.Context, c client.Client, dda *v2alpha1.DatadogAgent, scheme *runtime.Scheme, now time.Time, timeout time.Duration) (bool, reconcile.Result, error) {
	// Step 1: Handle terminal phases and rollback/restore phases.
	// These run BEFORE revision creation.
	if dda.Status.Experiment != nil {
		switch dda.Status.Experiment.Phase {
		case v2alpha1.ExperimentPhaseRollback:
			if dda.Status.Experiment.BaselineRevision != "" {
				// First reconcile after stopExperiment: restore spec from baseline.
				// handleRestore clears baselineRevision so the next reconcile
				// knows the restore already happened.
				return handleRestore(ctx, c, dda, v2alpha1.ExperimentPhaseRollback)
			}
			// Second reconcile: restore already done, clear experiment.
			dda.Status.Experiment = nil

		case v2alpha1.ExperimentPhaseTimeout:
			if dda.Status.Experiment.BaselineRevision != "" {
				// Same as rollback: first reconcile after timeout detection.
				// This path is entered when the timeout phase was persisted on
				// a prior early return that triggered the restore.
				return handleRestore(ctx, c, dda, v2alpha1.ExperimentPhaseTimeout)
			}
			// Second reconcile: restore already done, clear experiment.
			dda.Status.Experiment = nil

		case v2alpha1.ExperimentPhasePromoted:
			// Promoted: clear experiment state, continue to revision tracking.
			dda.Status.Experiment = nil

		case v2alpha1.ExperimentPhaseAborted:
			// Aborted: no action needed. Experiment stays in aborted state
			// until FA acknowledges (or a new experiment starts).
		}
	}

	// Step 2: Create ControllerRevision for current spec if it changed.
	revName, created, err := CreateControllerRevision(ctx, c, dda, scheme)
	if err != nil {
		return false, reconcile.Result{}, err
	}

	// Step 3: Update revision pointers if spec changed.
	// Save the old currentRevision BEFORE updating — needed for conflict detection.
	prevCurrentRevision := dda.Status.CurrentRevision
	specChanged := false
	if created || dda.Status.CurrentRevision == "" {
		if dda.Status.CurrentRevision != "" && dda.Status.CurrentRevision != revName {
			dda.Status.PreviousRevision = dda.Status.CurrentRevision
			specChanged = true
		}
		dda.Status.CurrentRevision = revName
	} else if dda.Status.CurrentRevision != revName {
		dda.Status.PreviousRevision = dda.Status.CurrentRevision
		dda.Status.CurrentRevision = revName
		specChanged = true
	}

	// Step 4: Handle running experiment (timeout + conflict detection).
	if dda.Status.Experiment != nil && dda.Status.Experiment.Phase == v2alpha1.ExperimentPhaseRunning {
		shouldReturn, result, err := handleRunning(ctx, c, dda, prevCurrentRevision, specChanged, now, timeout)
		if err != nil || shouldReturn {
			return shouldReturn, result, err
		}
	}

	// Step 5: GC old revisions
	keep := BuildKeepSet(&dda.Status)
	if err := GarbageCollectRevisions(ctx, c, dda, keep); err != nil {
		return false, reconcile.Result{}, err
	}

	return false, reconcile.Result{}, nil
}

// handleRestore restores the DDA spec from the baseline revision, then
// persists the terminal experiment phase directly via a status update.
//
// The spec restore (client.Update) bumps the resourceVersion, so we must
// re-fetch the object before writing the status. This avoids the conflict
// that would occur if the caller tried to persist status using a stale copy.
func handleRestore(ctx context.Context, c client.Client, dda *v2alpha1.DatadogAgent, phase v2alpha1.ExperimentPhase) (bool, reconcile.Result, error) {
	exp := dda.Status.Experiment
	if exp == nil || exp.BaselineRevision == "" {
		return false, reconcile.Result{}, nil
	}

	if err := RestoreSpecFromRevision(ctx, c, dda, exp.BaselineRevision); err != nil {
		return false, reconcile.Result{}, err
	}

	// Re-fetch to get the current resourceVersion after the spec update.
	refreshed := dda.DeepCopy()
	if err := c.Get(ctx, client.ObjectKeyFromObject(dda), refreshed); err != nil {
		return false, reconcile.Result{}, fmt.Errorf("failed to re-fetch DDA after spec restore: %w", err)
	}

	// Set terminal phase and clear baselineRevision to signal the restore is done.
	refreshed.Status.Experiment = &v2alpha1.ExperimentStatus{
		Phase: phase,
		ID:    exp.ID,
	}
	if err := c.Status().Update(ctx, refreshed); err != nil {
		return false, reconcile.Result{}, fmt.Errorf("failed to persist experiment status after restore: %w", err)
	}

	// Update the in-memory object so the caller sees the persisted state.
	dda.Status.Experiment = refreshed.Status.Experiment

	return true, reconcile.Result{Requeue: true}, nil
}

// handleRunning checks for timeout and conflict during a running experiment.
// prevCurrentRevision is the currentRevision value before this reconcile updated it.
// specChanged is true if the spec hash differs from prevCurrentRevision.
func handleRunning(ctx context.Context, c client.Client, dda *v2alpha1.DatadogAgent, prevCurrentRevision string, specChanged bool, now time.Time, timeout time.Duration) (bool, reconcile.Result, error) {
	exp := dda.Status.Experiment

	// Set startedAt if not already set (first reconcile after experiment start)
	if exp.StartedAt == nil {
		mt := metav1.NewTime(now)
		exp.StartedAt = &mt
	}

	// Check timeout — restore baseline and set phase=timeout
	if CheckTimeout(exp, now, timeout) {
		return handleRestore(ctx, c, dda, v2alpha1.ExperimentPhaseTimeout)
	}

	// Check conflict: if the spec changed during a running experiment, determine
	// whether it's the expected FA-owned mutation or an external edit.
	if specChanged && prevCurrentRevision != "" {
		if exp.ExpectedSpecHash != "" {
			// ExpectedSpecHash is set — this is the first spec change since
			// startExperiment. Verify it matches what FA sent.
			currentHash, err := ComputeSpecHash(&dda.Spec)
			if err != nil {
				return false, reconcile.Result{}, err
			}
			if currentHash != exp.ExpectedSpecHash {
				// Spec doesn't match FA payload — user edited after RC patched
				exp.Phase = v2alpha1.ExperimentPhaseAborted
				exp.StartedAt = nil
				return false, reconcile.Result{}, nil
			}
			// Spec matches FA payload — expected mutation, clear the hash
			exp.ExpectedSpecHash = ""
		} else {
			// ExpectedSpecHash already cleared (validated on a prior reconcile)
			// — any subsequent spec change is an external edit.
			exp.Phase = v2alpha1.ExperimentPhaseAborted
			exp.StartedAt = nil
			return false, reconcile.Result{}, nil
		}
	}
	// When specChanged is false and ExpectedSpecHash is still set, keep it.
	// The RC status update (which sets ExpectedSpecHash) arrives before the
	// spec patch, so the first reconcile may see the hash but not the spec
	// change yet. The hash must survive until the spec patch arrives.

	return false, reconcile.Result{}, nil
}

// CheckTimeout returns true if the experiment has exceeded the timeout duration.
func CheckTimeout(exp *v2alpha1.ExperimentStatus, now time.Time, timeout time.Duration) bool {
	if exp == nil || exp.StartedAt == nil {
		return false
	}
	return now.Sub(exp.StartedAt.Time) >= timeout
}

// CheckConflict returns true if the current DDA spec hash differs from the
// ControllerRevision pointed to by currentRevision (external edit detected).
func CheckConflict(ctx context.Context, c client.Client, dda *v2alpha1.DatadogAgent) (bool, error) {
	if dda.Status.CurrentRevision == "" {
		return false, nil
	}

	currentHash, err := ComputeSpecHash(&dda.Spec)
	if err != nil {
		return false, err
	}

	currentRevName := RevisionName(dda.Name, currentHash)
	return currentRevName != dda.Status.CurrentRevision, nil
}

// BuildKeepSet returns the set of revision names that should be protected from GC.
func BuildKeepSet(status *v2alpha1.DatadogAgentStatus) map[string]bool {
	keep := make(map[string]bool)
	if status.CurrentRevision != "" {
		keep[status.CurrentRevision] = true
	}
	if status.PreviousRevision != "" {
		keep[status.PreviousRevision] = true
	}
	if status.Experiment != nil && status.Experiment.BaselineRevision != "" {
		keep[status.Experiment.BaselineRevision] = true
	}
	return keep
}
