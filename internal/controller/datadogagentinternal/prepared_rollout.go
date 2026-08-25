// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// Copyright 2016-present Datadog, Inc.

package datadogagentinternal

import (
	"fmt"
	"slices"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	agentcommon "github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
)

const (
	preparedRolloutModeAnnotation  = "experimental.agent.datadoghq.com/node-agent-rollout-mode"
	preparedRolloutArmedAnnotation = "experimental.agent.datadoghq.com/node-agent-rollout-armed"

	preparedRolloutLockVolume     = "agent-rollout-locks"
	preparedRolloutLockDir        = "/var/run/datadog-agent-rollout"
	preparedRolloutStateVolume    = "agent-rollout-state"
	preparedRolloutStateDir       = "/var/run/datadog-agent-rollout-state"
	preparedRolloutToolsVolume    = "agent-rollout-tools"
	preparedRolloutToolsDir       = "/opt/datadog-agent-rollout"
	preparedRolloutToolInit       = "prepare-agent-rollout-tools"
	preparedRolloutToolSource     = "/usr/bin/s6-setlock"
	preparedRolloutSetlockBinary  = preparedRolloutToolsDir + "/s6-setlock"
	preparedRolloutTrueBinary     = "/usr/bin/true"
	preparedRolloutProbeShell     = "/bin/sh"
	preparedRolloutProbePeriodSec = int32(1)

	coreAgentCmdPortEnv   = "DD_CMD_PORT"
	greenCoreAgentCmdPort = int32(5002)

	maxStartupProbeFailures = int32(2147483647)
)

var preparedRolloutContainerCommands = map[string]string{
	string(apicommon.CoreAgentContainerName):           "agent",
	string(apicommon.TraceAgentContainerName):          "trace-agent",
	string(apicommon.ProcessAgentContainerName):        "process-agent",
	string(apicommon.SecurityAgentContainerName):       "security-agent",
	string(apicommon.SystemProbeContainerName):         "system-probe",
	string(apicommon.HostProfiler):                     "host-profiler",
	string(apicommon.OtelAgent):                        "otel-agent",
	string(apicommon.AgentDataPlaneContainerName):      "agent-data-plane",
	string(apicommon.FlightRecorderContainerName):      "/opt/datadog-agent/embedded/bin/flightrecorder",
	string(apicommon.PrivateActionRunnerContainerName): "/opt/datadog-agent/embedded/bin/privateactionrunner",
}

var preparedRolloutInitContainerNames = map[string]struct{}{
	string(apicommon.InitVolumeContainerName): {},
	string(apicommon.InitConfigContainerName): {},
	"seccomp-setup":               {},
	"host-profiler-seccomp-setup": {},
}

func daemonSetFullyRolledOut(ds *appsv1.DaemonSet) bool {
	desired := ds.Status.DesiredNumberScheduled
	return ds.Status.ObservedGeneration == ds.Generation &&
		ds.Status.UpdatedNumberScheduled == desired && ds.Status.NumberAvailable == desired && ds.Status.NumberUnavailable == 0
}

