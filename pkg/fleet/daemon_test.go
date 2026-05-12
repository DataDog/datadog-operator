// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fleet

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

// --- Test helpers ---

func testFleetScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = v2alpha1.AddToScheme(s)
	return s
}

func testDaemon(dda *v2alpha1.DatadogAgent, configs map[string]installerConfig) (*Daemon, client.Client) {
	s := testFleetScheme()
	b := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(&v2alpha1.DatadogAgent{})
	if dda != nil {
		b = b.WithObjects(dda)
	}
	c := b.Build()
	return &Daemon{
		client:           c,
		revisionsEnabled: true,
		configs:          configs,
		statusUpdates:    make(chan ddaStatusSnapshot, 32),
	}, c
}

var testDDAGVK = schema.GroupVersionKind{
	Group:   "datadoghq.com",
	Version: "v2alpha1",
	Kind:    "DatadogAgent",
}

var testDDANSN = types.NamespacedName{Namespace: "datadog", Name: "datadog-agent"}

const testExperimentID = "test-config"

func testDDAObject(phase v2alpha1.ExperimentPhase) *v2alpha1.DatadogAgent {
	dda := &v2alpha1.DatadogAgent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testDDANSN.Name,
			Namespace: testDDANSN.Namespace,
		},
	}
	if phase != "" {
		dda.Status.Experiment = &v2alpha1.ExperimentStatus{Phase: phase, ID: testExperimentID}
		// Set experiment annotations to match status — this is the state the daemon
		// would see after the controller has processed the start signal.
		if dda.Annotations == nil {
			dda.Annotations = make(map[string]string)
		}
		dda.Annotations[v2alpha1.AnnotationExperimentID] = testExperimentID
		dda.Annotations[v2alpha1.AnnotationExperimentSignal] = v2alpha1.ExperimentSignalStart
	}
	return dda
}

func syncTaskErr(op *pendingOperation, err error) error {
	if err == nil && op != nil {
		return errors.New("unexpected pending operation")
	}
	return err
}

func requireSyncNoError(t *testing.T, op *pendingOperation, err error) {
	t.Helper()
	require.NoError(t, err)
	require.Nil(t, op)
}

func requirePendingNoError(t *testing.T, op *pendingOperation, err error) *pendingOperation {
	t.Helper()
	require.NoError(t, err)
	require.NotNil(t, op)
	return op
}

func requireStartQueued(t *testing.T, d *Daemon, req remoteAPIRequest) *pendingOperation {
	t.Helper()
	op, err := d.startDatadogAgentExperiment(context.Background(), req)
	return requirePendingNoError(t, op, err)
}

func requireStopQueued(t *testing.T, d *Daemon, req remoteAPIRequest) *pendingOperation {
	t.Helper()
	op, err := d.stopDatadogAgentExperiment(context.Background(), req)
	return requirePendingNoError(t, op, err)
}

func requirePromoteQueued(t *testing.T, d *Daemon, req remoteAPIRequest) *pendingOperation {
	t.Helper()
	op, err := d.promoteDatadogAgentExperiment(context.Background(), req)
	return requirePendingNoError(t, op, err)
}

func requireOperationSuccess(t *testing.T, d *Daemon, op *pendingOperation) {
	t.Helper()
	d.finishPendingOperation(context.Background(), *op, nil)
}

func testInstallerConfigWithDDA() map[string]installerConfig {
	return map[string]installerConfig{
		"test-config": {
			ID: "test-config",
			Operations: []fleetManagementOperation{
				{
					Operation: OperationUpdate,
					Config:    json.RawMessage(`{}`),
				},
			},
		},
	}
}

func testCompletedAgentStatus() *v2alpha1.DaemonSetStatus {
	return &v2alpha1.DaemonSetStatus{Desired: 3, UpToDate: 3, Ready: 3}
}

func testStartRequest() remoteAPIRequest {
	return remoteAPIRequest{
		ID:      "exp-abc",
		Package: "datadog-operator",
		Method:  methodStartDatadogAgentExperiment,
		Params: experimentParams{
			Version:          "test-config",
			GroupVersionKind: testDDAGVK,
			NamespacedName:   testDDANSN,
		},
	}
}

// --- startDatadogAgentExperiment tests ---

func TestStartDatadogAgentExperiment_ConfigNotFound(t *testing.T) {
	d, _ := testDaemon(testDDAObject(""), testInstallerConfigWithDDA())
	req := testStartRequest()
	req.Params.Version = "nonexistent"
	assert.Error(t, syncTaskErr(d.startDatadogAgentExperiment(context.Background(), req)))
}

func TestStartDatadogAgentExperiment_WrongGVK(t *testing.T) {
	d, _ := testDaemon(testDDAObject(""), testInstallerConfigWithDDA())
	req := testStartRequest()
	req.Params.GroupVersionKind = schema.GroupVersionKind{Group: "other.io", Version: "v1", Kind: "Other"}
	assert.Error(t, syncTaskErr(d.startDatadogAgentExperiment(context.Background(), req)))
}

func TestStartDatadogAgentExperiment_DDANotFound(t *testing.T) {
	d, _ := testDaemon(nil, testInstallerConfigWithDDA()) // no DDA pre-created
	assert.Error(t, syncTaskErr(d.startDatadogAgentExperiment(context.Background(), testStartRequest())))
}

func TestStartDatadogAgentExperiment_Running_Idempotent(t *testing.T) {
	dda := testDDAObject(v2alpha1.ExperimentPhaseRunning)
	d, c := testDaemon(dda, testInstallerConfigWithDDA())
	// Same experiment ID retried — should be a no-op since it's already running.
	req := testStartRequest()
	req.Params.Version = testExperimentID
	op, err := d.startDatadogAgentExperiment(context.Background(), req)
	requireSyncNoError(t, op, err)

	// DDA should be unchanged — no re-patch, no status update.
	got := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), testDDANSN, got))
	require.NotNil(t, got.Status.Experiment)
	assert.Equal(t, v2alpha1.ExperimentPhaseRunning, got.Status.Experiment.Phase)
	assert.Equal(t, testExperimentID, got.Status.Experiment.ID)
}

func TestStartDatadogAgentExperiment_Running_Idempotent_RestoresConfigVersion(t *testing.T) {
	dda := testDDAObject(v2alpha1.ExperimentPhaseRunning)
	d, _ := testDaemon(dda, testInstallerConfigWithDDA())
	// Simulate post-restart: rcClient exists but has no config versions yet.
	rc := &mockRCClient{state: []*pbgo.PackageState{
		{Package: "datadog-operator"},
	}}
	d.rcClient = rc

	// Retry the same start request — idempotent path should restore config version.
	req := testStartRequest()
	req.Params.Version = testExperimentID
	op, err := d.startDatadogAgentExperiment(context.Background(), req)
	requireSyncNoError(t, op, err)

	require.Len(t, rc.state, 1)
	assert.Equal(t, req.Params.Version, rc.state[0].ExperimentConfigVersion,
		"idempotent start path should restore experiment config version")
}

func TestStartDatadogAgentExperiment_Running_DifferentID(t *testing.T) {
	dda := testDDAObject(v2alpha1.ExperimentPhaseRunning)
	configs := testInstallerConfigWithDDA()
	configs["different-exp"] = installerConfig{
		ID: "different-exp",
		Operations: []fleetManagementOperation{
			{
				Operation: OperationUpdate,
				Config:    json.RawMessage(`{}`),
			},
		},
	}
	d, _ := testDaemon(dda, configs)
	// Different experiment version while one is already running — should be rejected.
	req := testStartRequest()
	req.Params.Version = "different-exp"
	assert.Error(t, syncTaskErr(d.startDatadogAgentExperiment(context.Background(), req)))
}

func TestStartDatadogAgentExperiment_Success_NilPhase(t *testing.T) {
	d, c := testDaemon(testDDAObject(""), testInstallerConfigWithDDA())
	req := testStartRequest()
	requireStartQueued(t, d, req)

	dda := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), testDDANSN, dda))
	// Daemon writes annotations, not status. Status is written by the controller.
	assert.Equal(t, req.Params.Version, dda.Annotations[v2alpha1.AnnotationExperimentID])
	assert.Equal(t, v2alpha1.ExperimentSignalStart, dda.Annotations[v2alpha1.AnnotationExperimentSignal])
	assert.Equal(t, req.ID, dda.Annotations[v2alpha1.AnnotationPendingTaskID])
	assert.Equal(t, string(pendingIntentStart), dda.Annotations[v2alpha1.AnnotationPendingAction])
	assert.Equal(t, req.Params.Version, dda.Annotations[v2alpha1.AnnotationPendingExperimentID])
	assert.Equal(t, req.Package, dda.Annotations[v2alpha1.AnnotationPendingPackage])
}

func TestStartDatadogAgentExperiment_RejectsExistingPendingOperation(t *testing.T) {
	dda := testDDAObject("")
	dda.Annotations = map[string]string{
		v2alpha1.AnnotationPendingTaskID:       "stop-task",
		v2alpha1.AnnotationPendingAction:       string(pendingIntentStop),
		v2alpha1.AnnotationPendingExperimentID: testExperimentID,
		v2alpha1.AnnotationPendingPackage:      "datadog-operator",
	}
	d, _ := testDaemon(dda, testInstallerConfigWithDDA())

	err := syncTaskErr(d.startDatadogAgentExperiment(context.Background(), testStartRequest()))

	var stateErr *stateDoesntMatchError
	require.ErrorAs(t, err, &stateErr)
	assert.Contains(t, err.Error(), "already exists")
}

