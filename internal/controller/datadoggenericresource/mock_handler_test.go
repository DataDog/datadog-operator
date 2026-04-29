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
	mockDeleteCalls     int
)

// MockHandler is a test double for ResourceHandler.
type MockHandler struct{}

func (h *MockHandler) createResource(instance *v1alpha1.DatadogGenericResource, auth context.Context) (CreateResult, error) {
	mockCreateCalls++
	now := metav1.Now()
	return CreateResult{
		ID:          mockResourceID,
		CreatedTime: &now,
		Creator:     mockResourceCreator,
	}, nil
}

func (h *MockHandler) getResource(instance *v1alpha1.DatadogGenericResource, auth context.Context) error {
	return mockGetErr
}

func (h *MockHandler) updateResource(instance *v1alpha1.DatadogGenericResource, auth context.Context) error {
	return mockUpdateErr
}

func (h *MockHandler) deleteResource(instance *v1alpha1.DatadogGenericResource, auth context.Context) error {
	mockDeleteCalls++
	return mockDeleteErr
}

func resetMockHandlerState() {
	mockResourceID = "mock-id"
	mockResourceCreator = "mock-creator"
	mockGetErr = nil
	mockUpdateErr = nil
	mockDeleteErr = nil
	mockCreateCalls = 0
	mockDeleteCalls = 0
}
