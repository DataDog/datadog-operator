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
	ddai.Annotations[preparedRolloutModeAnnotation] = "prepared-surge-v1"
	assert.False(t, preparedRolloutEnabled(ddai))
	ddai.Annotations[preparedRolloutModeAnnotation] = preparedRolloutModeV3
	assert.True(t, preparedRolloutEnabled(ddai))
}

func TestPreparedRolloutDisabledLeavesDaemonSetUntouched(t *testing.T) {
	ddai := &datadoghqv1alpha1.DatadogAgentInternal{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{}}}
	ds := preparedTestDaemonSet(true)
	ds.Spec.Template.Spec.Containers[0].ReadinessProbe.HTTPGet.Path = "/custom-ready"
	original := ds.DeepCopy()

	migrating, err := configurePreparedRollout(ddai, ds, nil, intstr.FromInt(1))
	require.NoError(t, err)
	assert.False(t, migrating)
	assert.Equal(t, original, ds)
}

func TestPrepareAgentTemplatePreservesHostNetworkAndUDS(t *testing.T) {
	ds := preparedTestDaemonSet(true)
	require.NoError(t, prepareAgentTemplate(ds))
	assert.True(t, ds.Spec.Template.Spec.HostNetwork)
	for i := range ds.Spec.Template.Spec.Containers {
		container := &ds.Spec.Template.Spec.Containers[i]
		assert.Empty(t, container.Ports)
		assert.Equal(t, preparedRolloutLockDir+"/"+container.Name+".lock", containerEnv(container, rolloutLockPathEnv).Value)
		assert.Equal(t, preparedRolloutLockDir+"/"+container.Name+".prepared", containerEnv(container, rolloutPreparedPathEnv).Value)
		assert.True(t, hasVolumeMount(*container, preparedRolloutLockVolume, preparedRolloutLockDir))
	}
	assert.True(t, hasHostPath(ds.Spec.Template.Spec.Volumes, "/var/run/datadog"))
	assert.True(t, hasHostPath(ds.Spec.Template.Spec.Volumes, preparedRolloutLockDir))

	podNetwork := preparedTestDaemonSet(false)
	require.ErrorContains(t, prepareAgentTemplate(podNetwork), "requires hostNetwork")
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
	assert.Equal(t, preparedRolloutModeV3, ds.Spec.Template.Annotations[preparedRolloutModeAnnotation])
}

func TestPreparedRolloutArmsExistingOrdinaryDaemonSetBeforeSurge(t *testing.T) {
	budget := intstr.FromInt(1)
	current := preparedTestDaemonSet(true)
	current.Generation = 1
	current.Status = appsv1.DaemonSetStatus{ObservedGeneration: 1, DesiredNumberScheduled: 2, UpdatedNumberScheduled: 2, NumberAvailable: 2}
	desired := preparedTestDaemonSet(true)

	migrating, err := configurePreparedRollout(preparedRolloutDDAI(), desired, current, budget)
	require.NoError(t, err)
	assert.True(t, migrating)
	assert.Equal(t, intstr.FromInt(0), *desired.Spec.UpdateStrategy.RollingUpdate.MaxSurge)
	assert.Equal(t, budget, *desired.Spec.UpdateStrategy.RollingUpdate.MaxUnavailable)
	assert.Equal(t, preparedRolloutArmedV3, desired.Spec.Template.Annotations[preparedRolloutArmedAnnotation])
	assert.Empty(t, desired.Spec.Template.Annotations[preparedRolloutModeAnnotation])
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
	assert.Equal(t, []string{preparedRolloutGateBinary}, desired.Spec.Template.Spec.Containers[0].Command)
	assert.Equal(t, preparedRolloutArmedV3, desired.Spec.Template.Annotations[preparedRolloutArmedAnnotation])
	assert.Empty(t, desired.Spec.Template.Annotations[preparedRolloutModeAnnotation])

	current = desired.DeepCopy()
	current.Generation = 2
	current.Status = appsv1.DaemonSetStatus{ObservedGeneration: 2, DesiredNumberScheduled: 2, UpdatedNumberScheduled: 2, NumberAvailable: 2}
	desired = preparedTestDaemonSet(true)
	migrating, err = configurePreparedRollout(preparedRolloutDDAI(), desired, current, budget)
	require.NoError(t, err)
	assert.False(t, migrating)
	assert.Equal(t, budget, *desired.Spec.UpdateStrategy.RollingUpdate.MaxSurge)
}