func prepareAgentTemplate(ds *appsv1.DaemonSet) error {
	spec := &ds.Spec.Template.Spec
	if spec.OS != nil && spec.OS.Name != corev1.Linux || spec.NodeSelector[corev1.LabelOSStable] != "" && spec.NodeSelector[corev1.LabelOSStable] != string(corev1.Linux) {
		return fmt.Errorf("prepared Agent rollout is Linux-only")
	}
	if spec.NodeSelector == nil {
		spec.NodeSelector = map[string]string{}
	}
	spec.NodeSelector[corev1.LabelOSStable] = string(corev1.Linux)
	if err := validatePreparedContainers(spec); err != nil {
		return err
	}
	if !prepareProfileAntiAffinityForOverlap(&ds.Spec.Template) {
		return fmt.Errorf("prepared Agent rollout does not support custom Pod anti-affinity")
	}
	for _, container := range append(slices.Clone(spec.Containers), spec.InitContainers...) {
		for _, port := range container.Ports {
			switch {
			case !spec.HostNetwork && port.HostPort != 0:
				return fmt.Errorf("prepared Agent rollout does not support hostPort %d on pod-networked container %q", port.HostPort, container.Name)
			case spec.HostNetwork && port.HostPort != 0 && port.HostPort != port.ContainerPort:
				return fmt.Errorf("prepared Agent rollout cannot preserve hostPort %d to containerPort %d mapping on host-networked container %q", port.HostPort, port.ContainerPort, container.Name)
			}
		}
	}
	if err := addPreparedRolloutVolumesAndTool(spec); err != nil {
		return err
	}
	for i := range spec.Containers {
		container := &spec.Containers[i]
		container.VolumeMounts = append(container.VolumeMounts,
			corev1.VolumeMount{Name: preparedRolloutLockVolume, MountPath: preparedRolloutLockDir},
			corev1.VolumeMount{Name: preparedRolloutStateVolume, MountPath: preparedRolloutStateDir},
			corev1.VolumeMount{Name: preparedRolloutToolsVolume, MountPath: preparedRolloutToolsDir, ReadOnly: true},
		)
		if spec.HostNetwork {
			// Kubernetes defaults declared containerPort entries on host-networked
			// Pods into host-port scheduling claims, even when hostPort is cleared.
			// Remove the declarations so the waiting generation may overlap; the
			// process still binds the same numeric node ports after lock handoff.
			container.Ports = nil
		}
	}
	if spec.HostNetwork {
		for i := range spec.InitContainers {
			spec.InitContainers[i].Ports = nil
		}
	}
	if ds.Spec.Template.Annotations == nil {
		ds.Spec.Template.Annotations = map[string]string{}
	}
	ds.Spec.Template.Annotations[preparedRolloutModeAnnotation] = preparedBlueGreenMode
	return nil
}

func wrapPreparedContainerCommand(container *corev1.Container) {
	original := make([]string, 0, len(container.Command)+len(container.Args))
	original = append(original, container.Command...)
	original = append(original, container.Args...)
	container.Command = []string{preparedRolloutSetlockBinary}
	container.Args = []string{
		preparedComponentLockPath(container.Name),
		preparedRolloutSetlockBinary,
		preparedActiveLockPath(container.Name),
	}
	container.Args = append(container.Args, original...)
}

func addPreparedRolloutVolumesAndTool(spec *corev1.PodSpec) error {
	coreImage := ""
	coreImagePullPolicy := corev1.PullIfNotPresent
	for i := range spec.Containers {
		if spec.Containers[i].Name == string(apicommon.CoreAgentContainerName) {
			coreImage = spec.Containers[i].Image
			coreImagePullPolicy = spec.Containers[i].ImagePullPolicy
			break
		}
	}
	if coreImage == "" {
		return fmt.Errorf("prepared Agent rollout requires the core Agent container image")
	}
	for i := range spec.Volumes {
		if spec.Volumes[i].Name == preparedRolloutLockVolume || spec.Volumes[i].Name == preparedRolloutStateVolume || spec.Volumes[i].Name == preparedRolloutToolsVolume {
			return fmt.Errorf("prepared Agent rollout volume name %q is reserved", spec.Volumes[i].Name)
		}
	}
	for _, container := range append(slices.Clone(spec.Containers), spec.InitContainers...) {
		for _, mount := range container.VolumeMounts {
			if mount.Name == preparedRolloutLockVolume || pathsOverlap(mount.MountPath, preparedRolloutLockDir) {
				return fmt.Errorf("prepared Agent rollout lock mount conflicts with container %q", container.Name)
			}
			if mount.Name == preparedRolloutStateVolume || pathsOverlap(mount.MountPath, preparedRolloutStateDir) {
				return fmt.Errorf("prepared Agent rollout state mount conflicts with container %q", container.Name)
			}
			if mount.Name == preparedRolloutToolsVolume || pathsOverlap(mount.MountPath, preparedRolloutToolsDir) {
				return fmt.Errorf("prepared Agent rollout tools mount conflicts with container %q", container.Name)
			}
		}
	}
	lockDirType := corev1.HostPathDirectoryOrCreate
	spec.Volumes = append(spec.Volumes, corev1.Volume{
		Name: preparedRolloutLockVolume,
		VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{
			Path: preparedRolloutLockDir,
			Type: &lockDirType,
		}},
	})
	spec.Volumes = append(spec.Volumes, corev1.Volume{
		Name:         preparedRolloutStateVolume,
		VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
	})
	spec.Volumes = append(spec.Volumes, corev1.Volume{
		Name:         preparedRolloutToolsVolume,
		VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
	})
	spec.InitContainers = append(spec.InitContainers, corev1.Container{
		Name:            preparedRolloutToolInit,
		Image:           coreImage,
		ImagePullPolicy: coreImagePullPolicy,
		Command:         []string{"/usr/bin/cp"},
		Args:            []string{preparedRolloutToolSource, preparedRolloutSetlockBinary},
		VolumeMounts: []corev1.VolumeMount{{
			Name:      preparedRolloutToolsVolume,
			MountPath: preparedRolloutToolsDir,
		}},
		SecurityContext: &corev1.SecurityContext{
			ReadOnlyRootFilesystem: new(true),
			RunAsNonRoot:           new(false),
			RunAsUser:              new(int64(0)),
		},
	})
	return nil
}

