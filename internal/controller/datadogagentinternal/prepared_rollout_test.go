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
	agentcommon "github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrepareAgentTemplatePreservesHostNetworkAndUDS(t *testing.T) {
	ds := preparedTestDaemonSet(true)
	require.NoError(t, prepareAgentTemplate(ds))
	assert.True(t, ds.Spec.Template.Spec.HostNetwork)
	for i := range ds.Spec.Template.Spec.Containers {
		container := &ds.Spec.Template.Spec.Containers[i]
		assert.Empty(t, container.Ports)
		assert.True(t, hasVolumeMount(*container, preparedRolloutLockVolume, preparedRolloutLockDir))
		assert.True(t, hasVolumeMountWithMode(*container, preparedRolloutStateVolume, preparedRolloutStateDir, false))
		assert.True(t, hasVolumeMountWithMode(*container, preparedRolloutToolsVolume, preparedRolloutToolsDir, true))
	}
	assert.True(t, hasHostPath(ds.Spec.Template.Spec.Volumes, "/var/run/datadog"))
	assert.True(t, hasHostPath(ds.Spec.Template.Spec.Volumes, preparedRolloutLockDir))
	assert.True(t, hasEmptyDir(ds.Spec.Template.Spec.Volumes, preparedRolloutStateVolume))
	assert.True(t, hasEmptyDir(ds.Spec.Template.Spec.Volumes, preparedRolloutToolsVolume))

	podNetwork := preparedTestDaemonSet(false)
	for i := range podNetwork.Spec.Template.Spec.Containers {
		for j := range podNetwork.Spec.Template.Spec.Containers[i].Ports {
			podNetwork.Spec.Template.Spec.Containers[i].Ports[j].HostPort = 0
		}
	}
	require.NoError(t, prepareAgentTemplate(podNetwork))
	assert.NotEmpty(t, podNetwork.Spec.Template.Spec.Containers[0].Ports, "pod-network port metadata must be preserved")

	podNetworkWithHostPort := preparedTestDaemonSet(false)
	require.ErrorContains(t, prepareAgentTemplate(podNetworkWithHostPort), "does not support hostPort 8126")

	hostNetworkWithPortMapping := preparedTestDaemonSet(true)
	hostNetworkWithPortMapping.Spec.Template.Spec.Containers[0].Ports[0].HostPort = 18126
	require.ErrorContains(t, prepareAgentTemplate(hostNetworkWithPortMapping), "cannot preserve hostPort 18126 to containerPort 8126 mapping")
}

func TestPreparedRolloutUsesS6LocksAndActiveStartupProbe(t *testing.T) {
	base := preparedTestDaemonSet(true)
	original := append([]string{}, base.Spec.Template.Spec.Containers[0].Command...)
	require.NoError(t, prepareAgentTemplate(base))

	ds, err := preparedSlotDaemonSet(base, rolloutSlotBlue, "rollout.example/slot", "revision", true)
	require.NoError(t, err)
	for i := range ds.Spec.Template.Spec.Containers {
		container := &ds.Spec.Template.Spec.Containers[i]
		assert.Equal(t, []string{preparedRolloutSetlockBinary}, container.Command)
		require.GreaterOrEqual(t, len(container.Args), 4)
		assert.Equal(t, preparedComponentLockPath(container.Name), container.Args[0])
		assert.Equal(t, preparedRolloutSetlockBinary, container.Args[1])
		assert.Equal(t, preparedActiveLockPath(container.Name), container.Args[2])
		require.NotNil(t, container.StartupProbe)
		require.NotNil(t, container.StartupProbe.Exec)
		assert.Contains(t, container.StartupProbe.Exec.Command[2], preparedActiveLockPath(container.Name))
		assert.Equal(t, maxStartupProbeFailures, container.StartupProbe.FailureThreshold)
	}
	assert.Equal(t, original, ds.Spec.Template.Spec.Containers[0].Args[3:])
	assert.NotNil(t, ds.Spec.Template.Spec.Containers[0].LivenessProbe.HTTPGet)
	assert.NotNil(t, ds.Spec.Template.Spec.Containers[0].ReadinessProbe.HTTPGet)
	assert.NotNil(t, ds.Spec.Template.Spec.Containers[1].LivenessProbe.TCPSocket)
	assert.Equal(t, int32(5), ds.Spec.Template.Spec.Containers[1].LivenessProbe.FailureThreshold)
	assert.Nil(t, ds.Spec.Template.Spec.Containers[1].ReadinessProbe)
}

