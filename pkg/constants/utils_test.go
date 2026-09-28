// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.
package constants

import (
	"fmt"
	"testing"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/stretchr/testify/assert"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestServiceAccountNameOverride(t *testing.T) {
	customServiceAccount := "fake"
	ddaName := "test-dda"
	tests := []struct {
		name string
		dda  *v2alpha1.DatadogAgent
		want map[v2alpha1.ComponentName]string
	}{
		{
			name: "custom serviceaccount for dca and clc",
			dda: &v2alpha1.DatadogAgent{
				ObjectMeta: v1.ObjectMeta{
					Name: ddaName,
				},
				Spec: v2alpha1.DatadogAgentSpec{
					Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.ClusterAgentComponentName: {
							ServiceAccountName: &customServiceAccount,
						},
						v2alpha1.ClusterChecksRunnerComponentName: {
							ServiceAccountName: &customServiceAccount,
						},
					},
				},
			},
			want: map[v2alpha1.ComponentName]string{
				v2alpha1.ClusterAgentComponentName:        customServiceAccount,
				v2alpha1.NodeAgentComponentName:           fmt.Sprintf("%s-%s", ddaName, DefaultAgentResourceSuffix),
				v2alpha1.ClusterChecksRunnerComponentName: customServiceAccount,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := map[v2alpha1.ComponentName]string{}
			res[v2alpha1.NodeAgentComponentName] = GetAgentServiceAccount(tt.dda.Name, &tt.dda.Spec)
			res[v2alpha1.ClusterChecksRunnerComponentName] = GetClusterChecksRunnerServiceAccount(tt.dda.Name, &tt.dda.Spec)
			res[v2alpha1.ClusterAgentComponentName] = GetClusterAgentServiceAccount(tt.dda.Name, &tt.dda.Spec)
			for name, sa := range tt.want {
				if res[name] != sa {
					t.Errorf("Service Account Override error = %v, want %v", res[name], tt.want[name])
				}
			}
		})
	}
}

func TestIsCCRComponentRequired(t *testing.T) {
	dda := func(clusterChecksEnabled, useRunners *bool, kubeKnob string) *v2alpha1.DatadogAgent {
		return &v2alpha1.DatadogAgent{
			ObjectMeta: v1.ObjectMeta{
				Name: "test-dda",
				Annotations: map[string]string{
					v2alpha1.AnnotationExperimentalKubeChecksRunnerDefault: kubeKnob,
				},
			},
			Spec: v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					ClusterChecks: &v2alpha1.ClusterChecksFeatureConfig{
						Enabled:                 clusterChecksEnabled,
						UseClusterChecksRunners: useRunners,
					},
				},
			},
		}
	}

	tests := []struct {
		name string
		dda  *v2alpha1.DatadogAgent
		want bool
	}{
		{
			name: "cluster checks disabled, knob on",
			dda:  dda(new(false), new(false), "true"),
			want: false,
		},
		{
			name: "runners off, knob off",
			dda:  dda(new(true), new(false), "false"),
			want: false,
		},
		{
			name: "runners on, knob off",
			dda:  dda(new(true), new(true), "false"),
			want: true,
		},
		{
			name: "runners off, knob on (mixed mode)",
			dda:  dda(new(true), new(false), "true"),
			want: true,
		},
		{
			name: "runners on, knob on",
			dda:  dda(new(true), new(true), "true"),
			want: true,
		},
		{
			name: "no cluster checks feature at all, knob on",
			dda: func() *v2alpha1.DatadogAgent {
				return &v2alpha1.DatadogAgent{
					ObjectMeta: v1.ObjectMeta{
						Name: "test-dda",
						Annotations: map[string]string{
							v2alpha1.AnnotationExperimentalKubeChecksRunnerDefault: "true",
						},
					},
					Spec: v2alpha1.DatadogAgentSpec{
						Features: &v2alpha1.DatadogFeatures{},
					},
				}
			}(),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsCCRComponentRequired(tt.dda, &tt.dda.Spec))
		})
	}
}
