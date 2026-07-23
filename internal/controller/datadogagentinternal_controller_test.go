// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// Copyright 2016-present Datadog, Inc.

package controller

import (
	"context"
	"testing"

	edsdatadoghqv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	ctrlconfig "sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	componentagent "github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/agent"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type noopMetricsForwardersManager struct{}

func (noopMetricsForwardersManager) Register(client.Object)                     {}
func (noopMetricsForwardersManager) Unregister(client.Object)                   {}
func (noopMetricsForwardersManager) ProcessError(client.Object, error)          {}
func (noopMetricsForwardersManager) ProcessEvent(client.Object, datadog.Event)  {}
func (noopMetricsForwardersManager) SetEnabledFeatures(client.Object, []string) {}
func (noopMetricsForwardersManager) MetricsForwarderStatusForObj(client.Object) *datadog.ConditionCommon {
	return nil
}

func TestDatadogAgentInternalSetupWithManager(t *testing.T) {
	tests := []struct {
		name    string
		options datadogagentinternal.ReconcilerOptions
	}{
		{name: "default"},
		{
			name: "optional watches and metrics",
			options: datadogagentinternal.ReconcilerOptions{
				ExtendedDaemonsetOptions: componentagent.ExtendedDaemonsetOptions{Enabled: true},
				SupportCilium:            true,
				OperatorMetricsEnabled:   true,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			require.NoError(t, clientgoscheme.AddToScheme(scheme))
			require.NoError(t, datadoghqv1alpha1.AddToScheme(scheme))
			require.NoError(t, edsdatadoghqv1alpha1.AddToScheme(scheme))

			mgr, err := ctrl.NewManager(&rest.Config{}, manager.Options{
				Scheme:         scheme,
				LeaderElection: false,
				Controller: ctrlconfig.Controller{
					SkipNameValidation: ptr.To(true),
				},
			})
			require.NoError(t, err)

			reconciler := &DatadogAgentInternalReconciler{
				Client:       mgr.GetClient(),
				PlatformInfo: kubernetes.PlatformInfo{},
				Scheme:       scheme,
				Recorder:     mgr.GetEventRecorderFor("datadogagentinternal-test"),
				Options:      test.options,
			}
			require.NoError(t, reconciler.SetupWithManager(mgr, noopMetricsForwardersManager{}))
			require.NotNil(t, reconciler.internal)
		})
	}
}

func TestResourceFallbackPodPredicate(t *testing.T) {
	predicate := resourceFallbackPodPredicate()
	assert.False(t, predicate.Create(event.CreateEvent{Object: &corev1.ConfigMap{}}))
	assert.False(t, predicate.Create(event.CreateEvent{Object: &corev1.Pod{}}))
	oldPod := &corev1.Pod{Status: corev1.PodStatus{Conditions: []corev1.PodCondition{{Type: corev1.PodScheduled, Status: corev1.ConditionFalse, Reason: corev1.PodReasonUnschedulable, Message: "old"}}}}
	assert.True(t, predicate.Create(event.CreateEvent{Object: oldPod}))
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
	assert.False(t, predicate.Update(event.UpdateEvent{ObjectOld: &corev1.ConfigMap{}, ObjectNew: &corev1.ConfigMap{}}))
	assert.True(t, predicate.Delete(event.DeleteEvent{Object: oldPod}))
	assert.False(t, predicate.Generic(event.GenericEvent{Object: oldPod}))
}

func TestContainerRolloutStatusChanged(t *testing.T) {
	running := &corev1.ContainerStateRunning{}
	terminated := &corev1.ContainerStateTerminated{}
	tests := []struct {
		name string
		old  []corev1.ContainerStatus
		new  []corev1.ContainerStatus
		want bool
	}{
		{name: "identical empty", want: false},
		{name: "length", new: []corev1.ContainerStatus{{Name: "agent"}}, want: true},
		{name: "name", old: []corev1.ContainerStatus{{Name: "agent"}}, new: []corev1.ContainerStatus{{Name: "trace-agent"}}, want: true},
		{name: "started", old: []corev1.ContainerStatus{{Name: "agent"}}, new: []corev1.ContainerStatus{{Name: "agent", Started: ptr.To(false)}}, want: true},
		{name: "restart", old: []corev1.ContainerStatus{{Name: "agent"}}, new: []corev1.ContainerStatus{{Name: "agent", RestartCount: 1}}, want: true},
		{name: "running", old: []corev1.ContainerStatus{{Name: "agent"}}, new: []corev1.ContainerStatus{{Name: "agent", State: corev1.ContainerState{Running: running}}}, want: true},
		{name: "terminated", old: []corev1.ContainerStatus{{Name: "agent"}}, new: []corev1.ContainerStatus{{Name: "agent", State: corev1.ContainerState{Terminated: terminated}}}, want: true},
		{name: "identical", old: []corev1.ContainerStatus{{Name: "agent", Started: ptr.To(true), RestartCount: 1, State: corev1.ContainerState{Running: running}}}, new: []corev1.ContainerStatus{{Name: "agent", Started: ptr.To(true), RestartCount: 1, State: corev1.ContainerState{Running: running}}}, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, containerRolloutStatusChanged(test.old, test.new))
		})
	}
}

