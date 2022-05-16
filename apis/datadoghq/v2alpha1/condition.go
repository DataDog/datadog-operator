package v2alpha1

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// UpdateDatadogAgentStatusConditionsFailure used to update the failure StatusConditions
func UpdateDatadogAgentStatusConditionsFailure(status *DatadogAgentStatus, now metav1.Time, conditionType, reason, message string, err error) {
	if err != nil {
		UpdateDatadogAgentStatusConditions(status, now, conditionType, metav1.ConditionTrue, reason, fmt.Sprintf("msg:%s, err:%v", message, err), true)
	} else {
		UpdateDatadogAgentStatusConditions(status, now, conditionType, metav1.ConditionFalse, reason, message, true)
	}
}

// UpdateDatadogAgentStatusConditions used to update a specific string in conditions
func UpdateDatadogAgentStatusConditions(status *DatadogAgentStatus, now metav1.Time, t string, conditionStatus metav1.ConditionStatus, reason, message string, writeFalseIfNotExist bool) {
	idConditionComplete := getIndexForConditionType(status, t)
	if idConditionComplete >= 0 {
		UpdateDatadogAgentStatusCondition(status.Conditions[idConditionComplete], now, t, conditionStatus, reason, message)
	} else if conditionStatus == metav1.ConditionTrue || writeFalseIfNotExist {
		// Only add if the condition is True
		status.Conditions = append(status.Conditions, NewDatadogAgentStatusCondition(t, conditionStatus, now, reason, message))
	}
}

// UpdateDatadogAgentStatusCondition used to update a specific string
func UpdateDatadogAgentStatusCondition(condition metav1.Condition, now metav1.Time, t string, conditionStatus metav1.ConditionStatus, reason, message string) metav1.Condition {
	if condition.Status != conditionStatus {
		condition.LastTransitionTime = now
		condition.Status = conditionStatus
	}
	condition.LastTransitionTime = now
	condition.Message = message
	condition.Reason = reason

	return condition
}

// SetDatadogAgentStatusCondition use to set a condition
func SetDatadogAgentStatusCondition(status *DatadogAgentStatus, condition *metav1.Condition) {
	idConditionComplete := getIndexForConditionType(status, condition.Type)
	if idConditionComplete >= 0 {
		status.Conditions[idConditionComplete] = *condition
	} else {
		status.Conditions = append(status.Conditions, *condition)
	}
}

// NewDatadogAgentStatusCondition returns new DatadogAgentCondition instance
func NewDatadogAgentStatusCondition(conditionType string, conditionStatus metav1.ConditionStatus, now metav1.Time, reason, message string) metav1.Condition {
	return metav1.Condition{
		Type:               conditionType,
		Status:             conditionStatus,
		LastTransitionTime: now,
		Reason:             reason,
		Message:            message,
	}
}

func getIndexForConditionType(status *DatadogAgentStatus, t string) int {
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
