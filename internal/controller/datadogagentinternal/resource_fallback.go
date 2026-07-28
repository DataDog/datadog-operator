// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// Copyright 2016-present Datadog, Inc.

package datadogagentinternal

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

type fallbackCandidate struct {
	pending  *corev1.Pod
	old      *corev1.Pod
	nodeName string
}

type runningHandoffCandidate struct {
	replacement *corev1.Pod
	old         *corev1.Pod
	nodeName    string
}

const resourceFallbackPollInterval = 5 * time.Second

// reconcilePreparedRollout authorizes one node handoff when a surged replacement
// is running but intentionally unready. If no replacement fits on its node, it
// retains the CPU/memory-only delete-before-create fallback.
func (r *Reconciler) reconcilePreparedRollout(ctx context.Context, ddai *datadoghqv1alpha1.DatadogAgentInternal, expectedDS *appsv1.DaemonSet, budgetValue intstr.IntOrString) (reconcile.Result, error) {
	reader := r.apiReader
	if reader == nil {
		reader = r.client
	}
	ds := &appsv1.DaemonSet{}
	if err := reader.Get(ctx, client.ObjectKeyFromObject(expectedDS), ds); err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}
	if !daemonSetControlledByDDAI(ds, ddai) || !preparedRolloutDaemonSetEligible(ds) || !hasRolloutMode(ds.Spec.Template.Annotations) {
		return reconcile.Result{}, nil
	}
	if ds.Status.DesiredNumberScheduled <= 0 || ds.Status.ObservedGeneration != ds.Generation {
		return resourceFallbackPollResult(ds), nil
	}
	budget, budgetErr := intstr.GetScaledValueFromIntOrPercent(&budgetValue, int(ds.Status.DesiredNumberScheduled), true)
	if budgetErr != nil {
		return reconcile.Result{}, fmt.Errorf("resolve Agent resource fallback budget: %w", budgetErr)
	}
	if budget <= 0 {
		return reconcile.Result{}, nil
	}
	pods, podsErr := daemonSetPods(ctx, reader, ds)
	if podsErr != nil {
		return reconcile.Result{}, podsErr
	}
	if consumedPreparedRolloutBudget(ds, pods) >= budget {
		return resourceFallbackPollResult(ds), nil
	}
	desiredRevision, revisionErr := currentDaemonSetRevision(ctx, reader, ds)
	if revisionErr != nil {
		return reconcile.Result{}, revisionErr
	}
	if desiredRevision == "" {
		return resourceFallbackPollResult(ds), nil
	}

	runningCandidates := runningHandoffCandidates(ds, pods, desiredRevision, time.Now())
	if len(runningCandidates) > 0 {
		candidate := runningCandidates[0]
		replacement := &corev1.Pod{}
		old := &corev1.Pod{}
		if err := reader.Get(ctx, client.ObjectKeyFromObject(candidate.replacement), replacement); err != nil {
			return reconcile.Result{}, client.IgnoreNotFound(err)
		}
		if err := reader.Get(ctx, client.ObjectKeyFromObject(candidate.old), old); err != nil {
			return reconcile.Result{}, client.IgnoreNotFound(err)
		}
		if replacement.UID != candidate.replacement.UID || old.UID != candidate.old.UID ||
			!controlledByUID(replacement, ds.UID) || !controlledByUID(old, ds.UID) ||
			!replacementRunningForHandoff(replacement, ds, desiredRevision) ||
			old.Spec.NodeName != candidate.nodeName || replacement.Spec.NodeName != candidate.nodeName ||
			old.DeletionTimestamp != nil || !podAvailable(old, ds.Spec.MinReadySeconds, time.Now()) ||
			podRevision(old) == "" || podRevision(old) == desiredRevision {
			return resourceFallbackPollResult(ds), nil
		}
		allowed, err := preparedRolloutDeletionAllowed(ctx, reader, ds, budget, desiredRevision)
		if err != nil {
			return reconcile.Result{}, err
		}
		if !allowed {
			return resourceFallbackPollResult(ds), nil
		}

		uid := old.UID
		if err := r.client.Delete(ctx, old, &client.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid}}); err != nil && !apierrors.IsNotFound(err) {
			return reconcile.Result{}, fmt.Errorf("delete old Agent Pod %s/%s for prepared handoff: %w", old.Namespace, old.Name, err)
		}
		ctrl.LoggerFrom(ctx).WithValues("daemonset", ds.Name, "node", candidate.nodeName, "oldPod", old.Name, "replacementPod", replacement.Name).Info("Deleted old Agent Pod after every replacement container started")
		if r.recorder != nil {
			r.recorder.Eventf(ddai, corev1.EventTypeNormal, "AgentPreparedHandoff", "Deleted old Agent Pod %s on node %s after every container in replacement %s started", old.Name, candidate.nodeName, replacement.Name)
		}
		return reconcile.Result{RequeueAfter: time.Second}, nil
	}

	candidates := fallbackCandidates(ds, pods, desiredRevision, time.Now())
	if len(candidates) == 0 {
		return resourceFallbackPollResult(ds), nil
	}
	candidate := candidates[0]

	// Re-read both Pods immediately before deletion. The scheduler condition,
	// target node and old Pod identity must still describe the same handoff.
	pending := &corev1.Pod{}
	old := &corev1.Pod{}
	if err := reader.Get(ctx, client.ObjectKeyFromObject(candidate.pending), pending); err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}
	if err := reader.Get(ctx, client.ObjectKeyFromObject(candidate.old), old); err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}
	if pending.UID != candidate.pending.UID || old.UID != candidate.old.UID || !controlledByUID(pending, ds.UID) || !controlledByUID(old, ds.UID) {
		return resourceFallbackPollResult(ds), nil
	}
	if _, ok := resourceOnlyUnschedulable(pending); !ok || podRevision(pending) != desiredRevision || !replacementSpecMatchesTemplate(pending, ds) {
		return resourceFallbackPollResult(ds), nil
	}
	nodeName, ok := targetNodeFromDaemonSetAffinity(pending)
	if !ok || nodeName != candidate.nodeName || pending.Spec.NodeName != "" || pending.DeletionTimestamp != nil || old.Spec.NodeName != nodeName || old.DeletionTimestamp != nil || !podAvailable(old, ds.Spec.MinReadySeconds, time.Now()) || podRevision(old) == "" || podRevision(old) == desiredRevision {
		return resourceFallbackPollResult(ds), nil
	}
	allowed, err := preparedRolloutDeletionAllowed(ctx, reader, ds, budget, desiredRevision)
	if err != nil {
		return reconcile.Result{}, err
	}
	if !allowed {
		return resourceFallbackPollResult(ds), nil
	}

	uid := old.UID
	if err := r.client.Delete(ctx, old, &client.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid}}); err != nil && !apierrors.IsNotFound(err) {
		return reconcile.Result{}, fmt.Errorf("delete old Agent Pod %s/%s for resource fallback: %w", old.Namespace, old.Name, err)
	}
	ctrl.LoggerFrom(ctx).WithValues("daemonset", ds.Name, "node", nodeName, "oldPod", old.Name, "replacementPod", pending.Name).Info("Deleted old Agent Pod because the surged replacement was blocked by node CPU or memory")
	if r.recorder != nil {
		r.recorder.Eventf(ddai, corev1.EventTypeWarning, "AgentResourceFallback", "Deleted old Agent Pod %s on node %s because replacement %s was blocked by CPU or memory", old.Name, nodeName, pending.Name)
	}
	return reconcile.Result{RequeueAfter: time.Second}, nil
}

