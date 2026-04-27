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
	"time"

	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	}, c
}

var testDDAGVK = schema.GroupVersionKind{
	Group:   "datadoghq.com",
	Version: "v2alpha1",
	Kind:    "DatadogAgent",
}

var testDDANSN = types.NamespacedName{Namespace: "datadog", Name: "datadog-agent"}

func testDDAObject(phase v2alpha1.ExperimentPhase) *v2alpha1.DatadogAgent {
	dda := &v2alpha1.DatadogAgent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testDDANSN.Name,
			Namespace: testDDANSN.Namespace,
		},
	}
	if phase != "" {
		dda.Status.Experiment = &v2alpha1.ExperimentStatus{Phase: phase, ID: "old-exp"}
		// Set experiment annotations to match status — this is the state the daemon
		// would see after the controller has processed the start signal.
		if dda.Annotations == nil {
			dda.Annotations = make(map[string]string)
		}
		dda.Annotations[v2alpha1.AnnotationExperimentID] = "old-exp"
		dda.Annotations[v2alpha1.AnnotationExperimentSignal] = v2alpha1.ExperimentSignalStart
	}
	return dda
}

func testInstallerConfigWithDDA() map[string]installerConfig {
	return map[string]installerConfig{
		"test-config": {
			ID: "test-config",
			Operations: []fleetManagementOperation{
				{
					Operation:        OperationUpdate,
					GroupVersionKind: testDDAGVK,
					NamespacedName:   testDDANSN,
					Config:           json.RawMessage(`{}`),
				},
			},
		},
	}
}

func testStartRequest() remoteAPIRequest {
	return remoteAPIRequest{
		ID:      "exp-abc",
		Package: "datadog-operator",
		Method:  methodStartDatadogAgentExperiment,
		Params:  experimentParams{Version: "test-config"},
	}
}

// --- startDatadogAgentExperiment tests ---

func TestStartDatadogAgentExperiment_ConfigNotFound(t *testing.T) {
	d, _ := testDaemon(testDDAObject(""), testInstallerConfigWithDDA())
	req := testStartRequest()
	req.Params.Version = "nonexistent"
	assert.Error(t, d.startDatadogAgentExperiment(context.Background(), req))
}

func TestStartDatadogAgentExperiment_NoDDAOperation(t *testing.T) {
	configs := map[string]installerConfig{
		"no-dda-config": {
			ID: "no-dda-config",
			Operations: []fleetManagementOperation{
				{
					Operation:        OperationUpdate,
					GroupVersionKind: schema.GroupVersionKind{Group: "other.io", Version: "v1", Kind: "Other"},
					NamespacedName:   testDDANSN,
					Config:           json.RawMessage(`{}`),
				},
			},
		},
	}
	d, _ := testDaemon(testDDAObject(""), configs)
	req := testStartRequest()
	req.Params.Version = "no-dda-config"
	assert.Error(t, d.startDatadogAgentExperiment(context.Background(), req))
}

func TestStartDatadogAgentExperiment_DDANotFound(t *testing.T) {
	d, _ := testDaemon(nil, testInstallerConfigWithDDA()) // no DDA pre-created
	assert.Error(t, d.startDatadogAgentExperiment(context.Background(), testStartRequest()))
}

func TestStartDatadogAgentExperiment_Running_Idempotent(t *testing.T) {
	dda := testDDAObject(v2alpha1.ExperimentPhaseRunning)
	d, c := testDaemon(dda, testInstallerConfigWithDDA())
	// Same experiment ID retried — should be a no-op since it's already running.
	req := testStartRequest()
	req.ID = "old-exp"
	require.NoError(t, d.startDatadogAgentExperiment(context.Background(), req))

	// DDA should be unchanged — no re-patch, no status update.
	got := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), testDDANSN, got))
	require.NotNil(t, got.Status.Experiment)
	assert.Equal(t, v2alpha1.ExperimentPhaseRunning, got.Status.Experiment.Phase)
	assert.Equal(t, "old-exp", got.Status.Experiment.ID)
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
	req.ID = "old-exp"
	require.NoError(t, d.startDatadogAgentExperiment(context.Background(), req))

	require.Len(t, rc.state, 1)
	assert.Equal(t, req.Params.Version, rc.state[0].ExperimentConfigVersion,
		"idempotent start path should restore experiment config version")
}

