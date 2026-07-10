// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
)

// testDDA returns a minimal metav1.Object suitable for builder calls.
func testDDA(name string) metav1.Object {
	return &metav1.ObjectMeta{Name: name, Namespace: "datadog"}
}

// --- overrideAuthTokenPath ---

func TestOverrideAuthTokenPath_ReplacesExisting(t *testing.T) {
	envs := []corev1.EnvVar{
		{Name: "DD_SITE", Value: "datadoghq.com"},
		{Name: common.DDAuthTokenFilePath, Value: "/etc/datadog-agent/auth/token"},
	}
	original := make([]corev1.EnvVar, len(envs))
	copy(original, envs)

	out := overrideAuthTokenPath(envs)

	// Input slice is not mutated.
	assert.Equal(t, original, envs, "overrideAuthTokenPath must not mutate the input slice")

	val, found := findEnvVar(out, common.DDAuthTokenFilePath)
	assert.True(t, found)
	assert.Equal(t, windowsAuthTokenFilePath, val)
}

func TestOverrideAuthTokenPath_AppendsWhenAbsent(t *testing.T) {
	envs := []corev1.EnvVar{{Name: "DD_SITE", Value: "datadoghq.com"}}
	out := overrideAuthTokenPath(envs)

	val, found := findEnvVar(out, common.DDAuthTokenFilePath)
	assert.True(t, found)
	assert.Equal(t, windowsAuthTokenFilePath, val)
}

// --- StripLinuxOnlySettings ---

// --- StripLinuxOnlySettingsFromTemplate (allowlist model) ---

// Only core/trace/process agent containers survive; everything else (including
// fips-proxy, otel-agent, system-probe, etc.) is removed regardless of denylist.
func TestStripLinuxOnly_ContainerAllowlist(t *testing.T) {
	tmpl := &corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "agent"},
				{Name: "trace-agent"},
				{Name: "process-agent"},
				{Name: string(apicommon.SystemProbeContainerName)},
				{Name: string(apicommon.SecurityAgentContainerName)},
				{Name: string(apicommon.HostProfiler)},
				{Name: string(apicommon.FlightRecorderContainerName)},
				{Name: "fips-proxy"},
				{Name: "otel-agent"},
			},
		},
	}
	StripLinuxOnlySettingsFromTemplate(tmpl)

	names := containerNames(tmpl.Spec.Containers)
	assert.ElementsMatch(t, []string{"agent", "trace-agent", "process-agent"}, names,
		"only core/trace/process agent should survive; fips-proxy/otel/system-probe/etc. removed")
}

// Init containers: all are stripped on Windows (the allowlist is empty — the Windows agent
// needs no init container).
func TestStripLinuxOnly_InitContainerAllowlist(t *testing.T) {
	tmpl := &corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			InitContainers: []corev1.Container{
				{Name: string(apicommon.SeccompSetupContainerName)},
				{Name: string(apicommon.InitVolumeContainerName)},
				{Name: string(apicommon.InitConfigContainerName)},
			},
		},
	}
	StripLinuxOnlySettingsFromTemplate(tmpl)

	assert.Empty(t, tmpl.Spec.InitContainers, "all Linux init containers must be stripped")
}

