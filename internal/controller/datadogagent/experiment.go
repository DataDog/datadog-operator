// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/pkg/condition"
)

// ExperimentDefaultTimeout is the duration after which a running experiment is automatically rolled back.
const ExperimentDefaultTimeout = 15 * time.Minute

// errBaselineNotFound indicates a checkpointed rollback-target ControllerRevision
// is absent from the API server or fails ownedByDDA (including a same
// namespace/name revision owned by a different DDA UID). Distinct from a
// generic error so callers can record TerminationReasonBaselineNotFound
// instead of just requeuing on error.
var errBaselineNotFound = errors.New("rollback target ControllerRevision not found or not owned by this DatadogAgent")

// isTerminalPhase returns true if the phase is a terminal state (terminated, promoted, aborted).
func isTerminalPhase(phase v2alpha1.ExperimentPhase) bool {
	switch phase {
	case v2alpha1.ExperimentPhaseTerminated, v2alpha1.ExperimentPhasePromoted, v2alpha1.ExperimentPhaseAborted:
		return true
	default:
		return false
	}
}

// startSignalMatchesRecordedStatus reports whether a start signal's two pins
// are the ones already reflected in exp, which is what makes a same-ID signal
// against a terminal experiment a harmless leftover rather than a new write.
//
// On a successful start the pins are copied into the checkpoint verbatim, so
// comparing them there is exact. A terminal experiment with no checkpoint got
// there through a start-validation abort — the recorded verdict on these same
// pins — and re-judging them would only reach the same conclusion.
func startSignalMatchesRecordedStatus(
	exp *v2alpha1.ExperimentStatus,
	rollbackTargetRevision string,
	expectedSpecHash string,
) bool {
	if exp == nil {
		return false
	}
	if exp.Checkpoint != nil {
		return exp.Checkpoint.RollbackTargetRevision == rollbackTargetRevision &&
			exp.Checkpoint.ExpectedSpecHash == expectedSpecHash
	}
	if exp.Phase != v2alpha1.ExperimentPhaseAborted {
		return false
	}
	switch exp.TerminationReason {
	case v2alpha1.ExperimentTerminationReasonBaselineMissing,
		v2alpha1.ExperimentTerminationReasonBaselineNotFound,
		v2alpha1.ExperimentTerminationReasonManualSpecChange:
		return true
	default:
		return false
	}
}

// syncExperimentConfigStrandedCondition derives the ExperimentConfigStranded
// condition from status.Experiment every reconcile. It is True only when the
// experiment was aborted because its rollback baseline is unrecoverable
// (baseline_missing: never had a checkpoint; baseline_not_found: the
// checkpointed ControllerRevision is gone) — cases where the cluster is left
// running a config the user never approved and cannot be automatically
// restored. A manual-spec-change abort is not stranding: the user's own edit
// is standing, so there's nothing to report here.
//
// Follows the write-True-only convention: status.Conditions is rebuilt from
// scratch each reconcile (see generateNewStatusFromDDA), so there is no
// existing entry to flip back to False — omitting the call when the
// predicate is false is equivalent to a False write that's never appended.
func syncExperimentConfigStrandedCondition(newStatus *v2alpha1.DatadogAgentStatus, now metav1.Time) {
	exp := newStatus.Experiment
	if exp == nil || exp.Phase != v2alpha1.ExperimentPhaseAborted {
		return
	}
	switch exp.TerminationReason {
	case v2alpha1.ExperimentTerminationReasonBaselineMissing:
		condition.UpdateDatadogAgentStatusConditions(newStatus, now, common.ExperimentConfigStrandedConditionType,
			metav1.ConditionTrue, string(exp.TerminationReason),
			fmt.Sprintf("Experiment %q aborted with no recoverable rollback baseline; the running config was never approved and cannot be automatically restored", exp.ID),
			false)
	case v2alpha1.ExperimentTerminationReasonBaselineNotFound:
		condition.UpdateDatadogAgentStatusConditions(newStatus, now, common.ExperimentConfigStrandedConditionType,
			metav1.ConditionTrue, string(exp.TerminationReason),
			fmt.Sprintf("Experiment %q aborted because its rollback baseline could not be found; the running config was never approved and cannot be automatically restored", exp.ID),
			false)
	}
}