func TestResourceFallbackConditionChanged(t *testing.T) {
	empty := &corev1.Pod{}
	scheduled := &corev1.Pod{Status: corev1.PodStatus{Conditions: []corev1.PodCondition{{Type: corev1.PodScheduled, Status: corev1.ConditionFalse, Reason: "Unschedulable", Message: "cpu"}}}}
	assert.False(t, resourceFallbackConditionChanged(empty, empty, corev1.PodScheduled))
	assert.True(t, resourceFallbackConditionChanged(empty, scheduled, corev1.PodScheduled))
	assert.False(t, resourceFallbackConditionChanged(scheduled, scheduled.DeepCopy(), corev1.PodScheduled))
	for _, mutate := range []func(*corev1.PodCondition){
		func(condition *corev1.PodCondition) { condition.Status = corev1.ConditionTrue },
		func(condition *corev1.PodCondition) { condition.Reason = "Scheduled" },
		func(condition *corev1.PodCondition) { condition.Message = "memory" },
	} {
		changed := scheduled.DeepCopy()
		mutate(&changed.Status.Conditions[0])
		assert.True(t, resourceFallbackConditionChanged(scheduled, changed, corev1.PodScheduled))
	}
}

func TestDatadogAgentInternalEventPredicate(t *testing.T) {
	p := datadogAgentInternalEventPredicate()
	old := &datadoghqv1alpha1.DatadogAgentInternal{ObjectMeta: metav1.ObjectMeta{Generation: 1, Annotations: map[string]string{"example.com/ignored": "old"}}}

	generation := old.DeepCopy()
	generation.Generation = 2
	assert.True(t, p.Update(event.UpdateEvent{ObjectOld: old, ObjectNew: generation}))

	prepared := old.DeepCopy()
	prepared.Annotations["experimental.agent.datadoghq.com/host-network-surge-prepared"] = "true"
	assert.True(t, p.Update(event.UpdateEvent{ObjectOld: old, ObjectNew: prepared}))

	fallback := prepared.DeepCopy()
	fallback.Annotations["experimental.agent.datadoghq.com/resource-fallback"] = "true"
	assert.True(t, p.Update(event.UpdateEvent{ObjectOld: prepared, ObjectNew: fallback}))

	removed := fallback.DeepCopy()
	delete(removed.Annotations, "experimental.agent.datadoghq.com/resource-fallback")
	assert.True(t, p.Update(event.UpdateEvent{ObjectOld: fallback, ObjectNew: removed}))

	unrelated := old.DeepCopy()
	unrelated.Annotations["example.com/ignored"] = "new"
	assert.False(t, p.Update(event.UpdateEvent{ObjectOld: old, ObjectNew: unrelated}))
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

	assert.Empty(t, enqueueDatadogAgentInternalForPod(reader)(context.Background(), &corev1.ConfigMap{}))
	withoutLabel := pod.DeepCopy()
	withoutLabel.Labels = nil
	assert.Empty(t, enqueueDatadogAgentInternalForPod(reader)(context.Background(), withoutLabel))
	withoutOwner := pod.DeepCopy()
	withoutOwner.OwnerReferences = nil
	assert.Empty(t, enqueueDatadogAgentInternalForPod(reader)(context.Background(), withoutOwner))
	wrongOwnerKind := pod.DeepCopy()
	wrongOwnerKind.OwnerReferences[0].Kind = "Deployment"
	assert.Empty(t, enqueueDatadogAgentInternalForPod(reader)(context.Background(), wrongOwnerKind))
	missingDaemonSet := pod.DeepCopy()
	missingDaemonSet.OwnerReferences[0].Name = "missing"
	assert.Empty(t, enqueueDatadogAgentInternalForPod(reader)(context.Background(), missingDaemonSet))

	dsWithoutOwner := ds.DeepCopy()
	dsWithoutOwner.Name = "unowned-agent"
	dsWithoutOwner.UID = "unowned-ds-uid"
	dsWithoutOwner.OwnerReferences = nil
	unownedReader := fake.NewClientBuilder().WithScheme(scheme).WithObjects(dsWithoutOwner).Build()
	unownedPod := pod.DeepCopy()
	unownedPod.OwnerReferences[0].Name = dsWithoutOwner.Name
	unownedPod.OwnerReferences[0].UID = dsWithoutOwner.UID
	assert.Empty(t, enqueueDatadogAgentInternalForPod(unownedReader)(context.Background(), unownedPod))
}

func TestEnqueueIfOwnedByDatadogAgentInternal(t *testing.T) {
	unmanaged := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
		kubernetes.AppKubernetesPartOfLabelKey: "default-profile--ddai",
	}}}
	assert.Empty(t, enqueueIfOwnedByDatadogAgentInternal(context.Background(), unmanaged))

	managed := unmanaged.DeepCopy()
	managed.Labels[kubernetes.AppKubernetesManageByLabelKey] = "datadog-operator"
	requests := enqueueIfOwnedByDatadogAgentInternal(context.Background(), managed)
	require.Len(t, requests, 1)
	assert.Equal(t, "default", requests[0].Namespace)
	assert.Equal(t, "profile-ddai", requests[0].Name)
}
