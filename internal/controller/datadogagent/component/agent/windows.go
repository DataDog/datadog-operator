// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Windows DaemonSet builder — prototype for CONTP-1448.
// Creates a minimal Windows-compatible DaemonSet alongside the Linux one:
//   - nodeSelector: kubernetes.io/os=windows
//   - toleration for node.kubernetes.io/os=windows:NoSchedule
//   - Windows agent image (servercore variant)
//   - PowerShell init container creates an empty datadog.yaml + auth/ dir in a shared vol
//   - Core agent + trace agent share config vol at C:/ProgramData/Datadog
//     so the auth_token written by core agent is visible to trace agent
//   - Core agent + trace agent only (no system-probe/security-agent — Linux/eBPF only)
//   - No Linux capabilities, no seccomp, no readOnlyRootFilesystem
//   - Only emptyDir/configMap volumes (no Linux hostPath mounts)
//   - StripLinuxOnlySettings (allowlist) removes anything Linux-only that features inject

package agent

import (
	"slices"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/pkg/images"
)

const (
	// windowsInitContainerName is the name of the Windows config-bootstrap init container.
	windowsInitContainerName = "init-config-windows"

	// windowsConfigVolumeName is the emptyDir volume shared between the init container
	// and the core/trace agent containers on Windows. The init container creates an empty
	// datadog.yaml (and the auth/ dir) in this volume; both agent containers then mount it
	// at windowsDatadogConfigPath so they share the same datadog.yaml and the auth_token
	// the core agent writes is visible to the trace agent.
	windowsConfigVolumeName = "win-datadog-config"

	// windowsDatadogConfigPath is the Windows default config directory. Both the core
	// agent and trace agent mount the shared config volume here.
	windowsDatadogConfigPath = "C:/ProgramData/Datadog"

	// windowsAuthTokenFilePath is the auth token path for Windows containers.
	// This overrides the Linux default (DD_AUTH_TOKEN_FILE_PATH=/etc/datadog-agent/auth/token)
	// that commonEnvVars injects, since both containers mount the shared vol at
	// windowsDatadogConfigPath and the core agent writes auth_token there.
	windowsAuthTokenFilePath = windowsDatadogConfigPath + "/auth/token"

	// Windows log-collection host paths (mirrors the Helm chart's daemonset-volumes-windows).
	// On Windows, pod log files under C:/var/log/pods are symlinks into C:/ProgramData
	// (containerd/docker), so the agent needs C:/ProgramData mounted to follow them.
	windowsVarLogPath      = "C:/var/log"      // pointer dir (RW: agent stores log tail offsets)
	windowsPodLogsPath     = "C:/var/log/pods" // Kubernetes pod log files
	windowsProgramDataPath = "C:/ProgramData"  // symlink targets for the pod log files

	windowsPointerVolumeName      = "win-pointerdir"
	windowsPodLogsVolumeName      = "win-logpodpath"
	windowsContainerLogVolumeName = "win-logcontainerpath"
)

// WindowsLogPaths holds the host log paths to mount on the Windows agent. Empty fields fall
// back to the Windows defaults. These come from features.logCollection.{tempStoragePath,
// podLogsPath,containerLogsPath} so custom Windows log paths take effect.
type WindowsLogPaths struct {
	TempStoragePath   string // pointer dir (RW); default C:/var/log
	PodLogsPath       string // pod logs (RO);   default C:/var/log/pods
	ContainerLogsPath string // symlink targets (RO); default C:/ProgramData
}