// manageExperiment handles all experiment state transitions for a reconcile cycle.
// Must be called before manageRevision.
//
// Returns specUpdated=true when a rollback restored the DDA spec via an
// Update. The caller must not continue rendering DDAI/dependencies from its
// now-stale defaulted instance in that case — publish terminal status and
// requeue instead.
func (r *Reconciler) manageExperiment(
	ctx context.Context,
	instance *v2alpha1.DatadogAgent,
	newStatus *v2alpha1.DatadogAgentStatus,
	now metav1.Time,
) (specUpdated bool, err error) {
	if experiment := instance.Status.Experiment; experiment != nil &&
		experiment.Phase == v2alpha1.ExperimentPhaseRunning &&
		experiment.Checkpoint == nil {
		// Preview-incompatible carryover state: a running experiment predating
		// the checkpoint contract, or a status write that never completed. The
		// checkpoint-dependent logic below assumes a running experiment always
		// carries a checkpoint, so fail closed here instead of letting it
		// nil-dereference.
		ctrl.LoggerFrom(ctx).Info("Aborting in-flight experiment with no checkpoint", "experimentID", experiment.ID)
		newStatus.Experiment = experiment.DeepCopy()
		newStatus.Experiment.Phase = v2alpha1.ExperimentPhaseAborted
		newStatus.Experiment.TerminationReason = v2alpha1.ExperimentTerminationReasonBaselineMissing
		return false, nil
	}

	// Snapshot the experiment status before processing to detect mutations.
	// The reason is part of the snapshot because a transition can keep the
	// phase and ID and change only the reason — aborting an already-aborted
	// experiment for a new cause — and that still has to count as a mutation
	// below, or the status write races the annotation clear.
	var oldPhase v2alpha1.ExperimentPhase
	var oldID string
	var oldReason v2alpha1.ExperimentTerminationReason
	if newStatus.Experiment != nil {
		oldPhase = newStatus.Experiment.Phase
		oldID = newStatus.Experiment.ID
		oldReason = newStatus.Experiment.TerminationReason
	}

	// Process annotation-based signals first — they take priority over
	// automatic timeout since they represent explicit human/RC intent.
	pendingClearID, signalUpdated, err := r.processExperimentSignal(ctx, instance, newStatus, now)
	if err != nil {
		return false, err
	}
	specUpdated = signalUpdated

	experiment := instance.Status.Experiment
	if experiment == nil {
		// No active experiment. If a signal was processed but did NOT create
		// a new experiment (i.e. it was a no-op like rollback/promote with
		// nothing to act on), clear the annotations so they don't get
		// reprocessed on every reconcile. Skip clearing when the signal
		// actually mutated status (e.g. a start signal that created an
		// experiment) — annotations will be cleared on the next reconcile
		// after the status update succeeds.
		if pendingClearID != "" && newStatus.Experiment == nil {
			if clearErr := r.clearExperimentAnnotations(ctx, instance, pendingClearID); clearErr != nil {
				ctrl.LoggerFrom(ctx).Error(clearErr, "Failed to clear experiment annotations, will retry on next reconcile")
			}
		}
		return specUpdated, nil
	}

	ctx = ctrl.LoggerInto(ctx, ctrl.LoggerFrom(ctx).WithValues("experimentID", experiment.ID))

	rollbackUpdated, err := r.handleRollback(ctx, instance, newStatus, now)
	if err != nil {
		return specUpdated, err
	}
	specUpdated = specUpdated || rollbackUpdated

	if err := r.abortExperiment(ctx, instance, experiment, newStatus); err != nil {
		return specUpdated, err
	}

	// Clear annotations only if the entire experiment management cycle did
	// not mutate the experiment status. Clearing bumps the DDA's
	// ResourceVersion, which would cause the subsequent status update to 409.
	// When status IS mutated, annotations are left for the next reconcile —
	// the idempotent path will detect the signal was already processed and
	// clear them then.
	if pendingClearID != "" {
		newPhase := v2alpha1.ExperimentPhase("")
		newID := ""
		newReason := v2alpha1.ExperimentTerminationReason("")
		if newStatus.Experiment != nil {
			newPhase = newStatus.Experiment.Phase
			newID = newStatus.Experiment.ID
			newReason = newStatus.Experiment.TerminationReason
		}
		if newPhase == oldPhase && newID == oldID && newReason == oldReason {
			logger := ctrl.LoggerFrom(ctx)
			if clearErr := r.clearExperimentAnnotations(ctx, instance, pendingClearID); clearErr != nil {
				logger.Error(clearErr, "Failed to clear experiment annotations, will retry on next reconcile")
			}
		}
	}

	return specUpdated, nil
}