func TestPreparedRolloutZeroNodeProfileFinishesArming(t *testing.T) {
	current := preparedTestDaemonSet(true)
	current.Generation = 2
	current.Spec.Template.Spec.Affinity = &corev1.Affinity{PodAntiAffinity: broadAgentPodAntiAffinity()}
	current.Spec.Template.Annotations = map[string]string{}
	current.Spec.Template.Annotations[preparedRolloutArmedAnnotation] = preparedRolloutArmedV3
	current.Status = appsv1.DaemonSetStatus{ObservedGeneration: 2}
	desired := preparedTestDaemonSet(true)
	desired.Spec.Template.Spec.Affinity = &corev1.Affinity{PodAntiAffinity: broadAgentPodAntiAffinity()}

	migrating, err := configurePreparedRollout(preparedRolloutDDAI(), desired, current, intstr.FromInt(1))
	require.NoError(t, err)
	assert.False(t, migrating)
	assert.Equal(t, preparedRolloutModeV3, desired.Spec.Template.Annotations[preparedRolloutModeAnnotation])
}

func TestPreparedRolloutUsesGenerationSafePreparedStartupProbes(t *testing.T) {
	ds := preparedTestDaemonSet(true)
	original := make(map[string]struct {
		liveness  *corev1.Probe
		readiness *corev1.Probe
	}, len(ds.Spec.Template.Spec.Containers))
	for i := range ds.Spec.Template.Spec.Containers {
		container := &ds.Spec.Template.Spec.Containers[i]
		original[container.Name] = struct {
			liveness  *corev1.Probe
			readiness *corev1.Probe
		}{container.LivenessProbe.DeepCopy(), container.ReadinessProbe.DeepCopy()}
	}
	require.NoError(t, prepareAgentTemplate(ds))
	for i := range ds.Spec.Template.Spec.Containers {
		container := &ds.Spec.Template.Spec.Containers[i]
		require.NotNil(t, container.StartupProbe)
		require.NotNil(t, container.StartupProbe.Exec)
		assert.Equal(t, []string{"sh", "-c", `read uid pid fd < "$DD_EXPERIMENTAL_NODE_AGENT_ROLLOUT_PREPARED_PATH" && test "$uid" = "$DD_EXPERIMENTAL_NODE_AGENT_ROLLOUT_POD_UID" && test "/proc/$pid/fd/$fd" -ef "$DD_EXPERIMENTAL_NODE_AGENT_ROLLOUT_PREPARED_PATH"`}, container.StartupProbe.Exec.Command)
		assert.Equal(t, maxStartupProbeFailures, container.StartupProbe.FailureThreshold)
		assert.Equal(t, original[container.Name].liveness, container.LivenessProbe)
		assert.Equal(t, original[container.Name].readiness, container.ReadinessProbe)
		assert.Equal(t, "metadata.uid", containerEnv(container, rolloutPodUIDEnv).ValueFrom.FieldRef.FieldPath)
		assert.Equal(t, preparedRolloutLockDir+"/"+container.Name+".lock", containerEnv(container, rolloutLockPathEnv).Value)
		assert.Equal(t, preparedRolloutLockDir+"/"+container.Name+".prepared", containerEnv(container, rolloutPreparedPathEnv).Value)
	}
	assert.Equal(t, int32(constants.DefaultAgentHealthPort), ds.Spec.Template.Spec.Containers[0].LivenessProbe.HTTPGet.Port.IntVal)
	assert.Equal(t, int32(constants.DefaultAgentHealthPort), ds.Spec.Template.Spec.Containers[0].ReadinessProbe.HTTPGet.Port.IntVal)
	assert.Equal(t, int32(constants.DefaultApmPort), ds.Spec.Template.Spec.Containers[1].LivenessProbe.TCPSocket.Port.IntVal)
}

