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
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	v2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

const (
	testAddonInstallOperationID   = "123e4567-e89b-42d3-a456-426614174010"
	testAddonUninstallOperationID = "123e4567-e89b-42d3-a456-426614174011"
)

type rejectAddonLifecycleTerminalStateClient struct {
	client.Client
}

func (c *rejectAddonLifecycleTerminalStateClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	configMap, ok := obj.(*corev1.ConfigMap)
	if ok && client.ObjectKeyFromObject(configMap) == addonLifecycleStateKey && configMap.Data[addonLifecycleStateTaskStateKey] != pbgo.TaskState_RUNNING.String() {
		return fmt.Errorf("transient lifecycle state write failure")
	}
	return c.Client.Patch(ctx, obj, patch, opts...)
}

func TestDecodeAddonLifecycleIntent(t *testing.T) {
	raw := testAddonLifecycleIntent(t, testAddonInstallOperationID, addonLifecycleDesiredStateInstalled)

	intent, config, digest, err := decodeAddonLifecycleIntent(raw, testLifecycleIdentity)

	require.NoError(t, err)
	assert.Equal(t, testAddonInstallOperationID, intent.OperationID)
	var decoded datadogAgentLifecycleConfig
	require.NoError(t, json.Unmarshal(config, &decoded))
	require.NotNil(t, decoded.Spec)
	require.NotNil(t, decoded.Spec.Global)
	assert.Equal(t, "test-cluster", *decoded.Spec.Global.ClusterName)
	assert.Equal(t, "datadoghq.com", *decoded.Spec.Global.Site)
	assert.Equal(t, map[string]string{corev1.LabelOSStable: string(corev1.Linux)}, decoded.Spec.Override[v2alpha1.NodeAgentComponentName].NodeSelector)
	assert.Len(t, digest, 64)

	_, _, repeatedDigest, err := decodeAddonLifecycleIntent(raw, testLifecycleIdentity)
	require.NoError(t, err)
	assert.Equal(t, digest, repeatedDigest)
}

func TestDecodeAddonLifecycleIntentRejectsUnsafeInput(t *testing.T) {
	valid := fmt.Sprintf(`{"version":"v1","installationID":"%s","eksARNSHA256":"%s","operationID":"%s","desiredState":"installed","bootstrap":{"clusterName":"test-cluster","site":"datadoghq.com"}}`, testLifecycleIdentity.InstallationID, testLifecycleIdentity.TargetHash, testAddonInstallOperationID)
	tests := []struct {
		name string
		raw  []byte
	}{
		{
			name: "mismatched installation",
			raw:  []byte(strings.Replace(valid, testLifecycleIdentity.InstallationID, "223e4567-e89b-42d3-a456-426614174000", 1)),
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
			raw:  []byte(strings.Replace(valid, testLifecycleIdentity.TargetHash, strings.Repeat("f", 64), 1)),
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
			_, _, _, err := decodeAddonLifecycleIntent(test.raw, testLifecycleIdentity)
			require.Error(t, err)
		})
	}
}

