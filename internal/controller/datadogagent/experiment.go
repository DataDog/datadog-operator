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
)

// ExperimentDefaultTimeout is the duration after which a running experiment is automatically rolled back.
const ExperimentDefaultTimeout = 15 * time.Minute

// Termination reasons for ExperimentPhaseTerminated.
const (
	// ExperimentTerminationReasonStopped indicates the experiment was explicitly rolled back via a rollback signal.
	ExperimentTerminationReasonStopped = "stopped"
	// ExperimentTerminationReasonTimedOut indicates the experiment exceeded the timeout and was auto-rolled back.
	ExperimentTerminationReasonTimedOut = "timed_out"
	// ExperimentTerminationReasonBaselineMissing indicates a start signal
	// arrived without the daemon-written rollback checkpoint annotation, so
	// the experiment could not be safely accepted.
	ExperimentTerminationReasonBaselineMissing = "baseline_missing"
	// ExperimentTerminationReasonBaselineNotFound indicates the ControllerRevision
	// named by Status.Experiment.RollbackTargetRevision no longer exists at
	// rollback time (e.g. external deletion).
	ExperimentTerminationReasonBaselineNotFound = "baseline_not_found"
)

// annotationExperimentState records the terminal outcome of an experiment
// on a ControllerRevision. The value is one of experimentRevisionState.
//
// Used by handleRollback to skip the timeout check on revisions whose
// CreationTimestamp is stale from a prior experiment, and by ensureRevision
// to refresh a rolled-back revision's timestamp when the same spec is
// re-applied. The annotation is single-valued so a revision cannot
// simultaneously represent two terminal outcomes.
const annotationExperimentState = "operator.datadoghq.com/experiment-state"

// experimentRevisionState is the value stored at annotationExperimentState.
type experimentRevisionState string

const (
	experimentRevisionStatePromoted   experimentRevisionState = "promoted"
	experimentRevisionStateRolledBack experimentRevisionState = "rolled-back"
)

// isTerminalPhase returns true if the phase is a terminal state (terminated, promoted, aborted).
func isTerminalPhase(phase v2alpha1.ExperimentPhase) bool {
	switch phase {
	case v2alpha1.ExperimentPhaseTerminated, v2alpha1.ExperimentPhasePromoted, v2alpha1.ExperimentPhaseAborted:
		return true
	default:
		return false
	}
}

// manageExperiment handles all experiment state transitions for a reconcile cycle.
// Must be called before manageRevision.
func (r *Reconciler) manageExperiment(
	ctx context.Context,
	instance *v2alpha1.DatadogAgent,
	newStatus *v2alpha1.DatadogAgentStatus,
	now metav1.Time,
	revList []appsv1.ControllerRevision,
) error {
	// Snapshot the experiment status before processing to detect mutations.
	var oldPhase v2alpha1.ExperimentPhase
	var oldID string
	if newStatus.Experiment != nil {
		oldPhase = newStatus.Experiment.Phase
		oldID = newStatus.Experiment.ID
	}

	// Process annotation-based signals first — they take priority over
	// automatic timeout since they represent explicit human/RC intent.
	pendingClearID, err := r.processExperimentSignal(ctx, instance, newStatus, now, revList)
	if err != nil {
		return err
	}

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
		return nil
	}

	ctx = ctrl.LoggerInto(ctx, ctrl.LoggerFrom(ctx).WithValues("experimentID", experiment.ID))

	if err := r.handleRollback(ctx, instance, newStatus, now, revList); err != nil {
		return err
	}
	// Mark the highest revision when promoted so its stale timestamp doesn't
	// cause a false timeout if a new experiment starts before manageRevision
	// creates a fresh revision.
	if experiment.Phase == v2alpha1.ExperimentPhasePromoted {
		if rev := highestRevision(revList); rev != nil {
			r.markRevisionState(ctx, rev, experimentRevisionStatePromoted)
		}
	}
	r.abortExperiment(ctx, instance, experiment, newStatus, revList)

	// Clear annotations only if the entire experiment management cycle did
	// not mutate the experiment status. Clearing bumps the DDA's
	// ResourceVersion, which would cause the subsequent status update to 409.
	// When status IS mutated, annotations are left for the next reconcile —
	// the idempotent path will detect the signal was already processed and
	// clear them then.
	if pendingClearID != "" {
		newPhase := v2alpha1.ExperimentPhase("")
		newID := ""
		if newStatus.Experiment != nil {
			newPhase = newStatus.Experiment.Phase
			newID = newStatus.Experiment.ID
		}
		if newPhase == oldPhase && newID == oldID {
			logger := ctrl.LoggerFrom(ctx)
			if clearErr := r.clearExperimentAnnotations(ctx, instance, pendingClearID); clearErr != nil {
				logger.Error(clearErr, "Failed to clear experiment annotations, will retry on next reconcile")
			}
		}
	}

	return nil
}