func TestStartDatadogAgentExperiment_Running_DifferentID(t *testing.T) {
	dda := testDDAObject(v2alpha1.ExperimentPhaseRunning)
	d, _ := testDaemon(dda, testInstallerConfigWithDDA())
	// Different experiment ID while one is already running — should be rejected.
	req := testStartRequest()
	req.ID = "different-exp"
	assert.Error(t, d.startDatadogAgentExperiment(context.Background(), req))
}

func TestStartDatadogAgentExperiment_Success_NilPhase(t *testing.T) {
	d, c := testDaemon(testDDAObject(""), testInstallerConfigWithDDA())
	req := testStartRequest()
	require.NoError(t, d.startDatadogAgentExperiment(context.Background(), req))

	dda := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), testDDANSN, dda))
	// Daemon writes annotations, not status. Status is written by the controller.
	assert.Equal(t, req.ID, dda.Annotations[v2alpha1.AnnotationExperimentID])
	assert.Equal(t, v2alpha1.ExperimentSignalStart, dda.Annotations[v2alpha1.AnnotationExperimentSignal])
}

func TestStartDatadogAgentExperiment_Success_FromTerminated(t *testing.T) {
	d, c := testDaemon(testDDAObject(v2alpha1.ExperimentPhaseTerminated), testInstallerConfigWithDDA())
	req := testStartRequest()
	require.NoError(t, d.startDatadogAgentExperiment(context.Background(), req))

	dda := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), testDDANSN, dda))
	assert.Equal(t, req.ID, dda.Annotations[v2alpha1.AnnotationExperimentID])
	assert.Equal(t, v2alpha1.ExperimentSignalStart, dda.Annotations[v2alpha1.AnnotationExperimentSignal])
}

func TestStartDatadogAgentExperiment_Success_FromAborted(t *testing.T) {
	d, c := testDaemon(testDDAObject(v2alpha1.ExperimentPhaseAborted), testInstallerConfigWithDDA())
	req := testStartRequest()
	require.NoError(t, d.startDatadogAgentExperiment(context.Background(), req))

	dda := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), testDDANSN, dda))
	assert.Equal(t, req.ID, dda.Annotations[v2alpha1.AnnotationExperimentID])
	assert.Equal(t, v2alpha1.ExperimentSignalStart, dda.Annotations[v2alpha1.AnnotationExperimentSignal])
}

func TestStartDatadogAgentExperiment_Success_OverwritesPreviousExperiment(t *testing.T) {
	// Start a new experiment when a previous one exists (e.g. after termination).
	// The old experiment's annotations must be fully replaced.
	d, c := testDaemon(testDDAObject(v2alpha1.ExperimentPhaseTerminated), testInstallerConfigWithDDA())
	req := testStartRequest()
	require.NoError(t, d.startDatadogAgentExperiment(context.Background(), req))

	dda := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), testDDANSN, dda))
	assert.Equal(t, req.ID, dda.Annotations[v2alpha1.AnnotationExperimentID])
	assert.Equal(t, v2alpha1.ExperimentSignalStart, dda.Annotations[v2alpha1.AnnotationExperimentSignal])
	assert.NotEqual(t, "old-exp", dda.Annotations[v2alpha1.AnnotationExperimentID])
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
					Operation:        OperationUpdate,
					GroupVersionKind: testDDAGVK,
					NamespacedName:   testDDANSN,
					Config:           json.RawMessage(`{"spec":{"features":{"apm":{"enabled":true}}}}`),
				},
			},
		},
	}
	d, c := testDaemon(testDDAObject(""), configs)
	req := remoteAPIRequest{
		ID:     "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
		Method: methodStartDatadogAgentExperiment,
		Params: experimentParams{Version: "aaaa-bbbb-cccc"},
		ExpectedState: expectedState{
			Stable:       "0.0.1",
			StableConfig: "0.0.1",
			ClientID:     "aAbBcCdDeEfFgGhHiIjJk",
		},
	}
	require.NoError(t, d.startDatadogAgentExperiment(context.Background(), req))

	dda := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), testDDANSN, dda))
	assert.Equal(t, req.ID, dda.Annotations[v2alpha1.AnnotationExperimentID])
	assert.Equal(t, v2alpha1.ExperimentSignalStart, dda.Annotations[v2alpha1.AnnotationExperimentSignal])
}

