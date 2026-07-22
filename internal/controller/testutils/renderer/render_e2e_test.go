// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package renderer

import (
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	common "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// baseKindCounts is the expected resource inventory for a minimal DatadogAgent
// with no extra features enabled. The three Services are the Cluster Agent,
// the admission controller, and the node Agent local service (rendered because
// the simulated Kubernetes version is >= 1.22).
var baseKindCounts = map[string]int{
	"ServiceAccount":       2,
	"ClusterRole":          5,
	"ClusterRoleBinding":   5,
	"Role":                 1,
	"RoleBinding":          1,
	"Secret":               1,
	"ConfigMap":            5,
	"Service":              3,
	"DatadogAgentInternal": 1,
	"DaemonSet":            1,
	"Deployment":           1,
}

// baseKindSeq is the expected serialized kind order for the same input,
// reflecting the dependency-aware sort in kindOrder.
var baseKindSeq = []string{
	"ServiceAccount", "ServiceAccount",
	"ClusterRole", "ClusterRole", "ClusterRole", "ClusterRole", "ClusterRole",
	"ClusterRoleBinding", "ClusterRoleBinding", "ClusterRoleBinding", "ClusterRoleBinding", "ClusterRoleBinding",
	"Role",
	"RoleBinding",
	"Secret",
	"ConfigMap", "ConfigMap", "ConfigMap", "ConfigMap", "ConfigMap",
	"Service", "Service", "Service",
	"DatadogAgentInternal",
	"DaemonSet",
	"Deployment",
}

// TestRender_MinimalDDA is the primary e2e test. It loads a real DatadogAgent
// manifest, runs both reconciliation passes, and asserts:
//   - exact kind inventory
//   - full serialized kind order (dependency-aware + alphabetical within kind)
//   - dynamic metadata fields are stripped from serialized output
func TestRender_MinimalDDA(t *testing.T) {
	dda, err := LoadDDA("testdata/minimal-dda.yaml")
	require.NoError(t, err)

	objects, scheme, err := Render(Options{DDA: dda})
	require.NoError(t, err)

	assert.Equal(t, baseKindCounts, countKinds(objects, scheme))

	out, err := Serialize(objects, scheme, "yaml", false)
	require.NoError(t, err)
	s := string(out)

	assert.Equal(t, baseKindSeq, kindSequence(s))

	for _, banned := range []string{"resourceVersion:", "generation:", "creationTimestamp:", "managedFields:"} {
		assert.NotContains(t, s, banned, "field %q must be stripped", banned)
	}
}

// TestRender_WithDAP exercises the ProfileEnabled code path with two valid profiles.
// Each profile produces its own DDAI and DaemonSet, so the output has 3 DDAIs
// (default + linux-profile + gpu-profile) and 3 DaemonSets.
func TestRender_WithDAP(t *testing.T) {
	dda, err := LoadDDA("testdata/minimal-dda.yaml")
	require.NoError(t, err)

	daps, err := LoadDAPs([]string{"testdata/linux-profile.yaml", "testdata/gpu-profile.yaml"})
	require.NoError(t, err)

	objects, scheme, err := Render(Options{
		DDA:            dda,
		DAPs:           daps,
		ProfileEnabled: true,
	})
	require.NoError(t, err)

	assert.Equal(t, map[string]int{
		"ServiceAccount":       2,
		"ClusterRole":          5,
		"ClusterRoleBinding":   5,
		"Role":                 1,
		"RoleBinding":          1,
		"Secret":               1,
		"ConfigMap":            5,
		"Service":              3,
		"DatadogAgentInternal": 3,
		"DaemonSet":            3,
		"Deployment":           1,
	}, countKinds(objects, scheme))

	out, err := Serialize(objects, scheme, "yaml", false)
	require.NoError(t, err)
	assert.Equal(t, []string{
		"ServiceAccount", "ServiceAccount",
		"ClusterRole", "ClusterRole", "ClusterRole", "ClusterRole", "ClusterRole",
		"ClusterRoleBinding", "ClusterRoleBinding", "ClusterRoleBinding", "ClusterRoleBinding", "ClusterRoleBinding",
		"Role",
		"RoleBinding",
		"Secret",
		"ConfigMap", "ConfigMap", "ConfigMap", "ConfigMap", "ConfigMap",
		"Service", "Service", "Service",
		"DatadogAgentInternal", "DatadogAgentInternal", "DatadogAgentInternal",
		"DaemonSet", "DaemonSet", "DaemonSet",
		"Deployment",
	}, kindSequence(string(out)))
}

func TestRender_AppArmorProfileVersionGate(t *testing.T) {
	tests := []struct {
		name              string
		kubernetesVersion string
		wantAnnotation    bool
		wantProfileField  bool
	}{
		{
			name:              "Kubernetes 1.29 uses the annotation",
			kubernetesVersion: "v1.29.9",
			wantAnnotation:    true,
		},
		{
			name:              "Kubernetes 1.30 uses the field",
			kubernetesVersion: "v1.30.0",
			wantProfileField:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dda, err := LoadDDA("testdata/comprehensive-dda.yaml")
			require.NoError(t, err)

			objects, scheme, err := Render(Options{DDA: dda, KubernetesVersion: tt.kubernetesVersion})
			require.NoError(t, err)

			out, err := Serialize(objects, scheme, "yaml", false)
			require.NoError(t, err)
			rendered := string(out)

			assert.Equal(t, tt.wantAnnotation, strings.Contains(rendered, "container.apparmor.security.beta.kubernetes.io/system-probe: unconfined"))
			assert.Equal(t, tt.wantProfileField, strings.Contains(rendered, "appArmorProfile:\n            type: Unconfined"))
		})
	}
}