func TestPreparedRolloutCopiesS6FromCoreImage(t *testing.T) {
	ds := preparedTestDaemonSet(true)
	ds.Spec.Template.Spec.Containers[0].ImagePullPolicy = corev1.PullAlways
	require.NoError(t, prepareAgentTemplate(ds))

	init := ds.Spec.Template.Spec.InitContainers[len(ds.Spec.Template.Spec.InitContainers)-1]
	assert.Equal(t, preparedRolloutToolInit, init.Name)
	assert.Equal(t, "agent:test", init.Image)
	assert.Equal(t, corev1.PullAlways, init.ImagePullPolicy)
	assert.Equal(t, []string{"/usr/bin/cp"}, init.Command)
	assert.Equal(t, []string{preparedRolloutToolSource, preparedRolloutSetlockBinary}, init.Args)
	assert.True(t, hasVolumeMount(init, preparedRolloutToolsVolume, preparedRolloutToolsDir))
	assert.True(t, hasEmptyDir(ds.Spec.Template.Spec.Volumes, preparedRolloutToolsVolume))
}

func TestPreparedRolloutRequiresCoreImageForS6(t *testing.T) {
	ds := preparedTestDaemonSet(true)
	ds.Spec.Template.Spec.Containers[0].Image = ""
	require.ErrorContains(t, prepareAgentTemplate(ds), "requires the core Agent container image")
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
	require.ErrorContains(t, prepareAgentTemplate(ds), "does not support named readiness probe port")
}

func TestPreparedRolloutPreservesReadinessAndLiveness(t *testing.T) {
	ds := preparedTestDaemonSet(true)
	core := &ds.Spec.Template.Spec.Containers[0]
	readiness := core.ReadinessProbe.DeepCopy()
	liveness := core.LivenessProbe.DeepCopy()
	require.NoError(t, prepareAgentTemplate(ds))
	assert.Equal(t, readiness, core.ReadinessProbe)
	assert.Equal(t, liveness, core.LivenessProbe)
}

