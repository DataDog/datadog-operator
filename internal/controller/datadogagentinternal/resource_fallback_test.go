// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// Copyright 2016-present Datadog, Inc.

package datadogagentinternal

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	datadoghqcommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	componentagent "github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/agent"
	"github.com/DataDog/datadog-operator/pkg/constants"
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

func TestResourceFallbackBudgetPrecedence(t *testing.T) {
	overrideBudget := intstr.FromString("25%")
	ddai := &datadoghqv1alpha1.DatadogAgentInternal{Spec: datadoghqv2alpha1.DatadogAgentSpec{
		Override: map[datadoghqv2alpha1.ComponentName]*datadoghqv2alpha1.DatadogAgentComponentOverride{
			datadoghqv2alpha1.NodeAgentComponentName: {
				UpdateStrategy: &datadoghqcommon.UpdateStrategy{RollingUpdate: &datadoghqcommon.RollingUpdate{MaxUnavailable: &overrideBudget}},
			},
		},
	}}
	options := &componentagent.ExtendedDaemonsetOptions{MaxPodUnavailable: "2"}

	assert.Equal(t, overrideBudget, resourceFallbackBudget(ddai, options), "the DatadogAgent override is the requested rollout budget")
	ddai.Spec.Override = nil
	assert.Equal(t, intstr.FromInt(2), resourceFallbackBudget(ddai, options), "the Operator option is the compatibility fallback")
	assert.Equal(t, intstr.FromInt(defaultFallbackMaxUnavailable), resourceFallbackBudget(ddai, nil), "the fallback remains bounded when neither source is configured")
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
		{name: "empty primary reason", reason: corev1.PodReasonUnschedulable, message: "preemption:", ok: false},
		{name: "reason without count", reason: corev1.PodReasonUnschedulable, message: "0/1 nodes are available: cpu.", ok: false},
		{name: "non-numeric count", reason: corev1.PodReasonUnschedulable, message: "0/1 nodes are available: many Insufficient cpu.", ok: false},
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
	requirement := func(operator corev1.NodeSelectorOperator, values ...string) corev1.NodeSelectorRequirement {
		return corev1.NodeSelectorRequirement{Key: metav1.ObjectNameField, Operator: operator, Values: values}
	}
	podWithTerms := func(terms ...corev1.NodeSelectorTerm) *corev1.Pod {
		return &corev1.Pod{Spec: corev1.PodSpec{Affinity: &corev1.Affinity{NodeAffinity: &corev1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{NodeSelectorTerms: terms},
		}}}}
	}

	tests := []struct {
		name string
		pod  *corev1.Pod
		want string
		ok   bool
	}{
		{name: "daemonset target", pod: pendingPodForNode("node-a"), want: "node-a", ok: true},
		{name: "no affinity", pod: &corev1.Pod{}},
		{name: "no node affinity", pod: &corev1.Pod{Spec: corev1.PodSpec{Affinity: &corev1.Affinity{}}}},
		{name: "no required node affinity", pod: &corev1.Pod{Spec: corev1.PodSpec{Affinity: &corev1.Affinity{NodeAffinity: &corev1.NodeAffinity{}}}}},
		{name: "empty terms", pod: podWithTerms()},
		{name: "term without target field", pod: podWithTerms(corev1.NodeSelectorTerm{MatchFields: []corev1.NodeSelectorRequirement{{Key: "metadata.namespace", Operator: corev1.NodeSelectorOpIn, Values: []string{"datadog"}}}})},
		{name: "wrong operator", pod: podWithTerms(corev1.NodeSelectorTerm{MatchFields: []corev1.NodeSelectorRequirement{requirement(corev1.NodeSelectorOpNotIn, "node-a")}})},
		{name: "multiple values", pod: podWithTerms(corev1.NodeSelectorTerm{MatchFields: []corev1.NodeSelectorRequirement{requirement(corev1.NodeSelectorOpIn, "node-a", "node-b")}})},
		{name: "duplicate target field", pod: podWithTerms(corev1.NodeSelectorTerm{MatchFields: []corev1.NodeSelectorRequirement{requirement(corev1.NodeSelectorOpIn, "node-a"), requirement(corev1.NodeSelectorOpIn, "node-a")}})},
		{name: "consistent terms", pod: podWithTerms(
			corev1.NodeSelectorTerm{MatchFields: []corev1.NodeSelectorRequirement{requirement(corev1.NodeSelectorOpIn, "node-a")}},
			corev1.NodeSelectorTerm{MatchFields: []corev1.NodeSelectorRequirement{requirement(corev1.NodeSelectorOpIn, "node-a")}},
		), want: "node-a", ok: true},
		{name: "conflicting terms", pod: podWithTerms(
			corev1.NodeSelectorTerm{MatchFields: []corev1.NodeSelectorRequirement{requirement(corev1.NodeSelectorOpIn, "node-a")}},
			corev1.NodeSelectorTerm{MatchFields: []corev1.NodeSelectorRequirement{requirement(corev1.NodeSelectorOpIn, "node-b")}},
		)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := targetNodeFromDaemonSetAffinity(tt.pod)
			assert.Equal(t, tt.ok, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestPrepareProfileAntiAffinityForSurge(t *testing.T) {
	labels := map[string]string{
		datadoghqcommon.AgentDeploymentNameLabelKey: "datadog-agent",
		constants.ProfileLabelKey:                   "linux",
	}

	t.Run("no anti-affinity", func(t *testing.T) {
		for _, template := range []*corev1.PodTemplateSpec{
			{},
			{Spec: corev1.PodSpec{Affinity: &corev1.Affinity{}}},
		} {
			assert.True(t, prepareProfileAntiAffinityForSurge(template))
		}
	})

	t.Run("custom anti-affinity is rejected without mutation", func(t *testing.T) {
		template := &corev1.PodTemplateSpec{Spec: corev1.PodSpec{Affinity: &corev1.Affinity{PodAntiAffinity: &corev1.PodAntiAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{{TopologyKey: "topology.kubernetes.io/zone"}},
		}}}}
		original := template.DeepCopy()
		assert.False(t, prepareProfileAntiAffinityForSurge(template))
		assert.Equal(t, original, template)
	})

	t.Run("missing deployment identity is rejected", func(t *testing.T) {
		template := &corev1.PodTemplateSpec{Spec: corev1.PodSpec{Affinity: &corev1.Affinity{PodAntiAffinity: broadAgentPodAntiAffinity()}}}
		assert.False(t, prepareProfileAntiAffinityForSurge(template))
	})

	t.Run("standard affinity is narrowed", func(t *testing.T) {
		template := &corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: labels}, Spec: corev1.PodSpec{
			Affinity: &corev1.Affinity{PodAntiAffinity: broadAgentPodAntiAffinity()},
		}}
		expected, ok := profileSurgePodAntiAffinity(labels)
		require.True(t, ok)
		require.True(t, prepareProfileAntiAffinityForSurge(template))
		assert.Equal(t, expected, template.Spec.Affinity.PodAntiAffinity)
	})
}

