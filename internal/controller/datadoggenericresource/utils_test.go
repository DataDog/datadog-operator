package datadoggenericresource

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"testing"

	datadogapi "github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	ctrutils "github.com/DataDog/datadog-operator/pkg/controller/utils"
)

func Test_getHandler(t *testing.T) {
	r := &Reconciler{
		handlers: map[v1alpha1.SupportedResourcesType]ResourceHandler{
			mockSubresource: &MockHandler{},
		},
	}

	// Known type returns a non-nil handler
	assert.NotNil(t, r.getHandler(mockSubresource))

	// Unsupported type panics
	assert.PanicsWithError(t, "unsupported type: unsupportedType", func() {
		r.getHandler("unsupportedType")
	})
}

func Test_translateClientError(t *testing.T) {
	var ErrGeneric = errors.New("generic error")

	testCases := []struct {
		name                   string
		error                  error
		httpResp               *http.Response
		message                string
		expectedErrorType      error
		expectedErrorString    string
		expectedErrorInterface any
		wantPermanent          bool
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
			name:                "generic message, error type *url.Error",
			error:               &url.Error{Err: fmt.Errorf("generic url error")},
			message:             "generic message",
			expectedErrorString: "generic message (url.Error):  \"\": generic url error",
		},
		{
			name:          "400 response is classified as a permanent error",
			error:         datadogapi.GenericOpenAPIError{},
			httpResp:      &http.Response{StatusCode: http.StatusBadRequest},
			message:       "error creating resource",
			wantPermanent: true,
		},
		{
			name:          "500 response is classified as a transient error",
			error:         datadogapi.GenericOpenAPIError{},
			httpResp:      &http.Response{StatusCode: http.StatusInternalServerError},
			message:       "error creating resource",
			wantPermanent: false,
		},
	}
	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			result := translateClientError(test.error, test.httpResp, test.message)

			if test.expectedErrorType != nil {
				assert.True(t, errors.Is(result, test.expectedErrorType))
			}

			if test.expectedErrorInterface != nil {
				assert.True(t, errors.As(result, test.expectedErrorInterface))
			}

			if test.expectedErrorString != "" {
				assert.EqualError(t, result, test.expectedErrorString)
			}

			assert.Equal(t, test.wantPermanent, ctrutils.IsPermanentAPIError(result))
		})
	}
}

func Test_translateUnmarshalError(t *testing.T) {
	err := translateUnmarshalError(errors.New("unexpected end of JSON input"), "error unmarshalling monitor spec")
	assert.EqualError(t, err, "error unmarshalling monitor spec: unexpected end of JSON input")
	// A spec that never parses will never succeed on retry, no HTTP request was
	// made, so this must be classified as permanent rather than defaulting to
	// transient the way a genuine network failure (nil response) would.
	assert.True(t, ctrutils.IsPermanentAPIError(err))
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
