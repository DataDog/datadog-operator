// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentprofile

import (
	"testing"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

func TestSetDatadogAgentProfileCondition(t *testing.T) {
	now := metav1.Now()
	tests := []struct {
		name                   string
		existingConditionsList []metav1.Condition
		condition              metav1.Condition
		expectedConditionsList []metav1.Condition
	}{
		{
			name:                   "empty existingConditionsList, empty condition",
			existingConditionsList: []metav1.Condition{},
			condition:              metav1.Condition{},
			expectedConditionsList: []metav1.Condition{},
		},
		{
			name:                   "empty existingConditionsList, non-empty condition",
			existingConditionsList: []metav1.Condition{},
			condition: metav1.Condition{
				Type:               "foo-type",
				Status:             metav1.ConditionTrue,
				LastTransitionTime: now,
				Reason:             "foo-reason",
				Message:            "foo-message",
			},
			expectedConditionsList: []metav1.Condition{
				{
					Type:               "foo-type",
					Status:             metav1.ConditionTrue,
					LastTransitionTime: now,
					Reason:             "foo-reason",
					Message:            "foo-message",
				},
			},
		},
		{
			name: "non-empty existingConditionsList, non-empty condition, different types",
			existingConditionsList: []metav1.Condition{
				{
					Type:               "bar-type",
					Status:             metav1.ConditionFalse,
					LastTransitionTime: now,
					Reason:             "bar-reason",
					Message:            "bar-message",
				},
			},
			condition: metav1.Condition{
				Type:               "foo-type",
				Status:             metav1.ConditionTrue,
				LastTransitionTime: now,
				Reason:             "foo-reason",
				Message:            "foo-message",
			},
			expectedConditionsList: []metav1.Condition{
				{
					Type:               "bar-type",
					Status:             metav1.ConditionFalse,
					LastTransitionTime: now,
					Reason:             "bar-reason",
					Message:            "bar-message",
				},
				{
					Type:               "foo-type",
					Status:             metav1.ConditionTrue,
					LastTransitionTime: now,
					Reason:             "foo-reason",
					Message:            "foo-message",
				},
			},
		},
		{
			name: "non-empty existingConditionsList, non-empty condition, same types",
			existingConditionsList: []metav1.Condition{
				{
					Type:               "foo-type",
					Status:             metav1.ConditionTrue,
					LastTransitionTime: now,
					Reason:             "foo-reason",
					Message:            "foo-message",
				},
				{
					Type:               "bar-type",
					Status:             metav1.ConditionFalse,
					LastTransitionTime: now,
					Reason:             "bar-reason",
					Message:            "bar-message",
				},
			},
			condition: metav1.Condition{
				Type:               "foo-type",
				Status:             metav1.ConditionUnknown,
				LastTransitionTime: now,
				Reason:             "foo2-reason",
				Message:            "foo2-message",
			},
			expectedConditionsList: []metav1.Condition{
				{
					Type:               "foo-type",
					Status:             metav1.ConditionUnknown,
					LastTransitionTime: now,
					Reason:             "foo2-reason",
					Message:            "foo2-message",
				},
				{
					Type:               "bar-type",
					Status:             metav1.ConditionFalse,
					LastTransitionTime: now,
					Reason:             "bar-reason",
					Message:            "bar-message",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conditionsList := SetDatadogAgentProfileCondition(tt.existingConditionsList, tt.condition)
			assert.Equal(t, tt.expectedConditionsList, conditionsList)
		})
	}
}

func TestUpdateProfileStatus(t *testing.T) {
	now := metav1.Now()
	tests := []struct {
		name            string
		profile         *datadoghqv1alpha1.DatadogAgentProfile
		newStatus       datadoghqv1alpha1.DatadogAgentProfileStatus
		expectedProfile *datadoghqv1alpha1.DatadogAgentProfile
	}{
		{
			name:            "nil profile",
			profile:         nil,
			newStatus:       datadoghqv1alpha1.DatadogAgentProfileStatus{},
			expectedProfile: nil,
		},
		{
			name:    "empty profile, non-empty new status",
			profile: &datadoghqv1alpha1.DatadogAgentProfile{},
			newStatus: datadoghqv1alpha1.DatadogAgentProfileStatus{
				LastUpdate:  &metav1.Time{},
				CurrentHash: "foo",
				Conditions:  []metav1.Condition{},
				Valid:       metav1.ConditionFalse,
				Applied:     metav1.ConditionFalse,
			},
			expectedProfile: &datadoghqv1alpha1.DatadogAgentProfile{},
		},
		{
			name: "non-empty profile, empty new status",
			profile: &datadoghqv1alpha1.DatadogAgentProfile{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Status: datadoghqv1alpha1.DatadogAgentProfileStatus{
					LastUpdate:  &now,
					CurrentHash: "foo",
					Conditions:  []metav1.Condition{},
					Valid:       metav1.ConditionFalse,
					Applied:     metav1.ConditionFalse,
				},
			},
			newStatus: datadoghqv1alpha1.DatadogAgentProfileStatus{},
			expectedProfile: &datadoghqv1alpha1.DatadogAgentProfile{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Status: datadoghqv1alpha1.DatadogAgentProfileStatus{
					LastUpdate:  &now,
					CurrentHash: "",
					Conditions:  nil,
					Valid:       metav1.ConditionUnknown,
					Applied:     metav1.ConditionUnknown,
				},
			},
		},
		{
			name: "non-empty profile, non-empty new status",
			profile: &datadoghqv1alpha1.DatadogAgentProfile{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Status: datadoghqv1alpha1.DatadogAgentProfileStatus{
					LastUpdate:  &now,
					CurrentHash: "foo",
					Conditions:  []metav1.Condition{},
					Valid:       metav1.ConditionFalse,
					Applied:     metav1.ConditionFalse,
				},
			},
			newStatus: datadoghqv1alpha1.DatadogAgentProfileStatus{
				LastUpdate:  &metav1.Time{},
				CurrentHash: "bar",
				Conditions:  []metav1.Condition{},
				Valid:       metav1.ConditionFalse,
				Applied:     metav1.ConditionFalse,
			},
			expectedProfile: &datadoghqv1alpha1.DatadogAgentProfile{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Status: datadoghqv1alpha1.DatadogAgentProfileStatus{
					LastUpdate:  &now,
					CurrentHash: "bar",
					Conditions:  []metav1.Condition{},
					Valid:       metav1.ConditionFalse,
					Applied:     metav1.ConditionFalse,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := logf.Log.WithName("testUpdateProfileStatus")
			UpdateProfileStatus(logger, tt.profile, tt.newStatus, now)
			assert.Equal(t, tt.expectedProfile, tt.profile)
		})
	}
}
