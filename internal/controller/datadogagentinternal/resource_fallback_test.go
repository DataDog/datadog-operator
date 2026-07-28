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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResourceFallbackDeletesOneOldPodForResourceOnlyFailure(t *testing.T) {
	fixture := newResourceFallbackFixture(t)
	result, err := fixture.reconciler.reconcilePreparedRollout(context.Background(), fixture.ddai, fixture.ds, intstr.FromInt(1))
	require.NoError(t, err)
	assert.Equal(t, time.Second, result.RequeueAfter)
	err = fixture.client.Get(context.Background(), client.ObjectKeyFromObject(fixture.old), &corev1.Pod{})
	assert.True(t, apierrors.IsNotFound(err))
	assert.NoError(t, fixture.client.Get(context.Background(), client.ObjectKeyFromObject(fixture.pending), &corev1.Pod{}))
}

func TestResourceFallbackIgnoresOtherSchedulingFailures(t *testing.T) {
	fixture := newResourceFallbackFixture(t)
	fixture.pending.Status.Conditions[0].Message = "0/1 nodes are available: 1 node(s) didn't have free ports for the requested pod ports, 1 Insufficient cpu."
	updatePodStatus(t, fixture.client, fixture.pending)

	result, err := fixture.reconciler.reconcilePreparedRollout(context.Background(), fixture.ddai, fixture.ds, intstr.FromInt(1))
	require.NoError(t, err)
	assert.Equal(t, resourceFallbackPollInterval, result.RequeueAfter)
	assert.NoError(t, fixture.client.Get(context.Background(), client.ObjectKeyFromObject(fixture.old), &corev1.Pod{}))
}

func TestResourceFallbackRespectsExistingUnavailableBudget(t *testing.T) {
	fixture := newResourceFallbackFixture(t)
	fixture.ds.Status.NumberUnavailable = 1
	updateDaemonSetStatus(t, fixture.client, fixture.ds)

	result, err := fixture.reconciler.reconcilePreparedRollout(context.Background(), fixture.ddai, fixture.ds, intstr.FromInt(1))
	require.NoError(t, err)
	assert.Equal(t, resourceFallbackPollInterval, result.RequeueAfter)
	assert.NoError(t, fixture.client.Get(context.Background(), client.ObjectKeyFromObject(fixture.old), &corev1.Pod{}))
}

func TestResourceFallbackRequiresAnOldRevisionOnTheTargetNode(t *testing.T) {
	fixture := newResourceFallbackFixture(t)
	fixture.old.Labels[appsv1.DefaultDaemonSetUniqueLabelKey] = "new-hash"
	require.NoError(t, fixture.client.Update(context.Background(), fixture.old))

	result, err := fixture.reconciler.reconcilePreparedRollout(context.Background(), fixture.ddai, fixture.ds, intstr.FromInt(1))
	require.NoError(t, err)
	assert.Equal(t, resourceFallbackPollInterval, result.RequeueAfter)
	assert.NoError(t, fixture.client.Get(context.Background(), client.ObjectKeyFromObject(fixture.old), &corev1.Pod{}))
}

func TestPreparedRolloutDeletionRechecksLiveBudgetAndGeneration(t *testing.T) {
	fixture := newResourceFallbackFixture(t)

	allowed, err := preparedRolloutDeletionAllowed(context.Background(), fixture.client, fixture.ds, 1, "new-hash")
	require.NoError(t, err)
	assert.True(t, allowed)

	fixture.ds.Status.NumberUnavailable = 1
	updateDaemonSetStatus(t, fixture.client, fixture.ds)
	allowed, err = preparedRolloutDeletionAllowed(context.Background(), fixture.client, fixture.ds, 1, "new-hash")
	require.NoError(t, err)
	assert.False(t, allowed)

	fixture.ds.Status.NumberUnavailable = 0
	fixture.ds.Status.ObservedGeneration--
	updateDaemonSetStatus(t, fixture.client, fixture.ds)
	allowed, err = preparedRolloutDeletionAllowed(context.Background(), fixture.client, fixture.ds, 1, "new-hash")
	require.NoError(t, err)
	assert.False(t, allowed)
}

func TestResourceFallbackStopsPollingAfterRollout(t *testing.T) {
	fixture := newResourceFallbackFixture(t)
	fixture.ds.Status.UpdatedNumberScheduled = 1
	updateDaemonSetStatus(t, fixture.client, fixture.ds)
	fixture.pending.Status.Conditions[0].Message = "0/1 nodes are available: 1 node(s) had untolerated taint."
	updatePodStatus(t, fixture.client, fixture.pending)

	result, err := fixture.reconciler.reconcilePreparedRollout(context.Background(), fixture.ddai, fixture.ds, intstr.FromInt(1))
	require.NoError(t, err)
	assert.Zero(t, result.RequeueAfter)
}

