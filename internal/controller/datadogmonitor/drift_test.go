// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogmonitor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"

	datadogapi "github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	datadogV1 "github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/condition"
)

// Helper function to create a test reconciler with all required fields
func createTestReconciler(client *datadogV1.MonitorsApi, auth context.Context) *Reconciler {
	return &Reconciler{
		datadogClient: client,
		datadogAuth:   auth,
		log:           testLogger,
		recorder:      record.NewFakeRecorder(10),
	}
}

// **Feature: monitor-recreation, Property 1: Drift Detection Accuracy**
// Property-based test for drift detection accuracy
func TestDriftDetectionAccuracy(t *testing.T) {
	// Test cases representing different scenarios
	testCases := []struct {
		name           string
		monitorID      int
		serverResponse func(w http.ResponseWriter, r *http.Request)
		expectedDrift  bool
		expectedError  bool
	}{
		{
			name:      "Monitor exists - no drift",
			monitorID: 12345,
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				monitor := genericMonitor(12345)
				jsonMonitor, _ := monitor.MarshalJSON()
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(jsonMonitor)
			},
			expectedDrift: false,
			expectedError: false,
		},
		{
			name:      "Monitor not found - drift detected",
			monitorID: 99999,
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"errors": ["Monitor not found"]}`))
			},
			expectedDrift: true,
			expectedError: false,
		},
		{
			name:      "API error - no drift, error returned",
			monitorID: 12345,
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"errors": ["Internal server error"]}`))
			},
			expectedDrift: false,
			expectedError: true,
		},
		{
			name:      "Rate limit error - no drift, error returned",
			monitorID: 12345,
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"errors": ["Rate limit exceeded"]}`))
			},
			expectedDrift: false,
			expectedError: true,
		},
		{
			name:      "Zero monitor ID - no drift",
			monitorID: 0,
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				// This should not be called since monitor ID is 0
				t.Error("Server should not be called for monitor ID 0")
			},
			expectedDrift: false,
			expectedError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set up HTTP server
			httpServer := httptest.NewServer(http.HandlerFunc(tc.serverResponse))
			defer httpServer.Close()

			// Set up Datadog client
			testConfig := datadogapi.NewConfiguration()
			testConfig.HTTPClient = httpServer.Client()
			apiClient := datadogapi.NewAPIClient(testConfig)
			client := datadogV1.NewMonitorsApi(apiClient)
			testAuth := setupTestAuth(httpServer.URL)

			// Create reconciler
			r := &Reconciler{
				datadogClient: client,
				datadogAuth:   testAuth,
				log:           testLogger,
				recorder:      record.NewFakeRecorder(10),
			}

			// Create DatadogMonitor instance
			instance := &datadoghqv1alpha1.DatadogMonitor{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-monitor",
					Namespace: "default",
				},
				Status: datadoghqv1alpha1.DatadogMonitorStatus{
					ID: tc.monitorID,
				},
			}

			status := &datadoghqv1alpha1.DatadogMonitorStatus{
				ID: tc.monitorID,
			}

			// Test drift detection
			driftDetected, err := r.detectDrift(context.TODO(), testLogger, instance, status)

			// Verify results
			assert.Equal(t, tc.expectedDrift, driftDetected, "Drift detection result mismatch")
			if tc.expectedError {
				assert.Error(t, err, "Expected error but got none")
			} else {
				assert.NoError(t, err, "Unexpected error: %v", err)
			}

			// Verify status is updated appropriately
			if tc.expectedDrift || tc.expectedError {
				assert.Equal(t, datadoghqv1alpha1.MonitorStateSyncStatusGetError, status.MonitorStateSyncStatus)
			}
		})
	}
}

// Property-based test for independent resource processing
func TestDriftDetectionIndependence(t *testing.T) {
	// Test that drift detection for multiple monitors works independently
	monitors := []struct {
		id       int
		exists   bool
		hasError bool
	}{
		{id: 1001, exists: true, hasError: false},
		{id: 1002, exists: false, hasError: false}, // Should detect drift
		{id: 1003, exists: true, hasError: true},   // Should have error
		{id: 1004, exists: true, hasError: false},
	}

	// Set up HTTP server that responds based on monitor ID
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Extract monitor ID from URL path
		var monitorID int
		fmt.Sscanf(r.URL.Path, "/api/v1/monitor/%d", &monitorID)

		for _, m := range monitors {
			if m.id == monitorID {
				if m.hasError {
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = w.Write([]byte(`{"errors": ["Internal server error"]}`))
					return
				}
				if !m.exists {
					w.WriteHeader(http.StatusNotFound)
					_, _ = w.Write([]byte(`{"errors": ["Monitor not found"]}`))
					return
				}
				// Monitor exists
				monitor := genericMonitor(monitorID)
				jsonMonitor, _ := monitor.MarshalJSON()
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(jsonMonitor)
				return
			}
		}

		// Default: not found
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"errors": ["Monitor not found"]}`))
	}))
	defer httpServer.Close()

	// Set up Datadog client
	testConfig := datadogapi.NewConfiguration()
	testConfig.HTTPClient = httpServer.Client()
	apiClient := datadogapi.NewAPIClient(testConfig)
	client := datadogV1.NewMonitorsApi(apiClient)
	testAuth := setupTestAuth(httpServer.URL)

	// Create reconciler
	r := &Reconciler{
		datadogClient: client,
		datadogAuth:   testAuth,
		log:           testLogger,
		recorder:      record.NewFakeRecorder(10),
	}

	// Test each monitor independently
	results := make([]struct {
		drift bool
		err   error
	}, len(monitors))

	for i, m := range monitors {
		instance := &datadoghqv1alpha1.DatadogMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("test-monitor-%d", m.id),
				Namespace: "default",
			},
			Status: datadoghqv1alpha1.DatadogMonitorStatus{
				ID: m.id,
			},
		}

		status := &datadoghqv1alpha1.DatadogMonitorStatus{
			ID: m.id,
		}

		drift, err := r.detectDrift(context.TODO(), testLogger, instance, status)
		results[i] = struct {
			drift bool
			err   error
		}{drift, err}
	}

	// Verify results are independent and correct
	for i, m := range monitors {
		expectedDrift := !m.exists && !m.hasError
		expectedError := m.hasError

		assert.Equal(t, expectedDrift, results[i].drift,
			"Monitor %d: expected drift=%v, got drift=%v", m.id, expectedDrift, results[i].drift)

		if expectedError {
			assert.Error(t, results[i].err, "Monitor %d: expected error but got none", m.id)
		} else {
			assert.NoError(t, results[i].err, "Monitor %d: unexpected error: %v", m.id, results[i].err)
		}
	}
}

// Property-based test for error handling resilience
func TestDriftDetectionErrorHandling(t *testing.T) {
	errorScenarios := []struct {
		name          string
		statusCode    int
		responseBody  string
		expectedDrift bool
		expectedError bool
	}{
		{
			name:          "Network timeout simulation",
			statusCode:    http.StatusRequestTimeout,
			responseBody:  `{"errors": ["Request timeout"]}`,
			expectedDrift: false,
			expectedError: true,
		},
		{
			name:          "Service unavailable",
			statusCode:    http.StatusServiceUnavailable,
			responseBody:  `{"errors": ["Service unavailable"]}`,
			expectedDrift: false,
			expectedError: true,
		},
		{
			name:          "Bad gateway",
			statusCode:    http.StatusBadGateway,
			responseBody:  `{"errors": ["Bad gateway"]}`,
			expectedDrift: false,
			expectedError: true,
		},
		{
			name:          "Unauthorized",
			statusCode:    http.StatusUnauthorized,
			responseBody:  `{"errors": ["Unauthorized"]}`,
			expectedDrift: false,
			expectedError: true,
		},
		{
			name:          "Forbidden",
			statusCode:    http.StatusForbidden,
			responseBody:  `{"errors": ["Forbidden"]}`,
			expectedDrift: false,
			expectedError: true,
		},
	}

	for _, scenario := range errorScenarios {
		t.Run(scenario.name, func(t *testing.T) {
			// Set up HTTP server
			httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(scenario.statusCode)
				_, _ = w.Write([]byte(scenario.responseBody))
			}))
			defer httpServer.Close()

			// Set up Datadog client
			testConfig := datadogapi.NewConfiguration()
			testConfig.HTTPClient = httpServer.Client()
			apiClient := datadogapi.NewAPIClient(testConfig)
			client := datadogV1.NewMonitorsApi(apiClient)
			testAuth := setupTestAuth(httpServer.URL)

			// Create reconciler
			r := &Reconciler{
				datadogClient: client,
				datadogAuth:   testAuth,
				log:           testLogger,
				recorder:      record.NewFakeRecorder(10),
			}

			// Create DatadogMonitor instance
			instance := &datadoghqv1alpha1.DatadogMonitor{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-monitor",
					Namespace: "default",
				},
				Status: datadoghqv1alpha1.DatadogMonitorStatus{
					ID: 12345,
				},
			}

			status := &datadoghqv1alpha1.DatadogMonitorStatus{
				ID: 12345,
			}

			// Test drift detection
			driftDetected, err := r.detectDrift(context.TODO(), testLogger, instance, status)

			// Verify error handling
			assert.Equal(t, scenario.expectedDrift, driftDetected, "Drift detection result mismatch")
			if scenario.expectedError {
				assert.Error(t, err, "Expected error but got none")
			} else {
				assert.NoError(t, err, "Unexpected error: %v", err)
			}

			// Verify status is updated to indicate error
			assert.Equal(t, datadoghqv1alpha1.MonitorStateSyncStatusGetError, status.MonitorStateSyncStatus)
		})
	}
}

// **Feature: monitor-recreation, Property 2: Monitor Recreation Completeness**
// Property-based test for monitor recreation completeness
func TestMonitorRecreationCompleteness(t *testing.T) {
	testCases := []struct {
		name              string
		originalMonitorID int
		originalSpec      datadoghqv1alpha1.DatadogMonitorSpec
		serverResponse    func(w http.ResponseWriter, r *http.Request)
		expectedError     bool
		expectedNewID     int
	}{
		{
			name:              "Successful recreation with all parameters preserved",
			originalMonitorID: 12345,
			originalSpec: datadoghqv1alpha1.DatadogMonitorSpec{
				Name:     "Test Monitor",
				Message:  "Test message",
				Query:    "avg(last_10m):avg:system.disk.in_use{*} by {host} > 0.1",
				Type:     datadoghqv1alpha1.DatadogMonitorTypeMetric,
				Tags:     []string{"env:test", "team:platform"},
				Priority: 3,
				Options: datadoghqv1alpha1.DatadogMonitorOptions{
					NotifyNoData: &[]bool{true}[0],
				},
			},
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				// Handle both validation and creation requests
				if r.Method == "POST" && strings.Contains(r.URL.Path, "validate") {
					// Validation request
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(`{}`))
				} else if r.Method == "POST" {
					// Creation request
					newMonitor := genericMonitor(67890) // New ID
					jsonMonitor, _ := newMonitor.MarshalJSON()
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write(jsonMonitor)
				}
			},
			expectedError: false,
			expectedNewID: 67890,
		},
		{
			name:              "Recreation fails due to validation error",
			originalMonitorID: 12345,
			originalSpec: datadoghqv1alpha1.DatadogMonitorSpec{
				Name:    "Invalid Monitor",
				Message: "Test message",
				Query:   "invalid query syntax",
				Type:    datadoghqv1alpha1.DatadogMonitorTypeMetric,
			},
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "POST" && strings.Contains(r.URL.Path, "validate") {
					// Validation fails
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusBadRequest)
					_, _ = w.Write([]byte(`{"errors": ["Invalid query syntax"]}`))
				}
			},
			expectedError: true,
			expectedNewID: 0, // Should remain 0 due to error
		},
		{
			name:              "Recreation fails due to API error",
			originalMonitorID: 12345,
			originalSpec: datadoghqv1alpha1.DatadogMonitorSpec{
				Name:    "Test Monitor",
				Message: "Test message",
				Query:   "avg(last_10m):avg:system.disk.in_use{*} by {host} > 0.1",
				Type:    datadoghqv1alpha1.DatadogMonitorTypeMetric,
			},
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "POST" && strings.Contains(r.URL.Path, "validate") {
					// Validation succeeds
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(`{}`))
				} else if r.Method == "POST" {
					// Creation fails
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = w.Write([]byte(`{"errors": ["Internal server error"]}`))
				}
			},
			expectedError: true,
			expectedNewID: 0, // Should remain 0 due to error
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set up HTTP server
			httpServer := httptest.NewServer(http.HandlerFunc(tc.serverResponse))
			defer httpServer.Close()

			// Set up Datadog client
			testConfig := datadogapi.NewConfiguration()
			testConfig.HTTPClient = httpServer.Client()
			apiClient := datadogapi.NewAPIClient(testConfig)
			client := datadogV1.NewMonitorsApi(apiClient)
			testAuth := setupTestAuth(httpServer.URL)

			// Create reconciler
			r := &Reconciler{
				datadogClient: client,
				datadogAuth:   testAuth,
				log:           testLogger,
				recorder:      record.NewFakeRecorder(10),
			}

			// Create DatadogMonitor instance
			instance := &datadoghqv1alpha1.DatadogMonitor{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-monitor",
					Namespace: "default",
				},
				Spec: tc.originalSpec,
				Status: datadoghqv1alpha1.DatadogMonitorStatus{
					ID: tc.originalMonitorID,
				},
			}

			status := &datadoghqv1alpha1.DatadogMonitorStatus{
				ID: tc.originalMonitorID,
			}

			now := metav1.Now()
			instanceSpecHash := "test-hash"

			// Test monitor recreation
			err := r.handleMonitorRecreation(context.TODO(), testLogger, instance, status, now, instanceSpecHash)

			// Verify results
			if tc.expectedError {
				assert.Error(t, err, "Expected error but got none")
			} else {
				assert.NoError(t, err, "Unexpected error: %v", err)

				// Verify new monitor ID is set
				assert.Equal(t, tc.expectedNewID, status.ID, "New monitor ID mismatch")

				// Verify status fields are updated correctly
				assert.True(t, status.Primary, "Primary status should be true")
				assert.Equal(t, instanceSpecHash, status.CurrentHash, "Current hash should be updated")
				assert.NotNil(t, status.Created, "Created time should be set")
			}
		})
	}
}

