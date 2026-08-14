// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// Copyright 2016-present Datadog, Inc.

package datadogagentinternal

import (
	"fmt"
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
	}
	assert.True(t, hasHostPath(ds.Spec.Template.Spec.Volumes, "/var/run/datadog"))
	assert.True(t, hasHostPath(ds.Spec.Template.Spec.Volumes, preparedRolloutLockDir))

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

func TestPreparedRolloutUsesGenerationSafePreparedStartupProbes(t *testing.T) {
	ds := preparedTestDaemonSet(true)
	require.NoError(t, prepareAgentTemplate(ds))
	for i := range ds.Spec.Template.Spec.Containers {
		container := &ds.Spec.Template.Spec.Containers[i]
		require.NotNil(t, container.StartupProbe)
		require.NotNil(t, container.StartupProbe.Exec)
		assert.Equal(t, preparedRolloutGateBinary, container.StartupProbe.Exec.Command[0])
		assert.Contains(t, container.StartupProbe.Exec.Command, "startup")
		assert.Equal(t, container.Name, probeArgument(t, container.StartupProbe.Exec.Command, "--component"))
		assert.Equal(t, maxStartupProbeFailures, container.StartupProbe.FailureThreshold)
		assert.Equal(t, "metadata.uid", containerEnv(container, rolloutPodUIDEnv).ValueFrom.FieldRef.FieldPath)
		assert.Equal(t, "status.podIP", containerEnv(container, rolloutPodIPEnv).ValueFrom.FieldRef.FieldPath)
	}
	assert.Contains(t, ds.Spec.Template.Spec.Containers[0].StartupProbe.Exec.Command, fmt.Sprint(constants.DefaultAgentHealthPort))
	assert.NotNil(t, ds.Spec.Template.Spec.Containers[0].LivenessProbe.HTTPGet)
	assert.NotNil(t, ds.Spec.Template.Spec.Containers[0].ReadinessProbe.HTTPGet)
	assert.NotNil(t, ds.Spec.Template.Spec.Containers[1].LivenessProbe.TCPSocket)
	assert.Nil(t, ds.Spec.Template.Spec.Containers[1].ReadinessProbe)
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
			error: "readiness probe must use HTTP or TCP",
		},
		{
			name: "HTTP headers",
			mutate: func(container *corev1.Container) {
				container.ReadinessProbe.HTTPGet.HTTPHeaders = []corev1.HTTPHeader{{Name: "X-Test", Value: "value"}}
			},
			error: "readiness HTTP headers are not supported",
		},
		{
			name: "HTTPS",
			mutate: func(container *corev1.Container) {
				container.ReadinessProbe.HTTPGet.Scheme = corev1.URISchemeHTTPS
			},
			error: "readiness HTTP scheme must be HTTP",
		},
		{
			name: "gRPC",
			mutate: func(container *corev1.Container) {
				container.ReadinessProbe.ProbeHandler = corev1.ProbeHandler{GRPC: &corev1.GRPCAction{Port: 5555}}
			},
			error: "readiness probe must use HTTP or TCP",
		},
		{
			name: "relative HTTP path",
			mutate: func(container *corev1.Container) {
				container.ReadinessProbe.HTTPGet.Path = "ready"
			},
			error: "readiness HTTP path must be absolute",
		},
		{
			name: "multiple handlers",
			mutate: func(container *corev1.Container) {
				container.ReadinessProbe.TCPSocket = &corev1.TCPSocketAction{Port: intstr.FromInt(5555)}
			},
			error: "readiness probe must have exactly one handler",
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

func TestPreparedRolloutPreservesStartupOnlyHealthSignal(t *testing.T) {
	ds := preparedTestDaemonSet(true)
	core := &ds.Spec.Template.Spec.Containers[0]
	core.LivenessProbe = nil
	core.ReadinessProbe = nil
	require.NoError(t, prepareAgentTemplate(ds))
	assert.Nil(t, core.LivenessProbe)
	assert.Nil(t, core.ReadinessProbe)
	assert.Contains(t, core.StartupProbe.Exec.Command, constants.DefaultStartupProbeHTTPPath)
}

func TestPreparedStartupProbePreservesActiveRestartBudget(t *testing.T) {
	ds := preparedTestDaemonSet(true)
	podGrace := int64(42)
	probeGrace := int64(7)
	ds.Spec.Template.Spec.TerminationGracePeriodSeconds = &podGrace
	core := &ds.Spec.Template.Spec.Containers[0]
	core.StartupProbe.FailureThreshold = 6
	core.StartupProbe.TerminationGracePeriodSeconds = &probeGrace

	require.NoError(t, prepareAgentTemplate(ds))
	command := ds.Spec.Template.Spec.Containers[0].StartupProbe.Exec.Command
	assert.Equal(t, "6", probeArgument(t, command, "--failure-threshold"))
	assert.Equal(t, "7s", probeArgument(t, command, "--termination-grace-period"))

	traceCommand := ds.Spec.Template.Spec.Containers[1].StartupProbe.Exec.Command
	assert.NotContains(t, traceCommand, "--failure-threshold", "a synthesized active-only startup probe cannot fail after handoff")
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
	assert.NotNil(t, core.LivenessProbe.HTTPGet)
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

func TestPreparedRolloutSupportsOptimizedAgentContainers(t *testing.T) {
	ds := preparedTestDaemonSet(true)
	ds.Spec.Template.Spec.Containers = []corev1.Container{
		{Name: string(apicommon.CoreAgentContainerName), Command: []string{"agent", "run"}, StartupProbe: constants.GetDefaultStartupProbe(), LivenessProbe: constants.GetDefaultLivenessProbe(), ReadinessProbe: constants.GetDefaultReadinessProbe()},
		{Name: string(apicommon.TraceAgentContainerName), Command: []string{"/entrypoint.sh", "trace-agent"}, LivenessProbe: constants.GetDefaultTraceAgentProbe()},
		{Name: string(apicommon.ProcessAgentContainerName), Command: []string{"process-agent"}},
		{Name: string(apicommon.SecurityAgentContainerName), Command: []string{"security-agent", "start"}},
		{Name: string(apicommon.SystemProbeContainerName), Command: []string{"system-probe"}},
		{Name: string(apicommon.HostProfiler), Command: []string{"host-profiler", "--core-config=/etc/datadog-agent/datadog.yaml"}},
		{Name: string(apicommon.OtelAgent), Command: []string{"otel-agent"}},
		{Name: string(apicommon.AgentDataPlaneContainerName), Command: []string{"agent-data-plane", "--config", "/etc/datadog-agent/datadog.yaml", "run"}, LivenessProbe: constants.GetDefaultAgentDataPlaneLivenessProbe(), ReadinessProbe: constants.GetDefaultAgentDataPlaneReadinessProbe()},
		{Name: string(apicommon.FlightRecorderContainerName), Command: []string{"/opt/datadog-agent/embedded/bin/flightrecorder"}},
	}
	ds.Spec.Template.Spec.InitContainers = append(ds.Spec.Template.Spec.InitContainers,
		corev1.Container{Name: "seccomp-setup", Command: []string{"cp", "/etc/config/system-probe-seccomp.json", "/host/var/lib/kubelet/seccomp/system-probe"}},
		corev1.Container{Name: "host-profiler-seccomp-setup", Command: []string{"cp", "/etc/dd-host-profiler/seccomp.json", "/host/var/lib/kubelet/seccomp/host-profiler-deadbeef"}})
	original := append([]corev1.Container(nil), ds.Spec.Template.Spec.Containers...)
	require.NoError(t, prepareAgentTemplate(ds))
	for i := range ds.Spec.Template.Spec.Containers {
		container := &ds.Spec.Template.Spec.Containers[i]
		assert.Equal(t, []string{preparedRolloutGateBinary}, container.Command)
		prefix := []string{"--component", container.Name}
		if container.Name != string(apicommon.CoreAgentContainerName) && container.Name != string(apicommon.FlightRecorderContainerName) {
			prefix = append(prefix, "--wait-file", preparedRolloutAuthToken)
		}
		prefix = append(prefix, "--")
		require.GreaterOrEqual(t, len(container.Args), len(prefix)+1)
		assert.Equal(t, prefix, container.Args[:len(prefix)])
		assert.Equal(t, append(original[i].Command, original[i].Args...), container.Args[len(prefix):])
	}
	assert.Nil(t, ds.Spec.Template.Spec.Containers[2].ReadinessProbe, "components without readiness keep baseline behavior")
	flightRecorder := ds.Spec.Template.Spec.Containers[8]
	assert.NotContains(t, flightRecorder.Args, "--wait-file", "Flight Recorder has no auth-token mount and must not wait for the core token")
}

func TestPreparedRolloutRejectsPrivateActionRunnerWithoutDurableIdentity(t *testing.T) {
	ds := preparedTestDaemonSet(true)
	ds.Spec.Template.Spec.Containers = append(ds.Spec.Template.Spec.Containers, corev1.Container{
		Name: string(apicommon.PrivateActionRunnerContainerName), Command: []string{"/opt/datadog-agent/embedded/bin/privateactionrunner"},
	})
	require.ErrorContains(t, prepareAgentTemplate(ds), "does not support container \"private-action-runner\"")
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

func TestPreparedRolloutRejectsExplicitCoreCommandPort(t *testing.T) {
	ds := preparedTestDaemonSet(true)
	ds.Spec.Template.Spec.Containers[0].Env = append(ds.Spec.Template.Spec.Containers[0].Env, corev1.EnvVar{Name: coreAgentCmdPortEnv, Value: "6001"})
	require.ErrorContains(t, prepareAgentTemplate(ds), "does not support an explicit DD_CMD_PORT")
}

func TestPreparedRolloutRejectsSingleContainerStrategy(t *testing.T) {
	ds := preparedTestDaemonSet(true)
	ds.Spec.Template.Spec.Containers = []corev1.Container{{Name: string(apicommon.UnprivilegedSingleAgentContainerName)}}
	require.ErrorContains(t, prepareAgentTemplate(ds), "does not support container")
}

func TestPreparedRolloutRejectsFIPSProxy(t *testing.T) {
	ds := preparedTestDaemonSet(true)
	ds.Spec.Template.Spec.Containers = append(ds.Spec.Template.Spec.Containers, corev1.Container{Name: string(apicommon.FIPSProxyContainerName)})
	require.ErrorContains(t, prepareAgentTemplate(ds), "does not support container")
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
	tests := []struct {
		name   string
		mutate func(*corev1.Probe)
		error  string
	}{
		{
			name: "multiple handlers",
			mutate: func(probe *corev1.Probe) {
				probe.Exec = &corev1.ExecAction{Command: []string{"true"}}
			},
			error: "must have exactly one handler",
		},
		{
			name: "named port",
			mutate: func(probe *corev1.Probe) {
				probe.TCPSocket.Port = intstr.FromString("trace")
			},
			error: "port must be a positive number",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ds := preparedTestDaemonSet(true)
			probe := ds.Spec.Template.Spec.Containers[1].LivenessProbe
			tt.mutate(probe)
			require.ErrorContains(t, prepareAgentTemplate(ds), tt.error)
		})
	}
}

func TestPreparedStartupProbeUsesKubernetesDefaults(t *testing.T) {
	probe := constants.GetDefaultStartupProbe()
	probe.FailureThreshold = 0
	probe.TimeoutSeconds = 0
	probe.TerminationGracePeriodSeconds = nil

	handler := preparedStartupProbe(string(apicommon.CoreAgentContainerName), probe, nil)
	require.NotNil(t, handler.Exec)
	assert.Equal(t, "3", probeArgument(t, handler.Exec.Command, "--failure-threshold"))
	assert.Equal(t, "30s", probeArgument(t, handler.Exec.Command, "--termination-grace-period"))
	assert.Equal(t, "900ms", probeArgument(t, handler.Exec.Command, "--timeout"))
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

	core := corev1.Container{Env: []corev1.EnvVar{{Name: rolloutPodUIDEnv, Value: "stale"}}}
	setContainerEnv(&core, corev1.EnvVar{Name: rolloutPodUIDEnv, Value: "current"})
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
						{Name: string(apicommon.InitVolumeContainerName), Command: []string{"bash", "-c"}, Args: []string{"cp -vnr /etc/datadog-agent /opt"}},
						{Name: string(apicommon.InitConfigContainerName), Command: []string{"bash", "-c"}, Args: []string{"for script in $(find /etc/cont-init.d/ -type f -name '*.sh' | sort) ; do bash $script ; done"}},
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

func probeArgument(t *testing.T, command []string, name string) string {
	t.Helper()
	for i := 0; i+1 < len(command); i++ {
		if command[i] == name {
			return command[i+1]
		}
	}
	t.Fatalf("probe command %v has no %s argument", command, name)
	return ""
}