func TestResourceFallbackSchedulingShapeAllowsOnlyProfileSurgeAntiAffinity(t *testing.T) {
	namedLabels := map[string]string{
		datadoghqcommon.AgentDeploymentNameLabelKey: "datadog-agent",
		constants.ProfileLabelKey:                   "linux",
	}
	namedAntiAffinity, ok := profileSurgePodAntiAffinity(namedLabels)
	require.True(t, ok)
	named := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Labels: namedLabels}, Spec: corev1.PodSpec{
		Affinity:   &corev1.Affinity{PodAntiAffinity: namedAntiAffinity},
		Containers: []corev1.Container{{Name: "agent"}},
	}}
	assert.True(t, resourceFallbackSchedulingShapeSafe(named))

	defaultProfile := named.DeepCopy()
	defaultProfile.Labels = map[string]string{datadoghqcommon.AgentDeploymentNameLabelKey: "datadog-agent"}
	defaultProfile.Spec.Affinity.PodAntiAffinity, ok = profileSurgePodAntiAffinity(defaultProfile.Labels)
	require.True(t, ok)
	assert.True(t, resourceFallbackSchedulingShapeSafe(defaultProfile))

	wrongProfile := named.DeepCopy()
	wrongProfile.Spec.Affinity.PodAntiAffinity, ok = profileSurgePodAntiAffinity(map[string]string{
		datadoghqcommon.AgentDeploymentNameLabelKey: "datadog-agent",
		constants.ProfileLabelKey:                   "gpu",
	})
	require.True(t, ok)
	assert.False(t, resourceFallbackSchedulingShapeSafe(wrongProfile))

	custom := named.DeepCopy()
	custom.Spec.Affinity.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution[0].TopologyKey = "topology.kubernetes.io/zone"
	assert.False(t, resourceFallbackSchedulingShapeSafe(custom))
}

