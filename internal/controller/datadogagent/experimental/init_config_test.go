// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package experimental_test

import (
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	componentagent "github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/agent"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/experimental"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/override"
)

const startupAnnotation = experimental.ExperimentalAnnotationPrefix + "/" + experimental.ExperimentalWithoutInitConfigSubkey

func startupTemplate() (*v1alpha1.DatadogAgentInternal, *corev1.PodTemplateSpec) {
	ddai := &v1alpha1.DatadogAgentInternal{ObjectMeta: metav1.ObjectMeta{Name: "test", Annotations: map[string]string{startupAnnotation: "true"}}}
	config := "api_key: ENC[test]\n"
	ddai.Spec.Override = map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
		v2alpha1.NodeAgentComponentName: {
			CustomConfigurations: map[v2alpha1.AgentConfigFileName]v2alpha1.CustomConfig{v2alpha1.AgentGeneralConfigFile: {ConfigData: &config}},
			ExtraConfd: &v2alpha1.MultiCustomConfig{ConfigMap: &v2alpha1.ConfigMapConfig{Name: "redis", Items: []corev1.KeyToPath{
				{Key: "cache", Path: "redisdb.d/cache.yaml"}, {Key: "queue", Path: "redisdb.d/queue.yaml"},
			}}},
		},
	}
	ds := componentagent.NewDefaultAgentDaemonset(ddai, feature.RequiredComponent{Containers: []apicommon.AgentContainerName{
		apicommon.CoreAgentContainerName, apicommon.TraceAgentContainerName, apicommon.ProcessAgentContainerName, apicommon.SystemProbeContainerName,
		apicommon.SecurityAgentContainerName, apicommon.OtelAgent, apicommon.HostProfiler, apicommon.PrivateActionRunnerContainerName,
		apicommon.AgentDataPlaneContainerName, apicommon.AgentCheckRunnerContainerName,
	}}, "test-agent")
	manager := feature.NewPodTemplateManagers(&ds.Spec.Template)
	override.PodTemplateSpec(logr.Discard(), manager, ddai.Spec.Override[v2alpha1.NodeAgentComponentName], v2alpha1.NodeAgentComponentName, ddai.Name)
	return ddai, &ds.Spec.Template
}

func TestWithoutInitConfigMounts(t *testing.T) {
	ddai, template := startupTemplate()
	template.Spec.Containers = append(template.Spec.Containers, corev1.Container{Name: "unrelated-sidecar"})
	template.Spec.InitContainers = append(template.Spec.InitContainers, corev1.Container{Name: "host-profiler-seccomp-setup"})
	original := template.DeepCopy()
	require.NoError(t, experimental.ApplyWithoutInitConfig(ddai, template))
	var names []string
	for _, container := range template.Spec.InitContainers {
		names = append(names, container.Name)
	}
	assert.Equal(t, []string{"init-volume", "seccomp-setup", "host-profiler-seccomp-setup"}, names)
	for i, container := range template.Spec.Containers {
		if container.Name == "unrelated-sidecar" {
			assert.Empty(t, container.VolumeMounts)
			continue
		}
		assert.Subset(t, container.VolumeMounts, original.Spec.Containers[i].VolumeMounts)
		assert.Equal(t, original.Spec.Containers[i].Env, container.Env)
		assert.Equal(t, original.Spec.Containers[i].Command, container.Command)
		assert.Equal(t, original.Spec.Containers[i].Args, container.Args)
		assert.Contains(t, container.VolumeMounts, common.GetVolumeMountForConfig())
		for _, file := range []string{"cache.yaml", "queue.yaml"} {
			assert.Contains(t, container.VolumeMounts, corev1.VolumeMount{Name: "confd", MountPath: "/etc/datadog-agent/conf.d/redisdb.d/" + file, SubPath: "redisdb.d/" + file, ReadOnly: true}, container.Name)
		}
	}
	assert.Equal(t, original.Spec.Volumes, template.Spec.Volumes)
	// Applying it twice must preserve mount ordering and avoid duplicate mounts.
	after := template.DeepCopy()
	require.NoError(t, experimental.ApplyWithoutInitConfig(ddai, template))
	assert.Equal(t, after, template)
}

func TestWithoutInitConfigDisabled(t *testing.T) {
	for _, value := range []string{"", "false"} {
		t.Run(value, func(t *testing.T) {
			ddai, template := startupTemplate()
			ddai.Annotations[startupAnnotation] = value
			original := template.DeepCopy()
			require.NoError(t, experimental.ApplyWithoutInitConfig(ddai, template))
			assert.Equal(t, original, template)
		})
	}
}