func TestStartDatadogAgentExperiment_EmptyVersion(t *testing.T) {
	d, _ := testDaemon(testDDAObject(""), testInstallerConfigWithDDA())
	req := testStartRequest()
	req.Params.Version = ""
	assert.Error(t, d.startDatadogAgentExperiment(context.Background(), req))
}

func TestStartDatadogAgentExperiment_VersionMismatch(t *testing.T) {
	// params.version doesn't match any installer config ID
	d, _ := testDaemon(testDDAObject(""), testInstallerConfigWithDDA())
	req := testStartRequest()
	req.Params.Version = "xxxx-yyyy-zzzz" // no config with this ID
	err := d.startDatadogAgentExperiment(context.Background(), req)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// --- Stop tests ---

func testStopRequest() remoteAPIRequest {
	return remoteAPIRequest{
		ID:      "exp-abc",
		Package: "datadog-operator",
		Method:  methodStopDatadogAgentExperiment,
		Params:  experimentParams{Version: "test-config"},
	}
}

func TestStopDatadogAgentExperiment_DDANotFound(t *testing.T) {
	d, _ := testDaemon(nil, testInstallerConfigWithDDA())
	assert.Error(t, d.stopDatadogAgentExperiment(context.Background(), testStopRequest()))
}

func TestStopDatadogAgentExperiment_Success_NilPhase(t *testing.T) {
	// Daemon writes unconditionally — the reconciler handles the no-op.
	d, c := testDaemon(testDDAObject(""), testInstallerConfigWithDDA())
	require.NoError(t, d.stopDatadogAgentExperiment(context.Background(), testStopRequest()))

	dda := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), testDDANSN, dda))
	assert.Equal(t, v2alpha1.ExperimentSignalRollback, dda.Annotations[v2alpha1.AnnotationExperimentSignal])
	assert.Equal(t, "exp-abc", dda.Annotations[v2alpha1.AnnotationExperimentID])
}

func TestStopDatadogAgentExperiment_Success_Running(t *testing.T) {
	d, c := testDaemon(testDDAObject(v2alpha1.ExperimentPhaseRunning), testInstallerConfigWithDDA())
	require.NoError(t, d.stopDatadogAgentExperiment(context.Background(), testStopRequest()))

	dda := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), testDDANSN, dda))
	// Daemon writes rollback annotation, not status. Status is written by the controller.
	assert.Equal(t, v2alpha1.ExperimentSignalRollback, dda.Annotations[v2alpha1.AnnotationExperimentSignal])
	assert.Equal(t, "exp-abc", dda.Annotations[v2alpha1.AnnotationExperimentID])
}

func TestStopDatadogAgentExperiment_Success_Terminated(t *testing.T) {
	// Already in terminal phase — GET guard skips the patch, returns nil.
	d, _ := testDaemon(testDDAObject(v2alpha1.ExperimentPhaseTerminated), testInstallerConfigWithDDA())
	require.NoError(t, d.stopDatadogAgentExperiment(context.Background(), testStopRequest()))
}

func TestStopDatadogAgentExperiment_Success_Promoted(t *testing.T) {
	// Already in terminal phase — GET guard skips the patch, returns nil.
	d, _ := testDaemon(testDDAObject(v2alpha1.ExperimentPhasePromoted), testInstallerConfigWithDDA())
	require.NoError(t, d.stopDatadogAgentExperiment(context.Background(), testStopRequest()))
}

func TestStopDatadogAgentExperiment_Success_Aborted(t *testing.T) {
	// Already in terminal phase — GET guard skips the patch, returns nil.
	d, _ := testDaemon(testDDAObject(v2alpha1.ExperimentPhaseAborted), testInstallerConfigWithDDA())
	require.NoError(t, d.stopDatadogAgentExperiment(context.Background(), testStopRequest()))
}

// --- promoteDatadogAgentExperiment tests ---

func testPromoteRequest() remoteAPIRequest {
	return remoteAPIRequest{
		ID:      "exp-abc",
		Package: "datadog-operator",
		Method:  methodPromoteDatadogAgentExperiment,
		Params:  experimentParams{Version: "test-config"},
	}
}