func TestRender_PreparedRolloutArmsBeforeSurge(t *testing.T) {
	renderAgentDaemonSet := func(t *testing.T, prepared bool) *appsv1.DaemonSet {
		t.Helper()
		dda, err := LoadDDA("testdata/minimal-dda.yaml")
		require.NoError(t, err)
		dda.Spec.Features = preparedRolloutTestFeatures()

		override := &datadoghqv2alpha1.DatadogAgentComponentOverride{HostNetwork: ptr.To(true)}
		override.UpdateStrategy = &common.UpdateStrategy{
			Type: string(appsv1.RollingUpdateDaemonSetStrategyType),
			RollingUpdate: &common.RollingUpdate{
				MaxUnavailable: ptr.To(intstr.FromInt(1)),
				MaxSurge:       ptr.To(intstr.FromInt(1)),
			},
		}
		if prepared {
			dda.Annotations = map[string]string{"experimental.agent.datadoghq.com/host-network-surge-prepared": "true"}
		}
		dda.Spec.Override = map[datadoghqv2alpha1.ComponentName]*datadoghqv2alpha1.DatadogAgentComponentOverride{
			datadoghqv2alpha1.NodeAgentComponentName: override,
		}

		objects, _, err := Render(Options{DDA: dda})
		require.NoError(t, err)
		for _, object := range objects {
			if ds, ok := object.(*appsv1.DaemonSet); ok {
				return ds
			}
		}
		t.Fatal("render produced no Agent DaemonSet")
		return nil
	}

	baseline := renderAgentDaemonSet(t, false)
	require.True(t, baseline.Spec.Template.Spec.HostNetwork)
	require.NotNil(t, baseline.Spec.UpdateStrategy.RollingUpdate)
	assert.Equal(t, intstr.FromInt(1), *baseline.Spec.UpdateStrategy.RollingUpdate.MaxSurge)
	assert.Equal(t, intstr.FromInt(1), *baseline.Spec.UpdateStrategy.RollingUpdate.MaxUnavailable,
		"ordinary native surge must remain unchanged when prepared rollout is disabled")
	baselinePortCount := 0
	for _, container := range baseline.Spec.Template.Spec.Containers {
		baselinePortCount += len(container.Ports)
	}
	require.Positive(t, baselinePortCount, "the host-network baseline must exercise Kubernetes's implicit hostPort defaulting")

	armed := renderAgentDaemonSet(t, true)
	require.True(t, armed.Spec.Template.Spec.HostNetwork)
	require.NotNil(t, armed.Spec.UpdateStrategy.RollingUpdate)
	assert.Equal(t, intstr.FromInt(0), *armed.Spec.UpdateStrategy.RollingUpdate.MaxSurge)
	assert.Equal(t, intstr.FromInt(1), *armed.Spec.UpdateStrategy.RollingUpdate.MaxUnavailable)
	assert.Equal(t, "arm", armed.Spec.Template.Annotations["experimental.agent.datadoghq.com/prepared-rollout-phase"])
	armedPortCount := 0
	for _, container := range armed.Spec.Template.Spec.Containers {
		armedPortCount += len(container.Ports)
		require.NotNil(t, container.StartupProbe)
		require.NotNil(t, container.StartupProbe.Exec)
		require.NotNil(t, container.ReadinessProbe)
		require.NotNil(t, container.ReadinessProbe.Exec)
		if container.Name == "trace-agent" {
			assert.Equal(t, "trace-agent", container.Command[0], "prepared mode must bypass trace-loader")
		}
	}
	assert.Equal(t, baselinePortCount, armedPortCount, "arming is a conventional rollout and keeps port declarations")
}