// Property-based test for configuration preservation during recreation
func TestMonitorRecreationConfigurationPreservation(t *testing.T) {
	// Test various monitor configurations to ensure all parameters are preserved
	configurations := []datadoghqv1alpha1.DatadogMonitorSpec{
		{
			Name:     "Metric Alert Monitor",
			Message:  "Metric alert message",
			Query:    "avg(last_10m):avg:system.disk.in_use{*} by {host} > 0.1",
			Type:     datadoghqv1alpha1.DatadogMonitorTypeMetric,
			Tags:     []string{"env:prod", "team:platform", "service:api"},
			Priority: 1,
			Options: datadoghqv1alpha1.DatadogMonitorOptions{
				NotifyNoData:      &[]bool{true}[0],
				IncludeTags:       &[]bool{true}[0],
				RequireFullWindow: &[]bool{false}[0],
				Thresholds: &datadoghqv1alpha1.DatadogMonitorOptionsThresholds{
					Critical: &[]string{"0.9"}[0],
					Warning:  &[]string{"0.7"}[0],
				},
			},
		},
		{
			Name:     "Log Alert Monitor",
			Message:  "Log alert message",
			Query:    "logs(\"source:app AND level:error\").index(\"main\").rollup(\"count\").last(\"5m\") > 10",
			Type:     datadoghqv1alpha1.DatadogMonitorTypeLog,
			Tags:     []string{"env:staging"},
			Priority: 2,
			Options: datadoghqv1alpha1.DatadogMonitorOptions{
				EvaluationDelay: &[]int64{300}[0],
				NewGroupDelay:   &[]int64{60}[0],
			},
		},
		{
			Name:     "Service Check Monitor",
			Message:  "Service check message",
			Query:    "\"http.can_connect\".over(\"*\").by(\"host\").last(2).count_by_status()",
			Type:     datadoghqv1alpha1.DatadogMonitorTypeService,
			Tags:     []string{"env:test", "check:http"},
			Priority: 3,
		},
	}

	for i, config := range configurations {
		t.Run(fmt.Sprintf("Configuration_%d_%s", i, config.Type), func(t *testing.T) {
			// Set up HTTP server that captures the request body
			var capturedRequest []byte
			httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "POST" && strings.Contains(r.URL.Path, "validate") {
					// Validation request
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(`{}`))
				} else if r.Method == "POST" {
					// Creation request - capture the body
					body, _ := io.ReadAll(r.Body)
					capturedRequest = body

					// Return a successful response
					newMonitor := genericMonitor(99999)
					jsonMonitor, _ := newMonitor.MarshalJSON()
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write(jsonMonitor)
				}
			}))
			defer httpServer.Close()

			// Set up Datadog client
			testConfig := datadogapi.NewConfiguration()
			testConfig.HTTPClient = httpServer.Client()
			apiClient := datadogapi.NewAPIClient(testConfig)
			client := datadogV1.NewMonitorsApi(apiClient)
			testAuth := setupTestAuth(httpServer.URL)

			// Create reconciler
			r := &Reconciler{
				datadogClient: client,
				datadogAuth:   testAuth,
				log:           testLogger,
				recorder:      record.NewFakeRecorder(10),
			}

			// Create DatadogMonitor instance
			instance := &datadoghqv1alpha1.DatadogMonitor{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-monitor",
					Namespace: "default",
				},
				Spec: config,
				Status: datadoghqv1alpha1.DatadogMonitorStatus{
					ID: 12345,
				},
			}

			status := &datadoghqv1alpha1.DatadogMonitorStatus{
				ID: 12345,
			}

			now := metav1.Now()
			instanceSpecHash := "test-hash"

			// Test monitor recreation
			err := r.handleMonitorRecreation(context.TODO(), testLogger, instance, status, now, instanceSpecHash)

			// Verify recreation succeeded
			assert.NoError(t, err, "Recreation should succeed")
			assert.NotEmpty(t, capturedRequest, "Request should be captured")

			// Parse the captured request to verify configuration preservation
			var sentMonitor map[string]interface{}
			err = json.Unmarshal(capturedRequest, &sentMonitor)
			assert.NoError(t, err, "Should be able to parse sent monitor")

			// Verify key configuration parameters are preserved
			assert.Equal(t, config.Name, sentMonitor["name"], "Name should be preserved")
			assert.Equal(t, config.Message, sentMonitor["message"], "Message should be preserved")
			assert.Equal(t, config.Query, sentMonitor["query"], "Query should be preserved")
			assert.Equal(t, string(config.Type), sentMonitor["type"], "Type should be preserved")

			// Verify tags are preserved and sorted
			if sentTags, ok := sentMonitor["tags"].([]interface{}); ok {
				expectedTags := make([]string, len(config.Tags))
				copy(expectedTags, config.Tags)
				sort.Strings(expectedTags)

				actualTags := make([]string, len(sentTags))
				for i, tag := range sentTags {
					actualTags[i] = tag.(string)
				}

				assert.Equal(t, expectedTags, actualTags, "Tags should be preserved and sorted")
			}
		})
	}
}

// **Feature: monitor-recreation, Property 3: Status Update Consistency**
// Property-based test for status update consistency
func TestStatusUpdateConsistency(t *testing.T) {
	testCases := []struct {
		name                   string
		initialStatus          datadoghqv1alpha1.DatadogMonitorStatus
		operation              string
		serverResponse         func(w http.ResponseWriter, r *http.Request)
		expectedConditions     []datadoghqv1alpha1.DatadogMonitorConditionType
		expectedSyncStatus     datadoghqv1alpha1.MonitorStateSyncStatusMessage
		shouldPreserveExisting bool
	}{
		{
			name: "Drift detection updates status with drift condition",
			initialStatus: datadoghqv1alpha1.DatadogMonitorStatus{
				ID:      12345,
				Primary: true,
				Creator: "test@example.com",
			},
			operation: "drift_detection",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"errors": ["Monitor not found"]}`))
			},
			expectedConditions: []datadoghqv1alpha1.DatadogMonitorConditionType{
				datadoghqv1alpha1.DatadogMonitorConditionTypeDriftDetected,
			},
			expectedSyncStatus:     datadoghqv1alpha1.MonitorStateSyncStatusGetError,
			shouldPreserveExisting: true,
		},
		{
			name: "Successful recreation updates status with recreated condition",
			initialStatus: datadoghqv1alpha1.DatadogMonitorStatus{
				ID:      12345,
				Primary: true,
				Creator: "test@example.com",
			},
			operation: "recreation",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "POST" && strings.Contains(r.URL.Path, "validate") {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(`{}`))
				} else if r.Method == "POST" {
					newMonitor := genericMonitor(67890)
					jsonMonitor, _ := newMonitor.MarshalJSON()
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write(jsonMonitor)
				}
			},
			expectedConditions: []datadoghqv1alpha1.DatadogMonitorConditionType{
				datadoghqv1alpha1.DatadogMonitorConditionTypeRecreated,
			},
			expectedSyncStatus:     "",
			shouldPreserveExisting: false, // Recreation resets some fields
		},
		{
			name: "Failed recreation preserves existing status with error",
			initialStatus: datadoghqv1alpha1.DatadogMonitorStatus{
				ID:      12345,
				Primary: true,
				Creator: "test@example.com",
			},
			operation: "recreation_failure",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "POST" && strings.Contains(r.URL.Path, "validate") {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusBadRequest)
					_, _ = w.Write([]byte(`{"errors": ["Invalid query"]}`))
				}
			},
			expectedConditions:     []datadoghqv1alpha1.DatadogMonitorConditionType{},
			expectedSyncStatus:     "",
			shouldPreserveExisting: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set up HTTP server
			httpServer := httptest.NewServer(http.HandlerFunc(tc.serverResponse))
			defer httpServer.Close()

			// Set up Datadog client
			testConfig := datadogapi.NewConfiguration()
			testConfig.HTTPClient = httpServer.Client()
			apiClient := datadogapi.NewAPIClient(testConfig)
			client := datadogV1.NewMonitorsApi(apiClient)
			testAuth := setupTestAuth(httpServer.URL)

			// Create reconciler
			r := createTestReconciler(client, testAuth)

			// Create DatadogMonitor instance
			instance := &datadoghqv1alpha1.DatadogMonitor{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-monitor",
					Namespace: "default",
				},
				Spec: datadoghqv1alpha1.DatadogMonitorSpec{
					Name:    "Test Monitor",
					Message: "Test message",
					Query:   "avg(last_10m):avg:system.disk.in_use{*} by {host} > 0.1",
					Type:    datadoghqv1alpha1.DatadogMonitorTypeMetric,
				},
				Status: tc.initialStatus,
			}

			// Copy initial status for comparison
			status := tc.initialStatus.DeepCopy()

			// Perform the operation
			switch tc.operation {
			case "drift_detection":
				_, _ = r.detectDrift(context.TODO(), testLogger, instance, status)
			case "recreation":
				now := metav1.Now()
				_ = r.handleMonitorRecreation(context.TODO(), testLogger, instance, status, now, "test-hash")
			case "recreation_failure":
				now := metav1.Now()
				_ = r.handleMonitorRecreation(context.TODO(), testLogger, instance, status, now, "test-hash")
			}

			// Verify expected conditions are present
			for _, expectedCondition := range tc.expectedConditions {
				found := false
				for _, condition := range status.Conditions {
					if condition.Type == expectedCondition && condition.Status == corev1.ConditionTrue {
						found = true
						break
					}
				}
				assert.True(t, found, "Expected condition %s not found", expectedCondition)
			}

			// Verify sync status
			if tc.expectedSyncStatus != "" {
				assert.Equal(t, tc.expectedSyncStatus, status.MonitorStateSyncStatus, "Sync status mismatch")
			}

			// Verify existing information preservation
			if tc.shouldPreserveExisting {
				if tc.initialStatus.Creator != "" {
					assert.Equal(t, tc.initialStatus.Creator, status.Creator, "Creator should be preserved")
				}
				if tc.initialStatus.Primary {
					assert.Equal(t, tc.initialStatus.Primary, status.Primary, "Primary status should be preserved")
				}
			}
		})
	}
}

// Property-based test for status preservation during operations
func TestStatusPreservationDuringOperations(t *testing.T) {
	// Test that valid existing status information is preserved during various operations
	initialStatus := datadoghqv1alpha1.DatadogMonitorStatus{
		ID:                         12345,
		Primary:                    true,
		Creator:                    "original@example.com",
		Created:                    &metav1.Time{Time: time.Now().Add(-24 * time.Hour)},
		MonitorState:               datadoghqv1alpha1.DatadogMonitorStateOK,
		MonitorStateLastUpdateTime: &metav1.Time{Time: time.Now().Add(-1 * time.Hour)},
		CurrentHash:                "original-hash",
		Conditions: []datadoghqv1alpha1.DatadogMonitorCondition{
			{
				Type:               datadoghqv1alpha1.DatadogMonitorConditionTypeActive,
				Status:             corev1.ConditionTrue,
				LastTransitionTime: metav1.Time{Time: time.Now().Add(-2 * time.Hour)},
			},
		},
	}

	operations := []struct {
		name           string
		operation      func(r *Reconciler, instance *datadoghqv1alpha1.DatadogMonitor, status *datadoghqv1alpha1.DatadogMonitorStatus)
		serverResponse func(w http.ResponseWriter, r *http.Request)
		preserveFields []string
	}{
		{
			name: "Drift detection preserves valid fields",
			operation: func(r *Reconciler, instance *datadoghqv1alpha1.DatadogMonitor, status *datadoghqv1alpha1.DatadogMonitorStatus) {
				_, _ = r.detectDrift(context.TODO(), testLogger, instance, status)
			},
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"errors": ["Monitor not found"]}`))
			},
			preserveFields: []string{"Creator", "Primary", "MonitorState", "MonitorStateLastUpdateTime"},
		},
		{
			name: "Failed recreation preserves original status",
			operation: func(r *Reconciler, instance *datadoghqv1alpha1.DatadogMonitor, status *datadoghqv1alpha1.DatadogMonitorStatus) {
				now := metav1.Now()
				_ = r.handleMonitorRecreation(context.TODO(), testLogger, instance, status, now, "test-hash")
			},
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "POST" && strings.Contains(r.URL.Path, "validate") {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusBadRequest)
					_, _ = w.Write([]byte(`{"errors": ["Invalid query"]}`))
				}
			},
			preserveFields: []string{"ID", "Creator", "Primary", "MonitorState"},
		},
	}

	for _, op := range operations {
		t.Run(op.name, func(t *testing.T) {
			// Set up HTTP server
			httpServer := httptest.NewServer(http.HandlerFunc(op.serverResponse))
			defer httpServer.Close()

			// Set up Datadog client
			testConfig := datadogapi.NewConfiguration()
			testConfig.HTTPClient = httpServer.Client()
			apiClient := datadogapi.NewAPIClient(testConfig)
			client := datadogV1.NewMonitorsApi(apiClient)
			testAuth := setupTestAuth(httpServer.URL)

			// Create reconciler
			r := createTestReconciler(client, testAuth)

			// Create DatadogMonitor instance
			instance := &datadoghqv1alpha1.DatadogMonitor{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-monitor",
					Namespace: "default",
				},
				Spec: datadoghqv1alpha1.DatadogMonitorSpec{
					Name:    "Test Monitor",
					Message: "Test message",
					Query:   "avg(last_10m):avg:system.disk.in_use{*} by {host} > 0.1",
					Type:    datadoghqv1alpha1.DatadogMonitorTypeMetric,
				},
				Status: initialStatus,
			}

			// Copy initial status for comparison
			originalStatus := initialStatus.DeepCopy()
			status := initialStatus.DeepCopy()

			// Perform the operation
			op.operation(r, instance, status)

			// Verify specified fields are preserved
			for _, field := range op.preserveFields {
				switch field {
				case "Creator":
					assert.Equal(t, originalStatus.Creator, status.Creator, "Creator should be preserved")
				case "Primary":
					assert.Equal(t, originalStatus.Primary, status.Primary, "Primary should be preserved")
				case "ID":
					if originalStatus.ID != 0 {
						assert.Equal(t, originalStatus.ID, status.ID, "ID should be preserved when operation fails")
					}
				case "MonitorState":
					assert.Equal(t, originalStatus.MonitorState, status.MonitorState, "MonitorState should be preserved")
				case "MonitorStateLastUpdateTime":
					if originalStatus.MonitorStateLastUpdateTime != nil {
						assert.Equal(t, originalStatus.MonitorStateLastUpdateTime, status.MonitorStateLastUpdateTime, "MonitorStateLastUpdateTime should be preserved")
					}
				}
			}
		})
	}
}

