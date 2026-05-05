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
	"maps"

	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"
	toolscache "k8s.io/client-go/tools/cache"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

// ddaStatusSnapshot is the immutable subset of DDA state the async worker
// needs from informer events. We copy only these fields because informer
// objects must not be shared across goroutines, but the worker does not need a
// full DatadogAgent deep copy.
type ddaStatusSnapshot struct {
	nsn         types.NamespacedName
	annotations map[string]string
	experiment  *v2alpha1.ExperimentStatus
}

type pendingOperation struct {
	// intent is the normalized Fleet-side operation we are waiting to finish.
	// It is intentionally distinct from the controller-facing DDA signal:
	// Fleet "stop" is implemented by controller signal "rollback".
	intent pendingIntent
	// taskID and packageName identify which RC PackageState.Task and config
	// versions should be updated when this intent finishes.
	taskID      string
	packageName string
	// nsn identifies the DDA object being watched for reconciler-owned status
	// transitions.
	nsn types.NamespacedName
	// experimentID is the stable experiment identity (`params.version`) used to
	// match status.experiment.id across retries and restarts.
	experimentID string
	// resultVersion is the config version to write into RC package state on
	// success when it differs from the stable experiment identity. For promote,
	// this is the current ExperimentConfigVersion, and it is also persisted in
	// the durable pending annotations for restart recovery.
	resultVersion string
	// complete exists only for the current process and current Fleet delivery.
	// It is intentionally in-memory only; restart recovery uses the durable DDA
	// pending annotations plus RC Task.State/config-version updates instead.
	complete func(error)
}

type pendingIntent string

const (
	pendingIntentStart   pendingIntent = "start"
	pendingIntentStop    pendingIntent = "stop"
	pendingIntentPromote pendingIntent = "promote"
)

type supersededOperationError struct {
	msg string
}

func (e *supersededOperationError) Error() string {
	return e.msg
}

func (op pendingOperation) scopeKey() string {
	return operationScopeKey(op.packageName, op.nsn)
}

func (op pendingOperation) matches(other pendingOperation) bool {
	return op.intent == other.intent &&
		op.taskID == other.taskID &&
		op.packageName == other.packageName &&
		op.nsn == other.nsn &&
		op.experimentID == other.experimentID &&
		op.resultVersion == other.resultVersion
}

type operationTracker struct {
	daemon *Daemon
	logger logr.Logger
	// pending is the live-process cache of currently tracked operations. It is
	// rebuilt opportunistically from daemon-owned DDA pending annotations on
	// status updates after restart; it is not itself a durable store.
	//
	// Durability boundary:
	//   - durable across restart: DDA pending annotations + RC Task.State/config state
	//   - process-local only: this map and the live completion callback
	//
	// So a restart can recover the pending intent and continue driving RC state,
	// but it cannot resurrect the old in-process callback from the previous
	// process.
	pending map[string]pendingOperation
}

func newOperationTracker(d *Daemon, logger logr.Logger) *operationTracker {
	return &operationTracker{
		daemon:  d,
		logger:  logger,
		pending: make(map[string]pendingOperation),
	}
}

// run is the single async worker loop for Fleet experiment intents.
//
// It merges two event streams:
//   - task-driven input from Fleet (`pendingOps`)
//   - status-driven input from Kubernetes (`statusUpdates`)
//
// The task stream tells us that new intent exists and which RC task/package it
// belongs to. The status stream tells us when the reconciler has actually
// reached a matching experiment state. Durable DDA pending annotations bridge
// those two streams across process restarts.
func (t *operationTracker) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		// pendingOps is task-driven input from Fleet. This is not just an early
		// wake-up optimization: the worker needs the accepted task identity,
		// package, and live completion callback immediately so it can track the
		// op, apply supersession rules, and later update the correct RC Task.State.
		case task := <-t.daemon.pendingOps:
			t.enqueue(ctx, task)
		case dda := <-t.daemon.statusUpdates:
			t.onStatusUpdate(ctx, dda)
		}
	}
}

