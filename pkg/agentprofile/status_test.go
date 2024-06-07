// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentprofile

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
