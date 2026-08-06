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
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/constants"
)

const (
	preparedRolloutModeAnnotation  = "experimental.agent.datadoghq.com/node-agent-rollout-mode"
	preparedRolloutModeV3          = "prepared-surge-v3"
	preparedRolloutArmedAnnotation = "experimental.agent.datadoghq.com/node-agent-rollout-armed"
	preparedRolloutArmedV3         = "v3"

	preparedRolloutLockVolume = "agent-rollout-locks"
	preparedRolloutLockDir    = "/var/run/datadog-agent-rollout"
	preparedRolloutGateBinary = "/opt/datadog-agent/embedded/bin/agent-rollout-gate"
	preparedRolloutAuthToken  = "/etc/datadog-agent/auth/token"

	rolloutPodUIDEnv       = "DD_EXPERIMENTAL_NODE_AGENT_ROLLOUT_POD_UID"
	rolloutLockPathEnv     = "DD_EXPERIMENTAL_NODE_AGENT_ROLLOUT_LOCK_PATH"
	rolloutPreparedPathEnv = "DD_EXPERIMENTAL_NODE_AGENT_ROLLOUT_PREPARED_PATH"

	maxStartupProbeFailures = int32(2147483647)
)

var preparedRolloutContainerNames = map[string]struct{}{
	string(apicommon.CoreAgentContainerName):           {},
	string(apicommon.TraceAgentContainerName):          {},
	string(apicommon.ProcessAgentContainerName):        {},
	string(apicommon.SecurityAgentContainerName):       {},
	string(apicommon.SystemProbeContainerName):         {},
	string(apicommon.HostProfiler):                     {},
	string(apicommon.OtelAgent):                        {},
	string(apicommon.PrivateActionRunnerContainerName): {},
}

var preparedRolloutInitContainerNames = map[string]struct{}{
	string(apicommon.InitVolumeContainerName): {},
	string(apicommon.InitConfigContainerName): {},
	"seccomp-setup":               {},
	"host-profiler-seccomp-setup": {},
}

func preparedRolloutEnabled(ddai *datadoghqv1alpha1.DatadogAgentInternal) bool {
	return ddai != nil && ddai.Annotations[preparedRolloutModeAnnotation] == preparedRolloutModeV3
}

// configurePreparedRollout enables native DaemonSet surge. Existing profile
// DaemonSets need one ordinary rollout to narrow their anti-affinity before two
// revisions can share a node.
func configurePreparedRollout(ddai *datadoghqv1alpha1.DatadogAgentInternal, ds, current *appsv1.DaemonSet, budget intstr.IntOrString) (bool, error) {
	if !preparedRolloutEnabled(ddai) {
		return false, nil
	}
	if !positiveIntOrPercent(&budget) {
		return false, fmt.Errorf("prepared Agent rollout requires a positive, valid maxUnavailable budget")
	}
	if ds.Spec.UpdateStrategy.Type != "" && ds.Spec.UpdateStrategy.Type != appsv1.RollingUpdateDaemonSetStrategyType {
		return false, fmt.Errorf("prepared Agent rollout requires RollingUpdate strategy")
	}
	prepared := ds.DeepCopy()
	if err := prepareAgentTemplate(prepared); err != nil {
		return false, err
	}
	if !configurePreparedSurge(prepared, budget) {
		return false, fmt.Errorf("prepared Agent rollout requires a positive, valid maxUnavailable budget")
	}

	if current != nil && preparedRolloutArmingPending(current) {
		armingTemplate := prepared.Spec.Template.DeepCopy()
		delete(armingTemplate.Annotations, preparedRolloutModeAnnotation)
		armingTemplate.Annotations[preparedRolloutArmedAnnotation] = preparedRolloutArmedV3
		ds.Spec.Template = *armingTemplate
		configureConventionalMigration(ds, budget)
		return true, nil
	}

	ds.Spec.Template = prepared.Spec.Template
	ds.Spec.UpdateStrategy = prepared.Spec.UpdateStrategy
	return false, nil
}

