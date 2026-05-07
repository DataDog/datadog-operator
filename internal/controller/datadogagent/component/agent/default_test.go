package agent

import (
	"fmt"
	"testing"

	"k8s.io/utils/ptr"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/pkg/constants"
)

func findVolume(volumes []corev1.Volume, name string) *corev1.Volume {
	for i := range volumes {
		if volumes[i].Name == name {
			return &volumes[i]
		}
	}
	return nil
}

func TestVolumesForAgent(t *testing.T) {
	tests := []struct {
		name                string
		dda                 metav1.Object
		requiredContainers  []apicommon.AgentContainerName
		expectedSeccompName string
		expectedInstallName string
	}{
		{
			name: "foo DDA",
			dda: &metav1.ObjectMeta{
				Name:      "foo",
				Namespace: "default",
				Labels:    map[string]string{},
			},
			requiredContainers:  []apicommon.AgentContainerName{apicommon.SystemProbeContainerName},
			expectedSeccompName: "foo-system-probe-seccomp",
			expectedInstallName: "foo-install-info",
		},
		{
			name: "profile DDAI",
			dda: &metav1.ObjectMeta{
				Name:      "my-profile",
				Namespace: "default",
				Labels: map[string]string{
					constants.ProfileLabelKey:          "my-profile",
					apicommon.DatadogAgentNameLabelKey: "foo",
				},
			},
			requiredContainers:  []apicommon.AgentContainerName{apicommon.SystemProbeContainerName},
			expectedSeccompName: "foo-system-probe-seccomp",
			expectedInstallName: "foo-install-info",
		},
		{
			name: "foo DDAI (same name as original DDA, no profile label)",
			dda: &metav1.ObjectMeta{
				Name:      "foo",
				Namespace: "default",
				Labels: map[string]string{
					apicommon.DatadogAgentNameLabelKey: "foo",
				},
			},
			requiredContainers:  []apicommon.AgentContainerName{apicommon.SystemProbeContainerName},
			expectedSeccompName: "foo-system-probe-seccomp",
			expectedInstallName: "foo-install-info",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			volumes := volumesForAgent(tt.dda, tt.requiredContainers)

			installVol := findVolume(volumes, common.InstallInfoVolumeName)
			assert.NotNil(t, installVol, "install-info volume should exist")
			assert.Equal(t, tt.expectedInstallName, installVol.ConfigMap.Name)

			seccompVol := findVolume(volumes, common.SeccompSecurityVolumeName)
			assert.NotNil(t, seccompVol, "seccomp security volume should exist")
			assert.Equal(t, tt.expectedSeccompName, seccompVol.ConfigMap.Name)
		})
	}
}

func TestVolumesForAgentHostProfiler(t *testing.T) {
	dda := &metav1.ObjectMeta{Name: "foo", Namespace: "default", Labels: map[string]string{}}

	t.Run("host-profiler only", func(t *testing.T) {
		volumes := volumesForAgent(dda, []apicommon.AgentContainerName{apicommon.HostProfiler})

		hpVol := findVolume(volumes, common.HostProfilerSecurityVolumeName)
		assert.NotNil(t, hpVol, "host-profiler security volume should exist")
		assert.Equal(t, "foo-host-profiler-seccomp", hpVol.ConfigMap.Name)

		assert.NotNil(t, findVolume(volumes, common.SeccompRootVolumeName), "seccomp-root volume should exist")
	})

	t.Run("system-probe and host-profiler share seccomp-root", func(t *testing.T) {
		volumes := volumesForAgent(dda, []apicommon.AgentContainerName{apicommon.SystemProbeContainerName, apicommon.HostProfiler})

		count := 0
		for _, v := range volumes {
			if v.Name == common.SeccompRootVolumeName {
				count++
			}
		}
		assert.Equal(t, 1, count, "seccomp-root volume should appear exactly once")
	})
}

