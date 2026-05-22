// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fleet

import (
	"context"
	"testing"

	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"

	v2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

// fakeRecorderDaemon wires a record.FakeRecorder onto a Daemon backed by
// a fake client containing dda so emitDDAEvent's lookup succeeds.
func fakeRecorderDaemon(t *testing.T, dda *v2alpha1.DatadogAgent) (*Daemon, *record.FakeRecorder) {
	t.Helper()
	rec := record.NewFakeRecorder(16)
	d, _ := testDaemon(dda, nil)
	d.recorder = rec
	return d, rec
}

func drainOneDaemonEvent(t *testing.T, rec *record.FakeRecorder) (event string, ok bool) {
	t.Helper()
	select {
	case e := <-rec.Events:
		return e, true
	default:
		return "", false
	}
}

func TestEmitDaemonEvent_DisabledByDefault(t *testing.T) {
	dda := testDDAObject(v2alpha1.ExperimentPhaseRunning)
	d, rec := fakeRecorderDaemon(t, dda)

	d.emitTaskReceivedEvent(context.Background(), testStartRequest())
	d.emitTaskCompletedEvent(context.Background(), pendingOperation{
		taskID: "t1", intent: pendingIntentStart, nsn: testDDANSN,
	})

	_, got := drainOneDaemonEvent(t, rec)
	assert.False(t, got, "no events emitted when DD_FLEET_MANAGEMENT_EVENTS_ENABLED is unset")
}

// TestEmitDaemonEvent_AllReasons drives each of the five daemon-side
// helpers under DD_FLEET_MANAGEMENT_EVENTS_ENABLED=true and asserts the
// reason + a substring of the message. The fake recorder's channel
// receives events in `<Type> <Reason> <Message>` form.
func TestEmitDaemonEvent_AllReasons(t *testing.T) {
	t.Setenv(envFleetManagementEventsEnabled, "true")

	cases := []struct {
		name       string
		emit       func(d *Daemon)
		wantReason string
		wantType   string
		wantInMsg  string
	}{
		{
			name: "RemoteTaskReceived",
			emit: func(d *Daemon) {
				d.emitTaskReceivedEvent(context.Background(), testStartRequest())
			},
			wantReason: eventReasonRemoteTaskReceived,
			wantType:   "Normal",
			wantInMsg:  "start",
		},
		{
			name: "RemoteTaskRejected",
			emit: func(d *Daemon) {
				d.emitTaskRejectedEvent(context.Background(), testDDANSN, testStartRequest(),
					"experiment already terminated")
			},
			wantReason: eventReasonRemoteTaskRejected,
			wantType:   "Warning",
			wantInMsg:  "experiment already terminated",
		},
		{
			name: "RemoteTaskCompleted",
			emit: func(d *Daemon) {
				d.emitTaskCompletedEvent(context.Background(), pendingOperation{
					taskID:       "task-xyz",
					intent:       pendingIntentStart,
					nsn:          testDDANSN,
					experimentID: testExperimentID,
					packageName:  "datadog-operator",
				})
			},
			wantReason: eventReasonRemoteTaskCompleted,
			wantType:   "Normal",
			wantInMsg:  "task-xyz",
		},
		{
			name: "LocalTerminationPublishedToRC",
			emit: func(d *Daemon) {
				d.emitLocalTerminationPublishedEvent(context.Background(), testDDANSN,
					testExperimentID, "experiment "+testExperimentID+" timed out")
			},
			wantReason: eventReasonLocalTerminationPublishedRC,
			wantType:   "Normal",
			wantInMsg:  "timed out",
		},
		{
			name: "InstallerStateRehydrated",
			emit: func(d *Daemon) {
				d.emitInstallerStateRehydratedEvent(context.Background(), testDDANSN,
					testExperimentID, v2alpha1.ExperimentPhaseRunning)
			},
			wantReason: eventReasonInstallerStateRehydrated,
			wantType:   "Normal",
			wantInMsg:  string(v2alpha1.ExperimentPhaseRunning),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dda := testDDAObject(v2alpha1.ExperimentPhaseRunning)
			d, rec := fakeRecorderDaemon(t, dda)

			tc.emit(d)

			ev, got := drainOneDaemonEvent(t, rec)
			require.True(t, got, "expected one event")
			assert.Contains(t, ev, tc.wantType)
			assert.Contains(t, ev, tc.wantReason)
			assert.Contains(t, ev, tc.wantInMsg)
		})
	}
}

// TestEmitDDAEvent_NoLookupWhenDisabled verifies that the env-var-disabled
// path short-circuits before the client.Get lookup runs. This matters
// because some emission sites are on hot paths (every received task,
// every reconcile snapshot) — even a cache Get per event would add load
// when events are off, which they will be by default.
func TestEmitDDAEvent_NoLookupWhenDisabled(t *testing.T) {
	// Construct a Daemon with a nil client. Any code path that tries to
	// dereference it would panic. emitDDAEvent must short-circuit on the
	// env-var check before touching client.
	rec := record.NewFakeRecorder(1)
	d := &Daemon{recorder: rec}
	d.emitDDAEventf(context.Background(), types.NamespacedName{Namespace: "ns", Name: "n"},
		"Normal", "TestReason", "irrelevant")
	_, got := drainOneDaemonEvent(t, rec)
	assert.False(t, got, "no event when env var is unset; also no panic from nil client")
}

// TestHandleTask_IdempotentDONE_EmitsCompletedEvent verifies that the
// immediate-DONE path in handleTask (pending == nil, err == nil) emits
// both the incoming-edge RemoteTaskReceived event AND a terminal
// RemoteTaskCompleted event. Without the latter, idempotent retries
// of an already-running experiment would show as half-finished on the
// kubectl-events timeline.
func TestHandleTask_IdempotentDONE_EmitsCompletedEvent(t *testing.T) {
	t.Setenv(envFleetManagementEventsEnabled, "true")

	// DDA already in Running with the target experiment ID — so the
	// next start task for the same ID is an idempotent no-op that goes
	// straight to TaskState_DONE without engaging the worker.
	dda := testDDAObject(v2alpha1.ExperimentPhaseRunning)
	d, rec := fakeRecorderDaemon(t, dda)
	d.rcClient = &mockRCClient{state: []*pbgo.PackageState{{Package: "datadog-operator"}}}
	d.configs = testInstallerConfigWithDDA()

	req := testStartRequest()
	req.Params.Version = testExperimentID

	require.NoError(t, d.handleTask(context.Background(), req))

	// Expect two events, in order: RemoteTaskReceived then RemoteTaskCompleted.
	ev1, got1 := drainOneDaemonEvent(t, rec)
	require.True(t, got1, "RemoteTaskReceived should fire on handleTask entry")
	assert.Contains(t, ev1, eventReasonRemoteTaskReceived)

	ev2, got2 := drainOneDaemonEvent(t, rec)
	require.True(t, got2, "RemoteTaskCompleted should fire on the immediate-DONE path")
	assert.Contains(t, ev2, eventReasonRemoteTaskCompleted)
	assert.Contains(t, ev2, req.ID, "completed event should reference the task ID")
}
