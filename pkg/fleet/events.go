// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fleet

import (
	"context"
	"os"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	v2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

// envFleetManagementEventsEnabled gates emission of fleet-daemon-source
// Kubernetes events. Read inline at each emission site so toggling the env
// var on a running operator pod takes effect immediately without restart.
const envFleetManagementEventsEnabled = "DD_FLEET_MANAGEMENT_EVENTS_ENABLED"

// fleetManagementEventsEnabled returns true when the env var is set to "true".
// Any other value (including unset) is off.
func fleetManagementEventsEnabled() bool {
	return os.Getenv(envFleetManagementEventsEnabled) == "true"
}

// Event reasons emitted by the daemon. Stable identifiers users can grep
// for in `kubectl get events --field-selector reason=...`.
const (
	eventReasonRemoteTaskReceived          = "RemoteTaskReceived"
	eventReasonRemoteTaskRejected          = "RemoteTaskRejected"
	eventReasonRemoteTaskCompleted         = "RemoteTaskCompleted"
	eventReasonLocalTerminationPublishedRC = "LocalTerminationPublishedToRC"
	eventReasonInstallerStateRehydrated    = "InstallerStateRehydrated"
)

// emitDDAEventf looks up the DDA in the informer cache and records a
// Kubernetes event on it. Best-effort — if the env var is unset, the
// recorder is nil (test setups that construct Daemon directly), or the
// DDA is not in cache, the call is a no-op and does not propagate
// errors. Returning an error from event emission would be worse than
// missing an observability signal.
func (d *Daemon) emitDDAEventf(ctx context.Context, nsn types.NamespacedName, eventType, reason, format string, args ...any) {
	if !fleetManagementEventsEnabled() || d.recorder == nil || d.client == nil {
		return
	}
	dda := &v2alpha1.DatadogAgent{}
	if err := d.client.Get(ctx, nsn, dda); err != nil {
		ctrl.LoggerFrom(ctx).V(1).Info("event emit: DDA lookup failed, skipping",
			"namespace", nsn.Namespace, "name", nsn.Name, "reason", reason, "error", err.Error())
		return
	}
	d.recorder.Eventf(dda, eventType, reason, format, args...)
}

// emitTaskReceivedEvent records the incoming-edge event when the daemon
// receives a Fleet task. Fires at handleTask entry, before processing —
// the only event that does not follow a successful commit.
func (d *Daemon) emitTaskReceivedEvent(ctx context.Context, req remoteAPIRequest) {
	d.emitDDAEventf(ctx, req.Params.NamespacedName,
		corev1.EventTypeNormal, eventReasonRemoteTaskReceived,
		"Received %s task %q for experiment %q", methodLabel(req.Method), req.ID, req.Params.Version)
}

// emitTaskRejectedEvent records that handleTask refused a task because
// the local state is impossible (INVALID_STATE) or processing failed
// (ERROR). Fires after setTaskState commits the rejection.
func (d *Daemon) emitTaskRejectedEvent(ctx context.Context, nsn types.NamespacedName, req remoteAPIRequest, reason string) {
	d.emitDDAEventf(ctx, nsn,
		corev1.EventTypeWarning, eventReasonRemoteTaskRejected,
		"Rejected task %q (%s): %s", req.ID, methodLabel(req.Method), reason)
}

// emitTaskCompletedEvent records that the daemon reported DONE for a
// Fleet task to RC. Fires after setTaskState(DONE) commits.
func (d *Daemon) emitTaskCompletedEvent(ctx context.Context, op pendingOperation) {
	d.emitDDAEventf(ctx, op.nsn,
		corev1.EventTypeNormal, eventReasonRemoteTaskCompleted,
		"Task %q (%s) reported to Fleet Automation as DONE", op.taskID, op.intent)
}

// emitLocalTerminationPublishedEvent records that reconcileLocallyTerminatedExperiment
// successfully published a controller-driven terminal state (timeout or abort)
// to RC — both task-state ERROR on StartTaskID and experimentConfigVersion
// clear have shipped.
func (d *Daemon) emitLocalTerminationPublishedEvent(ctx context.Context, nsn types.NamespacedName, experimentID, reason string) {
	d.emitDDAEventf(ctx, nsn,
		corev1.EventTypeNormal, eventReasonLocalTerminationPublishedRC,
		"Published locally-terminated experiment %q (%s) to Fleet Automation", experimentID, reason)
}

// emitInstallerStateRehydratedEvent records that the daemon's installer
// state was seeded from an existing DDA's Status.Experiment on startup
// (recovery after a daemon restart mid-experiment).
func (d *Daemon) emitInstallerStateRehydratedEvent(ctx context.Context, nsn types.NamespacedName, experimentID string, phase v2alpha1.ExperimentPhase) {
	d.emitDDAEventf(ctx, nsn,
		corev1.EventTypeNormal, eventReasonInstallerStateRehydrated,
		"Rehydrated installer state from DatadogAgent: experiment %q (phase %s)", experimentID, phase)
}

// methodLabel maps the wire-format Fleet method to a short word for the
// event message (start / stop / promote / <method>).
func methodLabel(method string) string {
	switch method {
	case methodStartDatadogAgentExperiment:
		return "start"
	case methodStopDatadogAgentExperiment:
		return "stop"
	case methodPromoteDatadogAgentExperiment:
		return "promote"
	default:
		return method
	}
}
