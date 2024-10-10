// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentprofile

import (
	"fmt"
	"os"

	"github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"

	"github.com/go-logr/logr"
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

func UpdateProfileStatus(logger logr.Logger, profile *datadoghqv1alpha1.DatadogAgentProfile, newStatus datadoghqv1alpha1.DatadogAgentProfileStatus, now metav1.Time) {
	if profile == nil || profile.Name == "" {
		logger.Error(fmt.Errorf("empty profile"), "Unable to update profile status")
		return
	}

	newStatus.LastUpdate = &now
	if newStatus.Valid == "" {
		newStatus.Valid = metav1.ConditionUnknown
	}
	if newStatus.Applied == "" {
		newStatus.Applied = metav1.ConditionUnknown
	}

	if os.Getenv(common.CreateStrategyEnabled) == "true" {
		if newStatus.CreateStrategy == nil {
			logger.Error(fmt.Errorf("new create strategy status empty"), "Unable to update profile status")
			return
		}
		if newStatus.CreateStrategy.Status == datadoghqv1alpha1.InProgressStatus {
			newStatus.CreateStrategy.Status = datadoghqv1alpha1.WaitingStatus
		}
		if profile.Status.CreateStrategy == nil || profile.Status.CreateStrategy.Status == "" || profile.Status.CreateStrategy.Status != newStatus.CreateStrategy.Status {
			newStatus.CreateStrategy.LastTransition = &now
		}
	} else {
		newStatus.CreateStrategy = nil
	}

	profile.Status = newStatus
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
