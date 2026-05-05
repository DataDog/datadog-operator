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

func operationScopeKey(pkgName string, nsn types.NamespacedName) string {
	return fmt.Sprintf("%s/%s/%s", pkgName, nsn.Namespace, nsn.Name)
}

type operationPlan struct {
	noop    bool
	patch   []byte
	pending *pendingOperation
}

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

func (d *Daemon) newPendingOperation(intent pendingIntent, req remoteAPIRequest, nsn types.NamespacedName, experimentID string) *pendingOperation {
	// The pending intent carries the Fleet-side identity needed to finish the
	// task later; the controller-facing signal is written separately in the DDA
	// annotations patch.
	return &pendingOperation{
		intent:       intent,
		taskID:       req.ID,
		packageName:  req.Package,
		nsn:          nsn,
		experimentID: experimentID,
	}
}

func (d *Daemon) applyOperationPlan(ctx context.Context, nsn types.NamespacedName, signalLog string, plan operationPlan) (*pendingOperation, error) {
	if plan.noop {
		return nil, nil
	}
	patch := plan.patch
	if plan.pending != nil {
		// Persist the daemon-owned pending record in the same patch as the
		// controller signal so a crash cannot leave us with "signal written but no
		// durable recovery record".
		var err error
		patch, err = buildPendingOperationPatch(patch, *plan.pending)
		if err != nil {
			return nil, fmt.Errorf("%s: failed to build pending operation patch: %w", signalLog, err)
		}
	}
	if len(patch) != 0 {
		dda := &v2alpha1.DatadogAgent{}
		dda.Name = nsn.Name
		dda.Namespace = nsn.Namespace
		if err := retryWithBackoff(ctx, func() error {
			return d.client.Patch(ctx, dda, client.RawPatch(types.MergePatchType, patch), client.FieldOwner("fleet-daemon"))
		}); err != nil {
			return nil, fmt.Errorf("%s: failed to patch annotation: %w", signalLog, err)
		}
		ctrl.LoggerFrom(ctx).Info("Wrote signal")
	}
	return plan.pending, nil
}

func buildPendingOperationPatch(basePatch []byte, op pendingOperation) ([]byte, error) {
	var patch map[string]any
	if len(basePatch) != 0 {
		if err := json.Unmarshal(basePatch, &patch); err != nil {
			return nil, fmt.Errorf("failed to unmarshal base patch: %w", err)
		}
	} else {
		patch = make(map[string]any)
	}

	metadata, ok := patch["metadata"].(map[string]any)
	if !ok {
		metadata = make(map[string]any)
		patch["metadata"] = metadata
	}
	annotations, ok := metadata["annotations"].(map[string]any)
	if !ok {
		annotations = make(map[string]any)
		metadata["annotations"] = annotations
	}
	// These daemon-owned annotations are the durable crash-recovery record for
	// the single current async intent on this DDA.
	annotations[v2alpha1.AnnotationPendingTaskID] = op.taskID
	annotations[v2alpha1.AnnotationPendingAction] = string(op.intent)
	annotations[v2alpha1.AnnotationPendingExperimentID] = op.experimentID
	annotations[v2alpha1.AnnotationPendingPackage] = op.packageName
	if op.resultVersion != "" {
		annotations[v2alpha1.AnnotationPendingResultVersion] = op.resultVersion
	} else {
		// Always write the full durable pending record. Merge patch leaves
		// unspecified annotation keys untouched, so omitting this field would
		// preserve a stale promote result-version from an older intent.
		annotations[v2alpha1.AnnotationPendingResultVersion] = nil
	}

	return json.Marshal(patch)
}

// startDatadogAgentExperiment starts a DatadogAgent experiment by atomically
// patching both the DDA spec (experiment config) and experiment signal annotations.
// If the annotation ID already matches and the reconciler has already set
// phase=running, the patch is skipped. After writing, the daemon waits for the
// reconciler to set phase=running before acking the task to RC.
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
	plan, err := d.planStart(ctx, req, op)
	if err != nil {
		return nil, err
	}
	logger.Info("Queued DatadogAgent experiment start acknowledgement")
	return d.applyOperationPlan(ctx, op.NamespacedName, "start DatadogAgent experiment", plan)
}

