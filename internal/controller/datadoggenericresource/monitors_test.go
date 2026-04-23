// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadoggenericresource

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	datadogapi "github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
)

func Test_deleteMonitor(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		wantErr    bool
	}{
		{
			name:       "200 deletes cleanly",
			statusCode: http.StatusOK,
			body:       `{"deleted_monitor_id": 12345}`,
		},
		{
			name:       "404 treated as success (already deleted in UI)",
			statusCode: http.StatusNotFound,
			body:       `{"errors":["Monitor not found"]}`,
		},
		{
			name:       "400 composite-reference error is propagated",
			statusCode: http.StatusBadRequest,
			body:       `{"errors":["monitor [12345] is referenced in composite monitors: [67890]"]}`,
			wantErr:    true,
		},
		{
			name:       "500 error is propagated",
			statusCode: http.StatusInternalServerError,
			body:       `{"errors":["Internal Server Error"]}`,
			wantErr:    true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.statusCode)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()

			cfg := datadogapi.NewConfiguration()
			cfg.HTTPClient = server.Client()
			apiClient := datadogapi.NewAPIClient(cfg)
			client := datadogV1.NewMonitorsApi(apiClient)
			auth := setupTestAuth(server.URL)

			err := deleteMonitor(auth, client, "12345")
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
