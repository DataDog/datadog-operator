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
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
)

// testDDA returns a minimal metav1.Object suitable for builder calls.
func testDDA(name string) metav1.Object {
	return &metav1.ObjectMeta{Name: name, Namespace: "datadog"}
}

func testRequiredComponent(containers ...apicommon.AgentContainerName) feature.RequiredComponent {
	return feature.RequiredComponent{
		IsRequired: ptr.To(true),
		Containers: containers,
	}
}

// --- GetWindowsAgentName ---

func TestGetWindowsAgentName(t *testing.T) {
	assert.Equal(t, "myagent-agent-windows", GetWindowsAgentName(testDDA("myagent")))
}

// --- NewDefaultWindowsAgentDaemonset ---

func TestNewDefaultWindowsAgentDaemonset_NodeSelector(t *testing.T) {
	dda := testDDA("dd")
	rc := testRequiredComponent(apicommon.CoreAgentContainerName)
	ds := NewDefaultWindowsAgentDaemonset(dda, &ExtendedDaemonsetOptions{}, rc, GetWindowsAgentName(dda))

	ns := ds.Spec.Template.Spec.NodeSelector
	require.NotNil(t, ns)
	assert.Equal(t, "windows", ns["kubernetes.io/os"])
}

func TestNewDefaultWindowsAgentDaemonset_Toleration(t *testing.T) {
	dda := testDDA("dd")
	rc := testRequiredComponent(apicommon.CoreAgentContainerName)
	ds := NewDefaultWindowsAgentDaemonset(dda, &ExtendedDaemonsetOptions{}, rc, GetWindowsAgentName(dda))

	tols := ds.Spec.Template.Spec.Tolerations
	require.Len(t, tols, 1)
	assert.Equal(t, "node.kubernetes.io/os", tols[0].Key)
	assert.Equal(t, corev1.TolerationOpEqual, tols[0].Operator)
	assert.Equal(t, "windows", tols[0].Value)
	assert.Equal(t, corev1.TaintEffectNoSchedule, tols[0].Effect)
}

func TestNewDefaultWindowsAgentDaemonset_NoPodSecurityContext(t *testing.T) {
	dda := testDDA("dd")
	rc := testRequiredComponent(apicommon.CoreAgentContainerName)
	ds := NewDefaultWindowsAgentDaemonset(dda, &ExtendedDaemonsetOptions{}, rc, GetWindowsAgentName(dda))

	// Windows does not support RunAsUser — PodSecurityContext must be nil.
	assert.Nil(t, ds.Spec.Template.Spec.SecurityContext)
}

func TestNewDefaultWindowsAgentDaemonset_InitContainer(t *testing.T) {
	dda := testDDA("dd")
	rc := testRequiredComponent(apicommon.CoreAgentContainerName)
	ds := NewDefaultWindowsAgentDaemonset(dda, &ExtendedDaemonsetOptions{}, rc, GetWindowsAgentName(dda))

	inits := ds.Spec.Template.Spec.InitContainers
	require.Len(t, inits, 1, "expected exactly one init container")

	init := inits[0]
	assert.Equal(t, "init-config-windows", init.Name)
	// Must create datadog.yaml inside the shared config volume.
	assert.Contains(t, strings.Join(init.Args, " "), "datadog.yaml")

	// Init container mounts at C:/ProgramData/Datadog — same as main containers —
	// so all three share the same emptyDir and the created datadog.yaml is visible.
	require.Len(t, init.VolumeMounts, 1)
	assert.Equal(t, windowsConfigVolumeName, init.VolumeMounts[0].Name)
	assert.Equal(t, windowsDatadogConfigPath, init.VolumeMounts[0].MountPath)
}

