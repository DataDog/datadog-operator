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

func TestCCRPlacement(t *testing.T) {
	dda := func(clusterChecksEnabled, useRunners bool, annotations map[string]string) *v2alpha1.DatadogAgent {
		return &v2alpha1.DatadogAgent{
			ObjectMeta: v1.ObjectMeta{Name: "test-dda", Annotations: annotations},
			Spec: v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					ClusterChecks: &v2alpha1.ClusterChecksFeatureConfig{
						Enabled:                 new(clusterChecksEnabled),
						UseClusterChecksRunners: new(useRunners),
					},
				},
				Override: runnerGroupsImages("7.86.0", "7.86.0"),
			},
		}
	}
	knob := map[string]string{v2alpha1.AnnotationExperimentalKubeChecksRunnerDefault: "true"}
	httpGroup := map[string]string{v2alpha1.AnnotationExperimentalClusterChecksRunnerGroups: `[{"name":"http","checksInclude":["http_check"]}]`}

	tests := []struct {
		name         string
		dda          *v2alpha1.DatadogAgent
		wantRequired bool
		wantKSMOnCCR bool
	}{
		{"cluster checks disabled, knob on", dda(false, false, knob), false, false},
		{"runners off, no groups", dda(true, false, nil), false, false},
		{"runners on, no groups", dda(true, true, nil), true, true},
		{"runners off, knob on (mixed mode)", dda(true, false, knob), true, true},
		{"runners off, unrelated user group", dda(true, false, httpGroup), true, false},
		{"runners off, malformed groups", dda(true, false, map[string]string{v2alpha1.AnnotationExperimentalClusterChecksRunnerGroups: "not-json"}), false, false},
		{"no cluster checks feature, knob on", &v2alpha1.DatadogAgent{
			ObjectMeta: v1.ObjectMeta{Name: "test-dda", Annotations: knob},
			Spec:       v2alpha1.DatadogAgentSpec{Features: &v2alpha1.DatadogFeatures{}},
		}, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantRequired, IsCCRComponentRequired(tt.dda, &tt.dda.Spec))
			assert.Equal(t, tt.wantKSMOnCCR, RunsOnCCR(tt.dda, &tt.dda.Spec, "kubernetes_state_core"))
		})
	}
}

func runnerGroupsImages(clusterAgentTag, runnerTag string) map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride {
	return map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
		v2alpha1.ClusterAgentComponentName:        {Image: &v2alpha1.AgentImageConfig{Tag: clusterAgentTag}},
		v2alpha1.ClusterChecksRunnerComponentName: {Image: &v2alpha1.AgentImageConfig{Name: "gcr.io/datadoghq/agent:" + runnerTag}},
	}
}

func TestGetClusterChecksRunnerGroups(t *testing.T) {
	groups := `[{"name":"http","checksInclude":["http_check"]},{"name":"redis","checksInclude":["redisdb"],"override":{"image":{"tag":"7.86.1"}}}]`
	tests := []struct {
		name       string
		override   map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride
		wantGroups []string
		wantErr    bool
	}{
		{"both images recent enough", runnerGroupsImages("7.86.0", "7.86.0"), []string{"http", "redis"}, false},
		{"Cluster Agent too old", runnerGroupsImages("7.85.2", "7.86.0"), nil, true},
		{"runners too old, a group overrides its image", runnerGroupsImages("7.86.0", "7.85.0"), []string{"redis"}, true},
		{"release candidate", runnerGroupsImages("7.86.0-rc.1", "7.86.0-rc.1"), []string{"http", "redis"}, false},
		{"unparseable versions are assumed recent enough", runnerGroupsImages("latest", "dev"), []string{"http", "redis"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dda := &v2alpha1.DatadogAgent{
				ObjectMeta: v1.ObjectMeta{Annotations: map[string]string{v2alpha1.AnnotationExperimentalClusterChecksRunnerGroups: groups}},
				Spec:       v2alpha1.DatadogAgentSpec{Override: tt.override},
			}
			got, err := GetClusterChecksRunnerGroups(dda, &dda.Spec)
			var names []string
			for _, g := range got {
				names = append(names, g.Name)
			}
			assert.Equal(t, tt.wantGroups, names)
			assert.Equal(t, tt.wantErr, err != nil)
		})
	}
}
