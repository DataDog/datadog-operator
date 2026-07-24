// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// Copyright 2016-present Datadog, Inc.

package datadogagentinternal

import (
	"fmt"
	"path"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/util/intstr"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

const (
	preparedRolloutModeAnnotation = "experimental.agent.datadoghq.com/node-agent-rollout-mode"
	preparedRolloutModeV1         = "prepared-surge-v1"

	preparedRolloutStateVolume = "agent-rollout-state"
	preparedRolloutStateDir    = "/var/run/datadog-agent-rollout-state"

	rolloutEnabledEnv   = "DD_EXPERIMENTAL_NODE_AGENT_ROLLOUT_ENABLED"
	rolloutStatePathEnv = "DD_EXPERIMENTAL_NODE_AGENT_ROLLOUT_STATE_PATH"
	rolloutPodUIDEnv    = "DD_EXPERIMENTAL_NODE_AGENT_ROLLOUT_POD_UID"
)

var preparedRolloutContainerNames = []string{
	string(apicommon.CoreAgentContainerName),
	string(apicommon.TraceAgentContainerName),
}

func preparedRolloutEnabled(ddai *datadoghqv1alpha1.DatadogAgentInternal) bool {
	return ddai != nil && ddai.Annotations[preparedRolloutModeAnnotation] == preparedRolloutModeV1
}

// configurePreparedRollout enables native DaemonSet surge. Profile-managed
// DaemonSets first need one conventional affinity-only rollout because an old
// Pod's broad required anti-affinity also rejects an incoming replacement.
// The returned boolean is true while that prerequisite rollout is in progress.
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

	if current != nil && profileAffinityMigrationPending(current) {
		migrationTemplate := current.Spec.Template.DeepCopy()
		if apiequality.Semantic.DeepEqual(migrationTemplate.Spec.Affinity.PodAntiAffinity, broadAgentPodAntiAffinity()) {
			if !prepareProfileAntiAffinityForSurge(migrationTemplate) {
				return false, fmt.Errorf("prepared Agent rollout cannot migrate profile anti-affinity")
			}
		}
		ds.Spec.Template = *migrationTemplate
		configureConventionalMigration(ds, budget)
		return true, nil
	}

	ds.Spec.Template = prepared.Spec.Template
	ds.Spec.UpdateStrategy = prepared.Spec.UpdateStrategy
	return false, nil
}

func profileAffinityMigrationPending(current *appsv1.DaemonSet) bool {
	antiAffinity := current.Spec.Template.Spec.Affinity
	if antiAffinity == nil || antiAffinity.PodAntiAffinity == nil {
		return false
	}
	if apiequality.Semantic.DeepEqual(antiAffinity.PodAntiAffinity, broadAgentPodAntiAffinity()) {
		return true
	}
	expected, ok := profileSurgePodAntiAffinity(current.Spec.Template.Labels)
	return ok && apiequality.Semantic.DeepEqual(antiAffinity.PodAntiAffinity, expected) &&
		!hasRolloutMode(current.Spec.Template.Annotations) && !daemonSetFullyRolledOut(current)
}

func daemonSetFullyRolledOut(ds *appsv1.DaemonSet) bool {
	desired := ds.Status.DesiredNumberScheduled
	return desired > 0 && ds.Status.ObservedGeneration == ds.Generation &&
		ds.Status.UpdatedNumberScheduled == desired && ds.Status.NumberAvailable == desired && ds.Status.NumberUnavailable == 0
}

