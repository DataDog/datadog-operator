// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fleet

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	v2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

const (
	testAddonInstallOperationID   = "123e4567-e89b-42d3-a456-426614174010"
	testAddonUninstallOperationID = "123e4567-e89b-42d3-a456-426614174011"
)

type rejectManagedAgentInstallationTerminalStateClient struct {
	client.Client
}

type blockManagedAgentInstallationStateCreateClient struct {
	client.Client
	started chan struct{}
	release chan struct{}
}

type rejectManagedAgentInstallationStateCreateClient struct {
	client.Client
}

type rejectManagedAgentInstallationRunningResultClient struct {
	client.Client
}

type failManagedAgentInstallationTargetReadClient struct {
	client.Reader
}

type failManagedAgentInstallationTargetCreateClient struct {
	client.Client
}

type managedAgentInstallationRCClientWithoutRefresh struct {
	state []*pbgo.PackageState
}

func (*managedAgentInstallationRCClientWithoutRefresh) Subscribe(string, func(map[string]state.RawConfig, func(string, state.ApplyStatus))) {
}

func (c *managedAgentInstallationRCClientWithoutRefresh) GetInstallerState() []*pbgo.PackageState {
	return c.state
}

func (c *managedAgentInstallationRCClientWithoutRefresh) SetInstallerState(installerState []*pbgo.PackageState) {
	c.state = installerState
}

func (c *rejectManagedAgentInstallationTerminalStateClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	configMap, ok := obj.(*corev1.ConfigMap)
	if ok && client.ObjectKeyFromObject(configMap) == managedAgentInstallationStateKey && configMap.Data[managedAgentInstallationStateTaskStateKey] != pbgo.TaskState_RUNNING.String() {
		return fmt.Errorf("transient managed Agent installation state write failure")
	}
	return c.Client.Patch(ctx, obj, patch, opts...)
}

func (c *blockManagedAgentInstallationStateCreateClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if client.ObjectKeyFromObject(obj) == managedAgentInstallationStateKey {
		close(c.started)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-c.release:
		}
	}
	return c.Client.Create(ctx, obj, opts...)
}

func (c *rejectManagedAgentInstallationStateCreateClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if client.ObjectKeyFromObject(obj) == managedAgentInstallationStateKey {
		return fmt.Errorf("transient managed Agent installation state create failure")
	}
	return c.Client.Create(ctx, obj, opts...)
}

func (c *rejectManagedAgentInstallationRunningResultClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	configMap, ok := obj.(*corev1.ConfigMap)
	if ok && client.ObjectKeyFromObject(configMap) == managedAgentInstallationStateKey &&
		configMap.Data[managedAgentInstallationStateTaskStateKey] == pbgo.TaskState_RUNNING.String() &&
		configMap.Data[managedAgentInstallationStateErrorKey] != "" {
		return fmt.Errorf("transient managed Agent installation running-state write failure")
	}
	return c.Client.Patch(ctx, obj, patch, opts...)
}

func (c *failManagedAgentInstallationTargetReadClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if key == managedAgentInstallationTarget {
		return fmt.Errorf("transient managed Agent installation target read failure")
	}
	return c.Reader.Get(ctx, key, obj, opts...)
}

func (c *failManagedAgentInstallationTargetCreateClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if client.ObjectKeyFromObject(obj) == managedAgentInstallationTarget {
		return apierrors.NewTimeoutError("timed out creating managed DatadogAgent", 1)
	}
	return c.Client.Create(ctx, obj, opts...)
}

func TestDecodeManagedAgentInstallationIntent(t *testing.T) {
	raw := testManagedAgentInstallationIntent(t, testAddonInstallOperationID, managedAgentInstallationDesiredStateInstalled)

	intent, config, digest, err := decodeManagedAgentInstallationIntent(raw, testManagedAgentInstallationIdentity)

	require.NoError(t, err)
	assert.Equal(t, testAddonInstallOperationID, intent.OperationID)
	var decoded datadogAgentManagedAgentInstallationConfig
	require.NoError(t, json.Unmarshal(config, &decoded))
	require.NotNil(t, decoded.Spec)
	require.NotNil(t, decoded.Spec.Global)
	assert.Equal(t, "test-cluster", *decoded.Spec.Global.ClusterName)
	assert.Equal(t, "datadoghq.com", *decoded.Spec.Global.Site)
	assert.Equal(t, map[string]string{corev1.LabelOSStable: string(corev1.Linux)}, decoded.Spec.Override[v2alpha1.NodeAgentComponentName].NodeSelector)
	assert.Len(t, digest, 64)

	_, _, repeatedDigest, err := decodeManagedAgentInstallationIntent(raw, testManagedAgentInstallationIdentity)
	require.NoError(t, err)
	assert.Equal(t, digest, repeatedDigest)
}

func TestDecodeManagedAgentInstallationIntentRejectsUnsafeInput(t *testing.T) {
	valid := fmt.Sprintf(`{"version":"v1","installationID":"%s","eksARNSHA256":"%s","operationID":"%s","desiredState":"installed","bootstrap":{"clusterName":"test-cluster","site":"datadoghq.com"}}`, testManagedAgentInstallationIdentity.InstallationID(), testManagedAgentInstallationTargetHash, testAddonInstallOperationID)
	tests := []struct {
		name      string
		raw       []byte
		wantError string
	}{
		{
			name:      "missing intent",
			raw:       nil,
			wantError: "is missing",
		},
		{
			name:      "oversized intent",
			raw:       bytes.Repeat([]byte("x"), managedAgentInstallationMaxIntentSize+1),
			wantError: "exceeds",
		},
		{
			name:      "malformed JSON",
			raw:       []byte(`{`),
			wantError: "decode EKS",
		},
		{
			name:      "trailing JSON",
			raw:       append([]byte(valid), []byte(` {}`)...),
			wantError: "trailing JSON content",
		},
		{
			name:      "unsupported version",
			raw:       []byte(strings.Replace(valid, `"version":"v1"`, `"version":"v2"`, 1)),
			wantError: "unsupported EKS managed Agent installation version",
		},
		{
			name:      "invalid installation ID",
			raw:       []byte(strings.Replace(valid, testManagedAgentInstallationIdentity.InstallationID(), "not-a-uuid", 1)),
			wantError: "invalid EKS managed Agent installation identity",
		},
		{
			name:      "mismatched installation",
			raw:       []byte(strings.Replace(valid, testManagedAgentInstallationIdentity.InstallationID(), "223e4567-e89b-42d3-a456-426614174000", 1)),
			wantError: "installation ID does not match",
		},
		{
			name:      "noncanonical operation",
			raw:       []byte(strings.Replace(valid, testAddonInstallOperationID, strings.ToUpper(testAddonInstallOperationID), 1)),
			wantError: "operation_id must be a canonical",
		},
		{
			name:      "noncanonical acknowledgement",
			raw:       []byte(strings.Replace(valid, `"desiredState":"installed"`, `"desiredState":"installed","acknowledgedOperationID":"`+strings.ToUpper(testAddonInstallOperationID)+`"`, 1)),
			wantError: "acknowledged_operation_id must be a canonical",
		},
		{
			name:      "mismatched install acknowledgement",
			raw:       []byte(strings.Replace(valid, `"desiredState":"installed"`, `"desiredState":"installed","acknowledgedOperationID":"`+testAddonUninstallOperationID+`"`, 1)),
			wantError: "acknowledgement must match",
		},
		{
			name:      "caller managed generation",
			raw:       []byte(strings.Replace(valid, `"desiredState":"installed"`, `"generation":1,"desiredState":"installed"`, 1)),
			wantError: "unknown field",
		},
		{
			name:      "mismatched EKS ARN",
			raw:       []byte(strings.Replace(valid, testManagedAgentInstallationTargetHash, strings.Repeat("f", 64), 1)),
			wantError: "ARN hash does not match",
		},
		{
			name:      "invalid cluster name",
			raw:       []byte(strings.Replace(valid, `"clusterName":"test-cluster"`, `"clusterName":" test-cluster"`, 1)),
			wantError: "cluster name is invalid",
		},
		{
			name:      "unsupported site",
			raw:       []byte(strings.Replace(valid, `"site":"datadoghq.com"`, `"site":"example.com"`, 1)),
			wantError: "site \"example.com\" is unsupported",
		},
		{
			name:      "unsupported desired state",
			raw:       []byte(strings.Replace(valid, `"desiredState":"installed"`, `"desiredState":"paused"`, 1)),
			wantError: "unsupported EKS managed Agent installation desired state",
		},
		{
			name:      "topology input",
			raw:       []byte(strings.Replace(valid, `"site":"datadoghq.com"`, `"site":"datadoghq.com","topology":"mixed"`, 1)),
			wantError: "unknown field",
		},
		{
			name:      "unknown field",
			raw:       []byte(strings.Replace(valid, `"version":"v1"`, `"version":"v1","extra":true`, 1)),
			wantError: "unknown field",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, _, err := decodeManagedAgentInstallationIntent(test.raw, testManagedAgentInstallationIdentity)
			require.ErrorContains(t, err, test.wantError)
		})
	}
}

