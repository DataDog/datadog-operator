// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package condition

import (
	"fmt"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/pkg/apis/datadoghq/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// UpdateDatadogAgentStatusConditionsFailure used to update the failure StatusConditions
func UpdateDatadogAgentStatusConditionsFailure(status *datadoghqv1alpha1.DatadogAgentStatus, now metav1.Time, conditionType datadoghqv1alpha1.DatadogAgentConditionType, err error) {
	if err != nil {
		UpdateDatadogAgentStatusConditions(status, now, conditionType, corev1.ConditionTrue, fmt.Sprintf("%v", err), false)
	} else {
		UpdateDatadogAgentStatusConditions(status, now, conditionType, corev1.ConditionFalse, "", false)
	}
}

// UpdateDatadogAgentStatusConditions used to update a specific DatadogAgentConditionType in conditions
func UpdateDatadogAgentStatusConditions(status *datadoghqv1alpha1.DatadogAgentStatus, now metav1.Time, t datadoghqv1alpha1.DatadogAgentConditionType, conditionStatus corev1.ConditionStatus, desc string, writeFalseIfNotExist bool) {
	idConditionComplete := getIndexForConditionType(status, t)
	if idConditionComplete >= 0 {
		UpdateDatadogAgentStatusCondition(&status.Conditions[idConditionComplete], now, t, conditionStatus, desc)
	} else if conditionStatus == corev1.ConditionTrue || writeFalseIfNotExist {
		// Only add if the condition is True
		status.Conditions = append(status.Conditions, NewDatadogAgentStatusCondition(t, conditionStatus, now, "", desc))
	}
}

// UpdateDatadogAgentStatusCondition used to update a specific DatadogAgentConditionType
func UpdateDatadogAgentStatusCondition(condition *datadoghqv1alpha1.DatadogAgentCondition, now metav1.Time, t datadoghqv1alpha1.DatadogAgentConditionType, conditionStatus corev1.ConditionStatus, desc string) *datadoghqv1alpha1.DatadogAgentCondition {
	if condition.Status != conditionStatus {
		condition.LastTransitionTime = now
		condition.Status = conditionStatus
	}
	condition.LastUpdateTime = now
	condition.Message = desc
	return condition
}

// SetDatadogAgentStatusCondition use to set a condition
func SetDatadogAgentStatusCondition(status *datadoghqv1alpha1.DatadogAgentStatus, condition *datadoghqv1alpha1.DatadogAgentCondition) {
	idConditionComplete := getIndexForConditionType(status, condition.Type)
	if idConditionComplete >= 0 {
		status.Conditions[idConditionComplete] = *condition
	} else {
		status.Conditions = append(status.Conditions, *condition)
	}
}

// NewDatadogAgentStatusCondition returns new DatadogAgentCondition instance
func NewDatadogAgentStatusCondition(conditionType datadoghqv1alpha1.DatadogAgentConditionType, conditionStatus corev1.ConditionStatus, now metav1.Time, reason, message string) datadoghqv1alpha1.DatadogAgentCondition {
	return datadoghqv1alpha1.DatadogAgentCondition{
		Type:               conditionType,
		Status:             conditionStatus,
		LastUpdateTime:     now,
		LastTransitionTime: now,
		Reason:             reason,
		Message:            message,
	}
}

func getIndexForConditionType(status *datadoghqv1alpha1.DatadogAgentStatus, t datadoghqv1alpha1.DatadogAgentConditionType) int {
	idCondition := -1
	if status == nil {
		return idCondition
	}
	for i, condition := range status.Conditions {
		if condition.Type == t {
			idCondition = i
			break
		}
	}
	return idCondition
}