func TestAddonLifecycleIntentInstallAndUninstall(t *testing.T) {
	ctx := context.Background()
	daemon, kubeClient, rcClient := testLifecycleDaemon(
		nil,
		[]*pbgo.PackageState{{Package: packageDatadogOperator}},
		testFleetCredentialSecret(),
	)
	daemon.addonConfigs = make(map[string]installerConfig)

	install := addonLifecycleIntentSnapshot{raw: testAddonLifecycleIntent(
		t,
		testAddonInstallOperationID,
		addonLifecycleDesiredStateInstalled,
	)}
	putAddonLifecycleIntentConfigMap(t, kubeClient, install.raw)
	require.NoError(t, daemon.handleAddonLifecycleIntent(ctx, install))

	dda := &v2alpha1.DatadogAgent{}
	require.NoError(t, kubeClient.Get(ctx, testDDANSN, dda))
	assert.Equal(t, testAddonInstallOperationID, dda.Labels[fleetConfigIDLabel])
	assert.Equal(t, testLifecycleIdentity.InstallationID, dda.Labels[fleetInstallationIDLabel])
	assert.Equal(t, testLifecycleIdentity.TargetID(), dda.Labels[fleetTargetIDLabel])
	assert.Equal(t, testAddonInstallOperationID, rcClient.state[0].GetTask().GetId())
	assert.Equal(t, pbgo.TaskState_DONE, rcClient.state[0].GetTask().GetState())
	assert.Equal(t, testAddonInstallOperationID, rcClient.state[0].GetStableConfigVersion())
	profile := &v1alpha1.DatadogAgentProfile{}
	require.NoError(t, kubeClient.Get(ctx, addonLifecycleWindowsProfileKey, profile))
	assert.Equal(t, testLifecycleIdentity.InstallationID, profile.Labels[fleetInstallationIDLabel])
	assert.Equal(t, testLifecycleIdentity.TargetID(), profile.Labels[fleetTargetIDLabel])

	acknowledge := addonLifecycleIntentSnapshot{raw: testAddonLifecycleIntent(
		t,
		testAddonInstallOperationID,
		addonLifecycleDesiredStateInstalled,
		testAddonInstallOperationID,
	)}
	putAddonLifecycleIntentConfigMap(t, kubeClient, acknowledge.raw)
	refreshCallsBeforeAcknowledgement := rcClient.refreshCalls
	require.NoError(t, daemon.handleAddonLifecycleIntent(ctx, acknowledge))
	assert.Equal(t, refreshCallsBeforeAcknowledgement+2, rcClient.refreshCalls)

	uninstall := addonLifecycleIntentSnapshot{raw: testAddonLifecycleIntent(
		t,
		testAddonUninstallOperationID,
		addonLifecycleDesiredStateAbsent,
		testAddonInstallOperationID,
	)}
	putAddonLifecycleIntentConfigMap(t, kubeClient, uninstall.raw)
	require.NoError(t, daemon.handleAddonLifecycleIntent(ctx, uninstall))

	err := kubeClient.Get(ctx, testDDANSN, &v2alpha1.DatadogAgent{})
	require.True(t, apierrors.IsNotFound(err))
	assert.Equal(t, testAddonUninstallOperationID, rcClient.state[0].GetTask().GetId())
	assert.Equal(t, pbgo.TaskState_DONE, rcClient.state[0].GetTask().GetState())
	assert.Empty(t, rcClient.state[0].GetStableConfigVersion())

	persisted, err := daemon.readAddonLifecycleState(ctx)
	require.NoError(t, err)
	require.NotNil(t, persisted)
	assert.Equal(t, testAddonUninstallOperationID, persisted.OperationID)
	assert.Equal(t, testAddonInstallOperationID, persisted.AcknowledgedOperationID)
	assert.Equal(t, pbgo.TaskState_DONE, persisted.TaskState)
}

func TestAddonLifecycleIntentWorkerRetriesTransientFailure(t *testing.T) {
	originalRetryInterval := addonLifecycleRetryInterval
	addonLifecycleRetryInterval = time.Millisecond
	t.Cleanup(func() { addonLifecycleRetryInterval = originalRetryInterval })

	daemon, kubeClient, rcClient := testLifecycleDaemon(
		nil,
		[]*pbgo.PackageState{{Package: packageDatadogOperator}},
		testFleetCredentialSecret(),
	)
	daemon.addonConfigs = make(map[string]installerConfig)
	daemon.addonLifecycleUpdates = make(chan addonLifecycleIntentSnapshot)
	daemon.addonLifecycleRetries = make(chan struct{}, 1)
	rcClient.refreshErr = fmt.Errorf("transient updater tag refresh")
	rcClient.refreshFailures = 1
	raw := testAddonLifecycleIntent(t, testAddonInstallOperationID, addonLifecycleDesiredStateInstalled)
	putAddonLifecycleIntentConfigMap(t, kubeClient, raw)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go daemon.runAddonLifecycleIntentWorker(ctx)
	daemon.addonLifecycleUpdates <- addonLifecycleIntentSnapshot{raw: raw, resourceVersion: "1"}

	require.Eventually(t, func() bool {
		persisted, err := daemon.readAddonLifecycleState(context.Background())
		return err == nil && persisted != nil && persisted.TaskState == pbgo.TaskState_DONE
	}, time.Second, time.Millisecond)
	assert.GreaterOrEqual(t, rcClient.refreshCalls, 2)
}