// processExperimentSignal reads the experiment annotations on the DDA and
// translates them into status transitions. The daemon writes annotations;
// the controller is the sole writer of status.experiment.
//
// Returns the annotation ID that should be cleared after all experiment
// processing is complete. The caller (manageExperiment) decides when it is
// safe to clear based on whether the overall experiment status was mutated.
// specUpdated reports whether a rollback signal restored the DDA spec.
func (r *Reconciler) processExperimentSignal(
	ctx context.Context,
	instance *v2alpha1.DatadogAgent,
	newStatus *v2alpha1.DatadogAgentStatus,
	now metav1.Time,
) (pendingClearID string, specUpdated bool, err error) {
	annotations := instance.GetAnnotations()
	signal := annotations[v2alpha1.AnnotationExperimentSignal]
	annotationID := annotations[v2alpha1.AnnotationExperimentID]

	if signal == "" || annotationID == "" {
		return "", false, nil
	}

	ctx = ctrl.LoggerInto(ctx, ctrl.LoggerFrom(ctx).WithValues("signal", signal, "annotationID", annotationID))
	logger := ctrl.LoggerFrom(ctx)

	experiment := instance.Status.Experiment
	currentPhase := v2alpha1.ExperimentPhase("")
	currentID := ""
	if experiment != nil {
		currentPhase = experiment.Phase
		currentID = experiment.ID
	}

	var acted bool

	switch signal {
	case v2alpha1.ExperimentSignalStart:
		// The daemon writes its task identity into fleet.datadoghq.com/pending-task-id
		// alongside the start signal. Capture it here so we don't depend on the
		// pending annotations being available later (the worker clears them when
		// the start task completes).
		pendingTaskID := annotations[v2alpha1.AnnotationPendingTaskID]
		rollbackTargetRevision := annotations[v2alpha1.AnnotationExperimentRollbackTargetRevision]
		expectedSpecHash := annotations[v2alpha1.AnnotationExperimentExpectedSpecHash]
		acted, err = r.processStartSignal(ctx, instance, annotationID, currentPhase, currentID, newStatus, now, pendingTaskID, rollbackTargetRevision, expectedSpecHash)

	case v2alpha1.ExperimentSignalRollback:
		acted, specUpdated, err = r.processRollbackSignal(ctx, instance, annotationID, currentPhase, newStatus)

	case v2alpha1.ExperimentSignalPromote:
		acted, err = r.processPromoteSignal(ctx, instance, currentPhase, newStatus)

	default:
		logger.Info("Unknown experiment signal, ignoring")
		acted = true // clear unknown annotations to avoid infinite requeue
	}

	if err != nil {
		return "", specUpdated, err
	}

	if acted {
		return annotationID, specUpdated, nil
	}
	return "", specUpdated, nil
}

// processStartSignal handles the start annotation signal.
// Returns (true, nil) if it acted (or is a no-op that should clear annotations).
//
// now is captured into Status.Experiment.StartedAt and used by
// handleRollback as the timeout anchor, removing the dependency on
// ControllerRevision creation timestamps that could be stale for
// re-used revisions.
//
// The daemon's pending-task-id annotation is captured into
// Status.Experiment.StartTaskID so the daemon can later report
// TaskState_ERROR for the original task on local timeout. Persisting
// it on Status keeps the value durable across daemon restarts (the
// pending annotations get cleared once the start task completes).
//
// rollbackTargetRevision comes from the daemon's rollback-target-revision
// annotation (planStart writes it from the pre-experiment status.currentRevision
// barrier). If it's missing, there is no safe baseline to roll back to, so the
// experiment is immediately aborted with baseline_missing instead of starting —
// running an experiment nobody can roll back is worse than refusing to start it.
// If it's present but no longer resolves to an owned ControllerRevision, the
// abort reason is baseline_not_found: the difference matters because a name
// that resolved once and no longer does means Fleet's spec may be live with no
// way back, which is what ExperimentConfigStranded reports.
//
// instance must carry the raw (non-defaulted) spec — see rawInstance at the
// manageExperiment call site — so the recorded ExpectedSpecHash matches the
// same snapshot stored in ControllerRevisions.
func (r *Reconciler) processStartSignal(
	ctx context.Context,
	instance *v2alpha1.DatadogAgent,
	annotationID string,
	currentPhase v2alpha1.ExperimentPhase,
	currentID string,
	newStatus *v2alpha1.DatadogAgentStatus,
	now metav1.Time,
	pendingTaskID string,
	rollbackTargetRevision string,
	expectedSpecHashAnnotation string,
) (bool, error) {
	logger := ctrl.LoggerFrom(ctx)

	startedAt := now
	abort := func(reason v2alpha1.ExperimentTerminationReason) {
		newStatus.Experiment = &v2alpha1.ExperimentStatus{
			Phase:             v2alpha1.ExperimentPhaseAborted,
			ID:                annotationID,
			StartedAt:         &startedAt,
			StartTaskID:       pendingTaskID,
			TerminationReason: reason,
		}
	}

	// A same-ID start signal is usually the very signal that produced the
	// current status, still sitting on the object because clearing is deferred
	// to a pass that does not mutate status (see manageExperiment). But it can
	// also be a *new* start signal written against an experiment that already
	// finished. Fleet's planStart refuses to make that write, so only a dropped
	// guard or a hand-edited DDA produces it — and whatever wrote the signal
	// wrote the experiment spec with it, leaving unapproved config live under a
	// terminal status. Clearing that quietly would destroy the only evidence.
	//
	// The two cases are told apart by the pins, not the phase: the reconciler
	// copies a start signal's pins into the checkpoint verbatim, so pins that
	// still agree with the recorded status are the ones it already acted on.
	if annotationID == currentID {
		if !isTerminalPhase(currentPhase) {
			// Running, or the transitional empty phase. Already processed.
			return true, nil // idempotent — clear annotations
		}
		if startSignalMatchesRecordedStatus(instance.Status.Experiment, rollbackTargetRevision, expectedSpecHashAnnotation) {
			return true, nil // leftover from the start already processed
		}
		// Abort, which is what raises ExperimentConfigStranded: the operator
		// needs to see that the live spec was never approved and that no
		// baseline is recorded for getting back off it.
		logger.Info("Aborting experiment start: start signal was re-written for an experiment that already completed",
			"experimentID", annotationID, "terminalPhase", currentPhase)
		abort(v2alpha1.ExperimentTerminationReasonBaselineMissing)
		return true, nil
	}

	// Refuse to start a new experiment over a running one.
	if currentPhase == v2alpha1.ExperimentPhaseRunning {
		logger.Info("Ignoring start signal: experiment already running with different ID", "currentID", currentID)
		return true, nil // clear annotations — can't act on this
	}

	// Both pins are written atomically with the start signal, so either one
	// missing means the write was malformed or landed partially: there is no
	// trustworthy baseline to record.
	if rollbackTargetRevision == "" || expectedSpecHashAnnotation == "" {
		logger.Info("Aborting experiment start: start signal is missing a pinned baseline",
			"hasRollbackTarget", rollbackTargetRevision != "",
			"hasExpectedSpecHash", expectedSpecHashAnnotation != "")
		abort(v2alpha1.ExperimentTerminationReasonBaselineMissing)
		return true, nil
	}

	liveSpecHash, err := v2alpha1.ComputeSpecHash(instance.Spec, instance.GetAnnotations())
	if err != nil {
		return false, fmt.Errorf("failed to compute live spec hash: %w", err)
	}
	if liveSpecHash != expectedSpecHashAnnotation {
		// Something wrote the spec between Fleet's atomic start write and this
		// read. Recording the live hash here would enshrine that write as the
		// approved experiment state, so abort instead. The user's config is
		// what is live and it stands: this is not a stranded baseline.
		logger.Info("Aborting experiment start: live spec does not match the hash Fleet pinned with the start signal")
		abort(v2alpha1.ExperimentTerminationReasonManualSpecChange)
		return true, nil
	}

	// Fleet pins the rollback target from status.currentRevision, which is
	// owned by construction -- but the annotation is still a mutable name until
	// it lands in status, and a concurrent GC or delete can invalidate it in the
	// window before this read. Recording Running with an unverified target would
	// let Fleet mark the start task DONE and defer the failure to the next
	// rollback or timeout, by which point the config is stranded and RC already
	// reported success.
	rollbackTarget, err := r.getOwnedRevision(ctx, instance, rollbackTargetRevision)
	if err != nil {
		// Transient read failure: not proof the baseline is gone. Retry on the
		// next reconcile with the pending signal still in place, rather than
		// converting it into a permanent abort.
		return false, err
	}
	if rollbackTarget == nil {
		// The pin names something real but it does not resolve to an owned
		// ControllerRevision -- deleted, or a spoofed/foreign name. Fleet's
		// experiment spec is live and rollback cannot be proven safe.
		logger.Info("Aborting experiment start: rollback-target-revision annotation does not resolve to an owned ControllerRevision",
			"rollbackTargetRevision", rollbackTargetRevision)
		abort(v2alpha1.ExperimentTerminationReasonBaselineNotFound)
		return true, nil
	}

	logger.Info("Processing start signal")
	newStatus.Experiment = &v2alpha1.ExperimentStatus{
		Phase:       v2alpha1.ExperimentPhaseRunning,
		ID:          annotationID,
		StartedAt:   &startedAt,
		StartTaskID: pendingTaskID,
		Checkpoint: &v2alpha1.ExperimentCheckpoint{
			RollbackTargetRevision: rollbackTargetRevision,
			// Copied, never re-derived: the pin is Fleet's statement of what it
			// applied, and the equality check above is what makes copying safe.
			ExpectedSpecHash: expectedSpecHashAnnotation,
		},
	}
	return true, nil
}

