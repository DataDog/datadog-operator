// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2021 Datadog, Inc.

package datadogmonitor

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	datadogapiclientv1 "github.com/DataDog/datadog-api-client-go/api/v1/datadog"
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/v1alpha1"
)

const dateFormat = "2006-01-02 15:04:05.999999999 -0700 MST"

func Test_buildMonitor(t *testing.T) {
	// Define a monitor dm *datadoghqv1alpha1.DatadogMonitor
	// Assert that each of the components of dm is equal to the output components in *datadogapiclientv1.Monitor and *datadogapiclientv1.MonitorUpdateRequest

	// What types of monitors to define?
	dm := &datadoghqv1alpha1.DatadogMonitor{
		Spec: datadoghqv1alpha1.DatadogMonitorSpec{
			Query:   "avg(last_10m):avg:system.disk.in_use{*} by {host} > 0.05",
			Type:    "metric alert",
			Name:    "Test monitor",
			Message: "Something went wrong",
			Tags: []string{
				"env:staging",
				"kube_namespace:test",
				"kube_cluster:test.staging",
			},
		},
	}

	monitor, monitorUR := buildMonitor(dm)

	assert.Equal(t, dm.Spec.Query, *monitor.Query, "discrepancy found in parameter: Query")
	assert.Equal(t, dm.Spec.Query, *monitorUR.Query, "discrepancy found in parameter: Query")

	assert.Equal(t, string(dm.Spec.Type), string(*monitor.Type), "discrepancy found in parameter: Type")
	assert.Equal(t, string(dm.Spec.Type), string(*monitorUR.Type), "discrepancy found in parameter: Type")

	assert.Equal(t, dm.Spec.Name, *monitor.Name, "discrepancy found in parameter: Name")
	assert.Equal(t, dm.Spec.Name, *monitorUR.Name, "discrepancy found in parameter: Name")

	assert.Equal(t, dm.Spec.Message, *monitor.Message, "discrepancy found in parameter: Message")
	assert.Equal(t, dm.Spec.Message, *monitorUR.Message, "discrepancy found in parameter: Message")

	assert.Equal(t, dm.Spec.Tags, *monitor.Tags, "discrepancy found in parameter: Tags")
	assert.Equal(t, dm.Spec.Tags, *monitorUR.Tags, "discrepancy found in parameter: Tags")

	// Also make sure tags are sorted
	assert.Equal(t, "env:staging", (*monitor.Tags)[0], "tags are not properly sorted")
	assert.Equal(t, "kube_cluster:test.staging", (*monitor.Tags)[1], "tags are not properly sorted")
	assert.Equal(t, "kube_namespace:test", (*monitor.Tags)[2], "tags are not properly sorted")

	assert.Equal(t, "env:staging", (*monitorUR.Tags)[0], "tags are not properly sorted")
	assert.Equal(t, "kube_cluster:test.staging", (*monitorUR.Tags)[1], "tags are not properly sorted")
	assert.Equal(t, "kube_namespace:test", (*monitorUR.Tags)[2], "tags are not properly sorted")
}

func Test_getMonitor(t *testing.T) {
	mId := 12345
	expectedMonitor := genericMonitor(mId)
	jsonMonitor, _ := expectedMonitor.MarshalJSON()
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(jsonMonitor)
	}))
	defer httpServer.Close()

	testConfig := datadogapiclientv1.NewConfiguration()
	testConfig.HTTPClient = httpServer.Client()
	client := datadogapiclientv1.NewAPIClient(testConfig)
	testAuth := setupTestAuth(httpServer.URL)

	val, err := getMonitor(testAuth, client, mId)
	assert.Nil(t, err)
	assert.Equal(t, expectedMonitor, val)
}

func Test_validateMonitor(t *testing.T) {
	dm := &datadoghqv1alpha1.DatadogMonitor{
		Spec: datadoghqv1alpha1.DatadogMonitorSpec{
			Query:   "avg(last_10m):avg:system.disk.in_use{*} by {host} > 0.05",
			Type:    "metric alert",
			Name:    "Test monitor",
			Message: "Something went wrong",
			Tags: []string{
				"env:staging",
				"kube_namespace:test",
				"kube_cluster:test.staging",
			},
		},
	}

	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
	}))
	defer httpServer.Close()

	testConfig := datadogapiclientv1.NewConfiguration()
	testConfig.HTTPClient = httpServer.Client()
	client := datadogapiclientv1.NewAPIClient(testConfig)
	testAuth := setupTestAuth(httpServer.URL)

	err := validateMonitor(testAuth, client, dm)
	assert.Nil(t, err)
}

func Test_createMonitor(t *testing.T) {
	mId := 12345
	expectedMonitor := genericMonitor(mId)

	dm := &datadoghqv1alpha1.DatadogMonitor{
		Spec: datadoghqv1alpha1.DatadogMonitorSpec{
			Query:   "avg(last_10m):avg:system.disk.in_use{*} by {host} > 0.05",
			Type:    "metric alert",
			Name:    "Test monitor",
			Message: "Something went wrong",
			Tags: []string{
				"env:staging",
				"kube_cluster:test.staging",
				"kube_namespace:test",
			},
		},
	}

	jsonMonitor, _ := expectedMonitor.MarshalJSON()
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(jsonMonitor)
	}))
	defer httpServer.Close()

	testConfig := datadogapiclientv1.NewConfiguration()
	testConfig.HTTPClient = httpServer.Client()
	client := datadogapiclientv1.NewAPIClient(testConfig)
	testAuth := setupTestAuth(httpServer.URL)

	monitor, err := createMonitor(testAuth, client, dm)
	assert.Nil(t, err)

	assert.Equal(t, dm.Spec.Query, *monitor.Query, "discrepancy found in parameter: Query")
	assert.Equal(t, string(dm.Spec.Type), string(*monitor.Type), "discrepancy found in parameter: Type")
	assert.Equal(t, dm.Spec.Name, *monitor.Name, "discrepancy found in parameter: Name")
	assert.Equal(t, dm.Spec.Message, *monitor.Message, "discrepancy found in parameter: Message")
	assert.Equal(t, dm.Spec.Tags, *monitor.Tags, "discrepancy found in parameter: Tags")
}

