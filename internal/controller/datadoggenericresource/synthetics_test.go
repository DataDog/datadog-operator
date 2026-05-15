package datadoggenericresource

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_createResultFromSyntheticsTest(t *testing.T) {
	tests := []struct {
		name                 string
		additionalProperties map[string]any
		expectedID           string
		expectedCreator      string
		expectedCreatedTime  *metav1.Time // nil means caller should use fallback
	}{
		{
			name: "valid properties",
			additionalProperties: map[string]any{
				"created_at": "2024-01-01T00:00:00Z",
				"created_by": map[string]any{
					"handle": "test-handle",
				},
			},
			expectedID:          "123456789",
			expectedCreator:     "test-handle",
			expectedCreatedTime: &metav1.Time{Time: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
		},
		{
			name: "missing created_at",
			additionalProperties: map[string]any{
				"created_by": map[string]any{
					"handle": "test-handle",
				},
			},
			expectedID:          "123456789",
			expectedCreator:     "test-handle",
			expectedCreatedTime: nil,
		},
		{
			name: "invalid created_at",
			additionalProperties: map[string]any{
				"created_at": "invalid-date",
				"created_by": map[string]any{
					"handle": "test-handle",
				},
			},
			expectedID:          "123456789",
			expectedCreator:     "test-handle",
			expectedCreatedTime: nil,
		},
		{
			name: "missing created_by",
			additionalProperties: map[string]any{
				"created_at": "2024-01-01T00:00:00Z",
			},
			expectedID:          "123456789",
			expectedCreator:     "",
			expectedCreatedTime: &metav1.Time{Time: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
		},
		{
			name: "missing handle in created_by",
			additionalProperties: map[string]any{
				"created_at": "2024-01-01T00:00:00Z",
				"created_by": map[string]any{},
			},
			expectedID:          "123456789",
			expectedCreator:     "",
			expectedCreatedTime: &metav1.Time{Time: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			syntheticTest := &datadogV1.SyntheticsAPITest{}
			syntheticTest.SetPublicId("123456789")
			result := createResultFromSyntheticsTest(syntheticTest, tt.additionalProperties)
			assert.Equal(t, tt.expectedID, result.ID)
			assert.Equal(t, tt.expectedCreator, result.Creator)
			if tt.expectedCreatedTime == nil {
				assert.Nil(t, result.CreatedTime)
			} else {
				assert.Equal(t, tt.expectedCreatedTime.Time, result.CreatedTime.Time)
			}
		})
	}
}
