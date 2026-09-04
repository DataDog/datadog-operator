// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fleet

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

// planResult is what a planFn produces: the pending operation bookkeeping (if
// any), the signal patch to apply, and the resourceVersion of the DatadogAgent
// the plan was computed against (used as an optimistic-lock precondition).
// A zero-value planResult means the operation is a no-op.
type planResult struct {
	pending         *pendingOperation
	patch           []byte
	resourceVersion string
}

func (r planResult) isNoop() bool {
	return r.pending == nil && len(r.patch) == 0
}

// planFn computes (or recomputes, after a conflict) the patch to apply for an
// experiment signal. It re-reads the DatadogAgent each time it is called so
// that a replan after a conflict sees the latest object.
type planFn func(ctx context.Context) (planResult, error)

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

// buildFinalPatch merges the pending-operation bookkeeping annotations into
// result.patch and, if result.resourceVersion is set, adds it to the patch body
// as an optimistic-lock precondition (a merge patch that names a resourceVersion
// different from the live object's is rejected by the API server with a 409).
func buildFinalPatch(result planResult) ([]byte, error) {
	var patchMap map[string]any
	if len(result.patch) != 0 {
		if err := json.Unmarshal(result.patch, &patchMap); err != nil {
			return nil, fmt.Errorf("failed to unmarshal base patch: %w", err)
		}
	} else {
		patchMap = make(map[string]any)
	}

	metadata, ok := patchMap["metadata"].(map[string]any)
	if !ok {
		metadata = make(map[string]any)
		patchMap["metadata"] = metadata
	}

	if result.pending != nil {
		annotations, ok := metadata["annotations"].(map[string]any)
		if !ok {
			annotations = make(map[string]any)
			metadata["annotations"] = annotations
		}
		// Write the pending task in the same patch as the signal. If the daemon
		// restarts, the worker can read these annotations and continue.
		annotations[v2alpha1.AnnotationPendingTaskID] = result.pending.taskID
		annotations[v2alpha1.AnnotationPendingAction] = string(result.pending.intent)
		annotations[v2alpha1.AnnotationPendingExperimentID] = result.pending.experimentID
		annotations[v2alpha1.AnnotationPendingPackage] = result.pending.packageName
		if result.pending.resultVersion != "" {
			annotations[v2alpha1.AnnotationPendingResultVersion] = result.pending.resultVersion
		} else {
			// Clear any old promote result version. Merge patch leaves keys alone
			// when they are omitted.
			annotations[v2alpha1.AnnotationPendingResultVersion] = nil
		}
	}

	if result.resourceVersion != "" {
		metadata["resourceVersion"] = result.resourceVersion
	}

	return json.Marshal(patchMap)
}

