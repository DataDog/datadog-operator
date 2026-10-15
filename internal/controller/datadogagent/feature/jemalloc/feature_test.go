// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package jemalloc

import (
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	featureutils "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/merger"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/override"
)

func TestConfigure(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		enabled     bool
	}{
		{name: "annotations omitted"},
		{name: "annotation omitted", annotations: map[string]string{"unrelated": "true"}},
		{name: "disabled", annotations: map[string]string{featureutils.EnableJemallocAnnotation: "false"}},
		{name: "empty value", annotations: map[string]string{featureutils.EnableJemallocAnnotation: ""}},
		{name: "invalid value", annotations: map[string]string{featureutils.EnableJemallocAnnotation: "TRUE"}},
		{name: "enabled", annotations: map[string]string{featureutils.EnableJemallocAnnotation: "true"}, enabled: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := buildJemallocFeature(nil)
			assert.Equal(t, feature.IDType(feature.JemallocIDType), f.ID())
			required := f.Configure(&metav1.ObjectMeta{Annotations: tt.annotations}, nil, nil)
			assert.Equal(t, tt.enabled, required.IsEnabled())
			assert.Empty(t, required.ClusterAgent)
			assert.Nil(t, required.ClusterChecksRunner.IsRequired, "jemalloc must not enable runners")
			assert.Empty(t, required.OtelAgentGateway)
			if tt.enabled {
				assert.Equal(t, []apicommon.AgentContainerName{apicommon.CoreAgentContainerName}, required.Agent.Containers)
				assert.Equal(t, []apicommon.AgentContainerName{apicommon.ClusterChecksRunnersContainerName}, required.ClusterChecksRunner.Containers)
			} else {
				assert.Empty(t, required.Agent)
				assert.Empty(t, required.ClusterChecksRunner)
			}
			pod := nodeAgentPod()
			before := pod.DeepCopy()
			require.NoError(t, f.ManageNodeAgent(feature.NewPodTemplateManagers(pod)))
			require.NoError(t, f.ManageClusterChecksRunner(feature.NewPodTemplateManagers(pod)))
			if !tt.enabled {
				assert.Equal(t, before, pod)
			}
		})
	}
}