func TestCommonEnvVars(t *testing.T) {
	tests := []struct {
		name                string
		dda                 metav1.Object
		expectedServiceName string
		expectedSecretName  string
	}{
		{
			name: "foo DDA",
			dda: &metav1.ObjectMeta{
				Name:      "foo",
				Namespace: "default",
				Labels:    map[string]string{},
			},
			expectedServiceName: "foo-cluster-agent",
			expectedSecretName:  "foo-token",
		},
		{
			name: "profile DDAI",
			dda: &metav1.ObjectMeta{
				Name:      "my-profile",
				Namespace: "default",
				Labels: map[string]string{
					constants.ProfileLabelKey:          "my-profile",
					apicommon.DatadogAgentNameLabelKey: "foo",
				},
			},
			expectedServiceName: "foo-cluster-agent",
			expectedSecretName:  "foo-token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			envVars := commonEnvVars(tt.dda)

			// Find the relevant env vars
			var clusterAgentServiceName string
			var clusterAgentTokenName string

			for _, env := range envVars {
				switch env.Name {
				case common.DDClusterAgentKubeServiceName:
					clusterAgentServiceName = env.Value
				case common.DDClusterAgentTokenName:
					clusterAgentTokenName = env.Value
				}
			}

			assert.Equal(t, tt.expectedServiceName, clusterAgentServiceName)
			assert.Equal(t, tt.expectedSecretName, clusterAgentTokenName)
		})
	}
}

func TestDefaultSyscallsForSystemProbe(t *testing.T) {
	tests := []struct {
		name             string
		ddaSpec          *v2alpha1.DatadogAgentSpec
		expectedSyscalls []string
	}{
		{
			name: "default syscalls",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{},
			},
			expectedSyscalls: DefaultSyscallsForSystemProbe(),
		},
		{
			name: "cws enabled and enforcement disabled",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					CWS: &v2alpha1.CWSFeatureConfig{
						Enabled: ptr.To(true),
					},
				},
			},
			expectedSyscalls: DefaultSyscallsForSystemProbe(),
		},
		{
			name: "cws enabled and enforcement enabled",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					CWS: &v2alpha1.CWSFeatureConfig{
						Enabled: ptr.To(true),
						Enforcement: &v2alpha1.CWSEnforcementConfig{
							Enabled: ptr.To(true),
						},
					},
				},
			},
			expectedSyscalls: append(DefaultSyscallsForSystemProbe(), "kill"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			syscalls := syscallsForSystemProbe(tt.ddaSpec)
			assert.Equal(t, tt.expectedSyscalls, syscalls)
		})
	}
}

func TestHostProfilerContainer(t *testing.T) {
	dda := &metav1.ObjectMeta{Name: "foo", Namespace: "default", Labels: map[string]string{}}

	containers := agentOptimizedContainers(dda, []apicommon.AgentContainerName{
		apicommon.CoreAgentContainerName,
		apicommon.HostProfiler,
	})
	assert.Len(t, containers, 2)

	c := containers[1]
	assert.Equal(t, string(apicommon.HostProfiler), c.Name)
	assert.NotNil(t, c.SecurityContext)
	assert.Nil(t, c.SecurityContext.Privileged, "host-profiler should not run as privileged")
	assert.NotNil(t, c.SecurityContext.SeccompProfile)
	assert.Equal(t, corev1.SeccompProfileTypeLocalhost, c.SecurityContext.SeccompProfile.Type)
	assert.Equal(t, common.HostProfilerSeccompProfileName, *c.SecurityContext.SeccompProfile.LocalhostProfile)
	assert.NotNil(t, c.SecurityContext.Capabilities)
	assert.Contains(t, c.SecurityContext.Capabilities.Drop, corev1.Capability("ALL"), "host-profiler should drop all capabilities by default")
}

func TestInitHostProfilerSeccompSetupContainer(t *testing.T) {
	dda := &metav1.ObjectMeta{Name: "foo", Namespace: "default", Labels: map[string]string{}}

	initContainers := initContainers(dda, []apicommon.AgentContainerName{apicommon.HostProfiler})

	var setupContainer *corev1.Container
	for i := range initContainers {
		if initContainers[i].Name == "host-profiler-seccomp-setup" {
			setupContainer = &initContainers[i]
			break
		}
	}
	assert.NotNil(t, setupContainer, "host-profiler-seccomp-setup init container should exist")
	dst := fmt.Sprintf("%s/%s", common.SeccompRootVolumePath, common.HostProfilerSeccompProfileName)
	assert.Contains(t, setupContainer.Command, dst, "cp command should copy to the kubelet seccomp path")

	mountNames := map[string]bool{}
	for _, m := range setupContainer.VolumeMounts {
		mountNames[m.Name] = true
	}
	assert.True(t, mountNames[common.HostProfilerSecurityVolumeName])
	assert.True(t, mountNames[common.SeccompRootVolumeName])
}