// applyOperation runs plan to compute a patch and applies it with an
// optimistic-lock precondition. If the patch is rejected with a conflict, it
// replans exactly once against the latest object before giving up with
// errBaselineConflict; a second conflict is not retried indefinitely because
// that would mask a real concurrent-change race as a hang.
func (d *Daemon) applyOperation(ctx context.Context, nsn types.NamespacedName, signalLog string, plan planFn) (*pendingOperation, error) {
	result, err := plan(ctx)
	if err != nil {
		return nil, err
	}
	if result.isNoop() {
		return nil, nil
	}

	replanned := false
	for {
		patch, err := buildFinalPatch(result)
		if err != nil {
			return nil, fmt.Errorf("%s: failed to build pending operation patch: %w", signalLog, err)
		}

		dda := &v2alpha1.DatadogAgent{}
		dda.Name = nsn.Name
		dda.Namespace = nsn.Namespace
		patchErr := retryWithBackoffPreconditioned(ctx, func() error {
			return d.client.Patch(ctx, dda, client.RawPatch(types.MergePatchType, patch), client.FieldOwner("fleet-daemon"))
		})
		if patchErr == nil {
			ctrl.LoggerFrom(ctx).Info("Wrote signal")
			return result.pending, nil
		}
		if !apierrors.IsConflict(patchErr) {
			return nil, fmt.Errorf("%s: failed to patch DatadogAgent: %w", signalLog, patchErr)
		}
		if replanned {
			return nil, fmt.Errorf("%s: %w", signalLog, errors.Join(errBaselineConflict, patchErr))
		}
		replanned = true
		result, err = plan(ctx)
		if err != nil {
			return nil, err
		}
		if result.isNoop() {
			return nil, nil
		}
	}
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
	pending, err := d.applyOperation(ctx, op.NamespacedName, "start DatadogAgent experiment", func(ctx context.Context) (planResult, error) {
		return d.planStart(ctx, req, op)
	})
	if err != nil {
		return nil, err
	}
	logger.Info("Prepared DatadogAgent experiment start signal")
	return pending, nil
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
	pending, err := d.applyOperation(ctx, op.NamespacedName, "stop DatadogAgent experiment", func(ctx context.Context) (planResult, error) {
		return d.planStop(ctx, req, op)
	})
	if err != nil {
		return nil, err
	}
	logger.Info("Prepared DatadogAgent experiment stop signal")
	return pending, nil
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
	pending, err := d.applyOperation(ctx, op.NamespacedName, "promote DatadogAgent experiment", func(ctx context.Context) (planResult, error) {
		return d.planPromote(ctx, req, op)
	})
	if err != nil {
		return nil, err
	}
	logger.Info("Prepared DatadogAgent experiment promote signal")
	return pending, nil
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
	if dda.Status.Experiment != nil &&
		dda.Status.Experiment.ID == experimentID &&
		isTerminalPhase(dda.Status.Experiment.Phase) {
		// This experiment already ran to completion. Its signal annotations were
		// cleared when it terminated, so the resend branch below will not catch
		// it and the fresh-start path would happily write the experiment spec
		// back on top of whatever is live now -- the baseline after a rollback,
		// or the stable spec after a promote -- while Status.Experiment stays
		// frozen at the terminal phase. That is stranded config nobody can see:
		// no signal annotations, no in-flight experiment, and a spec that
		// silently disagrees with the terminal status. Refuse instead, and tell
		// RC its premise is wrong rather than accepting the drift.
		return planResult{}, &stateDoesntMatchError{msg: fmt.Sprintf(
			"DatadogAgent %s/%s already completed experiment %q: phase %q (reason %q)",
			dda.Namespace, dda.Name, experimentID,
			dda.Status.Experiment.Phase, dda.Status.Experiment.TerminationReason)}
	}
	if dda.Annotations[v2alpha1.AnnotationExperimentSignal] == v2alpha1.ExperimentSignalStart &&
		dda.Annotations[v2alpha1.AnnotationExperimentID] == experimentID {
		return d.planStartResend(ctx, req, op, dda, pending)
	}
	if runningID := runningExperimentID(dda); runningID != "" {
		return planResult{}, fmt.Errorf("start DatadogAgent experiment: experiment %q already running", runningID)
	}
	// Do not overwrite another unfinished task.
	if err := d.guardPendingOperationSlot(dda.Annotations, op.NamespacedName, *pending); err != nil {
		return planResult{}, err
	}
	// Refuse to start a new experiment against a baseline the reconciler has not
	// finished checkpointing yet. The rollback target recorded below must be the
	// revision the reconciler will actually roll back to, and it must still resolve
	// to a ControllerRevision owned by this DDA.
	if err := d.checkBaselineReady(ctx, dda); err != nil {
		return planResult{}, fmt.Errorf("start DatadogAgent experiment: %w", err)
	}
	patch, err := BuildStartPatch(dda, experimentID, op.Config, dda.Status.CurrentRevision)
	if err != nil {
		return planResult{}, fmt.Errorf("start DatadogAgent experiment: %w", err)
	}
	return planResult{pending: pending, patch: patch, resourceVersion: dda.ResourceVersion}, nil
}

