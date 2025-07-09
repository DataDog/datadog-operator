// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

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
	"k8s.io/utils/ptr"

	datadogapi "github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

const dateFormat = "2006-01-02 15:04:05.999999999 -0700 MST"

func Test_buildMonitor(t *testing.T) {
	dm := &datadoghqv1alpha1.DatadogMonitor{
		Spec: datadoghqv1alpha1.DatadogMonitorSpec{
			RestrictedRoles: []string{"an-admin-uuid"},
			Query:           "avg(last_10m):avg:system.disk.in_use{*} by {host} > 0.05",
			Type:            "metric alert",
			Name:            "Test monitor",
			Message:         "Something went wrong",
			Priority:        int64(3),
			Tags: []string{
				"env:staging",
				"kube_namespace:test",
				"kube_cluster:test.staging",
			},
			Options: datadoghqv1alpha1.DatadogMonitorOptions{
				EnableLogsSample:       ptr.To(true),
				EvaluationDelay:        ptr.To(int64(100)),
				EscalationMessage:      ptr.To("This is an escalation message"),
				IncludeTags:            ptr.To(true),
				GroupbySimpleMonitor:   ptr.To(true),
				Locked:                 ptr.To(true),
				NewGroupDelay:          ptr.To(int64(400)),
				NoDataTimeframe:        ptr.To(int64(15)),
				NotificationPresetName: "show_all",
				NotifyBy: []string{
					"env",
					"kube_namespace",
					"kube_cluster",
				},
				NotifyNoData:        ptr.To(true),
				OnMissingData:       "default",
				RenotifyOccurrences: ptr.To(int64(1)),
				RenotifyInterval:    ptr.To(int64(1440)),
				RenotifyStatuses: []datadogV1.MonitorRenotifyStatusType{
					datadogV1.MONITORRENOTIFYSTATUSTYPE_ALERT,
					datadogV1.MONITORRENOTIFYSTATUSTYPE_WARN,
				},
				SchedulingOptions: &datadoghqv1alpha1.DatadogMonitorOptionsSchedulingOptions{
					CustomSchedule: &datadoghqv1alpha1.DatadogMonitorOptionsSchedulingOptionsCustomSchedule{
						Recurrence: datadoghqv1alpha1.DatadogMonitorOptionsSchedulingOptionsCustomScheduleRecurrence{
							Rrule:    ptr.To("FREQ=MONTHLY;BYMONTHDAY=28,29,30,31;BYSETPOS=-1"),
							Timezone: ptr.To("Europe/Madrid"),
							Start:    ptr.To("2025-01-01T00:00:00"),
						},
					},
					EvaluationWindow: &datadoghqv1alpha1.DatadogMonitorOptionsSchedulingOptionsEvaluationWindow{
						DayStarts:   ptr.To("01:00"),
						HourStarts:  ptr.To(int32(2)),
						MonthStarts: ptr.To(int32(3)),
					},
				},
				TimeoutH: ptr.To(int64(2)),
				Thresholds: &datadoghqv1alpha1.DatadogMonitorOptionsThresholds{
					Critical: ptr.To("0.05"),
					Warning:  ptr.To("0.02"),
				},
			},
		},
	}

	monitor, monitorUR := buildMonitor(testLogger, dm)

	assert.Equal(t, dm.Spec.Query, monitor.GetQuery(), "discrepancy found in parameter: Query")
	assert.Equal(t, dm.Spec.Query, monitorUR.GetQuery(), "discrepancy found in parameter: Query")

	assert.Equal(t, string(dm.Spec.Type), string(monitor.GetType()), "discrepancy found in parameter: Type")
	assert.Equal(t, string(dm.Spec.Type), string(monitorUR.GetType()), "discrepancy found in parameter: Type")

	assert.Equal(t, dm.Spec.Name, monitor.GetName(), "discrepancy found in parameter: Name")
	assert.Equal(t, dm.Spec.Name, monitorUR.GetName(), "discrepancy found in parameter: Name")

	assert.Equal(t, dm.Spec.Message, monitor.GetMessage(), "discrepancy found in parameter: Message")
	assert.Equal(t, dm.Spec.Message, monitorUR.GetMessage(), "discrepancy found in parameter: Message")

	assert.Equal(t, dm.Spec.Priority, monitor.GetPriority(), "discrepancy found in parameter: Priority")
	assert.Equal(t, dm.Spec.Priority, monitorUR.GetPriority(), "discrepancy found in parameter: Priority")

	assert.Equal(t, dm.Spec.Tags, monitor.GetTags(), "discrepancy found in parameter: Tags")
	assert.Equal(t, dm.Spec.Tags, monitorUR.GetTags(), "discrepancy found in parameter: Tags")

	assert.Equal(t, *dm.Spec.Options.EnableLogsSample, monitor.Options.GetEnableLogsSample(), "discrepancy found in parameter: EnableLogsSample")
	assert.Equal(t, *dm.Spec.Options.EnableLogsSample, monitorUR.Options.GetEnableLogsSample(), "discrepancy found in parameter: EnableLogsSample")

	assert.Equal(t, *dm.Spec.Options.EvaluationDelay, monitor.Options.GetEvaluationDelay(), "discrepancy found in parameter: EvaluationDelay")
	assert.Equal(t, *dm.Spec.Options.EvaluationDelay, monitorUR.Options.GetEvaluationDelay(), "discrepancy found in parameter: EvaluationDelay")

	assert.Equal(t, *dm.Spec.Options.EscalationMessage, monitor.Options.GetEscalationMessage(), "discrepancy found in parameter: EscalationMessage")
	assert.Equal(t, *dm.Spec.Options.EscalationMessage, monitorUR.Options.GetEscalationMessage(), "discrepancy found in parameter: EscalationMessage")

	assert.Equal(t, *dm.Spec.Options.GroupbySimpleMonitor, monitor.Options.GetGroupbySimpleMonitor(), "discrepancy found in parameter: GroupbySimpleMonitor")
	assert.Equal(t, *dm.Spec.Options.GroupbySimpleMonitor, monitorUR.Options.GetGroupbySimpleMonitor(), "discrepancy found in parameter: GroupbySimpleMonitor")

	assert.Equal(t, *dm.Spec.Options.IncludeTags, monitor.Options.GetIncludeTags(), "discrepancy found in parameter: IncludeTags")
	assert.Equal(t, *dm.Spec.Options.IncludeTags, monitorUR.Options.GetIncludeTags(), "discrepancy found in parameter: IncludeTags")

	assert.Equal(t, *dm.Spec.Options.Locked, monitor.Options.GetLocked(), "discrepancy found in parameter: Locked")
	assert.Equal(t, *dm.Spec.Options.Locked, monitorUR.Options.GetLocked(), "discrepancy found in parameter: Locked")

	assert.Equal(t, *dm.Spec.Options.NewGroupDelay, monitor.Options.GetNewGroupDelay(), "discrepancy found in parameter: NewGroupDelay")
	assert.Equal(t, *dm.Spec.Options.NewGroupDelay, monitorUR.Options.GetNewGroupDelay(), "discrepancy found in parameter: NewGroupDelay")

	assert.Equal(t, *dm.Spec.Options.NoDataTimeframe, monitor.Options.GetNoDataTimeframe(), "discrepancy found in parameter: NoDataTimeframe")
	assert.Equal(t, *dm.Spec.Options.NoDataTimeframe, monitorUR.Options.GetNoDataTimeframe(), "discrepancy found in parameter: NoDataTimeframe")

	assert.Equal(t, string(dm.Spec.Options.NotificationPresetName), string(monitor.Options.GetNotificationPresetName()), "discrepancy found in parameter: NotificationPresetName")
	assert.Equal(t, string(dm.Spec.Options.NotificationPresetName), string(monitorUR.Options.GetNotificationPresetName()), "discrepancy found in parameter: NotificationPresetName")

	assert.Equal(t, dm.Spec.Options.NotifyBy, monitor.Options.GetNotifyBy(), "discrepancy found in parameter: NotifyBy")
	assert.Equal(t, dm.Spec.Options.NotifyBy, monitorUR.Options.GetNotifyBy(), "discrepancy found in parameter: NotifyBy")

	assert.Equal(t, *dm.Spec.Options.NotifyNoData, monitor.Options.GetNotifyNoData(), "discrepancy found in parameter: NotifyNoData")
	assert.Equal(t, *dm.Spec.Options.NotifyNoData, monitorUR.Options.GetNotifyNoData(), "discrepancy found in parameter: NotifyNoData")

	assert.Equal(t, string(dm.Spec.Options.OnMissingData), string(monitor.Options.GetOnMissingData()), "discrepancy found in parameter: OnMissingData")
	assert.Equal(t, string(dm.Spec.Options.OnMissingData), string(monitorUR.Options.GetOnMissingData()), "discrepancy found in parameter: OnMissingData")

	assert.Equal(t, *dm.Spec.Options.RenotifyInterval, monitor.Options.GetRenotifyInterval(), "discrepancy found in parameter: RenotifyInterval")
	assert.Equal(t, *dm.Spec.Options.RenotifyInterval, monitorUR.Options.GetRenotifyInterval(), "discrepancy found in parameter: RenotifyInterval")

	assert.Equal(t, *dm.Spec.Options.RenotifyOccurrences, monitor.Options.GetRenotifyOccurrences(), "discrepancy found in parameter: RenotifyOccurrences")
	assert.Equal(t, *dm.Spec.Options.RenotifyOccurrences, monitorUR.Options.GetRenotifyOccurrences(), "discrepancy found in parameter: RenotifyOccurrences")

	assert.Equal(t, dm.Spec.Options.RenotifyStatuses, monitor.Options.GetRenotifyStatuses(), "discrepancy found in parameter: RenotifyStatuses")
	assert.Equal(t, dm.Spec.Options.RenotifyStatuses, monitorUR.Options.GetRenotifyStatuses(), "discrepancy found in parameter: RenotifyStatuses")

	recurrences := monitor.Options.SchedulingOptions.CustomSchedule.GetRecurrences()
	var recurrence datadogV1.MonitorOptionsCustomScheduleRecurrence
	if assert.Len(t, recurrences, 1, "discrepancy found in parameter: CustomeSchedule.Recurrence") {
		recurrence = recurrences[0]
	}

	recurrencesUR := monitorUR.Options.SchedulingOptions.CustomSchedule.GetRecurrences()
	var recurrenceUR datadogV1.MonitorOptionsCustomScheduleRecurrence
	if assert.Len(t, recurrencesUR, 1, "discrepancy found in parameter: CustomeSchedule.Recurrence") {
		recurrenceUR = recurrencesUR[0]
	}

	assert.Equal(t, *dm.Spec.Options.SchedulingOptions.CustomSchedule.Recurrence.Rrule, recurrence.GetRrule(), "discrepancy found in parameter: Recurrence.Rrule")
	assert.Equal(t, *dm.Spec.Options.SchedulingOptions.CustomSchedule.Recurrence.Rrule, recurrenceUR.GetRrule(), "discrepancy found in parameter: Recurrence.Rrule")

	assert.Equal(t, *dm.Spec.Options.SchedulingOptions.CustomSchedule.Recurrence.Timezone, recurrence.GetTimezone(), "discrepancy found in parameter: Recurrence.Timezone")
	assert.Equal(t, *dm.Spec.Options.SchedulingOptions.CustomSchedule.Recurrence.Timezone, recurrenceUR.GetTimezone(), "discrepancy found in parameter: Recurrence.Timezone")

	assert.Equal(t, *dm.Spec.Options.SchedulingOptions.CustomSchedule.Recurrence.Start, recurrence.GetStart(), "discrepancy found in parameter: Recurrence.Start")
	assert.Equal(t, *dm.Spec.Options.SchedulingOptions.CustomSchedule.Recurrence.Start, recurrenceUR.GetStart(), "discrepancy found in parameter: Recurrence.Start")

	assert.Equal(t, *dm.Spec.Options.SchedulingOptions.EvaluationWindow.DayStarts, monitor.Options.SchedulingOptions.EvaluationWindow.GetDayStarts(), "discrepancy found in parameter: EvaluationWindow.DayStarts")
	assert.Equal(t, *dm.Spec.Options.SchedulingOptions.EvaluationWindow.DayStarts, monitorUR.Options.SchedulingOptions.EvaluationWindow.GetDayStarts(), "discrepancy found in parameter: EvaluationWindow.DayStarts")

	assert.Equal(t, *dm.Spec.Options.SchedulingOptions.EvaluationWindow.HourStarts, monitor.Options.SchedulingOptions.EvaluationWindow.GetHourStarts(), "discrepancy found in parameter: EvaluationWindow.HourStarts")
	assert.Equal(t, *dm.Spec.Options.SchedulingOptions.EvaluationWindow.HourStarts, monitorUR.Options.SchedulingOptions.EvaluationWindow.GetHourStarts(), "discrepancy found in parameter: EvaluationWindow.HourStarts")

	assert.Equal(t, *dm.Spec.Options.SchedulingOptions.EvaluationWindow.MonthStarts, monitor.Options.SchedulingOptions.EvaluationWindow.GetMonthStarts(), "discrepancy found in parameter: EvaluationWindow.MonthStarts")
	assert.Equal(t, *dm.Spec.Options.SchedulingOptions.EvaluationWindow.MonthStarts, monitorUR.Options.SchedulingOptions.EvaluationWindow.GetMonthStarts(), "discrepancy found in parameter: EvaluationWindow.MonthStarts")

	assert.Equal(t, *dm.Spec.Options.TimeoutH, monitor.Options.GetTimeoutH(), "discrepancy found in parameter: TimeoutH")
	assert.Equal(t, *dm.Spec.Options.TimeoutH, monitorUR.Options.GetTimeoutH(), "discrepancy found in parameter: TimeoutH")

	apiMonitorThresholds := monitor.Options.GetThresholds()
	apiMonitorURThresholds := monitorUR.Options.GetThresholds()
	warnVal, _ := strconv.ParseFloat(*dm.Spec.Options.Thresholds.Warning, 64)
	critVal, _ := strconv.ParseFloat(*dm.Spec.Options.Thresholds.Critical, 64)
	assert.Equal(t, warnVal, (&apiMonitorThresholds).GetWarning(), "discrepancy found in parameter: Threshold.Warning")
	assert.Equal(t, critVal, (&apiMonitorURThresholds).GetCritical(), "discrepancy found in parameter: Threshold.Critical")

	// Also make sure tags are sorted
	assert.Equal(t, "env:staging", (monitor.GetTags())[0], "tags are not properly sorted")
	assert.Equal(t, "kube_cluster:test.staging", (monitor.GetTags())[1], "tags are not properly sorted")
	assert.Equal(t, "kube_namespace:test", (monitor.GetTags())[2], "tags are not properly sorted")

	assert.Equal(t, "env:staging", (monitorUR.GetTags())[0], "tags are not properly sorted")
	assert.Equal(t, "kube_cluster:test.staging", (monitorUR.GetTags())[1], "tags are not properly sorted")
	assert.Equal(t, "kube_namespace:test", (monitorUR.GetTags())[2], "tags are not properly sorted")
}

