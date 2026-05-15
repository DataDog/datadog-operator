// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package condition

import (
	"testing"
	"time"

	edsdatadoghqv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/pkg/constants"
)

func TestUpdateDeploymentStatus(t *testing.T) {
	now := metav1.NewTime(time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC))

	tests := []struct {
		name      string
		deploy    *appsv1.Deployment
		wantState string
	}{
		{
			name:      "missing deployment is failed",
			deploy:    nil,
			wantState: string(DatadogAgentStateFailed),
		},
		{
			name: "replica failure condition is failed",
			deploy: deploymentWithStatus("agent", appsv1.DeploymentStatus{
				Replicas:        3,
				UpdatedReplicas: 3,
				ReadyReplicas:   3,
				Conditions: []appsv1.DeploymentCondition{
					{Type: appsv1.DeploymentReplicaFailure, Status: corev1.ConditionTrue},
				},
			}),
			wantState: string(DatadogAgentStateFailed),
		},
		{
			name: "updated replicas behind desired replicas is updating",
			deploy: deploymentWithStatus("agent", appsv1.DeploymentStatus{
				Replicas:        3,
				UpdatedReplicas: 2,
				ReadyReplicas:   2,
			}),
			wantState: string(DatadogAgentStateUpdating),
		},
		{
			name: "no ready replicas is progressing",
			deploy: deploymentWithStatus("agent", appsv1.DeploymentStatus{
				Replicas:        3,
				UpdatedReplicas: 3,
				ReadyReplicas:   0,
			}),
			wantState: string(DatadogAgentStateProgressing),
		},
		{
			name: "ready deployment is running",
			deploy: deploymentWithStatus("agent", appsv1.DeploymentStatus{
				Replicas:        3,
				UpdatedReplicas: 3,
				ReadyReplicas:   3,
			}),
			wantState: string(DatadogAgentStateRunning),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := UpdateDeploymentStatus(tt.deploy, nil, &now)

			require.Equal(t, tt.wantState, got.State)
		})
	}
}

func TestUpdateDaemonSetStatusStates(t *testing.T) {
	tests := []struct {
		name      string
		status    appsv1.DaemonSetStatus
		wantState string
	}{
		{
			name: "updated nodes behind desired nodes is updating",
			status: appsv1.DaemonSetStatus{
				DesiredNumberScheduled: 3,
				UpdatedNumberScheduled: 2,
				NumberReady:            2,
			},
			wantState: string(DatadogAgentStateUpdating),
		},
		{
			name: "desired nodes with no ready nodes is progressing",
			status: appsv1.DaemonSetStatus{
				DesiredNumberScheduled: 3,
				UpdatedNumberScheduled: 3,
				NumberReady:            0,
			},
			wantState: string(DatadogAgentStateProgressing),
		},
		{
			name: "up-to-date ready nodes are running",
			status: appsv1.DaemonSetStatus{
				DesiredNumberScheduled: 3,
				UpdatedNumberScheduled: 3,
				NumberReady:            3,
			},
			wantState: string(DatadogAgentStateRunning),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ds := &appsv1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name: "agent",
					Annotations: map[string]string{
						constants.MD5AgentDeploymentAnnotationKey: "hash",
					},
				},
				Status: tt.status,
			}

			got := UpdateDaemonSetStatus("agent", ds, nil, nil)

			require.Len(t, got, 1)
			require.Equal(t, tt.wantState, got[0].State)
			require.Equal(t, "hash", got[0].CurrentHash)
		})
	}
}

func TestUpdateExtendedDaemonSetStatusStates(t *testing.T) {
	tests := []struct {
		name      string
		status    edsdatadoghqv1alpha1.ExtendedDaemonSetStatus
		wantState string
	}{
		{
			name: "canary status has priority",
			status: edsdatadoghqv1alpha1.ExtendedDaemonSetStatus{
				Desired:  3,
				Ready:    3,
				UpToDate: 3,
				Canary:   &edsdatadoghqv1alpha1.ExtendedDaemonSetStatusCanary{},
			},
			wantState: string(DatadogAgentStateCanary),
		},
		{
			name: "updated nodes behind desired nodes is updating",
			status: edsdatadoghqv1alpha1.ExtendedDaemonSetStatus{
				Desired:  3,
				Ready:    2,
				UpToDate: 2,
			},
			wantState: string(DatadogAgentStateUpdating),
		},
		{
			name: "desired nodes with no ready nodes is progressing",
			status: edsdatadoghqv1alpha1.ExtendedDaemonSetStatus{
				Desired:  3,
				Ready:    0,
				UpToDate: 3,
			},
			wantState: string(DatadogAgentStateProgressing),
		},
		{
			name: "up-to-date ready nodes are running",
			status: edsdatadoghqv1alpha1.ExtendedDaemonSetStatus{
				Desired:  3,
				Ready:    3,
				UpToDate: 3,
			},
			wantState: string(DatadogAgentStateRunning),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eds := &edsdatadoghqv1alpha1.ExtendedDaemonSet{
				ObjectMeta: metav1.ObjectMeta{Name: "agent"},
				Status:     tt.status,
			}

			got := UpdateExtendedDaemonSetStatus(eds, nil, nil)

			require.Len(t, got, 1)
			require.Equal(t, tt.wantState, got[0].State)
		})
	}
}

func TestUpdateCombinedDaemonSetStatus(t *testing.T) {
	got := UpdateCombinedDaemonSetStatus([]*v2alpha1.DaemonSetStatus{
		{
			Desired:   2,
			Current:   2,
			Ready:     2,
			Available: 2,
			UpToDate:  2,
			State:     string(DatadogAgentStateRunning),
		},
		{
			Desired:   1,
			Current:   1,
			Ready:     0,
			Available: 0,
			UpToDate:  0,
			State:     string(DatadogAgentStateUpdating),
		},
	})

	require.Equal(t, int32(3), got.Desired)
	require.Equal(t, int32(2), got.Ready)
	require.Equal(t, int32(2), got.UpToDate)
	require.Equal(t, string(DatadogAgentStateUpdating), got.State)
	require.Equal(t, "Updating (3/2/2)", got.Status)
}

func TestIsEqualConditions(t *testing.T) {
	current := []metav1.Condition{
		{Type: "Ready", Status: metav1.ConditionTrue, Reason: "Ready", Message: "ready"},
		{Type: "Valid", Status: metav1.ConditionFalse, Reason: "Invalid", Message: "invalid"},
	}
	sameDifferentOrder := []metav1.Condition{
		{Type: "Valid", Status: metav1.ConditionFalse, Reason: "Invalid", Message: "invalid"},
		{Type: "Ready", Status: metav1.ConditionTrue, Reason: "Ready", Message: "ready"},
	}
	differentMessage := []metav1.Condition{
		{Type: "Ready", Status: metav1.ConditionTrue, Reason: "Ready", Message: "changed"},
		{Type: "Valid", Status: metav1.ConditionFalse, Reason: "Invalid", Message: "invalid"},
	}

	require.True(t, IsEqualConditions(current, sameDifferentOrder))
	require.False(t, IsEqualConditions(current, differentMessage))
	require.False(t, IsEqualConditions(current, current[:1]))
}

func deploymentWithStatus(name string, status appsv1.DeploymentStatus) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Annotations: map[string]string{
				constants.MD5AgentDeploymentAnnotationKey: "hash",
			},
		},
		Status: status,
	}
}
