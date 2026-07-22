// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// Copyright 2016-present Datadog, Inc.

package datadogagentinternal

import (
	"context"
	"encoding/json"
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
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	resourcehelper "k8s.io/component-helpers/resource"
	"k8s.io/component-helpers/scheduling/corev1/nodeaffinity"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	datadoghqcommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	componentagent "github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/agent"
	"github.com/DataDog/datadog-operator/pkg/constants"
)

const (
	resourceFallbackOldPodAnnotation = "agent.datadoghq.com/resource-fallback-old-pod-uid"
	apiPodNodeNameField              = "spec.nodeName"
	defaultFallbackMaxUnavailable    = 1
)

// configureResourceFallback keeps surge opt-in. When it is requested, the
// existing maxUnavailable setting becomes both the surge limit and the
// Operator's resource-pressure fallback budget. The emitted maxUnavailable is
// intentionally 0 so the native DaemonSet controller never proactively
// deletes an old Pod; only the resource-proven fallback below may do that.
func configureResourceFallback(ds *appsv1.DaemonSet, budget intstr.IntOrString) bool {
	strategy := &ds.Spec.UpdateStrategy
	if strategy.Type != "" && strategy.Type != appsv1.RollingUpdateDaemonSetStrategyType {
		return false
	}
	if strategy.RollingUpdate == nil || !positiveIntOrPercent(strategy.RollingUpdate.MaxSurge) {
		return false
	}
	if _, err := intstr.GetScaledValueFromIntOrPercent(&budget, 100, true); err != nil {
		return false
	}

	strategy.Type = appsv1.RollingUpdateDaemonSetStrategyType
	zero := intstr.FromInt(0)
	strategy.RollingUpdate.MaxUnavailable = &zero
	if positiveIntOrPercent(&budget) {
		surge := budget
		strategy.RollingUpdate.MaxSurge = &surge
		return true
	}
	return false
}

// prepareProfileAntiAffinityForSurge narrows the standard DAP anti-affinity so
// only old and new revisions of the same profile and DDA may overlap. Unknown
// or user-supplied anti-affinity fails closed.
func prepareProfileAntiAffinityForSurge(template *corev1.PodTemplateSpec) bool {
	if template.Spec.Affinity == nil || template.Spec.Affinity.PodAntiAffinity == nil {
		return true
	}
	if !apiequality.Semantic.DeepEqual(template.Spec.Affinity.PodAntiAffinity, broadAgentPodAntiAffinity()) {
		return false
	}
	narrowed, ok := profileSurgePodAntiAffinity(template.Labels)
	if !ok {
		return false
	}
	template.Spec.Affinity.PodAntiAffinity = narrowed
	return true
}

func broadAgentPodAntiAffinity() *corev1.PodAntiAffinity {
	return &corev1.PodAntiAffinity{RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{{
		LabelSelector: &metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{{
			Key:      datadoghqcommon.AgentDeploymentComponentLabelKey,
			Operator: metav1.LabelSelectorOpIn,
			Values:   []string{constants.DefaultAgentResourceSuffix},
		}}},
		TopologyKey: corev1.LabelHostname,
	}}}
}

func profileSurgePodAntiAffinity(podLabels map[string]string) (*corev1.PodAntiAffinity, bool) {
	ddaName := podLabels[datadoghqcommon.AgentDeploymentNameLabelKey]
	if ddaName == "" {
		return nil, false
	}

	profileRequirement := metav1.LabelSelectorRequirement{Key: constants.ProfileLabelKey}
	if profileName := podLabels[constants.ProfileLabelKey]; profileName != "" {
		profileRequirement.Operator = metav1.LabelSelectorOpNotIn
		profileRequirement.Values = []string{profileName}
	} else {
		profileRequirement.Operator = metav1.LabelSelectorOpExists
	}
	componentRequirement := metav1.LabelSelectorRequirement{
		Key:      datadoghqcommon.AgentDeploymentComponentLabelKey,
		Operator: metav1.LabelSelectorOpIn,
		Values:   []string{constants.DefaultAgentResourceSuffix},
	}

	return &corev1.PodAntiAffinity{RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
		{
			LabelSelector: &metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{
				componentRequirement,
				{Key: datadoghqcommon.AgentDeploymentNameLabelKey, Operator: metav1.LabelSelectorOpIn, Values: []string{ddaName}},
				profileRequirement,
			}},
			TopologyKey: corev1.LabelHostname,
		},
		{
			LabelSelector: &metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{
				componentRequirement,
				{Key: datadoghqcommon.AgentDeploymentNameLabelKey, Operator: metav1.LabelSelectorOpNotIn, Values: []string{ddaName}},
			}},
			TopologyKey: corev1.LabelHostname,
		},
	}}, true
}