// processRollbackSignal handles the rollback annotation signal.
// Returns (acted, specUpdated, err). acted indicates whether the caller
// should clear the signal annotations once it decides it's safe to do so.
func (r *Reconciler) processRollbackSignal(
	ctx context.Context,
	instance *v2alpha1.DatadogAgent,
	annotationID string,
	currentPhase v2alpha1.ExperimentPhase,
	newStatus *v2alpha1.DatadogAgentStatus,
) (acted bool, specUpdated bool, err error) {
	logger := ctrl.LoggerFrom(ctx)

	// Terminal phases: no-op, clear annotations.
	if isTerminalPhase(currentPhase) {
		logger.Info("Rollback signal ignored: experiment already in terminal phase", "phase", currentPhase)
		return true, false, nil
	}

	if currentPhase == v2alpha1.ExperimentPhaseRunning {
		// manageExperiment aborts a running experiment with a nil checkpoint
		// before signals are processed, so checkpoint is guaranteed non-nil here.
		checkpoint := instance.Status.Experiment.Checkpoint
		rollbackTarget, err := r.getOwnedRevision(ctx, instance, checkpoint.RollbackTargetRevision)
		if err != nil {
			return false, false, err
		}
		blocked, err := r.rollbackBlockedByManualChange(instance, checkpoint, rollbackTarget)
		if err != nil {
			return false, false, err
		}
		if blocked {
			logger.Info("Aborting experiment instead of rolling back: spec was manually changed")
			newStatus.Experiment.Phase = v2alpha1.ExperimentPhaseAborted
			newStatus.Experiment.TerminationReason = v2alpha1.ExperimentTerminationReasonManualSpecChange
			return true, false, nil
		}

		logger.Info("Processing rollback signal")
		updated, err := r.restorePreviousSpec(ctx, instance, newStatus, v2alpha1.ExperimentTerminationReasonStopped)
		return true, updated, err
	}

	// Transition 6: phase=="" but rollback annotation present. Recovery path
	// for a start signal whose spec patch landed but whose status write
	// (Phase=Running + checkpoint) never completed — the daemon retried with
	// a stop instead. Reconstruct the checkpoint from the validated
	// rollback-target annotation rather than treating this as a no-op.
	if currentPhase == "" {
		annotations := instance.GetAnnotations()
		rollbackTargetRevision := annotations[v2alpha1.AnnotationExperimentRollbackTargetRevision]
		expectedSpecHashAnnotation := annotations[v2alpha1.AnnotationExperimentExpectedSpecHash]
		if rollbackTargetRevision == "" {
			logger.Info("Rollback signal at nil phase: no rollback-target-revision annotation, nothing to roll back, clearing annotation")
			return true, false, nil
		}

		abort := func(reason v2alpha1.ExperimentTerminationReason) {
			newStatus.Experiment = &v2alpha1.ExperimentStatus{
				ID:                annotationID,
				StartTaskID:       annotations[v2alpha1.AnnotationPendingTaskID],
				Phase:             v2alpha1.ExperimentPhaseAborted,
				TerminationReason: reason,
			}
		}

		if expectedSpecHashAnnotation == "" {
			// Fleet writes both pins in one patch, so a rollback target without a
			// hash is a malformed signal, not a user edit. Classify it the same
			// way the start path does.
			logger.Info("Aborting recovered experiment: start signal pinned a rollback target but no expected spec hash")
			abort(v2alpha1.ExperimentTerminationReasonBaselineMissing)
			return true, false, nil
		}

		rollbackTarget, err := r.getOwnedRevision(ctx, instance, rollbackTargetRevision)
		if err != nil {
			return false, false, err
		}
		if rollbackTarget == nil {
			// The annotation names something real (unlike the empty-annotation
			// case above), but it doesn't resolve to an owned ControllerRevision —
			// deleted, or a spoofed/foreign name. Fleet's experiment spec is live
			// and we cannot prove this is a safe target, so the config is stranded.
			logger.Info("Aborting recovered experiment: rollback-target-revision annotation does not resolve to an owned ControllerRevision")
			abort(v2alpha1.ExperimentTerminationReasonBaselineNotFound)
			return true, false, nil
		}

		liveSpecHash, err := v2alpha1.ComputeSpecHash(instance.Spec, instance.GetAnnotations())
		if err != nil {
			return false, false, fmt.Errorf("failed to compute live spec hash: %w", err)
		}
		if liveSpecHash != expectedSpecHashAnnotation {
			// A user apply landed between Fleet's write and this reconcile. What
			// is live is the user's spec, not Fleet's, so there is nothing
			// stranded and nothing to restore — their edit stands.
			logger.Info("Aborting recovered experiment: live spec does not match the hash Fleet pinned with the start signal")
			abort(v2alpha1.ExperimentTerminationReasonManualSpecChange)
			return true, false, nil
		}

		logger.Info("Recovering from nil-phase rollback signal: synthesizing checkpoint and restoring baseline")
		newStatus.Experiment = &v2alpha1.ExperimentStatus{
			ID:          annotationID,
			StartTaskID: annotations[v2alpha1.AnnotationPendingTaskID],
			Checkpoint: &v2alpha1.ExperimentCheckpoint{
				RollbackTargetRevision: rollbackTargetRevision,
				ExpectedSpecHash:       expectedSpecHashAnnotation,
			},
		}
		updated, err := r.restorePreviousSpec(ctx, instance, newStatus, v2alpha1.ExperimentTerminationReasonStopped)
		return true, updated, err
	}

	return true, false, nil
}

