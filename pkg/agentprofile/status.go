// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentprofile

import (
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// ValidConditionType is a type of condition for a DatadogAgentProfile
	ValidConditionType = "Valid"
	// ValidConditionType is a type of condition for a DatadogAgentProfile
	AppliedConditionType = "Applied"

	// ValidConditionReason is for DatadogAgentProfiles with a valid manifest
	ValidConditionReason = "Valid"
	// InvalidConditionReason is for DatadogAgentProfiles with an invalid manifest
	InvalidConditionReason = "Invalid"
	// AppliedConditionReason is for DatadogAgentProfiles that are applied to at least one node
	AppliedConditionReason = "Applied"
	// ConflictConditionReason is for DatadogAgentProfiles that conflict with an existing DatadogAgentProfile
	ConflictConditionReason = "Conflict"
)

func UpdateProfileStatus(profile *datadoghqv1alpha1.DatadogAgentProfile, profileStatus datadoghqv1alpha1.DatadogAgentProfileStatus, now metav1.Time) {
	if profile == nil {
		profile.Status = datadoghqv1alpha1.DatadogAgentProfileStatus{
			LastUpdate:  &now,
			CurrentHash: "",
			Conditions: []metav1.Condition{
				NewDatadogAgentProfileCondition(ValidConditionType, metav1.ConditionFalse, now, InvalidConditionReason, "Profile is empty"),
			},
		}
		return
	}

	profileStatus.LastUpdate = &now
	if profileStatus.Valid == "" {
		profileStatus.Valid = metav1.ConditionUnknown
	}
	if profileStatus.Applied == "" {
		profileStatus.Applied = metav1.ConditionUnknown
	}

	profile.Status = profileStatus
}

// NewDatadogAgentProfileCondition returns a new metav1.Condition instance
func NewDatadogAgentProfileCondition(conditionType string, conditionStatus metav1.ConditionStatus, now metav1.Time, reason, message string) metav1.Condition {
	return metav1.Condition{
		Type:               conditionType,
		Status:             conditionStatus,
		LastTransitionTime: now,
		Reason:             reason,
		Message:            message,
	}
}

// SetDatadogAgentProfileCondition is used to update a condition
func SetDatadogAgentProfileCondition(conditionsList []metav1.Condition, newCondition metav1.Condition) []metav1.Condition {
	if newCondition.Type == "" {
		return conditionsList
	}

	found := false
	for i, condition := range conditionsList {
		if newCondition.Type == condition.Type {
			found = true
			conditionsList[i] = newCondition
		}
	}

	if !found {
		conditionsList = append(conditionsList, newCondition)
	}

	return conditionsList
}
