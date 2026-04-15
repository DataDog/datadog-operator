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
		experimentTarget: testDDANSN,
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
	// Backend retries with a new task UUID; should be treated as idempotent.
	req := testStartRequest()
	req.ID = "retry-task-uuid"
	require.NoError(t, d.startDatadogAgentExperiment(context.Background(), req))

	// DDA should be unchanged — no re-patch, no status update.
	got := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), testDDANSN, got))
	require.NotNil(t, got.Status.Experiment)
	assert.Equal(t, v2alpha1.ExperimentPhaseRunning, got.Status.Experiment.Phase)
	assert.Equal(t, "old-exp", got.Status.Experiment.ID)
}

func TestStartDatadogAgentExperiment_Stopped(t *testing.T) {
	d, _ := testDaemon(testDDAObject(v2alpha1.ExperimentPhaseStopped), testInstallerConfigWithDDA())
	assert.Error(t, d.startDatadogAgentExperiment(context.Background(), testStartRequest()))
}

func TestStartDatadogAgentExperiment_Success_NilPhase(t *testing.T) {
	d, c := testDaemon(testDDAObject(""), testInstallerConfigWithDDA())
	req := testStartRequest()
	require.NoError(t, d.startDatadogAgentExperiment(context.Background(), req))

	dda := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), testDDANSN, dda))
	require.NotNil(t, dda.Status.Experiment)
	assert.Equal(t, v2alpha1.ExperimentPhaseRunning, dda.Status.Experiment.Phase)
	assert.Equal(t, req.ID, dda.Status.Experiment.ID)
}

func TestStartDatadogAgentExperiment_Success_FromRollback(t *testing.T) {
	d, c := testDaemon(testDDAObject(v2alpha1.ExperimentPhaseRollback), testInstallerConfigWithDDA())
	req := testStartRequest()
	require.NoError(t, d.startDatadogAgentExperiment(context.Background(), req))

	dda := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), testDDANSN, dda))
	require.NotNil(t, dda.Status.Experiment)
	assert.Equal(t, v2alpha1.ExperimentPhaseRunning, dda.Status.Experiment.Phase)
	assert.Equal(t, req.ID, dda.Status.Experiment.ID)
}

func TestStartDatadogAgentExperiment_Success_FromTimeout(t *testing.T) {
	d, c := testDaemon(testDDAObject(v2alpha1.ExperimentPhaseTimeout), testInstallerConfigWithDDA())
	req := testStartRequest()
	require.NoError(t, d.startDatadogAgentExperiment(context.Background(), req))

	dda := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), testDDANSN, dda))
	assert.Equal(t, v2alpha1.ExperimentPhaseRunning, dda.Status.Experiment.Phase)
}

func TestStartDatadogAgentExperiment_Success_FromAborted(t *testing.T) {
	d, c := testDaemon(testDDAObject(v2alpha1.ExperimentPhaseAborted), testInstallerConfigWithDDA())
	req := testStartRequest()
	require.NoError(t, d.startDatadogAgentExperiment(context.Background(), req))

	dda := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), testDDANSN, dda))
	assert.Equal(t, v2alpha1.ExperimentPhaseRunning, dda.Status.Experiment.Phase)
}

func TestStartDatadogAgentExperiment_Success_OverwritesPreviousExperiment(t *testing.T) {
	// Start a new experiment when a previous one exists (e.g. after rollback).
	// The old experiment's ID and generation must be fully replaced.
	d, c := testDaemon(testDDAObject(v2alpha1.ExperimentPhaseRollback), testInstallerConfigWithDDA())
	req := testStartRequest()
	require.NoError(t, d.startDatadogAgentExperiment(context.Background(), req))

	dda := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), testDDANSN, dda))
	require.NotNil(t, dda.Status.Experiment)
	assert.Equal(t, v2alpha1.ExperimentPhaseRunning, dda.Status.Experiment.Phase)
	assert.Equal(t, req.ID, dda.Status.Experiment.ID)
	assert.NotEqual(t, "old-exp", dda.Status.Experiment.ID)
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
	require.NotNil(t, dda.Status.Experiment)
	assert.Equal(t, v2alpha1.ExperimentPhaseRunning, dda.Status.Experiment.Phase)
	assert.Equal(t, req.ID, dda.Status.Experiment.ID)
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

// --- stopDatadogAgentExperiment tests ---

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

func TestStopDatadogAgentExperiment_NilPhase(t *testing.T) {
	d, _ := testDaemon(testDDAObject(""), testInstallerConfigWithDDA())
	assert.Error(t, d.stopDatadogAgentExperiment(context.Background(), testStopRequest()))
}

func TestStopDatadogAgentExperiment_Aborted(t *testing.T) {
	d, _ := testDaemon(testDDAObject(v2alpha1.ExperimentPhaseAborted), testInstallerConfigWithDDA())
	assert.Error(t, d.stopDatadogAgentExperiment(context.Background(), testStopRequest()))
}