func TestStartDatadogAgentExperiment_RejectsDifferentPendingStart(t *testing.T) {
	dda := testDDAObject("")
	dda.Annotations = map[string]string{
		v2alpha1.AnnotationExperimentID:        testExperimentID,
		v2alpha1.AnnotationExperimentSignal:    v2alpha1.ExperimentSignalStart,
		v2alpha1.AnnotationPendingTaskID:       "task-1",
		v2alpha1.AnnotationPendingAction:       string(pendingIntentStart),
		v2alpha1.AnnotationPendingExperimentID: testExperimentID,
		v2alpha1.AnnotationPendingPackage:      "datadog-operator",
	}
	configs := testInstallerConfigWithDDA()
	configs["different-exp"] = installerConfig{
		ID: "different-exp",
		Operations: []fleetManagementOperation{
			{Operation: OperationUpdate, Config: json.RawMessage(`{}`)},
		},
	}
	d, _ := testDaemon(dda, configs)
	req := testStartRequest()
	req.ID = "task-2"
	req.Params.Version = "different-exp"

	err := syncTaskErr(d.startDatadogAgentExperiment(context.Background(), req))

	var stateErr *stateDoesntMatchError
	require.ErrorAs(t, err, &stateErr)
	assert.Contains(t, err.Error(), "already exists")
}

func TestStartDatadogAgentExperiment_Success_FromTerminated(t *testing.T) {
	d, c := testDaemon(testDDAObject(v2alpha1.ExperimentPhaseTerminated), testInstallerConfigWithDDA())
	req := testStartRequest()
	requireStartQueued(t, d, req)

	dda := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), testDDANSN, dda))
	assert.Equal(t, req.Params.Version, dda.Annotations[v2alpha1.AnnotationExperimentID])
	assert.Equal(t, v2alpha1.ExperimentSignalStart, dda.Annotations[v2alpha1.AnnotationExperimentSignal])
}

func TestStartDatadogAgentExperiment_Success_FromAborted(t *testing.T) {
	d, c := testDaemon(testDDAObject(v2alpha1.ExperimentPhaseAborted), testInstallerConfigWithDDA())
	req := testStartRequest()
	requireStartQueued(t, d, req)

	dda := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), testDDANSN, dda))
	assert.Equal(t, req.Params.Version, dda.Annotations[v2alpha1.AnnotationExperimentID])
	assert.Equal(t, v2alpha1.ExperimentSignalStart, dda.Annotations[v2alpha1.AnnotationExperimentSignal])
}

func TestStartDatadogAgentExperiment_Success_OverwritesPreviousExperiment(t *testing.T) {
	// Start a new experiment when a previous one exists (e.g. after termination).
	// The old experiment's annotations must be fully replaced.
	d, c := testDaemon(testDDAObject(v2alpha1.ExperimentPhaseTerminated), testInstallerConfigWithDDA())
	req := testStartRequest()
	requireStartQueued(t, d, req)

	dda := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), testDDANSN, dda))
	assert.Equal(t, req.Params.Version, dda.Annotations[v2alpha1.AnnotationExperimentID])
	assert.Equal(t, v2alpha1.ExperimentSignalStart, dda.Annotations[v2alpha1.AnnotationExperimentSignal])
}

// --- resolveOperation: params.version matching tests ---

func TestStartDatadogAgentExperiment_VersionMatchesInstallerConfig(t *testing.T) {
	// Simulates the real payload pairing:
	// - installer_config has id "cyg9-1ztz-cdnn" with an APM-enabling patch
	// - updater_task has params.version "cyg9-1ztz-cdnn" linking them together
	configs := map[string]installerConfig{
		"aaaa-bbbb-cccc": {
			ID: "aaaa-bbbb-cccc",
			Operations: []fleetManagementOperation{
				{
					Operation: OperationUpdate,
					Config:    json.RawMessage(`{"spec":{"features":{"apm":{"enabled":true}}}}`),
				},
			},
		},
	}
	d, c := testDaemon(testDDAObject(""), configs)
	req := remoteAPIRequest{
		ID:     "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
		Method: methodStartDatadogAgentExperiment,
		Params: experimentParams{
			Version:          "aaaa-bbbb-cccc",
			GroupVersionKind: testDDAGVK,
			NamespacedName:   testDDANSN,
		},
		ExpectedState: expectedState{
			Stable:       "0.0.1",
			StableConfig: "0.0.1",
			ClientID:     "aAbBcCdDeEfFgGhHiIjJk",
		},
	}
	requireStartQueued(t, d, req)

	dda := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), testDDANSN, dda))
	assert.Equal(t, req.Params.Version, dda.Annotations[v2alpha1.AnnotationExperimentID])
	assert.Equal(t, v2alpha1.ExperimentSignalStart, dda.Annotations[v2alpha1.AnnotationExperimentSignal])
}

func TestStartDatadogAgentExperiment_EmptyVersion(t *testing.T) {
	d, _ := testDaemon(testDDAObject(""), testInstallerConfigWithDDA())
	req := testStartRequest()
	req.Params.Version = ""
	assert.Error(t, syncTaskErr(d.startDatadogAgentExperiment(context.Background(), req)))
}

func TestStartDatadogAgentExperiment_VersionMismatch(t *testing.T) {
	// params.version doesn't match any installer config ID
	d, _ := testDaemon(testDDAObject(""), testInstallerConfigWithDDA())
	req := testStartRequest()
	req.Params.Version = "xxxx-yyyy-zzzz" // no config with this ID
	err := syncTaskErr(d.startDatadogAgentExperiment(context.Background(), req))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// --- Stop tests ---

func testStopRequest() remoteAPIRequest {
	return remoteAPIRequest{
		ID:      "exp-abc",
		Package: "datadog-operator",
		Method:  methodStopDatadogAgentExperiment,
		Params: experimentParams{
			Version:          "test-config",
			GroupVersionKind: testDDAGVK,
			NamespacedName:   testDDANSN,
		},
	}
}

func TestStopDatadogAgentExperiment_DDANotFound(t *testing.T) {
	d, _ := testDaemon(nil, testInstallerConfigWithDDA())
	assert.Error(t, syncTaskErr(d.stopDatadogAgentExperiment(context.Background(), testStopRequest())))
}

func TestStopDatadogAgentExperiment_NilExperiment_NoOp(t *testing.T) {
	// Nil experiment status means there is no active experiment object to stop.
	d, c := testDaemon(testDDAObject(""), testInstallerConfigWithDDA())
	rc := &mockRCClient{state: []*pbgo.PackageState{
		{Package: "datadog-operator", StableConfigVersion: "stable-1", ExperimentConfigVersion: "exp-1"},
	}}
	d.rcClient = rc
	op, err := d.stopDatadogAgentExperiment(context.Background(), testStopRequest())
	requireSyncNoError(t, op, err)

	dda := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), testDDANSN, dda))
	assert.Empty(t, dda.Annotations[v2alpha1.AnnotationExperimentSignal])
	assert.Empty(t, dda.Annotations[v2alpha1.AnnotationExperimentID])
	assert.Equal(t, "stable-1", rc.state[0].StableConfigVersion)
	assert.Empty(t, rc.state[0].ExperimentConfigVersion)
}

func TestStopDatadogAgentExperiment_NilExperiment_WithStartAnnotation_WritesRollback(t *testing.T) {
	dda := testDDAObject("")
	dda.Annotations = map[string]string{
		v2alpha1.AnnotationExperimentID:     testExperimentID,
		v2alpha1.AnnotationExperimentSignal: v2alpha1.ExperimentSignalStart,
	}
	d, c := testDaemon(dda, testInstallerConfigWithDDA())
	req := testStopRequest()
	req.Params.Version = "ignored-request-version"
	op := requireStopQueued(t, d, req)
	assert.Equal(t, testExperimentID, op.experimentID)

	got := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), testDDANSN, got))
	assert.Equal(t, v2alpha1.ExperimentSignalRollback, got.Annotations[v2alpha1.AnnotationExperimentSignal])
	assert.Equal(t, testExperimentID, got.Annotations[v2alpha1.AnnotationExperimentID])
}

func TestStopDatadogAgentExperiment_SupersedesPendingStart(t *testing.T) {
	dda := testDDAObject("")
	dda.Annotations = map[string]string{
		v2alpha1.AnnotationExperimentID:         testExperimentID,
		v2alpha1.AnnotationExperimentSignal:     v2alpha1.ExperimentSignalStart,
		v2alpha1.AnnotationPendingTaskID:        "start-task",
		v2alpha1.AnnotationPendingAction:        string(pendingIntentStart),
		v2alpha1.AnnotationPendingExperimentID:  testExperimentID,
		v2alpha1.AnnotationPendingPackage:       "datadog-operator",
		v2alpha1.AnnotationPendingResultVersion: "",
	}
	d, c := testDaemon(dda, testInstallerConfigWithDDA())

	req := testStopRequest()
	req.ID = "stop-task"
	op := requireStopQueued(t, d, req)

	assert.Equal(t, pendingIntentStop, op.intent)
	got := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), testDDANSN, got))
	assert.Equal(t, v2alpha1.ExperimentSignalRollback, got.Annotations[v2alpha1.AnnotationExperimentSignal])
	assert.Equal(t, "stop-task", got.Annotations[v2alpha1.AnnotationPendingTaskID])
	assert.Equal(t, string(pendingIntentStop), got.Annotations[v2alpha1.AnnotationPendingAction])
}