// enqueue registers a newly accepted pending intent with the live worker.
//
// It first checks the current DDA state in case the reconciler already reached
// the expected phase before the worker saw this task. Otherwise it stores the
// intent in the live cache. If another intent is already pending for the same
// DDA/package scope, the older one is finished as superseded.
func (t *operationTracker) enqueue(ctx context.Context, task pendingOperation) {
	current := &v2alpha1.DatadogAgent{}
	err := t.daemon.client.Get(ctx, task.nsn, current)
	if err == nil {
		if done, resultErr := t.daemon.evaluatePendingTask(newDDAStatusSnapshot(current), task); done {
			// Fast path: by the time the worker sees the intent, the reconciler may
			// already have written the expected status.
			t.daemon.finishPendingOperation(ctx, task, resultErr)
			return
		}
	}
	scopeKey := task.scopeKey()
	if previous, ok := t.pending[scopeKey]; ok {
		if previous.matches(task) {
			// Duplicate delivery for the same current intent. Preserve the current
			// live callback from this process so a redelivery after restart can still
			// drive RC completion, while keeping the rest of the recovered record.
			if task.complete != nil {
				previous.complete = task.complete
			}
			t.pending[scopeKey] = previous
			return
		}
		supersededErr := &supersededOperationError{
			msg: fmt.Sprintf("pending operation superseded by newer %s task %q", task.intent, task.taskID),
		}
		t.logger.Info("Superseding pending operation", "scopeKey", scopeKey, "previousTaskID", previous.taskID, "newTaskID", task.taskID, "newIntent", task.intent)
		t.daemon.finishPendingOperation(ctx, previous, supersededErr)
	}
	t.pending[scopeKey] = task
}

// onStatusUpdate reacts to reconciler-owned DDA status changes.
//
// This is the normal completion path for async intents, and also the restart
// recovery path: if the process lost its in-memory cache, the durable pending
// annotations on the DDA are used to rebuild the current pending intent before
// checking whether the new status satisfies it.
func (t *operationTracker) onStatusUpdate(ctx context.Context, snapshot ddaStatusSnapshot) {
	t.daemon.reconcileTimedOutExperiment(ctx, snapshot)
	// Crash recovery: if this process restarted and lost its in-memory pending
	// map, rebuild the current op from daemon-owned DDA annotations. The
	// reconciler-owned status.experiment still determines completion.
	if recovered, ok := pendingOperationFromSnapshot(snapshot); ok {
		if state, exists := t.pending[recovered.scopeKey()]; !exists || !state.matches(recovered) {
			t.enqueue(ctx, recovered)
		}
	}
	for scopeKey, op := range t.pending {
		done, resultErr := t.daemon.evaluatePendingTask(snapshot, op)
		if !done {
			continue
		}
		delete(t.pending, scopeKey)
		t.daemon.finishPendingOperation(ctx, op, resultErr)
	}
}

func (d *Daemon) runPendingOperationWorker(ctx context.Context) {
	newOperationTracker(d, ctrl.LoggerFrom(ctx).WithName("fleet-operation-worker")).run(ctx)
}

// installDDAStatusForwarder wires the DDA informer into the async worker and
// seeds the worker with the current DDA list so pending intents can be
// recovered immediately after restart, without waiting for a new informer event.
func (d *Daemon) installDDAStatusForwarder(ctx context.Context) error {
	ddaInformer, err := d.cache.GetInformer(ctx, &v2alpha1.DatadogAgent{})
	if err != nil {
		return fmt.Errorf("failed to get DatadogAgent informer: %w", err)
	}
	ddaInformer.AddEventHandler(toolscache.ResourceEventHandlerFuncs{
		AddFunc:    d.forwardDDAStatusUpdate,
		UpdateFunc: func(_, newObj any) { d.forwardDDAStatusUpdate(newObj) },
	})
	go d.runPendingOperationWorker(ctx)
	ddaList := &v2alpha1.DatadogAgentList{}
	if err := d.client.List(ctx, ddaList); err != nil {
		return fmt.Errorf("failed to list DatadogAgents for pending operation recovery: %w", err)
	}
	for i := range ddaList.Items {
		d.forwardDDAStatusUpdate(&ddaList.Items[i])
	}
	return nil
}

// forwardDDAStatusUpdate converts informer objects into a small immutable
// snapshot before handing them to the worker. This avoids sharing informer
// objects across goroutines and copies only the fields the worker actually
// reads: object identity, annotations, and status.experiment.
func (d *Daemon) forwardDDAStatusUpdate(obj any) {
	if dda, ok := obj.(*v2alpha1.DatadogAgent); ok {
		d.statusUpdates <- newDDAStatusSnapshot(dda)
	}
}

func newDDAStatusSnapshot(dda *v2alpha1.DatadogAgent) ddaStatusSnapshot {
	snapshot := ddaStatusSnapshot{
		nsn: types.NamespacedName{
			Namespace: dda.Namespace,
			Name:      dda.Name,
		},
	}
	if len(dda.Annotations) != 0 {
		snapshot.annotations = maps.Clone(dda.Annotations)
	}
	if dda.Status.Experiment != nil {
		snapshot.experiment = dda.Status.Experiment.DeepCopy()
	}
	return snapshot
}