func TestVersionGate(t *testing.T) {
	tests := []struct {
		name        string
		nodeImage   *v2alpha1.AgentImageConfig
		runnerImage *v2alpha1.AgentImageConfig
		wantNode    bool
		wantRunner  bool
	}{
		{name: "default images", wantNode: true, wantRunner: true},
		{name: "empty image configuration", nodeImage: &v2alpha1.AgentImageConfig{}, runnerImage: &v2alpha1.AgentImageConfig{}, wantNode: true, wantRunner: true},
		{name: "both too old", nodeImage: &v2alpha1.AgentImageConfig{Tag: "7.83.9"}, runnerImage: &v2alpha1.AgentImageConfig{Tag: "7.83.9"}},
		{name: "minimum version", nodeImage: &v2alpha1.AgentImageConfig{Tag: "7.84.0"}, runnerImage: &v2alpha1.AgentImageConfig{Tag: "7.84.0"}, wantNode: true, wantRunner: true},
		{name: "minimum prerelease", nodeImage: &v2alpha1.AgentImageConfig{Tag: "7.84.0-rc.1"}, runnerImage: &v2alpha1.AgentImageConfig{Tag: "7.84.0-rc.1"}, wantNode: true, wantRunner: true},
		{name: "newer version", nodeImage: &v2alpha1.AgentImageConfig{Tag: "7.85.0"}, runnerImage: &v2alpha1.AgentImageConfig{Tag: "7.85.0"}, wantNode: true, wantRunner: true},
		{name: "old node new runner", nodeImage: &v2alpha1.AgentImageConfig{Tag: "7.83.0"}, runnerImage: &v2alpha1.AgentImageConfig{Tag: "7.84.0"}, wantRunner: true},
		{name: "new node old runner", nodeImage: &v2alpha1.AgentImageConfig{Tag: "7.84.0"}, runnerImage: &v2alpha1.AgentImageConfig{Tag: "7.83.0"}, wantNode: true},
		{name: "version from image name", nodeImage: &v2alpha1.AgentImageConfig{Name: "registry.example/agent:7.84.0-jmx"}, runnerImage: &v2alpha1.AgentImageConfig{Name: "registry.example/agent:7.83.0"}, wantNode: true},
		{name: "tag overrides image name", nodeImage: &v2alpha1.AgentImageConfig{Name: "registry.example/agent:7.83.0", Tag: "7.84.0"}, runnerImage: &v2alpha1.AgentImageConfig{Name: "registry.example/agent:7.84.0", Tag: "7.83.0"}, wantNode: true},
		{name: "unknown versions", nodeImage: &v2alpha1.AgentImageConfig{Tag: "latest"}, runnerImage: &v2alpha1.AgentImageConfig{Name: "registry.example/agent:custom"}, wantNode: true, wantRunner: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := buildJemallocFeature(nil)
			spec := &v2alpha1.DatadogAgentSpec{Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
				v2alpha1.NodeAgentComponentName:           {Image: tt.nodeImage},
				v2alpha1.ClusterChecksRunnerComponentName: {Image: tt.runnerImage},
			}}
			required := f.Configure(jemallocMetadata(true), spec, nil)
			assert.Equal(t, tt.wantNode, required.Agent.IsEnabled())
			assert.Equal(t, tt.wantRunner, required.ClusterChecksRunner.IsConfigured())
			assert.Nil(t, required.ClusterChecksRunner.IsRequired, "must not create runners")
			for _, component := range []struct {
				manage func(feature.PodTemplateManagers) error
				want   bool
			}{
				{f.ManageNodeAgent, tt.wantNode},
				{f.ManageClusterChecksRunner, tt.wantRunner},
			} {
				pod := &corev1.PodTemplateSpec{Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: string(apicommon.CoreAgentContainerName)}},
				}}
				require.NoError(t, component.manage(feature.NewPodTemplateManagers(pod)))
				if component.want {
					assert.Equal(t, []corev1.EnvVar{
						{Name: ldPreload, Value: defaultPreload},
						{Name: mallocConf, Value: defaultMallocConf},
					}, pod.Spec.Containers[0].Env)
				} else {
					assert.Empty(t, pod.Spec.Containers[0].Env)
				}
			}
			// Reconfiguration must clear any previously enabled state.
			f.Configure(jemallocMetadata(false), spec, nil)
			pod := nodeAgentPod()
			require.NoError(t, f.ManageNodeAgent(feature.NewPodTemplateManagers(pod)))
			require.NoError(t, f.ManageClusterChecksRunner(feature.NewPodTemplateManagers(pod)))
			for _, c := range pod.Spec.Containers {
				assert.Empty(t, c.Env)
			}
		})
	}
}

func TestManageNodeAgent(t *testing.T) {
	f := buildJemallocFeature(nil)
	f.Configure(jemallocMetadata(true), nil, nil)
	pod := nodeAgentPod()
	require.NoError(t, f.ManageNodeAgent(feature.NewPodTemplateManagers(pod)))
	// Applying defaults twice must not duplicate environment variables or the library.
	require.NoError(t, f.ManageNodeAgent(feature.NewPodTemplateManagers(pod)))
	for _, container := range pod.Spec.Containers {
		switch apicommon.AgentContainerName(container.Name) {
		case apicommon.CoreAgentContainerName, apicommon.SystemProbeContainerName, apicommon.TraceAgentContainerName:
		default:
			assert.Empty(t, container.Env)
			continue
		}
		assert.Equal(t, []corev1.EnvVar{
			{Name: ldPreload, Value: defaultPreload},
			{Name: mallocConf, Value: defaultMallocConf},
		}, container.Env, container.Name)
	}
	for _, container := range pod.Spec.InitContainers {
		assert.Empty(t, container.Env, container.Name)
	}
	for _, manage := range []func(feature.PodTemplateManagers) error{
		f.ManageClusterAgent, f.ManageOtelAgentGateway, f.ManageSingleContainerNodeAgent,
	} {
		other := nodeAgentPod()
		before := other.DeepCopy()
		require.NoError(t, manage(feature.NewPodTemplateManagers(other)))
		assert.Equal(t, before, other)
	}
}

