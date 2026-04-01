// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fleet

import (
	"encoding/json"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

// Test data

var testInstallerConfig = installerConfig{
	ID: "test",
	Operations: []fleetManagementOperation{
		{
			Operation: OperationUpdate,
			GroupVersionKind: schema.GroupVersionKind{
				Group:   "datadoghq.com",
				Version: "v2alpha1",
				Kind:    "DatadogAgent",
			},
			NamespacedName: types.NamespacedName{
				Namespace: "datadog",
				Name:      "datadog-agent",
			},
			Config: json.RawMessage(`{"spec":{"features":{"apm":{"enabled":true}}}}`),
		},
	},
}

var testRemoteAPIRequest = remoteAPIRequest{
	ID:     "test",
	Method: "some_method",
	Params: json.RawMessage(`{}`),
}

// callbackMock records calls made by the RC handler callbacks.
type callbackMock struct {
	mock.Mock
}

func (c *callbackMock) handleConfigs(configs map[string]installerConfig) error {
	args := c.Called(configs)
	return args.Error(0)
}

func (c *callbackMock) handleRemoteAPIRequest(req remoteAPIRequest) error {
	args := c.Called(req)
	return args.Error(0)
}

func (c *callbackMock) applyStateCallback(id string, status state.ApplyStatus) {
	c.Called(id, status)
}

// marshalRawConfig serialises v into a state.RawConfig for use in tests.
func marshalRawConfig(t *testing.T, v any) state.RawConfig {
	t.Helper()
	data, err := json.Marshal(v)
	assert.NoError(t, err)
	return state.RawConfig{Config: data}
}

// --- handleInstallerConfigUpdate tests ---

func TestInstallerConfigUpdate(t *testing.T) {
	cb := &callbackMock{}
	handler := handleInstallerConfigUpdate(cb.handleConfigs)

	raw := marshalRawConfig(t, testInstallerConfig)
	updates := map[string]state.RawConfig{"path/to/config": raw}

	expectedConfigs := map[string]installerConfig{"path/to/config": testInstallerConfig}
	cb.On("handleConfigs", expectedConfigs).Return(nil)
	cb.On("applyStateCallback", "path/to/config", state.ApplyStatus{State: state.ApplyStateAcknowledged}).Return()

	handler(updates, cb.applyStateCallback)

	cb.AssertCalled(t, "handleConfigs", expectedConfigs)
	cb.AssertCalled(t, "applyStateCallback", "path/to/config", state.ApplyStatus{State: state.ApplyStateAcknowledged})
}

func TestInstallerConfigUpdateBadConfig(t *testing.T) {
	cb := &callbackMock{}
	handler := handleInstallerConfigUpdate(cb.handleConfigs)

	updates := map[string]state.RawConfig{
		"path/to/config": {Config: []byte("not json")},
	}

	cb.On("applyStateCallback", "path/to/config", mock.MatchedBy(func(s state.ApplyStatus) bool {
		return s.State == state.ApplyStateError && s.Error != ""
	})).Return()

	handler(updates, cb.applyStateCallback)

	cb.AssertNotCalled(t, "handleConfigs", mock.Anything)
	cb.AssertCalled(t, "applyStateCallback", "path/to/config", mock.MatchedBy(func(s state.ApplyStatus) bool {
		return s.State == state.ApplyStateError
	}))
}

func TestInstallerConfigUpdateError(t *testing.T) {
	cb := &callbackMock{}
	handler := handleInstallerConfigUpdate(cb.handleConfigs)

	raw := marshalRawConfig(t, testInstallerConfig)
	updates := map[string]state.RawConfig{"path/to/config": raw}
	expectedConfigs := map[string]installerConfig{"path/to/config": testInstallerConfig}

	cb.On("handleConfigs", expectedConfigs).Return(assert.AnError)
	cb.On("applyStateCallback", "path/to/config", mock.MatchedBy(func(s state.ApplyStatus) bool {
		return s.State == state.ApplyStateError && s.Error != ""
	})).Return()

	handler(updates, cb.applyStateCallback)

	cb.AssertCalled(t, "handleConfigs", expectedConfigs)
	cb.AssertCalled(t, "applyStateCallback", "path/to/config", mock.MatchedBy(func(s state.ApplyStatus) bool {
		return s.State == state.ApplyStateError
	}))
}

// --- handleUpdaterTaskUpdate tests ---

func TestRemoteAPIRequest(t *testing.T) {
	cb := &callbackMock{}
	handler := handleUpdaterTaskUpdate(cb.handleRemoteAPIRequest)

	raw := marshalRawConfig(t, testRemoteAPIRequest)
	updates := map[string]state.RawConfig{"path/to/task": raw}

	cb.On("handleRemoteAPIRequest", testRemoteAPIRequest).Return(nil)
	cb.On("applyStateCallback", "path/to/task", state.ApplyStatus{State: state.ApplyStateUnacknowledged}).Return()
	cb.On("applyStateCallback", "path/to/task", state.ApplyStatus{State: state.ApplyStateAcknowledged}).Return()

	handler(updates, cb.applyStateCallback)

	cb.AssertCalled(t, "handleRemoteAPIRequest", testRemoteAPIRequest)
	cb.AssertCalled(t, "applyStateCallback", "path/to/task", state.ApplyStatus{State: state.ApplyStateUnacknowledged})
	cb.AssertCalled(t, "applyStateCallback", "path/to/task", state.ApplyStatus{State: state.ApplyStateAcknowledged})
}

func TestRemoteAPIRequestBadConfig(t *testing.T) {
	cb := &callbackMock{}
	handler := handleUpdaterTaskUpdate(cb.handleRemoteAPIRequest)

	updates := map[string]state.RawConfig{
		"path/to/task": {Config: []byte("not json")},
	}

	cb.On("applyStateCallback", "path/to/task", mock.MatchedBy(func(s state.ApplyStatus) bool {
		return s.State == state.ApplyStateError && s.Error != ""
	})).Return()

	handler(updates, cb.applyStateCallback)

	cb.AssertNotCalled(t, "handleRemoteAPIRequest", mock.Anything)
	cb.AssertCalled(t, "applyStateCallback", "path/to/task", mock.MatchedBy(func(s state.ApplyStatus) bool {
		return s.State == state.ApplyStateError
	}))
}

func TestRemoteAPIRequestError(t *testing.T) {
	cb := &callbackMock{}
	handler := handleUpdaterTaskUpdate(cb.handleRemoteAPIRequest)

	raw := marshalRawConfig(t, testRemoteAPIRequest)
	updates := map[string]state.RawConfig{"path/to/task": raw}

	cb.On("handleRemoteAPIRequest", testRemoteAPIRequest).Return(assert.AnError)
	cb.On("applyStateCallback", "path/to/task", state.ApplyStatus{State: state.ApplyStateUnacknowledged}).Return()
	cb.On("applyStateCallback", "path/to/task", mock.MatchedBy(func(s state.ApplyStatus) bool {
		return s.State == state.ApplyStateError && s.Error != ""
	})).Return()

	handler(updates, cb.applyStateCallback)

	cb.AssertCalled(t, "handleRemoteAPIRequest", testRemoteAPIRequest)
	cb.AssertCalled(t, "applyStateCallback", "path/to/task", state.ApplyStatus{State: state.ApplyStateUnacknowledged})
	cb.AssertCalled(t, "applyStateCallback", "path/to/task", mock.MatchedBy(func(s state.ApplyStatus) bool {
		return s.State == state.ApplyStateError
	}))
}

func TestRemoteAPIRequestIgnoresAlreadyExecutedRequests(t *testing.T) {
	cb := &callbackMock{}
	handler := handleUpdaterTaskUpdate(cb.handleRemoteAPIRequest)

	raw := marshalRawConfig(t, testRemoteAPIRequest)
	updates := map[string]state.RawConfig{"path/to/task": raw}

	cb.On("handleRemoteAPIRequest", testRemoteAPIRequest).Return(nil)
	cb.On("applyStateCallback", "path/to/task", state.ApplyStatus{State: state.ApplyStateUnacknowledged}).Return()
	cb.On("applyStateCallback", "path/to/task", state.ApplyStatus{State: state.ApplyStateAcknowledged}).Return()

	// First call — should invoke the handler; sends Unacknowledged then Acknowledged.
	handler(updates, cb.applyStateCallback)
	cb.AssertNumberOfCalls(t, "handleRemoteAPIRequest", 1)

	// Second call with same request ID — handler must NOT be called again; sends only Acknowledged.
	handler(updates, cb.applyStateCallback)
	cb.AssertNumberOfCalls(t, "handleRemoteAPIRequest", 1)

	// First call: Unacknowledged + Acknowledged. Second call: Acknowledged only.
	cb.AssertNumberOfCalls(t, "applyStateCallback", 3)
}