func TestManagedAgentInstallationIntentInstallAndUninstall(t *testing.T) {
	ctx := context.Background()
	unrelated := &v2alpha1.DatadogAgent{ObjectMeta: metav1.ObjectMeta{
		Namespace: "monitoring",
		Name:      "customer-agent",
		UID:       types.UID("customer-agent-uid"),
	}}
	daemon, kubeClient, rcClient := testManagedAgentInstallationDaemon(
		[]*pbgo.PackageState{{Package: packageDatadogOperator}},
		testFleetCredentialSecret(),
		unrelated,
	)

	install := managedAgentInstallationIntentSnapshot{raw: testManagedAgentInstallationIntent(
		t,
		testAddonInstallOperationID,
		managedAgentInstallationDesiredStateInstalled,
	)}
	putManagedAgentInstallationIntentConfigMap(t, kubeClient, install.raw)
	require.NoError(t, daemon.handleManagedAgentInstallationIntent(ctx, install))

	dda := &v2alpha1.DatadogAgent{}
	require.NoError(t, kubeClient.Get(ctx, testDDANSN, dda))
	assert.Equal(t, testAddonInstallOperationID, dda.Labels[fleetConfigIDLabel])
	assert.Equal(t, string(testManagedAgentInstallationIdentity.Provider()), dda.Labels[fleetManagedAgentInstallationProviderLabel])
	assert.Equal(t, testManagedAgentInstallationIdentity.InstallationID(), dda.Labels[fleetInstallationIDLabel])
	assert.Equal(t, testManagedAgentInstallationIdentity.TargetID(), dda.Labels[fleetTargetIDLabel])
	assert.Equal(t, testAddonInstallOperationID, rcClient.state[0].GetTask().GetId())
	assert.Equal(t, pbgo.TaskState_DONE, rcClient.state[0].GetTask().GetState())
	assert.Equal(t, testAddonInstallOperationID, rcClient.state[0].GetStableConfigVersion())
	stateConfigMap := &corev1.ConfigMap{}
	require.NoError(t, kubeClient.Get(ctx, managedAgentInstallationStateKey, stateConfigMap))
	require.Len(t, stateConfigMap.OwnerReferences, 1)
	assert.Nil(t, stateConfigMap.OwnerReferences[0].BlockOwnerDeletion)
	profile := &v1alpha1.DatadogAgentProfile{}
	require.NoError(t, kubeClient.Get(ctx, managedAgentInstallationWindowsProfileKey, profile))
	assert.Equal(t, string(testManagedAgentInstallationIdentity.Provider()), profile.Labels[fleetManagedAgentInstallationProviderLabel])
	assert.Equal(t, testManagedAgentInstallationIdentity.InstallationID(), profile.Labels[fleetInstallationIDLabel])
	assert.Equal(t, testManagedAgentInstallationIdentity.TargetID(), profile.Labels[fleetTargetIDLabel])

	acknowledge := managedAgentInstallationIntentSnapshot{raw: testManagedAgentInstallationIntent(
		t,
		testAddonInstallOperationID,
		managedAgentInstallationDesiredStateInstalled,
		testAddonInstallOperationID,
	)}
	putManagedAgentInstallationIntentConfigMap(t, kubeClient, acknowledge.raw)
	refreshCallsBeforeAcknowledgement := rcClient.refreshCalls
	releasedDuringAcknowledgementRefresh := false
	rcClient.refreshHook = func() {
		daemon.taskMu.Lock()
		releasedDuringAcknowledgementRefresh = !daemon.managedAgentInstallationTaskReserved
		daemon.taskMu.Unlock()
	}
	require.NoError(t, daemon.handleManagedAgentInstallationIntent(ctx, acknowledge))
	assert.Equal(t, refreshCallsBeforeAcknowledgement+1, rcClient.refreshCalls)
	assert.True(t, releasedDuringAcknowledgementRefresh)
	assert.False(t, daemon.managedAgentInstallationTaskReserved)

	dda.Status.Experiment = &v2alpha1.ExperimentStatus{
		ID:    "fleet-experiment",
		Phase: v2alpha1.ExperimentPhaseRunning,
	}
	require.NoError(t, kubeClient.Status().Update(ctx, dda))
	rcClient.state[0].ExperimentConfigVersion = "fleet-experiment"
	reservedDuringUninstallRefresh := false
	rcClient.refreshHook = func() {
		daemon.taskMu.Lock()
		reservedDuringUninstallRefresh = daemon.managedAgentInstallationTaskReserved
		daemon.taskMu.Unlock()
	}

	uninstall := managedAgentInstallationIntentSnapshot{raw: testManagedAgentInstallationIntent(
		t,
		testAddonUninstallOperationID,
		managedAgentInstallationDesiredStateAbsent,
		testAddonInstallOperationID,
	)}
	putManagedAgentInstallationIntentConfigMap(t, kubeClient, uninstall.raw)
	require.NoError(t, daemon.handleManagedAgentInstallationIntent(ctx, uninstall))
	assert.True(t, reservedDuringUninstallRefresh)

	err := kubeClient.Get(ctx, testDDANSN, &v2alpha1.DatadogAgent{})
	require.True(t, apierrors.IsNotFound(err))
	require.NoError(t, kubeClient.Get(ctx, client.ObjectKeyFromObject(unrelated), &v2alpha1.DatadogAgent{}))
	assert.Equal(t, testAddonUninstallOperationID, rcClient.state[0].GetTask().GetId())
	assert.Equal(t, pbgo.TaskState_DONE, rcClient.state[0].GetTask().GetState())
	assert.Empty(t, rcClient.state[0].GetStableConfigVersion())
	assert.Empty(t, rcClient.state[0].GetExperimentConfigVersion())
	require.NoError(t, daemon.uninstallDatadogAgent(ctx))

	persisted, err := daemon.readManagedAgentInstallationState(ctx)
	require.NoError(t, err)
	require.NotNil(t, persisted)
	assert.Equal(t, testAddonUninstallOperationID, persisted.OperationID)
	assert.Equal(t, testAddonInstallOperationID, persisted.AcknowledgedOperationID)
	assert.Equal(t, pbgo.TaskState_DONE, persisted.TaskState)
}

func TestManagedAgentInstallationIntentWorkerReadsCurrentIntent(t *testing.T) {
	daemon, kubeClient, _ := testManagedAgentInstallationDaemon(
		[]*pbgo.PackageState{{Package: packageDatadogOperator}},
		testFleetCredentialSecret(),
	)
	daemon.managedAgentInstallationUpdates = make(chan struct{}, 1)
	raw := testManagedAgentInstallationIntent(t, testAddonInstallOperationID, managedAgentInstallationDesiredStateInstalled)
	putManagedAgentInstallationIntentConfigMap(t, kubeClient, raw)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go daemon.runManagedAgentInstallationIntentWorker(ctx)
	daemon.requestManagedAgentInstallationRetry()

	require.Eventually(t, func() bool {
		persisted, err := daemon.readManagedAgentInstallationState(context.Background())
		return err == nil && persisted != nil && persisted.TaskState == pbgo.TaskState_DONE
	}, time.Second, time.Millisecond)
}

func TestManagedAgentInstallationResumesPersistedRunningInstall(t *testing.T) {
	ctx := context.Background()
	_, kubeClient, _ := testManagedAgentInstallationDaemon(
		[]*pbgo.PackageState{{Package: packageDatadogOperator}},
		testFleetCredentialSecret(),
	)
	raw := testManagedAgentInstallationIntent(t, testAddonInstallOperationID, managedAgentInstallationDesiredStateInstalled)
	putManagedAgentInstallationIntentConfigMap(t, kubeClient, raw)
	intent, config, digest, err := decodeManagedAgentInstallationIntent(raw, testManagedAgentInstallationIdentity)
	require.NoError(t, err)

	restarted := testRestartedManagedAgentInstallationDaemon(kubeClient)
	command := newManagedAgentInstallationCommand(intent, config, digest)
	require.NoError(t, restarted.writeManagedAgentInstallationState(ctx, managedAgentInstallationStateFromCommand(command, pbgo.TaskState_RUNNING, nil)))

	require.NoError(t, restarted.handleManagedAgentInstallationIntent(ctx, managedAgentInstallationIntentSnapshot{raw: raw}))
	persisted, err := restarted.readManagedAgentInstallationState(ctx)
	require.NoError(t, err)
	require.NotNil(t, persisted)
	assert.Equal(t, pbgo.TaskState_DONE, persisted.TaskState)
	require.NoError(t, kubeClient.Get(ctx, managedAgentInstallationTarget, &v2alpha1.DatadogAgent{}))
}

func TestManagedAgentInstallationResumesPersistedRunningUninstall(t *testing.T) {
	ctx := context.Background()
	daemon, kubeClient, _ := testManagedAgentInstallationDaemon(
		[]*pbgo.PackageState{{Package: packageDatadogOperator}},
		testFleetCredentialSecret(),
	)
	installRaw := testManagedAgentInstallationIntent(t, testAddonInstallOperationID, managedAgentInstallationDesiredStateInstalled)
	putManagedAgentInstallationIntentConfigMap(t, kubeClient, installRaw)
	require.NoError(t, daemon.handleManagedAgentInstallationIntent(ctx, managedAgentInstallationIntentSnapshot{raw: installRaw}))

	uninstallRaw := testManagedAgentInstallationIntent(t, testAddonUninstallOperationID, managedAgentInstallationDesiredStateAbsent)
	putManagedAgentInstallationIntentConfigMap(t, kubeClient, uninstallRaw)
	intent, config, digest, err := decodeManagedAgentInstallationIntent(uninstallRaw, testManagedAgentInstallationIdentity)
	require.NoError(t, err)
	restarted := testRestartedManagedAgentInstallationDaemon(kubeClient)
	command := newManagedAgentInstallationCommand(intent, config, digest)
	require.NoError(t, restarted.writeManagedAgentInstallationState(ctx, managedAgentInstallationStateFromCommand(command, pbgo.TaskState_RUNNING, nil)))

	require.NoError(t, restarted.handleManagedAgentInstallationIntent(ctx, managedAgentInstallationIntentSnapshot{raw: uninstallRaw}))
	persisted, err := restarted.readManagedAgentInstallationState(ctx)
	require.NoError(t, err)
	require.NotNil(t, persisted)
	assert.Equal(t, pbgo.TaskState_DONE, persisted.TaskState)
	assert.True(t, apierrors.IsNotFound(kubeClient.Get(ctx, managedAgentInstallationTarget, &v2alpha1.DatadogAgent{})))
}

func TestManagedAgentInstallationIntentForwarderCoalescesWhenWorkerIsBusy(t *testing.T) {
	daemon := &Daemon{
		managedAgentInstallationNamespace: testManagedAgentInstallationNamespace,
		managedAgentInstallationUpdates:   make(chan struct{}, 1),
	}
	daemon.managedAgentInstallationUpdates <- struct{}{}

	daemon.forwardManagedAgentInstallationIntent(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:       managedAgentInstallationIntentKey.Namespace,
			Name:            managedAgentInstallationIntentKey.Name,
			ResourceVersion: "2",
		},
		Data: map[string]string{managedAgentInstallationIntentDataKey: `{}`},
	})

	require.Len(t, daemon.managedAgentInstallationUpdates, 1)
}

func TestManagedAgentInstallationTerminalStateIsDurableBeforeRemoteConfigCompletion(t *testing.T) {
	ctx := context.Background()
	daemon, kubeClient, rcClient := testManagedAgentInstallationDaemon(
		[]*pbgo.PackageState{{Package: packageDatadogOperator}},
		testFleetCredentialSecret(),
	)
	daemon.managedAgentInstallationUpdates = make(chan struct{}, 1)
	daemon.client = &rejectManagedAgentInstallationTerminalStateClient{Client: daemon.client}
	raw := testManagedAgentInstallationIntent(t, testAddonInstallOperationID, managedAgentInstallationDesiredStateInstalled)
	putManagedAgentInstallationIntentConfigMap(t, kubeClient, raw)

	err := daemon.handleManagedAgentInstallationIntent(ctx, managedAgentInstallationIntentSnapshot{raw: raw})

	require.ErrorContains(t, err, "transient managed Agent installation state write failure")
	require.Len(t, rcClient.state, 1)
	require.NotNil(t, rcClient.state[0].GetTask())
	assert.Equal(t, pbgo.TaskState_RUNNING, rcClient.state[0].GetTask().GetState())
	assert.Equal(t, fleetPartialConfigVersionPrefix+testAddonInstallOperationID, rcClient.state[0].GetStableConfigVersion())
	persisted, readErr := daemon.readManagedAgentInstallationState(ctx)
	require.NoError(t, readErr)
	require.NotNil(t, persisted)
	assert.Equal(t, pbgo.TaskState_RUNNING, persisted.TaskState)
	require.Eventually(t, func() bool {
		return len(daemon.managedAgentInstallationUpdates) == 1
	}, 2*time.Second, 10*time.Millisecond)
}