func TestRender_PreparedHostNetworkSurgeWithProfiles(t *testing.T) {
	dda, err := LoadDDA("testdata/minimal-dda.yaml")
	require.NoError(t, err)
	dda.Spec.Features = preparedRolloutTestFeatures()
	dda.Annotations = map[string]string{"experimental.agent.datadoghq.com/host-network-surge-prepared": "true"}

	surgeOverride := func() *datadoghqv2alpha1.DatadogAgentComponentOverride {
		return &datadoghqv2alpha1.DatadogAgentComponentOverride{
			HostNetwork: ptr.To(true),
			UpdateStrategy: &common.UpdateStrategy{
				Type: string(appsv1.RollingUpdateDaemonSetStrategyType),
				RollingUpdate: &common.RollingUpdate{
					MaxUnavailable: ptr.To(intstr.FromInt(1)),
					MaxSurge:       ptr.To(intstr.FromInt(1)),
				},
			},
		}
	}
	dda.Spec.Override = map[datadoghqv2alpha1.ComponentName]*datadoghqv2alpha1.DatadogAgentComponentOverride{
		datadoghqv2alpha1.NodeAgentComponentName: surgeOverride(),
	}

	daps, err := LoadDAPs([]string{"testdata/linux-profile.yaml", "testdata/gpu-profile.yaml"})
	require.NoError(t, err)

	objects, _, err := Render(Options{DDA: dda, DAPs: daps, ProfileEnabled: true})
	require.NoError(t, err)
	daemonSets := 0
	for _, object := range objects {
		ds, ok := object.(*appsv1.DaemonSet)
		if !ok {
			continue
		}
		daemonSets++
		require.True(t, ds.Spec.Template.Spec.HostNetwork)
		assert.Equal(t, "arm", ds.Spec.Template.Annotations["experimental.agent.datadoghq.com/prepared-rollout-phase"])
		require.NotNil(t, ds.Spec.Template.Spec.Affinity)
		require.NotNil(t, ds.Spec.Template.Spec.Affinity.PodAntiAffinity)
		assert.Len(t, ds.Spec.Template.Spec.Affinity.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution, 2,
			"DaemonSet %s must scope overlap to the same DDA and profile", ds.Name)
	}
	assert.Equal(t, 3, daemonSets)
}

func preparedRolloutTestFeatures() *datadoghqv2alpha1.DatadogFeatures {
	return &datadoghqv2alpha1.DatadogFeatures{
		APM:                     &datadoghqv2alpha1.APMFeatureConfig{Enabled: ptr.To(true)},
		LiveProcessCollection:   &datadoghqv2alpha1.LiveProcessCollectionFeatureConfig{Enabled: ptr.To(false)},
		LiveContainerCollection: &datadoghqv2alpha1.LiveContainerCollectionFeatureConfig{Enabled: ptr.To(false)},
		ProcessDiscovery:        &datadoghqv2alpha1.ProcessDiscoveryFeatureConfig{Enabled: ptr.To(false)},
		ServiceDiscovery:        &datadoghqv2alpha1.ServiceDiscoveryFeatureConfig{Enabled: ptr.To(false)},
	}
}

// kindSequence extracts the ordered list of "kind: X" values from serialized YAML.
func kindSequence(yaml string) []string {
	var kinds []string
	for line := range strings.SplitSeq(yaml, "\n") {
		if kind, ok := strings.CutPrefix(line, "kind: "); ok {
			kinds = append(kinds, kind)
		}
	}
	return kinds
}

// countKinds tallies each GVK Kind in the object slice.
func countKinds(objects []client.Object, scheme *runtime.Scheme) map[string]int {
	counts := map[string]int{}
	for _, obj := range objects {
		counts[resolveKind(obj, scheme)]++
	}
	return counts
}
