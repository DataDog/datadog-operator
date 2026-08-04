// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"os"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

// envFleetManagementEventsEnabled gates emission of fleet/experiment
// Kubernetes events. Read inline at each emission site so toggling the env
// var on a running operator pod takes effect immediately without restart.
const envFleetManagementEventsEnabled = "DD_FLEET_MANAGEMENT_EVENTS_ENABLED"

// fleetManagementEventsEnabled returns true when the env var is set to "true".
// Any other value (including unset) is off.
func fleetManagementEventsEnabled() bool {
	return os.Getenv(envFleetManagementEventsEnabled) == "true"
}

// Event reasons emitted by the reconciler when the experiment phase
// transitions. Values are stable identifiers users can grep for in
// `kubectl get events --field-selector reason=...`.
const (
	eventReasonExperimentStartProcessed = "ExperimentStartProcessed"
	eventReasonExperimentPromoted       = "ExperimentPromoted"
	eventReasonExperimentRolledBack     = "ExperimentRolledBack"
	eventReasonExperimentTimedOut       = "ExperimentTimedOut"
	eventReasonExperimentAborted        = "ExperimentAborted"
)

// emitExperimentTransitionEvent records a single Kubernetes event on the
// DDA describing the experiment phase transition observed in this
// reconcile, if any. Called from updateStatusIfNeededV2 after the status
// write succeeds — emitting at the commit point guarantees we don't
// announce a transition that then 409s and never lands.
//
// The decision of which reason/type to emit is driven by the *transition*
// (old phase → new phase), not the new state alone, so reconciles that
// don't change the experiment phase emit nothing.
//
// No-op when fleetManagementEventsEnabled() is false or the recorder is
// unset (unit tests construct Reconciler without one).
func (r *Reconciler) emitExperimentTransitionEvent(dda client.Object, oldStatus, newStatus *v2alpha1.ExperimentStatus) {
	if !fleetManagementEventsEnabled() || r.recorder == nil {
		return
	}
	var oldPhase v2alpha1.ExperimentPhase
	if oldStatus != nil {
		oldPhase = oldStatus.Phase
	}
	if newStatus == nil || newStatus.Phase == oldPhase {
		return
	}
	switch {
	// nil → Running (fresh start) and terminal → Running (new experiment
	// after a previous Promoted/Aborted/Terminated). processStartSignal
	// allows the latter when the new annotationID differs from the
	// current ID and the current phase is non-Running.
	case newStatus.Phase == v2alpha1.ExperimentPhaseRunning && (oldPhase == "" || isTerminalPhase(oldPhase)):
		r.recorder.Eventf(dda, corev1.EventTypeNormal, eventReasonExperimentStartProcessed,
			"Experiment %q started (task %q)", newStatus.ID, newStatus.StartTaskID)
	case oldPhase == v2alpha1.ExperimentPhaseRunning && newStatus.Phase == v2alpha1.ExperimentPhasePromoted:
		r.recorder.Eventf(dda, corev1.EventTypeNormal, eventReasonExperimentPromoted,
			"Experiment %q promoted", newStatus.ID)
	case oldPhase == v2alpha1.ExperimentPhaseRunning && newStatus.Phase == v2alpha1.ExperimentPhaseAborted:
		r.recorder.Eventf(dda, corev1.EventTypeWarning, eventReasonExperimentAborted,
			"Experiment %q aborted: manual spec change detected", newStatus.ID)
	// Running → Terminated and the "transition 6" recovery path (nil →
	// Terminated/stopped) in processRollbackSignal — when a rollback
	// signal arrives at nil phase with the spec matching the experiment
	// revision, restorePreviousSpec commits Phase=Terminated directly.
	case newStatus.Phase == v2alpha1.ExperimentPhaseTerminated && (oldPhase == v2alpha1.ExperimentPhaseRunning || oldPhase == ""):
		switch newStatus.TerminationReason {
		case ExperimentTerminationReasonTimedOut:
			r.recorder.Eventf(dda, corev1.EventTypeWarning, eventReasonExperimentTimedOut,
				"Experiment %q timed out", newStatus.ID)
		case ExperimentTerminationReasonStopped:
			r.recorder.Eventf(dda, corev1.EventTypeNormal, eventReasonExperimentRolledBack,
				"Experiment %q rolled back", newStatus.ID)
		}
	}
}
