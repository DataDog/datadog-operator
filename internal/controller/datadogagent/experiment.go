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
)

// annotationExperimentRollback marks a ControllerRevision whose experiment was
// rejected (rollback, timeout, or abort). The annotation tells handleRollback
// to skip the timeout check (the CreationTimestamp is stale from the previous
// experiment) and tells ensureRevision to delete+recreate the revision so it
// gets a fresh timestamp when the same spec is re-applied.
const annotationExperimentRollback = "operator.datadoghq.com/experiment-rollback"

// annotationExperimentPromoted marks a ControllerRevision whose experiment was
// promoted. The annotation tells handleRollback to skip the timeout check so
// the stale CreationTimestamp doesn't cause a false timeout when a subsequent
// experiment starts before its own revision is created.
const annotationExperimentPromoted = "operator.datadoghq.com/experiment-promoted"

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
	pendingClearID, err := r.processExperimentSignal(ctx, instance, newStatus, revList)
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
			r.annotateRevision(ctx, rev, annotationExperimentPromoted)
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
		acted, err = r.processStartSignal(ctx, annotationID, currentPhase, currentID, newStatus)

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
func (r *Reconciler) processStartSignal(
	ctx context.Context,
	annotationID string,
	currentPhase v2alpha1.ExperimentPhase,
	currentID string,
	newStatus *v2alpha1.DatadogAgentStatus,
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

	logger.Info("Processing start signal")
	newStatus.Experiment = &v2alpha1.ExperimentStatus{
		Phase: v2alpha1.ExperimentPhaseRunning,
		ID:    annotationID,
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
		if len(revisions) >= 2 && findMostRecentMatchingRevision(revisions, instance) == nil {
			logger.Info("Aborting experiment instead of rolling back: spec was manually changed")
			newStatus.Experiment.Phase = v2alpha1.ExperimentPhaseAborted
			if rev := highestRevision(revisions); rev != nil {
				r.annotateRevision(ctx, rev, annotationExperimentRollback)
			}
			return true, nil
		}

		logger.Info("Processing rollback signal")
		return true, r.restorePreviousSpec(ctx, instance, newStatus, revisions, ExperimentTerminationReasonStopped)
	}

	// Transition 6: phase=nil but rollback annotation present.
	// Recovery path for C2 (spec patched but phase never written).
	if currentPhase == "" {
		// Check if current spec matches a non-baseline ControllerRevision.
		if len(revisions) >= 2 {
			matchingRev := findMostRecentMatchingRevision(revisions, instance)
			if matchingRev != nil && matchingRev.Revision == highestRevision(revisions).Revision {
				// Spec matches the highest revision (likely the experiment spec).
				// Restore to baseline.
				logger.Info("Transition 6 recovery: rollback signal at nil phase, spec matches experiment revision — restoring baseline")
				newStatus.Experiment = &v2alpha1.ExperimentStatus{
					ID: annotationID,
				}
				return true, r.restorePreviousSpec(ctx, instance, newStatus, revisions, ExperimentTerminationReasonStopped)
			}
		}
		logger.Info("Rollback signal at nil phase: nothing to roll back, clearing annotation")
		return true, nil
	}

	return true, nil
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

	// Verify spec still matches the experiment revision. If user manually
	// changed the spec, abort instead of promoting.
	if len(revisions) >= 2 && findMostRecentMatchingRevision(revisions, instance) == nil {
		logger.Info("Aborting experiment instead of promoting: spec was manually changed")
		newStatus.Experiment.Phase = v2alpha1.ExperimentPhaseAborted
		if rev := highestRevision(revisions); rev != nil {
			r.annotateRevision(ctx, rev, annotationExperimentRollback)
		}
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

// abortExperiment marks the experiment as aborted in newStatus if a manual spec
// change is detected (current spec doesn't match any known ControllerRevision).
// It is a no-op if processExperimentSignal or handleRollback has already set a
// terminal phase, preventing spurious abort logs and phase overwrites.
func (r *Reconciler) abortExperiment(
	ctx context.Context,
	instance *v2alpha1.DatadogAgent,
	experiment *v2alpha1.ExperimentStatus,
	newStatus *v2alpha1.DatadogAgentStatus,
	revisions []appsv1.ControllerRevision,
) {
	if experiment.Phase != v2alpha1.ExperimentPhaseRunning {
		return
	}
	if newStatus.Experiment.Phase != v2alpha1.ExperimentPhaseRunning {
		// handleRollback already determined a terminal phase (e.g. timeout); don't overwrite or log.
		return
	}
	// On the first reconcile after experiment start, the new revision hasn't
	// been created yet (manageExperiment runs before manageRevision). With
	// only one revision (the pre-experiment baseline), the current spec won't
	// match it — but that's expected, not a manual change. Skip the check
	// when fewer than 2 revisions exist.
	if len(revisions) < 2 {
		return
	}
	if findMostRecentMatchingRevision(revisions, instance) != nil {
		// Spec matches a known revision — no manual change detected.
		// Edge case: if the user manually reverts to the pre-experiment spec, it
		// matches the baseline revision, so abort does not fire. The experiment
		// still terminates via timeout (the baseline revision's old timestamp
		// exceeds the timeout threshold), and the rollback is a no-op because
		// the spec already matches the target. The phase will read "terminated"
		// rather than "aborted", but the end state is correct.
		return
	}
	ctrl.LoggerFrom(ctx).Info("Aborting experiment due to manual spec change")
	newStatus.Experiment.Phase = v2alpha1.ExperimentPhaseAborted
	// Mark the experiment revision (highest-numbered) so its stale timestamp
	// doesn't cause an immediate timeout if the same spec is re-applied.
	if rev := highestRevision(revisions); rev != nil {
		r.annotateRevision(ctx, rev, annotationExperimentRollback)
	}
}

// handleRollback checks if the experiment needs timeout-based rollback.
// Rollback signals are handled by processExperimentSignal (annotation-based).
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

	logger := ctrl.LoggerFrom(ctx)

	rev := findMostRecentMatchingRevision(revisions, instance)
	if rev == nil && len(revisions) >= 2 {
		// Spec was manually changed — no revision matches the current spec.
		// Don't fall back to highest revision for timeout: let abortExperiment
		// handle this case (it correctly detects the manual change).
		// Only check annotated fallback revisions to avoid false timeouts from
		// stale timestamps of prior experiments.
		rev = highestRevision(revisions)
		if rev != nil && (rev.Annotations[annotationExperimentRollback] == "true" || rev.Annotations[annotationExperimentPromoted] == "true") {
			return nil
		}
		// Spec doesn't match any revision and highest rev is unannotated:
		// this is a manual spec change. Let abortExperiment handle it.
		if rev != nil {
			return nil
		}
	}
	if rev != nil {
		elapsed := now.Sub(rev.CreationTimestamp.Time)
		if elapsed >= getExperimentTimeout(r.options.ExperimentTimeout) {
			logger.Info("Experiment timed out, rolling back", "elapsed", elapsed.String())
			return r.restorePreviousSpec(ctx, instance, newStatus, revisions, ExperimentTerminationReasonTimedOut)
		}
	}

	return nil
}

// restorePreviousSpec restores the DDA spec from the previous ControllerRevision
// and, on success, sets the terminal experiment phase to terminated with the given
// reason. It also marks the experiment revision (the highest-numbered, non-rollback-target
// revision) with the rollback annotation so its stale CreationTimestamp doesn't cause
// an immediate timeout if the same spec is re-applied later.
func (r *Reconciler) restorePreviousSpec(
	ctx context.Context,
	instance *v2alpha1.DatadogAgent,
	newStatus *v2alpha1.DatadogAgentStatus,
	revisions []appsv1.ControllerRevision,
	terminationReason string,
) error {
	rollbackTarget := findRollbackTarget(revisions)
	if err := r.rollback(ctx, instance, rollbackTarget); err != nil {
		return err
	}
	newStatus.Experiment.Phase = v2alpha1.ExperimentPhaseTerminated
	newStatus.Experiment.TerminationReason = terminationReason
	// Mark the experiment revision (highest-numbered) so its stale timestamp
	// doesn't cause an immediate timeout if the same spec is re-applied.
	// Only annotate the highest revision rather than all non-rollback-target
	// revisions: if GC failed on a prior reconcile there may be 3+ revisions,
	// and annotating old baselines would cause needless delete+recreate in
	// ensureRevision if those specs are ever re-applied.
	if rev := highestRevision(revisions); rev != nil && rev.Name != rollbackTarget {
		r.annotateRevision(ctx, rev, annotationExperimentRollback)
	}
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
	currentSnap, err := json.Marshal(revisionSnapshot{Spec: current.Spec, Annotations: datadogAnnotations(current.GetAnnotations())})
	if err != nil {
		return fmt.Errorf("failed to marshal current snapshot for comparison: %w", err)
	}
	if bytes.Equal(currentSnap, cr.Data.Raw) {
		ctrl.LoggerFrom(ctx).Info("Rollback spec already matches target, skipping update", "rollbackTarget", rollbackTarget)
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
func findMostRecentMatchingRevision(revisions []appsv1.ControllerRevision, instance *v2alpha1.DatadogAgent) *appsv1.ControllerRevision {
	snap := revisionSnapshot{Spec: instance.Spec, Annotations: datadogAnnotations(instance.GetAnnotations())}
	snapBytes, err := json.Marshal(snap)
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

// annotateRevision sets the given annotation on a ControllerRevision.
// This is best-effort: if the patch fails, the stale timestamp remains
// but is no worse than the existing behavior.
func (r *Reconciler) annotateRevision(ctx context.Context, rev *appsv1.ControllerRevision, annotation string) {
	if rev.Annotations[annotation] == "true" {
		return // already annotated
	}
	logger := ctrl.LoggerFrom(ctx).WithValues(
		"object.kind", "ControllerRevision",
		"object.namespace", rev.Namespace,
		"object.name", rev.Name,
	)
	patch := []byte(`{"metadata":{"annotations":{"` + annotation + `":"true"}}}`)
	if err := r.client.Patch(ctx, rev, client.RawPatch(types.MergePatchType, patch)); err != nil {
		logger.Error(err, "Failed to annotate experiment revision", "annotation", annotation)
		return
	}
	logger.Info("Annotated experiment revision", "annotation", annotation)
}

func getExperimentTimeout(timeout time.Duration) time.Duration {
	if timeout == 0 {
		return ExperimentDefaultTimeout
	}
	return timeout
}
