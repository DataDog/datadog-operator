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
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/images"
)

const (
	// WindowsAgentResourceSuffix is the suffix used to name Windows agent resources.
	WindowsAgentResourceSuffix = "agent-windows"

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

// GetWindowsAgentName returns the Windows DaemonSet name for a given DatadogAgent.
func GetWindowsAgentName(dda metav1.Object) string {
	return fmt.Sprintf("%s-%s", dda.GetName(), WindowsAgentResourceSuffix)
}

// EnsureWindowsIntakeReachable forces non-local-traffic on for APM (trace-agent) and
// DogStatsD (core agent) so they bind reachable TCP/UDP listeners on Windows, where Unix
// sockets are unavailable. It MUST run after the feature loop and overrides: the DogStatsD
// feature writes DD_DOGSTATSD_NON_LOCAL_TRAFFIC unconditionally to the user's value
// (default false) with last-writer-wins merge, so setting it earlier (in the builder) would
// be clobbered. Applied only to containers that exist in the (already stripped) pod spec.
func EnsureWindowsIntakeReachable(managers feature.PodTemplateManagers) {
	for _, c := range managers.PodTemplateSpec().Spec.Containers {
		switch c.Name {
		case string(apicommon.CoreAgentContainerName):
			managers.EnvVar().AddEnvVarToContainer(apicommon.CoreAgentContainerName, &corev1.EnvVar{
				Name: "DD_DOGSTATSD_NON_LOCAL_TRAFFIC", Value: "true",
			})
		case string(apicommon.TraceAgentContainerName):
			managers.EnvVar().AddEnvVarToContainer(apicommon.TraceAgentContainerName, &corev1.EnvVar{
				Name: "DD_APM_NON_LOCAL_TRAFFIC", Value: "true",
			})
		}
	}
}

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

	idx := -1
	for i := range spec.Containers {
		if spec.Containers[i].Name == string(apicommon.CoreAgentContainerName) {
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
// applies spec.override.windowsNodeAgent.image: the generic OverrideAgentImage path replaces
// the whole tag (servercore is not modeled as a preserved suffix), so a user pinning
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
	if strings.Contains(image[colon+1:], "servercore") {
		return image
	}
	return image + images.WindowsServerCoreTagSuffix
}

// NewDefaultWindowsAgentDaemonset returns a new Windows-targeted DaemonSet.
// It is meant to run alongside the Linux DaemonSet when Windows nodes are present.
func NewDefaultWindowsAgentDaemonset(dda metav1.Object, edsOptions *ExtendedDaemonsetOptions, agentComponent feature.RequiredComponent, instanceName string) *appsv1.DaemonSet {
	daemonset := NewDaemonset(dda, edsOptions, WindowsAgentResourceSuffix, GetWindowsAgentName(dda), common.GetAgentVersion(dda), nil, instanceName)
	podTemplate := NewDefaultWindowsAgentPodTemplateSpec(dda, agentComponent, daemonset.GetLabels())
	daemonset.Spec.Template = *podTemplate
	return daemonset
}

// NewDefaultWindowsAgentPodTemplateSpec returns a Windows-compatible PodTemplateSpec.
func NewDefaultWindowsAgentPodTemplateSpec(dda metav1.Object, agentComponent feature.RequiredComponent, labels map[string]string) *corev1.PodTemplateSpec {
	return &corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels:      labels,
			Annotations: make(map[string]string),
		},
		Spec: corev1.PodSpec{
			// Windows does not support RunAsUser — omit PodSecurityContext entirely.
			SecurityContext:    nil,
			ServiceAccountName: getDefaultServiceAccountName(dda),
			InitContainers:     windowsInitContainers(images.GetLatestWindowsAgentImage()),
			Containers:         windowsAgentContainers(dda, agentComponent.Containers),
			Volumes:            volumesForWindowsAgent(dda),
			NodeSelector: map[string]string{
				"kubernetes.io/os": "windows",
			},
			Tolerations: []corev1.Toleration{
				{
					// All Kubernetes distributions automatically taint Windows nodes
					// with node.kubernetes.io/os=windows:NoSchedule via kubelet.
					Key:      "node.kubernetes.io/os",
					Operator: corev1.TolerationOpEqual,
					Value:    "windows",
					Effect:   corev1.TaintEffectNoSchedule,
				},
			},
		},
	}
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

// windowsAgentContainers returns the containers for the Windows DaemonSet.
// Only core agent and trace agent are included; system-probe and security-agent are
// Linux/eBPF only and have no Windows equivalent.
func windowsAgentContainers(dda metav1.Object, requiredContainers []apicommon.AgentContainerName) []corev1.Container {
	img := images.GetLatestWindowsAgentImage()

	containers := []corev1.Container{windowsCoreAgentContainer(dda, img)}

	for _, c := range requiredContainers {
		switch c {
		case apicommon.TraceAgentContainerName:
			containers = append(containers, windowsTraceAgentContainer(dda, img))
		case apicommon.ProcessAgentContainerName:
			containers = append(containers, windowsProcessAgentContainer(dda, img))
			// SystemProbe, SecurityAgent: skip — Linux/eBPF only.
		}
	}
	return containers
}

func windowsCoreAgentContainer(dda metav1.Object, img string) corev1.Container {
	return corev1.Container{
		Name:  string(apicommon.CoreAgentContainerName),
		Image: img,
		// "agent run" uses windowsDatadogConfigPath as the default config directory
		// on Windows (mounted from the shared init-populated volume).
		Command:        []string{"agent", "run"},
		Env:            windowsEnvVarsForCoreAgent(dda),
		VolumeMounts:   volumeMountsForWindowsCoreAgent(),
		LivenessProbe:  constants.GetDefaultLivenessProbe(),
		ReadinessProbe: constants.GetDefaultReadinessProbe(),
		// No security context: Linux capabilities, seccomp, and readOnlyRootFilesystem
		// are not supported by the Windows container runtime.
	}
}

func windowsTraceAgentContainer(dda metav1.Object, img string) corev1.Container {
	return corev1.Container{
		Name:  string(apicommon.TraceAgentContainerName),
		Image: img,
		// "trace-agent run --foreground": on the Windows servercore image, "run" IS the
		// trace-agent's own foreground subcommand (validated live — the container serves
		// :8126 and registers with the core agent). This differs from the Linux container,
		// which invokes "trace-agent --config=...". Do not "simplify" to drop "run": without
		// it the binary prints usage and exits. It reads config from windowsDatadogConfigPath
		// (shared with the core agent) so it can reach the auth_token written there.
		Command:      []string{"trace-agent", "run", "--foreground"},
		Env:          windowsEnvVarsForTraceAgent(dda),
		VolumeMounts: volumeMountsForWindowsTraceAgent(),
		// No security context.
	}
}

func windowsProcessAgentContainer(dda metav1.Object, img string) corev1.Container {
	return corev1.Container{
		Name:         string(apicommon.ProcessAgentContainerName),
		Image:        img,
		Command:      []string{"process-agent", "--foreground"},
		Env:          windowsCommonEnvVars(dda),
		VolumeMounts: volumeMountsForWindowsProcessAgent(),
		// No security context.
	}
}

// windowsCommonEnvVars returns commonEnvVars with DD_AUTH_TOKEN_FILE_PATH overridden
// to the Windows path. commonEnvVars sets the Linux default (/etc/datadog-agent/auth/token);
// on Windows both containers mount the shared vol at windowsDatadogConfigPath so the
// auth token lives there instead.
func windowsCommonEnvVars(dda metav1.Object) []corev1.EnvVar {
	return overrideAuthTokenPath(commonEnvVars(dda))
}

func windowsEnvVarsForCoreAgent(dda metav1.Object) []corev1.EnvVar {
	return overrideAuthTokenPath(envVarsForCoreAgent(dda))
}

func windowsEnvVarsForTraceAgent(dda metav1.Object) []corev1.EnvVar {
	return overrideAuthTokenPath(envVarsForTraceAgent(dda))
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
	string(apicommon.CoreAgentContainerName):    true,
	string(apicommon.TraceAgentContainerName):   true,
	string(apicommon.ProcessAgentContainerName): true,
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

func stripLinuxOnlyFromSpec(spec *corev1.PodSpec, annotations *map[string]string) {
	// 0. Clear Linux pod-level namespace-sharing fields. Features can set these (e.g.
	//    DogStatsD origin detection sets hostPID=true); they are Linux-only and produce
	//    a Windows-invalid PodSpec.
	spec.HostPID = false
	spec.HostIPC = false

	// 1. Keep only allowlisted main containers; drop their Linux mounts, Unix-socket
	//    env vars, and security context.
	var keepContainers []corev1.Container
	for _, c := range spec.Containers {
		if !windowsAllowedContainerNames[c.Name] {
			continue
		}
		c.VolumeMounts = stripLinuxMounts(c.VolumeMounts)
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
		c.VolumeMounts = stripLinuxMounts(c.VolumeMounts)
		c.Env = stripWindowsIncompatibleEnvVars(c.Env)
		c.SecurityContext = stripLinuxSecurityContext(c.SecurityContext)
		keepInits = append(keepInits, c)
	}
	spec.InitContainers = keepInits

	// 3. Drop any volume no longer referenced by a surviving container's mounts.
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

	// 4. Remove pod annotations that reference now-removed containers (e.g. AppArmor
	//    profiles: container.apparmor.security.beta.kubernetes.io/<name>). Kubernetes
	//    rejects pods with annotations referencing non-existent containers.
	if annotations != nil && *annotations != nil {
		present := make(map[string]bool)
		for _, c := range spec.Containers {
			present[c.Name] = true
		}
		for _, c := range spec.InitContainers {
			present[c.Name] = true
		}
		for key := range *annotations {
			if containerName, ok := strings.CutPrefix(key, "container.apparmor.security.beta.kubernetes.io/"); ok {
				if !present[containerName] {
					delete(*annotations, key)
				}
			}
		}
	}
}

// stripLinuxMounts removes any volume mount whose MountPath is a Linux absolute path
// (starts with "/"). Windows mounts use C:/... drive-letter paths, so this is safe and
// catches hostPath mounts, emptyDir mounts at Linux paths, and config mounts uniformly.
func stripLinuxMounts(mounts []corev1.VolumeMount) []corev1.VolumeMount {
	var keep []corev1.VolumeMount
	for _, m := range mounts {
		if strings.HasPrefix(m.MountPath, "/") {
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
		if strings.Contains(e.Name, "SOCKET") || linuxOnlyEnvVarNames[e.Name] {
			continue
		}
		keep = append(keep, e)
	}
	return keep
}

func stripLinuxSecurityContext(sc *corev1.SecurityContext) *corev1.SecurityContext {
	if sc == nil {
		return nil
	}
	stripped := sc.DeepCopy()
	// Linux capabilities not supported on Windows.
	stripped.Capabilities = nil
	// seccomp profiles are Linux-only.
	stripped.SeccompProfile = nil
	// SELinux is Linux-only; Windows kubelet rejects pods with SELinuxOptions set.
	stripped.SELinuxOptions = nil
	// AppArmor profiles are Linux-only (added in k8s 1.30).
	stripped.AppArmorProfile = nil
	// readOnlyRootFilesystem is not supported by the Windows container runtime.
	stripped.ReadOnlyRootFilesystem = nil
	// Return nil if nothing meaningful remains.
	if stripped.RunAsUser == nil && stripped.RunAsGroup == nil &&
		stripped.RunAsNonRoot == nil && stripped.AllowPrivilegeEscalation == nil &&
		stripped.Privileged == nil && stripped.ProcMount == nil &&
		stripped.WindowsOptions == nil {
		return nil
	}
	return stripped
}