func preparedRolloutArmingPending(current *appsv1.DaemonSet) bool {
	if hasRolloutMode(current.Spec.Template.Annotations) {
		return false
	}
	// Existing containers do not own component locks. Roll them once through
	// the lightweight wrapper before allowing a second generation onto a node.
	return current.Spec.Template.Annotations[preparedRolloutArmedAnnotation] != preparedRolloutArmedV3 || !daemonSetFullyRolledOut(current)
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
	if !prepareProfileAntiAffinityForSurge(&ds.Spec.Template) {
		return fmt.Errorf("prepared Agent rollout does not support custom Pod anti-affinity")
	}
	if !spec.HostNetwork {
		return fmt.Errorf("prepared Agent rollout currently requires hostNetwork so the waiting replacement's network probes reach the old generation")
	}
	if err := addPreparedRolloutLockVolume(spec); err != nil {
		return err
	}
	for i := range spec.Containers {
		container := &spec.Containers[i]
		wrapPreparedContainerCommand(container)
		configurePreparedContainer(container)
		configurePreparedStartupProbe(container)
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{Name: preparedRolloutLockVolume, MountPath: preparedRolloutLockDir})
		if spec.HostNetwork {
			// On hostNetwork, declared container ports are scheduler host-port
			// claims. The process still binds the same node ports after its older
			// peer stops. Pod-networked containers keep their port metadata.
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
	ds.Spec.Template.Annotations[preparedRolloutModeAnnotation] = preparedRolloutModeV3
	return nil
}

func wrapPreparedContainerCommand(container *corev1.Container) {
	original := make([]string, 0, len(container.Command)+len(container.Args))
	original = append(original, container.Command...)
	original = append(original, container.Args...)
	container.Command = []string{preparedRolloutGateBinary}
	gateArgs := []string{"--component", container.Name}
	if container.Name != string(apicommon.CoreAgentContainerName) {
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
	for i := range spec.Containers {
		for _, mount := range spec.Containers[i].VolumeMounts {
			if mount.Name == preparedRolloutLockVolume || mount.MountPath == preparedRolloutLockDir || strings.HasPrefix(preparedRolloutLockDir, strings.TrimRight(mount.MountPath, "/")+"/") {
				return fmt.Errorf("prepared Agent rollout lock mount conflicts with container %q", spec.Containers[i].Name)
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
	}
	return nil
}

func validatePreparedProbes(container *corev1.Container) error {
	// After Prepared, host-network probes deliberately reach the listener owned
	// by the old generation. Restrict them to listeners owned by the matching
	// Agent component; an arbitrary healthy node port must not authorize deleting
	// the old Agent.
	switch container.Name {
	case string(apicommon.CoreAgentContainerName):
		startupPort, err := validateCoreHTTPProbe("startup", container.StartupProbe, "")
		if err != nil {
			return probeValidationError(container.Name, err)
		}
		if path := container.StartupProbe.HTTPGet.Path; path != constants.DefaultStartupProbeHTTPPath && path != constants.DefaultReadinessProbeHTTPPath {
			return probeValidationError(container.Name, fmt.Errorf("startup HTTP path must be %q or %q", constants.DefaultStartupProbeHTTPPath, constants.DefaultReadinessProbeHTTPPath))
		}
		for _, probe := range []struct {
			name  string
			path  string
			probe *corev1.Probe
		}{
			{name: "liveness", path: constants.DefaultLivenessProbeHTTPPath, probe: container.LivenessProbe},
			{name: "readiness", path: constants.DefaultReadinessProbeHTTPPath, probe: container.ReadinessProbe},
		} {
			port, err := validateCoreHTTPProbe(probe.name, probe.probe, probe.path)
			if err != nil {
				return probeValidationError(container.Name, err)
			}
			if port != startupPort {
				return probeValidationError(container.Name, fmt.Errorf("%s port %d differs from startup port %d", probe.name, port, startupPort))
			}
		}
		return nil

	case string(apicommon.TraceAgentContainerName):
		if err := validateTraceProbe("liveness", container.LivenessProbe); err != nil {
			return probeValidationError(container.Name, err)
		}
		if container.ReadinessProbe != nil {
			if err := validateTraceProbe("readiness", container.ReadinessProbe); err != nil {
				return probeValidationError(container.Name, err)
			}
		}
		return nil

	case string(apicommon.SystemProbeContainerName):
		if container.LivenessProbe != nil {
			if _, err := validateCoreHTTPProbe("liveness", container.LivenessProbe, constants.DefaultLivenessProbeHTTPPath); err != nil {
				return probeValidationError(container.Name, err)
			}
		}
		if container.ReadinessProbe != nil {
			return probeValidationError(container.Name, fmt.Errorf("readiness probe has no proven old-generation listener"))
		}
		return nil

	default:
		if container.LivenessProbe != nil || container.ReadinessProbe != nil {
			return probeValidationError(container.Name, fmt.Errorf("network probes have no proven old-generation listener"))
		}
		return nil
	}
}

func probeValidationError(container string, err error) error {
	return fmt.Errorf("prepared Agent rollout does not support probes on container %q: %w", container, err)
}

func validateCoreHTTPProbe(name string, probe *corev1.Probe, path string) (int32, error) {
	if probe == nil || probe.HTTPGet == nil {
		return 0, fmt.Errorf("%s probe must use the Agent HTTP listener", name)
	}
	if probe.HTTPGet.Host != "" {
		return 0, fmt.Errorf("%s HTTP host must be empty", name)
	}
	if probe.HTTPGet.Scheme != "" && probe.HTTPGet.Scheme != corev1.URISchemeHTTP {
		return 0, fmt.Errorf("%s HTTP scheme must be HTTP", name)
	}
	if path != "" && probe.HTTPGet.Path != path {
		return 0, fmt.Errorf("%s HTTP path must be %q", name, path)
	}
	if err := validateNumericProbePort(probe.HTTPGet.Port); err != nil {
		return 0, fmt.Errorf("%s %w", name, err)
	}
	return probe.HTTPGet.Port.IntVal, nil
}

func validateTraceProbe(name string, probe *corev1.Probe) error {
	if probe == nil || probe.TCPSocket == nil {
		return fmt.Errorf("%s probe must use the trace Agent TCP listener", name)
	}
	if err := validateNumericProbePort(probe.TCPSocket.Port); err != nil {
		return fmt.Errorf("%s %w", name, err)
	}
	return nil
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
		string(apicommon.CoreAgentContainerName):           "agent",
		string(apicommon.TraceAgentContainerName):          "trace-agent",
		string(apicommon.ProcessAgentContainerName):        "process-agent",
		string(apicommon.SecurityAgentContainerName):       "security-agent",
		string(apicommon.SystemProbeContainerName):         "system-probe",
		string(apicommon.HostProfiler):                     "host-profiler",
		string(apicommon.OtelAgent):                        "otel-agent",
		string(apicommon.PrivateActionRunnerContainerName): "/opt/datadog-agent/embedded/bin/privateactionrunner",
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
	setContainerEnv(container, corev1.EnvVar{Name: rolloutLockPathEnv, Value: preparedRolloutLockDir + "/" + container.Name + ".lock"})
	setContainerEnv(container, corev1.EnvVar{Name: rolloutPreparedPathEnv, Value: preparedRolloutLockDir + "/" + container.Name + ".prepared"})
	setContainerEnv(container, corev1.EnvVar{
		Name: rolloutPodUIDEnv,
		ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{
			APIVersion: "v1",
			FieldPath:  "metadata.uid",
		}},
	})
}

func configurePreparedStartupProbe(container *corev1.Container) {
	probe := container.StartupProbe.DeepCopy()
	if probe == nil {
		probe = &corev1.Probe{PeriodSeconds: 1, TimeoutSeconds: 1}
	}
	probe.ProbeHandler = corev1.ProbeHandler{Exec: &corev1.ExecAction{Command: []string{
		"sh", "-c", `read uid pid fd < "$DD_EXPERIMENTAL_NODE_AGENT_ROLLOUT_PREPARED_PATH" && test "$uid" = "$DD_EXPERIMENTAL_NODE_AGENT_ROLLOUT_POD_UID" && test "/proc/$pid/fd/$fd" -ef "$DD_EXPERIMENTAL_NODE_AGENT_ROLLOUT_PREPARED_PATH"`,
	}}}
	// Waiting for an old generation is not a startup failure. Keep probing for
	// the lifetime of the container instead of restarting the prepared process.
	probe.FailureThreshold = maxStartupProbeFailures
	container.StartupProbe = probe
}

func containerRunsAsNonRoot(pod *corev1.PodSecurityContext, container *corev1.SecurityContext) bool {
	if container != nil {
		if container.RunAsNonRoot != nil && *container.RunAsNonRoot {
			return true
		}
		if container.RunAsUser != nil {
			return *container.RunAsUser != 0
		}
	}
	if pod != nil {
		if pod.RunAsNonRoot != nil && *pod.RunAsNonRoot {
			return true
		}
		if pod.RunAsUser != nil {
			return *pod.RunAsUser != 0
		}
	}
	return false
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

func hasRolloutMode(annotations map[string]string) bool {
	return strings.EqualFold(annotations[preparedRolloutModeAnnotation], preparedRolloutModeV3)
}
