// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fleet

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
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

	op := cfg.Operations[0]
	config := op.Config
	switch op.Operation {
	case OperationReplace:
		spec, err := extractReplaceSpec(op.Config)
		if err != nil {
			return resolvedOperation{}, fmt.Errorf("%s: invalid replace config: %w", signal, err)
		}
		config = spec
	case OperationUpdate:
		// config is already op.Config.
	default:
		return resolvedOperation{}, fmt.Errorf("%s: invalid operation: %s", signal, op.Operation)
	}

	return resolvedOperation{
		NamespacedName: req.Params.NamespacedName,
		Operation:      op.Operation,
		Config:         config,
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

func (d *Daemon) applyOperation(ctx context.Context, signal experimentSignal, pending *pendingOperation, sp signalPatch) (*pendingOperation, error) {
	if pending == nil {
		// A nil pending always means there's nothing to patch.
		return nil, nil
	}

	// Write the pending task in the same patch as the signal, whatever patch type
	// sp is. If the daemon restarts, the worker can read these annotations and
	// continue.
	patch, err := injectPendingAnnotations(sp, pending)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", signal, err)
	}

	dda := &v2alpha1.DatadogAgent{}
	dda.Name = pending.nsn.Name
	dda.Namespace = pending.nsn.Namespace
	if err := retryWithBackoff(ctx, func() error {
		// Strict field validation makes the API server reject unrecognized fields
		// instead of silently pruning them. This matters most for replace: without
		// it, a spec with fields the installed CRD doesn't know about (e.g. version
		// skew between fleet and the operator) would get pruned down to an empty
		// spec and accepted, wiping the resource without any error.
		return d.client.Patch(ctx, dda, client.RawPatch(sp.Type, patch), client.FieldOwner("fleet-daemon"), client.FieldValidation("Strict"))
	}); err != nil {
		return nil, fmt.Errorf("%s: failed to patch DatadogAgent: %w", signal, err)
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
	return d.applyOperation(ctx, signalStartDatadogAgentExperiment, pending, patch)
}

// stopDatadogAgentExperiment writes a rollback signal annotation on the DDA.
// If the phase is already terminal, the patch is skipped. After writing, the
// status worker waits for any terminal phase before marking the task done.
func (d *Daemon) stopDatadogAgentExperiment(ctx context.Context, req remoteAPIRequest) (*pendingOperation, error) {
	op, err := d.resolveOperation(req, signalStopDatadogAgentExperiment)
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
	return d.applyOperation(ctx, signalStopDatadogAgentExperiment, pending, patch)
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
	return d.applyOperation(ctx, signalPromoteDatadogAgentExperiment, pending, patch)
}

func (d *Daemon) planStart(ctx context.Context, req remoteAPIRequest, op resolvedOperation) (*pendingOperation, signalPatch, error) {
	experimentID := req.Params.Version
	pending := d.newPendingOperation(pendingIntentStart, req, op.NamespacedName, experimentID)
	dda := &v2alpha1.DatadogAgent{}
	if err := d.client.Get(ctx, op.NamespacedName, dda); err != nil {
		return nil, signalPatch{}, fmt.Errorf("%s: failed to get DatadogAgent: %w", signalStartDatadogAgentExperiment, err)
	}
	if experimentHasPhase(dda, experimentID, v2alpha1.ExperimentPhaseRunning) {
		// The controller already started this experiment. Update RC now and let
		// handleTask mark the task done.
		stable, _ := d.getPackageConfigVersions(req.Package)
		d.setPackageConfigVersions(req.Package, stable, req.Params.Version)
		return nil, signalPatch{}, nil
	}
	if dda.Annotations[v2alpha1.AnnotationExperimentID] == experimentID {
		// The start signal is already on the DDA. Keep the same signal, but make
		// sure the pending task annotations exist.
		if err := d.guardPendingOperationSlot(dda.Annotations, op.NamespacedName, *pending); err != nil {
			return nil, signalPatch{}, err
		}
		return pending, mergePatch(nil), nil
	}
	if runningID := runningExperimentID(dda); runningID != "" {
		return nil, signalPatch{}, fmt.Errorf("%s: experiment %q already running", signalStartDatadogAgentExperiment, runningID)
	}
	// Do not overwrite another unfinished task.
	if err := d.guardPendingOperationSlot(dda.Annotations, op.NamespacedName, *pending); err != nil {
		return nil, signalPatch{}, err
	}
	if op.Operation == OperationReplace {
		// If the operator has never reconciled this DDA, there's no
		// ControllerRevision to roll back to, so stopping the experiment later
		// would have nothing to restore. Require a baseline revision first.
		hasBaseline, err := d.hasControllerRevision(ctx, dda)
		if err != nil {
			return nil, signalPatch{}, fmt.Errorf("%s: %w", signalStartDatadogAgentExperiment, err)
		}
		if !hasBaseline {
			return nil, signalPatch{}, fmt.Errorf("%s: no baseline ControllerRevision exists yet for %s; wait for the operator to reconcile before starting a replace experiment", signalStartDatadogAgentExperiment, op.NamespacedName)
		}
		ops := buildReplaceSignalPatch(v2alpha1.ExperimentSignalStart, experimentID, op.Config, len(dda.Annotations) == 0)
		return pending, jsonPatch(ops), nil
	}

	patch, err := buildSignalPatch(v2alpha1.ExperimentSignalStart, experimentID, op.Config)
	if err != nil {
		return nil, signalPatch{}, fmt.Errorf("%s: %w", signalStartDatadogAgentExperiment, err)
	}
	return pending, mergePatch(patch), nil
}

// hasControllerRevision reports whether dda already owns a ControllerRevision.
func (d *Daemon) hasControllerRevision(ctx context.Context, dda *v2alpha1.DatadogAgent) (bool, error) {
	revList := &appsv1.ControllerRevisionList{}
	if err := d.client.List(ctx, revList,
		client.InNamespace(dda.Namespace),
		client.MatchingLabels{apicommon.DatadogAgentNameLabelKey: dda.Name},
	); err != nil {
		return false, fmt.Errorf("failed to list ControllerRevisions: %w", err)
	}
	for i := range revList.Items {
		for _, ref := range revList.Items[i].OwnerReferences {
			if ref.Controller != nil && *ref.Controller && ref.UID == dda.UID {
				return true, nil
			}
		}
	}
	return false, nil
}

func (d *Daemon) planStop(ctx context.Context, req remoteAPIRequest, op resolvedOperation) (*pendingOperation, signalPatch, error) {
	dda := &v2alpha1.DatadogAgent{}
	if getErr := d.client.Get(ctx, op.NamespacedName, dda); getErr != nil {
		return nil, signalPatch{}, fmt.Errorf("%s: failed to get DatadogAgent: %w", signalStopDatadogAgentExperiment, getErr)
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
			return nil, signalPatch{}, nil
		}
	} else {
		if isTerminalPhase(dda.Status.Experiment.Phase) {
			// The experiment is already stopped/promoted/aborted.
			d.clearExperimentConfigVersion(req.Package)
			return nil, signalPatch{}, nil
		}
		switch dda.Status.Experiment.Phase {
		case v2alpha1.ExperimentPhaseRunning:
			if experimentID == "" {
				return nil, signalPatch{}, fmt.Errorf("%s: running experiment is missing an ID", signalStopDatadogAgentExperiment)
			}
		case "":
			// Start was requested, but the reconciler has not written a phase yet.
			if experimentID == "" {
				return nil, signalPatch{}, fmt.Errorf("%s: current experiment is missing an ID", signalStopDatadogAgentExperiment)
			}
		default:
			return nil, signalPatch{}, fmt.Errorf("%s: cannot stop, current phase is %q", signalStopDatadogAgentExperiment, dda.Status.Experiment.Phase)
		}
	}
	pending := d.newPendingOperation(pendingIntentStop, req, op.NamespacedName, experimentID)
	if err := d.guardPendingOperationSlot(dda.Annotations, op.NamespacedName, *pending); err != nil {
		return nil, signalPatch{}, err
	}
	patch, err := buildSignalPatch(v2alpha1.ExperimentSignalRollback, experimentID)
	if err != nil {
		return nil, signalPatch{}, fmt.Errorf("%s: %w", signalStopDatadogAgentExperiment, err)
	}
	return pending, mergePatch(patch), nil
}

