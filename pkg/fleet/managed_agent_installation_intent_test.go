// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fleet

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
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
		name string
		raw  []byte
	}{
		{
			name: "mismatched installation",
			raw:  []byte(strings.Replace(valid, testManagedAgentInstallationIdentity.InstallationID(), "223e4567-e89b-42d3-a456-426614174000", 1)),
		},
		{
			name: "noncanonical operation",
			raw:  []byte(strings.Replace(valid, testAddonInstallOperationID, strings.ToUpper(testAddonInstallOperationID), 1)),
		},
		{
			name: "caller managed generation",
			raw:  []byte(strings.Replace(valid, `"desiredState":"installed"`, `"generation":1,"desiredState":"installed"`, 1)),
		},
		{
			name: "mismatched EKS ARN",
			raw:  []byte(strings.Replace(valid, testManagedAgentInstallationTargetHash, strings.Repeat("f", 64), 1)),
		},
		{
			name: "topology input",
			raw:  []byte(strings.Replace(valid, `"site":"datadoghq.com"`, `"site":"datadoghq.com","topology":"mixed"`, 1)),
		},
		{
			name: "unknown field",
			raw:  []byte(strings.Replace(valid, `"version":"v1"`, `"version":"v1","extra":true`, 1)),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, _, err := decodeManagedAgentInstallationIntent(test.raw, testManagedAgentInstallationIdentity)
			require.Error(t, err)
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
		managedAgentInstallationUpdates: make(chan struct{}, 1),
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
	require.Equal(t, testAddonInstallOperationID, rcClient.state[0].GetTask().GetId())
	require.Equal(t, pbgo.TaskState_DONE, rcClient.state[0].GetTask().GetState())
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

func TestManagedAgentInstallationReadinessTagsRequireDurableAcknowledgement(t *testing.T) {
	ctx := context.Background()
	daemon, kubeClient, _ := testManagedAgentInstallationDaemon(nil)
	acknowledgedIntent := testManagedAgentInstallationIntent(
		t,
		testAddonInstallOperationID,
		managedAgentInstallationDesiredStateInstalled,
		testAddonInstallOperationID,
	)
	putManagedAgentInstallationIntentConfigMap(t, kubeClient, acknowledgedIntent)
	require.NoError(t, daemon.writeManagedAgentInstallationState(ctx, managedAgentInstallationPersistedState{
		Provider:                testManagedAgentInstallationIdentity.Provider(),
		InstallationID:          testManagedAgentInstallationIdentity.InstallationID(),
		TargetID:                testManagedAgentInstallationIdentity.TargetID(),
		OperationID:             testAddonInstallOperationID,
		Digest:                  strings.Repeat("a", 64),
		DesiredState:            managedAgentInstallationDesiredStateInstalled,
		AcknowledgedOperationID: testAddonInstallOperationID,
		TaskState:               pbgo.TaskState_DONE,
	}))

	tags, err := ManagedAgentInstallationReadinessTags(ctx, kubeClient, testManagedAgentInstallationIdentity)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{
		"managed_agent_installation_ack:" + testAddonInstallOperationID,
		"operator_config_updates:ready",
	}, tags)

	invalid := strings.Replace(string(acknowledgedIntent), `"version":"v1"`, `"version":"v1","unknown":true`, 1)
	putManagedAgentInstallationIntentConfigMap(t, kubeClient, []byte(invalid))
	_, err = ManagedAgentInstallationReadinessTags(ctx, kubeClient, testManagedAgentInstallationIdentity)
	require.Error(t, err)
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
		managedAgentInstallationTaskReserved: true,
		configs:                              make(map[string]installerConfig),
		statusUpdates:                        make(chan ddaStatusSnapshot, 32),
	}
}
