package datadoggenericcr

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

func Test_executeHandler(t *testing.T) {
	mockReconciler := &Reconciler{}
	instance := &v1alpha1.DatadogGenericCR{
		Spec: v1alpha1.DatadogGenericCRSpec{
			Type: mockSubresource,
		},
	}

	// Valid operation and subresource case
	err := executeHandler(operationGet, mockReconciler, instance)
	assert.NoError(t, err)

	// Valid operation and invalid subresource case
	instance.Spec.Type = "unsupportedType"
	err = executeHandler(operationGet, mockReconciler, instance)
	assert.EqualError(t, err, "unsupported type: unsupportedType")
}

func Test_executeCreateHandler(t *testing.T) {
	mockReconciler := &Reconciler{}
	logger := &logr.Logger{}
	instance := &v1alpha1.DatadogGenericCR{
		Spec: v1alpha1.DatadogGenericCRSpec{
			Type: mockSubresource,
		},
	}
	status := &v1alpha1.DatadogGenericCRStatus{}

	// Valid subresource case
	err := executeCreateHandler(mockReconciler, *logger, instance, status, metav1.Now(), "test-hash")
	assert.NoError(t, err)

	// Invalid subresource case
	instance.Spec.Type = "unsupportedType"
	err = executeCreateHandler(mockReconciler, *logger, instance, status, metav1.Now(), "test-hash")
	assert.EqualError(t, err, "unsupported type: unsupportedType")
}

func Test_apiGet(t *testing.T) {
	mockReconciler := &Reconciler{}
	instance := &v1alpha1.DatadogGenericCR{
		Spec: v1alpha1.DatadogGenericCRSpec{
			Type: mockSubresource,
		},
	}

	err := apiGet(mockReconciler, instance)
	assert.NoError(t, err)
}

func Test_apiUpdate(t *testing.T) {
	mockReconciler := &Reconciler{}
	instance := &v1alpha1.DatadogGenericCR{
		Spec: v1alpha1.DatadogGenericCRSpec{
			Type: mockSubresource,
		},
	}

	err := apiUpdate(mockReconciler, instance)
	assert.NoError(t, err)
}

func Test_apiDelete(t *testing.T) {
	mockReconciler := &Reconciler{}
	instance := &v1alpha1.DatadogGenericCR{
		Spec: v1alpha1.DatadogGenericCRSpec{
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

// func TestUnsupportedInstanceType(t *testing.T) {
// 	instance := &v1alpha1.DatadogGenericCR{
// 		Spec: v1alpha1.DatadogGenericCRSpec{
// 			Type: "unsupported",
// 		},
// 	}
// 	err := unsupportedInstanceType(instance)
// 	assert.Contains(t, err.Error(), "unsupported type: unsupported")
// }

// func TestApiDelete(t *testing.T) {
// 	mockReconciler := &MockReconciler{}
// 	instance := &v1alpha1.DatadogGenericCR{
// 		Spec: v1alpha1.DatadogGenericCRSpec{
// 			Type: v1alpha1.SyntheticsBrowserTest,
// 		},
// 		Status: v1alpha1.DatadogGenericCRStatus{
// 			Id: "test-id",
// 		},
// 	}

// 	apiHandlers[apiHandlerKey{v1alpha1.SyntheticsBrowserTest, operationDelete}] = func(r *Reconciler, instance *v1alpha1.DatadogGenericCR) error {
// 		return nil
// 	}

// 	err := apiDelete(mockReconciler, instance)
// 	assert.NoError(t, err)
// }

// func TestApiGet(t *testing.T) {
// 	mockReconciler := &MockReconciler{}
// 	instance := &v1alpha1.DatadogGenericCR{
// 		Spec: v1alpha1.DatadogGenericCRSpec{
// 			Type: v1alpha1.SyntheticsBrowserTest,
// 		},
// 		Status: v1alpha1.DatadogGenericCRStatus{
// 			Id: "test-id",
// 		},
// 	}

// 	apiHandlers[apiHandlerKey{v1alpha1.SyntheticsBrowserTest, operationGet}] = func(r *Reconciler, instance *v1alpha1.DatadogGenericCR) error {
// 		return nil
// 	}

// 	err := apiGet(mockReconciler, instance)
// 	assert.NoError(t, err)
// }

// func TestApiUpdate(t *testing.T) {
// 	mockReconciler := &MockReconciler{}
// 	instance := &v1alpha1.DatadogGenericCR{
// 		Spec: v1alpha1.DatadogGenericCRSpec{
// 			Type: v1alpha1.SyntheticsBrowserTest,
// 		},
// 		Status: v1alpha1.DatadogGenericCRStatus{
// 			Id: "test-id",
// 		},
// 	}

// 	apiHandlers[apiHandlerKey{v1alpha1.SyntheticsBrowserTest, operationUpdate}] = func(r *Reconciler, instance *v1alpha1.DatadogGenericCR) error {
// 		return nil
// 	}

// 	err := apiUpdate(mockReconciler, instance)
// 	assert.NoError(t, err)
// }

// func TestApiCreateAndUpdateStatus(t *testing.T) {
// 	mockReconciler := &MockReconciler{}
// 	instance := &v1alpha1.DatadogGenericCR{
// 		Spec: v1alpha1.DatadogGenericCRSpec{
// 			Type: v1alpha1.SyntheticsBrowserTest,
// 		},
// 	}
// 	status := &v1alpha1.DatadogGenericCRStatus{}
// 	now := metav1.Now()
// 	hash := "test-hash"
// 	logger := logr.Discard()

// 	createHandlers[v1alpha1.SyntheticsBrowserTest] = func(r *Reconciler, logger logr.Logger, instance *v1alpha1.DatadogGenericCR, status *v1alpha1.DatadogGenericCRStatus, now metav1.Time, hash string) error {
// 		return nil
// 	}

// 	err := apiCreateAndUpdateStatus(mockReconciler, logger, instance, status, now, hash)
// 	assert.NoError(t, err)
// }
