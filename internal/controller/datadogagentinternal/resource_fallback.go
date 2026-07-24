// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// Copyright 2016-present Datadog, Inc.

package datadogagentinternal

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/util/intstr"
	resourcehelper "k8s.io/component-helpers/resource"
	"k8s.io/component-helpers/scheduling/corev1/nodeaffinity"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

const resourceFallbackOldPodAnnotation = "agent.datadoghq.com/resource-fallback-old-pod-uid"

type resourceShortage struct {
	cpu    bool
	memory bool
}

type fallbackCandidate struct {
	pending  *corev1.Pod
	old      *corev1.Pod
	nodeName string
	shortage resourceShortage
	reserved bool
}

// reconcileResourceFallback breaks the maxSurge capacity deadlock only when
// the scheduler reports CPU and/or memory as the sole target-node blocker and
// removing the exact old Agent Pod is sufficient to make the replacement fit.
// The original maxUnavailable value is reused as the deletion budget.
func (r *Reconciler) reconcileResourceFallback(ctx context.Context, ddai *datadoghqv1alpha1.DatadogAgentInternal, expectedDS *appsv1.DaemonSet, budgetValue intstr.IntOrString) (reconcile.Result, error) {
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
	budget, err := intstr.GetScaledValueFromIntOrPercent(&budgetValue, int(ds.Status.DesiredNumberScheduled), true)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("resolve Agent resource fallback budget: %w", err)
	}
	if budget <= 0 {
		return reconcile.Result{}, nil
	}
	revision, err := currentDaemonSetRevision(ctx, reader, ds)
	if err != nil || revision == "" {
		return reconcile.Result{}, err
	}
	pods, err := daemonSetPods(ctx, reader, ds)
	if err != nil {
		return reconcile.Result{}, err
	}
	consumed := consumedResourceFallbackBudget(ds, pods, revision, time.Now())
	if consumed > budget {
		return reconcile.Result{}, nil
	}

	for _, candidate := range fallbackCandidates(ds, pods, revision, time.Now()) {
		if !candidate.reserved && consumed >= budget {
			break
		}
		if !candidate.reserved {
			live, err := r.revalidateFallbackCandidate(ctx, reader, ds, candidate, revision, false)
			if err != nil {
				return reconcile.Result{}, err
			}
			if live == nil {
				continue
			}
			candidate = *live
			base := candidate.pending.DeepCopy()
			patched := candidate.pending.DeepCopy()
			if patched.Annotations == nil {
				patched.Annotations = map[string]string{}
			}
			patched.Annotations[resourceFallbackOldPodAnnotation] = string(candidate.old.UID)
			if err := r.client.Patch(ctx, patched, client.MergeFrom(base)); err != nil {
				return reconcile.Result{}, fmt.Errorf("reserve Agent resource fallback for Pod %s/%s: %w", patched.Namespace, patched.Name, err)
			}
			candidate.pending = patched
			candidate.reserved = true
		}

		live, err := r.revalidateFallbackCandidate(ctx, reader, ds, candidate, revision, true)
		if err != nil {
			return reconcile.Result{}, err
		}
		if live == nil {
			continue
		}
		withinBudget, err := resourceFallbackBudgetWithinLimit(ctx, reader, ds, budgetValue, revision)
		if err != nil {
			return reconcile.Result{}, err
		}
		if !withinBudget {
			return reconcile.Result{RequeueAfter: time.Second}, nil
		}
		uid := live.old.UID
		if err := r.client.Delete(ctx, live.old, &client.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid}}); err != nil && !apierrors.IsNotFound(err) {
			return reconcile.Result{}, fmt.Errorf("delete old Agent Pod %s/%s for resource fallback: %w", live.old.Namespace, live.old.Name, err)
		}
		ctrl.LoggerFrom(ctx).WithValues("daemonset", ds.Name, "node", live.nodeName, "oldPod", live.old.Name, "replacementPod", live.pending.Name).Info("Deleted old Agent Pod after proving the surged replacement was blocked only by node CPU or memory")
		if r.recorder != nil {
			r.recorder.Eventf(ddai, corev1.EventTypeWarning, "AgentResourceFallback", "Deleted old Agent Pod %s on node %s because replacement %s could not fit alongside it", live.old.Name, live.nodeName, live.pending.Name)
		}
		return reconcile.Result{RequeueAfter: time.Second}, nil
	}
	return reconcile.Result{}, nil
}