func TestPromoteDatadogAgentExperiment_DDANotFound(t *testing.T) {
	d, _ := testDaemon(nil, testInstallerConfigWithDDA())
	assert.Error(t, d.promoteDatadogAgentExperiment(context.Background(), testPromoteRequest()))
}

func TestPromoteDatadogAgentExperiment_NoExperimentVersion(t *testing.T) {
	d, _ := testDaemon(testDDAObject(v2alpha1.ExperimentPhaseRunning), testInstallerConfigWithDDA())
	d.rcClient = &mockRCClient{state: []*pbgo.PackageState{
		{Package: "datadog-operator", StableConfigVersion: "stable-1"},
	}}
	assert.Error(t, d.promoteDatadogAgentExperiment(context.Background(), testPromoteRequest()))
}

func TestPromoteDatadogAgentExperiment_Success_NilPhase(t *testing.T) {
	// Daemon writes unconditionally — the reconciler handles the no-op.
	d, c := testDaemon(testDDAObject(""), testInstallerConfigWithDDA())
	d.rcClient = &mockRCClient{state: []*pbgo.PackageState{
		{Package: "datadog-operator", StableConfigVersion: "stable-1", ExperimentConfigVersion: "exp-1"},
	}}
	require.NoError(t, d.promoteDatadogAgentExperiment(context.Background(), testPromoteRequest()))

	dda := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), testDDANSN, dda))
	assert.Equal(t, v2alpha1.ExperimentSignalPromote, dda.Annotations[v2alpha1.AnnotationExperimentSignal])
	assert.Equal(t, "exp-abc", dda.Annotations[v2alpha1.AnnotationExperimentID])
}

func TestPromoteDatadogAgentExperiment_Success_Running(t *testing.T) {
	d, c := testDaemon(testDDAObject(v2alpha1.ExperimentPhaseRunning), testInstallerConfigWithDDA())
	rc := &mockRCClient{state: []*pbgo.PackageState{
		{Package: "datadog-operator", StableConfigVersion: "stable-1", ExperimentConfigVersion: "exp-1"},
	}}
	d.rcClient = rc
	require.NoError(t, d.promoteDatadogAgentExperiment(context.Background(), testPromoteRequest()))

	dda := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), testDDANSN, dda))
	// Daemon writes promote annotation, not status. Status is written by the controller.
	assert.Equal(t, v2alpha1.ExperimentSignalPromote, dda.Annotations[v2alpha1.AnnotationExperimentSignal])
	assert.Equal(t, "exp-abc", dda.Annotations[v2alpha1.AnnotationExperimentID])
	// Stable should now be the old experiment, experiment should be cleared.
	require.Len(t, rc.state, 1)
	// StableVersion/ExperimentVersion are not set by config experiments.
	assert.Equal(t, "", rc.state[0].StableVersion)
	assert.Equal(t, "", rc.state[0].ExperimentVersion)
	assert.Equal(t, "exp-1", rc.state[0].StableConfigVersion)
	assert.Equal(t, "", rc.state[0].ExperimentConfigVersion)
}

func TestPromoteDatadogAgentExperiment_Success_Terminated(t *testing.T) {
	// Daemon writes unconditionally — the reconciler handles the no-op for terminal phases.
	d, c := testDaemon(testDDAObject(v2alpha1.ExperimentPhaseTerminated), testInstallerConfigWithDDA())
	d.rcClient = &mockRCClient{state: []*pbgo.PackageState{
		{Package: "datadog-operator", StableConfigVersion: "stable-1", ExperimentConfigVersion: "exp-1"},
	}}
	require.NoError(t, d.promoteDatadogAgentExperiment(context.Background(), testPromoteRequest()))

	dda := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), testDDANSN, dda))
	assert.Equal(t, v2alpha1.ExperimentSignalPromote, dda.Annotations[v2alpha1.AnnotationExperimentSignal])
}

func TestPromoteDatadogAgentExperiment_Success_Promoted(t *testing.T) {
	// Already promoted — GET guard skips the patch, returns nil.
	d, _ := testDaemon(testDDAObject(v2alpha1.ExperimentPhasePromoted), testInstallerConfigWithDDA())
	d.rcClient = &mockRCClient{state: []*pbgo.PackageState{
		{Package: "datadog-operator", StableConfigVersion: "stable-1", ExperimentConfigVersion: "exp-1"},
	}}
	require.NoError(t, d.promoteDatadogAgentExperiment(context.Background(), testPromoteRequest()))
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
	err := d.handleRemoteAPIRequest(context.Background(), req)
	require.Error(t, err)
	var stateErr *stateDoesntMatchError
	assert.True(t, errors.As(err, &stateErr))
}

