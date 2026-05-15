// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package common

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/version"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

func TestNewDeploymentUsesDefaultMetadata(t *testing.T) {
	dda := testDatadogAgent()

	deployment := NewDeployment(dda, constants.DefaultClusterAgentResourceSuffix, "datadog-cluster-agent", "7.77.0", nil)

	require.Equal(t, "datadog-cluster-agent", deployment.Name)
	require.Equal(t, "agents", deployment.Namespace)
	require.Equal(t, "datadog-cluster-agent", deployment.Labels[kubernetes.AppKubernetesInstanceLabelKey])
	require.Equal(t, constants.DefaultClusterAgentResourceSuffix, deployment.Labels[apicommon.AgentDeploymentComponentLabelKey])
	require.Equal(t, map[string]string{
		kubernetes.AppKubernetesInstanceLabelKey:   "datadog-cluster-agent",
		apicommon.AgentDeploymentComponentLabelKey: constants.DefaultClusterAgentResourceSuffix,
	}, deployment.Spec.Selector.MatchLabels)
}

func TestGetDefaultMetadata(t *testing.T) {
	t.Run("copies explicit selector labels into object labels", func(t *testing.T) {
		selector := &metav1.LabelSelector{MatchLabels: map[string]string{"custom": "selector"}}

		labels, _, gotSelector := GetDefaultMetadata(testDatadogAgent(), constants.DefaultAgentResourceSuffix, "datadog-agent", "7.77.0", selector)

		require.Equal(t, selector, gotSelector)
		require.Equal(t, "selector", labels["custom"])
	})

	t.Run("uses legacy selector labels when metadata update is disabled", func(t *testing.T) {
		dda := testDatadogAgent()
		dda.Annotations = map[string]string{apicommon.UpdateMetadataAnnotationKey: "false"}

		_, _, selector := GetDefaultMetadata(dda, constants.DefaultAgentResourceSuffix, "datadog-agent", "7.77.0", nil)

		require.Equal(t, map[string]string{
			apicommon.AgentDeploymentNameLabelKey:      "datadog",
			apicommon.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix,
		}, selector.MatchLabels)
	})
}

func TestComponentVersionHelpers(t *testing.T) {
	t.Run("uses the component image tag override", func(t *testing.T) {
		dda := testDatadogAgent()
		dda.Spec.Override = map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
			v2alpha1.NodeAgentComponentName: {
				Image: &v2alpha1.AgentImageConfig{Tag: "7.76.0"},
			},
		}

		require.Equal(t, "7.76.0", GetComponentVersion(dda, v2alpha1.NodeAgentComponentName))
	})

	t.Run("extracts a version from image name and removes jmx suffix", func(t *testing.T) {
		require.Equal(t, "7.75.1", GetAgentVersionFromImage(v2alpha1.AgentImageConfig{
			Name: "gcr.io/datadoghq/agent:7.75.1-jmx",
		}))
	})

	t.Run("image tag takes precedence over image name", func(t *testing.T) {
		require.Equal(t, "7.77.0", GetAgentVersionFromImage(v2alpha1.AgentImageConfig{
			Name: "gcr.io/datadoghq/agent:7.75.1",
			Tag:  "7.77.0",
		}))
	})
}

func TestEnvVarBuilders(t *testing.T) {
	source := BuildEnvVarFromSecret("datadog-secret", "api-key")
	envVar := BuildEnvVarFromSource("DD_API_KEY", source)

	require.Equal(t, "DD_API_KEY", envVar.Name)
	require.Equal(t, "datadog-secret", envVar.ValueFrom.SecretKeyRef.Name)
	require.Equal(t, "api-key", envVar.ValueFrom.SecretKeyRef.Key)
}

func TestServiceSelectors(t *testing.T) {
	dda := testDatadogAgent()

	require.Equal(t, map[string]string{
		kubernetes.AppKubernetesPartOfLabelKey:     "agents-datadog",
		apicommon.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix,
	}, GetAgentLocalServiceSelector(dda))

	require.Equal(t, map[string]string{
		kubernetes.AppKubernetesPartOfLabelKey:     "agents-datadog",
		apicommon.AgentDeploymentComponentLabelKey: constants.DefaultOtelAgentGatewayResourceSuffix,
	}, GetOtelAgentGatewayServiceSelector(dda))
}

func TestShouldCreateAgentLocalService(t *testing.T) {
	require.False(t, ShouldCreateAgentLocalService(nil, true))
	require.False(t, ShouldCreateAgentLocalService(&version.Info{}, true))
	require.False(t, ShouldCreateAgentLocalService(&version.Info{GitVersion: "v1.21.0"}, false))
	require.True(t, ShouldCreateAgentLocalService(&version.Info{GitVersion: "v1.21.0"}, true))
	require.True(t, ShouldCreateAgentLocalService(&version.Info{GitVersion: "v1.22.0"}, false))
}

func TestMergeAffinities(t *testing.T) {
	first := &corev1.Affinity{
		NodeAffinity: &corev1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
				NodeSelectorTerms: []corev1.NodeSelectorTerm{
					{MatchExpressions: []corev1.NodeSelectorRequirement{{Key: "disk", Operator: corev1.NodeSelectorOpIn, Values: []string{"ssd"}}}},
				},
			},
		},
		PodAffinity: &corev1.PodAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{{TopologyKey: "zone-a"}},
		},
	}
	second := &corev1.Affinity{
		NodeAffinity: &corev1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
				NodeSelectorTerms: []corev1.NodeSelectorTerm{
					{MatchExpressions: []corev1.NodeSelectorRequirement{{Key: "arch", Operator: corev1.NodeSelectorOpIn, Values: []string{"arm64"}}}},
				},
			},
		},
		PodAffinity: &corev1.PodAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{{TopologyKey: "zone-b"}},
		},
	}

	merged := MergeAffinities(first, second)

	require.Len(t, merged.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms, 1)
	require.Len(t, merged.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions, 2)
	require.Equal(t, []corev1.PodAffinityTerm{{TopologyKey: "zone-a"}, {TopologyKey: "zone-b"}}, merged.PodAffinity.RequiredDuringSchedulingIgnoredDuringExecution)
}

func testDatadogAgent() *v2alpha1.DatadogAgent {
	return &v2alpha1.DatadogAgent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "datadog",
			Namespace: "agents",
		},
	}
}
