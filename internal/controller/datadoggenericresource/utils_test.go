package datadoggenericresource

import (
	"errors"
	"fmt"
	"net/url"
	"testing"

	datadogapi "github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_apiCreateAndUpdateStatus(t *testing.T) {
	mockReconciler := &Reconciler{}
	logger := &logr.Logger{}
	instance := &v1alpha1.DatadogGenericResource{
		Spec: v1alpha1.DatadogGenericResourceSpec{
			Type: mockSubresource,
		},
	}
	status := &v1alpha1.DatadogGenericResourceStatus{}

	// Valid subresource case
	err := apiCreateAndUpdateStatus(mockReconciler, *logger, instance, status, metav1.Now(), "test-hash")
	assert.NoError(t, err)

	// Invalid subresource case
	instance.Spec.Type = "unsupportedType"
	assert.PanicsWithError(t, "unsupported type: unsupportedType", func() {
		apiCreateAndUpdateStatus(mockReconciler, *logger, instance, status, metav1.Now(), "test-hash")
	})
}

func Test_apiGet(t *testing.T) {
	mockReconciler := &Reconciler{}
	instance := &v1alpha1.DatadogGenericResource{
		Spec: v1alpha1.DatadogGenericResourceSpec{
			Type: mockSubresource,
		},
	}

	err := apiGet(mockReconciler, instance)
	assert.NoError(t, err)
}

func Test_apiUpdate(t *testing.T) {
	mockReconciler := &Reconciler{}
	instance := &v1alpha1.DatadogGenericResource{
		Spec: v1alpha1.DatadogGenericResourceSpec{
			Type: mockSubresource,
		},
	}

	err := apiUpdate(mockReconciler, instance)
	assert.NoError(t, err)
}

func Test_apiDelete(t *testing.T) {
	mockReconciler := &Reconciler{}
	instance := &v1alpha1.DatadogGenericResource{
		Spec: v1alpha1.DatadogGenericResourceSpec{
			Type: mockSubresource,
		},
	}

	err := apiDelete(mockReconciler, instance)
	assert.NoError(t, err)
}

func Test_translateClientError(t *testing.T) {
	var ErrGeneric = errors.New("generic error")

	testCases := []struct {
		name                   string
		error                  error
		message                string
		expectedErrorType      error
		expectedError          error
		expectedErrorInterface interface{}
	}{
		{
			name:              "no message, generic error",
			error:             ErrGeneric,
			message:           "",
			expectedErrorType: ErrGeneric,
		},
		{
			name:              "generic message, generic error",
			error:             ErrGeneric,
			message:           "generic message",
			expectedErrorType: ErrGeneric,
		},
		{
			name:                   "generic message, error type datadogV1.GenericOpenAPIError",
			error:                  datadogapi.GenericOpenAPIError{},
			message:                "generic message",
			expectedErrorInterface: &datadogapi.GenericOpenAPIError{},
		},
		{
			name:          "generic message, error type *url.Error",
			error:         &url.Error{Err: fmt.Errorf("generic url error")},
			message:       "generic message",
			expectedError: fmt.Errorf("generic message (url.Error):  \"\": generic url error"),
		},
	}
	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			result := translateClientError(test.error, test.message)

			if test.expectedErrorType != nil {
				assert.True(t, errors.Is(result, test.expectedErrorType))
			}

			if test.expectedErrorInterface != nil {
				assert.True(t, errors.As(result, test.expectedErrorInterface))
			}

			if test.expectedError != nil {
				assert.Equal(t, test.expectedError, result)
			}
		})
	}
}

func Test_resourceStringToInt64ID(t *testing.T) {
	originalResourceID := "123"
	expectedResourceID := int64(123)
	convertedResourceID, err := resourceStringToInt64ID(originalResourceID)
	assert.NoError(t, err)
	assert.Equal(t, expectedResourceID, convertedResourceID)

	// Invalid resource ID - cannot be converted to int64
	originalResourceID = "invalid"
	convertedResourceID, err = resourceStringToInt64ID(originalResourceID)
	assert.EqualError(t, err, "error parsing resource ID: strconv.ParseInt: parsing \"invalid\": invalid syntax")
}

func Test_resourceInt64ToStringID(t *testing.T) {
	originalResourceID := int64(123)
	expectedResourceID := "123"
	convertedResourceID := resourceInt64ToStringID(originalResourceID)
	assert.Equal(t, expectedResourceID, convertedResourceID)
}
