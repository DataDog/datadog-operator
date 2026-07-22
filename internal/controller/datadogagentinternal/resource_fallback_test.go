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

func TestConfigureResourceFallback(t *testing.T) {
	tests := []struct {
		name            string
		strategyType    appsv1.DaemonSetUpdateStrategyType
		maxSurge        *intstr.IntOrString
		budget          intstr.IntOrString
		enabled         bool
		wantMaxSurge    *intstr.IntOrString
		wantUnavailable *intstr.IntOrString
	}{
		{
			name:            "surge is bounded by the existing percentage budget",
			maxSurge:        ptr.To(intstr.FromString("100%")),
			budget:          intstr.FromString("20%"),
			enabled:         true,
			wantMaxSurge:    ptr.To(intstr.FromString("20%")),
			wantUnavailable: ptr.To(intstr.FromInt(0)),
		},
		{
			name:            "absolute budget",
			maxSurge:        ptr.To(intstr.FromInt(20)),
			budget:          intstr.FromInt(2),
			enabled:         true,
			wantMaxSurge:    ptr.To(intstr.FromInt(2)),
			wantUnavailable: ptr.To(intstr.FromInt(0)),
		},
		{
			name:         "surge remains opt in",
			budget:       intstr.FromInt(1),
			wantMaxSurge: nil,
		},
		{
			name:         "on delete is untouched",
			strategyType: appsv1.OnDeleteDaemonSetStrategyType,
			maxSurge:     ptr.To(intstr.FromInt(1)),
			budget:       intstr.FromInt(1),
			wantMaxSurge: ptr.To(intstr.FromInt(1)),
		},
		{
			name:            "zero budget disables fallback but preserves requested surge",
			maxSurge:        ptr.To(intstr.FromInt(3)),
			budget:          intstr.FromInt(0),
			wantMaxSurge:    ptr.To(intstr.FromInt(3)),
			wantUnavailable: ptr.To(intstr.FromInt(0)),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ds := &appsv1.DaemonSet{Spec: appsv1.DaemonSetSpec{UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
				Type:          tt.strategyType,
				RollingUpdate: &appsv1.RollingUpdateDaemonSet{MaxSurge: tt.maxSurge},
			}}}
			assert.Equal(t, tt.enabled, configureResourceFallback(ds, tt.budget))
			assert.Equal(t, tt.wantMaxSurge, ds.Spec.UpdateStrategy.RollingUpdate.MaxSurge)
			assert.Equal(t, tt.wantUnavailable, ds.Spec.UpdateStrategy.RollingUpdate.MaxUnavailable)
		})
	}
}

