// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package experimental_test

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

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
	require.NoError(t, experimental.ApplyWithoutInitConfig(context.Background(), fake.NewClientBuilder().Build(), ddai, template))
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
	require.NoError(t, experimental.ApplyWithoutInitConfig(context.Background(), fake.NewClientBuilder().Build(), ddai, template))
	assert.Equal(t, after, template)
}

func TestWithoutInitConfigDisabled(t *testing.T) {
	for _, value := range []string{"", "false"} {
		t.Run(value, func(t *testing.T) {
			ddai, template := startupTemplate()
			ddai.Annotations[startupAnnotation] = value
			original := template.DeepCopy()
			require.NoError(t, experimental.ApplyWithoutInitConfig(context.Background(), fake.NewClientBuilder().Build(), ddai, template))
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
		{"read-only configuration parent", func(_ *v1alpha1.DatadogAgentInternal, p *corev1.PodTemplateSpec) {
			for i := range p.Spec.Containers[0].VolumeMounts {
				m := &p.Spec.Containers[0].VolumeMounts[i]
				if m.Name == common.ConfigVolumeName && m.MountPath == common.ConfigVolumePath {
					m.ReadOnly = true
				}
			}
		}, "requires a writable shared configuration mount"},
		{"missing init-volume", func(_ *v1alpha1.DatadogAgentInternal, p *corev1.PodTemplateSpec) { p.Spec.InitContainers = nil }, "requires init-volume"},
		{"unknown ConfigMap keys", func(_ *v1alpha1.DatadogAgentInternal, p *corev1.PodTemplateSpec) {
			for i := range p.Spec.Volumes {
				if p.Spec.Volumes[i].Name == "confd" {
					p.Spec.Volumes[i].ConfigMap.Items = nil
				}
			}
		}, "not found"},
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
			require.ErrorContains(t, experimental.ApplyWithoutInitConfig(context.Background(), fake.NewClientBuilder().Build(), ddai, template), tt.errorText)
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
	require.NoError(t, experimental.ApplyWithoutInitConfig(context.Background(), fake.NewClientBuilder().Build(), ddai, template))
	for _, container := range template.Spec.Containers {
		for _, file := range []string{"redis-cache.yaml", "redis-queue.yaml"} {
			assert.Contains(t, container.VolumeMounts, corev1.VolumeMount{Name: "confd", MountPath: "/etc/datadog-agent/conf.d/" + file, SubPath: file, ReadOnly: true})
		}
		assert.Contains(t, container.VolumeMounts, corev1.VolumeMount{Name: "checksd", MountPath: "/etc/datadog-agent/checks.d/check.py", SubPath: "check.py", ReadOnly: true})
	}
}

func TestWithoutInitConfigExternalConfigMaps(t *testing.T) {
	for _, volume := range []string{"confd", "checksd"} {
		for _, tc := range []struct {
			name     string
			data     map[string]string
			binary   map[string][]byte
			missing  bool
			optional bool
			want     []string
		}{
			{name: "all files", data: map[string]string{"z.yaml": "", "b.py": "", "notes.txt": "ignored"}, binary: map[string][]byte{"a.yaml": {}, "a.py": {}}, want: []string{"a.yaml", "z.yaml"}},
			{name: "empty ConfigMap"},
			{name: "missing ConfigMap", missing: true},
			{name: "optional missing ConfigMap", missing: true, optional: true},
		} {
			t.Run(volume+"/"+tc.name, func(t *testing.T) {
				ddai, template := startupTemplate()
				ddai.Namespace = "test"
				for i := range template.Spec.Volumes {
					v := &template.Spec.Volumes[i]
					if v.Name == volume || v.Name == "confd" {
						*v = corev1.Volume{Name: v.Name, VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}}
					}
					if v.Name == volume {
						v.EmptyDir = nil
						v.ConfigMap = &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "files"}, Optional: ptr.To(tc.optional)}
					}
				}
				cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "files", Namespace: "test"}, Data: tc.data, BinaryData: tc.binary}
				builder := fake.NewClientBuilder()
				if !tc.missing {
					builder.WithObjects(cm)
				}
				reader := builder.Build()
				before := template.DeepCopy()
				err := experimental.ApplyWithoutInitConfig(context.Background(), reader, ddai, template)
				if tc.missing && !tc.optional {
					require.ErrorContains(t, err, "read ConfigMap test/files")
					assert.Equal(t, before, template)
					return
				}
				require.NoError(t, err)
				want := tc.want
				if volume == "checksd" && len(want) > 0 {
					want = []string{"a.py", "b.py"}
				}
				for _, container := range template.Spec.Containers {
					var paths []string
					for _, mount := range container.VolumeMounts {
						if mount.Name == volume && mount.SubPath != "" {
							paths = append(paths, mount.SubPath)
							assert.True(t, mount.ReadOnly)
						}
					}
					assert.Equal(t, want, paths, container.Name)
				}
			})
		}
	}
}