func fallbackCandidates(ds *appsv1.DaemonSet, pods []corev1.Pod, revision string, now time.Time) []fallbackCandidate {
	var candidates []fallbackCandidate
	for i := range pods {
		pending := &pods[i]
		shortage, ok := resourceOnlyUnschedulable(pending)
		if !ok || !resourceFallbackSchedulingShapeSafe(pending) || pending.DeletionTimestamp != nil || pending.Spec.NodeName != "" || pending.Status.NominatedNodeName != "" || pending.Labels[appsv1.DefaultDaemonSetUniqueLabelKey] != revision {
			continue
		}
		nodeName, ok := targetNodeFromDaemonSetAffinity(pending)
		if !ok {
			continue
		}
		var oldPods []*corev1.Pod
		for j := range pods {
			old := &pods[j]
			oldRevision := old.Labels[appsv1.DefaultDaemonSetUniqueLabelKey]
			if old.Spec.NodeName == nodeName && oldRevision != "" && oldRevision != revision && podAvailable(old, ds.Spec.MinReadySeconds, now) {
				oldPods = append(oldPods, old)
			}
		}
		if len(oldPods) != 1 {
			continue
		}
		reservedUID := pending.Annotations[resourceFallbackOldPodAnnotation]
		if reservedUID != "" && reservedUID != string(oldPods[0].UID) {
			continue
		}
		candidates = append(candidates, fallbackCandidate{pending: pending, old: oldPods[0], nodeName: nodeName, shortage: shortage, reserved: reservedUID != ""})
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].reserved != candidates[j].reserved {
			return candidates[i].reserved
		}
		return candidates[i].nodeName < candidates[j].nodeName
	})
	return candidates
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

func resourceFallbackSchedulingShapeSafe(pod *corev1.Pod) bool {
	if pod.Spec.SchedulerName != "" && pod.Spec.SchedulerName != corev1.DefaultSchedulerName || pod.Spec.RuntimeClassName != nil || len(pod.Spec.TopologySpreadConstraints) > 0 {
		return false
	}
	if pod.Spec.Affinity != nil {
		if pod.Spec.Affinity.PodAffinity != nil {
			return false
		}
		if pod.Spec.Affinity.PodAntiAffinity != nil {
			expected, ok := profileSurgePodAntiAffinity(pod.Labels)
			if !ok || !apiequality.Semantic.DeepEqual(pod.Spec.Affinity.PodAntiAffinity, expected) {
				return false
			}
		}
	}
	containers := append(append([]corev1.Container{}, pod.Spec.InitContainers...), pod.Spec.Containers...)
	for i := range containers {
		for _, port := range containers[i].Ports {
			if port.HostPort != 0 {
				return false
			}
		}
	}
	for i := range pod.Spec.Volumes {
		source := pod.Spec.Volumes[i].VolumeSource
		if source.EmptyDir == nil && source.HostPath == nil && source.ConfigMap == nil && source.Secret == nil && source.DownwardAPI == nil && source.Projected == nil {
			return false
		}
	}
	return true
}

func profileSurgePodAntiAffinitySatisfied(pending *corev1.Pod, nodePods []corev1.Pod) (bool, error) {
	if pending.Spec.Affinity == nil || pending.Spec.Affinity.PodAntiAffinity == nil {
		return true, nil
	}
	for _, term := range pending.Spec.Affinity.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution {
		selector, err := metav1.LabelSelectorAsSelector(term.LabelSelector)
		if err != nil {
			return false, fmt.Errorf("parse prepared Agent Pod anti-affinity: %w", err)
		}
		for i := range nodePods {
			pod := &nodePods[i]
			if pod.Namespace == pending.Namespace && selector.Matches(labels.Set(pod.Labels)) {
				return false, nil
			}
		}
	}
	return true, nil
}

// existingPodsAllowPendingByRequiredAntiAffinity checks the symmetric half of
// inter-pod anti-affinity: a scheduled Pod can reject the pending replacement.
func existingPodsAllowPendingByRequiredAntiAffinity(pending *corev1.Pod, existingPods []corev1.Pod, targetNodeName string) (bool, error) {
	for i := range existingPods {
		existing := &existingPods[i]
		if existing.Spec.NodeName == "" || existing.Spec.Affinity == nil || existing.Spec.Affinity.PodAntiAffinity == nil {
			continue
		}
		for _, term := range existing.Spec.Affinity.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution {
			selector, err := podAffinityTermSelector(&term, existing.Labels)
			if err != nil {
				return false, fmt.Errorf("parse existing Pod %s/%s anti-affinity: %w", existing.Namespace, existing.Name, err)
			}
			if !selector.Matches(labels.Set(pending.Labels)) || !affinityTermMaySelectNamespace(&term, existing.Namespace, pending.Namespace) {
				continue
			}
			if term.TopologyKey != corev1.LabelHostname || existing.Spec.NodeName == targetNodeName {
				return false, nil
			}
		}
	}
	return true, nil
}

