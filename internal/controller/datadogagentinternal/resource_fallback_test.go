// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// Copyright 2016-present Datadog, Inc.

package datadogagentinternal

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResourceFallbackDeletesOnlyOldPodThatMakesReplacementFit(t *testing.T) {
	fixture := newResourceFallbackFixture(t)
	result, err := fixture.reconciler.reconcileResourceFallback(context.Background(), fixture.ddai, fixture.ds, intstr.FromInt(1))
	require.NoError(t, err)
	assert.Equal(t, time.Second, result.RequeueAfter)
	err = fixture.client.Get(context.Background(), client.ObjectKeyFromObject(fixture.old), &corev1.Pod{})
	assert.True(t, apierrors.IsNotFound(err))
	require.NoError(t, fixture.client.Get(context.Background(), client.ObjectKeyFromObject(fixture.pending), fixture.pending))
	assert.Equal(t, string(fixture.old.UID), fixture.pending.Annotations[resourceFallbackOldPodAnnotation])
}

func TestResourceFallbackFailsClosedForOtherSchedulingBlockers(t *testing.T) {
	for _, tt := range []struct {
		name   string
		mutate func(*resourceFallbackFixture)
	}{
		{name: "mixed scheduler reason", mutate: func(f *resourceFallbackFixture) {
			f.pending.Status.Conditions[0].Message = "0/1 nodes are available: 1 node(s) didn't have free ports for the requested pod ports, 1 Insufficient cpu."
		}},
		{name: "hostPort introduced by admission", mutate: func(f *resourceFallbackFixture) {
			f.pending.Spec.Containers[0].Ports = []corev1.ContainerPort{{ContainerPort: 8126, HostPort: 8126}}
		}},
		{name: "old requests are insufficient", mutate: func(f *resourceFallbackFixture) {
			f.pending.Spec.Containers[0].Resources.Requests[corev1.ResourceCPU] = resource.MustParse("2")
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newResourceFallbackFixture(t)
			tt.mutate(&fixture)
			if tt.name == "mixed scheduler reason" {
				_, safe := resourceOnlyUnschedulable(fixture.pending)
				require.False(t, safe)
			}
			desiredStatus := *fixture.pending.Status.DeepCopy()
			require.NoError(t, fixture.client.Update(context.Background(), fixture.pending))
			livePending := &corev1.Pod{}
			require.NoError(t, fixture.client.Get(context.Background(), client.ObjectKeyFromObject(fixture.pending), livePending))
			livePending.Status = desiredStatus
			require.NoError(t, fixture.client.Status().Update(context.Background(), livePending))
			fixture.pending = livePending
			_, err := fixture.reconciler.reconcileResourceFallback(context.Background(), fixture.ddai, fixture.ds, intstr.FromInt(1))
			require.NoError(t, err)
			assert.NoError(t, fixture.client.Get(context.Background(), client.ObjectKeyFromObject(fixture.old), &corev1.Pod{}))
		})
	}
}

func TestResourceOnlyUnschedulableParsing(t *testing.T) {
	pod := &corev1.Pod{Status: corev1.PodStatus{Conditions: []corev1.PodCondition{{
		Type: corev1.PodScheduled, Status: corev1.ConditionFalse, Reason: corev1.PodReasonUnschedulable,
		Message: "0/3 nodes are available: 1 Insufficient cpu, 2 node(s) didn't match Pod's node affinity/selector. preemption: 0/3 nodes are available",
	}}}}
	shortage, ok := resourceOnlyUnschedulable(pod)
	assert.True(t, ok)
	assert.True(t, shortage.cpu)
	assert.False(t, shortage.memory)
	pod.Status.Conditions[0].Message = "0/1 nodes are available: 1 node(s) had untolerated taint."
	_, ok = resourceOnlyUnschedulable(pod)
	assert.False(t, ok)
}

func TestResourceFallbackRejectsExistingPodAntiAffinity(t *testing.T) {
	fixture := newResourceFallbackFixture(t)
	blocker := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "blocker", Namespace: fixture.pending.Namespace, Labels: map[string]string{"app": "other"}},
		Spec: corev1.PodSpec{
			NodeName: "node-a",
			Affinity: &corev1.Affinity{PodAntiAffinity: &corev1.PodAntiAffinity{RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{{
				LabelSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "agent"}},
				TopologyKey:   corev1.LabelHostname,
			}}}},
			Containers: []corev1.Container{{Name: "blocker"}},
		},
	}
	require.NoError(t, fixture.client.Create(context.Background(), blocker))

	_, err := fixture.reconciler.reconcileResourceFallback(context.Background(), fixture.ddai, fixture.ds, intstr.FromInt(1))
	require.NoError(t, err)
	assert.NoError(t, fixture.client.Get(context.Background(), client.ObjectKeyFromObject(fixture.old), &corev1.Pod{}))
}

func TestConsumedResourceFallbackBudgetCountsDisjointUnavailableNodes(t *testing.T) {
	fixture := newResourceFallbackFixture(t)
	fixture.ds.Status.DesiredNumberScheduled = 2
	fixture.ds.Status.NumberUnavailable = 1
	fixture.pending.Annotations = map[string]string{resourceFallbackOldPodAnnotation: string(fixture.old.UID)}
	unavailable := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "unavailable", Namespace: "default", Labels: map[string]string{appsv1.DefaultDaemonSetUniqueLabelKey: "old"}},
		Spec:       corev1.PodSpec{NodeName: "node-b"},
		Status:     corev1.PodStatus{Phase: corev1.PodPending},
	}

	consumed := consumedResourceFallbackBudget(fixture.ds, []corev1.Pod{*fixture.old, *fixture.pending, unavailable}, "new", time.Now())
	assert.Equal(t, 2, consumed)
}