func TestNewDefaultWindowsAgentDaemonset_Volumes(t *testing.T) {
	dda := testDDA("dd")
	rc := testRequiredComponent(apicommon.CoreAgentContainerName)
	ds := NewDefaultWindowsAgentDaemonset(dda, &ExtendedDaemonsetOptions{}, rc, GetWindowsAgentName(dda))

	vols := ds.Spec.Template.Spec.Volumes

	// Only the shared config volume should be declared.
	require.Len(t, vols, 1)
	assert.Equal(t, windowsConfigVolumeName, vols[0].Name)
	assert.NotNil(t, vols[0].EmptyDir)

	// Linux-only volumes must not be present.
	for _, v := range vols {
		assert.NotEqual(t, "procdir", v.Name, "/proc volume must not be present on Windows")
		assert.NotEqual(t, "cgroups", v.Name, "/sys/fs/cgroup volume must not be present on Windows")
		assert.NotEqual(t, "passwd", v.Name, "/etc/passwd volume must not be present on Windows")
	}
}

func TestNewDefaultWindowsAgentDaemonset_NoLinuxHostPathVolumes(t *testing.T) {
	dda := testDDA("dd")
	rc := testRequiredComponent(apicommon.CoreAgentContainerName)
	ds := NewDefaultWindowsAgentDaemonset(dda, &ExtendedDaemonsetOptions{}, rc, GetWindowsAgentName(dda))

	for _, v := range ds.Spec.Template.Spec.Volumes {
		if v.HostPath != nil {
			assert.Fail(t, "unexpected hostPath volume on Windows DaemonSet", "volume %q has hostPath %q", v.Name, v.HostPath.Path)
		}
	}
}

// --- Container selection ---

func TestWindowsAgentContainers_CoreOnly(t *testing.T) {
	dda := testDDA("dd")
	// Only core agent requested.
	rc := testRequiredComponent(apicommon.CoreAgentContainerName)
	ds := NewDefaultWindowsAgentDaemonset(dda, &ExtendedDaemonsetOptions{}, rc, GetWindowsAgentName(dda))

	names := containerNames(ds.Spec.Template.Spec.Containers)
	assert.Contains(t, names, string(apicommon.CoreAgentContainerName))
	assert.NotContains(t, names, string(apicommon.SystemProbeContainerName))
	assert.NotContains(t, names, string(apicommon.SecurityAgentContainerName))
}

func TestWindowsAgentContainers_WithTraceAgent(t *testing.T) {
	dda := testDDA("dd")
	rc := testRequiredComponent(apicommon.CoreAgentContainerName, apicommon.TraceAgentContainerName)
	ds := NewDefaultWindowsAgentDaemonset(dda, &ExtendedDaemonsetOptions{}, rc, GetWindowsAgentName(dda))

	names := containerNames(ds.Spec.Template.Spec.Containers)
	assert.Contains(t, names, string(apicommon.CoreAgentContainerName))
	assert.Contains(t, names, string(apicommon.TraceAgentContainerName))
}

func TestWindowsAgentContainers_SystemProbeExcluded(t *testing.T) {
	dda := testDDA("dd")
	// Even when system-probe is requested it must be excluded on Windows.
	rc := testRequiredComponent(
		apicommon.CoreAgentContainerName,
		apicommon.SystemProbeContainerName,
		apicommon.SecurityAgentContainerName,
	)
	ds := NewDefaultWindowsAgentDaemonset(dda, &ExtendedDaemonsetOptions{}, rc, GetWindowsAgentName(dda))

	names := containerNames(ds.Spec.Template.Spec.Containers)
	assert.NotContains(t, names, string(apicommon.SystemProbeContainerName))
	assert.NotContains(t, names, string(apicommon.SecurityAgentContainerName))
}

// --- SecurityContext ---

func TestWindowsContainers_NoSecurityContext(t *testing.T) {
	dda := testDDA("dd")
	rc := testRequiredComponent(apicommon.CoreAgentContainerName, apicommon.TraceAgentContainerName)
	ds := NewDefaultWindowsAgentDaemonset(dda, &ExtendedDaemonsetOptions{}, rc, GetWindowsAgentName(dda))

	for _, c := range ds.Spec.Template.Spec.Containers {
		assert.Nil(t, c.SecurityContext,
			"container %q must have no securityContext on Windows", c.Name)
	}
}

// --- Volume mounts ---