// --- validateOperation tests ---

func TestValidateOperation_Valid(t *testing.T) {
	op := fleetManagementOperation{
		Operation:        OperationUpdate,
		GroupVersionKind: testDDAGVK,
		NamespacedName:   testDDANSN,
		Config:           json.RawMessage(`{}`),
	}
	assert.NoError(t, validateOperation(op))
}

func TestValidateOperation_EmptyName(t *testing.T) {
	op := fleetManagementOperation{
		GroupVersionKind: testDDAGVK,
		NamespacedName:   types.NamespacedName{Namespace: "datadog", Name: ""},
	}
	assert.Error(t, validateOperation(op))
}

func TestValidateOperation_EmptyNamespace(t *testing.T) {
	op := fleetManagementOperation{
		GroupVersionKind: testDDAGVK,
		NamespacedName:   types.NamespacedName{Namespace: "", Name: "datadog-agent"},
	}
	assert.Error(t, validateOperation(op))
}

func TestValidateOperation_EmptyGroup_Allowed(t *testing.T) {
	// Group is auto-detected from the cluster; empty group is valid input.
	op := fleetManagementOperation{
		GroupVersionKind: schema.GroupVersionKind{Group: "", Version: "", Kind: "DatadogAgent"},
		NamespacedName:   testDDANSN,
	}
	assert.NoError(t, validateOperation(op))
}

func TestValidateOperation_WrongKind(t *testing.T) {
	op := fleetManagementOperation{
		GroupVersionKind: schema.GroupVersionKind{Group: "datadoghq.com", Version: "v2alpha1", Kind: "DatadogMonitor"},
		NamespacedName:   testDDANSN,
	}
	assert.Error(t, validateOperation(op))
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

// --- acceptPhase tests ---

func TestAcceptPhase_MatchingPhase(t *testing.T) {
	accept := acceptPhase(v2alpha1.ExperimentPhaseRunning)
	done, err := accept(v2alpha1.ExperimentPhaseRunning)
	assert.True(t, done)
	assert.NoError(t, err)
}

func TestAcceptPhase_NilPhase(t *testing.T) {
	accept := acceptPhase(v2alpha1.ExperimentPhaseRunning)
	done, err := accept("")
	assert.False(t, done)
	assert.NoError(t, err)
}

func TestAcceptPhase_UnexpectedTerminal(t *testing.T) {
	accept := acceptPhase(v2alpha1.ExperimentPhaseRunning)
	done, err := accept(v2alpha1.ExperimentPhaseTerminated)
	assert.True(t, done)
	assert.Error(t, err)
}

func TestAcceptPhase_MultiplePhases(t *testing.T) {
	accept := acceptPhase(v2alpha1.ExperimentPhaseTerminated, v2alpha1.ExperimentPhasePromoted, v2alpha1.ExperimentPhaseAborted)

	for _, phase := range []v2alpha1.ExperimentPhase{
		v2alpha1.ExperimentPhaseTerminated,
		v2alpha1.ExperimentPhasePromoted,
		v2alpha1.ExperimentPhaseAborted,
	} {
		done, err := accept(phase)
		assert.True(t, done, "expected done for phase %q", phase)
		assert.NoError(t, err, "expected no error for phase %q", phase)
	}

	// Non-terminal, not in set — keep waiting.
	done, err := accept(v2alpha1.ExperimentPhaseRunning)
	assert.False(t, done)
	assert.NoError(t, err)

	// Nil phase — keep waiting.
	done, err = accept("")
	assert.False(t, done)
	assert.NoError(t, err)
}

func TestAcceptPhase_SinglePhase_OtherTerminalErrors(t *testing.T) {
	accept := acceptPhase(v2alpha1.ExperimentPhasePromoted)

	done, err := accept(v2alpha1.ExperimentPhaseTerminated)
	assert.True(t, done)
	assert.Error(t, err)

	done, err = accept(v2alpha1.ExperimentPhaseAborted)
	assert.True(t, done)
	assert.Error(t, err)
}

// --- waitForPhase tests ---

// testPhaseWatcher creates a phaseWatcher without an informer for unit tests.
// Use pw.evaluate() to simulate informer events.
func testPhaseWatcher(c client.Client) *phaseWatcher {
	return &phaseWatcher{k8sClient: c}
}

func TestWaitForPhase_ImmediateSuccess(t *testing.T) {
	// Simulate informer delivering DDA with expected phase immediately.
	dda := testDDAObject(v2alpha1.ExperimentPhaseRunning)
	s := testFleetScheme()
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(dda).WithStatusSubresource(dda).Build()
	pw := testPhaseWatcher(c)

	// Simulate the informer event in a goroutine — waitForPhase registers
	// the waiter first, then we deliver the event.
	go func() {
		// Small delay to let waitForPhase register.
		time.Sleep(10 * time.Millisecond)
		pw.evaluate(dda)
	}()

	err := pw.waitForPhase(context.Background(), testDDANSN, "old-exp", acceptPhase(v2alpha1.ExperimentPhaseRunning))
	assert.NoError(t, err)
}

func TestWaitForPhase_AlreadyAtExpectedPhase(t *testing.T) {
	// The DDA is already at the expected phase before waitForPhase is called.
	// No informer event is needed — the synchronous Get check should return immediately.
	dda := testDDAObject(v2alpha1.ExperimentPhaseRunning)
	s := testFleetScheme()
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(dda).WithStatusSubresource(dda).Build()
	pw := testPhaseWatcher(c)

	err := pw.waitForPhase(context.Background(), testDDANSN, "old-exp", acceptPhase(v2alpha1.ExperimentPhaseRunning))
	assert.NoError(t, err)
}

func TestWaitForPhase_AlreadyTerminal(t *testing.T) {
	// The DDA is already at a terminal phase that doesn't match the expected phase.
	// The synchronous Get check should detect this and return an error immediately
	// without waiting for an informer event.
	dda := testDDAObject(v2alpha1.ExperimentPhaseTerminated)
	s := testFleetScheme()
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(dda).WithStatusSubresource(dda).Build()
	pw := testPhaseWatcher(c)

	err := pw.waitForPhase(context.Background(), testDDANSN, "old-exp", acceptPhase(v2alpha1.ExperimentPhaseRunning))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "terminal phase")
}