func TestResourceOnlyUnschedulable(t *testing.T) {
	tests := []struct {
		name    string
		reason  string
		message string
		want    resourceShortage
		ok      bool
	}{
		{name: "cpu", reason: corev1.PodReasonUnschedulable, message: "0/1 nodes are available: 1 Insufficient cpu.", want: resourceShortage{cpu: true}, ok: true},
		{name: "memory and pinning affinity", reason: corev1.PodReasonUnschedulable, message: "0/3 nodes are available: 1 Insufficient memory, 2 node(s) didn't satisfy plugin(s) [NodeAffinity]. preemption: 0/3 nodes are available: 3 Preemption is not helpful for scheduling.", want: resourceShortage{memory: true}, ok: true},
		{name: "cpu and memory", reason: corev1.PodReasonUnschedulable, message: "0/1 nodes are available: 1 Insufficient cpu, 1 Insufficient memory.", want: resourceShortage{cpu: true, memory: true}, ok: true},
		{name: "taint is rejected", reason: corev1.PodReasonUnschedulable, message: "0/1 nodes are available: 1 Insufficient cpu, 1 node(s) had untolerated taint.", ok: false},
		{name: "host port is rejected", reason: corev1.PodReasonUnschedulable, message: "0/1 nodes are available: 1 Insufficient cpu, 1 node(s) didn't have free ports for the requested pod ports.", ok: false},
		{name: "ephemeral storage is rejected", reason: corev1.PodReasonUnschedulable, message: "0/1 nodes are available: 1 Insufficient ephemeral-storage.", ok: false},
		{name: "custom reason containing cpu text is rejected", reason: corev1.PodReasonUnschedulable, message: "0/1 nodes are available: 1 custom plugin: Insufficient cpu.", ok: false},
		{name: "wrong condition reason", reason: "SchedulingGated", message: "0/1 nodes are available: 1 Insufficient cpu.", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pod := &corev1.Pod{Status: corev1.PodStatus{Conditions: []corev1.PodCondition{{Type: corev1.PodScheduled, Status: corev1.ConditionFalse, Reason: tt.reason, Message: tt.message}}}}
			got, ok := resourceOnlyUnschedulable(pod)
			assert.Equal(t, tt.ok, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestTargetNodeFromDaemonSetAffinity(t *testing.T) {
	pod := pendingPodForNode("node-a")
	got, ok := targetNodeFromDaemonSetAffinity(pod)
	require.True(t, ok)
	assert.Equal(t, "node-a", got)

	ambiguous := pendingPodForNode("node-a")
	ambiguous.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms = append(
		ambiguous.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms,
		corev1.NodeSelectorTerm{MatchFields: []corev1.NodeSelectorRequirement{{Key: metav1.ObjectNameField, Operator: corev1.NodeSelectorOpIn, Values: []string{"node-b"}}}},
	)
	_, ok = targetNodeFromDaemonSetAffinity(ambiguous)
	assert.False(t, ok)
}

func TestResourceFitAfterOldPodRemoval(t *testing.T) {
	node := &corev1.Node{Status: corev1.NodeStatus{Allocatable: corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("1500m"),
		corev1.ResourceMemory: resource.MustParse("1Gi"),
		corev1.ResourcePods:   resource.MustParse("10"),
	}}}
	old := scheduledResourcePod("old", "old-uid", "node-a", "1", "128Mi")
	replacement := scheduledResourcePod("new", "new-uid", "", "1", "128Mi")

	assert.True(t, resourceFitAfterOldPodRemoval(node, []corev1.Pod{*old}, replacement, old, resourceShortage{cpu: true}))

	tooLarge := replacement.DeepCopy()
	tooLarge.Spec.Containers[0].Resources.Requests[corev1.ResourceCPU] = resource.MustParse("2")
	assert.False(t, resourceFitAfterOldPodRemoval(node, []corev1.Pod{*old}, tooLarge, old, resourceShortage{cpu: true}), "replacement must fit after old exits")

	noCPUOld := old.DeepCopy()
	noCPUOld.Spec.Containers[0].Resources.Requests[corev1.ResourceCPU] = resource.MustParse("0")
	assert.False(t, resourceFitAfterOldPodRemoval(node, []corev1.Pod{*noCPUOld}, replacement, noCPUOld, resourceShortage{cpu: true}), "old Pod must contribute to the shortage")

	staleMessage := node.DeepCopy()
	staleMessage.Status.Allocatable[corev1.ResourceCPU] = resource.MustParse("3")
	assert.False(t, resourceFitAfterOldPodRemoval(staleMessage, []corev1.Pod{*old}, replacement, old, resourceShortage{cpu: true}), "reported shortage must still be observable")
}

func TestSchedulerPodRequestsIncludesInitAndOverhead(t *testing.T) {
	pod := scheduledResourcePod("pod", "uid", "node-a", "250m", "100Mi")
	pod.Spec.InitContainers = []corev1.Container{{Name: "init", Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("1")}}}}
	pod.Spec.Overhead = corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m")}
	requests := schedulerPodRequests(pod)
	assert.Equal(t, int64(1100), requests.Cpu().MilliValue())
}

func TestConsumedFallbackBudget(t *testing.T) {
	now := time.Now()
	ds := &appsv1.DaemonSet{Spec: appsv1.DaemonSetSpec{MinReadySeconds: 0}, Status: appsv1.DaemonSetStatus{NumberUnavailable: 1}}
	old := readyPod("old", "old-uid", "node-a", "old", now.Add(-time.Minute))
	reserved := pendingPodForNode("node-a")
	reserved.Name = "new"
	reserved.Labels = map[string]string{appsv1.DefaultDaemonSetUniqueLabelKey: "new"}
	reserved.Annotations = map[string]string{resourceFallbackOldPodAnnotation: "old-uid"}

	assert.Equal(t, 2, consumedFallbackBudget(ds, []corev1.Pod{*old, *reserved}, "new", now), "reservation is separate while old remains available")
	old.DeletionTimestamp = &metav1.Time{Time: now}
	assert.Equal(t, 1, consumedFallbackBudget(ds, []corev1.Pod{*old, *reserved}, "new", now), "reservation overlaps status once its node is unavailable")
}

func TestReconcileResourceFallbackDeletesOnlyResourceBlockingOldPod(t *testing.T) {
	fixture := newFallbackTestFixture(t, healthyNodeConditions())
	result, err := fixture.reconciler.reconcileResourceFallback(context.Background(), fixture.ddai, fixture.ds, intstr.FromInt(1))
	require.NoError(t, err)
	assert.Equal(t, time.Second, result.RequeueAfter)
	err = fixture.client.Get(context.Background(), client.ObjectKeyFromObject(fixture.old), &corev1.Pod{})
	assert.True(t, apierrors.IsNotFound(err), "old Pod should be deleted")
	updatedPending := &corev1.Pod{}
	require.NoError(t, fixture.client.Get(context.Background(), client.ObjectKeyFromObject(fixture.pending), updatedPending))
	assert.Equal(t, string(fixture.old.UID), updatedPending.Annotations[resourceFallbackOldPodAnnotation])
}

func TestReconcileResourceFallbackKeepsOldPodDuringNodePressure(t *testing.T) {
	conditions := healthyNodeConditions()
	for i := range conditions {
		if conditions[i].Type == corev1.NodeDiskPressure {
			conditions[i].Status = corev1.ConditionTrue
		}
	}
	fixture := newFallbackTestFixture(t, conditions)
	result, err := fixture.reconciler.reconcileResourceFallback(context.Background(), fixture.ddai, fixture.ds, intstr.FromInt(1))
	require.NoError(t, err)
	assert.Zero(t, result.RequeueAfter, "permanently ineligible candidates must not cause a one-second polling loop")
	require.NoError(t, fixture.client.Get(context.Background(), client.ObjectKeyFromObject(fixture.old), &corev1.Pod{}), "old Pod must remain during DiskPressure")
	updatedPending := &corev1.Pod{}
	require.NoError(t, fixture.client.Get(context.Background(), client.ObjectKeyFromObject(fixture.pending), updatedPending))
	assert.Empty(t, updatedPending.Annotations[resourceFallbackOldPodAnnotation], "ineligible fallback must not reserve budget")
}

func TestReconcileResourceFallbackKeepsOldPodForHiddenSchedulerConstraints(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*corev1.Pod)
	}{
		{
			name: "persistent volume",
			mutate: func(pod *corev1.Pod) {
				pod.Spec.Volumes = []corev1.Volume{{Name: "data", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "data"}}}}
			},
		},
		{
			name: "pod affinity",
			mutate: func(pod *corev1.Pod) {
				pod.Spec.Affinity.PodAffinity = &corev1.PodAffinity{RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{{TopologyKey: "kubernetes.io/hostname", LabelSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "peer"}}}}}
			},
		},
		{
			name: "declared host port",
			mutate: func(pod *corev1.Pod) {
				pod.Spec.Containers[0].Ports = []corev1.ContainerPort{{ContainerPort: 8126, HostPort: 8126}}
			},
		},
		{
			name: "topology spread",
			mutate: func(pod *corev1.Pod) {
				pod.Spec.TopologySpreadConstraints = []corev1.TopologySpreadConstraint{{MaxSkew: 1, TopologyKey: "kubernetes.io/hostname", WhenUnsatisfiable: corev1.DoNotSchedule, LabelSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "agent"}}}}
			},
		},
		{
			name: "custom scheduler",
			mutate: func(pod *corev1.Pod) {
				pod.Spec.SchedulerName = "custom-scheduler"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newFallbackTestFixture(t, healthyNodeConditions())
			pending := &corev1.Pod{}
			require.NoError(t, fixture.client.Get(context.Background(), client.ObjectKeyFromObject(fixture.pending), pending))
			tt.mutate(pending)
			require.NoError(t, fixture.client.Update(context.Background(), pending))
			_, err := fixture.reconciler.reconcileResourceFallback(context.Background(), fixture.ddai, fixture.ds, intstr.FromInt(1))
			require.NoError(t, err)
			require.NoError(t, fixture.client.Get(context.Background(), client.ObjectKeyFromObject(fixture.old), &corev1.Pod{}), "old Pod must remain")
			updatedPending := &corev1.Pod{}
			require.NoError(t, fixture.client.Get(context.Background(), client.ObjectKeyFromObject(fixture.pending), updatedPending))
			assert.Empty(t, updatedPending.Annotations[resourceFallbackOldPodAnnotation])
		})
	}
}

