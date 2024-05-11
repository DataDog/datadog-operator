// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// Package conditions contains status conditions helpers.
package conditions

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	datadoghqv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
)

// NewExtendedDaemonSetCondition returns new ExtendedDaemonSetCondition instance.
func NewExtendedDaemonSetCondition(conditionType datadoghqv1alpha1.ExtendedDaemonSetConditionType, conditionStatus corev1.ConditionStatus, now metav1.Time, reason, message string, supportLastUpdate bool) datadoghqv1alpha1.ExtendedDaemonSetCondition {
	return datadoghqv1alpha1.ExtendedDaemonSetCondition{
		Type:               conditionType,
		Status:             conditionStatus,
		LastUpdateTime:     now,
		LastTransitionTime: now,
		Reason:             reason,
		Message:            message,
	}
}

// UpdateConditionOptions used to tune how te condition can be updated
// also if the condition doesn't exist yet, it will create it.
type UpdateConditionOptions struct {
	// IgnoreFalseConditionIfNotExist used to avoid creating the condition when this condition if false.
	// If `IgnoreFalseConditionIfNotExist == `true`, the condition will not be created if the status is equal to `corev1.ConditionFalse`
	IgnoreFalseConditionIfNotExist bool
	// SupportLastUpdate is an option to avoid updating the `LastUpdateTime` during every reconcile loop.
	// This option is useful when only `LastTransitionTime` is the important information.
	SupportLastUpdate bool
}

// UpdateExtendedDaemonSetStatusCondition used to update a specific ExtendedDaemonSetConditionType.
func UpdateExtendedDaemonSetStatusCondition(status *datadoghqv1alpha1.ExtendedDaemonSetStatus, now metav1.Time, t datadoghqv1alpha1.ExtendedDaemonSetConditionType, conditionStatus corev1.ConditionStatus, reason, desc string, options *UpdateConditionOptions) {
	// manage options
	var writeFalseIfNotExist, supportLastUpdate bool
	if options != nil {
		writeFalseIfNotExist = options.IgnoreFalseConditionIfNotExist
		supportLastUpdate = options.SupportLastUpdate
	}

	idCondition := getIndexForConditionType(status, t)
	if idCondition >= 0 {
		if status.Conditions[idCondition].Status != conditionStatus {
			status.Conditions[idCondition].LastTransitionTime = now
			status.Conditions[idCondition].Status = conditionStatus
			status.Conditions[idCondition].LastUpdateTime = now
		}
		if supportLastUpdate {
			status.Conditions[idCondition].LastUpdateTime = now
		}
		if conditionStatus == corev1.ConditionTrue {
			status.Conditions[idCondition].Message = desc
			status.Conditions[idCondition].Reason = reason
		}
	} else if conditionStatus == corev1.ConditionTrue || writeFalseIfNotExist {
		// Only add if the condition is True
		status.Conditions = append(status.Conditions, NewExtendedDaemonSetCondition(t, conditionStatus, now, reason, desc, supportLastUpdate))
	}
}

// UpdateErrorCondition used to update the ExtendedDaemonSet status error condition.
func UpdateErrorCondition(status *datadoghqv1alpha1.ExtendedDaemonSetStatus, now metav1.Time, err error, desc string) {
	options := UpdateConditionOptions{
		IgnoreFalseConditionIfNotExist: false,
		SupportLastUpdate:              true,
	}
	if err != nil {
		UpdateExtendedDaemonSetStatusCondition(status, now, datadoghqv1alpha1.ConditionTypeEDSReconcileError, corev1.ConditionTrue, "", desc, &options)
	} else {
		UpdateExtendedDaemonSetStatusCondition(status, now, datadoghqv1alpha1.ConditionTypeEDSReconcileError, corev1.ConditionFalse, "", desc, &options)
	}
}

func getIndexForConditionType(status *datadoghqv1alpha1.ExtendedDaemonSetStatus, t datadoghqv1alpha1.ExtendedDaemonSetConditionType) int {
	if status == nil {
		return -1
	}
	for i, condition := range status.Conditions {
		if condition.Type == t {
			return i
		}
	}

	return -1
}

// GetExtendedDaemonSetStatusCondition return the condition struct corresponding to the ExtendedDaemonSetConditionType provided in argument.
// return nil if not found.
func GetExtendedDaemonSetStatusCondition(status *datadoghqv1alpha1.ExtendedDaemonSetStatus, t datadoghqv1alpha1.ExtendedDaemonSetConditionType) *datadoghqv1alpha1.ExtendedDaemonSetCondition {
	idCondition := getIndexForConditionType(status, t)
	if idCondition == -1 {
		return nil
	}

	return &status.Conditions[idCondition]
}

// IsConditionTrue check if a condition is True. It not set return False.
func IsConditionTrue(status *datadoghqv1alpha1.ExtendedDaemonSetStatus, t datadoghqv1alpha1.ExtendedDaemonSetConditionType) bool {
	cond := GetExtendedDaemonSetStatusCondition(status, t)
	if cond != nil && cond.Status == corev1.ConditionTrue {
		return true
	}

	return false
}

// BoolToCondition convert bool to corev1.ConditionStatus.
func BoolToCondition(value bool) corev1.ConditionStatus {
	if value {
		return corev1.ConditionTrue
	}

	return corev1.ConditionFalse
}