func TestPreparedRolloutReplacesCustomStartupAndPreservesHealthProbes(t *testing.T) {
	ds := preparedTestDaemonSet(true)
	core := &ds.Spec.Template.Spec.Containers[0]
	core.StartupProbe = &corev1.Probe{
		ProbeHandler:  corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/ready", Port: intstr.FromInt(5555)}},
		PeriodSeconds: 1, FailureThreshold: 200,
	}
	core.LivenessProbe.TimeoutSeconds++
	core.LivenessProbe.PeriodSeconds = 15
	core.LivenessProbe.FailureThreshold = 1
	ds.Spec.Template.Spec.Containers = append(ds.Spec.Template.Spec.Containers, corev1.Container{
		Name: string(apicommon.SystemProbeContainerName), Command: []string{"system-probe"},
		LivenessProbe: &corev1.Probe{ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/live", Port: intstr.FromInt(5558)}}},
	})

	require.NoError(t, prepareAgentTemplate(ds))
	ds, err := preparedSlotDaemonSet(ds, rolloutSlotBlue, "rollout.example/slot", "revision", true)
	require.NoError(t, err)
	core = &ds.Spec.Template.Spec.Containers[0]
	assert.Equal(t, preparedRolloutProbePeriodSec, core.StartupProbe.PeriodSeconds)
	assert.Equal(t, maxStartupProbeFailures, core.StartupProbe.FailureThreshold)
	assert.NotNil(t, core.StartupProbe.Exec)
	assert.NotNil(t, core.LivenessProbe.HTTPGet)
	assert.Equal(t, int32(15), core.LivenessProbe.FailureThreshold)
	assert.Equal(t, int32(5558), ds.Spec.Template.Spec.Containers[2].LivenessProbe.HTTPGet.Port.IntVal)
}

func TestPreparedRolloutRejectsStartupProbeWithoutLiveness(t *testing.T) {
	ds := preparedTestDaemonSet(true)
	core := &ds.Spec.Template.Spec.Containers[0]
	core.LivenessProbe = nil
	require.ErrorContains(t, prepareAgentTemplate(ds), "requires a liveness probe")
}

func TestPreparedRolloutSupportsOptimizedAgentContainers(t *testing.T) {
	ds := preparedTestDaemonSet(true)
	ds.Spec.Template.Spec.Containers = []corev1.Container{
		{Name: string(apicommon.CoreAgentContainerName), Image: "agent:test", Command: []string{"agent", "run"}, StartupProbe: constants.GetDefaultStartupProbe(), LivenessProbe: constants.GetDefaultLivenessProbe(), ReadinessProbe: constants.GetDefaultReadinessProbe()},
		{Name: string(apicommon.TraceAgentContainerName), Command: []string{"/entrypoint.sh", "trace-agent"}, LivenessProbe: constants.GetDefaultTraceAgentProbe()},
		{Name: string(apicommon.ProcessAgentContainerName), Command: []string{"process-agent"}},
		{Name: string(apicommon.SecurityAgentContainerName), Command: []string{"security-agent", "start"}},
		{Name: string(apicommon.SystemProbeContainerName), Command: []string{"system-probe"}},
		{Name: string(apicommon.HostProfiler), Command: []string{"host-profiler", "--core-config=/etc/datadog-agent/datadog.yaml"}},
		{Name: string(apicommon.OtelAgent), Command: []string{"otel-agent"}},
		{Name: string(apicommon.AgentDataPlaneContainerName), Command: []string{"agent-data-plane", "--config", "/etc/datadog-agent/datadog.yaml", "run"}, LivenessProbe: constants.GetDefaultAgentDataPlaneLivenessProbe(), ReadinessProbe: constants.GetDefaultAgentDataPlaneReadinessProbe()},
		{Name: string(apicommon.FlightRecorderContainerName), Command: []string{"/opt/datadog-agent/embedded/bin/flightrecorder"}},
		{Name: string(apicommon.PrivateActionRunnerContainerName), Command: []string{"/opt/datadog-agent/embedded/bin/privateactionrunner", "run", "-c=/etc/datadog-agent/datadog.yaml", "-E=/etc/datadog-agent/privateactionrunner.yaml"}},
	}
	ds.Spec.Template.Spec.InitContainers = append(ds.Spec.Template.Spec.InitContainers,
		corev1.Container{Name: "seccomp-setup", Command: []string{"cp", "/etc/config/system-probe-seccomp.json", "/host/var/lib/kubelet/seccomp/system-probe"}},
		corev1.Container{Name: "host-profiler-seccomp-setup", Command: []string{"cp", "/etc/dd-host-profiler/seccomp.json", "/host/var/lib/kubelet/seccomp/host-profiler-deadbeef"}})
	original := append([]corev1.Container(nil), ds.Spec.Template.Spec.Containers...)
	require.NoError(t, prepareAgentTemplate(ds))
	ds, err := preparedSlotDaemonSet(ds, rolloutSlotGreen, "rollout.example/slot", "revision", true)
	require.NoError(t, err)
	for i := range ds.Spec.Template.Spec.Containers {
		container := &ds.Spec.Template.Spec.Containers[i]
		assert.Equal(t, []string{preparedRolloutSetlockBinary}, container.Command)
		prefix := []string{preparedComponentLockPath(container.Name), preparedRolloutSetlockBinary, preparedActiveLockPath(container.Name)}
		require.GreaterOrEqual(t, len(container.Args), len(prefix)+1)
		assert.Equal(t, prefix, container.Args[:len(prefix)])
		assert.Equal(t, append(original[i].Command, original[i].Args...), container.Args[len(prefix):])
		assert.Contains(t, container.StartupProbe.Exec.Command[2], preparedActiveLockPath(container.Name))
	}
	assert.Nil(t, ds.Spec.Template.Spec.Containers[2].ReadinessProbe, "components without readiness keep baseline behavior")
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

func TestPreparedRolloutLeavesUnknownSidecarUnchanged(t *testing.T) {
	ds := preparedTestDaemonSet(true)
	sidecar := corev1.Container{
		Name: "unknown-sidecar", Command: []string{"sidecar"}, Args: []string{"serve"},
		Lifecycle: &corev1.Lifecycle{}, SecurityContext: &corev1.SecurityContext{RunAsUser: ptr.To[int64](1000)},
		Ports: []corev1.ContainerPort{{Name: "sidecar", ContainerPort: 9000, HostPort: 9000}},
	}
	ds.Spec.Template.Spec.Containers = append(ds.Spec.Template.Spec.Containers, sidecar)
	require.NoError(t, prepareAgentTemplate(ds))
	green, err := preparedSlotDaemonSet(ds, rolloutSlotGreen, "rollout.example/slot", "revision", false)
	require.NoError(t, err)
	actual := green.Spec.Template.Spec.Containers[len(green.Spec.Template.Spec.Containers)-1]
	assert.Equal(t, sidecar.Command, actual.Command)
	assert.Equal(t, sidecar.Args, actual.Args)
	assert.Equal(t, sidecar.Lifecycle, actual.Lifecycle)
	assert.Empty(t, actual.Ports, "host-network port declarations must not block overlap")
	assert.Nil(t, actual.StartupProbe)
	assert.Nil(t, containerEnv(&actual, coreAgentCmdPortEnv))
	assert.False(t, hasVolumeMount(actual, preparedRolloutLockVolume, preparedRolloutLockDir))
}

func TestPreparedRolloutRejectsExplicitCoreCommandPort(t *testing.T) {
	ds := preparedTestDaemonSet(true)
	ds.Spec.Template.Spec.Containers[0].Env = append(ds.Spec.Template.Spec.Containers[0].Env, corev1.EnvVar{Name: coreAgentCmdPortEnv, Value: "6001"})
	require.ErrorContains(t, prepareAgentTemplate(ds), "does not support an explicit DD_CMD_PORT")
}

func TestPreparedRolloutRejectsSingleContainerStrategy(t *testing.T) {
	ds := preparedTestDaemonSet(true)
	ds.Spec.Template.Spec.Containers = []corev1.Container{{Name: string(apicommon.UnprivilegedSingleAgentContainerName)}}
	require.ErrorContains(t, prepareAgentTemplate(ds), "single-container Agent")
}

func TestPreparedRolloutRejectsFIPSProxy(t *testing.T) {
	ds := preparedTestDaemonSet(true)
	ds.Spec.Template.Spec.Containers = append(ds.Spec.Template.Spec.Containers, corev1.Container{Name: string(apicommon.FIPSProxyContainerName)})
	require.ErrorContains(t, prepareAgentTemplate(ds), "FIPS proxy")
}

func TestPreparedRolloutRejectsMutatedAllowlistedInitContainer(t *testing.T) {
	ds := preparedTestDaemonSet(true)
	ds.Spec.Template.Spec.InitContainers[0].Command = []string{"sh", "-c"}
	require.ErrorContains(t, prepareAgentTemplate(ds), "does not support command")
}

func TestPreparedRolloutRejectsWritableInitHostPathBeforeHandoff(t *testing.T) {
	ds := preparedTestDaemonSet(true)
	hostPathType := corev1.HostPathDirectoryOrCreate
	ds.Spec.Template.Spec.Volumes = append(ds.Spec.Template.Spec.Volumes, corev1.Volume{
		Name: "mutable-host", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/var/lib/example", Type: &hostPathType}},
	})
	ds.Spec.Template.Spec.InitContainers[0].VolumeMounts = append(ds.Spec.Template.Spec.InitContainers[0].VolumeMounts,
		corev1.VolumeMount{Name: "mutable-host", MountPath: "/host-state"})
	require.ErrorContains(t, prepareAgentTemplate(ds), "does not support writable hostPath")
}

func TestPreparedRolloutAllowsKnownSeccompHostWrite(t *testing.T) {
	ds := preparedTestDaemonSet(true)
	hostPathType := corev1.HostPathDirectoryOrCreate
	ds.Spec.Template.Spec.Volumes = append(ds.Spec.Template.Spec.Volumes, corev1.Volume{
		Name: "seccomp-root", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/var/lib/kubelet/seccomp", Type: &hostPathType}},
	})
	ds.Spec.Template.Spec.InitContainers = append(ds.Spec.Template.Spec.InitContainers, corev1.Container{
		Name: "seccomp-setup", Command: []string{"cp", "/etc/config/system-probe-seccomp.json", "/host/var/lib/kubelet/seccomp/system-probe"},
		VolumeMounts: []corev1.VolumeMount{{Name: "seccomp-root", MountPath: "/host/var/lib/kubelet/seccomp"}},
	})
	require.NoError(t, prepareAgentTemplate(ds))
}

func TestPreparedRolloutRejectsInitLockPathCollision(t *testing.T) {
	ds := preparedTestDaemonSet(true)
	ds.Spec.Template.Spec.Volumes = append(ds.Spec.Template.Spec.Volumes, corev1.Volume{Name: "collision", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}})
	ds.Spec.Template.Spec.InitContainers[0].VolumeMounts = append(ds.Spec.Template.Spec.InitContainers[0].VolumeMounts,
		corev1.VolumeMount{Name: "collision", MountPath: preparedRolloutLockDir})
	require.ErrorContains(t, prepareAgentTemplate(ds), "lock mount conflicts")
}

func TestPreparedRolloutValidationEdgeCases(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*appsv1.DaemonSet)
		error  string
	}{
		{
			name: "custom pod anti-affinity",
			mutate: func(ds *appsv1.DaemonSet) {
				ds.Spec.Template.Spec.Affinity = &corev1.Affinity{PodAntiAffinity: &corev1.PodAntiAffinity{}}
			},
			error: "custom Pod anti-affinity",
		},
		{
			name: "reserved volume name",
			mutate: func(ds *appsv1.DaemonSet) {
				ds.Spec.Template.Spec.Volumes = append(ds.Spec.Template.Spec.Volumes, corev1.Volume{Name: preparedRolloutLockVolume})
			},
			error: "volume name",
		},
		{
			name: "reserved tools volume name",
			mutate: func(ds *appsv1.DaemonSet) {
				ds.Spec.Template.Spec.Volumes = append(ds.Spec.Template.Spec.Volumes, corev1.Volume{Name: preparedRolloutToolsVolume})
			},
			error: preparedRolloutToolsVolume,
		},
		{
			name: "container lifecycle",
			mutate: func(ds *appsv1.DaemonSet) {
				ds.Spec.Template.Spec.Containers[0].Lifecycle = &corev1.Lifecycle{}
			},
			error: "lifecycle hooks on container",
		},
		{
			name: "unknown init container",
			mutate: func(ds *appsv1.DaemonSet) {
				ds.Spec.Template.Spec.InitContainers = append(ds.Spec.Template.Spec.InitContainers, corev1.Container{Name: "unknown-init"})
			},
			error: "does not support init container",
		},
		{
			name: "init container lifecycle",
			mutate: func(ds *appsv1.DaemonSet) {
				ds.Spec.Template.Spec.InitContainers[0].Lifecycle = &corev1.Lifecycle{}
			},
			error: "lifecycle hooks on init container",
		},
		{
			name: "seccomp arguments",
			mutate: func(ds *appsv1.DaemonSet) {
				ds.Spec.Template.Spec.InitContainers = append(ds.Spec.Template.Spec.InitContainers, corev1.Container{
					Name: "seccomp-setup", Command: []string{"cp", agentcommon.SeccompSecurityVolumePath + "/" + agentcommon.SystemProbeSeccompKey, agentcommon.SeccompRootVolumePath + "/" + agentcommon.SystemProbeSeccompProfileName}, Args: []string{"unexpected"},
				})
			},
			error: "does not support command",
		},
		{
			name: "missing command",
			mutate: func(ds *appsv1.DaemonSet) {
				ds.Spec.Template.Spec.Containers[0].Command = nil
			},
			error: "does not support command",
		},
		{
			name: "unexpected core command",
			mutate: func(ds *appsv1.DaemonSet) {
				ds.Spec.Template.Spec.Containers[0].Command = []string{"not-agent"}
			},
			error: "does not support command",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ds := preparedTestDaemonSet(true)
			tt.mutate(ds)
			require.ErrorContains(t, prepareAgentTemplate(ds), tt.error)
		})
	}
}