// Any mount with a Linux absolute path is stripped from surviving containers;
// Windows (C:/...) mounts are kept. Covers the reviewer's verified leaks:
// hostPath / (SBOM), emptyDir /var/run/sysprobe (NPM/USM), /var/run/datadog/flightrecorder.
func TestStripLinuxOnly_StripsLinuxMountPaths(t *testing.T) {
	tmpl := &corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "agent",
					VolumeMounts: []corev1.VolumeMount{
						{Name: "hostroot", MountPath: "/host/root"},
						{Name: "sysprobe-socket", MountPath: "/var/run/sysprobe"},
						{Name: "runtimesocket", MountPath: "/run"},
						{Name: "win-datadog-config", MountPath: "C:/ProgramData/Datadog"},
					},
				},
			},
			Volumes: []corev1.Volume{
				{Name: "hostroot", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/"}}},
				{Name: "sysprobe-socket", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
				{Name: "runtimesocket", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/run"}}},
				{Name: "win-datadog-config", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
			},
		},
	}
	StripLinuxOnlySettingsFromTemplate(tmpl)

	// Only the Windows mount survives on the container.
	require.Len(t, tmpl.Spec.Containers, 1)
	require.Len(t, tmpl.Spec.Containers[0].VolumeMounts, 1)
	assert.Equal(t, "win-datadog-config", tmpl.Spec.Containers[0].VolumeMounts[0].Name)

	// Only the referenced Windows volume survives; hostPath / and Linux emptyDirs are dropped.
	require.Len(t, tmpl.Spec.Volumes, 1)
	assert.Equal(t, "win-datadog-config", tmpl.Spec.Volumes[0].Name)
}

// Linux-only pod-level security settings (SELinux/sysctls/fsGroup/runAsUser/...) and
// namespace-sharing fields must be cleared, while the Windows-compatible WindowsOptions
// (runAsUserName / GMSA) is preserved. Guards against a nodeAgent securityContext
// override producing a Windows-rejected pod.
func TestStripLinuxOnly_PodSecurityContext(t *testing.T) {
	runAsNonRoot := true
	fsGroup := int64(1000)
	shareNS := true
	userName := "ContainerUser"
	tmpl := &corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			HostPID:               true,
			HostIPC:               true,
			ShareProcessNamespace: &shareNS,
			SecurityContext: &corev1.PodSecurityContext{
				RunAsNonRoot:   &runAsNonRoot,
				FSGroup:        &fsGroup,
				SELinuxOptions: &corev1.SELinuxOptions{Level: "s0"},
				Sysctls:        []corev1.Sysctl{{Name: "net.core.somaxconn", Value: "1024"}},
				WindowsOptions: &corev1.WindowsSecurityContextOptions{RunAsUserName: &userName},
			},
			Containers: []corev1.Container{{Name: "agent"}},
		},
	}
	StripLinuxOnlySettingsFromTemplate(tmpl)

	assert.False(t, tmpl.Spec.HostPID, "HostPID must be cleared")
	assert.False(t, tmpl.Spec.HostIPC, "HostIPC must be cleared")
	assert.Nil(t, tmpl.Spec.ShareProcessNamespace, "ShareProcessNamespace must be cleared")
	require.NotNil(t, tmpl.Spec.SecurityContext, "WindowsOptions must be preserved")
	assert.Nil(t, tmpl.Spec.SecurityContext.RunAsNonRoot, "Linux RunAsNonRoot must be cleared")
	assert.Nil(t, tmpl.Spec.SecurityContext.FSGroup, "Linux FSGroup must be cleared")
	assert.Nil(t, tmpl.Spec.SecurityContext.SELinuxOptions, "SELinuxOptions must be cleared")
	assert.Nil(t, tmpl.Spec.SecurityContext.Sysctls, "Sysctls must be cleared")
	require.NotNil(t, tmpl.Spec.SecurityContext.WindowsOptions)
	assert.Equal(t, "ContainerUser", *tmpl.Spec.SecurityContext.WindowsOptions.RunAsUserName)
}

// A pod SecurityContext with only Linux fields collapses to nil after stripping.
func TestStripLinuxOnly_PodSecurityContext_NilWhenOnlyLinux(t *testing.T) {
	fsGroup := int64(1000)
	tmpl := &corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			SecurityContext: &corev1.PodSecurityContext{FSGroup: &fsGroup},
			Containers:      []corev1.Container{{Name: "agent"}},
		},
	}
	StripLinuxOnlySettingsFromTemplate(tmpl)
	assert.Nil(t, tmpl.Spec.SecurityContext, "pod SecurityContext with only Linux fields must become nil")
}

// Volumes not referenced by any surviving container are dropped (even emptyDirs).
func TestStripLinuxOnly_DropsUnreferencedVolumes(t *testing.T) {
	tmpl := &corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "agent", VolumeMounts: []corev1.VolumeMount{{Name: "win-datadog-config", MountPath: "C:/ProgramData/Datadog"}}},
				// system-probe gets removed; its emptyDir should then be unreferenced and dropped.
				{Name: string(apicommon.SystemProbeContainerName), VolumeMounts: []corev1.VolumeMount{{Name: "sysprobe-only", MountPath: "C:/foo"}}},
			},
			Volumes: []corev1.Volume{
				{Name: "win-datadog-config", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
				{Name: "sysprobe-only", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
			},
		},
	}
	StripLinuxOnlySettingsFromTemplate(tmpl)

	require.Len(t, tmpl.Spec.Volumes, 1)
	assert.Equal(t, "win-datadog-config", tmpl.Spec.Volumes[0].Name)
}

func TestStripLinuxOnly_ClearsLinuxSecurityContext(t *testing.T) {
	tmpl := &corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "agent",
					SecurityContext: &corev1.SecurityContext{
						Capabilities:           &corev1.Capabilities{Add: []corev1.Capability{"SYS_ADMIN"}},
						SeccompProfile:         &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeLocalhost},
						SELinuxOptions:         &corev1.SELinuxOptions{Level: "s0"},
						ReadOnlyRootFilesystem: ptr.To(true),
					},
				},
			},
		},
	}
	StripLinuxOnlySettingsFromTemplate(tmpl)

	require.Len(t, tmpl.Spec.Containers, 1)
	assert.Nil(t, tmpl.Spec.Containers[0].SecurityContext,
		"SecurityContext should be nil after stripping all Linux fields")
}