// AddWindowsLogCollectionVolumes adds the Windows host-log hostPath volumes + mounts to the
// core agent so log collection actually works (the logcollection feature only injects the
// Linux paths, which the strip removes). Mirrors the Helm chart's daemonset-volumes-windows,
// but honors the configured logCollection paths (falling back to the Windows defaults):
//   - tempStoragePath   (pointer dir, RW — agent persists tail offsets); default C:/var/log
//   - podLogsPath       (RO — Kubernetes pod log files);                 default C:/var/log/pods
//   - containerLogsPath (RO — symlink targets the pod logs point into);  default C:/ProgramData
//
// It is a no-op if there is no core agent container (e.g. it was stripped). Safe to call only
// when log collection is enabled.
//
// Following the Linux logcollection feature model, the configured paths set the HOST path only;
// the in-container mount path is always the Windows Agent's standard path (C:/var/log,
// C:/var/log/pods, C:/ProgramData). The Agent reads logs from those fixed in-container paths, so
// a custom host path (e.g. D:/logs/pods) is surfaced where the Agent expects it without extra
// Agent config. Mounting a custom path at itself would instead leave the Agent reading an empty
// default path and silently collecting nothing.
func AddWindowsLogCollectionVolumes(spec *corev1.PodSpec, paths WindowsLogPaths) {
	pointerHostPath := windowsPathOrDefault(paths.TempStoragePath, windowsVarLogPath)
	podLogsHostPath := windowsPathOrDefault(paths.PodLogsPath, windowsPodLogsPath)
	containerLogsHostPath := windowsPathOrDefault(paths.ContainerLogsPath, windowsProgramDataPath)

	// Mount on the core agent, or the consolidated single agent under single-container strategy.
	idx := -1
	for i := range spec.Containers {
		if isWindowsMainAgentContainer(spec.Containers[i].Name) {
			idx = i
			break
		}
	}
	if idx == -1 {
		return
	}

	hostPathVol := func(name, path string) corev1.Volume {
		hpType := corev1.HostPathDirectoryOrCreate
		return corev1.Volume{
			Name:         name,
			VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: path, Type: &hpType}},
		}
	}
	spec.Volumes = append(spec.Volumes,
		hostPathVol(windowsPointerVolumeName, pointerHostPath),
		hostPathVol(windowsPodLogsVolumeName, podLogsHostPath),
		hostPathVol(windowsContainerLogVolumeName, containerLogsHostPath),
	)
	// Mount paths are fixed to the Agent's standard Windows paths regardless of the host path.
	spec.Containers[idx].VolumeMounts = append(spec.Containers[idx].VolumeMounts,
		corev1.VolumeMount{Name: windowsPointerVolumeName, MountPath: windowsVarLogPath},
		corev1.VolumeMount{Name: windowsPodLogsVolumeName, MountPath: windowsPodLogsPath, ReadOnly: true},
		corev1.VolumeMount{Name: windowsContainerLogVolumeName, MountPath: windowsProgramDataPath, ReadOnly: true},
	)

	// Force file-based container log collection on Windows. The only Windows log source we
	// mount is the file tree (C:/var/log/pods + C:/ProgramData); runtime/socket-based
	// collection has no Windows container-runtime socket/pipe wired up, so if the user set
	// containerCollectUsingFiles=false the agent would collect nothing. Override to file mode.
	spec.Containers[idx].Env = setEnvVar(spec.Containers[idx].Env, "DD_LOGS_CONFIG_K8S_CONTAINER_USE_FILE", "true")
}

// windowsPathOrDefault returns the Windows default when the configured path is empty OR is a
// Linux absolute path (starts with "/"). The latter is essential: the logCollection feature
// defaults these fields to Linux paths (/var/log/pods, /var/lib/docker/containers, …), so a
// plain empty-check would let those defaults through and produce an invalid Windows DaemonSet.
// Only genuinely Windows paths (e.g. C:/...) are honored as overrides.
func windowsPathOrDefault(v, def string) string {
	if v == "" || strings.HasPrefix(v, "/") {
		return def
	}
	return v
}

// setEnvVar replaces the named env var (or appends it) and returns the slice.
func setEnvVar(envs []corev1.EnvVar, name, value string) []corev1.EnvVar {
	for i := range envs {
		if envs[i].Name == name {
			envs[i] = corev1.EnvVar{Name: name, Value: value}
			return envs
		}
	}
	return append(envs, corev1.EnvVar{Name: name, Value: value})
}

// EnsureWindowsServercoreImage makes sure the default Datadog Agent image on the Windows
// pod spec carries the "-servercore" tag suffix. It MUST run after override.PodTemplateSpec
// applies the nodeAgent image override: the generic OverrideAgentImage path replaces the
// whole tag (servercore is not modeled as a preserved suffix), so a user pinning
// image.tag=7.81.0 on the default image would otherwise get the Linux agent image.
//
// Coercion is intentionally limited to the default Datadog Agent image name ("agent"): a
// fully custom image (e.g. registry.local/custom-agent:win2022) is the user's responsibility
// and must already be Windows-compatible — we must not mangle its tag. Images whose tag
// already contains "servercore" are left unchanged.
func EnsureWindowsServercoreImage(spec *corev1.PodSpec) {
	for i := range spec.Containers {
		spec.Containers[i].Image = ensureServercoreTag(spec.Containers[i].Image)
	}
	for i := range spec.InitContainers {
		spec.InitContainers[i].Image = ensureServercoreTag(spec.InitContainers[i].Image)
	}
}