func TestPreparedRolloutAcceptsTemplatesWithoutNodeSelector(t *testing.T) {
	ds := preparedTestDaemonSet(true)
	ds.Spec.Template.Spec.NodeSelector = nil

	require.NoError(t, prepareAgentTemplate(ds))
	assert.Equal(t, string(corev1.Linux), ds.Spec.Template.Spec.NodeSelector[corev1.LabelOSStable])
}

func TestPreparedRolloutRejectsUnsafeTCPProbeVariants(t *testing.T) {
	ds := preparedTestDaemonSet(true)
	ds.Spec.Template.Spec.Containers[1].LivenessProbe.TCPSocket.Port = intstr.FromString("trace")
	require.ErrorContains(t, prepareAgentTemplate(ds), "does not support named liveness probe port")
}

func TestPreparedHostProfilerInitCommandVariants(t *testing.T) {
	destination := agentcommon.SeccompRootVolumePath + "/host-profiler-deadbeef-logging"
	validShell := &corev1.Container{
		Name: "host-profiler-seccomp-setup",
		Command: []string{"sh", "-c",
			"if [ -f /etc/dd-host-profiler/logging-seccomp.json ]; then cp /etc/dd-host-profiler/logging-seccomp.json " + destination +
				"; else echo 'WARNING: logging-seccomp.json not found in image, falling back to default seccomp profile'; cp /etc/dd-host-profiler/seccomp.json " + destination + "; fi"},
	}
	assert.True(t, preparedInitCommandSupported(validShell))

	withArgs := validShell.DeepCopy()
	withArgs.Args = []string{"unexpected"}
	assert.False(t, preparedInitCommandSupported(withArgs))

	appendedWrite := validShell.DeepCopy()
	appendedWrite.Command[2] += " && touch " + agentcommon.SeccompRootVolumePath + "/evil"
	assert.False(t, preparedInitCommandSupported(appendedWrite))

	differentFallbackDestination := validShell.DeepCopy()
	differentFallbackDestination.Command[2] = strings.Replace(
		differentFallbackDestination.Command[2],
		"cp /etc/dd-host-profiler/seccomp.json "+destination,
		"cp /etc/dd-host-profiler/seccomp.json "+agentcommon.SeccompRootVolumePath+"/host-profiler-other",
		1,
	)
	assert.False(t, preparedInitCommandSupported(differentFallbackDestination))

	injectedDestination := validShell.DeepCopy()
	injectedDestination.Command[2] = strings.ReplaceAll(
		injectedDestination.Command[2],
		destination,
		destination+"; touch pwn",
	)
	assert.False(t, preparedInitCommandSupported(injectedDestination))

	invalidCommands := map[string]string{
		"different shell structure": strings.Replace(validShell.Command[2], "if [ -f", "while [ -f", 1),
		"different fallback":        strings.Replace(validShell.Command[2], "else echo", "else printf", 1),
		"destination outside root":  strings.ReplaceAll(validShell.Command[2], destination, "/tmp/host-profiler-deadbeef-logging"),
		"non-hex profile hash":      strings.ReplaceAll(validShell.Command[2], "deadbeef", "deadbeeG"),
	}
	for name, command := range invalidCommands {
		t.Run(name, func(t *testing.T) {
			invalid := validShell.DeepCopy()
			invalid.Command[2] = command
			assert.False(t, preparedInitCommandSupported(invalid))
		})
	}
	assert.False(t, preparedInitCommandSupported(&corev1.Container{Name: "unknown-init"}))
}

