// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadoggenericresource

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	datadogapi "github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

func Test_createMonitorNotificationRule_marshalling(t *testing.T) {
	tests := []struct {
		name            string
		jsonSpec        string
		wantErr         bool
		wantRuleName    string
		wantRecipients  []string
	}{
		{
			name: "valid spec with recipients",
			jsonSpec: `{
				"data": {
					"type": "monitor-notification-rule",
					"attributes": {
						"name": "test-rule",
						"recipients": ["@slack-test-channel"]
					}
				}
			}`,
			wantRuleName:   "test-rule",
			wantRecipients: []string{"@slack-test-channel"},
		},
		{
			name:     "empty jsonSpec",
			jsonSpec: "",
			wantErr:  true,
		},
		{
			name:     "invalid JSON",
			jsonSpec: `{invalid`,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedBody []byte
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var err error
				capturedBody, err = io.ReadAll(r.Body)
				require.NoError(t, err)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"data":{"id":"rule-abc","type":"monitor-notification-rule","attributes":{"name":"test-rule"}}}`))
			}))
			defer server.Close()

			cfg := datadogapi.NewConfiguration()
			cfg.HTTPClient = server.Client()
			client := datadogV2.NewMonitorsApi(datadogapi.NewAPIClient(cfg))
			auth := setupTestAuth(server.URL)

			instance := &v1alpha1.DatadogGenericResource{
				Spec: v1alpha1.DatadogGenericResourceSpec{
					JsonSpec: tt.jsonSpec,
				},
			}

			_, err := createMonitorNotificationRule(auth, client, instance)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)

			var sent datadogV2.MonitorNotificationRuleCreateRequest
			require.NoError(t, json.Unmarshal(capturedBody, &sent))
			assert.Equal(t, tt.wantRuleName, sent.Data.Attributes.GetName())
		})
	}
}

func Test_updateMonitorNotificationRule_marshalling(t *testing.T) {
	tests := []struct {
		name         string
		statusID     string
		jsonSpec     string
		wantErr      bool
		wantSentID   string
		wantRuleName string
	}{
		{
			name:     "valid update",
			statusID: "rule-abc",
			jsonSpec: `{
				"data": {
					"attributes": {
						"name": "updated-rule",
						"recipients": ["@pagerduty-team"]
					}
				}
			}`,
			wantSentID:   "rule-abc",
			wantRuleName: "updated-rule",
		},
		{
			name:     "empty status ID",
			statusID: "",
			jsonSpec: `{"data":{"attributes":{"name":"x"}}}`,
			wantErr:  true,
		},
		{
			name:     "empty jsonSpec",
			statusID: "rule-abc",
			jsonSpec: "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedBody []byte
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var err error
				capturedBody, err = io.ReadAll(r.Body)
				require.NoError(t, err)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"data":{"id":"rule-abc","type":"monitor-notification-rule","attributes":{"name":"updated-rule"}}}`))
			}))
			defer server.Close()

			cfg := datadogapi.NewConfiguration()
			cfg.HTTPClient = server.Client()
			client := datadogV2.NewMonitorsApi(datadogapi.NewAPIClient(cfg))
			auth := setupTestAuth(server.URL)

			instance := &v1alpha1.DatadogGenericResource{
				Spec: v1alpha1.DatadogGenericResourceSpec{
					JsonSpec: tt.jsonSpec,
				},
			}
			instance.Status.Id = tt.statusID

			_, err := updateMonitorNotificationRule(auth, client, instance)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)

			var sent datadogV2.MonitorNotificationRuleUpdateRequest
			require.NoError(t, json.Unmarshal(capturedBody, &sent))
			assert.Equal(t, tt.wantSentID, sent.Data.Id)
			assert.Equal(t, tt.wantRuleName, sent.Data.Attributes.GetName())
		})
	}
}