func TestWaitForPhase_Timeout(t *testing.T) {
	// No informer event delivered — waitForPhase should time out.
	dda := testDDAObject("")
	s := testFleetScheme()
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(dda).WithStatusSubresource(dda).Build()
	pw := testPhaseWatcher(c)

	// Use a very short context timeout to avoid waiting 5 minutes.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := pw.waitForPhase(ctx, testDDANSN, "exp-1", acceptPhase(v2alpha1.ExperimentPhaseRunning))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "timed out")

	// Verify the self-abort rollback signal was written.
	got := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), testDDANSN, got))
	assert.Equal(t, v2alpha1.ExperimentSignalRollback, got.Annotations[v2alpha1.AnnotationExperimentSignal])
	assert.Equal(t, "exp-1", got.Annotations[v2alpha1.AnnotationExperimentID])
}

func TestWaitForPhase_UnexpectedTerminal(t *testing.T) {
	// Informer delivers DDA with unexpected terminal phase.
	dda := testDDAObject(v2alpha1.ExperimentPhaseTerminated)
	s := testFleetScheme()
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(dda).WithStatusSubresource(dda).Build()
	pw := testPhaseWatcher(c)

	go func() {
		time.Sleep(10 * time.Millisecond)
		pw.evaluate(dda)
	}()

	err := pw.waitForPhase(context.Background(), testDDANSN, "old-exp", acceptPhase(v2alpha1.ExperimentPhaseRunning))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "terminal phase")
}

