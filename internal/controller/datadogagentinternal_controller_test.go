// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// Copyright 2016-present Datadog, Inc.

package controller

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResourceFallbackPodPredicate(t *testing.T) {
	predicate := resourceFallbackPodPredicate()
	oldPod := &corev1.Pod{Status: corev1.PodStatus{Conditions: []corev1.PodCondition{{Type: corev1.PodScheduled, Status: corev1.ConditionFalse, Reason: corev1.PodReasonUnschedulable, Message: "old"}}}}
	newPod := oldPod.DeepCopy()
	newPod.Status.Conditions[0].Message = "0/1 nodes are available: 1 Insufficient cpu."
	assert.True(t, predicate.Update(event.UpdateEvent{ObjectOld: oldPod, ObjectNew: newPod}), "PodScheduled message-only updates must enqueue")

	readyPod := newPod.DeepCopy()
	readyPod.Status.Conditions = append(readyPod.Status.Conditions, corev1.PodCondition{Type: corev1.PodReady, Status: corev1.ConditionTrue})
	assert.True(t, predicate.Update(event.UpdateEvent{ObjectOld: newPod, ObjectNew: readyPod}), "PodReady transitions must release fallback reservations promptly")

	waiting := readyPod.DeepCopy()
	waiting.Status.ContainerStatuses = []corev1.ContainerStatus{{Name: "agent", Started: ptr.To(false), State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}}}
	prepared := waiting.DeepCopy()
	prepared.Status.ContainerStatuses[0].Started = ptr.To(true)
	assert.True(t, predicate.Update(event.UpdateEvent{ObjectOld: waiting, ObjectNew: prepared}), "startup-probe Prepared transitions must enqueue the handoff controller")
}

func TestEnqueueDatadogAgentInternalForPodFollowsDaemonSetOwner(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, appsv1.AddToScheme(scheme))
	ds := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{
		Name:      "profile-agent",
		Namespace: "default",
		UID:       types.UID("ds-uid"),
		OwnerReferences: []metav1.OwnerReference{{
			APIVersion: datadoghqv1alpha1.GroupVersion.String(),
			Kind:       "DatadogAgentInternal",
			Name:       "profile-ddai",
			UID:        types.UID("ddai-uid"),
			Controller: ptr.To(true),
		}},
	}}
	reader := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ds).Build()
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name:      "profile-agent-new",
		Namespace: "default",
		Labels:    map[string]string{apicommon.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix},
		OwnerReferences: []metav1.OwnerReference{{
			APIVersion: appsv1.SchemeGroupVersion.String(),
			Kind:       "DaemonSet",
			Name:       ds.Name,
			UID:        ds.UID,
			Controller: ptr.To(true),
		}},
	}}

	requests := enqueueDatadogAgentInternalForPod(reader)(context.Background(), pod)
	require.Len(t, requests, 1)
	assert.Equal(t, "default", requests[0].Namespace)
	assert.Equal(t, "profile-ddai", requests[0].Name)

	pod.OwnerReferences[0].UID = "wrong-uid"
	assert.Empty(t, enqueueDatadogAgentInternalForPod(reader)(context.Background(), pod), "stale Pod owner UIDs must not enqueue")
}
