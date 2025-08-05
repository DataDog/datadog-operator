// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogmonitor

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"

	datadogapi "github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	"github.com/go-logr/logr"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

func buildMonitor(logger logr.Logger, dm *datadoghqv1alpha1.DatadogMonitor) (*datadogV1.Monitor, *datadogV1.MonitorUpdateRequest) {
	monitorType := datadogV1.MonitorType(dm.Spec.Type)
	name := dm.Spec.Name
	msg := dm.Spec.Message
	priority := dm.Spec.Priority
	query := dm.Spec.Query
	restrictedRoles := dm.Spec.RestrictedRoles
	options := dm.Spec.Options

	var (
		thresholds       datadogV1.MonitorThresholds
		thresholdWindows datadogV1.MonitorThresholdWindowOptions
		t                float64
		err              error
	)

	if options.Thresholds != nil {
		if options.Thresholds.OK != nil {
			if t, err = strconv.ParseFloat(*options.Thresholds.OK, 64); err == nil {
				thresholds.SetOk(t)
			} else {
				logger.Error(err, "error parsing OK threshold value")
			}
		}
		if options.Thresholds.Warning != nil {
			if t, err = strconv.ParseFloat(*options.Thresholds.Warning, 64); err == nil {
				thresholds.SetWarning(t)
			} else {
				logger.Error(err, "error parsing Warning threshold value")
			}
		}
		if options.Thresholds.Unknown != nil {
			if t, err = strconv.ParseFloat(*options.Thresholds.Unknown, 64); err == nil {
				thresholds.SetUnknown(t)
			} else {
				logger.Error(err, "error parsing Unknown threshold value")
			}
		}
		if options.Thresholds.Critical != nil {
			if t, err = strconv.ParseFloat(*options.Thresholds.Critical, 64); err == nil {
				thresholds.SetCritical(t)
			} else {
				logger.Error(err, "error parsing Critical threshold value")
			}
		}
		if options.Thresholds.WarningRecovery != nil {
			if t, err = strconv.ParseFloat(*options.Thresholds.WarningRecovery, 64); err == nil {
				thresholds.SetWarningRecovery(t)
			} else {
				logger.Error(err, "error parsing WarningRecovery threshold value")
			}
		}
		if options.Thresholds.CriticalRecovery != nil {
			if t, err = strconv.ParseFloat(*options.Thresholds.CriticalRecovery, 64); err == nil {
				thresholds.SetCriticalRecovery(t)
			} else {
				logger.Error(err, "error parsing CriticalRecovery threshold value")
			}
		}
	}

	if options.ThresholdWindows != nil {
		if options.ThresholdWindows.RecoveryWindow != nil {
			thresholdWindows.SetRecoveryWindow(*options.ThresholdWindows.RecoveryWindow)
		}
		if options.ThresholdWindows.TriggerWindow != nil {
			thresholdWindows.SetTriggerWindow(*options.ThresholdWindows.TriggerWindow)
		}
	}

	o := datadogV1.MonitorOptions{}
	o.SetThresholds(thresholds)

	if thresholdWindows.HasRecoveryWindow() || thresholdWindows.HasTriggerWindow() {
		o.SetThresholdWindows(thresholdWindows)
	}

	if options.EscalationMessage != nil {
		o.SetEscalationMessage(*options.EscalationMessage)
	}

	if options.EvaluationDelay != nil {
		o.SetEvaluationDelay(*options.EvaluationDelay)
	}

	if options.GroupbySimpleMonitor != nil {
		o.SetGroupbySimpleMonitor(*options.GroupbySimpleMonitor)
	}

	if options.IncludeTags != nil {
		o.SetIncludeTags(*options.IncludeTags)
	}

	if options.Locked != nil {
		o.SetLocked(*options.Locked)
	}

	if options.NewGroupDelay != nil {
		o.SetNewGroupDelay(*options.NewGroupDelay)
	}

	if options.EnableLogsSample != nil {
		o.SetEnableLogsSample(*options.EnableLogsSample)
	}

	if options.NoDataTimeframe != nil {
		o.SetNoDataTimeframe(*options.NoDataTimeframe)
	}

	if options.NotifyAudit != nil {
		o.SetNotifyAudit(*options.NotifyAudit)
	}

	if options.NotifyBy != nil {
		o.SetNotifyBy(options.NotifyBy)
	}

	if options.NotifyNoData != nil {
		o.SetNotifyNoData(*options.NotifyNoData)
	}

	if options.RequireFullWindow != nil {
		o.SetRequireFullWindow(*options.RequireFullWindow)
	}

	if options.RenotifyInterval != nil {
		o.SetRenotifyInterval(*options.RenotifyInterval)
	}

	if options.RenotifyOccurrences != nil {
		o.SetRenotifyOccurrences(*options.RenotifyOccurrences)
	}

	if options.RenotifyStatuses != nil {
		o.SetRenotifyStatuses(options.RenotifyStatuses)
	}

	if so := options.SchedulingOptions; so != nil {
		sops := datadogV1.MonitorOptionsSchedulingOptions{}

		if cs := options.SchedulingOptions.CustomSchedule; cs != nil {
			csrecurrence := datadogV1.MonitorOptionsCustomScheduleRecurrence{}
			if cs.Recurrence.Rrule != nil {
				csrecurrence.Rrule = cs.Recurrence.Rrule
			}
			if cs.Recurrence.Timezone != nil {
				csrecurrence.Timezone = cs.Recurrence.Timezone
			}
			if cs.Recurrence.Start != nil {
				csrecurrence.Start = cs.Recurrence.Start
			}

			csops := datadogV1.MonitorOptionsCustomSchedule{
				Recurrences: []datadogV1.MonitorOptionsCustomScheduleRecurrence{csrecurrence},
			}

			sops.SetCustomSchedule(csops)
		}

		if ew := options.SchedulingOptions.EvaluationWindow; ew != nil {
			ewops := datadogV1.MonitorOptionsSchedulingOptionsEvaluationWindow{}
			if ew.DayStarts != nil {
				ewops.DayStarts = ew.DayStarts
			}
			if ew.HourStarts != nil {
				ewops.HourStarts = ew.HourStarts
			}
			if ew.MonthStarts != nil {
				ewops.MonthStarts = ew.MonthStarts
			}
			sops.SetEvaluationWindow(ewops)
		}
		o.SetSchedulingOptions(sops)
	}

	if options.TimeoutH != nil {
		o.SetTimeoutH(*options.TimeoutH)
	}

	if options.NotificationPresetName != "" {
		o.SetNotificationPresetName(datadogV1.MonitorOptionsNotificationPresets(options.NotificationPresetName))
	}

	if options.OnMissingData != "" {
		o.SetOnMissingData(datadogV1.OnMissingDataOption(options.OnMissingData))
	}

	m := datadogV1.NewMonitor(query, monitorType)
	{
		m.SetName(name)
		m.SetMessage(msg)
		m.SetPriority(priority)
		m.SetOptions(o)
	}

	u := datadogV1.NewMonitorUpdateRequest()
	{
		u.SetType(monitorType)
		u.SetName(name)
		u.SetMessage(msg)
		u.SetPriority(priority)
		u.SetQuery(query)
		u.SetRestrictedRoles(restrictedRoles)
		u.SetOptions(o)
	}

	tags := dm.Spec.Tags
	sort.Strings(tags)
	m.SetTags(tags)
	u.SetTags(tags)

	return m, u
}

