// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

func TestShouldRunProcessChecksInCoreAgent(t *testing.T) {
	tests := []struct {
		name string
		tag  string
		want bool
	}{
		{name: "agent before minimum version is unsupported", tag: "7.59.0", want: false},
		{name: "agent at minimum version is supported", tag: "7.60.0", want: true},
		{name: "agent after minimum version is supported", tag: "7.61.0", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := datadogAgentSpecWithNodeAgentTag(tt.tag)

			require.Equal(t, tt.want, ShouldRunProcessChecksInCoreAgent(spec))
		})
	}
}

func TestFeatureAnnotations(t *testing.T) {
	dda := &metav1.ObjectMeta{
		Annotations: map[string]string{
			EnableADPAnnotation:                     "true",
			EnableHostProfilerAnnotation:            "false",
			PrivateActionRunnerConfigDataAnnotation: "config-data",
		},
	}

	require.True(t, HasFeatureEnableAnnotation(dda, EnableADPAnnotation))
	require.False(t, HasFeatureEnableAnnotation(dda, EnableHostProfilerAnnotation))
	require.False(t, HasFeatureEnableAnnotation(dda, EnableFlightRecorderAnnotation))

	value, ok := GetFeatureConfigAnnotation(dda, PrivateActionRunnerConfigDataAnnotation)
	require.True(t, ok)
	require.Equal(t, "config-data", value)

	_, ok = GetFeatureConfigAnnotation(dda, ClusterAgentPrivateActionRunnerConfigDataAnnotation)
	require.False(t, ok)
}

func TestAgentSupportsADPDogstatsdDelegation(t *testing.T) {
	tests := []struct {
		name string
		tag  string
		want bool
	}{
		{name: "agent before minimum version needs operator delegation", tag: "7.74.0", want: false},
		{name: "agent at minimum version supports delegation", tag: "7.75.0", want: true},
		{name: "agent after minimum version supports delegation", tag: "7.76.0", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := datadogAgentSpecWithNodeAgentTag(tt.tag)

			require.Equal(t, tt.want, AgentSupportsADPDogstatsdDelegation(spec))
		})
	}
}

func TestIsDataPlaneEnabled(t *testing.T) {
	t.Run("CRD setting takes precedence over annotation", func(t *testing.T) {
		dda := &metav1.ObjectMeta{Annotations: map[string]string{EnableADPAnnotation: "true"}}
		spec := &v2alpha1.DatadogAgentSpec{
			Features: &v2alpha1.DatadogFeatures{
				DataPlane: &v2alpha1.DataPlaneFeatureConfig{Enabled: ptr.To(false)},
			},
		}

		require.False(t, IsDataPlaneEnabled(dda, spec))
	})

	t.Run("annotation enables data plane when CRD setting is omitted", func(t *testing.T) {
		dda := &metav1.ObjectMeta{Annotations: map[string]string{EnableADPAnnotation: "true"}}

		require.True(t, IsDataPlaneEnabled(dda, &v2alpha1.DatadogAgentSpec{}))
	})

	t.Run("defaults to disabled", func(t *testing.T) {
		require.False(t, IsDataPlaneEnabled(&metav1.ObjectMeta{}, &v2alpha1.DatadogAgentSpec{}))
	})
}

func TestIsDataPlaneDogstatsdEnabled(t *testing.T) {
	require.True(t, IsDataPlaneDogstatsdEnabled(&v2alpha1.DatadogAgentSpec{}))

	require.False(t, IsDataPlaneDogstatsdEnabled(&v2alpha1.DatadogAgentSpec{
		Features: &v2alpha1.DatadogFeatures{
			DataPlane: &v2alpha1.DataPlaneFeatureConfig{
				Dogstatsd: &v2alpha1.DataPlaneDogstatsdConfig{Enabled: ptr.To(false)},
			},
		},
	}))
}

func datadogAgentSpecWithNodeAgentTag(tag string) *v2alpha1.DatadogAgentSpec {
	return &v2alpha1.DatadogAgentSpec{
		Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
			v2alpha1.NodeAgentComponentName: {
				Image: &v2alpha1.AgentImageConfig{Tag: tag},
			},
		},
	}
}
