// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clusterchecksrunner

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/images"
)

const (
	groupsKey   = v2alpha1.AnnotationExperimentalClusterChecksRunnerGroups
	coreKey     = v2alpha1.AnnotationExperimentalCoreChecksRunnerDefault
	kafkaGroups = `[{"name":"kafka","checks":["kafka_consumer"]}]`
)

var kafka = Group{Name: "kafka", Checks: []string{"kafka_consumer"}}

func newDDA(clusterChecks, useRunners bool, annotations map[string]string, override map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride) *v2alpha1.DatadogAgent {
	return &v2alpha1.DatadogAgent{
		ObjectMeta: metav1.ObjectMeta{Name: "datadog", Annotations: annotations},
		Spec: v2alpha1.DatadogAgentSpec{
			Features: &v2alpha1.DatadogFeatures{
				ClusterChecks: &v2alpha1.ClusterChecksFeatureConfig{
					Enabled:                 new(clusterChecks),
					UseClusterChecksRunners: new(useRunners),
				},
				KubeStateMetricsCore: &v2alpha1.KubeStateMetricsCoreFeatureConfig{Enabled: new(true)},
				OrchestratorExplorer: &v2alpha1.OrchestratorExplorerFeatureConfig{Enabled: new(true)},
				HelmCheck:            &v2alpha1.HelmCheckFeatureConfig{Enabled: new(true)},
			},
			Override: override,
		},
	}
}

func images786() map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride {
	return map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
		v2alpha1.ClusterAgentComponentName:        {Image: &v2alpha1.AgentImageConfig{Tag: "7.86.0"}},
		v2alpha1.ClusterChecksRunnerComponentName: {Image: &v2alpha1.AgentImageConfig{Tag: "7.86.0"}},
	}
}

func groupNames(groups []Group) []string {
	var names []string
	for _, g := range groups {
		names = append(names, g.Name)
	}
	return names
}

func core(checks ...string) Group {
	if len(checks) == 0 {
		checks = []string{constants.KubeStateMetricsCoreCheckName, constants.OrchestratorCheckName, constants.HelmCheckName}
	}
	return Group{Name: CoreGroupName, Checks: checks, Override: &v2alpha1.DatadogAgentComponentOverride{Replicas: new(int32(2))}}
}