// HasFIPSAgentImage reports whether any container uses the default Datadog Agent image with a
// FIPS-flavored tag (e.g. gcr.io/datadoghq/agent:7.80.2-fips). ensureServercoreTag would silently
// rewrite such a tag to a non-FIPS -servercore image, so the reconciler uses this to reject FIPS
// on Windows even when it was requested via an image tag override (not the global FIPS flags).
func HasFIPSAgentImage(spec *corev1.PodSpec) bool {
	isFIPS := func(image string) bool {
		slash := strings.LastIndex(image, "/")
		colon := strings.LastIndex(image, ":")
		if colon <= slash {
			return false
		}
		return image[slash+1:colon] == images.DefaultAgentImageName && strings.Contains(image[colon+1:], images.FIPSTagSuffix)
	}
	for _, c := range spec.Containers {
		if isFIPS(c.Image) {
			return true
		}
	}
	for _, c := range spec.InitContainers {
		if isFIPS(c.Image) {
			return true
		}
	}
	return false
}

func ensureServercoreTag(image string) string {
	// The tag is the segment after the last ":" that follows the last "/" (so a registry
	// port like localhost:5000/agent:7.81.0 isn't mistaken for the tag).
	slash := strings.LastIndex(image, "/")
	colon := strings.LastIndex(image, ":")
	if colon <= slash {
		return image // no tag present; nothing safe to do
	}
	// Only coerce the default Datadog Agent image ("<registry>/agent"); leave custom images alone.
	name := image[slash+1 : colon]
	if name != images.DefaultAgentImageName {
		return image
	}
	tag := image[colon+1:]
	if strings.Contains(tag, "servercore") {
		return image
	}
	base := image[:colon+1] // includes the trailing ":"
	// Windows agent images are published as "<version>-servercore" and "<version>-servercore-jmx".
	// The built tag may carry Linux flavor suffixes (-jmx/-full/-fips) that must not be appended
	// verbatim ("<version>-jmx-servercore" does not exist). Strip them, insert -servercore, and
	// re-append only -jmx (the sole flavor with a Windows variant). -full and -fips have no
	// Windows image, so they are dropped in favor of the plain servercore image.
	hasJMX := strings.Contains(tag, images.JMXTagSuffix)
	for _, suffix := range []string{images.JMXTagSuffix, images.FullTagSuffix, images.FIPSTagSuffix} {
		tag = strings.ReplaceAll(tag, suffix, "")
	}
	tag += images.WindowsServerCoreTagSuffix
	if hasJMX {
		tag += images.JMXTagSuffix
	}
	return base + tag
}

// windowsInitContainers returns a PowerShell init container that creates a minimal
// datadog.yaml inside the shared win-datadog-config emptyDir volume, which is mounted
// at windowsDatadogConfigPath. All meaningful configuration (API key, site, cluster-agent
// token) is supplied via environment variables injected by global settings, so an empty
// config file is sufficient to satisfy the agent's startup requirement.
//
// All three containers (init, core agent, trace agent) mount the same emptyDir at
// C:/ProgramData/Datadog, so the auth_token written by the core agent is visible to
// the trace agent without any additional path trickery.
func windowsInitContainers(img string) []corev1.Container {
	return []corev1.Container{
		{
			Name:    windowsInitContainerName,
			Image:   img,
			Command: []string{"powershell", "-command"},
			// Create the auth/ subdirectory before datadog.yaml so the trace agent
			// can read the token path (C:/ProgramData/Datadog/auth/token) immediately
			// on startup without waiting for the core agent to create it.
			Args: []string{
				"New-Item -ItemType Directory -Force -Path C:/ProgramData/Datadog/auth; " +
					"New-Item -ItemType File -Force -Path C:/ProgramData/Datadog/datadog.yaml",
			},
			VolumeMounts: []corev1.VolumeMount{
				{
					Name:      windowsConfigVolumeName,
					MountPath: windowsDatadogConfigPath,
				},
			},
		},
	}
}

