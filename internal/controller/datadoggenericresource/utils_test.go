package datadoggenericresource

import (
	"errors"
	"fmt"
	"net/url"
	"testing"
	"time"

	datadogapi "github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
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

func Test_updateStatusFromSyntheticsTest(t *testing.T) {
	mockLogger := logr.Discard()
	hash := "test-hash"

	tests := []struct {
		name                 string
		additionalProperties map[string]interface{}
		expectedStatus       v1alpha1.DatadogGenericResourceStatus
	}{
		{
			name: "valid properties",
			additionalProperties: map[string]interface{}{
				"created_at": "2024-01-01T00:00:00Z",
				"created_by": map[string]interface{}{
					"handle": "test-handle",
				},
			},
			expectedStatus: v1alpha1.DatadogGenericResourceStatus{
				Id:                "test-id",
				Creator:           "test-handle",
				SyncStatus:        v1alpha1.DatadogSyncStatusOK,
				CurrentHash:       hash,
				Created:           &metav1.Time{Time: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
				LastForceSyncTime: &metav1.Time{Time: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
			},
		},
		{
			name: "missing created_at",
			additionalProperties: map[string]interface{}{
				"created_by": map[string]interface{}{
					"handle": "test-handle",
				},
			},
			expectedStatus: v1alpha1.DatadogGenericResourceStatus{
				Id:                "test-id",
				Creator:           "test-handle",
				SyncStatus:        v1alpha1.DatadogSyncStatusOK,
				CurrentHash:       hash,
				Created:           &metav1.Time{Time: time.Now()},
				LastForceSyncTime: &metav1.Time{Time: time.Now()},
			},
		},
		{
			name: "invalid created_at",
			additionalProperties: map[string]interface{}{
				"created_at": "invalid-date",
				"created_by": map[string]interface{}{
					"handle": "test-handle",
				},
			},
			expectedStatus: v1alpha1.DatadogGenericResourceStatus{
				Id:                "test-id",
				Creator:           "test-handle",
				SyncStatus:        v1alpha1.DatadogSyncStatusOK,
				CurrentHash:       hash,
				Created:           &metav1.Time{Time: time.Now()},
				LastForceSyncTime: &metav1.Time{Time: time.Now()},
			},
		},
		{
			name: "missing created_by",
			additionalProperties: map[string]interface{}{
				"created_at": "2024-01-01T00:00:00Z",
			},
			expectedStatus: v1alpha1.DatadogGenericResourceStatus{
				Id:                "test-id",
				Creator:           "",
				SyncStatus:        v1alpha1.DatadogSyncStatusOK,
				CurrentHash:       hash,
				Created:           &metav1.Time{Time: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
				LastForceSyncTime: &metav1.Time{Time: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
			},
		},
		{
			name: "missing handle in created_by",
			additionalProperties: map[string]interface{}{
				"created_at": "2024-01-01T00:00:00Z",
				"created_by": map[string]interface{}{},
			},
			expectedStatus: v1alpha1.DatadogGenericResourceStatus{
				Id:                "test-id",
				Creator:           "",
				SyncStatus:        v1alpha1.DatadogSyncStatusOK,
				CurrentHash:       hash,
				Created:           &metav1.Time{Time: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
				LastForceSyncTime: &metav1.Time{Time: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := &v1alpha1.DatadogGenericResourceStatus{}
			syntheticTest := &datadogV1.SyntheticsAPITest{}
			syntheticTest.SetPublicId("test-id")
			err := updateStatusFromSyntheticsTest(syntheticTest, tt.additionalProperties, status, mockLogger, hash)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedStatus.Id, status.Id)
			assert.Equal(t, tt.expectedStatus.Creator, status.Creator)
			assert.Equal(t, tt.expectedStatus.SyncStatus, status.SyncStatus)
			assert.Equal(t, tt.expectedStatus.CurrentHash, status.CurrentHash)
			// Compare time with a tolerance of 1 ms (time.Now() is called in the function)
			assert.True(t, status.Created.Time.Sub(tt.expectedStatus.Created.Time) < time.Millisecond)
			assert.True(t, status.LastForceSyncTime.Time.Sub(tt.expectedStatus.LastForceSyncTime.Time) < time.Millisecond)
		})
	}
}