func positiveIntOrPercent(value *intstr.IntOrString) bool {
	if value == nil {
		return false
	}
	scaled, err := intstr.GetScaledValueFromIntOrPercent(value, 100, true)
	return err == nil && scaled > 0
}

func resourceFallbackBudget(ddai *datadoghqv1alpha1.DatadogAgentInternal, options *componentagent.ExtendedDaemonsetOptions) intstr.IntOrString {
	if override, ok := ddai.Spec.Override[datadoghqv2alpha1.NodeAgentComponentName]; ok && override != nil && override.UpdateStrategy != nil && override.UpdateStrategy.RollingUpdate != nil && override.UpdateStrategy.RollingUpdate.MaxUnavailable != nil {
		return *override.UpdateStrategy.RollingUpdate.MaxUnavailable
	}
	if options != nil && options.MaxPodUnavailable != "" {
		return intstr.Parse(options.MaxPodUnavailable)
	}
	return intstr.FromInt(defaultFallbackMaxUnavailable)
}

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

func (r *Reconciler) reconcileResourceFallback(ctx context.Context, ddai *datadoghqv1alpha1.DatadogAgentInternal, expectedDS *appsv1.DaemonSet, budgetValue intstr.IntOrString) (reconcile.Result, error) {
	reader := r.apiReader
	if reader == nil {
		reader = r.client
	}

	ds := &appsv1.DaemonSet{}
	key := client.ObjectKeyFromObject(expectedDS)
	if err := reader.Get(ctx, key, ds); err != nil {
		if apierrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, fmt.Errorf("get Agent DaemonSet for resource fallback: %w", err)
	}
	if !daemonSetControlledByDDAI(ds, ddai) || !resourceFallbackDaemonSetEligible(ds) {
		return reconcile.Result{}, nil
	}

	desired := int(ds.Status.DesiredNumberScheduled)
	budget, err := intstr.GetScaledValueFromIntOrPercent(&budgetValue, desired, true)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("resolve Agent resource fallback budget: %w", err)
	}
	if budget <= 0 {
		return reconcile.Result{}, nil
	}

	currentRevision, err := currentDaemonSetRevision(ctx, reader, ds)
	if err != nil {
		return reconcile.Result{}, err
	}
	if currentRevision == "" {
		return reconcile.Result{}, nil
	}

	pods, err := daemonSetPods(ctx, reader, ds)
	if err != nil {
		return reconcile.Result{}, err
	}
	consumed := consumedFallbackBudget(ds, pods, currentRevision, time.Now())
	candidates := fallbackCandidates(ds, pods, currentRevision, time.Now())
	if consumed > budget {
		return reconcile.Result{}, nil
	}

	for _, candidate := range candidates {
		if !candidate.reserved && consumed >= budget {
			break
		}

		if !candidate.reserved {
			liveCandidate, err := r.revalidateFallbackCandidate(ctx, reader, ds, candidate, currentRevision, false)
			if err != nil {
				return reconcile.Result{}, err
			}
			if liveCandidate == nil {
				continue
			}
			candidate = *liveCandidate
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
			consumed++
		}

		liveCandidate, err := r.revalidateFallbackCandidate(ctx, reader, ds, candidate, currentRevision, true)
		if err != nil {
			return reconcile.Result{}, err
		}
		if liveCandidate == nil {
			continue
		}
		withinBudget, err := fallbackBudgetWithinLimit(ctx, reader, ds, budgetValue, currentRevision)
		if err != nil {
			return reconcile.Result{}, err
		}
		if !withinBudget {
			return reconcile.Result{RequeueAfter: time.Second}, nil
		}

		uid := liveCandidate.old.UID
		if err := r.client.Delete(ctx, liveCandidate.old, &client.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid}}); err != nil && !apierrors.IsNotFound(err) {
			return reconcile.Result{}, fmt.Errorf("delete old Agent Pod %s/%s for resource fallback: %w", liveCandidate.old.Namespace, liveCandidate.old.Name, err)
		}

		logger := ctrl.LoggerFrom(ctx).WithValues("daemonset", ds.Name, "node", liveCandidate.nodeName, "oldPod", liveCandidate.old.Name, "replacementPod", liveCandidate.pending.Name)
		logger.Info("Deleted old Agent Pod after proving the surged replacement was blocked only by node CPU or memory")
		if r.recorder != nil {
			r.recorder.Eventf(ddai, corev1.EventTypeWarning, "AgentResourceFallback", "Deleted old Agent Pod %s on node %s because replacement %s could not fit alongside it", liveCandidate.old.Name, liveCandidate.nodeName, liveCandidate.pending.Name)
		}
		return reconcile.Result{RequeueAfter: time.Second}, nil
	}

	return reconcile.Result{}, nil
}