func (d *Daemon) planPromote(ctx context.Context, req remoteAPIRequest, op resolvedOperation) (*pendingOperation, signalPatch, error) {
	_, experiment := d.getPackageConfigVersions(req.Package)
	if experiment == "" {
		return nil, signalPatch{}, fmt.Errorf("%s: no experiment config version set", signalPromoteDatadogAgentExperiment)
	}
	dda := &v2alpha1.DatadogAgent{}
	if err := d.client.Get(ctx, op.NamespacedName, dda); err != nil {
		return nil, signalPatch{}, fmt.Errorf("%s: failed to get DatadogAgent: %w", signalPromoteDatadogAgentExperiment, err)
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
		// Promotion already happened. Update RC now and let handleTask mark the
		// task done.
		d.setPackageConfigVersions(req.Package, experiment, "")
		return nil, signalPatch{}, nil
	}
	if !experimentHasPhase(dda, experimentID, v2alpha1.ExperimentPhaseRunning) {
		currentPhase := ""
		if dda.Status.Experiment != nil {
			currentPhase = string(dda.Status.Experiment.Phase)
		}
		return nil, signalPatch{}, fmt.Errorf("%s: cannot promote, current phase is %q", signalPromoteDatadogAgentExperiment, currentPhase)
	}
	pending := d.newPendingOperation(pendingIntentPromote, req, op.NamespacedName, experimentID)
	// Promote makes the current experiment config the stable config on success.
	pending.resultVersion = experiment
	if err := d.guardPendingOperationSlot(dda.Annotations, op.NamespacedName, *pending); err != nil {
		return nil, signalPatch{}, err
	}
	patch, err := buildSignalPatch(v2alpha1.ExperimentSignalPromote, experimentID)
	if err != nil {
		return nil, signalPatch{}, fmt.Errorf("%s: %w", signalPromoteDatadogAgentExperiment, err)
	}
	return pending, mergePatch(patch), nil
}