func TestWithoutInitConfigExternalConfigMapUpdates(t *testing.T) {
	ddai, template := startupTemplate()
	ddai.Namespace = "test"
	for i := range template.Spec.Volumes {
		if v := &template.Spec.Volumes[i]; v.Name == "confd" {
			v.ConfigMap.Items = nil
		}
	}
	ctx := context.Background()
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "redis", Namespace: "test"}, Data: map[string]string{"old.yaml": ""}}
	reader := fake.NewClientBuilder().WithObjects(cm).Build()
	for _, filename := range []string{"old.yaml", "new.yaml"} {
		cm.Data = map[string]string{filename: ""}
		require.NoError(t, reader.Update(ctx, cm))
		updated := template.DeepCopy()
		require.NoError(t, experimental.ApplyWithoutInitConfig(ctx, reader, ddai, updated))
		for _, container := range updated.Spec.Containers {
			var files []string
			for _, m := range container.VolumeMounts {
				if m.Name == "confd" && m.SubPath != "" {
					files = append(files, m.SubPath)
				}
			}
			assert.Equal(t, []string{filename}, files)
		}
	}
	// Removing the ConfigMap must preserve the last template on a failed read.
	require.NoError(t, reader.Delete(ctx, cm))
	before := template.DeepCopy()
	require.ErrorContains(t, experimental.ApplyWithoutInitConfig(ctx, reader, ddai, template), "not found")
	assert.Equal(t, before, template)
}

func TestWithoutInitConfigExplicitItemsDoNotRequireConfigMapRead(t *testing.T) {
	ddai, template := startupTemplate()
	ddai.Namespace = "test"
	cm := &corev1.ConfigMap{}
	reader := fake.NewClientBuilder().Build()
	require.Error(t, reader.Get(context.Background(), types.NamespacedName{Namespace: "test", Name: "redis"}, cm))
	// Explicit nested mappings are sufficient even before the ConfigMap exists.
	require.NoError(t, experimental.ApplyWithoutInitConfig(context.Background(), reader, ddai, template))
}

func TestWithoutInitConfigOptionalItems(t *testing.T) {
	ddai, template := startupTemplate()
	ddai.Namespace = "test"
	for i := range template.Spec.Volumes {
		if v := &template.Spec.Volumes[i]; v.Name == "confd" {
			v.ConfigMap.Optional = ptr.To(true)
		}
	}
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "redis", Namespace: "test"}, Data: map[string]string{"cache": ""}}
	reader := fake.NewClientBuilder().WithObjects(cm).Build()
	require.NoError(t, experimental.ApplyWithoutInitConfig(context.Background(), reader, ddai, template))
	for _, c := range template.Spec.Containers {
		var paths []string
		for _, m := range c.VolumeMounts {
			if m.Name == "confd" && m.SubPath != "" {
				paths = append(paths, m.SubPath)
			}
		}
		assert.Equal(t, []string{"redisdb.d/cache.yaml"}, paths)
	}
}