// overrideAuthTokenPath returns a copy of envs with DD_AUTH_TOKEN_FILE_PATH set to the
// Windows auth token path. It builds a new slice to avoid mutating the caller's backing array.
func overrideAuthTokenPath(envs []corev1.EnvVar) []corev1.EnvVar {
	out := make([]corev1.EnvVar, 0, len(envs)+1)
	replaced := false
	for _, e := range envs {
		if e.Name == common.DDAuthTokenFilePath {
			out = append(out, corev1.EnvVar{Name: common.DDAuthTokenFilePath, Value: windowsAuthTokenFilePath})
			replaced = true
			continue
		}
		out = append(out, e)
	}
	if !replaced {
		out = append(out, corev1.EnvVar{Name: common.DDAuthTokenFilePath, Value: windowsAuthTokenFilePath})
	}
	return out
}

// volumesForWindowsAgent returns the volumes for the Windows DaemonSet.
// Only the shared config volume is declared; Linux-only volumes (/proc, /sys/fs/cgroup,
// /etc/passwd, seccomp, Unix sockets) are omitted. Additional volumes (log collection
// paths, named pipes for DogStatsD/APM) will be added as Windows feature support lands.
func volumesForWindowsAgent(_ metav1.Object) []corev1.Volume {
	return []corev1.Volume{
		// Shared config volume: init-config-windows copies the image's C:/ProgramData/Datadog
		// here; core agent and trace agent both mount it at windowsDatadogConfigPath so they
		// share the same datadog.yaml and auth_token.
		{
			Name:         windowsConfigVolumeName,
			VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
		},
	}
}

func volumeMountsForWindowsCoreAgent() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		// Mount shared config vol so core agent reads from init-populated
		// C:/ProgramData/Datadog and writes auth_token there for trace agent.
		{Name: windowsConfigVolumeName, MountPath: windowsDatadogConfigPath},
	}
}

func volumeMountsForWindowsTraceAgent() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		// Mount same shared config vol so trace agent sees auth_token from core agent.
		{Name: windowsConfigVolumeName, MountPath: windowsDatadogConfigPath},
	}
}

func volumeMountsForWindowsProcessAgent() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		{Name: windowsConfigVolumeName, MountPath: windowsDatadogConfigPath},
	}
}

// windowsAllowedContainerNames is the allowlist of agent containers supported on Windows.
// StripLinuxOnlySettings keeps ONLY these; everything else a feature might inject
// (system-probe, security-agent, host-profiler, flightrecorder, fips-proxy, otel-agent,
// agent-data-plane, private-action-runner, …) is removed. An allowlist is used instead
// of a denylist so new Linux-only container classes don't silently leak onto Windows.
var windowsAllowedContainerNames = map[string]bool{
	string(apicommon.CoreAgentContainerName):               true,
	string(apicommon.TraceAgentContainerName):              true,
	string(apicommon.ProcessAgentContainerName):            true,
	string(apicommon.UnprivilegedSingleAgentContainerName): true, // single-container strategy (behaves as core)
}

// windowsAllowedInitContainerNames is the allowlist of init containers on Windows.
// Only the Windows config-bootstrap init container is kept; Linux init containers
// (seccomp-setup, init-volume, init-config, …) are removed.
var windowsAllowedInitContainerNames = map[string]bool{
	windowsInitContainerName: true,
}

// StripLinuxOnlySettings removes Linux-incompatible mutations from a Windows PodSpec.
// See stripLinuxOnlyFromSpec for the full description; prefer
// StripLinuxOnlySettingsFromTemplate which also cleans container-scoped pod annotations.
func StripLinuxOnlySettings(spec *corev1.PodSpec) {
	stripLinuxOnlyFromSpec(spec, nil)
}

