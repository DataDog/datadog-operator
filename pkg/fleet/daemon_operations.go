// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fleet

import (
	"context"
	"encoding/json"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
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

// planResult carries a plan* function's outputs. resourceVersion is the DDA's
// resourceVersion at the moment the plan was constructed — bound to the same
// read that produced the baseline annotation, so the server rejects the write
// via optimistic-lock precondition if any actor mutated the DDA between plan
// and apply.
type planResult struct {
	pending         *pendingOperation
	patch           []byte
	resourceVersion string
}

// planFn produces a planResult. Called by applyOperation once per attempt,
// so on optimistic-lock conflict the caller re-plans from a fresh read
// (fresh baseline + fresh resourceVersion together, never one without
// the other).
type planFn func(ctx context.Context) (planResult, error)

// applyOperation wraps a plan-and-patch cycle with two nested retry policies:
// (a) transient retry via retryWithBackoff, which handles timeouts, throttling,
// and other retryable apiserver errors up to the retry budget; (b) an outer
// conflict-retry-once loop that re-invokes plan on 409 so both the baseline
// annotation and the resourceVersion precondition come from the same fresh
// read. Terminal failure surfaces errBaselineConflict on repeated conflicts.
func (d *Daemon) applyOperation(ctx context.Context, nsn types.NamespacedName, signalLog string, plan planFn) (*pendingOperation, error) {
	logger := ctrl.LoggerFrom(ctx)
	var (
		lastPending *pendingOperation
		lastErr     error
	)
	for conflictAttempt := 0; conflictAttempt < 2; conflictAttempt++ {
		result, err := plan(ctx)
		if err != nil {
			return nil, err
		}
		if result.pending == nil && len(result.patch) == 0 {
			return nil, nil
		}
		lastPending = result.pending

		finalPatch, err := composePatchWithPendingAndRV(result.patch, result.pending, result.resourceVersion)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", signalLog, err)
		}

		lastErr = retryWithBackoff(ctx, func() error {
			target := &v2alpha1.DatadogAgent{}
			target.Name = nsn.Name
			target.Namespace = nsn.Namespace
			return d.client.Patch(ctx, target,
				client.RawPatch(types.MergePatchType, finalPatch),
				client.FieldOwner("fleet-daemon"),
			)
		})
		if !apierrors.IsConflict(lastErr) {
			break
		}
		logger.V(1).Info("Optimistic lock conflict on signal patch, replanning and retrying once")
	}
	if apierrors.IsConflict(lastErr) {
		return nil, fmt.Errorf("%s: %w", signalLog, errBaselineConflict)
	}
	if lastErr != nil {
		return nil, fmt.Errorf("%s: failed to patch DatadogAgent: %w", signalLog, lastErr)
	}
	logger.Info("Wrote signal")
	return lastPending, nil
}

// composePatchWithPendingAndRV merges pending-operation annotations into the
// plan patch and injects metadata.resourceVersion as the optimistic-lock
// precondition. The resourceVersion belongs to the DDA read that produced
// the plan; the server rejects with 409 if it no longer matches.
func composePatchWithPendingAndRV(patch []byte, pending *pendingOperation, resourceVersion string) ([]byte, error) {
	var m map[string]any
	if len(patch) != 0 {
		if err := json.Unmarshal(patch, &m); err != nil {
			return nil, fmt.Errorf("failed to unmarshal base patch: %w", err)
		}
	} else {
		m = make(map[string]any)
	}
	metadata, ok := m["metadata"].(map[string]any)
	if !ok {
		metadata = map[string]any{}
		m["metadata"] = metadata
	}
	if resourceVersion != "" {
		metadata["resourceVersion"] = resourceVersion
	}
	if pending != nil {
		annotations, ok := metadata["annotations"].(map[string]any)
		if !ok {
			annotations = map[string]any{}
			metadata["annotations"] = annotations
		}
		annotations[v2alpha1.AnnotationPendingTaskID] = pending.taskID
		annotations[v2alpha1.AnnotationPendingAction] = string(pending.intent)
		annotations[v2alpha1.AnnotationPendingExperimentID] = pending.experimentID
		annotations[v2alpha1.AnnotationPendingPackage] = pending.packageName
		if pending.resultVersion != "" {
			annotations[v2alpha1.AnnotationPendingResultVersion] = pending.resultVersion
		} else {
			// Clear any old promote result version. Merge patch leaves keys
			// alone when they are omitted.
			annotations[v2alpha1.AnnotationPendingResultVersion] = nil
		}
	}
	out, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal composed patch: %w", err)
	}
	return out, nil
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
	return d.applyOperation(ctx, op.NamespacedName, "start DatadogAgent experiment", func(ctx context.Context) (planResult, error) {
		return d.planStart(ctx, req, op)
	})
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
	ctrl.LoggerFrom(ctx).V(1).Info("Stopping DatadogAgent experiment")
	return d.applyOperation(ctx, op.NamespacedName, "stop DatadogAgent experiment", func(ctx context.Context) (planResult, error) {
		return d.planStop(ctx, req, op)
	})
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
	ctrl.LoggerFrom(ctx).V(1).Info("Promoting DatadogAgent experiment")
	return d.applyOperation(ctx, op.NamespacedName, "promote DatadogAgent experiment", func(ctx context.Context) (planResult, error) {
		return d.planPromote(ctx, req, op)
	})
}

