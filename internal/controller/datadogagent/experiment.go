// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	v2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

// ExperimentDefaultTimeout is the duration after which a running experiment is automatically rolled back.
const ExperimentDefaultTimeout = 15 * time.Minute

// manageExperiment handles all experiment state transitions for a reconcile cycle.
// Must be called before manageRevision.
func (r *Reconciler) manageExperiment(
	ctx context.Context,
	instance *v2alpha1.DatadogAgent,
	newStatus *v2alpha1.DatadogAgentStatus,
	now metav1.Time,
	revList []appsv1.ControllerRevision,
) error {
	experiment := instance.Status.Experiment
	if experiment == nil {
		return nil
	}

	ctx = ctrl.LoggerInto(ctx, ctrl.LoggerFrom(ctx).WithValues("experimentID", experiment.ID))

	if err := r.handleRollback(ctx, instance, newStatus, now, revList); err != nil {
		return err
	}
	abortExperiment(ctx, instance.Generation, experiment, newStatus)
	return nil
}

// abortExperiment marks the experiment as aborted in newStatus if a manual spec
// change is detected (DDA generation differs from the recorded experiment generation).
func abortExperiment(
	ctx context.Context,
	instanceGeneration int64,
	experiment *v2alpha1.ExperimentStatus,
	newStatus *v2alpha1.DatadogAgentStatus,
) {
	if experiment.Phase != v2alpha1.ExperimentPhaseRunning {
		return
	}
	if experiment.Generation == 0 || instanceGeneration == experiment.Generation {
		return
	}
	ctrl.LoggerFrom(ctx).Info("Aborting experiment due to manual spec change")
	newStatus.Experiment.Phase = v2alpha1.ExperimentPhaseAborted
}

// handleRollback checks if the experiment needs rollback (explicit stop or timeout).
func (r *Reconciler) handleRollback(
	ctx context.Context,
	instance *v2alpha1.DatadogAgent,
	newStatus *v2alpha1.DatadogAgentStatus,
	now metav1.Time,
	revisions []appsv1.ControllerRevision,
) error {
	logger := ctrl.LoggerFrom(ctx)
	if instance.Status.Experiment == nil {
		return nil
	}

	phase := instance.Status.Experiment.Phase

	switch {
	// stopped signal from RC: restore DDA spec and ack by setting phase to rollback.
	case phase == v2alpha1.ExperimentPhaseStopped:
		logger.Info("Experiment stopped, rolling back")
		return r.restorePreviousSpec(ctx, instance.ObjectMeta, newStatus, revisions, v2alpha1.ExperimentPhaseRollback)
	case phase == v2alpha1.ExperimentPhaseRunning:
		rev := findMostRecentRevision(revisions)
		if rev != nil {
			elapsed := now.Sub(rev.CreationTimestamp.Time)
			if elapsed >= getExperimentTimeout(r.options.ExperimentTimeout) {
				logger.Info("Experiment timed out, rolling back", "elapsed", elapsed.String())
				return r.restorePreviousSpec(ctx, instance.ObjectMeta, newStatus, revisions, v2alpha1.ExperimentPhaseTimeout)
			}
		}
	}

	return nil
}

// restorePreviousSpec sets the terminal experiment phase and restores the DDA spec
// from the previous ControllerRevision.
func (r *Reconciler) restorePreviousSpec(
	ctx context.Context,
	instanceMeta metav1.ObjectMeta,
	newStatus *v2alpha1.DatadogAgentStatus,
	revisions []appsv1.ControllerRevision,
	terminalPhase v2alpha1.ExperimentPhase,
) error {
	newStatus.Experiment.Phase = terminalPhase
	return r.rollback(ctx, instanceMeta, findRollbackTarget(revisions))
}

// rollback restores the DDA spec from the named ControllerRevision.
func (r *Reconciler) rollback(
	ctx context.Context,
	instanceMeta metav1.ObjectMeta,
	rollbackTarget string,
) error {
	if rollbackTarget == "" {
		return fmt.Errorf("no previous revision to roll back to")
	}

	cr := &appsv1.ControllerRevision{}
	if err := r.client.Get(ctx, types.NamespacedName{Namespace: instanceMeta.Namespace, Name: rollbackTarget}, cr); err != nil {
		return fmt.Errorf("failed to get previous ControllerRevision %s: %w", rollbackTarget, err)
	}

	var snapshot revisionSnapshot
	if err := json.Unmarshal(cr.Data.Raw, &snapshot); err != nil {
		return fmt.Errorf("failed to decode ControllerRevision data: %w", err)
	}

	// Re-fetch for the latest ResourceVersion and to check whether the spec is
	// rolled back already. If it is, skip the update.
	current := &v2alpha1.DatadogAgent{}
	if err := r.client.Get(ctx, types.NamespacedName{Namespace: instanceMeta.Namespace, Name: instanceMeta.Name}, current); err != nil {
		return fmt.Errorf("failed to get current DDA for rollback: %w", err)
	}
	currentSnap, err := json.Marshal(revisionSnapshot{Spec: current.Spec, Annotations: datadogAnnotations(current.GetAnnotations())})
	if err != nil {
		return fmt.Errorf("failed to marshal current snapshot for comparison: %w", err)
	}
	if bytes.Equal(currentSnap, cr.Data.Raw) {
		ctrl.LoggerFrom(ctx).Info("Rollback spec already matches target, skipping update", "rollbackTarget", rollbackTarget)
		return nil
	}

	toUpdate := &v2alpha1.DatadogAgent{
		ObjectMeta: current.ObjectMeta,
		Spec:       snapshot.Spec,
	}
	toUpdate.Annotations = snapshot.Annotations
	return r.client.Update(ctx, toUpdate)
}

// findRollbackTarget returns the name of the previous ControllerRevision to restore.
// GC keeps at most two revisions (current and previous), so this returns whichever
// revision has the lower revision number.
func findRollbackTarget(revisions []appsv1.ControllerRevision) string {
	var curRev, prevRev int64 = -1, -1
	var curName, prevName string
	for i := range revisions {
		rev := &revisions[i]
		if rev.Revision > curRev {
			prevRev, prevName = curRev, curName
			curRev, curName = rev.Revision, rev.Name
		} else if rev.Revision > prevRev {
			prevRev, prevName = rev.Revision, rev.Name
		}
	}
	return prevName
}

// findMostRecentRevision returns the ControllerRevision with the highest Revision number,
// or nil if the list is empty. Used to determine the experiment start time for timeout checks.
func findMostRecentRevision(revisions []appsv1.ControllerRevision) *appsv1.ControllerRevision {
	var result *appsv1.ControllerRevision
	for i := range revisions {
		if result == nil || revisions[i].Revision > result.Revision {
			result = &revisions[i]
		}
	}
	return result
}

func getExperimentTimeout(timeout time.Duration) time.Duration {
	if timeout == 0 {
		return ExperimentDefaultTimeout
	}
	return timeout
}
