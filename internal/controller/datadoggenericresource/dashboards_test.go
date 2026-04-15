// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadoggenericresource

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

func Test_updateStatusFromDashboard(t *testing.T) {
	hash := "test-hash"
	createdAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name           string
		dashboard      datadogV1.Dashboard
		expectedStatus v1alpha1.DatadogGenericResourceStatus
	}{
		{
			name: "all fields populated",
			dashboard: func() datadogV1.Dashboard {
				d := datadogV1.Dashboard{}
				d.SetId("abc-123")
				d.SetAuthorHandle("wassim.dhif@datadoghq.com")
				d.SetCreatedAt(createdAt)
				return d
			}(),
			expectedStatus: v1alpha1.DatadogGenericResourceStatus{
				Id:          "abc-123",
				Creator:     "wassim.dhif@datadoghq.com",
				SyncStatus:  v1alpha1.DatadogSyncStatusOK,
				CurrentHash: hash,
			},
		},
		{
			name: "missing author handle",
			dashboard: func() datadogV1.Dashboard {
				d := datadogV1.Dashboard{}
				d.SetId("abc-456")
				d.SetCreatedAt(createdAt)
				return d
			}(),
			expectedStatus: v1alpha1.DatadogGenericResourceStatus{
				Id:          "abc-456",
				Creator:     "",
				SyncStatus:  v1alpha1.DatadogSyncStatusOK,
				CurrentHash: hash,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := &v1alpha1.DatadogGenericResourceStatus{}
			updateStatusFromDashboard(tt.dashboard, status, hash)

			assert.Equal(t, tt.expectedStatus.Id, status.Id)
			assert.Equal(t, tt.expectedStatus.Creator, status.Creator)
			assert.Equal(t, tt.expectedStatus.SyncStatus, status.SyncStatus)
			assert.Equal(t, tt.expectedStatus.CurrentHash, status.CurrentHash)
			assert.Equal(t, createdAt, status.Created.Time)
			assert.Equal(t, createdAt, status.LastForceSyncTime.Time)
		})
	}
}

func Test_DashboardHandler_getHandler(t *testing.T) {
	handler := getHandler(v1alpha1.Dashboard)
	assert.IsType(t, &DashboardHandler{}, handler)
}