// StripLinuxOnlySettingsFromTemplate removes Linux-incompatible mutations from a full
// PodTemplateSpec — including container-scoped pod annotations (AppArmor profiles, etc.)
// that reference removed containers, which Kubernetes would otherwise reject.
//
// It is called AFTER features run their ManageNodeAgent hooks. The model is an allowlist,
// not a denylist, because the feature surface grows over time and a denylist is always
// one feature behind:
//
//   - Containers: keep ONLY windowsAllowedContainerNames (core/trace/process agent).
//     Everything else (system-probe, security-agent, host-profiler, flightrecorder,
//     fips-proxy, otel-agent, …) is removed.
//   - Init containers: keep ONLY windowsAllowedInitContainerNames (the Windows bootstrap).
//   - Volume mounts: on each surviving container, drop any mount whose MountPath is a
//     Linux absolute path (starts with "/"). Windows mounts use C:/... paths, so this
//     uniformly catches hostPath mounts (/proc, /), emptyDir mounts at Linux paths
//     (/var/run/sysprobe, /var/run/datadog/flightrecorder), and config mounts.
//   - Volumes: drop any volume not referenced by a surviving container's mounts.
//   - SecurityContext: clear Linux fields (capabilities, seccomp, SELinux, AppArmor,
//     readOnlyRootFilesystem).
//
// Preserved: all feature env vars, Windows-path mounts/volumes added by the Windows builder.
func StripLinuxOnlySettingsFromTemplate(tmpl *corev1.PodTemplateSpec) {
	stripLinuxOnlyFromSpec(&tmpl.Spec, &tmpl.Annotations)
}

// isWindowsMainAgentContainer reports whether the container is the primary agent process on
// Windows — the core agent, or the consolidated single agent under single-container strategy.
func isWindowsMainAgentContainer(name string) bool {
	return name == string(apicommon.CoreAgentContainerName) ||
		name == string(apicommon.UnprivilegedSingleAgentContainerName)
}