type resourceFallbackFixture struct {
	client     client.Client
	reconciler *Reconciler
	ddai       *datadoghqv1alpha1.DatadogAgentInternal
	ds         *appsv1.DaemonSet
	old        *corev1.Pod
	pending    *corev1.Pod
}

func newResourceFallbackFixture(t *testing.T) resourceFallbackFixture {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, datadoghqv1alpha1.AddToScheme(scheme))

	ddai := &datadoghqv1alpha1.DatadogAgentInternal{ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: "default", UID: "ddai-uid"}}
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: "default", UID: "ds-uid", Generation: 2, OwnerReferences: []metav1.OwnerReference{{
			APIVersion: datadoghqv1alpha1.GroupVersion.String(), Kind: "DatadogAgentInternal", Name: ddai.Name, UID: ddai.UID, Controller: ptr.To(true),
		}}},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "agent"}},
			Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{"app": "agent"}, Annotations: map[string]string{preparedRolloutModeAnnotation: preparedRolloutModeV1},
			}},
			UpdateStrategy: appsv1.DaemonSetUpdateStrategy{Type: appsv1.RollingUpdateDaemonSetStrategyType, RollingUpdate: &appsv1.RollingUpdateDaemonSet{
				MaxSurge: ptr.To(intstr.FromInt(1)), MaxUnavailable: ptr.To(intstr.FromInt(0)),
			}},
		},
		Status: appsv1.DaemonSetStatus{ObservedGeneration: 2, DesiredNumberScheduled: 1, NumberReady: 1, NumberAvailable: 1},
	}
	owner := metav1.OwnerReference{APIVersion: appsv1.SchemeGroupVersion.String(), Kind: "DaemonSet", Name: ds.Name, UID: ds.UID, Controller: ptr.To(true)}
	old := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "old", Namespace: "default", UID: "old-uid", Labels: map[string]string{"app": "agent", appsv1.DefaultDaemonSetUniqueLabelKey: "old"}, OwnerReferences: []metav1.OwnerReference{owner}},
		Spec:       corev1.PodSpec{NodeName: "node-a", Containers: []corev1.Container{{Name: "agent", Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("1")}}}}},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning, Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue, LastTransitionTime: metav1.NewTime(time.Now().Add(-time.Minute))}}},
	}
	pending := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "new", Namespace: "default", UID: "new-uid", Labels: map[string]string{"app": "agent", appsv1.DefaultDaemonSetUniqueLabelKey: "new"}, OwnerReferences: []metav1.OwnerReference{owner}},
		Spec: corev1.PodSpec{
			Affinity:   &corev1.Affinity{NodeAffinity: &corev1.NodeAffinity{RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{NodeSelectorTerms: []corev1.NodeSelectorTerm{{MatchFields: []corev1.NodeSelectorRequirement{{Key: metav1.ObjectNameField, Operator: corev1.NodeSelectorOpIn, Values: []string{"node-a"}}}}}}}},
			Containers: []corev1.Container{{Name: "agent", Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("1")}}}},
		},
		Status: corev1.PodStatus{Conditions: []corev1.PodCondition{{Type: corev1.PodScheduled, Status: corev1.ConditionFalse, Reason: corev1.PodReasonUnschedulable, Message: "0/1 nodes are available: 1 Insufficient cpu."}}},
	}
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a"},
		Status: corev1.NodeStatus{
			Allocatable: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("1500m"), corev1.ResourceMemory: resource.MustParse("1Gi"), corev1.ResourcePods: resource.MustParse("10")},
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
				{Type: corev1.NodeMemoryPressure, Status: corev1.ConditionFalse},
				{Type: corev1.NodeDiskPressure, Status: corev1.ConditionFalse},
				{Type: corev1.NodePIDPressure, Status: corev1.ConditionFalse},
				{Type: corev1.NodeNetworkUnavailable, Status: corev1.ConditionFalse},
			},
		},
	}
	revision := controllerRevisionForFallbackTest(t, ds, "new")
	c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&corev1.Pod{}, &appsv1.DaemonSet{}).WithObjects(ddai, ds, old, pending, node, revision).WithIndex(&corev1.Pod{}, "spec.nodeName", func(obj client.Object) []string {
		pod := obj.(*corev1.Pod)
		if pod.Spec.NodeName == "" {
			return nil
		}
		return []string{pod.Spec.NodeName}
	}).Build()
	return resourceFallbackFixture{client: c, reconciler: &Reconciler{client: c, apiReader: c}, ddai: ddai, ds: ds, old: old, pending: pending}
}

func controllerRevisionForFallbackTest(t *testing.T, ds *appsv1.DaemonSet, hash string) *appsv1.ControllerRevision {
	t.Helper()
	template, err := json.Marshal(ds.Spec.Template)
	require.NoError(t, err)
	var patch map[string]any
	require.NoError(t, json.Unmarshal(template, &patch))
	patch["$patch"] = "replace"
	raw, err := json.Marshal(map[string]any{"spec": map[string]any{"template": patch}})
	require.NoError(t, err)
	return &appsv1.ControllerRevision{
		ObjectMeta: metav1.ObjectMeta{Name: "agent-new", Namespace: ds.Namespace, UID: types.UID("revision-uid"), Labels: map[string]string{appsv1.DefaultDaemonSetUniqueLabelKey: hash}, OwnerReferences: []metav1.OwnerReference{{
			APIVersion: appsv1.SchemeGroupVersion.String(), Kind: "DaemonSet", Name: ds.Name, UID: ds.UID, Controller: ptr.To(true),
		}}},
		Revision: 2,
		Data:     runtime.RawExtension{Raw: raw},
	}
}