func TestWithoutInitConfigEmptyInlineFiles(t *testing.T) {
	ddai, template := startupTemplate()
	settings := ddai.Spec.Override[v2alpha1.NodeAgentComponentName]
	settings.ExtraConfd = &v2alpha1.MultiCustomConfig{ConfigDataMap: map[string]string{}}
	settings.ExtraChecksd = &v2alpha1.MultiCustomConfig{ConfigDataMap: map[string]string{}}
	ds := componentagent.NewDefaultAgentDaemonset(ddai, feature.RequiredComponent{Containers: []apicommon.AgentContainerName{apicommon.CoreAgentContainerName}}, "test-agent")
	template = &ds.Spec.Template
	override.PodTemplateSpec(logr.Discard(), feature.NewPodTemplateManagers(template), settings, v2alpha1.NodeAgentComponentName, ddai.Name)
	require.NoError(t, experimental.ApplyWithoutInitConfig(context.Background(), fake.NewClientBuilder().Build(), ddai, template))
	for _, c := range template.Spec.Containers {
		for _, m := range c.VolumeMounts {
			if m.Name == "confd" || m.Name == "checksd" {
				assert.Empty(t, m.SubPath)
			}
		}
	}
}

func TestWithoutInitConfigManualInputs(t *testing.T) {
	for _, tc := range []struct {
		name       string
		mountPath  string
		subPath    string
		nested     bool
		wantPath   string
		wantSource string
	}{
		{name: "override only confd", mountPath: "/conf.d", wantPath: "conf.d/redis.yaml", wantSource: "redis.yaml"},
		{name: "override replaces CRD input", nested: true, mountPath: "/conf.d", wantPath: "conf.d/redis.yaml", wantSource: "redis.yaml"},
		{name: "override only checksd", mountPath: "/checks.d", wantPath: "checks.d/check.py", wantSource: "check.py"},
		{name: "file subPath renamed", mountPath: "/conf.d/renamed.yaml", subPath: "redis.yaml", wantPath: "conf.d/renamed.yaml", wantSource: "redis.yaml"},
		{name: "directory subPath", mountPath: "/conf.d/redisdb.d", subPath: "nested", wantPath: "conf.d/redisdb.d/cache.yaml", wantSource: "nested/cache.yaml"},
		{name: "nested input", nested: true, mountPath: "/conf.d/redisdb.d", wantPath: "conf.d/redisdb.d/redis.yaml", wantSource: "redis.yaml"},
		{name: "nested file hides CRD file", nested: true, mountPath: "/conf.d/redisdb.d/cache.yaml", subPath: "redis.yaml", wantPath: "conf.d/redisdb.d/cache.yaml", wantSource: "redis.yaml"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ddai, template := startupTemplate()
			settings := ddai.Spec.Override[v2alpha1.NodeAgentComponentName]
			if !tc.nested {
				settings.ExtraConfd = nil
			}
			settings.Volumes = []corev1.Volume{{Name: "custom-files", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: "manual"},
				Items:                []corev1.KeyToPath{{Key: "redis", Path: "redis.yaml"}, {Key: "python", Path: "check.py"}, {Key: "nested", Path: "nested/cache.yaml"}},
			}}}}
			settings.Containers = map[apicommon.AgentContainerName]*v2alpha1.DatadogAgentGenericContainer{
				"init-config": {VolumeMounts: []corev1.VolumeMount{{Name: "custom-files", MountPath: tc.mountPath, SubPath: tc.subPath, ReadOnly: true}}},
			}
			// Build a fresh template so override-only cases do not retain earlier CRD volumes.
			ds := componentagent.NewDefaultAgentDaemonset(ddai, feature.RequiredComponent{Containers: []apicommon.AgentContainerName{
				apicommon.CoreAgentContainerName, apicommon.TraceAgentContainerName,
			}}, "test-agent")
			template = &ds.Spec.Template
			override.PodTemplateSpec(logr.Discard(), feature.NewPodTemplateManagers(template), settings, v2alpha1.NodeAgentComponentName, ddai.Name)
			require.NoError(t, experimental.ApplyWithoutInitConfig(context.Background(), fake.NewClientBuilder().Build(), ddai, template))
			for _, c := range template.Spec.Containers {
				assert.Contains(t, c.VolumeMounts, corev1.VolumeMount{Name: "custom-files", MountPath: "/etc/datadog-agent/" + tc.wantPath, SubPath: tc.wantSource, ReadOnly: true})
				if tc.name == "nested file hides CRD file" {
					assert.Contains(t, c.VolumeMounts, corev1.VolumeMount{Name: "confd", MountPath: "/etc/datadog-agent/conf.d/redisdb.d/queue.yaml", SubPath: "redisdb.d/queue.yaml", ReadOnly: true})
				}
				if tc.nested {
					for _, m := range c.VolumeMounts {
						assert.False(t, m.Name == "confd" && m.SubPath == "redisdb.d/cache.yaml", "hidden CRD file must not be mounted")
					}
				}
			}
			for _, c := range template.Spec.InitContainers {
				assert.NotEqual(t, "init-config", c.Name)
			}
		})
	}
}