func TestReconcileResourceFallbackUsesLiveUnavailablePodsWhenStatusLags(t *testing.T) {
	fixture := newFallbackTestFixture(t, healthyNodeConditions())
	liveDS := &appsv1.DaemonSet{}
	require.NoError(t, fixture.client.Get(context.Background(), client.ObjectKeyFromObject(fixture.ds), liveDS))
	liveDS.Status.DesiredNumberScheduled = 2
	require.NoError(t, fixture.client.Status().Update(context.Background(), liveDS))

	unavailable := readyPod("unavailable", "unavailable-uid", "node-b", "old-revision", time.Now().Add(-time.Minute))
	unavailable.Namespace = fixture.ds.Namespace
	unavailable.Labels["app"] = "agent"
	unavailable.OwnerReferences = []metav1.OwnerReference{daemonSetOwner(fixture.ds)}
	unavailable.Status.Conditions[0].Status = corev1.ConditionFalse
	require.NoError(t, fixture.client.Create(context.Background(), unavailable))

	result, err := fixture.reconciler.reconcileResourceFallback(context.Background(), fixture.ddai, fixture.ds, intstr.FromInt(1))
	require.NoError(t, err)
	assert.Zero(t, result.RequeueAfter, "an already-consumed budget must not cause a one-second polling loop")
	require.NoError(t, fixture.client.Get(context.Background(), client.ObjectKeyFromObject(fixture.old), &corev1.Pod{}), "fallback must not exceed maxUnavailable while DaemonSet status lags")
}