func pathsOverlap(left, right string) bool {
	left = strings.TrimRight(left, "/")
	right = strings.TrimRight(right, "/")
	return left == right || strings.HasPrefix(left, right+"/") || strings.HasPrefix(right, left+"/")
}

func preparedComponentLockPath(component string) string {
	return preparedRolloutLockDir + "/" + component + ".lock"
}

func preparedActiveLockPath(component string) string {
	return preparedRolloutStateDir + "/" + component + ".active.lock"
}

func configurePreparedStartupProbe(container *corev1.Container) {
	preservePreparedStartupWindow(container)
	activeLock := preparedActiveLockPath(container.Name)
	// s6-setlock returns 1 when -n finds a busy lock. A free lock runs true and
	// returns 0. The startup probe inverts those two results.
	command := fmt.Sprintf("%s -n %s %s; status=$?; [ \"$status\" -eq 1 ]", preparedRolloutSetlockBinary, activeLock, preparedRolloutTrueBinary)
	container.StartupProbe = &corev1.Probe{
		ProbeHandler:     corev1.ProbeHandler{Exec: &corev1.ExecAction{Command: []string{preparedRolloutProbeShell, "-c", command}}},
		PeriodSeconds:    preparedRolloutProbePeriodSec,
		TimeoutSeconds:   1,
		FailureThreshold: maxStartupProbeFailures,
	}
}

func preservePreparedStartupWindow(container *corev1.Container) {
	if container.LivenessProbe == nil {
		return
	}
	livenessFailures := probeValueOrDefault(container.LivenessProbe.FailureThreshold, 3)
	livenessPeriod := int64(probeValueOrDefault(container.LivenessProbe.PeriodSeconds, 10))
	preservedWindow := int64(container.LivenessProbe.InitialDelaySeconds) + livenessPeriod*int64(livenessFailures)
	if container.StartupProbe != nil {
		startupPeriod := probeValueOrDefault(container.StartupProbe.PeriodSeconds, 10)
		startupTimeout := probeValueOrDefault(container.StartupProbe.TimeoutSeconds, 1)
		startupFailures := probeValueOrDefault(container.StartupProbe.FailureThreshold, 3)
		startupInterval := max(startupPeriod, startupTimeout)
		startupWindow := int64(container.StartupProbe.InitialDelaySeconds) + int64(startupInterval)*int64(startupFailures) + int64(startupTimeout)
		preservedWindow = max(preservedWindow, startupWindow)
	}
	requiredFailures := (preservedWindow+livenessPeriod-1)/livenessPeriod + 1
	requiredFailures = min(requiredFailures, int64(maxStartupProbeFailures))
	if requiredFailures > int64(livenessFailures) {
		container.LivenessProbe.FailureThreshold = int32(requiredFailures)
	}
}

func probeValueOrDefault(value, fallback int32) int32 {
	if value == 0 {
		return fallback
	}
	return value
}