// ApplyWindowsPodTransformation converts an already-built (Linux) node-agent pod template
// into a Windows-compatible one, in place. It is used on the DatadogAgentProfile+provider
// path: a profile targeting Windows nodes (annotation datadoghq.com/provider=windows) builds
// a normal agent DaemonSet through the shared machinery — which owns naming, labels, node
// affinity and overrides — and this function then Windows-ifies the pod contents:
//
//  1. Strip every Linux-only artifact (StripLinuxOnlySettingsFromTemplate): non-Windows
//     containers, "/"-path mounts/volumes, Unix-socket + Linux-only env, container and pod
//     securityContext, hostPID/hostIPC, and all Linux init containers.
//  2. Re-add the Windows config bootstrap: a shared emptyDir + PowerShell init container that
//     seeds datadog.yaml and the auth dir, mounted at C:/ProgramData/Datadog on the surviving
//     agent containers, plus the Windows container commands and the Windows auth-token path.
//  3. Coerce the default agent image to the -servercore variant.
//  4. Add the Windows host-log hostPath mounts when log collection is enabled.
//  5. Force non-local traffic so APM/DogStatsD are reachable without Unix sockets.
//  6. Add the Windows nodeSelector + the node.kubernetes.io/os=windows:NoSchedule toleration.
//
// logPaths carries the configured logCollection paths (Windows defaults are applied for the
// Linux/empty values).
func ApplyWindowsPodTransformation(tmpl *corev1.PodTemplateSpec, dda metav1.Object, logCollectionEnabled bool, logPaths WindowsLogPaths) {
	StripLinuxOnlySettingsFromTemplate(tmpl)
	spec := &tmpl.Spec

	// Re-add the Windows config bootstrap (init container + shared volume + mounts/commands).
	// The init container must use the SAME image + pull policy as the main agent container so
	// that registry overrides / pinned tags applied to the agent (via global registry rewriting
	// and the nodeAgent image override, both of which ran before this transformation) are also
	// honored by the init container — otherwise it would pull the default gcr.io image and break
	// private-registry / air-gapped installs. EnsureWindowsServercoreImage below coerces both to
	// the -servercore variant consistently. Falls back to the default image if no agent container
	// survives (should not happen).
	initImage := images.GetLatestWindowsAgentImage()
	var initPullPolicy corev1.PullPolicy
	for i := range spec.Containers {
		if isWindowsMainAgentContainer(spec.Containers[i].Name) {
			initImage = spec.Containers[i].Image
			initPullPolicy = spec.Containers[i].ImagePullPolicy
			break
		}
	}
	spec.InitContainers = windowsInitContainers(initImage)
	if initPullPolicy != "" {
		spec.InitContainers[0].ImagePullPolicy = initPullPolicy
	}
	spec.Volumes = append(spec.Volumes, volumesForWindowsAgent(dda)...)
	for i := range spec.Containers {
		c := &spec.Containers[i]
		switch c.Name {
		case string(apicommon.CoreAgentContainerName):
			c.Command = []string{"agent", "run"}
			c.VolumeMounts = append(c.VolumeMounts, volumeMountsForWindowsCoreAgent()...)
			c.Env = overrideAuthTokenPath(c.Env)
			// DogStatsD has no Unix socket on Windows: accept UDP from other pods.
			c.Env = setEnvVar(c.Env, "DD_DOGSTATSD_NON_LOCAL_TRAFFIC", "true")
		case string(apicommon.TraceAgentContainerName):
			c.Command = []string{"trace-agent", "run", "--foreground"}
			c.VolumeMounts = append(c.VolumeMounts, volumeMountsForWindowsTraceAgent()...)
			c.Env = overrideAuthTokenPath(c.Env)
			// APM receiver must accept TCP from other pods (no Unix socket on Windows).
			c.Env = setEnvVar(c.Env, "DD_APM_NON_LOCAL_TRAFFIC", "true")
		case string(apicommon.ProcessAgentContainerName):
			c.Command = []string{"process-agent", "--foreground"}
			c.VolumeMounts = append(c.VolumeMounts, volumeMountsForWindowsProcessAgent()...)
			c.Env = overrideAuthTokenPath(c.Env)
		case string(apicommon.UnprivilegedSingleAgentContainerName):
			// Single-container strategy: one consolidated agent process. Treat it as the core
			// agent on Windows (runs `agent run`, needs the shared config + non-local traffic
			// for the in-process APM/DogStatsD). Without this it would be stripped, leaving a
			// container-less (invalid) pod.
			c.Command = []string{"agent", "run"}
			c.VolumeMounts = append(c.VolumeMounts, volumeMountsForWindowsCoreAgent()...)
			c.Env = overrideAuthTokenPath(c.Env)
			c.Env = setEnvVar(c.Env, "DD_DOGSTATSD_NON_LOCAL_TRAFFIC", "true")
			c.Env = setEnvVar(c.Env, "DD_APM_NON_LOCAL_TRAFFIC", "true")
		}
	}

	EnsureWindowsServercoreImage(spec)

	if logCollectionEnabled {
		AddWindowsLogCollectionVolumes(spec, logPaths)
	}

	// Node scheduling: target Windows nodes and tolerate the automatic Windows taint. The
	// profile's node affinity also targets Windows nodes; the nodeSelector is belt-and-braces.
	if spec.NodeSelector == nil {
		spec.NodeSelector = map[string]string{}
	}
	spec.NodeSelector["kubernetes.io/os"] = "windows"
	windowsToleration := corev1.Toleration{
		Key:      "node.kubernetes.io/os",
		Operator: corev1.TolerationOpEqual,
		Value:    "windows",
		Effect:   corev1.TaintEffectNoSchedule,
	}
	// Avoid a duplicate if an override already added the same toleration.
	if !slices.Contains(spec.Tolerations, windowsToleration) {
		spec.Tolerations = append(spec.Tolerations, windowsToleration)
	}
}