func TestStopDatadogAgentExperiment_NoOp_Rollback(t *testing.T) {
	d, c := testDaemon(testDDAObject(v2alpha1.ExperimentPhaseRollback), testInstallerConfigWithDDA())
	require.NoError(t, d.stopDatadogAgentExperiment(context.Background(), testStopRequest()))

	// Status must not change — already rolled back.
	dda := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), testDDANSN, dda))
	assert.Equal(t, v2alpha1.ExperimentPhaseRollback, dda.Status.Experiment.Phase)
}

func TestStopDatadogAgentExperiment_NoOp_Timeout(t *testing.T) {
	d, c := testDaemon(testDDAObject(v2alpha1.ExperimentPhaseTimeout), testInstallerConfigWithDDA())
	require.NoError(t, d.stopDatadogAgentExperiment(context.Background(), testStopRequest()))

	dda := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), testDDANSN, dda))
	assert.Equal(t, v2alpha1.ExperimentPhaseTimeout, dda.Status.Experiment.Phase)
}

func TestStopDatadogAgentExperiment_NoOp_Promoted(t *testing.T) {
	d, c := testDaemon(testDDAObject(v2alpha1.ExperimentPhasePromoted), testInstallerConfigWithDDA())
	require.NoError(t, d.stopDatadogAgentExperiment(context.Background(), testStopRequest()))

	dda := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), testDDANSN, dda))
	assert.Equal(t, v2alpha1.ExperimentPhasePromoted, dda.Status.Experiment.Phase)
}

func TestStopDatadogAgentExperiment_Success_Running(t *testing.T) {
	d, c := testDaemon(testDDAObject(v2alpha1.ExperimentPhaseRunning), testInstallerConfigWithDDA())
	require.NoError(t, d.stopDatadogAgentExperiment(context.Background(), testStopRequest()))

	dda := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), testDDANSN, dda))
	require.NotNil(t, dda.Status.Experiment)
	assert.Equal(t, v2alpha1.ExperimentPhaseStopped, dda.Status.Experiment.Phase)
	// ID must be preserved.
	assert.Equal(t, "old-exp", dda.Status.Experiment.ID)
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

func TestPromoteDatadogAgentExperiment_NilPhase(t *testing.T) {
	d, _ := testDaemon(testDDAObject(""), testInstallerConfigWithDDA())
	assert.Error(t, d.promoteDatadogAgentExperiment(context.Background(), testPromoteRequest()))
}

func TestPromoteDatadogAgentExperiment_Aborted(t *testing.T) {
	d, _ := testDaemon(testDDAObject(v2alpha1.ExperimentPhaseAborted), testInstallerConfigWithDDA())
	assert.Error(t, d.promoteDatadogAgentExperiment(context.Background(), testPromoteRequest()))
}

func TestPromoteDatadogAgentExperiment_NoExperimentVersion(t *testing.T) {
	d, _ := testDaemon(testDDAObject(v2alpha1.ExperimentPhaseRunning), testInstallerConfigWithDDA())
	d.rcClient = &mockRCClient{state: []*pbgo.PackageState{
		{Package: "datadog-operator", StableConfigVersion: "stable-1"},
	}}
	assert.Error(t, d.promoteDatadogAgentExperiment(context.Background(), testPromoteRequest()))
}

func TestPromoteDatadogAgentExperiment_NoOp_Rollback(t *testing.T) {
	d, c := testDaemon(testDDAObject(v2alpha1.ExperimentPhaseRollback), testInstallerConfigWithDDA())
	d.rcClient = &mockRCClient{state: []*pbgo.PackageState{
		{Package: "datadog-operator", StableConfigVersion: "stable-1", ExperimentConfigVersion: "exp-1"},
	}}
	require.NoError(t, d.promoteDatadogAgentExperiment(context.Background(), testPromoteRequest()))

	dda := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), testDDANSN, dda))
	assert.Equal(t, v2alpha1.ExperimentPhaseRollback, dda.Status.Experiment.Phase)
}

func TestPromoteDatadogAgentExperiment_NoOp_Timeout(t *testing.T) {
	d, c := testDaemon(testDDAObject(v2alpha1.ExperimentPhaseTimeout), testInstallerConfigWithDDA())
	d.rcClient = &mockRCClient{state: []*pbgo.PackageState{
		{Package: "datadog-operator", StableConfigVersion: "stable-1", ExperimentConfigVersion: "exp-1"},
	}}
	require.NoError(t, d.promoteDatadogAgentExperiment(context.Background(), testPromoteRequest()))

	dda := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), testDDANSN, dda))
	assert.Equal(t, v2alpha1.ExperimentPhaseTimeout, dda.Status.Experiment.Phase)
}

