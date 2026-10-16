// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package renderer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/component"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

func TestRender_Jemalloc(t *testing.T) {
	const preload = "/opt/lib/nosys.so /opt/datadog-agent/embedded/lib/libjemalloc.so"
	for _, tt := range []struct {
		name          string
		enabled       *bool
		comprehensive bool
	}{
		{name: "omitted"},
		{name: "disabled", enabled: new(false)},
		{name: "enabled", enabled: new(true)},
		{name: "optional containers", enabled: new(true), comprehensive: true},
		{name: "runners annotation omitted", comprehensive: true},
		{name: "runners annotation disabled", enabled: new(false), comprehensive: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := "testdata/minimal-dda.yaml"
			if tt.comprehensive {
				path = "testdata/comprehensive-dda.yaml"
			}
			dda, err := LoadDDA(path)
			require.NoError(t, err)
			if dda.Spec.Features == nil {
				dda.Spec.Features = &v2alpha1.DatadogFeatures{}
			}
			if tt.enabled != nil {
				if dda.Annotations == nil {
					dda.Annotations = map[string]string{}
				}
				value := "false"
				if *tt.enabled {
					value = "true"
				}
				dda.Annotations[component.EnableJemallocAnnotation] = value
			}
			if tt.comprehensive {
				dda.Spec.Features.CSPM = &v2alpha1.CSPMFeatureConfig{
					Enabled:          new(true),
					RunInSystemProbe: new(false),
				}
			}
			objects, _, err := Render(Options{DDA: dda})
			require.NoError(t, err)
			var expected []corev1.EnvVar
			if tt.enabled != nil && *tt.enabled {
				expected = []corev1.EnvVar{
					{Name: "LD_PRELOAD", Value: preload},
					{Name: "MALLOC_CONF", Value: "background_thread:true,dirty_decay_ms:5000"},
				}
			}
			var node *appsv1.DaemonSet
			var runnerFound bool
			for _, object := range objects {
				switch o := object.(type) {
				case *appsv1.DaemonSet:
					node = o
				case *appsv1.Deployment:
					if o.Labels[apicommon.AgentDeploymentComponentLabelKey] == constants.DefaultClusterChecksRunnerResourceSuffix {
						runnerFound = true
						assertAllocatorEnvironment(t, o.Spec.Template.Spec.Containers, expected)
					} else {
						assertAllocatorEnvironment(t, o.Spec.Template.Spec.Containers, nil)
					}
					assertAllocatorEnvironment(t, o.Spec.Template.Spec.InitContainers, nil)
				}
			}
			require.NotNil(t, node, "node Agent workload must be produced")
			assert.Equal(t, tt.comprehensive, runnerFound, "jemalloc must not enable runners on its own")
			assertAllocatorEnvironment(t, node.Spec.Template.Spec.InitContainers, nil)
			for _, c := range node.Spec.Template.Spec.Containers {
				var want []corev1.EnvVar
				switch c.Name {
				case "agent", "system-probe", "trace-agent":
					want = expected
				}
				assertAllocatorEnvironment(t, []corev1.Container{c}, want)
			}
			if tt.comprehensive {
				var names []string
				for _, c := range node.Spec.Template.Spec.Containers {
					names = append(names, c.Name)
				}
				assert.Subset(t, names, []string{"agent", "trace-agent", "system-probe", "security-agent"})
			}
		})
	}
}

func TestRender_JemallocVersionGate(t *testing.T) {
	for _, tt := range []struct {
		name       string
		nodeTag    string
		runnerTag  string
		wantNode   bool
		wantRunner bool
	}{
		{name: "both too old", nodeTag: "7.83.0", runnerTag: "7.83.0"},
		{name: "both supported", nodeTag: "7.84.0", runnerTag: "7.84.0", wantNode: true, wantRunner: true},
		{name: "only runner supported", nodeTag: "7.83.0", runnerTag: "7.84.0", wantRunner: true},
		{name: "only node supported", nodeTag: "7.84.0", runnerTag: "7.83.0", wantNode: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dda, err := LoadDDA("testdata/comprehensive-dda.yaml")
			require.NoError(t, err)
			dda.Annotations = map[string]string{component.EnableJemallocAnnotation: "true"}
			dda.Spec.Override = map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
				v2alpha1.NodeAgentComponentName:           {Image: &v2alpha1.AgentImageConfig{Tag: tt.nodeTag}},
				v2alpha1.ClusterChecksRunnerComponentName: {Image: &v2alpha1.AgentImageConfig{Tag: tt.runnerTag}},
			}
			objects, _, err := Render(Options{DDA: dda})
			require.NoError(t, err)
			var nodeFound, runnerFound bool
			for _, object := range objects {
				var pod *corev1.PodSpec
				var enabled bool
				switch obj := object.(type) {
				case *appsv1.DaemonSet:
					nodeFound = true
					pod = &obj.Spec.Template.Spec
					enabled = tt.wantNode
				case *appsv1.Deployment:
					if obj.Labels[apicommon.AgentDeploymentComponentLabelKey] != constants.DefaultClusterChecksRunnerResourceSuffix {
						continue
					}
					runnerFound = true
					pod = &obj.Spec.Template.Spec
					enabled = tt.wantRunner
				default:
					continue
				}
				var expected []corev1.EnvVar
				if enabled {
					expected = []corev1.EnvVar{
						{Name: "LD_PRELOAD", Value: "/opt/lib/nosys.so /opt/datadog-agent/embedded/lib/libjemalloc.so"},
						{Name: "MALLOC_CONF", Value: "background_thread:true,dirty_decay_ms:5000"},
					}
				}
				assertAllocatorEnvironment(t, pod.Containers, expected)
				assertAllocatorEnvironment(t, pod.InitContainers, nil)
			}
			require.True(t, nodeFound)
			require.True(t, runnerFound)
		})
	}
}

