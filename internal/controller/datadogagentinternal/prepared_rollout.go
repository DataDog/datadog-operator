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
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/util/intstr"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

const (
	preparedRolloutModeAnnotation = "experimental.agent.datadoghq.com/node-agent-rollout-mode"
	preparedRolloutModeV1         = "prepared-surge-v1"

	rolloutEnabledEnv = "DD_EXPERIMENTAL_NODE_AGENT_ROLLOUT_ENABLED"
	rolloutPodUIDEnv  = "DD_EXPERIMENTAL_NODE_AGENT_ROLLOUT_POD_UID"
	kubeletHostEnv    = "DD_KUBERNETES_KUBELET_HOST"
)

var preparedRolloutContainerNames = map[string]struct{}{
	string(apicommon.CoreAgentContainerName):           {},
	string(apicommon.TraceAgentContainerName):          {},
	string(apicommon.ProcessAgentContainerName):        {},
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
	return ddai != nil && ddai.Annotations[preparedRolloutModeAnnotation] == preparedRolloutModeV1
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

	if current != nil && profileAffinityMigrationPending(current) {
		migrationTemplate := current.Spec.Template.DeepCopy()
		if apiequality.Semantic.DeepEqual(migrationTemplate.Spec.Affinity.PodAntiAffinity, broadAgentPodAntiAffinity()) && !prepareProfileAntiAffinityForSurge(migrationTemplate) {
			return false, fmt.Errorf("prepared Agent rollout cannot migrate profile anti-affinity")
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
	if spec.OS != nil && spec.OS.Name != corev1.Linux || spec.NodeSelector[corev1.LabelOSStable] == "windows" || spec.NodeSelector["beta.kubernetes.io/os"] == "windows" {
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
	for i := range spec.Containers {
		container := &spec.Containers[i]
		if container.Name == string(apicommon.TraceAgentContainerName) {
			traceIndex := slices.Index(container.Command, "trace-agent")
			if traceIndex < 0 {
				return fmt.Errorf("prepared Agent rollout cannot bypass an unknown trace-agent loader command")
			}
			container.Command = append([]string(nil), container.Command[traceIndex:]...)
		}
		configurePreparedContainer(container)
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

func preparedContainerCommandSupported(container *corev1.Container) bool {
	if len(container.Command) == 0 {
		return false
	}
	expected := map[string]string{
		string(apicommon.CoreAgentContainerName):           "agent",
		string(apicommon.TraceAgentContainerName):          "trace-agent",
		string(apicommon.ProcessAgentContainerName):        "process-agent",
		string(apicommon.SystemProbeContainerName):         "system-probe",
		string(apicommon.HostProfiler):                     "host-profiler",
		string(apicommon.OtelAgent):                        "otel-agent",
		string(apicommon.PrivateActionRunnerContainerName): "/opt/datadog-agent/embedded/bin/privateactionrunner",
	}
	want := expected[container.Name]
	if container.Name == string(apicommon.TraceAgentContainerName) {
		return slices.Contains(container.Command, want)
	}
	if container.Command[0] != want {
		return false
	}
	return true
}

func configurePreparedContainer(container *corev1.Container) {
	setContainerEnv(container, corev1.EnvVar{Name: rolloutEnabledEnv, Value: "true"})
	setContainerEnv(container, corev1.EnvVar{
		Name: rolloutPodUIDEnv,
		ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{
			APIVersion: "v1",
			FieldPath:  "metadata.uid",
		}},
	})
	setContainerEnv(container, corev1.EnvVar{
		Name: kubeletHostEnv,
		ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{
			APIVersion: "v1",
			FieldPath:  "status.hostIP",
		}},
	})
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
	return strings.EqualFold(annotations[preparedRolloutModeAnnotation], preparedRolloutModeV1)
}
