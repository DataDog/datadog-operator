// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadoggenericresource

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	datadogapi "github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

func TestMonitorComparisonOptionsAreScoped(t *testing.T) {
	var policyReads, ordinaryReads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "DUMMY_API_KEY", r.Header.Get("DD-API-KEY"))
		assert.Equal(t, "DUMMY_APP_KEY", r.Header.Get("DD-APPLICATION-KEY"))
		if r.URL.Query().Get("with_restriction_policy") == "true" {
			assert.Equal(t, "true", r.URL.Query().Get("with_assets"), "preserve the SDK query parameters")
			policyReads.Add(1)
		} else {
			assert.Empty(t, r.URL.RawQuery)
			ordinaryReads.Add(1)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"query":"q","type":"query alert"}`)
	}))
	defer server.Close()
	cfg := datadogapi.NewConfiguration()
	cfg.HTTPClient = server.Client()
	api := datadogV1.NewMonitorsApi(datadogapi.NewAPIClient(cfg))
	instance := &v1alpha1.DatadogGenericResource{Spec: v1alpha1.DatadogGenericResourceSpec{JsonSpec: `{"restriction_policy":{"bindings":[{"relation":"editor","principals":["user:owner"]}]}}`}}
	desired, err := buildMonitorUpdateRequest(instance)
	require.NoError(t, err)
	auth := setupTestAuth(server.URL)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Go(func() {
			// A policy read must leave the shared client usable concurrently for
			// ordinary GETs, including its auth and HTTP transport configuration.
			comparisonAPI := monitorComparisonClient(api, desired)
			_, err := getMonitor(auth, comparisonAPI, "12345", *datadogV1.NewGetMonitorOptionalParameters().WithWithAssets(true))
			assert.NoError(t, err)
		})
		wg.Go(func() {
			_, err := getMonitor(auth, api, "12345")
			assert.NoError(t, err)
		})
	}
	wg.Wait()
	assert.EqualValues(t, 10, policyReads.Load())
	assert.EqualValues(t, 10, ordinaryReads.Load())
}

func TestMonitorIgnoredPolicyInputs(t *testing.T) {
	for _, input := range []string{
		`{"restriction_policy":null}`,
		`{"query":"q","type":"future monitor","restriction_policy":null}`,
	} {
		t.Run(input, func(t *testing.T) {
			var raw map[string]any
			require.NoError(t, json.Unmarshal([]byte(input), &raw))
			monitorType := any("query alert")
			if raw["type"] != nil {
				monitorType = raw["type"]
			}
			var mu sync.Mutex
			existing := map[string]any{"bindings": []any{map[string]any{"relation": "editor", "principals": []string{"user:owner"}}}}
			var puts int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodGet {
					assert.Empty(t, r.URL.Query().Get("with_restriction_policy"), "ignored inputs must not require policy data")
				} else if r.Method == http.MethodPut {
					var payload map[string]any
					assert.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
					data, err := json.Marshal(payload)
					assert.NoError(t, err)
					assert.JSONEq(t, input, string(data), "keep the SDK write payload unchanged")
					puts++
					// The real update API accepts null but leaves bindings unchanged.
				}
				assert.NoError(t, json.NewEncoder(w).Encode(map[string]any{"query": "q", "type": monitorType, "restriction_policy": existing}))
			}))
			defer server.Close()
			cfg := datadogapi.NewConfiguration()
			cfg.HTTPClient = server.Client()
			h := &MonitorHandler{client: datadogV1.NewMonitorsApi(datadogapi.NewAPIClient(cfg))}
			instance := &v1alpha1.DatadogGenericResource{
				Spec:   v1alpha1.DatadogGenericResourceSpec{JsonSpec: input},
				Status: v1alpha1.DatadogGenericResourceStatus{Id: "12345"},
			}
			auth := setupTestAuth(server.URL)
			matches, err := h.remoteMatchesSpec(auth, instance)
			require.NoError(t, err)
			assert.True(t, matches, "ignored inputs do not own an existing nonempty policy")
			require.NoError(t, h.updateResource(auth, instance))
			matches, err = h.remoteMatchesSpec(auth, instance)
			require.NoError(t, err)
			assert.True(t, matches, "unchanged bindings must not cause repeated PUTs")
			mu.Lock()
			assert.Equal(t, 1, puts)
			mu.Unlock()
		})
	}
}

func TestMonitorPolicyComparisonPreservesWritePayload(t *testing.T) {
	for _, input := range []string{
		`{"restriction_policy":null}`,
		`{"query":"q","type":"future monitor","restriction_policy":null}`,
	} {
		t.Run(input, func(t *testing.T) {
			instance := &v1alpha1.DatadogGenericResource{Spec: v1alpha1.DatadogGenericResourceSpec{JsonSpec: input}}
			desired, err := buildMonitorUpdateRequest(instance)
			require.NoError(t, err)
			before, err := json.Marshal(desired)
			require.NoError(t, err)
			canonical := monitorComparisonRequest(desired)
			after, err := json.Marshal(desired)
			require.NoError(t, err)
			assert.JSONEq(t, string(before), string(after))
			data, err := json.Marshal(canonical)
			require.NoError(t, err)
			var actual map[string]any
			require.NoError(t, json.Unmarshal(data, &actual))
			assert.NotContains(t, actual, "restriction_policy", "ignored values are excluded from ownership")
		})
	}
}

func TestMonitorPolicyNullResponseMatches(t *testing.T) {
	server := newTestHTTPServer(http.StatusOK, `{"query":"q","type":"query alert","restriction_policy":null}`)
	defer server.Close()
	cfg := datadogapi.NewConfiguration()
	cfg.HTTPClient = server.Client()
	h := &MonitorHandler{client: datadogV1.NewMonitorsApi(datadogapi.NewAPIClient(cfg))}
	instance := &v1alpha1.DatadogGenericResource{
		Spec:   v1alpha1.DatadogGenericResourceSpec{JsonSpec: `{"restriction_policy":null}`},
		Status: v1alpha1.DatadogGenericResourceStatus{Id: "12345"},
	}
	matches, err := h.remoteMatchesSpec(setupTestAuth(server.URL), instance)
	require.NoError(t, err)
	assert.True(t, matches, "a backend returning literal null must match the ignored null input")
}

func TestMonitorEmptyPolicyBindingsClear(t *testing.T) {
	var mu sync.Mutex
	policy := any(map[string]any{"bindings": []any{map[string]any{"relation": "editor", "principals": []string{"user:owner"}}}})
	var puts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			assert.Equal(t, "true", r.URL.Query().Get("with_restriction_policy"))
		} else if r.Method == http.MethodPut {
			var payload map[string]any
			assert.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
			assert.Equal(t, map[string]any{"bindings": []any{}}, payload["restriction_policy"])
			puts++
			policy = map[string]any{"bindings": []any{}}
		}
		assert.NoError(t, json.NewEncoder(w).Encode(map[string]any{"query": "q", "type": "query alert", "restriction_policy": policy}))
	}))
	defer server.Close()
	cfg := datadogapi.NewConfiguration()
	cfg.HTTPClient = server.Client()
	h := &MonitorHandler{client: datadogV1.NewMonitorsApi(datadogapi.NewAPIClient(cfg))}
	instance := &v1alpha1.DatadogGenericResource{
		Spec:   v1alpha1.DatadogGenericResourceSpec{JsonSpec: `{"restriction_policy":{"bindings":[]}}`},
		Status: v1alpha1.DatadogGenericResourceStatus{Id: "12345"},
	}
	auth := setupTestAuth(server.URL)
	matches, err := h.remoteMatchesSpec(auth, instance)
	require.NoError(t, err)
	require.False(t, matches, "an empty binding list owns and clears an existing policy")
	require.NoError(t, h.updateResource(auth, instance))
	matches, err = h.remoteMatchesSpec(auth, instance)
	require.NoError(t, err)
	assert.True(t, matches, "cleared bindings must stop subsequent PUTs")
	mu.Lock()
	assert.Equal(t, 1, puts)
	mu.Unlock()
}