// processExperimentSignal reads the experiment annotations on the DDA and
// translates them into status transitions. The daemon writes annotations;
// the controller is the sole writer of status.experiment.
//
// Returns the annotation ID that should be cleared after all experiment
// processing is complete. The caller (manageExperiment) decides when it is
// safe to clear based on whether the overall experiment status was mutated.
func (r *Reconciler) processExperimentSignal(
	ctx context.Context,
	instance *v2alpha1.DatadogAgent,
	newStatus *v2alpha1.DatadogAgentStatus,
	now metav1.Time,
	revisions []appsv1.ControllerRevision,
) (pendingClearID string, err error) {
	annotations := instance.GetAnnotations()
	signal := annotations[v2alpha1.AnnotationExperimentSignal]
	annotationID := annotations[v2alpha1.AnnotationExperimentID]

	if signal == "" || annotationID == "" {
		return "", nil
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
		rollbackTarget := annotations[v2alpha1.AnnotationExperimentRollbackTargetRevision]
		acted, err = r.processStartSignal(ctx, instance, annotationID, currentPhase, currentID, newStatus, now, pendingTaskID, rollbackTarget)

	case v2alpha1.ExperimentSignalRollback:
		acted, err = r.processRollbackSignal(ctx, instance, annotationID, currentPhase, newStatus, revisions)

	case v2alpha1.ExperimentSignalPromote:
		acted, err = r.processPromoteSignal(ctx, instance, currentPhase, newStatus, revisions)

	default:
		logger.Info("Unknown experiment signal, ignoring")
		acted = true // clear unknown annotations to avoid infinite requeue
	}

	if err != nil {
		return "", err
	}

	if acted {
		return annotationID, nil
	}
	return "", nil
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
// rollbackTarget is the ControllerRevision name the daemon captured
// as the pre-experiment baseline in the same MergePatch that carried
// this start signal. It is copied into Status.Experiment.RollbackTargetRevision
// so rollback can restore by name without walking revision history.
// A missing value aborts the experiment: without a proven baseline,
// rollback cannot be safely performed.
//
// instance.Spec at this point is the experiment spec (the daemon's
// MergePatch overwrote spec + signal annotations atomically), so
// ExpectedSpecHash captured here is the content hash of the intended
// experiment state, used by later reconciles to detect manual changes.
func (r *Reconciler) processStartSignal(
	ctx context.Context,
	instance *v2alpha1.DatadogAgent,
	annotationID string,
	currentPhase v2alpha1.ExperimentPhase,
	currentID string,
	newStatus *v2alpha1.DatadogAgentStatus,
	now metav1.Time,
	pendingTaskID string,
	rollbackTarget string,
) (bool, error) {
	logger := ctrl.LoggerFrom(ctx)
	// Already processed: same ID already in status.
	if annotationID == currentID {
		return true, nil // idempotent — clear annotations
	}

	// Refuse to start a new experiment over a running one.
	if currentPhase == v2alpha1.ExperimentPhaseRunning {
		logger.Info("Ignoring start signal: experiment already running with different ID", "currentID", currentID)
		return true, nil // clear annotations — can't act on this
	}

	// Missing rollback checkpoint: abort. Without a named baseline, rollback
	// cannot be proven safe. Persist StartTaskID before publishing Aborted so
	// the daemon's reconcileLocallyTerminatedExperiment path can report the
	// start task as ERROR to Remote Config.
	if rollbackTarget == "" {
		logger.Info("Aborting start signal: rollback checkpoint missing from annotations")
		startedAt := now
		newStatus.Experiment = &v2alpha1.ExperimentStatus{
			Phase:             v2alpha1.ExperimentPhaseAborted,
			ID:                annotationID,
			StartedAt:         &startedAt,
			StartTaskID:       pendingTaskID,
			TerminationReason: ExperimentTerminationReasonBaselineMissing,
		}
		return true, nil
	}

	specHash, err := computeSpecHash(instance.Spec, instance.GetAnnotations())
	if err != nil {
		return false, fmt.Errorf("compute expected spec hash: %w", err)
	}

	logger.Info("Processing start signal", "rollbackTarget", rollbackTarget)
	startedAt := now
	newStatus.Experiment = &v2alpha1.ExperimentStatus{
		Phase:                  v2alpha1.ExperimentPhaseRunning,
		ID:                     annotationID,
		StartedAt:              &startedAt,
		StartTaskID:            pendingTaskID,
		RollbackTargetRevision: rollbackTarget,
		ExpectedSpecHash:       specHash,
	}
	return true, nil
}

// processRollbackSignal handles the rollback annotation signal.
// Returns (true, nil) if it acted (or is a no-op that should clear annotations).
func (r *Reconciler) processRollbackSignal(
	ctx context.Context,
	instance *v2alpha1.DatadogAgent,
	annotationID string,
	currentPhase v2alpha1.ExperimentPhase,
	newStatus *v2alpha1.DatadogAgentStatus,
	revisions []appsv1.ControllerRevision,
) (bool, error) {
	logger := ctrl.LoggerFrom(ctx)

	// Terminal phases: no-op, clear annotations.
	if isTerminalPhase(currentPhase) {
		logger.Info("Rollback signal ignored: experiment already in terminal phase", "phase", currentPhase)
		return true, nil
	}

	if currentPhase == v2alpha1.ExperimentPhaseRunning {
		// Check if spec was manually changed (user edit takes precedence over rollback).
		changed, err := specHashDiffers(instance, newStatus.Experiment)
		if err != nil {
			return false, err
		}
		if changed {
			logger.Info("Aborting experiment instead of rolling back: spec was manually changed")
			newStatus.Experiment.Phase = v2alpha1.ExperimentPhaseAborted
			return true, nil
		}

		logger.Info("Processing rollback signal")
		return true, r.restorePreviousSpec(ctx, instance, newStatus, revisions, ExperimentTerminationReasonStopped)
	}

	// currentPhase == "" is a no-op: without Status.Experiment, there is no
	// tracked experiment to roll back. Under the checkpoint model,
	// processStartSignal always creates Status.Experiment atomically with
	// reading the rollback-target annotation, so a rollback signal at nil
	// phase means the experiment is either already fully cleared or was
	// never started.
	if currentPhase == "" {
		logger.Info("Rollback signal at nil phase: nothing to roll back, clearing annotation")
		return true, nil
	}

	return true, nil
}

// specHashDiffers returns true when the current DatadogAgent's canonical spec
// hash differs from the ExpectedSpecHash captured at experiment start. Callers
// pre-check the phase (only meaningful while Running). Returns (false, nil)
// when ExpectedSpecHash is empty (upgrade path — no captured hash to compare).
func specHashDiffers(instance *v2alpha1.DatadogAgent, exp *v2alpha1.ExperimentStatus) (bool, error) {
	if exp == nil || exp.ExpectedSpecHash == "" {
		return false, nil
	}
	live, err := computeSpecHash(instance.Spec, instance.GetAnnotations())
	if err != nil {
		return false, fmt.Errorf("compute live spec hash: %w", err)
	}
	return live != exp.ExpectedSpecHash, nil
}

// processPromoteSignal handles the promote annotation signal.
// Returns (true, nil) if it acted (or is a no-op that should clear annotations).
func (r *Reconciler) processPromoteSignal(
	ctx context.Context,
	instance *v2alpha1.DatadogAgent,
	currentPhase v2alpha1.ExperimentPhase,
	newStatus *v2alpha1.DatadogAgentStatus,
	revisions []appsv1.ControllerRevision,
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

	// Verify the live spec still matches the checkpoint captured at start.
	// If the user manually changed the spec, abort instead of promoting.
	changed, err := specHashDiffers(instance, newStatus.Experiment)
	if err != nil {
		return false, err
	}
	if changed {
		logger.Info("Aborting experiment instead of promoting: spec was manually changed")
		newStatus.Experiment.Phase = v2alpha1.ExperimentPhaseAborted
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

// abortExperiment marks the experiment as aborted in newStatus if the live
// canonical spec hash differs from Status.Experiment.ExpectedSpecHash captured
// at Running transition — the user has changed the spec while the experiment
// was running. Runs only while Phase=Running (a no-op after handleRollback or
// processExperimentSignal has set a terminal phase). No-op when the checkpoint
// hash is empty (upgrade path — see the compatibility handling in Phase 8).
func (r *Reconciler) abortExperiment(
	ctx context.Context,
	instance *v2alpha1.DatadogAgent,
	experiment *v2alpha1.ExperimentStatus,
	newStatus *v2alpha1.DatadogAgentStatus,
	_ []appsv1.ControllerRevision,
) {
	if experiment.Phase != v2alpha1.ExperimentPhaseRunning {
		return
	}
	if newStatus.Experiment.Phase != v2alpha1.ExperimentPhaseRunning {
		return
	}
	changed, err := specHashDiffers(instance, newStatus.Experiment)
	if err != nil {
		ctrl.LoggerFrom(ctx).Error(err, "Skipping manual-change check")
		return
	}
	if !changed {
		return
	}
	ctrl.LoggerFrom(ctx).Info("Aborting experiment due to manual spec change")
	newStatus.Experiment.Phase = v2alpha1.ExperimentPhaseAborted
}

// handleRollback checks if the experiment needs timeout-based rollback.
// Rollback signals are handled by processExperimentSignal (annotation-based).
//
// Under the checkpoint model timeout is a pure phase-and-elapsed check on
// Status.Experiment.StartedAt — the previous logic that walked revList to
// disambiguate stale timestamps is gone. abortExperiment (also driven by
// hash comparison) covers manual-change detection independently, so no
// gating on "does the current spec match any revision" is required here.
func (r *Reconciler) handleRollback(
	ctx context.Context,
	instance *v2alpha1.DatadogAgent,
	newStatus *v2alpha1.DatadogAgentStatus,
	now metav1.Time,
	revisions []appsv1.ControllerRevision,
) error {
	if instance.Status.Experiment == nil {
		return nil
	}

	phase := instance.Status.Experiment.Phase

	// If processExperimentSignal already set a new phase, skip timeout logic.
	if newStatus.Experiment != nil && newStatus.Experiment.Phase != phase {
		return nil
	}

	if phase != v2alpha1.ExperimentPhaseRunning {
		return nil
	}
	if instance.Status.Experiment.StartedAt == nil {
		return nil
	}

	logger := ctrl.LoggerFrom(ctx)
	elapsed := now.Sub(instance.Status.Experiment.StartedAt.Time)
	if elapsed >= getExperimentTimeout(r.options.ExperimentTimeout) {
		logger.Info("Experiment timed out, rolling back", "elapsed", elapsed.String())
		return r.restorePreviousSpec(ctx, instance, newStatus, revisions, ExperimentTerminationReasonTimedOut)
	}
	return nil
}

// restorePreviousSpec restores the DDA spec from the ControllerRevision named
// by newStatus.Experiment.RollbackTargetRevision — the pre-experiment baseline
// captured by the Fleet daemon at start time. On success, sets the terminal
// experiment phase to terminated with the given reason.
//
// When the checkpoint is missing (upgrade case, or a user stripped the field)
// or the named revision no longer exists (external delete evaded the GC pin),
// the experiment is aborted with a distinct reason so the daemon reports
// ERROR to Remote Config rather than silently rolling back to a heuristic
// target. This matches the plan's "no fallback to revision ordering" rule.
func (r *Reconciler) restorePreviousSpec(
	ctx context.Context,
	instance *v2alpha1.DatadogAgent,
	newStatus *v2alpha1.DatadogAgentStatus,
	revisions []appsv1.ControllerRevision,
	terminationReason string,
) error {
	logger := ctrl.LoggerFrom(ctx)
	rollbackTarget := ""
	if newStatus.Experiment != nil {
		rollbackTarget = newStatus.Experiment.RollbackTargetRevision
	}
	if rollbackTarget == "" {
		logger.Info("Aborting rollback: no rollback checkpoint recorded in status.experiment.rollbackTargetRevision")
		newStatus.Experiment.Phase = v2alpha1.ExperimentPhaseAborted
		newStatus.Experiment.TerminationReason = ExperimentTerminationReasonBaselineNotFound
		return nil
	}

	// Verify the named revision exists before invoking rollback so we can
	// distinguish NotFound (structural baseline loss → abort) from transient
	// API errors (return + retry).
	cr := &appsv1.ControllerRevision{}
	getErr := r.client.Get(ctx, types.NamespacedName{Namespace: instance.Namespace, Name: rollbackTarget}, cr)
	if apierrors.IsNotFound(getErr) {
		logger.Info("Aborting rollback: named baseline ControllerRevision not found (external delete?)",
			"rollbackTarget", rollbackTarget)
		newStatus.Experiment.Phase = v2alpha1.ExperimentPhaseAborted
		newStatus.Experiment.TerminationReason = ExperimentTerminationReasonBaselineNotFound
		return nil
	}
	if getErr != nil {
		return fmt.Errorf("failed to get rollback target %q: %w", rollbackTarget, getErr)
	}

	if err := r.rollback(ctx, instance, rollbackTarget); err != nil {
		return err
	}
	newStatus.Experiment.Phase = v2alpha1.ExperimentPhaseTerminated
	newStatus.Experiment.TerminationReason = terminationReason
	return nil
}

// rollback restores the DDA spec from the named ControllerRevision.
// After a successful spec Update, it syncs the new ResourceVersion back to
// instance so the caller's subsequent status update won't 409.
func (r *Reconciler) rollback(
	ctx context.Context,
	instance *v2alpha1.DatadogAgent,
	rollbackTarget string,
) error {
	if rollbackTarget == "" {
		ctrl.LoggerFrom(ctx).Info("No previous revision to roll back to, skipping spec restore")
		return nil
	}

	nsn := types.NamespacedName{Namespace: instance.Namespace, Name: instance.Name}

	cr := &appsv1.ControllerRevision{}
	if err := r.client.Get(ctx, types.NamespacedName{Namespace: instance.Namespace, Name: rollbackTarget}, cr); err != nil {
		return fmt.Errorf("failed to get previous ControllerRevision %s: %w", rollbackTarget, err)
	}

	var snapshot revisionSnapshot
	if err := json.Unmarshal(cr.Data.Raw, &snapshot); err != nil {
		return fmt.Errorf("failed to decode ControllerRevision data: %w", err)
	}

	// Re-fetch for the latest ResourceVersion and to check whether the spec is
	// rolled back already. If it is, skip the update.
	current := &v2alpha1.DatadogAgent{}
	if err := r.client.Get(ctx, nsn, current); err != nil {
		return fmt.Errorf("failed to get current DDA for rollback: %w", err)
	}
	currentSnap, err := buildRevisionSnapshot(current.Spec, current.GetAnnotations())
	if err != nil {
		return fmt.Errorf("failed to marshal current snapshot for comparison: %w", err)
	}
	if bytes.Equal(currentSnap, cr.Data.Raw) {
		ctrl.LoggerFrom(ctx).Info("Rollback spec already matches target, skipping update", "rollbackTarget", rollbackTarget)
		// No update happened, but still sync the re-fetched ResourceVersion so
		// the caller's status update doesn't 409 against a concurrent write.
		instance.ResourceVersion = current.ResourceVersion
		return nil
	}

	// Merge snapshot annotations (Datadog-only keys) on top of current annotations
	// so that non-Datadog annotations (user metadata, tooling labels, etc.) are preserved.
	merged := maps.Clone(current.Annotations)
	if merged == nil {
		merged = make(map[string]string, len(snapshot.Annotations))
	}
	maps.Copy(merged, snapshot.Annotations)

	toUpdate := &v2alpha1.DatadogAgent{
		ObjectMeta: current.ObjectMeta,
		Spec:       snapshot.Spec,
	}
	toUpdate.Annotations = merged
	if err := r.client.Update(ctx, toUpdate); err != nil {
		return err
	}
	// Sync the new ResourceVersion back so the caller's status update
	// uses the correct RV and doesn't 409.
	instance.ResourceVersion = toUpdate.ResourceVersion
	return nil
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

// findMostRecentMatchingRevision returns the revision with the highest Revision number
// whose snapshot content matches the current instance spec and annotations, or nil if
// none match. This serves two purposes:
//
//   - First reconcile after experiment start: the revision for the new spec has not been
//     created yet, so no revision matches → nil → timeout check is skipped, preventing a
//     spurious immediate timeout from an old pre-experiment revision's timestamp.
//
//   - Post-rollback reconcile: the spec has been restored to the pre-experiment value.
//     The matching revision is the pre-experiment one (old timestamp), so elapsed is
//     large, timeout fires, and the idempotent rollback path sets phase=terminated cleanly
//     without a spec-update conflict (ResourceVersion unchanged → status write succeeds).
//
// instance.Spec must be the raw, user-submitted spec, not the in-memory
// defaulted copy — pass rawInstance, not instance. Stored revisions are raw,
// so a defaulted spec never matches any of them.
func findMostRecentMatchingRevision(revisions []appsv1.ControllerRevision, instance *v2alpha1.DatadogAgent) *appsv1.ControllerRevision {
	snapBytes, err := buildRevisionSnapshot(instance.Spec, instance.GetAnnotations())
	if err != nil {
		return nil
	}
	var result *appsv1.ControllerRevision
	for i := range revisions {
		rev := &revisions[i]
		if bytes.Equal(rev.Data.Raw, snapBytes) {
			if result == nil || rev.Revision > result.Revision {
				result = rev
			}
		}
	}
	return result
}

// highestRevision returns the revision with the largest Revision number.
func highestRevision(revisions []appsv1.ControllerRevision) *appsv1.ControllerRevision {
	var result *appsv1.ControllerRevision
	for i := range revisions {
		if result == nil || revisions[i].Revision > result.Revision {
			result = &revisions[i]
		}
	}
	return result
}

// revisionExperimentState returns the recorded experiment outcome on a
// ControllerRevision, or "" if none is set.
func revisionExperimentState(rev *appsv1.ControllerRevision) experimentRevisionState {
	if rev == nil {
		return ""
	}
	return experimentRevisionState(rev.Annotations[annotationExperimentState])
}

// markRevisionState records `state` on the ControllerRevision.
// Best-effort: if the patch fails, the timeout fallback still applies.
func (r *Reconciler) markRevisionState(ctx context.Context, rev *appsv1.ControllerRevision, state experimentRevisionState) {
	if revisionExperimentState(rev) == state {
		return
	}
	logger := ctrl.LoggerFrom(ctx).WithValues(
		"object.kind", "ControllerRevision",
		"object.namespace", rev.Namespace,
		"object.name", rev.Name,
	)
	patch := fmt.Appendf(nil, `{"metadata":{"annotations":{%q:%q}}}`, annotationExperimentState, string(state))
	if err := r.client.Patch(ctx, rev, client.RawPatch(types.MergePatchType, patch)); err != nil {
		logger.Error(err, "Failed to mark experiment revision state", "state", state)
		return
	}
	logger.Info("Marked experiment revision state", "state", state)
}

func getExperimentTimeout(timeout time.Duration) time.Duration {
	if timeout == 0 {
		return ExperimentDefaultTimeout
	}
	return timeout
}