func TestReconcileResourceFallbackRejectsForeignDaemonSet(t *testing.T) {
	fixture := newFallbackTestFixture(t, healthyNodeConditions())
	liveDS := &appsv1.DaemonSet{}
	require.NoError(t, fixture.client.Get(context.Background(), client.ObjectKeyFromObject(fixture.ds), liveDS))
	liveDS.OwnerReferences[0].UID = "foreign-ddai"
	require.NoError(t, fixture.client.Update(context.Background(), liveDS))

	_, err := fixture.reconciler.reconcileResourceFallback(context.Background(), fixture.ddai, fixture.ds, intstr.FromInt(1))
	require.NoError(t, err)
	require.NoError(t, fixture.client.Get(context.Background(), client.ObjectKeyFromObject(fixture.old), &corev1.Pod{}), "foreign DaemonSet Pods must never be deleted")
}

type fallbackTestFixture struct {
	client     client.Client
	reconciler *Reconciler
	ddai       *datadoghqv1alpha1.DatadogAgentInternal
	ds         *appsv1.DaemonSet
	old        *corev1.Pod
	pending    *corev1.Pod
}

func newFallbackTestFixture(t *testing.T, nodeConditions []corev1.NodeCondition) fallbackTestFixture {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, datadoghqv1alpha1.AddToScheme(scheme))

	ddai := &datadoghqv1alpha1.DatadogAgentInternal{ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: "default", UID: "ddai-uid"}}
	ds := testFallbackDaemonSet(t, ddai)
	old := readyPod("old", "old-uid", "node-a", "old-revision", time.Now().Add(-time.Minute))
	pending := pendingPodForNode("node-a")
	pending.ObjectMeta = metav1.ObjectMeta{Name: "new", Namespace: "default", UID: "new-uid", Labels: map[string]string{"app": "agent", appsv1.DefaultDaemonSetUniqueLabelKey: "new-revision"}, OwnerReferences: []metav1.OwnerReference{daemonSetOwner(ds)}}
	pending.Spec.Containers = []corev1.Container{{Name: "agent", Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("1")}}}}
	pending.Status.Conditions = []corev1.PodCondition{{Type: corev1.PodScheduled, Status: corev1.ConditionFalse, Reason: corev1.PodReasonUnschedulable, Message: "0/1 nodes are available: 1 Insufficient cpu."}}
	old.Namespace = "default"
	old.Labels["app"] = "agent"
	old.OwnerReferences = []metav1.OwnerReference{daemonSetOwner(ds)}
	old.Spec.Containers[0].Resources.Requests[corev1.ResourceCPU] = resource.MustParse("1")
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a"}, Status: corev1.NodeStatus{Allocatable: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("1500m"), corev1.ResourceMemory: resource.MustParse("1Gi"), corev1.ResourcePods: resource.MustParse("10")}, Conditions: nodeConditions}}
	revision := controllerRevisionForTemplate(t, ds, "new-revision")

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ddai, ds, old, pending, node, revision).WithIndex(&corev1.Pod{}, apiPodNodeNameField, func(obj client.Object) []string {
		pod := obj.(*corev1.Pod)
		if pod.Spec.NodeName == "" {
			return nil
		}
		return []string{pod.Spec.NodeName}
	}).Build()
	r := &Reconciler{client: c, apiReader: c}
	return fallbackTestFixture{client: c, reconciler: r, ddai: ddai, ds: ds, old: old, pending: pending}
}