func TestPhaseWatcher_Evaluate_WrongNamespace(t *testing.T) {
	// Informer delivers a DDA from a different namespace — should be ignored.
	c := fake.NewClientBuilder().WithScheme(testFleetScheme()).Build()
	pw := testPhaseWatcher(c)

	w := &phaseWaiter{
		nsn:    testDDANSN,
		accept: acceptPhase(v2alpha1.ExperimentPhaseRunning),
		ch:     make(chan phaseResult, 1),
	}
	pw.mu.Lock()
	pw.current = w
	pw.mu.Unlock()

	wrongDDA := &v2alpha1.DatadogAgent{
		ObjectMeta: metav1.ObjectMeta{Name: "datadog-agent", Namespace: "other-ns"},
		Status: v2alpha1.DatadogAgentStatus{
			Experiment: &v2alpha1.ExperimentStatus{Phase: v2alpha1.ExperimentPhaseRunning},
		},
	}
	pw.evaluate(wrongDDA)

	select {
	case <-w.ch:
		t.Fatal("expected no result for wrong namespace")
	default:
		// expected — no signal
	}
}

func TestPhaseWatcher_Evaluate_NoWaiter(t *testing.T) {
	// No active waiter — evaluate should not panic.
	c := fake.NewClientBuilder().WithScheme(testFleetScheme()).Build()
	pw := testPhaseWatcher(c)

	dda := testDDAObject(v2alpha1.ExperimentPhaseRunning)
	assert.NotPanics(t, func() {
		pw.evaluate(dda)
	})
}

// --- Start idempotency with annotations already applied ---

func TestStartDatadogAgentExperiment_Idempotent_AnnotationAlreadyApplied(t *testing.T) {
	dda := testDDAObject("")
	dda.Annotations = map[string]string{
		v2alpha1.AnnotationExperimentID:     "exp-abc",
		v2alpha1.AnnotationExperimentSignal: v2alpha1.ExperimentSignalStart,
	}
	d, _ := testDaemon(dda, testInstallerConfigWithDDA())
	req := testStartRequest()
	require.NoError(t, d.startDatadogAgentExperiment(context.Background(), req))
}

// --- Start with running experiment and same ID (controller already processed) ---

func TestStartDatadogAgentExperiment_Idempotent_ControllerAlreadyProcessed(t *testing.T) {
	dda := testDDAObject(v2alpha1.ExperimentPhaseRunning)
	dda.Status.Experiment.ID = "exp-abc" // controller already set this
	d, _ := testDaemon(dda, testInstallerConfigWithDDA())
	req := testStartRequest()
	require.NoError(t, d.startDatadogAgentExperiment(context.Background(), req))
}

// --- Stop idempotency with annotation already applied ---

func TestStopDatadogAgentExperiment_Idempotent_AnnotationAlreadyApplied(t *testing.T) {
	dda := testDDAObject(v2alpha1.ExperimentPhaseRunning)
	dda.Annotations[v2alpha1.AnnotationExperimentSignal] = v2alpha1.ExperimentSignalRollback
	d, _ := testDaemon(dda, testInstallerConfigWithDDA())
	require.NoError(t, d.stopDatadogAgentExperiment(context.Background(), testStopRequest()))
}

// --- Promote idempotency with annotation already applied ---

func TestPromoteDatadogAgentExperiment_Idempotent_AnnotationAlreadyApplied(t *testing.T) {
	dda := testDDAObject(v2alpha1.ExperimentPhaseRunning)
	dda.Annotations[v2alpha1.AnnotationExperimentSignal] = v2alpha1.ExperimentSignalPromote
	d, _ := testDaemon(dda, testInstallerConfigWithDDA())
	d.rcClient = &mockRCClient{state: []*pbgo.PackageState{
		{Package: "datadog-operator", StableConfigVersion: "stable-1", ExperimentConfigVersion: "exp-1"},
	}}
	require.NoError(t, d.promoteDatadogAgentExperiment(context.Background(), testPromoteRequest()))
}

// --- mockRCClient ---

// mockRCClient is a minimal RCClient used to test package state updates.
type mockRCClient struct {
	state []*pbgo.PackageState
}

func (m *mockRCClient) Subscribe(_ string, _ func(map[string]state.RawConfig, func(string, state.ApplyStatus))) {
}

func (m *mockRCClient) GetInstallerState() []*pbgo.PackageState {
	return m.state
}

func (m *mockRCClient) SetInstallerState(packages []*pbgo.PackageState) {
	m.state = packages
}

func testDaemonWithRC(initialState []*pbgo.PackageState) (*Daemon, *mockRCClient) {
	rc := &mockRCClient{state: initialState}
	d := &Daemon{
		rcClient: rc,
		configs:  make(map[string]installerConfig),
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