func TestStopDatadogAgentExperiment_RejectsExistingPendingStop(t *testing.T) {
	dda := testDDAObject(v2alpha1.ExperimentPhaseRunning)
	dda.Annotations[v2alpha1.AnnotationPendingTaskID] = "task-1"
	dda.Annotations[v2alpha1.AnnotationPendingAction] = string(pendingIntentStop)
	dda.Annotations[v2alpha1.AnnotationPendingExperimentID] = testExperimentID
	dda.Annotations[v2alpha1.AnnotationPendingPackage] = "datadog-operator"
	d, _ := testDaemon(dda, testInstallerConfigWithDDA())
	req := testStopRequest()
	req.ID = "task-2"

	err := syncTaskErr(d.stopDatadogAgentExperiment(context.Background(), req))

	var stateErr *stateDoesntMatchError
	require.ErrorAs(t, err, &stateErr)
	assert.Contains(t, err.Error(), "already exists")
}

func TestStopDatadogAgentExperiment_EmptyPhase_WritesRollback(t *testing.T) {
	// Empty phase with a status object is Transition 6: spec patched but phase
	// never written. The rollback annotation must still be sent.
	dda := testDDAObject("")
	dda.Status.Experiment = &v2alpha1.ExperimentStatus{ID: testExperimentID, Phase: ""}
	d, c := testDaemon(dda, testInstallerConfigWithDDA())
	requireStopQueued(t, d, testStopRequest())

	got := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), testDDANSN, got))
	assert.Equal(t, v2alpha1.ExperimentSignalRollback, got.Annotations[v2alpha1.AnnotationExperimentSignal])
	assert.Equal(t, testExperimentID, got.Annotations[v2alpha1.AnnotationExperimentID])
}

func TestStopDatadogAgentExperiment_Success_Running(t *testing.T) {
	d, c := testDaemon(testDDAObject(v2alpha1.ExperimentPhaseRunning), testInstallerConfigWithDDA())
	requireStopQueued(t, d, testStopRequest())

	dda := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), testDDANSN, dda))
	// Daemon writes rollback annotation, not status. Status is written by the controller.
	assert.Equal(t, v2alpha1.ExperimentSignalRollback, dda.Annotations[v2alpha1.AnnotationExperimentSignal])
	assert.Equal(t, testExperimentID, dda.Annotations[v2alpha1.AnnotationExperimentID])
}

func TestStopDatadogAgentExperiment_Running_IgnoresRequestVersion(t *testing.T) {
	d, c := testDaemon(testDDAObject(v2alpha1.ExperimentPhaseRunning), testInstallerConfigWithDDA())
	req := testStopRequest()
	req.Params.Version = "ignored-request-version"
	op := requireStopQueued(t, d, req)
	assert.Equal(t, testExperimentID, op.experimentID)

	dda := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), testDDANSN, dda))
	assert.Equal(t, v2alpha1.ExperimentSignalRollback, dda.Annotations[v2alpha1.AnnotationExperimentSignal])
	assert.Equal(t, testExperimentID, dda.Annotations[v2alpha1.AnnotationExperimentID])
}

func TestStopDatadogAgentExperiment_Success_Terminated(t *testing.T) {
	// Already in terminal phase — GET guard skips the patch, clears experiment config version.
	d, _ := testDaemon(testDDAObject(v2alpha1.ExperimentPhaseTerminated), testInstallerConfigWithDDA())
	rc := &mockRCClient{state: []*pbgo.PackageState{
		{Package: "datadog-operator", StableConfigVersion: "stable-1", ExperimentConfigVersion: "exp-1"},
	}}
	d.rcClient = rc
	op, err := d.stopDatadogAgentExperiment(context.Background(), testStopRequest())
	requireSyncNoError(t, op, err)
	assert.Equal(t, "stable-1", rc.state[0].StableConfigVersion)
	assert.Empty(t, rc.state[0].ExperimentConfigVersion, "experiment config version should be cleared")
}

func TestStopDatadogAgentExperiment_Success_Promoted(t *testing.T) {
	// Already in terminal phase — GET guard skips the patch, clears experiment config version.
	d, _ := testDaemon(testDDAObject(v2alpha1.ExperimentPhasePromoted), testInstallerConfigWithDDA())
	rc := &mockRCClient{state: []*pbgo.PackageState{
		{Package: "datadog-operator", StableConfigVersion: "stable-1", ExperimentConfigVersion: "exp-1"},
	}}
	d.rcClient = rc
	op, err := d.stopDatadogAgentExperiment(context.Background(), testStopRequest())
	requireSyncNoError(t, op, err)
	assert.Equal(t, "stable-1", rc.state[0].StableConfigVersion)
	assert.Empty(t, rc.state[0].ExperimentConfigVersion, "experiment config version should be cleared")
}

func TestStopDatadogAgentExperiment_Success_Aborted(t *testing.T) {
	// Already in terminal phase — GET guard skips the patch, clears experiment config version.
	d, _ := testDaemon(testDDAObject(v2alpha1.ExperimentPhaseAborted), testInstallerConfigWithDDA())
	rc := &mockRCClient{state: []*pbgo.PackageState{
		{Package: "datadog-operator", StableConfigVersion: "stable-1", ExperimentConfigVersion: "exp-1"},
	}}
	d.rcClient = rc
	op, err := d.stopDatadogAgentExperiment(context.Background(), testStopRequest())
	requireSyncNoError(t, op, err)
	assert.Equal(t, "stable-1", rc.state[0].StableConfigVersion)
	assert.Empty(t, rc.state[0].ExperimentConfigVersion, "experiment config version should be cleared")
}

func TestStopDatadogAgentExperiment_UnexpectedPhase_Error(t *testing.T) {
	dda := testDDAObject("")
	dda.Status.Experiment = &v2alpha1.ExperimentStatus{ID: "exp-1", Phase: "weird"}
	d, _ := testDaemon(dda, testInstallerConfigWithDDA())
	err := syncTaskErr(d.stopDatadogAgentExperiment(context.Background(), testStopRequest()))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot stop")
}

// --- promoteDatadogAgentExperiment tests ---

func testPromoteRequest() remoteAPIRequest {
	return remoteAPIRequest{
		ID:      "exp-abc",
		Package: "datadog-operator",
		Method:  methodPromoteDatadogAgentExperiment,
		Params: experimentParams{
			Version:          "test-config",
			GroupVersionKind: testDDAGVK,
			NamespacedName:   testDDANSN,
		},
	}
}

func TestPromoteDatadogAgentExperiment_DDANotFound(t *testing.T) {
	d, _ := testDaemon(nil, testInstallerConfigWithDDA())
	assert.Error(t, syncTaskErr(d.promoteDatadogAgentExperiment(context.Background(), testPromoteRequest())))
}

func TestPromoteDatadogAgentExperiment_NoExperimentVersion(t *testing.T) {
	d, _ := testDaemon(testDDAObject(v2alpha1.ExperimentPhaseRunning), testInstallerConfigWithDDA())
	d.rcClient = &mockRCClient{state: []*pbgo.PackageState{
		{Package: "datadog-operator", StableConfigVersion: "stable-1"},
	}}
	assert.Error(t, syncTaskErr(d.promoteDatadogAgentExperiment(context.Background(), testPromoteRequest())))
}

func TestPromoteDatadogAgentExperiment_NilPhase_Error(t *testing.T) {
	// No experiment running (nil status) — guard returns error.
	d, _ := testDaemon(testDDAObject(""), testInstallerConfigWithDDA())
	d.rcClient = &mockRCClient{state: []*pbgo.PackageState{
		{Package: "datadog-operator", StableConfigVersion: "stable-1", ExperimentConfigVersion: "exp-1"},
	}}
	err := syncTaskErr(d.promoteDatadogAgentExperiment(context.Background(), testPromoteRequest()))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot promote")
}

func TestPromoteDatadogAgentExperiment_Success_Running(t *testing.T) {
	d, c := testDaemon(testDDAObject(v2alpha1.ExperimentPhaseRunning), testInstallerConfigWithDDA())
	rc := &mockRCClient{state: []*pbgo.PackageState{
		{Package: "datadog-operator", StableConfigVersion: "stable-1", ExperimentConfigVersion: "exp-1"},
	}}
	d.rcClient = rc
	op := requirePromoteQueued(t, d, testPromoteRequest())

	dda := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), testDDANSN, dda))
	// Daemon writes promote annotation, not status. Status is written by the controller.
	assert.Equal(t, v2alpha1.ExperimentSignalPromote, dda.Annotations[v2alpha1.AnnotationExperimentSignal])
	assert.Equal(t, testExperimentID, dda.Annotations[v2alpha1.AnnotationExperimentID])
	assert.Equal(t, "exp-1", dda.Annotations[v2alpha1.AnnotationPendingResultVersion])
	requireOperationSuccess(t, d, op)
	// Stable should now be the old experiment, experiment should be cleared.
	require.Len(t, rc.state, 1)
	// StableVersion/ExperimentVersion are not set by config experiments.
	assert.Equal(t, "", rc.state[0].StableVersion)
	assert.Equal(t, "", rc.state[0].ExperimentVersion)
	assert.Equal(t, "exp-1", rc.state[0].StableConfigVersion)
	assert.Equal(t, "", rc.state[0].ExperimentConfigVersion)
}

func TestPromoteDatadogAgentExperiment_Terminated_Error(t *testing.T) {
	// Terminated phase — non-running guard returns error with current phase.
	d, _ := testDaemon(testDDAObject(v2alpha1.ExperimentPhaseTerminated), testInstallerConfigWithDDA())
	d.rcClient = &mockRCClient{state: []*pbgo.PackageState{
		{Package: "datadog-operator", StableConfigVersion: "stable-1", ExperimentConfigVersion: "exp-1"},
	}}
	err := syncTaskErr(d.promoteDatadogAgentExperiment(context.Background(), testPromoteRequest()))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot promote")
	assert.Contains(t, err.Error(), "terminated")
}

