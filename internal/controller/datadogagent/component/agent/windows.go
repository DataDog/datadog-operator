// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Windows node support — CONTP-1448.
// ApplyWindowsPodTransformation turns an already-built Linux node-agent pod template into a
// Windows-compatible one (used on the DatadogAgentProfile + provider=windows path):
//   - nodeSelector: kubernetes.io/os=windows + toleration for node.kubernetes.io/os=windows:NoSchedule
//   - Windows agent image (servercore variant)
//   - Core agent + trace/process agent only (no system-probe/security-agent — Linux/eBPF only)
//   - An init container copies the image's conf.d (default check configs) + a datadog.yaml into a
//     shared emptyDir, which the agent containers then mount at C:/ProgramData/Datadog. This gives
//     the agents a single writable config dir that keeps the image's default checks AND lets the
//     core agent share its IPC auth token with the trace/process agents. (The copy is essential:
//     the agents override the image entrypoint with `agent run`/`trace-agent run`, so nothing
//     generates datadog.yaml at start, and the image ships conf.d but no datadog.yaml.)
//   - No Linux capabilities/seccomp/securityContext; no Linux hostPath mounts
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
	// windowsConfigVolumeName is the emptyDir shared across the agent containers on Windows,
	// mounted at the whole config dir C:/ProgramData/Datadog. An init container seeds it with a
	// copy of the image's conf.d (default check configs) + a datadog.yaml before the agents start
	// (see windowsConfigInitContainer), so it serves three purposes at once: it carries the
	// default checks, provides the datadog.yaml the trace/process agents require, and holds the
	// core agent's IPC auth token (under auth/) for the other agents to read.
	//
	// It must be the WHOLE config dir, not just auth/: when log collection is enabled the host
	// C:/ProgramData is mounted read-only (to follow pod-log symlinks), which would otherwise
	// hide the image's C:/ProgramData/Datadog. This emptyDir is mounted one level deeper, at
	// C:/ProgramData/Datadog, so it overlays that host mount and restores a writable config dir.
	windowsConfigVolumeName = "win-datadog-config"

	// windowsDatadogConfigPath is the Windows Agent config directory. The shared emptyDir is
	// mounted here on the agent containers (seeded by the init container with conf.d + datadog.yaml).
	windowsDatadogConfigPath = "C:/ProgramData/Datadog"

	// windowsConfigInitPath is where the init container stages the config into the shared emptyDir.
	// It is deliberately NOT C:/ProgramData/Datadog: mounting the emptyDir there in the init
	// container would hide the very image conf.d the init container needs to copy. Staging at a
	// separate path lets the init read the image's C:/ProgramData/Datadog and write the emptyDir.
	windowsConfigInitPath = "C:/config-init"

	// windowsAuthTokenFilePath is the IPC auth token path, inside the shared config dir's auth
	// subdir. It overrides the Linux default (DD_AUTH_TOKEN_FILE_PATH=/etc/datadog-agent/auth/token)
	// that commonEnvVars injects, so the token the core agent writes is visible to the trace agent.
	windowsAuthTokenFilePath = windowsDatadogConfigPath + "/auth/token"

	// Windows log-collection host paths. Pod logs are regular files under C:/var/log/pods on
	// containerd, so only C:/var/log + C:/var/log/pods are mounted; C:/ProgramData is deliberately
	// NOT mounted (it would overlap the config dir — see AddWindowsLogCollectionVolumes).
	windowsVarLogPath      = "C:/var/log"      // pointer dir (RW: agent stores log tail offsets)
	windowsPodLogsPath     = "C:/var/log/pods" // Kubernetes pod log files (regular files here)
	windowsProgramDataPath = "C:/ProgramData"  // config-dir parent; referenced only by tests

	windowsPointerVolumeName      = "win-pointerdir"
	windowsPodLogsVolumeName      = "win-logpodpath"
	windowsContainerLogVolumeName = "win-logcontainerpath"
)

// WindowsLogPaths holds the host log paths to mount on the Windows agent. Empty fields fall
// back to the Windows defaults. These come from features.logCollection.{tempStoragePath,
// podLogsPath,containerLogsPath} so custom Windows log paths take effect.
type WindowsLogPaths struct {
	TempStoragePath string // pointer dir (RW); default C:/var/log
	PodLogsPath     string // pod logs (RO);    default C:/var/log/pods
	// ContainerLogsPath is the container-runtime log store; only mounted when set to a
	// non-overlapping Windows path. See AddWindowsLogCollectionVolumes.
	ContainerLogsPath string
}