func TestProfileSurgePodAntiAffinityIdentity(t *testing.T) {
	incomingLabels := map[string]string{
		datadoghqcommon.AgentDeploymentNameLabelKey: "datadog-agent",
		constants.ProfileLabelKey:                   "linux",
	}
	antiAffinity, ok := profileSurgePodAntiAffinity(incomingLabels)
	require.True(t, ok)

	conflicts := func(existingLabels map[string]string) bool {
		for _, term := range antiAffinity.RequiredDuringSchedulingIgnoredDuringExecution {
			selector, err := metav1.LabelSelectorAsSelector(term.LabelSelector)
			require.NoError(t, err)
			if selector.Matches(labels.Set(existingLabels)) {
				return true
			}
		}
		return false
	}

	assert.False(t, conflicts(map[string]string{
		datadoghqcommon.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix,
		datadoghqcommon.AgentDeploymentNameLabelKey:      "datadog-agent",
		constants.ProfileLabelKey:                        "linux",
	}), "old and new revisions of the same DDA profile may overlap")
	assert.True(t, conflicts(map[string]string{
		datadoghqcommon.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix,
		datadoghqcommon.AgentDeploymentNameLabelKey:      "datadog-agent",
		constants.ProfileLabelKey:                        "gpu",
	}), "another profile of the same DDA must remain excluded")
	assert.True(t, conflicts(map[string]string{
		datadoghqcommon.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix,
		datadoghqcommon.AgentDeploymentNameLabelKey:      "other-datadog-agent",
		constants.ProfileLabelKey:                        "linux",
	}), "the same profile name from another DDA must remain excluded")
}

func TestProfileSurgePodAntiAffinitySatisfiedOnTargetNode(t *testing.T) {
	pendingLabels := map[string]string{
		datadoghqcommon.AgentDeploymentNameLabelKey:      "datadog-agent",
		datadoghqcommon.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix,
		constants.ProfileLabelKey:                        "linux",
	}
	antiAffinity, ok := profileSurgePodAntiAffinity(pendingLabels)
	require.True(t, ok)
	pending := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "datadog", Labels: pendingLabels}, Spec: corev1.PodSpec{
		Affinity: &corev1.Affinity{PodAntiAffinity: antiAffinity},
	}}
	sameProfile := corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "datadog", Labels: maps.Clone(pendingLabels)}}

	satisfied, err := profileSurgePodAntiAffinitySatisfied(pending, []corev1.Pod{sameProfile})
	require.NoError(t, err)
	assert.True(t, satisfied)

	otherProfile := sameProfile.DeepCopy()
	otherProfile.Labels[constants.ProfileLabelKey] = "gpu"
	satisfied, err = profileSurgePodAntiAffinitySatisfied(pending, []corev1.Pod{sameProfile, *otherProfile})
	require.NoError(t, err)
	assert.False(t, satisfied, "a masked different-profile blocker must prevent old Pod deletion")

	otherProfile.Namespace = "another-namespace"
	satisfied, err = profileSurgePodAntiAffinitySatisfied(pending, []corev1.Pod{sameProfile, *otherProfile})
	require.NoError(t, err)
	assert.True(t, satisfied, "Pod anti-affinity without explicit namespaces is namespace-scoped")
}

func TestExistingPodRequiredAntiAffinityCanRejectReplacement(t *testing.T) {
	pending := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "datadog", Labels: map[string]string{
		datadoghqcommon.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix,
		"rollout": "new",
	}}}
	existing := corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "datadog", Name: "peer", Labels: map[string]string{"rollout": "old"}}, Spec: corev1.PodSpec{
		NodeName: "node-a",
		Affinity: &corev1.Affinity{PodAntiAffinity: &corev1.PodAntiAffinity{RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{{
			LabelSelector: &metav1.LabelSelector{MatchLabels: map[string]string{
				datadoghqcommon.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix,
			}},
			TopologyKey: corev1.LabelHostname,
		}}}},
	}}

	allowed, err := existingPodsAllowPendingByRequiredAntiAffinity(pending, []corev1.Pod{existing}, "node-a")
	require.NoError(t, err)
	assert.False(t, allowed, "an existing Pod's required anti-affinity must block fallback deletion")

	existing.Namespace = "another-namespace"
	allowed, err = existingPodsAllowPendingByRequiredAntiAffinity(pending, []corev1.Pod{existing}, "node-a")
	require.NoError(t, err)
	assert.True(t, allowed)

	emptyNamespaceSelector := &metav1.LabelSelector{}
	existing.Spec.Affinity.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution[0].NamespaceSelector = emptyNamespaceSelector
	allowed, err = existingPodsAllowPendingByRequiredAntiAffinity(pending, []corev1.Pod{existing}, "node-a")
	require.NoError(t, err)
	assert.False(t, allowed, "namespace selectors fail closed because namespace labels are not cached")

	existing.Spec.Affinity.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution[0].LabelSelector = &metav1.LabelSelector{MatchLabels: map[string]string{"rollout": "old"}}
	existing.Spec.Affinity.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution[0].NamespaceSelector = nil
	existing.Namespace = "datadog"
	allowed, err = existingPodsAllowPendingByRequiredAntiAffinity(pending, []corev1.Pod{existing}, "node-a")
	require.NoError(t, err)
	assert.True(t, allowed, "non-matching selectors do not block the replacement")

	existing.Spec.Affinity.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution[0].LabelSelector = &metav1.LabelSelector{MatchLabels: map[string]string{
		datadoghqcommon.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix,
	}}
	existing.Spec.NodeName = "node-b"
	allowed, err = existingPodsAllowPendingByRequiredAntiAffinity(pending, []corev1.Pod{existing}, "node-a")
	require.NoError(t, err)
	assert.True(t, allowed, "hostname anti-affinity on another node does not block")

	existing.Spec.Affinity.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution[0].TopologyKey = "topology.kubernetes.io/zone"
	allowed, err = existingPodsAllowPendingByRequiredAntiAffinity(pending, []corev1.Pod{existing}, "node-a")
	require.NoError(t, err)
	assert.False(t, allowed, "wider topology terms fail closed without loading every Node's topology labels")
}