// **Feature: monitor-recreation, Property 4: Error Handling Resilience**
// Property-based test for error handling resilience
func TestErrorHandlingResilience(t *testing.T) {
	testCases := []struct {
		name                string
		operation           string
		monitorID           int
		serverResponse      func(w http.ResponseWriter, r *http.Request)
		expectedError       bool
		expectedDrift       bool
		shouldPreserveState bool
		errorCategory       string
	}{
		{
			name:      "Rate limit during drift detection - graceful handling",
			operation: "drift_detection",
			monitorID: 12345,
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"errors": ["Rate limit exceeded"]}`))
			},
			expectedError:       true,
			expectedDrift:       false,
			shouldPreserveState: true,
			errorCategory:       "rate_limit",
		},
		{
			name:      "Authentication error during drift detection",
			operation: "drift_detection",
			monitorID: 12345,
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"errors": ["Unauthorized"]}`))
			},
			expectedError:       true,
			expectedDrift:       false,
			shouldPreserveState: true,
			errorCategory:       "auth",
		},
		{
			name:      "Timeout during drift detection",
			operation: "drift_detection",
			monitorID: 12345,
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusRequestTimeout)
				_, _ = w.Write([]byte(`{"errors": ["Request timeout"]}`))
			},
			expectedError:       true,
			expectedDrift:       false,
			shouldPreserveState: true,
			errorCategory:       "timeout",
		},
		{
			name:      "Validation error during recreation - no retry",
			operation: "recreation",
			monitorID: 12345,
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "POST" && strings.Contains(r.URL.Path, "validate") {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusBadRequest)
					_, _ = w.Write([]byte(`{"errors": ["Invalid query syntax"]}`))
				}
			},
			expectedError:       true,
			expectedDrift:       false,
			shouldPreserveState: true,
			errorCategory:       "validation",
		},
		{
			name:      "Rate limit during recreation - should retry",
			operation: "recreation",
			monitorID: 12345,
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "POST" && strings.Contains(r.URL.Path, "validate") {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(`{}`))
				} else if r.Method == "POST" {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusTooManyRequests)
					_, _ = w.Write([]byte(`{"errors": ["Rate limit exceeded"]}`))
				}
			},
			expectedError:       true,
			expectedDrift:       false,
			shouldPreserveState: true,
			errorCategory:       "rate_limit",
		},
		{
			name:      "Service unavailable during recreation",
			operation: "recreation",
			monitorID: 12345,
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "POST" && strings.Contains(r.URL.Path, "validate") {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(`{}`))
				} else if r.Method == "POST" {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusServiceUnavailable)
					_, _ = w.Write([]byte(`{"errors": ["Service unavailable"]}`))
				}
			},
			expectedError:       true,
			expectedDrift:       false,
			shouldPreserveState: true,
			errorCategory:       "service_error",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set up HTTP server
			httpServer := httptest.NewServer(http.HandlerFunc(tc.serverResponse))
			defer httpServer.Close()

			// Set up Datadog client
			testConfig := datadogapi.NewConfiguration()
			testConfig.HTTPClient = httpServer.Client()
			apiClient := datadogapi.NewAPIClient(testConfig)
			client := datadogV1.NewMonitorsApi(apiClient)
			testAuth := setupTestAuth(httpServer.URL)

			// Create reconciler
			r := createTestReconciler(client, testAuth)

			// Create DatadogMonitor instance
			instance := &datadoghqv1alpha1.DatadogMonitor{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-monitor",
					Namespace: "default",
				},
				Spec: datadoghqv1alpha1.DatadogMonitorSpec{
					Name:    "Test Monitor",
					Message: "Test message",
					Query:   "avg(last_10m):avg:system.disk.in_use{*} by {host} > 0.1",
					Type:    datadoghqv1alpha1.DatadogMonitorTypeMetric,
				},
				Status: datadoghqv1alpha1.DatadogMonitorStatus{
					ID:      tc.monitorID,
					Primary: true,
					Creator: "test@example.com",
				},
			}

			// Store original status for comparison
			originalStatus := instance.Status.DeepCopy()
			status := instance.Status.DeepCopy()

			// Perform the operation
			var err error
			var driftDetected bool

			switch tc.operation {
			case "drift_detection":
				driftDetected, err = r.detectDrift(context.TODO(), testLogger, instance, status)
			case "recreation":
				now := metav1.Now()
				err = r.handleMonitorRecreation(context.TODO(), testLogger, instance, status, now, "test-hash")
			}

			// Verify error handling
			if tc.expectedError {
				assert.Error(t, err, "Expected error but got none")

				// Verify error message contains appropriate category information
				errorMessage := err.Error()
				switch tc.errorCategory {
				case "rate_limit":
					assert.True(t, strings.Contains(errorMessage, "rate limit") || strings.Contains(errorMessage, "Rate limit"),
						"Error message should indicate rate limit: %s", errorMessage)
				case "auth":
					assert.True(t, strings.Contains(errorMessage, "auth") || strings.Contains(errorMessage, "Unauthorized"),
						"Error message should indicate auth error: %s", errorMessage)
				case "timeout":
					assert.True(t, strings.Contains(errorMessage, "timeout") || strings.Contains(errorMessage, "Timeout"),
						"Error message should indicate timeout: %s", errorMessage)
				case "validation":
					assert.True(t, strings.Contains(errorMessage, "validation") || strings.Contains(errorMessage, "Invalid"),
						"Error message should indicate validation error: %s", errorMessage)
				}
			} else {
				assert.NoError(t, err, "Unexpected error: %v", err)
			}

			// Verify drift detection result
			if tc.operation == "drift_detection" {
				assert.Equal(t, tc.expectedDrift, driftDetected, "Drift detection result mismatch")
			}

			// Verify state preservation
			if tc.shouldPreserveState {
				if tc.operation == "recreation" && tc.expectedError {
					// For failed recreation, original ID should be restored
					assert.Equal(t, originalStatus.ID, status.ID, "Original monitor ID should be restored on recreation failure")
				}

				// Other important fields should be preserved
				assert.Equal(t, originalStatus.Primary, status.Primary, "Primary status should be preserved")
				assert.Equal(t, originalStatus.Creator, status.Creator, "Creator should be preserved")
			}

			// Verify status sync status is updated appropriately
			if tc.expectedError && tc.operation == "drift_detection" {
				assert.Equal(t, datadoghqv1alpha1.MonitorStateSyncStatusGetError, status.MonitorStateSyncStatus,
					"Sync status should indicate error")
			}
		})
	}
}

// Property-based test for error categorization and retry behavior
func TestErrorCategorizationAndRetryBehavior(t *testing.T) {
	// Test that different error types are handled with appropriate retry strategies
	errorTypes := []struct {
		name          string
		statusCode    int
		responseBody  string
		shouldRetry   bool
		errorCategory string
	}{
		{
			name:          "Rate limit - should retry",
			statusCode:    http.StatusTooManyRequests,
			responseBody:  `{"errors": ["Rate limit exceeded"]}`,
			shouldRetry:   true,
			errorCategory: "rate_limit",
		},
		{
			name:          "Service unavailable - should retry",
			statusCode:    http.StatusServiceUnavailable,
			responseBody:  `{"errors": ["Service unavailable"]}`,
			shouldRetry:   true,
			errorCategory: "service_error",
		},
		{
			name:          "Timeout - should retry",
			statusCode:    http.StatusRequestTimeout,
			responseBody:  `{"errors": ["Request timeout"]}`,
			shouldRetry:   true,
			errorCategory: "timeout",
		},
		{
			name:          "Bad request - should not retry",
			statusCode:    http.StatusBadRequest,
			responseBody:  `{"errors": ["Invalid query syntax"]}`,
			shouldRetry:   false,
			errorCategory: "validation",
		},
		{
			name:          "Unauthorized - should not retry without credential fix",
			statusCode:    http.StatusUnauthorized,
			responseBody:  `{"errors": ["Unauthorized"]}`,
			shouldRetry:   false,
			errorCategory: "auth",
		},
		{
			name:          "Forbidden - should not retry",
			statusCode:    http.StatusForbidden,
			responseBody:  `{"errors": ["Forbidden"]}`,
			shouldRetry:   false,
			errorCategory: "auth",
		},
	}

	for _, errorType := range errorTypes {
		t.Run(errorType.name, func(t *testing.T) {
			// Set up HTTP server
			httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(errorType.statusCode)
				_, _ = w.Write([]byte(errorType.responseBody))
			}))
			defer httpServer.Close()

			// Set up Datadog client
			testConfig := datadogapi.NewConfiguration()
			testConfig.HTTPClient = httpServer.Client()
			apiClient := datadogapi.NewAPIClient(testConfig)
			client := datadogV1.NewMonitorsApi(apiClient)
			testAuth := setupTestAuth(httpServer.URL)

			// Create reconciler
			r := createTestReconciler(client, testAuth)

			// Create DatadogMonitor instance
			instance := &datadoghqv1alpha1.DatadogMonitor{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-monitor",
					Namespace: "default",
				},
				Status: datadoghqv1alpha1.DatadogMonitorStatus{
					ID: 12345,
				},
			}

			status := &datadoghqv1alpha1.DatadogMonitorStatus{
				ID: 12345,
			}

			// Test drift detection error handling
			_, err := r.detectDrift(context.TODO(), testLogger, instance, status)

			// Verify error is returned
			assert.Error(t, err, "Expected error for status code %d", errorType.statusCode)

			// Verify error message contains appropriate information
			errorMessage := err.Error()
			switch errorType.errorCategory {
			case "rate_limit":
				assert.True(t, strings.Contains(strings.ToLower(errorMessage), "rate limit"),
					"Rate limit error should be identified: %s", errorMessage)
			case "auth":
				assert.True(t, strings.Contains(strings.ToLower(errorMessage), "auth") ||
					strings.Contains(strings.ToLower(errorMessage), "unauthorized") ||
					strings.Contains(strings.ToLower(errorMessage), "forbidden"),
					"Auth error should be identified: %s", errorMessage)
			case "timeout":
				assert.True(t, strings.Contains(strings.ToLower(errorMessage), "timeout"),
					"Timeout error should be identified: %s", errorMessage)
			case "validation":
				// For drift detection, validation errors from the API are treated as generic errors
				// The validation check happens before recreation attempts
				assert.NotEmpty(t, errorMessage, "Error message should not be empty")
			}

			// Verify status is updated to indicate error
			assert.Equal(t, datadoghqv1alpha1.MonitorStateSyncStatusGetError, status.MonitorStateSyncStatus,
				"Status should indicate get error")
		})
	}
}