func TestPromoteDatadogAgentExperiment_Success_Promoted(t *testing.T) {
	// Already promoted — GET guard skips the patch, swaps stable ← experiment
	// (matching the normal success path so daemon restarts are handled correctly).
	d, _ := testDaemon(testDDAObject(v2alpha1.ExperimentPhasePromoted), testInstallerConfigWithDDA())
	rc := &mockRCClient{state: []*pbgo.PackageState{
		{Package: "datadog-operator", StableConfigVersion: "stable-1", ExperimentConfigVersion: "exp-1"},
	}}
	d.rcClient = rc
	op, err := d.promoteDatadogAgentExperiment(context.Background(), testPromoteRequest())
	requireSyncNoError(t, op, err)
	assert.Equal(t, "exp-1", rc.state[0].StableConfigVersion, "stable should be promoted to experiment version")
	assert.Empty(t, rc.state[0].ExperimentConfigVersion, "experiment config version should be cleared")
}

func TestPromoteDatadogAgentExperiment_PromotedDifferentID_Error(t *testing.T) {
	dda := testDDAObject(v2alpha1.ExperimentPhasePromoted)
	dda.Status.Experiment.ID = "old-exp"
	d, _ := testDaemon(dda, testInstallerConfigWithDDA())
	d.rcClient = &mockRCClient{state: []*pbgo.PackageState{
		{Package: "datadog-operator", StableConfigVersion: "stable-1", ExperimentConfigVersion: "exp-1"},
	}}

	err := syncTaskErr(d.promoteDatadogAgentExperiment(context.Background(), testPromoteRequest()))

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot promote")
}

// --- verifyExpectedState tests ---

func TestVerifyExpectedState_Match(t *testing.T) {
	d, rc := testDaemonWithRC([]*pbgo.PackageState{
		{Package: "datadog-operator", StableConfigVersion: "1.0.0", ExperimentConfigVersion: "2.0.0"},
	})
	d.rcClient = rc
	req := remoteAPIRequest{
		Package:       "datadog-operator",
		ExpectedState: expectedState{StableConfig: "1.0.0", ExperimentConfig: "2.0.0"},
	}
	assert.NoError(t, d.verifyExpectedState(req))
}

func TestVerifyExpectedState_StableMismatch(t *testing.T) {
	d, rc := testDaemonWithRC([]*pbgo.PackageState{
		{Package: "datadog-operator", StableConfigVersion: "1.0.0", ExperimentConfigVersion: ""},
	})
	d.rcClient = rc
	req := remoteAPIRequest{
		Package:       "datadog-operator",
		ExpectedState: expectedState{StableConfig: "wrong", ExperimentConfig: ""},
	}
	err := d.verifyExpectedState(req)
	require.Error(t, err)
	var stateErr *stateDoesntMatchError
	assert.True(t, errors.As(err, &stateErr))
}

func TestVerifyExpectedState_ExperimentMismatch(t *testing.T) {
	d, rc := testDaemonWithRC([]*pbgo.PackageState{
		{Package: "datadog-operator", StableConfigVersion: "1.0.0", ExperimentConfigVersion: "2.0.0"},
	})
	d.rcClient = rc
	req := remoteAPIRequest{
		Package:       "datadog-operator",
		ExpectedState: expectedState{StableConfig: "1.0.0", ExperimentConfig: "wrong"},
	}
	err := d.verifyExpectedState(req)
	require.Error(t, err)
	var stateErr *stateDoesntMatchError
	assert.True(t, errors.As(err, &stateErr))
}

func TestVerifyExpectedState_NilClient(t *testing.T) {
	// When rcClient is nil, getPackageConfigVersions returns ("", "").
	// A request with empty expected state should pass.
	d := &Daemon{}
	req := remoteAPIRequest{
		Package:       "datadog-operator",
		ExpectedState: expectedState{StableConfig: "", ExperimentConfig: ""},
	}
	assert.NoError(t, d.verifyExpectedState(req))
}

func TestHandleRemoteAPIRequest_InvalidState(t *testing.T) {
	d, _ := testDaemon(testDDAObject(""), testInstallerConfigWithDDA())
	d.rcClient = &mockRCClient{state: []*pbgo.PackageState{
		{Package: "datadog-operator", StableConfigVersion: "1.0.0", ExperimentConfigVersion: ""},
	}}
	req := testStartRequest()
	req.ExpectedState = expectedState{StableConfig: "stale", ExperimentConfig: ""}
	_, err := d.handleRemoteAPIRequest(context.Background(), req)
	require.Error(t, err)
	var stateErr *stateDoesntMatchError
	assert.True(t, errors.As(err, &stateErr))
}

// --- validateParams tests ---

func TestValidateParams_Valid(t *testing.T) {
	p := experimentParams{
		Version:          "test-config",
		GroupVersionKind: testDDAGVK,
		NamespacedName:   testDDANSN,
	}
	assert.NoError(t, validateParams(p))
}

func TestValidateParams_EmptyName(t *testing.T) {
	p := experimentParams{
		GroupVersionKind: testDDAGVK,
		NamespacedName:   types.NamespacedName{Namespace: "datadog", Name: ""},
	}
	assert.Error(t, validateParams(p))
}

func TestValidateParams_EmptyNamespace(t *testing.T) {
	p := experimentParams{
		GroupVersionKind: testDDAGVK,
		NamespacedName:   types.NamespacedName{Namespace: "", Name: "datadog-agent"},
	}
	assert.Error(t, validateParams(p))
}

func TestValidateParams_EmptyGroup_Allowed(t *testing.T) {
	// Group is auto-detected from the cluster; empty group is valid input.
	p := experimentParams{
		GroupVersionKind: schema.GroupVersionKind{Group: "", Version: "", Kind: "DatadogAgent"},
		NamespacedName:   testDDANSN,
	}
	assert.NoError(t, validateParams(p))
}

func TestValidateParams_WrongKind(t *testing.T) {
	p := experimentParams{
		GroupVersionKind: schema.GroupVersionKind{Group: "datadoghq.com", Version: "v2alpha1", Kind: "DatadogMonitor"},
		NamespacedName:   testDDANSN,
	}
	assert.Error(t, validateParams(p))
}

// --- buildSignalPatch tests ---

func TestBuildSignalPatch_WithConfig(t *testing.T) {
	config := json.RawMessage(`{"spec":{"features":{"apm":{"enabled":true}}}}`)
	patch, err := buildSignalPatch(v2alpha1.ExperimentSignalStart, "exp-123", config)
	require.NoError(t, err)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(patch, &result))

	// Check annotations are present.
	metadata := result["metadata"].(map[string]interface{})
	annotations := metadata["annotations"].(map[string]interface{})
	assert.Equal(t, "exp-123", annotations[v2alpha1.AnnotationExperimentID])
	assert.Equal(t, v2alpha1.ExperimentSignalStart, annotations[v2alpha1.AnnotationExperimentSignal])

	// Check spec is merged at top level.
	spec := result["spec"].(map[string]interface{})
	features := spec["features"].(map[string]interface{})
	apm := features["apm"].(map[string]interface{})
	assert.Equal(t, true, apm["enabled"])
}

func TestBuildSignalPatch_InvalidConfig(t *testing.T) {
	_, err := buildSignalPatch(v2alpha1.ExperimentSignalStart, "exp-123", json.RawMessage(`not-json`))
	assert.Error(t, err)
}

func TestBuildSignalPatch_WithoutConfig(t *testing.T) {
	patch, err := buildSignalPatch(v2alpha1.ExperimentSignalRollback, "exp-123")
	require.NoError(t, err)
	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(patch, &result))

	metadata := result["metadata"].(map[string]interface{})
	annotations := metadata["annotations"].(map[string]interface{})
	assert.Equal(t, v2alpha1.ExperimentSignalRollback, annotations[v2alpha1.AnnotationExperimentSignal])
	assert.Equal(t, "exp-123", annotations[v2alpha1.AnnotationExperimentID])
}

// --- Start idempotency with annotations already applied ---

func TestStartDatadogAgentExperiment_Idempotent_AnnotationAlreadyApplied(t *testing.T) {
	dda := testDDAObject("")
	dda.Annotations = map[string]string{
		v2alpha1.AnnotationExperimentID:     testExperimentID,
		v2alpha1.AnnotationExperimentSignal: v2alpha1.ExperimentSignalStart,
	}
	d, c := testDaemon(dda, testInstallerConfigWithDDA())
	req := testStartRequest()
	op, err := d.startDatadogAgentExperiment(context.Background(), req)
	requirePendingNoError(t, op, err)

	got := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), testDDANSN, got))
	assert.Equal(t, req.ID, got.Annotations[v2alpha1.AnnotationPendingTaskID])
	assert.Equal(t, string(pendingIntentStart), got.Annotations[v2alpha1.AnnotationPendingAction])
	assert.Equal(t, testExperimentID, got.Annotations[v2alpha1.AnnotationPendingExperimentID])
}

// --- Start with running experiment and same ID (controller already processed) ---

func TestStartDatadogAgentExperiment_Idempotent_ControllerAlreadyProcessed(t *testing.T) {
	dda := testDDAObject(v2alpha1.ExperimentPhaseRunning)
	dda.Status.Experiment.ID = testExperimentID // controller already set this
	d, _ := testDaemon(dda, testInstallerConfigWithDDA())
	req := testStartRequest()
	op, err := d.startDatadogAgentExperiment(context.Background(), req)
	requireSyncNoError(t, op, err)
}

// --- Stop idempotency with annotation already applied ---

