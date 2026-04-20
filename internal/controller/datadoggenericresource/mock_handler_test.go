package datadoggenericresource

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

const mockSubresource v1alpha1.SupportedResourcesType = "mock_resource"

var (
	mockResourceID      = "mock-id"
	mockResourceCreator = "mock-creator"
	mockGetErr          error
	mockUpdateErr       error
	mockDeleteErr       error
	mockCreateCalls     int
)

func init() {
	testHandlers = map[v1alpha1.SupportedResourcesType]ResourceHandler{
		mockSubresource: &MockHandler{},
	}
}

// MockHandler is a test double for ResourceHandler.
type MockHandler struct{}

func (h *MockHandler) createResourcefunc(_ *Reconciler, _ context.Context, instance *v1alpha1.DatadogGenericResource, status *v1alpha1.DatadogGenericResourceStatus, now metav1.Time, hash string) error {
	mockCreateCalls++
	status.Id = mockResourceID
	status.Created = &now
	status.LastForceSyncTime = &now
	status.Creator = mockResourceCreator
	status.SyncStatus = v1alpha1.DatadogSyncStatusOK
	status.CurrentHash = hash
	return nil
}

func (h *MockHandler) getResourcefunc(_ *Reconciler, _ *v1alpha1.DatadogGenericResource) error {
	return mockGetErr
}

func (h *MockHandler) updateResourcefunc(_ *Reconciler, _ *v1alpha1.DatadogGenericResource) error {
	return mockUpdateErr
}

func (h *MockHandler) deleteResourcefunc(_ *Reconciler, _ *v1alpha1.DatadogGenericResource) error {
	return mockDeleteErr
}

func resetMockHandlerState() {
	mockResourceID = "mock-id"
	mockResourceCreator = "mock-creator"
	mockGetErr = nil
	mockUpdateErr = nil
	mockDeleteErr = nil
	mockCreateCalls = 0
}