func TestStripLinuxOnly_RemovesAllAppArmorAnnotations(t *testing.T) {
	tmpl := &corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"container.apparmor.security.beta.kubernetes.io/system-probe": "localhost/system-probe",
				"container.apparmor.security.beta.kubernetes.io/agent":        "runtime/default",
				"ad.datadoghq.com/tags":                                       "{}",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "agent"},
				{Name: string(apicommon.SystemProbeContainerName)},
			},
		},
	}
	StripLinuxOnlySettingsFromTemplate(tmpl)

	// AppArmor is Linux-only: ALL container.apparmor annotations are removed, even for the
	// surviving agent container.
	_, hasSystemProbe := tmpl.Annotations["container.apparmor.security.beta.kubernetes.io/system-probe"]
	assert.False(t, hasSystemProbe, "AppArmor annotation for stripped system-probe must be removed")
	_, hasAgent := tmpl.Annotations["container.apparmor.security.beta.kubernetes.io/agent"]
	assert.False(t, hasAgent, "AppArmor annotation for surviving agent must ALSO be removed (Linux-only)")
	_, hasTags := tmpl.Annotations["ad.datadoghq.com/tags"]
	assert.True(t, hasTags, "unrelated annotation must be kept")
}

// Windows-incompatible env vars must be removed: Unix-domain-socket config (anything with
// "SOCKET" in the name) and the Linux-only global env vars (DOCKER_HOST, DD_KUBELET_CLIENT_CA,
// DD_VSOCK_ADDR). The agent then falls back to TCP/UDP. Non-matching env vars are preserved.
func TestStripLinuxOnly_StripsWindowsIncompatibleEnvVars(t *testing.T) {
	tmpl := &corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "agent",
					Env: []corev1.EnvVar{
						{Name: "DD_APM_RECEIVER_SOCKET", Value: "/var/run/datadog/apm.socket"},
						{Name: "DD_DOGSTATSD_SOCKET", Value: "/var/run/datadog/dsd.socket"},
						{Name: "DD_DOGSTATSD_HOST_SOCKET_PATH", Value: "/var/run/datadog"},
						{Name: "DD_CRI_SOCKET_PATH", Value: "/var/run/containerd/containerd.sock"},
						{Name: "DOCKER_HOST", Value: "unix:///host/var/run/docker.sock"},
						{Name: "DD_KUBELET_CLIENT_CA", Value: "/var/run/host-kubelet-ca.crt"},
						{Name: "DD_VSOCK_ADDR", Value: "vsock://..."},
						// Server-side DogStatsD origin detection needs the (stripped) hostPID → drop it.
						{Name: "DD_DOGSTATSD_ORIGIN_DETECTION", Value: "true"},
						{Name: "DD_APM_ENABLED", Value: "true"},
						{Name: "DD_API_KEY", Value: "x"},
						// Client-side origin detection needs no hostPID → preserved.
						{Name: "DD_DOGSTATSD_ORIGIN_DETECTION_CLIENT", Value: "true"},
					},
				},
			},
		},
	}
	StripLinuxOnlySettingsFromTemplate(tmpl)

	require.Len(t, tmpl.Spec.Containers, 1)
	names := make(map[string]bool)
	for _, e := range tmpl.Spec.Containers[0].Env {
		names[e.Name] = true
	}
	for _, stripped := range []string{
		"DD_APM_RECEIVER_SOCKET", "DD_DOGSTATSD_SOCKET", "DD_DOGSTATSD_HOST_SOCKET_PATH",
		"DD_CRI_SOCKET_PATH", "DOCKER_HOST", "DD_KUBELET_CLIENT_CA", "DD_VSOCK_ADDR",
		"DD_DOGSTATSD_ORIGIN_DETECTION",
	} {
		assert.False(t, names[stripped], "%s must be stripped on Windows", stripped)
	}
	assert.True(t, names["DD_APM_ENABLED"], "non-Linux env must be preserved")
	assert.True(t, names["DD_API_KEY"], "non-Linux env must be preserved")
	assert.True(t, names["DD_DOGSTATSD_ORIGIN_DETECTION_CLIENT"], "client origin detection must be preserved")
}

func TestStripLinuxOnly_PreservesEnvVars(t *testing.T) {
	tmpl := &corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "agent",
					Env: []corev1.EnvVar{
						{Name: "DD_APM_ENABLED", Value: "true"},
						{Name: "DD_LOGS_ENABLED", Value: "true"},
						{Name: "DD_TAGS", Value: "env:prod"},
					},
				},
			},
		},
	}
	StripLinuxOnlySettingsFromTemplate(tmpl)

	require.Len(t, tmpl.Spec.Containers, 1)
	require.Len(t, tmpl.Spec.Containers[0].Env, 3)
}

// --- helpers ---

func containerNames(containers []corev1.Container) []string {
	names := make([]string, len(containers))
	for i, c := range containers {
		names[i] = c.Name
	}
	return names
}