// processPromoteSignal handles the promote annotation signal.
// Returns (true, nil) if it acted (or is a no-op that should clear annotations).
func (r *Reconciler) processPromoteSignal(
	ctx context.Context,
	instance *v2alpha1.DatadogAgent,
	currentPhase v2alpha1.ExperimentPhase,
	newStatus *v2alpha1.DatadogAgentStatus,
) (bool, error) {
	logger := ctrl.LoggerFrom(ctx)

	// Terminal phases: no-op, clear annotations.
	if isTerminalPhase(currentPhase) {
		logger.Info("Promote signal ignored: experiment already in terminal phase", "phase", currentPhase)
		return true, nil
	}

	// Can't promote if not running.
	if currentPhase != v2alpha1.ExperimentPhaseRunning {
		logger.Info("Promote signal ignored: no running experiment", "phase", currentPhase)
		return true, nil
	}

	// Verify spec still matches the experiment's checkpointed hash. If the
	// user manually changed the spec, abort instead of promoting. Unlike
	// rollback-bound paths, promote does not treat a spec matching the
	// rollback target as acceptable — promoting a reverted spec makes no sense.
	liveHash, err := v2alpha1.ComputeSpecHash(instance.Spec, instance.GetAnnotations())
	if err != nil {
		return false, fmt.Errorf("failed to compute live spec hash: %w", err)
	}
	if liveHash != instance.Status.Experiment.Checkpoint.ExpectedSpecHash {
		logger.Info("Aborting experiment instead of promoting: spec was manually changed")
		newStatus.Experiment.Phase = v2alpha1.ExperimentPhaseAborted
		newStatus.Experiment.TerminationReason = v2alpha1.ExperimentTerminationReasonManualSpecChange
		return true, nil
	}

	logger.Info("Processing promote signal")
	newStatus.Experiment.Phase = v2alpha1.ExperimentPhasePromoted
	return true, nil
}

