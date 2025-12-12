// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentprofile

import (
	"testing"

	"github.com/DataDog/datadog-operator/api/datadoghq/common"
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
		createStrategy  string
		expectedProfile *datadoghqv1alpha1.DatadogAgentProfile
	}{
		{
			name:            "nil profile, create strategy false",
			profile:         nil,
			newStatus:       datadoghqv1alpha1.DatadogAgentProfileStatus{},
			createStrategy:  "false",
			expectedProfile: nil,
		},
		{
			name:            "nil profile, create strategy true",
			profile:         nil,
			newStatus:       datadoghqv1alpha1.DatadogAgentProfileStatus{},
			createStrategy:  "true",
			expectedProfile: nil,
		},
		{
			name:    "empty profile, non-empty new status, create strategy false",
			profile: &datadoghqv1alpha1.DatadogAgentProfile{},
			newStatus: datadoghqv1alpha1.DatadogAgentProfileStatus{
				LastUpdate:  &metav1.Time{},
				CurrentHash: "foo",
				Conditions:  []metav1.Condition{},
				Valid:       metav1.ConditionFalse,
				Applied:     metav1.ConditionFalse,
				CreateStrategy: &datadoghqv1alpha1.CreateStrategy{
					Status: datadoghqv1alpha1.CompletedStatus,
				},
			},
			createStrategy:  "false",
			expectedProfile: &datadoghqv1alpha1.DatadogAgentProfile{},
		},
		{
			name:    "empty profile, non-empty new status, create strategy true",
			profile: &datadoghqv1alpha1.DatadogAgentProfile{},
			newStatus: datadoghqv1alpha1.DatadogAgentProfileStatus{
				LastUpdate:  &metav1.Time{},
				CurrentHash: "foo",
				Conditions:  []metav1.Condition{},
				Valid:       metav1.ConditionFalse,
				Applied:     metav1.ConditionFalse,
				CreateStrategy: &datadoghqv1alpha1.CreateStrategy{
					Status: datadoghqv1alpha1.CompletedStatus,
				},
			},
			createStrategy:  "true",
			expectedProfile: &datadoghqv1alpha1.DatadogAgentProfile{},
		},
		{
			name: "non-empty profile, empty new status, create strategy false",
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
					CreateStrategy: &datadoghqv1alpha1.CreateStrategy{
						Status:         datadoghqv1alpha1.CompletedStatus,
						LastTransition: &now,
					},
				},
			},
			newStatus:      datadoghqv1alpha1.DatadogAgentProfileStatus{},
			createStrategy: "false",
			expectedProfile: &datadoghqv1alpha1.DatadogAgentProfile{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Status: datadoghqv1alpha1.DatadogAgentProfileStatus{
					LastUpdate:     &now,
					CurrentHash:    "",
					Conditions:     nil,
					Valid:          metav1.ConditionUnknown,
					Applied:        metav1.ConditionUnknown,
					CreateStrategy: nil,
				},
			},
		},
		{
			name: "non-empty profile, empty new status, create strategy true",
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
					CreateStrategy: &datadoghqv1alpha1.CreateStrategy{
						Status:         datadoghqv1alpha1.CompletedStatus,
						LastTransition: &now,
					},
				},
			},
			newStatus:      datadoghqv1alpha1.DatadogAgentProfileStatus{},
			createStrategy: "true",
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
					CreateStrategy: &datadoghqv1alpha1.CreateStrategy{
						Status:         datadoghqv1alpha1.CompletedStatus,
						LastTransition: &now,
					},
				},
			},
		},
		{
			name: "non-empty profile, non-empty new status, create strategy false",
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
					CreateStrategy: &datadoghqv1alpha1.CreateStrategy{
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
				CreateStrategy: &datadoghqv1alpha1.CreateStrategy{
					Status:       datadoghqv1alpha1.InProgressStatus,
					NodesLabeled: 32,
					PodsReady:    21,
				},
			},
			createStrategy: "false",
			expectedProfile: &datadoghqv1alpha1.DatadogAgentProfile{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Status: datadoghqv1alpha1.DatadogAgentProfileStatus{
					LastUpdate:     &now,
					CurrentHash:    "bar",
					Conditions:     []metav1.Condition{},
					Valid:          metav1.ConditionFalse,
					Applied:        metav1.ConditionFalse,
					CreateStrategy: nil,
				},
			},
		},
		{
			name: "non-empty profile, non-empty new status, create strategy true",
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
					CreateStrategy: &datadoghqv1alpha1.CreateStrategy{
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
				CreateStrategy: &datadoghqv1alpha1.CreateStrategy{
					Status:       datadoghqv1alpha1.InProgressStatus,
					NodesLabeled: 32,
					PodsReady:    21,
				},
			},
			createStrategy: "true",
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
					CreateStrategy: &datadoghqv1alpha1.CreateStrategy{
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
			t.Setenv(common.CreateStrategyEnabled, tt.createStrategy)
			UpdateProfileStatus(logger, tt.profile, tt.newStatus, now)
			assert.Equal(t, tt.expectedProfile, tt.profile)
		})
	}
}

