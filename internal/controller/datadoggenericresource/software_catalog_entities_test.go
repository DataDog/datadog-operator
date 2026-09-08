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

func Test_upsertSoftwareCatalogEntity_marshalling(t *testing.T) {
	tests := []struct {
		name      string
		jsonSpec  string
		wantErr   bool
		wantName  string
		wantOwner string
	}{
		{
			name: "valid service entity",
			jsonSpec: `{
				"apiVersion": "v3",
				"kind": "service",
				"metadata": {
					"name": "test-service",
					"owner": "test-team"
				},
				"spec": {
					"tier": "1"
				}
			}`,
			wantName:  "test-service",
			wantOwner: "test-team",
		},
		{name: "empty jsonSpec", jsonSpec: "", wantErr: true},
		{name: "invalid JSON", jsonSpec: `{invalid`, wantErr: true},
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
				_, _ = w.Write([]byte(`{"data":[{"id":"entity-abc","type":"entity","attributes":{"kind":"service","name":"test-service"},"meta":{"createdAt":"2026-01-01T00:00:00Z"}}]}`))
			}))
			defer server.Close()

			cfg := datadogapi.NewConfiguration()
			cfg.HTTPClient = server.Client()
			client := datadogV2.NewSoftwareCatalogApi(datadogapi.NewAPIClient(cfg))
			auth := setupTestAuth(server.URL)

			instance := &v1alpha1.DatadogGenericResource{
				Spec: v1alpha1.DatadogGenericResourceSpec{JsonSpec: tt.jsonSpec},
			}

			_, err := upsertSoftwareCatalogEntity(auth, client, instance)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)

			var sent datadogV2.EntityV3
			require.NoError(t, json.Unmarshal(capturedBody, &sent))
			require.NotNil(t, sent.EntityV3Service)
			assert.Equal(t, tt.wantName, sent.EntityV3Service.Metadata.Name)
			assert.Equal(t, tt.wantOwner, sent.EntityV3Service.Metadata.GetOwner())
		})
	}
}

func Test_createResource_populatesIDAndCreatedTime(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":[{"id":"entity-abc","type":"entity","attributes":{"kind":"service","name":"test-service"},"meta":{"createdAt":"2026-01-01T00:00:00Z"}}]}`))
	}))
	defer server.Close()

	cfg := datadogapi.NewConfiguration()
	cfg.HTTPClient = server.Client()
	client := datadogV2.NewSoftwareCatalogApi(datadogapi.NewAPIClient(cfg))
	auth := setupTestAuth(server.URL)

	handler := &SoftwareCatalogEntityHandler{client: client}
	instance := &v1alpha1.DatadogGenericResource{
		Spec: v1alpha1.DatadogGenericResourceSpec{
			JsonSpec: `{"apiVersion":"v3","kind":"service","metadata":{"name":"test-service"}}`,
		},
	}

	result, err := handler.createResource(auth, instance)
	require.NoError(t, err)
	assert.Equal(t, "entity-abc", result.ID)
	require.NotNil(t, result.CreatedTime)
	assert.Equal(t, 2026, result.CreatedTime.Year())
}

func Test_getSoftwareCatalogEntity(t *testing.T) {
	tests := []struct {
		name       string
		entityID   string
		statusCode int
		body       string
		wantErr    bool
	}{
		{
			name:       "entity found",
			entityID:   "entity-abc",
			statusCode: http.StatusOK,
			body:       `{"data":[{"id":"entity-abc","type":"entity","attributes":{"kind":"service","name":"test-service"}}]}`,
		},
		{
			name:       "entity not found (empty data)",
			entityID:   "entity-missing",
			statusCode: http.StatusOK,
			body:       `{"data":[]}`,
			wantErr:    true,
		},
		{
			name:     "empty entityID",
			entityID: "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newTestHTTPServer(tt.statusCode, tt.body)
			defer server.Close()

			cfg := datadogapi.NewConfiguration()
			cfg.HTTPClient = server.Client()
			client := datadogV2.NewSoftwareCatalogApi(datadogapi.NewAPIClient(cfg))
			auth := setupTestAuth(server.URL)

			_, err := getSoftwareCatalogEntity(auth, client, tt.entityID)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func Test_deleteSoftwareCatalogEntity_idempotent(t *testing.T) {
	for _, tc := range defaultDeleteCases {
		t.Run(tc.name, func(t *testing.T) {
			server := newTestHTTPServer(tc.statusCode, tc.body)
			defer server.Close()

			cfg := datadogapi.NewConfiguration()
			cfg.HTTPClient = server.Client()
			client := datadogV2.NewSoftwareCatalogApi(datadogapi.NewAPIClient(cfg))
			auth := setupTestAuth(server.URL)

			err := deleteSoftwareCatalogEntity(auth, client, "entity-123")
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func Test_softwareCatalogEntityHandler_refreshState(t *testing.T) {
	handler := &SoftwareCatalogEntityHandler{}
	state, err := handler.refreshState(nil, &v1alpha1.DatadogGenericResource{})
	assert.NoError(t, err)
	assert.Nil(t, state)
}