func Test_updateMonitor(t *testing.T) {
	mId := 12345
	expectedMonitor := genericMonitor(mId)

	dm := &datadoghqv1alpha1.DatadogMonitor{
		Spec: datadoghqv1alpha1.DatadogMonitorSpec{
			Query:   "avg(last_10m):avg:system.disk.in_use{*} by {host} > 0.05",
			Type:    "metric alert",
			Name:    "Test monitor",
			Message: "Something went wrong",
			Tags: []string{
				"env:staging",
				"kube_cluster:test.staging",
				"kube_namespace:test",
			},
		},
		Status: datadoghqv1alpha1.DatadogMonitorStatus{
			ID: mId,
		},
	}

	jsonMonitor, _ := expectedMonitor.MarshalJSON()
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(jsonMonitor)
	}))
	defer httpServer.Close()

	testConfig := datadogapiclientv1.NewConfiguration()
	testConfig.HTTPClient = httpServer.Client()
	client := datadogapiclientv1.NewAPIClient(testConfig)
	testAuth := setupTestAuth(httpServer.URL)

	monitor, err := updateMonitor(testAuth, client, dm)
	assert.Nil(t, err)

	assert.Equal(t, dm.Spec.Query, *monitor.Query, "discrepancy found in parameter: Query")
	assert.Equal(t, string(dm.Spec.Type), string(*monitor.Type), "discrepancy found in parameter: Type")
	assert.Equal(t, dm.Spec.Name, *monitor.Name, "discrepancy found in parameter: Name")
	assert.Equal(t, dm.Spec.Message, *monitor.Message, "discrepancy found in parameter: Message")
	assert.Equal(t, dm.Spec.Tags, *monitor.Tags, "discrepancy found in parameter: Tags")

}

func Test_deleteMonitor(t *testing.T) {
	mId := 12345

	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
	}))
	defer httpServer.Close()

	testConfig := datadogapiclientv1.NewConfiguration()
	testConfig.HTTPClient = httpServer.Client()
	client := datadogapiclientv1.NewAPIClient(testConfig)
	testAuth := setupTestAuth(httpServer.URL)

	err := deleteMonitor(testAuth, client, mId)
	assert.Nil(t, err)
}

func genericMonitor(mId int) datadogapiclientv1.Monitor {
	rawNow := time.Now()
	now, _ := time.Parse(dateFormat, strings.Split(rawNow.String(), " m=")[0])
	mId64 := int64(mId)
	msg := "Something went wrong"
	name := "Test monitor"
	handle := "test_user"
	query := "avg(last_10m):avg:system.disk.in_use{*} by {host} > 0.05"
	mType := datadogapiclientv1.MONITORTYPE_METRIC_ALERT
	tags := []string{
		"env:staging",
		"kube_cluster:test.staging",
		"kube_namespace:test",
	}
	return datadogapiclientv1.Monitor{
		Created: &now,
		Creator: &datadogapiclientv1.Creator{
			Handle: &handle,
		},
		Id:       &mId64,
		Message:  &msg,
		Modified: &now,
		Name:     &name,
		Query:    &query,
		Tags:     &tags,
		Type:     &mType,
	}
}

func setupTestAuth(apiURL string) context.Context {
	testAuth := context.WithValue(
		context.Background(),
		datadogapiclientv1.ContextAPIKeys,
		map[string]datadogapiclientv1.APIKey{
			"apiKeyAuth": {
				Key: "DUMMY_API_KEY",
			},
			"appKeyAuth": {
				Key: "DUMMY_APP_KEY",
			},
		},
	)
	parsedAPIURL, _ := url.Parse(apiURL)
	testAuth = context.WithValue(testAuth, datadogapiclientv1.ContextServerIndex, 1)
	testAuth = context.WithValue(testAuth, datadogapiclientv1.ContextServerVariables, map[string]string{
		"name":     parsedAPIURL.Host,
		"protocol": parsedAPIURL.Scheme,
	})

	return testAuth
}

func Test_translateClientError(t *testing.T) {

	testCases := []struct {
		name          string
		error         error
		message       string
		expectedError error
	}{
		{
			name:          "no message, generic error",
			error:         fmt.Errorf("generic error"),
			message:       "",
			expectedError: fmt.Errorf("an error occurred: generic error"),
		},
		{
			name:          "generic message, generic error",
			error:         fmt.Errorf("generic error"),
			message:       "generic message",
			expectedError: fmt.Errorf("generic message: generic error"),
		},
		{
			name:          "generic message, error type datadogapiclientv1.GenericOpenAPIError",
			error:         datadogapiclientv1.GenericOpenAPIError{},
			message:       "generic message",
			expectedError: fmt.Errorf("generic message: : "),
		},
		{
			name:          "generic message, error type *url.Error",
			error:         &url.Error{Err: fmt.Errorf("generic url error")},
			message:       "generic message",
			expectedError: fmt.Errorf("generic message (url.Error):  \"\": generic url error"),
		},
	}
	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			result := translateClientError(test.error, test.message)
			assert.Equal(t, test.expectedError, result)
		})
	}
}
