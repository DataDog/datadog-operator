package datadoggenericresource

import (
	"errors"
	"fmt"
	"net/url"
	"testing"

	datadogapi "github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

func Test_getHandler(t *testing.T) {
	// Known types return a non-nil handler
	for _, rt := range []v1alpha1.SupportedResourcesType{
		v1alpha1.Dashboard, v1alpha1.Downtime, v1alpha1.Monitor,
		v1alpha1.Notebook, v1alpha1.SyntheticsAPITest, v1alpha1.SyntheticsBrowserTest,
		mockSubresource,
	} {
		assert.NotNil(t, getHandler(rt), "expected handler for %s", rt)
	}

	// Unsupported type panics
	assert.PanicsWithError(t, "unsupported type: unsupportedType", func() {
		getHandler("unsupportedType")
	})
}

func Test_translateClientError(t *testing.T) {
	var ErrGeneric = errors.New("generic error")

	testCases := []struct {
		name                   string
		error                  error
		message                string
		expectedErrorType      error
		expectedError          error
		expectedErrorInterface any
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