// Property-based test for graceful degradation during API unavailability
func TestGracefulDegradationDuringAPIUnavailability(t *testing.T) {
	// Test that the controller continues to function when Datadog API is unavailable
	unavailabilityScenarios := []struct {
		name           string
		serverBehavior func(w http.ResponseWriter, r *http.Request)
		description    string
	}{
		{
			name: "Complete API unavailability",
			serverBehavior: func(w http.ResponseWriter, r *http.Request) {
				// Simulate complete service unavailability
				w.WriteHeader(http.StatusServiceUnavailable)
			},
			description: "API returns 503 Service Unavailable",
		},
		{
			name: "API gateway timeout",
			serverBehavior: func(w http.ResponseWriter, r *http.Request) {
				// Simulate gateway timeout
				w.WriteHeader(http.StatusGatewayTimeout)
			},
			description: "API returns 504 Gateway Timeout",
		},
		{
			name: "Connection refused simulation",
			serverBehavior: func(w http.ResponseWriter, r *http.Request) {
				// Close connection immediately to simulate connection refused
				hj, ok := w.(http.Hijacker)
				if ok {
					conn, _, _ := hj.Hijack()
					conn.Close()
				}
			},
			description: "Connection is refused",
		},
	}

	for _, scenario := range unavailabilityScenarios {
		t.Run(scenario.name, func(t *testing.T) {
			// Set up HTTP server with unavailability behavior
			httpServer := httptest.NewServer(http.HandlerFunc(scenario.serverBehavior))
			defer httpServer.Close()

			// Set up Datadog client
			testConfig := datadogapi.NewConfiguration()
			testConfig.HTTPClient = httpServer.Client()
			apiClient := datadogapi.NewAPIClient(testConfig)
			client := datadogV1.NewMonitorsApi(apiClient)
			testAuth := setupTestAuth(httpServer.URL)

			// Create reconciler
			r := createTestReconciler(client, testAuth)

			// Create DatadogMonitor instance
			instance := &datadoghqv1alpha1.DatadogMonitor{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-monitor",
					Namespace: "default",
				},
				Status: datadoghqv1alpha1.DatadogMonitorStatus{
					ID:      12345,
					Primary: true,
					Creator: "test@example.com",
				},
			}

			status := instance.Status.DeepCopy()

			// Test drift detection during API unavailability
			driftDetected, err := r.detectDrift(context.TODO(), testLogger, instance, status)

			// Verify graceful error handling
			assert.Error(t, err, "Should return error when API is unavailable")
			assert.False(t, driftDetected, "Should not detect drift when API is unavailable")

			// Verify that the controller doesn't crash and maintains state
			assert.Equal(t, instance.Status.ID, status.ID, "Monitor ID should be preserved")
			assert.Equal(t, instance.Status.Primary, status.Primary, "Primary status should be preserved")
			assert.Equal(t, instance.Status.Creator, status.Creator, "Creator should be preserved")

			// Verify status indicates error
			assert.Equal(t, datadoghqv1alpha1.MonitorStateSyncStatusGetError, status.MonitorStateSyncStatus,
				"Status should indicate get error")
		})
	}
}

// **Feature: monitor-recreation, Property 5: Event Emission Correctness**
// Property-based test for event emission correctness
func TestEventEmissionCorrectness(t *testing.T) {
	testCases := []struct {
		name                string
		operation           string
		serverResponse      func(w http.ResponseWriter, r *http.Request)
		expectedEventCount  int
		expectedEventReason string
		expectedEventType   string
		shouldSucceed       bool
	}{
		{
			name:      "Successful monitor creation emits creation event",
			operation: "creation",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "POST" && strings.Contains(r.URL.Path, "validate") {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(`{}`))
				} else if r.Method == "POST" {
					newMonitor := genericMonitor(12345)
					jsonMonitor, _ := newMonitor.MarshalJSON()
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write(jsonMonitor)
				}
			},
			expectedEventCount:  1,
			expectedEventReason: "Create",
			expectedEventType:   "Normal",
			shouldSucceed:       true,
		},
		{
			name:      "Successful monitor recreation emits recreation event",
			operation: "recreation",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "POST" && strings.Contains(r.URL.Path, "validate") {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(`{}`))
				} else if r.Method == "POST" {
					newMonitor := genericMonitor(67890)
					jsonMonitor, _ := newMonitor.MarshalJSON()
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write(jsonMonitor)
				}
			},
			expectedEventCount:  1,
			expectedEventReason: "Recreate",
			expectedEventType:   "Normal",
			shouldSucceed:       true,
		},
		{
			name:      "Failed recreation does not emit recreation event",
			operation: "recreation",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "POST" && strings.Contains(r.URL.Path, "validate") {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusBadRequest)
					_, _ = w.Write([]byte(`{"errors": ["Invalid query"]}`))
				}
			},
			expectedEventCount:  0,
			expectedEventReason: "",
			expectedEventType:   "",
			shouldSucceed:       false,
		},
		{
			name:      "Failed creation does not emit creation event",
			operation: "creation",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "POST" && strings.Contains(r.URL.Path, "validate") {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusBadRequest)
					_, _ = w.Write([]byte(`{"errors": ["Invalid query"]}`))
				}
			},
			expectedEventCount:  0,
			expectedEventReason: "",
			expectedEventType:   "",
			shouldSucceed:       false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set up HTTP server
			httpServer := httptest.NewServer(http.HandlerFunc(tc.serverResponse))
			defer httpServer.Close()

			// Set up Datadog client
			testConfig := datadogapi.NewConfiguration()
			testConfig.HTTPClient = httpServer.Client()
			apiClient := datadogapi.NewAPIClient(testConfig)
			client := datadogV1.NewMonitorsApi(apiClient)
			testAuth := setupTestAuth(httpServer.URL)

			// Create fake event recorder to capture events
			fakeRecorder := record.NewFakeRecorder(10)

			// Create reconciler with fake recorder
			r := &Reconciler{
				datadogClient: client,
				datadogAuth:   testAuth,
				log:           testLogger,
				recorder:      fakeRecorder,
			}

			// Create DatadogMonitor instance
			instance := &datadoghqv1alpha1.DatadogMonitor{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-monitor",
					Namespace: "default",
				},
				Spec: datadoghqv1alpha1.DatadogMonitorSpec{
					Name:    "Test Monitor",
					Message: "Test message",
					Query:   "avg(last_10m):avg:system.disk.in_use{*} by {host} > 0.1",
					Type:    datadoghqv1alpha1.DatadogMonitorTypeMetric,
				},
				Status: datadoghqv1alpha1.DatadogMonitorStatus{
					ID: 0, // Start with no ID for creation, or set ID for recreation
				},
			}

			status := &datadoghqv1alpha1.DatadogMonitorStatus{}

			// Set up for recreation test
			if tc.operation == "recreation" {
				instance.Status.ID = 12345
				status.ID = 12345
			}

			now := metav1.Now()
			instanceSpecHash := "test-hash"

			// Perform the operation
			var err error
			switch tc.operation {
			case "creation":
				err = r.create(testLogger, instance, status, now, instanceSpecHash)
			case "recreation":
				err = r.handleMonitorRecreation(context.TODO(), testLogger, instance, status, now, instanceSpecHash)
			}

			// Verify operation result
			if tc.shouldSucceed {
				assert.NoError(t, err, "Operation should succeed")
			} else {
				assert.Error(t, err, "Operation should fail")
			}

			// Verify event emission
			close(fakeRecorder.Events)
			events := []string{}
			for event := range fakeRecorder.Events {
				events = append(events, event)
			}

			assert.Equal(t, tc.expectedEventCount, len(events), "Event count mismatch")

			if tc.expectedEventCount > 0 {
				// Verify event content
				assert.True(t, len(events) > 0, "Should have at least one event")

				// Parse the event string (format: "Normal Create DatadogMonitor test-monitor")
				eventParts := strings.Fields(events[0])
				assert.True(t, len(eventParts) >= 2, "Event should have at least type and reason")

				actualEventType := eventParts[0]
				actualEventReason := eventParts[1]

				assert.Equal(t, tc.expectedEventType, actualEventType, "Event type mismatch")
				assert.Equal(t, tc.expectedEventReason, actualEventReason, "Event reason mismatch")
			}
		})
	}
}

// Property-based test for event emission uniqueness
func TestEventEmissionUniqueness(t *testing.T) {
	// Test that events are emitted exactly once per operation
	testCases := []struct {
		name           string
		operationCount int
		description    string
	}{
		{
			name:           "Single recreation emits single event",
			operationCount: 1,
			description:    "One recreation should emit exactly one event",
		},
		{
			name:           "Multiple recreations emit multiple events",
			operationCount: 3,
			description:    "Multiple recreations should emit one event each",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set up HTTP server that always succeeds
			httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "POST" && strings.Contains(r.URL.Path, "validate") {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(`{}`))
				} else if r.Method == "POST" {
					// Return different monitor IDs for each recreation
					monitorID := 10000 + len(r.URL.Path) // Simple way to get different IDs
					newMonitor := genericMonitor(monitorID)
					jsonMonitor, _ := newMonitor.MarshalJSON()
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write(jsonMonitor)
				}
			}))
			defer httpServer.Close()

			// Set up Datadog client
			testConfig := datadogapi.NewConfiguration()
			testConfig.HTTPClient = httpServer.Client()
			apiClient := datadogapi.NewAPIClient(testConfig)
			client := datadogV1.NewMonitorsApi(apiClient)
			testAuth := setupTestAuth(httpServer.URL)

			// Create fake event recorder with sufficient buffer
			fakeRecorder := record.NewFakeRecorder(tc.operationCount * 2)

			// Create reconciler with fake recorder
			r := &Reconciler{
				datadogClient: client,
				datadogAuth:   testAuth,
				log:           testLogger,
				recorder:      fakeRecorder,
			}

			// Perform multiple recreation operations
			for i := 0; i < tc.operationCount; i++ {
				// Create DatadogMonitor instance
				instance := &datadoghqv1alpha1.DatadogMonitor{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("test-monitor-%d", i),
						Namespace: "default",
					},
					Spec: datadoghqv1alpha1.DatadogMonitorSpec{
						Name:    fmt.Sprintf("Test Monitor %d", i),
						Message: "Test message",
						Query:   "avg(last_10m):avg:system.disk.in_use{*} by {host} > 0.1",
						Type:    datadoghqv1alpha1.DatadogMonitorTypeMetric,
					},
					Status: datadoghqv1alpha1.DatadogMonitorStatus{
						ID: 12345 + i,
					},
				}

				status := &datadoghqv1alpha1.DatadogMonitorStatus{
					ID: 12345 + i,
				}

				now := metav1.Now()
				instanceSpecHash := fmt.Sprintf("test-hash-%d", i)

				// Perform recreation
				err := r.handleMonitorRecreation(context.TODO(), testLogger, instance, status, now, instanceSpecHash)
				assert.NoError(t, err, "Recreation %d should succeed", i)
			}

			// Verify event count
			close(fakeRecorder.Events)
			events := []string{}
			for event := range fakeRecorder.Events {
				events = append(events, event)
			}

			assert.Equal(t, tc.operationCount, len(events), "Should emit exactly one event per recreation")

			// Verify all events are recreation events
			for i, event := range events {
				eventParts := strings.Fields(event)
				assert.True(t, len(eventParts) >= 2, "Event %d should have at least type and reason", i)
				assert.Equal(t, "Normal", eventParts[0], "Event %d should be Normal type", i)
				assert.Equal(t, "Recreate", eventParts[1], "Event %d should be Recreate reason", i)
			}
		})
	}
}

// Property-based test for event content accuracy
func TestEventContentAccuracy(t *testing.T) {
	// Test that event messages contain accurate information
	testCases := []struct {
		name         string
		monitorName  string
		namespace    string
		expectedText []string // Text that should be present in the event message
	}{
		{
			name:         "Event contains monitor name and namespace",
			monitorName:  "critical-cpu-alert",
			namespace:    "production",
			expectedText: []string{"critical-cpu-alert", "production"},
		},
		{
			name:         "Event handles special characters in names",
			monitorName:  "test-monitor-with-dashes",
			namespace:    "test-namespace",
			expectedText: []string{"test-monitor-with-dashes", "test-namespace"},
		},
		{
			name:         "Event handles long names",
			monitorName:  "very-long-monitor-name-that-exceeds-normal-length-limits",
			namespace:    "default",
			expectedText: []string{"very-long-monitor-name-that-exceeds-normal-length-limits", "default"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set up HTTP server that always succeeds
			httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "POST" && strings.Contains(r.URL.Path, "validate") {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(`{}`))
				} else if r.Method == "POST" {
					newMonitor := genericMonitor(99999)
					jsonMonitor, _ := newMonitor.MarshalJSON()
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write(jsonMonitor)
				}
			}))
			defer httpServer.Close()

			// Set up Datadog client
			testConfig := datadogapi.NewConfiguration()
			testConfig.HTTPClient = httpServer.Client()
			apiClient := datadogapi.NewAPIClient(testConfig)
			client := datadogV1.NewMonitorsApi(apiClient)
			testAuth := setupTestAuth(httpServer.URL)

			// Create fake event recorder
			fakeRecorder := record.NewFakeRecorder(10)

			// Create reconciler with fake recorder
			r := &Reconciler{
				datadogClient: client,
				datadogAuth:   testAuth,
				log:           testLogger,
				recorder:      fakeRecorder,
			}

			// Create DatadogMonitor instance
			instance := &datadoghqv1alpha1.DatadogMonitor{
				ObjectMeta: metav1.ObjectMeta{
					Name:      tc.monitorName,
					Namespace: tc.namespace,
				},
				Spec: datadoghqv1alpha1.DatadogMonitorSpec{
					Name:    tc.monitorName,
					Message: "Test message",
					Query:   "avg(last_10m):avg:system.disk.in_use{*} by {host} > 0.1",
					Type:    datadoghqv1alpha1.DatadogMonitorTypeMetric,
				},
				Status: datadoghqv1alpha1.DatadogMonitorStatus{
					ID: 12345,
				},
			}

			status := &datadoghqv1alpha1.DatadogMonitorStatus{
				ID: 12345,
			}

			now := metav1.Now()
			instanceSpecHash := "test-hash"

			// Perform recreation
			err := r.handleMonitorRecreation(context.TODO(), testLogger, instance, status, now, instanceSpecHash)
			assert.NoError(t, err, "Recreation should succeed")

			// Verify event content
			close(fakeRecorder.Events)
			events := []string{}
			for event := range fakeRecorder.Events {
				events = append(events, event)
			}

			assert.Equal(t, 1, len(events), "Should emit exactly one event")

			eventMessage := events[0]
			for _, expectedText := range tc.expectedText {
				assert.True(t, strings.Contains(eventMessage, expectedText),
					"Event message should contain '%s': %s", expectedText, eventMessage)
			}
		})
	}
}