func Test_getMonitor(t *testing.T) {
	mID := 12345
	expectedMonitor := genericMonitor(mID)
	jsonMonitor, _ := expectedMonitor.MarshalJSON()
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(jsonMonitor)
	}))
	defer httpServer.Close()

	testConfig := datadogapi.NewConfiguration()
	testConfig.HTTPClient = httpServer.Client()
	apiClient := datadogapi.NewAPIClient(testConfig)
	client := datadogV1.NewMonitorsApi(apiClient)
	testAuth := setupTestAuth(httpServer.URL)

	val, err := getMonitor(testAuth, client, mID)
	assert.Nil(t, err)
	assert.Equal(t, expectedMonitor, val)
}

func Test_validateMonitor(t *testing.T) {
	dm := genericDatadogMonitor()

	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
	}))
	defer httpServer.Close()

	testConfig := datadogapi.NewConfiguration()
	testConfig.HTTPClient = httpServer.Client()
	apiClient := datadogapi.NewAPIClient(testConfig)
	client := datadogV1.NewMonitorsApi(apiClient)
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

	testConfig := datadogapi.NewConfiguration()
	testConfig.HTTPClient = httpServer.Client()
	apiClient := datadogapi.NewAPIClient(testConfig)
	client := datadogV1.NewMonitorsApi(apiClient)
	testAuth := setupTestAuth(httpServer.URL)

	monitor, err := createMonitor(testAuth, testLogger, client, dm)
	assert.Nil(t, err)

	assert.Equal(t, dm.Spec.Query, monitor.GetQuery(), "discrepancy found in parameter: Query")
	assert.Equal(t, string(dm.Spec.Type), string(monitor.GetType()), "discrepancy found in parameter: Type")
	assert.Equal(t, dm.Spec.Name, monitor.GetName(), "discrepancy found in parameter: Name")
	assert.Equal(t, dm.Spec.Message, monitor.GetMessage(), "discrepancy found in parameter: Message")
	assert.Equal(t, dm.Spec.Tags, monitor.GetTags(), "discrepancy found in parameter: Tags")
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

	testConfig := datadogapi.NewConfiguration()
	testConfig.HTTPClient = httpServer.Client()
	apiClient := datadogapi.NewAPIClient(testConfig)
	client := datadogV1.NewMonitorsApi(apiClient)
	testAuth := setupTestAuth(httpServer.URL)

	monitor, err := updateMonitor(testAuth, testLogger, client, dm)
	assert.Nil(t, err)

	assert.Equal(t, dm.Spec.Query, monitor.GetQuery(), "discrepancy found in parameter: Query")
	assert.Equal(t, string(dm.Spec.Type), string(monitor.GetType()), "discrepancy found in parameter: Type")
	assert.Equal(t, dm.Spec.Name, monitor.GetName(), "discrepancy found in parameter: Name")
	assert.Equal(t, dm.Spec.Message, monitor.GetMessage(), "discrepancy found in parameter: Message")
	assert.Equal(t, dm.Spec.Tags, monitor.GetTags(), "discrepancy found in parameter: Tags")

}