func TestAffinityTermMaySelectNamespace(t *testing.T) {
	assert.True(t, affinityTermMaySelectNamespace(&corev1.PodAffinityTerm{Namespaces: []string{"target"}}, "source", "target"))
	assert.False(t, affinityTermMaySelectNamespace(&corev1.PodAffinityTerm{Namespaces: []string{"other"}}, "source", "target"))
}

func TestPodAffinityTermSelector(t *testing.T) {
	term := &corev1.PodAffinityTerm{
		LabelSelector:     &metav1.LabelSelector{MatchLabels: map[string]string{"app": "agent"}},
		MatchLabelKeys:    []string{"rollout"},
		MismatchLabelKeys: []string{"profile"},
	}
	selector, err := podAffinityTermSelector(term, map[string]string{"rollout": "new", "profile": "linux"})
	require.NoError(t, err)
	assert.True(t, selector.Matches(labels.Set{"app": "agent", "rollout": "new", "profile": "gpu"}))
	assert.False(t, selector.Matches(labels.Set{"app": "agent", "rollout": "old", "profile": "gpu"}))
	assert.False(t, selector.Matches(labels.Set{"app": "agent", "rollout": "new", "profile": "linux"}))

	selector, err = podAffinityTermSelector(term, nil)
	require.NoError(t, err)
	assert.True(t, selector.Matches(labels.Set{"app": "agent"}), "keys missing from the source Pod must not add selector requirements")

	_, err = podAffinityTermSelector(&corev1.PodAffinityTerm{LabelSelector: &metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{{
		Key: "app", Operator: metav1.LabelSelectorOperator("Invalid"), Values: []string{"agent"},
	}}}}, nil)
	require.Error(t, err)

	_, err = podAffinityTermSelector(&corev1.PodAffinityTerm{
		LabelSelector:  &metav1.LabelSelector{},
		MatchLabelKeys: []string{"bad key"},
	}, map[string]string{"bad key": "value"})
	require.Error(t, err)

	_, err = podAffinityTermSelector(&corev1.PodAffinityTerm{
		LabelSelector:     &metav1.LabelSelector{},
		MismatchLabelKeys: []string{"bad key"},
	}, map[string]string{"bad key": "value"})
	require.Error(t, err)
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

	claims := replacement.DeepCopy()
	claims.Spec.ResourceClaims = []corev1.PodResourceClaim{{Name: "accelerator"}}
	assert.False(t, resourceFitAfterOldPodRemoval(node, []corev1.Pod{*old}, claims, old, resourceShortage{cpu: true}), "dynamic resource claims are not modeled")

	assert.False(t, resourceFitAfterOldPodRemoval(node, nil, replacement, old, resourceShortage{cpu: true}), "the exact old Pod must still be present")
	assert.False(t, resourceFitAfterOldPodRemoval(node, []corev1.Pod{*old}, replacement, old, resourceShortage{}), "a scheduler-reported CPU or memory shortage is required")

	podLimited := node.DeepCopy()
	podLimited.Status.Allocatable[corev1.ResourcePods] = resource.MustParse("0")
	assert.False(t, resourceFitAfterOldPodRemoval(podLimited, []corev1.Pod{*old}, replacement, old, resourceShortage{cpu: true}), "the replacement must fit the node Pod limit")

	finished := scheduledResourcePod("finished", "finished-uid", "node-a", "10", "10Gi")
	finished.Status.Phase = corev1.PodSucceeded
	assert.True(t, resourceFitAfterOldPodRemoval(node, []corev1.Pod{*old, *finished}, replacement, old, resourceShortage{cpu: true}), "terminal Pods do not consume scheduler capacity")
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
			name: "unrecognized pod anti affinity",
			mutate: func(pod *corev1.Pod) {
				pod.Spec.Affinity.PodAntiAffinity = &corev1.PodAntiAffinity{RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{{TopologyKey: "topology.kubernetes.io/zone"}}}
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

func TestReconcileResourceFallbackEarlyExitsAndErrors(t *testing.T) {
	t.Run("uses cached client when API reader is absent", func(t *testing.T) {
		fixture := newFallbackTestFixture(t, healthyNodeConditions())
		fixture.reconciler.apiReader = nil
		result, err := fixture.reconciler.reconcileResourceFallback(context.Background(), fixture.ddai, fixture.ds, intstr.FromInt(0))
		require.NoError(t, err)
		assert.Zero(t, result)
	})

	t.Run("missing daemonset", func(t *testing.T) {
		fixture := newFallbackTestFixture(t, healthyNodeConditions())
		require.NoError(t, fixture.client.Delete(context.Background(), fixture.ds))
		result, err := fixture.reconciler.reconcileResourceFallback(context.Background(), fixture.ddai, fixture.ds, intstr.FromInt(1))
		require.NoError(t, err)
		assert.Zero(t, result)
	})

	t.Run("daemonset read error", func(t *testing.T) {
		fixture := newFallbackTestFixture(t, healthyNodeConditions())
		reader := interceptor.NewClient(fixture.client.(client.WithWatch), interceptor.Funcs{
			Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if _, ok := obj.(*appsv1.DaemonSet); ok {
					return errors.New("read daemonset")
				}
				return c.Get(ctx, key, obj, opts...)
			},
		})
		fixture.reconciler.apiReader = reader
		_, err := fixture.reconciler.reconcileResourceFallback(context.Background(), fixture.ddai, fixture.ds, intstr.FromInt(1))
		require.ErrorContains(t, err, "get Agent DaemonSet")
	})

	t.Run("invalid and zero budgets", func(t *testing.T) {
		for _, budget := range []intstr.IntOrString{intstr.FromString("invalid"), intstr.FromInt(0)} {
			fixture := newFallbackTestFixture(t, healthyNodeConditions())
			result, err := fixture.reconciler.reconcileResourceFallback(context.Background(), fixture.ddai, fixture.ds, budget)
			if budget.Type == intstr.String {
				require.ErrorContains(t, err, "resolve Agent resource fallback budget")
			} else {
				require.NoError(t, err)
				assert.Zero(t, result)
			}
		}
	})

	t.Run("missing current revision", func(t *testing.T) {
		fixture := newFallbackTestFixture(t, healthyNodeConditions())
		revisions := &appsv1.ControllerRevisionList{}
		require.NoError(t, fixture.client.List(context.Background(), revisions))
		require.NotEmpty(t, revisions.Items)
		require.NoError(t, fixture.client.Delete(context.Background(), &revisions.Items[0]))
		result, err := fixture.reconciler.reconcileResourceFallback(context.Background(), fixture.ddai, fixture.ds, intstr.FromInt(1))
		require.NoError(t, err)
		assert.Zero(t, result)
	})

	t.Run("revision and pod list errors", func(t *testing.T) {
		for _, failPods := range []bool{false, true} {
			fixture := newFallbackTestFixture(t, healthyNodeConditions())
			reader := interceptor.NewClient(fixture.client.(client.WithWatch), interceptor.Funcs{
				List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
					if _, ok := list.(*appsv1.ControllerRevisionList); ok && !failPods {
						return errors.New("list revisions")
					}
					if _, ok := list.(*corev1.PodList); ok && failPods {
						return errors.New("list pods")
					}
					return c.List(ctx, list, opts...)
				},
			})
			fixture.reconciler.apiReader = reader
			_, err := fixture.reconciler.reconcileResourceFallback(context.Background(), fixture.ddai, fixture.ds, intstr.FromInt(1))
			require.Error(t, err)
		}
	})

	t.Run("reservation patch error", func(t *testing.T) {
		fixture := newFallbackTestFixture(t, healthyNodeConditions())
		writer := interceptor.NewClient(fixture.client.(client.WithWatch), interceptor.Funcs{
			Patch: func(context.Context, client.WithWatch, client.Object, client.Patch, ...client.PatchOption) error {
				return errors.New("patch reservation")
			},
		})
		fixture.reconciler.client = writer
		_, err := fixture.reconciler.reconcileResourceFallback(context.Background(), fixture.ddai, fixture.ds, intstr.FromInt(1))
		require.ErrorContains(t, err, "reserve Agent resource fallback")
	})

	t.Run("old pod delete error", func(t *testing.T) {
		fixture := newFallbackTestFixture(t, healthyNodeConditions())
		writer := interceptor.NewClient(fixture.client.(client.WithWatch), interceptor.Funcs{
			Delete: func(context.Context, client.WithWatch, client.Object, ...client.DeleteOption) error {
				return errors.New("delete old pod")
			},
		})
		fixture.reconciler.client = writer
		_, err := fixture.reconciler.reconcileResourceFallback(context.Background(), fixture.ddai, fixture.ds, intstr.FromInt(1))
		require.ErrorContains(t, err, "delete old Agent Pod")
	})
}