func healthyNodeConditions() []corev1.NodeCondition {
	return []corev1.NodeCondition{
		{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
		{Type: corev1.NodeMemoryPressure, Status: corev1.ConditionFalse},
		{Type: corev1.NodeDiskPressure, Status: corev1.ConditionFalse},
		{Type: corev1.NodePIDPressure, Status: corev1.ConditionFalse},
		{Type: corev1.NodeNetworkUnavailable, Status: corev1.ConditionFalse},
	}
}

func testFallbackDaemonSet(t *testing.T, ddai *datadoghqv1alpha1.DatadogAgentInternal) *appsv1.DaemonSet {
	t.Helper()
	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: "default", UID: "ds-uid", Generation: 2, OwnerReferences: []metav1.OwnerReference{{APIVersion: datadoghqv1alpha1.GroupVersion.String(), Kind: "DatadogAgentInternal", Name: ddai.Name, UID: ddai.UID, Controller: ptr.To(true)}}},
		Spec: appsv1.DaemonSetSpec{
			Selector:       &metav1.LabelSelector{MatchLabels: map[string]string{"app": "agent"}},
			Template:       corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "agent"}}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "agent"}}}},
			UpdateStrategy: appsv1.DaemonSetUpdateStrategy{Type: appsv1.RollingUpdateDaemonSetStrategyType, RollingUpdate: &appsv1.RollingUpdateDaemonSet{MaxSurge: ptr.To(intstr.FromInt(1)), MaxUnavailable: ptr.To(intstr.FromInt(0))}},
		},
		Status: appsv1.DaemonSetStatus{ObservedGeneration: 2, DesiredNumberScheduled: 1},
	}
}

func controllerRevisionForTemplate(t *testing.T, ds *appsv1.DaemonSet, hash string) *appsv1.ControllerRevision {
	t.Helper()
	templateJSON, err := json.Marshal(ds.Spec.Template)
	require.NoError(t, err)
	var templatePatch map[string]any
	require.NoError(t, json.Unmarshal(templateJSON, &templatePatch))
	templatePatch["$patch"] = "replace"
	data, err := json.Marshal(map[string]any{"spec": map[string]any{"template": templatePatch}})
	require.NoError(t, err)
	return &appsv1.ControllerRevision{ObjectMeta: metav1.ObjectMeta{Name: "agent-" + hash, Namespace: ds.Namespace, UID: types.UID("revision-uid"), Labels: map[string]string{appsv1.DefaultDaemonSetUniqueLabelKey: hash}, OwnerReferences: []metav1.OwnerReference{daemonSetOwner(ds)}}, Revision: 2, Data: runtime.RawExtension{Raw: data}}
}

func daemonSetOwner(ds *appsv1.DaemonSet) metav1.OwnerReference {
	return metav1.OwnerReference{APIVersion: appsv1.SchemeGroupVersion.String(), Kind: "DaemonSet", Name: ds.Name, UID: ds.UID, Controller: ptr.To(true)}
}

func pendingPodForNode(nodeName string) *corev1.Pod {
	return &corev1.Pod{Spec: corev1.PodSpec{Affinity: &corev1.Affinity{NodeAffinity: &corev1.NodeAffinity{RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{NodeSelectorTerms: []corev1.NodeSelectorTerm{{MatchFields: []corev1.NodeSelectorRequirement{{Key: metav1.ObjectNameField, Operator: corev1.NodeSelectorOpIn, Values: []string{nodeName}}}}}}}}}}
}

func scheduledResourcePod(name, uid, nodeName, cpu, memory string) *corev1.Pod {
	return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: name, UID: types.UID(uid)}, Spec: corev1.PodSpec{NodeName: nodeName, Containers: []corev1.Container{{Name: "agent", Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse(cpu), corev1.ResourceMemory: resource.MustParse(memory)}}}}}}
}

func readyPod(name, uid, nodeName, revision string, readyAt time.Time) *corev1.Pod {
	pod := scheduledResourcePod(name, uid, nodeName, "100m", "128Mi")
	pod.Labels = map[string]string{appsv1.DefaultDaemonSetUniqueLabelKey: revision}
	pod.Status = corev1.PodStatus{Phase: corev1.PodRunning, Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue, LastTransitionTime: metav1.NewTime(readyAt)}}}
	return pod
}
