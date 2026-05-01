// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package remoteconfig

import (
	"errors"
	"testing"

	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-operator/pkg/config"
	"github.com/DataDog/datadog-operator/pkg/constants"
)

type fakeRuntimeFactory struct {
	runtimes []*fakeRuntime
	failFor  map[string]error
}

type fakeRuntime struct {
	apiKey  string
	service *fakeService
	client  *fakeClient
}

type fakeService struct {
	startCount int
	stopCount  int
	stopErr    error
}

type fakeClient struct {
	startCount               int
	closeCount               int
	subscriptions            []string
	installerState           []*pbgo.PackageState
	installerStateStartCount int
}

func (f *fakeRuntimeFactory) factory(conf RcServiceConfiguration) (rcService, rcRuntimeClient, error) {
	if err := f.failFor[conf.apiKey]; err != nil {
		return nil, nil, err
	}
	runtime := &fakeRuntime{
		apiKey:  conf.apiKey,
		service: &fakeService{},
		client:  &fakeClient{},
	}
	f.runtimes = append(f.runtimes, runtime)
	return runtime.service, runtime.client, nil
}

func (f *fakeService) Start() {
	f.startCount++
}

func (f *fakeService) Stop() error {
	f.stopCount++
	return f.stopErr
}

func (f *fakeClient) Start() {
	f.startCount++
}

func (f *fakeClient) Close() {
	f.closeCount++
}

func (f *fakeClient) Subscribe(product string, _ func(update map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus))) {
	f.subscriptions = append(f.subscriptions, product)
}

func (f *fakeClient) GetInstallerState() []*pbgo.PackageState {
	return f.installerState
}

func (f *fakeClient) SetInstallerState(packages []*pbgo.PackageState) {
	f.installerState = packages
	f.installerStateStartCount = f.startCount
}

func newTestUpdater(factory *fakeRuntimeFactory) *RemoteConfigUpdater {
	r := NewRemoteConfigUpdater(nil, logr.Discard())
	r.remoteConfigFactory = factory.factory
	return r
}

func newTestCredentialManager(t *testing.T, apiKey string) *config.CredentialManager {
	t.Helper()
	t.Setenv(constants.DDAPIKey, apiKey)
	t.Setenv(constants.DDAppKey, "app-key")
	return config.NewCredentialManager(nil)
}

func TestSetupFetchesCredentialsFromManager(t *testing.T) {
	factory := &fakeRuntimeFactory{}
	r := newTestUpdater(factory)
	credsManager := newTestCredentialManager(t, "setup-api-key")

	require.NoError(t, r.Setup(credsManager))
	require.Len(t, factory.runtimes, 1)
	assert.Equal(t, "setup-api-key", factory.runtimes[0].apiKey)
	assert.Equal(t, 1, factory.runtimes[0].service.startCount)
	assert.Equal(t, 1, factory.runtimes[0].client.startCount)
}

func TestSyncCredentialsRestartsRuntimeWhenAPIKeyChanges(t *testing.T) {
	factory := &fakeRuntimeFactory{}
	r := newTestUpdater(factory)
	credsManager := newTestCredentialManager(t, "old-api-key")
	require.NoError(t, r.Setup(credsManager))

	t.Setenv(constants.DDAPIKey, "new-api-key")
	t.Setenv(constants.DDAppKey, "new-app-key")
	require.NoError(t, credsManager.Refresh(logr.Discard()))
	require.NoError(t, r.syncCredentials(credsManager))

	require.Len(t, factory.runtimes, 2)
	assert.Equal(t, "new-api-key", factory.runtimes[1].apiKey)
	assert.Equal(t, 1, factory.runtimes[0].service.stopCount)
	assert.Equal(t, 1, factory.runtimes[0].client.closeCount)
	assert.Equal(t, 1, factory.runtimes[1].service.startCount)
	assert.Equal(t, 1, factory.runtimes[1].client.startCount)
}

func TestSyncCredentialsNoopsWhenAPIKeyUnchanged(t *testing.T) {
	factory := &fakeRuntimeFactory{}
	r := newTestUpdater(factory)
	credsManager := newTestCredentialManager(t, "same-api-key")
	require.NoError(t, r.Setup(credsManager))

	require.NoError(t, r.syncCredentials(credsManager))

	require.Len(t, factory.runtimes, 1)
	assert.Equal(t, 0, factory.runtimes[0].service.stopCount)
	assert.Equal(t, 0, factory.runtimes[0].client.closeCount)
}