func TestManageClusterChecksRunner(t *testing.T) {
	f := buildJemallocFeature(nil)
	f.Configure(jemallocMetadata(true), nil, nil)
	pod := &corev1.PodTemplateSpec{Spec: corev1.PodSpec{
		Containers:     []corev1.Container{{Name: string(apicommon.ClusterChecksRunnersContainerName)}},
		InitContainers: []corev1.Container{{Name: "init-config"}},
	}}
	managers := feature.NewPodTemplateManagers(pod)
	require.NoError(t, f.ManageClusterChecksRunner(managers))
	require.NoError(t, f.ManageClusterChecksRunner(managers))
	assert.Equal(t, []corev1.EnvVar{
		{Name: ldPreload, Value: defaultPreload},
		{Name: mallocConf, Value: defaultMallocConf},
	}, pod.Spec.Containers[0].Env)
	assert.Empty(t, pod.Spec.InitContainers[0].Env)

	override.PodTemplateSpec(logr.Discard(), managers, &v2alpha1.DatadogAgentComponentOverride{
		Env: []corev1.EnvVar{{Name: mallocConf, Value: "dirty_decay_ms:1000"}},
		Containers: map[apicommon.AgentContainerName]*v2alpha1.DatadogAgentGenericContainer{
			apicommon.ClusterChecksRunnersContainerName: {
				Env: []corev1.EnvVar{{Name: ldPreload, Value: "/custom/allocator.so"}},
			},
		},
	}, v2alpha1.ClusterChecksRunnerComponentName, "datadog")
	assert.Equal(t, []corev1.EnvVar{
		{Name: ldPreload, Value: "/custom/allocator.so"},
		{Name: mallocConf, Value: "dirty_decay_ms:1000"},
	}, pod.Spec.Containers[0].Env)
}

func TestOverridePrecedence(t *testing.T) {
	ref := &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{
		LocalObjectReference: corev1.LocalObjectReference{Name: "allocator"},
		Key:                  "config",
	}}
	pod := &corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{
		{Name: string(apicommon.CoreAgentContainerName)},
		{Name: string(apicommon.TraceAgentContainerName)},
	}}}
	f := buildJemallocFeature(nil)
	f.Configure(jemallocMetadata(true), nil, nil)
	managers := feature.NewPodTemplateManagers(pod)
	require.NoError(t, f.ManageNodeAgent(managers))
	override.PodTemplateSpec(logr.Discard(), managers, &v2alpha1.DatadogAgentComponentOverride{
		Env: []corev1.EnvVar{
			{Name: ldPreload, Value: "/custom/profiler.so " + libraryPath},
			{Name: mallocConf, Value: "dirty_decay_ms:1000"},
		},
		Containers: map[apicommon.AgentContainerName]*v2alpha1.DatadogAgentGenericContainer{
			apicommon.TraceAgentContainerName: {Env: []corev1.EnvVar{
				{Name: ldPreload, Value: ""},
				{Name: mallocConf, ValueFrom: ref},
			}},
		},
	}, v2alpha1.NodeAgentComponentName, "datadog")
	assert.Equal(t, []corev1.EnvVar{
		{Name: ldPreload, Value: "/custom/profiler.so " + libraryPath},
		{Name: mallocConf, Value: "dirty_decay_ms:1000"},
	}, pod.Spec.Containers[0].Env)
	assert.Equal(t, []corev1.EnvVar{
		{Name: ldPreload, Value: ""},
		{Name: mallocConf, ValueFrom: ref},
	}, pod.Spec.Containers[1].Env)
}

func jemallocMetadata(enabled bool) *metav1.ObjectMeta {
	value := "false"
	if enabled {
		value = "true"
	}
	return &metav1.ObjectMeta{Annotations: map[string]string{featureutils.EnableJemallocAnnotation: value}}
}

func nodeAgentPod() *corev1.PodTemplateSpec {
	pod := &corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Name: "datadog"}}
	for name := range merger.AllAgentContainers {
		if name != apicommon.ClusterAgentContainerName {
			pod.Spec.Containers = append(pod.Spec.Containers, corev1.Container{Name: string(name)})
		}
	}
	pod.Spec.Containers = append(pod.Spec.Containers, corev1.Container{Name: "custom-sidecar"})
	pod.Spec.InitContainers = []corev1.Container{{Name: "init-config"}, {Name: string(apicommon.CoreAgentContainerName)}}
	return pod
}