func TestGenerateProfileStatusFromConditions(t *testing.T) {
	now := metav1.Now()
	tests := []struct {
		name                  string
		profile               *datadoghqv1alpha1.DatadogAgentProfile
		expectedProfileStatus datadoghqv1alpha1.DatadogAgentProfileStatus
	}{
		{
			name: "profile with no conditions",
			profile: &datadoghqv1alpha1.DatadogAgentProfile{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-profile",
					Namespace: "test-namespace",
				},
				Spec: datadoghqv1alpha1.DatadogAgentProfileSpec{},
				Status: datadoghqv1alpha1.DatadogAgentProfileStatus{
					Conditions: []metav1.Condition{},
				},
			},
			expectedProfileStatus: datadoghqv1alpha1.DatadogAgentProfileStatus{
				LastUpdate:  &now,
				CurrentHash: "99914b932bd37a50b983c5e7c90ae93b",
				Valid:       "",
				Applied:     "",
			},
		},
		{
			name: "profile with Valid condition True",
			profile: &datadoghqv1alpha1.DatadogAgentProfile{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-profile",
					Namespace: "test-namespace",
				},
				Spec: datadoghqv1alpha1.DatadogAgentProfileSpec{},
				Status: datadoghqv1alpha1.DatadogAgentProfileStatus{
					Conditions: []metav1.Condition{
						{
							Type:               ValidConditionType,
							Status:             metav1.ConditionTrue,
							LastTransitionTime: now,
							Reason:             ValidConditionReason,
							Message:            "Profile is valid",
						},
					},
				},
			},
			expectedProfileStatus: datadoghqv1alpha1.DatadogAgentProfileStatus{
				LastUpdate:  &now,
				CurrentHash: "99914b932bd37a50b983c5e7c90ae93b",
				Valid:       metav1.ConditionTrue,
				Applied:     "",
			},
		},
		{
			name: "profile with Applied condition True",
			profile: &datadoghqv1alpha1.DatadogAgentProfile{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-profile",
					Namespace: "test-namespace",
				},
				Spec: datadoghqv1alpha1.DatadogAgentProfileSpec{},
				Status: datadoghqv1alpha1.DatadogAgentProfileStatus{
					Conditions: []metav1.Condition{
						{
							Type:               AppliedConditionType,
							Status:             metav1.ConditionTrue,
							LastTransitionTime: now,
							Reason:             AppliedConditionReason,
							Message:            "Profile is applied",
						},
					},
				},
			},
			expectedProfileStatus: datadoghqv1alpha1.DatadogAgentProfileStatus{
				LastUpdate:  &now,
				CurrentHash: "99914b932bd37a50b983c5e7c90ae93b",
				Valid:       "",
				Applied:     metav1.ConditionTrue,
			},
		},
		{
			name: "profile with both Valid and Applied conditions",
			profile: &datadoghqv1alpha1.DatadogAgentProfile{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-profile",
					Namespace: "test-namespace",
				},
				Spec: datadoghqv1alpha1.DatadogAgentProfileSpec{},
				Status: datadoghqv1alpha1.DatadogAgentProfileStatus{
					Conditions: []metav1.Condition{
						{
							Type:               ValidConditionType,
							Status:             metav1.ConditionTrue,
							LastTransitionTime: now,
							Reason:             ValidConditionReason,
							Message:            "Profile is valid",
						},
						{
							Type:               AppliedConditionType,
							Status:             metav1.ConditionFalse,
							LastTransitionTime: now,
							Reason:             ConflictConditionReason,
							Message:            "Profile has conflicts",
						},
					},
				},
			},
			expectedProfileStatus: datadoghqv1alpha1.DatadogAgentProfileStatus{
				LastUpdate:  &now,
				CurrentHash: "99914b932bd37a50b983c5e7c90ae93b",
				Valid:       metav1.ConditionTrue,
				Applied:     metav1.ConditionFalse,
			},
		},
		{
			name: "profile with multiple Valid conditions uses last one",
			profile: &datadoghqv1alpha1.DatadogAgentProfile{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-profile",
					Namespace: "test-namespace",
				},
				Spec: datadoghqv1alpha1.DatadogAgentProfileSpec{},
				Status: datadoghqv1alpha1.DatadogAgentProfileStatus{
					Conditions: []metav1.Condition{
						{
							Type:               ValidConditionType,
							Status:             metav1.ConditionFalse,
							LastTransitionTime: now,
							Reason:             InvalidConditionReason,
							Message:            "First valid condition - false",
						},
						{
							Type:               ValidConditionType,
							Status:             metav1.ConditionTrue,
							LastTransitionTime: now,
							Reason:             ValidConditionReason,
							Message:            "Second valid condition - true",
						},
					},
				},
			},
			expectedProfileStatus: datadoghqv1alpha1.DatadogAgentProfileStatus{
				LastUpdate:  &now,
				CurrentHash: "99914b932bd37a50b983c5e7c90ae93b",
				Valid:       metav1.ConditionTrue,
				Applied:     "",
			},
		},
		{
			name: "profile with unrelated conditions ignores them",
			profile: &datadoghqv1alpha1.DatadogAgentProfile{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-profile",
					Namespace: "test-namespace",
				},
				Spec: datadoghqv1alpha1.DatadogAgentProfileSpec{},
				Status: datadoghqv1alpha1.DatadogAgentProfileStatus{
					Conditions: []metav1.Condition{
						{
							Type:               "UnrelatedCondition",
							Status:             metav1.ConditionTrue,
							LastTransitionTime: now,
							Reason:             "SomeReason",
							Message:            "Unrelated condition",
						},
						{
							Type:               ValidConditionType,
							Status:             metav1.ConditionTrue,
							LastTransitionTime: now,
							Reason:             ValidConditionReason,
							Message:            "Profile is valid",
						},
					},
				},
			},
			expectedProfileStatus: datadoghqv1alpha1.DatadogAgentProfileStatus{
				LastUpdate:  &now,
				CurrentHash: "99914b932bd37a50b983c5e7c90ae93b",
				Valid:       metav1.ConditionTrue,
				Applied:     "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := logf.Log.WithName("testGenerateProfileStatusFromConditions")

			GenerateProfileStatusFromConditions(logger, tt.profile, now)
			assert.Equal(t, tt.expectedProfileStatus, tt.profile.Status)
		})
	}
}
