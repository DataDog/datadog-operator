// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package condition

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	assert "github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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