func findEnvVar(envs []corev1.EnvVar, name string) (string, bool) {
	for _, e := range envs {
		if e.Name == name {
			return e.Value, true
		}
	}
	return "", false
}

// EnsureWindowsServercoreImage must re-add the -servercore suffix that the generic image
// override drops (e.g. a user pinning image.tag=7.81.0), while leaving images that already
// carry servercore untouched.
func TestEnsureWindowsServercoreImage(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"gcr.io/datadoghq/agent:7.81.0", "gcr.io/datadoghq/agent:7.81.0-servercore"},
		{"gcr.io/datadoghq/agent:7.81.0-servercore", "gcr.io/datadoghq/agent:7.81.0-servercore"},
		{"gcr.io/datadoghq/agent:7.81.0-servercore-ltsc2022", "gcr.io/datadoghq/agent:7.81.0-servercore-ltsc2022"},
		{"localhost:5000/agent:7.81.0", "localhost:5000/agent:7.81.0-servercore"}, // registry port not mistaken for tag
		// Custom (non-"agent") images are the user's responsibility — left untouched.
		{"registry.local/custom-agent:win2022", "registry.local/custom-agent:win2022"},
	}
	for _, c := range cases {
		spec := &corev1.PodSpec{
			Containers:     []corev1.Container{{Name: "agent", Image: c.in}},
			InitContainers: []corev1.Container{{Name: "init-config-windows", Image: c.in}},
		}
		EnsureWindowsServercoreImage(spec)
		assert.Equal(t, c.want, spec.Containers[0].Image, "container image for %q", c.in)
		assert.Equal(t, c.want, spec.InitContainers[0].Image, "init image for %q", c.in)
	}
}

// AddWindowsLogCollectionVolumes adds the two default Windows host-log hostPath volumes + mounts
// on the core agent, which survive the strip because they use C:/ paths.
func TestAddWindowsLogCollectionVolumes(t *testing.T) {
	spec := &corev1.PodSpec{
		Containers: []corev1.Container{
			{Name: string(apicommon.CoreAgentContainerName)},
			{Name: string(apicommon.TraceAgentContainerName)},
		},
	}
	AddWindowsLogCollectionVolumes(spec, WindowsLogPaths{}) // empty → Windows defaults

	// 2 hostPath volumes with the expected default Windows paths. The container-log store
	// (C:/ProgramData) is NOT mounted by default — it would overlap the config dir and pod logs
	// are regular files under C:/var/log/pods.
	paths := map[string]string{}
	for _, v := range spec.Volumes {
		require.NotNil(t, v.HostPath, "volume %q must be hostPath", v.Name)
		paths[v.Name] = v.HostPath.Path
	}
	assert.Equal(t, "C:/var/log", paths[windowsPointerVolumeName])
	assert.Equal(t, "C:/var/log/pods", paths[windowsPodLogsVolumeName])
	_, hasContainerLog := paths[windowsContainerLogVolumeName]
	assert.False(t, hasContainerLog, "container-log (C:/ProgramData) volume must NOT be mounted by default")

	// Mounts land on the core agent only, at the same Windows paths.
	var core, trace corev1.Container
	for _, c := range spec.Containers {
		if c.Name == string(apicommon.CoreAgentContainerName) {
			core = c
		}
		if c.Name == string(apicommon.TraceAgentContainerName) {
			trace = c
		}
	}
	assert.Len(t, core.VolumeMounts, 2, "core agent gets the 2 default log mounts")
	assert.Empty(t, trace.VolumeMounts, "trace agent gets no log mounts")
	for _, m := range core.VolumeMounts {
		assert.True(t, strings.HasPrefix(m.MountPath, "C:/"), "mount %q must be a Windows path", m.MountPath)
		assert.NotEqual(t, windowsProgramDataPath, m.MountPath, "no C:/ProgramData mount (would overlap config dir)")
	}

	// And they survive the strip (Windows paths, not Linux).
	tmpl := &corev1.PodTemplateSpec{Spec: *spec}
	StripLinuxOnlySettingsFromTemplate(tmpl)
	surviving := 0
	for _, v := range tmpl.Spec.Volumes {
		if v.Name == windowsPointerVolumeName || v.Name == windowsPodLogsVolumeName {
			surviving++
		}
	}
	assert.Equal(t, 2, surviving, "Windows log volumes must survive the strip")
}

