// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// Copyright 2016-present Datadog, Inc.

package datadogagentinternal

import (
	"strings"
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

func TestPrepareAgentTemplateNetworkingMatrix(t *testing.T) {
	tests := []struct {
		name          string
		hostNetwork   bool
		hostPort      int32
		wantError     string
		wantPortCount int
	}{
		{
			name:          "pod network and UDS preserves declared container ports",
			wantPortCount: 1,
		},
		{
			name:      "pod network and hostPort is rejected",
			hostPort:  8126,
			wantError: "cannot overlap Pod-networked containers that declare hostPort",
		},
		{
			name:          "host network strips scheduling port claims",
			hostNetwork:   true,
			hostPort:      8126,
			wantPortCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ds := preparedTestDaemonSet(tt.hostNetwork)
			ds.Spec.Template.Spec.Containers[1].Ports[0].HostPort = tt.hostPort
			err := prepareAgentTemplate(ds)
			if tt.wantError != "" {
				require.ErrorContains(t, err, tt.wantError)
				return
			}
			require.NoError(t, err)
			for i := range ds.Spec.Template.Spec.Containers {
				assert.Len(t, ds.Spec.Template.Spec.Containers[i].Ports, tt.wantPortCount)
			}
			// UDS hostPath volumes are deliberately preserved; sleeping processes
			// do not bind or unlink the shared pathname.
			assert.True(t, hasHostPath(ds.Spec.Template.Spec.Volumes, "/var/run/datadog"))
		})
	}
}

func TestConfigurePreparedRolloutUsesExistingBudget(t *testing.T) {
	ddai := &datadoghqv1alpha1.DatadogAgentInternal{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{
		preparedRolloutModeAnnotation: preparedRolloutModeV1,
	}}}
	ds := preparedTestDaemonSet(false)
	ds.Spec.UpdateStrategy.RollingUpdate.MaxSurge = nil
	budget := intstr.FromString("10%")
	migrating, err := configurePreparedRollout(ddai, ds, nil, budget)
	require.NoError(t, err)
	assert.False(t, migrating)
	require.NotNil(t, ds.Spec.UpdateStrategy.RollingUpdate)
	assert.Equal(t, intstr.FromString("10%"), *ds.Spec.UpdateStrategy.RollingUpdate.MaxSurge)
	assert.Equal(t, intstr.FromInt(0), *ds.Spec.UpdateStrategy.RollingUpdate.MaxUnavailable)
	assert.Equal(t, preparedRolloutModeV1, ds.Spec.Template.Annotations[preparedRolloutModeAnnotation])
}

func TestPreparedRolloutMigratesExistingProfileAntiAffinityBeforeSurge(t *testing.T) {
	ddai := &datadoghqv1alpha1.DatadogAgentInternal{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{
		preparedRolloutModeAnnotation: preparedRolloutModeV1,
	}}}
	budget := intstr.FromInt(1)
	current := preparedTestDaemonSet(true)
	current.Generation = 1
	current.Spec.Template.Spec.Affinity = &corev1.Affinity{PodAntiAffinity: broadAgentPodAntiAffinity()}

	desired := preparedTestDaemonSet(true)
	migrating, err := configurePreparedRollout(ddai, desired, current, budget)
	require.NoError(t, err)
	require.True(t, migrating)
	assert.Equal(t, intstr.FromInt(0), *desired.Spec.UpdateStrategy.RollingUpdate.MaxSurge)
	assert.Equal(t, budget, *desired.Spec.UpdateStrategy.RollingUpdate.MaxUnavailable)
	assert.False(t, hasRolloutMode(desired.Spec.Template.Annotations))
	expectedAffinity, ok := profileSurgePodAntiAffinity(desired.Spec.Template.Labels)
	require.True(t, ok)
	assert.Equal(t, expectedAffinity, desired.Spec.Template.Spec.Affinity.PodAntiAffinity)
	assert.Nil(t, containerEnv(&desired.Spec.Template.Spec.Containers[0], rolloutEnabledEnv))

	current = desired.DeepCopy()
	current.Generation = 2
	current.Status = appsv1.DaemonSetStatus{ObservedGeneration: 1, DesiredNumberScheduled: 2, UpdatedNumberScheduled: 1, NumberAvailable: 1, NumberUnavailable: 1}
	desired = preparedTestDaemonSet(true)
	migrating, err = configurePreparedRollout(ddai, desired, current, budget)
	require.NoError(t, err)
	require.True(t, migrating, "surge must wait until every old broad-affinity Pod is replaced")

	current.Status = appsv1.DaemonSetStatus{ObservedGeneration: 2, DesiredNumberScheduled: 2, UpdatedNumberScheduled: 2, NumberAvailable: 2}
	desired = preparedTestDaemonSet(true)
	migrating, err = configurePreparedRollout(ddai, desired, current, budget)
	require.NoError(t, err)
	assert.False(t, migrating)
	assert.Equal(t, budget, *desired.Spec.UpdateStrategy.RollingUpdate.MaxSurge)
	assert.True(t, hasRolloutMode(desired.Spec.Template.Annotations))
}