func TestFallbackBudgetWithinLimitFailsClosed(t *testing.T) {
	t.Run("missing DaemonSet", func(t *testing.T) {
		fixture := newFallbackTestFixture(t, healthyNodeConditions())
		require.NoError(t, fixture.client.Delete(context.Background(), fixture.ds))
		ok, err := fallbackBudgetWithinLimit(context.Background(), fixture.client, fixture.ds, intstr.FromInt(1), "new-revision")
		require.NoError(t, err)
		assert.False(t, ok)
	})

	t.Run("stale DaemonSet identity", func(t *testing.T) {
		fixture := newFallbackTestFixture(t, healthyNodeConditions())
		expected := fixture.ds.DeepCopy()
		expected.UID = "stale-uid"
		ok, err := fallbackBudgetWithinLimit(context.Background(), fixture.client, expected, intstr.FromInt(1), "new-revision")
		require.NoError(t, err)
		assert.False(t, ok)
	})

	t.Run("revision changed", func(t *testing.T) {
		fixture := newFallbackTestFixture(t, healthyNodeConditions())
		ok, err := fallbackBudgetWithinLimit(context.Background(), fixture.client, fixture.ds, intstr.FromInt(1), "other-revision")
		require.NoError(t, err)
		assert.False(t, ok)
	})

	t.Run("invalid budget", func(t *testing.T) {
		fixture := newFallbackTestFixture(t, healthyNodeConditions())
		ok, err := fallbackBudgetWithinLimit(context.Background(), fixture.client, fixture.ds, intstr.FromString("invalid"), "new-revision")
		require.Error(t, err)
		assert.False(t, ok)
	})
}