// preparedRolloutDeletionAllowed narrows the unavoidable observation-to-delete
// race by rechecking the live rollout generation, revision and unavailability
// immediately before deleting an old Pod.
func preparedRolloutDeletionAllowed(ctx context.Context, reader client.Reader, expectedDS *appsv1.DaemonSet, budget int, desiredRevision string) (bool, error) {
	ds := &appsv1.DaemonSet{}
	if err := reader.Get(ctx, client.ObjectKeyFromObject(expectedDS), ds); err != nil {
		return false, client.IgnoreNotFound(err)
	}
	if ds.UID != expectedDS.UID || ds.Generation != expectedDS.Generation || ds.Status.ObservedGeneration != ds.Generation || !preparedRolloutDaemonSetEligible(ds) || !hasRolloutMode(ds.Spec.Template.Annotations) {
		return false, nil
	}
	liveRevision, err := currentDaemonSetRevision(ctx, reader, ds)
	if err != nil {
		return false, err
	}
	if liveRevision == "" || liveRevision != desiredRevision {
		return false, nil
	}
	pods, err := daemonSetPods(ctx, reader, ds)
	if err != nil {
		return false, err
	}
	return consumedPreparedRolloutBudget(ds, pods) < budget, nil
}

func runningHandoffCandidates(ds *appsv1.DaemonSet, pods []corev1.Pod, desiredRevision string, now time.Time) []runningHandoffCandidate {
	newByNode := map[string][]*corev1.Pod{}
	oldByNode := map[string][]*corev1.Pod{}
	for i := range pods {
		pod := &pods[i]
		if replacementRunningForHandoff(pod, ds, desiredRevision) {
			newByNode[pod.Spec.NodeName] = append(newByNode[pod.Spec.NodeName], pod)
			continue
		}
		if pod.Spec.NodeName != "" && pod.DeletionTimestamp == nil && podRevision(pod) != "" && podRevision(pod) != desiredRevision && podAvailable(pod, ds.Spec.MinReadySeconds, now) {
			oldByNode[pod.Spec.NodeName] = append(oldByNode[pod.Spec.NodeName], pod)
		}
	}

	var candidates []runningHandoffCandidate
	for nodeName, replacements := range newByNode {
		olds := oldByNode[nodeName]
		if len(replacements) == 1 && len(olds) == 1 {
			candidates = append(candidates, runningHandoffCandidate{replacement: replacements[0], old: olds[0], nodeName: nodeName})
		}
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].nodeName < candidates[j].nodeName })
	return candidates
}