func TestRender_JemallocWindowsProfile(t *testing.T) {
	dda, err := LoadDDA("testdata/minimal-dda.yaml")
	require.NoError(t, err)
	dda.Annotations = map[string]string{component.EnableJemallocAnnotation: "true"}
	profiles, err := LoadDAPs([]string{"testdata/linux-profile.yaml"})
	require.NoError(t, err)
	profiles[0].Name = "windows-profile"
	profiles[0].Annotations = map[string]string{kubernetes.ProviderAnnotationKey: kubernetes.WindowsProvider}
	profiles[0].Spec.ProfileAffinity.ProfileNodeAffinity[0].Values = []string{"windows"}
	objects, _, err := Render(Options{DDA: dda, DAPs: profiles, ProfileEnabled: true})
	require.NoError(t, err)
	var windowsFound bool
	for _, object := range objects {
		if ds, ok := object.(*appsv1.DaemonSet); ok && ds.Spec.Template.Spec.NodeSelector["kubernetes.io/os"] == "windows" {
			windowsFound = true
			assertAllocatorEnvironment(t, ds.Spec.Template.Spec.Containers, nil)
		}
	}
	require.True(t, windowsFound, "Windows profile workload must be produced")
}

func TestRender_JemallocOverrides(t *testing.T) {
	dda, err := LoadDDA("testdata/minimal-dda.yaml")
	require.NoError(t, err)
	dda.Annotations = map[string]string{component.EnableJemallocAnnotation: "true"}
	dda.Spec.Features = &v2alpha1.DatadogFeatures{}
	dda.Spec.Override = map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
		v2alpha1.NodeAgentComponentName: {
			Env: []corev1.EnvVar{
				{Name: "LD_PRELOAD", Value: "/custom/profiler.so /opt/datadog-agent/embedded/lib/libjemalloc.so"},
				{Name: "MALLOC_CONF", Value: "dirty_decay_ms:1000"},
			},
			Containers: map[apicommon.AgentContainerName]*v2alpha1.DatadogAgentGenericContainer{
				apicommon.CoreAgentContainerName: {Env: []corev1.EnvVar{
					{Name: "LD_PRELOAD", Value: ""},
					{Name: "MALLOC_CONF", ValueFrom: &corev1.EnvVarSource{
						ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{Name: "allocator"},
							Key:                  "config",
						},
					}},
				}},
			},
		},
	}
	dda.Spec.Features.ClusterChecks = &v2alpha1.ClusterChecksFeatureConfig{
		Enabled: new(true), UseClusterChecksRunners: new(true),
	}
	dda.Spec.Override[v2alpha1.ClusterChecksRunnerComponentName] = dda.Spec.Override[v2alpha1.NodeAgentComponentName].DeepCopy()
	objects, _, err := Render(Options{DDA: dda})
	require.NoError(t, err)
	var found, runnerFound bool
	for _, object := range objects {
		if deployment, ok := object.(*appsv1.Deployment); ok &&
			deployment.Labels[apicommon.AgentDeploymentComponentLabelKey] == constants.DefaultClusterChecksRunnerResourceSuffix {
			runnerFound = true
			want := dda.Spec.Override[v2alpha1.ClusterChecksRunnerComponentName].Containers[apicommon.CoreAgentContainerName].Env
			assertAllocatorEnvironment(t, deployment.Spec.Template.Spec.Containers, want)
			assertAllocatorEnvironment(t, deployment.Spec.Template.Spec.InitContainers, nil)
		}
		if ds, ok := object.(*appsv1.DaemonSet); ok {
			found = true
			for _, c := range ds.Spec.Template.Spec.Containers {
				want := dda.Spec.Override[v2alpha1.NodeAgentComponentName].Env
				if c.Name == string(apicommon.CoreAgentContainerName) {
					want = dda.Spec.Override[v2alpha1.NodeAgentComponentName].Containers[apicommon.CoreAgentContainerName].Env
				}
				assertAllocatorEnvironment(t, []corev1.Container{c}, want)
			}
		}
	}
	require.True(t, found)
	require.True(t, runnerFound)
}

func assertAllocatorEnvironment(t *testing.T, containers []corev1.Container, expected []corev1.EnvVar) {
	t.Helper()
	for _, c := range containers {
		var actual []corev1.EnvVar
		for _, env := range c.Env {
			if env.Name == "LD_PRELOAD" || env.Name == "MALLOC_CONF" {
				actual = append(actual, env)
			}
		}
		assert.Equal(t, expected, actual, c.Name)
	}
}