func fallbackBudgetWithinLimit(ctx context.Context, reader client.Reader, expectedDS *appsv1.DaemonSet, budgetValue intstr.IntOrString, expectedRevision string) (bool, error) {
	liveDS := &appsv1.DaemonSet{}
	if err := reader.Get(ctx, client.ObjectKeyFromObject(expectedDS), liveDS); err != nil {
		return false, client.IgnoreNotFound(err)
	}
	if liveDS.UID != expectedDS.UID || liveDS.Generation != expectedDS.Generation || !resourceFallbackDaemonSetEligible(liveDS) {
		return false, nil
	}
	revision, err := currentDaemonSetRevision(ctx, reader, liveDS)
	if err != nil || revision != expectedRevision {
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
	return consumedFallbackBudget(liveDS, pods, revision, time.Now()) <= budget, nil
}

func daemonSetControlledByDDAI(ds *appsv1.DaemonSet, ddai *datadoghqv1alpha1.DatadogAgentInternal) bool {
	owner := metav1.GetControllerOf(ds)
	return owner != nil && owner.APIVersion == datadoghqv1alpha1.GroupVersion.String() && owner.Kind == "DatadogAgentInternal" && owner.UID == ddai.UID
}

func resourceFallbackDaemonSetEligible(ds *appsv1.DaemonSet) bool {
	if ds.DeletionTimestamp != nil || ds.Status.DesiredNumberScheduled <= 0 || ds.Status.ObservedGeneration != ds.Generation {
		return false
	}
	if ds.Spec.UpdateStrategy.Type != appsv1.RollingUpdateDaemonSetStrategyType || ds.Spec.UpdateStrategy.RollingUpdate == nil || !positiveIntOrPercent(ds.Spec.UpdateStrategy.RollingUpdate.MaxSurge) {
		return false
	}
	return true
}

func currentDaemonSetRevision(ctx context.Context, reader client.Reader, ds *appsv1.DaemonSet) (string, error) {
	revisions := &appsv1.ControllerRevisionList{}
	if err := reader.List(ctx, revisions, client.InNamespace(ds.Namespace)); err != nil {
		return "", fmt.Errorf("list revisions for Agent DaemonSet %s/%s: %w", ds.Namespace, ds.Name, err)
	}
	var current *appsv1.ControllerRevision
	for i := range revisions.Items {
		revision := &revisions.Items[i]
		if !controlledByUID(revision, ds.UID) {
			continue
		}
		matches, err := controllerRevisionMatchesTemplate(revision, &ds.Spec.Template)
		if err != nil {
			return "", fmt.Errorf("decode revision %s for Agent DaemonSet %s/%s: %w", revision.Name, ds.Namespace, ds.Name, err)
		}
		if matches && (current == nil || revision.Revision > current.Revision) {
			current = revision
		}
	}
	if current == nil {
		return "", nil
	}
	return current.Labels[appsv1.DefaultDaemonSetUniqueLabelKey], nil
}

func controllerRevisionMatchesTemplate(revision *appsv1.ControllerRevision, template *corev1.PodTemplateSpec) (bool, error) {
	var patch struct {
		Spec struct {
			Template corev1.PodTemplateSpec `json:"template"`
		} `json:"spec"`
	}
	if err := json.Unmarshal(revision.Data.Raw, &patch); err != nil {
		return false, err
	}
	return apiequality.Semantic.DeepEqual(patch.Spec.Template, *template), nil
}

func daemonSetPods(ctx context.Context, reader client.Reader, ds *appsv1.DaemonSet) ([]corev1.Pod, error) {
	selector, err := metav1.LabelSelectorAsSelector(ds.Spec.Selector)
	if err != nil {
		return nil, fmt.Errorf("build selector for Agent DaemonSet %s/%s: %w", ds.Namespace, ds.Name, err)
	}
	list := &corev1.PodList{}
	if err := reader.List(ctx, list, client.InNamespace(ds.Namespace), client.MatchingLabelsSelector{Selector: selector}); err != nil {
		return nil, fmt.Errorf("list Pods for Agent DaemonSet %s/%s: %w", ds.Namespace, ds.Name, err)
	}
	result := make([]corev1.Pod, 0, len(list.Items))
	for i := range list.Items {
		if controlledByUID(&list.Items[i], ds.UID) {
			result = append(result, list.Items[i])
		}
	}
	return result, nil
}

func controlledByUID(obj metav1.Object, uid types.UID) bool {
	owner := metav1.GetControllerOf(obj)
	return owner != nil && owner.UID == uid
}

func fallbackCandidates(ds *appsv1.DaemonSet, pods []corev1.Pod, currentRevision string, now time.Time) []fallbackCandidate {
	result := make([]fallbackCandidate, 0)
	for i := range pods {
		pending := &pods[i]
		shortage, ok := resourceOnlyUnschedulable(pending)
		if !ok || !resourceFallbackSchedulingShapeSafe(pending) || pending.DeletionTimestamp != nil || pending.Spec.NodeName != "" || pending.Status.NominatedNodeName != "" || pending.Labels[appsv1.DefaultDaemonSetUniqueLabelKey] != currentRevision {
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
			if old.Spec.NodeName == nodeName && oldRevision != "" && oldRevision != currentRevision && podAvailable(old, ds.Spec.MinReadySeconds, now) {
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
		result = append(result, fallbackCandidate{pending: pending, old: oldPods[0], nodeName: nodeName, shortage: shortage, reserved: reservedUID != ""})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].reserved != result[j].reserved {
			return result[i].reserved
		}
		return result[i].nodeName < result[j].nodeName
	})
	return result
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
	if primary == "" {
		return resourceShortage{}, false
	}

	var shortage resourceShortage
	for reason := range strings.SplitSeq(primary, ", ") {
		fields := strings.Fields(strings.ToLower(strings.TrimSpace(reason)))
		if len(fields) < 2 {
			return resourceShortage{}, false
		}
		if _, err := strconv.Atoi(fields[0]); err != nil {
			return resourceShortage{}, false
		}
		normalized := strings.Join(fields[1:], " ")
		switch normalized {
		case "insufficient cpu":
			shortage.cpu = true
		case "insufficient memory":
			shortage.memory = true
		case "node(s) didn't match pod's node affinity/selector", "node(s) didn't satisfy plugin(s) [nodeaffinity]":
			// Expected for every non-target node because DaemonSet surge Pods are
			// pinned through required node affinity.
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
		termTarget := ""
		for _, requirement := range term.MatchFields {
			if requirement.Key != metav1.ObjectNameField {
				continue
			}
			if requirement.Operator != corev1.NodeSelectorOpIn || len(requirement.Values) != 1 || requirement.Values[0] == "" || termTarget != "" {
				return "", false
			}
			termTarget = requirement.Values[0]
		}
		if termTarget == "" || (target != "" && termTarget != target) {
			return "", false
		}
		target = termTarget
	}
	return target, target != ""
}

// resourceFallbackSchedulingShapeSafe rejects Pod-declared constraints whose
// scheduler failure could be masked by a simultaneous CPU or memory shortage.
// Cluster-specific plugins configured under the default scheduler name are not
// visible through the Pod API and must be excluded operationally.
func resourceFallbackSchedulingShapeSafe(pod *corev1.Pod) bool {
	if pod.Spec.SchedulerName != "" && pod.Spec.SchedulerName != corev1.DefaultSchedulerName {
		return false
	}
	if pod.Spec.RuntimeClassName != nil || len(pod.Spec.TopologySpreadConstraints) > 0 {
		return false
	}
	if pod.Spec.Affinity != nil {
		if pod.Spec.Affinity.PodAffinity != nil {
			return false
		}
		if pod.Spec.Affinity.PodAntiAffinity != nil && !profileSurgePodAntiAffinitySafe(pod) {
			return false
		}
	}
	for _, container := range append(append([]corev1.Container{}, pod.Spec.InitContainers...), pod.Spec.Containers...) {
		for _, port := range container.Ports {
			if port.HostPort != 0 {
				return false
			}
		}
	}
	for _, volume := range pod.Spec.Volumes {
		source := volume.VolumeSource
		if source.EmptyDir == nil && source.HostPath == nil && source.ConfigMap == nil && source.Secret == nil && source.DownwardAPI == nil && source.Projected == nil {
			return false
		}
	}
	return true
}

// profileSurgePodAntiAffinitySafe recognizes the exact anti-affinity emitted
// for DatadogAgentProfiles. It excludes only other profiles, so deleting the
// old Pod of the same profile cannot reveal a hidden anti-affinity blocker.
func profileSurgePodAntiAffinitySafe(pod *corev1.Pod) bool {
	expected, ok := profileSurgePodAntiAffinity(pod.Labels)
	if !ok {
		return false
	}
	return apiequality.Semantic.DeepEqual(pod.Spec.Affinity.PodAntiAffinity, expected)
}

func profileSurgePodAntiAffinitySatisfied(pending *corev1.Pod, nodePods []corev1.Pod) (bool, error) {
	if pending.Spec.Affinity != nil && pending.Spec.Affinity.PodAntiAffinity != nil {
		if !profileSurgePodAntiAffinitySafe(pending) {
			return false, nil
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
	}
	return true, nil
}

// existingPodsAllowPendingByRequiredAntiAffinity checks the symmetric half of
// inter-pod anti-affinity: an already scheduled Pod can reject the pending
// replacement even when the replacement's own terms allow it.
func existingPodsAllowPendingByRequiredAntiAffinity(pending *corev1.Pod, existingPods []corev1.Pod, targetNodeName string) (bool, error) {
	for i := range existingPods {
		existing := &existingPods[i]
		if existing.Spec.NodeName == "" {
			continue
		}
		if existing.Spec.Affinity == nil || existing.Spec.Affinity.PodAntiAffinity == nil {
			continue
		}
		for _, term := range existing.Spec.Affinity.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution {
			selector, err := podAffinityTermSelector(&term, existing.Labels)
			if err != nil {
				return false, fmt.Errorf("parse existing Pod %s/%s anti-affinity: %w", existing.Namespace, existing.Name, err)
			}
			if !selector.Matches(labels.Set(pending.Labels)) {
				continue
			}
			if !affinityTermMaySelectNamespace(&term, existing.Namespace, pending.Namespace) {
				continue
			}
			// Pods on the target node cover hostname topology. For wider
			// topologies, conservatively reject because this lightweight check
			// does not fetch every Node's topology labels.
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
			requirement, err := labels.NewRequirement(key, selection.In, []string{value})
			if err != nil {
				return nil, err
			}
			selector = selector.Add(*requirement)
		}
	}
	for _, key := range term.MismatchLabelKeys {
		if value, ok := sourceLabels[key]; ok {
			requirement, err := labels.NewRequirement(key, selection.NotIn, []string{value})
			if err != nil {
				return nil, err
			}
			selector = selector.Add(*requirement)
		}
	}
	return selector, nil
}

func affinityTermMaySelectNamespace(term *corev1.PodAffinityTerm, sourceNamespace, targetNamespace string) bool {
	if slices.Contains(term.Namespaces, targetNamespace) {
		return true
	}
	// Namespace labels are intentionally not part of the fallback cache. Fail
	// closed when a namespace selector could include the pending Pod.
	if term.NamespaceSelector != nil {
		return true
	}
	return len(term.Namespaces) == 0 && sourceNamespace == targetNamespace
}

func consumedFallbackBudget(ds *appsv1.DaemonSet, pods []corev1.Pod, currentRevision string, now time.Time) int {
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
		if pod.Labels[appsv1.DefaultDaemonSetUniqueLabelKey] != currentRevision || pod.Annotations[resourceFallbackOldPodAnnotation] == "" || podAvailable(pod, ds.Spec.MinReadySeconds, now) {
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
	if missingNodes := int(ds.Status.DesiredNumberScheduled) - len(knownNodes); missingNodes > 0 {
		liveUnavailable += missingNodes
	}

	statusUnavailable := int(ds.Status.NumberUnavailable)
	statusBeyondLive := max(0, statusUnavailable-liveUnavailable)
	return reservations + liveUnavailable - min(liveUnavailable, reservedUnavailable) + statusBeyondLive
}

func podAvailable(pod *corev1.Pod, minReadySeconds int32, now time.Time) bool {
	if pod.DeletionTimestamp != nil || pod.Status.Phase != corev1.PodRunning {
		return false
	}
	for i := range pod.Status.Conditions {
		condition := &pod.Status.Conditions[i]
		if condition.Type != corev1.PodReady || condition.Status != corev1.ConditionTrue {
			continue
		}
		return minReadySeconds == 0 || !condition.LastTransitionTime.IsZero() && condition.LastTransitionTime.Add(time.Duration(minReadySeconds)*time.Second).Before(now)
	}
	return false
}

func (r *Reconciler) revalidateFallbackCandidate(ctx context.Context, reader client.Reader, expectedDS *appsv1.DaemonSet, candidate fallbackCandidate, expectedRevision string, requireReservation bool) (*fallbackCandidate, error) {
	liveDS := &appsv1.DaemonSet{}
	if err := reader.Get(ctx, client.ObjectKeyFromObject(expectedDS), liveDS); err != nil {
		return nil, client.IgnoreNotFound(err)
	}
	if liveDS.UID != expectedDS.UID || liveDS.Generation != expectedDS.Generation || !resourceFallbackDaemonSetEligible(liveDS) {
		return nil, nil
	}
	liveRevision, err := currentDaemonSetRevision(ctx, reader, liveDS)
	if err != nil || liveRevision != expectedRevision {
		return nil, err
	}

	pending := &corev1.Pod{}
	if getErr := reader.Get(ctx, client.ObjectKeyFromObject(candidate.pending), pending); getErr != nil {
		return nil, client.IgnoreNotFound(getErr)
	}
	old := &corev1.Pod{}
	if getErr := reader.Get(ctx, client.ObjectKeyFromObject(candidate.old), old); getErr != nil {
		return nil, client.IgnoreNotFound(getErr)
	}
	if !controlledByUID(pending, liveDS.UID) || !controlledByUID(old, liveDS.UID) || pending.UID != candidate.pending.UID || old.UID != candidate.old.UID {
		return nil, nil
	}
	shortage, ok := resourceOnlyUnschedulable(pending)
	nodeName, targetOK := targetNodeFromDaemonSetAffinity(pending)
	reservation := pending.Annotations[resourceFallbackOldPodAnnotation]
	if !ok || !resourceFallbackSchedulingShapeSafe(pending) || !targetOK || nodeName != candidate.nodeName || pending.Spec.NodeName != "" || pending.Status.NominatedNodeName != "" || pending.DeletionTimestamp != nil || pending.Labels[appsv1.DefaultDaemonSetUniqueLabelKey] != liveRevision || requireReservation && reservation != string(old.UID) || reservation != "" && reservation != string(old.UID) {
		return nil, nil
	}
	if old.Spec.NodeName != nodeName || old.Labels[appsv1.DefaultDaemonSetUniqueLabelKey] == "" || old.Labels[appsv1.DefaultDaemonSetUniqueLabelKey] == liveRevision || !podAvailable(old, liveDS.Spec.MinReadySeconds, time.Now()) {
		return nil, nil
	}

	node := &corev1.Node{}
	if getErr := reader.Get(ctx, client.ObjectKey{Name: nodeName}, node); getErr != nil {
		return nil, client.IgnoreNotFound(getErr)
	}
	if !nodeReadyForResourceFallback(node) {
		return nil, nil
	}
	if !toleratesBlockingNodeTaints(pending.Spec.Tolerations, node.Spec.Taints) {
		return nil, nil
	}
	matches, err := nodeaffinity.GetRequiredNodeAffinity(pending).Match(node)
	if err != nil || !matches {
		return nil, err
	}
	nodePods := &corev1.PodList{}
	if listErr := reader.List(ctx, nodePods, client.MatchingFields{apiPodNodeNameField: nodeName}); listErr != nil {
		return nil, fmt.Errorf("list Pods on node %s for Agent resource fallback: %w", nodeName, listErr)
	}
	affinitySatisfied, err := profileSurgePodAntiAffinitySatisfied(pending, nodePods.Items)
	if err != nil {
		return nil, err
	}
	if !affinitySatisfied {
		return nil, nil
	}
	clusterPods := &corev1.PodList{}
	if listErr := reader.List(ctx, clusterPods); listErr != nil {
		return nil, fmt.Errorf("list cluster Pods for Agent anti-affinity fallback safety: %w", listErr)
	}
	existingAffinitySatisfied, err := existingPodsAllowPendingByRequiredAntiAffinity(pending, clusterPods.Items, nodeName)
	if err != nil {
		return nil, err
	}
	if !existingAffinitySatisfied {
		return nil, nil
	}
	if !resourceFitAfterOldPodRemoval(node, nodePods.Items, pending, old, shortage) {
		return nil, nil
	}

	return &fallbackCandidate{pending: pending, old: old, nodeName: nodeName, shortage: shortage, reserved: reservation != ""}, nil
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
			switch operator {
			case corev1.TolerationOpExists:
				tolerated = toleration.Key == "" || toleration.Key == taint.Key
			case corev1.TolerationOpEqual:
				tolerated = toleration.Key == taint.Key && toleration.Value == taint.Value
			}
			if tolerated {
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
	pressureHealthy := map[corev1.NodeConditionType]bool{
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
			pressureHealthy[condition.Type] = true
		case corev1.NodeNetworkUnavailable:
			if condition.Status != corev1.ConditionFalse {
				return false
			}
		}
	}
	return ready && pressureHealthy[corev1.NodeMemoryPressure] && pressureHealthy[corev1.NodeDiskPressure] && pressureHealthy[corev1.NodePIDPressure]
}

func resourceFitAfterOldPodRemoval(node *corev1.Node, nodePods []corev1.Pod, replacement, old *corev1.Pod, shortage resourceShortage) bool {
	if len(replacement.Spec.ResourceClaims) > 0 {
		return false
	}
	used := corev1.ResourceList{}
	oldFound := false
	podCount := int64(0)
	for i := range nodePods {
		pod := &nodePods[i]
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		podCount++
		addResources(used, schedulerPodRequests(pod))
		if pod.UID == old.UID {
			oldFound = true
		}
	}
	if !oldFound {
		return false
	}

	replacementRequests := schedulerPodRequests(replacement)
	oldRequests := schedulerPodRequests(old)
	before := copyResources(used)
	addResources(before, replacementRequests)
	after := copyResources(used)
	subtractResources(after, oldRequests)
	addResources(after, replacementRequests)

	shortageStillPresent := false
	for _, resourceName := range []corev1.ResourceName{corev1.ResourceCPU, corev1.ResourceMemory} {
		reported := resourceName == corev1.ResourceCPU && shortage.cpu || resourceName == corev1.ResourceMemory && shortage.memory
		if !reported {
			continue
		}
		if !resourceExceeds(before, node.Status.Allocatable, resourceName) {
			return false
		}
		oldRequest := oldRequests[resourceName]
		if oldRequest.Sign() <= 0 {
			return false
		}
		shortageStillPresent = true
	}
	if !shortageStillPresent || !resourcesFit(after, node.Status.Allocatable) {
		return false
	}
	if allocatablePods, ok := node.Status.Allocatable[corev1.ResourcePods]; ok && podCount > allocatablePods.Value() {
		return false
	}
	return true
}

func schedulerPodRequests(pod *corev1.Pod) corev1.ResourceList {
	return resourcehelper.PodRequests(pod, resourcehelper.PodResourcesOptions{
		UseStatusResources: true,
		InPlacePodLevelResourcesVerticalScalingEnabled: true,
	})
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