func TestWithoutInitConfigInvalid(t *testing.T) {
	tests := []struct {
		name      string
		change    func(*v1alpha1.DatadogAgentInternal, *corev1.PodTemplateSpec)
		errorText string
	}{
		{"invalid annotation", func(d *v1alpha1.DatadogAgentInternal, _ *corev1.PodTemplateSpec) {
			d.Annotations[startupAnnotation] = "yes"
		}, "must be true or false"},
		{"missing main YAML", func(_ *v1alpha1.DatadogAgentInternal, p *corev1.PodTemplateSpec) {
			p.Spec.Containers[0].VolumeMounts = []corev1.VolumeMount{common.GetVolumeMountForConfig()}
		}, "requires an explicit"},
		{"missing init-volume", func(_ *v1alpha1.DatadogAgentInternal, p *corev1.PodTemplateSpec) { p.Spec.InitContainers = nil }, "requires init-volume"},
		{"unknown ConfigMap keys", func(_ *v1alpha1.DatadogAgentInternal, p *corev1.PodTemplateSpec) {
			for i := range p.Spec.Volumes {
				if p.Spec.Volumes[i].Name == "confd" {
					p.Spec.Volumes[i].ConfigMap.Items = nil
				}
			}
		}, "requires explicit items"},
		{"path traversal", func(_ *v1alpha1.DatadogAgentInternal, p *corev1.PodTemplateSpec) {
			for i := range p.Spec.Volumes {
				if p.Spec.Volumes[i].Name == "confd" {
					p.Spec.Volumes[i].ConfigMap.Items[0].Path = "../datadog.yaml"
				}
			}
		}, "unsafe or duplicate"},
		{"duplicate destination", func(_ *v1alpha1.DatadogAgentInternal, p *corev1.PodTemplateSpec) {
			for i := range p.Spec.Volumes {
				if p.Spec.Volumes[i].Name == "confd" {
					p.Spec.Volumes[i].ConfigMap.Items[1].Path = "redisdb.d/cache.yaml"
				}
			}
		}, "unsafe or duplicate"},
		{"conflicting directory mount", func(_ *v1alpha1.DatadogAgentInternal, p *corev1.PodTemplateSpec) {
			p.Spec.Containers[1].VolumeMounts = append(p.Spec.Containers[1].VolumeMounts, corev1.VolumeMount{Name: "other", MountPath: "/etc/datadog-agent/conf.d"})
		}, "conflicting mount"},
		{"unsupported script branch", func(_ *v1alpha1.DatadogAgentInternal, p *corev1.PodTemplateSpec) {
			p.Spec.Containers[0].Env = append(p.Spec.Containers[0].Env, corev1.EnvVar{Name: "DD_INSIDE_CI", Value: "true"})
		}, "outside this experiment"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ddai, template := startupTemplate()
			tt.change(ddai, template)
			before := template.DeepCopy()
			require.ErrorContains(t, experimental.ApplyWithoutInitConfig(ddai, template), tt.errorText)
			assert.Equal(t, before, template, "invalid experiment must leave the template untouched")
		})
	}
}

func TestWithoutInitConfigInlineFiles(t *testing.T) {
	ddai, template := startupTemplate()
	settings := ddai.Spec.Override[v2alpha1.NodeAgentComponentName]
	settings.ExtraConfd = &v2alpha1.MultiCustomConfig{ConfigDataMap: map[string]string{
		"redis-cache.yaml": "instances:\n- host: cache\n  port: 6379\n- host: cache-2\n  port: 6379\n",
		"redis-queue.yaml": "instances:\n- host: queue\n  port: 6379\n",
	}}
	settings.ExtraChecksd = &v2alpha1.MultiCustomConfig{ConfigDataMap: map[string]string{"check.py": "from datadog_checks.base import AgentCheck\nclass Check(AgentCheck):\n    pass\n"}}
	override.PodTemplateSpec(logr.Discard(), feature.NewPodTemplateManagers(template), settings, v2alpha1.NodeAgentComponentName, ddai.Name)
	require.NoError(t, experimental.ApplyWithoutInitConfig(ddai, template))
	for _, container := range template.Spec.Containers {
		for _, file := range []string{"redis-cache.yaml", "redis-queue.yaml"} {
			assert.Contains(t, container.VolumeMounts, corev1.VolumeMount{Name: "confd", MountPath: "/etc/datadog-agent/conf.d/" + file, SubPath: file, ReadOnly: true})
		}
		assert.Contains(t, container.VolumeMounts, corev1.VolumeMount{Name: "checksd", MountPath: "/etc/datadog-agent/checks.d/check.py", SubPath: "check.py", ReadOnly: true})
	}
}
