// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadoggenericresource

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	datadogapi "github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

func Test_extractSLOState(t *testing.T) {
	tests := []struct {
		name          string
		response      string
		expectedState string
		wantErr       string
	}{
		{
			name: "uses overall status state",
			response: `{
				"data": {
					"attributes": {
						"slos": [
							{
								"data": {
									"id": "slo-123",
									"type": "slo",
									"attributes": {
										"overall_status": [
											{"state": "warning", "timeframe": "7d"}
										],
										"status": {"state": "ok"}
									}
								}
							}
						]
					}
				}
			}`,
			expectedState: "warning",
		},
		{
			name: "falls back to primary status state",
			response: `{
				"data": {
					"attributes": {
						"slos": [
							{
								"data": {
									"id": "slo-123",
									"type": "slo",
									"attributes": {
										"status": {"state": "breached"}
									}
								}
							}
						]
					}
				}
			}`,
			expectedState: "breached",
		},
		{
			name: "filters by SLO ID",
			response: `{
				"data": {
					"attributes": {
						"slos": [
							{
								"data": {
									"id": "other-slo",
									"type": "slo",
									"attributes": {
										"overall_status": [
											{"state": "ok", "timeframe": "7d"}
										]
									}
								}
							},
							{
								"data": {
									"id": "slo-123",
									"type": "slo",
									"attributes": {
										"overall_status": [
											{"state": "no_data", "timeframe": "7d"}
										]
									}
								}
							}
						]
					}
				}
			}`,
			expectedState: "no_data",
		},
		{
			name: "errors when SLO is missing",
			response: `{
				"data": {
					"attributes": {
						"slos": []
					}
				}
			}`,
			wantErr: "error getting SLO state: SLO slo-123 not found",
		},
		{
			name: "errors when state is missing",
			response: `{
				"data": {
					"attributes": {
						"slos": [
							{
								"data": {
									"id": "slo-123",
									"type": "slo",
									"attributes": {}
								}
							}
						]
					}
				}
			}`,
			wantErr: "error getting SLO state: SLO slo-123 does not include state",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var response datadogV1.SearchSLOResponse
			require.NoError(t, json.Unmarshal([]byte(tt.response), &response))

			state, err := extractSLOState(response, "slo-123")
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expectedState, state)
		})
	}
}

func Test_getSLOState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/slo/search", r.URL.Path)
		assert.Equal(t, "Example SLO", r.URL.Query().Get("query"))
		assert.Equal(t, "100", r.URL.Query().Get("page[size]"))

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"data": {
				"attributes": {
					"slos": [
						{
							"data": {
								"id": "slo-123",
								"type": "slo",
								"attributes": {
									"overall_status": [
										{"state": "ok", "timeframe": "7d"}
									]
								}
							}
						}
					]
				}
			}
		}`))
	}))
	defer server.Close()

	cfg := datadogapi.NewConfiguration()
	cfg.HTTPClient = server.Client()
	client := datadogV1.NewServiceLevelObjectivesApi(datadogapi.NewAPIClient(cfg))
	auth := setupTestAuth(server.URL)

	state, err := getSLOState(auth, client, "slo-123", "Example SLO")
	require.NoError(t, err)
	require.NotNil(t, state)
	assert.Equal(t, "ok", *state)
}

func Test_getSLONameFromSpec(t *testing.T) {
	tests := []struct {
		name         string
		jsonSpec     string
		expectedName string
		wantErr      string
	}{
		{
			name:         "returns name",
			jsonSpec:     `{"name":"Example SLO","type":"metric"}`,
			expectedName: "Example SLO",
		},
		{
			name:     "invalid JSON",
			jsonSpec: `{`,
			wantErr:  "error unmarshalling SLO spec: unexpected end of JSON input",
		},
		{
			name:     "missing name",
			jsonSpec: `{"type":"metric"}`,
			wantErr:  "error getting SLO state: SLO spec does not include name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			instance := &v1alpha1.DatadogGenericResource{
				Spec: v1alpha1.DatadogGenericResourceSpec{
					JsonSpec: tt.jsonSpec,
				},
			}

			name, err := getSLONameFromSpec(instance)
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expectedName, name)
		})
	}
}