func TestSyncCredentialsReturnsErrorWithoutStoppingActiveRuntimeWhenCredentialsCannotBeRead(t *testing.T) {
	factory := &fakeRuntimeFactory{}
	r := newTestUpdater(factory)
	credsManager := newTestCredentialManager(t, "old-api-key")
	require.NoError(t, r.Setup(credsManager))

	t.Setenv(constants.DDAPIKey, "")
	t.Setenv(constants.DDAppKey, "")
	credsManager = config.NewCredentialManager(nil)

	err := r.syncCredentials(credsManager)
	require.Error(t, err)
	require.Len(t, factory.runtimes, 1)
	assert.Equal(t, 0, factory.runtimes[0].service.stopCount)
	assert.Equal(t, 0, factory.runtimes[0].client.closeCount)
}

func TestSyncCredentialsLeavesOldRuntimeActiveWhenReplacementCreationFails(t *testing.T) {
	factory := &fakeRuntimeFactory{failFor: map[string]error{"new-api-key": errors.New("boom")}}
	r := newTestUpdater(factory)
	credsManager := newTestCredentialManager(t, "old-api-key")
	require.NoError(t, r.Setup(credsManager))

	t.Setenv(constants.DDAPIKey, "new-api-key")
	t.Setenv(constants.DDAppKey, "new-app-key")
	require.NoError(t, credsManager.Refresh(logr.Discard()))
	err := r.syncCredentials(credsManager)

	require.Error(t, err)
	require.Len(t, factory.runtimes, 1)
	assert.Equal(t, 0, factory.runtimes[0].service.stopCount)
	assert.Equal(t, 0, factory.runtimes[0].client.closeCount)
	assert.Equal(t, "old-api-key", r.activeAPIKey)
}

func TestSyncCredentialsTreatsOldRuntimeCleanupErrorAsNonFatal(t *testing.T) {
	factory := &fakeRuntimeFactory{}
	r := newTestUpdater(factory)
	credsManager := newTestCredentialManager(t, "old-api-key")
	require.NoError(t, r.Setup(credsManager))
	factory.runtimes[0].service.stopErr = errors.New("stop failed")

	t.Setenv(constants.DDAPIKey, "new-api-key")
	t.Setenv(constants.DDAppKey, "new-app-key")
	require.NoError(t, credsManager.Refresh(logr.Discard()))
	require.NoError(t, r.syncCredentials(credsManager))

	require.Len(t, factory.runtimes, 2)
	assert.Equal(t, "new-api-key", r.activeAPIKey)
	assert.Equal(t, factory.runtimes[1].service, r.rcService)
	assert.Equal(t, factory.runtimes[1].client, r.rcClient)
	assert.Equal(t, 1, factory.runtimes[0].service.stopCount)
	assert.Equal(t, 1, factory.runtimes[0].client.closeCount)
}

func TestSyncCredentialsPreservesSubscriptionsAndInstallerStateAcrossRestarts(t *testing.T) {
	factory := &fakeRuntimeFactory{}
	r := newTestUpdater(factory)
	credsManager := newTestCredentialManager(t, "old-api-key")
	require.NoError(t, r.Setup(credsManager))

	r.Subscribe("fleet-product", func(map[string]state.RawConfig, func(string, state.ApplyStatus)) {})
	r.SetInstallerState([]*pbgo.PackageState{{
		Package:             "fleet-package",
		StableVersion:       "1.2.3",
		StableConfigVersion: "4.5.6",
	}})

	t.Setenv(constants.DDAPIKey, "new-api-key")
	t.Setenv(constants.DDAppKey, "new-app-key")
	require.NoError(t, credsManager.Refresh(logr.Discard()))
	require.NoError(t, r.syncCredentials(credsManager))

	require.Len(t, factory.runtimes, 2)
	newClient := factory.runtimes[1].client
	assert.ElementsMatch(t, []string{
		string(state.ProductAgentConfig),
		string(state.ProductOrchestratorK8sCRDs),
		"fleet-product",
	}, newClient.subscriptions)
	require.Len(t, newClient.installerState, 1)
	assert.Equal(t, "fleet-package", newClient.installerState[0].Package)
	assert.Equal(t, 0, newClient.installerStateStartCount)
	assert.Equal(t, 1, newClient.startCount)
}
