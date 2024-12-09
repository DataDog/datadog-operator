// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package condition

import (
	"fmt"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SetErrorActiveConditions sets the Error and Active DatadogMonitorConditionTypes to True or False
func SetErrorActiveConditions(status *datadoghqv1alpha1.DatadogMonitorStatus, now metav1.Time, err error) {
	if err != nil {
		// Set the error condition to True
		UpdateDatadogMonitorConditions(status, now, datadoghqv1alpha1.DatadogMonitorConditionTypeError, corev1.ConditionTrue, fmt.Sprintf("%v", err))
		// Set the active condition to False
		UpdateDatadogMonitorConditions(status, now, datadoghqv1alpha1.DatadogMonitorConditionTypeActive, corev1.ConditionFalse, "DatadogMonitor error")
	} else {
		// Set the error condition to False
		UpdateDatadogMonitorConditions(status, now, datadoghqv1alpha1.DatadogMonitorConditionTypeError, corev1.ConditionFalse, "")
		// Set the active condition to True
		UpdateDatadogMonitorConditions(status, now, datadoghqv1alpha1.DatadogMonitorConditionTypeActive, corev1.ConditionTrue, "DatadogMonitor ready")
	}
}

// UpdateDatadogMonitorConditions is used to update a DatadogMonitorConditionType in conditions
func UpdateDatadogMonitorConditions(status *datadoghqv1alpha1.DatadogMonitorStatus, now metav1.Time, t datadoghqv1alpha1.DatadogMonitorConditionType, conditionStatus corev1.ConditionStatus, desc string) {
	conditionIndex := getIndexForDatadogMonitorConditionType(status, t)
	// If condition type already exists, update it. Otherwise, create it (if the new condition status is True)
	if conditionIndex > -1 {
		SetDatadogMonitorCondition(&status.Conditions[conditionIndex], now, conditionStatus, desc)
	} else if conditionStatus == corev1.ConditionTrue {
		status.Conditions = append(status.Conditions, NewDatadogMonitorCondition(t, conditionStatus, now, "", desc))
	}
}

// SetDatadogMonitorCondition is used to set a specific DatadogMonitorConditionType
func SetDatadogMonitorCondition(condition *datadoghqv1alpha1.DatadogMonitorCondition, now metav1.Time, conditionStatus corev1.ConditionStatus, desc string) *datadoghqv1alpha1.DatadogMonitorCondition {
	if condition.Status != conditionStatus {
		condition.LastTransitionTime = now
		condition.Status = conditionStatus
		condition.LastUpdateTime = now
		condition.Message = desc
	} else if condition.Message != desc {
		condition.LastUpdateTime = now
		condition.Message = desc
	}

	return condition
}

// NewDatadogMonitorCondition returns a new DatadogMonitorCondition
func NewDatadogMonitorCondition(conditionType datadoghqv1alpha1.DatadogMonitorConditionType, conditionStatus corev1.ConditionStatus, now metav1.Time, reason, message string) datadoghqv1alpha1.DatadogMonitorCondition {
	return datadoghqv1alpha1.DatadogMonitorCondition{
		Type:               conditionType,
		Status:             conditionStatus,
		LastUpdateTime:     now,
		LastTransitionTime: now,
		Reason:             reason,
		Message:            message,
	}
}

func getIndexForDatadogMonitorConditionType(status *datadoghqv1alpha1.DatadogMonitorStatus, t datadoghqv1alpha1.DatadogMonitorConditionType) int {
	idx := -1
	if status == nil {
		return idx
	}

	for i, condition := range status.Conditions {
		if condition.Type == t {
			idx = i
			break
		}
	}

	return idx
}