// annotationToJSONPatchPath converts an annotation key to a JSON Patch path
// under /metadata/annotations, escaping "/" as "~1" per RFC 6901.
func annotationToJSONPatchPath(key string) string {
	return "/metadata/annotations/" + strings.ReplaceAll(key, "/", "~1")
}

// jsonPatchOp represents a single JSON Patch operation (RFC 6902).
type jsonPatchOp struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value string `json:"value,omitempty"`
}

// clearExperimentAnnotations removes the experiment signal annotations from the
// DDA using a conditional JSON Patch. The patch asserts the annotation ID matches
// the one we just processed, preventing accidental removal of a newer signal
// written concurrently by the daemon.
func (r *Reconciler) clearExperimentAnnotations(ctx context.Context, instance *v2alpha1.DatadogAgent, expectedID string) error {
	ops := []jsonPatchOp{
		{Op: "test", Path: annotationToJSONPatchPath(v2alpha1.AnnotationExperimentID), Value: expectedID},
		{Op: "remove", Path: annotationToJSONPatchPath(v2alpha1.AnnotationExperimentSignal)},
		{Op: "remove", Path: annotationToJSONPatchPath(v2alpha1.AnnotationExperimentID)},
	}
	// Only start signals carry the two pinned-baseline annotations, and a JSON
	// Patch "remove" on an absent path 422s, so only queue each when present.
	// Both are cleared together: leaving the hash behind would let a later
	// start signal inherit a stale pin. The leading "test" op above still
	// guards these removals against a signal written concurrently by the
	// daemon, since the whole patch is atomic.
	for _, key := range []string{
		v2alpha1.AnnotationExperimentRollbackTargetRevision,
		v2alpha1.AnnotationExperimentExpectedSpecHash,
	} {
		if _, ok := instance.GetAnnotations()[key]; ok {
			ops = append(ops, jsonPatchOp{Op: "remove", Path: annotationToJSONPatchPath(key)})
		}
	}
	patch, err := json.Marshal(ops)
	if err != nil {
		return fmt.Errorf("failed to marshal annotation clear patch: %w", err)
	}
	// Use a separate object for the Patch call so the server response (which
	// contains the non-defaulted spec) does not overwrite the caller's
	// defaulted instance.
	target := &v2alpha1.DatadogAgent{}
	target.Name = instance.Name
	target.Namespace = instance.Namespace
	return r.client.Patch(ctx, target, client.RawPatch(types.JSONPatchType, patch))
}

// abortExperiment marks the experiment as aborted in newStatus if a manual spec
// change is detected: the live raw spec's hash no longer matches
// Checkpoint.ExpectedSpecHash. It is a no-op if processExperimentSignal or
// handleRollback has already set a terminal phase, preventing spurious abort
// logs and phase overwrites.
func (r *Reconciler) abortExperiment(
	ctx context.Context,
	instance *v2alpha1.DatadogAgent,
	experiment *v2alpha1.ExperimentStatus,
	newStatus *v2alpha1.DatadogAgentStatus,
) error {
	if experiment.Phase != v2alpha1.ExperimentPhaseRunning {
		return nil
	}
	if newStatus.Experiment.Phase != v2alpha1.ExperimentPhaseRunning {
		// handleRollback already determined a terminal phase (e.g. timeout); don't overwrite or log.
		return nil
	}
	// manageExperiment aborts a running experiment with a nil checkpoint
	// before this is reached, so Checkpoint is guaranteed non-nil here.
	liveHash, err := v2alpha1.ComputeSpecHash(instance.Spec, instance.GetAnnotations())
	if err != nil {
		return fmt.Errorf("failed to compute live spec hash: %w", err)
	}
	if liveHash == experiment.Checkpoint.ExpectedSpecHash {
		// Spec matches the checkpointed experiment spec — no manual change detected.
		return nil
	}
	ctrl.LoggerFrom(ctx).Info("Aborting experiment due to manual spec change")
	newStatus.Experiment.Phase = v2alpha1.ExperimentPhaseAborted
	newStatus.Experiment.TerminationReason = v2alpha1.ExperimentTerminationReasonManualSpecChange
	return nil
}

