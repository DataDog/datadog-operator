// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// Copyright 2016-present Datadog, Inc.

package datadogagentinternal

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

const (
	preparedRolloutAnnotation      = "experimental.agent.datadoghq.com/host-network-surge-prepared"
	preparedRolloutPhaseAnnotation = "experimental.agent.datadoghq.com/prepared-rollout-phase"
	resourceFallbackAnnotation     = "experimental.agent.datadoghq.com/resource-fallback"
	preparedRolloutPhaseArm        = "arm"
	preparedRolloutPhaseStandby    = "standby"

	preparedRolloutLockVolume  = "agent-rollout-locks"
	preparedRolloutStateVolume = "agent-rollout-state"
	preparedRolloutLockDir     = "/var/run/datadog-agent-rollout"
	preparedRolloutStateDir    = "/var/run/datadog-agent-rollout-state"

	rolloutEnabledEnv   = "DD_EXPERIMENTAL_NODE_AGENT_ROLLOUT_ENABLED"
	rolloutLockPathEnv  = "DD_EXPERIMENTAL_NODE_AGENT_ROLLOUT_LOCK_PATH"
	rolloutStatePathEnv = "DD_EXPERIMENTAL_NODE_AGENT_ROLLOUT_STATE_PATH"
)

var preparedRolloutContainerNames = []string{
	string(apicommon.CoreAgentContainerName),
	string(apicommon.TraceAgentContainerName),
}

func preparedRolloutEnabled(ddai *datadoghqv1alpha1.DatadogAgentInternal) bool {
	return strings.EqualFold(ddai.Annotations[preparedRolloutAnnotation], "true")
}

func resourceFallbackEnabled(ddai *datadoghqv1alpha1.DatadogAgentInternal) bool {
	return preparedRolloutEnabled(ddai) && strings.EqualFold(ddai.Annotations[resourceFallbackAnnotation], "true")
}

// configurePreparedRollout installs a restart-safe two-phase protocol. A
// conventional rollout first arms every old process with the node-local lock.
// Only an exact, fully available arm revision may transition to standby surge.
func (r *Reconciler) configurePreparedRollout(ctx context.Context, ddai *datadoghqv1alpha1.DatadogAgentInternal, desired *appsv1.DaemonSet, budget intstr.IntOrString) (string, error) {
	if !preparedRolloutEnabled(ddai) {
		return "", nil
	}
	if !positiveIntOrPercent(&budget) {
		return "", fmt.Errorf("prepared Agent rollout requires a positive, valid maxUnavailable budget")
	}
	if desired.Spec.UpdateStrategy.Type != "" && desired.Spec.UpdateStrategy.Type != appsv1.RollingUpdateDaemonSetStrategyType {
		return "", fmt.Errorf("prepared Agent rollout requires RollingUpdate strategy")
	}

	armed := desired.DeepCopy()
	if err := prepareAgentTemplate(armed, preparedRolloutPhaseArm); err != nil {
		return "", err
	}
	configureArmStrategy(armed, budget)

	live := &appsv1.DaemonSet{}
	err := r.apiReader.Get(ctx, client.ObjectKeyFromObject(desired), live)
	if err != nil && !apierrors.IsNotFound(err) {
		return "", fmt.Errorf("get live Agent DaemonSet for prepared rollout: %w", err)
	}

	phase := preparedRolloutPhaseArm
	if err == nil {
		livePhase := live.Spec.Template.Annotations[preparedRolloutPhaseAnnotation]
		switch livePhase {
		case preparedRolloutPhaseStandby:
			// Never oscillate back to arm during a mixed or failed surged rollout.
			phase = preparedRolloutPhaseStandby
		case preparedRolloutPhaseArm:
			if apiequality.Semantic.DeepEqual(live.Spec.Template, armed.Spec.Template) && daemonSetArmComplete(live) {
				phase = preparedRolloutPhaseStandby
			}
		}
	}

	if phase == preparedRolloutPhaseArm {
		*desired = *armed
		return phase, nil
	}

	standby := desired.DeepCopy()
	if err := prepareAgentTemplate(standby, preparedRolloutPhaseStandby); err != nil {
		return "", err
	}
	if !configureResourceFallback(standby, budget) {
		return "", fmt.Errorf("prepared Agent rollout requires a positive, valid maxUnavailable budget")
	}
	*desired = *standby
	return phase, nil
}