func TestStopDatadogAgentExperiment_Idempotent_AnnotationAlreadyApplied(t *testing.T) {
	dda := testDDAObject(v2alpha1.ExperimentPhaseRunning)
	dda.Annotations[v2alpha1.AnnotationExperimentSignal] = v2alpha1.ExperimentSignalRollback
	d, _ := testDaemon(dda, testInstallerConfigWithDDA())
	op, err := d.stopDatadogAgentExperiment(context.Background(), testStopRequest())
	requirePendingNoError(t, op, err)
}

// --- Promote idempotency with annotation already applied ---

func TestPromoteDatadogAgentExperiment_Idempotent_AnnotationAlreadyApplied(t *testing.T) {
	dda := testDDAObject(v2alpha1.ExperimentPhaseRunning)
	dda.Annotations[v2alpha1.AnnotationExperimentSignal] = v2alpha1.ExperimentSignalPromote
	d, _ := testDaemon(dda, testInstallerConfigWithDDA())
	d.rcClient = &mockRCClient{state: []*pbgo.PackageState{
		{Package: "datadog-operator", StableConfigVersion: "stable-1", ExperimentConfigVersion: "exp-1"},
	}}
	op, err := d.promoteDatadogAgentExperiment(context.Background(), testPromoteRequest())
	requirePendingNoError(t, op, err)
}

func TestPromoteDatadogAgentExperiment_RejectsExistingPendingOperation(t *testing.T) {
	dda := testDDAObject(v2alpha1.ExperimentPhaseRunning)
	dda.Annotations[v2alpha1.AnnotationPendingTaskID] = "start-task"
	dda.Annotations[v2alpha1.AnnotationPendingAction] = string(pendingIntentStart)
	dda.Annotations[v2alpha1.AnnotationPendingExperimentID] = testExperimentID
	dda.Annotations[v2alpha1.AnnotationPendingPackage] = "datadog-operator"
	d, _ := testDaemon(dda, testInstallerConfigWithDDA())
	d.rcClient = &mockRCClient{state: []*pbgo.PackageState{
		{Package: "datadog-operator", StableConfigVersion: "stable-1", ExperimentConfigVersion: "exp-1"},
	}}

	err := syncTaskErr(d.promoteDatadogAgentExperiment(context.Background(), testPromoteRequest()))

	var stateErr *stateDoesntMatchError
	require.ErrorAs(t, err, &stateErr)
	assert.Contains(t, err.Error(), "already exists")
}

// --- Fix 2: stop/promote non-running guard tests ---

func TestPromoteDatadogAgentExperiment_Aborted_Error(t *testing.T) {
	// Aborted phase — non-running guard returns error.
	d, _ := testDaemon(testDDAObject(v2alpha1.ExperimentPhaseAborted), testInstallerConfigWithDDA())
	d.rcClient = &mockRCClient{state: []*pbgo.PackageState{
		{Package: "datadog-operator", StableConfigVersion: "stable-1", ExperimentConfigVersion: "exp-1"},
	}}
	err := syncTaskErr(d.promoteDatadogAgentExperiment(context.Background(), testPromoteRequest()))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot promote")
	assert.Contains(t, err.Error(), "aborted")
}

// --- Fix 3: RC state timing tests ---

func TestStartDatadogAgentExperiment_ConfigVersionSetAfterSuccess(t *testing.T) {
	// Verify config version is set only after successful completion.
	d, _ := testDaemon(testDDAObject(""), testInstallerConfigWithDDA())
	rc := &mockRCClient{state: []*pbgo.PackageState{
		{Package: "datadog-operator", StableConfigVersion: "stable-1"},
	}}
	d.rcClient = rc
	req := testStartRequest()
	op := requireStartQueued(t, d, req)
	require.Empty(t, rc.state[0].ExperimentConfigVersion)
	requireOperationSuccess(t, d, op)

	assert.Equal(t, req.Params.Version, rc.state[0].ExperimentConfigVersion)
}

func TestStopDatadogAgentExperiment_ConfigVersionClearedAfterSuccess(t *testing.T) {
	// Stop clears config version only after successful completion.
	d, _ := testDaemon(testDDAObject(v2alpha1.ExperimentPhaseRunning), testInstallerConfigWithDDA())
	rc := &mockRCClient{state: []*pbgo.PackageState{
		{Package: "datadog-operator", StableConfigVersion: "stable-1", ExperimentConfigVersion: "exp-1"},
	}}
	d.rcClient = rc
	op := requireStopQueued(t, d, testStopRequest())
	assert.Equal(t, "exp-1", rc.state[0].ExperimentConfigVersion)
	requireOperationSuccess(t, d, op)

	assert.Equal(t, "stable-1", rc.state[0].StableConfigVersion)
	assert.Empty(t, rc.state[0].ExperimentConfigVersion)
}

func TestPromoteDatadogAgentExperiment_ConfigVersionUpdatedAfterSuccess(t *testing.T) {
	// Promote updates config versions only after successful completion.
	d, _ := testDaemon(testDDAObject(v2alpha1.ExperimentPhaseRunning), testInstallerConfigWithDDA())
	rc := &mockRCClient{state: []*pbgo.PackageState{
		{Package: "datadog-operator", StableConfigVersion: "stable-1", ExperimentConfigVersion: "exp-1"},
	}}
	d.rcClient = rc
	op := requirePromoteQueued(t, d, testPromoteRequest())
	assert.Equal(t, "stable-1", rc.state[0].StableConfigVersion)
	assert.Equal(t, "exp-1", rc.state[0].ExperimentConfigVersion)
	requireOperationSuccess(t, d, op)

	assert.Equal(t, "exp-1", rc.state[0].StableConfigVersion)
	assert.Empty(t, rc.state[0].ExperimentConfigVersion)
}

// --- mockRCClient ---

// mockRCClient is a minimal RCClient used to test package state updates.
type mockRCClient struct {
	state       []*pbgo.PackageState
	taskHistory []*pbgo.PackageStateTask
}

func (m *mockRCClient) Subscribe(_ string, _ func(map[string]state.RawConfig, func(string, state.ApplyStatus))) {
}

func (m *mockRCClient) GetInstallerState() []*pbgo.PackageState {
	return m.state
}

func (m *mockRCClient) SetInstallerState(packages []*pbgo.PackageState) {
	m.state = packages
	for _, pkg := range packages {
		if pkg.GetTask() == nil {
			continue
		}
		m.taskHistory = append(m.taskHistory, proto.Clone(pkg.GetTask()).(*pbgo.PackageStateTask))
	}
}

func testDaemonWithRC(initialState []*pbgo.PackageState) (*Daemon, *mockRCClient) {
	rc := &mockRCClient{state: initialState}
	d := &Daemon{
		rcClient:      rc,
		configs:       make(map[string]installerConfig),
		statusUpdates: make(chan ddaStatusSnapshot, 32),
	}
	return d, rc
}

// --- setTaskState tests ---

func TestSetTaskState_Running(t *testing.T) {
	d, rc := testDaemonWithRC([]*pbgo.PackageState{
		{Package: "datadog-operator", StableVersion: "1.0.0"},
	})
	d.setTaskState("datadog-operator", "task-1", pbgo.TaskState_RUNNING, nil)

	require.Len(t, rc.state, 1)
	pkg := rc.state[0]
	assert.Equal(t, "datadog-operator", pkg.Package)
	assert.Equal(t, "1.0.0", pkg.StableVersion)
	require.NotNil(t, pkg.Task)
	assert.Equal(t, "task-1", pkg.Task.Id)
	assert.Equal(t, pbgo.TaskState_RUNNING, pkg.Task.State)
	assert.Nil(t, pkg.Task.Error)
}

func TestSetTaskState_Done(t *testing.T) {
	d, rc := testDaemonWithRC([]*pbgo.PackageState{
		{Package: "datadog-operator", StableVersion: "1.0.0"},
	})
	d.setTaskState("datadog-operator", "task-1", pbgo.TaskState_DONE, nil)

	require.Len(t, rc.state, 1)
	require.NotNil(t, rc.state[0].Task)
	assert.Equal(t, pbgo.TaskState_DONE, rc.state[0].Task.State)
	assert.Nil(t, rc.state[0].Task.Error)
}

func TestSetTaskState_Error(t *testing.T) {
	d, rc := testDaemonWithRC([]*pbgo.PackageState{
		{Package: "datadog-operator", StableVersion: "1.0.0"},
	})
	d.setTaskState("datadog-operator", "task-1", pbgo.TaskState_ERROR, errors.New("something went wrong"))

	require.Len(t, rc.state, 1)
	require.NotNil(t, rc.state[0].Task)
	assert.Equal(t, pbgo.TaskState_ERROR, rc.state[0].Task.State)
	require.NotNil(t, rc.state[0].Task.Error)
	assert.Equal(t, "something went wrong", rc.state[0].Task.Error.Message)
}

func TestSetTaskState_PackageNotInState(t *testing.T) {
	d, rc := testDaemonWithRC([]*pbgo.PackageState{})
	d.setTaskState("datadog-operator", "task-1", pbgo.TaskState_RUNNING, nil)

	require.Len(t, rc.state, 1)
	assert.Equal(t, "datadog-operator", rc.state[0].Package)
	require.NotNil(t, rc.state[0].Task)
	assert.Equal(t, pbgo.TaskState_RUNNING, rc.state[0].Task.State)
}

func TestSetTaskState_PreservesOtherPackages(t *testing.T) {
	d, rc := testDaemonWithRC([]*pbgo.PackageState{
		{Package: "other-package", StableVersion: "2.0.0"},
		{Package: "datadog-operator", StableVersion: "1.0.0"},
	})
	d.setTaskState("datadog-operator", "task-1", pbgo.TaskState_DONE, nil)

	require.Len(t, rc.state, 2)
	// other-package must be unchanged
	assert.Equal(t, "other-package", rc.state[0].Package)
	assert.Nil(t, rc.state[0].Task)
	// datadog-operator has task set
	assert.Equal(t, "datadog-operator", rc.state[1].Package)
	require.NotNil(t, rc.state[1].Task)
	assert.Equal(t, pbgo.TaskState_DONE, rc.state[1].Task.State)
}

