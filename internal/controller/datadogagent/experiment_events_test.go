// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/record"

	v2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

// fakeRecorderReconciler wires a record.FakeRecorder onto a Reconciler so
// emitExperimentTransitionEvent's emissions can be asserted via channel reads.
func fakeRecorderReconciler() (*Reconciler, *record.FakeRecorder) {
	rec := record.NewFakeRecorder(16)
	return &Reconciler{recorder: rec}, rec
}

func drainOne(t *testing.T, rec *record.FakeRecorder) (event string, ok bool) {
	t.Helper()
	select {
	case e := <-rec.Events:
		return e, true
	default:
		return "", false
	}
}

func TestEmitExperimentTransitionEvent_DisabledByDefault(t *testing.T) {
	r, rec := fakeRecorderReconciler()
	dda := newRevisionTestOwner("test-dda", "default")
	r.emitExperimentTransitionEvent(dda, nil, &v2alpha1.ExperimentStatus{
		Phase: v2alpha1.ExperimentPhaseRunning, ID: "exp-1",
	})
	_, got := drainOne(t, rec)
	assert.False(t, got, "no event emitted when DD_FLEET_MANAGEMENT_EVENTS_ENABLED is unset")
}

// TestEmitExperimentTransitionEvent_AllTransitions drives each
// (old → new) transition the helper recognizes and asserts the emitted
// event's reason and type. Sub-tests share a single helper so the
// switch-statement coverage is explicit and easy to read.
func TestEmitExperimentTransitionEvent_AllTransitions(t *testing.T) {
	t.Setenv(envFleetManagementEventsEnabled, "true")

	cases := []struct {
		name       string
		oldStatus  *v2alpha1.ExperimentStatus
		newStatus  *v2alpha1.ExperimentStatus
		wantReason string
		wantType   string // "Normal" | "Warning"
		wantInMsg  string // a substring the message must contain
	}{
		{
			name:       "nil to Running",
			oldStatus:  nil,
			newStatus:  &v2alpha1.ExperimentStatus{Phase: v2alpha1.ExperimentPhaseRunning, ID: "exp-1", StartTaskID: "task-A"},
			wantReason: eventReasonExperimentStartProcessed,
			wantType:   "Normal",
			wantInMsg:  "exp-1",
		},
		{
			name:       "Running to Promoted",
			oldStatus:  &v2alpha1.ExperimentStatus{Phase: v2alpha1.ExperimentPhaseRunning, ID: "exp-1"},
			newStatus:  &v2alpha1.ExperimentStatus{Phase: v2alpha1.ExperimentPhasePromoted, ID: "exp-1"},
			wantReason: eventReasonExperimentPromoted,
			wantType:   "Normal",
			wantInMsg:  "promoted",
		},
		{
			name:       "Running to Aborted",
			oldStatus:  &v2alpha1.ExperimentStatus{Phase: v2alpha1.ExperimentPhaseRunning, ID: "exp-1"},
			newStatus:  &v2alpha1.ExperimentStatus{Phase: v2alpha1.ExperimentPhaseAborted, ID: "exp-1"},
			wantReason: eventReasonExperimentAborted,
			wantType:   "Warning",
			wantInMsg:  "manual spec change",
		},
		{
			name:       "Running to Terminated/timed_out",
			oldStatus:  &v2alpha1.ExperimentStatus{Phase: v2alpha1.ExperimentPhaseRunning, ID: "exp-1"},
			newStatus:  &v2alpha1.ExperimentStatus{Phase: v2alpha1.ExperimentPhaseTerminated, ID: "exp-1", TerminationReason: ExperimentTerminationReasonTimedOut},
			wantReason: eventReasonExperimentTimedOut,
			wantType:   "Warning",
			wantInMsg:  "timed out",
		},
		{
			name:       "Running to Terminated/stopped",
			oldStatus:  &v2alpha1.ExperimentStatus{Phase: v2alpha1.ExperimentPhaseRunning, ID: "exp-1"},
			newStatus:  &v2alpha1.ExperimentStatus{Phase: v2alpha1.ExperimentPhaseTerminated, ID: "exp-1", TerminationReason: ExperimentTerminationReasonStopped},
			wantReason: eventReasonExperimentRolledBack,
			wantType:   "Normal",
			wantInMsg:  "rolled back",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, rec := fakeRecorderReconciler()
			dda := newRevisionTestOwner("test-dda", "default")
			r.emitExperimentTransitionEvent(dda, tc.oldStatus, tc.newStatus)
			ev, got := drainOne(t, rec)
			require.True(t, got, "expected one event for transition %q → %q",
				phaseOf(tc.oldStatus), phaseOf(tc.newStatus))
			assert.Contains(t, ev, tc.wantType, "event type")
			assert.Contains(t, ev, tc.wantReason, "event reason")
			assert.Contains(t, ev, tc.wantInMsg, "event message substring")
			_, more := drainOne(t, rec)
			assert.False(t, more, "exactly one event expected per transition")
		})
	}
}

func TestEmitExperimentTransitionEvent_NoOpOnSamePhase(t *testing.T) {
	t.Setenv(envFleetManagementEventsEnabled, "true")
	r, rec := fakeRecorderReconciler()
	dda := newRevisionTestOwner("test-dda", "default")
	status := &v2alpha1.ExperimentStatus{Phase: v2alpha1.ExperimentPhaseRunning, ID: "exp-1"}
	r.emitExperimentTransitionEvent(dda, status, status)
	_, got := drainOne(t, rec)
	assert.False(t, got, "no event when phase didn't change")
}

func phaseOf(s *v2alpha1.ExperimentStatus) v2alpha1.ExperimentPhase {
	if s == nil {
		return ""
	}
	return s.Phase
}