func daemonSetArmComplete(ds *appsv1.DaemonSet) bool {
	desired := ds.Status.DesiredNumberScheduled
	return desired > 0 &&
		ds.Status.ObservedGeneration == ds.Generation &&
		ds.Status.UpdatedNumberScheduled == desired &&
		ds.Status.NumberReady == desired &&
		ds.Status.NumberAvailable == desired &&
		ds.Status.NumberUnavailable == 0
}

func configureArmStrategy(ds *appsv1.DaemonSet, budget intstr.IntOrString) {
	ds.Spec.UpdateStrategy.Type = appsv1.RollingUpdateDaemonSetStrategyType
	if ds.Spec.UpdateStrategy.RollingUpdate == nil {
		ds.Spec.UpdateStrategy.RollingUpdate = &appsv1.RollingUpdateDaemonSet{}
	}
	zero := intstr.FromInt(0)
	ds.Spec.UpdateStrategy.RollingUpdate.MaxSurge = &zero
	if positiveIntOrPercent(&budget) {
		value := budget
		ds.Spec.UpdateStrategy.RollingUpdate.MaxUnavailable = &value
	}
}

func prepareAgentTemplate(ds *appsv1.DaemonSet, phase string) error {
	spec := &ds.Spec.Template.Spec
	if !spec.HostNetwork {
		return fmt.Errorf("prepared Agent rollout requires hostNetwork=true")
	}
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
	if err := addPreparedRolloutVolumes(spec); err != nil {
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
		if phase == preparedRolloutPhaseStandby {
			container.Ports = nil
		}
	}
	if phase == preparedRolloutPhaseStandby {
		for i := range spec.InitContainers {
			spec.InitContainers[i].Ports = nil
		}
	}
	if ds.Spec.Template.Annotations == nil {
		ds.Spec.Template.Annotations = map[string]string{}
	}
	ds.Spec.Template.Annotations[preparedRolloutPhaseAnnotation] = phase
	return nil
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
			if mount.Name == preparedRolloutLockVolume || mount.Name == preparedRolloutStateVolume || mount.MountPath == preparedRolloutLockDir || mount.MountPath == preparedRolloutStateDir {
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

func addPreparedRolloutVolumes(spec *corev1.PodSpec) error {
	for i := range spec.Volumes {
		if spec.Volumes[i].Name == preparedRolloutLockVolume || spec.Volumes[i].Name == preparedRolloutStateVolume {
			return fmt.Errorf("prepared Agent rollout volume name %q is reserved", spec.Volumes[i].Name)
		}
	}
	directoryOrCreate := corev1.HostPathDirectoryOrCreate
	spec.Volumes = append(spec.Volumes,
		corev1.Volume{
			Name: preparedRolloutLockVolume,
			VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{
				Path: preparedRolloutLockDir,
				Type: &directoryOrCreate,
			}},
		},
		corev1.Volume{
			Name:         preparedRolloutStateVolume,
			VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
		},
	)
	return nil
}

func configurePreparedContainer(container *corev1.Container) {
	lockPath := preparedRolloutLockDir + "/" + container.Name + ".lock"
	statePath := preparedRolloutStateDir + "/" + container.Name + ".state"
	setContainerEnv(container, rolloutEnabledEnv, "true")
	setContainerEnv(container, rolloutLockPathEnv, lockPath)
	setContainerEnv(container, rolloutStatePathEnv, statePath)
	container.VolumeMounts = append(container.VolumeMounts,
		corev1.VolumeMount{Name: preparedRolloutLockVolume, MountPath: preparedRolloutLockDir},
		corev1.VolumeMount{Name: preparedRolloutStateVolume, MountPath: preparedRolloutStateDir},
	)
	container.StartupProbe = rolloutStateProbe(statePath, "prepared|activating|active", 1, 300)
	container.LivenessProbe = rolloutStateProbe(statePath, "prepared|activating|active", 10, 3)
	container.ReadinessProbe = rolloutStateProbe(statePath, "active", 1, 3)
}

func setContainerEnv(container *corev1.Container, name, value string) {
	for i := range container.Env {
		if container.Env[i].Name == name {
			container.Env[i] = corev1.EnvVar{Name: name, Value: value}
			return
		}
	}
	container.Env = append(container.Env, corev1.EnvVar{Name: name, Value: value})
}

func rolloutStateProbe(path, accepted string, period, failures int32) *corev1.Probe {
	command := fmt.Sprintf(`case "$(cat %s 2>/dev/null)" in %s) exit 0;; *) exit 1;; esac`, path, accepted)
	return &corev1.Probe{
		ProbeHandler:     corev1.ProbeHandler{Exec: &corev1.ExecAction{Command: []string{"sh", "-c", command}}},
		PeriodSeconds:    period,
		TimeoutSeconds:   1,
		FailureThreshold: failures,
	}
}

type preparedHandoffCandidate struct {
	replacement *corev1.Pod
	old         *corev1.Pod
	nodeName    string
	reserved    bool
}

func (r *Reconciler) reconcilePreparedHandoff(ctx context.Context, ddai *datadoghqv1alpha1.DatadogAgentInternal, expectedDS *appsv1.DaemonSet, budgetValue intstr.IntOrString) (reconcile.Result, error) {
	reader := r.apiReader
	liveDS := &appsv1.DaemonSet{}
	if err := reader.Get(ctx, client.ObjectKeyFromObject(expectedDS), liveDS); err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}
	if !daemonSetControlledByDDAI(liveDS, ddai) || liveDS.Spec.Template.Annotations[preparedRolloutPhaseAnnotation] != preparedRolloutPhaseStandby || !resourceFallbackDaemonSetEligible(liveDS) {
		return reconcile.Result{}, nil
	}
	currentRevision, err := currentDaemonSetRevision(ctx, reader, liveDS)
	if err != nil || currentRevision == "" {
		return reconcile.Result{}, err
	}
	pods, err := daemonSetPods(ctx, reader, liveDS)
	if err != nil {
		return reconcile.Result{}, err
	}
	budget, err := intstr.GetScaledValueFromIntOrPercent(&budgetValue, int(liveDS.Status.DesiredNumberScheduled), true)
	if err != nil || budget <= 0 {
		return reconcile.Result{}, err
	}
	consumed := consumedFallbackBudget(liveDS, pods, currentRevision, time.Now())
	if consumed >= budget {
		return reconcile.Result{}, nil
	}
	candidates := preparedHandoffCandidates(liveDS, pods, currentRevision)
	for _, candidate := range candidates {
		if !candidate.reserved {
			base := candidate.replacement.DeepCopy()
			patched := candidate.replacement.DeepCopy()
			if patched.Annotations == nil {
				patched.Annotations = map[string]string{}
			}
			patched.Annotations[resourceFallbackOldPodAnnotation] = string(candidate.old.UID)
			if err := r.client.Patch(ctx, patched, client.MergeFrom(base)); err != nil {
				return reconcile.Result{}, fmt.Errorf("reserve prepared Agent handoff for Pod %s/%s: %w", patched.Namespace, patched.Name, err)
			}
			candidate.replacement = patched
		}
		liveCandidate, err := r.revalidatePreparedHandoff(ctx, liveDS, candidate, currentRevision)
		if err != nil {
			return reconcile.Result{}, err
		}
		if liveCandidate == nil {
			continue
		}
		withinBudget, err := fallbackBudgetWithinLimit(ctx, reader, liveDS, budgetValue, currentRevision)
		if err != nil {
			return reconcile.Result{}, err
		}
		if !withinBudget {
			return reconcile.Result{RequeueAfter: time.Second}, nil
		}
		uid := liveCandidate.old.UID
		if err := r.client.Delete(ctx, liveCandidate.old, &client.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid}}); err != nil && !apierrors.IsNotFound(err) {
			return reconcile.Result{}, fmt.Errorf("delete old Agent Pod %s/%s for prepared handoff: %w", liveCandidate.old.Namespace, liveCandidate.old.Name, err)
		}
		if r.recorder != nil {
			r.recorder.Eventf(ddai, corev1.EventTypeNormal, "AgentPreparedHandoff", "Deleted old Agent Pod %s on node %s after replacement %s reported Prepared", liveCandidate.old.Name, liveCandidate.nodeName, liveCandidate.replacement.Name)
		}
		return reconcile.Result{RequeueAfter: time.Second}, nil
	}
	return reconcile.Result{}, nil
}