func TestSetTaskState_NilClient(t *testing.T) {
	d := &Daemon{}
	// Must not panic when rcClient is nil.
	assert.NotPanics(t, func() {
		d.setTaskState("datadog-operator", "task-1", pbgo.TaskState_RUNNING, nil)
	})
}

func TestSetPackageConfigVersions_PreservesOtherPackageState(t *testing.T) {
	d, rc := testDaemonWithRC([]*pbgo.PackageState{
		{Package: "other-package", StableConfigVersion: "other-stable", ExperimentConfigVersion: "other-exp"},
		{
			Package:             "datadog-operator",
			StableVersion:       "1.0.0",
			ExperimentVersion:   "2.0.0",
			StableConfigVersion: "old-stable",
			Task:                &pbgo.PackageStateTask{Id: "task-1", State: pbgo.TaskState_RUNNING},
		},
	})

	d.setPackageConfigVersions("datadog-operator", "new-stable", "new-exp")

	require.Len(t, rc.state, 2)
	assert.Equal(t, "other-package", rc.state[0].Package)
	assert.Equal(t, "other-stable", rc.state[0].StableConfigVersion)
	assert.Equal(t, "other-exp", rc.state[0].ExperimentConfigVersion)
	assert.Equal(t, "datadog-operator", rc.state[1].Package)
	assert.Equal(t, "1.0.0", rc.state[1].StableVersion)
	assert.Equal(t, "2.0.0", rc.state[1].ExperimentVersion)
	assert.Equal(t, "new-stable", rc.state[1].StableConfigVersion)
	assert.Equal(t, "new-exp", rc.state[1].ExperimentConfigVersion)
	require.NotNil(t, rc.state[1].Task)
	assert.Equal(t, "task-1", rc.state[1].Task.Id)
	assert.Equal(t, pbgo.TaskState_RUNNING, rc.state[1].Task.State)
}

func TestDaemonStartRequiresCache(t *testing.T) {
	d := &Daemon{}
	err := d.Start(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "controller cache")
}

// --- handleTask state-transition tests ---

func testDaemonFull(dda *v2alpha1.DatadogAgent, configs map[string]installerConfig, rcState []*pbgo.PackageState) (*Daemon, client.Client, *mockRCClient) {
	d, c := testDaemon(dda, configs)
	rc := &mockRCClient{state: rcState}
	d.rcClient = rc
	return d, c, rc
}

func TestHandleTask_DispatchesWithoutSettingTaskState(t *testing.T) {
	rc := []*pbgo.PackageState{{Package: "datadog-operator", StableConfigVersion: "0.0.1"}}
	d, _, m := testDaemonFull(testDDAObject(""), testInstallerConfigWithDDA(), rc)
	req := testStartRequest()
	req.ExpectedState = expectedState{StableConfig: "0.0.1"}
	require.NoError(t, d.handleTask(context.Background(), req))
	assert.Nil(t, m.state[0].Task)
}

func TestHandleTask_Error(t *testing.T) {
	rc := []*pbgo.PackageState{{Package: "datadog-operator", StableConfigVersion: "0.0.1"}}
	d, _, m := testDaemonFull(testDDAObject(""), testInstallerConfigWithDDA(), rc)
	req := testStartRequest()
	req.Method = "unknown/method"
	req.ExpectedState = expectedState{StableConfig: "0.0.1"}
	err := d.handleTask(context.Background(), req)
	require.Error(t, err)
	require.NotNil(t, m.state[0].Task)
	assert.Equal(t, pbgo.TaskState_ERROR, m.state[0].Task.State)
	require.NotNil(t, m.state[0].Task.Error)
}

func TestHandleTask_InvalidState(t *testing.T) {
	rc := []*pbgo.PackageState{{Package: "datadog-operator", StableConfigVersion: "0.0.1"}}
	d, _, m := testDaemonFull(testDDAObject(""), testInstallerConfigWithDDA(), rc)
	req := testStartRequest()
	req.ExpectedState = expectedState{StableConfig: "wrong-version"}
	err := d.handleTask(context.Background(), req)
	require.Error(t, err)
	require.NotNil(t, m.state[0].Task)
	assert.Equal(t, pbgo.TaskState_INVALID_STATE, m.state[0].Task.State)
}

func TestReconcileTimedOutExperiment_ClearsExperimentConfigVersion(t *testing.T) {
	d, rc := testDaemonWithRC([]*pbgo.PackageState{
		{Package: "datadog-operator", StableConfigVersion: "stable-1", ExperimentConfigVersion: testExperimentID},
	})
	dda := testDDAObject(v2alpha1.ExperimentPhaseTerminated)
	dda.Status.Experiment.TerminationReason = "timed_out"

	d.reconcileTimedOutExperiment(context.Background(), newDDAStatusSnapshot(dda))

	require.Len(t, rc.state, 1)
	assert.Equal(t, "stable-1", rc.state[0].StableConfigVersion)
	assert.Empty(t, rc.state[0].ExperimentConfigVersion)
}

func TestReconcileTimedOutExperiment_IgnoresNonTimeoutTermination(t *testing.T) {
	d, rc := testDaemonWithRC([]*pbgo.PackageState{
		{Package: "datadog-operator", StableConfigVersion: "stable-1", ExperimentConfigVersion: testExperimentID},
	})
	dda := testDDAObject(v2alpha1.ExperimentPhaseTerminated)
	dda.Status.Experiment.TerminationReason = "stopped"

	d.reconcileTimedOutExperiment(context.Background(), newDDAStatusSnapshot(dda))

	require.Len(t, rc.state, 1)
	assert.Equal(t, "stable-1", rc.state[0].StableConfigVersion)
	assert.Equal(t, testExperimentID, rc.state[0].ExperimentConfigVersion)
}

func TestRunPendingOperationWorker_UsesCurrentPendingAnnotations(t *testing.T) {
	dda := testDDAObject("")
	dda.Annotations = map[string]string{
		v2alpha1.AnnotationPendingTaskID:       "task-1",
		v2alpha1.AnnotationPendingAction:       string(pendingIntentStart),
		v2alpha1.AnnotationPendingExperimentID: "exp-1",
		v2alpha1.AnnotationPendingPackage:      "datadog-operator",
	}
	d, c := testDaemon(dda, testInstallerConfigWithDDA())
	d.rcClient = &mockRCClient{state: []*pbgo.PackageState{
		{Package: "datadog-operator", StableConfigVersion: "stable-1"},
	}}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tracker := newOperationTracker(d)
	tracker.onStatusUpdate(ctx, newDDAStatusSnapshot(dda))

	current := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(ctx, testDDANSN, current))
	current.Annotations[v2alpha1.AnnotationPendingTaskID] = "task-2"
	current.Annotations[v2alpha1.AnnotationPendingAction] = string(pendingIntentStop)
	current.Annotations[v2alpha1.AnnotationPendingExperimentID] = "exp-2"
	require.NoError(t, c.Update(ctx, current))
	tracker.onStatusUpdate(ctx, newDDAStatusSnapshot(current))

	rc := d.rcClient.(*mockRCClient)
	require.NotNil(t, rc.state[0].Task)
	assert.Equal(t, "task-2", rc.state[0].Task.Id)
	assert.Equal(t, pbgo.TaskState_RUNNING, rc.state[0].Task.State)
	for _, task := range rc.taskHistory {
		if task.Id == "task-1" {
			assert.NotEqual(t, pbgo.TaskState_INVALID_STATE, task.State)
		}
	}
}

func TestRunPendingOperationWorker_SetsRunningFromPendingAnnotations(t *testing.T) {
	dda := testDDAObject("")
	dda.Annotations = map[string]string{
		v2alpha1.AnnotationPendingTaskID:       "task-1",
		v2alpha1.AnnotationPendingAction:       string(pendingIntentStart),
		v2alpha1.AnnotationPendingExperimentID: testExperimentID,
		v2alpha1.AnnotationPendingPackage:      "datadog-operator",
	}
	d, _ := testDaemon(dda, testInstallerConfigWithDDA())
	d.rcClient = &mockRCClient{state: []*pbgo.PackageState{
		{Package: "datadog-operator", StableConfigVersion: "stable-1"},
	}}

	tracker := newOperationTracker(d)
	tracker.onStatusUpdate(context.Background(), newDDAStatusSnapshot(dda))

	rc := d.rcClient.(*mockRCClient)
	require.NotNil(t, rc.state[0].Task)
	assert.Equal(t, "task-1", rc.state[0].Task.Id)
	assert.Equal(t, pbgo.TaskState_RUNNING, rc.state[0].Task.State)
}

func TestRunPendingOperationWorker_IgnoresStatusForDifferentExperiment(t *testing.T) {
	dda := testDDAObject(v2alpha1.ExperimentPhaseRunning)
	dda.Status.Experiment.ID = "old-exp"
	dda.Annotations[v2alpha1.AnnotationPendingTaskID] = "task-1"
	dda.Annotations[v2alpha1.AnnotationPendingAction] = string(pendingIntentStart)
	dda.Annotations[v2alpha1.AnnotationPendingExperimentID] = testExperimentID
	dda.Annotations[v2alpha1.AnnotationPendingPackage] = "datadog-operator"

	d, c := testDaemon(dda, testInstallerConfigWithDDA())
	d.rcClient = &mockRCClient{state: []*pbgo.PackageState{
		{Package: "datadog-operator", StableConfigVersion: "stable-1"},
	}}

	tracker := newOperationTracker(d)
	tracker.onStatusUpdate(context.Background(), newDDAStatusSnapshot(dda))

	rc := d.rcClient.(*mockRCClient)
	require.NotNil(t, rc.state[0].Task)
	assert.Equal(t, "task-1", rc.state[0].Task.Id)
	assert.Equal(t, pbgo.TaskState_RUNNING, rc.state[0].Task.State)
	assert.Empty(t, rc.state[0].ExperimentConfigVersion)

	got := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), testDDANSN, got))
	assert.Equal(t, "task-1", got.Annotations[v2alpha1.AnnotationPendingTaskID])
}