func podAffinityTermSelector(term *corev1.PodAffinityTerm, sourceLabels map[string]string) (labels.Selector, error) {
	selector, err := metav1.LabelSelectorAsSelector(term.LabelSelector)
	if err != nil {
		return nil, err
	}
	for _, key := range term.MatchLabelKeys {
		if value, ok := sourceLabels[key]; ok {
			requirement, reqErr := labels.NewRequirement(key, selection.In, []string{value})
			if reqErr != nil {
				return nil, reqErr
			}
			selector = selector.Add(*requirement)
		}
	}
	for _, key := range term.MismatchLabelKeys {
		if value, ok := sourceLabels[key]; ok {
			requirement, reqErr := labels.NewRequirement(key, selection.NotIn, []string{value})
			if reqErr != nil {
				return nil, reqErr
			}
			selector = selector.Add(*requirement)
		}
	}
	return selector, nil
}

func affinityTermMaySelectNamespace(term *corev1.PodAffinityTerm, sourceNamespace, targetNamespace string) bool {
	if slices.Contains(term.Namespaces, targetNamespace) || term.NamespaceSelector != nil {
		return true
	}
	return len(term.Namespaces) == 0 && sourceNamespace == targetNamespace
}

func (r *Reconciler) revalidateFallbackCandidate(ctx context.Context, reader client.Reader, expectedDS *appsv1.DaemonSet, candidate fallbackCandidate, revision string, requireReservation bool) (*fallbackCandidate, error) {
	liveDS := &appsv1.DaemonSet{}
	if getErr := reader.Get(ctx, client.ObjectKeyFromObject(expectedDS), liveDS); getErr != nil {
		return nil, client.IgnoreNotFound(getErr)
	}
	if liveDS.UID != expectedDS.UID || liveDS.Generation != expectedDS.Generation || !preparedRolloutDaemonSetEligible(liveDS) || !hasRolloutMode(liveDS.Spec.Template.Annotations) {
		return nil, nil
	}
	liveRevision, err := currentDaemonSetRevision(ctx, reader, liveDS)
	if err != nil || liveRevision != revision {
		return nil, err
	}
	pending := &corev1.Pod{}
	old := &corev1.Pod{}
	if getErr := reader.Get(ctx, client.ObjectKeyFromObject(candidate.pending), pending); getErr != nil {
		return nil, client.IgnoreNotFound(getErr)
	}
	if getErr := reader.Get(ctx, client.ObjectKeyFromObject(candidate.old), old); getErr != nil {
		return nil, client.IgnoreNotFound(getErr)
	}
	if pending.UID != candidate.pending.UID || old.UID != candidate.old.UID || !controlledByUID(pending, liveDS.UID) || !controlledByUID(old, liveDS.UID) {
		return nil, nil
	}
	shortage, ok := resourceOnlyUnschedulable(pending)
	nodeName, targetOK := targetNodeFromDaemonSetAffinity(pending)
	reservation := pending.Annotations[resourceFallbackOldPodAnnotation]
	if !ok || !resourceFallbackSchedulingShapeSafe(pending) || !targetOK || nodeName != candidate.nodeName || pending.Spec.NodeName != "" || pending.Status.NominatedNodeName != "" || pending.DeletionTimestamp != nil || pending.Labels[appsv1.DefaultDaemonSetUniqueLabelKey] != revision || requireReservation && reservation != string(old.UID) || reservation != "" && reservation != string(old.UID) {
		return nil, nil
	}
	if old.Spec.NodeName != nodeName || old.Labels[appsv1.DefaultDaemonSetUniqueLabelKey] == "" || old.Labels[appsv1.DefaultDaemonSetUniqueLabelKey] == revision || !podAvailable(old, liveDS.Spec.MinReadySeconds, time.Now()) {
		return nil, nil
	}
	node := &corev1.Node{}
	if getErr := reader.Get(ctx, client.ObjectKey{Name: nodeName}, node); getErr != nil {
		return nil, client.IgnoreNotFound(getErr)
	}
	if !nodeReadyForResourceFallback(node) || !toleratesBlockingNodeTaints(pending.Spec.Tolerations, node.Spec.Taints) {
		return nil, nil
	}
	matches, err := nodeaffinity.GetRequiredNodeAffinity(pending).Match(node)
	if err != nil || !matches {
		return nil, err
	}
	nodePods := &corev1.PodList{}
	if listErr := reader.List(ctx, nodePods, client.MatchingFields{"spec.nodeName": nodeName}); listErr != nil {
		return nil, fmt.Errorf("list Pods on node %s for Agent resource fallback: %w", nodeName, listErr)
	}
	affinitySatisfied, err := profileSurgePodAntiAffinitySatisfied(pending, nodePods.Items)
	if err != nil || !affinitySatisfied {
		return nil, err
	}
	clusterPods := &corev1.PodList{}
	if listErr := reader.List(ctx, clusterPods); listErr != nil {
		return nil, fmt.Errorf("list cluster Pods for Agent anti-affinity fallback safety: %w", listErr)
	}
	existingAffinitySatisfied, err := existingPodsAllowPendingByRequiredAntiAffinity(pending, clusterPods.Items, nodeName)
	if err != nil || !existingAffinitySatisfied {
		return nil, err
	}
	if !resourceFitAfterOldPodRemoval(node, nodePods.Items, pending, old, shortage) {
		return nil, nil
	}
	return &fallbackCandidate{pending: pending, old: old, nodeName: nodeName, shortage: shortage, reserved: reservation != ""}, nil
}