func (d *Daemon) planStart(ctx context.Context, req remoteAPIRequest, op resolvedOperation) (planResult, error) {
	experimentID := req.Params.Version
	pending := d.newPendingOperation(pendingIntentStart, req, op.NamespacedName, experimentID)
	if d.managedAgentInstallationIdentity.Configured() {
		if _, err := decodeRemoteDatadogAgentConfig(op.Config, false); err != nil {
			return planResult{}, fmt.Errorf("start DatadogAgent experiment: %w", err)
		}
	}
	dda := &v2alpha1.DatadogAgent{}
	if err := d.client.Get(ctx, op.NamespacedName, dda); err != nil {
		return planResult{}, fmt.Errorf("start DatadogAgent experiment: failed to get DatadogAgent: %w", err)
	}
	if err := d.validateBridgeExperimentTarget(dda); err != nil {
		return planResult{}, fmt.Errorf("start DatadogAgent experiment: %w", err)
	}
	if experimentHasPhase(dda, experimentID, v2alpha1.ExperimentPhaseRunning) {
		// The controller already started this experiment. Update RC now and let
		// handleTask mark the task done.
		stable, _ := d.getPackageConfigVersions(req.Package)
		d.setPackageConfigVersions(req.Package, stable, req.Params.Version)
		return planResult{}, nil
	}
	if dda.Annotations[v2alpha1.AnnotationExperimentID] == experimentID {
		// The start signal is already on the DDA. Keep the same signal, but make
		// sure the pending task annotations exist.
		if err := d.guardPendingOperationSlot(dda.Annotations, op.NamespacedName, *pending); err != nil {
			return planResult{}, err
		}
		return planResult{pending: pending, resourceVersion: dda.ResourceVersion}, nil
	}
	if runningID := runningExperimentID(dda); runningID != "" {
		return planResult{}, fmt.Errorf("start DatadogAgent experiment: experiment %q already running", runningID)
	}
	// Do not overwrite another unfinished task.
	if err := d.guardPendingOperationSlot(dda.Annotations, op.NamespacedName, *pending); err != nil {
		return planResult{}, err
	}
	// Checkpoint the pre-experiment baseline from the reconciler-published
	// current-revision pointer AND capture the DDA's resourceVersion from the
	// same read. applyOperation writes both into the merge patch, so the
	// server rejects the write with 409 Conflict if any actor mutated the DDA
	// between this read and the patch. That prevents a stale
	// Status.CurrentRevision from being persisted as the rollback baseline
	// when a concurrent user apply bumped the generation after this read.
	extraAnnotations := map[string]string{
		v2alpha1.AnnotationExperimentRollbackTargetRevision: dda.Status.CurrentRevision,
	}
	patch, err := buildSignalPatchWithAnnotations(v2alpha1.ExperimentSignalStart, experimentID, extraAnnotations, op.Config)
	if err != nil {
		return planResult{}, fmt.Errorf("start DatadogAgent experiment: %w", err)
	}
	return planResult{pending: pending, patch: patch, resourceVersion: dda.ResourceVersion}, nil
}

