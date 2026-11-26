// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package condition

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	assert "github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
)

func TestDeleteDatadogAgentStatusCondition(t *testing.T) {
	type args struct {
		status    *v2alpha1.DatadogAgentStatus
		condition string
	}
	tests := []struct {
		name           string
		args           args
		expectedStatus *v2alpha1.DatadogAgentStatus
	}{
		{
			name: "empty status",
			args: args{
				status:    &v2alpha1.DatadogAgentStatus{},
				condition: "fooType",
			},
			expectedStatus: &v2alpha1.DatadogAgentStatus{},
		},
		{
			name: "not present status",
			args: args{
				status: &v2alpha1.DatadogAgentStatus{
					Conditions: []v1.Condition{
						{
							Type: "barType",
						},
					},
				},
				condition: "fooType",
			},
			expectedStatus: &v2alpha1.DatadogAgentStatus{
				Conditions: []v1.Condition{
					{
						Type: "barType",
					},
				},
			},
		},
		{
			name: "status present at the end",
			args: args{
				status: &v2alpha1.DatadogAgentStatus{
					Conditions: []v1.Condition{
						{
							Type: "barType",
						},
						{
							Type: "fooType",
						},
					},
				},
				condition: "fooType",
			},
			expectedStatus: &v2alpha1.DatadogAgentStatus{
				Conditions: []v1.Condition{
					{
						Type: "barType",
					},
				},
			},
		},
		{
			name: "status present at the begining",
			args: args{
				status: &v2alpha1.DatadogAgentStatus{
					Conditions: []v1.Condition{
						{
							Type: "fooType",
						},
						{
							Type: "barType",
						},
					},
				},
				condition: "fooType",
			},
			expectedStatus: &v2alpha1.DatadogAgentStatus{
				Conditions: []v1.Condition{
					{
						Type: "barType",
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			DeleteDatadogAgentStatusCondition(tt.args.status, tt.args.condition)
			assert.True(t, apiutils.IsEqualStruct(tt.args.status, tt.expectedStatus), "status \ndiff = %s", cmp.Diff(tt.args.status, tt.expectedStatus))
		})
	}
}

func TestUpdateDaemonSetStatusDDAIFailed(t *testing.T) {
	now := v1.Now()
	previous := func() *v2alpha1.DaemonSetStatus {
		return &v2alpha1.DaemonSetStatus{
			Desired:   2,
			Current:   2,
			Ready:     2,
			Available: 2,
			UpToDate:  2,
			State:     string(DatadogAgentStateRunning),
			Status:    "Running (2/2/2)",
		}
	}

	tests := []struct {
		name           string
		ds             *appsv1.DaemonSet
		expectedState  string
		expectedStatus string
		expectedCounts [3]int32 // desired, ready, up to date
	}{
		{
			name: "live DaemonSet: Failed with its current counts",
			ds: &appsv1.DaemonSet{
				ObjectMeta: v1.ObjectMeta{Name: "datadog-agent"},
				Status: appsv1.DaemonSetStatus{
					DesiredNumberScheduled: 3,
					CurrentNumberScheduled: 2,
					NumberReady:            2,
					NumberAvailable:        2,
					UpdatedNumberScheduled: 0,
				},
			},
			expectedState:  "Failed",
			expectedStatus: "Failed (3/2/0)",
			expectedCounts: [3]int32{3, 2, 0},
		},
		{
			name:           "DaemonSet not found: Failed, previous counts kept",
			ds:             nil,
			expectedState:  "Failed",
			expectedStatus: "Failed",
			expectedCounts: [3]int32{2, 2, 2},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := UpdateDaemonSetStatusDDAIFailed("datadog-agent", tt.ds, previous(), &now)
			assert.Equal(t, tt.expectedState, got.State)
			assert.Equal(t, tt.expectedStatus, got.Status)
			assert.Equal(t, "datadog-agent", got.DaemonsetName)
			assert.Equal(t, tt.expectedCounts, [3]int32{got.Desired, got.Ready, got.UpToDate})
		})
	}
}

func TestCombineDaemonSetStatusFailedWins(t *testing.T) {
	now := v1.Now()
	tests := []struct {
		name          string
		defaultState  DatadogAgentState
		profileState  DatadogAgentState
		expectedState string
	}{
		{name: "default Running, profile Failed", defaultState: DatadogAgentStateRunning, profileState: DatadogAgentStateFailed, expectedState: "Failed"},
		{name: "default Failed, profile Updating", defaultState: DatadogAgentStateFailed, profileState: DatadogAgentStateUpdating, expectedState: "Failed"},
		{name: "both Running", defaultState: DatadogAgentStateRunning, profileState: DatadogAgentStateRunning, expectedState: "Running"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			combined := CombineDaemonSetStatus(nil, &v2alpha1.DaemonSetStatus{State: string(tt.defaultState), LastUpdate: &now})
			combined = CombineDaemonSetStatus(combined, &v2alpha1.DaemonSetStatus{State: string(tt.profileState), LastUpdate: &now})
			assert.Equal(t, tt.expectedState, combined.State)
		})
	}
}

