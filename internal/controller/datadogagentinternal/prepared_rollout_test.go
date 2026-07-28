// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// Copyright 2016-present Datadog, Inc.

package datadogagentinternal

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPreparedRolloutRequiresExplicitMode(t *testing.T) {
	ddai := &datadoghqv1alpha1.DatadogAgentInternal{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{}}}
	assert.False(t, preparedRolloutEnabled(ddai))
	ddai.Annotations[preparedRolloutModeAnnotation] = "true"
	assert.False(t, preparedRolloutEnabled(ddai))
	ddai.Annotations[preparedRolloutModeAnnotation] = preparedRolloutModeV1
	assert.True(t, preparedRolloutEnabled(ddai))
}

func TestPrepareAgentTemplatePreservesHostNetworkAndUDS(t *testing.T) {
	ds := preparedTestDaemonSet(true)
	require.NoError(t, prepareAgentTemplate(ds))
	assert.True(t, ds.Spec.Template.Spec.HostNetwork)
	for i := range ds.Spec.Template.Spec.Containers {
		assert.Empty(t, ds.Spec.Template.Spec.Containers[i].Ports)
	}
	assert.True(t, hasHostPath(ds.Spec.Template.Spec.Volumes, "/var/run/datadog"))

	podNetwork := preparedTestDaemonSet(false)
	for i := range podNetwork.Spec.Template.Spec.Containers {
		for j := range podNetwork.Spec.Template.Spec.Containers[i].Ports {
			podNetwork.Spec.Template.Spec.Containers[i].Ports[j].HostPort = 0
		}
	}
	require.NoError(t, prepareAgentTemplate(podNetwork))
	assert.False(t, podNetwork.Spec.Template.Spec.HostNetwork)
	for i := range podNetwork.Spec.Template.Spec.Containers {
		assert.NotEmpty(t, podNetwork.Spec.Template.Spec.Containers[i].Ports)
	}

	podNetworkWithHostPort := preparedTestDaemonSet(false)
	require.ErrorContains(t, prepareAgentTemplate(podNetworkWithHostPort), "declare hostPort")
}

func TestConfigurePreparedRolloutUsesExistingUnavailableBudgetAsSurge(t *testing.T) {
	ddai := preparedRolloutDDAI()
	ds := preparedTestDaemonSet(true)
	budget := intstr.FromString("10%")
	migrating, err := configurePreparedRollout(ddai, ds, nil, budget)
	require.NoError(t, err)
	assert.False(t, migrating)
	assert.Equal(t, budget, *ds.Spec.UpdateStrategy.RollingUpdate.MaxSurge)
	assert.Equal(t, intstr.FromInt(0), *ds.Spec.UpdateStrategy.RollingUpdate.MaxUnavailable)
	assert.Equal(t, preparedRolloutModeV1, ds.Spec.Template.Annotations[preparedRolloutModeAnnotation])
}

func TestPreparedRolloutMigratesExistingProfileAntiAffinityBeforeSurge(t *testing.T) {
	budget := intstr.FromInt(1)
	current := preparedTestDaemonSet(true)
	current.Generation = 1
	current.Spec.Template.Spec.Affinity = &corev1.Affinity{PodAntiAffinity: broadAgentPodAntiAffinity()}

	desired := preparedTestDaemonSet(true)
	migrating, err := configurePreparedRollout(preparedRolloutDDAI(), desired, current, budget)
	require.NoError(t, err)
	require.True(t, migrating)
	assert.Equal(t, intstr.FromInt(0), *desired.Spec.UpdateStrategy.RollingUpdate.MaxSurge)
	assert.Equal(t, budget, *desired.Spec.UpdateStrategy.RollingUpdate.MaxUnavailable)
	assert.Nil(t, containerEnv(&desired.Spec.Template.Spec.Containers[0], rolloutEnabledEnv))

	current = desired.DeepCopy()
	current.Generation = 2
	current.Status = appsv1.DaemonSetStatus{ObservedGeneration: 2, DesiredNumberScheduled: 2, UpdatedNumberScheduled: 2, NumberAvailable: 2}
	desired = preparedTestDaemonSet(true)
	migrating, err = configurePreparedRollout(preparedRolloutDDAI(), desired, current, budget)
	require.NoError(t, err)
	assert.False(t, migrating)
	assert.Equal(t, budget, *desired.Spec.UpdateStrategy.RollingUpdate.MaxSurge)
}