func validatePreparedContainers(spec *corev1.PodSpec) error {
	for i := range spec.Containers {
		container := &spec.Containers[i]
		if findContainerEnv(container, coreAgentCmdPortEnv) != nil {
			return fmt.Errorf("prepared Agent rollout does not support an explicit %s override because the green slot reserves port %d", coreAgentCmdPortEnv, greenCoreAgentCmdPort)
		}
		if _, ok := preparedRolloutContainerCommands[container.Name]; !ok {
			return fmt.Errorf("prepared Agent rollout does not support container %q", container.Name)
		}
		if container.Lifecycle != nil {
			return fmt.Errorf("prepared Agent rollout does not support lifecycle hooks on container %q", container.Name)
		}
		if !preparedContainerCommandSupported(container) {
			return fmt.Errorf("prepared Agent rollout does not support command %q on container %q", container.Command, container.Name)
		}
		if container.StartupProbe != nil && container.LivenessProbe == nil {
			return fmt.Errorf("prepared Agent rollout requires a liveness probe on container %q because its startup probe becomes the lock signal", container.Name)
		}
		if spec.HostNetwork {
			if err := validatePreparedProbePorts(container); err != nil {
				return err
			}
		}
		if containerRunsAsNonRoot(spec.SecurityContext, container.SecurityContext) {
			return fmt.Errorf("prepared Agent rollout requires container %q to run as root so it can create host-local lock files", container.Name)
		}
	}
	for i := range spec.InitContainers {
		container := &spec.InitContainers[i]
		if _, ok := preparedRolloutInitContainerNames[container.Name]; !ok {
			return fmt.Errorf("prepared Agent rollout does not support init container %q", container.Name)
		}
		if container.Lifecycle != nil {
			return fmt.Errorf("prepared Agent rollout does not support lifecycle hooks on init container %q", container.Name)
		}
		if !preparedInitCommandSupported(container) {
			return fmt.Errorf("prepared Agent rollout does not support command %q on init container %q", container.Command, container.Name)
		}
		if err := validatePreparedInitHostMounts(spec, container); err != nil {
			return err
		}
	}
	return nil
}

func findContainerEnv(container *corev1.Container, name string) *corev1.EnvVar {
	for i := range container.Env {
		if container.Env[i].Name == name {
			return &container.Env[i]
		}
	}
	return nil
}

func preparedInitCommandSupported(container *corev1.Container) bool {
	switch container.Name {
	case string(apicommon.InitVolumeContainerName), string(apicommon.InitConfigContainerName):
		return slices.Equal(container.Command, []string{"bash", "-c"}) && len(container.Args) == 1 && container.Args[0] != ""
	case "seccomp-setup":
		return len(container.Command) == 3 && container.Command[0] == "cp" &&
			container.Command[1] == agentcommon.SeccompSecurityVolumePath+"/"+agentcommon.SystemProbeSeccompKey &&
			container.Command[2] == agentcommon.SeccompRootVolumePath+"/"+agentcommon.SystemProbeSeccompProfileName && len(container.Args) == 0
	case "host-profiler-seccomp-setup":
		if len(container.Args) != 0 {
			return false
		}
		if len(container.Command) == 3 && container.Command[0] == "cp" {
			return container.Command[1] == "/etc/dd-host-profiler/seccomp.json" && preparedHostProfilerSeccompDestination(container.Command[2])
		}
		return len(container.Command) == 3 && container.Command[0] == "sh" && container.Command[1] == "-c" &&
			preparedHostProfilerLoggingSeccompCommand(container.Command[2])
	default:
		return false
	}
}

func preparedHostProfilerLoggingSeccompCommand(command string) bool {
	const (
		loggingSource = "/etc/dd-host-profiler/logging-seccomp.json"
		defaultSource = "/etc/dd-host-profiler/seccomp.json"
		warning       = "WARNING: logging-seccomp.json not found in image, falling back to default seccomp profile"
	)
	prefix := "if [ -f " + loggingSource + " ]; then cp " + loggingSource + " "
	middle := "; else echo '" + warning + "'; cp " + defaultSource + " "

	remainder, found := strings.CutPrefix(command, prefix)
	if !found {
		return false
	}
	destination, remainder, found := strings.Cut(remainder, middle)
	if !found {
		return false
	}
	fallbackDestination, found := strings.CutSuffix(remainder, "; fi")
	return found && destination == fallbackDestination && preparedHostProfilerGeneratedSeccompDestination(destination)
}