func resourceFallbackBudgetWithinLimit(ctx context.Context, reader client.Reader, expectedDS *appsv1.DaemonSet, budgetValue intstr.IntOrString, revision string) (bool, error) {
	liveDS := &appsv1.DaemonSet{}
	if err := reader.Get(ctx, client.ObjectKeyFromObject(expectedDS), liveDS); err != nil {
		return false, client.IgnoreNotFound(err)
	}
	if liveDS.UID != expectedDS.UID || liveDS.Generation != expectedDS.Generation || !preparedRolloutDaemonSetEligible(liveDS) {
		return false, nil
	}
	liveRevision, err := currentDaemonSetRevision(ctx, reader, liveDS)
	if err != nil || liveRevision != revision {
		return false, err
	}
	pods, err := daemonSetPods(ctx, reader, liveDS)
	if err != nil {
		return false, err
	}
	budget, err := intstr.GetScaledValueFromIntOrPercent(&budgetValue, int(liveDS.Status.DesiredNumberScheduled), true)
	if err != nil || budget <= 0 {
		return false, err
	}
	return consumedResourceFallbackBudget(liveDS, pods, revision, time.Now()) <= budget, nil
}

func consumedResourceFallbackBudget(ds *appsv1.DaemonSet, pods []corev1.Pod, revision string, now time.Time) int {
	availableByNode := map[string]bool{}
	knownNodes := map[string]bool{}
	for i := range pods {
		pod := &pods[i]
		nodeName := pod.Spec.NodeName
		if nodeName == "" {
			nodeName, _ = targetNodeFromDaemonSetAffinity(pod)
		}
		if nodeName == "" {
			continue
		}
		knownNodes[nodeName] = true
		if podAvailable(pod, ds.Spec.MinReadySeconds, now) {
			availableByNode[nodeName] = true
		}
	}

	reservations := 0
	reservedUnavailable := 0
	for i := range pods {
		pod := &pods[i]
		if pod.Labels[appsv1.DefaultDaemonSetUniqueLabelKey] != revision || pod.Annotations[resourceFallbackOldPodAnnotation] == "" || podAvailable(pod, ds.Spec.MinReadySeconds, now) {
			continue
		}
		nodeName := pod.Spec.NodeName
		if nodeName == "" {
			nodeName, _ = targetNodeFromDaemonSetAffinity(pod)
		}
		reservations++
		if nodeName == "" || !availableByNode[nodeName] {
			reservedUnavailable++
		}
	}

	liveUnavailable := 0
	for nodeName := range knownNodes {
		if !availableByNode[nodeName] {
			liveUnavailable++
		}
	}
	if missing := int(ds.Status.DesiredNumberScheduled) - len(knownNodes); missing > 0 {
		liveUnavailable += missing
	}
	statusBeyondLive := max(0, int(ds.Status.NumberUnavailable)-liveUnavailable)
	return reservations + liveUnavailable - min(liveUnavailable, reservedUnavailable) + statusBeyondLive
}