func TestAddonLifecycleTerminalStateIsDurableBeforeRemoteConfigCompletion(t *testing.T) {
	ctx := context.Background()
	daemon, kubeClient, rcClient := testLifecycleDaemon(
		nil,
		[]*pbgo.PackageState{{Package: packageDatadogOperator}},
		testFleetCredentialSecret(),
	)
	daemon.addonConfigs = make(map[string]installerConfig)
	daemon.addonLifecycleRetries = make(chan struct{}, 1)
	daemon.client = &rejectAddonLifecycleTerminalStateClient{Client: daemon.client}
	raw := testAddonLifecycleIntent(t, testAddonInstallOperationID, addonLifecycleDesiredStateInstalled)
	putAddonLifecycleIntentConfigMap(t, kubeClient, raw)

	err := daemon.handleAddonLifecycleIntent(ctx, addonLifecycleIntentSnapshot{raw: raw})

	require.ErrorContains(t, err, "transient lifecycle state write failure")
	require.Len(t, rcClient.state, 1)
	require.NotNil(t, rcClient.state[0].GetTask())
	assert.Equal(t, pbgo.TaskState_RUNNING, rcClient.state[0].GetTask().GetState())
	assert.Equal(t, fleetPartialConfigVersionPrefix+testAddonInstallOperationID, rcClient.state[0].GetStableConfigVersion())
	persisted, readErr := daemon.readAddonLifecycleState(ctx)
	require.NoError(t, readErr)
	require.NotNil(t, persisted)
	assert.Equal(t, pbgo.TaskState_RUNNING, persisted.TaskState)
	assert.Len(t, daemon.addonLifecycleRetries, 1)
}

func TestAddonLifecycleUninstallWaitsForAndSupersedesActiveInstall(t *testing.T) {
	ctx := context.Background()
	daemon, kubeClient, _ := testLifecycleDaemon(
		nil,
		[]*pbgo.PackageState{{Package: packageDatadogOperator}},
		testFleetCredentialSecret(),
	)
	daemon.addonConfigs = make(map[string]installerConfig)
	tasks := make(chan func(), 2)
	daemon.lifecycleTaskRunner = func(task func()) {
		tasks <- task
	}

	install := addonLifecycleIntentSnapshot{raw: testAddonLifecycleIntent(
		t,
		testAddonInstallOperationID,
		addonLifecycleDesiredStateInstalled,
	)}
	putAddonLifecycleIntentConfigMap(t, kubeClient, install.raw)
	require.NoError(t, daemon.handleAddonLifecycleIntent(ctx, install))
	installTask := <-tasks

	uninstall := addonLifecycleIntentSnapshot{raw: testAddonLifecycleIntent(
		t,
		testAddonUninstallOperationID,
		addonLifecycleDesiredStateAbsent,
	)}
	putAddonLifecycleIntentConfigMap(t, kubeClient, uninstall.raw)
	uninstallResult := make(chan error, 1)
	uninstallReachedActiveOperation := make(chan struct{})
	originalCancel := daemon.lifecycleCancel
	var signalOnce sync.Once
	daemon.lifecycleCancel = func() {
		signalOnce.Do(func() { close(uninstallReachedActiveOperation) })
		originalCancel()
	}
	go func() {
		uninstallResult <- daemon.handleAddonLifecycleIntent(ctx, uninstall)
	}()

	<-uninstallReachedActiveOperation
	select {
	case err := <-uninstallResult:
		require.Failf(t, "uninstall returned before install completed", "error: %v", err)
	default:
	}
	installTask()
	require.NoError(t, <-uninstallResult)
	persisted, err := daemon.readAddonLifecycleState(ctx)
	require.NoError(t, err)
	require.NotNil(t, persisted)
	assert.Equal(t, testAddonUninstallOperationID, persisted.OperationID)
	assert.Equal(t, addonLifecycleDesiredStateAbsent, persisted.DesiredState)
	assert.Equal(t, pbgo.TaskState_RUNNING, persisted.TaskState)

	uninstallTask := <-tasks
	uninstallTask()

	persisted, err = daemon.readAddonLifecycleState(ctx)
	require.NoError(t, err)
	require.NotNil(t, persisted)
	assert.Equal(t, testAddonUninstallOperationID, persisted.OperationID)
	assert.Equal(t, addonLifecycleDesiredStateAbsent, persisted.DesiredState)
	assert.Equal(t, pbgo.TaskState_DONE, persisted.TaskState)
}