func preparedHostProfilerGeneratedSeccompDestination(destination string) bool {
	name := strings.TrimPrefix(destination, agentcommon.SeccompRootVolumePath+"/host-profiler-")
	if name == destination {
		return false
	}
	name = strings.TrimSuffix(name, "-logging")
	if len(name) != 8 {
		return false
	}
	for _, character := range name {
		if !strings.ContainsRune("0123456789abcdef", character) {
			return false
		}
	}
	return true
}

func preparedHostProfilerSeccompDestination(destination string) bool {
	name := strings.TrimPrefix(destination, agentcommon.SeccompRootVolumePath+"/")
	return name != destination && strings.HasPrefix(name, "host-profiler-") && !strings.Contains(name, "/")
}

func validatePreparedInitHostMounts(spec *corev1.PodSpec, container *corev1.Container) error {
	volumes := make(map[string]corev1.Volume, len(spec.Volumes))
	for i := range spec.Volumes {
		volumes[spec.Volumes[i].Name] = spec.Volumes[i]
	}
	for _, mount := range container.VolumeMounts {
		volume, found := volumes[mount.Name]
		if !found || volume.HostPath == nil || mount.ReadOnly {
			continue
		}
		seccompWriter := (container.Name == "seccomp-setup" || container.Name == "host-profiler-seccomp-setup") &&
			volume.HostPath.Path == agentcommon.SeccompRootPath && mount.MountPath == agentcommon.SeccompRootVolumePath
		if !seccompWriter {
			return fmt.Errorf("prepared Agent rollout does not support writable hostPath %q on init container %q before handoff", volume.HostPath.Path, container.Name)
		}
	}
	return nil
}

func validatePreparedProbePorts(container *corev1.Container) error {
	for _, entry := range []struct {
		name  string
		probe *corev1.Probe
	}{
		{name: "liveness", probe: container.LivenessProbe},
		{name: "readiness", probe: container.ReadinessProbe},
	} {
		if entry.probe == nil {
			continue
		}
		var port *intstr.IntOrString
		if entry.probe.HTTPGet != nil {
			port = &entry.probe.HTTPGet.Port
		} else if entry.probe.TCPSocket != nil {
			port = &entry.probe.TCPSocket.Port
		}
		if port != nil && port.Type != intstr.Int {
			return fmt.Errorf("prepared Agent rollout does not support named %s probe port %q on host-networked container %q because declared ports are removed", entry.name, port.StrVal, container.Name)
		}
	}
	return nil
}

func preparedContainerCommandSupported(container *corev1.Container) bool {
	if len(container.Command) == 0 {
		return false
	}
	want := preparedRolloutContainerCommands[container.Name]
	if container.Name == string(apicommon.TraceAgentContainerName) {
		return slices.Contains(container.Command, want)
	}
	if container.Name == string(apicommon.SecurityAgentContainerName) {
		return len(container.Command) >= 2 && container.Command[0] == want && container.Command[1] == "start"
	}
	if container.Command[0] != want {
		return false
	}
	if container.Name == string(apicommon.HostProfiler) {
		return slices.ContainsFunc(container.Command[1:], func(arg string) bool {
			return strings.HasPrefix(arg, "--core-config=")
		})
	}
	return true
}

func containerRunsAsNonRoot(pod *corev1.PodSecurityContext, container *corev1.SecurityContext) bool {
	runAsNonRoot := (*bool)(nil)
	runAsUser := (*int64)(nil)
	if pod != nil {
		runAsNonRoot = pod.RunAsNonRoot
		runAsUser = pod.RunAsUser
	}
	if container != nil {
		if container.RunAsNonRoot != nil {
			runAsNonRoot = container.RunAsNonRoot
		}
		if container.RunAsUser != nil {
			runAsUser = container.RunAsUser
		}
	}
	return runAsNonRoot != nil && *runAsNonRoot || runAsUser != nil && *runAsUser != 0
}

func setContainerEnv(container *corev1.Container, env corev1.EnvVar) {
	for i := range container.Env {
		if container.Env[i].Name == env.Name {
			container.Env[i] = env
			return
		}
	}
	container.Env = append(container.Env, env)
}
