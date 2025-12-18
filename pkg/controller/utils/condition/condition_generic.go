// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package condition

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Type string

const (
	// DatadogConditionTypeCreated means the Datadog CRD is created successfully
	DatadogConditionTypeCreated Type = "Created"
	// DatadogConditionTypeActive means the Datadog CRD is active
	DatadogConditionTypeActive Type = "Active"
	// DatadogConditionTypeUpdated means the Datadog CRD is updated
	DatadogConditionTypeUpdated Type = "Updated"
	// DatadogConditionTypeError means the  Datadog CRD has error
	DatadogConditionTypeError Type = "Error"
)

// UpdateFailureStatusConditions is a generic method to update the failure StatusConditions.
func UpdateFailureStatusConditions(conditions *[]metav1.Condition, now metav1.Time, conditionType Type, reason string, err error) {
	UpdateStatusConditions(conditions, now, conditionType, metav1.ConditionTrue, reason, err.Error())
}

// UpdateStatusConditions is a generic method to update a specific condition type in the status conditions.
func UpdateStatusConditions(conditions *[]metav1.Condition, now metav1.Time, t Type, conditionStatus metav1.ConditionStatus, reason, msg string) {
	meta.SetStatusCondition(conditions, metav1.Condition{
		Type:               string(t),
		Status:             conditionStatus,
		LastTransitionTime: now,
		Reason:             reason,
		Message:            msg,
	})
}

// RemoveStatusCondition is a generic method/wrapper to remove a specific condition type in the status conditions.
func RemoveStatusCondition(conditions *[]metav1.Condition, conditionType Type) {
	meta.RemoveStatusCondition(conditions, string(conditionType))
}
