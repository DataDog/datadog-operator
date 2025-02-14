package datadoggenericresource

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

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
