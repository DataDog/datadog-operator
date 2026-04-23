// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Tests that each DDGR resource-type delete function treats HTTP 404 from the
// Datadog API as success (idempotent finalization). Without this behavior,
// deleting a DDGR whose remote object was already removed out-of-band would
// leave the Kubernetes resource stuck in Terminating indefinitely once errors
// propagate through the finalizer (CONS-8253).
package datadoggenericresource

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	datadogapi "github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
)

// deleteIdempotencyTestCase is the shared shape for each delete subtest.
type deleteIdempotencyTestCase struct {
	name       string
	statusCode int
	body       string
	wantErr    bool
}

// defaultDeleteCases covers the 404-idempotency contract: 200 and 404 must
// succeed; other error codes must propagate so the finalizer keeps retrying.
var defaultDeleteCases = []deleteIdempotencyTestCase{
	{name: "200 deletes cleanly", statusCode: http.StatusOK, body: `{}`},
	{name: "404 treated as success (already deleted in UI)", statusCode: http.StatusNotFound, body: `{"errors":["Not found"]}`},
	{name: "400 is propagated", statusCode: http.StatusBadRequest, body: `{"errors":["Bad request"]}`, wantErr: true},
	{name: "500 is propagated", statusCode: http.StatusInternalServerError, body: `{"errors":["Internal Server Error"]}`, wantErr: true},
}

// newTestHTTPServer returns a server that always responds with the given
// status code and body, simulating a Datadog API response.
func newTestHTTPServer(statusCode int, body string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		_, _ = w.Write([]byte(body))
	}))
}

func Test_deleteMonitor_idempotent(t *testing.T) {
	for _, tc := range defaultDeleteCases {
		t.Run(tc.name, func(t *testing.T) {
			server := newTestHTTPServer(tc.statusCode, tc.body)
			defer server.Close()

			cfg := datadogapi.NewConfiguration()
			cfg.HTTPClient = server.Client()
			client := datadogV1.NewMonitorsApi(datadogapi.NewAPIClient(cfg))
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

func Test_deleteNotebook_idempotent(t *testing.T) {
	for _, tc := range defaultDeleteCases {
		t.Run(tc.name, func(t *testing.T) {
			server := newTestHTTPServer(tc.statusCode, tc.body)
			defer server.Close()

			cfg := datadogapi.NewConfiguration()
			cfg.HTTPClient = server.Client()
			client := datadogV1.NewNotebooksApi(datadogapi.NewAPIClient(cfg))
			auth := setupTestAuth(server.URL)

			err := deleteNotebook(auth, client, "12345")
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func Test_deleteDashboard_idempotent(t *testing.T) {
	for _, tc := range defaultDeleteCases {
		t.Run(tc.name, func(t *testing.T) {
			server := newTestHTTPServer(tc.statusCode, tc.body)
			defer server.Close()

			cfg := datadogapi.NewConfiguration()
			cfg.HTTPClient = server.Client()
			client := datadogV1.NewDashboardsApi(datadogapi.NewAPIClient(cfg))
			auth := setupTestAuth(server.URL)

			err := deleteDashboard(auth, client, "abc-def-123")
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func Test_deleteSyntheticTest_idempotent(t *testing.T) {
	for _, tc := range defaultDeleteCases {
		t.Run(tc.name, func(t *testing.T) {
			server := newTestHTTPServer(tc.statusCode, tc.body)
			defer server.Close()

			cfg := datadogapi.NewConfiguration()
			cfg.HTTPClient = server.Client()
			client := datadogV1.NewSyntheticsApi(datadogapi.NewAPIClient(cfg))
			auth := setupTestAuth(server.URL)

			err := deleteSyntheticTest(auth, client, "abc-def-ghi")
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func Test_deleteDowntime_idempotent(t *testing.T) {
	for _, tc := range defaultDeleteCases {
		t.Run(tc.name, func(t *testing.T) {
			server := newTestHTTPServer(tc.statusCode, tc.body)
			defer server.Close()

			cfg := datadogapi.NewConfiguration()
			cfg.HTTPClient = server.Client()
			client := datadogV2.NewDowntimesApi(datadogapi.NewAPIClient(cfg))
			auth := setupTestAuth(server.URL)

			err := deleteDowntime(auth, client, "downtime-123")
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
