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

	preparedRolloutLockVolume = "agent-rollout-locks"
	preparedRolloutLockDir    = "/var/run/datadog-agent-rollout"
	preparedRolloutGateBinary = "/opt/datadog-agent/embedded/bin/agent-rollout-gate"
	preparedRolloutAuthToken  = "/etc/datadog-agent/auth/token"

	rolloutPodUIDEnv      = "DD_EXPERIMENTAL_NODE_AGENT_ROLLOUT_POD_UID"
	coreAgentCmdPortEnv   = "DD_CMD_PORT"
	rolloutPodIPEnv       = "DD_EXPERIMENTAL_NODE_AGENT_ROLLOUT_POD_IP"
	greenCoreAgentCmdPort = int32(5002)

	maxStartupProbeFailures          = int32(2147483647)
	preparedMarkerProbePeriodSeconds = int32(5)
	defaultStartupFailureThreshold   = int32(3)
	defaultTerminationGraceSeconds   = int64(30)
)

var preparedRolloutContainerNames = map[string]struct{}{
	string(apicommon.CoreAgentContainerName):      {},
	string(apicommon.TraceAgentContainerName):     {},
	string(apicommon.ProcessAgentContainerName):   {},
	string(apicommon.SecurityAgentContainerName):  {},
	string(apicommon.SystemProbeContainerName):    {},
	string(apicommon.HostProfiler):                {},
	string(apicommon.OtelAgent):                   {},
	string(apicommon.AgentDataPlaneContainerName): {},
	string(apicommon.FlightRecorderContainerName): {},
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
	if !spec.HostNetwork {
		for _, container := range append(slices.Clone(spec.Containers), spec.InitContainers...) {
			for _, port := range container.Ports {
				if port.HostPort != 0 {
					return fmt.Errorf("prepared Agent rollout does not support hostPort %d on pod-networked container %q", port.HostPort, container.Name)
				}
			}
		}
	} else {
		for _, container := range append(slices.Clone(spec.Containers), spec.InitContainers...) {
			for _, port := range container.Ports {
				if port.HostPort != 0 && port.HostPort != port.ContainerPort {
					return fmt.Errorf("prepared Agent rollout cannot preserve hostPort %d to containerPort %d mapping on host-networked container %q", port.HostPort, port.ContainerPort, container.Name)
				}
			}
		}
	}
	if err := addPreparedRolloutLockVolume(spec); err != nil {
		return err
	}
	for i := range spec.Containers {
		container := &spec.Containers[i]
		wrapPreparedContainerCommand(container)
		configurePreparedContainer(container)
		configurePreparedProbes(container, spec.TerminationGracePeriodSeconds)
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{Name: preparedRolloutLockVolume, MountPath: preparedRolloutLockDir})
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
	container.Command = []string{preparedRolloutGateBinary}
	gateArgs := []string{"--component", container.Name}
	// The core creates the token. Flight Recorder is independent of the core
	// and intentionally has no auth-volume mount, so waiting for that path would
	// leave an otherwise prepared Flight Recorder asleep forever.
	if container.Name != string(apicommon.CoreAgentContainerName) && container.Name != string(apicommon.FlightRecorderContainerName) {
		gateArgs = append(gateArgs, "--wait-file", preparedRolloutAuthToken)
	}
	gateArgs = append(gateArgs, "--")
	container.Args = append(gateArgs, original...)
}