func replacementRunningForHandoff(pod *corev1.Pod, ds *appsv1.DaemonSet, desiredRevision string) bool {
	if pod.DeletionTimestamp != nil || pod.Spec.NodeName == "" || pod.Status.Phase != corev1.PodRunning || podRevision(pod) != desiredRevision || !podInitialized(pod) {
		return false
	}
	if !replacementSpecMatchesTemplate(pod, ds) {
		return false
	}

	desiredImages := make(map[string]string, len(ds.Spec.Template.Spec.Containers))
	for i := range ds.Spec.Template.Spec.Containers {
		container := &ds.Spec.Template.Spec.Containers[i]
		if container.Name == "" {
			return false
		}
		desiredImages[container.Name] = container.Image
	}
	if len(pod.Status.ContainerStatuses) != len(desiredImages) {
		return false
	}

	seen := make(map[string]struct{}, len(pod.Status.ContainerStatuses))
	for i := range pod.Status.ContainerStatuses {
		status := &pod.Status.ContainerStatuses[i]
		if _, ok := desiredImages[status.Name]; !ok || status.State.Running == nil || status.RestartCount != 0 || status.ContainerID == "" || status.ImageID == "" {
			return false
		}
		if _, duplicate := seen[status.Name]; duplicate {
			return false
		}
		seen[status.Name] = struct{}{}
	}
	return len(seen) == len(desiredImages)
}

func replacementSpecMatchesTemplate(pod *corev1.Pod, ds *appsv1.DaemonSet) bool {
	desiredImages := make(map[string]string, len(ds.Spec.Template.Spec.Containers))
	for i := range ds.Spec.Template.Spec.Containers {
		container := &ds.Spec.Template.Spec.Containers[i]
		if container.Name == "" {
			return false
		}
		desiredImages[container.Name] = container.Image
	}
	if len(pod.Spec.Containers) != len(desiredImages) {
		return false
	}
	for i := range pod.Spec.Containers {
		container := &pod.Spec.Containers[i]
		if desiredImage, ok := desiredImages[container.Name]; !ok || desiredImage != container.Image {
			return false
		}
	}
	return true
}