// handleRollback checks if the experiment needs timeout-based rollback.
// Rollback signals are handled by processExperimentSignal (annotation-based).
// Returns whether the spec was updated (see restorePreviousSpec/rollback).
func (r *Reconciler) handleRollback(
	ctx context.Context,
	instance *v2alpha1.DatadogAgent,
	newStatus *v2alpha1.DatadogAgentStatus,
	now metav1.Time,
) (bool, error) {
	if instance.Status.Experiment == nil {
		return false, nil
	}

	phase := instance.Status.Experiment.Phase

	// If processExperimentSignal already set a new phase, skip timeout logic.
	if newStatus.Experiment != nil && newStatus.Experiment.Phase != phase {
		return false, nil
	}

	if phase != v2alpha1.ExperimentPhaseRunning {
		return false, nil
	}

	if instance.Status.Experiment.StartedAt == nil {
		return false, nil
	}
	// status.experiment.startedAt is the timeout anchor rather than any
	// revision's CreationTimestamp, which can be hours/days older than the
	// experiment and would otherwise trigger an immediate timeout on the
	// first reconcile after Phase=Running.
	elapsed := now.Sub(instance.Status.Experiment.StartedAt.Time)
	if elapsed < getExperimentTimeout(r.options.ExperimentTimeout) {
		return false, nil
	}

	logger := ctrl.LoggerFrom(ctx)

	// manageExperiment aborts a running experiment with a nil checkpoint
	// before this is reached, so Checkpoint is guaranteed non-nil here.
	checkpoint := instance.Status.Experiment.Checkpoint
	rollbackTarget, err := r.getOwnedRevision(ctx, instance, checkpoint.RollbackTargetRevision)
	if err != nil {
		return false, err
	}
	blocked, err := r.rollbackBlockedByManualChange(instance, checkpoint, rollbackTarget)
	if err != nil {
		return false, err
	}
	if blocked {
		logger.Info("Timeout elapsed but spec was manually changed; aborting instead of rolling back", "elapsed", elapsed.String())
		newStatus.Experiment.Phase = v2alpha1.ExperimentPhaseAborted
		newStatus.Experiment.TerminationReason = v2alpha1.ExperimentTerminationReasonManualSpecChange
		return false, nil
	}

	logger.Info("Experiment timed out, rolling back", "elapsed", elapsed.String())
	return r.restorePreviousSpec(ctx, instance, newStatus, v2alpha1.ExperimentTerminationReasonTimedOut)
}

// restorePreviousSpec restores the DDA spec from the experiment checkpoint's
// rollback target and, on success, sets the terminal experiment phase to
// terminated with the given reason. If the rollback target cannot be
// validated (absent or not owned by this DDA), the experiment is instead
// aborted with baseline_not_found rather than left running.
//
// Returns whether the spec was actually updated (false for an idempotent
// rollback whose live spec already matched the target, or a baseline_not_found
// abort).
func (r *Reconciler) restorePreviousSpec(
	ctx context.Context,
	instance *v2alpha1.DatadogAgent,
	newStatus *v2alpha1.DatadogAgentStatus,
	terminationReason v2alpha1.ExperimentTerminationReason,
) (bool, error) {
	checkpoint := newStatus.Experiment.Checkpoint
	if checkpoint == nil {
		return false, fmt.Errorf("cannot restore previous spec: experiment has no checkpoint")
	}

	updated, err := r.rollback(ctx, instance, checkpoint.RollbackTargetRevision)
	if err != nil {
		if errors.Is(err, errBaselineNotFound) {
			ctrl.LoggerFrom(ctx).Info("Aborting experiment: rollback baseline not found", "error", err.Error())
			newStatus.Experiment.Phase = v2alpha1.ExperimentPhaseAborted
			newStatus.Experiment.TerminationReason = v2alpha1.ExperimentTerminationReasonBaselineNotFound
			return false, nil
		}
		return false, err
	}
	newStatus.Experiment.Phase = v2alpha1.ExperimentPhaseTerminated
	newStatus.Experiment.TerminationReason = terminationReason
	return updated, nil
}