func TestWithoutInitConfigManualInputErrors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*corev1.PodTemplateSpec)
		want   string
	}{
		{name: "missing volume", change: func(p *corev1.PodTemplateSpec) {
			p.Spec.InitContainers[1].VolumeMounts = append(p.Spec.InitContainers[1].VolumeMounts, corev1.VolumeMount{Name: "missing", MountPath: "/conf.d/manual"})
		}, want: "references missing volume"},
		{name: "dynamic subPath", change: func(p *corev1.PodTemplateSpec) {
			p.Spec.InitContainers[1].VolumeMounts = append(p.Spec.InitContainers[1].VolumeMounts, corev1.VolumeMount{Name: "confd", MountPath: "/conf.d/manual", SubPathExpr: "$(CONFIG)"})
		}, want: "unsupported init-config mount"},
		{name: "missing subPath", change: func(p *corev1.PodTemplateSpec) {
			p.Spec.InitContainers[1].VolumeMounts = append(p.Spec.InitContainers[1].VolumeMounts, corev1.VolumeMount{Name: "confd", MountPath: "/conf.d/manual", SubPath: "missing.yaml"})
		}, want: "references missing subPath"},
		{name: "custom emptyDir", change: func(p *corev1.PodTemplateSpec) {
			p.Spec.Volumes = append(p.Spec.Volumes, corev1.Volume{Name: "dynamic", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}})
			p.Spec.InitContainers[1].VolumeMounts = append(p.Spec.InitContainers[1].VolumeMounts, corev1.VolumeMount{Name: "dynamic", MountPath: "/conf.d/manual"})
		}, want: "volume dynamic must use a ConfigMap"},
		{name: "Secret", change: func(p *corev1.PodTemplateSpec) {
			p.Spec.Volumes = append(p.Spec.Volumes, corev1.Volume{Name: "private", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: "configs"}}})
			p.Spec.InitContainers[1].VolumeMounts = append(p.Spec.InitContainers[1].VolumeMounts, corev1.VolumeMount{Name: "private", MountPath: "/checks.d/manual"})
		}, want: "volume private must use a ConfigMap"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ddai, template := startupTemplate()
			tc.change(template)
			before := template.DeepCopy()
			require.ErrorContains(t, experimental.ApplyWithoutInitConfig(context.Background(), fake.NewClientBuilder().Build(), ddai, template), tc.want)
			assert.Equal(t, before, template)
		})
	}
}

