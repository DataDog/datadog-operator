// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2021 Datadog, Inc.

package datadogmonitor

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	datadogapiclientv1 "github.com/DataDog/datadog-api-client-go/api/v1/datadog"
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/v1alpha1"
)

const dateFormat = "2006-01-02 15:04:05.999999999 -0700 MST"

func Test_buildMonitor(t *testing.T) {
	evalDelay := int64(100)
	escalationMsg := "This is an escalation message"
	valTrue := true
	newHostDelay := int64(400)
	noDataTimeframe := int64(15)
	renotifyInterval := int64(1440)
	timeoutH := int64(2)
	critThreshold := "0.05"
	warnThreshold := "0.02"

	dm := &datadoghqv1alpha1.DatadogMonitor{
		Spec: datadoghqv1alpha1.DatadogMonitorSpec{
			Query:    "avg(last_10m):avg:system.disk.in_use{*} by {host} > 0.05",
			Type:     "metric alert",
			Name:     "Test monitor",
			Message:  "Something went wrong",
			Priority: 3,
			Tags: []string{
				"env:staging",
				"kube_namespace:test",
				"kube_cluster:test.staging",
			},
			Options: datadoghqv1alpha1.DatadogMonitorOptions{
				EvaluationDelay:   &evalDelay,
				EscalationMessage: &escalationMsg,
				IncludeTags:       &valTrue,
				Locked:            &valTrue,
				NewHostDelay:      &newHostDelay,
				NotifyNoData:      &valTrue,
				NoDataTimeframe:   &noDataTimeframe,
				RenotifyInterval:  &renotifyInterval,
				TimeoutH:          &timeoutH,
				Thresholds: &datadoghqv1alpha1.DatadogMonitorOptionsThresholds{
					Critical: &critThreshold,
					Warning:  &warnThreshold,
				},
			},
		},
	}

	monitor, monitorUR := buildMonitor(testLogger, dm)

	assert.Equal(t, dm.Spec.Query, *monitor.Query, "discrepancy found in parameter: Query")
	assert.Equal(t, dm.Spec.Query, *monitorUR.Query, "discrepancy found in parameter: Query")

	assert.Equal(t, string(dm.Spec.Type), string(*monitor.Type), "discrepancy found in parameter: Type")
	assert.Equal(t, string(dm.Spec.Type), string(*monitorUR.Type), "discrepancy found in parameter: Type")

	assert.Equal(t, dm.Spec.Name, *monitor.Name, "discrepancy found in parameter: Name")
	assert.Equal(t, dm.Spec.Name, *monitorUR.Name, "discrepancy found in parameter: Name")

	assert.Equal(t, dm.Spec.Message, *monitor.Message, "discrepancy found in parameter: Message")
	assert.Equal(t, dm.Spec.Message, *monitorUR.Message, "discrepancy found in parameter: Message")

	assert.Equal(t, dm.Spec.Priority, *monitor.Priority, "discrepancy found in parameter: Priority")
	assert.Equal(t, dm.Spec.Priority, *monitorUR.Priority, "discrepancy found in parameter: Priority")

	assert.Equal(t, dm.Spec.Tags, *monitor.Tags, "discrepancy found in parameter: Tags")
	assert.Equal(t, dm.Spec.Tags, *monitorUR.Tags, "discrepancy found in parameter: Tags")

	assert.Equal(t, *dm.Spec.Options.EvaluationDelay, monitor.Options.GetEvaluationDelay(), "discrepancy found in parameter: EvaluationDelay")
	assert.Equal(t, *dm.Spec.Options.EvaluationDelay, monitorUR.Options.GetEvaluationDelay(), "discrepancy found in parameter: EvaluationDelay")

	assert.Equal(t, *dm.Spec.Options.EscalationMessage, monitor.Options.GetEscalationMessage(), "discrepancy found in parameter: EscalationMessage")
	assert.Equal(t, *dm.Spec.Options.EscalationMessage, monitorUR.Options.GetEscalationMessage(), "discrepancy found in parameter: EscalationMessage")

	assert.Equal(t, *dm.Spec.Options.IncludeTags, monitor.Options.GetIncludeTags(), "discrepancy found in parameter: IncludeTags")
	assert.Equal(t, *dm.Spec.Options.IncludeTags, monitorUR.Options.GetIncludeTags(), "discrepancy found in parameter: IncludeTags")

	assert.Equal(t, *dm.Spec.Options.Locked, monitor.Options.GetLocked(), "discrepancy found in parameter: Locked")
	assert.Equal(t, *dm.Spec.Options.Locked, monitorUR.Options.GetLocked(), "discrepancy found in parameter: Locked")

	assert.Equal(t, *dm.Spec.Options.NewHostDelay, monitor.Options.GetNewHostDelay(), "discrepancy found in parameter: NewHostDelay")
	assert.Equal(t, *dm.Spec.Options.NewHostDelay, monitorUR.Options.GetNewHostDelay(), "discrepancy found in parameter: NewHostDelay")

	assert.Equal(t, *dm.Spec.Options.NotifyNoData, monitor.Options.GetNotifyNoData(), "discrepancy found in parameter: NotifyNoData")
	assert.Equal(t, *dm.Spec.Options.NotifyNoData, monitorUR.Options.GetNotifyNoData(), "discrepancy found in parameter: NotifyNoData")

	assert.Equal(t, *dm.Spec.Options.NoDataTimeframe, monitor.Options.GetNoDataTimeframe(), "discrepancy found in parameter: NoDataTimeframe")
	assert.Equal(t, *dm.Spec.Options.NoDataTimeframe, monitorUR.Options.GetNoDataTimeframe(), "discrepancy found in parameter: NoDataTimeframe")

	assert.Equal(t, *dm.Spec.Options.RenotifyInterval, monitor.Options.GetRenotifyInterval(), "discrepancy found in parameter: RenotifyInterval")
	assert.Equal(t, *dm.Spec.Options.RenotifyInterval, monitorUR.Options.GetRenotifyInterval(), "discrepancy found in parameter: RenotifyInterval")

	assert.Equal(t, *dm.Spec.Options.TimeoutH, monitor.Options.GetTimeoutH(), "discrepancy found in parameter: TimeoutH")
	assert.Equal(t, *dm.Spec.Options.TimeoutH, monitorUR.Options.GetTimeoutH(), "discrepancy found in parameter: TimeoutH")

	apiMonitorThresholds := monitor.Options.GetThresholds()
	apiMonitorURThresholds := monitorUR.Options.GetThresholds()
	warnVal, _ := strconv.ParseFloat(*dm.Spec.Options.Thresholds.Warning, 64)
	critVal, _ := strconv.ParseFloat(*dm.Spec.Options.Thresholds.Critical, 64)
	assert.Equal(t, warnVal, (&apiMonitorThresholds).GetWarning(), "discrepancy found in parameter: Threshold.Warning")
	assert.Equal(t, critVal, (&apiMonitorURThresholds).GetCritical(), "discrepancy found in parameter: Threshold.Critical")

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

	err := validateMonitor(testAuth, testLogger, client, dm)
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

	monitor, err := createMonitor(testAuth, testLogger, client, dm)
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

	monitor, err := updateMonitor(testAuth, testLogger, client, dm)
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
	var ErrGeneric = errors.New("generic error")

	testCases := []struct {
		name                   string
		error                  error
		message                string
		expectedErrorType      error
		expectedError          error
		expectedErrorInterface interface{}
	}{
		{
			name:              "no message, generic error",
			error:             ErrGeneric,
			message:           "",
			expectedErrorType: ErrGeneric,
		},
		{
			name:              "generic message, generic error",
			error:             ErrGeneric,
			message:           "generic message",
			expectedErrorType: ErrGeneric,
		},
		{
			name:                   "generic message, error type datadogapiclientv1.GenericOpenAPIError",
			error:                  datadogapiclientv1.GenericOpenAPIError{},
			message:                "generic message",
			expectedErrorInterface: &datadogapiclientv1.GenericOpenAPIError{},
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

			if test.expectedErrorType != nil {
				assert.True(t, errors.Is(result, test.expectedErrorType))
			}

			if test.expectedErrorInterface != nil {
				assert.True(t, errors.As(result, test.expectedErrorInterface))
			}

			if test.expectedError != nil {
				assert.Equal(t, test.expectedError, result)
			}
		})
	}
}