func TestRunPendingOperationWorker_StartTerminalPhaseSetsError(t *testing.T) {
	dda := testDDAObject(v2alpha1.ExperimentPhaseTerminated)
	dda.Annotations[v2alpha1.AnnotationPendingTaskID] = "task-1"
	dda.Annotations[v2alpha1.AnnotationPendingAction] = string(pendingIntentStart)
	dda.Annotations[v2alpha1.AnnotationPendingExperimentID] = testExperimentID
	dda.Annotations[v2alpha1.AnnotationPendingPackage] = "datadog-operator"

	d, c := testDaemon(dda, testInstallerConfigWithDDA())
	d.rcClient = &mockRCClient{state: []*pbgo.PackageState{
		{Package: "datadog-operator", StableConfigVersion: "stable-1"},
	}}

	tracker := newOperationTracker(d)
	tracker.onStatusUpdate(context.Background(), newDDAStatusSnapshot(dda))

	rc := d.rcClient.(*mockRCClient)
	require.NotNil(t, rc.state[0].Task)
	assert.Equal(t, "task-1", rc.state[0].Task.Id)
	assert.Equal(t, pbgo.TaskState_ERROR, rc.state[0].Task.State)
	require.NotNil(t, rc.state[0].Task.Error)
	assert.Contains(t, rc.state[0].Task.Error.Message, "terminal phase")

	got := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), testDDANSN, got))
	assert.Empty(t, got.Annotations[v2alpha1.AnnotationPendingTaskID])
}

func TestRunPendingOperationWorker_CompletesStatusUpdateForNonDefaultPackage(t *testing.T) {
	dda := testDDAObject(v2alpha1.ExperimentPhaseRunning)
	dda.Status.Agent = testCompletedAgentStatus()
	dda.Annotations[v2alpha1.AnnotationPendingTaskID] = "task-1"
	dda.Annotations[v2alpha1.AnnotationPendingAction] = string(pendingIntentStart)
	dda.Annotations[v2alpha1.AnnotationPendingExperimentID] = testExperimentID
	dda.Annotations[v2alpha1.AnnotationPendingPackage] = "other-package"

	d, _ := testDaemon(dda, testInstallerConfigWithDDA())
	d.rcClient = &mockRCClient{state: []*pbgo.PackageState{
		{Package: "other-package", StableConfigVersion: "stable-1"},
	}}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tracker := newOperationTracker(d)
	tracker.onStatusUpdate(ctx, newDDAStatusSnapshot(dda))

	rc := d.rcClient.(*mockRCClient)
	require.NotNil(t, rc.state[0].Task)
	assert.Equal(t, "task-1", rc.state[0].Task.Id)
	assert.Equal(t, pbgo.TaskState_DONE, rc.state[0].Task.State)
	assert.Equal(t, testExperimentID, rc.state[0].ExperimentConfigVersion)
}

func TestRunPendingOperationWorker_RecoversStopFromAnnotations(t *testing.T) {
	dda := testDDAObject(v2alpha1.ExperimentPhaseTerminated)
	dda.Annotations[v2alpha1.AnnotationPendingTaskID] = "task-1"
	dda.Annotations[v2alpha1.AnnotationPendingAction] = string(pendingIntentStop)
	dda.Annotations[v2alpha1.AnnotationPendingExperimentID] = testExperimentID
	dda.Annotations[v2alpha1.AnnotationPendingPackage] = "datadog-operator"

	d, c := testDaemon(dda, testInstallerConfigWithDDA())
	d.rcClient = &mockRCClient{state: []*pbgo.PackageState{
		{Package: "datadog-operator", StableConfigVersion: "stable-1", ExperimentConfigVersion: testExperimentID},
	}}

	tracker := newOperationTracker(d)
	tracker.onStatusUpdate(context.Background(), newDDAStatusSnapshot(dda))

	rc := d.rcClient.(*mockRCClient)
	require.NotNil(t, rc.state[0].Task)
	assert.Equal(t, "task-1", rc.state[0].Task.Id)
	assert.Equal(t, pbgo.TaskState_DONE, rc.state[0].Task.State)
	assert.Equal(t, "stable-1", rc.state[0].StableConfigVersion)
	assert.Empty(t, rc.state[0].ExperimentConfigVersion)

	got := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), testDDANSN, got))
	assert.Empty(t, got.Annotations[v2alpha1.AnnotationPendingTaskID])
	assert.Empty(t, got.Annotations[v2alpha1.AnnotationPendingAction])
	assert.Empty(t, got.Annotations[v2alpha1.AnnotationPendingExperimentID])
	assert.Empty(t, got.Annotations[v2alpha1.AnnotationPendingPackage])
}

func TestRunPendingOperationWorker_IgnoresIncompletePendingAnnotations(t *testing.T) {
	dda := testDDAObject(v2alpha1.ExperimentPhaseRunning)
	dda.Annotations[v2alpha1.AnnotationPendingTaskID] = "task-1"

	d, _ := testDaemon(dda, testInstallerConfigWithDDA())
	d.rcClient = &mockRCClient{state: []*pbgo.PackageState{
		{Package: "datadog-operator", StableConfigVersion: "stable-1"},
	}}

	tracker := newOperationTracker(d)
	tracker.onStatusUpdate(context.Background(), newDDAStatusSnapshot(dda))

	rc := d.rcClient.(*mockRCClient)
	assert.Nil(t, rc.state[0].Task)
	assert.Empty(t, rc.state[0].ExperimentConfigVersion)
}

func TestFinishPendingOperation_ClearsPendingAnnotations(t *testing.T) {
	d, c := testDaemon(testDDAObject(""), testInstallerConfigWithDDA())
	req := testStartRequest()
	op := requireStartQueued(t, d, req)

	d.finishPendingOperation(context.Background(), *op, nil)

	dda := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), testDDANSN, dda))
	assert.Empty(t, dda.Annotations[v2alpha1.AnnotationPendingTaskID])
	assert.Empty(t, dda.Annotations[v2alpha1.AnnotationPendingAction])
	assert.Empty(t, dda.Annotations[v2alpha1.AnnotationPendingExperimentID])
	assert.Empty(t, dda.Annotations[v2alpha1.AnnotationPendingPackage])
}

func TestClearPendingAnnotationsIfCurrent_PreservesOtherAnnotations(t *testing.T) {
	dda := testDDAObject("")
	dda.Annotations = map[string]string{
		v2alpha1.AnnotationPendingTaskID:       "task-1",
		v2alpha1.AnnotationPendingAction:       string(pendingIntentStart),
		v2alpha1.AnnotationPendingExperimentID: testExperimentID,
		v2alpha1.AnnotationPendingPackage:      "datadog-operator",
		v2alpha1.AnnotationExperimentSignal:    v2alpha1.ExperimentSignalStart,
		v2alpha1.AnnotationExperimentID:        testExperimentID,
		"example.com/keep":                     "keep-me",
	}
	d, c := testDaemon(dda, testInstallerConfigWithDDA())
	task := pendingOperation{
		intent:       pendingIntentStart,
		taskID:       "task-1",
		packageName:  "datadog-operator",
		nsn:          testDDANSN,
		experimentID: testExperimentID,
	}

	require.NoError(t, d.clearPendingAnnotationsIfCurrent(context.Background(), task))

	got := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), testDDANSN, got))
	assert.Empty(t, got.Annotations[v2alpha1.AnnotationPendingTaskID])
	assert.Empty(t, got.Annotations[v2alpha1.AnnotationPendingAction])
	assert.Empty(t, got.Annotations[v2alpha1.AnnotationPendingExperimentID])
	assert.Empty(t, got.Annotations[v2alpha1.AnnotationPendingPackage])
	assert.Equal(t, v2alpha1.ExperimentSignalStart, got.Annotations[v2alpha1.AnnotationExperimentSignal])
	assert.Equal(t, testExperimentID, got.Annotations[v2alpha1.AnnotationExperimentID])
	assert.Equal(t, "keep-me", got.Annotations["example.com/keep"])
}

func TestClearPendingAnnotationsIfCurrent_DoesNotClearNewerPendingTask(t *testing.T) {
	dda := testDDAObject("")
	dda.Annotations = map[string]string{
		v2alpha1.AnnotationPendingTaskID:       "task-2",
		v2alpha1.AnnotationPendingAction:       string(pendingIntentStop),
		v2alpha1.AnnotationPendingExperimentID: testExperimentID,
		v2alpha1.AnnotationPendingPackage:      "datadog-operator",
		"example.com/keep":                     "keep-me",
	}
	d, c := testDaemon(dda, testInstallerConfigWithDDA())
	oldTask := pendingOperation{
		intent:       pendingIntentStart,
		taskID:       "task-1",
		packageName:  "datadog-operator",
		nsn:          testDDANSN,
		experimentID: testExperimentID,
	}

	require.NoError(t, d.clearPendingAnnotationsIfCurrent(context.Background(), oldTask))

	got := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), testDDANSN, got))
	assert.Equal(t, "task-2", got.Annotations[v2alpha1.AnnotationPendingTaskID])
	assert.Equal(t, string(pendingIntentStop), got.Annotations[v2alpha1.AnnotationPendingAction])
	assert.Equal(t, testExperimentID, got.Annotations[v2alpha1.AnnotationPendingExperimentID])
	assert.Equal(t, "datadog-operator", got.Annotations[v2alpha1.AnnotationPendingPackage])
	assert.Equal(t, "keep-me", got.Annotations["example.com/keep"])
}

