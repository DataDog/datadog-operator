package datadoggenericresource

import (
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

// MockHandler is a test double for ResourceHandler.
type MockHandler struct{}

func (h *MockHandler) createResource(_ *v1alpha1.DatadogGenericResource) (CreateResult, error) {
	mockCreateCalls++
	now := metav1.Now()
	return CreateResult{
		ID:          mockResourceID,
		CreatedTime: &now,
		Creator:     mockResourceCreator,
	}, nil
}

func (h *MockHandler) getResource(_ *v1alpha1.DatadogGenericResource) error {
	return mockGetErr
}

func (h *MockHandler) updateResource(_ *v1alpha1.DatadogGenericResource) error {
	return mockUpdateErr
}

func (h *MockHandler) deleteResource(_ *v1alpha1.DatadogGenericResource) error {
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
