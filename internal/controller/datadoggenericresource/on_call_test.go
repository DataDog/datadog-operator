// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadoggenericresource

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	datadogapi "github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	ctrutils "github.com/DataDog/datadog-operator/pkg/controller/utils"
)

type onCallHandlerTestCase struct {
	name        string
	apiType     string
	endpoint    string
	nestedField string
	jsonSpec    string
	newHandler  func(*datadogV2.OnCallApi) ResourceHandler
}

func onCallHandlerTestCases() []onCallHandlerTestCase {
	return []onCallHandlerTestCase{
		{
			name:        "schedule",
			apiType:     "schedules",
			endpoint:    "/api/v2/on-call/schedules",
			nestedField: "layers",
			jsonSpec: `{
				"data": {
					"type": "schedules",
					"attributes": {
						"name": "Primary rotation",
						"time_zone": "UTC",
						"layers": [{
							"name": "Primary",
							"effective_date": "2026-11-05T00:00:00Z",
							"rotation_start": "2026-11-05T00:00:00Z",
							"interval": {"days": 7},
							"members": [{"user": {"id": "user-123"}}]
						}]
					},
					"relationships": {
						"teams": {"data": [{"id": "team-123", "type": "teams"}]}
					}
				}
			}`,
			newHandler: func(client *datadogV2.OnCallApi) ResourceHandler {
				return &OnCallScheduleHandler{client: client}
			},
		},
		{
			name:        "escalation policy",
			apiType:     "policies",
			endpoint:    "/api/v2/on-call/escalation-policies",
			nestedField: "steps",
			jsonSpec: `{
				"data": {
					"type": "policies",
					"attributes": {
						"name": "Primary escalation",
						"resolve_page_on_policy_end": false,
						"retries": 0,
						"steps": [{
							"assignment": "default",
							"escalate_after_seconds": 300,
							"targets": [{"id": "user-123", "type": "users"}]
						}]
					},
					"relationships": {
						"teams": {"data": [{"id": "team-123", "type": "teams"}]}
					}
				}
			}`,
			newHandler: func(client *datadogV2.OnCallApi) ResourceHandler {
				return &OnCallEscalationPolicyHandler{client: client}
			},
		},
	}
}

func newOnCallTestClient(server *httptest.Server) *datadogV2.OnCallApi {
	cfg := datadogapi.NewConfiguration()
	cfg.HTTPClient = server.Client()
	return datadogV2.NewOnCallApi(datadogapi.NewAPIClient(cfg))
}

func TestOnCallHandlers_Create(t *testing.T) {
	for _, tc := range onCallHandlerTestCases() {
		t.Run(tc.name, func(t *testing.T) {
			bodies := make(chan []byte, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Equal(t, tc.endpoint, r.URL.Path)
				body, err := io.ReadAll(r.Body)
				assert.NoError(t, err)
				bodies <- body
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				_, _ = fmt.Fprintf(w, `{"data":{"id":"remote-123","type":%q}}`, tc.apiType)
			}))
			defer server.Close()

			instance := &v1alpha1.DatadogGenericResource{
				Spec: v1alpha1.DatadogGenericResourceSpec{JsonSpec: tc.jsonSpec},
			}
			result, err := tc.newHandler(newOnCallTestClient(server)).createResource(setupTestAuth(server.URL), instance)
			require.NoError(t, err)
			assert.Equal(t, "remote-123", result.ID)
			assert.Nil(t, result.CreatedTime, "On-Call responses do not expose creation metadata")
			assert.Empty(t, result.Creator)
			assert.JSONEq(t, tc.jsonSpec, string(<-bodies))
		})
	}
}

func TestOnCallHandlers_UpdateUsesStoredID(t *testing.T) {
	for _, tc := range onCallHandlerTestCases() {
		for _, specID := range []string{"", "another-resource"} {
			t.Run(tc.name+"/spec ID="+specID, func(t *testing.T) {
				var spec map[string]any
				require.NoError(t, json.Unmarshal([]byte(tc.jsonSpec), &spec))
				data := spec["data"].(map[string]any)
				if specID != "" {
					data["id"] = specID
				}
				// Unlike create models, update models expose IDs for existing
				// layers and steps. Those IDs must survive payload conversion.
				attributes := data["attributes"].(map[string]any)
				items := attributes[tc.nestedField].([]any)
				items[0].(map[string]any)["id"] = "existing-child"
				jsonSpec, err := json.Marshal(spec)
				require.NoError(t, err)
				data["id"] = "remote-123"
				expectedBody, err := json.Marshal(spec)
				require.NoError(t, err)

				bodies := make(chan []byte, 1)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					assert.Equal(t, http.MethodPut, r.Method)
					assert.Equal(t, tc.endpoint+"/remote-123", r.URL.Path)
					body, readErr := io.ReadAll(r.Body)
					assert.NoError(t, readErr)
					bodies <- body
					w.Header().Set("Content-Type", "application/json")
					_, _ = fmt.Fprintf(w, `{"data":{"id":"remote-123","type":%q}}`, tc.apiType)
				}))
				defer server.Close()

				instance := &v1alpha1.DatadogGenericResource{
					Spec:   v1alpha1.DatadogGenericResourceSpec{JsonSpec: string(jsonSpec)},
					Status: v1alpha1.DatadogGenericResourceStatus{Id: "remote-123"},
				}
				err = tc.newHandler(newOnCallTestClient(server)).updateResource(setupTestAuth(server.URL), instance)
				require.NoError(t, err)
				assert.JSONEq(t, string(expectedBody), string(<-bodies), "retain attributes, nested IDs, explicit false/zero, and team relationships")
			})
		}
	}
}