func prepareAgentTemplate(ds *appsv1.DaemonSet) error {
	spec := &ds.Spec.Template.Spec
	if spec.OS != nil && spec.OS.Name != corev1.Linux {
		return fmt.Errorf("prepared Agent rollout is Linux-only")
	}
	if spec.NodeSelector[corev1.LabelOSStable] == "windows" || spec.NodeSelector["beta.kubernetes.io/os"] == "windows" {
		return fmt.Errorf("prepared Agent rollout is Linux-only")
	}
	if err := validatePreparedContainers(spec); err != nil {
		return err
	}
	if !prepareProfileAntiAffinityForSurge(&ds.Spec.Template) {
		return fmt.Errorf("prepared Agent rollout does not support custom Pod anti-affinity")
	}
	if !spec.HostNetwork && podUsesHostPorts(spec) {
		return fmt.Errorf("prepared Agent rollout cannot overlap Pod-networked containers that declare hostPort")
	}
	if err := addPreparedRolloutStateVolume(spec); err != nil {
		return err
	}
	for i := range spec.Containers {
		container := &spec.Containers[i]
		if container.Name == string(apicommon.TraceAgentContainerName) {
			traceIndex := -1
			for commandIndex, command := range container.Command {
				if command == "trace-agent" {
					traceIndex = commandIndex
					break
				}
			}
			if traceIndex < 0 {
				return fmt.Errorf("prepared Agent rollout cannot bypass an unknown trace-agent loader command")
			}
			container.Command = append([]string(nil), container.Command[traceIndex:]...)
		}
		configurePreparedContainer(container)
		// With host networking, Kubernetes' scheduler treats declared container
		// ports as node-local claims. The process can still bind the same host
		// address after the older Pod exits without these declarations.
		if spec.HostNetwork {
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
	ds.Spec.Template.Annotations[preparedRolloutModeAnnotation] = preparedRolloutModeV1
	return nil
}

func podUsesHostPorts(spec *corev1.PodSpec) bool {
	for i := range spec.InitContainers {
		for _, port := range spec.InitContainers[i].Ports {
			if port.HostPort != 0 {
				return true
			}
		}
	}
	for i := range spec.Containers {
		for _, port := range spec.Containers[i].Ports {
			if port.HostPort != 0 {
				return true
			}
		}
	}
	return false
}

func validatePreparedContainers(spec *corev1.PodSpec) error {
	if len(spec.Containers) != len(preparedRolloutContainerNames) {
		return fmt.Errorf("prepared Agent rollout initially supports exactly agent and trace-agent containers")
	}
	seen := map[string]bool{}
	for i := range spec.Containers {
		container := &spec.Containers[i]
		if container.Name != string(apicommon.CoreAgentContainerName) && container.Name != string(apicommon.TraceAgentContainerName) {
			return fmt.Errorf("prepared Agent rollout does not support container %q", container.Name)
		}
		if seen[container.Name] {
			return fmt.Errorf("prepared Agent rollout found duplicate container %q", container.Name)
		}
		seen[container.Name] = true
		if container.Lifecycle != nil {
			return fmt.Errorf("prepared Agent rollout does not support lifecycle hooks on container %q", container.Name)
		}
		if len(container.Args) != 0 {
			return fmt.Errorf("prepared Agent rollout does not support command arguments on container %q", container.Name)
		}
		if container.Name == string(apicommon.CoreAgentContainerName) && (len(container.Command) != 2 || container.Command[0] != "agent" || container.Command[1] != "run") {
			return fmt.Errorf("prepared Agent rollout requires the standard agent run command")
		}
		for _, mount := range container.VolumeMounts {
			if mount.Name == preparedRolloutStateVolume || mountContainsPath(mount.MountPath, preparedRolloutStateDir) {
				return fmt.Errorf("prepared Agent rollout volume mount on container %q conflicts with a reserved name or path", container.Name)
			}
		}
	}
	if !seen[string(apicommon.CoreAgentContainerName)] || !seen[string(apicommon.TraceAgentContainerName)] {
		return fmt.Errorf("prepared Agent rollout requires agent and trace-agent containers")
	}

	if len(spec.InitContainers) != 2 {
		return fmt.Errorf("prepared Agent rollout initially supports only init-volume and init-config init containers")
	}
	seenInit := map[string]bool{}
	for i := range spec.InitContainers {
		container := &spec.InitContainers[i]
		if container.Name != string(apicommon.InitVolumeContainerName) && container.Name != string(apicommon.InitConfigContainerName) {
			return fmt.Errorf("prepared Agent rollout does not support init container %q", container.Name)
		}
		if container.Lifecycle != nil || len(container.Ports) != 0 {
			return fmt.Errorf("prepared Agent rollout does not support ports or lifecycle hooks on init container %q", container.Name)
		}
		seenInit[container.Name] = true
	}
	if !seenInit[string(apicommon.InitVolumeContainerName)] || !seenInit[string(apicommon.InitConfigContainerName)] {
		return fmt.Errorf("prepared Agent rollout requires init-volume and init-config")
	}
	return nil
}

func mountContainsPath(mountPath, target string) bool {
	mountPath = path.Clean(mountPath)
	target = path.Clean(target)
	return mountPath == "/" || target == mountPath || strings.HasPrefix(target, mountPath+"/")
}

func addPreparedRolloutStateVolume(spec *corev1.PodSpec) error {
	for i := range spec.Volumes {
		if spec.Volumes[i].Name == preparedRolloutStateVolume {
			return fmt.Errorf("prepared Agent rollout volume name %q is reserved", preparedRolloutStateVolume)
		}
	}
	spec.Volumes = append(spec.Volumes, corev1.Volume{
		Name:         preparedRolloutStateVolume,
		VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
	})
	return nil
}

func configurePreparedContainer(container *corev1.Container) {
	statePath := preparedRolloutStateDir + "/" + container.Name + ".state"
	originalLiveness := container.LivenessProbe.DeepCopy()
	originalReadiness := container.ReadinessProbe.DeepCopy()
	if originalReadiness == nil {
		originalReadiness = originalLiveness
	}
	setContainerEnv(container, corev1.EnvVar{Name: rolloutEnabledEnv, Value: "true"})
	setContainerEnv(container, corev1.EnvVar{Name: rolloutStatePathEnv, Value: statePath})
	setContainerEnv(container, corev1.EnvVar{
		Name: rolloutPodUIDEnv,
		ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{
			APIVersion: "v1",
			FieldPath:  "metadata.uid",
		}},
	})
	container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{Name: preparedRolloutStateVolume, MountPath: preparedRolloutStateDir})
	container.StartupProbe = rolloutStateProbe(statePath, "prepared|activating|active", 1, 300)
	container.LivenessProbe = rolloutHealthProbe(container.Name, statePath, originalLiveness)
	container.ReadinessProbe = rolloutHealthProbe(container.Name, statePath, originalReadiness)
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

func rolloutStateProbe(path, accepted string, period, failures int32) *corev1.Probe {
	command := rolloutStateReadCommand(path) + fmt.Sprintf(`case "$state" in %s) exit 0;; *) exit 1;; esac`, accepted)
	return &corev1.Probe{
		ProbeHandler:     corev1.ProbeHandler{Exec: &corev1.ExecAction{Command: []string{"sh", "-c", command}}},
		PeriodSeconds:    period,
		TimeoutSeconds:   1,
		FailureThreshold: failures,
	}
}

// rolloutHealthProbe deliberately treats Prepared as ready. That is the
// contract consumed by native DaemonSet maxSurge: images, init containers and
// the Agent graph are ready, while data-producing Fx hooks remain stopped.
func rolloutHealthProbe(containerName, statePath string, base *corev1.Probe) *corev1.Probe {
	activeHealth := "exit 1"
	switch containerName {
	case string(apicommon.CoreAgentContainerName):
		activeHealth = "exec /opt/datadog-agent/bin/agent/agent health"
	case string(apicommon.TraceAgentContainerName):
		activeHealth = "exec 3<>/dev/tcp/127.0.0.1/8126; exec 3>&-; exec 3<&-"
	}
	// Prepared may wait indefinitely for the old Pod. Activating must use the
	// normal liveness failure budget so a hung Fx start is eventually restarted.
	acceptedWaiting := "prepared) exit 0;; "
	command := rolloutStateReadCommand(statePath) + fmt.Sprintf(`case "$state" in %sactive) %s;; *) exit 1;; esac`, acceptedWaiting, activeHealth)

	probe := &corev1.Probe{PeriodSeconds: 10, TimeoutSeconds: 1, FailureThreshold: 3}
	if base != nil {
		probe = base.DeepCopy()
	}
	probe.ProbeHandler = corev1.ProbeHandler{Exec: &corev1.ExecAction{Command: []string{"bash", "-c", command}}}
	return probe
}

// rolloutStateReadCommand rejects state left by a previous container
// generation. EmptyDir volumes are Pod-lifetime, not container-lifetime, so
// every marker includes the writer PID and its Linux /proc start time.
func rolloutStateReadCommand(statePath string) string {
	return fmt.Sprintf(`read -r state pid started extra < %s || exit 1; [ -z "$extra" ] || exit 1; case "$pid:$started" in *[!0-9:]*|:*|*:) exit 1;; esac; procstat="$(cat /proc/$pid/stat 2>/dev/null)" || exit 1; procstat="${procstat##*) }"; set -- $procstat; [ "$#" -ge 20 ] && [ "${20}" = "$started" ] || exit 1; `, statePath)
}

func hasRolloutMode(annotations map[string]string) bool {
	return strings.EqualFold(annotations[preparedRolloutModeAnnotation], preparedRolloutModeV1)
}