func TestPreparedRolloutPreservesProbes(t *testing.T) {
	ds := preparedTestDaemonSet(true)
	original := make(map[string]struct {
		startup   *corev1.Probe
		liveness  *corev1.Probe
		readiness *corev1.Probe
	}, len(ds.Spec.Template.Spec.Containers))
	for i := range ds.Spec.Template.Spec.Containers {
		container := &ds.Spec.Template.Spec.Containers[i]
		original[container.Name] = struct {
			startup   *corev1.Probe
			liveness  *corev1.Probe
			readiness *corev1.Probe
		}{container.StartupProbe.DeepCopy(), container.LivenessProbe.DeepCopy(), container.ReadinessProbe.DeepCopy()}
	}
	require.NoError(t, prepareAgentTemplate(ds))
	for i := range ds.Spec.Template.Spec.Containers {
		container := &ds.Spec.Template.Spec.Containers[i]
		assert.Equal(t, original[container.Name].startup, container.StartupProbe)
		assert.Equal(t, original[container.Name].liveness, container.LivenessProbe)
		assert.Equal(t, original[container.Name].readiness, container.ReadinessProbe)
		assert.Equal(t, "metadata.uid", containerEnv(container, rolloutPodUIDEnv).ValueFrom.FieldRef.FieldPath)
		assert.Equal(t, "status.hostIP", containerEnv(container, kubeletHostEnv).ValueFrom.FieldRef.FieldPath)
	}
}

func TestPreparedRolloutSupportsAllRenderedAgentContainers(t *testing.T) {
	ds := preparedTestDaemonSet(true)
	ds.Spec.Template.Spec.Containers = []corev1.Container{
		{Name: string(apicommon.CoreAgentContainerName), Command: []string{"agent", "run"}},
		{Name: string(apicommon.TraceAgentContainerName), Command: []string{"/entrypoint.sh", "trace-agent"}},
		{Name: string(apicommon.ProcessAgentContainerName), Command: []string{"process-agent"}},
		{Name: string(apicommon.SystemProbeContainerName), Command: []string{"system-probe"}},
		{Name: string(apicommon.HostProfiler), Command: []string{"host-profiler", "--core-config=/etc/datadog-agent/datadog.yaml"}},
		{Name: string(apicommon.OtelAgent), Command: []string{"otel-agent"}},
		{Name: string(apicommon.PrivateActionRunnerContainerName), Command: []string{"/opt/datadog-agent/embedded/bin/privateactionrunner"}},
	}
	ds.Spec.Template.Spec.InitContainers = append(ds.Spec.Template.Spec.InitContainers,
		corev1.Container{Name: "seccomp-setup"}, corev1.Container{Name: "host-profiler-seccomp-setup"})
	require.NoError(t, prepareAgentTemplate(ds))
	assert.Equal(t, []string{"trace-agent"}, ds.Spec.Template.Spec.Containers[1].Command)
}

func TestPreparedRolloutSupportsStandaloneHostProfiler(t *testing.T) {
	ds := preparedTestDaemonSet(true)
	ds.Spec.Template.Spec.Containers = append(ds.Spec.Template.Spec.Containers,
		corev1.Container{Name: string(apicommon.HostProfiler), Command: []string{"host-profiler"}})
	require.NoError(t, prepareAgentTemplate(ds))
}

func TestPreparedRolloutRejectsUnknownSidecar(t *testing.T) {
	ds := preparedTestDaemonSet(true)
	ds.Spec.Template.Spec.Containers = append(ds.Spec.Template.Spec.Containers, corev1.Container{Name: "unknown-sidecar", Command: []string{"sidecar"}})
	require.ErrorContains(t, prepareAgentTemplate(ds), "does not support container")
}

