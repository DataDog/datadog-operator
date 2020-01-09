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
		UpdateDatadogAgentStatusCondition(status, now, conditionType, corev1.ConditionTrue, fmt.Sprintf("%v", err), false)
	} else {
		UpdateDatadogAgentStatusCondition(status, now, conditionType, corev1.ConditionFalse, "", false)
	}
}

// UpdateDatadogAgentStatusCondition used to update a specific DatadogAgentConditionType
func UpdateDatadogAgentStatusCondition(status *datadoghqv1alpha1.DatadogAgentStatus, now metav1.Time, t datadoghqv1alpha1.DatadogAgentConditionType, conditionStatus corev1.ConditionStatus, desc string, writeFalseIfNotExist bool) {
	idConditionComplete := getIndexForConditionType(status, t)
	if idConditionComplete >= 0 {
		if status.Conditions[idConditionComplete].Status != conditionStatus {
			status.Conditions[idConditionComplete].LastTransitionTime = now
			status.Conditions[idConditionComplete].Status = conditionStatus
		}
		status.Conditions[idConditionComplete].LastUpdateTime = now
		status.Conditions[idConditionComplete].Message = desc
	} else if conditionStatus == corev1.ConditionTrue || writeFalseIfNotExist {
		// Only add if the condition is True
		status.Conditions = append(status.Conditions, NewDatadogAgentStatusCondition(t, conditionStatus, now, "", desc))
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