func TestAddonLifecycleResultCannotOverwriteNewerOperation(t *testing.T) {
	ctx := context.Background()
	daemon, kubeClient, _ := testLifecycleDaemon(nil, []*pbgo.PackageState{{Package: packageDatadogOperator}})
	uninstallRaw := testAddonLifecycleIntent(t, testAddonUninstallOperationID, addonLifecycleDesiredStateAbsent)
	putAddonLifecycleIntentConfigMap(t, kubeClient, uninstallRaw)
	require.NoError(t, daemon.writeAddonLifecycleState(ctx, addonLifecyclePersistedState{
		InstallationID: testLifecycleIdentity.InstallationID,
		TargetHash:     testLifecycleIdentity.TargetHash,
		OperationID:    testAddonUninstallOperationID,
		Digest:         strings.Repeat("b", 64),
		DesiredState:   addonLifecycleDesiredStateAbsent,
		Bootstrap:      addonLifecycleBootstrap{ClusterName: "test-cluster", Site: "datadoghq.com"},
		ConfigID:       testAddonUninstallOperationID,
		TaskState:      pbgo.TaskState_RUNNING,
	}))

	oldRequest := daemon.newAddonLifecycleRequest(addonLifecycleIntent{
		Version:        addonLifecycleVersion,
		InstallationID: testLifecycleIdentity.InstallationID,
		TargetHash:     testLifecycleIdentity.TargetHash,
		OperationID:    testAddonInstallOperationID,
		DesiredState:   addonLifecycleDesiredStateInstalled,
		Bootstrap:      addonLifecycleBootstrap{ClusterName: "test-cluster", Site: "datadoghq.com"},
	}, testAddonInstallOperationID, strings.Repeat("a", 64))
	require.NoError(t, daemon.recordAddonLifecycleResult(ctx, oldRequest, pbgo.TaskState_DONE, nil))

	persisted, err := daemon.readAddonLifecycleState(ctx)
	require.NoError(t, err)
	require.NotNil(t, persisted)
	assert.Equal(t, testAddonUninstallOperationID, persisted.OperationID)
	assert.Equal(t, pbgo.TaskState_RUNNING, persisted.TaskState)
}