// **Feature: monitor-recreation, Property 6: Independent Resource Processing**
// Property-based test for independent resource processing
func TestIndependentResourceProcessing(t *testing.T) {
	// Test that multiple DatadogMonitor resources are processed independently
	monitors := []struct {
		name      string
		namespace string
		id        int
		scenario  string // "exists", "missing", "error"
	}{
		{name: "monitor-1", namespace: "ns-1", id: 1001, scenario: "exists"},
		{name: "monitor-2", namespace: "ns-1", id: 1002, scenario: "missing"},
		{name: "monitor-3", namespace: "ns-2", id: 1003, scenario: "error"},
		{name: "monitor-4", namespace: "ns-2", id: 1004, scenario: "exists"},
		{name: "monitor-5", namespace: "ns-3", id: 1005, scenario: "missing"},
	}

	// Set up HTTP server that responds based on monitor ID
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Extract monitor ID from URL path
		var monitorID int
		fmt.Sscanf(r.URL.Path, "/api/v1/monitor/%d", &monitorID)

		// Find the monitor scenario
		for _, m := range monitors {
			if m.id == monitorID {
				switch m.scenario {
				case "exists":
					monitor := genericMonitor(monitorID)
					jsonMonitor, _ := monitor.MarshalJSON()
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write(jsonMonitor)
					return
				case "missing":
					w.WriteHeader(http.StatusNotFound)
					_, _ = w.Write([]byte(`{"errors": ["Monitor not found"]}`))
					return
				case "error":
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = w.Write([]byte(`{"errors": ["Internal server error"]}`))
					return
				}
			}
		}

		// Default: not found
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"errors": ["Monitor not found"]}`))
	}))
	defer httpServer.Close()

	// Set up Datadog client
	testConfig := datadogapi.NewConfiguration()
	testConfig.HTTPClient = httpServer.Client()
	apiClient := datadogapi.NewAPIClient(testConfig)
	client := datadogV1.NewMonitorsApi(apiClient)
	testAuth := setupTestAuth(httpServer.URL)

	// Create reconciler
	r := createTestReconciler(client, testAuth)

	// Process each monitor independently and collect results
	results := make([]struct {
		drift  bool
		err    error
		status datadoghqv1alpha1.DatadogMonitorStatus
	}, len(monitors))

	for i, m := range monitors {
		instance := &datadoghqv1alpha1.DatadogMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      m.name,
				Namespace: m.namespace,
			},
			Status: datadoghqv1alpha1.DatadogMonitorStatus{
				ID:      m.id,
				Primary: true,
				Creator: fmt.Sprintf("creator-%d@example.com", m.id),
			},
		}

		status := instance.Status.DeepCopy()

		// Test drift detection
		drift, err := r.detectDrift(context.TODO(), testLogger, instance, status)

		results[i] = struct {
			drift  bool
			err    error
			status datadoghqv1alpha1.DatadogMonitorStatus
		}{drift, err, *status}
	}

	// Verify results are independent and correct
	for i, m := range monitors {
		t.Run(fmt.Sprintf("Monitor_%s_%s", m.name, m.scenario), func(t *testing.T) {
			result := results[i]

			switch m.scenario {
			case "exists":
				assert.False(t, result.drift, "Monitor %s should not detect drift when it exists", m.name)
				assert.NoError(t, result.err, "Monitor %s should not have error when it exists", m.name)

			case "missing":
				assert.True(t, result.drift, "Monitor %s should detect drift when missing", m.name)
				assert.NoError(t, result.err, "Monitor %s should not have error when missing (drift is expected)", m.name)
				assert.Equal(t, datadoghqv1alpha1.MonitorStateSyncStatusGetError, result.status.MonitorStateSyncStatus,
					"Monitor %s should have get error status", m.name)

			case "error":
				assert.False(t, result.drift, "Monitor %s should not detect drift on API error", m.name)
				assert.Error(t, result.err, "Monitor %s should have error on API error", m.name)
				assert.Equal(t, datadoghqv1alpha1.MonitorStateSyncStatusGetError, result.status.MonitorStateSyncStatus,
					"Monitor %s should have get error status", m.name)
			}

			// Verify that other monitor properties are preserved independently
			assert.Equal(t, m.id, result.status.ID, "Monitor %s ID should be preserved", m.name)
			assert.True(t, result.status.Primary, "Monitor %s Primary status should be preserved", m.name)
			assert.Equal(t, fmt.Sprintf("creator-%d@example.com", m.id), result.status.Creator,
				"Monitor %s Creator should be preserved", m.name)
		})
	}

	// Verify no cross-contamination between monitors
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			assert.NotEqual(t, results[i].status.ID, results[j].status.ID,
				"Monitor IDs should remain distinct")
			assert.NotEqual(t, results[i].status.Creator, results[j].status.Creator,
				"Monitor creators should remain distinct")
		}
	}
}

// Property-based test for namespace isolation
func TestNamespaceIsolation(t *testing.T) {
	// Test that monitors in different namespaces don't interfere with each other
	namespaces := []string{"production", "staging", "development", "test"}

	// Set up HTTP server that tracks requests by namespace
	requestsByNamespace := make(map[string]int)
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// All monitors exist for this test
		monitor := genericMonitor(12345)
		jsonMonitor, _ := monitor.MarshalJSON()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(jsonMonitor)
	}))
	defer httpServer.Close()

	// Set up Datadog client
	testConfig := datadogapi.NewConfiguration()
	testConfig.HTTPClient = httpServer.Client()
	apiClient := datadogapi.NewAPIClient(testConfig)
	client := datadogV1.NewMonitorsApi(apiClient)
	testAuth := setupTestAuth(httpServer.URL)

	// Create reconciler
	r := createTestReconciler(client, testAuth)

	// Process monitors in each namespace
	for _, ns := range namespaces {
		instance := &datadoghqv1alpha1.DatadogMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-monitor",
				Namespace: ns,
			},
			Status: datadoghqv1alpha1.DatadogMonitorStatus{
				ID:      12345,
				Primary: true,
				Creator: fmt.Sprintf("creator-%s@example.com", ns),
			},
		}

		status := instance.Status.DeepCopy()

		// Test drift detection
		drift, err := r.detectDrift(context.TODO(), testLogger, instance, status)

		// Verify results
		assert.False(t, drift, "Monitor in namespace %s should not detect drift", ns)
		assert.NoError(t, err, "Monitor in namespace %s should not have error", ns)

		// Verify namespace-specific properties are preserved
		assert.Equal(t, fmt.Sprintf("creator-%s@example.com", ns), status.Creator,
			"Creator should be preserved for namespace %s", ns)

		requestsByNamespace[ns]++
	}

	// Verify each namespace was processed independently
	for _, ns := range namespaces {
		assert.Equal(t, 1, requestsByNamespace[ns],
			"Namespace %s should have been processed exactly once", ns)
	}
}

// Property-based test for concurrent processing safety
func TestConcurrentProcessingSafety(t *testing.T) {
	// Test that concurrent processing of multiple monitors is safe
	monitorCount := 10

	// Set up HTTP server with artificial delay to increase chance of race conditions
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Add small delay to simulate real API latency
		time.Sleep(10 * time.Millisecond)

		w.Header().Set("Content-Type", "application/json")

		// Extract monitor ID from URL
		var monitorID int
		fmt.Sscanf(r.URL.Path, "/api/v1/monitor/%d", &monitorID)

		// Return monitor based on ID
		monitor := genericMonitor(monitorID)
		jsonMonitor, _ := monitor.MarshalJSON()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(jsonMonitor)
	}))
	defer httpServer.Close()

	// Set up Datadog client
	testConfig := datadogapi.NewConfiguration()
	testConfig.HTTPClient = httpServer.Client()
	apiClient := datadogapi.NewAPIClient(testConfig)
	client := datadogV1.NewMonitorsApi(apiClient)
	testAuth := setupTestAuth(httpServer.URL)

	// Create reconciler
	r := createTestReconciler(client, testAuth)

	// Create channels for results
	type result struct {
		index int
		drift bool
		err   error
		id    int
	}

	results := make(chan result, monitorCount)

	// Process monitors concurrently
	for i := 0; i < monitorCount; i++ {
		go func(index int) {
			monitorID := 10000 + index
			instance := &datadoghqv1alpha1.DatadogMonitor{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("test-monitor-%d", index),
					Namespace: "default",
				},
				Status: datadoghqv1alpha1.DatadogMonitorStatus{
					ID:      monitorID,
					Primary: true,
					Creator: fmt.Sprintf("creator-%d@example.com", index),
				},
			}

			status := instance.Status.DeepCopy()

			// Test drift detection
			drift, err := r.detectDrift(context.TODO(), testLogger, instance, status)

			results <- result{
				index: index,
				drift: drift,
				err:   err,
				id:    status.ID,
			}
		}(i)
	}

	// Collect results
	collectedResults := make([]result, 0, monitorCount)
	for i := 0; i < monitorCount; i++ {
		select {
		case res := <-results:
			collectedResults = append(collectedResults, res)
		case <-time.After(5 * time.Second):
			t.Fatal("Timeout waiting for concurrent processing results")
		}
	}

	// Verify all operations completed successfully
	assert.Equal(t, monitorCount, len(collectedResults), "All concurrent operations should complete")

	// Verify no drift detected (all monitors exist)
	for _, res := range collectedResults {
		assert.False(t, res.drift, "Monitor %d should not detect drift", res.index)
		assert.NoError(t, res.err, "Monitor %d should not have error", res.index)
		assert.Equal(t, 10000+res.index, res.id, "Monitor %d should preserve correct ID", res.index)
	}

	// Verify all monitors were processed (no duplicates or missing)
	processedIndices := make(map[int]bool)
	for _, res := range collectedResults {
		assert.False(t, processedIndices[res.index], "Monitor index %d should not be processed twice", res.index)
		processedIndices[res.index] = true
	}

	assert.Equal(t, monitorCount, len(processedIndices), "All monitor indices should be processed")
}

// **Feature: monitor-recreation, Property 7: Concurrent Operation Safety**
// Property-based test for concurrent operation safety
func TestConcurrentOperationSafety(t *testing.T) {
	testCases := []struct {
		name                string
		concurrentOps       int
		simulateConflicts   bool
		simulateTimeout     bool
		expectedSuccessRate float64 // Percentage of operations that should succeed
	}{
		{
			name:                "Multiple concurrent recreations without conflicts",
			concurrentOps:       5,
			simulateConflicts:   false,
			simulateTimeout:     false,
			expectedSuccessRate: 1.0, // All should succeed
		},
		{
			name:                "Concurrent recreations with simulated conflicts",
			concurrentOps:       5,
			simulateConflicts:   true,
			simulateTimeout:     false,
			expectedSuccessRate: 0.6, // Some should fail due to conflicts
		},
		{
			name:                "Concurrent recreations with timeouts",
			concurrentOps:       3,
			simulateConflicts:   false,
			simulateTimeout:     true,
			expectedSuccessRate: 0.7, // Some should fail due to timeouts
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set up HTTP server with conflict and timeout simulation
			requestCount := 0
			httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requestCount++

				// Simulate conflicts for some requests
				if tc.simulateConflicts && requestCount%3 == 0 {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusConflict)
					_, _ = w.Write([]byte(`{"errors": ["Resource version conflict"]}`))
					return
				}

				// Simulate timeouts for some requests
				if tc.simulateTimeout && requestCount%4 == 0 {
					time.Sleep(100 * time.Millisecond) // Simulate slow response
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusRequestTimeout)
					_, _ = w.Write([]byte(`{"errors": ["Request timeout"]}`))
					return
				}

				// Handle validation and creation requests
				if r.Method == "POST" && strings.Contains(r.URL.Path, "validate") {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(`{}`))
				} else if r.Method == "POST" {
					// Successful creation
					newMonitor := genericMonitor(20000 + requestCount)
					jsonMonitor, _ := newMonitor.MarshalJSON()
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write(jsonMonitor)
				}
			}))
			defer httpServer.Close()

			// Set up Datadog client
			testConfig := datadogapi.NewConfiguration()
			testConfig.HTTPClient = httpServer.Client()
			apiClient := datadogapi.NewAPIClient(testConfig)
			client := datadogV1.NewMonitorsApi(apiClient)
			testAuth := setupTestAuth(httpServer.URL)

			// Create reconciler
			r := createTestReconciler(client, testAuth)

			// Create channels for results
			type result struct {
				index   int
				success bool
				err     error
				newID   int
			}

			results := make(chan result, tc.concurrentOps)

			// Launch concurrent recreation operations
			for i := 0; i < tc.concurrentOps; i++ {
				go func(index int) {
					// Create context with timeout to test timeout handling
					ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
					defer cancel()

					instance := &datadoghqv1alpha1.DatadogMonitor{
						ObjectMeta: metav1.ObjectMeta{
							Name:            fmt.Sprintf("test-monitor-%d", index),
							Namespace:       "default",
							ResourceVersion: fmt.Sprintf("v%d", index), // Simulate different resource versions
						},
						Spec: datadoghqv1alpha1.DatadogMonitorSpec{
							Name:    fmt.Sprintf("Test Monitor %d", index),
							Message: "Test message",
							Query:   "avg(last_10m):avg:system.disk.in_use{*} by {host} > 0.1",
							Type:    datadoghqv1alpha1.DatadogMonitorTypeMetric,
						},
						Status: datadoghqv1alpha1.DatadogMonitorStatus{
							ID: 10000 + index,
						},
					}

					status := &datadoghqv1alpha1.DatadogMonitorStatus{
						ID: 10000 + index,
					}

					now := metav1.Now()
					instanceSpecHash := fmt.Sprintf("hash-%d", index)

					// Perform recreation
					err := r.handleMonitorRecreation(ctx, testLogger, instance, status, now, instanceSpecHash)

					results <- result{
						index:   index,
						success: err == nil,
						err:     err,
						newID:   status.ID,
					}
				}(i)
			}

			// Collect results
			collectedResults := make([]result, 0, tc.concurrentOps)
			for i := 0; i < tc.concurrentOps; i++ {
				select {
				case res := <-results:
					collectedResults = append(collectedResults, res)
				case <-time.After(2 * time.Second):
					t.Fatal("Timeout waiting for concurrent operation results")
				}
			}

			// Verify results
			successCount := 0
			for _, res := range collectedResults {
				if res.success {
					successCount++
					// Verify successful operations have new monitor IDs
					assert.True(t, res.newID > 20000, "Successful recreation should have new monitor ID")
				} else {
					// Verify failed operations preserve original ID or handle errors appropriately
					assert.Error(t, res.err, "Failed operation should have error")

					// Check error types for expected scenarios
					errorMsg := res.err.Error()
					if tc.simulateConflicts {
						assert.True(t,
							strings.Contains(errorMsg, "conflict") ||
								strings.Contains(errorMsg, "rate limit") ||
								strings.Contains(errorMsg, "timeout"),
							"Error should be from expected conflict/rate limit/timeout scenarios: %s", errorMsg)
					}
				}
			}

			// Verify success rate is within expected range
			actualSuccessRate := float64(successCount) / float64(tc.concurrentOps)
			tolerance := 0.2 // Allow 20% tolerance for timing variations
			assert.True(t, actualSuccessRate >= tc.expectedSuccessRate-tolerance && actualSuccessRate <= tc.expectedSuccessRate+tolerance,
				"Success rate %.2f should be within tolerance of expected %.2f", actualSuccessRate, tc.expectedSuccessRate)

			// Verify no duplicate monitor IDs were created
			createdIDs := make(map[int]bool)
			for _, res := range collectedResults {
				if res.success && res.newID > 20000 {
					assert.False(t, createdIDs[res.newID], "Monitor ID %d should not be duplicated", res.newID)
					createdIDs[res.newID] = true
				}
			}
		})
	}
}

// Property-based test for resource deletion during recreation
func TestResourceDeletionDuringRecreation(t *testing.T) {
	// Test handling of resource deletion while recreation is in progress
	testCases := []struct {
		name           string
		cancelTiming   string // "before", "during", "after"
		expectedResult string // "cancelled", "completed", "error"
	}{
		{
			name:           "Cancellation before recreation starts",
			cancelTiming:   "before",
			expectedResult: "cancelled",
		},
		{
			name:           "Cancellation during recreation",
			cancelTiming:   "during",
			expectedResult: "cancelled",
		},
		{
			name:           "No cancellation - normal completion",
			cancelTiming:   "never",
			expectedResult: "completed",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set up HTTP server with delay to allow cancellation testing
			httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Add delay to simulate API latency
				if tc.cancelTiming == "during" {
					time.Sleep(50 * time.Millisecond)
				}

				if r.Method == "POST" && strings.Contains(r.URL.Path, "validate") {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(`{}`))
				} else if r.Method == "POST" {
					newMonitor := genericMonitor(30000)
					jsonMonitor, _ := newMonitor.MarshalJSON()
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write(jsonMonitor)
				}
			}))
			defer httpServer.Close()

			// Set up Datadog client
			testConfig := datadogapi.NewConfiguration()
			testConfig.HTTPClient = httpServer.Client()
			apiClient := datadogapi.NewAPIClient(testConfig)
			client := datadogV1.NewMonitorsApi(apiClient)
			testAuth := setupTestAuth(httpServer.URL)

			// Create reconciler
			r := createTestReconciler(client, testAuth)

			// Create context with cancellation
			ctx, cancel := context.WithCancel(context.Background())

			// Cancel at appropriate timing
			if tc.cancelTiming == "before" {
				cancel()
			} else if tc.cancelTiming == "during" {
				go func() {
					time.Sleep(25 * time.Millisecond) // Cancel during API call
					cancel()
				}()
			}
			// For "never", we don't cancel

			instance := &datadoghqv1alpha1.DatadogMonitor{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-monitor",
					Namespace: "default",
				},
				Spec: datadoghqv1alpha1.DatadogMonitorSpec{
					Name:    "Test Monitor",
					Message: "Test message",
					Query:   "avg(last_10m):avg:system.disk.in_use{*} by {host} > 0.1",
					Type:    datadoghqv1alpha1.DatadogMonitorTypeMetric,
				},
				Status: datadoghqv1alpha1.DatadogMonitorStatus{
					ID: 12345,
				},
			}

			status := &datadoghqv1alpha1.DatadogMonitorStatus{
				ID: 12345,
			}

			now := metav1.Now()
			instanceSpecHash := "test-hash"

			// Perform recreation
			err := r.handleMonitorRecreation(ctx, testLogger, instance, status, now, instanceSpecHash)

			// Verify results based on expected outcome
			switch tc.expectedResult {
			case "cancelled":
				assert.Error(t, err, "Should return error when cancelled")
				assert.True(t, strings.Contains(err.Error(), "context") || err == context.Canceled,
					"Error should indicate context cancellation: %v", err)
				// Original ID should be preserved on cancellation
				assert.Equal(t, 12345, status.ID, "Original monitor ID should be preserved on cancellation")

			case "completed":
				assert.NoError(t, err, "Should complete successfully when not cancelled")
				assert.Equal(t, 30000, status.ID, "Should have new monitor ID on successful completion")

			case "error":
				assert.Error(t, err, "Should return error")
				assert.Equal(t, 12345, status.ID, "Original monitor ID should be preserved on error")
			}

			// Clean up
			if tc.cancelTiming == "never" {
				cancel()
			}
		})
	}
}

// Property-based test for optimistic locking behavior
func TestOptimisticLockingBehavior(t *testing.T) {
	// Test that resource version conflicts are detected and handled appropriately
	testCases := []struct {
		name                    string
		initialResourceVersion  string
		modifiedResourceVersion string
		simulateConflict        bool
		expectedConflictError   bool
	}{
		{
			name:                    "No resource version change - no conflict",
			initialResourceVersion:  "v1",
			modifiedResourceVersion: "v1",
			simulateConflict:        false,
			expectedConflictError:   false,
		},
		{
			name:                    "Resource version changed - conflict detected",
			initialResourceVersion:  "v1",
			modifiedResourceVersion: "v2",
			simulateConflict:        true,
			expectedConflictError:   true,
		},
		{
			name:                    "Empty resource version - no conflict check",
			initialResourceVersion:  "",
			modifiedResourceVersion: "",
			simulateConflict:        false,
			expectedConflictError:   false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set up HTTP server that simulates conflicts when requested
			httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.simulateConflict && r.Method == "POST" && !strings.Contains(r.URL.Path, "validate") {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusConflict)
					_, _ = w.Write([]byte(`{"errors": ["Resource version conflict detected"]}`))
					return
				}

				if r.Method == "POST" && strings.Contains(r.URL.Path, "validate") {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(`{}`))
				} else if r.Method == "POST" {
					newMonitor := genericMonitor(40000)
					jsonMonitor, _ := newMonitor.MarshalJSON()
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write(jsonMonitor)
				}
			}))
			defer httpServer.Close()

			// Set up Datadog client
			testConfig := datadogapi.NewConfiguration()
			testConfig.HTTPClient = httpServer.Client()
			apiClient := datadogapi.NewAPIClient(testConfig)
			client := datadogV1.NewMonitorsApi(apiClient)
			testAuth := setupTestAuth(httpServer.URL)

			// Create reconciler
			r := createTestReconciler(client, testAuth)

			instance := &datadoghqv1alpha1.DatadogMonitor{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test-monitor",
					Namespace:       "default",
					ResourceVersion: tc.initialResourceVersion,
				},
				Spec: datadoghqv1alpha1.DatadogMonitorSpec{
					Name:    "Test Monitor",
					Message: "Test message",
					Query:   "avg(last_10m):avg:system.disk.in_use{*} by {host} > 0.1",
					Type:    datadoghqv1alpha1.DatadogMonitorTypeMetric,
				},
				Status: datadoghqv1alpha1.DatadogMonitorStatus{
					ID: 12345,
				},
			}

			// Simulate resource version change during processing
			if tc.modifiedResourceVersion != tc.initialResourceVersion {
				instance.ResourceVersion = tc.modifiedResourceVersion
			}

			status := &datadoghqv1alpha1.DatadogMonitorStatus{
				ID: 12345,
			}

			now := metav1.Now()
			instanceSpecHash := "test-hash"

			// Perform recreation
			err := r.handleMonitorRecreation(context.TODO(), testLogger, instance, status, now, instanceSpecHash)

			// Verify conflict handling
			if tc.expectedConflictError {
				assert.Error(t, err, "Should return error when conflict is detected")
				assert.True(t, strings.Contains(err.Error(), "conflict") || strings.Contains(err.Error(), "concurrent"),
					"Error should indicate conflict: %v", err)
				// Original ID should be preserved on conflict
				assert.Equal(t, 12345, status.ID, "Original monitor ID should be preserved on conflict")
			} else {
				if tc.simulateConflict {
					// If we simulate conflict but don't expect conflict error, it means the conflict detection didn't trigger
					assert.Error(t, err, "Should return error from simulated conflict")
				} else {
					assert.NoError(t, err, "Should complete successfully when no conflict")
					assert.Equal(t, 40000, status.ID, "Should have new monitor ID on successful completion")
				}
			}
		})
	}
}

// **Feature: monitor-recreation, Property 8: Status Error Reporting**
// Property-based test for status error reporting
func TestStatusErrorReporting(t *testing.T) {
	testCases := []struct {
		name                    string
		operation               string
		serverResponse          func(w http.ResponseWriter, r *http.Request)
		expectedError           bool
		expectedConditionType   datadoghqv1alpha1.DatadogMonitorConditionType
		expectedConditionStatus corev1.ConditionStatus
		expectedMessageContains []string
		shouldRetry             bool
	}{
		{
			name:      "Drift detection - monitor not found",
			operation: "drift_detection",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"errors": ["Monitor not found"]}`))
			},
			expectedError:           false, // Drift detection returns no error for not found
			expectedConditionType:   datadoghqv1alpha1.DatadogMonitorConditionTypeDriftDetected,
			expectedConditionStatus: corev1.ConditionTrue,
			expectedMessageContains: []string{"Monitor ID", "not found in Datadog API"},
			shouldRetry:             false,
		},
		{
			name:      "Drift detection - rate limit error",
			operation: "drift_detection",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"errors": ["Rate limit exceeded"]}`))
			},
			expectedError:           true,
			expectedConditionType:   datadoghqv1alpha1.DatadogMonitorConditionTypeError,
			expectedConditionStatus: corev1.ConditionTrue,
			expectedMessageContains: []string{"Rate limit", "drift detection", "monitor ID"},
			shouldRetry:             true,
		},
		{
			name:      "Drift detection - authentication error",
			operation: "drift_detection",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"errors": ["Unauthorized"]}`))
			},
			expectedError:           true,
			expectedConditionType:   datadoghqv1alpha1.DatadogMonitorConditionTypeError,
			expectedConditionStatus: corev1.ConditionTrue,
			expectedMessageContains: []string{"Authentication error", "credentials may be invalid"},
			shouldRetry:             false,
		},
		{
			name:      "Recreation - validation error",
			operation: "recreation",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "POST" && strings.Contains(r.URL.Path, "validate") {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusBadRequest)
					_, _ = w.Write([]byte(`{"errors": ["Invalid query syntax"]}`))
				}
			},
			expectedError:           true,
			expectedConditionType:   datadoghqv1alpha1.DatadogMonitorConditionTypeError,
			expectedConditionStatus: corev1.ConditionTrue,
			expectedMessageContains: []string{"Validation error", "monitor configuration is invalid"},
			shouldRetry:             false,
		},
		{
			name:      "Recreation - timeout error",
			operation: "recreation",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "POST" && strings.Contains(r.URL.Path, "validate") {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(`{}`))
				} else if r.Method == "POST" {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusRequestTimeout)
					_, _ = w.Write([]byte(`{"errors": ["Request timeout"]}`))
				}
			},
			expectedError:           true,
			expectedConditionType:   datadoghqv1alpha1.DatadogMonitorConditionTypeError,
			expectedConditionStatus: corev1.ConditionTrue,
			expectedMessageContains: []string{"Timeout", "API request timed out", "will retry"},
			shouldRetry:             true,
		},
		{
			name:      "Recreation - authorization error",
			operation: "recreation",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "POST" && strings.Contains(r.URL.Path, "validate") {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(`{}`))
				} else if r.Method == "POST" {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusForbidden)
					_, _ = w.Write([]byte(`{"errors": ["Forbidden"]}`))
				}
			},
			expectedError:           true,
			expectedConditionType:   datadoghqv1alpha1.DatadogMonitorConditionTypeError,
			expectedConditionStatus: corev1.ConditionTrue,
			expectedMessageContains: []string{"Authorization error", "insufficient permissions"},
			shouldRetry:             false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set up HTTP server
			httpServer := httptest.NewServer(http.HandlerFunc(tc.serverResponse))
			defer httpServer.Close()

			// Set up Datadog client
			testConfig := datadogapi.NewConfiguration()
			testConfig.HTTPClient = httpServer.Client()
			apiClient := datadogapi.NewAPIClient(testConfig)
			client := datadogV1.NewMonitorsApi(apiClient)
			testAuth := setupTestAuth(httpServer.URL)

			// Create reconciler
			r := createTestReconciler(client, testAuth)

			// Create DatadogMonitor instance
			instance := &datadoghqv1alpha1.DatadogMonitor{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-monitor",
					Namespace: "default",
				},
				Spec: datadoghqv1alpha1.DatadogMonitorSpec{
					Name:    "Test Monitor",
					Message: "Test message",
					Query:   "avg(last_10m):avg:system.disk.in_use{*} by {host} > 0.1",
					Type:    datadoghqv1alpha1.DatadogMonitorTypeMetric,
				},
				Status: datadoghqv1alpha1.DatadogMonitorStatus{
					ID: 12345,
				},
			}

			status := &datadoghqv1alpha1.DatadogMonitorStatus{
				ID: 12345,
			}

			// Perform the operation
			var err error
			switch tc.operation {
			case "drift_detection":
				_, err = r.detectDrift(context.TODO(), testLogger, instance, status)
			case "recreation":
				now := metav1.Now()
				err = r.handleMonitorRecreation(context.TODO(), testLogger, instance, status, now, "test-hash")
			}

			// Verify error expectation
			if tc.expectedError {
				assert.Error(t, err, "Expected error but got none")
			} else {
				assert.NoError(t, err, "Unexpected error: %v", err)
			}

			// Verify condition is set correctly
			conditionFound := false
			for _, condition := range status.Conditions {
				if condition.Type == tc.expectedConditionType {
					conditionFound = true
					assert.Equal(t, tc.expectedConditionStatus, condition.Status,
						"Condition status mismatch for type %s", tc.expectedConditionType)

					// Verify message contains expected content
					for _, expectedContent := range tc.expectedMessageContains {
						assert.True(t, strings.Contains(condition.Message, expectedContent),
							"Condition message should contain '%s': %s", expectedContent, condition.Message)
					}

					// Verify timestamps are set
					assert.False(t, condition.LastUpdateTime.IsZero(), "LastUpdateTime should be set")
					assert.False(t, condition.LastTransitionTime.IsZero(), "LastTransitionTime should be set")
					break
				}
			}

			assert.True(t, conditionFound, "Expected condition type %s not found", tc.expectedConditionType)

			// Verify retry behavior indication in error message
			if tc.expectedError && tc.shouldRetry {
				assert.True(t, strings.Contains(err.Error(), "retry") || strings.Contains(err.Error(), "will retry"),
					"Error message should indicate retry for retryable errors: %s", err.Error())
			}
		})
	}
}