func TestManagedAgentInstallationParksUntilCredentialSecretExists(t *testing.T) {
	ctx := context.Background()
	daemon, kubeClient, rcClient := testManagedAgentInstallationDaemon(
		[]*pbgo.PackageState{{Package: packageDatadogOperator}},
	)
	originalRetryDelays := managedAgentInstallationCredentialRetryDelays
	managedAgentInstallationCredentialRetryDelays = []time.Duration{0, 0}
	t.Cleanup(func() {
		managedAgentInstallationCredentialRetryDelays = originalRetryDelays
	})
	raw := testManagedAgentInstallationIntent(t, testAddonInstallOperationID, managedAgentInstallationDesiredStateInstalled)
	putManagedAgentInstallationIntentConfigMap(t, kubeClient, raw)
	snapshot := managedAgentInstallationIntentSnapshot{raw: raw}

	for range len(managedAgentInstallationCredentialRetryDelays) + 1 {
		require.ErrorContains(t, daemon.handleManagedAgentInstallationIntent(ctx, snapshot), "credential Secret datadog-agent/datadog-secret is not ready")
		assert.False(t, daemon.managedAgentInstallationActive)
		assert.False(t, daemon.managedAgentInstallationTaskReserved)
	}
	persisted, err := daemon.readManagedAgentInstallationState(ctx)
	require.NoError(t, err)
	require.NotNil(t, persisted)
	assert.Equal(t, pbgo.TaskState_RUNNING, persisted.TaskState)
	require.Len(t, rcClient.state, 1)
	assert.Equal(t, pbgo.TaskState_RUNNING, rcClient.state[0].GetTask().GetState())
	assert.False(t, daemon.managedAgentInstallationActive)
	assert.False(t, daemon.managedAgentInstallationTaskReserved)
	require.True(t, apierrors.IsNotFound(kubeClient.Get(ctx, managedAgentInstallationTarget, &v2alpha1.DatadogAgent{})))

	secret := testFleetCredentialSecret()
	require.NoError(t, kubeClient.Create(ctx, secret))
	daemon.managedAgentInstallationUpdates = make(chan struct{}, 1)
	daemon.forwardManagedAgentInstallationCredential(secret)
	require.Len(t, daemon.managedAgentInstallationUpdates, 1)
	assert.Zero(t, daemon.managedAgentInstallationCredentialRetryIndex)
	assert.False(t, daemon.managedAgentInstallationTaskReserved)
	require.NoError(t, daemon.handleManagedAgentInstallationIntent(ctx, snapshot))
	persisted, err = daemon.readManagedAgentInstallationState(ctx)
	require.NoError(t, err)
	require.NotNil(t, persisted)
	assert.Equal(t, pbgo.TaskState_DONE, persisted.TaskState)
	require.NoError(t, kubeClient.Get(ctx, managedAgentInstallationTarget, &v2alpha1.DatadogAgent{}))
}

func TestManagedAgentInstallationDefersToFleetTask(t *testing.T) {
	ctx := context.Background()
	daemon, kubeClient, _ := testManagedAgentInstallationDaemon(
		[]*pbgo.PackageState{{
			Package:                 packageDatadogOperator,
			StableConfigVersion:     "stable-config",
			ExperimentConfigVersion: "fleet-experiment",
		}},
		testFleetCredentialSecret(),
	)
	daemon.managedAgentInstallationTaskReserved = false
	daemon.managedAgentInstallationIntentsEnabled = true
	daemon.managedAgentInstallationUpdates = make(chan struct{}, 1)
	raw := testManagedAgentInstallationIntent(t, testAddonInstallOperationID, managedAgentInstallationDesiredStateInstalled)
	putManagedAgentInstallationIntentConfigMap(t, kubeClient, raw)

	require.NoError(t, daemon.handleManagedAgentInstallationIntent(ctx, managedAgentInstallationIntentSnapshot{raw: raw}))
	assert.False(t, daemon.managedAgentInstallationActive)
	assert.False(t, daemon.managedAgentInstallationTaskReserved)
	persisted, err := daemon.readManagedAgentInstallationState(ctx)
	require.NoError(t, err)
	assert.Nil(t, persisted)
	require.True(t, apierrors.IsNotFound(kubeClient.Get(ctx, managedAgentInstallationTarget, &v2alpha1.DatadogAgent{})))
	time.Sleep(2 * managedAgentInstallationRetryInterval)
	assert.Empty(t, daemon.managedAgentInstallationUpdates)
	daemon.forwardDDAStatusUpdate(testFleetManagedDatadogAgent(t, v2alpha1.ExperimentPhaseRunning, "stable-config"))
	assert.Len(t, daemon.managedAgentInstallationUpdates, 1)
}

func TestManagedAgentInstallationUninstallDefersToDispatchingFleetTask(t *testing.T) {
	ctx := context.Background()
	dda := testDDAObject(v2alpha1.ExperimentPhaseRunning)
	dda.Annotations[v2alpha1.AnnotationPendingTaskID] = "fleet-task"
	dda.Annotations[v2alpha1.AnnotationPendingAction] = string(pendingIntentStart)
	dda.Annotations[v2alpha1.AnnotationPendingExperimentID] = testExperimentID
	dda.Annotations[v2alpha1.AnnotationPendingPackage] = packageDatadogOperator
	daemon, kubeClient, _ := testManagedAgentInstallationDaemon(
		[]*pbgo.PackageState{{
			Package:                 packageDatadogOperator,
			StableConfigVersion:     testAddonInstallOperationID,
			ExperimentConfigVersion: testExperimentID,
		}},
		dda,
	)
	daemon.managedAgentInstallationIntentsEnabled = true
	daemon.managedAgentInstallationUpdates = make(chan struct{}, 1)
	raw := testManagedAgentInstallationIntent(
		t,
		testAddonUninstallOperationID,
		managedAgentInstallationDesiredStateAbsent,
		testAddonInstallOperationID,
	)
	intent, config, digest, err := decodeManagedAgentInstallationIntent(raw, testManagedAgentInstallationIdentity)
	require.NoError(t, err)

	require.NoError(t, daemon.handleManagedAgentInstallationCommand(ctx, newManagedAgentInstallationCommand(intent, config, digest)))

	assert.False(t, daemon.managedAgentInstallationActive)
	require.NoError(t, kubeClient.Get(ctx, managedAgentInstallationTarget, &v2alpha1.DatadogAgent{}))
	time.Sleep(2 * managedAgentInstallationRetryInterval)
	assert.Empty(t, daemon.managedAgentInstallationUpdates)
	pending, ok := pendingOperationFromAnnotations(testDDANSN, dda.Annotations)
	require.True(t, ok)
	daemon.finishPendingOperation(ctx, pending, nil)
	assert.Len(t, daemon.managedAgentInstallationUpdates, 1)
}

func TestManagedAgentInstallationRetriesAfterUncertainCreate(t *testing.T) {
	ctx := context.Background()
	daemon, kubeClient, rcClient := testManagedAgentInstallationDaemon(
		[]*pbgo.PackageState{{Package: packageDatadogOperator}},
		testFleetCredentialSecret(),
	)
	daemon.client = &failManagedAgentInstallationTargetCreateClient{Client: daemon.client}
	raw := testManagedAgentInstallationIntent(t, testAddonInstallOperationID, managedAgentInstallationDesiredStateInstalled)
	putManagedAgentInstallationIntentConfigMap(t, kubeClient, raw)

	err := daemon.handleManagedAgentInstallationIntent(ctx, managedAgentInstallationIntentSnapshot{raw: raw})
	require.ErrorContains(t, err, "resource could not be recovered")
	persisted, readErr := daemon.readManagedAgentInstallationState(ctx)
	require.NoError(t, readErr)
	require.NotNil(t, persisted)
	assert.Equal(t, pbgo.TaskState_RUNNING, persisted.TaskState)
	require.Len(t, rcClient.state, 1)
	assert.Equal(t, pbgo.TaskState_RUNNING, rcClient.state[0].GetTask().GetState())
}

func TestManagedAgentInstallationCommandReportsTerminalFailures(t *testing.T) {
	ctx := context.Background()

	for _, test := range []struct {
		name      string
		prepare   func(*Daemon, client.Client, *managedAgentInstallationCommand)
		wantState pbgo.TaskState
		wantError string
	}{
		{
			name: "ownership conflict",
			prepare: func(daemon *Daemon, kubeClient client.Client, _ *managedAgentInstallationCommand) {
				require.NoError(t, kubeClient.Create(ctx, testDDAObject("")))
				daemon.apiReader = kubeClient
			},
			wantState: pbgo.TaskState_INVALID_STATE,
			wantError: "is not owned by Fleet Automation",
		},
		{
			name: "non-retryable Kubernetes error",
			prepare: func(daemon *Daemon, _ client.Client, _ *managedAgentInstallationCommand) {
				daemon.client = &rejectManagedAgentInstallationTargetCreateClient{Client: daemon.client}
			},
			wantState: pbgo.TaskState_ERROR,
			wantError: "target create denied",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			daemon, kubeClient, rcClient := testManagedAgentInstallationDaemon(
				[]*pbgo.PackageState{{Package: packageDatadogOperator}},
				testFleetCredentialSecret(),
			)
			raw := testManagedAgentInstallationIntent(t, testAddonInstallOperationID, managedAgentInstallationDesiredStateInstalled)
			putManagedAgentInstallationIntentConfigMap(t, kubeClient, raw)
			command := testManagedAgentInstallationCommand(t, testAddonInstallOperationID, managedAgentInstallationDesiredStateInstalled)
			test.prepare(daemon, kubeClient, &command)

			err := daemon.handleManagedAgentInstallationCommand(ctx, command)
			require.ErrorContains(t, err, test.wantError)
			persisted, readErr := daemon.readManagedAgentInstallationState(ctx)
			require.NoError(t, readErr)
			require.NotNil(t, persisted)
			assert.Equal(t, test.wantState, persisted.TaskState)
			require.Len(t, rcClient.state, 1)
			require.NotNil(t, rcClient.state[0].GetTask())
			assert.Equal(t, test.wantState, rcClient.state[0].GetTask().GetState())
			assert.False(t, daemon.managedAgentInstallationTaskReserved)
		})
	}
}

func TestManagedAgentInstallationCommandRejectsConcurrentOperation(t *testing.T) {
	daemon, _, _ := testManagedAgentInstallationDaemon([]*pbgo.PackageState{{Package: packageDatadogOperator}})
	daemon.managedAgentInstallationActive = true
	command := testManagedAgentInstallationCommand(t, testAddonInstallOperationID, managedAgentInstallationDesiredStateInstalled)

	err := daemon.handleManagedAgentInstallationCommand(context.Background(), command)

	require.ErrorContains(t, err, "transition is already in progress")
}

func TestManagedAgentInstallationCommandReturnsTargetReadFailure(t *testing.T) {
	daemon, kubeClient, _ := testManagedAgentInstallationDaemon([]*pbgo.PackageState{{Package: packageDatadogOperator}})
	daemon.apiReader = &failManagedAgentInstallationTargetReadClient{Reader: kubeClient}
	command := testManagedAgentInstallationCommand(t, testAddonInstallOperationID, managedAgentInstallationDesiredStateInstalled)

	err := daemon.handleManagedAgentInstallationCommand(context.Background(), command)

	require.ErrorContains(t, err, "transient managed Agent installation target read failure")
}