func Test_deleteMonitor(t *testing.T) {
	mId := 12345

	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
	}))
	defer httpServer.Close()

	testConfig := datadogapi.NewConfiguration()
	testConfig.HTTPClient = httpServer.Client()
	apiClient := datadogapi.NewAPIClient(testConfig)
	client := datadogV1.NewMonitorsApi(apiClient)
	testAuth := setupTestAuth(httpServer.URL)

	err := deleteMonitor(testAuth, client, mId)
	assert.Nil(t, err)
}

func genericMonitor(mID int) datadogV1.Monitor {
	fakeRawNow := time.Unix(1612244495, 0)
	fakeNow, _ := time.Parse(dateFormat, strings.Split(fakeRawNow.String(), " m=")[0])
	mID64 := int64(mID)
	msg := "Something went wrong"
	name := "Test monitor"
	handle := "test_user"
	query := "avg(last_10m):avg:system.disk.in_use{*} by {host} > 0.05"
	mType := datadogV1.MONITORTYPE_METRIC_ALERT
	tags := []string{
		"env:staging",
		"kube_cluster:test.staging",
		"kube_namespace:test",
	}
	return datadogV1.Monitor{
		Created: &fakeNow,
		Creator: &datadogV1.Creator{
			Handle: &handle,
		},
		Id:       &mID64,
		Message:  &msg,
		Modified: &fakeNow,
		Name:     &name,
		Query:    query,
		Tags:     tags,
		Type:     mType,
	}
}

func setupTestAuth(apiURL string) context.Context {
	testAuth := context.WithValue(
		context.Background(),
		datadogapi.ContextAPIKeys,
		map[string]datadogapi.APIKey{
			"apiKeyAuth": {
				Key: "DUMMY_API_KEY",
			},
			"appKeyAuth": {
				Key: "DUMMY_APP_KEY",
			},
		},
	)
	parsedAPIURL, _ := url.Parse(apiURL)
	testAuth = context.WithValue(testAuth, datadogapi.ContextServerIndex, 1)
	testAuth = context.WithValue(testAuth, datadogapi.ContextServerVariables, map[string]string{
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
			name:                   "generic message, error type datadogV1.GenericOpenAPIError",
			error:                  datadogapi.GenericOpenAPIError{},
			message:                "generic message",
			expectedErrorInterface: &datadogapi.GenericOpenAPIError{},
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