// evaluatePendingTask answers whether the given DDA state satisfies a pending
// intent for this exact experiment identity.
//
// The worker intentionally keys on status.experiment.id as well as phase so
// stale status from an older experiment cannot incorrectly satisfy or fail the
// current one.
func (d *Daemon) evaluatePendingTask(snapshot ddaStatusSnapshot, task pendingOperation) (bool, error) {
	if snapshot.nsn != task.nsn {
		return false, nil
	}
	switch task.intent {
	case pendingIntentStart:
		// Start succeeds once the reconciler acknowledges the experiment as
		// running for this exact experiment identity.
		return acceptPhase(task.experimentID, v2alpha1.ExperimentPhaseRunning)(snapshot.experiment)
	case pendingIntentStop:
		// Stop is satisfied by any terminal phase for this experiment. The
		// controller may report terminated, promoted, or aborted depending on the
		// underlying race/outcome.
		return acceptPhase(task.experimentID, v2alpha1.ExperimentPhaseTerminated, v2alpha1.ExperimentPhasePromoted, v2alpha1.ExperimentPhaseAborted)(snapshot.experiment)
	case pendingIntentPromote:
		return acceptPhase(task.experimentID, v2alpha1.ExperimentPhasePromoted)(snapshot.experiment)
	default:
		return false, fmt.Errorf("unknown pending intent %q", task.intent)
	}
}

// finishPendingOperation applies the final RC-side effects for a completed
// intent and then clears the durable pending record if it still matches.
//
// Crash-safety note:
//   - RC Task.State/config-version updates are the authoritative backend signal
//   - the durable DDA pending annotations are only a recovery breadcrumb
//   - the live callback is best-effort and cannot be recovered after restart
//
// Because of that, finalization updates RC state first and clears annotations
// afterward. A crash in between may leave stale pending annotations, but that
// is recoverable and preferable to losing the durable record before RC state is
// updated.
func (d *Daemon) finishPendingOperation(ctx context.Context, task pendingOperation, resultErr error) {
	d.taskMu.Lock()
	defer d.taskMu.Unlock()

	// Finalization always happens in the same order:
	// 1. mutate RC state
	// 2. notify the live callback, if any
	// 3. clear the durable pending record, but only if it still matches this task
	// This keeps restart recovery safe if the process dies mid-finish.
	if resultErr != nil {
		d.applyPendingOperationFailure(task, resultErr)
	} else {
		d.applyPendingOperationSuccess(task)
	}
	d.completePendingOperationCallback(task, resultErr)
	d.clearPendingOperationRecord(ctx, task)
}

// applyPendingOperationFailure maps a failed/superseded intent into the RC task
// state model. Superseded intents are INVALID_STATE rather than generic ERROR
// because a newer intent intentionally made the older one obsolete.
func (d *Daemon) applyPendingOperationFailure(task pendingOperation, resultErr error) {
	taskState := pbgo.TaskState_ERROR
	var supersededErr *supersededOperationError
	if errors.As(resultErr, &supersededErr) {
		// Superseded means a newer intent made this task obsolete; it is not a
		// generic execution failure.
		taskState = pbgo.TaskState_INVALID_STATE
	}
	d.setTaskState(task.packageName, task.taskID, taskState, resultErr)
}

// applyPendingOperationSuccess updates RC config-version state once the
// reconciler has confirmed success for the intent.
func (d *Daemon) applyPendingOperationSuccess(task pendingOperation) {
	switch task.intent {
	case pendingIntentStart:
		stable, _ := d.getPackageConfigVersions(task.packageName)
		d.setPackageConfigVersions(task.packageName, stable, task.experimentID)
	case pendingIntentStop:
		stable, _ := d.getPackageConfigVersions(task.packageName)
		d.setPackageConfigVersions(task.packageName, stable, "")
	case pendingIntentPromote:
		d.setPackageConfigVersions(task.packageName, task.resultVersion, "")
	}
	d.setTaskState(task.packageName, task.taskID, pbgo.TaskState_DONE, nil)
}

// completePendingOperationCallback notifies the live Fleet delivery path, if
// this process still has one. After restart there is no callback to replay, so
// crash recovery relies on RC Task.State rather than this hook.
func (d *Daemon) completePendingOperationCallback(task pendingOperation, resultErr error) {
	if task.complete != nil {
		task.complete(resultErr)
	}
}

// clearPendingOperationRecord removes the daemon-owned durable pending
// annotations once an intent is fully resolved.
func (d *Daemon) clearPendingOperationRecord(ctx context.Context, task pendingOperation) {
	if err := d.clearPendingOperationAnnotations(ctx, task); err != nil {
		ctrl.LoggerFrom(ctx).Error(err, "Failed to clear pending operation annotations", "taskID", task.taskID, "package", task.packageName, "namespace", task.nsn.Namespace, "name", task.nsn.Name)
	}
}

