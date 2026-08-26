// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// Copyright 2016-present Datadog, Inc.

package datadogagentinternal

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	common "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	componentagent "github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/agent"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPreparedRolloutBudgetUsesExistingMaxUnavailable(t *testing.T) {
	fivePercent := intstr.FromString("5%")
	ddai := &datadoghqv1alpha1.DatadogAgentInternal{Spec: datadoghqv2alpha1.DatadogAgentSpec{
		Override: map[datadoghqv2alpha1.ComponentName]*datadoghqv2alpha1.DatadogAgentComponentOverride{
			datadoghqv2alpha1.NodeAgentComponentName: {
				UpdateStrategy: &common.UpdateStrategy{RollingUpdate: &common.RollingUpdate{MaxUnavailable: ptr.To(fivePercent)}},
			},
		},
	}}

	assert.Equal(t, fivePercent, preparedRolloutBudget(ddai, &componentagent.ExtendedDaemonsetOptions{MaxPodUnavailable: "20%"}))
	assert.Equal(t, intstr.FromString("20%"), preparedRolloutBudget(&datadoghqv1alpha1.DatadogAgentInternal{}, &componentagent.ExtendedDaemonsetOptions{MaxPodUnavailable: "20%"}))
	assert.Equal(t, intstr.FromInt(1), preparedRolloutBudget(&datadoghqv1alpha1.DatadogAgentInternal{}, nil))
}

func TestPrepareProfileAntiAffinityForOverlapNarrowsGeneratedRule(t *testing.T) {
	template := corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
			common.AgentDeploymentNameLabelKey: "agent",
			constants.ProfileLabelKey:          "profile-a",
		}},
		Spec: corev1.PodSpec{Affinity: &corev1.Affinity{PodAntiAffinity: broadAgentPodAntiAffinity()}},
	}

	require.True(t, prepareProfileAntiAffinityForOverlap(&template))
	require.Len(t, template.Spec.Affinity.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution, 2)
	assert.NotEqual(t, broadAgentPodAntiAffinity(), template.Spec.Affinity.PodAntiAffinity)
}

func TestPrepareProfileAntiAffinityForOverlapFailsWithoutDeploymentIdentity(t *testing.T) {
	template := corev1.PodTemplateSpec{Spec: corev1.PodSpec{Affinity: &corev1.Affinity{PodAntiAffinity: broadAgentPodAntiAffinity()}}}
	assert.False(t, prepareProfileAntiAffinityForOverlap(&template))
}

func TestProfileOverlapAntiAffinityWithoutProfileBlocksProfiledPods(t *testing.T) {
	affinity, ok := profileOverlapPodAntiAffinity(map[string]string{common.AgentDeploymentNameLabelKey: "agent"})
	require.True(t, ok)
	require.Len(t, affinity.RequiredDuringSchedulingIgnoredDuringExecution, 2)
	blocked := func(podLabels map[string]string) bool {
		for _, term := range affinity.RequiredDuringSchedulingIgnoredDuringExecution {
			selector, err := metav1.LabelSelectorAsSelector(term.LabelSelector)
			require.NoError(t, err)
			if selector.Matches(labels.Set(podLabels)) {
				return true
			}
		}
		return false
	}
	base := map[string]string{
		common.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix,
		common.AgentDeploymentNameLabelKey:      "agent",
	}
	assert.False(t, blocked(base), "the same unprofiled deployment may overlap")
	assert.True(t, blocked(map[string]string{
		common.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix,
		common.AgentDeploymentNameLabelKey:      "agent",
		constants.ProfileLabelKey:               "gpu",
	}), "a different profile of the same deployment remains blocked")
	assert.True(t, blocked(map[string]string{
		common.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix,
		common.AgentDeploymentNameLabelKey:      "other-agent",
	}), "a different deployment remains blocked")
	assert.False(t, blocked(map[string]string{
		common.AgentDeploymentComponentLabelKey: "cluster-agent",
		common.AgentDeploymentNameLabelKey:      "other-agent",
	}), "non-node-Agent components are outside this anti-affinity")
}

func TestPreparedRolloutBudgetValidationAndConventionalStrategy(t *testing.T) {
	zero := intstr.FromInt(0)
	assert.False(t, validMaxUnavailable(zero))
	fivePercent := intstr.FromString("5%")
	assert.True(t, validMaxUnavailable(fivePercent))
	assert.False(t, validMaxUnavailable(intstr.FromString("101%")))
	assert.False(t, validMaxUnavailable(intstr.FromString("invalid")))
	assert.True(t, validMaxUnavailable(intstr.FromInt(101)), "Kubernetes allows absolute values above the node count")

	ds := &appsv1.DaemonSet{}
	configureConventionalMigration(ds, fivePercent)
	require.NotNil(t, ds.Spec.UpdateStrategy.RollingUpdate)
	assert.Equal(t, appsv1.RollingUpdateDaemonSetStrategyType, ds.Spec.UpdateStrategy.Type)
	assert.Equal(t, fivePercent, *ds.Spec.UpdateStrategy.RollingUpdate.MaxUnavailable)
	assert.Equal(t, intstr.FromInt(0), *ds.Spec.UpdateStrategy.RollingUpdate.MaxSurge)
}
