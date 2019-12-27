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

// UpdateDatadogAgentDeploymentStatusConditionsFailure used to update the failure StatusConditions
func UpdateDatadogAgentDeploymentStatusConditionsFailure(status *datadoghqv1alpha1.DatadogAgentDeploymentStatus, now metav1.Time, conditionType datadoghqv1alpha1.DatadogAgentDeploymentConditionType, err error) {
	if err != nil {
		UpdateDatadogAgentDeploymentStatusCondition(status, now, conditionType, corev1.ConditionTrue, fmt.Sprintf("%v", err), false)
	} else {
		UpdateDatadogAgentDeploymentStatusCondition(status, now, conditionType, corev1.ConditionFalse, "", false)
	}
}

// UpdateDatadogAgentDeploymentStatusCondition used to update a specific DatadogAgentDeploymentConditionType
func UpdateDatadogAgentDeploymentStatusCondition(status *datadoghqv1alpha1.DatadogAgentDeploymentStatus, now metav1.Time, t datadoghqv1alpha1.DatadogAgentDeploymentConditionType, conditionStatus corev1.ConditionStatus, desc string, writeFalseIfNotExist bool) {
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
		status.Conditions = append(status.Conditions, NewDatadogAgentDeploymentStatusCondition(t, conditionStatus, now, "", desc))
	}
}

// NewDatadogAgentDeploymentStatusCondition returns new DatadogAgentDeploymentCondition instance
func NewDatadogAgentDeploymentStatusCondition(conditionType datadoghqv1alpha1.DatadogAgentDeploymentConditionType, conditionStatus corev1.ConditionStatus, now metav1.Time, reason, message string) datadoghqv1alpha1.DatadogAgentDeploymentCondition {
	return datadoghqv1alpha1.DatadogAgentDeploymentCondition{
		Type:               conditionType,
		Status:             conditionStatus,
		LastUpdateTime:     now,
		LastTransitionTime: now,
		Reason:             reason,
		Message:            message,
	}
}

func getIndexForConditionType(status *datadoghqv1alpha1.DatadogAgentDeploymentStatus, t datadoghqv1alpha1.DatadogAgentDeploymentConditionType) int {
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
