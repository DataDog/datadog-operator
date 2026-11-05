// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadoggenericresource

import (
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
	instance := &v1alpha1.DatadogGenericResource{Spec: v1alpha1.DatadogGenericResourceSpec{JsonSpec: `{"restriction_policy":null}`}}
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
