// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fleet

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"

	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"k8s.io/apimachinery/pkg/types"
	toolscache "k8s.io/client-go/tools/cache"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

// ddaStatusSnapshot is the small DDA state the worker needs from informer
// events. We copy these fields because informer objects must not be shared
// across goroutines.
type ddaStatusSnapshot struct {
	nsn         types.NamespacedName
	annotations map[string]string
	experiment  *v2alpha1.ExperimentStatus
}

type pendingOperation struct {
	// intent says which Fleet task this is. Stop maps to the DDA rollback signal.
	intent pendingIntent
	// taskID and packageName say which RC package task to update.
	taskID      string
	packageName string
	// nsn is the DatadogAgent to read and patch.
	nsn types.NamespacedName
	// experimentID must match status.experiment.id before this task can finish.
	experimentID string
	// resultVersion is only used by promote. It becomes stable_config on success.
	resultVersion string
}

type pendingIntent string

const (
	pendingIntentStart   pendingIntent = "start"
	pendingIntentStop    pendingIntent = "stop"
	pendingIntentPromote pendingIntent = "promote"
)

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
}

func newOperationTracker(d *Daemon) *operationTracker {
	return &operationTracker{
		daemon: d,
	}
}

// run waits for DatadogAgent status updates and checks whether a pending task
// is still running or finished.
func (t *operationTracker) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case dda := <-t.daemon.statusUpdates:
			t.onStatusUpdate(ctx, dda)
		}
	}
}

// onStatusUpdate checks the pending task recorded on the DDA annotations
// against the latest DDA status.
//
// The worker does not keep its own pending-task map. If stop replaces a pending
// start in the annotations, the old start is forgotten here. A later retry of
// that old start is rejected by guardPendingOperationSlot.
func (t *operationTracker) onStatusUpdate(ctx context.Context, snapshot ddaStatusSnapshot) {
	t.daemon.transitionMu.Lock()
	defer t.daemon.transitionMu.Unlock()
	t.daemon.taskMu.Lock()
	managedAgentInstallationActive := t.daemon.managedAgentInstallationActive
	t.daemon.taskMu.Unlock()
	if managedAgentInstallationActive {
		return
	}
	t.daemon.reconcileLocallyTerminatedExperiment(ctx, snapshot)
	op, ok := pendingOperationFromAnnotations(snapshot.nsn, snapshot.annotations)
	if !ok {
		return
	}
	done, resultErr := evaluatePendingTask(snapshot, op)
	if !done {
		t.daemon.taskMu.Lock()
		t.daemon.setTaskState(op.packageName, op.taskID, pbgo.TaskState_RUNNING, nil)
		t.daemon.taskMu.Unlock()
		return
	}

	t.daemon.finishPendingOperation(ctx, op, resultErr)
}

// installDDAStatusForwarder wires the DDA informer into the worker. The informer
// sends Add events for existing DDAs from its initial list, so a restarted
// daemon sees existing pending annotations.
func (d *Daemon) installDDAStatusForwarder(ctx context.Context) error {
	ddaInformer, err := d.cache.GetInformer(ctx, &v2alpha1.DatadogAgent{})
	if err != nil {
		return fmt.Errorf("failed to get DatadogAgent informer: %w", err)
	}
	ddaInformer.AddEventHandler(toolscache.ResourceEventHandlerFuncs{
		AddFunc:    d.forwardDDAStatusUpdate,
		UpdateFunc: func(_, newObj any) { d.forwardDDAStatusUpdate(newObj) },
	})
	return nil
}

