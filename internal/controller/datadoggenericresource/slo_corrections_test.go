// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadoggenericresource

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	datadogapi "github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

func Test_createSLOCorrection_marshalling(t *testing.T) {
	tests := []struct {
		name         string
		jsonSpec     string
		wantErr      bool
		wantSloID    string
		wantCategory string
	}{
		{
			name: "valid spec",
			jsonSpec: `{
				"data": {
					"type": "correction",
					"attributes": {
						"category": "Scheduled Maintenance",
						"slo_id": "slo-abc",
						"start": 1735689600
					}
				}
			}`,
			wantSloID:    "slo-abc",
			wantCategory: "Scheduled Maintenance",
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
				_, _ = w.Write([]byte(`{"data":{"id":"correction-abc","type":"correction","attributes":{"slo_id":"slo-abc"}}}`))
			}))
			defer server.Close()

			cfg := datadogapi.NewConfiguration()
			cfg.HTTPClient = server.Client()
			client := datadogV1.NewServiceLevelObjectiveCorrectionsApi(datadogapi.NewAPIClient(cfg))
			auth := setupTestAuth(server.URL)

			instance := &v1alpha1.DatadogGenericResource{
				Spec: v1alpha1.DatadogGenericResourceSpec{
					JsonSpec: tt.jsonSpec,
				},
			}

			_, err := createSLOCorrection(auth, client, instance)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)

			var sent datadogV1.SLOCorrectionCreateRequest
			require.NoError(t, json.Unmarshal(capturedBody, &sent))
			assert.Equal(t, tt.wantSloID, sent.Data.Attributes.GetSloId())
			assert.Equal(t, tt.wantCategory, string(sent.Data.Attributes.GetCategory()))
		})
	}
}

func Test_updateSLOCorrection_marshalling(t *testing.T) {
	tests := []struct {
		name         string
		statusID     string
		jsonSpec     string
		wantErr      bool
		wantSentPath string
	}{
		{
			name:     "valid update",
			statusID: "correction-abc",
			jsonSpec: `{
				"data": {
					"attributes": {
						"category": "Deployment"
					}
				}
			}`,
			wantSentPath: "/api/v1/slo/correction/correction-abc",
		},
		{
			name:     "empty status ID",
			statusID: "",
			jsonSpec: `{"data":{"attributes":{"category":"Deployment"}}}`,
			wantErr:  true,
		},
		{
			name:     "empty jsonSpec",
			statusID: "correction-abc",
			jsonSpec: "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedBody []byte
			var capturedPath string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var err error
				capturedPath = r.URL.Path
				capturedBody, err = io.ReadAll(r.Body)
				require.NoError(t, err)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"data":{"id":"correction-abc","type":"correction","attributes":{"category":"Deployment"}}}`))
			}))
			defer server.Close()

			cfg := datadogapi.NewConfiguration()
			cfg.HTTPClient = server.Client()
			client := datadogV1.NewServiceLevelObjectiveCorrectionsApi(datadogapi.NewAPIClient(cfg))
			auth := setupTestAuth(server.URL)

			instance := &v1alpha1.DatadogGenericResource{
				Spec: v1alpha1.DatadogGenericResourceSpec{
					JsonSpec: tt.jsonSpec,
				},
			}
			instance.Status.Id = tt.statusID

			_, err := updateSLOCorrection(auth, client, instance)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)

			// The ID travels only in the URL path, never in the request body.
			assert.Equal(t, tt.wantSentPath, capturedPath)

			var sent datadogV1.SLOCorrectionUpdateRequest
			require.NoError(t, json.Unmarshal(capturedBody, &sent))
			assert.Equal(t, datadogV1.SLOCORRECTIONTYPE_CORRECTION, sent.Data.GetType())
		})
	}
}

func Test_updateSLOCorrection_sloIdChange(t *testing.T) {
	tests := []struct {
		name       string
		specSloID  string
		liveSloID  string
		wantErr    bool
		wantPUTHit bool
	}{
		{
			name:       "slo_id changed is rejected",
			specSloID:  "slo-new",
			liveSloID:  "slo-original",
			wantErr:    true,
			wantPUTHit: false,
		},
		{
			name:       "slo_id unchanged proceeds",
			specSloID:  "slo-original",
			liveSloID:  "slo-original",
			wantPUTHit: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var putHit bool
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				switch r.Method {
				case http.MethodGet:
					_, _ = fmt.Fprintf(w, `{"data":{"id":"correction-abc","type":"correction","attributes":{"slo_id":%q}}}`, tt.liveSloID)
				case http.MethodPatch, http.MethodPut:
					putHit = true
					_, _ = fmt.Fprintf(w, `{"data":{"id":"correction-abc","type":"correction","attributes":{"slo_id":%q}}}`, tt.specSloID)
				}
			}))
			defer server.Close()

			cfg := datadogapi.NewConfiguration()
			cfg.HTTPClient = server.Client()
			client := datadogV1.NewServiceLevelObjectiveCorrectionsApi(datadogapi.NewAPIClient(cfg))
			auth := setupTestAuth(server.URL)

			instance := &v1alpha1.DatadogGenericResource{
				Spec: v1alpha1.DatadogGenericResourceSpec{
					JsonSpec: fmt.Sprintf(`{"data":{"attributes":{"slo_id":%q,"category":"Deployment"}}}`, tt.specSloID),
				},
			}
			instance.Status.Id = "correction-abc"

			_, err := updateSLOCorrection(auth, client, instance)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "slo_id cannot be changed")
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantPUTHit, putHit)
		})
	}
}