// stopDatadogAgentExperiment writes a rollback signal annotation on the DDA.
// If the phase is already terminal, the patch is skipped. After writing, the
// daemon waits for any terminal phase before acking the task to RC.
func (d *Daemon) stopDatadogAgentExperiment(ctx context.Context, req remoteAPIRequest) (*pendingOperation, error) {
	op, err := d.resolveOperation(req, "stop DatadogAgent experiment")
	if err != nil {
		return nil, err
	}

	ctx = ctrl.LoggerInto(ctx, ctrl.LoggerFrom(ctx).WithValues("id", req.ID, "namespace", op.NamespacedName.Namespace, "name", op.NamespacedName.Name))
	logger := ctrl.LoggerFrom(ctx)
	logger.V(1).Info("Stopping DatadogAgent experiment")
	plan, err := d.planStop(ctx, req, op)
	if err != nil {
		return nil, err
	}
	logger.Info("Queued DatadogAgent experiment stop acknowledgement")
	return d.applyOperationPlan(ctx, op.NamespacedName, "stop DatadogAgent experiment", plan)
}

// promoteDatadogAgentExperiment writes a promote signal annotation on the DDA.
// If the phase is already promoted, the patch is skipped. After writing, the
// daemon waits for phase=promoted before acking the task to RC.
func (d *Daemon) promoteDatadogAgentExperiment(ctx context.Context, req remoteAPIRequest) (*pendingOperation, error) {
	op, err := d.resolveOperation(req, signalPromoteDatadogAgentExperiment)
	if err != nil {
		return nil, err
	}

	ctx = ctrl.LoggerInto(ctx, ctrl.LoggerFrom(ctx).WithValues("id", req.ID, "namespace", op.NamespacedName.Namespace, "name", op.NamespacedName.Name))
	logger := ctrl.LoggerFrom(ctx)
	logger.V(1).Info("Promoting DatadogAgent experiment")
	plan, err := d.planPromote(ctx, req, op)
	if err != nil {
		return nil, err
	}
	logger.Info("Queued DatadogAgent experiment promote acknowledgement")
	return d.applyOperationPlan(ctx, op.NamespacedName, "promote DatadogAgent experiment", plan)
}

func (d *Daemon) planStart(ctx context.Context, req remoteAPIRequest, op resolvedOperation) (operationPlan, error) {
	experimentID := req.Params.Version
	plan := operationPlan{
		pending: d.newPendingOperation(pendingIntentStart, req, op.NamespacedName, experimentID),
	}
	dda := &v2alpha1.DatadogAgent{}
	if err := d.client.Get(ctx, op.NamespacedName, dda); err == nil {
		if dda.Annotations[v2alpha1.AnnotationExperimentID] == experimentID {
			if dda.Status.Experiment != nil && dda.Status.Experiment.Phase == v2alpha1.ExperimentPhaseRunning && dda.Status.Experiment.ID == experimentID {
				// Retry after restart: the controller already accepted this exact
				// experiment, so restore RC config state and complete synchronously.
				stable, _ := d.getPackageConfigVersions(req.Package)
				d.setPackageConfigVersions(req.Package, stable, req.Params.Version)
				return operationPlan{noop: true}, nil
			}
			// The start signal was already written for this experiment. Let the
			// background worker finish the task once status catches up.
			return plan, nil
		}
		if dda.Status.Experiment != nil && dda.Status.Experiment.Phase == v2alpha1.ExperimentPhaseRunning && dda.Status.Experiment.ID != experimentID {
			return operationPlan{}, fmt.Errorf("start DatadogAgent experiment: experiment %q already running", dda.Status.Experiment.ID)
		}
	}
	patch, err := buildSignalPatch(v2alpha1.ExperimentSignalStart, experimentID, op.Config)
	if err != nil {
		return operationPlan{}, fmt.Errorf("start DatadogAgent experiment: %w", err)
	}
	plan.patch = patch
	return plan, nil
}

