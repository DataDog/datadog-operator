// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fleet

import (
	"context"
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

// resolveOperation looks up the installer config for the request, validates the
// task params, and returns the resolved namespace/name and config for the single
// DatadogAgent operation.
func (d *Daemon) resolveOperation(req remoteAPIRequest, signal experimentSignal) (resolvedOperation, error) {
	if err := validateParams(req.Params); err != nil {
		return resolvedOperation{}, fmt.Errorf("%s: invalid params: %w", signal, err)
	}

	if signal != signalStartDatadogAgentExperiment {
		return resolvedOperation{NamespacedName: req.Params.NamespacedName}, nil
	}

	id := req.Params.Version
	if id == "" {
		return resolvedOperation{}, fmt.Errorf("%s: version is required", signal)
	}

	cfg, err := d.getConfig(id)
	if err != nil {
		return resolvedOperation{}, fmt.Errorf("%s: %w", signal, err)
	}

	if len(cfg.Operations) != 1 {
		return resolvedOperation{}, fmt.Errorf("%s: config %s must have exactly 1 operation, got %d", signal, cfg.ID, len(cfg.Operations))
	}
	if cfg.Operations[0].Operation != OperationUpdate {
		return resolvedOperation{}, fmt.Errorf("%s: invalid operation: %s", signal, cfg.Operations[0].Operation)
	}

	return resolvedOperation{
		NamespacedName: req.Params.NamespacedName,
		Config:         cfg.Operations[0].Config,
	}, nil
}

func (d *Daemon) clearExperimentConfigVersion(pkgName string) {
	stable, _ := d.getPackageConfigVersions(pkgName)
	d.setPackageConfigVersions(pkgName, stable, "")
}

func experimentHasPhase(dda *v2alpha1.DatadogAgent, experimentID string, phase v2alpha1.ExperimentPhase) bool {
	return dda.Status.Experiment != nil &&
		dda.Status.Experiment.ID == experimentID &&
		dda.Status.Experiment.Phase == phase
}

func runningExperimentID(dda *v2alpha1.DatadogAgent) string {
	if dda.Status.Experiment != nil && dda.Status.Experiment.Phase == v2alpha1.ExperimentPhaseRunning {
		return dda.Status.Experiment.ID
	}
	return ""
}

func (d *Daemon) newPendingOperation(intent pendingIntent, req remoteAPIRequest, nsn types.NamespacedName, experimentID string) *pendingOperation {
	// These are the fields the worker needs later to update RC task state.
	return &pendingOperation{
		intent:       intent,
		taskID:       req.ID,
		packageName:  req.Package,
		nsn:          nsn,
		experimentID: experimentID,
	}
}

// guardPendingOperationSlot checks whether a new task can use the pending
// annotations on this DatadogAgent.
//
// The annotations can hold one task. The same task can reuse them. A stop can
// replace a start. Other replacements are rejected so the worker does not lose
// the task it still needs to finish.
func (d *Daemon) guardPendingOperationSlot(annotations map[string]string, nsn types.NamespacedName, next pendingOperation) error {
	current, ok := pendingOperationFromAnnotations(nsn, annotations)
	if !ok || current.matches(next) {
		return nil
	}

	// Allow stop to cancel an in-flight start.
	if current.intent == pendingIntentStart && next.intent == pendingIntentStop {
		return nil
	}

	return &stateDoesntMatchError{
		msg: fmt.Sprintf(
			"pending %s task %q already exists for %s/%s; DDA pending annotations can track only one task, refusing to overwrite with %s task %q",
			current.intent,
			current.taskID,
			nsn.Namespace,
			nsn.Name,
			next.intent,
			next.taskID,
		),
	}
}

func (d *Daemon) applyOperation(ctx context.Context, nsn types.NamespacedName, signalLog string, pending *pendingOperation, patch []byte) (*pendingOperation, error) {
	if pending == nil && len(patch) == 0 {
		return nil, nil
	}
	if pending != nil {
		var patchMap map[string]any
		if len(patch) != 0 {
			if err := json.Unmarshal(patch, &patchMap); err != nil {
				return nil, fmt.Errorf("%s: failed to unmarshal base patch: %w", signalLog, err)
			}
		} else {
			patchMap = make(map[string]any)
		}

		metadata, ok := patchMap["metadata"].(map[string]any)
		if !ok {
			metadata = make(map[string]any)
			patchMap["metadata"] = metadata
		}
		annotations, ok := metadata["annotations"].(map[string]any)
		if !ok {
			annotations = make(map[string]any)
			metadata["annotations"] = annotations
		}
		// Write the pending task in the same patch as the signal. If the daemon
		// restarts, the worker can read these annotations and continue.
		annotations[v2alpha1.AnnotationPendingTaskID] = pending.taskID
		annotations[v2alpha1.AnnotationPendingAction] = string(pending.intent)
		annotations[v2alpha1.AnnotationPendingExperimentID] = pending.experimentID
		annotations[v2alpha1.AnnotationPendingPackage] = pending.packageName
		if pending.resultVersion != "" {
			annotations[v2alpha1.AnnotationPendingResultVersion] = pending.resultVersion
		} else {
			// Clear any old promote result version. Merge patch leaves keys alone
			// when they are omitted.
			annotations[v2alpha1.AnnotationPendingResultVersion] = nil
		}

		var err error
		patch, err = json.Marshal(patchMap)
		if err != nil {
			return nil, fmt.Errorf("%s: failed to build pending operation patch: %w", signalLog, err)
		}
	}
	dda := &v2alpha1.DatadogAgent{}
	dda.Name = nsn.Name
	dda.Namespace = nsn.Namespace
	if err := retryWithBackoff(ctx, func() error {
		return d.client.Patch(ctx, dda, client.RawPatch(types.MergePatchType, patch), client.FieldOwner("fleet-daemon"))
	}); err != nil {
		return nil, fmt.Errorf("%s: failed to patch DatadogAgent: %w", signalLog, err)
	}
	ctrl.LoggerFrom(ctx).Info("Wrote signal")
	return pending, nil
}

// startDatadogAgentExperiment starts a DatadogAgent experiment by atomically
// patching both the DDA spec (experiment config) and experiment signal annotations.
// If the annotation ID already matches and the reconciler has already set
// phase=running, the patch is skipped. After writing, the status worker waits
// for the reconciler to set phase=running before marking the task done.
func (d *Daemon) startDatadogAgentExperiment(ctx context.Context, req remoteAPIRequest) (*pendingOperation, error) {
	logger := ctrl.LoggerFrom(ctx).WithValues("id", req.ID)
	logger.V(1).Info("Starting DatadogAgent experiment", "config", req.Params.Version)
	op, err := d.resolveOperation(req, signalStartDatadogAgentExperiment)
	if err != nil {
		logger.Error(err, "Failed to resolve operation")
		return nil, err
	}

	logger = logger.WithValues("namespace", op.NamespacedName.Namespace, "name", op.NamespacedName.Name)
	ctx = ctrl.LoggerInto(ctx, logger)
	pending, patch, err := d.planStart(ctx, req, op)
	if err != nil {
		return nil, err
	}
	logger.Info("Prepared DatadogAgent experiment start signal")
	return d.applyOperation(ctx, op.NamespacedName, "start DatadogAgent experiment", pending, patch)
}

// stopDatadogAgentExperiment writes a rollback signal annotation on the DDA.
// If the phase is already terminal, the patch is skipped. After writing, the
// status worker waits for any terminal phase before marking the task done.
func (d *Daemon) stopDatadogAgentExperiment(ctx context.Context, req remoteAPIRequest) (*pendingOperation, error) {
	op, err := d.resolveOperation(req, "stop DatadogAgent experiment")
	if err != nil {
		return nil, err
	}

	ctx = ctrl.LoggerInto(ctx, ctrl.LoggerFrom(ctx).WithValues("id", req.ID, "namespace", op.NamespacedName.Namespace, "name", op.NamespacedName.Name))
	logger := ctrl.LoggerFrom(ctx)
	logger.V(1).Info("Stopping DatadogAgent experiment")
	pending, patch, err := d.planStop(ctx, req, op)
	if err != nil {
		return nil, err
	}
	logger.Info("Prepared DatadogAgent experiment stop signal")
	return d.applyOperation(ctx, op.NamespacedName, "stop DatadogAgent experiment", pending, patch)
}

// promoteDatadogAgentExperiment writes a promote signal annotation on the DDA.
// If the phase is already promoted, the patch is skipped. After writing, the
// status worker waits for phase=promoted before marking the task done.
func (d *Daemon) promoteDatadogAgentExperiment(ctx context.Context, req remoteAPIRequest) (*pendingOperation, error) {
	op, err := d.resolveOperation(req, signalPromoteDatadogAgentExperiment)
	if err != nil {
		return nil, err
	}

	ctx = ctrl.LoggerInto(ctx, ctrl.LoggerFrom(ctx).WithValues("id", req.ID, "namespace", op.NamespacedName.Namespace, "name", op.NamespacedName.Name))
	logger := ctrl.LoggerFrom(ctx)
	logger.V(1).Info("Promoting DatadogAgent experiment")
	pending, patch, err := d.planPromote(ctx, req, op)
	if err != nil {
		return nil, err
	}
	logger.Info("Prepared DatadogAgent experiment promote signal")
	return d.applyOperation(ctx, op.NamespacedName, "promote DatadogAgent experiment", pending, patch)
}

func (d *Daemon) planStart(ctx context.Context, req remoteAPIRequest, op resolvedOperation) (*pendingOperation, []byte, error) {
	experimentID := req.Params.Version
	pending := d.newPendingOperation(pendingIntentStart, req, op.NamespacedName, experimentID)
	dda := &v2alpha1.DatadogAgent{}
	if err := d.client.Get(ctx, op.NamespacedName, dda); err != nil {
		return nil, nil, fmt.Errorf("start DatadogAgent experiment: failed to get DatadogAgent: %w", err)
	}
	if experimentHasPhase(dda, experimentID, v2alpha1.ExperimentPhaseRunning) {
		// The controller already started this experiment. Update RC now and let
		// handleTask mark the task done.
		stable, _ := d.getPackageConfigVersions(req.Package)
		d.setPackageConfigVersions(req.Package, stable, req.Params.Version)
		return nil, nil, nil
	}
	if dda.Annotations[v2alpha1.AnnotationExperimentID] == experimentID {
		// The start signal is already on the DDA. Keep the same signal, but make
		// sure the pending task annotations exist.
		if err := d.guardPendingOperationSlot(dda.Annotations, op.NamespacedName, *pending); err != nil {
			return nil, nil, err
		}
		return pending, nil, nil
	}
	if runningID := runningExperimentID(dda); runningID != "" {
		return nil, nil, fmt.Errorf("start DatadogAgent experiment: experiment %q already running", runningID)
	}
	// Do not overwrite another unfinished task.
	if err := d.guardPendingOperationSlot(dda.Annotations, op.NamespacedName, *pending); err != nil {
		return nil, nil, err
	}
	patch, err := buildSignalPatch(v2alpha1.ExperimentSignalStart, experimentID, op.Config)
	if err != nil {
		return nil, nil, fmt.Errorf("start DatadogAgent experiment: %w", err)
	}
	return pending, patch, nil
}

func (d *Daemon) planStop(ctx context.Context, req remoteAPIRequest, op resolvedOperation) (*pendingOperation, []byte, error) {
	dda := &v2alpha1.DatadogAgent{}
	if getErr := d.client.Get(ctx, op.NamespacedName, dda); getErr != nil {
		return nil, nil, fmt.Errorf("stop DatadogAgent experiment: failed to get DatadogAgent: %w", getErr)
	}

	// Stop requests intentionally do not use params.version as the experiment
	// identity. verifyExpectedState already guarded the RC state transition, so
	// rollback should target whichever experiment is currently recorded on the
	// DDA: status first, then an in-flight start annotation, then RC state.
	experimentID := dda.Annotations[v2alpha1.AnnotationExperimentID]
	if dda.Status.Experiment != nil && dda.Status.Experiment.ID != "" {
		experimentID = dda.Status.Experiment.ID
	}
	if experimentID == "" {
		_, experimentID = d.getPackageConfigVersions(req.Package)
	}

	if dda.Status.Experiment == nil {
		if experimentID == "" || dda.Annotations[v2alpha1.AnnotationExperimentSignal] != v2alpha1.ExperimentSignalStart {
			// Nothing is running and there is no start signal to roll back.
			d.clearExperimentConfigVersion(req.Package)
			return nil, nil, nil
		}
	} else {
		if isTerminalPhase(dda.Status.Experiment.Phase) {
			// The experiment is already stopped/promoted/aborted.
			d.clearExperimentConfigVersion(req.Package)
			return nil, nil, nil
		}
		switch dda.Status.Experiment.Phase {
		case v2alpha1.ExperimentPhaseRunning:
			if experimentID == "" {
				return nil, nil, fmt.Errorf("stop DatadogAgent experiment: running experiment is missing an ID")
			}
		case "":
			// Start was requested, but the reconciler has not written a phase yet.
			if experimentID == "" {
				return nil, nil, fmt.Errorf("stop DatadogAgent experiment: current experiment is missing an ID")
			}
		default:
			return nil, nil, fmt.Errorf("stop DatadogAgent experiment: cannot stop, current phase is %q", dda.Status.Experiment.Phase)
		}
	}
	pending := d.newPendingOperation(pendingIntentStop, req, op.NamespacedName, experimentID)
	if err := d.guardPendingOperationSlot(dda.Annotations, op.NamespacedName, *pending); err != nil {
		return nil, nil, err
	}
	patch, err := buildSignalPatch(v2alpha1.ExperimentSignalRollback, experimentID)
	if err != nil {
		return nil, nil, fmt.Errorf("stop DatadogAgent experiment: %w", err)
	}
	return pending, patch, nil
}

func (d *Daemon) planPromote(ctx context.Context, req remoteAPIRequest, op resolvedOperation) (*pendingOperation, []byte, error) {
	experimentID := req.Params.Version
	_, experiment := d.getPackageConfigVersions(req.Package)
	if experiment == "" {
		return nil, nil, fmt.Errorf("promote DatadogAgent experiment: no experiment config version set")
	}
	expectedExperimentID := experimentID
	dda := &v2alpha1.DatadogAgent{}
	if err := d.client.Get(ctx, op.NamespacedName, dda); err != nil {
		return nil, nil, fmt.Errorf("promote DatadogAgent experiment: failed to get DatadogAgent: %w", err)
	}
	if experimentHasPhase(dda, experimentID, v2alpha1.ExperimentPhasePromoted) {
		// Promotion already happened. Update RC now and let handleTask mark the
		// task done.
		d.setPackageConfigVersions(req.Package, experiment, "")
		return nil, nil, nil
	}
	if !experimentHasPhase(dda, experimentID, v2alpha1.ExperimentPhaseRunning) {
		if runningID := runningExperimentID(dda); runningID != "" {
			return nil, nil, fmt.Errorf("promote DatadogAgent experiment: running experiment %q does not match requested version %q", runningID, experimentID)
		}
		currentPhase := ""
		if dda.Status.Experiment != nil {
			currentPhase = string(dda.Status.Experiment.Phase)
		}
		return nil, nil, fmt.Errorf("promote DatadogAgent experiment: cannot promote, current phase is %q", currentPhase)
	}
	pending := d.newPendingOperation(pendingIntentPromote, req, op.NamespacedName, expectedExperimentID)
	// Promote makes the current experiment config the stable config on success.
	pending.resultVersion = experiment
	if err := d.guardPendingOperationSlot(dda.Annotations, op.NamespacedName, *pending); err != nil {
		return nil, nil, err
	}
	patch, err := buildSignalPatch(v2alpha1.ExperimentSignalPromote, experimentID)
	if err != nil {
		return nil, nil, fmt.Errorf("promote DatadogAgent experiment: %w", err)
	}
	return pending, patch, nil
}
