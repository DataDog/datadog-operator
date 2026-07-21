// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fleet

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	toolscache "k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	ctrlcache "sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	v1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	v2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/pkg/remoteconfig"
)

// --- Test helpers ---

func testFleetScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = corev1.AddToScheme(s)
	_ = rbacv1.AddToScheme(s)
	_ = admissionregistrationv1.AddToScheme(s)
	_ = apiregistrationv1.AddToScheme(s)
	_ = v1alpha1.AddToScheme(s)
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
		apiReader:        c,
		revisionsEnabled: true,
		configs:          configs,
		statusUpdates:    make(chan ddaStatusSnapshot, 32),
	}, c
}

type recordingInformer struct {
	ctrlcache.Informer
	handlers []toolscache.ResourceEventHandler
}

func (i *recordingInformer) AddEventHandler(handler toolscache.ResourceEventHandler) (toolscache.ResourceEventHandlerRegistration, error) {
	i.handlers = append(i.handlers, handler)
	return nil, nil
}

type recordingCache struct {
	ctrlcache.Cache
	informer    *recordingInformer
	informerFor func(client.Object) (ctrlcache.Informer, error)
}

type recordingManager struct {
	manager.Manager
	client   client.Client
	reader   client.Reader
	cache    ctrlcache.Cache
	recorder record.EventRecorder
}

func (m *recordingManager) GetClient() client.Client {
	return m.client
}

func (m *recordingManager) GetAPIReader() client.Reader {
	return m.reader
}

func (m *recordingManager) GetCache() ctrlcache.Cache {
	return m.cache
}

func (m *recordingManager) GetEventRecorderFor(string) record.EventRecorder {
	return m.recorder
}

func (c *recordingCache) GetInformer(_ context.Context, obj client.Object, _ ...ctrlcache.InformerGetOption) (ctrlcache.Informer, error) {
	if c.informerFor != nil {
		return c.informerFor(obj)
	}
	return c.informer, nil
}

var testDDAGVK = schema.GroupVersionKind{
	Group:   "datadoghq.com",
	Version: "v2alpha1",
	Kind:    "DatadogAgent",
}

var testDDANSN = types.NamespacedName{Namespace: "datadog-agent", Name: "datadog-agent"}

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
			// Stop requests intentionally do not use params.version as the experiment
			// identity. It should be empty.
			Version:          "",
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
			// Promote requests intentionally do not use params.version as the experiment
			// identity. It should be empty.
			Version:          "",
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

func TestManagedAgentInstallationFleetExperimentOperations(t *testing.T) {
	for _, test := range []struct {
		name       string
		phase      v2alpha1.ExperimentPhase
		experiment string
		run        func(*Daemon) (*pendingOperation, error)
	}{
		{
			name: "start",
			run: func(d *Daemon) (*pendingOperation, error) {
				return d.startDatadogAgentExperiment(context.Background(), testStartRequest())
			},
		},
		{
			name:       "stop",
			phase:      v2alpha1.ExperimentPhaseRunning,
			experiment: testExperimentID,
			run: func(d *Daemon) (*pendingOperation, error) {
				return d.stopDatadogAgentExperiment(context.Background(), testStopRequest())
			},
		},
		{
			name:       "promote",
			phase:      v2alpha1.ExperimentPhaseRunning,
			experiment: testExperimentID,
			run: func(d *Daemon) (*pendingOperation, error) {
				return d.promoteDatadogAgentExperiment(context.Background(), testPromoteRequest())
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			dda := testFleetManagedDatadogAgent(t, test.phase, testAddonInstallOperationID)
			d, _, rc := testManagedAgentInstallationDaemon([]*pbgo.PackageState{{
				Package:                 packageDatadogOperator,
				StableConfigVersion:     testAddonInstallOperationID,
				ExperimentConfigVersion: test.experiment,
			}}, dda)
			d.configs = testInstallerConfigWithDDA()

			op, err := test.run(d)

			require.NoError(t, err)
			require.NotNil(t, op)
			require.Nil(t, rc.state[0].GetTask())
		})
	}
}