// rollback restores the DDA spec from the named ControllerRevision. The
// rollback target name is selected by Fleet but travels through a
// user-writable annotation before the reconciler copies it into the
// checkpoint, so it is treated as untrusted input: the revision is read
// through the uncached API reader and validated with ownedByDDA rather than
// applied by name alone.
//
// Returns whether the spec was actually updated. false means there was no
// target, or the live spec already matched it (idempotent rollback). After a
// successful Update, instance.ResourceVersion is synced so the caller's
// subsequent status update won't 409.
func (r *Reconciler) rollback(
	ctx context.Context,
	instance *v2alpha1.DatadogAgent,
	rollbackTarget string,
) (bool, error) {
	if rollbackTarget == "" {
		ctrl.LoggerFrom(ctx).Info("No previous revision to roll back to, skipping spec restore")
		return false, nil
	}

	cr, err := r.getOwnedRevision(ctx, instance, rollbackTarget)
	if err != nil {
		return false, err
	}
	if cr == nil {
		return false, fmt.Errorf("%w: %s", errBaselineNotFound, rollbackTarget)
	}

	var snapshot v2alpha1.RevisionSnapshot
	if err = json.Unmarshal(cr.Data.Raw, &snapshot); err != nil {
		return false, fmt.Errorf("failed to decode ControllerRevision data: %w", err)
	}

	// Re-fetch for the latest ResourceVersion and to check whether the spec is
	// rolled back already. If it is, skip the update.
	nsn := types.NamespacedName{Namespace: instance.Namespace, Name: instance.Name}
	current := &v2alpha1.DatadogAgent{}
	if err = r.client.Get(ctx, nsn, current); err != nil {
		return false, fmt.Errorf("failed to get current DDA for rollback: %w", err)
	}
	currentSnap, err := v2alpha1.BuildRevisionSnapshot(current.Spec, current.GetAnnotations())
	if err != nil {
		return false, fmt.Errorf("failed to marshal current snapshot for comparison: %w", err)
	}
	if bytes.Equal(currentSnap, cr.Data.Raw) {
		ctrl.LoggerFrom(ctx).Info("Rollback spec already matches target, skipping update", "rollbackTarget", rollbackTarget)
		// No update happened, but still sync the re-fetched ResourceVersion so
		// the caller's status update doesn't 409 against a concurrent write.
		instance.ResourceVersion = current.ResourceVersion
		return false, nil
	}

	// Restore the baseline's Datadog-filter annotation set. This is a
	// set-replace, not an overlay: every Datadog-filter key currently live is
	// dropped before the snapshot's keys go back on, so an annotation the
	// experiment (or a user) added after the baseline does not survive the
	// rollback. Annotations outside the filter -- user metadata, tooling keys
	// -- are untouched.
	merged := maps.Clone(current.Annotations)
	if merged == nil {
		merged = make(map[string]string, len(snapshot.Annotations))
	}
	for key := range v2alpha1.DatadogAnnotations(current.Annotations) {
		delete(merged, key)
	}
	maps.Copy(merged, snapshot.Annotations)

	toUpdate := &v2alpha1.DatadogAgent{
		ObjectMeta: current.ObjectMeta,
		Spec:       snapshot.Spec,
	}
	toUpdate.Annotations = merged
	if err := r.client.Update(ctx, toUpdate); err != nil {
		return false, err
	}
	// Sync the new ResourceVersion back so the caller's status update
	// uses the correct RV and doesn't 409.
	instance.ResourceVersion = toUpdate.ResourceVersion
	return true, nil
}

// rollbackBlockedByManualChange reports whether a rollback-bound path
// (explicit rollback signal or timeout rollback) must refuse to restore the
// baseline because the live spec is neither the checkpointed experiment spec
// nor the validated rollback target. A live snapshot that already equals the
// rollback target is an interrupted rollback — a prior restore whose status
// write was lost — not a manual change, so rollback-bound paths complete it
// instead of misreporting it as an abort. Plain running reconciliation
// (abortExperiment) and promote do not get this exception: they compare
// against ExpectedSpecHash only.
func (r *Reconciler) rollbackBlockedByManualChange(
	instance *v2alpha1.DatadogAgent,
	checkpoint *v2alpha1.ExperimentCheckpoint,
	rollbackTarget *appsv1.ControllerRevision,
) (bool, error) {
	liveHash, err := v2alpha1.ComputeSpecHash(instance.Spec, instance.GetAnnotations())
	if err != nil {
		return false, fmt.Errorf("failed to compute live spec hash: %w", err)
	}
	if liveHash == checkpoint.ExpectedSpecHash {
		return false, nil
	}
	if rollbackTarget != nil {
		liveSnap, err := v2alpha1.BuildRevisionSnapshot(instance.Spec, instance.GetAnnotations())
		if err != nil {
			return false, fmt.Errorf("failed to build live snapshot: %w", err)
		}
		if bytes.Equal(liveSnap, rollbackTarget.Data.Raw) {
			return false, nil
		}
	}
	return true, nil
}

// getOwnedRevision reads the named ControllerRevision through the uncached
// API reader and verifies ownership with ownedByDDA. A cached/informer-backed
// client's stale NotFound could be mistaken for a permanently-lost baseline,
// and applying a revision by name alone (the name travels through a
// user-writable annotation) without an ownership check would let a crafted
// annotation apply an arbitrary revision's snapshot.
//
// Returns (nil, nil) — not an error — when the revision is absent or not
// owned by instance; callers treat that as baseline_not_found. A transient
// read failure is returned as an error so the caller can retry instead of
// recording a terminal experiment status.
func (r *Reconciler) getOwnedRevision(ctx context.Context, instance *v2alpha1.DatadogAgent, name string) (*appsv1.ControllerRevision, error) {
	if name == "" {
		return nil, nil
	}
	rev := &appsv1.ControllerRevision{}
	if err := r.options.APIReader.Get(ctx, types.NamespacedName{Namespace: instance.Namespace, Name: name}, rev); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get ControllerRevision %s: %w", name, err)
	}
	if !ownedByDDA(rev, instance) {
		return nil, nil
	}
	return rev, nil
}

func getExperimentTimeout(timeout time.Duration) time.Duration {
	if timeout == 0 {
		return ExperimentDefaultTimeout
	}
	return timeout
}