func preparedHandoffCandidates(ds *appsv1.DaemonSet, pods []corev1.Pod, currentRevision string) []preparedHandoffCandidate {
	oldByNode := map[string]*corev1.Pod{}
	for i := range pods {
		pod := &pods[i]
		if pod.Spec.NodeName != "" && pod.Labels[appsv1.DefaultDaemonSetUniqueLabelKey] != currentRevision && podAvailable(pod, ds.Spec.MinReadySeconds, time.Now()) {
			oldByNode[pod.Spec.NodeName] = pod
		}
	}
	var candidates []preparedHandoffCandidate
	for i := range pods {
		pod := &pods[i]
		if pod.Labels[appsv1.DefaultDaemonSetUniqueLabelKey] != currentRevision || !podPreparedForHandoff(pod) || podAvailable(pod, ds.Spec.MinReadySeconds, time.Now()) {
			continue
		}
		old := oldByNode[pod.Spec.NodeName]
		if old == nil {
			continue
		}
		reservation := pod.Annotations[resourceFallbackOldPodAnnotation]
		if reservation != "" && reservation != string(old.UID) {
			continue
		}
		candidates = append(candidates, preparedHandoffCandidate{replacement: pod, old: old, nodeName: pod.Spec.NodeName, reserved: reservation != ""})
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].nodeName < candidates[j].nodeName })
	return candidates
}

