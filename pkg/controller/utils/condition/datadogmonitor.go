// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package condition

import (
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// UpdateDatadogMonitorConditions used to update a DatadogMonitorConditionType in conditions
func UpdateDatadogMonitorConditions(status *datadoghqv1alpha1.DatadogMonitorStatus, now metav1.Time, t datadoghqv1alpha1.DatadogMonitorConditionType, conditionStatus corev1.ConditionStatus, desc string) {
	conditionIndex := getIndexForDatadogMonitorConditionType(status, t)
	// If condition type already exists, update it. Otherwise, create it (if the new condition status is True)
	if conditionIndex > -1 {
		SetDatadogMonitorCondition(&status.Conditions[conditionIndex], now, t, conditionStatus, desc)
	} else if conditionStatus == corev1.ConditionTrue {
		status.Conditions = append(status.Conditions, NewDatadogMonitorCondition(t, conditionStatus, now, "", desc))
	}
}

// SetDatadogMonitorCondition used to set a specific DatadogMonitorConditionType
func SetDatadogMonitorCondition(condition *datadoghqv1alpha1.DatadogMonitorCondition, now metav1.Time, t datadoghqv1alpha1.DatadogMonitorConditionType, conditionStatus corev1.ConditionStatus, desc string) *datadoghqv1alpha1.DatadogMonitorCondition {
	if condition.Status != conditionStatus {
		condition.LastTransitionTime = now
		condition.Status = conditionStatus
	}
	condition.LastUpdateTime = now
	condition.Message = desc
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