func TestAppendDatadogAgentInternalStatusConditions(t *testing.T) {
	now := v1.NewTime(time.Unix(100, 0))
	later := v1.NewTime(time.Unix(200, 0))
	status := &v1alpha1.DatadogAgentInternalStatus{}
	UpdateDatadogAgentInternalStatusConditions(status, now, "Conflict", v1.ConditionTrue, "Disabled", "old component", true)
	UpdateDatadogAgentInternalStatusConditions(status, now, "Ready", v1.ConditionTrue, "Ready", "unchanged", true)

	messages := make(map[string][]string)
	AppendDatadogAgentInternalStatusConditions(status, messages, later, "Conflict", v1.ConditionTrue, "Disabled", "DCA disabled", true)
	AppendDatadogAgentInternalStatusConditions(status, messages, later, "Conflict", v1.ConditionTrue, "Disabled", "CCR disabled", true)
	AppendDatadogAgentInternalStatusConditions(status, messages, later, "Conflict", v1.ConditionTrue, "Disabled", "DCA disabled", true)
	assert.Equal(t, "DCA disabled; CCR disabled", GetDDAICondition(status, "Conflict").Message)
	assert.Equal(t, now, GetDDAICondition(status, "Conflict").LastTransitionTime)
	assert.Len(t, status.Conditions, 2)
	assert.Equal(t, "unchanged", GetDDAICondition(status, "Ready").Message)

	// An early return without evaluation preserves the previous status.
	messages = make(map[string][]string)
	assert.Equal(t, "DCA disabled; CCR disabled", GetDDAICondition(status, "Conflict").Message)
	// The next reconciliation only finds one remaining conflict.
	AppendDatadogAgentInternalStatusConditions(status, messages, later, "Conflict", v1.ConditionTrue, "Disabled", "CCR disabled", true)
	assert.Equal(t, "CCR disabled", GetDDAICondition(status, "Conflict").Message)
	assert.Equal(t, now, GetDDAICondition(status, "Conflict").LastTransitionTime)
	// A new status or reason must not inherit messages with a different meaning.
	AppendDatadogAgentInternalStatusConditions(status, messages, later, "Conflict", v1.ConditionFalse, "Resolved", "resolved", true)
	assert.Equal(t, "resolved", GetDDAICondition(status, "Conflict").Message)
	assert.Equal(t, later, GetDDAICondition(status, "Conflict").LastTransitionTime)
	AppendDatadogAgentInternalStatusConditions(status, messages, later, "Conflict", v1.ConditionFalse, "OtherReason", "other", true)
	assert.Equal(t, "other", GetDDAICondition(status, "Conflict").Message)
}

func TestAppendDatadogAgentInternalStatusConditionsIndependentConditionTypes(t *testing.T) {
	status := &v1alpha1.DatadogAgentInternalStatus{}
	messages := make(map[string][]string)
	now := v1.NewTime(time.Unix(100, 0))
	AppendDatadogAgentInternalStatusConditions(status, messages, now, "First", v1.ConditionTrue, "Failed", "one; two", true)
	AppendDatadogAgentInternalStatusConditions(status, messages, now, "Second", v1.ConditionTrue, "Failed", "three", true)
	AppendDatadogAgentInternalStatusConditions(status, messages, now, "First", v1.ConditionTrue, "Failed", "two", true)
	assert.Equal(t, "one; two; two", GetDDAICondition(status, "First").Message)
	assert.Equal(t, "three", GetDDAICondition(status, "Second").Message)
	UpdateDatadogAgentInternalStatusConditions(status, now, "Absent", v1.ConditionFalse, "Ready", "ready", false)
	assert.Nil(t, GetDDAICondition(status, "Absent"))
	UpdateDatadogAgentInternalStatusConditions(status, now, "Absent", v1.ConditionFalse, "Ready", "ready", true)
	assert.Equal(t, v1.ConditionFalse, GetDDAICondition(status, "Absent").Status)
}

func TestAppendDatadogAgentInternalStatusConditionsAppendAfterSkippedFalseCondition(t *testing.T) {
	status := &v1alpha1.DatadogAgentInternalStatus{}
	messages := make(map[string][]string)
	now := v1.NewTime(time.Unix(100, 0))
	AppendDatadogAgentInternalStatusConditions(status, messages, now, "Conflict", v1.ConditionFalse, "Resolved", "resolved", false)
	assert.Empty(t, status.Conditions)
	AppendDatadogAgentInternalStatusConditions(status, messages, now, "Conflict", v1.ConditionTrue, "Disabled", "disabled", true)
	assert.Equal(t, "disabled", GetDDAICondition(status, "Conflict").Message)
}