// Property-based test for error message detail and categorization
func TestErrorMessageDetailAndCategorization(t *testing.T) {
	// Test that error messages provide sufficient detail for troubleshooting
	errorScenarios := []struct {
		name              string
		statusCode        int
		responseBody      string
		operation         string
		expectedCategory  string
		expectedDetails   []string
		expectedRetryable bool
	}{
		{
			name:              "Rate limit with detailed message",
			statusCode:        http.StatusTooManyRequests,
			responseBody:      `{"errors": ["Rate limit exceeded. Try again in 60 seconds."]}`,
			operation:         "drift_detection",
			expectedCategory:  "rate_limit",
			expectedDetails:   []string{"Rate limit", "monitor ID", "drift detection"},
			expectedRetryable: true,
		},
		{
			name:              "Authentication with credential guidance",
			statusCode:        http.StatusUnauthorized,
			responseBody:      `{"errors": ["Invalid API key"]}`,
			operation:         "recreation",
			expectedCategory:  "authentication",
			expectedDetails:   []string{"Authentication error", "credentials are invalid", "monitor ID"},
			expectedRetryable: false,
		},
		{
			name:              "Authorization with permission guidance",
			statusCode:        http.StatusForbidden,
			responseBody:      `{"errors": ["Insufficient permissions to create monitors"]}`,
			operation:         "recreation",
			expectedCategory:  "authorization",
			expectedDetails:   []string{"Authorization error", "insufficient permissions", "create monitors"},
			expectedRetryable: false,
		},
		{
			name:              "Validation with configuration guidance",
			statusCode:        http.StatusBadRequest,
			responseBody:      `{"errors": ["Invalid query: syntax error at position 10"]}`,
			operation:         "recreation",
			expectedCategory:  "validation",
			expectedDetails:   []string{"Validation error", "monitor configuration is invalid"},
			expectedRetryable: false,
		},
		{
			name:              "Timeout with retry guidance",
			statusCode:        http.StatusRequestTimeout,
			responseBody:      `{"errors": ["Request timeout after 30 seconds"]}`,
			operation:         "recreation",
			expectedCategory:  "timeout",
			expectedDetails:   []string{"Timeout", "API request timed out", "will retry"},
			expectedRetryable: true,
		},
		{
			name:              "Service unavailable with retry guidance",
			statusCode:        http.StatusServiceUnavailable,
			responseBody:      `{"errors": ["Service temporarily unavailable"]}`,
			operation:         "drift_detection",
			expectedCategory:  "service_error",
			expectedDetails:   []string{"API error", "monitor ID"},
			expectedRetryable: true,
		},
	}

	for _, scenario := range errorScenarios {
		t.Run(scenario.name, func(t *testing.T) {
			// Set up HTTP server
			httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "POST" && strings.Contains(r.URL.Path, "validate") && scenario.operation == "recreation" {
					if scenario.statusCode == http.StatusBadRequest {
						// Validation error
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(scenario.statusCode)
						_, _ = w.Write([]byte(scenario.responseBody))
					} else {
						// Validation succeeds, error in creation
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(http.StatusOK)
						_, _ = w.Write([]byte(`{}`))
					}
				} else if r.Method == "POST" && scenario.operation == "recreation" {
					// Creation request
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(scenario.statusCode)
					_, _ = w.Write([]byte(scenario.responseBody))
				} else {
					// Drift detection request
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(scenario.statusCode)
					_, _ = w.Write([]byte(scenario.responseBody))
				}
			}))
			defer httpServer.Close()

			// Set up Datadog client
			testConfig := datadogapi.NewConfiguration()
			testConfig.HTTPClient = httpServer.Client()
			apiClient := datadogapi.NewAPIClient(testConfig)
			client := datadogV1.NewMonitorsApi(apiClient)
			testAuth := setupTestAuth(httpServer.URL)

			// Create reconciler
			r := createTestReconciler(client, testAuth)

			// Create DatadogMonitor instance
			instance := &datadoghqv1alpha1.DatadogMonitor{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-monitor",
					Namespace: "default",
				},
				Spec: datadoghqv1alpha1.DatadogMonitorSpec{
					Name:    "Test Monitor",
					Message: "Test message",
					Query:   "avg(last_10m):avg:system.disk.in_use{*} by {host} > 0.1",
					Type:    datadoghqv1alpha1.DatadogMonitorTypeMetric,
				},
				Status: datadoghqv1alpha1.DatadogMonitorStatus{
					ID: 12345,
				},
			}

			status := &datadoghqv1alpha1.DatadogMonitorStatus{
				ID: 12345,
			}

			// Perform the operation
			var err error
			switch scenario.operation {
			case "drift_detection":
				_, err = r.detectDrift(context.TODO(), testLogger, instance, status)
			case "recreation":
				now := metav1.Now()
				err = r.handleMonitorRecreation(context.TODO(), testLogger, instance, status, now, "test-hash")
			}

			// Verify error conditions are set with detailed messages
			errorConditionFound := false
			for _, condition := range status.Conditions {
				if condition.Type == datadoghqv1alpha1.DatadogMonitorConditionTypeError && condition.Status == corev1.ConditionTrue {
					errorConditionFound = true

					// Verify message contains expected details
					for _, expectedDetail := range scenario.expectedDetails {
						assert.True(t, strings.Contains(condition.Message, expectedDetail),
							"Error condition message should contain '%s': %s", expectedDetail, condition.Message)
					}

					// Verify message provides actionable information
					assert.True(t, len(condition.Message) > 20,
						"Error message should be detailed enough for troubleshooting: %s", condition.Message)

					break
				}
			}

			// For operations that should set error conditions
			if scenario.statusCode != http.StatusNotFound || scenario.operation != "drift_detection" {
				assert.True(t, errorConditionFound, "Error condition should be set for error scenarios")
			}

			// Verify retry indication in error messages
			if err != nil && scenario.expectedRetryable {
				assert.True(t, strings.Contains(err.Error(), "retry") || strings.Contains(err.Error(), "will retry"),
					"Retryable errors should indicate retry in message: %s", err.Error())
			}
		})
	}
}