func stripLinuxOnlyFromSpec(spec *corev1.PodSpec, annotations *map[string]string) {
	// 0. Clear Linux pod-level namespace-sharing fields and pod-level security context.
	//    Features can set these (e.g. DogStatsD origin detection sets hostPID=true), and a
	//    nodeAgent securityContext override lands on the pod spec before this strip runs.
	//    All are Linux-only (SELinux/seccomp/sysctls/fsGroup/…) and produce a
	//    Windows-invalid PodSpec that the API server / kubelet rejects.
	spec.HostPID = false
	spec.HostIPC = false
	// hostNetwork (Linux host networking) is not supported by regular Windows ServerCore
	// containers; an override could set it, so clear it here.
	spec.HostNetwork = false
	spec.ShareProcessNamespace = nil
	spec.SecurityContext = stripLinuxPodSecurityContext(spec.SecurityContext)

	// Volumes backed by a Linux absolute hostPath (e.g. /var/run/docker.sock) are invalid on
	// Windows even when a user override mounts them at a Windows-looking path (C:/...), so their
	// mounts must be dropped too — not only mounts whose MountPath starts with "/".
	linuxHostPathVols := make(map[string]bool)
	for _, v := range spec.Volumes {
		if v.HostPath != nil && strings.HasPrefix(v.HostPath.Path, "/") {
			linuxHostPathVols[v.Name] = true
		}
	}

	// 1. Keep only allowlisted main containers; drop their Linux mounts, Unix-socket
	//    env vars, and security context.
	var keepContainers []corev1.Container
	for _, c := range spec.Containers {
		if !windowsAllowedContainerNames[c.Name] {
			continue
		}
		c.VolumeMounts = stripLinuxMounts(c.VolumeMounts, linuxHostPathVols)
		c.Env = stripWindowsIncompatibleEnvVars(c.Env)
		c.SecurityContext = stripLinuxSecurityContext(c.SecurityContext)
		keepContainers = append(keepContainers, c)
	}
	spec.Containers = keepContainers

	// 2. Keep only allowlisted init containers; drop their Linux mounts + security context.
	var keepInits []corev1.Container
	for _, c := range spec.InitContainers {
		if !windowsAllowedInitContainerNames[c.Name] {
			continue
		}
		c.VolumeMounts = stripLinuxMounts(c.VolumeMounts, linuxHostPathVols)
		c.Env = stripWindowsIncompatibleEnvVars(c.Env)
		c.SecurityContext = stripLinuxSecurityContext(c.SecurityContext)
		keepInits = append(keepInits, c)
	}
	spec.InitContainers = keepInits

	// 3. Drop any volume no longer referenced by a surviving container's mounts (this also
	//    removes the Linux-hostPath volumes whose mounts were just dropped).
	referenced := make(map[string]bool)
	for _, c := range spec.Containers {
		for _, m := range c.VolumeMounts {
			referenced[m.Name] = true
		}
	}
	for _, c := range spec.InitContainers {
		for _, m := range c.VolumeMounts {
			referenced[m.Name] = true
		}
	}
	var keepVols []corev1.Volume
	for _, v := range spec.Volumes {
		if referenced[v.Name] {
			keepVols = append(keepVols, v)
		}
	}
	spec.Volumes = keepVols

	// 4. Remove ALL AppArmor pod annotations (container.apparmor.security.beta.kubernetes.io/<name>).
	//    AppArmor is Linux-only, so these are invalid on a Windows pod regardless of whether the
	//    referenced container survived; Kubernetes also rejects annotations referencing removed
	//    containers.
	if annotations != nil && *annotations != nil {
		for key := range *annotations {
			if strings.HasPrefix(key, "container.apparmor.security.beta.kubernetes.io/") {
				delete(*annotations, key)
			}
		}
	}
}

// stripLinuxMounts removes any volume mount whose MountPath is a Linux absolute path
// (starts with "/"). Windows mounts use C:/... drive-letter paths, so this is safe and
// catches hostPath mounts, emptyDir mounts at Linux paths, and config mounts uniformly.
// stripLinuxMounts drops mounts that are invalid on Windows: those mounted at a Linux absolute
// path ("/..."), and those referencing a Linux-hostPath-backed volume (linuxHostPathVols) even
// when the mount path itself looks Windows (e.g. an override mounting /var/run/docker.sock at
// C:/somewhere) — otherwise the backing hostPath would make the Windows pod invalid.
func stripLinuxMounts(mounts []corev1.VolumeMount, linuxHostPathVols map[string]bool) []corev1.VolumeMount {
	var keep []corev1.VolumeMount
	for _, m := range mounts {
		if strings.HasPrefix(m.MountPath, "/") || linuxHostPathVols[m.Name] {
			continue
		}
		keep = append(keep, m)
	}
	return keep
}