func TestPreparedRolloutAddsLinuxSchedulingConstraint(t *testing.T) {
	ds := preparedTestDaemonSet(true)
	delete(ds.Spec.Template.Spec.NodeSelector, corev1.LabelOSStable)
	require.NoError(t, prepareAgentTemplate(ds))
	assert.Equal(t, "linux", ds.Spec.Template.Spec.NodeSelector[corev1.LabelOSStable])

	ds = preparedTestDaemonSet(true)
	ds.Spec.Template.Spec.NodeSelector[corev1.LabelOSStable] = "windows"
	require.ErrorContains(t, prepareAgentTemplate(ds), "Linux-only")
}

func TestPreparedRolloutRejectsNonRootContainers(t *testing.T) {
	ds := preparedTestDaemonSet(true)
	ds.Spec.Template.Spec.Containers[0].SecurityContext = &corev1.SecurityContext{RunAsUser: ptr.To[int64](1000)}
	require.ErrorContains(t, prepareAgentTemplate(ds), "run as root")

	ds = preparedTestDaemonSet(true)
	ds.Spec.Template.Spec.SecurityContext = &corev1.PodSecurityContext{RunAsNonRoot: ptr.To(true)}
	require.ErrorContains(t, prepareAgentTemplate(ds), "run as root")
}

func TestPreparedRolloutRejectsNamedPortsNeededByPreservedProbes(t *testing.T) {
	ds := preparedTestDaemonSet(true)
	ds.Spec.Template.Spec.Containers[0].ReadinessProbe = &corev1.Probe{ProbeHandler: corev1.ProbeHandler{
		HTTPGet: &corev1.HTTPGetAction{Path: constants.DefaultReadinessProbeHTTPPath, Port: intstr.FromString("health")},
	}}
	require.ErrorContains(t, prepareAgentTemplate(ds), "port must be a positive number")
}

func TestPreparedRolloutRejectsCustomProbes(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*corev1.Container)
		error  string
	}{
		{
			name: "named port",
			mutate: func(container *corev1.Container) {
				container.ReadinessProbe.HTTPGet.Port = intstr.FromString("health")
			},
			error: "port must be a positive number",
		},
		{
			name: "HTTP host",
			mutate: func(container *corev1.Container) {
				container.LivenessProbe.HTTPGet.Host = "127.0.0.1"
			},
			error: "liveness HTTP host must be empty",
		},
		{
			name: "exec readiness",
			mutate: func(container *corev1.Container) {
				container.ReadinessProbe.ProbeHandler = corev1.ProbeHandler{Exec: &corev1.ExecAction{Command: []string{"true"}}}
			},
			error: "readiness probe must use the Agent HTTP listener",
		},
		{
			name: "unrelated HTTP path",
			mutate: func(container *corev1.Container) {
				container.ReadinessProbe.HTTPGet.Path = "/healthy-node-service"
			},
			error: `readiness HTTP path must be "/ready"`,
		},
		{
			name: "different health port",
			mutate: func(container *corev1.Container) {
				container.ReadinessProbe.HTTPGet.Port = intstr.FromInt(5556)
			},
			error: "readiness port 5556 differs from startup port 5555",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ds := preparedTestDaemonSet(true)
			tt.mutate(&ds.Spec.Template.Spec.Containers[0])
			require.ErrorContains(t, prepareAgentTemplate(ds), tt.error)
		})
	}
}