func toleratesBlockingNodeTaints(tolerations []corev1.Toleration, taints []corev1.Taint) bool {
	for i := range taints {
		taint := &taints[i]
		if taint.Effect != corev1.TaintEffectNoSchedule && taint.Effect != corev1.TaintEffectNoExecute {
			continue
		}
		tolerated := false
		for j := range tolerations {
			toleration := &tolerations[j]
			if toleration.Effect != "" && toleration.Effect != taint.Effect {
				continue
			}
			operator := toleration.Operator
			if operator == "" {
				operator = corev1.TolerationOpEqual
			}
			if operator == corev1.TolerationOpExists && (toleration.Key == "" || toleration.Key == taint.Key) || operator == corev1.TolerationOpEqual && toleration.Key == taint.Key && toleration.Value == taint.Value {
				tolerated = true
				break
			}
		}
		if !tolerated {
			return false
		}
	}
	return true
}

func nodeReadyForResourceFallback(node *corev1.Node) bool {
	if node.Spec.Unschedulable || node.DeletionTimestamp != nil {
		return false
	}
	ready := false
	pressure := map[corev1.NodeConditionType]bool{
		corev1.NodeMemoryPressure: false,
		corev1.NodeDiskPressure:   false,
		corev1.NodePIDPressure:    false,
	}
	for i := range node.Status.Conditions {
		condition := node.Status.Conditions[i]
		switch condition.Type {
		case corev1.NodeReady:
			ready = condition.Status == corev1.ConditionTrue
		case corev1.NodeMemoryPressure, corev1.NodeDiskPressure, corev1.NodePIDPressure:
			if condition.Status != corev1.ConditionFalse {
				return false
			}
			pressure[condition.Type] = true
		case corev1.NodeNetworkUnavailable:
			if condition.Status != corev1.ConditionFalse {
				return false
			}
		}
	}
	return ready && pressure[corev1.NodeMemoryPressure] && pressure[corev1.NodeDiskPressure] && pressure[corev1.NodePIDPressure]
}

func resourceFitAfterOldPodRemoval(node *corev1.Node, nodePods []corev1.Pod, replacement, old *corev1.Pod, shortage resourceShortage) bool {
	if len(replacement.Spec.ResourceClaims) > 0 {
		return false
	}
	used := corev1.ResourceList{}
	oldFound := false
	for i := range nodePods {
		pod := &nodePods[i]
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		addResources(used, schedulerPodRequests(pod))
		oldFound = oldFound || pod.UID == old.UID
	}
	if !oldFound {
		return false
	}
	before := copyResources(used)
	addResources(before, schedulerPodRequests(replacement))
	after := copyResources(used)
	subtractResources(after, schedulerPodRequests(old))
	addResources(after, schedulerPodRequests(replacement))
	for _, name := range []corev1.ResourceName{corev1.ResourceCPU, corev1.ResourceMemory} {
		reported := name == corev1.ResourceCPU && shortage.cpu || name == corev1.ResourceMemory && shortage.memory
		oldRequest := schedulerPodRequests(old)[name]
		if reported && (!resourceExceeds(before, node.Status.Allocatable, name) || oldRequest.Sign() <= 0) {
			return false
		}
	}
	return resourcesFit(after, node.Status.Allocatable)
}

func schedulerPodRequests(pod *corev1.Pod) corev1.ResourceList {
	return resourcehelper.PodRequests(pod, resourcehelper.PodResourcesOptions{UseStatusResources: true, InPlacePodLevelResourcesVerticalScalingEnabled: true})
}

func copyResources(resources corev1.ResourceList) corev1.ResourceList {
	result := make(corev1.ResourceList, len(resources))
	for name, quantity := range resources {
		result[name] = quantity.DeepCopy()
	}
	return result
}

func addResources(target, values corev1.ResourceList) {
	for name, value := range values {
		quantity := target[name]
		quantity.Add(value)
		target[name] = quantity
	}
}

func subtractResources(target, values corev1.ResourceList) {
	for name, value := range values {
		quantity := target[name]
		quantity.Sub(value)
		target[name] = quantity
	}
}

func resourceExceeds(requests, allocatable corev1.ResourceList, name corev1.ResourceName) bool {
	request := requests[name]
	available := allocatable[name]
	return request.Cmp(available) > 0
}

func resourcesFit(requests, allocatable corev1.ResourceList) bool {
	for name, request := range requests {
		if request.Sign() <= 0 {
			continue
		}
		available, ok := allocatable[name]
		if !ok || request.Cmp(available) > 0 {
			return false
		}
	}
	return true
}