func TestStartDatadogAgentExperiment_OverwritesStalePendingResultVersion(t *testing.T) {
	dda := testDDAObject("")
	if dda.Annotations == nil {
		dda.Annotations = map[string]string{}
	}
	dda.Annotations[v2alpha1.AnnotationPendingResultVersion] = "stale-promote-version"

	d, c := testDaemon(dda, testInstallerConfigWithDDA())
	req := testStartRequest()
	op := requireStartQueued(t, d, req)

	got := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), testDDANSN, got))
	assert.Empty(t, got.Annotations[v2alpha1.AnnotationPendingResultVersion])

	d.finishPendingOperation(context.Background(), *op, nil)

	require.NoError(t, c.Get(context.Background(), testDDANSN, got))
	assert.Empty(t, got.Annotations[v2alpha1.AnnotationPendingResultVersion])
}

func TestRunPendingOperationWorker_RecoversPendingOperationFromAnnotations(t *testing.T) {
	dda := testDDAObject(v2alpha1.ExperimentPhaseRunning)
	dda.Status.Agent = testCompletedAgentStatus()
	dda.Annotations[v2alpha1.AnnotationPendingTaskID] = "task-1"
	dda.Annotations[v2alpha1.AnnotationPendingAction] = string(pendingIntentStart)
	dda.Annotations[v2alpha1.AnnotationPendingExperimentID] = testExperimentID
	dda.Annotations[v2alpha1.AnnotationPendingPackage] = "datadog-operator"

	d, c := testDaemon(dda, testInstallerConfigWithDDA())
	d.rcClient = &mockRCClient{state: []*pbgo.PackageState{
		{Package: "datadog-operator", StableConfigVersion: "stable-1"},
	}}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tracker := newOperationTracker(d)
	tracker.onStatusUpdate(ctx, newDDAStatusSnapshot(dda))

	rc := d.rcClient.(*mockRCClient)
	require.NotNil(t, rc.state[0].Task)
	assert.Equal(t, "task-1", rc.state[0].Task.Id)
	assert.Equal(t, pbgo.TaskState_DONE, rc.state[0].Task.State)
	assert.Equal(t, testExperimentID, rc.state[0].ExperimentConfigVersion)

	got := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), testDDANSN, got))
	assert.Empty(t, got.Annotations[v2alpha1.AnnotationPendingTaskID])
	assert.Empty(t, got.Annotations[v2alpha1.AnnotationPendingAction])
	assert.Empty(t, got.Annotations[v2alpha1.AnnotationPendingExperimentID])
	assert.Empty(t, got.Annotations[v2alpha1.AnnotationPendingPackage])
}

func TestRunPendingOperationWorker_RecoversPromoteResultVersionFromAnnotations(t *testing.T) {
	dda := testDDAObject(v2alpha1.ExperimentPhasePromoted)
	dda.Annotations[v2alpha1.AnnotationPendingTaskID] = "task-1"
	dda.Annotations[v2alpha1.AnnotationPendingAction] = string(pendingIntentPromote)
	dda.Annotations[v2alpha1.AnnotationPendingExperimentID] = testExperimentID
	dda.Annotations[v2alpha1.AnnotationPendingPackage] = "datadog-operator"
	dda.Annotations[v2alpha1.AnnotationPendingResultVersion] = "exp-1"

	d, c := testDaemon(dda, testInstallerConfigWithDDA())
	d.rcClient = &mockRCClient{state: []*pbgo.PackageState{
		{Package: "datadog-operator", StableConfigVersion: "stable-1", ExperimentConfigVersion: "exp-1"},
	}}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tracker := newOperationTracker(d)
	tracker.onStatusUpdate(ctx, newDDAStatusSnapshot(dda))

	rc := d.rcClient.(*mockRCClient)
	require.NotNil(t, rc.state[0].Task)
	assert.Equal(t, "task-1", rc.state[0].Task.Id)
	assert.Equal(t, pbgo.TaskState_DONE, rc.state[0].Task.State)
	assert.Equal(t, "exp-1", rc.state[0].StableConfigVersion)
	assert.Empty(t, rc.state[0].ExperimentConfigVersion)

	got := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), testDDANSN, got))
	assert.Empty(t, got.Annotations[v2alpha1.AnnotationPendingResultVersion])
}

// --- isDaemonSetRolloutComplete tests ---

func TestIsDaemonSetRolloutComplete(t *testing.T) {
	tests := []struct {
		name  string
		agent *v2alpha1.DaemonSetStatus
		want  bool
	}{
		{
			name:  "nil agent — rollout not yet reported",
			agent: nil,
			want:  false,
		},
		{
			name:  "zero desired — trivially complete",
			agent: &v2alpha1.DaemonSetStatus{Desired: 0},
			want:  true,
		},
		{
			name:  "rolling out — some pods not yet updated",
			agent: &v2alpha1.DaemonSetStatus{Desired: 3, UpToDate: 1, Ready: 1},
			want:  false,
		},
		{
			name:  "updated but none ready",
			agent: &v2alpha1.DaemonSetStatus{Desired: 3, UpToDate: 3, Ready: 0},
			want:  false,
		},
		{
			name:  "all updated and ready",
			agent: &v2alpha1.DaemonSetStatus{Desired: 3, UpToDate: 3, Ready: 3},
			want:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isDaemonSetRolloutComplete(tt.agent))
		})
	}
}

// --- Worker start gating tests ---

func TestRunPendingOperationWorker_StartWaitsForRollout_NilAgent(t *testing.T) {
	dda := testDDAObject(v2alpha1.ExperimentPhaseRunning)
	// status.Agent deliberately left nil — rollout not yet reflected in status
	dda.Annotations[v2alpha1.AnnotationPendingTaskID] = "task-1"
	dda.Annotations[v2alpha1.AnnotationPendingAction] = string(pendingIntentStart)
	dda.Annotations[v2alpha1.AnnotationPendingExperimentID] = testExperimentID
	dda.Annotations[v2alpha1.AnnotationPendingPackage] = "datadog-operator"

	d, _ := testDaemon(dda, testInstallerConfigWithDDA())
	d.rcClient = &mockRCClient{state: []*pbgo.PackageState{
		{Package: "datadog-operator", StableConfigVersion: "stable-1"},
	}}

	tracker := newOperationTracker(d)
	tracker.onStatusUpdate(context.Background(), newDDAStatusSnapshot(dda))

	rc := d.rcClient.(*mockRCClient)
	require.NotNil(t, rc.state[0].Task)
	assert.Equal(t, pbgo.TaskState_RUNNING, rc.state[0].Task.State)
}

func TestRunPendingOperationWorker_StartWaitsForRollout_PartialUpdate(t *testing.T) {
	dda := testDDAObject(v2alpha1.ExperimentPhaseRunning)
	dda.Status.Agent = &v2alpha1.DaemonSetStatus{Desired: 3, UpToDate: 1, Ready: 1}
	dda.Annotations[v2alpha1.AnnotationPendingTaskID] = "task-1"
	dda.Annotations[v2alpha1.AnnotationPendingAction] = string(pendingIntentStart)
	dda.Annotations[v2alpha1.AnnotationPendingExperimentID] = testExperimentID
	dda.Annotations[v2alpha1.AnnotationPendingPackage] = "datadog-operator"

	d, _ := testDaemon(dda, testInstallerConfigWithDDA())
	d.rcClient = &mockRCClient{state: []*pbgo.PackageState{
		{Package: "datadog-operator", StableConfigVersion: "stable-1"},
	}}

	tracker := newOperationTracker(d)
	tracker.onStatusUpdate(context.Background(), newDDAStatusSnapshot(dda))

	rc := d.rcClient.(*mockRCClient)
	require.NotNil(t, rc.state[0].Task)
	assert.Equal(t, pbgo.TaskState_RUNNING, rc.state[0].Task.State)
}

func TestRunPendingOperationWorker_StartDoneAfterRolloutComplete(t *testing.T) {
	dda := testDDAObject(v2alpha1.ExperimentPhaseRunning)
	dda.Status.Agent = testCompletedAgentStatus()
	dda.Annotations[v2alpha1.AnnotationPendingTaskID] = "task-1"
	dda.Annotations[v2alpha1.AnnotationPendingAction] = string(pendingIntentStart)
	dda.Annotations[v2alpha1.AnnotationPendingExperimentID] = testExperimentID
	dda.Annotations[v2alpha1.AnnotationPendingPackage] = "datadog-operator"

	d, c := testDaemon(dda, testInstallerConfigWithDDA())
	d.rcClient = &mockRCClient{state: []*pbgo.PackageState{
		{Package: "datadog-operator", StableConfigVersion: "stable-1"},
	}}

	tracker := newOperationTracker(d)
	tracker.onStatusUpdate(context.Background(), newDDAStatusSnapshot(dda))

	rc := d.rcClient.(*mockRCClient)
	require.NotNil(t, rc.state[0].Task)
	assert.Equal(t, pbgo.TaskState_DONE, rc.state[0].Task.State)
	assert.Equal(t, testExperimentID, rc.state[0].ExperimentConfigVersion)

	got := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), testDDANSN, got))
	assert.Empty(t, got.Annotations[v2alpha1.AnnotationPendingTaskID])
}