// reconcileTimedOutExperiment passively repairs RC experiment config state when
// the controller later times out and terminates a running experiment on its
// own, without a new Fleet task driving that transition.
func (d *Daemon) reconcileTimedOutExperiment(ctx context.Context, snapshot ddaStatusSnapshot) {
	if d.rcClient == nil || snapshot.experiment == nil {
		return
	}
	if snapshot.experiment.Phase != v2alpha1.ExperimentPhaseTerminated ||
		snapshot.experiment.TerminationReason != "timed_out" {
		return
	}

	logger := ctrl.LoggerFrom(ctx).WithValues("namespace", snapshot.nsn.Namespace, "name", snapshot.nsn.Name, "experimentID", snapshot.experiment.ID)

	d.taskMu.Lock()
	defer d.taskMu.Unlock()

	changed := false
	for _, pkg := range d.rcClient.GetInstallerState() {
		if pkg.GetExperimentConfigVersion() != snapshot.experiment.ID {
			continue
		}
		stable, _ := d.getPackageConfigVersions(pkg.GetPackage())
		d.setPackageConfigVersions(pkg.GetPackage(), stable, "")
		changed = true
	}
	if changed {
		logger.Info("Cleared timed out experiment config version from RC state")
	}
}

// pendingOperationFromSnapshot reconstructs the durable pending intent record
// from daemon-owned DDA annotations after restart.
//
// Only the durable fields are recovered here. Transient process-local data such
// as the live callback are not recoverable.
func pendingOperationFromSnapshot(snapshot ddaStatusSnapshot) (pendingOperation, bool) {
	if len(snapshot.annotations) == 0 {
		return pendingOperation{}, false
	}
	taskID := snapshot.annotations[v2alpha1.AnnotationPendingTaskID]
	action := snapshot.annotations[v2alpha1.AnnotationPendingAction]
	experimentID := snapshot.annotations[v2alpha1.AnnotationPendingExperimentID]
	packageName := snapshot.annotations[v2alpha1.AnnotationPendingPackage]
	if taskID == "" || action == "" || experimentID == "" || packageName == "" {
		return pendingOperation{}, false
	}

	intent := pendingIntent(action)
	switch intent {
	case pendingIntentStart, pendingIntentStop, pendingIntentPromote:
	default:
		return pendingOperation{}, false
	}

	// Only the durable fields are reconstructed here. Transient process-local
	// data such as the live completion callback is not recoverable from
	// annotations; backend correctness relies on RC Task.State, not on replaying
	// the old callback.
	return pendingOperation{
		intent:        intent,
		taskID:        taskID,
		packageName:   packageName,
		nsn:           snapshot.nsn,
		experimentID:  experimentID,
		resultVersion: snapshot.annotations[v2alpha1.AnnotationPendingResultVersion],
	}, true
}

func (d *Daemon) clearPendingOperationAnnotations(ctx context.Context, task pendingOperation) error {
	current := &v2alpha1.DatadogAgent{}
	if err := d.client.Get(ctx, task.nsn, current); err != nil {
		return err
	}
	// Only clear when the durable record still matches this task. This prevents
	// an old finisher from erasing a newer pending op that intentionally
	// overwrote the annotations after supersession.
	if !annotationsMatchPendingOperation(current.Annotations, task) {
		return nil
	}
	patch, err := buildClearPendingOperationPatch()
	if err != nil {
		return err
	}
	dda := &v2alpha1.DatadogAgent{}
	dda.Name = task.nsn.Name
	dda.Namespace = task.nsn.Namespace
	return retryWithBackoff(ctx, func() error {
		return d.client.Patch(ctx, dda, client.RawPatch(types.MergePatchType, patch), client.FieldOwner("fleet-daemon"))
	})
}

func annotationsMatchPendingOperation(annotations map[string]string, task pendingOperation) bool {
	if len(annotations) == 0 {
		return false
	}
	// Match on the full durable record so an older finisher cannot erase a newer
	// pending intent after supersession.
	return annotations[v2alpha1.AnnotationPendingTaskID] == task.taskID &&
		annotations[v2alpha1.AnnotationPendingAction] == string(task.intent) &&
		annotations[v2alpha1.AnnotationPendingExperimentID] == task.experimentID &&
		annotations[v2alpha1.AnnotationPendingPackage] == task.packageName &&
		annotations[v2alpha1.AnnotationPendingResultVersion] == task.resultVersion
}

func buildClearPendingOperationPatch() ([]byte, error) {
	return json.Marshal(map[string]any{
		"metadata": map[string]any{
			"annotations": map[string]any{
				v2alpha1.AnnotationPendingTaskID:        nil,
				v2alpha1.AnnotationPendingAction:        nil,
				v2alpha1.AnnotationPendingExperimentID:  nil,
				v2alpha1.AnnotationPendingPackage:       nil,
				v2alpha1.AnnotationPendingResultVersion: nil,
			},
		},
	})
}