func TestControllerRevisionMatchesTemplateRejectsInvalidData(t *testing.T) {
	fixture := newFallbackTestFixture(t, healthyNodeConditions())
	revisions := &appsv1.ControllerRevisionList{}
	require.NoError(t, fixture.client.List(context.Background(), revisions))
	require.Len(t, revisions.Items, 1)
	revision := revisions.Items[0].DeepCopy()
	revision.Data.Raw = []byte("not-json")

	got, err := controllerRevisionMatchesTemplate(revision, &fixture.ds.Spec.Template)
	require.Error(t, err)
	assert.False(t, got)
}

func TestFallbackCandidatesFailClosedAndSortReservations(t *testing.T) {
	now := time.Now()
	ds := &appsv1.DaemonSet{Spec: appsv1.DaemonSetSpec{MinReadySeconds: 0}}
	pending := func(name, node, reservation string) corev1.Pod {
		pod := pendingPodForNode(node)
		pod.Name = name
		pod.UID = types.UID(name + "-uid")
		pod.Labels = map[string]string{appsv1.DefaultDaemonSetUniqueLabelKey: "new"}
		pod.Status.Conditions = []corev1.PodCondition{{Type: corev1.PodScheduled, Status: corev1.ConditionFalse, Reason: corev1.PodReasonUnschedulable, Message: "0/1 nodes are available: 1 Insufficient cpu."}}
		if reservation != "" {
			pod.Annotations = map[string]string{resourceFallbackOldPodAnnotation: reservation}
		}
		return *pod
	}
	old := func(name, uid, node string) corev1.Pod {
		pod := readyPod(name, uid, node, "old", now.Add(-time.Minute))
		return *pod
	}

	noTarget := pending("no-target", "node-a", "")
	noTarget.Spec.Affinity = nil
	assert.Empty(t, fallbackCandidates(ds, []corev1.Pod{noTarget, old("old-a", "old-a-uid", "node-a")}, "new", now))
	assert.Empty(t, fallbackCandidates(ds, []corev1.Pod{pending("no-old", "node-a", "")}, "new", now))
	assert.Empty(t, fallbackCandidates(ds, []corev1.Pod{
		pending("two-old", "node-a", ""), old("old-a", "old-a-uid", "node-a"), old("old-b", "old-b-uid", "node-a"),
	}, "new", now))
	assert.Empty(t, fallbackCandidates(ds, []corev1.Pod{
		pending("wrong-reservation", "node-a", "other-uid"), old("old-a", "old-a-uid", "node-a"),
	}, "new", now))

	pods := []corev1.Pod{
		pending("new-c", "node-c", ""), old("old-c", "old-c-uid", "node-c"),
		pending("new-b", "node-b", ""), old("old-b", "old-b-uid", "node-b"),
		pending("new-a", "node-a", "old-a-uid"), old("old-a", "old-a-uid", "node-a"),
	}
	candidates := fallbackCandidates(ds, pods, "new", now)
	require.Len(t, candidates, 3)
	assert.True(t, candidates[0].reserved)
	assert.Equal(t, "node-a", candidates[0].nodeName)
	assert.Equal(t, "node-b", candidates[1].nodeName)
	assert.Equal(t, "node-c", candidates[2].nodeName)
}

