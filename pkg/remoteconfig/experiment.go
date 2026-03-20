// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package remoteconfig

import (
	"context"
	"encoding/json"
	"fmt"

	"dario.cat/mergo"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kubeclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/defaults"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/experiment"
)

// ExperimentAction represents the type of experiment signal from Fleet Automation.
type ExperimentAction string

const (
	// ExperimentActionStart signals the operator to start a new experiment.
	ExperimentActionStart ExperimentAction = "startExperiment"
	// ExperimentActionStop signals the operator to stop (rollback) an experiment.
	ExperimentActionStop ExperimentAction = "stopExperiment"
	// ExperimentActionPromote signals the operator to promote an experiment.
	ExperimentActionPromote ExperimentAction = "promoteExperiment"
)

// ExperimentSignal is the RC payload for Fleet Automation experiment signals.
type ExperimentSignal struct {
	// Action is the experiment command: startExperiment, stopExperiment, promoteExperiment.
	Action ExperimentAction `json:"action"`
	// ExperimentID is the unique ID for this experiment, set by FA.
	ExperimentID string `json:"experiment_id"`
	// Config is the DDA spec patch to apply (only present for startExperiment).
	Config *v2alpha1.DatadogAgentSpec `json:"config,omitempty"`
}

// parseExperimentSignal attempts to parse an RC payload as an experiment signal.
// Returns nil if the payload is not an experiment signal (i.e., a regular agent config).
func parseExperimentSignal(data []byte) (*ExperimentSignal, error) {
	// Try to detect if this is an experiment signal by checking for the "action" field
	var probe struct {
		Action string `json:"action"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return nil, fmt.Errorf("failed to unmarshal RC payload: %w", err)
	}
	if probe.Action == "" {
		return nil, nil // Regular agent config, not an experiment signal
	}

	signal := &ExperimentSignal{}
	if err := json.Unmarshal(data, signal); err != nil {
		return nil, fmt.Errorf("failed to unmarshal experiment signal: %w", err)
	}

	switch signal.Action {
	case ExperimentActionStart, ExperimentActionStop, ExperimentActionPromote:
		return signal, nil
	default:
		return nil, fmt.Errorf("unknown experiment action: %s", signal.Action)
	}
}

// handleExperimentSignal processes an experiment signal from Fleet Automation.
// It updates the DDA status and (for startExperiment) patches the DDA spec.
func (r *RemoteConfigUpdater) handleExperimentSignal(ctx context.Context, signal *ExperimentSignal) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	ddaList := &v2alpha1.DatadogAgentList{}
	if err := r.kubeClient.List(ctx, ddaList); err != nil {
		return fmt.Errorf("unable to list DatadogAgents: %w", err)
	}
	if len(ddaList.Items) == 0 {
		return fmt.Errorf("cannot find any DatadogAgent")
	}
	dda := ddaList.Items[0]

	if signal.ExperimentID == "" {
		return fmt.Errorf("experiment signal missing experiment_id")
	}

	switch signal.Action {
	case ExperimentActionStart:
		return r.handleStartExperiment(ctx, &dda, signal)
	case ExperimentActionStop:
		return r.handleStopExperiment(ctx, &dda, signal)
	case ExperimentActionPromote:
		return r.handlePromoteExperiment(ctx, &dda, signal)
	default:
		return fmt.Errorf("unknown experiment action: %s", signal.Action)
	}
}

// handleStartExperiment sets experiment status to running and patches DDA spec.
// Order: status first (with baselineRevision), then spec patch.
func (r *RemoteConfigUpdater) handleStartExperiment(ctx context.Context, dda *v2alpha1.DatadogAgent, signal *ExperimentSignal) error {
	if signal.Config == nil {
		return fmt.Errorf("startExperiment signal missing config payload")
	}

	// Reject if no baseline exists yet (DDA hasn't been reconciled).
	// Without a baseline, rollback/timeout can't restore the previous spec.
	if dda.Status.CurrentRevision == "" {
		return fmt.Errorf("cannot start experiment: no currentRevision set (DDA not yet reconciled)")
	}

	// Guard against starting an experiment while one is actively in progress.
	// Terminal phases (aborted, timeout, promoted) are safe to overwrite —
	// these experiments are done and a new start should clear them.
	// Active phases (running, rollback) must be resolved first.
	if dda.Status.Experiment != nil {
		exp := dda.Status.Experiment
		switch exp.Phase {
		case v2alpha1.ExperimentPhaseAborted, v2alpha1.ExperimentPhaseTimeout, v2alpha1.ExperimentPhasePromoted:
			// Terminal — safe to start a new experiment, will overwrite
			r.logger.Info("Starting new experiment, replacing terminal experiment",
				"oldID", exp.ID, "oldPhase", exp.Phase, "newID", signal.ExperimentID)

		case v2alpha1.ExperimentPhaseRunning:
			// Allow retry of the same experiment if spec wasn't applied yet
			if exp.ExpectedSpecHash != "" && signal.ExperimentID == exp.ID {
				r.logger.Info("Retrying startExperiment (prior attempt may have failed during spec patch)",
					"id", signal.ExperimentID)
			} else {
				return fmt.Errorf("cannot start experiment %s: experiment %s is active (phase=%s)",
					signal.ExperimentID, exp.ID, exp.Phase)
			}

		case v2alpha1.ExperimentPhaseRollback:
			// Rollback in progress — must complete before starting new experiment
			return fmt.Errorf("cannot start experiment %s: experiment %s is active (phase=%s)",
				signal.ExperimentID, exp.ID, exp.Phase)
		}
	}

	// Step 1: Update status — set phase=running, lock baseline.
	// ExpectedSpecHash is computed later from the refreshed spec to avoid races.
	if err := r.setExperimentStatus(ctx, dda, &v2alpha1.ExperimentStatus{
		Phase:            v2alpha1.ExperimentPhaseRunning,
		BaselineRevision: dda.Status.CurrentRevision,
		ID:               signal.ExperimentID,
	}); err != nil {
		return fmt.Errorf("failed to set experiment status for start: %w", err)
	}

	// Step 2: Re-fetch DDA to get the latest spec and resourceVersion.
	// This ensures we merge into the most recent spec, not a stale copy
	// that could silently overwrite concurrent edits.
	refreshed := &v2alpha1.DatadogAgent{}
	if err := r.kubeClient.Get(ctx, kubeclient.ObjectKeyFromObject(dda), refreshed); err != nil {
		return fmt.Errorf("failed to re-fetch DDA after status update: %w", err)
	}

	// Step 3: Merge FA patch into the refreshed spec.
	if err := mergo.Merge(&refreshed.Spec, signal.Config, mergo.WithOverride); err != nil {
		return fmt.Errorf("failed to merge experiment config into current spec: %w", err)
	}

	// Step 4: Compute ExpectedSpecHash from the merged+defaulted spec and
	// update the experiment status with it.
	defaultedSpec := refreshed.Spec.DeepCopy()
	defaults.DefaultDatadogAgentSpec(defaultedSpec)
	specHash, err := experiment.ComputeSpecHash(defaultedSpec)
	if err != nil {
		return fmt.Errorf("failed to compute spec hash for experiment config: %w", err)
	}
	if err := r.statusUpdateWithRetry(ctx, refreshed, func(d *v2alpha1.DatadogAgent) {
		if d.Status.Experiment != nil {
			d.Status.Experiment.ExpectedSpecHash = specHash
		}
	}); err != nil {
		return fmt.Errorf("failed to update ExpectedSpecHash: %w", err)
	}

	// Step 5: Re-fetch again since status update bumped resourceVersion,
	// then apply the merged spec.
	if err := r.kubeClient.Get(ctx, kubeclient.ObjectKeyFromObject(dda), refreshed); err != nil {
		return fmt.Errorf("failed to re-fetch DDA after hash update: %w", err)
	}
	// Re-merge into the latest spec (in case it changed during hash update)
	if err := mergo.Merge(&refreshed.Spec, signal.Config, mergo.WithOverride); err != nil {
		return fmt.Errorf("failed to re-merge experiment config: %w", err)
	}
	if err := r.kubeClient.Update(ctx, refreshed); err != nil {
		return fmt.Errorf("failed to update DDA spec for startExperiment: %w", err)
	}

	r.logger.Info("Started experiment", "id", signal.ExperimentID)
	return nil
}

// handleStopExperiment sets experiment status to rollback.
// The reconciler will detect this and restore from baselineRevision.
func (r *RemoteConfigUpdater) handleStopExperiment(ctx context.Context, dda *v2alpha1.DatadogAgent, signal *ExperimentSignal) error {
	if valid, err := r.validateExperimentSignal(dda, signal, "stopExperiment"); err != nil {
		return err
	} else if !valid {
		return nil
	}

	if err := r.setExperimentStatus(ctx, dda, &v2alpha1.ExperimentStatus{
		Phase:            v2alpha1.ExperimentPhaseRollback,
		BaselineRevision: dda.Status.Experiment.BaselineRevision,
		ID:               dda.Status.Experiment.ID,
	}); err != nil {
		return fmt.Errorf("failed to set experiment status for stop: %w", err)
	}

	r.logger.Info("Stopped experiment", "id", signal.ExperimentID)
	return nil
}

// handlePromoteExperiment sets experiment status to promoted.
// The reconciler will detect this and clear experiment state.
func (r *RemoteConfigUpdater) handlePromoteExperiment(ctx context.Context, dda *v2alpha1.DatadogAgent, signal *ExperimentSignal) error {
	if valid, err := r.validateExperimentSignal(dda, signal, "promoteExperiment"); err != nil {
		return err
	} else if !valid {
		return nil
	}

	if err := r.setExperimentStatus(ctx, dda, &v2alpha1.ExperimentStatus{
		Phase: v2alpha1.ExperimentPhasePromoted,
		ID:    dda.Status.Experiment.ID,
		// BaselineRevision and StartedAt cleared — reconciler will nil out experiment
	}); err != nil {
		return fmt.Errorf("failed to set experiment status for promote: %w", err)
	}

	r.logger.Info("Promoted experiment", "id", signal.ExperimentID)
	return nil
}

// validateExperimentSignal checks that a stop/promote signal targets the
// currently running experiment. Returns (true, nil) if valid. Returns
// (false, nil) for stale/mismatched signals — these are logged and silently
// dropped so they don't block processing of other RC updates.
func (r *RemoteConfigUpdater) validateExperimentSignal(dda *v2alpha1.DatadogAgent, signal *ExperimentSignal, action string) (bool, error) {
	if dda.Status.Experiment == nil || dda.Status.Experiment.Phase != v2alpha1.ExperimentPhaseRunning {
		r.logger.Info(fmt.Sprintf("Ignoring %s: no running experiment", action),
			"currentPhase", experimentPhase(dda))
		return false, nil
	}
	if signal.ExperimentID != "" && dda.Status.Experiment.ID != "" &&
		signal.ExperimentID != dda.Status.Experiment.ID {
		r.logger.Info(fmt.Sprintf("Ignoring %s: experiment ID mismatch", action),
			"signalID", signal.ExperimentID,
			"runningID", dda.Status.Experiment.ID)
		return false, nil
	}
	return true, nil
}

// statusUpdateWithRetry applies a status mutation to the DDA and persists it.
// On conflict errors, it re-fetches the DDA and retries (up to 3 times) to
// handle concurrent status writes from the reconciler. The mutate function
// is called on each attempt with the latest DDA to apply the desired change.
func (r *RemoteConfigUpdater) statusUpdateWithRetry(ctx context.Context, dda *v2alpha1.DatadogAgent, mutate func(*v2alpha1.DatadogAgent)) error {
	const maxRetries = 3
	for i := range maxRetries {
		ddaUpdate := dda.DeepCopy()
		mutate(ddaUpdate)

		if apiequality.Semantic.DeepEqual(&dda.Status, &ddaUpdate.Status) {
			return nil // No change
		}

		updateErr := r.kubeClient.Status().Update(ctx, ddaUpdate)
		if updateErr == nil {
			dda.Status = ddaUpdate.Status
			return nil
		}
		if !apierrors.IsConflict(updateErr) {
			return updateErr
		}
		r.logger.Info("Status update conflict, retrying",
			"attempt", i+1, "maxRetries", maxRetries)
		if getErr := r.kubeClient.Get(ctx, kubeclient.ObjectKeyFromObject(dda), dda); getErr != nil {
			return fmt.Errorf("failed to re-fetch DDA after status conflict: %w", getErr)
		}
	}
	return fmt.Errorf("failed to update status after %d retries due to conflicts", maxRetries)
}

// setExperimentStatus updates the DDA experiment status using statusUpdateWithRetry.
func (r *RemoteConfigUpdater) setExperimentStatus(ctx context.Context, dda *v2alpha1.DatadogAgent, experimentStatus *v2alpha1.ExperimentStatus) error {
	return r.statusUpdateWithRetry(ctx, dda, func(d *v2alpha1.DatadogAgent) {
		d.Status.Experiment = experimentStatus
	})
}

// experimentPhase returns the current experiment phase string, or "none" if no experiment.
func experimentPhase(dda *v2alpha1.DatadogAgent) string {
	if dda.Status.Experiment == nil {
		return "none"
	}
	return string(dda.Status.Experiment.Phase)
}