func podInitialized(pod *corev1.Pod) bool {
	for i := range pod.Status.Conditions {
		condition := &pod.Status.Conditions[i]
		if condition.Type == corev1.PodInitialized {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}

func podRevision(pod *corev1.Pod) string {
	return pod.Labels[appsv1.DefaultDaemonSetUniqueLabelKey]
}

func resourceFallbackPollResult(ds *appsv1.DaemonSet) reconcile.Result {
	if daemonSetFullyRolledOut(ds) {
		return reconcile.Result{}
	}
	return reconcile.Result{RequeueAfter: resourceFallbackPollInterval}
}

func fallbackCandidates(ds *appsv1.DaemonSet, pods []corev1.Pod, desiredRevision string, now time.Time) []fallbackCandidate {
	var candidates []fallbackCandidate
	for i := range pods {
		pending := &pods[i]
		if _, ok := resourceOnlyUnschedulable(pending); !ok || pending.DeletionTimestamp != nil || pending.Spec.NodeName != "" || pending.Status.NominatedNodeName != "" || podRevision(pending) != desiredRevision || !replacementSpecMatchesTemplate(pending, ds) {
			continue
		}
		nodeName, ok := targetNodeFromDaemonSetAffinity(pending)
		if !ok {
			continue
		}
		var oldPods []*corev1.Pod
		for j := range pods {
			old := &pods[j]
			if old.Spec.NodeName == nodeName && old.DeletionTimestamp == nil && podRevision(old) != "" && podRevision(old) != desiredRevision && podAvailable(old, ds.Spec.MinReadySeconds, now) {
				oldPods = append(oldPods, old)
			}
		}
		if len(oldPods) == 1 {
			candidates = append(candidates, fallbackCandidate{pending: pending, old: oldPods[0], nodeName: nodeName})
		}
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].nodeName < candidates[j].nodeName })
	return candidates
}

func consumedPreparedRolloutBudget(ds *appsv1.DaemonSet, pods []corev1.Pod) int {
	terminatingNodes := map[string]struct{}{}
	for i := range pods {
		if pods[i].DeletionTimestamp != nil && pods[i].Spec.NodeName != "" {
			terminatingNodes[pods[i].Spec.NodeName] = struct{}{}
		}
	}
	// NumberUnavailable may already include some terminating Pods. Counting both
	// is deliberately conservative: fallback must never delete more old Agents
	// merely because DaemonSet status and Pod deletion are observed at different
	// times.
	return int(ds.Status.NumberUnavailable) + len(terminatingNodes)
}

type resourceShortage struct {
	cpu    bool
	memory bool
}

func resourceOnlyUnschedulable(pod *corev1.Pod) (resourceShortage, bool) {
	condition := scheduledCondition(pod)
	if condition == nil || condition.Status != corev1.ConditionFalse || condition.Reason != corev1.PodReasonUnschedulable {
		return resourceShortage{}, false
	}
	primary := condition.Message
	lower := strings.ToLower(primary)
	if i := strings.Index(lower, "preemption:"); i >= 0 {
		primary = primary[:i]
		lower = lower[:i]
	}
	if i := strings.Index(lower, "nodes are available:"); i >= 0 {
		primary = primary[i+len("nodes are available:"):]
	}
	primary = strings.TrimSuffix(strings.TrimSpace(primary), ".")
	var shortage resourceShortage
	for reason := range strings.SplitSeq(primary, ", ") {
		fields := strings.Fields(strings.ToLower(strings.TrimSpace(reason)))
		if len(fields) < 2 {
			return resourceShortage{}, false
		}
		if _, err := strconv.Atoi(fields[0]); err != nil {
			return resourceShortage{}, false
		}
		switch strings.Join(fields[1:], " ") {
		case "insufficient cpu":
			shortage.cpu = true
		case "insufficient memory":
			shortage.memory = true
		case "node(s) didn't match pod's node affinity/selector", "node(s) didn't satisfy plugin(s) [nodeaffinity]":
			// Expected on non-target nodes for DaemonSet surge Pods.
		default:
			return resourceShortage{}, false
		}
	}
	return shortage, shortage.cpu || shortage.memory
}

func scheduledCondition(pod *corev1.Pod) *corev1.PodCondition {
	for i := range pod.Status.Conditions {
		if pod.Status.Conditions[i].Type == corev1.PodScheduled {
			return &pod.Status.Conditions[i]
		}
	}
	return nil
}

func targetNodeFromDaemonSetAffinity(pod *corev1.Pod) (string, bool) {
	if pod.Spec.Affinity == nil || pod.Spec.Affinity.NodeAffinity == nil || pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution == nil {
		return "", false
	}
	terms := pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms
	if len(terms) == 0 {
		return "", false
	}
	var target string
	for _, term := range terms {
		var termTarget string
		for _, requirement := range term.MatchFields {
			if requirement.Key != metav1.ObjectNameField {
				continue
			}
			if requirement.Operator != corev1.NodeSelectorOpIn || len(requirement.Values) != 1 || requirement.Values[0] == "" || termTarget != "" {
				return "", false
			}
			termTarget = requirement.Values[0]
		}
		if termTarget == "" || target != "" && termTarget != target {
			return "", false
		}
		target = termTarget
	}
	return target, target != ""
}