func TestManagedAgentInstallationInvalidCommandRetriesWhenResultCannotPersist(t *testing.T) {
	daemon, _, _ := testManagedAgentInstallationDaemon([]*pbgo.PackageState{{Package: packageDatadogOperator}})
	daemon.managedAgentInstallationUpdates = make(chan struct{}, 1)
	command := testManagedAgentInstallationCommand(t, testAddonInstallOperationID, managedAgentInstallationDesiredStateInstalled)
	command.Intent.Provider = "other"

	err := daemon.handleManagedAgentInstallationCommand(context.Background(), command)

	require.ErrorContains(t, err, "different provider target")
	require.ErrorContains(t, err, "state is missing")
	require.Eventually(t, func() bool {
		return len(daemon.managedAgentInstallationUpdates) == 1
	}, 2*time.Second, 10*time.Millisecond)
}

func TestManagedAgentInstallationTerminalErrorRetriesWhenResultCannotPersist(t *testing.T) {
	ctx := context.Background()
	daemon, kubeClient, _ := testManagedAgentInstallationDaemon(
		[]*pbgo.PackageState{{Package: packageDatadogOperator}},
		testFleetCredentialSecret(),
		testDDAObject(""),
	)
	daemon.managedAgentInstallationUpdates = make(chan struct{}, 1)
	daemon.client = &rejectManagedAgentInstallationTerminalStateClient{Client: daemon.client}
	raw := testManagedAgentInstallationIntent(t, testAddonInstallOperationID, managedAgentInstallationDesiredStateInstalled)
	putManagedAgentInstallationIntentConfigMap(t, kubeClient, raw)
	command := testManagedAgentInstallationCommand(t, testAddonInstallOperationID, managedAgentInstallationDesiredStateInstalled)

	err := daemon.handleManagedAgentInstallationCommand(ctx, command)

	require.ErrorContains(t, err, "is not owned by Fleet Automation")
	require.ErrorContains(t, err, "transient managed Agent installation state write failure")
	require.Eventually(t, func() bool {
		return len(daemon.managedAgentInstallationUpdates) == 1
	}, 2*time.Second, 10*time.Millisecond)
}

func TestDispatchManagedAgentInstallationCommandRejectsUnknownState(t *testing.T) {
	daemon, _, _ := testManagedAgentInstallationDaemon(nil)
	command := testManagedAgentInstallationCommand(t, testAddonInstallOperationID, managedAgentInstallationDesiredStateInstalled)
	command.Intent.DesiredState = "unknown"

	err := daemon.dispatchManagedAgentInstallationCommand(context.Background(), command)

	require.ErrorContains(t, err, "unknown managed Agent installation desired state")
}

func TestManagedAgentInstallationTerminalDoneReconcilesPackageState(t *testing.T) {
	ctx := context.Background()
	daemon, kubeClient, rcClient := testManagedAgentInstallationDaemon(
		[]*pbgo.PackageState{{Package: packageDatadogOperator}},
		testFleetCredentialSecret(),
	)
	raw := testManagedAgentInstallationIntent(t, testAddonInstallOperationID, managedAgentInstallationDesiredStateInstalled)
	putManagedAgentInstallationIntentConfigMap(t, kubeClient, raw)
	intent, config, digest, err := decodeManagedAgentInstallationIntent(raw, testManagedAgentInstallationIdentity)
	require.NoError(t, err)
	require.NoError(t, daemon.writeManagedAgentInstallationState(ctx, managedAgentInstallationStateFromCommand(
		newManagedAgentInstallationCommand(intent, config, digest),
		pbgo.TaskState_DONE,
		nil,
	)))

	require.NoError(t, daemon.handleManagedAgentInstallationIntent(ctx, managedAgentInstallationIntentSnapshot{raw: raw}))

	require.NoError(t, kubeClient.Get(ctx, managedAgentInstallationTarget, &v2alpha1.DatadogAgent{}))
	require.Len(t, rcClient.state, 1)
	assert.Equal(t, testAddonInstallOperationID, rcClient.state[0].GetStableConfigVersion())
	require.NotNil(t, rcClient.state[0].GetTask())
	assert.Equal(t, testAddonInstallOperationID, rcClient.state[0].GetTask().GetId())
	assert.Equal(t, pbgo.TaskState_DONE, rcClient.state[0].GetTask().GetState())
}

func TestManagedAgentInstallationTerminalErrorReconcilesTaskState(t *testing.T) {
	ctx := context.Background()
	daemon, kubeClient, rcClient := testManagedAgentInstallationDaemon(
		[]*pbgo.PackageState{{Package: packageDatadogOperator}},
	)
	raw := testManagedAgentInstallationIntent(t, testAddonInstallOperationID, managedAgentInstallationDesiredStateInstalled)
	putManagedAgentInstallationIntentConfigMap(t, kubeClient, raw)
	intent, config, digest, err := decodeManagedAgentInstallationIntent(raw, testManagedAgentInstallationIdentity)
	require.NoError(t, err)
	terminalErr := fmt.Errorf("installation failed")
	require.NoError(t, daemon.writeManagedAgentInstallationState(ctx, managedAgentInstallationStateFromCommand(
		newManagedAgentInstallationCommand(intent, config, digest),
		pbgo.TaskState_ERROR,
		terminalErr,
	)))

	require.NoError(t, daemon.handleManagedAgentInstallationIntent(ctx, managedAgentInstallationIntentSnapshot{raw: raw}))

	require.Len(t, rcClient.state, 1)
	require.NotNil(t, rcClient.state[0].GetTask())
	assert.Equal(t, testAddonInstallOperationID, rcClient.state[0].GetTask().GetId())
	assert.Equal(t, pbgo.TaskState_ERROR, rcClient.state[0].GetTask().GetState())
	assert.Equal(t, terminalErr.Error(), rcClient.state[0].GetTask().GetError().GetMessage())
}