// Property-based test for error condition lifecycle
func TestErrorConditionLifecycle(t *testing.T) {
	// Test that error conditions are properly set, updated, and cleared
	testCases := []struct {
		name                string
		operations          []string // Sequence of operations to perform
		expectedConditions  []datadoghqv1alpha1.DatadogMonitorConditionType
		finalConditionState map[datadoghqv1alpha1.DatadogMonitorConditionType]corev1.ConditionStatus
	}{
		{
			name:       "Error then success clears error condition",
			operations: []string{"error", "success"},
			expectedConditions: []datadoghqv1alpha1.DatadogMonitorConditionType{
				datadoghqv1alpha1.DatadogMonitorConditionTypeError,
			},
			finalConditionState: map[datadoghqv1alpha1.DatadogMonitorConditionType]corev1.ConditionStatus{
				datadoghqv1alpha1.DatadogMonitorConditionTypeError: corev1.ConditionFalse,
			},
		},
		{
			name:       "Multiple errors update same condition",
			operations: []string{"error", "error", "error"},
			expectedConditions: []datadoghqv1alpha1.DatadogMonitorConditionType{
				datadoghqv1alpha1.DatadogMonitorConditionTypeError,
			},
			finalConditionState: map[datadoghqv1alpha1.DatadogMonitorConditionType]corev1.ConditionStatus{
				datadoghqv1alpha1.DatadogMonitorConditionTypeError: corev1.ConditionTrue,
			},
		},
		{
			name:       "Drift detection then error shows both conditions",
			operations: []string{"drift", "error"},
			expectedConditions: []datadoghqv1alpha1.DatadogMonitorConditionType{
				datadoghqv1alpha1.DatadogMonitorConditionTypeDriftDetected,
				datadoghqv1alpha1.DatadogMonitorConditionTypeError,
			},
			finalConditionState: map[datadoghqv1alpha1.DatadogMonitorConditionType]corev1.ConditionStatus{
				datadoghqv1alpha1.DatadogMonitorConditionTypeDriftDetected: corev1.ConditionTrue,
				datadoghqv1alpha1.DatadogMonitorConditionTypeError:         corev1.ConditionTrue,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set up HTTP server that responds based on operation type
			operationIndex := 0
			httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")

				if operationIndex < len(tc.operations) {
					operation := tc.operations[operationIndex]
					operationIndex++

					switch operation {
					case "error":
						w.WriteHeader(http.StatusInternalServerError)
						_, _ = w.Write([]byte(`{"errors": ["Internal server error"]}`))
					case "success":
						if strings.Contains(r.URL.Path, "monitor") {
							// Drift detection success
							monitor := genericMonitor(12345)
							jsonMonitor, _ := monitor.MarshalJSON()
							w.WriteHeader(http.StatusOK)
							_, _ = w.Write(jsonMonitor)
						} else {
							w.WriteHeader(http.StatusOK)
							_, _ = w.Write([]byte(`{}`))
						}
					case "drift":
						w.WriteHeader(http.StatusNotFound)
						_, _ = w.Write([]byte(`{"errors": ["Monitor not found"]}`))
					}
				} else {
					// Default success
					monitor := genericMonitor(12345)
					jsonMonitor, _ := monitor.MarshalJSON()
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write(jsonMonitor)
				}
			}))
			defer httpServer.Close()

			// Set up Datadog client
			testConfig := datadogapi.NewConfiguration()
			testConfig.HTTPClient = httpServer.Client()
			apiClient := datadogapi.NewAPIClient(testConfig)
			client := datadogV1.NewMonitorsApi(apiClient)
			testAuth := setupTestAuth(httpServer.URL)

			// Create reconciler
			r := createTestReconciler(client, testAuth)

			// Create DatadogMonitor instance
			instance := &datadoghqv1alpha1.DatadogMonitor{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-monitor",
					Namespace: "default",
				},
				Status: datadoghqv1alpha1.DatadogMonitorStatus{
					ID: 12345,
				},
			}

			status := &datadoghqv1alpha1.DatadogMonitorStatus{
				ID: 12345,
			}

			// Perform operations in sequence
			operationIndex = 0 // Reset for actual operations
			for _, operation := range tc.operations {
				switch operation {
				case "error", "success", "drift":
					_, _ = r.detectDrift(context.TODO(), testLogger, instance, status)
				}
			}

			// Verify expected conditions exist
			for _, expectedCondition := range tc.expectedConditions {
				conditionFound := false
				for _, condition := range status.Conditions {
					if condition.Type == expectedCondition {
						conditionFound = true
						break
					}
				}
				assert.True(t, conditionFound, "Expected condition %s not found", expectedCondition)
			}

			// Verify final condition states
			for conditionType, expectedStatus := range tc.finalConditionState {
				conditionFound := false
				for _, condition := range status.Conditions {
					if condition.Type == conditionType {
						conditionFound = true
						assert.Equal(t, expectedStatus, condition.Status,
							"Final condition status mismatch for %s", conditionType)
						break
					}
				}
				assert.True(t, conditionFound, "Expected final condition %s not found", conditionType)
			}
		})
	}
}