func TestDeclaredGroups(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		want        []Group
		wantErr     bool
	}{
		{name: "no annotations"},
		{name: "groups annotation only", annotations: map[string]string{groupsKey: kafkaGroups}, want: []Group{kafka}},
		{name: "core only", annotations: map[string]string{coreKey: "true"}, want: []Group{core()}},
		{name: "core is listed first", annotations: map[string]string{coreKey: "true", groupsKey: kafkaGroups}, want: []Group{core(), kafka}},
		{name: "core annotation set to false", annotations: map[string]string{coreKey: "false"}},
		{
			name:        "a user group claiming a core check takes it from core",
			annotations: map[string]string{coreKey: "true", groupsKey: `[{"name":"ksm","checks":["kubernetes_state_core"]},{"name":"kafka","checks":["kafka_consumer"]}]`},
			want:        []Group{core(constants.OrchestratorCheckName, constants.HelmCheckName), {Name: "ksm", Checks: []string{constants.KubeStateMetricsCoreCheckName}}, kafka},
		},
		{
			name:        "core isn't deployed when user groups claim all its checks",
			annotations: map[string]string{coreKey: "true", groupsKey: `[{"name":"mine","checks":["kubernetes_state_core","orchestrator","helm"]}]`},
			want:        []Group{{Name: "mine", Checks: []string{constants.KubeStateMetricsCoreCheckName, constants.OrchestratorCheckName, constants.HelmCheckName}}},
		},
		{
			name:        "a core entry sets the built-in group's override, its checks stay fixed",
			annotations: map[string]string{coreKey: "true", groupsKey: `[{"name":"core","checks":["http_check"],"override":{"replicas":5}}]`},
			want:        []Group{{Name: CoreGroupName, Checks: core().Checks, Override: &v2alpha1.DatadogAgentComponentOverride{Replicas: new(int32(5))}}},
			wantErr:     true,
		},
		{
			name:        "a core entry without replicas keeps the default of 2",
			annotations: map[string]string{coreKey: "true", groupsKey: `[{"name":"core","override":{"priorityClassName":"high"}}]`},
			want:        []Group{{Name: CoreGroupName, Checks: core().Checks, Override: &v2alpha1.DatadogAgentComponentOverride{Replicas: new(int32(2)), PriorityClassName: new("high")}}},
		},
		{
			name:        "a core entry is skipped when the built-in group is off",
			annotations: map[string]string{groupsKey: `[{"name":"core","checks":["kubernetes_state_core"]},{"name":"kafka","checks":["kafka_consumer"]}]`},
			want:        []Group{kafka},
			wantErr:     true,
		},
		{name: "malformed groups annotation", annotations: map[string]string{groupsKey: "not-json"}, wantErr: true},
		{name: "malformed groups annotation keeps core", annotations: map[string]string{coreKey: "true", groupsKey: "not-json"}, want: []Group{core()}, wantErr: true},
		{
			name:        "a duplicate name skips the later group",
			annotations: map[string]string{groupsKey: `[{"name":"kafka","checks":["kafka_consumer"]},{"name":"kafka","checks":["redisdb"]}]`},
			want:        []Group{kafka},
			wantErr:     true,
		},
		{
			name:        "a group without checks is skipped",
			annotations: map[string]string{groupsKey: `[{"name":"a"},` + kafkaGroups[1:]},
			want:        []Group{kafka},
			wantErr:     true,
		},
		{
			name:        "a check claimed twice stays with the first group",
			annotations: map[string]string{groupsKey: `[{"name":"kafka","checks":["kafka_consumer"]},{"name":"b","checks":["kafka_consumer","redisdb"]}]`},
			want:        []Group{kafka},
			wantErr:     true,
		},
		{
			name:        "a name that isn't a DNS label is skipped, the others are kept",
			annotations: map[string]string{groupsKey: `[{"name":"HTTP_checks","checks":["http_check"]},{"name":"kafka","checks":["kafka_consumer"]}]`},
			want:        []Group{kafka},
			wantErr:     true,
		},
		{
			name:        "a skipped group doesn't take checks from core",
			annotations: map[string]string{coreKey: "true", groupsKey: `[{"name":"KSM","checks":["kubernetes_state_core"]}]`},
			want:        []Group{core()},
			wantErr:     true,
		},
		{
			// "datadog-cluster-checks-runner-" is 30 characters, leaving 33 for the group name.
			name:        "a name at the 63-character limit is kept",
			annotations: map[string]string{groupsKey: `[{"name":"` + strings.Repeat("a", 33) + `","checks":["http_check"]}]`},
			want:        []Group{{Name: strings.Repeat("a", 33), Checks: []string{"http_check"}}},
		},
		{
			name:        "a name over the 63-character limit is skipped",
			annotations: map[string]string{groupsKey: `[{"name":"` + strings.Repeat("a", 34) + `","checks":["http_check"]}]`},
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dda := newDDA(true, false, tt.annotations, images786())
			got, err := declaredGroups(dda, &dda.Spec)
			assert.Equal(t, tt.wantErr, err != nil, "error: %v", err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCoreGroupFollowsFeatures(t *testing.T) {
	dda := newDDA(true, false, map[string]string{coreKey: "true"}, images786())
	dda.Spec.Features.HelmCheck.Enabled = new(false)
	got, err := declaredGroups(dda, &dda.Spec)
	require.NoError(t, err)
	assert.Equal(t, []Group{core(constants.KubeStateMetricsCoreCheckName, constants.OrchestratorCheckName)}, got)

	dda.Spec.Features.KubeStateMetricsCore.Enabled = new(false)
	dda.Spec.Features.OrchestratorExplorer.Enabled = new(false)
	got, err = declaredGroups(dda, &dda.Spec)
	require.NoError(t, err)
	assert.Empty(t, got, "core isn't deployed when none of its features are on")
}

func TestGroupsVersionGate(t *testing.T) {
	groups := map[string]string{groupsKey: `[{"name":"http","checks":["http_check"]},{"name":"redis","checks":["redisdb"],"override":{"image":{"tag":"7.86.1"}}}]`}
	withRunnerImage := func(image v2alpha1.AgentImageConfig) map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride {
		override := images786()
		override[v2alpha1.ClusterChecksRunnerComponentName].Image = &image
		return override
	}

	tests := []struct {
		name       string
		dda        *v2alpha1.DatadogAgent
		wantGroups []string
		wantErr    bool
	}{
		{"both images recent enough", newDDA(true, false, groups, images786()), []string{"http", "redis"}, false},
		{"Cluster Agent too old", newDDA(true, false, groups, func() map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride {
			o := images786()
			o[v2alpha1.ClusterAgentComponentName].Image.Tag = "7.85.2"
			return o
		}()), nil, true},
		{"runners too old, a group overrides its image", newDDA(true, false, groups, withRunnerImage(v2alpha1.AgentImageConfig{Tag: "7.85.0"})), []string{"redis"}, true},
		{"a tag in the image name wins over the tag field", newDDA(true, false, groups, withRunnerImage(v2alpha1.AgentImageConfig{Name: "gcr.io/datadoghq/agent:7.85.0", Tag: "7.86.0"})), []string{"redis"}, true},
		{"release candidate", newDDA(true, false, groups, withRunnerImage(v2alpha1.AgentImageConfig{Tag: "7.86.0-rc.1"})), []string{"http", "redis"}, false},
		{"unparseable versions are assumed recent enough", newDDA(true, false, groups, withRunnerImage(v2alpha1.AgentImageConfig{Tag: "latest"})), []string{"http", "redis"}, false},
		{"cluster checks disabled", newDDA(false, false, groups, images786()), nil, false},
		{"Cluster Agent disabled", newDDA(true, false, groups, func() map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride {
			o := images786()
			o[v2alpha1.ClusterAgentComponentName].Disabled = new(true)
			return o
		}()), nil, false},
		{"runners component disabled", newDDA(true, false, groups, func() map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride {
			o := images786()
			o[v2alpha1.ClusterChecksRunnerComponentName].Disabled = new(true)
			return o
		}()), nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Groups(tt.dda, &tt.dda.Spec)
			assert.Equal(t, tt.wantGroups, groupNames(got))
			assert.Equal(t, tt.wantErr, err != nil, "error: %v", err)
		})
	}
}

func TestRunnerVersion(t *testing.T) {
	annotations := map[string]string{coreKey: "true", groupsKey: `[{"name":"http","checks":["http_check"],"override":{"image":{"tag":"7.87.0"}}}]`}
	dda := newDDA(true, false, annotations, images786())

	assert.Equal(t, "7.87.0", RunnerVersion(dda, &dda.Spec, "http_check"), "a group's own image")
	assert.Equal(t, "7.86.0", RunnerVersion(dda, &dda.Spec, constants.KubeStateMetricsCoreCheckName), "core inherits the runner image")
	assert.Equal(t, "7.86.0", RunnerVersion(dda, &dda.Spec, "redisdb"), "unclaimed checks use the default runners' image")

	untagged := newDDA(true, false, nil, map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
		v2alpha1.ClusterChecksRunnerComponentName: {Image: &v2alpha1.AgentImageConfig{Name: "registry.example.com/agent"}},
	})
	assert.Equal(t, images.AgentLatestVersion, RunnerVersion(untagged, &untagged.Spec, "redisdb"), "an image name without a tag keeps the default version")
}

func TestPlacement(t *testing.T) {
	core := map[string]string{coreKey: "true"}
	http := map[string]string{groupsKey: `[{"name":"http","checks":["http_check"]}]`}

	tests := []struct {
		name         string
		dda          *v2alpha1.DatadogAgent
		wantRequired bool
		wantOnRunner map[string]bool
	}{
		{"cluster checks disabled, core on", newDDA(false, false, core, images786()), false, map[string]bool{constants.KubeStateMetricsCoreCheckName: false}},
		{"runners off, no groups", newDDA(true, false, nil, images786()), false, map[string]bool{constants.KubeStateMetricsCoreCheckName: false}},
		{"runners on, no groups", newDDA(true, true, nil, images786()), true, map[string]bool{constants.KubeStateMetricsCoreCheckName: true, constants.HelmCheckName: true}},
		{"runners off, core on", newDDA(true, false, core, images786()), true, map[string]bool{
			constants.KubeStateMetricsCoreCheckName: true,
			constants.OrchestratorCheckName:         true,
			constants.HelmCheckName:                 true,
			"http_check":                            false,
		}},
		{"runners off, unrelated user group", newDDA(true, false, http, images786()), true, map[string]bool{constants.KubeStateMetricsCoreCheckName: false, "http_check": true}},
		{"runners off, malformed groups", newDDA(true, false, map[string]string{groupsKey: "not-json"}, images786()), false, map[string]bool{constants.KubeStateMetricsCoreCheckName: false}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantRequired, RunnersRequired(tt.dda, &tt.dda.Spec))
			for check, want := range tt.wantOnRunner {
				assert.Equal(t, want, RunsOnRunners(tt.dda, &tt.dda.Spec, check), check)
			}
		})
	}
}
