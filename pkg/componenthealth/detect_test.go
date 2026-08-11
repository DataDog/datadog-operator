// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package componenthealth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/pkg/constants"
)

func componentPod(component string, status corev1.PodStatus) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod-1",
			Namespace: "datadog",
			Labels:    map[string]string{common.AgentDeploymentComponentLabelKey: component},
		},
		Status: status,
	}
}

func issueTypes(issues []DetectedIssue) []string {
	out := make([]string, 0, len(issues))
	for _, i := range issues {
		out = append(out, i.IssueType)
	}
	return out
}

func TestManagedComponent(t *testing.T) {
	tests := []struct {
		name  string
		label string
		want  string
	}{
		{"cluster agent", constants.DefaultClusterAgentResourceSuffix, constants.DefaultClusterAgentResourceSuffix},
		{"cluster checks runner", constants.DefaultClusterChecksRunnerResourceSuffix, constants.DefaultClusterChecksRunnerResourceSuffix},
		{"node agent excluded", constants.DefaultAgentResourceSuffix, ""},
		{"unlabeled excluded", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ManagedComponent(componentPod(tt.label, corev1.PodStatus{})))
		})
	}
}

func TestDetectPodIssues_NonManagedReturnsNil(t *testing.T) {
	// Node agent pod with an OOMKill must be ignored (out of scope).
	pod := componentPod(constants.DefaultAgentResourceSuffix, corev1.PodStatus{
		ContainerStatuses: []corev1.ContainerStatus{{
			Name:                 "agent",
			LastTerminationState: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: "OOMKilled", ExitCode: 137}},
		}},
	})
	assert.Nil(t, DetectPodIssues(pod, defaultTestThreshold))
}

const defaultTestThreshold int32 = 5

func TestDetectPodIssues(t *testing.T) {
	tests := []struct {
		name      string
		status    corev1.PodStatus
		threshold int32
		want      []string
	}{
		{
			name:   "healthy pod",
			status: corev1.PodStatus{Phase: corev1.PodRunning, ContainerStatuses: []corev1.ContainerStatus{{Name: "cluster-agent", RestartCount: 0}}},
			want:   nil,
		},
		{
			name: "oomkilled via last termination",
			status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{
				Name:                 "cluster-agent",
				LastTerminationState: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: "OOMKilled", ExitCode: 137}},
			}}},
			want: []string{IssueOOMKilled},
		},
		{
			name: "oomkilled via current terminated state",
			status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{
				Name:  "cluster-agent",
				State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: "OOMKilled", ExitCode: 137}},
			}}},
			want: []string{IssueOOMKilled},
		},
		{
			name: "crash loop backoff",
			status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{
				Name:  "cluster-agent",
				State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff", Message: "back-off"}},
			}}},
			want: []string{IssueCrashLooping},
		},
		{
			name: "image pull backoff",
			status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{
				Name:  "cluster-agent",
				State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff", Message: "cannot pull"}},
			}}},
			want: []string{IssueImagePullFailure},
		},
		{
			name: "err image pull",
			status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{
				Name:  "cluster-agent",
				State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ErrImagePull"}},
			}}},
			want: []string{IssueImagePullFailure},
		},
		{
			name: "unschedulable",
			status: corev1.PodStatus{
				Phase: corev1.PodPending,
				Conditions: []corev1.PodCondition{{
					Type:    corev1.PodScheduled,
					Status:  corev1.ConditionFalse,
					Reason:  corev1.PodReasonUnschedulable,
					Message: "0/3 nodes are available",
				}},
			},
			want: []string{IssueUnschedulable},
		},
		{
			name: "pending but scheduled is not unschedulable",
			status: corev1.PodStatus{
				Phase:      corev1.PodPending,
				Conditions: []corev1.PodCondition{{Type: corev1.PodScheduled, Status: corev1.ConditionTrue}},
			},
			want: nil,
		},
		{
			name: "high restart count flags crash looping",
			status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{
				Name:         "cluster-agent",
				RestartCount: 7,
			}}},
			threshold: 5,
			want:      []string{IssueCrashLooping},
		},
		{
			name: "restart count below threshold is healthy",
			status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{
				Name:         "cluster-agent",
				RestartCount: 2,
			}}},
			threshold: 5,
			want:      nil,
		},
		{
			name: "crashloopbackoff not double-counted with high restarts",
			status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{
				Name:         "cluster-agent",
				RestartCount: 9,
				State:        corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}},
			}}},
			threshold: 5,
			want:      []string{IssueCrashLooping},
		},
		{
			name: "init container oomkill detected",
			status: corev1.PodStatus{InitContainerStatuses: []corev1.ContainerStatus{{
				Name:                 "init-config",
				LastTerminationState: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: "OOMKilled", ExitCode: 137}},
			}}},
			want: []string{IssueOOMKilled},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			threshold := tt.threshold
			if threshold == 0 {
				threshold = defaultTestThreshold
			}
			issues := DetectPodIssues(componentPod(constants.DefaultClusterAgentResourceSuffix, tt.status), threshold)
			assert.ElementsMatch(t, tt.want, issueTypes(issues))
			for _, i := range issues {
				assert.Equal(t, constants.DefaultClusterAgentResourceSuffix, i.Component)
				assert.Equal(t, "datadog", i.Namespace)
				assert.Equal(t, "pod-1", i.PodName)
			}
		})
	}
}

func TestDetectPodIssues_RestartThresholdDisabled(t *testing.T) {
	pod := componentPod(constants.DefaultClusterAgentResourceSuffix, corev1.PodStatus{
		ContainerStatuses: []corev1.ContainerStatus{{Name: "cluster-agent", RestartCount: 100}},
	})
	// threshold <= 0 disables the high-restart heuristic.
	assert.Nil(t, DetectPodIssues(pod, 0))
}