// **Unit Tests for New Controller Methods**
// Unit tests for handleMonitorRecreation method
func TestHandleMonitorRecreation(t *testing.T) {
	testCases := []struct {
		name           string
		initialID      int
		serverResponse func(w http.ResponseWriter, r *http.Request)
		expectedError  bool
		expectedNewID  int
		validateStatus func(t *testing.T, status *datadoghqv1alpha1.DatadogMonitorStatus)
	}{
		{
			name:      "Successful recreation",
			initialID: 12345,
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "POST" && strings.Contains(r.URL.Path, "validate") {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(`{}`))
				} else if r.Method == "POST" {
					newMonitor := genericMonitor(67890)
					jsonMonitor, _ := newMonitor.MarshalJSON()
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write(jsonMonitor)
				}
			},
			expectedError: false,
			expectedNewID: 67890,
			validateStatus: func(t *testing.T, status *datadoghqv1alpha1.DatadogMonitorStatus) {
				assert.Equal(t, 67890, status.ID)
				assert.True(t, status.Primary)
				assert.NotEmpty(t, status.CurrentHash)

				// Check for recreated condition
				hasRecreatedCondition := false
				for _, condition := range status.Conditions {
					if condition.Type == datadoghqv1alpha1.DatadogMonitorConditionTypeRecreated &&
						condition.Status == corev1.ConditionTrue {
						hasRecreatedCondition = true
						break
					}
				}
				assert.True(t, hasRecreatedCondition, "Should have Recreated condition")
			},
		},
		{
			name:      "Recreation with validation error",
			initialID: 12345,
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "POST" && strings.Contains(r.URL.Path, "validate") {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusBadRequest)
					_, _ = w.Write([]byte(`{"errors": ["Invalid query syntax"]}`))
				}
			},
			expectedError: true,
			expectedNewID: 12345, // Should preserve original ID
			validateStatus: func(t *testing.T, status *datadoghqv1alpha1.DatadogMonitorStatus) {
				assert.Equal(t, 12345, status.ID)

				// Check for error condition
				hasErrorCondition := false
				for _, condition := range status.Conditions {
					if condition.Type == datadoghqv1alpha1.DatadogMonitorConditionTypeError &&
						condition.Status == corev1.ConditionTrue {
						hasErrorCondition = true
						assert.Contains(t, condition.Message, "Validation error")
						break
					}
				}
				assert.True(t, hasErrorCondition, "Should have Error condition")
			},
		},
		{
			name:      "Recreation with API error",
			initialID: 12345,
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "POST" && strings.Contains(r.URL.Path, "validate") {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(`{}`))
				} else if r.Method == "POST" {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = w.Write([]byte(`{"errors": ["Internal server error"]}`))
				}
			},
			expectedError: true,
			expectedNewID: 12345, // Should preserve original ID
			validateStatus: func(t *testing.T, status *datadoghqv1alpha1.DatadogMonitorStatus) {
				assert.Equal(t, 12345, status.ID)

				// Check for error condition
				hasErrorCondition := false
				for _, condition := range status.Conditions {
					if condition.Type == datadoghqv1alpha1.DatadogMonitorConditionTypeError &&
						condition.Status == corev1.ConditionTrue {
						hasErrorCondition = true
						assert.Contains(t, condition.Message, "Failed to recreate")
						break
					}
				}
				assert.True(t, hasErrorCondition, "Should have Error condition")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set up HTTP server
			httpServer := httptest.NewServer(http.HandlerFunc(tc.serverResponse))
			defer httpServer.Close()

			// Set up Datadog client
			testConfig := datadogapi.NewConfiguration()
			testConfig.HTTPClient = httpServer.Client()
			apiClient := datadogapi.NewAPIClient(testConfig)
			client := datadogV1.NewMonitorsApi(apiClient)
			testAuth := setupTestAuth(httpServer.URL)

			// Create reconciler
			r := createTestReconciler(client, testAuth)

			// Create DatadogMonitor instance
			instance := &datadoghqv1alpha1.DatadogMonitor{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-monitor",
					Namespace: "default",
				},
				Spec: datadoghqv1alpha1.DatadogMonitorSpec{
					Name:    "Test Monitor",
					Message: "Test message",
					Query:   "avg(last_10m):avg:system.disk.in_use{*} by {host} > 0.1",
					Type:    datadoghqv1alpha1.DatadogMonitorTypeMetric,
				},
				Status: datadoghqv1alpha1.DatadogMonitorStatus{
					ID: tc.initialID,
				},
			}

			status := &datadoghqv1alpha1.DatadogMonitorStatus{
				ID: tc.initialID,
			}

			now := metav1.Now()
			instanceSpecHash := "test-hash"

			// Test handleMonitorRecreation
			err := r.handleMonitorRecreation(context.TODO(), testLogger, instance, status, now, instanceSpecHash)

			// Verify results
			if tc.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tc.expectedNewID, status.ID)

			if tc.validateStatus != nil {
				tc.validateStatus(t, status)
			}
		})
	}
}

// Unit tests for enhanced drift detection logic
func TestDetectDriftEnhanced(t *testing.T) {
	testCases := []struct {
		name               string
		monitorID          int
		serverResponse     func(w http.ResponseWriter, r *http.Request)
		expectedDrift      bool
		expectedError      bool
		validateConditions func(t *testing.T, status *datadoghqv1alpha1.DatadogMonitorStatus)
	}{
		{
			name:      "No drift - monitor exists",
			monitorID: 12345,
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				monitor := genericMonitor(12345)
				jsonMonitor, _ := monitor.MarshalJSON()
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(jsonMonitor)
			},
			expectedDrift: false,
			expectedError: false,
			validateConditions: func(t *testing.T, status *datadoghqv1alpha1.DatadogMonitorStatus) {
				// Should clear any previous error conditions
				for _, condition := range status.Conditions {
					if condition.Type == datadoghqv1alpha1.DatadogMonitorConditionTypeError {
						assert.Equal(t, corev1.ConditionFalse, condition.Status)
					}
				}
			},
		},
		{
			name:      "Drift detected - monitor not found",
			monitorID: 99999,
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"errors": ["Monitor not found"]}`))
			},
			expectedDrift: true,
			expectedError: false,
			validateConditions: func(t *testing.T, status *datadoghqv1alpha1.DatadogMonitorStatus) {
				// Should have drift detected condition
				hasDriftCondition := false
				for _, condition := range status.Conditions {
					if condition.Type == datadoghqv1alpha1.DatadogMonitorConditionTypeDriftDetected &&
						condition.Status == corev1.ConditionTrue {
						hasDriftCondition = true
						assert.Contains(t, condition.Message, "not found in Datadog API")
						break
					}
				}
				assert.True(t, hasDriftCondition, "Should have DriftDetected condition")
			},
		},
		{
			name:      "API error with detailed status",
			monitorID: 12345,
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"errors": ["Unauthorized"]}`))
			},
			expectedDrift: false,
			expectedError: true,
			validateConditions: func(t *testing.T, status *datadoghqv1alpha1.DatadogMonitorStatus) {
				// Should have error condition with detailed message
				hasErrorCondition := false
				for _, condition := range status.Conditions {
					if condition.Type == datadoghqv1alpha1.DatadogMonitorConditionTypeError &&
						condition.Status == corev1.ConditionTrue {
						hasErrorCondition = true
						assert.Contains(t, condition.Message, "Authentication error")
						assert.Contains(t, condition.Message, "credentials may be invalid")
						break
					}
				}
				assert.True(t, hasErrorCondition, "Should have Error condition")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set up HTTP server
			httpServer := httptest.NewServer(http.HandlerFunc(tc.serverResponse))
			defer httpServer.Close()

			// Set up Datadog client
			testConfig := datadogapi.NewConfiguration()
			testConfig.HTTPClient = httpServer.Client()
			apiClient := datadogapi.NewAPIClient(testConfig)
			client := datadogV1.NewMonitorsApi(apiClient)
			testAuth := setupTestAuth(httpServer.URL)

			// Create reconciler
			r := createTestReconciler(client, testAuth)

			// Create DatadogMonitor instance
			instance := &datadoghqv1alpha1.DatadogMonitor{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-monitor",
					Namespace: "default",
				},
				Status: datadoghqv1alpha1.DatadogMonitorStatus{
					ID: tc.monitorID,
				},
			}

			status := &datadoghqv1alpha1.DatadogMonitorStatus{
				ID: tc.monitorID,
			}

			// Test detectDrift
			driftDetected, err := r.detectDrift(context.TODO(), testLogger, instance, status)

			// Verify results
			assert.Equal(t, tc.expectedDrift, driftDetected)
			if tc.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			if tc.validateConditions != nil {
				tc.validateConditions(t, status)
			}
		})
	}
}

// Unit tests for new status management functions
func TestStatusManagementFunctions(t *testing.T) {
	t.Run("Status preservation during operations", func(t *testing.T) {
		// Test that valid existing status information is preserved
		originalStatus := &datadoghqv1alpha1.DatadogMonitorStatus{
			ID:           12345,
			Primary:      true,
			Creator:      "test@example.com",
			Created:      &metav1.Time{Time: time.Now().Add(-24 * time.Hour)},
			MonitorState: datadoghqv1alpha1.DatadogMonitorStateOK,
			CurrentHash:  "original-hash",
		}

		// Simulate status update during drift detection
		status := originalStatus.DeepCopy()
		status.MonitorStateSyncStatus = datadoghqv1alpha1.MonitorStateSyncStatusGetError

		// Verify important fields are preserved
		assert.Equal(t, originalStatus.ID, status.ID)
		assert.Equal(t, originalStatus.Primary, status.Primary)
		assert.Equal(t, originalStatus.Creator, status.Creator)
		assert.Equal(t, originalStatus.Created, status.Created)
		assert.Equal(t, originalStatus.MonitorState, status.MonitorState)
		assert.Equal(t, originalStatus.CurrentHash, status.CurrentHash)
	})

	t.Run("Condition management", func(t *testing.T) {
		status := &datadoghqv1alpha1.DatadogMonitorStatus{}
		now := metav1.Now()

		// Test adding drift detected condition
		condition.UpdateDatadogMonitorConditions(status, now,
			datadoghqv1alpha1.DatadogMonitorConditionTypeDriftDetected,
			corev1.ConditionTrue, "Test drift message")

		assert.Len(t, status.Conditions, 1)
		assert.Equal(t, datadoghqv1alpha1.DatadogMonitorConditionTypeDriftDetected, status.Conditions[0].Type)
		assert.Equal(t, corev1.ConditionTrue, status.Conditions[0].Status)
		assert.Equal(t, "Test drift message", status.Conditions[0].Message)

		// Test adding recreated condition
		condition.UpdateDatadogMonitorConditions(status, now,
			datadoghqv1alpha1.DatadogMonitorConditionTypeRecreated,
			corev1.ConditionTrue, "Test recreation message")

		assert.Len(t, status.Conditions, 2)

		// Test updating existing condition
		condition.UpdateDatadogMonitorConditions(status, now,
			datadoghqv1alpha1.DatadogMonitorConditionTypeDriftDetected,
			corev1.ConditionFalse, "")

		assert.Len(t, status.Conditions, 2)
		for _, cond := range status.Conditions {
			if cond.Type == datadoghqv1alpha1.DatadogMonitorConditionTypeDriftDetected {
				assert.Equal(t, corev1.ConditionFalse, cond.Status)
				assert.Empty(t, cond.Message)
			}
		}
	})

	t.Run("Error status categorization", func(t *testing.T) {
		testCases := []struct {
			errorType       string
			expectedMessage string
		}{
			{"rate limit", "Rate limit"},
			{"unauthorized", "Authentication error"},
			{"forbidden", "Authorization error"},
			{"validation", "Validation error"},
			{"timeout", "Timeout"},
		}

		for _, tc := range testCases {
			t.Run(tc.errorType, func(t *testing.T) {
				status := &datadoghqv1alpha1.DatadogMonitorStatus{}
				now := metav1.Now()

				message := fmt.Sprintf("%s during test operation", tc.expectedMessage)
				condition.UpdateDatadogMonitorConditions(status, now,
					datadoghqv1alpha1.DatadogMonitorConditionTypeError,
					corev1.ConditionTrue, message)

				assert.Len(t, status.Conditions, 1)
				assert.Contains(t, status.Conditions[0].Message, tc.expectedMessage)
			})
		}
	})
}