// AddWindowsLogCollectionVolumes adds the Windows host-log hostPath volumes + mounts to the core
// agent (the logcollection feature only injects the Linux paths, which the strip removes). It
// mounts tempStoragePath (default C:/var/log) and podLogsPath (default C:/var/log/pods).
//
// containerLogsPath (the container-runtime log store that pod logs may symlink into) has no
// default and is mounted only when set to a Windows path that does NOT overlap the config dir. On
// containerd (the supported Windows runtime) pod logs are regular files under C:/var/log/pods, so
// it is normally unset; defaulting it to C:/ProgramData would overlap C:/ProgramData/Datadog and
// rely on non-guaranteed Windows nested-mount overlay. Symlink-based runtimes set it to the
// store's own subdirectory (a sibling of the config dir, e.g. C:/ProgramData/docker/containers). A
// path that overlaps the config dir is skipped and returned (empty otherwise) so the caller can
// surface a condition instead of silently failing log collection.
//
// It is a no-op if there is no core agent container. Configured paths set the HOST path only; the
// in-container mount path is the Agent's standard Windows path.
func AddWindowsLogCollectionVolumes(spec *corev1.PodSpec, paths WindowsLogPaths) (skippedOverlappingContainerLogsPath string) {
	pointerHostPath := windowsPathOrDefault(paths.TempStoragePath, windowsVarLogPath)
	podLogsHostPath := windowsPathOrDefault(paths.PodLogsPath, windowsPodLogsPath)

	// Mount on the core agent, or the consolidated single agent under single-container strategy.
	idx := -1
	for i := range spec.Containers {
		if isWindowsMainAgentContainer(spec.Containers[i].Name) {
			idx = i
			break
		}
	}
	if idx == -1 {
		return ""
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
	)
	// Mount paths are fixed to the Agent's standard Windows paths regardless of the host path.
	spec.Containers[idx].VolumeMounts = append(spec.Containers[idx].VolumeMounts,
		corev1.VolumeMount{Name: windowsPointerVolumeName, MountPath: windowsVarLogPath},
		corev1.VolumeMount{Name: windowsPodLogsVolumeName, MountPath: windowsPodLogsPath, ReadOnly: true},
	)

	// Only mount the container-log store when the user explicitly configured a Windows path that
	// does not overlap the config dir (see the function doc). Never default it to C:/ProgramData.
	if isExplicitWindowsPath(paths.ContainerLogsPath) {
		if windowsPathOverlapsConfigDir(paths.ContainerLogsPath) {
			// Would re-shadow the seeded config; skip and report.
			skippedOverlappingContainerLogsPath = paths.ContainerLogsPath
		} else {
			spec.Volumes = append(spec.Volumes, hostPathVol(windowsContainerLogVolumeName, paths.ContainerLogsPath))
			spec.Containers[idx].VolumeMounts = append(spec.Containers[idx].VolumeMounts,
				corev1.VolumeMount{Name: windowsContainerLogVolumeName, MountPath: paths.ContainerLogsPath, ReadOnly: true},
			)
		}
	}

	// Force file-based container log collection on Windows. The only Windows log source we mount
	// is the file tree under C:/var/log/pods; runtime/socket-based collection has no Windows
	// container-runtime socket/pipe wired up, so if the user set containerCollectUsingFiles=false
	// the agent would collect nothing. Override to file mode.
	spec.Containers[idx].Env = setEnvVar(spec.Containers[idx].Env, "DD_LOGS_CONFIG_K8S_CONTAINER_USE_FILE", "true")
	return skippedOverlappingContainerLogsPath
}

// isExplicitWindowsPath reports whether v is a non-empty genuine Windows path (not a Linux
// "/"-prefixed default). Used to gate optional mounts that have no safe Windows default.
func isExplicitWindowsPath(v string) bool {
	return v != "" && !strings.HasPrefix(v, "/")
}

// windowsPathOverlapsConfigDir reports whether a host mount at p would overlap the agent config
// dir C:/ProgramData/Datadog — i.e. p is a parent of, equal to, or nested under it. Overlapping
// parent/child mounts on Windows are not a guaranteed overlay, so such a container-log path is
// skipped to avoid re-shadowing the seeded config.
func windowsPathOverlapsConfigDir(p string) bool {
	norm := func(s string) string {
		return strings.ToLower(strings.TrimRight(strings.ReplaceAll(s, "\\", "/"), "/"))
	}
	a, b := norm(p), norm(windowsDatadogConfigPath)
	return a == b || strings.HasPrefix(a+"/", b+"/") || strings.HasPrefix(b+"/", a+"/")
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
// /etc/passwd, seccomp, Unix sockets) are omitted. Additional volumes (named pipes for
// DogStatsD/APM) will be added as Windows feature support lands.
func volumesForWindowsAgent(_ metav1.Object) []corev1.Volume {
	return []corev1.Volume{
		// Shared config emptyDir seeded by the init container; see windowsConfigVolumeName.
		{
			Name:         windowsConfigVolumeName,
			VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
		},
	}
}

// volumeMountsForWindowsAgent returns the shared config-dir mount applied to every surviving
// agent container: the whole C:/ProgramData/Datadog (init-seeded conf.d + datadog.yaml + auth).
func volumeMountsForWindowsAgent() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		{Name: windowsConfigVolumeName, MountPath: windowsDatadogConfigPath},
	}
}