func podPreparedForHandoff(pod *corev1.Pod) bool {
	if pod.DeletionTimestamp != nil || pod.Status.Phase != corev1.PodRunning || len(pod.Status.InitContainerStatuses) != 2 || len(pod.Status.ContainerStatuses) != len(preparedRolloutContainerNames) {
		return false
	}
	for i := range pod.Status.InitContainerStatuses {
		status := &pod.Status.InitContainerStatuses[i]
		if status.State.Terminated == nil || status.State.Terminated.ExitCode != 0 {
			return false
		}
	}
	seen := map[string]bool{}
	for i := range pod.Status.ContainerStatuses {
		status := &pod.Status.ContainerStatuses[i]
		if status.Name != string(apicommon.CoreAgentContainerName) && status.Name != string(apicommon.TraceAgentContainerName) || status.State.Running == nil || status.Started == nil || !*status.Started || status.RestartCount != 0 {
			return false
		}
		seen[status.Name] = true
	}
	return seen[string(apicommon.CoreAgentContainerName)] && seen[string(apicommon.TraceAgentContainerName)]
}

func (r *Reconciler) revalidatePreparedHandoff(ctx context.Context, expectedDS *appsv1.DaemonSet, candidate preparedHandoffCandidate, expectedRevision string) (*preparedHandoffCandidate, error) {
	liveDS := &appsv1.DaemonSet{}
	if err := r.apiReader.Get(ctx, client.ObjectKeyFromObject(expectedDS), liveDS); err != nil {
		return nil, client.IgnoreNotFound(err)
	}
	if liveDS.UID != expectedDS.UID || liveDS.Generation != expectedDS.Generation || liveDS.Spec.Template.Annotations[preparedRolloutPhaseAnnotation] != preparedRolloutPhaseStandby {
		return nil, nil
	}
	revision, err := currentDaemonSetRevision(ctx, r.apiReader, liveDS)
	if err != nil || revision != expectedRevision {
		return nil, err
	}
	replacement := &corev1.Pod{}
	old := &corev1.Pod{}
	if err := r.apiReader.Get(ctx, client.ObjectKeyFromObject(candidate.replacement), replacement); err != nil {
		return nil, client.IgnoreNotFound(err)
	}
	if err := r.apiReader.Get(ctx, client.ObjectKeyFromObject(candidate.old), old); err != nil {
		return nil, client.IgnoreNotFound(err)
	}
	if replacement.UID != candidate.replacement.UID || old.UID != candidate.old.UID || !controlledByUID(replacement, liveDS.UID) || !controlledByUID(old, liveDS.UID) || replacement.Spec.NodeName != candidate.nodeName || old.Spec.NodeName != candidate.nodeName {
		return nil, nil
	}
	if replacement.Labels[appsv1.DefaultDaemonSetUniqueLabelKey] != revision || old.Labels[appsv1.DefaultDaemonSetUniqueLabelKey] == revision || replacement.Annotations[resourceFallbackOldPodAnnotation] != string(old.UID) || !podPreparedForHandoff(replacement) || !podAvailable(old, liveDS.Spec.MinReadySeconds, time.Now()) {
		return nil, nil
	}
	return &preparedHandoffCandidate{replacement: replacement, old: old, nodeName: candidate.nodeName, reserved: true}, nil
}