func (d *Daemon) planStop(ctx context.Context, req remoteAPIRequest, op resolvedOperation) (planResult, error) {
	dda := &v2alpha1.DatadogAgent{}
	if getErr := d.client.Get(ctx, op.NamespacedName, dda); getErr != nil {
		return planResult{}, fmt.Errorf("stop DatadogAgent experiment: failed to get DatadogAgent: %w", getErr)
	}
	if err := d.validateBridgeExperimentTarget(dda); err != nil {
		return planResult{}, fmt.Errorf("stop DatadogAgent experiment: %w", err)
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
			return planResult{}, nil
		}
	} else {
		if isTerminalPhase(dda.Status.Experiment.Phase) {
			// The experiment is already stopped/promoted/aborted.
			d.clearExperimentConfigVersion(req.Package)
			return planResult{}, nil
		}
		switch dda.Status.Experiment.Phase {
		case v2alpha1.ExperimentPhaseRunning:
			if experimentID == "" {
				return planResult{}, fmt.Errorf("stop DatadogAgent experiment: running experiment is missing an ID")
			}
		case "":
			// Start was requested, but the reconciler has not written a phase yet.
			if experimentID == "" {
				return planResult{}, fmt.Errorf("stop DatadogAgent experiment: current experiment is missing an ID")
			}
		default:
			return planResult{}, fmt.Errorf("stop DatadogAgent experiment: cannot stop, current phase is %q", dda.Status.Experiment.Phase)
		}
	}
	pending := d.newPendingOperation(pendingIntentStop, req, op.NamespacedName, experimentID)
	if err := d.guardPendingOperationSlot(dda.Annotations, op.NamespacedName, *pending); err != nil {
		return planResult{}, err
	}
	patch, err := buildSignalPatch(v2alpha1.ExperimentSignalRollback, experimentID)
	if err != nil {
		return planResult{}, fmt.Errorf("stop DatadogAgent experiment: %w", err)
	}
	return planResult{pending: pending, patch: patch, resourceVersion: dda.ResourceVersion}, nil
}

func (d *Daemon) planPromote(ctx context.Context, req remoteAPIRequest, op resolvedOperation) (planResult, error) {
	_, experiment := d.getPackageConfigVersions(req.Package)
	if experiment == "" {
		return planResult{}, fmt.Errorf("promote DatadogAgent experiment: no experiment config version set")
	}
	dda := &v2alpha1.DatadogAgent{}
	if err := d.client.Get(ctx, op.NamespacedName, dda); err != nil {
		return planResult{}, fmt.Errorf("promote DatadogAgent experiment: failed to get DatadogAgent: %w", err)
	}
	if err := d.validateBridgeExperimentTarget(dda); err != nil {
		return planResult{}, fmt.Errorf("promote DatadogAgent experiment: %w", err)
	}

	// Promote requests intentionally do not use params.version as the experiment
	// identity. RC does not include a version on promote; the signal applies to
	// whichever experiment is currently recorded on the DDA: status first, then
	// an in-flight start annotation, then RC state.
	experimentID := dda.Annotations[v2alpha1.AnnotationExperimentID]
	if dda.Status.Experiment != nil && dda.Status.Experiment.ID != "" {
		experimentID = dda.Status.Experiment.ID
	}
	if experimentID == "" {
		experimentID = experiment
	}

	if experimentHasPhase(dda, experimentID, v2alpha1.ExperimentPhasePromoted) {
		if err := d.persistManagedAgentInstallationStableConfig(ctx, op.NamespacedName, experimentID, experiment); err != nil {
			return planResult{}, fmt.Errorf("promote DatadogAgent experiment: persist promoted config: %w", err)
		}
		// Promotion already happened. Update RC now and let handleTask mark the task done.
		d.setPackageConfigVersions(req.Package, experiment, "")
		return planResult{}, nil
	}
	if !experimentHasPhase(dda, experimentID, v2alpha1.ExperimentPhaseRunning) {
		currentPhase := ""
		if dda.Status.Experiment != nil {
			currentPhase = string(dda.Status.Experiment.Phase)
		}
		return planResult{}, fmt.Errorf("promote DatadogAgent experiment: cannot promote, current phase is %q", currentPhase)
	}
	pending := d.newPendingOperation(pendingIntentPromote, req, op.NamespacedName, experimentID)
	// Promote makes the current experiment config the stable config on success.
	pending.resultVersion = experiment
	if err := d.guardPendingOperationSlot(dda.Annotations, op.NamespacedName, *pending); err != nil {
		return planResult{}, err
	}
	patch, err := buildSignalPatch(v2alpha1.ExperimentSignalPromote, experimentID)
	if err != nil {
		return planResult{}, fmt.Errorf("promote DatadogAgent experiment: %w", err)
	}
	return planResult{pending: pending, patch: patch, resourceVersion: dda.ResourceVersion}, nil
}

func (d *Daemon) validateBridgeExperimentTarget(dda *v2alpha1.DatadogAgent) error {
	if !d.managedAgentInstallationIdentity.Configured() {
		return nil
	}
	if err := d.validateFleetDatadogAgentInstallation(dda); err != nil {
		return err
	}
	return validateFleetDatadogAgentManagedAgentInstallationReady(dda)
}