// planStartResend classifies a start request whose signal annotations are
// already on the DDA under the same experiment ID. Only one of the three
// outcomes is idempotent, and telling them apart matters because they need
// opposite RC signals.
//
// A clean resend repairs the pending-task annotations and nothing else: no new
// baseline is pinned, so the baseline freshness that justified the original
// signal does not need re-checking. The other two outcomes return
// *stateDoesntMatchError, the only error type handleTask maps to
// TaskState_INVALID_STATE — a plain error would map to TaskState_ERROR and
// report a permanent state mismatch as a transient failure. Neither writes the
// current task's ID into the pending-task annotations: pending state lives on
// the DDA, so not writing it is how "do not attach to someone else's signal"
// is expressed. The reconciler is the single authority that resolves the
// on-DDA signal, on its own timeline.
func (d *Daemon) planStartResend(
	ctx context.Context,
	req remoteAPIRequest,
	op resolvedOperation,
	dda *v2alpha1.DatadogAgent,
	pending *pendingOperation,
) (planResult, error) {
	rollbackTarget := dda.Annotations[v2alpha1.AnnotationExperimentRollbackTargetRevision]
	pinnedHash := dda.Annotations[v2alpha1.AnnotationExperimentExpectedSpecHash]
	if rollbackTarget == "" || pinnedHash == "" {
		return planResult{}, &stateDoesntMatchError{msg: fmt.Sprintf(
			"DatadogAgent %s/%s has an incomplete start signal for experiment %q; the reconciler will resolve it",
			dda.Namespace, dda.Name, req.Params.Version)}
	}
	if !d.rollbackTargetIsOwned(ctx, dda, rollbackTarget) {
		return planResult{}, &stateDoesntMatchError{msg: fmt.Sprintf(
			"DatadogAgent %s/%s pins rollback target %q, which does not resolve to an owned ControllerRevision",
			dda.Namespace, dda.Name, rollbackTarget)}
	}
	expectedHash, err := expectedSpecHashAfterMerge(dda, op.Config)
	if err != nil {
		return planResult{}, fmt.Errorf("start DatadogAgent experiment: %w", err)
	}
	if expectedHash != pinnedHash {
		// Same experiment ID, different intent: the pending signal belongs to a
		// prior write this task does not own.
		return planResult{}, &stateDoesntMatchError{msg: fmt.Sprintf(
			"DatadogAgent %s/%s has a start signal for experiment %q pinned to a different spec",
			dda.Namespace, dda.Name, req.Params.Version)}
	}
	if err := d.guardPendingOperationSlot(dda.Annotations, op.NamespacedName, *pending); err != nil {
		return planResult{}, err
	}
	return planResult{pending: pending}, nil
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
		switch dda.Status.Experiment.Phase {
		case v2alpha1.ExperimentPhaseTerminated:
			// Rollback already landed and the baseline is live, so a late stop
			// asks for something that is already true. Clear the experiment
			// config version and report DONE via the zero-value planResult.
			d.clearExperimentConfigVersion(req.Package)
			return planResult{}, nil
		case v2alpha1.ExperimentPhaseAborted, v2alpha1.ExperimentPhasePromoted:
			// Not legitimate no-ops: stop's revert-to-baseline intent was not
			// achieved. Aborted can leave the experiment spec stranded live, and
			// promoted made it the stable spec. Reporting DONE here (and
			// clearing the experiment config version) would tell the backend the
			// rollback happened. Surface it as a permanent state mismatch and
			// leave RC's experiment state alone -- only *stateDoesntMatchError
			// maps to TaskState_INVALID_STATE in handleTask.
			return planResult{}, &stateDoesntMatchError{msg: fmt.Sprintf(
				"stop DatadogAgent experiment: experiment %q did not roll back to the baseline: phase %q (reason %q)",
				experimentID, dda.Status.Experiment.Phase, dda.Status.Experiment.TerminationReason)}
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