func TestProfileSurgeAntiAffinityAllowsOnlySameProfileOverlap(t *testing.T) {
	antiAffinity, ok := profileSurgePodAntiAffinity(map[string]string{
		apicommon.AgentDeploymentNameLabelKey: "agent-a",
		constants.ProfileLabelKey:             "blue",
	})
	require.True(t, ok)
	blocked := func(podLabels map[string]string) bool {
		for _, term := range antiAffinity.RequiredDuringSchedulingIgnoredDuringExecution {
			selector, err := metav1.LabelSelectorAsSelector(term.LabelSelector)
			require.NoError(t, err)
			if selector.Matches(labels.Set(podLabels)) {
				return true
			}
		}
		return false
	}
	base := map[string]string{apicommon.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix}
	assert.False(t, blocked(mergeLabels(base, map[string]string{apicommon.AgentDeploymentNameLabelKey: "agent-a", constants.ProfileLabelKey: "blue"})))
	assert.True(t, blocked(mergeLabels(base, map[string]string{apicommon.AgentDeploymentNameLabelKey: "agent-a", constants.ProfileLabelKey: "green"})))
}

func preparedRolloutDDAI() *datadoghqv1alpha1.DatadogAgentInternal {
	return &datadoghqv1alpha1.DatadogAgentInternal{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{
		preparedRolloutModeAnnotation: preparedRolloutModeV1,
	}}}
}

func preparedTestDaemonSet(hostNetwork bool) *appsv1.DaemonSet {
	port := corev1.ContainerPort{Name: "declared", ContainerPort: 8126, HostPort: 8126, Protocol: corev1.ProtocolTCP}
	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: "default"},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "agent"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
					"app":                                 "agent",
					apicommon.AgentDeploymentNameLabelKey: "agent",
					apicommon.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix,
				}},
				Spec: corev1.PodSpec{
					HostNetwork:  hostNetwork,
					NodeSelector: map[string]string{corev1.LabelOSStable: "linux"},
					InitContainers: []corev1.Container{
						{Name: string(apicommon.InitVolumeContainerName)},
						{Name: string(apicommon.InitConfigContainerName)},
					},
					Containers: []corev1.Container{
						{Name: string(apicommon.CoreAgentContainerName), Command: []string{"agent", "run"}, Ports: []corev1.ContainerPort{port}, StartupProbe: &corev1.Probe{}, LivenessProbe: &corev1.Probe{}, ReadinessProbe: &corev1.Probe{}},
						{Name: string(apicommon.TraceAgentContainerName), Command: []string{"/entrypoint.sh", "trace-agent"}, Ports: []corev1.ContainerPort{port}, StartupProbe: &corev1.Probe{}, LivenessProbe: &corev1.Probe{}, ReadinessProbe: &corev1.Probe{}},
					},
					Volumes: []corev1.Volume{{Name: "sockets", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/var/run/datadog", Type: ptr.To(corev1.HostPathDirectoryOrCreate)}}}},
				},
			},
			UpdateStrategy: appsv1.DaemonSetUpdateStrategy{Type: appsv1.RollingUpdateDaemonSetStrategyType, RollingUpdate: &appsv1.RollingUpdateDaemonSet{MaxUnavailable: ptr.To(intstr.FromInt(1)), MaxSurge: ptr.To(intstr.FromInt(1))}},
		},
	}
}

func hasHostPath(volumes []corev1.Volume, path string) bool {
	for i := range volumes {
		if volumes[i].HostPath != nil && volumes[i].HostPath.Path == path {
			return true
		}
	}
	return false
}

func containerEnv(container *corev1.Container, name string) *corev1.EnvVar {
	for i := range container.Env {
		if container.Env[i].Name == name {
			return &container.Env[i]
		}
	}
	return nil
}

func mergeLabels(left, right map[string]string) map[string]string {
	result := make(map[string]string, len(left)+len(right))
	for key, value := range left {
		result[key] = value
	}
	for key, value := range right {
		result[key] = value
	}
	return result
}