func TestManagedAgentInstallationReservesFleetTaskSlotBeforeDurableAcceptance(t *testing.T) {
	ctx := context.Background()
	daemon, kubeClient, _ := testManagedAgentInstallationDaemon(
		[]*pbgo.PackageState{{Package: packageDatadogOperator}},
		testFleetCredentialSecret(),
	)
	blockingClient := &blockManagedAgentInstallationStateCreateClient{
		Client:  daemon.client,
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	daemon.client = blockingClient
	raw := testManagedAgentInstallationIntent(t, testAddonInstallOperationID, managedAgentInstallationDesiredStateInstalled)
	putManagedAgentInstallationIntentConfigMap(t, kubeClient, raw)
	result := make(chan error, 1)
	go func() {
		result <- daemon.handleManagedAgentInstallationIntent(ctx, managedAgentInstallationIntentSnapshot{raw: raw})
	}()

	<-blockingClient.started
	daemon.taskMu.Lock()
	reserved := daemon.managedAgentInstallationTaskReserved
	daemon.taskMu.Unlock()
	assert.True(t, reserved)
	close(blockingClient.release)
	require.NoError(t, <-result)
}

func TestManagedAgentInstallationRetainsFleetTaskReservationWhenDurableAcceptanceFails(t *testing.T) {
	ctx := context.Background()
	daemon, kubeClient, _ := testManagedAgentInstallationDaemon(
		[]*pbgo.PackageState{{Package: packageDatadogOperator}},
		testFleetCredentialSecret(),
	)
	raw := testManagedAgentInstallationIntent(t, testAddonInstallOperationID, managedAgentInstallationDesiredStateInstalled)
	putManagedAgentInstallationIntentConfigMap(t, kubeClient, raw)
	daemon.managedAgentInstallationTaskReserved = false
	daemon.client = &rejectManagedAgentInstallationStateCreateClient{Client: daemon.client}

	err := daemon.handleManagedAgentInstallationIntent(ctx, managedAgentInstallationIntentSnapshot{raw: raw})

	require.ErrorContains(t, err, "transient managed Agent installation state create failure")
	assert.True(t, daemon.managedAgentInstallationTaskReserved)
	assert.False(t, daemon.managedAgentInstallationActive)
}

func TestManagedAgentInstallationRetriesWhenRunningStatePersistenceFails(t *testing.T) {
	ctx := context.Background()
	daemon, kubeClient, _ := testManagedAgentInstallationDaemon(
		[]*pbgo.PackageState{{Package: packageDatadogOperator}},
		testFleetCredentialSecret(),
	)
	daemon.managedAgentInstallationUpdates = make(chan struct{}, 1)
	raw := testManagedAgentInstallationIntent(t, testAddonInstallOperationID, managedAgentInstallationDesiredStateInstalled)
	putManagedAgentInstallationIntentConfigMap(t, kubeClient, raw)
	intent, config, digest, err := decodeManagedAgentInstallationIntent(raw, testManagedAgentInstallationIdentity)
	require.NoError(t, err)
	command := newManagedAgentInstallationCommand(intent, config, digest)
	require.NoError(t, daemon.writeManagedAgentInstallationState(ctx, managedAgentInstallationStateFromCommand(command, pbgo.TaskState_RUNNING, nil)))
	daemon.apiReader = &failManagedAgentInstallationTargetReadClient{Reader: kubeClient}
	daemon.client = &rejectManagedAgentInstallationRunningResultClient{Client: daemon.client}

	err = daemon.executeManagedAgentInstallationCommand(ctx, command)

	require.ErrorContains(t, err, "transient managed Agent installation running-state write failure")
	require.Eventually(t, func() bool {
		return len(daemon.managedAgentInstallationUpdates) == 1
	}, 2*time.Second, 10*time.Millisecond)
}

func TestManagedAgentInstallationUninstallWaitsForAndSupersedesActiveInstall(t *testing.T) {
	ctx := context.Background()
	daemon, kubeClient, _ := testManagedAgentInstallationDaemon(
		[]*pbgo.PackageState{{Package: packageDatadogOperator}},
		testFleetCredentialSecret(),
	)
	tasks := make(chan func(), 2)
	daemon.managedAgentInstallationTaskRunner = func(task func()) {
		tasks <- task
	}

	install := managedAgentInstallationIntentSnapshot{raw: testManagedAgentInstallationIntent(
		t,
		testAddonInstallOperationID,
		managedAgentInstallationDesiredStateInstalled,
	)}
	putManagedAgentInstallationIntentConfigMap(t, kubeClient, install.raw)
	require.NoError(t, daemon.handleManagedAgentInstallationIntent(ctx, install))
	installTask := <-tasks

	uninstall := managedAgentInstallationIntentSnapshot{raw: testManagedAgentInstallationIntent(
		t,
		testAddonUninstallOperationID,
		managedAgentInstallationDesiredStateAbsent,
	)}
	putManagedAgentInstallationIntentConfigMap(t, kubeClient, uninstall.raw)
	uninstallResult := make(chan error, 1)
	uninstallReachedActiveOperation := make(chan struct{})
	originalCancel := daemon.managedAgentInstallationCancel
	var signalOnce sync.Once
	daemon.managedAgentInstallationCancel = func() {
		signalOnce.Do(func() { close(uninstallReachedActiveOperation) })
		originalCancel()
	}
	go func() {
		uninstallResult <- daemon.handleManagedAgentInstallationIntent(ctx, uninstall)
	}()

	<-uninstallReachedActiveOperation
	select {
	case err := <-uninstallResult:
		require.Failf(t, "uninstall returned before install completed", "error: %v", err)
	default:
	}
	installTask()
	require.NoError(t, <-uninstallResult)
	persisted, err := daemon.readManagedAgentInstallationState(ctx)
	require.NoError(t, err)
	require.NotNil(t, persisted)
	assert.Equal(t, testAddonUninstallOperationID, persisted.OperationID)
	assert.Equal(t, managedAgentInstallationDesiredStateAbsent, persisted.DesiredState)
	assert.Equal(t, pbgo.TaskState_RUNNING, persisted.TaskState)

	uninstallTask := <-tasks
	uninstallTask()

	persisted, err = daemon.readManagedAgentInstallationState(ctx)
	require.NoError(t, err)
	require.NotNil(t, persisted)
	assert.Equal(t, testAddonUninstallOperationID, persisted.OperationID)
	assert.Equal(t, managedAgentInstallationDesiredStateAbsent, persisted.DesiredState)
	assert.Equal(t, pbgo.TaskState_DONE, persisted.TaskState)
}

func TestManagedAgentInstallationResultCannotOverwriteNewerOperation(t *testing.T) {
	ctx := context.Background()
	daemon, kubeClient, _ := testManagedAgentInstallationDaemon([]*pbgo.PackageState{{Package: packageDatadogOperator}})
	uninstallRaw := testManagedAgentInstallationIntent(t, testAddonUninstallOperationID, managedAgentInstallationDesiredStateAbsent)
	putManagedAgentInstallationIntentConfigMap(t, kubeClient, uninstallRaw)
	require.NoError(t, daemon.writeManagedAgentInstallationState(ctx, managedAgentInstallationPersistedState{
		Provider:       testManagedAgentInstallationIdentity.Provider(),
		InstallationID: testManagedAgentInstallationIdentity.InstallationID(),
		TargetID:       testManagedAgentInstallationIdentity.TargetID(),
		OperationID:    testAddonUninstallOperationID,
		Digest:         strings.Repeat("b", 64),
		DesiredState:   managedAgentInstallationDesiredStateAbsent,
		TaskState:      pbgo.TaskState_RUNNING,
	}))

	oldCommand := newManagedAgentInstallationCommand(managedAgentInstallationIntent{
		Version:        managedAgentInstallationVersion,
		Provider:       testManagedAgentInstallationIdentity.Provider(),
		InstallationID: testManagedAgentInstallationIdentity.InstallationID(),
		TargetID:       testManagedAgentInstallationIdentity.TargetID(),
		OperationID:    testAddonInstallOperationID,
		DesiredState:   managedAgentInstallationDesiredStateInstalled,
		Bootstrap:      managedAgentInstallationBootstrap{ClusterName: "test-cluster", Site: "datadoghq.com"},
	}, json.RawMessage(`{"spec":{}}`), strings.Repeat("a", 64))
	require.NoError(t, daemon.recordManagedAgentInstallationResult(ctx, oldCommand, pbgo.TaskState_DONE, nil))

	persisted, err := daemon.readManagedAgentInstallationState(ctx)
	require.NoError(t, err)
	require.NotNil(t, persisted)
	assert.Equal(t, testAddonUninstallOperationID, persisted.OperationID)
	assert.Equal(t, pbgo.TaskState_RUNNING, persisted.TaskState)
}

func TestValidateManagedAgentInstallationProgress(t *testing.T) {
	install := managedAgentInstallationIntent{
		Version:        managedAgentInstallationVersion,
		Provider:       testManagedAgentInstallationIdentity.Provider(),
		InstallationID: testManagedAgentInstallationIdentity.InstallationID(),
		TargetID:       testManagedAgentInstallationIdentity.TargetID(),
		OperationID:    testAddonInstallOperationID,
		DesiredState:   managedAgentInstallationDesiredStateInstalled,
	}
	current := &managedAgentInstallationPersistedState{
		Provider:       testManagedAgentInstallationIdentity.Provider(),
		InstallationID: testManagedAgentInstallationIdentity.InstallationID(),
		TargetID:       testManagedAgentInstallationIdentity.TargetID(),
		OperationID:    testAddonInstallOperationID,
		Digest:         "install-digest",
		DesiredState:   managedAgentInstallationDesiredStateInstalled,
		TaskState:      pbgo.TaskState_DONE,
	}

	require.NoError(t, validateManagedAgentInstallationProgress(nil, install, "install-digest"))
	require.NoError(t, validateManagedAgentInstallationProgress(current, install, "install-digest"))

	acknowledged := install
	acknowledged.AcknowledgedOperationID = testAddonInstallOperationID
	require.NoError(t, validateManagedAgentInstallationProgress(current, acknowledged, "install-digest"))
	current.AcknowledgedOperationID = testAddonInstallOperationID
	require.NoError(t, validateManagedAgentInstallationProgress(current, acknowledged, "install-digest"))
	require.Error(t, validateManagedAgentInstallationProgress(current, install, "install-digest"))

	uninstall := managedAgentInstallationIntent{
		Version:                 managedAgentInstallationVersion,
		Provider:                testManagedAgentInstallationIdentity.Provider(),
		InstallationID:          testManagedAgentInstallationIdentity.InstallationID(),
		TargetID:                testManagedAgentInstallationIdentity.TargetID(),
		OperationID:             testAddonUninstallOperationID,
		DesiredState:            managedAgentInstallationDesiredStateAbsent,
		AcknowledgedOperationID: testAddonInstallOperationID,
	}
	require.NoError(t, validateManagedAgentInstallationProgress(current, uninstall, "uninstall-digest"))
	uninstall.AcknowledgedOperationID = ""
	require.Error(t, validateManagedAgentInstallationProgress(current, uninstall, "uninstall-digest"))
	require.Error(t, validateManagedAgentInstallationProgress(nil, uninstall, "uninstall-digest"))

	current.OperationID = testAddonUninstallOperationID
	current.Digest = "uninstall-digest"
	current.DesiredState = managedAgentInstallationDesiredStateAbsent
	uninstall.AcknowledgedOperationID = testAddonInstallOperationID
	require.NoError(t, validateManagedAgentInstallationProgress(current, uninstall, "uninstall-digest"))
	require.Error(t, validateManagedAgentInstallationProgress(current, acknowledged, "install-digest"))
}

func TestManagedAgentInstallationIntentRejectsInstallOperationReplacement(t *testing.T) {
	ctx := context.Background()
	daemon, _, rcClient := testManagedAgentInstallationDaemon(
		[]*pbgo.PackageState{{Package: packageDatadogOperator}},
		testFleetCredentialSecret(),
	)

	install := managedAgentInstallationIntentSnapshot{raw: testManagedAgentInstallationIntent(
		t,
		testAddonInstallOperationID,
		managedAgentInstallationDesiredStateInstalled,
	)}
	putManagedAgentInstallationIntentConfigMap(t, daemon.client, install.raw)
	require.NoError(t, daemon.handleManagedAgentInstallationIntent(ctx, install))

	reused := managedAgentInstallationIntentSnapshot{raw: testManagedAgentInstallationIntent(
		t,
		testAddonUninstallOperationID,
		managedAgentInstallationDesiredStateInstalled,
	)}
	putManagedAgentInstallationIntentConfigMap(t, daemon.client, reused.raw)
	require.Error(t, daemon.handleManagedAgentInstallationIntent(ctx, reused))
	assert.Equal(t, testAddonUninstallOperationID, rcClient.state[0].GetTask().GetId())
	assert.Equal(t, pbgo.TaskState_INVALID_STATE, rcClient.state[0].GetTask().GetState())

	persisted, err := daemon.readManagedAgentInstallationState(ctx)
	require.NoError(t, err)
	require.NotNil(t, persisted)
	assert.Equal(t, testAddonInstallOperationID, persisted.OperationID)
}

func TestReadManagedAgentInstallationStateRejectsForeignOwnership(t *testing.T) {
	intent := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
		Namespace: managedAgentInstallationIntentKey.Namespace,
		Name:      managedAgentInstallationIntentKey.Name,
		UID:       "intent-uid",
	}}
	state := managedAgentInstallationStateData(managedAgentInstallationPersistedState{
		Provider:       testManagedAgentInstallationIdentity.Provider(),
		InstallationID: testManagedAgentInstallationIdentity.InstallationID(),
		TargetID:       testManagedAgentInstallationIdentity.TargetID(),
		OperationID:    testAddonInstallOperationID,
		Digest:         strings.Repeat("a", 64),
		DesiredState:   managedAgentInstallationDesiredStateInstalled,
		TaskState:      pbgo.TaskState_DONE,
	})
	foreign := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Namespace: managedAgentInstallationStateKey.Namespace, Name: managedAgentInstallationStateKey.Name},
		Data:       state,
	}
	d, _, _ := testManagedAgentInstallationDaemon(nil, intent, foreign)

	_, err := d.readManagedAgentInstallationState(context.Background())
	require.ErrorContains(t, err, "managed Agent installation state ownership")
}

func TestRehydrateManagedAgentInstallationStateKeepsTaskReserved(t *testing.T) {
	d, kubeClient, rcClient := testManagedAgentInstallationDaemon(
		[]*pbgo.PackageState{{Package: packageDatadogOperator, StableConfigVersion: testAddonInstallOperationID}},
	)
	acknowledgedIntent := testManagedAgentInstallationIntent(
		t,
		testAddonInstallOperationID,
		managedAgentInstallationDesiredStateInstalled,
		testAddonInstallOperationID,
	)
	putManagedAgentInstallationIntentConfigMap(t, kubeClient, acknowledgedIntent)
	_, _, digest, err := decodeManagedAgentInstallationIntent(acknowledgedIntent, testManagedAgentInstallationIdentity)
	require.NoError(t, err)
	require.NoError(t, d.writeManagedAgentInstallationState(context.Background(), managedAgentInstallationPersistedState{
		Provider:                testManagedAgentInstallationIdentity.Provider(),
		InstallationID:          testManagedAgentInstallationIdentity.InstallationID(),
		TargetID:                testManagedAgentInstallationIdentity.TargetID(),
		OperationID:             testAddonInstallOperationID,
		Digest:                  digest,
		DesiredState:            managedAgentInstallationDesiredStateInstalled,
		AcknowledgedOperationID: testAddonInstallOperationID,
		TaskState:               pbgo.TaskState_DONE,
	}))

	require.NoError(t, d.rehydrateManagedAgentInstallationState(context.Background()))

	require.True(t, d.managedAgentInstallationTaskReserved)
	require.Len(t, rcClient.state, 1)
	require.Equal(t, testAddonInstallOperationID, rcClient.state[0].GetStableConfigVersion())
	require.Nil(t, rcClient.state[0].GetTask())
}