func TestManagedAgentInstallationPromotionPersistsStableConfig(t *testing.T) {
	dda := testFleetManagedDatadogAgent(t, v2alpha1.ExperimentPhasePromoted, testAddonInstallOperationID)
	dda.Annotations[fleetConfigHashAnnotation] = "bootstrap-hash"
	dda.Annotations[v2alpha1.AnnotationPendingTaskID] = "promote-task"
	dda.Annotations[v2alpha1.AnnotationPendingAction] = string(pendingIntentPromote)
	dda.Annotations[v2alpha1.AnnotationPendingExperimentID] = testExperimentID
	dda.Annotations[v2alpha1.AnnotationPendingPackage] = packageDatadogOperator
	dda.Annotations[v2alpha1.AnnotationPendingResultVersion] = testExperimentID
	d, kubeClient, rc := testManagedAgentInstallationDaemon([]*pbgo.PackageState{{
		Package:                 packageDatadogOperator,
		StableConfigVersion:     testAddonInstallOperationID,
		ExperimentConfigVersion: testExperimentID,
	}}, dda)

	newOperationTracker(d).onStatusUpdate(context.Background(), newDDAStatusSnapshot(dda))

	got := &v2alpha1.DatadogAgent{}
	require.NoError(t, kubeClient.Get(context.Background(), testDDANSN, got))
	wantHash, err := fleetDatadogAgentSpecHash(&got.Spec)
	require.NoError(t, err)
	assert.Equal(t, testExperimentID, got.Labels[fleetConfigIDLabel])
	assert.Equal(t, wantHash, got.Annotations[fleetConfigHashAnnotation])
	assert.Equal(t, testExperimentID, rc.state[0].GetStableConfigVersion())
	assert.Empty(t, rc.state[0].GetExperimentConfigVersion())
	require.NotNil(t, rc.state[0].GetTask())
	assert.Equal(t, "promote-task", rc.state[0].GetTask().GetId())
	assert.Equal(t, pbgo.TaskState_DONE, rc.state[0].GetTask().GetState())

	restarted := testRestartedManagedAgentInstallationDaemon(kubeClient)
	require.NoError(t, restarted.rehydrateInstallerState(context.Background()))
	stable, experiment := restarted.getPackageConfigVersions(packageDatadogOperator)
	assert.Equal(t, testExperimentID, stable)
	assert.Empty(t, experiment)
}

func TestManagedAgentInstallationAlreadyPromotedPersistsStableConfig(t *testing.T) {
	dda := testFleetManagedDatadogAgent(t, v2alpha1.ExperimentPhasePromoted, testAddonInstallOperationID)
	dda.Annotations[fleetConfigHashAnnotation] = "bootstrap-hash"
	d, kubeClient, rc := testManagedAgentInstallationDaemon([]*pbgo.PackageState{{
		Package:                 packageDatadogOperator,
		StableConfigVersion:     testAddonInstallOperationID,
		ExperimentConfigVersion: testExperimentID,
	}}, dda)

	op, err := d.promoteDatadogAgentExperiment(context.Background(), testPromoteRequest())

	requireSyncNoError(t, op, err)
	got := &v2alpha1.DatadogAgent{}
	require.NoError(t, kubeClient.Get(context.Background(), testDDANSN, got))
	wantHash, hashErr := fleetDatadogAgentSpecHash(&got.Spec)
	require.NoError(t, hashErr)
	assert.Equal(t, testExperimentID, got.Labels[fleetConfigIDLabel])
	assert.Equal(t, wantHash, got.Annotations[fleetConfigHashAnnotation])
	assert.Equal(t, testExperimentID, rc.state[0].GetStableConfigVersion())
	assert.Empty(t, rc.state[0].GetExperimentConfigVersion())
}

func TestManagedAgentInstallationAlreadyPromotedRetriesStableConfigPersistence(t *testing.T) {
	dda := testFleetManagedDatadogAgent(t, v2alpha1.ExperimentPhasePromoted, testAddonInstallOperationID)
	dda.Annotations[fleetConfigHashAnnotation] = "bootstrap-hash"
	d, kubeClient, rc := testManagedAgentInstallationDaemon([]*pbgo.PackageState{{
		Package:                 packageDatadogOperator,
		StableConfigVersion:     testAddonInstallOperationID,
		ExperimentConfigVersion: testExperimentID,
	}}, dda)
	d.managedAgentInstallationTaskReserved = false
	d.revisionsEnabled = true
	patchFailed := false
	d.client = &managedAgentInstallationFaultClient{
		Client: kubeClient,
		patchError: func(client.Object) error {
			if patchFailed {
				return nil
			}
			patchFailed = true
			return errors.New("promoted config patch failed")
		},
	}
	req := testPromoteRequest()
	req.ExpectedState = expectedState{
		StableConfig:     testAddonInstallOperationID,
		ExperimentConfig: testExperimentID,
	}

	require.ErrorContains(t, d.handleTask(context.Background(), req), "promoted config patch failed")
	assert.Equal(t, pbgo.TaskState_ERROR, rc.state[0].GetTask().GetState())
	assert.Equal(t, testAddonInstallOperationID, rc.state[0].GetStableConfigVersion())
	assert.Equal(t, testExperimentID, rc.state[0].GetExperimentConfigVersion())

	require.NoError(t, d.handleTask(context.Background(), req))
	assert.Equal(t, pbgo.TaskState_DONE, rc.state[0].GetTask().GetState())
	assert.Equal(t, testExperimentID, rc.state[0].GetStableConfigVersion())
	assert.Empty(t, rc.state[0].GetExperimentConfigVersion())
	got := &v2alpha1.DatadogAgent{}
	require.NoError(t, kubeClient.Get(context.Background(), testDDANSN, got))
	wantHash, err := fleetDatadogAgentSpecHash(&got.Spec)
	require.NoError(t, err)
	assert.Equal(t, testExperimentID, got.Labels[fleetConfigIDLabel])
	assert.Equal(t, wantHash, got.Annotations[fleetConfigHashAnnotation])
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
	state           []*pbgo.PackageState
	taskHistory     []*pbgo.PackageStateTask
	refreshCalls    int
	refreshErr      error
	refreshFailures int
	refreshResults  []error
	refreshHook     func()
}