// Configured logCollection paths must override the Windows defaults.
func TestAddWindowsLogCollectionVolumes_CustomPaths(t *testing.T) {
	spec := &corev1.PodSpec{
		Containers: []corev1.Container{{Name: string(apicommon.CoreAgentContainerName)}},
	}
	AddWindowsLogCollectionVolumes(spec, WindowsLogPaths{
		TempStoragePath:   "D:/dd/run",
		PodLogsPath:       "D:/logs/pods",
		ContainerLogsPath: "D:/containerd",
	})

	paths := map[string]string{}
	for _, v := range spec.Volumes {
		paths[v.Name] = v.HostPath.Path
	}
	// Custom paths set the HOST path...
	assert.Equal(t, "D:/dd/run", paths[windowsPointerVolumeName])
	assert.Equal(t, "D:/logs/pods", paths[windowsPodLogsVolumeName])
	// An explicit, non-overlapping Windows container-log path IS mounted (D:/containerd).
	assert.Equal(t, "D:/containerd", paths[windowsContainerLogVolumeName])

	// The pointer + pod-log mounts use the Agent's standard Windows paths (Linux model), while the
	// explicit container-log store is mounted at its own configured path (it has no standard path).
	mountPaths := map[string]string{}
	for _, m := range spec.Containers[0].VolumeMounts {
		mountPaths[m.Name] = m.MountPath
	}
	assert.Equal(t, windowsVarLogPath, mountPaths[windowsPointerVolumeName])
	assert.Equal(t, windowsPodLogsPath, mountPaths[windowsPodLogsVolumeName])
	assert.Equal(t, "D:/containerd", mountPaths[windowsContainerLogVolumeName])
}

// An explicit container-log path that OVERLAPS the config dir must be skipped (it would re-shadow
// the seeded config via a non-guaranteed Windows nested mount) AND reported back to the caller.
func TestAddWindowsLogCollectionVolumes_OverlappingContainerPathSkipped(t *testing.T) {
	for _, overlap := range []string{"C:/ProgramData", "C:/ProgramData/Datadog", "C:/ProgramData/Datadog/conf.d", `C:\ProgramData`} {
		spec := &corev1.PodSpec{Containers: []corev1.Container{{Name: string(apicommon.CoreAgentContainerName)}}}
		skipped := AddWindowsLogCollectionVolumes(spec, WindowsLogPaths{ContainerLogsPath: overlap})
		for _, v := range spec.Volumes {
			assert.NotEqual(t, windowsContainerLogVolumeName, v.Name,
				"overlapping container-log path %q must not be mounted", overlap)
		}
		assert.Equal(t, overlap, skipped, "overlapping container-log path must be reported so the caller can surface it")
	}
}

// A non-overlapping (sibling) container-log path — the supported way to serve symlink-based pod
// logs — IS mounted and NOT reported as skipped.
func TestAddWindowsLogCollectionVolumes_SiblingContainerPathMounted(t *testing.T) {
	spec := &corev1.PodSpec{Containers: []corev1.Container{{Name: string(apicommon.CoreAgentContainerName)}}}
	skipped := AddWindowsLogCollectionVolumes(spec, WindowsLogPaths{ContainerLogsPath: "C:/ProgramData/docker/containers"})
	assert.Empty(t, skipped, "a non-overlapping container-log path must not be reported as skipped")
	mounted := false
	for _, v := range spec.Volumes {
		if v.Name == windowsContainerLogVolumeName {
			mounted = true
			assert.Equal(t, "C:/ProgramData/docker/containers", v.HostPath.Path)
		}
	}
	assert.True(t, mounted, "a non-overlapping sibling container-log path must be mounted")
}

// Linux-absolute paths (the logCollection feature's defaults, e.g. /var/log/pods) must be
// treated as unset so the Windows defaults apply — otherwise Linux hostPaths land on Windows.
func TestAddWindowsLogCollectionVolumes_LinuxDefaultsTreatedAsUnset(t *testing.T) {
	spec := &corev1.PodSpec{
		Containers: []corev1.Container{{Name: string(apicommon.CoreAgentContainerName)}},
	}
	AddWindowsLogCollectionVolumes(spec, WindowsLogPaths{
		TempStoragePath:   "/var/lib/datadog-agent/logs",
		PodLogsPath:       "/var/log/pods",
		ContainerLogsPath: "/var/lib/docker/containers",
	})

	for _, v := range spec.Volumes {
		require.NotNil(t, v.HostPath)
		assert.False(t, strings.HasPrefix(v.HostPath.Path, "/"),
			"Linux default %q must be replaced by a Windows path", v.HostPath.Path)
	}
	paths := map[string]string{}
	for _, v := range spec.Volumes {
		paths[v.Name] = v.HostPath.Path
	}
	assert.Equal(t, "C:/var/log", paths[windowsPointerVolumeName])
	assert.Equal(t, "C:/var/log/pods", paths[windowsPodLogsVolumeName])
	// A Linux container-log path is treated as unset → not mounted (no C:/ProgramData default).
	_, hasContainerLog := paths[windowsContainerLogVolumeName]
	assert.False(t, hasContainerLog, "Linux container-log path must be treated as unset, not mounted")
}

// --- ApplyWindowsPodTransformation ---