func TestPreparedRolloutAcceptsCompatibleCustomNetworkProbes(t *testing.T) {
	ds := preparedTestDaemonSet(true)
	core := &ds.Spec.Template.Spec.Containers[0]
	core.StartupProbe = &corev1.Probe{
		ProbeHandler:  corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/ready", Port: intstr.FromInt(5555)}},
		PeriodSeconds: 1, FailureThreshold: 200,
	}
	core.LivenessProbe.TimeoutSeconds++
	ds.Spec.Template.Spec.Containers = append(ds.Spec.Template.Spec.Containers, corev1.Container{
		Name: string(apicommon.SystemProbeContainerName), Command: []string{"system-probe"},
		LivenessProbe: &corev1.Probe{ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/live", Port: intstr.FromInt(5558)}}},
	})

	require.NoError(t, prepareAgentTemplate(ds))
	core = &ds.Spec.Template.Spec.Containers[0]
	assert.Equal(t, int32(1), core.StartupProbe.PeriodSeconds)
	assert.Equal(t, maxStartupProbeFailures, core.StartupProbe.FailureThreshold)
	assert.NotNil(t, core.StartupProbe.Exec)
	assert.Equal(t, constants.DefaultLivenessProbeHTTPPath, core.LivenessProbe.HTTPGet.Path)
	assert.Equal(t, int32(5558), ds.Spec.Template.Spec.Containers[2].LivenessProbe.HTTPGet.Port.IntVal)
}

func TestPreparedRolloutRejectsUnsafeTraceProbes(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*corev1.Container)
		error  string
	}{
		{
			name: "remote host",
			mutate: func(container *corev1.Container) {
				container.LivenessProbe.TCPSocket.Host = "192.0.2.10"
			},
			error: "liveness TCP host must be empty",
		},
		{
			name: "different readiness listener",
			mutate: func(container *corev1.Container) {
				container.ReadinessProbe = container.LivenessProbe.DeepCopy()
				container.ReadinessProbe.TCPSocket.Port = intstr.FromInt(9000)
			},
			error: "readiness port 9000 differs from liveness port 8126",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ds := preparedTestDaemonSet(true)
			trace := &ds.Spec.Template.Spec.Containers[1]
			tt.mutate(trace)
			require.ErrorContains(t, prepareAgentTemplate(ds), tt.error)
		})
	}
}

func TestPreparedRolloutOlderTemplateMustArmV3Conventionally(t *testing.T) {
	current := preparedTestDaemonSet(true)
	current.Generation = 1
	current.Spec.Template.Annotations = map[string]string{}
	current.Spec.Template.Annotations[preparedRolloutModeAnnotation] = "prepared-surge-v1"
	current.Status = appsv1.DaemonSetStatus{ObservedGeneration: 1, DesiredNumberScheduled: 2, UpdatedNumberScheduled: 2, NumberAvailable: 2}
	desired := preparedTestDaemonSet(true)
	migrating, err := configurePreparedRollout(preparedRolloutDDAI(), desired, current, intstr.FromInt(1))
	require.NoError(t, err)
	require.True(t, migrating)
	assert.Equal(t, intstr.FromInt(0), *desired.Spec.UpdateStrategy.RollingUpdate.MaxSurge)
	assert.Equal(t, preparedRolloutArmedV3, desired.Spec.Template.Annotations[preparedRolloutArmedAnnotation])
	assert.Empty(t, desired.Spec.Template.Annotations[preparedRolloutModeAnnotation])
}

