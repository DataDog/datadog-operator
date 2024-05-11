// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func TestIsPodNotReady(t *testing.T) {
	tests := []struct {
		name     string
		pod      *corev1.Pod
		notready bool
		reason   string
	}{
		{
			name: "ready",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					Conditions: []corev1.PodCondition{
						{Type: corev1.PodReady, Status: corev1.ConditionTrue},
					},
				},
			},
			notready: false,
			reason:   "",
		},
		{
			name: "pending",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodPending,
				},
			},
			notready: true,
			reason:   "",
		},
		{
			name: "evicted",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Phase:  corev1.PodFailed,
					Reason: "Evicted",
				},
			},
			notready: true,
			reason:   "Evicted",
		},
		{
			name: "container CLB",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					Conditions: []corev1.PodCondition{
						{Type: corev1.PodReady, Status: corev1.ConditionFalse, Reason: "ContainersNotReady"},
					},
				},
			},
			notready: true,
			reason:   "ContainersNotReady",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			notready, reason := isPodNotReady(tt.pod)
			assert.Equal(t, notready, tt.notready)
			assert.Equal(t, reason, tt.reason)
		})
	}
}

func TestContainersInfo(t *testing.T) {
	tests := []struct {
		name       string
		pod        *corev1.Pod
		ready      string
		containers string
		restarts   string
	}{
		{
			name: "one ready container",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					ContainerStatuses: []corev1.ContainerStatus{
						{Name: "foo", RestartCount: 0, Ready: true},
					},
				},
			},
			ready:      "1/1",
			containers: "",
			restarts:   "0",
		},
		{
			name: "multi containers with restarts, all ready",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					ContainerStatuses: []corev1.ContainerStatus{
						{Name: "foo", RestartCount: 1, Ready: true},
						{Name: "bar", RestartCount: 2, Ready: true},
						{Name: "baz", RestartCount: 3, Ready: true},
					},
				},
			},
			ready:      "3/3",
			containers: "",
			restarts:   "6",
		},
		{
			name: "multi containers with restarts, with notready containers",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					ContainerStatuses: []corev1.ContainerStatus{
						{Name: "foo", RestartCount: 1, Ready: true},
						{Name: "bar", RestartCount: 2, Ready: false},
						{Name: "baz", RestartCount: 3, Ready: true},
					},
				},
			},
			ready:      "2/3",
			containers: "bar",
			restarts:   "6",
		},
		{
			name: "multi containers with restarts, all containers notready",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					ContainerStatuses: []corev1.ContainerStatus{
						{Name: "foo", RestartCount: 1, Ready: false},
						{Name: "bar", RestartCount: 2, Ready: false},
						{Name: "baz", RestartCount: 3, Ready: false},
					},
				},
			},
			ready:      "0/3",
			containers: "foo, bar, baz",
			restarts:   "6",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ready, containers, restarts := containersInfo(tt.pod)
			assert.Equal(t, ready, tt.ready)
			assert.Equal(t, containers, tt.containers)
			assert.Equal(t, restarts, tt.restarts)
		})
	}
}
