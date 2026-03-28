// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fleet

import (
	"context"
	"encoding/json"
	"fmt"

	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	kubeclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

// fleetManagementOperation is a single fleet operation for config management.
type fleetManagementOperation struct {
	Operation        operation              `json:"operation"`
	GroupVersionKind schema.GroupVersionKind `json:"group_version_kind"`
	NamespacedName   types.NamespacedName   `json:"namespaced_name"`
	Config           json.RawMessage        `json:"config"`
}

type operation string

const (
	operationCreate operation = "create"
	operationUpdate operation = "update"
	operationDelete operation = "delete"
)

// extractDDAPatch extracts the first update operation targeting a DatadogAgent
// from the installer config. Returns the target NamespacedName and the raw
// JSON merge patch, or an error if no matching operation is found.
func extractDDAPatch(cfg installerConfig) (types.NamespacedName, json.RawMessage, error) {
	for _, op := range cfg.Operations {
		if op.Operation == operationUpdate && op.GroupVersionKind.Kind == "DatadogAgent" {
			return op.NamespacedName, op.Config, nil
		}
	}
	return types.NamespacedName{}, nil, fmt.Errorf("no DatadogAgent update operation found in config %s", cfg.ID)
}

func (d *Daemon) startDatadogAgentExperiment(req remoteAPIRequest) error {
	ctx := context.TODO()

	if req.ID == "" {
		return fmt.Errorf("experiment task missing ID")
	}

	cfg, err := d.getConfig(req.ExpectedState.ExperimentConfig)
	if err != nil {
		return fmt.Errorf("start experiment: %w", err)
	}

	target, patch, err := extractDDAPatch(cfg)
	if err != nil {
		return fmt.Errorf("start experiment: %w", err)
	}

	// Get the target DDA
	dda := &v2alpha1.DatadogAgent{}
	if err := d.kubeClient.Get(ctx, target, dda); err != nil {
		return fmt.Errorf("start experiment: failed to get DDA %s: %w", target, err)
	}

	// Guard: reject if an active experiment is in progress
	if dda.Status.Experiment != nil {
		phase := dda.Status.Experiment.Phase
		switch phase {
		case v2alpha1.ExperimentPhaseRunning, v2alpha1.ExperimentPhaseRollback, v2alpha1.ExperimentPhaseStopped:
			return fmt.Errorf("start experiment: experiment %s is active (phase=%s)", dda.Status.Experiment.ID, phase)
		}
	}

	// Step 1: Apply JSON merge patch to build the target spec
	patchedDDA := dda.DeepCopy()
	if err := json.Unmarshal(patch, patchedDDA); err != nil {
		return fmt.Errorf("start experiment: failed to apply merge patch: %w", err)
	}

	// Step 2: Set experiment status
	if err := d.setExperimentStatus(ctx, dda, &v2alpha1.ExperimentStatus{
		Phase: v2alpha1.ExperimentPhaseRunning,
		ID:    req.ID,
	}); err != nil {
		return fmt.Errorf("start experiment: failed to set status: %w", err)
	}

	// Step 3: Re-fetch for fresh resourceVersion, apply patched spec
	refreshed := &v2alpha1.DatadogAgent{}
	if err := d.kubeClient.Get(ctx, target, refreshed); err != nil {
		return fmt.Errorf("start experiment: failed to re-fetch DDA: %w", err)
	}
	refreshed.Spec = patchedDDA.Spec
	if patchedDDA.Annotations != nil {
		refreshed.Annotations = patchedDDA.Annotations
	}
	if err := d.kubeClient.Update(ctx, refreshed); err != nil {
		return fmt.Errorf("start experiment: failed to update DDA spec: %w", err)
	}

	// Step 4: Update experiment generation to post-patch value
	if err := d.kubeClient.Get(ctx, target, refreshed); err != nil {
		return fmt.Errorf("start experiment: failed to re-fetch for generation: %w", err)
	}
	if refreshed.Status.Experiment != nil {
		refreshed.Status.Experiment.Generation = refreshed.Generation
		if err := d.kubeClient.Status().Update(ctx, refreshed); err != nil {
			if !apierrors.IsConflict(err) {
				return fmt.Errorf("start experiment: failed to update generation: %w", err)
			}
			d.logger.Info("Generation update conflicted, will be set on next reconcile")
		}
	}

	d.logger.Info("Started experiment", "id", req.ID, "target", target)
	return nil
}

func (d *Daemon) stopDatadogAgentExperiment(req remoteAPIRequest) error {
	ctx := context.TODO()

	dda, err := d.findRunningExperiment(ctx, req.ID)
	if err != nil {
		return err
	}
	if dda == nil {
		d.logger.Info("Ignoring stop: no matching running experiment", "taskID", req.ID)
		return nil
	}

	if err := d.setExperimentStatus(ctx, dda, &v2alpha1.ExperimentStatus{
		Phase:      v2alpha1.ExperimentPhaseStopped,
		ID:         dda.Status.Experiment.ID,
		Generation: dda.Status.Experiment.Generation,
	}); err != nil {
		return fmt.Errorf("stop experiment: %w", err)
	}

	d.logger.Info("Stopped experiment", "id", req.ID)
	return nil
}

func (d *Daemon) promoteDatadogAgentExperiment(req remoteAPIRequest) error {
	ctx := context.TODO()

	dda, err := d.findRunningExperiment(ctx, req.ID)
	if err != nil {
		return err
	}
	if dda == nil {
		d.logger.Info("Ignoring promote: no matching running experiment", "taskID", req.ID)
		return nil
	}

	if err := d.setExperimentStatus(ctx, dda, &v2alpha1.ExperimentStatus{
		Phase: v2alpha1.ExperimentPhasePromoted,
		ID:    dda.Status.Experiment.ID,
	}); err != nil {
		return fmt.Errorf("promote experiment: %w", err)
	}

	d.logger.Info("Promoted experiment", "id", req.ID)
	return nil
}

func (d *Daemon) findRunningExperiment(ctx context.Context, taskID string) (*v2alpha1.DatadogAgent, error) {
	ddaList := &v2alpha1.DatadogAgentList{}
	if err := d.kubeClient.List(ctx, ddaList); err != nil {
		return nil, fmt.Errorf("failed to list DDAs: %w", err)
	}
	if len(ddaList.Items) == 0 {
		return nil, nil
	}

	dda := &ddaList.Items[0]
	if dda.Status.Experiment == nil || dda.Status.Experiment.Phase != v2alpha1.ExperimentPhaseRunning {
		return nil, nil
	}
	if dda.Status.Experiment.ID != "" && taskID != "" && dda.Status.Experiment.ID != taskID {
		return nil, nil
	}
	return dda, nil
}

func (d *Daemon) setExperimentStatus(ctx context.Context, dda *v2alpha1.DatadogAgent, experiment *v2alpha1.ExperimentStatus) error {
	const maxRetries = 3
	for i := range maxRetries {
		newStatus := dda.Status.DeepCopy()
		newStatus.Experiment = experiment

		if apiequality.Semantic.DeepEqual(&dda.Status, newStatus) {
			return nil
		}

		ddaUpdate := dda.DeepCopy()
		ddaUpdate.Status = *newStatus
		updateErr := d.kubeClient.Status().Update(ctx, ddaUpdate)
		if updateErr == nil {
			dda.Status = *newStatus
			return nil
		}
		if !apierrors.IsConflict(updateErr) {
			return updateErr
		}
		d.logger.Info("Status update conflict, retrying", "attempt", i+1)
		if getErr := d.kubeClient.Get(ctx, kubeclient.ObjectKeyFromObject(dda), dda); getErr != nil {
			return fmt.Errorf("re-fetch after conflict: %w", getErr)
		}
	}
	return fmt.Errorf("failed to update experiment status after %d retries", maxRetries)
}