func TestValidateAddonLifecycleProgress(t *testing.T) {
	install := addonLifecycleIntent{
		Version:        addonLifecycleVersion,
		InstallationID: testLifecycleIdentity.InstallationID,
		TargetHash:     testLifecycleIdentity.TargetHash,
		OperationID:    testAddonInstallOperationID,
		DesiredState:   addonLifecycleDesiredStateInstalled,
	}
	current := &addonLifecyclePersistedState{
		InstallationID: testLifecycleIdentity.InstallationID,
		TargetHash:     testLifecycleIdentity.TargetHash,
		OperationID:    testAddonInstallOperationID,
		Digest:         "install-digest",
		DesiredState:   addonLifecycleDesiredStateInstalled,
		TaskState:      pbgo.TaskState_DONE,
	}

	require.NoError(t, validateAddonLifecycleProgress(nil, install, "install-digest"))
	require.NoError(t, validateAddonLifecycleProgress(current, install, "install-digest"))

	acknowledged := install
	acknowledged.AcknowledgedOperationID = testAddonInstallOperationID
	require.NoError(t, validateAddonLifecycleProgress(current, acknowledged, "install-digest"))
	current.AcknowledgedOperationID = testAddonInstallOperationID
	require.NoError(t, validateAddonLifecycleProgress(current, acknowledged, "install-digest"))
	require.Error(t, validateAddonLifecycleProgress(current, install, "install-digest"))

	uninstall := addonLifecycleIntent{
		Version:                 addonLifecycleVersion,
		InstallationID:          testLifecycleIdentity.InstallationID,
		TargetHash:              testLifecycleIdentity.TargetHash,
		OperationID:             testAddonUninstallOperationID,
		DesiredState:            addonLifecycleDesiredStateAbsent,
		AcknowledgedOperationID: testAddonInstallOperationID,
	}
	require.NoError(t, validateAddonLifecycleProgress(current, uninstall, "uninstall-digest"))
	uninstall.AcknowledgedOperationID = ""
	require.Error(t, validateAddonLifecycleProgress(current, uninstall, "uninstall-digest"))
	require.Error(t, validateAddonLifecycleProgress(nil, uninstall, "uninstall-digest"))

	current.OperationID = testAddonUninstallOperationID
	current.Digest = "uninstall-digest"
	current.DesiredState = addonLifecycleDesiredStateAbsent
	uninstall.AcknowledgedOperationID = testAddonInstallOperationID
	require.NoError(t, validateAddonLifecycleProgress(current, uninstall, "uninstall-digest"))
	require.Error(t, validateAddonLifecycleProgress(current, acknowledged, "install-digest"))
}

func TestAddonLifecycleIntentRejectsInstallOperationReplacement(t *testing.T) {
	ctx := context.Background()
	daemon, _, rcClient := testLifecycleDaemon(
		nil,
		[]*pbgo.PackageState{{Package: packageDatadogOperator}},
		testFleetCredentialSecret(),
	)
	daemon.addonConfigs = make(map[string]installerConfig)

	install := addonLifecycleIntentSnapshot{raw: testAddonLifecycleIntent(
		t,
		testAddonInstallOperationID,
		addonLifecycleDesiredStateInstalled,
	)}
	putAddonLifecycleIntentConfigMap(t, daemon.client, install.raw)
	require.NoError(t, daemon.handleAddonLifecycleIntent(ctx, install))

	reused := addonLifecycleIntentSnapshot{raw: testAddonLifecycleIntent(
		t,
		testAddonUninstallOperationID,
		addonLifecycleDesiredStateInstalled,
	)}
	putAddonLifecycleIntentConfigMap(t, daemon.client, reused.raw)
	require.Error(t, daemon.handleAddonLifecycleIntent(ctx, reused))
	assert.Equal(t, testAddonUninstallOperationID, rcClient.state[0].GetTask().GetId())
	assert.Equal(t, pbgo.TaskState_INVALID_STATE, rcClient.state[0].GetTask().GetState())

	persisted, err := daemon.readAddonLifecycleState(ctx)
	require.NoError(t, err)
	require.NotNil(t, persisted)
	assert.Equal(t, testAddonInstallOperationID, persisted.OperationID)
}

func TestAddonLifecycleRejectsRemoteLifecycleTask(t *testing.T) {
	configID := "remote-lifecycle-config"
	d, _, _ := testLifecycleDaemon(
		testLifecycleInstallerConfig(configID, OperationCreate, `{"spec":{}}`),
		[]*pbgo.PackageState{{Package: packageDatadogOperator}},
		testFleetCredentialSecret(),
	)
	d.addonLifecycle = true
	req := testLifecycleRequest(methodInstallDatadogAgent, configID)

	err := d.validateLifecycleTask(req)
	require.ErrorContains(t, err, "local lifecycle intent adapter")
}