func TestPreparedContainerSecurityAndEnvironmentOverrides(t *testing.T) {
	tests := []struct {
		name      string
		pod       *corev1.PodSecurityContext
		container *corev1.SecurityContext
		want      bool
	}{
		{name: "container requires non-root", container: &corev1.SecurityContext{RunAsNonRoot: ptr.To(true)}, want: true},
		{name: "pod selects non-root UID", pod: &corev1.PodSecurityContext{RunAsUser: ptr.To[int64](1000)}, want: true},
		{name: "pod selects root UID", pod: &corev1.PodSecurityContext{RunAsUser: ptr.To[int64](0)}},
		{name: "pod non-root constraint remains with container root UID", pod: &corev1.PodSecurityContext{RunAsNonRoot: ptr.To(true)}, container: &corev1.SecurityContext{RunAsUser: ptr.To[int64](0)}, want: true},
		{name: "container clears pod non-root constraint", pod: &corev1.PodSecurityContext{RunAsNonRoot: ptr.To(true)}, container: &corev1.SecurityContext{RunAsNonRoot: ptr.To(false)}},
		{name: "container root UID overrides pod UID", pod: &corev1.PodSecurityContext{RunAsUser: ptr.To[int64](1000)}, container: &corev1.SecurityContext{RunAsUser: ptr.To[int64](0)}},
		{name: "container non-root UID overrides pod UID", pod: &corev1.PodSecurityContext{RunAsUser: ptr.To[int64](0)}, container: &corev1.SecurityContext{RunAsUser: ptr.To[int64](1000)}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, containerRunsAsNonRoot(tt.pod, tt.container))
		})
	}

	core := corev1.Container{Env: []corev1.EnvVar{{Name: coreAgentCmdPortEnv, Value: "stale"}}}
	setContainerEnv(&core, corev1.EnvVar{Name: coreAgentCmdPortEnv, Value: "current"})
	require.Len(t, core.Env, 1)
	assert.Equal(t, "current", core.Env[0].Value)
}