func TestRevalidateFallbackCandidateRejectsStaleStateAndReadErrors(t *testing.T) {
	candidateFor := func(fixture fallbackTestFixture) fallbackCandidate {
		return fallbackCandidate{pending: fixture.pending.DeepCopy(), old: fixture.old.DeepCopy(), nodeName: "node-a", shortage: resourceShortage{cpu: true}}
	}

	t.Run("DaemonSet read error", func(t *testing.T) {
		fixture := newFallbackTestFixture(t, healthyNodeConditions())
		reader := interceptor.NewClient(fixture.client.(client.WithWatch), interceptor.Funcs{
			Get: func(context.Context, client.WithWatch, client.ObjectKey, client.Object, ...client.GetOption) error {
				return errors.New("read failed")
			},
		})
		got, err := fixture.reconciler.revalidateFallbackCandidate(context.Background(), reader, fixture.ds, candidateFor(fixture), "new-revision", false)
		require.ErrorContains(t, err, "read failed")
		assert.Nil(t, got)
	})

	t.Run("stale DaemonSet", func(t *testing.T) {
		fixture := newFallbackTestFixture(t, healthyNodeConditions())
		expected := fixture.ds.DeepCopy()
		expected.UID = "stale-uid"
		got, err := fixture.reconciler.revalidateFallbackCandidate(context.Background(), fixture.client, expected, candidateFor(fixture), "new-revision", false)
		require.NoError(t, err)
		assert.Nil(t, got)
	})

	t.Run("revision changed", func(t *testing.T) {
		fixture := newFallbackTestFixture(t, healthyNodeConditions())
		got, err := fixture.reconciler.revalidateFallbackCandidate(context.Background(), fixture.client, fixture.ds, candidateFor(fixture), "other-revision", false)
		require.NoError(t, err)
		assert.Nil(t, got)
	})

	for _, objectName := range []string{"new", "old"} {
		t.Run("missing "+objectName+" Pod", func(t *testing.T) {
			fixture := newFallbackTestFixture(t, healthyNodeConditions())
			pod := fixture.pending
			if objectName == "old" {
				pod = fixture.old
			}
			require.NoError(t, fixture.client.Delete(context.Background(), pod))
			got, err := fixture.reconciler.revalidateFallbackCandidate(context.Background(), fixture.client, fixture.ds, candidateFor(fixture), "new-revision", false)
			require.NoError(t, err)
			assert.Nil(t, got)
		})
	}

	t.Run("stale Pod UID", func(t *testing.T) {
		fixture := newFallbackTestFixture(t, healthyNodeConditions())
		candidate := candidateFor(fixture)
		candidate.pending.UID = "stale-pending"
		got, err := fixture.reconciler.revalidateFallbackCandidate(context.Background(), fixture.client, fixture.ds, candidate, "new-revision", false)
		require.NoError(t, err)
		assert.Nil(t, got)
	})

	t.Run("reservation required", func(t *testing.T) {
		fixture := newFallbackTestFixture(t, healthyNodeConditions())
		got, err := fixture.reconciler.revalidateFallbackCandidate(context.Background(), fixture.client, fixture.ds, candidateFor(fixture), "new-revision", true)
		require.NoError(t, err)
		assert.Nil(t, got)
	})

	t.Run("node disappeared", func(t *testing.T) {
		fixture := newFallbackTestFixture(t, healthyNodeConditions())
		node := &corev1.Node{}
		require.NoError(t, fixture.client.Get(context.Background(), client.ObjectKey{Name: "node-a"}, node))
		require.NoError(t, fixture.client.Delete(context.Background(), node))
		got, err := fixture.reconciler.revalidateFallbackCandidate(context.Background(), fixture.client, fixture.ds, candidateFor(fixture), "new-revision", false)
		require.NoError(t, err)
		assert.Nil(t, got)
	})

	t.Run("Pod list errors", func(t *testing.T) {
		for _, failCall := range []int{1, 2} {
			fixture := newFallbackTestFixture(t, healthyNodeConditions())
			podLists := 0
			reader := interceptor.NewClient(fixture.client.(client.WithWatch), interceptor.Funcs{
				List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
					if _, ok := list.(*corev1.PodList); ok {
						podLists++
						if podLists == failCall {
							return errors.New("list failed")
						}
					}
					return c.List(ctx, list, opts...)
				},
			})
			got, err := fixture.reconciler.revalidateFallbackCandidate(context.Background(), reader, fixture.ds, candidateFor(fixture), "new-revision", false)
			require.ErrorContains(t, err, "list")
			assert.Nil(t, got)
		}
	})
}