func TestPreparedRolloutPollsUntilDaemonSetStatusObservesTemplate(t *testing.T) {
	fixture := newResourceFallbackFixture(t)
	fixture.ds.Status.ObservedGeneration--
	updateDaemonSetStatus(t, fixture.client, fixture.ds)

	result, err := fixture.reconciler.reconcilePreparedRollout(context.Background(), fixture.ddai, fixture.ds, intstr.FromInt(1))
	require.NoError(t, err)
	assert.Equal(t, resourceFallbackPollInterval, result.RequeueAfter)
}

func TestConsumedFallbackBudgetCountsTerminatingNodes(t *testing.T) {
	fixture := newResourceFallbackFixture(t)
	fixture.ds.Status.NumberUnavailable = 1
	now := metav1.Now()
	terminatingA := fixture.old.DeepCopy()
	terminatingA.DeletionTimestamp = &now
	terminatingB := fixture.old.DeepCopy()
	terminatingB.Name = "old-b"
	terminatingB.UID = "old-b-uid"
	terminatingB.Spec.NodeName = "node-b"
	terminatingB.DeletionTimestamp = &now
	assert.Equal(t, 3, consumedPreparedRolloutBudget(fixture.ds, []corev1.Pod{*terminatingA, *terminatingB}))
}

func TestPreparedRolloutDeletesOldPodAfterReplacementContainersAreRunning(t *testing.T) {
	fixture := newResourceFallbackFixture(t)
	replacement := fixture.pending.DeepCopy()
	replacement.Spec.NodeName = "node-a"
	replacement.Spec.Affinity = nil
	replacement.Labels[appsv1.DefaultDaemonSetUniqueLabelKey] = "new-hash"
	replacement.Status = corev1.PodStatus{
		Phase:      corev1.PodRunning,
		Conditions: []corev1.PodCondition{{Type: corev1.PodInitialized, Status: corev1.ConditionTrue}},
		ContainerStatuses: []corev1.ContainerStatus{{
			Name: "agent", Image: "agent:new", ImageID: "sha256:new", ContainerID: "containerd://new",
			State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{StartedAt: metav1.Now()}},
		}},
	}
	require.NoError(t, fixture.client.Delete(context.Background(), fixture.pending))
	replacement.ResourceVersion = ""
	require.NoError(t, fixture.client.Create(context.Background(), replacement))
	require.NoError(t, fixture.client.Status().Update(context.Background(), replacement))

	result, err := fixture.reconciler.reconcilePreparedRollout(context.Background(), fixture.ddai, fixture.ds, intstr.FromInt(1))
	require.NoError(t, err)
	assert.Equal(t, time.Second, result.RequeueAfter)
	err = fixture.client.Get(context.Background(), client.ObjectKeyFromObject(fixture.old), &corev1.Pod{})
	assert.True(t, apierrors.IsNotFound(err))
}