func TestWithoutInitConfigManualDestinationMounts(t *testing.T) {
	for _, tc := range []struct {
		name     string
		mount    corev1.VolumeMount
		conflict bool
	}{
		{name: "disjoint", mount: corev1.VolumeMount{Name: "manual", MountPath: "/etc/datadog-agent/conf.d/other.yaml"}},
		{name: "identical", mount: corev1.VolumeMount{Name: "confd", MountPath: "/etc/datadog-agent/conf.d/redisdb.d/cache.yaml", SubPath: "redisdb.d/cache.yaml", ReadOnly: true}},
		{name: "same file", mount: corev1.VolumeMount{Name: "manual", MountPath: "/etc/datadog-agent/conf.d/redisdb.d/cache.yaml"}, conflict: true},
		{name: "parent", mount: corev1.VolumeMount{Name: "manual", MountPath: "/etc/datadog-agent/conf.d/redisdb.d"}, conflict: true},
		{name: "child", mount: corev1.VolumeMount{Name: "manual", MountPath: "/etc/datadog-agent/conf.d/redisdb.d/cache.yaml/child"}, conflict: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ddai, template := startupTemplate()
			template.Spec.Containers[0].VolumeMounts = append(template.Spec.Containers[0].VolumeMounts, tc.mount)
			before := template.DeepCopy()
			err := experimental.ApplyWithoutInitConfig(context.Background(), fake.NewClientBuilder().Build(), ddai, template)
			if tc.conflict {
				require.ErrorContains(t, err, "conflicting mount")
				assert.Equal(t, before, template)
				return
			}
			require.NoError(t, err)
			count := 0
			for _, m := range template.Spec.Containers[0].VolumeMounts {
				if reflect.DeepEqual(m, tc.mount) {
					count++
				}
			}
			assert.Equal(t, 1, count)
			if tc.name == "disjoint" {
				assert.NotContains(t, template.Spec.Containers[1].VolumeMounts, tc.mount)
			}
		})
	}
}

func TestWithoutInitConfigPermissions(t *testing.T) {
	for _, mode := range []int32{0400, 0440, 0600, 0640, 0644} {
		for _, optional := range []bool{false, true} {
			t.Run(fmt.Sprintf("mode_%04o/optional_%t", mode, optional), func(t *testing.T) {
				ddai, template := startupTemplate()
				ddai.Namespace = "test"
				template.Spec.SecurityContext = &corev1.PodSecurityContext{
					RunAsUser: ptr.To[int64](1000), RunAsGroup: ptr.To[int64](1000), FSGroup: ptr.To[int64](2000),
				}
				template.Spec.Containers[0].SecurityContext = &corev1.SecurityContext{ReadOnlyRootFilesystem: ptr.To(true)}
				for i := range template.Spec.Volumes {
					if v := &template.Spec.Volumes[i]; v.Name == "confd" {
						v.ConfigMap.DefaultMode = ptr.To(mode)
						v.ConfigMap.Optional = ptr.To(optional)
						v.ConfigMap.Items[0].Mode = ptr.To[int32](0400)
						// The second item inherits defaultMode.
						v.ConfigMap.Items[1].Mode = nil
					}
				}
				cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "redis", Namespace: "test"}, Data: map[string]string{"cache": "", "queue": ""}}
				reader := fake.NewClientBuilder().WithObjects(cm).Build()
				before := template.DeepCopy()
				require.NoError(t, experimental.ApplyWithoutInitConfig(context.Background(), reader, ddai, template))
				assert.Equal(t, before.Spec.Volumes, template.Spec.Volumes, "do not broaden or discard ConfigMap modes")
				assert.Equal(t, before.Spec.SecurityContext, template.Spec.SecurityContext, "preserve UID, GID and fsGroup")
				for i, c := range template.Spec.Containers {
					assert.Equal(t, before.Spec.Containers[i].SecurityContext, c.SecurityContext)
					for _, name := range []string{"cache", "queue"} {
						assert.Contains(t, c.VolumeMounts, corev1.VolumeMount{
							Name: "confd", MountPath: "/etc/datadog-agent/conf.d/redisdb.d/" + name + ".yaml",
							SubPath: "redisdb.d/" + name + ".yaml", ReadOnly: true,
						})
					}
				}
			})
		}
	}
}