func (d *Daemon) planStop(ctx context.Context, req remoteAPIRequest, op resolvedOperation) (operationPlan, error) {
	experimentID := req.Params.Version
	expectedExperimentID := experimentID
	dda := &v2alpha1.DatadogAgent{}
	if getErr := d.client.Get(ctx, op.NamespacedName, dda); getErr == nil {
		if dda.Status.Experiment == nil {
			if dda.Annotations[v2alpha1.AnnotationExperimentID] != experimentID || dda.Annotations[v2alpha1.AnnotationExperimentSignal] != v2alpha1.ExperimentSignalStart {
				// Truly idle object: no experiment status and no matching in-flight
				// start signal to roll back.
				d.clearExperimentConfigVersion(req.Package)
				return operationPlan{noop: true}, nil
			}
		} else {
			if isTerminalPhase(dda.Status.Experiment.Phase) {
				// Stop is idempotent once the experiment is already terminal.
				d.clearExperimentConfigVersion(req.Package)
				return operationPlan{noop: true}, nil
			}
			switch dda.Status.Experiment.Phase {
			case v2alpha1.ExperimentPhaseRunning:
				if dda.Status.Experiment.ID != experimentID {
					return operationPlan{}, fmt.Errorf("stop DatadogAgent experiment: running experiment %q does not match requested version %q", dda.Status.Experiment.ID, experimentID)
				}
			case "":
				// Transition 6 recovery: the spec/start signal was already applied,
				// but the reconciler has not written a phase yet.
				expectedExperimentID = experimentID
			default:
				return operationPlan{}, fmt.Errorf("stop DatadogAgent experiment: cannot stop, current phase is %q", dda.Status.Experiment.Phase)
			}
		}
	}
	patch, err := buildSignalPatch(v2alpha1.ExperimentSignalRollback, experimentID)
	if err != nil {
		return operationPlan{}, fmt.Errorf("stop DatadogAgent experiment: %w", err)
	}
	return operationPlan{
		patch:   patch,
		pending: d.newPendingOperation(pendingIntentStop, req, op.NamespacedName, expectedExperimentID),
	}, nil
}

func (d *Daemon) planPromote(ctx context.Context, req remoteAPIRequest, op resolvedOperation) (operationPlan, error) {
	experimentID := req.Params.Version
	_, experiment := d.getPackageConfigVersions(req.Package)
	if experiment == "" {
		return operationPlan{}, fmt.Errorf("promote DatadogAgent experiment: no experiment config version set")
	}
	expectedExperimentID := experimentID
	dda := &v2alpha1.DatadogAgent{}
	if getErr := d.client.Get(ctx, op.NamespacedName, dda); getErr == nil {
		if dda.Status.Experiment == nil || dda.Status.Experiment.Phase != v2alpha1.ExperimentPhaseRunning {
			currentPhase := ""
			if dda.Status.Experiment != nil {
				currentPhase = string(dda.Status.Experiment.Phase)
			}
			if dda.Status.Experiment != nil && dda.Status.Experiment.Phase == v2alpha1.ExperimentPhasePromoted {
				// Retry after restart: promotion already happened in the cluster, so
				// repair RC package state and finish synchronously.
				d.setPackageConfigVersions(req.Package, experiment, "")
				return operationPlan{noop: true}, nil
			}
			return operationPlan{}, fmt.Errorf("promote DatadogAgent experiment: cannot promote, current phase is %q", currentPhase)
		}
		if dda.Status.Experiment.ID != experimentID {
			return operationPlan{}, fmt.Errorf("promote DatadogAgent experiment: running experiment %q does not match requested version %q", dda.Status.Experiment.ID, experimentID)
		}
	}
	patch, err := buildSignalPatch(v2alpha1.ExperimentSignalPromote, experimentID)
	if err != nil {
		return operationPlan{}, fmt.Errorf("promote DatadogAgent experiment: %w", err)
	}
	pending := d.newPendingOperation(pendingIntentPromote, req, op.NamespacedName, expectedExperimentID)
	// Promote writes the current experiment config version into stable RC state
	// on success, which is not always identical to the experiment identity.
	pending.resultVersion = experiment
	return operationPlan{
		patch:   patch,
		pending: pending,
	}, nil
}