func getMonitor(auth context.Context, client *datadogV1.MonitorsApi, monitorID int) (datadogV1.Monitor, error) {
	groupStates := "all"
	optionalParams := datadogV1.GetMonitorOptionalParameters{
		GroupStates: &groupStates,
	}
	m, _, err := client.GetMonitor(auth, int64(monitorID), optionalParams)
	if err != nil {
		return datadogV1.Monitor{}, translateClientError(err, "error getting monitor")
	}

	return m, nil
}

func validateMonitor(auth context.Context, logger logr.Logger, client *datadogV1.MonitorsApi, dm *datadoghqv1alpha1.DatadogMonitor) error {
	m, _ := buildMonitor(logger, dm)
	if _, _, err := client.ValidateMonitor(auth, *m); err != nil {
		return translateClientError(err, "error validating monitor")
	}

	return nil
}

func createMonitor(auth context.Context, logger logr.Logger, client *datadogV1.MonitorsApi, dm *datadoghqv1alpha1.DatadogMonitor) (datadogV1.Monitor, error) {
	m, _ := buildMonitor(logger, dm)
	mCreated, _, err := client.CreateMonitor(auth, *m)
	if err != nil {
		return datadogV1.Monitor{}, translateClientError(err, "error creating monitor")
	}

	return mCreated, nil
}

func updateMonitor(auth context.Context, logger logr.Logger, client *datadogV1.MonitorsApi, dm *datadoghqv1alpha1.DatadogMonitor) (datadogV1.Monitor, error) {
	_, u := buildMonitor(logger, dm)

	mUpdated, _, err := client.UpdateMonitor(auth, int64(dm.Status.ID), *u)
	if err != nil {
		return datadogV1.Monitor{}, translateClientError(err, "error updating monitor")
	}

	// TODO additional logic to handle downtimes (and silenced param if needed)

	return mUpdated, nil
}

func deleteMonitor(auth context.Context, client *datadogV1.MonitorsApi, monitorID int) error {
	force := "false"
	optionalParams := datadogV1.DeleteMonitorOptionalParameters{
		Force: &force,
	}
	if _, _, err := client.DeleteMonitor(auth, int64(monitorID), optionalParams); err != nil {
		return translateClientError(err, "error deleting monitor")
	}

	return nil
}

func translateClientError(err error, msg string) error {
	if msg == "" {
		msg = "an error occurred"
	}

	var apiErr datadogapi.GenericOpenAPIError
	var errURL *url.Error
	if errors.As(err, &apiErr) {
		return fmt.Errorf(msg+": %w: %s", err, apiErr.Body())
	}

	if errors.As(err, &errURL) {
		return fmt.Errorf(msg+" (url.Error): %s", errURL)
	}

	return fmt.Errorf(msg+": %w", err)
}