func addPreparedRolloutLockVolume(spec *corev1.PodSpec) error {
	for i := range spec.Volumes {
		if spec.Volumes[i].Name == preparedRolloutLockVolume {
			return fmt.Errorf("prepared Agent rollout volume name %q is reserved", preparedRolloutLockVolume)
		}
	}
	for _, container := range append(slices.Clone(spec.Containers), spec.InitContainers...) {
		for _, mount := range container.VolumeMounts {
			if mount.Name == preparedRolloutLockVolume || mount.MountPath == preparedRolloutLockDir || strings.HasPrefix(preparedRolloutLockDir, strings.TrimRight(mount.MountPath, "/")+"/") {
				return fmt.Errorf("prepared Agent rollout lock mount conflicts with container %q", container.Name)
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
	return nil
}

func validatePreparedContainers(spec *corev1.PodSpec) error {
	for i := range spec.Containers {
		container := &spec.Containers[i]
		if findContainerEnv(container, coreAgentCmdPortEnv) != nil {
			return fmt.Errorf("prepared Agent rollout does not support an explicit %s override because the green slot reserves port %d", coreAgentCmdPortEnv, greenCoreAgentCmdPort)
		}
		if _, ok := preparedRolloutContainerNames[container.Name]; !ok {
			return fmt.Errorf("prepared Agent rollout does not support container %q", container.Name)
		}
		if container.Lifecycle != nil {
			return fmt.Errorf("prepared Agent rollout does not support lifecycle hooks on container %q", container.Name)
		}
		if !preparedContainerCommandSupported(container) {
			return fmt.Errorf("prepared Agent rollout does not support command %q on container %q", container.Command, container.Name)
		}
		if err := validatePreparedProbes(container); err != nil {
			return err
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
			strings.Contains(container.Command[2], "/etc/dd-host-profiler/logging-seccomp.json") &&
			strings.Contains(container.Command[2], "/etc/dd-host-profiler/seccomp.json") &&
			strings.Contains(container.Command[2], agentcommon.SeccompRootVolumePath+"/host-profiler-")
	default:
		return false
	}
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

func validatePreparedProbes(container *corev1.Container) error {
	for _, entry := range []struct {
		name  string
		probe *corev1.Probe
	}{
		{name: "startup", probe: container.StartupProbe},
		{name: "liveness", probe: container.LivenessProbe},
		{name: "readiness", probe: container.ReadinessProbe},
	} {
		if entry.probe == nil {
			continue
		}
		if err := validateDelegatedProbe(entry.name, entry.probe); err != nil {
			return probeValidationError(container.Name, err)
		}
	}
	return nil
}

func probeValidationError(container string, err error) error {
	return fmt.Errorf("prepared Agent rollout does not support probes on container %q: %w", container, err)
}

func validateDelegatedProbe(name string, probe *corev1.Probe) error {
	if probe.HTTPGet != nil {
		if probe.TCPSocket != nil || probe.Exec != nil || probe.GRPC != nil {
			return fmt.Errorf("%s probe must have exactly one handler", name)
		}
		if probe.HTTPGet.Host != "" {
			return fmt.Errorf("%s HTTP host must be empty", name)
		}
		if probe.HTTPGet.Scheme != "" && probe.HTTPGet.Scheme != corev1.URISchemeHTTP {
			return fmt.Errorf("%s HTTP scheme must be HTTP", name)
		}
		if probe.HTTPGet.Path == "" || !strings.HasPrefix(probe.HTTPGet.Path, "/") {
			return fmt.Errorf("%s HTTP path must be absolute", name)
		}
		if len(probe.HTTPGet.HTTPHeaders) != 0 {
			return fmt.Errorf("%s HTTP headers are not supported", name)
		}
		if err := validateNumericProbePort(probe.HTTPGet.Port); err != nil {
			return fmt.Errorf("%s %w", name, err)
		}
		return nil
	}
	if probe.TCPSocket != nil {
		if probe.Exec != nil || probe.GRPC != nil {
			return fmt.Errorf("%s probe must have exactly one handler", name)
		}
		if probe.TCPSocket.Host != "" {
			return fmt.Errorf("%s TCP host must be empty", name)
		}
		if err := validateNumericProbePort(probe.TCPSocket.Port); err != nil {
			return fmt.Errorf("%s %w", name, err)
		}
		return nil
	}
	return fmt.Errorf("%s probe must use HTTP or TCP", name)
}
func validateNumericProbePort(port intstr.IntOrString) error {
	if port.Type != intstr.Int || port.IntVal <= 0 {
		return fmt.Errorf("port must be a positive number because declared named ports are removed")
	}
	return nil
}

func preparedContainerCommandSupported(container *corev1.Container) bool {
	if len(container.Command) == 0 {
		return false
	}
	expected := map[string]string{
		string(apicommon.CoreAgentContainerName):      "agent",
		string(apicommon.TraceAgentContainerName):     "trace-agent",
		string(apicommon.ProcessAgentContainerName):   "process-agent",
		string(apicommon.SecurityAgentContainerName):  "security-agent",
		string(apicommon.SystemProbeContainerName):    "system-probe",
		string(apicommon.HostProfiler):                "host-profiler",
		string(apicommon.OtelAgent):                   "otel-agent",
		string(apicommon.AgentDataPlaneContainerName): "agent-data-plane",
		string(apicommon.FlightRecorderContainerName): "/opt/datadog-agent/embedded/bin/flightrecorder",
	}
	want := expected[container.Name]
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

func configurePreparedContainer(container *corev1.Container) {
	setContainerEnv(container, corev1.EnvVar{
		Name: rolloutPodUIDEnv,
		ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{
			APIVersion: "v1",
			FieldPath:  "metadata.uid",
		}},
	})
	// Kubelet network probes with an empty host target status.podIP, not
	// loopback. The exec-based gate delegates to the same address so enabling a
	// prepared rollout does not silently change probe semantics (including IPv6).
	setContainerEnv(container, corev1.EnvVar{
		Name: rolloutPodIPEnv,
		ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{
			APIVersion: "v1",
			FieldPath:  "status.podIP",
		}},
	})
}

func configurePreparedProbes(container *corev1.Container, podTerminationGraceSeconds *int64) {
	originalStartup := container.StartupProbe
	startup := originalStartup.DeepCopy()
	if startup == nil {
		startup = &corev1.Probe{PeriodSeconds: preparedMarkerProbePeriodSeconds, TimeoutSeconds: 1}
	}
	startup.ProbeHandler = preparedStartupProbe(container.Name, originalStartup, podTerminationGraceSeconds)
	// The startup probe remains unsuccessful while the process is Prepared, so
	// Kubernetes keeps the container Unready without running its liveness or
	// readiness probes. The effectively infinite failure budget prevents a
	// sleeping replacement from being restarted. Once Active, any original
	// startup health check runs before Kubernetes restores the container's
	// unchanged liveness and readiness behavior.
	startup.FailureThreshold = maxStartupProbeFailures
	container.StartupProbe = startup
}

func preparedStartupProbe(component string, original *corev1.Probe, podTerminationGraceSeconds *int64) corev1.ProbeHandler {
	command := []string{preparedRolloutGateBinary, "probe", "--component", component, "--kind", "startup"}
	if original == nil {
		command = append(command, "--handler", "active")
	} else if original.HTTPGet != nil {
		command = append(command,
			"--handler", "http",
			"--port", original.HTTPGet.Port.String(),
			"--path", original.HTTPGet.Path,
			"--timeout", preparedNetworkProbeTimeout(original.TimeoutSeconds),
		)
	} else {
		command = append(command,
			"--handler", "tcp",
			"--port", original.TCPSocket.Port.String(),
			"--timeout", preparedNetworkProbeTimeout(original.TimeoutSeconds),
		)
	}
	if original != nil {
		failureThreshold := original.FailureThreshold
		if failureThreshold <= 0 {
			failureThreshold = defaultStartupFailureThreshold
		}
		terminationGrace := defaultTerminationGraceSeconds
		if podTerminationGraceSeconds != nil && *podTerminationGraceSeconds > 0 {
			terminationGrace = *podTerminationGraceSeconds
		}
		if original.TerminationGracePeriodSeconds != nil && *original.TerminationGracePeriodSeconds > 0 {
			terminationGrace = *original.TerminationGracePeriodSeconds
		}
		command = append(command,
			"--failure-threshold", fmt.Sprint(failureThreshold),
			"--termination-grace-period", fmt.Sprintf("%ds", terminationGrace),
		)
	}
	return corev1.ProbeHandler{Exec: &corev1.ExecAction{Command: command}}
}

func preparedNetworkProbeTimeout(seconds int32) string {
	if seconds <= 0 {
		seconds = 1
	}
	return fmt.Sprintf("%dms", seconds*900)
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