func TestRehydrateManagedAgentInstallationRunningState(t *testing.T) {
	ctx := context.Background()
	d, kubeClient, rcClient := testManagedAgentInstallationDaemon(
		[]*pbgo.PackageState{{Package: packageDatadogOperator}},
	)
	raw := testManagedAgentInstallationIntent(t, testAddonInstallOperationID, managedAgentInstallationDesiredStateInstalled)
	putManagedAgentInstallationIntentConfigMap(t, kubeClient, raw)
	command := testManagedAgentInstallationCommand(t, testAddonInstallOperationID, managedAgentInstallationDesiredStateInstalled)
	require.NoError(t, d.writeManagedAgentInstallationState(ctx, managedAgentInstallationStateFromCommand(
		command,
		pbgo.TaskState_RUNNING,
		errors.New("waiting for dependency"),
	)))

	require.NoError(t, d.rehydrateManagedAgentInstallationState(ctx))

	require.Len(t, rcClient.state, 1)
	require.NotNil(t, rcClient.state[0].GetTask())
	assert.Equal(t, testAddonInstallOperationID, rcClient.state[0].GetTask().GetId())
	assert.Equal(t, pbgo.TaskState_RUNNING, rcClient.state[0].GetTask().GetState())
	require.NotNil(t, rcClient.state[0].GetTask().GetError())
	assert.Contains(t, rcClient.state[0].GetTask().GetError().GetMessage(), "waiting for dependency")
}

func TestRehydrateManagedAgentInstallationRejectsDifferentIdentity(t *testing.T) {
	ctx := context.Background()
	d, kubeClient, _ := testManagedAgentInstallationDaemon([]*pbgo.PackageState{{Package: packageDatadogOperator}})
	raw := testManagedAgentInstallationIntent(t, testAddonInstallOperationID, managedAgentInstallationDesiredStateInstalled)
	putManagedAgentInstallationIntentConfigMap(t, kubeClient, raw)
	command := testManagedAgentInstallationCommand(t, testAddonInstallOperationID, managedAgentInstallationDesiredStateInstalled)
	require.NoError(t, d.writeManagedAgentInstallationState(ctx, managedAgentInstallationStateFromCommand(command, pbgo.TaskState_RUNNING, nil)))
	d.managedAgentInstallationIdentity = NewEKSManagedAgentInstallationIdentity(
		"223e4567-e89b-42d3-a456-426614174000",
		testManagedAgentInstallationTargetHash,
	)

	require.ErrorContains(t, d.rehydrateManagedAgentInstallationState(ctx), "different installation")
}

func TestParseManagedAgentInstallationTaskState(t *testing.T) {
	for _, state := range []pbgo.TaskState{
		pbgo.TaskState_RUNNING,
		pbgo.TaskState_DONE,
		pbgo.TaskState_ERROR,
		pbgo.TaskState_INVALID_STATE,
	} {
		got, err := parseManagedAgentInstallationTaskState(state.String())
		require.NoError(t, err)
		assert.Equal(t, state, got)
	}
	_, err := parseManagedAgentInstallationTaskState("unknown")
	require.Error(t, err)
}

func TestManagedAgentInstallationAcknowledgementRetryReleasesReservation(t *testing.T) {
	ctx := context.Background()
	daemon, kubeClient, rcClient := testManagedAgentInstallationDaemon(
		[]*pbgo.PackageState{{Package: packageDatadogOperator}},
		testFleetCredentialSecret(),
	)
	install := managedAgentInstallationIntentSnapshot{raw: testManagedAgentInstallationIntent(
		t,
		testAddonInstallOperationID,
		managedAgentInstallationDesiredStateInstalled,
	)}
	putManagedAgentInstallationIntentConfigMap(t, kubeClient, install.raw)
	require.NoError(t, daemon.handleManagedAgentInstallationIntent(ctx, install))
	require.True(t, daemon.managedAgentInstallationTaskReserved)

	acknowledge := managedAgentInstallationIntentSnapshot{raw: testManagedAgentInstallationIntent(
		t,
		testAddonInstallOperationID,
		managedAgentInstallationDesiredStateInstalled,
		testAddonInstallOperationID,
	)}
	putManagedAgentInstallationIntentConfigMap(t, kubeClient, acknowledge.raw)
	rcClient.refreshResults = []error{fmt.Errorf("transient updater tag refresh")}
	require.ErrorContains(t, daemon.handleManagedAgentInstallationIntent(ctx, acknowledge), "transient updater tag refresh")
	require.False(t, daemon.managedAgentInstallationTaskReserved)

	require.NoError(t, daemon.handleManagedAgentInstallationIntent(ctx, acknowledge))
	require.False(t, daemon.managedAgentInstallationTaskReserved)
}

func TestManagedAgentInstallationAcknowledgementValidation(t *testing.T) {
	ctx := context.Background()
	command := testManagedAgentInstallationCommand(t, testAddonInstallOperationID, managedAgentInstallationDesiredStateInstalled)
	current := managedAgentInstallationStateFromCommand(command, pbgo.TaskState_DONE, nil)
	intent := command.Intent
	intent.AcknowledgedOperationID = testAddonInstallOperationID
	managedDDA := func(t *testing.T) *v2alpha1.DatadogAgent {
		t.Helper()
		dda := testFleetManagedDatadogAgent(t, "", testAddonInstallOperationID)
		spec, err := buildFleetDatadogAgentSpec(command.Config)
		require.NoError(t, err)
		dda.Spec = *spec
		hash, err := fleetDatadogAgentSpecHash(&dda.Spec)
		require.NoError(t, err)
		dda.Annotations[fleetConfigHashAnnotation] = hash
		return dda
	}

	t.Run("requires completed install", func(t *testing.T) {
		daemon, _, _ := testManagedAgentInstallationDaemon(nil)
		require.ErrorContains(t, daemon.acknowledgeManagedAgentInstallationInstall(ctx, nil, intent), "cannot be acknowledged before matching completion")
		running := current
		running.TaskState = pbgo.TaskState_RUNNING
		require.ErrorContains(t, daemon.acknowledgeManagedAgentInstallationInstall(ctx, &running, intent), "cannot be acknowledged before matching completion")
	})

	t.Run("rejects a different acknowledgement", func(t *testing.T) {
		daemon, _, _ := testManagedAgentInstallationDaemon(nil)
		conflicting := current
		conflicting.AcknowledgedOperationID = testAddonUninstallOperationID
		require.ErrorContains(t, daemon.acknowledgeManagedAgentInstallationInstall(ctx, &conflicting, intent), "already acknowledged by a different operation")
	})

	t.Run("requires target", func(t *testing.T) {
		daemon, _, _ := testManagedAgentInstallationDaemon(nil)
		require.ErrorContains(t, daemon.acknowledgeManagedAgentInstallationInstall(ctx, &current, intent), "read DatadogAgent before bootstrap acknowledgement")
	})

	t.Run("requires completed install gate", func(t *testing.T) {
		dda := managedDDA(t)
		dda.Labels[fleetManagedAgentInstallationStateLabel] = fleetManagedAgentInstallationStatePartial
		daemon, _, _ := testManagedAgentInstallationDaemon(nil, dda)
		require.ErrorContains(t, daemon.acknowledgeManagedAgentInstallationInstall(ctx, &current, intent), "not ready at install completion")
	})

	t.Run("requires Windows profile", func(t *testing.T) {
		dda := managedDDA(t)
		daemon, _, _ := testManagedAgentInstallationDaemon(nil, dda)
		require.ErrorContains(t, daemon.acknowledgeManagedAgentInstallationInstall(ctx, &current, intent), "Windows DatadogAgentProfile")
	})

	t.Run("reconcile requires complete acknowledgement", func(t *testing.T) {
		daemon, _, _ := testManagedAgentInstallationDaemon(nil)
		require.ErrorContains(t, daemon.reconcileAcknowledgedManagedAgentInstallation(ctx, &current), "acknowledgement state is incomplete")
	})

	t.Run("validated acknowledgement propagates target read failure", func(t *testing.T) {
		daemon, kubeClient, _ := testManagedAgentInstallationDaemon(nil)
		daemon.apiReader = &failManagedAgentInstallationTargetReadClient{Reader: kubeClient}
		_, err := daemon.validateAcknowledgedManagedAgentInstallation(ctx)
		require.ErrorContains(t, err, "transient managed Agent installation target read failure")
	})

	t.Run("validated acknowledgement requires completed install gate", func(t *testing.T) {
		dda := managedDDA(t)
		dda.Labels[fleetManagedAgentInstallationStateLabel] = fleetManagedAgentInstallationStatePartial
		daemon, _, _ := testManagedAgentInstallationDaemon(nil, dda)
		_, err := daemon.validateAcknowledgedManagedAgentInstallation(ctx)
		require.ErrorContains(t, err, "install gate")
	})

	t.Run("validated acknowledgement requires Windows profile", func(t *testing.T) {
		daemon, _, _ := testManagedAgentInstallationDaemon(nil, managedDDA(t))
		_, err := daemon.validateAcknowledgedManagedAgentInstallation(ctx)
		require.ErrorContains(t, err, "Windows DatadogAgentProfile")
	})
}

func TestWaitForManagedAgentInstallationSlotFailureModes(t *testing.T) {
	intent := managedAgentInstallationIntent{
		OperationID:  testAddonInstallOperationID,
		DesiredState: managedAgentInstallationDesiredStateInstalled,
	}

	t.Run("missing completion signal", func(t *testing.T) {
		daemon := &Daemon{managedAgentInstallationActive: true}
		require.ErrorContains(t, daemon.waitForManagedAgentInstallationSlot(context.Background(), intent), "no completion signal")
	})

	t.Run("context cancelled", func(t *testing.T) {
		daemon := &Daemon{managedAgentInstallationActive: true, managedAgentInstallationDone: make(chan struct{})}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		require.ErrorContains(t, daemon.waitForManagedAgentInstallationSlot(ctx, intent), "context canceled")
	})
}

func TestReconcileTerminalManagedAgentInstallationRejectsRunningState(t *testing.T) {
	daemon, _, _ := testManagedAgentInstallationDaemon([]*pbgo.PackageState{{Package: packageDatadogOperator}})
	current := &managedAgentInstallationPersistedState{
		OperationID: testAddonInstallOperationID,
		TaskState:   pbgo.TaskState_RUNNING,
	}

	err := daemon.reconcileTerminalManagedAgentInstallation(
		context.Background(),
		current,
		managedAgentInstallationIntent{},
		nil,
		"",
	)

	require.ErrorContains(t, err, "unsupported terminal state")
}

func TestRefreshManagedAgentInstallationUpdaterTagsRequiresRefreshableClient(t *testing.T) {
	daemon := &Daemon{}
	require.NoError(t, daemon.refreshManagedAgentInstallationUpdaterTags(context.Background()))

	daemon.rcClient = &managedAgentInstallationRCClientWithoutRefresh{}
	require.ErrorContains(t, daemon.refreshManagedAgentInstallationUpdaterTags(context.Background()), "does not support updater tag refresh")
}