// End-to-end: a Linux-built node-agent template (extra containers, Linux mounts/volumes,
// Linux init, securityContext) is transformed into a valid Windows pod.
func TestApplyWindowsPodTransformation(t *testing.T) {
	tmpl := &corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			HostPID: true,
			InitContainers: []corev1.Container{
				{Name: string(apicommon.InitConfigContainerName), Command: []string{"bash"}},
			},
			Containers: []corev1.Container{
				{
					Name:            string(apicommon.CoreAgentContainerName),
					Image:           "gcr.io/datadoghq/agent:7.80.2",
					ImagePullPolicy: corev1.PullAlways,
					VolumeMounts: []corev1.VolumeMount{
						{Name: "logpodpath", MountPath: "/var/log/pods"},
						{Name: "auth", MountPath: "/etc/datadog-agent/auth"},
					},
					Env:             []corev1.EnvVar{{Name: "DD_DOGSTATSD_SOCKET", Value: "/var/run/dsd.socket"}},
					SecurityContext: &corev1.SecurityContext{ReadOnlyRootFilesystem: ptr.To(true)},
				},
				{Name: string(apicommon.TraceAgentContainerName), Image: "gcr.io/datadoghq/agent:7.80.2"},
				{Name: string(apicommon.SystemProbeContainerName), Image: "gcr.io/datadoghq/agent:7.80.2"},
			},
			Volumes: []corev1.Volume{
				{Name: "logpodpath", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/var/log/pods"}}},
				{Name: "auth", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
			},
		},
	}

	ApplyWindowsPodTransformation(tmpl, testDDA("dd"), true, WindowsLogPaths{})
	spec := tmpl.Spec

	// Only core/trace survive (system-probe removed); Linux hostPID cleared.
	assert.ElementsMatch(t, []string{"agent", "trace-agent"}, containerNames(spec.Containers))
	assert.False(t, spec.HostPID)

	// The config-seed init container is added (copies the image's conf.d + datadog.yaml into the
	// shared emptyDir); servercore image on all containers, including the init container.
	require.Len(t, spec.InitContainers, 1, "config-seed init container present")
	assert.Equal(t, "init-config-windows", spec.InitContainers[0].Name)
	assert.Contains(t, spec.InitContainers[0].Image, "-servercore", "init container image must be servercore")
	initMount := spec.InitContainers[0].VolumeMounts[0]
	assert.Equal(t, windowsConfigVolumeName, initMount.Name)
	assert.Equal(t, windowsConfigInitPath, initMount.MountPath,
		"init must stage into the emptyDir at a path other than the config dir, so it can read the image conf.d")
	// Init container inherits the main agent container's pull policy (it uses the same image).
	assert.Equal(t, corev1.PullAlways, spec.InitContainers[0].ImagePullPolicy,
		"init container must inherit the agent's ImagePullPolicy")
	for _, c := range spec.Containers {
		assert.Contains(t, c.Image, "-servercore", "container %s image must be servercore", c.Name)
	}

	// No Linux hostPaths remain; the shared config + Windows log volumes are present.
	for _, v := range spec.Volumes {
		if v.HostPath != nil {
			assert.False(t, strings.HasPrefix(v.HostPath.Path, "/"), "no Linux hostPath, got %q", v.HostPath.Path)
		}
	}
	volNames := map[string]bool{}
	for _, v := range spec.Volumes {
		volNames[v.Name] = true
	}
	assert.True(t, volNames[windowsConfigVolumeName], "shared config volume present")
	assert.True(t, volNames[windowsPodLogsVolumeName], "windows log volume present")

	// Core agent: Windows command, config mount, non-local DogStatsD, no Unix-socket env.
	core := findContainer(spec.Containers, "agent")
	require.NotNil(t, core)
	assert.Equal(t, []string{"agent", "run"}, core.Command)
	assert.Nil(t, core.SecurityContext, "Linux securityContext cleared")
	if v, ok := findEnvVar(core.Env, "DD_DOGSTATSD_NON_LOCAL_TRAFFIC"); assert.True(t, ok) {
		assert.Equal(t, "true", v)
	}
	_, hasSocket := findEnvVar(core.Env, "DD_DOGSTATSD_SOCKET")
	assert.False(t, hasSocket, "Unix-socket env stripped")

	// The shared config volume mounts at the whole config dir C:/ProgramData/Datadog, seeded by the
	// init container with conf.d + datadog.yaml.
	var sharedMount string
	for _, m := range core.VolumeMounts {
		if m.Name == windowsConfigVolumeName {
			sharedMount = m.MountPath
		}
	}
	assert.Equal(t, windowsDatadogConfigPath, sharedMount, "shared config vol must mount at the whole config dir")
	// CRITICAL anti-shadowing invariant: with log collection enabled, NO host mount may overlap the
	// config dir. In particular C:/ProgramData (the config dir's parent, the old default container-
	// log mount) must NOT be mounted — that overlap relied on non-guaranteed Windows nested-mount
	// overlay and could re-shadow the seeded config (the original bug).
	for _, m := range core.VolumeMounts {
		assert.NotEqual(t, windowsProgramDataPath, m.MountPath,
			"C:/ProgramData must not be mounted (would overlap config dir C:/ProgramData/Datadog)")
	}

	// nodeSelector + Windows taint toleration.
	assert.Equal(t, "windows", spec.NodeSelector["kubernetes.io/os"])
	found := false
	for _, tol := range spec.Tolerations {
		if tol.Key == "node.kubernetes.io/os" && tol.Value == "windows" {
			found = true
		}
	}
	assert.True(t, found, "windows taint toleration added")
}