func TestOnCallHandlers_RejectInvalidSpecsBeforeRequest(t *testing.T) {
	for _, tc := range onCallHandlerTestCases() {
		for _, jsonSpec := range []string{"", "{invalid", "{}", "null"} {
			for _, operation := range []string{"create", "update"} {
				t.Run(tc.name+"/"+operation+"/"+jsonSpec, func(t *testing.T) {
					var requests atomic.Int64
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
						requests.Add(1)
						w.WriteHeader(http.StatusInternalServerError)
					}))
					defer server.Close()
					h := tc.newHandler(newOnCallTestClient(server))
					instance := &v1alpha1.DatadogGenericResource{
						Spec:   v1alpha1.DatadogGenericResourceSpec{JsonSpec: jsonSpec},
						Status: v1alpha1.DatadogGenericResourceStatus{Id: "remote-123"},
					}
					var err error
					if operation == "create" {
						_, err = h.createResource(setupTestAuth(server.URL), instance)
					} else {
						err = h.updateResource(setupTestAuth(server.URL), instance)
					}
					assert.Error(t, err)
					assert.Zero(t, requests.Load())
					if jsonSpec == "{invalid" {
						assert.True(t, ctrutils.IsPermanentAPIError(err))
					}
				})
			}
		}
	}
}

func TestOnCallHandlers_RequireStoredID(t *testing.T) {
	for _, tc := range onCallHandlerTestCases() {
		t.Run(tc.name, func(t *testing.T) {
			var requests atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requests.Add(1)
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer server.Close()
			h := tc.newHandler(newOnCallTestClient(server))
			instance := &v1alpha1.DatadogGenericResource{
				Spec: v1alpha1.DatadogGenericResourceSpec{JsonSpec: tc.jsonSpec},
			}
			auth := setupTestAuth(server.URL)
			assert.Error(t, h.getResource(auth, instance))
			assert.Error(t, h.updateResource(auth, instance))
			assert.Error(t, h.deleteResource(auth, instance))
			assert.Zero(t, requests.Load())
		})
	}
}

func TestOnCallHandlers_Get(t *testing.T) {
	for _, tc := range onCallHandlerTestCases() {
		for _, statusCode := range []int{http.StatusOK, http.StatusNotFound, http.StatusInternalServerError} {
			t.Run(fmt.Sprintf("%s/%d", tc.name, statusCode), func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					assert.Equal(t, http.MethodGet, r.Method)
					assert.Equal(t, tc.endpoint+"/remote-123", r.URL.Path)
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(statusCode)
					if statusCode == http.StatusOK {
						_, _ = fmt.Fprintf(w, `{"data":{"id":"remote-123","type":%q}}`, tc.apiType)
					} else {
						_, _ = w.Write([]byte(`{"errors":["request failed"]}`))
					}
				}))
				defer server.Close()
				instance := &v1alpha1.DatadogGenericResource{
					Status: v1alpha1.DatadogGenericResourceStatus{Id: "remote-123"},
				}
				err := tc.newHandler(newOnCallTestClient(server)).getResource(setupTestAuth(server.URL), instance)
				if statusCode == http.StatusOK {
					assert.NoError(t, err)
				} else {
					assert.Error(t, err, "GET 404 must remain visible so reconciliation recreates the resource")
				}
			})
		}
	}
}

func TestOnCallHandlers_WriteErrors(t *testing.T) {
	for _, tc := range onCallHandlerTestCases() {
		for _, statusCode := range []int{http.StatusBadRequest, http.StatusInternalServerError} {
			for _, operation := range []string{"create", "update"} {
				t.Run(fmt.Sprintf("%s/%s/%d", tc.name, operation, statusCode), func(t *testing.T) {
					server := newTestHTTPServer(statusCode, `{"errors":["request failed"]}`)
					defer server.Close()
					h := tc.newHandler(newOnCallTestClient(server))
					instance := &v1alpha1.DatadogGenericResource{
						Spec:   v1alpha1.DatadogGenericResourceSpec{JsonSpec: tc.jsonSpec},
						Status: v1alpha1.DatadogGenericResourceStatus{Id: "remote-123"},
					}
					var err error
					if operation == "create" {
						_, err = h.createResource(setupTestAuth(server.URL), instance)
					} else {
						err = h.updateResource(setupTestAuth(server.URL), instance)
					}
					assert.Error(t, err)
					assert.Equal(t, statusCode == http.StatusBadRequest, ctrutils.IsPermanentAPIError(err))
				})
			}
		}
	}
}

func TestOnCallHandlers_DeleteIdempotency(t *testing.T) {
	for _, tc := range onCallHandlerTestCases() {
		for _, deleteCase := range defaultDeleteCases {
			t.Run(tc.name+"/"+deleteCase.name, func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					assert.Equal(t, http.MethodDelete, r.Method)
					assert.Equal(t, tc.endpoint+"/remote-123", r.URL.Path)
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(deleteCase.statusCode)
					_, _ = w.Write([]byte(deleteCase.body))
				}))
				defer server.Close()
				instance := &v1alpha1.DatadogGenericResource{
					Status: v1alpha1.DatadogGenericResourceStatus{Id: "remote-123"},
				}
				err := tc.newHandler(newOnCallTestClient(server)).deleteResource(setupTestAuth(server.URL), instance)
				if deleteCase.wantErr {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
			})
		}
	}
}

func TestOnCallHandlers_RefreshStateIsNoOp(t *testing.T) {
	for _, tc := range onCallHandlerTestCases() {
		t.Run(tc.name, func(t *testing.T) {
			// A nil client also catches accidental API calls in this no-op path.
			state, err := tc.newHandler(nil).refreshState(context.Background(), &v1alpha1.DatadogGenericResource{})
			assert.NoError(t, err)
			assert.Nil(t, state)
		})
	}
}