func TestWindowsContainers_MountSharedConfigVolume(t *testing.T) {
	dda := testDDA("dd")
	rc := testRequiredComponent(apicommon.CoreAgentContainerName, apicommon.TraceAgentContainerName)
	ds := NewDefaultWindowsAgentDaemonset(dda, &ExtendedDaemonsetOptions{}, rc, GetWindowsAgentName(dda))

	for _, c := range ds.Spec.Template.Spec.Containers {
		found := false
		for _, m := range c.VolumeMounts {
			if m.Name == windowsConfigVolumeName && m.MountPath == windowsDatadogConfigPath {
				found = true
				break
			}
		}
		assert.True(t, found,
			"container %q must mount %q at %q", c.Name, windowsConfigVolumeName, windowsDatadogConfigPath)
	}
}

// --- Auth token env var ---

func TestWindowsContainers_AuthTokenPathOverridden(t *testing.T) {
	dda := testDDA("dd")
	rc := testRequiredComponent(apicommon.CoreAgentContainerName, apicommon.TraceAgentContainerName)
	ds := NewDefaultWindowsAgentDaemonset(dda, &ExtendedDaemonsetOptions{}, rc, GetWindowsAgentName(dda))

	for _, c := range ds.Spec.Template.Spec.Containers {
		val, found := findEnvVar(c.Env, common.DDAuthTokenFilePath)
		require.True(t, found, "container %q must set %s", c.Name, common.DDAuthTokenFilePath)
		assert.Equal(t, windowsAuthTokenFilePath, val,
			"container %q: %s must be Windows path", c.Name, common.DDAuthTokenFilePath)
		assert.False(t, strings.Contains(val, "/etc/"),
			"container %q: %s must not contain Linux path /etc/", c.Name, common.DDAuthTokenFilePath)
	}
}

// --- DaemonSet name / labels ---

func TestNewDefaultWindowsAgentDaemonset_Name(t *testing.T) {
	dda := testDDA("myagent")
	rc := testRequiredComponent(apicommon.CoreAgentContainerName)
	ds := NewDefaultWindowsAgentDaemonset(dda, &ExtendedDaemonsetOptions{}, rc, GetWindowsAgentName(dda))

	assert.Equal(t, "myagent-agent-windows", ds.Name)
	assert.Equal(t, "datadog", ds.Namespace)
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

// Init containers: only the Windows bootstrap survives.
func TestStripLinuxOnly_InitContainerAllowlist(t *testing.T) {
	tmpl := &corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			InitContainers: []corev1.Container{
				{Name: "init-config-windows"},
				{Name: string(apicommon.SeccompSetupContainerName)},
				{Name: string(apicommon.InitVolumeContainerName)},
				{Name: string(apicommon.InitConfigContainerName)},
			},
		},
	}
	StripLinuxOnlySettingsFromTemplate(tmpl)

	require.Len(t, tmpl.Spec.InitContainers, 1)
	assert.Equal(t, "init-config-windows", tmpl.Spec.InitContainers[0].Name)
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

func TestStripLinuxOnly_RemovesAppArmorAnnotationsForStrippedContainers(t *testing.T) {
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

	_, hasSystemProbe := tmpl.Annotations["container.apparmor.security.beta.kubernetes.io/system-probe"]
	assert.False(t, hasSystemProbe, "AppArmor annotation for stripped system-probe must be removed")
	_, hasAgent := tmpl.Annotations["container.apparmor.security.beta.kubernetes.io/agent"]
	assert.True(t, hasAgent, "AppArmor annotation for surviving agent must be kept")
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
						{Name: "DD_APM_ENABLED", Value: "true"},
						{Name: "DD_API_KEY", Value: "x"},
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
	} {
		assert.False(t, names[stripped], "%s must be stripped on Windows", stripped)
	}
	assert.True(t, names["DD_APM_ENABLED"], "non-Linux env must be preserved")
	assert.True(t, names["DD_API_KEY"], "non-Linux env must be preserved")
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