func findContainer(containers []corev1.Container, name string) *corev1.Container {
	for i := range containers {
		if containers[i].Name == name {
			return &containers[i]
		}
	}
	return nil
}

// Single-container strategy on Windows must yield a valid pod: the consolidated
// unprivileged-single-agent container survives (as core) rather than being stripped.
func TestApplyWindowsPodTransformation_SingleContainerStrategy(t *testing.T) {
	tmpl := &corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: string(apicommon.UnprivilegedSingleAgentContainerName), Image: "gcr.io/datadoghq/agent:7.80.2"},
			},
		},
	}
	ApplyWindowsPodTransformation(tmpl, testDDA("dd"), false, WindowsLogPaths{})

	require.Len(t, tmpl.Spec.Containers, 1, "single agent container must survive (not be stripped)")
	c := tmpl.Spec.Containers[0]
	assert.Equal(t, string(apicommon.UnprivilegedSingleAgentContainerName), c.Name)
	assert.Equal(t, []string{"agent", "run"}, c.Command)
	assert.Contains(t, c.Image, "-servercore")
	if v, ok := findEnvVar(c.Env, "DD_APM_NON_LOCAL_TRAFFIC"); assert.True(t, ok) {
		assert.Equal(t, "true", v)
	}
}

// servercore coercion must place -servercore correctly for JMX and not fabricate broken
// tags for -full/-fips (which have no Windows image variant).
func TestEnsureServercoreTag_Flavors(t *testing.T) {
	cases := map[string]string{
		"gcr.io/datadoghq/agent:7.80.2":      "gcr.io/datadoghq/agent:7.80.2-servercore",
		"gcr.io/datadoghq/agent:7.80.2-jmx":  "gcr.io/datadoghq/agent:7.80.2-servercore-jmx",
		"gcr.io/datadoghq/agent:7.80.2-full": "gcr.io/datadoghq/agent:7.80.2-servercore",
		"gcr.io/datadoghq/agent:7.80.2-fips": "gcr.io/datadoghq/agent:7.80.2-servercore",
		// already servercore: unchanged
		"gcr.io/datadoghq/agent:7.80.2-servercore": "gcr.io/datadoghq/agent:7.80.2-servercore",
		// registry port not mistaken for tag; custom image untouched
		"localhost:5000/custom-agent:win2022": "localhost:5000/custom-agent:win2022",
	}
	for in, want := range cases {
		assert.Equal(t, want, ensureServercoreTag(in), "ensureServercoreTag(%q)", in)
	}
}

// FIPS proxy is Linux-only and stripped on Windows; its DD_FIPS_* env must be removed too,
// otherwise the agent routes intake at a proxy that no longer exists.
func TestApplyWindowsPodTransformation_StripsFIPSEnv(t *testing.T) {
	tmpl := &corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name:  string(apicommon.CoreAgentContainerName),
				Image: "gcr.io/datadoghq/agent:7.80.2",
				Env: []corev1.EnvVar{
					{Name: "DD_FIPS_ENABLED", Value: "true"},
					{Name: "DD_FIPS_PORT_RANGE_START", Value: "9803"},
					{Name: "DD_FIPS_LOCAL_ADDRESS", Value: "127.0.0.1"},
					{Name: "DD_API_KEY", Value: "x"},
				},
			}},
		},
	}
	ApplyWindowsPodTransformation(tmpl, testDDA("dd"), false, WindowsLogPaths{})
	c := findContainer(tmpl.Spec.Containers, string(apicommon.CoreAgentContainerName))
	require.NotNil(t, c)
	for _, n := range []string{"DD_FIPS_ENABLED", "DD_FIPS_PORT_RANGE_START", "DD_FIPS_LOCAL_ADDRESS"} {
		_, ok := findEnvVar(c.Env, n)
		assert.False(t, ok, "%s must be stripped on Windows", n)
	}
	_, ok := findEnvVar(c.Env, "DD_API_KEY")
	assert.True(t, ok, "non-FIPS env preserved")
}