func (m *mockRCClient) Subscribe(_ string, _ func(map[string]state.RawConfig, func(string, state.ApplyStatus))) {
}

func (m *mockRCClient) GetInstallerState() []*pbgo.PackageState {
	return m.state
}

func (m *mockRCClient) RefreshUpdaterTags(context.Context) error {
	if m.refreshHook != nil {
		m.refreshHook()
	}
	m.refreshCalls++
	if len(m.refreshResults) > 0 {
		result := m.refreshResults[0]
		m.refreshResults = m.refreshResults[1:]
		return result
	}
	if m.refreshFailures > 0 {
		m.refreshFailures--
		return m.refreshErr
	}
	return nil
}

type slowInstallerStateRCClient struct {
	mu    sync.Mutex
	state []*pbgo.PackageState
}

func (m *slowInstallerStateRCClient) Subscribe(_ string, _ func(map[string]state.RawConfig, func(string, state.ApplyStatus))) {
}

func (m *slowInstallerStateRCClient) RefreshUpdaterTags(context.Context) error {
	return nil
}

func (m *slowInstallerStateRCClient) GetInstallerState() []*pbgo.PackageState {
	m.mu.Lock()
	state := clonePackageStates(m.state)
	m.mu.Unlock()
	time.Sleep(10 * time.Millisecond)
	return state
}

func (m *slowInstallerStateRCClient) SetInstallerState(packages []*pbgo.PackageState) {
	m.mu.Lock()
	m.state = clonePackageStates(packages)
	m.mu.Unlock()
}

func clonePackageStates(packages []*pbgo.PackageState) []*pbgo.PackageState {
	cloned := make([]*pbgo.PackageState, 0, len(packages))
	for _, pkg := range packages {
		cloned = append(cloned, proto.Clone(pkg).(*pbgo.PackageState))
	}
	return cloned
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

func TestInstallerStateUpdatesAreSerialized(t *testing.T) {
	rcClient := &slowInstallerStateRCClient{state: []*pbgo.PackageState{{Package: packageDatadogOperator}}}
	daemon := &Daemon{rcClient: rcClient}
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		daemon.setTaskState(packageDatadogOperator, "task-id", pbgo.TaskState_RUNNING, nil)
	}()
	go func() {
		defer wg.Done()
		<-start
		daemon.setPackageConfigVersions(packageDatadogOperator, "stable-id", "")
	}()
	close(start)
	wg.Wait()

	state := rcClient.GetInstallerState()
	require.Len(t, state, 1)
	require.NotNil(t, state[0].GetTask())
	assert.Equal(t, "task-id", state[0].GetTask().GetId())
	assert.Equal(t, "stable-id", state[0].GetStableConfigVersion())
}

func TestDaemonStartRequiresCache(t *testing.T) {
	d := &Daemon{}
	err := d.Start(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "controller cache")
}

func TestNewDaemonAppliesManagedInstallationOption(t *testing.T) {
	_, kubeClient, rcClient := testManagedAgentInstallationDaemon([]*pbgo.PackageState{{Package: packageDatadogOperator}})
	informer := &recordingInformer{}
	cache := &recordingCache{informer: informer}
	recorder := record.NewFakeRecorder(1)
	mgr := &recordingManager{client: kubeClient, reader: kubeClient, cache: cache, recorder: recorder}

	d := NewDaemon(rcClient, mgr, true, WithManagedAgentInstallation(testManagedAgentInstallationIdentity, true))

	assert.Same(t, kubeClient, d.client)
	assert.Same(t, kubeClient, d.apiReader)
	assert.Same(t, cache, d.cache)
	assert.Same(t, recorder, d.recorder)
	assert.True(t, d.revisionsEnabled)
	assert.True(t, d.managedAgentInstallationIntentsEnabled)
	assert.True(t, d.managedAgentInstallationTaskReserved)
	assert.Equal(t, testManagedAgentInstallationIdentity, d.managedAgentInstallationIdentity)
	assert.NotNil(t, d.managedAgentInstallationTaskRunner)
	assert.NotNil(t, d.configs)
	assert.NotNil(t, d.statusUpdates)
	assert.NotNil(t, d.managedAgentInstallationUpdates)
	assert.True(t, d.NeedLeaderElection())
	runnerCalled := make(chan struct{})
	d.managedAgentInstallationTaskRunner(func() { close(runnerCalled) })
	select {
	case <-runnerCalled:
	case <-time.After(time.Second):
		require.Fail(t, "managed installation task runner did not run")
	}
}