func TestPreparedReplacementReportsPreparedAsReady(t *testing.T) {
	ds := preparedTestDaemonSet(false)
	require.NoError(t, prepareAgentTemplate(ds))
	for i := range ds.Spec.Template.Spec.Containers {
		container := &ds.Spec.Template.Spec.Containers[i]
		require.NotNil(t, container.ReadinessProbe)
		command := strings.Join(container.ReadinessProbe.Exec.Command, " ")
		assert.Contains(t, command, "prepared) exit 0")
		assert.NotContains(t, command, "prepared|activating")
		assert.Contains(t, command, `/proc/$pid/stat`)
		assert.Contains(t, command, `${20}`)
		require.NotNil(t, container.LivenessProbe)
		assert.NotContains(t, strings.Join(container.LivenessProbe.Exec.Command, " "), "prepared|activating")
		assert.Contains(t, strings.Join(container.LivenessProbe.Exec.Command, " "), "prepared) exit 0")

		uidEnv := containerEnv(container, rolloutPodUIDEnv)
		require.NotNil(t, uidEnv)
		require.NotNil(t, uidEnv.ValueFrom)
		require.NotNil(t, uidEnv.ValueFrom.FieldRef)
		assert.Equal(t, "metadata.uid", uidEnv.ValueFrom.FieldRef.FieldPath)
		stateEnv := containerEnv(container, rolloutStatePathEnv)
		require.NotNil(t, stateEnv)
		assert.True(t, strings.HasPrefix(stateEnv.Value, preparedRolloutStateDir+"/"))
	}
}

func TestPreparedRolloutStateUsesReservedPrivateEmptyDir(t *testing.T) {
	ds := preparedTestDaemonSet(false)
	require.NoError(t, prepareAgentTemplate(ds))
	volume := findVolumeByName(ds.Spec.Template.Spec.Volumes, preparedRolloutStateVolume)
	require.NotNil(t, volume)
	require.NotNil(t, volume.EmptyDir)
	for i := range ds.Spec.Template.Spec.Containers {
		assert.Contains(t, ds.Spec.Template.Spec.Containers[i].VolumeMounts, corev1.VolumeMount{Name: preparedRolloutStateVolume, MountPath: preparedRolloutStateDir})
	}

	conflicting := preparedTestDaemonSet(false)
	conflicting.Spec.Template.Spec.Volumes = append(conflicting.Spec.Template.Spec.Volumes, corev1.Volume{Name: "shared", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/var/run"}}})
	conflicting.Spec.Template.Spec.Containers[0].VolumeMounts = append(conflicting.Spec.Template.Spec.Containers[0].VolumeMounts, corev1.VolumeMount{Name: "shared", MountPath: "/var/run"})
	require.ErrorContains(t, prepareAgentTemplate(conflicting), "reserved name or path")
}

func TestPreparedRolloutRejectsUngatedComponents(t *testing.T) {
	ds := preparedTestDaemonSet(false)
	ds.Spec.Template.Spec.Containers = append(ds.Spec.Template.Spec.Containers, corev1.Container{Name: "system-probe"})
	require.ErrorContains(t, prepareAgentTemplate(ds), "supports exactly agent and trace-agent")
}

func TestProfileSurgeAntiAffinityAllowsOnlySameProfileOverlap(t *testing.T) {
	antiAffinity, ok := profileSurgePodAntiAffinity(map[string]string{
		apicommon.AgentDeploymentNameLabelKey: "agent-a",
		constants.ProfileLabelKey:             "blue",
	})
	require.True(t, ok)
	require.Len(t, antiAffinity.RequiredDuringSchedulingIgnoredDuringExecution, 2)

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
	assert.True(t, blocked(mergeLabels(base, map[string]string{apicommon.AgentDeploymentNameLabelKey: "agent-b", constants.ProfileLabelKey: "blue"})))
}

func preparedTestDaemonSet(hostNetwork bool) *appsv1.DaemonSet {
	port := corev1.ContainerPort{Name: "declared", ContainerPort: 8126, Protocol: corev1.ProtocolTCP}
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
						{Name: string(apicommon.CoreAgentContainerName), Command: []string{"agent", "run"}, Ports: []corev1.ContainerPort{port}, LivenessProbe: &corev1.Probe{}},
						{Name: string(apicommon.TraceAgentContainerName), Command: []string{"/entrypoint.sh", "trace-agent"}, Ports: []corev1.ContainerPort{port}, LivenessProbe: &corev1.Probe{}},
					},
					Volumes: []corev1.Volume{{
						Name: "sockets",
						VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{
							Path: "/var/run/datadog",
							Type: ptr.To(corev1.HostPathDirectoryOrCreate),
						}},
					}},
				},
			},
			UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
				Type: appsv1.RollingUpdateDaemonSetStrategyType,
				RollingUpdate: &appsv1.RollingUpdateDaemonSet{
					MaxUnavailable: ptr.To(intstr.FromInt(1)),
					MaxSurge:       ptr.To(intstr.FromInt(1)),
				},
			},
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

func findVolumeByName(volumes []corev1.Volume, name string) *corev1.Volume {
	for i := range volumes {
		if volumes[i].Name == name {
			return &volumes[i]
		}
	}
	return nil
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