// windowsConfigInitContainer returns the init container that seeds the shared config emptyDir.
// It runs on the same (servercore) agent image so it has the image's C:/ProgramData/Datadog, and
// copies conf.d + the *.yaml config files into the emptyDir staged at windowsConfigInitPath, then
// ensures an auth/ dir and a datadog.yaml exist. The agent containers override the image entrypoint
// with `agent run`/`trace-agent run`, so nothing would otherwise generate datadog.yaml; and the
// image ships conf.d but no datadog.yaml. Config values still come from env vars (DD_*); the copied
// conf.d supplies the default checks, and the empty datadog.yaml just satisfies the file the
// trace/process agents load at startup.
func windowsConfigInitContainer(image string, pullPolicy corev1.PullPolicy) corev1.Container {
	// Mirror the image's whole config dir (C:/ProgramData/Datadog) into the shared emptyDir, then
	// ensure the auth dir + a datadog.yaml exist. robocopy /E is used (not Copy-Item) because it is:
	//   - idempotent: the emptyDir persists for the pod lifetime while the init container re-runs
	//     when the pod sandbox is recreated (e.g. node reboot); robocopy mirrors without the
	//     Copy-Item "copy dir into existing dir" nesting quirk (C:\...\conf.d\conf.d).
	//   - tolerant: a missing/empty source conf.d (custom/minimal images) is not an error, and we
	//     guard on the source dir existing so a truly minimal image still gets auth/ + datadog.yaml.
	//   - complete: mirroring the WHOLE dir (not just conf.d + top-level *.yaml) carries checks.d
	//     and any other image-provided config subdir, which the later full-dir mount would else hide.
	// robocopy exit codes 0-7 are success; >=8 is a real failure, so map those to a nonzero exit.
	script := "$ErrorActionPreference='Stop';" +
		" if (Test-Path C:\\ProgramData\\Datadog) {" +
		" robocopy C:\\ProgramData\\Datadog C:\\config-init /E /NFL /NDL /NJH /NJS /NP /R:2 /W:2 | Out-Null;" +
		" if ($LASTEXITCODE -ge 8) { exit $LASTEXITCODE } };" +
		" New-Item -ItemType Directory -Force -Path C:\\config-init\\auth | Out-Null;" +
		" if (!(Test-Path C:\\config-init\\datadog.yaml)) { New-Item -ItemType File -Path C:\\config-init\\datadog.yaml | Out-Null };" +
		" exit 0"
	return corev1.Container{
		Name:            "init-config-windows",
		Image:           image,
		ImagePullPolicy: pullPolicy,
		Command:         []string{"powershell", "-Command", script},
		VolumeMounts: []corev1.VolumeMount{
			{Name: windowsConfigVolumeName, MountPath: windowsConfigInitPath},
		},
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

// windowsAllowedInitContainerNames is the allowlist of Linux init containers to KEEP on Windows.
// It is empty: every Linux init container a feature injects (seccomp-setup, init-volume,
// init-config, …) is stripped. The Windows config-seed init container is added afterwards, in
// ApplyWindowsPodTransformation (see windowsConfigInitContainer), so it is not subject to this
// allowlist.
var windowsAllowedInitContainerNames = map[string]bool{}

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

// windowsAgentImageForInit returns the image + pull policy to use for the config-seed init
// container so it carries the image's conf.d and pulls consistently with the agent. It prefers
// the primary agent container (core or single), but falls back to any surviving agent container
// (trace/process also run the agent image) so the shared config dir is still seeded — the
// trace/process agents require the datadog.yaml the init creates even if the core agent was
// dropped. Returns ok=false only when no agent container survives at all.
func windowsAgentImageForInit(spec *corev1.PodSpec) (image string, pullPolicy corev1.PullPolicy, ok bool) {
	for i := range spec.Containers {
		if isWindowsMainAgentContainer(spec.Containers[i].Name) {
			return spec.Containers[i].Image, spec.Containers[i].ImagePullPolicy, true
		}
	}
	// No primary agent: fall back to the first surviving container (same agent image).
	if len(spec.Containers) > 0 {
		return spec.Containers[0].Image, spec.Containers[0].ImagePullPolicy, true
	}
	return "", "", false
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
//     seeds it with the image's conf.d + a datadog.yaml, mounted at C:/ProgramData/Datadog on the
//     surviving agent containers, plus the Windows container commands and the Windows auth-token
//     path.
//  3. Coerce the default agent image to the -servercore variant.
//  4. Add the Windows host-log hostPath mounts when log collection is enabled.
//  5. Force non-local traffic so APM/DogStatsD are reachable without Unix sockets.
//  6. Add the Windows nodeSelector + the node.kubernetes.io/os=windows:NoSchedule toleration.
//
// logPaths carries the configured logCollection paths (Windows defaults are applied for the
// Linux/empty values).
//
// It returns a configured logCollection.containerLogsPath that was skipped for overlapping the
// agent config dir (empty string otherwise), so the caller can surface a warning/condition.
func ApplyWindowsPodTransformation(tmpl *corev1.PodTemplateSpec, dda metav1.Object, logCollectionEnabled bool, logPaths WindowsLogPaths) (skippedOverlappingContainerLogsPath string) {
	StripLinuxOnlySettingsFromTemplate(tmpl)
	spec := &tmpl.Spec

	// Re-add the shared config volume (mounted at C:/ProgramData/Datadog on the agent containers)
	// so they share a single writable config dir carrying the default checks + auth token.
	spec.Volumes = append(spec.Volumes, volumesForWindowsAgent(dda)...)
	// Windows agent commands differ from Linux (default.go): no --config (agents use their default
	// config dir C:/ProgramData/Datadog, seeded by the init container), and the trace/process
	// agents use the "foreground" form so they run in the foreground rather than as a Windows
	// service. `trace-agent run --foreground` is verified live (reaches "Trace agent running",
	// :8126). process-agent only exists on older (<7.60) pinned agents; normally checks run in-core.
	for i := range spec.Containers {
		c := &spec.Containers[i]
		switch c.Name {
		case string(apicommon.CoreAgentContainerName):
			c.Command = []string{"agent", "run"}
			c.Env = overrideAuthTokenPath(c.Env)
			// DogStatsD has no Unix socket on Windows: accept UDP from other pods.
			c.Env = setEnvVar(c.Env, "DD_DOGSTATSD_NON_LOCAL_TRAFFIC", "true")
		case string(apicommon.TraceAgentContainerName):
			c.Command = []string{"trace-agent", "run", "--foreground"}
			c.Env = overrideAuthTokenPath(c.Env)
			// APM receiver must accept TCP from other pods (no Unix socket on Windows).
			c.Env = setEnvVar(c.Env, "DD_APM_NON_LOCAL_TRAFFIC", "true")
		case string(apicommon.ProcessAgentContainerName):
			c.Command = []string{"process-agent", "--foreground"}
			c.Env = overrideAuthTokenPath(c.Env)
		case string(apicommon.UnprivilegedSingleAgentContainerName):
			// Single-container strategy: one consolidated agent process. Treat it as the core
			// agent on Windows (runs `agent run`, needs the shared config + non-local traffic
			// for the in-process APM/DogStatsD). Without this it would be stripped, leaving a
			// container-less (invalid) pod.
			c.Command = []string{"agent", "run"}
			c.Env = overrideAuthTokenPath(c.Env)
			c.Env = setEnvVar(c.Env, "DD_DOGSTATSD_NON_LOCAL_TRAFFIC", "true")
			c.Env = setEnvVar(c.Env, "DD_APM_NON_LOCAL_TRAFFIC", "true")
		}
	}

	// Seed the shared config dir before the agents start: an init container copies the image's
	// conf.d + a datadog.yaml into the emptyDir (the agents override the entrypoint so nothing
	// else creates datadog.yaml, and the image ships no datadog.yaml). Use the agent image so the
	// init container has the image's conf.d + the same pull policy; EnsureWindowsServercoreImage
	// then coerces it to the servercore variant along with the agent containers.
	if img, pullPolicy, ok := windowsAgentImageForInit(spec); ok {
		spec.InitContainers = append(spec.InitContainers, windowsConfigInitContainer(img, pullPolicy))
	}

	EnsureWindowsServercoreImage(spec)

	if logCollectionEnabled {
		skippedOverlappingContainerLogsPath = AddWindowsLogCollectionVolumes(spec, logPaths)
	}

	// Append the config mount after the log mounts (defensive parent-before-child ordering); no
	// host mount overlaps the config dir by default. See AddWindowsLogCollectionVolumes.
	for i := range spec.Containers {
		if windowsAllowedContainerNames[spec.Containers[i].Name] {
			spec.Containers[i].VolumeMounts = append(spec.Containers[i].VolumeMounts, volumeMountsForWindowsAgent()...)
		}
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

	return skippedOverlappingContainerLogsPath
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