func TestManagedAgentInstallationAcknowledgementPreservesNewerFleetState(t *testing.T) {
	ctx := context.Background()
	daemon, kubeClient, rcClient := testManagedAgentInstallationDaemon(
		[]*pbgo.PackageState{{Package: packageDatadogOperator}},
		testFleetCredentialSecret(),
	)
	install := managedAgentInstallationIntentSnapshot{raw: testManagedAgentInstallationIntent(
		t,
		testAddonInstallOperationID,
		managedAgentInstallationDesiredStateInstalled,
	)}
	putManagedAgentInstallationIntentConfigMap(t, kubeClient, install.raw)
	require.NoError(t, daemon.handleManagedAgentInstallationIntent(ctx, install))

	acknowledged := managedAgentInstallationIntentSnapshot{raw: testManagedAgentInstallationIntent(
		t,
		testAddonInstallOperationID,
		managedAgentInstallationDesiredStateInstalled,
		testAddonInstallOperationID,
	)}
	putManagedAgentInstallationIntentConfigMap(t, kubeClient, acknowledged.raw)
	require.NoError(t, daemon.handleManagedAgentInstallationIntent(ctx, acknowledged))

	daemon.setPackageConfigVersions(packageDatadogOperator, "fleet-config", "")
	daemon.setTaskState(packageDatadogOperator, "fleet-task", pbgo.TaskState_DONE, nil)
	daemon.taskMu.Lock()
	daemon.managedAgentInstallationTaskReserved = true
	daemon.taskMu.Unlock()

	require.NoError(t, daemon.handleManagedAgentInstallationIntent(ctx, acknowledged))

	require.Len(t, rcClient.state, 1)
	assert.Equal(t, "fleet-config", rcClient.state[0].GetStableConfigVersion())
	require.NotNil(t, rcClient.state[0].GetTask())
	assert.Equal(t, "fleet-task", rcClient.state[0].GetTask().GetId())
	assert.Equal(t, pbgo.TaskState_DONE, rcClient.state[0].GetTask().GetState())
	assert.False(t, daemon.managedAgentInstallationTaskReserved)
}

func TestManagedAgentInstallationReadinessTagsRequireDurableAcknowledgement(t *testing.T) {
	ctx := context.Background()
	daemon, kubeClient, _ := testManagedAgentInstallationDaemon(
		[]*pbgo.PackageState{{Package: packageDatadogOperator}},
		testFleetCredentialSecret(),
	)
	installIntent := testManagedAgentInstallationIntent(
		t,
		testAddonInstallOperationID,
		managedAgentInstallationDesiredStateInstalled,
	)
	putManagedAgentInstallationIntentConfigMap(t, kubeClient, installIntent)
	require.NoError(t, daemon.handleManagedAgentInstallationIntent(ctx, managedAgentInstallationIntentSnapshot{raw: installIntent}))

	acknowledgedIntent := testManagedAgentInstallationIntent(
		t,
		testAddonInstallOperationID,
		managedAgentInstallationDesiredStateInstalled,
		testAddonInstallOperationID,
	)
	putManagedAgentInstallationIntentConfigMap(t, kubeClient, acknowledgedIntent)
	require.NoError(t, daemon.handleManagedAgentInstallationIntent(ctx, managedAgentInstallationIntentSnapshot{raw: acknowledgedIntent}))

	tags, err := ManagedAgentInstallationReadinessTags(ctx, kubeClient, testManagedAgentInstallationIdentity, testManagedAgentInstallationNamespace)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{
		"managed_agent_installation_ack:" + testAddonInstallOperationID,
		"operator_config_updates:ready",
	}, tags)

	invalid := strings.Replace(string(acknowledgedIntent), `"version":"v1"`, `"version":"v1","unknown":true`, 1)
	putManagedAgentInstallationIntentConfigMap(t, kubeClient, []byte(invalid))
	_, err = ManagedAgentInstallationReadinessTags(ctx, kubeClient, testManagedAgentInstallationIdentity, testManagedAgentInstallationNamespace)
	require.Error(t, err)
}

func TestManagedAgentInstallationReadinessTagsPreserveAcknowledgementDuringUninstall(t *testing.T) {
	ctx := context.Background()
	daemon, kubeClient, _ := testManagedAgentInstallationDaemon(
		[]*pbgo.PackageState{{Package: packageDatadogOperator}},
		testFleetCredentialSecret(),
	)
	installIntent := testManagedAgentInstallationIntent(
		t,
		testAddonInstallOperationID,
		managedAgentInstallationDesiredStateInstalled,
	)
	putManagedAgentInstallationIntentConfigMap(t, kubeClient, installIntent)
	require.NoError(t, daemon.handleManagedAgentInstallationIntent(ctx, managedAgentInstallationIntentSnapshot{raw: installIntent}))

	acknowledgedIntent := testManagedAgentInstallationIntent(
		t,
		testAddonInstallOperationID,
		managedAgentInstallationDesiredStateInstalled,
		testAddonInstallOperationID,
	)
	putManagedAgentInstallationIntentConfigMap(t, kubeClient, acknowledgedIntent)
	require.NoError(t, daemon.handleManagedAgentInstallationIntent(ctx, managedAgentInstallationIntentSnapshot{raw: acknowledgedIntent}))

	uninstallIntent := testManagedAgentInstallationIntent(
		t,
		testAddonUninstallOperationID,
		managedAgentInstallationDesiredStateAbsent,
		testAddonInstallOperationID,
	)
	putManagedAgentInstallationIntentConfigMap(t, kubeClient, uninstallIntent)
	wantTags := []string{"managed_agent_installation_ack:" + testAddonInstallOperationID}

	tags, err := ManagedAgentInstallationReadinessTags(ctx, kubeClient, testManagedAgentInstallationIdentity, testManagedAgentInstallationNamespace)
	require.NoError(t, err)
	require.Equal(t, wantTags, tags)

	require.NoError(t, daemon.handleManagedAgentInstallationIntent(ctx, managedAgentInstallationIntentSnapshot{raw: uninstallIntent}))
	tags, err = ManagedAgentInstallationReadinessTags(ctx, kubeClient, testManagedAgentInstallationIdentity, testManagedAgentInstallationNamespace)
	require.NoError(t, err)
	require.Equal(t, wantTags, tags)
}

func TestManagedAgentInstallationReadinessTagsRejectsIncompleteState(t *testing.T) {
	ctx := context.Background()
	tags, err := ManagedAgentInstallationReadinessTags(ctx, nil, testManagedAgentInstallationIdentity, testManagedAgentInstallationNamespace)
	require.NoError(t, err)
	assert.Empty(t, tags)
	tags, err = ManagedAgentInstallationReadinessTags(ctx, nil, ManagedAgentInstallationIdentity{}, "")
	require.NoError(t, err)
	assert.Empty(t, tags)

	daemon, kubeClient, _ := testManagedAgentInstallationDaemon(
		[]*pbgo.PackageState{{Package: packageDatadogOperator}},
		testFleetCredentialSecret(),
	)
	_, err = ManagedAgentInstallationReadinessTags(ctx, kubeClient, testManagedAgentInstallationIdentity, testManagedAgentInstallationNamespace)
	require.ErrorContains(t, err, "read managed Agent installation intent")

	installIntent := testManagedAgentInstallationIntent(t, testAddonInstallOperationID, managedAgentInstallationDesiredStateInstalled)
	putManagedAgentInstallationIntentConfigMap(t, kubeClient, installIntent)
	tags, err = ManagedAgentInstallationReadinessTags(ctx, kubeClient, testManagedAgentInstallationIdentity, testManagedAgentInstallationNamespace)
	require.NoError(t, err)
	assert.Empty(t, tags)

	acknowledgedIntent := testManagedAgentInstallationIntent(
		t,
		testAddonInstallOperationID,
		managedAgentInstallationDesiredStateInstalled,
		testAddonInstallOperationID,
	)
	putManagedAgentInstallationIntentConfigMap(t, kubeClient, acknowledgedIntent)
	_, err = ManagedAgentInstallationReadinessTags(ctx, kubeClient, testManagedAgentInstallationIdentity, testManagedAgentInstallationNamespace)
	require.ErrorContains(t, err, "acknowledgement state is not consistent")

	putManagedAgentInstallationIntentConfigMap(t, kubeClient, installIntent)
	require.NoError(t, daemon.handleManagedAgentInstallationIntent(ctx, managedAgentInstallationIntentSnapshot{raw: installIntent}))
	putManagedAgentInstallationIntentConfigMap(t, kubeClient, acknowledgedIntent)
	require.NoError(t, daemon.handleManagedAgentInstallationIntent(ctx, managedAgentInstallationIntentSnapshot{raw: acknowledgedIntent}))
	state, err := daemon.readManagedAgentInstallationState(ctx)
	require.NoError(t, err)
	require.NotNil(t, state)
	state.TaskState = pbgo.TaskState_RUNNING
	require.NoError(t, daemon.writeManagedAgentInstallationState(ctx, *state))
	_, err = ManagedAgentInstallationReadinessTags(ctx, kubeClient, testManagedAgentInstallationIdentity, testManagedAgentInstallationNamespace)
	require.ErrorContains(t, err, "not yet consistent")

	state.TaskState = pbgo.TaskState_DONE
	require.NoError(t, daemon.writeManagedAgentInstallationState(ctx, *state))
	dda := &v2alpha1.DatadogAgent{}
	require.NoError(t, kubeClient.Get(ctx, managedAgentInstallationTarget, dda))
	require.NoError(t, kubeClient.Delete(ctx, dda))
	_, err = ManagedAgentInstallationReadinessTags(ctx, kubeClient, testManagedAgentInstallationIdentity, testManagedAgentInstallationNamespace)
	require.ErrorContains(t, err, "target is absent")
}