func TestReadAddonLifecycleStateRejectsForeignOwnership(t *testing.T) {
	intent := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
		Namespace: addonLifecycleIntentKey.Namespace,
		Name:      addonLifecycleIntentKey.Name,
		UID:       "intent-uid",
	}}
	state := addonLifecycleStateData(addonLifecyclePersistedState{
		InstallationID: testLifecycleIdentity.InstallationID,
		TargetHash:     testLifecycleIdentity.TargetHash,
		OperationID:    testAddonInstallOperationID,
		Digest:         strings.Repeat("a", 64),
		DesiredState:   addonLifecycleDesiredStateInstalled,
		Bootstrap: addonLifecycleBootstrap{
			ClusterName: "test-cluster",
			Site:        "datadoghq.com",
		},
		ConfigID:  testAddonInstallOperationID,
		TaskState: pbgo.TaskState_DONE,
	})
	foreign := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Namespace: addonLifecycleStateKey.Namespace, Name: addonLifecycleStateKey.Name},
		Data:       state,
	}
	d, _, _ := testLifecycleDaemon(nil, nil, intent, foreign)

	_, err := d.readAddonLifecycleState(context.Background())
	require.ErrorContains(t, err, "lifecycle state ownership")
}

func TestRehydrateAddonLifecycleStateRestoresAcknowledgedTask(t *testing.T) {
	d, kubeClient, rcClient := testLifecycleDaemon(
		nil,
		[]*pbgo.PackageState{{Package: packageDatadogOperator, StableConfigVersion: testAddonInstallOperationID}},
	)
	putAddonLifecycleIntentConfigMap(t, kubeClient, testAddonLifecycleIntent(
		t,
		testAddonInstallOperationID,
		addonLifecycleDesiredStateInstalled,
		testAddonInstallOperationID,
	))
	require.NoError(t, d.writeAddonLifecycleState(context.Background(), addonLifecyclePersistedState{
		InstallationID:          testLifecycleIdentity.InstallationID,
		TargetHash:              testLifecycleIdentity.TargetHash,
		OperationID:             testAddonInstallOperationID,
		Digest:                  strings.Repeat("a", 64),
		DesiredState:            addonLifecycleDesiredStateInstalled,
		Bootstrap:               addonLifecycleBootstrap{ClusterName: "test-cluster", Site: "datadoghq.com"},
		AcknowledgedOperationID: testAddonInstallOperationID,
		ConfigID:                testAddonInstallOperationID,
		TaskState:               pbgo.TaskState_DONE,
	}))

	require.NoError(t, d.rehydrateAddonLifecycleState(context.Background()))

	require.False(t, d.lifecycleTaskReserved)
	require.Len(t, rcClient.state, 1)
	require.Equal(t, testAddonInstallOperationID, rcClient.state[0].GetStableConfigVersion())
	require.Equal(t, testAddonInstallOperationID, rcClient.state[0].GetTask().GetId())
	require.Equal(t, pbgo.TaskState_DONE, rcClient.state[0].GetTask().GetState())
}

func testAddonLifecycleIntent(t *testing.T, operationID string, desiredState addonLifecycleDesiredState, acknowledgedOperationID ...string) []byte {
	t.Helper()
	payload := addonLifecycleIntent{
		Version:        addonLifecycleVersion,
		InstallationID: testLifecycleIdentity.InstallationID,
		TargetHash:     testLifecycleIdentity.TargetHash,
		OperationID:    operationID,
		DesiredState:   desiredState,
		Bootstrap: addonLifecycleBootstrap{
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

func putAddonLifecycleIntentConfigMap(t *testing.T, kubeClient client.Client, raw []byte) {
	t.Helper()
	ctx := context.Background()
	configMap := &corev1.ConfigMap{}
	err := kubeClient.Get(ctx, addonLifecycleIntentKey, configMap)
	if apierrors.IsNotFound(err) {
		require.NoError(t, kubeClient.Create(ctx, &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Namespace: addonLifecycleIntentKey.Namespace, Name: addonLifecycleIntentKey.Name, UID: "intent-uid"},
			Data:       map[string]string{addonLifecycleIntentDataKey: string(raw)},
		}))
		return
	}
	require.NoError(t, err)
	configMap.Data[addonLifecycleIntentDataKey] = string(raw)
	require.NoError(t, kubeClient.Update(ctx, configMap))
}