func TestDaemonStartRegistersManagedInstallationForwarders(t *testing.T) {
	d, _, _ := testManagedAgentInstallationDaemon(
		[]*pbgo.PackageState{{Package: packageDatadogOperator}},
		testFleetCredentialSecret(),
	)
	informer := &recordingInformer{}
	d.cache = &recordingCache{informer: informer}
	d.managedAgentInstallationIntentsEnabled = true
	d.managedAgentInstallationUpdates = make(chan struct{}, 1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	require.NoError(t, d.Start(ctx))
	require.Len(t, informer.handlers, 3)
}

func TestDaemonStartReturnsManagedInstallationForwarderErrors(t *testing.T) {
	for _, test := range []struct {
		name     string
		failType any
	}{
		{name: "intent ConfigMap", failType: &corev1.ConfigMap{}},
		{name: "credential Secret", failType: &corev1.Secret{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			daemon, _, _ := testManagedAgentInstallationDaemon([]*pbgo.PackageState{{Package: packageDatadogOperator}})
			informer := &recordingInformer{}
			daemon.cache = &recordingCache{
				informer: informer,
				informerFor: func(obj client.Object) (ctrlcache.Informer, error) {
					switch test.failType.(type) {
					case *corev1.ConfigMap:
						if _, ok := obj.(*corev1.ConfigMap); ok {
							return nil, errors.New("ConfigMap informer failed")
						}
					case *corev1.Secret:
						if _, ok := obj.(*corev1.Secret); ok {
							return nil, errors.New("Secret informer failed")
						}
					}
					return informer, nil
				},
			}
			daemon.managedAgentInstallationIntentsEnabled = true
			daemon.managedAgentInstallationUpdates = make(chan struct{}, 1)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			err := daemon.Start(ctx)

			require.ErrorContains(t, err, "informer failed")
		})
	}
}

func TestDaemonStartContinuesAfterManagedInstallationRehydrationFailure(t *testing.T) {
	stateConfigMap := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
		Namespace: managedAgentInstallationStateKey.Namespace,
		Name:      managedAgentInstallationStateKey.Name,
	}}
	daemon, _, _ := testManagedAgentInstallationDaemon(
		[]*pbgo.PackageState{{Package: packageDatadogOperator}},
		stateConfigMap,
	)
	daemon.cache = &recordingCache{informer: &recordingInformer{}}
	daemon.managedAgentInstallationIntentsEnabled = true
	daemon.managedAgentInstallationUpdates = make(chan struct{}, 1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	require.NoError(t, daemon.Start(ctx))
}

func TestDispatchRemoteAPIRequestRequiresRevisionsForExperiments(t *testing.T) {
	d, _ := testDaemon(nil, testInstallerConfigWithDDA())
	d.revisionsEnabled = false
	for _, method := range []string{
		methodStartDatadogAgentExperiment,
		methodStopDatadogAgentExperiment,
		methodPromoteDatadogAgentExperiment,
	} {
		t.Run(method, func(t *testing.T) {
			_, err := d.dispatchRemoteAPIRequest(context.Background(), remoteAPIRequest{Method: method})
			require.ErrorContains(t, err, "experiment signals require")
		})
	}
}

func TestHandleConfigsReplacesSnapshot(t *testing.T) {
	d := &Daemon{configs: map[string]installerConfig{"stale": {ID: "stale"}}}
	configs := map[string]installerConfig{
		"path/to/config": {ID: "current"},
	}

	require.NoError(t, d.handleConfigs(context.Background(), configs))

	_, err := d.getConfig("stale")
	require.ErrorContains(t, err, "not found")
	current, err := d.getConfig("current")
	require.NoError(t, err)
	assert.Equal(t, "current", current.ID)
}

func TestRehydrateInstallerStateManagedInstallationPrerequisites(t *testing.T) {
	t.Run("no Remote Configuration client", func(t *testing.T) {
		d, _, _ := testManagedAgentInstallationDaemon(nil)
		d.rcClient = nil
		require.NoError(t, d.rehydrateInstallerState(context.Background()))
	})

	t.Run("missing API reader", func(t *testing.T) {
		d, _, _ := testManagedAgentInstallationDaemon([]*pbgo.PackageState{{Package: packageDatadogOperator}})
		d.apiReader = nil
		require.ErrorContains(t, d.rehydrateInstallerState(context.Background()), "API reader is required")
	})

	t.Run("promoted Fleet update is retained for completion", func(t *testing.T) {
		dda := testFleetManagedDatadogAgent(t, v2alpha1.ExperimentPhasePromoted, testAddonInstallOperationID)
		d, _, _ := testManagedAgentInstallationDaemon([]*pbgo.PackageState{{Package: packageDatadogOperator}}, dda)

		require.NoError(t, d.rehydrateInstallerState(context.Background()))

		stable, experiment := d.getPackageConfigVersions(packageDatadogOperator)
		assert.Equal(t, testAddonInstallOperationID, stable)
		assert.Equal(t, testExperimentID, experiment)
	})
}