func TestProfileOverlapAntiAffinityAllowsOnlySameProfileOverlap(t *testing.T) {
	antiAffinity, ok := profileOverlapPodAntiAffinity(map[string]string{
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

func preparedTestDaemonSet(hostNetwork bool) *appsv1.DaemonSet {
	port := corev1.ContainerPort{Name: "declared", ContainerPort: 8126, HostPort: 8126, Protocol: corev1.ProtocolTCP}
	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: "default"},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "agent", "app.kubernetes.io/instance": "agent"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
					"app":                                      "agent",
					"app.kubernetes.io/instance":               "agent",
					apicommon.AgentDeploymentNameLabelKey:      "agent",
					apicommon.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix,
				}},
				Spec: corev1.PodSpec{
					HostNetwork:  hostNetwork,
					NodeSelector: map[string]string{corev1.LabelOSStable: "linux"},
					InitContainers: []corev1.Container{
						{Name: string(apicommon.InitVolumeContainerName), Command: []string{"bash", "-c"}, Args: []string{"cp -vnr /etc/datadog-agent /opt"}},
						{Name: string(apicommon.InitConfigContainerName), Command: []string{"bash", "-c"}, Args: []string{"for script in $(find /etc/cont-init.d/ -type f -name '*.sh' | sort) ; do bash $script ; done"}},
					},
					Containers: []corev1.Container{
						{Name: string(apicommon.CoreAgentContainerName), Image: "agent:test", Command: []string{"agent", "run"}, Ports: []corev1.ContainerPort{port}, StartupProbe: constants.GetDefaultStartupProbe(), LivenessProbe: constants.GetDefaultLivenessProbe(), ReadinessProbe: constants.GetDefaultReadinessProbe()},
						{Name: string(apicommon.TraceAgentContainerName), Image: "agent:test", Command: []string{"/entrypoint.sh", "trace-agent"}, Ports: []corev1.ContainerPort{port}, LivenessProbe: constants.GetDefaultTraceAgentProbe()},
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

func hasEmptyDir(volumes []corev1.Volume, name string) bool {
	for i := range volumes {
		if volumes[i].Name == name && volumes[i].EmptyDir != nil {
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

func hasVolumeMountWithMode(container corev1.Container, name, path string, readOnly bool) bool {
	for _, mount := range container.VolumeMounts {
		if mount.Name == name && mount.MountPath == path && mount.ReadOnly == readOnly {
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