func TestPromoteDatadogAgentExperiment_NoOp_Promoted(t *testing.T) {
	d, c := testDaemon(testDDAObject(v2alpha1.ExperimentPhasePromoted), testInstallerConfigWithDDA())
	d.rcClient = &mockRCClient{state: []*pbgo.PackageState{
		{Package: "datadog-operator", StableConfigVersion: "stable-1", ExperimentConfigVersion: "exp-1"},
	}}
	require.NoError(t, d.promoteDatadogAgentExperiment(context.Background(), testPromoteRequest()))

	dda := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), testDDANSN, dda))
	assert.Equal(t, v2alpha1.ExperimentPhasePromoted, dda.Status.Experiment.Phase)
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
	require.NotNil(t, dda.Status.Experiment)
	assert.Equal(t, v2alpha1.ExperimentPhasePromoted, dda.Status.Experiment.Phase)
	// ID must be preserved.
	assert.Equal(t, "old-exp", dda.Status.Experiment.ID)
	// Stable should now be the old experiment, experiment should be cleared.
	require.Len(t, rc.state, 1)
	assert.Equal(t, "exp-1", rc.state[0].StableVersion)
	assert.Equal(t, "", rc.state[0].ExperimentVersion)
	assert.Equal(t, "exp-1", rc.state[0].StableConfigVersion)
	assert.Equal(t, "", rc.state[0].ExperimentConfigVersion)
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

// --- canStart tests ---

func TestCanStart_NilPhase(t *testing.T) {
	assert.NoError(t, canStart(""))
}

func TestCanStart_Running(t *testing.T) {
	assert.Error(t, canStart(v2alpha1.ExperimentPhaseRunning))
}

func TestCanStart_Stopped(t *testing.T) {
	assert.Error(t, canStart(v2alpha1.ExperimentPhaseStopped))
}

func TestCanStart_Rollback(t *testing.T) {
	assert.NoError(t, canStart(v2alpha1.ExperimentPhaseRollback))
}

func TestCanStart_Timeout(t *testing.T) {
	assert.NoError(t, canStart(v2alpha1.ExperimentPhaseTimeout))
}

func TestCanStart_Promoted(t *testing.T) {
	assert.NoError(t, canStart(v2alpha1.ExperimentPhasePromoted))
}

func TestCanStart_Aborted(t *testing.T) {
	assert.NoError(t, canStart(v2alpha1.ExperimentPhaseAborted))
}

// --- canStop tests ---

func TestCanStop_NilPhase(t *testing.T) {
	isNoOp, err := canStop(context.Background(), "")
	assert.False(t, isNoOp)
	assert.Error(t, err)
}

func TestCanStop_Running(t *testing.T) {
	isNoOp, err := canStop(context.Background(), v2alpha1.ExperimentPhaseRunning)
	assert.False(t, isNoOp)
	assert.NoError(t, err)
}

func TestCanStop_Stopped(t *testing.T) {
	isNoOp, err := canStop(context.Background(), v2alpha1.ExperimentPhaseStopped)
	assert.True(t, isNoOp)
	assert.NoError(t, err)
}

func TestCanStop_Rollback(t *testing.T) {
	isNoOp, err := canStop(context.Background(), v2alpha1.ExperimentPhaseRollback)
	assert.True(t, isNoOp)
	assert.NoError(t, err)
}

func TestCanStop_Timeout(t *testing.T) {
	isNoOp, err := canStop(context.Background(), v2alpha1.ExperimentPhaseTimeout)
	assert.True(t, isNoOp)
	assert.NoError(t, err)
}

func TestCanStop_Promoted(t *testing.T) {
	isNoOp, err := canStop(context.Background(), v2alpha1.ExperimentPhasePromoted)
	assert.True(t, isNoOp)
	assert.NoError(t, err)
}

func TestCanStop_Aborted(t *testing.T) {
	isNoOp, err := canStop(context.Background(), v2alpha1.ExperimentPhaseAborted)
	assert.False(t, isNoOp)
	assert.Error(t, err)
}

// --- canPromote tests ---

func TestCanPromote_NilPhase(t *testing.T) {
	isNoOp, err := canPromote(context.Background(), "")
	assert.False(t, isNoOp)
	assert.Error(t, err)
}

func TestCanPromote_Running(t *testing.T) {
	isNoOp, err := canPromote(context.Background(), v2alpha1.ExperimentPhaseRunning)
	assert.False(t, isNoOp)
	assert.NoError(t, err)
}

func TestCanPromote_Stopped(t *testing.T) {
	isNoOp, err := canPromote(context.Background(), v2alpha1.ExperimentPhaseStopped)
	assert.True(t, isNoOp)
	assert.NoError(t, err)
}

func TestCanPromote_Rollback(t *testing.T) {
	isNoOp, err := canPromote(context.Background(), v2alpha1.ExperimentPhaseRollback)
	assert.True(t, isNoOp)
	assert.NoError(t, err)
}

func TestCanPromote_Timeout(t *testing.T) {
	isNoOp, err := canPromote(context.Background(), v2alpha1.ExperimentPhaseTimeout)
	assert.True(t, isNoOp)
	assert.NoError(t, err)
}

func TestCanPromote_Promoted(t *testing.T) {
	isNoOp, err := canPromote(context.Background(), v2alpha1.ExperimentPhasePromoted)
	assert.True(t, isNoOp)
	assert.NoError(t, err)
}

func TestCanPromote_Aborted(t *testing.T) {
	isNoOp, err := canPromote(context.Background(), v2alpha1.ExperimentPhaseAborted)
	assert.False(t, isNoOp)
	assert.Error(t, err)
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