// linuxOnlyEnvVarNames are env vars that global settings / features inject with
// Linux-specific path or transport values (Unix socket, host CA path, vsock) that are
// invalid on Windows. Their backing volumes/mounts are already stripped; the env vars
// must be removed too or the Windows agent gets fed bad paths/transports. Names mirror
// the constants in internal/.../global/envvar.go (literals used to avoid an import cycle).
var linuxOnlyEnvVarNames = map[string]bool{
	"DOCKER_HOST":          true, // global.dockerSocketPath -> unix:///... (DD_CRI_SOCKET_PATH is caught by the SOCKET rule)
	"DD_KUBELET_CLIENT_CA": true, // global.kubelet.hostCAPath -> /var/run/host-kubelet-ca.crt
	"DD_VSOCK_ADDR":        true, // global.useVSock -> Linux vsock transport
	// DogStatsD server-side origin detection resolves pod identity via the host PID
	// namespace (/proc). The DogStatsD feature sets hostPID=true alongside it, but the
	// strip clears hostPID (Windows-invalid), so this would just log errors on Windows.
	// The client-side variant (DD_DOGSTATSD_ORIGIN_DETECTION_CLIENT, container ID in the
	// datagram) needs no hostPID and is preserved.
	"DD_DOGSTATSD_ORIGIN_DETECTION": true,
}

// stripWindowsIncompatibleEnvVars removes env vars that don't work on Windows:
//   - any name containing "SOCKET" (Unix-domain-socket config: DD_APM_RECEIVER_SOCKET,
//     DD_DOGSTATSD_SOCKET, DD_*_HOST_SOCKET_PATH, DD_CRI_SOCKET_PATH). APM/DogStatsD
//     default to UDS-enabled, so removing these lets them fall back to TCP/UDP.
//   - the Linux-only global env vars in linuxOnlyEnvVarNames (docker host, kubelet CA path,
//     vsock) whose Linux path/transport values are invalid on Windows.
//
// Windows named-pipe config (DD_*_WINDOWS_PIPE_NAME) does not match and is preserved.
func stripWindowsIncompatibleEnvVars(envs []corev1.EnvVar) []corev1.EnvVar {
	var keep []corev1.EnvVar
	for _, e := range envs {
		// DD_FIPS_* configure routing through a local fips-proxy container. That container is
		// Linux-only and is removed by the allowlist strip, so leaving these set would make the
		// Windows agent route intake at a proxy that does not exist. FIPS is not supported on
		// Windows (no -servercore-fips image), so drop the config and talk to intake directly.
		if strings.Contains(e.Name, "SOCKET") || strings.HasPrefix(e.Name, "DD_FIPS_") || linuxOnlyEnvVarNames[e.Name] {
			continue
		}
		keep = append(keep, e)
	}
	return keep
}

// stripLinuxPodSecurityContext clears Linux-only pod-level security settings
// (SELinux, seccomp, sysctls, fsGroup, supplementalGroups, runAsUser/Group/NonRoot)
// that the Windows API server / kubelet rejects, while preserving the only
// Windows-compatible field, WindowsOptions (runAsUserName / GMSA). Returns nil if
// nothing Windows-relevant remains.
func stripLinuxPodSecurityContext(sc *corev1.PodSecurityContext) *corev1.PodSecurityContext {
	if sc == nil || sc.WindowsOptions == nil {
		return nil
	}
	return &corev1.PodSecurityContext{WindowsOptions: sanitizeWindowsOptions(sc.WindowsOptions)}
}

// sanitizeWindowsOptions returns a copy of WindowsOptions with HostProcess cleared. This
// transform builds regular ServerCore containers; HostProcess containers require the pod's
// hostNetwork=true (which the strip clears), so preserving hostProcess would produce an invalid
// pod. runAsUserName / GMSA are kept.
func sanitizeWindowsOptions(opts *corev1.WindowsSecurityContextOptions) *corev1.WindowsSecurityContextOptions {
	if opts == nil {
		return nil
	}
	out := opts.DeepCopy()
	out.HostProcess = nil
	return out
}

// stripLinuxSecurityContext clears Linux-only container security settings, preserving only the
// Windows-compatible field, WindowsOptions (runAsUserName / GMSA). Every other field —
// capabilities, seccomp, SELinux, AppArmor, readOnlyRootFilesystem, and the *nix process
// identity/privilege fields (runAsUser/Group/NonRoot, privileged, allowPrivilegeEscalation,
// procMount) — is invalid on a Windows ServerCore container and would make the pod invalid if
// set via an override. Returns nil when nothing Windows-relevant remains.
func stripLinuxSecurityContext(sc *corev1.SecurityContext) *corev1.SecurityContext {
	if sc == nil || sc.WindowsOptions == nil {
		return nil
	}
	return &corev1.SecurityContext{WindowsOptions: sanitizeWindowsOptions(sc.WindowsOptions)}
}