// Container-level Linux securityContext fields (runAsUser, privileged, …) set via override must
// be dropped on Windows; only WindowsOptions survives.
func TestStripLinuxSecurityContext_WindowsOptionsOnly(t *testing.T) {
	user := "ContainerUser"
	sc := &corev1.SecurityContext{
		RunAsUser:                ptr.To[int64](0),
		RunAsNonRoot:             ptr.To(true),
		Privileged:               ptr.To(true),
		AllowPrivilegeEscalation: ptr.To(true),
		WindowsOptions:           &corev1.WindowsSecurityContextOptions{RunAsUserName: &user},
	}
	got := stripLinuxSecurityContext(sc)
	require.NotNil(t, got)
	assert.Nil(t, got.RunAsUser)
	assert.Nil(t, got.RunAsNonRoot)
	assert.Nil(t, got.Privileged)
	assert.Nil(t, got.AllowPrivilegeEscalation)
	require.NotNil(t, got.WindowsOptions)
	assert.Equal(t, "ContainerUser", *got.WindowsOptions.RunAsUserName)

	// Only-Linux context collapses to nil.
	assert.Nil(t, stripLinuxSecurityContext(&corev1.SecurityContext{Privileged: ptr.To(true)}))
}

// hostNetwork (Linux host networking) is not supported by ServerCore Windows containers and
// must be cleared even when set via an override.
func TestStripLinuxOnly_ClearsHostNetwork(t *testing.T) {
	tmpl := &corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			HostNetwork: true,
			Containers:  []corev1.Container{{Name: "agent"}},
		},
	}
	StripLinuxOnlySettingsFromTemplate(tmpl)
	assert.False(t, tmpl.Spec.HostNetwork, "HostNetwork must be cleared on Windows")
}

// A Linux hostPath volume mounted at a Windows-looking path must still be dropped (both the
// mount and the volume), since the backing hostPath is invalid on Windows.
func TestStripLinuxOnly_DropsLinuxHostPathAtWindowsMountPath(t *testing.T) {
	tmpl := &corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name: "agent",
				VolumeMounts: []corev1.VolumeMount{
					{Name: "docker-sock", MountPath: "C:/docker"}, // Windows-looking mount path
					{Name: "win-datadog-config", MountPath: "C:/ProgramData/Datadog"},
				},
			}},
			Volumes: []corev1.Volume{
				{Name: "docker-sock", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/var/run/docker.sock"}}},
				{Name: "win-datadog-config", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
			},
		},
	}
	StripLinuxOnlySettingsFromTemplate(tmpl)

	require.Len(t, tmpl.Spec.Containers, 1)
	// The docker-sock mount (Linux hostPath at a C:/ path) is dropped; only the emptyDir survives.
	require.Len(t, tmpl.Spec.Containers[0].VolumeMounts, 1)
	assert.Equal(t, "win-datadog-config", tmpl.Spec.Containers[0].VolumeMounts[0].Name)
	require.Len(t, tmpl.Spec.Volumes, 1)
	assert.Equal(t, "win-datadog-config", tmpl.Spec.Volumes[0].Name)
}

// HostProcess must be stripped from preserved WindowsOptions: HostProcess containers require
// hostNetwork=true, which the Windows strip clears, so keeping it would make the pod invalid.
func TestStripWindowsOptions_DropsHostProcess(t *testing.T) {
	user := "ContainerUser"
	hp := true
	sc := &corev1.SecurityContext{
		WindowsOptions: &corev1.WindowsSecurityContextOptions{RunAsUserName: &user, HostProcess: &hp},
	}
	got := stripLinuxSecurityContext(sc)
	require.NotNil(t, got)
	require.NotNil(t, got.WindowsOptions)
	assert.Nil(t, got.WindowsOptions.HostProcess, "HostProcess must be stripped")
	assert.Equal(t, "ContainerUser", *got.WindowsOptions.RunAsUserName, "runAsUserName preserved")

	psc := &corev1.PodSecurityContext{
		WindowsOptions: &corev1.WindowsSecurityContextOptions{HostProcess: &hp},
	}
	pgot := stripLinuxPodSecurityContext(psc)
	require.NotNil(t, pgot)
	assert.Nil(t, pgot.WindowsOptions.HostProcess, "pod-level HostProcess must be stripped")
}

// A -fips agent image tag must be detected (so the reconciler can reject it) rather than
// silently rewritten to a non-FIPS -servercore image.
func TestHasFIPSAgentImage(t *testing.T) {
	fips := &corev1.PodSpec{Containers: []corev1.Container{{Name: "agent", Image: "gcr.io/datadoghq/agent:7.80.2-fips"}}}
	assert.True(t, HasFIPSAgentImage(fips), "default agent image with -fips tag must be detected")

	nonFips := &corev1.PodSpec{Containers: []corev1.Container{{Name: "agent", Image: "gcr.io/datadoghq/agent:7.80.2"}}}
	assert.False(t, HasFIPSAgentImage(nonFips))

	// A custom (non-"agent") image is the user's responsibility and is not rewritten, so not flagged.
	custom := &corev1.PodSpec{Containers: []corev1.Container{{Name: "agent", Image: "registry.local/custom-agent:win-fips"}}}
	assert.False(t, HasFIPSAgentImage(custom))
}