func TestPreparedRolloutRequiresEveryReplacementContainerToBePristineAndRunning(t *testing.T) {
	tests := map[string]func(*corev1.Pod){
		"not initialized": func(p *corev1.Pod) { p.Status.Conditions = nil },
		"container terminated": func(p *corev1.Pod) {
			p.Status.ContainerStatuses[0].State = corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{}}
		},
		"container restarted":  func(p *corev1.Pod) { p.Status.ContainerStatuses[0].RestartCount = 1 },
		"missing container id": func(p *corev1.Pod) { p.Status.ContainerStatuses[0].ContainerID = "" },
		"missing image id":     func(p *corev1.Pod) { p.Status.ContainerStatuses[0].ImageID = "" },
		"wrong image":          func(p *corev1.Pod) { p.Spec.Containers[0].Image = "agent:wrong" },
		"wrong revision":       func(p *corev1.Pod) { p.Labels[appsv1.DefaultDaemonSetUniqueLabelKey] = "old-hash" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			fixture := newResourceFallbackFixture(t)
			replacement := runningReplacement(fixture)
			mutate(replacement)
			require.False(t, replacementRunningForHandoff(replacement, fixture.ds, "new-hash"))
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
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "agent"}, Annotations: map[string]string{preparedRolloutModeAnnotation: preparedRolloutModeV1}},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "agent", Image: "agent:new"}}},
			},
			UpdateStrategy: appsv1.DaemonSetUpdateStrategy{Type: appsv1.RollingUpdateDaemonSetStrategyType, RollingUpdate: &appsv1.RollingUpdateDaemonSet{MaxSurge: ptr.To(intstr.FromInt(1)), MaxUnavailable: ptr.To(intstr.FromInt(0))}},
		},
		Status: appsv1.DaemonSetStatus{ObservedGeneration: 2, DesiredNumberScheduled: 1, NumberReady: 1, NumberAvailable: 1},
	}
	owner := metav1.OwnerReference{APIVersion: appsv1.SchemeGroupVersion.String(), Kind: "DaemonSet", Name: ds.Name, UID: ds.UID, Controller: ptr.To(true)}
	old := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "old", Namespace: "default", UID: "old-uid", Labels: map[string]string{"app": "agent", appsv1.DefaultDaemonSetUniqueLabelKey: "old-hash"}, OwnerReferences: []metav1.OwnerReference{owner}},
		Spec:       corev1.PodSpec{NodeName: "node-a", Containers: []corev1.Container{{Name: "agent", Image: "agent:old"}}},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning, Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue, LastTransitionTime: metav1.NewTime(time.Now().Add(-time.Minute))}}},
	}
	pending := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "new", Namespace: "default", UID: "new-uid", Labels: map[string]string{"app": "agent", appsv1.DefaultDaemonSetUniqueLabelKey: "new-hash"}, OwnerReferences: []metav1.OwnerReference{owner}},
		Spec: corev1.PodSpec{
			Affinity:   &corev1.Affinity{NodeAffinity: &corev1.NodeAffinity{RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{NodeSelectorTerms: []corev1.NodeSelectorTerm{{MatchFields: []corev1.NodeSelectorRequirement{{Key: metav1.ObjectNameField, Operator: corev1.NodeSelectorOpIn, Values: []string{"node-a"}}}}}}}},
			Containers: []corev1.Container{{Name: "agent", Image: "agent:new"}},
		},
		Status: corev1.PodStatus{Conditions: []corev1.PodCondition{{Type: corev1.PodScheduled, Status: corev1.ConditionFalse, Reason: corev1.PodReasonUnschedulable, Message: "0/1 nodes are available: 1 Insufficient cpu."}}},
	}
	revisionData, err := json.Marshal(struct {
		Spec struct {
			Template corev1.PodTemplateSpec `json:"template"`
		} `json:"spec"`
	}{Spec: struct {
		Template corev1.PodTemplateSpec `json:"template"`
	}{Template: ds.Spec.Template}})
	require.NoError(t, err)
	revision := &appsv1.ControllerRevision{
		ObjectMeta: metav1.ObjectMeta{
			Name: "agent-new-hash", Namespace: ds.Namespace,
			Labels:          map[string]string{appsv1.DefaultDaemonSetUniqueLabelKey: "new-hash"},
			OwnerReferences: []metav1.OwnerReference{owner},
		},
		Data:     runtime.RawExtension{Raw: revisionData},
		Revision: 2,
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&corev1.Pod{}, &appsv1.DaemonSet{}).WithObjects(ddai, ds, old, pending, revision).Build()
	return resourceFallbackFixture{client: c, reconciler: &Reconciler{client: c, apiReader: c}, ddai: ddai, ds: ds, old: old, pending: pending}
}

func runningReplacement(fixture resourceFallbackFixture) *corev1.Pod {
	pod := fixture.pending.DeepCopy()
	pod.Spec.NodeName = "node-a"
	pod.Spec.Affinity = nil
	pod.Status = corev1.PodStatus{
		Phase:      corev1.PodRunning,
		Conditions: []corev1.PodCondition{{Type: corev1.PodInitialized, Status: corev1.ConditionTrue}},
		ContainerStatuses: []corev1.ContainerStatus{{
			Name: "agent", Image: "agent:new", ImageID: "sha256:new", ContainerID: "containerd://new",
			State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{StartedAt: metav1.Now()}},
		}},
	}
	return pod
}

func updatePodStatus(t *testing.T, c client.Client, pod *corev1.Pod) {
	t.Helper()
	live := &corev1.Pod{}
	require.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(pod), live))
	live.Status = *pod.Status.DeepCopy()
	require.NoError(t, c.Status().Update(context.Background(), live))
}

func updateDaemonSetStatus(t *testing.T, c client.Client, ds *appsv1.DaemonSet) {
	t.Helper()
	live := &appsv1.DaemonSet{}
	require.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(ds), live))
	live.Status = ds.Status
	require.NoError(t, c.Status().Update(context.Background(), live))
}