func TestToleratesBlockingNodeTaints(t *testing.T) {
	taints := []corev1.Taint{
		{Key: "dedicated", Value: "agents", Effect: corev1.TaintEffectNoSchedule},
		{Key: "draining", Effect: corev1.TaintEffectNoExecute},
		{Key: "soft", Effect: corev1.TaintEffectPreferNoSchedule},
	}
	assert.False(t, toleratesBlockingNodeTaints(nil, taints))
	assert.False(t, toleratesBlockingNodeTaints([]corev1.Toleration{
		{Key: "dedicated", Value: "agents", Operator: corev1.TolerationOpEqual, Effect: corev1.TaintEffectNoSchedule},
	}, taints))
	assert.True(t, toleratesBlockingNodeTaints([]corev1.Toleration{
		{Key: "dedicated", Value: "agents", Operator: corev1.TolerationOpEqual, Effect: corev1.TaintEffectNoSchedule},
		{Key: "draining", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoExecute},
	}, taints))
	assert.True(t, toleratesBlockingNodeTaints([]corev1.Toleration{{Operator: corev1.TolerationOpExists}}, taints))
}

func TestNodeReadyForResourceFallbackFailsClosed(t *testing.T) {
	ready := &corev1.Node{Status: corev1.NodeStatus{Conditions: healthyNodeConditions()}}
	assert.True(t, nodeReadyForResourceFallback(ready))

	unschedulable := ready.DeepCopy()
	unschedulable.Spec.Unschedulable = true
	assert.False(t, nodeReadyForResourceFallback(unschedulable))

	deleting := ready.DeepCopy()
	deleting.DeletionTimestamp = &metav1.Time{Time: time.Now()}
	assert.False(t, nodeReadyForResourceFallback(deleting))

	networkUnavailable := ready.DeepCopy()
	for i := range networkUnavailable.Status.Conditions {
		if networkUnavailable.Status.Conditions[i].Type == corev1.NodeNetworkUnavailable {
			networkUnavailable.Status.Conditions[i].Status = corev1.ConditionTrue
		}
	}
	assert.False(t, nodeReadyForResourceFallback(networkUnavailable))
}

func TestResourceFallbackDaemonSetEligibleFailsClosed(t *testing.T) {
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Generation: 1},
		Spec: appsv1.DaemonSetSpec{UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
			Type:          appsv1.RollingUpdateDaemonSetStrategyType,
			RollingUpdate: &appsv1.RollingUpdateDaemonSet{MaxSurge: ptr.To(intstr.FromInt(1))},
		}},
		Status: appsv1.DaemonSetStatus{ObservedGeneration: 1, DesiredNumberScheduled: 1},
	}
	assert.True(t, resourceFallbackDaemonSetEligible(ds))

	for _, mutate := range []func(*appsv1.DaemonSet){
		func(value *appsv1.DaemonSet) { value.Status.DesiredNumberScheduled = 0 },
		func(value *appsv1.DaemonSet) { value.Status.ObservedGeneration = 0 },
		func(value *appsv1.DaemonSet) { value.Spec.UpdateStrategy.Type = appsv1.OnDeleteDaemonSetStrategyType },
		func(value *appsv1.DaemonSet) {
			value.Spec.UpdateStrategy.RollingUpdate.MaxSurge = ptr.To(intstr.FromInt(0))
		},
	} {
		copy := ds.DeepCopy()
		mutate(copy)
		assert.False(t, resourceFallbackDaemonSetEligible(copy))
	}
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