// forwardDDAStatusUpdate converts informer objects into a small immutable
// snapshot before handing them to the worker. This avoids sharing informer
// objects across goroutines and copies only the fields the worker actually
// reads: object identity, annotations, and status.experiment.
func (d *Daemon) forwardDDAStatusUpdate(obj any) {
	if dda, ok := obj.(*v2alpha1.DatadogAgent); ok {
		d.statusUpdates <- newDDAStatusSnapshot(dda)
		if client.ObjectKeyFromObject(dda) == managedAgentInstallationTarget {
			d.requestManagedAgentInstallationRetryIfWaiting()
		}
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

// evaluatePendingTask checks whether this DDA status finishes this task.
//
// The experiment ID must match so status from an older experiment is ignored.
func evaluatePendingTask(snapshot ddaStatusSnapshot, task pendingOperation) (bool, error) {
	if snapshot.nsn != task.nsn || snapshot.experiment == nil || snapshot.experiment.ID != task.experimentID {
		return false, nil
	}

	phase := snapshot.experiment.Phase
	switch task.intent {
	case pendingIntentStart:
		if phase == v2alpha1.ExperimentPhaseRunning {
			return true, nil
		}
	case pendingIntentStop:
		return isTerminalPhase(phase), nil
	case pendingIntentPromote:
		if phase == v2alpha1.ExperimentPhasePromoted {
			return true, nil
		}
	default:
		return true, fmt.Errorf("unknown pending intent %q", task.intent)
	}
	if isTerminalPhase(phase) {
		return true, fmt.Errorf("expected %s to finish, got terminal phase %q", task.intent, phase)
	}
	return false, nil
}

// finishPendingOperation writes the final RC state for a task.
//
// RC is updated before DDA annotations are cleared. If the daemon crashes in
// between, the annotations may be cleaned up later, but the task result is not
// lost.
func (d *Daemon) finishPendingOperation(ctx context.Context, task pendingOperation, resultErr error) {
	if resultErr == nil && task.intent == pendingIntentPromote {
		if err := d.persistManagedAgentInstallationStableConfig(ctx, task.nsn, task.experimentID, task.resultVersion); err != nil {
			resultErr = fmt.Errorf("persist promoted managed Agent installation config: %w", err)
		}
	}

	d.taskMu.Lock()

	if resultErr == nil {
		switch task.intent {
		case pendingIntentStart:
			stable, _ := d.getPackageConfigVersions(task.packageName)
			d.setPackageConfigVersions(task.packageName, stable, task.experimentID)
		case pendingIntentStop:
			stable, _ := d.getPackageConfigVersions(task.packageName)
			d.setPackageConfigVersions(task.packageName, stable, "")
		case pendingIntentPromote:
			d.setPackageConfigVersions(task.packageName, task.resultVersion, "")
		default:
			resultErr = fmt.Errorf("unknown pending intent %q", task.intent)
		}
	}
	if resultErr != nil {
		d.setTaskState(task.packageName, task.taskID, pbgo.TaskState_ERROR, resultErr)
	} else {
		d.setTaskState(task.packageName, task.taskID, pbgo.TaskState_DONE, nil)
	}
	d.taskMu.Unlock()

	// Emit outgoing-edge events after the in-memory RC state is committed.
	if resultErr != nil {
		d.emitTaskRejectedEvent(ctx, task.nsn,
			remoteAPIRequest{ID: task.taskID, Method: methodForIntent(task.intent)},
			resultErr.Error())
	} else {
		d.emitTaskCompletedEvent(ctx, task)
	}

	if err := d.clearPendingAnnotationsIfCurrent(ctx, task); err != nil {
		ctrl.LoggerFrom(ctx).Error(err, "Failed to clear pending operation annotations", "taskID", task.taskID, "package", task.packageName, "namespace", task.nsn.Namespace, "name", task.nsn.Name)
	}
	d.requestManagedAgentInstallationRetryIfWaiting()
}

// methodForIntent maps a pendingIntent to its wire Fleet method name,
// used when emitTaskRejectedEvent needs a synthetic remoteAPIRequest
// for the event message label.
func methodForIntent(intent pendingIntent) string {
	switch intent {
	case pendingIntentStart:
		return methodStartDatadogAgentExperiment
	case pendingIntentStop:
		return methodStopDatadogAgentExperiment
	case pendingIntentPromote:
		return methodPromoteDatadogAgentExperiment
	default:
		return string(intent)
	}
}

// localTerminationReason returns the FA-facing error message for an
// experiment that reached a terminal phase locally (not via a Fleet task),
// or "" if the snapshot is not in such a state.
//
// "Local terminal" covers two cases the controller can reach without FA
// driving the transition:
//   - Phase=Terminated, terminationReason="timed_out": experiment exceeded
//     the timeout while running.
//   - Phase=Aborted: a manual spec change was detected while the experiment
//     was running.
//
// Phase=Terminated/stopped and Phase=Promoted are excluded because those
// transitions are driven by Fleet tasks (stop/promote), whose task lifecycle
// is already reported via evaluatePendingTask + finishPendingOperation.
func localTerminationReason(exp *v2alpha1.ExperimentStatus) string {
	if exp == nil {
		return ""
	}
	switch {
	case exp.Phase == v2alpha1.ExperimentPhaseTerminated && exp.TerminationReason == "timed_out":
		return fmt.Sprintf("experiment %s timed out", exp.ID)
	case exp.Phase == v2alpha1.ExperimentPhaseAborted:
		return fmt.Sprintf("experiment %s aborted (manual spec change)", exp.ID)
	default:
		return ""
	}
}

// reconcileLocallyTerminatedExperiment passively repairs RC experiment
// config state when the controller terminates a running experiment on its
// own (timeout or abort), without a Fleet task driving that transition.
//
// When Status.Experiment.StartTaskID is recorded, the daemon also reports
// TaskState_ERROR for the original start task so Fleet Automation receives
// an explicit terminal failure tied to the task it sent. Without this, FA
// only observes experimentConfigVersion going to empty, which is ambiguous
// between "experiment finished cleanly" and "experiment never started" and
// tends to trigger retries of the same experiment ID.
//
// Task-state reporting happens before the experimentConfigVersion clear so
// the two changes ship in the same poll to FA.
func (d *Daemon) reconcileLocallyTerminatedExperiment(ctx context.Context, snapshot ddaStatusSnapshot) {
	if d.rcClient == nil || snapshot.experiment == nil {
		return
	}
	errMsg := localTerminationReason(snapshot.experiment)
	if errMsg == "" {
		return
	}

	logger := ctrl.LoggerFrom(ctx).WithValues(
		"namespace", snapshot.nsn.Namespace,
		"name", snapshot.nsn.Name,
		"experimentID", snapshot.experiment.ID,
		"phase", snapshot.experiment.Phase,
	)

	d.taskMu.Lock()
	defer d.taskMu.Unlock()

	for _, pkg := range d.rcClient.GetInstallerState() {
		if pkg.GetExperimentConfigVersion() != snapshot.experiment.ID {
			continue
		}
		if startTaskID := snapshot.experiment.StartTaskID; startTaskID != "" {
			d.setTaskState(pkg.GetPackage(), startTaskID, pbgo.TaskState_ERROR, fmt.Errorf("%s", errMsg))
			logger.Info("Reported locally-terminated experiment to RC as ERROR on original start task",
				"package", pkg.GetPackage(),
				"startTaskID", startTaskID,
				"reason", errMsg,
			)
		}
		d.setPackageConfigVersions(pkg.GetPackage(), pkg.GetStableConfigVersion(), "")
		logger.Info("Cleared locally-terminated experiment config version from RC state", "package", pkg.GetPackage())
		d.emitLocalTerminationPublishedEvent(ctx, snapshot.nsn, snapshot.experiment.ID, errMsg)
	}
}

// pendingOperationFromAnnotations reads the pending task from DDA annotations.
// Missing fields or an unknown action mean there is no task to track.
func pendingOperationFromAnnotations(nsn types.NamespacedName, annotations map[string]string) (pendingOperation, bool) {
	op := pendingOperation{
		intent:        pendingIntent(annotations[v2alpha1.AnnotationPendingAction]),
		taskID:        annotations[v2alpha1.AnnotationPendingTaskID],
		packageName:   annotations[v2alpha1.AnnotationPendingPackage],
		nsn:           nsn,
		experimentID:  annotations[v2alpha1.AnnotationPendingExperimentID],
		resultVersion: annotations[v2alpha1.AnnotationPendingResultVersion],
	}
	if op.taskID == "" || op.intent == "" || op.experimentID == "" || op.packageName == "" {
		return pendingOperation{}, false
	}

	switch op.intent {
	case pendingIntentStart, pendingIntentStop, pendingIntentPromote:
		return op, true
	default:
		return pendingOperation{}, false
	}
}

func (d *Daemon) clearPendingAnnotationsIfCurrent(ctx context.Context, task pendingOperation) error {
	current := &v2alpha1.DatadogAgent{}
	if err := d.client.Get(ctx, task.nsn, current); err != nil {
		return err
	}

	// Do not clear annotations if another task has replaced this one.
	currentTask, ok := pendingOperationFromAnnotations(task.nsn, current.Annotations)
	if !ok || !currentTask.matches(task) {
		return nil
	}

	patch, err := json.Marshal(map[string]any{
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
	if err != nil {
		return err
	}

	return retryWithBackoff(ctx, func() error {
		return d.client.Patch(ctx, current, client.RawPatch(types.MergePatchType, patch), client.FieldOwner("fleet-daemon"))
	})
}
