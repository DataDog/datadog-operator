// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// Copyright 2016-present Datadog, Inc.

package controller

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPreparedBlueGreenNodePredicateWatchesRelevantLabels(t *testing.T) {
	predicate := preparedBlueGreenNodePredicate()
	oldNode := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node", Labels: map[string]string{"profile": "linux"}}}
	newNode := oldNode.DeepCopy()
	newNode.Labels["experimental.agent.datadoghq.com/rollout-slot-0123456789ab"] = "blue"
	assert.True(t, predicate.Update(event.UpdateEvent{ObjectOld: oldNode, ObjectNew: newNode}), "external or controller slot changes must reconcile the state machine")

	profileChanged := newNode.DeepCopy()
	profileChanged.Labels["profile"] = "gpu"
	assert.True(t, predicate.Update(event.UpdateEvent{ObjectOld: newNode, ObjectNew: profileChanged}))
	unchanged := profileChanged.DeepCopy()
	assert.False(t, predicate.Update(event.UpdateEvent{ObjectOld: profileChanged, ObjectNew: unchanged}))
	assert.True(t, predicate.Create(event.CreateEvent{Object: oldNode}))
	assert.True(t, predicate.Delete(event.DeleteEvent{Object: oldNode}))
	assert.False(t, predicate.Generic(event.GenericEvent{Object: oldNode}))
	assert.False(t, predicate.Update(event.UpdateEvent{ObjectOld: oldNode, ObjectNew: &corev1.Pod{}}))
}

func TestPreparedBlueGreenEventPredicateWatchesModeAndNodeChanges(t *testing.T) {
	pred := preparedBlueGreenEventPredicate()
	disabled := &datadoghqv1alpha1.DatadogAgentInternal{}
	enabled := disabled.DeepCopy()
	enabled.Annotations = map[string]string{"experimental.agent.datadoghq.com/node-agent-rollout-mode": "prepared-blue-green-v1"}
	assert.True(t, pred.Update(event.UpdateEvent{ObjectOld: disabled, ObjectNew: enabled}))
	assert.False(t, pred.Update(event.UpdateEvent{ObjectOld: enabled, ObjectNew: enabled.DeepCopy()}))
	assert.False(t, pred.Update(event.UpdateEvent{ObjectOld: enabled, ObjectNew: &corev1.Node{}}))

	oldNode := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"profile": "default"}}}
	newNode := oldNode.DeepCopy()
	newNode.Labels["profile"] = "gpu"
	assert.True(t, pred.Update(event.UpdateEvent{ObjectOld: oldNode, ObjectNew: newNode}))
	assert.False(t, pred.Update(event.UpdateEvent{ObjectOld: oldNode, ObjectNew: &corev1.Pod{}}))
	assert.False(t, pred.Update(event.UpdateEvent{ObjectOld: &corev1.Pod{}, ObjectNew: &corev1.Pod{}}))
	assert.False(t, pred.Create(event.CreateEvent{Object: enabled}))
	assert.False(t, pred.Delete(event.DeleteEvent{Object: enabled}))
	assert.False(t, pred.Generic(event.GenericEvent{Object: enabled}))
}

func TestNodeEventsEnqueueOnlyPreparedBlueGreenDDAIs(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, datadoghqv1alpha1.AddToScheme(scheme))
	enabled := &datadoghqv1alpha1.DatadogAgentInternal{ObjectMeta: metav1.ObjectMeta{
		Namespace: "agents", Name: "enabled",
		Annotations: map[string]string{"experimental.agent.datadoghq.com/node-agent-rollout-mode": "prepared-blue-green-v1"},
	}}
	disabled := &datadoghqv1alpha1.DatadogAgentInternal{ObjectMeta: metav1.ObjectMeta{Namespace: "agents", Name: "disabled"}}
	reconciler := &DatadogAgentInternalReconciler{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(enabled, disabled).Build()}

	requests := reconciler.enqueuePreparedBlueGreenDDAIs(context.Background(), &corev1.Node{})
	require.Len(t, requests, 1)
	assert.Equal(t, "agents", requests[0].Namespace)
	assert.Equal(t, "enabled", requests[0].Name)
}