func TestPreparedRolloutSupportsAllRenderedAgentContainers(t *testing.T) {
	ds := preparedTestDaemonSet(true)
	ds.Spec.Template.Spec.Containers = []corev1.Container{
		{Name: string(apicommon.CoreAgentContainerName), Command: []string{"agent", "run"}, StartupProbe: constants.GetDefaultStartupProbe(), LivenessProbe: constants.GetDefaultLivenessProbe(), ReadinessProbe: constants.GetDefaultReadinessProbe()},
		{Name: string(apicommon.TraceAgentContainerName), Command: []string{"/entrypoint.sh", "trace-agent"}, LivenessProbe: constants.GetDefaultTraceAgentProbe()},
		{Name: string(apicommon.ProcessAgentContainerName), Command: []string{"process-agent"}},
		{Name: string(apicommon.SecurityAgentContainerName), Command: []string{"security-agent", "start"}},
		{Name: string(apicommon.SystemProbeContainerName), Command: []string{"system-probe"}},
		{Name: string(apicommon.HostProfiler), Command: []string{"host-profiler", "--core-config=/etc/datadog-agent/datadog.yaml"}},
		{Name: string(apicommon.OtelAgent), Command: []string{"otel-agent"}},
		{Name: string(apicommon.PrivateActionRunnerContainerName), Command: []string{"/opt/datadog-agent/embedded/bin/privateactionrunner"}},
	}
	ds.Spec.Template.Spec.InitContainers = append(ds.Spec.Template.Spec.InitContainers,
		corev1.Container{Name: "seccomp-setup"}, corev1.Container{Name: "host-profiler-seccomp-setup"})
	original := append([]corev1.Container(nil), ds.Spec.Template.Spec.Containers...)
	require.NoError(t, prepareAgentTemplate(ds))
	for i := range ds.Spec.Template.Spec.Containers {
		container := &ds.Spec.Template.Spec.Containers[i]
		assert.Equal(t, []string{preparedRolloutGateBinary}, container.Command)
		prefix := []string{"--component", container.Name}
		if container.Name != string(apicommon.CoreAgentContainerName) {
			prefix = append(prefix, "--wait-file", preparedRolloutAuthToken)
		}
		prefix = append(prefix, "--")
		require.GreaterOrEqual(t, len(container.Args), len(prefix)+1)
		assert.Equal(t, prefix, container.Args[:len(prefix)])
		assert.Equal(t, append(original[i].Command, original[i].Args...), container.Args[len(prefix):])
	}
}

func TestPreparedRolloutRejectsStandaloneHostProfiler(t *testing.T) {
	ds := preparedTestDaemonSet(true)
	ds.Spec.Template.Spec.Containers = append(ds.Spec.Template.Spec.Containers,
		corev1.Container{Name: string(apicommon.HostProfiler), Command: []string{"host-profiler"}})
	require.ErrorContains(t, prepareAgentTemplate(ds), "does not support command")
}

func TestPreparedRolloutRejectsUnknownSecurityAgentCommand(t *testing.T) {
	ds := preparedTestDaemonSet(true)
	ds.Spec.Template.Spec.Containers = append(ds.Spec.Template.Spec.Containers,
		corev1.Container{Name: string(apicommon.SecurityAgentContainerName), Command: []string{"security-agent", "status"}})
	require.ErrorContains(t, prepareAgentTemplate(ds), "does not support command")
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
		preparedRolloutModeAnnotation: preparedRolloutModeV3,
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
						{Name: string(apicommon.CoreAgentContainerName), Command: []string{"agent", "run"}, Ports: []corev1.ContainerPort{port}, StartupProbe: constants.GetDefaultStartupProbe(), LivenessProbe: constants.GetDefaultLivenessProbe(), ReadinessProbe: constants.GetDefaultReadinessProbe()},
						{Name: string(apicommon.TraceAgentContainerName), Command: []string{"/entrypoint.sh", "trace-agent"}, Ports: []corev1.ContainerPort{port}, LivenessProbe: constants.GetDefaultTraceAgentProbe()},
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

func hasVolumeMount(container corev1.Container, name, path string) bool {
	for _, mount := range container.VolumeMounts {
		if mount.Name == name && mount.MountPath == path {
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