func TestManagedAgentInstallationPersistedStateFailures(t *testing.T) {
	ctx := context.Background()
	newDaemonWithIntent := func(t *testing.T) (*Daemon, client.Client, managedAgentInstallationPersistedState) {
		t.Helper()
		daemon, kubeClient, _ := testManagedAgentInstallationDaemon(nil)
		raw := testManagedAgentInstallationIntent(t, testAddonInstallOperationID, managedAgentInstallationDesiredStateInstalled)
		putManagedAgentInstallationIntentConfigMap(t, kubeClient, raw)
		command := testManagedAgentInstallationCommand(t, testAddonInstallOperationID, managedAgentInstallationDesiredStateInstalled)
		return daemon, kubeClient, managedAgentInstallationStateFromCommand(command, pbgo.TaskState_DONE, nil)
	}

	t.Run("result requires existing operation", func(t *testing.T) {
		daemon, _, state := newDaemonWithIntent(t)
		require.ErrorContains(t, daemon.writeManagedAgentInstallationResult(ctx, state), "state is missing")
	})

	t.Run("new state requires intent owner", func(t *testing.T) {
		daemon, _, _ := testManagedAgentInstallationDaemon(nil)
		command := testManagedAgentInstallationCommand(t, testAddonInstallOperationID, managedAgentInstallationDesiredStateInstalled)
		state := managedAgentInstallationStateFromCommand(command, pbgo.TaskState_RUNNING, nil)
		require.ErrorContains(t, daemon.writeManagedAgentInstallationState(ctx, state), "intent owner")
	})

	t.Run("state read failure", func(t *testing.T) {
		daemon, kubeClient, state := newDaemonWithIntent(t)
		require.NoError(t, daemon.writeManagedAgentInstallationState(ctx, state))
		daemon.apiReader = &managedAgentInstallationFaultClient{
			Client: kubeClient,
			getError: func(key client.ObjectKey, _ client.Object) error {
				if key == managedAgentInstallationStateKey {
					return errors.New("state read failed")
				}
				return nil
			},
		}
		_, err := daemon.readManagedAgentInstallationState(ctx)
		require.ErrorContains(t, err, "state read failed")
	})

	t.Run("state write read failure", func(t *testing.T) {
		daemon, kubeClient, state := newDaemonWithIntent(t)
		require.NoError(t, daemon.writeManagedAgentInstallationState(ctx, state))
		daemon.client = &managedAgentInstallationFaultClient{
			Client: kubeClient,
			getError: func(key client.ObjectKey, _ client.Object) error {
				if key == managedAgentInstallationStateKey {
					return errors.New("state write read failed")
				}
				return nil
			},
		}
		require.ErrorContains(t, daemon.writeManagedAgentInstallationState(ctx, state), "state write read failed")
	})

	t.Run("state owner read failure", func(t *testing.T) {
		daemon, kubeClient, state := newDaemonWithIntent(t)
		require.NoError(t, daemon.writeManagedAgentInstallationState(ctx, state))
		daemon.apiReader = &managedAgentInstallationFaultClient{
			Client: kubeClient,
			getError: func(key client.ObjectKey, _ client.Object) error {
				if key == managedAgentInstallationIntentKey {
					return errors.New("intent owner read failed")
				}
				return nil
			},
		}
		_, err := daemon.readManagedAgentInstallationState(ctx)
		require.ErrorContains(t, err, "intent owner read failed")
	})

	for _, test := range []struct {
		name      string
		mutate    func(map[string]string)
		wantError string
	}{
		{
			name: "invalid task state",
			mutate: func(data map[string]string) {
				data[managedAgentInstallationStateTaskStateKey] = "UNKNOWN"
			},
			wantError: "unsupported task state",
		},
		{
			name: "incomplete state",
			mutate: func(data map[string]string) {
				delete(data, managedAgentInstallationStateDigestKey)
			},
			wantError: "state is incomplete",
		},
		{
			name: "unsupported desired state",
			mutate: func(data map[string]string) {
				data[managedAgentInstallationStateDesiredStateKey] = "unknown"
			},
			wantError: "unsupported desired state",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			daemon, kubeClient, state := newDaemonWithIntent(t)
			require.NoError(t, daemon.writeManagedAgentInstallationState(ctx, state))
			configMap := &corev1.ConfigMap{}
			require.NoError(t, kubeClient.Get(ctx, managedAgentInstallationStateKey, configMap))
			test.mutate(configMap.Data)
			require.NoError(t, kubeClient.Update(ctx, configMap))
			_, err := daemon.readManagedAgentInstallationState(ctx)
			require.ErrorContains(t, err, test.wantError)
		})
	}
}

func TestManagedAgentInstallationReadinessTagsRejectStateFailures(t *testing.T) {
	ctx := context.Background()

	t.Run("state read failure", func(t *testing.T) {
		_, kubeClient, _ := testManagedAgentInstallationDaemon(nil)
		acknowledged := testManagedAgentInstallationIntent(
			t,
			testAddonInstallOperationID,
			managedAgentInstallationDesiredStateInstalled,
			testAddonInstallOperationID,
		)
		putManagedAgentInstallationIntentConfigMap(t, kubeClient, acknowledged)
		reader := &managedAgentInstallationFaultClient{
			Client: kubeClient,
			getError: func(key client.ObjectKey, _ client.Object) error {
				if key == managedAgentInstallationStateKey {
					return errors.New("state read failed")
				}
				return nil
			},
		}

		_, err := ManagedAgentInstallationReadinessTags(ctx, reader, testManagedAgentInstallationIdentity, testManagedAgentInstallationNamespace)
		require.ErrorContains(t, err, "state read failed")
	})

	t.Run("inconsistent uninstall", func(t *testing.T) {
		daemon, kubeClient, _ := testManagedAgentInstallationDaemon(nil)
		uninstall := testManagedAgentInstallationIntent(
			t,
			testAddonUninstallOperationID,
			managedAgentInstallationDesiredStateAbsent,
			testAddonInstallOperationID,
		)
		putManagedAgentInstallationIntentConfigMap(t, kubeClient, uninstall)
		command := testManagedAgentInstallationCommand(t, testAddonUninstallOperationID, managedAgentInstallationDesiredStateAbsent)
		command.Intent.AcknowledgedOperationID = testAddonInstallOperationID
		state := managedAgentInstallationStateFromCommand(command, pbgo.TaskState_DONE, nil)
		state.OperationID = "other-operation"
		state.Digest = strings.Repeat("f", 64)
		require.NoError(t, daemon.writeManagedAgentInstallationState(ctx, state))

		_, err := ManagedAgentInstallationReadinessTags(ctx, kubeClient, testManagedAgentInstallationIdentity, testManagedAgentInstallationNamespace)
		require.ErrorContains(t, err, "not consistent with the uninstall intent")
	})
}

func TestManagedAgentInstallationIntentWorkerStopsOnReadFailure(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
	}{
		{
			name: "missing intent",
			err:  apierrors.NewNotFound(corev1.Resource("configmaps"), managedAgentInstallationIntentKey.Name),
		},
		{name: "transient error", err: errors.New("intent read failed")},
	} {
		t.Run(test.name, func(t *testing.T) {
			daemon, kubeClient, _ := testManagedAgentInstallationDaemon(nil)
			daemon.managedAgentInstallationUpdates = make(chan struct{}, 1)
			read := make(chan struct{}, 1)
			daemon.apiReader = &managedAgentInstallationFaultClient{
				Client: kubeClient,
				getError: func(key client.ObjectKey, _ client.Object) error {
					if key != managedAgentInstallationIntentKey {
						return nil
					}
					select {
					case read <- struct{}{}:
					default:
					}
					return test.err
				},
			}
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan struct{})
			go func() {
				daemon.runManagedAgentInstallationIntentWorker(ctx)
				close(done)
			}()
			daemon.requestManagedAgentInstallationRetry()
			select {
			case <-read:
			case <-time.After(time.Second):
				require.Fail(t, "worker did not read the intent")
			}
			cancel()
			select {
			case <-done:
			case <-time.After(time.Second):
				require.Fail(t, "worker did not stop")
			}
		})
	}
}

func TestManagedAgentInstallationForwardersIgnoreUnrelatedObjects(t *testing.T) {
	daemon := &Daemon{
		managedAgentInstallationNamespace: testManagedAgentInstallationNamespace,
		managedAgentInstallationUpdates:   make(chan struct{}, 1),
	}

	daemon.forwardManagedAgentInstallationIntent(&corev1.Secret{})
	daemon.forwardManagedAgentInstallationIntent(&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "other", Namespace: testManagedAgentInstallationNamespace}})
	assert.Empty(t, daemon.managedAgentInstallationUpdates)
	daemon.forwardManagedAgentInstallationIntent(&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: managedAgentInstallationIntentKey.Name, Namespace: managedAgentInstallationIntentKey.Namespace}})
	require.Len(t, daemon.managedAgentInstallationUpdates, 1)
	<-daemon.managedAgentInstallationUpdates

	daemon.forwardManagedAgentInstallationCredential(&corev1.ConfigMap{})
	daemon.forwardManagedAgentInstallationCredential(&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "other", Namespace: testManagedAgentInstallationNamespace}})
	daemon.forwardManagedAgentInstallationCredential(&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: managedAgentInstallationCredentialKey.Name, Namespace: managedAgentInstallationCredentialKey.Namespace}})
	assert.Empty(t, daemon.managedAgentInstallationUpdates)
	daemon.forwardManagedAgentInstallationCredential(testFleetCredentialSecret())
	require.Len(t, daemon.managedAgentInstallationUpdates, 1)

	(&Daemon{}).requestManagedAgentInstallationRetry()
}

func TestBoundedTaskErrorMessage(t *testing.T) {
	assert.Empty(t, boundedTaskErrorMessage(nil))
	assert.Equal(t, "short error", boundedTaskErrorMessage(errors.New("short error")))
	assert.Equal(t, strings.Repeat("x", 1024), boundedTaskErrorMessage(errors.New(strings.Repeat("x", 1025))))
	assert.Equal(t, "?", boundedTaskErrorMessage(errors.New(string([]byte{0xff}))))
}

func TestDecodeManagedAgentInstallationIntentRejectsUnsupportedProvider(t *testing.T) {
	identity := newManagedAgentInstallationIdentity(testManagedAgentInstallationProviderIdentity{})

	_, _, _, err := decodeManagedAgentInstallationIntent([]byte(`{}`), identity)
	require.ErrorContains(t, err, "unsupported managed Agent installation provider")
}

func testManagedAgentInstallationIntent(t *testing.T, operationID string, desiredState managedAgentInstallationDesiredState, acknowledgedOperationID ...string) []byte {
	t.Helper()
	payload := eksManagedAgentInstallationIntent{
		Version:        managedAgentInstallationVersion,
		InstallationID: testManagedAgentInstallationIdentity.InstallationID(),
		EKSARNSHA256:   testManagedAgentInstallationTargetHash,
		OperationID:    operationID,
		DesiredState:   desiredState,
		Bootstrap: managedAgentInstallationBootstrap{
			ClusterName: "test-cluster",
			Site:        "datadoghq.com",
		},
	}
	if len(acknowledgedOperationID) > 0 {
		payload.AcknowledgedOperationID = acknowledgedOperationID[0]
	}
	raw, err := json.Marshal(payload)
	require.NoError(t, err)
	return raw
}

func putManagedAgentInstallationIntentConfigMap(t *testing.T, kubeClient client.Client, raw []byte) {
	t.Helper()
	ctx := context.Background()
	configMap := &corev1.ConfigMap{}
	err := kubeClient.Get(ctx, managedAgentInstallationIntentKey, configMap)
	if apierrors.IsNotFound(err) {
		require.NoError(t, kubeClient.Create(ctx, &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Namespace: managedAgentInstallationIntentKey.Namespace, Name: managedAgentInstallationIntentKey.Name, UID: "intent-uid"},
			Data:       map[string]string{managedAgentInstallationIntentDataKey: string(raw)},
		}))
		return
	}
	require.NoError(t, err)
	configMap.Data[managedAgentInstallationIntentDataKey] = string(raw)
	require.NoError(t, kubeClient.Update(ctx, configMap))
}

func testRestartedManagedAgentInstallationDaemon(kubeClient client.Client) *Daemon {
	return &Daemon{
		rcClient:                             &mockRCClient{state: []*pbgo.PackageState{{Package: packageDatadogOperator}}},
		client:                               kubeClient,
		apiReader:                            kubeClient,
		managedAgentInstallationIdentity:     testManagedAgentInstallationIdentity,
		managedAgentInstallationNamespace:    testManagedAgentInstallationNamespace,
		managedAgentInstallationTaskReserved: true,
		configs:                              make(map[string]installerConfig),
		statusUpdates:                        make(chan ddaStatusSnapshot, 32),
	}
}