func TestRehydrateInstallerStateManagedInstallationValidation(t *testing.T) {
	t.Run("incomplete ownership", func(t *testing.T) {
		dda := testFleetManagedDatadogAgent(t, "", testAddonInstallOperationID)
		delete(dda.Labels, fleetManagedByLabel)
		daemon, _, _ := testManagedAgentInstallationDaemon([]*pbgo.PackageState{{Package: packageDatadogOperator}}, dda)

		require.ErrorContains(t, daemon.rehydrateInstallerState(context.Background()), "incomplete or conflicting")
	})

	t.Run("different installation identity", func(t *testing.T) {
		dda := testFleetManagedDatadogAgent(t, "", testAddonInstallOperationID)
		dda.Labels[fleetTargetIDLabel] = "other-target"
		daemon, _, _ := testManagedAgentInstallationDaemon([]*pbgo.PackageState{{Package: packageDatadogOperator}}, dda)

		require.ErrorContains(t, daemon.rehydrateInstallerState(context.Background()), "different managed target")
	})

	t.Run("running experiment", func(t *testing.T) {
		dda := testFleetManagedDatadogAgent(t, v2alpha1.ExperimentPhaseRunning, testAddonInstallOperationID)
		daemon, _, _ := testManagedAgentInstallationDaemon([]*pbgo.PackageState{{Package: packageDatadogOperator}}, dda)

		require.NoError(t, daemon.rehydrateInstallerState(context.Background()))
		stable, experiment := daemon.getPackageConfigVersions(packageDatadogOperator)
		assert.Equal(t, testAddonInstallOperationID, stable)
		assert.Equal(t, testExperimentID, experiment)
	})

	t.Run("legacy list failure", func(t *testing.T) {
		daemon, kubeClient := testDaemon(nil, nil)
		daemon.rcClient = &mockRCClient{state: []*pbgo.PackageState{{Package: packageDatadogOperator}}}
		daemon.apiReader = &managedAgentInstallationFaultClient{
			Client: kubeClient,
			listError: func(list client.ObjectList) error {
				if _, ok := list.(*v2alpha1.DatadogAgentList); ok {
					return errors.New("DatadogAgent list failed")
				}
				return nil
			},
		}

		require.ErrorContains(t, daemon.rehydrateInstallerState(context.Background()), "DatadogAgent list failed")
	})
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

func TestHandleTask_ManagedAgentInstallationBusyPreservesTaskState(t *testing.T) {
	managedTask := &pbgo.PackageStateTask{Id: "managed-install", State: pbgo.TaskState_RUNNING}
	rc := []*pbgo.PackageState{{
		Package:             packageDatadogOperator,
		StableConfigVersion: "managed-install",
		Task:                managedTask,
	}}
	d, _, state := testDaemonFull(testDDAObject(""), testInstallerConfigWithDDA(), rc)
	d.managedAgentInstallationTaskReserved = true

	err := d.handleTask(context.Background(), testStartRequest())

	require.ErrorContains(t, err, "managed Agent installation transition is already in progress")
	require.NotNil(t, state.state[0].Task)
	assert.Equal(t, managedTask.Id, state.state[0].Task.Id)
	assert.Equal(t, managedTask.State, state.state[0].Task.State)
}

func TestManagedAgentInstallationIntentsDisabledPreservesFleetUpdates(t *testing.T) {
	dda := testFleetManagedDatadogAgent(t, "", testAddonInstallOperationID)
	d, kubeClient, rc := testManagedAgentInstallationDaemon([]*pbgo.PackageState{{
		Package:             packageDatadogOperator,
		StableConfigVersion: remoteconfig.InstallerStateUnknownConfigVersion,
	}}, dda)
	d.configs = testInstallerConfigWithDDA()
	WithManagedAgentInstallation(testManagedAgentInstallationIdentity, false)(d)
	d.revisionsEnabled = true

	require.NoError(t, d.rehydrateInstallerState(context.Background()))
	stable, experiment := d.getPackageConfigVersions(packageDatadogOperator)
	assert.Equal(t, testAddonInstallOperationID, stable)
	assert.Empty(t, experiment)
	assert.False(t, d.managedAgentInstallationIntentsEnabled)
	assert.False(t, d.managedAgentInstallationTaskReserved)

	req := testStartRequest()
	req.ExpectedState = expectedState{StableConfig: testAddonInstallOperationID}
	require.NoError(t, d.handleTask(context.Background(), req))

	got := &v2alpha1.DatadogAgent{}
	require.NoError(t, kubeClient.Get(context.Background(), testDDANSN, got))
	assert.Equal(t, req.ID, got.Annotations[v2alpha1.AnnotationPendingTaskID])
	assert.Equal(t, testExperimentID, got.Annotations[v2alpha1.AnnotationPendingExperimentID])
	require.Nil(t, rc.state[0].GetTask())
}

func TestManagedAgentInstallationDisabledPreservesUnmanagedFleetUpdates(t *testing.T) {
	d, kubeClient := testDaemon(testDDAObject(""), testInstallerConfigWithDDA())
	WithManagedAgentInstallation(ManagedAgentInstallationIdentity{}, false)(d)

	requireStartQueued(t, d, testStartRequest())

	got := &v2alpha1.DatadogAgent{}
	require.NoError(t, kubeClient.Get(context.Background(), testDDANSN, got))
	assert.Equal(t, testExperimentID, got.Annotations[v2alpha1.AnnotationPendingExperimentID])
}

func TestReconcileLocallyTerminatedExperiment_ClearsExperimentConfigVersion(t *testing.T) {
	d, rc := testDaemonWithRC([]*pbgo.PackageState{
		{Package: "datadog-operator", StableConfigVersion: "stable-1", ExperimentConfigVersion: testExperimentID},
	})
	dda := testDDAObject(v2alpha1.ExperimentPhaseTerminated)
	dda.Status.Experiment.TerminationReason = "timed_out"

	d.reconcileLocallyTerminatedExperiment(context.Background(), newDDAStatusSnapshot(dda))

	require.Len(t, rc.state, 1)
	assert.Equal(t, "stable-1", rc.state[0].StableConfigVersion)
	assert.Empty(t, rc.state[0].ExperimentConfigVersion)
}

func TestReconcileLocallyTerminatedExperiment_IgnoresNonTimeoutTermination(t *testing.T) {
	d, rc := testDaemonWithRC([]*pbgo.PackageState{
		{Package: "datadog-operator", StableConfigVersion: "stable-1", ExperimentConfigVersion: testExperimentID},
	})
	dda := testDDAObject(v2alpha1.ExperimentPhaseTerminated)
	dda.Status.Experiment.TerminationReason = "stopped"

	d.reconcileLocallyTerminatedExperiment(context.Background(), newDDAStatusSnapshot(dda))

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

func TestRunPendingOperationWorker_RecoversPendingOperationWhileManagedInstallationReserved(t *testing.T) {
	dda := testDDAObject(v2alpha1.ExperimentPhaseRunning)
	dda.Annotations[v2alpha1.AnnotationPendingTaskID] = "task-1"
	dda.Annotations[v2alpha1.AnnotationPendingAction] = string(pendingIntentStart)
	dda.Annotations[v2alpha1.AnnotationPendingExperimentID] = testExperimentID
	dda.Annotations[v2alpha1.AnnotationPendingPackage] = "datadog-operator"

	d, c := testDaemon(dda, testInstallerConfigWithDDA())
	d.rcClient = &mockRCClient{state: []*pbgo.PackageState{
		{Package: "datadog-operator", StableConfigVersion: "stable-1"},
	}}
	d.managedAgentInstallationTaskReserved = true

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

// TestRehydrateInstallerState_RunningExperiment is the regression test for
// the lost-installer-state bug. After scale-down/up the daemon's in-memory
// installer state was reset to no-experiment, but the DDA's Status.Experiment
// still showed Running. Fleet Automation read the daemon's empty state, decided
// the experiment hadn't started, and re-sent the start task.
//
// rehydrateInstallerState seeds the daemon's installer state from the DDA at
// startup so the daemon reports the correct in-progress experiment to FA.
func TestRehydrateInstallerState_RunningExperiment(t *testing.T) {
	dda := testDDAObject(v2alpha1.ExperimentPhaseRunning)
	d, _ := testDaemon(dda, nil)
	rc := &mockRCClient{state: []*pbgo.PackageState{{
		Package:             packageDatadogOperator,
		StableVersion:       "v1.27.0",
		StableConfigVersion: "stable",
	}}}
	d.rcClient = rc

	require.NoError(t, d.rehydrateInstallerState(context.Background()))

	stable, experiment := d.getPackageConfigVersions(packageDatadogOperator)
	assert.Equal(t, "stable", stable, "stable version preserved")
	assert.Equal(t, testExperimentID, experiment, "experiment id rehydrated from DDA")
}

// TestRehydrateInstallerState_TerminalPhasesSkipped verifies that a DDA whose
// experiment is in a terminal phase (terminated, promoted, aborted) does not
// repopulate the installer state — those experiments are no longer in flight.
func TestRehydrateInstallerState_TerminalPhasesSkipped(t *testing.T) {
	terminals := []v2alpha1.ExperimentPhase{
		v2alpha1.ExperimentPhaseTerminated,
		v2alpha1.ExperimentPhasePromoted,
		v2alpha1.ExperimentPhaseAborted,
	}
	for _, phase := range terminals {
		t.Run(string(phase), func(t *testing.T) {
			dda := testDDAObject(phase)
			d, _ := testDaemon(dda, nil)
			rc := &mockRCClient{state: []*pbgo.PackageState{{
				Package:             packageDatadogOperator,
				StableVersion:       "v1.27.0",
				StableConfigVersion: "stable",
			}}}
			d.rcClient = rc

			require.NoError(t, d.rehydrateInstallerState(context.Background()))

			_, experiment := d.getPackageConfigVersions(packageDatadogOperator)
			assert.Empty(t, experiment, "terminal-phase DDA must not seed installer state")
		})
	}
}

// TestRehydrateInstallerState_NoDDA is a no-op: no DDAs in the cluster means
// nothing to rehydrate. The installer state is left as-is.
func TestRehydrateInstallerState_NoDDA(t *testing.T) {
	d, _ := testDaemon(nil, nil)
	rc := &mockRCClient{state: []*pbgo.PackageState{{
		Package:             packageDatadogOperator,
		StableVersion:       "v1.27.0",
		StableConfigVersion: "stable",
	}}}
	d.rcClient = rc

	require.NoError(t, d.rehydrateInstallerState(context.Background()))

	stable, experiment := d.getPackageConfigVersions(packageDatadogOperator)
	assert.Equal(t, "stable", stable)
	assert.Empty(t, experiment)
}

func TestRehydrateInstallerState_ManagedInstallationStates(t *testing.T) {
	for _, test := range []struct {
		name       string
		state      string
		wantStable string
	}{
		{name: "ready", state: fleetManagedAgentInstallationStateReady, wantStable: testAddonInstallOperationID},
		{name: "partial", state: fleetManagedAgentInstallationStatePartial, wantStable: fleetPartialConfigVersionPrefix + testAddonInstallOperationID},
	} {
		t.Run(test.name, func(t *testing.T) {
			dda := testFleetManagedDatadogAgent(t, "", testAddonInstallOperationID)
			dda.Labels[fleetManagedAgentInstallationStateLabel] = test.state
			d, _, _ := testManagedAgentInstallationDaemon([]*pbgo.PackageState{{
				Package:             packageDatadogOperator,
				StableConfigVersion: remoteconfig.InstallerStateUnknownConfigVersion,
			}}, dda)

			require.NoError(t, d.rehydrateInstallerState(context.Background()))

			stable, experiment := d.getPackageConfigVersions(packageDatadogOperator)
			assert.Equal(t, test.wantStable, stable)
			assert.Empty(t, experiment)
		})
	}
}

func TestRehydrateInstallerState_ManagedInstallationRecognizesUnmanagedTarget(t *testing.T) {
	d, _, _ := testManagedAgentInstallationDaemon([]*pbgo.PackageState{{
		Package:             packageDatadogOperator,
		StableConfigVersion: remoteconfig.InstallerStateUnknownConfigVersion,
	}}, testDDAObject(""))

	require.NoError(t, d.rehydrateInstallerState(context.Background()))

	stable, experiment := d.getPackageConfigVersions(packageDatadogOperator)
	assert.Equal(t, fleetUnmanagedConfigVersion, stable)
	assert.Empty(t, experiment)
}

// TestReconcileLocallyTerminatedExperiment_ReportsErrorOnStartTaskID verifies that
// when Status.Experiment.StartTaskID is set, reconcileLocallyTerminatedExperiment
// reports the original start task as TaskState_ERROR in addition to
// clearing the experiment config version. This gives Fleet Automation an
// explicit terminal failure tied to the task it originally sent, so it
// stops resending start signals for the same experiment ID.
func TestReconcileLocallyTerminatedExperiment_ReportsErrorOnStartTaskID(t *testing.T) {
	const startTaskID = "task-uuid-from-start"
	d, rc := testDaemonWithRC([]*pbgo.PackageState{
		{Package: "datadog-operator", StableConfigVersion: "stable-1", ExperimentConfigVersion: testExperimentID},
	})
	dda := testDDAObject(v2alpha1.ExperimentPhaseTerminated)
	dda.Status.Experiment.TerminationReason = "timed_out"
	dda.Status.Experiment.StartTaskID = startTaskID

	d.reconcileLocallyTerminatedExperiment(context.Background(), newDDAStatusSnapshot(dda))

	require.Len(t, rc.state, 1)
	assert.Empty(t, rc.state[0].ExperimentConfigVersion, "experimentConfigVersion cleared after timeout")
	require.NotNil(t, rc.state[0].Task, "task state populated for FA")
	assert.Equal(t, startTaskID, rc.state[0].Task.Id, "task id matches original start task")
	assert.Equal(t, pbgo.TaskState_ERROR, rc.state[0].Task.State, "task reported as ERROR")
	require.NotNil(t, rc.state[0].Task.Error)
	assert.Contains(t, rc.state[0].Task.Error.Message, testExperimentID, "error message references the timed-out experiment id")
}

// TestReconcileLocallyTerminatedExperiment_NoStartTaskID_LegacyFallback verifies
// that pre-v1.27 DDAs (or in-flight experiments at upgrade time) without
// Status.Experiment.StartTaskID still get their experimentConfigVersion
// cleared. The task-state report is skipped silently — without the task
// id there is nothing to report against.
func TestReconcileLocallyTerminatedExperiment_NoStartTaskID_LegacyFallback(t *testing.T) {
	d, rc := testDaemonWithRC([]*pbgo.PackageState{
		{Package: "datadog-operator", StableConfigVersion: "stable-1", ExperimentConfigVersion: testExperimentID},
	})
	dda := testDDAObject(v2alpha1.ExperimentPhaseTerminated)
	dda.Status.Experiment.TerminationReason = "timed_out"
	// StartTaskID intentionally empty.

	d.reconcileLocallyTerminatedExperiment(context.Background(), newDDAStatusSnapshot(dda))

	require.Len(t, rc.state, 1)
	assert.Empty(t, rc.state[0].ExperimentConfigVersion)
	assert.Nil(t, rc.state[0].Task, "no start task id → no task state reported")
}

// TestReconcileLocallyTerminatedExperiment_AbortClearsAndReports verifies
// that a manual-spec-change abort (Phase=Aborted) is published to RC the
// same way a timeout is:
//   - TaskState_ERROR against the original StartTaskID
//   - experimentConfigVersion cleared
//
// Without this, FA continues to believe the experiment is running because
// the daemon's last-published state still shows experimentConfigVersion
// set and the original start task as DONE.
func TestReconcileLocallyTerminatedExperiment_AbortClearsAndReports(t *testing.T) {
	const startTaskID = "task-uuid-from-start"
	d, rc := testDaemonWithRC([]*pbgo.PackageState{
		{Package: "datadog-operator", StableConfigVersion: "stable-1", ExperimentConfigVersion: testExperimentID},
	})
	dda := testDDAObject(v2alpha1.ExperimentPhaseAborted)
	dda.Status.Experiment.StartTaskID = startTaskID

	d.reconcileLocallyTerminatedExperiment(context.Background(), newDDAStatusSnapshot(dda))

	require.Len(t, rc.state, 1)
	assert.Equal(t, "stable-1", rc.state[0].StableConfigVersion)
	assert.Empty(t, rc.state[0].ExperimentConfigVersion, "experimentConfigVersion cleared after abort")
	require.NotNil(t, rc.state[0].Task, "task state populated for FA")
	assert.Equal(t, startTaskID, rc.state[0].Task.Id, "task id matches original start task")
	assert.Equal(t, pbgo.TaskState_ERROR, rc.state[0].Task.State, "task reported as ERROR")
	require.NotNil(t, rc.state[0].Task.Error)
	assert.Contains(t, rc.state[0].Task.Error.Message, testExperimentID,
		"error message references the aborted experiment id")
	assert.Contains(t, rc.state[0].Task.Error.Message, "abort",
		"error message indicates this was an abort, not a timeout")
}

// TestReconcileLocallyTerminatedExperiment_IgnoresPromoted verifies the
// guard correctly excludes Phase=Promoted, which is a Fleet-driven
// transition (promote task) whose lifecycle is handled by
// evaluatePendingTask / finishPendingOperation. Publishing a duplicate
// terminal state from here would clobber the promote task's DONE report.
func TestReconcileLocallyTerminatedExperiment_IgnoresPromoted(t *testing.T) {
	d, rc := testDaemonWithRC([]*pbgo.PackageState{
		{Package: "datadog-operator", StableConfigVersion: "stable-1", ExperimentConfigVersion: testExperimentID},
	})
	dda := testDDAObject(v2alpha1.ExperimentPhasePromoted)
	dda.Status.Experiment.StartTaskID = "task-promote-driven"

	d.reconcileLocallyTerminatedExperiment(context.Background(), newDDAStatusSnapshot(dda))

	require.Len(t, rc.state, 1)
	assert.Equal(t, testExperimentID, rc.state[0].ExperimentConfigVersion,
		"promoted is a Fleet-driven terminal phase; experimentConfigVersion must stay")
	assert.Nil(t, rc.state[0].Task, "no task state report for promoted")
}
