// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentprofile

import (
	"testing"

	"github.com/DataDog/datadog-operator/apis/datadoghq/common"
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"

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
		slowStart       string
		expectedProfile *datadoghqv1alpha1.DatadogAgentProfile
	}{
		{
			name:            "nil profile, slow start false",
			profile:         nil,
			newStatus:       datadoghqv1alpha1.DatadogAgentProfileStatus{},
			slowStart:       "false",
			expectedProfile: nil,
		},
		{
			name:            "nil profile, slow start true",
			profile:         nil,
			newStatus:       datadoghqv1alpha1.DatadogAgentProfileStatus{},
			slowStart:       "true",
			expectedProfile: nil,
		},
		{
			name:    "empty profile, non-empty new status, slow start false",
			profile: &datadoghqv1alpha1.DatadogAgentProfile{},
			newStatus: datadoghqv1alpha1.DatadogAgentProfileStatus{
				LastUpdate:  &metav1.Time{},
				CurrentHash: "foo",
				Conditions:  []metav1.Condition{},
				Valid:       metav1.ConditionFalse,
				Applied:     metav1.ConditionFalse,
				SlowStart: &datadoghqv1alpha1.SlowStart{
					Status: datadoghqv1alpha1.CompletedStatus,
				},
			},
			slowStart:       "false",
			expectedProfile: &datadoghqv1alpha1.DatadogAgentProfile{},
		},
		{
			name:    "empty profile, non-empty new status, slow start true",
			profile: &datadoghqv1alpha1.DatadogAgentProfile{},
			newStatus: datadoghqv1alpha1.DatadogAgentProfileStatus{
				LastUpdate:  &metav1.Time{},
				CurrentHash: "foo",
				Conditions:  []metav1.Condition{},
				Valid:       metav1.ConditionFalse,
				Applied:     metav1.ConditionFalse,
				SlowStart: &datadoghqv1alpha1.SlowStart{
					Status: datadoghqv1alpha1.CompletedStatus,
				},
			},
			slowStart:       "true",
			expectedProfile: &datadoghqv1alpha1.DatadogAgentProfile{},
		},
		{
			name: "non-empty profile, empty new status, slow start false",
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
					SlowStart: &datadoghqv1alpha1.SlowStart{
						Status:         datadoghqv1alpha1.CompletedStatus,
						LastTransition: &now,
					},
				},
			},
			newStatus: datadoghqv1alpha1.DatadogAgentProfileStatus{},
			slowStart: "false",
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
					SlowStart:   nil,
				},
			},
		},
		{
			name: "non-empty profile, empty new status, slow start true",
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
					SlowStart: &datadoghqv1alpha1.SlowStart{
						Status:         datadoghqv1alpha1.CompletedStatus,
						LastTransition: &now,
					},
				},
			},
			newStatus: datadoghqv1alpha1.DatadogAgentProfileStatus{},
			slowStart: "true",
			expectedProfile: &datadoghqv1alpha1.DatadogAgentProfile{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Status: datadoghqv1alpha1.DatadogAgentProfileStatus{
					LastUpdate:  &now,
					CurrentHash: "foo",
					Conditions:  []metav1.Condition{},
					Valid:       metav1.ConditionFalse,
					Applied:     metav1.ConditionFalse,
					SlowStart: &datadoghqv1alpha1.SlowStart{
						Status:         datadoghqv1alpha1.CompletedStatus,
						LastTransition: &now,
					},
				},
			},
		},
		{
			name: "non-empty profile, non-empty new status, slow start false",
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
					SlowStart: &datadoghqv1alpha1.SlowStart{
						Status:         datadoghqv1alpha1.InProgressStatus,
						LastTransition: &metav1.Time{},
					},
				},
			},
			newStatus: datadoghqv1alpha1.DatadogAgentProfileStatus{
				LastUpdate:  &metav1.Time{},
				CurrentHash: "bar",
				Conditions:  []metav1.Condition{},
				Valid:       metav1.ConditionFalse,
				Applied:     metav1.ConditionFalse,
				SlowStart: &datadoghqv1alpha1.SlowStart{
					Status:       datadoghqv1alpha1.InProgressStatus,
					NodesLabeled: 32,
					PodsReady:    21,
				},
			},
			slowStart: "false",
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
					SlowStart:   nil,
				},
			},
		},
		{
			name: "non-empty profile, non-empty new status, slow start true",
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
					SlowStart: &datadoghqv1alpha1.SlowStart{
						Status:         datadoghqv1alpha1.CompletedStatus,
						LastTransition: &now,
					},
				},
			},
			newStatus: datadoghqv1alpha1.DatadogAgentProfileStatus{
				LastUpdate:  &metav1.Time{},
				CurrentHash: "bar",
				Conditions:  []metav1.Condition{},
				Valid:       metav1.ConditionFalse,
				Applied:     metav1.ConditionFalse,
				SlowStart: &datadoghqv1alpha1.SlowStart{
					Status:       datadoghqv1alpha1.InProgressStatus,
					NodesLabeled: 32,
					PodsReady:    21,
				},
			},
			slowStart: "true",
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
					SlowStart: &datadoghqv1alpha1.SlowStart{
						LastTransition: &now,
						Status:         datadoghqv1alpha1.WaitingStatus,
						NodesLabeled:   32,
						PodsReady:      21,
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := logf.Log.WithName("testUpdateProfileStatus")
			t.Setenv(common.SlowStartEnabled, tt.slowStart)
			UpdateProfileStatus(logger, tt.profile, tt.newStatus, now)
			assert.Equal(t, tt.expectedProfile, tt.profile)
		})
	}
}