func TestDefaultCapabilitiesForHostProfiler(t *testing.T) {
	caps := DefaultCapabilitiesForHostProfiler()

	capSet := make(map[corev1.Capability]bool)
	for _, c := range caps {
		capSet[c] = true
	}

	assert.False(t, capSet["SYS_ADMIN"], "host-profiler should not have SYS_ADMIN")
	assert.True(t, capSet["BPF"], "host-profiler requires BPF for eBPF programs")
	assert.True(t, capSet["PERFMON"], "host-profiler requires PERFMON for perf_event_open")
	assert.True(t, capSet["CHECKPOINT_RESTORE"], "host-profiler requires CHECKPOINT_RESTORE for /proc/<pid>/map_files access")
	assert.True(t, capSet["SYS_PTRACE"], "host-profiler requires SYS_PTRACE for process tracing")
}

func TestDefaultSyscallsForHostProfiler(t *testing.T) {
	syscalls := DefaultSyscallsForHostProfiler()

	syscallSet := make(map[string]bool)
	for _, s := range syscalls {
		syscallSet[s] = true
	}

	assert.NotEmpty(t, syscalls)
	assert.True(t, syscallSet["bpf"], "host-profiler requires bpf syscall")
	assert.True(t, syscallSet["perf_event_open"], "host-profiler requires perf_event_open")
	assert.True(t, syscallSet["openat"], "host-profiler requires openat for /proc/<pid>/map_files access")
	assert.True(t, syscallSet["read"], "host-profiler requires read")
	assert.False(t, syscallSet["ptrace"], "host-profiler should not need ptrace syscall (uses /proc instead)")
}

func TestDefaultSeccompConfigDataForHostProfiler(t *testing.T) {
	data := DefaultSeccompConfigDataForHostProfiler()

	assert.Contains(t, data, common.HostProfilerSeccompKey, "seccomp configmap should have the host-profiler key")

	profile := data[common.HostProfilerSeccompKey]
	assert.Contains(t, profile, "SCMP_ACT_ERRNO", "default action should be SCMP_ACT_ERRNO")
	assert.Contains(t, profile, "SCMP_ACT_ALLOW", "syscalls should be allowed")
	assert.Contains(t, profile, `"bpf"`, "bpf syscall must be in the profile")
	assert.Contains(t, profile, `"perf_event_open"`, "perf_event_open syscall must be in the profile")
}

func TestPrivateActionRunnerContainer(t *testing.T) {
	dda := &metav1.ObjectMeta{
		Name:      "test-dda",
		Namespace: "default",
	}

	containers := agentOptimizedContainers(dda, []apicommon.AgentContainerName{
		apicommon.CoreAgentContainerName,
		apicommon.PrivateActionRunnerContainerName,
	})

	assert.Len(t, containers, 2)

	parContainer := containers[1]
	assert.Equal(t, string(apicommon.PrivateActionRunnerContainerName), parContainer.Name)
	assert.Equal(t, agentImage(), parContainer.Image)
	assert.Equal(t, []string{
		"/opt/datadog-agent/embedded/bin/privateactionrunner",
		"run",
		"-c=/etc/datadog-agent/datadog.yaml",
		"-E=/etc/datadog-agent/privateactionrunner.yaml",
	}, parContainer.Command)

	assert.True(t, *parContainer.SecurityContext.ReadOnlyRootFilesystem)
	mountNames := make(map[string]bool)
	for _, m := range parContainer.VolumeMounts {
		mountNames[m.Name] = true
	}
	assert.True(t, mountNames[common.LogDatadogVolumeName])
	assert.True(t, mountNames[common.AuthVolumeName])
	assert.True(t, mountNames[common.ConfigVolumeName])
	assert.True(t, mountNames[common.DogstatsdSocketVolumeName])
	assert.True(t, mountNames[common.TmpVolumeName])
}