// EnsureWindowsIntakeReachable must force non-local traffic on even when a feature
// (e.g. DogStatsD) already wrote the env var to "false" — last-writer-wins. This is the
// regression guard for the clobber the reviewer found.
func TestEnsureWindowsIntakeReachable_WinsOverFeatureValue(t *testing.T) {
	tmpl := &corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: string(apicommon.CoreAgentContainerName),
					// Simulate the DogStatsD feature having set it to the default false.
					Env: []corev1.EnvVar{{Name: "DD_DOGSTATSD_NON_LOCAL_TRAFFIC", Value: "false"}},
				},
				{Name: string(apicommon.TraceAgentContainerName)},
			},
		},
	}
	managers := feature.NewPodTemplateManagers(tmpl)
	EnsureWindowsIntakeReachable(managers)

	check := func(container, env string) {
		for _, c := range tmpl.Spec.Containers {
			if c.Name == container {
				for _, e := range c.Env {
					if e.Name == env {
						assert.Equal(t, "true", e.Value, "%s on %s must be forced true", env, container)
						return
					}
				}
				t.Fatalf("%s not found on %s", env, container)
			}
		}
	}
	check(string(apicommon.CoreAgentContainerName), "DD_DOGSTATSD_NON_LOCAL_TRAFFIC")
	check(string(apicommon.TraceAgentContainerName), "DD_APM_NON_LOCAL_TRAFFIC")
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

// AddWindowsLogCollectionVolumes adds the three Windows host-log hostPath volumes + mounts
// on the core agent (mirrors the Helm chart), and survives the strip because they use C:/ paths.
func TestAddWindowsLogCollectionVolumes(t *testing.T) {
	spec := &corev1.PodSpec{
		Containers: []corev1.Container{
			{Name: string(apicommon.CoreAgentContainerName)},
			{Name: string(apicommon.TraceAgentContainerName)},
		},
	}
	AddWindowsLogCollectionVolumes(spec, WindowsLogPaths{}) // empty → Windows defaults

	// 3 hostPath volumes with the expected default Windows paths.
	paths := map[string]string{}
	for _, v := range spec.Volumes {
		require.NotNil(t, v.HostPath, "volume %q must be hostPath", v.Name)
		paths[v.Name] = v.HostPath.Path
	}
	assert.Equal(t, "C:/var/log", paths[windowsPointerVolumeName])
	assert.Equal(t, "C:/var/log/pods", paths[windowsPodLogsVolumeName])
	assert.Equal(t, "C:/ProgramData", paths[windowsContainerLogVolumeName])

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
	assert.Len(t, core.VolumeMounts, 3, "core agent gets the 3 log mounts")
	assert.Empty(t, trace.VolumeMounts, "trace agent gets no log mounts")
	for _, m := range core.VolumeMounts {
		assert.True(t, strings.HasPrefix(m.MountPath, "C:/"), "mount %q must be a Windows path", m.MountPath)
	}

	// And they survive the strip (Windows paths, not Linux).
	tmpl := &corev1.PodTemplateSpec{Spec: *spec}
	StripLinuxOnlySettingsFromTemplate(tmpl)
	surviving := 0
	for _, v := range tmpl.Spec.Volumes {
		if v.Name == windowsPointerVolumeName || v.Name == windowsPodLogsVolumeName || v.Name == windowsContainerLogVolumeName {
			surviving++
		}
	}
	assert.Equal(t, 3, surviving, "Windows log volumes must survive the strip")
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
	assert.Equal(t, "D:/containerd", paths[windowsContainerLogVolumeName])

	// ...but the in-container mount path is always the Agent's standard Windows path, so the
	// Agent reads logs where it expects regardless of the host path (Linux model).
	mountPaths := map[string]string{}
	for _, m := range spec.Containers[0].VolumeMounts {
		mountPaths[m.Name] = m.MountPath
	}
	assert.Equal(t, windowsVarLogPath, mountPaths[windowsPointerVolumeName])
	assert.Equal(t, windowsPodLogsPath, mountPaths[windowsPodLogsVolumeName])
	assert.Equal(t, windowsProgramDataPath, mountPaths[windowsContainerLogVolumeName])
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
	assert.Equal(t, "C:/ProgramData", paths[windowsContainerLogVolumeName])
}
