package datadoggenericresource

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

const mockSubresource v1alpha1.SupportedResourcesType = "mock_resource"

var (
	mockResourceID         = "mock-id"
	mockResourceCreator    = "mock-creator"
	mockGetErr             error
	mockUpdateErr          error
	mockDeleteErr          error
	mockCreateErr          error
	mockCreateCalls        int
	mockGetCalls           int
	mockUpdateCalls        int
	mockDeleteCalls        int
	mockRefreshStateCalls  int
	mockRefreshStateErr    error
	mockRefreshStateResult *string
)

// MockHandler is a test double for ResourceHandler.
type MockHandler struct{}

func (h *MockHandler) createResource(context.Context, *v1alpha1.DatadogGenericResource) (CreateResult, error) {
	mockCreateCalls++
	if mockCreateErr != nil {
		return CreateResult{}, mockCreateErr
	}
	now := metav1.Now()
	return CreateResult{
		ID:          mockResourceID,
		CreatedTime: &now,
		Creator:     mockResourceCreator,
	}, nil
}

func (h *MockHandler) getResource(context.Context, *v1alpha1.DatadogGenericResource) error {
	mockGetCalls++
	return mockGetErr
}

func (h *MockHandler) updateResource(context.Context, *v1alpha1.DatadogGenericResource) error {
	mockUpdateCalls++
	return mockUpdateErr
}

func (h *MockHandler) deleteResource(context.Context, *v1alpha1.DatadogGenericResource) error {
	mockDeleteCalls++
	return mockDeleteErr
}

func (h *MockHandler) refreshState(context.Context, *v1alpha1.DatadogGenericResource) (*string, error) {
	mockRefreshStateCalls++
	if mockRefreshStateErr != nil {
		return nil, mockRefreshStateErr
	}
	return mockRefreshStateResult, nil
}

func resetMockHandlerState() {
	mockResourceID = "mock-id"
	mockResourceCreator = "mock-creator"
	mockGetErr = nil
	mockUpdateErr = nil
	mockDeleteErr = nil
	mockCreateErr = nil
	mockCreateCalls = 0
	mockGetCalls = 0
	mockUpdateCalls = 0
	mockDeleteCalls = 0
	mockRefreshStateCalls = 0
	mockRefreshStateErr = nil
	mockRefreshStateResult = nil
}
