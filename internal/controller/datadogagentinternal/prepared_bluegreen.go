// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// Copyright 2016-present Datadog, Inc.

package datadogagentinternal

import (
	"context"
	"crypto/sha256"
	"fmt"
	"maps"
	"sort"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	corev1helpers "k8s.io/component-helpers/scheduling/corev1"
	"k8s.io/component-helpers/scheduling/corev1/nodeaffinity"
	"k8s.io/klog/v2"
	"k8s.io/kubectl/pkg/util/podutils"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	controllercommon "github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/pkg/condition"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

const (
	preparedBlueGreenMode  = "prepared-blue-green-v1"
	preparedBlueGreenArmed = "blue-green-v1"

	preparedRolloutRevisionAnnotation        = "experimental.agent.datadoghq.com/node-agent-rollout-revision"
	preparedRolloutActiveSlotAnnotation      = "experimental.agent.datadoghq.com/node-agent-rollout-active-slot"
	preparedRolloutTargetSlotAnnotation      = "experimental.agent.datadoghq.com/node-agent-rollout-target-slot"
	preparedRolloutTargetRevisionAnnotation  = "experimental.agent.datadoghq.com/node-agent-rollout-target-revision"
	preparedRolloutPairInitializedAnnotation = "experimental.agent.datadoghq.com/node-agent-rollout-pair-initialized"
	preparedRolloutCleanupAnnotation         = constants.PreparedRolloutCleanupAnnotation
	preparedRolloutSchemaAnnotation          = "experimental.agent.datadoghq.com/node-agent-rollout-schema"
	preparedRolloutDisableRevisionAnnotation = "experimental.agent.datadoghq.com/node-agent-rollout-disable"
	preparedRolloutSchemaVersion             = "3"

	preparedRolloutNodeLabelPrefix           = "experimental.agent.datadoghq.com/rollout-slot-"
	preparedRolloutCandidateAnnotationPrefix = "experimental.agent.datadoghq.com/rollout-candidate-"
	preparedRolloutRequeue                   = 5 * time.Second

	rolloutSlotBlue  = "blue"
	rolloutSlotGreen = "green"
)

type preparedPair struct {
	blue  *appsv1.DaemonSet
	green *appsv1.DaemonSet
}

func (p preparedPair) daemonSets() [2]*appsv1.DaemonSet {
	return [2]*appsv1.DaemonSet{p.blue, p.green}
}

// reconcilePreparedDisable returns an initialized pair to the conventional
// single DaemonSet. If green is serving, it first uses the ordinary prepared
// handoff to make blue authoritative. Only then does it remove green and let
// the conventional DaemonSet strategy replace blue's lock-wrapped Pods.
// Disabling the experiment may reintroduce baseline per-node rollout downtime,
// but it never starts an unwrapped blue Pod beside a serving green Pod.
func (r *Reconciler) reconcilePreparedDisable(
	ctx context.Context,
	ddai *datadoghqv1alpha1.DatadogAgentInternal,
	rendered *appsv1.DaemonSet,
	budget intstr.IntOrString,
	newStatus *datadoghqv1alpha1.DatadogAgentInternalStatus,
) (reconcile.Result, error) {
	pair, err := r.getPreparedPair(ctx, ddai, rendered)
	if err != nil {
		return reconcile.Result{}, err
	}
	if err := validatePreparedPairSchema(pair); err != nil {
		return reconcile.Result{}, err
	}

	if pair.green != nil {
		target, _, targetErr := pairTarget(pair)
		if targetErr != nil {
			return reconcile.Result{}, targetErr
		}
		active, activeErr := pairActiveSlot(pair)
		if activeErr != nil {
			return reconcile.Result{}, activeErr
		}
		if target != "" || active != rolloutSlotBlue {
			// This stable, controller-owned template bit creates a new prepared
			// revision when green already serves the user's current configuration,
			// forcing the final overlap-safe handoff back to blue.
			returnToBlue := rendered.DeepCopy()
			if returnToBlue.Spec.Template.Annotations == nil {
				returnToBlue.Spec.Template.Annotations = map[string]string{}
			}
			returnToBlue.Spec.Template.Annotations[preparedRolloutDisableRevisionAnnotation] = preparedBlueGreenArmed
			return r.reconcilePreparedDaemonSetPair(ctx, ddai, returnToBlue, budget, newStatus)
		}
		updatedBlue := pair.blue.DeepCopy()
		addedFallback, fallbackErr := ensurePreparedUnlabeledFallback(&updatedBlue.Spec.Template.Spec, preparedRolloutNodeLabelKey(ddai))
		if fallbackErr != nil {
			return reconcile.Result{}, fallbackErr
		}
		if addedFallback {
			if err := r.client.Patch(ctx, updatedBlue, client.MergeFrom(pair.blue)); err != nil {
				return reconcile.Result{}, err
			}
			return requeuePreparedRollout(), nil
		}
		if !daemonSetObserved(pair.blue) {
			return requeuePreparedRollout(), nil
		}

		foreground := metav1.DeletePropagationForeground
		if err := r.client.Delete(ctx, pair.green, &client.DeleteOptions{PropagationPolicy: &foreground}); err != nil && !apierrors.IsNotFound(err) {
			return reconcile.Result{}, err
		}
		return requeuePreparedRollout(), nil
	}

	if preparedDaemonSetInitialized(pair.blue) {
		// Blue already accepts unlabeled nodes, so rollout labels can be removed
		// before its conventional template is restored without making it
		// ineligible. Persist this cleanup before the update: metadata merging
		// intentionally preserves datadoghq.com annotations and would otherwise
		// leave the DaemonSet permanently classified as prepared.
		pending, cleanupErr := r.preparedRolloutLabelsCleanup(ctx, ddai)
		if cleanupErr != nil {
			return reconcile.Result{}, cleanupErr
		}
		if pending {
			return requeuePreparedRollout(), nil
		}
		cleared, clearErr := r.clearPreparedDaemonSetAnnotations(ctx, pair.blue)
		if clearErr != nil {
			return reconcile.Result{}, clearErr
		}
		if cleared {
			return requeuePreparedRollout(), nil
		}
		return r.createOrUpdatePreparedDaemonSet(ctx, ddai, rendered, newStatus, updateDSStatusV2WithAgent)
	}
	pending, cleanupErr := r.preparedRolloutLabelsCleanup(ctx, ddai)
	if cleanupErr != nil {
		return reconcile.Result{}, cleanupErr
	}
	if pending {
		return requeuePreparedRollout(), nil
	}
	return r.createOrUpdateDaemonset(ctx, ddai, rendered, newStatus, updateDSStatusV2WithAgent)
}

func ensurePreparedUnlabeledFallback(spec *corev1.PodSpec, key string) (bool, error) {
	if spec.Affinity == nil || spec.Affinity.NodeAffinity == nil || spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution == nil {
		return false, fmt.Errorf("prepared blue slot has no required node affinity for %q", key)
	}
	required := spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution
	for _, term := range required.NodeSelectorTerms {
		for _, expression := range term.MatchExpressions {
			if expression.Key == key && expression.Operator == corev1.NodeSelectorOpDoesNotExist {
				return false, nil
			}
		}
	}
	originalTerms := len(required.NodeSelectorTerms)
	added := false
	for i := range originalTerms {
		fallback := required.NodeSelectorTerms[i].DeepCopy()
		found := false
		for j := range fallback.MatchExpressions {
			if fallback.MatchExpressions[j].Key != key {
				continue
			}
			fallback.MatchExpressions[j].Operator = corev1.NodeSelectorOpDoesNotExist
			fallback.MatchExpressions[j].Values = nil
			found = true
		}
		if found {
			required.NodeSelectorTerms = append(required.NodeSelectorTerms, *fallback)
			added = true
		}
	}
	if !added {
		return false, fmt.Errorf("prepared blue slot node affinity does not contain rollout key %q", key)
	}
	return true, nil
}

func (r *Reconciler) clearPreparedDaemonSetAnnotations(ctx context.Context, ds *appsv1.DaemonSet) (bool, error) {
	if ds == nil {
		return false, nil
	}
	updated := ds.DeepCopy()
	changed := false
	for _, key := range []string{
		preparedRolloutRevisionAnnotation,
		preparedRolloutActiveSlotAnnotation,
		preparedRolloutTargetSlotAnnotation,
		preparedRolloutTargetRevisionAnnotation,
		preparedRolloutPairInitializedAnnotation,
	} {
		if _, found := updated.Annotations[key]; found {
			delete(updated.Annotations, key)
			changed = true
		}
	}
	if !changed {
		return false, nil
	}
	if err := r.client.Patch(ctx, updated, client.MergeFrom(ds)); err != nil {
		return false, err
	}
	return true, nil
}

// PreparedBlueGreenEnabled reports whether a DatadogAgentInternal opted into the experimental rollout.
func PreparedBlueGreenEnabled(ddai *datadoghqv1alpha1.DatadogAgentInternal) bool {
	return ddai != nil && ddai.Annotations[preparedRolloutModeAnnotation] == preparedBlueGreenMode
}

// IsPreparedRolloutNodeLabel reports whether a node label belongs to this rollout controller.
func IsPreparedRolloutNodeLabel(key string) bool {
	return strings.HasPrefix(key, preparedRolloutNodeLabelPrefix)
}

// reconcilePreparedDaemonSetPair keeps one serving DaemonSet eligible while a
// second DaemonSet prepares on a bounded set of nodes. A node label is the only
// actuator which removes the old Pod; Kubernetes never interprets Prepared as
// application readiness.
func (r *Reconciler) reconcilePreparedDaemonSetPair(
	ctx context.Context,
	ddai *datadoghqv1alpha1.DatadogAgentInternal,
	rendered *appsv1.DaemonSet,
	budget intstr.IntOrString,
	newStatus *datadoghqv1alpha1.DatadogAgentInternalStatus,
) (reconcile.Result, error) {
	if !positiveIntOrPercent(&budget) {
		return reconcile.Result{}, fmt.Errorf("prepared blue/green rollout requires a positive maxUnavailable budget")
	}

	base := rendered.DeepCopy()
	if err := prepareAgentTemplate(base); err != nil {
		return reconcile.Result{}, err
	}
	base.Spec.Template.Annotations[preparedRolloutArmedAnnotation] = preparedBlueGreenArmed
	// Include controller-side template transforms in the persisted revision.
	// Otherwise an Operator upgrade which changes only slot-specific rendering
	// can incorrectly consider the old pair current.
	base.Spec.Template.Annotations[preparedRolloutSchemaAnnotation] = preparedRolloutSchemaVersion
	// createOrUpdateDaemonset applies the same compatibility transform. Hash it
	// here as well so an Operator or Kubernetes upgrade cannot leave the pair at
	// an older finalized template while its rollout revision appears current.
	controllercommon.FinalizeAppArmorProfile(&base.Spec.Template, r.platformInfo)
	desiredRevision, revisionErr := comparison.GenerateMD5ForSpec(base.Spec.Template)
	if revisionErr != nil {
		return reconcile.Result{}, revisionErr
	}
	labelKey := preparedRolloutNodeLabelKey(ddai)

	pair, pairErr := r.getPreparedPair(ctx, ddai, rendered)
	if pairErr != nil {
		return reconcile.Result{}, pairErr
	}
	if err := validatePreparedPairSchema(pair); err != nil {
		return reconcile.Result{}, err
	}
	if pair.blue != nil && pair.green == nil && pair.blue.Spec.Template.Annotations[preparedRolloutModeAnnotation] != preparedBlueGreenMode {
		if err := validatePreparedMigrationSource(pair.blue.Spec.Template, rendered.Spec.Template, daemonSetFullyRolledOut(pair.blue)); err != nil {
			return reconcile.Result{}, err
		}
	}
	if reference := preparedTopologyReference(pair); reference != nil {
		if err := validatePreparedContainerTopology(reference.Spec.Template, base.Spec.Template); err != nil {
			return reconcile.Result{}, err
		}
	}
	missingPreparedChild := preparedPairInitialized(pair) && (pair.blue == nil || pair.green == nil)
	if missingPreparedChild {
		recovered, changed, recoveryErr := r.recoverMissingPreparedSlot(ctx, base, pair, labelKey)
		if recoveryErr != nil {
			return reconcile.Result{}, recoveryErr
		}
		if !recovered || changed {
			return requeuePreparedRollout(), nil
		}
	}
	target, _, targetErr := pairTarget(pair)
	if targetErr != nil {
		return reconcile.Result{}, targetErr
	}
	initialSlot, slotErr := pairInitialSlot(pair, target)
	if slotErr != nil {
		return reconcile.Result{}, slotErr
	}
	nodes, changed, nodeErr := r.reconcilePreparedNodeLabels(ctx, base, labelKey, initialSlot, target)
	if nodeErr != nil {
		return reconcile.Result{}, nodeErr
	}
	if changed {
		return requeuePreparedRollout(), nil
	}

	// The last known-good source slot accepts unlabeled eligible nodes. This
	// preserves new-node coverage even while the Operator is unavailable; the
	// unproven target never owns bootstrap traffic merely because it is blue.
	blueDesired, blueErr := preparedSlotDaemonSet(base, rolloutSlotBlue, labelKey, desiredRevision, initialSlot == rolloutSlotBlue)
	if blueErr != nil {
		return reconcile.Result{}, blueErr
	}
	greenDesired, greenErr := preparedSlotDaemonSet(base, rolloutSlotGreen, labelKey, desiredRevision, initialSlot == rolloutSlotGreen)
	if greenErr != nil {
		return reconcile.Result{}, greenErr
	}
	desiredPair := preparedPair{blue: blueDesired, green: greenDesired}
	if missingPreparedChild {
		missingDesired, survivor := blueDesired, pair.green
		statusUpdate := updateDSStatusV2WithAgent
		if pair.green == nil {
			missingDesired, survivor = greenDesired, pair.blue
			statusUpdate = noOpDaemonSetStatus
		}
		// Recovery made the survivor authoritative on every eligible node. The
		// missing slot can now be recreated as an inactive copy of the newest
		// desired revision without combining stale target state with a new
		// template.
		copyPreparedPairState(missingDesired, survivor)
		return r.createOrUpdatePreparedDaemonSet(ctx, ddai, missingDesired, newStatus, statusUpdate)
	}
	if pair.blue == nil {
		configureConventionalMigration(blueDesired, budget)
		return r.createOrUpdatePreparedDaemonSet(ctx, ddai, blueDesired, newStatus, updateDSStatusV2WithAgent)
	}
	if pair.green == nil {
		// Existing installations get one conventional rollout which gives every
		// old container the lock wrapper and final slot affinity before overlap is
		// possible. A fresh install follows the same path without replacing a Pod.
		if !preparedBlueDaemonSetArmed(pair.blue, desiredRevision) {
			configureConventionalMigration(blueDesired, budget)
			return r.createOrUpdatePreparedDaemonSet(ctx, ddai, blueDesired, newStatus, updateDSStatusV2WithAgent)
		}
		if pair.blue.Spec.UpdateStrategy.Type != appsv1.OnDeleteDaemonSetStrategyType {
			return r.createOrUpdatePreparedDaemonSet(ctx, ddai, blueDesired, newStatus, updateDSStatusV2WithAgent)
		}
		copyPreparedPairState(greenDesired, pair.blue)
		return r.createOrUpdatePreparedDaemonSet(ctx, ddai, greenDesired, newStatus, noOpDaemonSetStatus)
	}
	if err := validatePreparedPairSelectors(pair, blueDesired, greenDesired); err != nil {
		return reconcile.Result{}, err
	}

	// Refresh after create/update stages so the state machine only reasons from
	// API-observed objects and statuses.
	refreshedPair, refreshErr := r.getPreparedPair(ctx, ddai, rendered)
	if refreshErr != nil {
		return reconcile.Result{}, refreshErr
	}
	pair = refreshedPair
	stateChanged, stateErr := r.reconcilePreparedPairState(ctx, pair)
	if stateErr != nil {
		return reconcile.Result{}, stateErr
	}
	if stateChanged {
		return requeuePreparedRollout(), nil
	}
	target, targetRevision, targetErr := pairTarget(pair)
	if targetErr != nil {
		return reconcile.Result{}, targetErr
	}
	if target == "" {
		recordedActive, activeErr := pairActiveSlot(pair)
		if activeErr != nil {
			return reconcile.Result{}, activeErr
		}
		active, activeErr := activeSlot(nodes, labelKey, recordedActive)
		if activeErr != nil {
			return reconcile.Result{}, activeErr
		}
		activeDS, inactiveDS := pair.forSlots(active)
		if activeDS == nil || inactiveDS == nil {
			return requeuePreparedRollout(), nil
		}
		if activeDS.Annotations[preparedRolloutRevisionAnnotation] == desiredRevision && activeDS.Spec.Template.Annotations[preparedRolloutRevisionAnnotation] != desiredRevision {
			activeDesired := desiredPair.slot(active)
			return r.createOrUpdatePreparedDaemonSet(ctx, ddai, activeDesired, newStatus, noOpDaemonSetStatus)
		}
		activeHasDesiredRevision := daemonSetHasPreparedRevision(activeDS, desiredRevision)
		if activeHasDesiredRevision {
			activeDesired := desiredPair.slot(active)
			// Slot ownership changes independently of the content revision. Apply
			// the new source affinity first and wait for the DaemonSet controller to
			// observe it before removing unlabeled-node fallback from the old source.
			if !preparedSlotAffinityMatches(activeDS, activeDesired) {
				return r.createOrUpdatePreparedDaemonSet(ctx, ddai, activeDesired, newStatus, noOpDaemonSetStatus)
			}
			if !daemonSetObserved(activeDS) {
				return requeuePreparedRollout(), nil
			}
			pairPods, listErr := r.listPreparedPairPods(ctx, pair)
			if listErr != nil {
				return reconcile.Result{}, listErr
			}
			staleServing, repaired, repairErr := r.reconcileStalePreparedPods(ctx, nodes, pairPods[active], activeDS)
			if repairErr != nil {
				return reconcile.Result{}, repairErr
			}
			if repaired {
				return requeuePreparedRollout(), nil
			}
			activeHasDesiredRevision = !staleServing
		}
		if activeHasDesiredRevision {
			// Keep the inactive slot at the last known-good desired template, but
			// remove its unlabeled-node fallback only after the active source owns it.
			inactiveDesired := desiredPair.slot(otherSlot(active))
			if !daemonSetHasPreparedRevision(inactiveDS, desiredRevision) ||
				!preparedSlotAffinityMatches(inactiveDS, inactiveDesired) ||
				!daemonSetObserved(inactiveDS) {
				return r.createOrUpdatePreparedDaemonSet(ctx, ddai, inactiveDesired, newStatus, noOpDaemonSetStatus)
			}
			updatePreparedPairStatus(newStatus, pair, active)
			return reconcile.Result{}, nil
		}

		target = otherSlot(active)
		targetDesired := desiredPair.slot(target)
		if !daemonSetHasPreparedRevision(inactiveDS, desiredRevision) || !daemonSetObserved(inactiveDS) {
			return r.createOrUpdatePreparedDaemonSet(ctx, ddai, targetDesired, newStatus, noOpDaemonSetStatus)
		}
		if err := r.setPairTarget(ctx, pair, target, desiredRevision); err != nil {
			return reconcile.Result{}, err
		}
		return requeuePreparedRollout(), nil
	}

	if target != rolloutSlotBlue && target != rolloutSlotGreen {
		return reconcile.Result{}, fmt.Errorf("invalid prepared rollout target %q", target)
	}
	targetDS := pair.slot(target)
	if targetDS != nil && targetDS.Annotations[preparedRolloutRevisionAnnotation] == targetRevision && targetDS.Spec.Template.Annotations[preparedRolloutRevisionAnnotation] != targetRevision {
		updated := targetDS.DeepCopy()
		if updated.Spec.Template.Annotations == nil {
			updated.Spec.Template.Annotations = map[string]string{}
		}
		updated.Spec.Template.Annotations[preparedRolloutRevisionAnnotation] = targetRevision
		if err := r.client.Patch(ctx, updated, client.MergeFrom(targetDS)); err != nil {
			return reconcile.Result{}, err
		}
		return requeuePreparedRollout(), nil
	}
	if !daemonSetHasPreparedRevision(targetDS, targetRevision) {
		return reconcile.Result{}, fmt.Errorf("prepared rollout target %s revision %q is not available", target, targetRevision)
	}
	if !daemonSetObserved(targetDS) {
		return requeuePreparedRollout(), nil
	}

	pods, podsErr := r.listPreparedPairPods(ctx, pair)
	if podsErr != nil {
		return reconcile.Result{}, podsErr
	}
	if targetRevision != desiredRevision {
		aborted, abortErr := r.abortObsoletePreparedTarget(ctx, nodes, pods, pair, labelKey, target)
		if abortErr != nil {
			return reconcile.Result{}, abortErr
		}
		if aborted {
			return requeuePreparedRollout(), nil
		}
		// Two slots cannot safely host a third revision after the source has been
		// made ineligible on any node. Finish the already-started, persisted target
		// before starting the newer desired revision. This temporarily rolls an
		// obsolete revision farther, but avoids a permanent mixed-version wedge.
	}
	changed, advanceErr := r.advancePreparedNodes(ctx, nodes, pods, pair, labelKey, target, budget)
	if advanceErr != nil {
		return reconcile.Result{}, advanceErr
	}
	if changed {
		return requeuePreparedRollout(), nil
	}

	if preparedRolloutComplete(nodes, pods, pair, labelKey, target) {
		if err := r.completePairTarget(ctx, pair, target); err != nil {
			return reconcile.Result{}, err
		}
		return requeuePreparedRollout(), nil
	}
	updatePreparedPairStatus(newStatus, pair, target)
	return requeuePreparedRollout(), nil
}

func preparedSlotAffinityMatches(current, desired *appsv1.DaemonSet) bool {
	return current != nil && desired != nil && apiequality.Semantic.DeepEqual(
		current.Spec.Template.Spec.Affinity,
		desired.Spec.Template.Spec.Affinity,
	)
}

func (r *Reconciler) createOrUpdatePreparedDaemonSet(
	ctx context.Context,
	ddai *datadoghqv1alpha1.DatadogAgentInternal,
	daemonSet *appsv1.DaemonSet,
	newStatus *datadoghqv1alpha1.DatadogAgentInternalStatus,
	updateStatus updateDSStatusComponentFunc,
) (reconcile.Result, error) {
	result, err := r.createOrUpdateDaemonset(ctx, ddai, daemonSet, newStatus, updateStatus)
	if err != nil {
		return result, err
	}
	return requeuePreparedRollout(), nil
}

// abortObsoletePreparedTarget cancels an obsolete target while no target Pod
// is Ready. A node whose failed target already replaced its source first
// returns to the last known-good slot. The normal planner then applies the
// availability budget while it starts the corrected revision.
func (r *Reconciler) abortObsoletePreparedTarget(ctx context.Context, nodes []corev1.Node, pods map[string][]corev1.Pod, pair preparedPair, key, target string) (bool, error) {
	source := otherSlot(target)
	transition := rolloutTransitionValue(source, target)
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].Name < nodes[j].Name })
	rollbackNodes := make([]int, 0)
	for i := range nodes {
		switch nodes[i].Labels[key] {
		case source:
		case transition:
			if podConditionReady(podOnNode(pods[target], nodes[i].Name), pair.slot(target)) {
				// A Ready target may already serve node-local traffic. Finish this
				// persisted handoff before reusing the slot for another revision.
				return false, nil
			}
			rollbackNodes = append(rollbackNodes, i)
		case target:
			if podConditionReady(podOnNode(pods[target], nodes[i].Name), pair.slot(target)) {
				return false, nil
			}
			rollbackNodes = append(rollbackNodes, i)
		default:
			return false, nil
		}
	}

	mutations := 0
	for _, i := range rollbackNodes {
		if mutations >= preparedRolloutMutationLimit {
			return true, nil
		}
		if err := r.setPreparedNodeState(ctx, &nodes[i], key, source); err != nil {
			return false, err
		}
		mutations++
	}
	if err := r.setPairTarget(ctx, pair, "", ""); err != nil {
		return false, err
	}
	return true, nil
}

// recoverMissingPreparedSlot makes the surviving DaemonSet authoritative
// before recreating a deleted child. Recreating immediately is unsafe when a
// newer revision is queued: the new template would inherit persistent target
// state which names a lost older revision and the rollout would wedge.
func (r *Reconciler) recoverMissingPreparedSlot(
	ctx context.Context,
	base *appsv1.DaemonSet,
	pair preparedPair,
	key string,
) (bool, bool, error) {
	if (pair.blue == nil) == (pair.green == nil) {
		return false, false, fmt.Errorf("missing-slot recovery requires exactly one surviving DaemonSet")
	}
	survivorSlot := rolloutSlotBlue
	if pair.blue == nil {
		survivorSlot = rolloutSlotGreen
	}
	missingSlot := otherSlot(survivorSlot)
	survivor := pair.slot(survivorSlot)
	survivorRevision := survivor.Annotations[preparedRolloutRevisionAnnotation]
	if survivorRevision == "" {
		return false, false, fmt.Errorf("cannot recover missing %s slot without the surviving %s revision", missingSlot, survivorSlot)
	}

	target, targetRevision, targetErr := pairTarget(pair)
	if targetErr != nil {
		return false, false, targetErr
	}
	active, activeErr := pairActiveSlot(pair)
	if activeErr != nil {
		return false, false, activeErr
	}
	if target == "" && active == survivorSlot {
		// The pair was already steady on the survivor; only prove that every
		// eligible node is actually served before recreating the inactive child.
	} else if target != survivorSlot || targetRevision != survivorRevision {
		if err := r.setPairTarget(ctx, pair, survivorSlot, survivorRevision); err != nil {
			return false, false, err
		}
		return false, true, nil
	}

	allNodes := &corev1.NodeList{}
	if err := r.client.List(ctx, allNodes); err != nil {
		return false, false, err
	}
	sort.Slice(allNodes.Items, func(i, j int) bool { return allNodes.Items[i].Name < allNodes.Items[j].Name })
	pods, podsErr := r.listPreparedPairPods(ctx, pair)
	if podsErr != nil {
		return false, false, podsErr
	}
	recoveryTransition := rolloutTransitionValue(missingSlot, survivorSlot)
	rollbackTransition := rolloutTransitionValue(survivorSlot, missingSlot)
	allServing := true
	changed := false
	mutations := 0
	for i := range allNodes.Items {
		node := &allNodes.Items[i]
		matches, matchErr := preparedNodeEligible(&base.Spec.Template, node)
		if matchErr != nil {
			return false, false, matchErr
		}
		if !matches {
			continue
		}
		// A scheduling change can make this node desired only by the new
		// template. The surviving DaemonSet cannot prove coverage there, and
		// waiting for it would prevent recreation of the missing slot forever.
		// Evaluate the survivor with its own persisted scheduling constraints and
		// the steady label it would receive; desired-only nodes are handled by the
		// ordinary bootstrap path after both controllers exist again.
		survivorNode := node.DeepCopy()
		if survivorNode.Labels == nil {
			survivorNode.Labels = map[string]string{}
		}
		survivorNode.Labels[key] = survivorSlot
		survivorEligible, survivorMatchErr := preparedNodeEligible(&survivor.Spec.Template, survivorNode)
		if survivorMatchErr != nil {
			return false, false, survivorMatchErr
		}
		if !survivorEligible {
			continue
		}
		survivorPod := podOnNode(pods[survivorSlot], node.Name)
		survivorReady := podServingReady(survivorPod, survivor)
		if survivorPod != nil && !podMatchesPreparedRevision(survivorPod, survivor) && !survivorReady {
			// The only remaining controller already records a newer template, and
			// this stale Pod is not serving. Recreate one at a time before waiting
			// for recovery readiness; the steady-pair repair path is unreachable
			// until the missing child has been recreated.
			if err := r.client.Delete(ctx, survivorPod); err != nil && !apierrors.IsNotFound(err) {
				return false, false, err
			}
			return false, true, nil
		}
		switch node.Labels[key] {
		case survivorSlot:
			if !survivorReady {
				allServing = false
			}
		case rollbackTransition:
			allServing = false
			if survivorReady {
				if mutations >= preparedRolloutMutationLimit {
					return false, true, nil
				}
				if err := r.setPreparedNodeState(ctx, node, key, survivorSlot); err != nil {
					return false, false, err
				}
				changed = true
				mutations++
			}
		case recoveryTransition:
			allServing = false
			if survivorReady {
				if mutations >= preparedRolloutMutationLimit {
					return false, true, nil
				}
				if err := r.setPreparedNodeState(ctx, node, key, survivorSlot); err != nil {
					return false, false, err
				}
				changed = true
				mutations++
			}
		default:
			// Missing steady states and unlabeled bootstrap nodes first
			// enter overlap eligibility. A stale terminating Pod can retain its
			// lock; the survivor waits behind it and only becomes Ready afterward.
			allServing = false
			if mutations >= preparedRolloutMutationLimit {
				return false, true, nil
			}
			if err := r.setPreparedNodeState(ctx, node, key, recoveryTransition); err != nil {
				return false, false, err
			}
			changed = true
			mutations++
		}
	}
	if changed || !allServing {
		return false, changed, nil
	}
	if target != "" {
		if err := r.completePairTarget(ctx, pair, survivorSlot); err != nil {
			return false, false, err
		}
		return false, true, nil
	}
	return true, false, nil
}

func preparedBlueDaemonSetArmed(ds *appsv1.DaemonSet, revision string) bool {
	return ds != nil &&
		ds.Spec.Template.Annotations[preparedRolloutArmedAnnotation] == preparedBlueGreenArmed &&
		daemonSetHasPreparedRevision(ds, revision) &&
		daemonSetFullyRolledOut(ds)
}

func daemonSetHasPreparedRevision(ds *appsv1.DaemonSet, revision string) bool {
	return ds != nil && revision != "" &&
		ds.Annotations[preparedRolloutRevisionAnnotation] == revision &&
		ds.Spec.Template.Annotations[preparedRolloutRevisionAnnotation] == revision
}

func copyPreparedPairState(destination, source *appsv1.DaemonSet) {
	if destination == nil || source == nil {
		return
	}
	if destination.Annotations == nil {
		destination.Annotations = map[string]string{}
	}
	for _, key := range []string{
		preparedRolloutPairInitializedAnnotation,
		preparedRolloutActiveSlotAnnotation,
		preparedRolloutTargetSlotAnnotation,
		preparedRolloutTargetRevisionAnnotation,
	} {
		if value := source.Annotations[key]; value != "" {
			destination.Annotations[key] = value
		}
	}
}

func preparedSlotDaemonSet(base *appsv1.DaemonSet, slot, nodeLabelKey, revision string, allowUnlabeled bool) (*appsv1.DaemonSet, error) {
	ds := base.DeepCopy()
	for i := range ds.Spec.Template.Spec.Containers {
		container := &ds.Spec.Template.Spec.Containers[i]
		wrapPreparedContainerCommand(container)
		configurePreparedStartupProbe(container)
	}
	if ds.Annotations == nil {
		ds.Annotations = map[string]string{}
	}
	ds.Annotations[preparedRolloutRevisionAnnotation] = revision
	if ds.Spec.Template.Annotations == nil {
		ds.Spec.Template.Annotations = map[string]string{}
	}
	ds.Spec.Template.Annotations[preparedRolloutRevisionAnnotation] = revision
	ds.Spec.UpdateStrategy = appsv1.DaemonSetUpdateStrategy{Type: appsv1.OnDeleteDaemonSetStrategyType}
	addPreparedSlotAffinity(&ds.Spec.Template.Spec, nodeLabelKey, slot, allowUnlabeled)

	if slot == rolloutSlotGreen {
		// Old clients of the core Agent can outlive their peer component during a
		// granular handoff (for example, old system-probe after new core Agent
		// starts). Keep them on different command ports so they cannot connect to
		// the other generation. Blue retains the conventional port.
		for i := range ds.Spec.Template.Spec.Containers {
			container := &ds.Spec.Template.Spec.Containers[i]
			setContainerEnv(container, corev1.EnvVar{Name: coreAgentCmdPortEnv, Value: fmt.Sprint(greenCoreAgentCmdPort)})
		}
		ds.Name = suffixedKubernetesName(ds.Name, "-green")
		instanceKey := kubernetes.AppKubernetesInstanceLabelKey
		instance, ok := ds.Spec.Selector.MatchLabels[instanceKey]
		if !ok || instance == "" {
			return nil, fmt.Errorf("prepared blue/green rollout requires the standard %q DaemonSet selector", instanceKey)
		}
		greenInstance := suffixedKubernetesName(instance, "-green")
		ds.Spec.Selector = ds.Spec.Selector.DeepCopy()
		ds.Spec.Selector.MatchLabels[instanceKey] = greenInstance
		ds.Spec.Template.Labels[instanceKey] = greenInstance
		if ds.Labels[instanceKey] == instance {
			ds.Labels[instanceKey] = greenInstance
		}
	}
	return ds, nil
}

func addPreparedSlotAffinity(spec *corev1.PodSpec, key, slot string, allowUnlabeled bool) {
	values := []string{slot, rolloutTransitionValue(rolloutSlotBlue, rolloutSlotGreen), rolloutTransitionValue(rolloutSlotGreen, rolloutSlotBlue)}
	requirement := corev1.NodeSelectorRequirement{Key: key, Operator: corev1.NodeSelectorOpIn, Values: values}
	if spec.Affinity == nil {
		spec.Affinity = &corev1.Affinity{}
	}
	if spec.Affinity.NodeAffinity == nil {
		spec.Affinity.NodeAffinity = &corev1.NodeAffinity{}
	}
	required := spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution
	if required == nil {
		required = &corev1.NodeSelector{NodeSelectorTerms: []corev1.NodeSelectorTerm{{}}}
		spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution = required
	}
	baseTermCount := len(required.NodeSelectorTerms)
	for i := range baseTermCount {
		required.NodeSelectorTerms[i].MatchExpressions = append(required.NodeSelectorTerms[i].MatchExpressions, requirement)
		if allowUnlabeled {
			fallback := *required.NodeSelectorTerms[i].DeepCopy()
			for j := range fallback.MatchExpressions {
				if fallback.MatchExpressions[j].Key == key {
					fallback.MatchExpressions[j].Operator = corev1.NodeSelectorOpDoesNotExist
					fallback.MatchExpressions[j].Values = nil
				}
			}
			required.NodeSelectorTerms = append(required.NodeSelectorTerms, fallback)
		}
	}
}

func (r *Reconciler) getPreparedPair(ctx context.Context, ddai *datadoghqv1alpha1.DatadogAgentInternal, rendered *appsv1.DaemonSet) (preparedPair, error) {
	pair := preparedPair{}
	// A prepared pair owns host-local locks and node-slot labels whose identity
	// is derived from the DDAI, not from the rendered DaemonSet name. Creating a
	// second pair after an override.name change would therefore make both pairs
	// compete for the same locks. Detect any previously initialized name before
	// looking up the currently rendered names and fail closed.
	owned := &appsv1.DaemonSetList{}
	if err := r.client.List(ctx, owned, client.InNamespace(rendered.Namespace)); err != nil {
		return pair, err
	}
	expectedGreen := suffixedKubernetesName(rendered.Name, "-green")
	for i := range owned.Items {
		ds := &owned.Items[i]
		if !metav1.IsControlledBy(ds, ddai) || !preparedDaemonSetInitialized(ds) {
			continue
		}
		if ds.Name != rendered.Name && ds.Name != expectedGreen {
			return pair, fmt.Errorf("prepared rollout DaemonSet name cannot change after initialization: found existing child %s/%s while desired blue name is %q", ds.Namespace, ds.Name, rendered.Name)
		}
	}
	for slot, name := range map[string]string{
		rolloutSlotBlue:  rendered.Name,
		rolloutSlotGreen: suffixedKubernetesName(rendered.Name, "-green"),
	} {
		ds := &appsv1.DaemonSet{}
		err := r.client.Get(ctx, types.NamespacedName{Namespace: rendered.Namespace, Name: name}, ds)
		if apierrors.IsNotFound(err) {
			continue
		}
		if err != nil {
			return pair, err
		}
		if !metav1.IsControlledBy(ds, ddai) {
			return pair, fmt.Errorf("prepared rollout DaemonSet name collision: %s/%s is not controlled by DatadogAgentInternal %s", ds.Namespace, ds.Name, ddai.Name)
		}
		if slot == rolloutSlotGreen && !preparedDaemonSetInitialized(ds) {
			return pair, fmt.Errorf("prepared rollout DaemonSet name collision: %s/%s is not a prepared green slot", ds.Namespace, ds.Name)
		}
		if slot == rolloutSlotBlue {
			pair.blue = ds
		} else {
			pair.green = ds
		}
	}
	return pair, nil
}

func (r *Reconciler) deletePreparedDaemonSetPairIfPresent(
	ctx context.Context,
	ddai *datadoghqv1alpha1.DatadogAgentInternal,
	rendered *appsv1.DaemonSet,
	newStatus *datadoghqv1alpha1.DatadogAgentInternalStatus,
) (bool, error) {
	pair, err := r.getPreparedPair(ctx, ddai, rendered)
	if err != nil {
		return false, err
	}
	prepared := preparedDaemonSetInitialized(pair.blue) || preparedDaemonSetInitialized(pair.green)
	cleanupPending := ddai.Annotations[preparedRolloutCleanupAnnotation] == preparedBlueGreenArmed
	if !prepared {
		if !cleanupPending {
			return false, nil
		}
	} else if !cleanupPending {
		if err := r.setPreparedCleanupPending(ctx, ddai, true); err != nil {
			return true, err
		}
	}
	for _, ds := range []*appsv1.DaemonSet{pair.green, pair.blue} {
		if ds == nil {
			continue
		}
		foreground := metav1.DeletePropagationForeground
		if err := r.deleteV2DaemonSetWithOptions(ctx, ddai, ds, newStatus, &client.DeleteOptions{PropagationPolicy: &foreground}); err != nil {
			return true, err
		}
	}
	// Kubernetes deletion is asynchronous. Keep cleanup intent and node state
	// until a later API observation confirms that both controllers are gone.
	// This prevents a concurrent re-enable from constructing a fresh pair while
	// an old slot is still terminating.
	if pair.blue != nil || pair.green != nil {
		return true, nil
	}
	pending, cleanupErr := r.preparedRolloutLabelsCleanup(ctx, ddai)
	if cleanupErr != nil {
		return true, cleanupErr
	}
	if pending {
		return true, nil
	}
	if err := r.setPreparedCleanupPending(ctx, ddai, false); err != nil {
		return true, err
	}
	return true, nil
}

func (r *Reconciler) preparedBlueGreenInitialized(ctx context.Context, ddai *datadoghqv1alpha1.DatadogAgentInternal, rendered *appsv1.DaemonSet) (bool, error) {
	if ddai.Annotations[preparedRolloutCleanupAnnotation] == preparedBlueGreenArmed {
		return true, nil
	}
	pair, err := r.getPreparedPair(ctx, ddai, rendered)
	if err != nil {
		return false, err
	}
	return preparedDaemonSetInitialized(pair.blue) || preparedDaemonSetInitialized(pair.green), nil
}

func preparedDaemonSetInitialized(ds *appsv1.DaemonSet) bool {
	return ds != nil && (ds.Spec.Template.Annotations[preparedRolloutModeAnnotation] == preparedBlueGreenMode ||
		ds.Annotations[preparedRolloutPairInitializedAnnotation] == preparedBlueGreenArmed ||
		ds.Annotations[preparedRolloutRevisionAnnotation] != "")
}

func validatePreparedPairSchema(pair preparedPair) error {
	for _, ds := range pair.daemonSets() {
		if !preparedDaemonSetInitialized(ds) {
			continue
		}
		schema := ds.Spec.Template.Annotations[preparedRolloutSchemaAnnotation]
		if schema != "" && schema != preparedRolloutSchemaVersion {
			return fmt.Errorf("prepared Agent rollout %q uses schema %q; disable it with its current Operator before upgrading to schema %q", ds.Name, schema, preparedRolloutSchemaVersion)
		}
	}
	return nil
}

func preparedTopologyReference(pair preparedPair) *appsv1.DaemonSet {
	if preparedDaemonSetInitialized(pair.blue) {
		return pair.blue
	}
	if preparedDaemonSetInitialized(pair.green) {
		return pair.green
	}
	return nil
}

func validatePreparedContainerTopology(current, desired corev1.PodTemplateSpec) error {
	currentNames := make(map[string]struct{}, len(current.Spec.Containers))
	desiredNames := make(map[string]struct{}, len(desired.Spec.Containers))
	for i := range current.Spec.Containers {
		currentNames[current.Spec.Containers[i].Name] = struct{}{}
	}
	for i := range desired.Spec.Containers {
		desiredNames[desired.Spec.Containers[i].Name] = struct{}{}
	}
	if maps.Equal(currentNames, desiredNames) {
		return nil
	}
	return fmt.Errorf("prepared blue/green rollout cannot add or remove Agent containers after arming because component locks would not protect cross-container host resources")
}

func (r *Reconciler) setPreparedCleanupPending(ctx context.Context, ddai *datadoghqv1alpha1.DatadogAgentInternal, pending bool) error {
	updated := ddai.DeepCopy()
	if updated.Annotations == nil {
		updated.Annotations = map[string]string{}
	}
	if pending {
		updated.Annotations[preparedRolloutCleanupAnnotation] = preparedBlueGreenArmed
	} else {
		delete(updated.Annotations, preparedRolloutCleanupAnnotation)
	}
	if err := r.client.Patch(ctx, updated, client.MergeFrom(ddai)); err != nil {
		return err
	}
	ddai.Annotations = updated.Annotations
	return nil
}

func validatePreparedPairSelectors(pair preparedPair, blueDesired, greenDesired *appsv1.DaemonSet) error {
	for _, entry := range []struct {
		current *appsv1.DaemonSet
		desired *appsv1.DaemonSet
	}{
		{current: pair.blue, desired: blueDesired},
		{current: pair.green, desired: greenDesired},
	} {
		if entry.current == nil || entry.desired == nil || maps.Equal(entry.current.Spec.Selector.MatchLabels, entry.desired.Spec.Selector.MatchLabels) {
			continue
		}
		return fmt.Errorf("prepared rollout cannot change immutable selector on initialized DaemonSet %s; keeping the serving pair unchanged", entry.current.Name)
	}
	return nil
}

func (p preparedPair) slot(slot string) *appsv1.DaemonSet {
	if slot == rolloutSlotGreen {
		return p.green
	}
	return p.blue
}

func (p preparedPair) forSlots(active string) (*appsv1.DaemonSet, *appsv1.DaemonSet) {
	return p.slot(active), p.slot(otherSlot(active))
}

func pairTarget(pair preparedPair) (string, string, error) {
	var target, revision string
	for _, ds := range pair.daemonSets() {
		if ds == nil {
			continue
		}
		candidateTarget := ds.Annotations[preparedRolloutTargetSlotAnnotation]
		candidateRevision := ds.Annotations[preparedRolloutTargetRevisionAnnotation]
		if (candidateTarget == "") != (candidateRevision == "") {
			return "", "", fmt.Errorf("prepared rollout target state on DaemonSet %s is incomplete", ds.Name)
		}
		if candidateTarget == "" {
			continue
		}
		if target == "" {
			target, revision = candidateTarget, candidateRevision
			continue
		}
		if candidateTarget != target || candidateRevision != revision {
			return "", "", fmt.Errorf("prepared rollout target state diverged between DaemonSets")
		}
	}
	return target, revision, nil
}

func pairInitialSlot(pair preparedPair, target string) (string, error) {
	if target != "" {
		return otherSlot(target), nil
	}
	return pairActiveSlot(pair)
}

func pairActiveSlot(pair preparedPair) (string, error) {
	active := ""
	for _, ds := range pair.daemonSets() {
		if ds == nil {
			continue
		}
		candidate := ds.Annotations[preparedRolloutActiveSlotAnnotation]
		if candidate != "" && candidate != rolloutSlotBlue && candidate != rolloutSlotGreen {
			return "", fmt.Errorf("invalid prepared rollout active slot %q on DaemonSet %s", candidate, ds.Name)
		}
		if candidate == "" {
			continue
		}
		if active == "" {
			active = candidate
			continue
		}
		if candidate != active {
			return "", fmt.Errorf("prepared rollout active state diverged between DaemonSets")
		}
	}
	if active == "" {
		active = rolloutSlotBlue
	}
	return active, nil
}

func (r *Reconciler) reconcilePreparedPairState(ctx context.Context, pair preparedPair) (bool, error) {
	target, revision, err := pairTarget(pair)
	if err != nil {
		return false, err
	}
	active := ""
	if target != "" {
		// Until both DaemonSets have committed completion, the source remains
		// authoritative. This also recovers a failed second patch where one
		// child already says "complete" and the other still records the target.
		active = otherSlot(target)
	} else {
		active, err = pairActiveSlot(pair)
		if err != nil {
			return false, err
		}
	}
	for _, ds := range pair.daemonSets() {
		if ds == nil {
			continue
		}
		updated := ds.DeepCopy()
		if updated.Annotations == nil {
			updated.Annotations = map[string]string{}
		}
		changed := false
		if updated.Annotations[preparedRolloutPairInitializedAnnotation] != preparedBlueGreenArmed {
			updated.Annotations[preparedRolloutPairInitializedAnnotation] = preparedBlueGreenArmed
			changed = true
		}
		if updated.Annotations[preparedRolloutActiveSlotAnnotation] == "" ||
			(target != "" && updated.Annotations[preparedRolloutActiveSlotAnnotation] != active) {
			updated.Annotations[preparedRolloutActiveSlotAnnotation] = active
			changed = true
		}
		if target != "" && (updated.Annotations[preparedRolloutTargetSlotAnnotation] != target ||
			updated.Annotations[preparedRolloutTargetRevisionAnnotation] != revision) {
			updated.Annotations[preparedRolloutTargetSlotAnnotation] = target
			updated.Annotations[preparedRolloutTargetRevisionAnnotation] = revision
			changed = true
		}
		if !changed {
			continue
		}
		if err := r.client.Patch(ctx, updated, client.MergeFrom(ds)); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}

func activeSlot(nodes []corev1.Node, key, recorded string) (string, error) {
	active := recorded
	for i := range nodes {
		value := nodes[i].Labels[key]
		if value == "" {
			continue
		}
		if value != rolloutSlotBlue && value != rolloutSlotGreen {
			return "", fmt.Errorf("node %s has in-flight rollout state %q without a recorded target", nodes[i].Name, value)
		}
		if active != "" && active != value {
			return "", fmt.Errorf("prepared rollout has mixed steady slots without a recorded target")
		}
		active = value
	}
	return active, nil
}

func (r *Reconciler) reconcilePreparedNodeLabels(ctx context.Context, base *appsv1.DaemonSet, key, initialSlot, target string) ([]corev1.Node, bool, error) {
	all := &corev1.NodeList{}
	if err := r.client.List(ctx, all); err != nil {
		return nil, false, err
	}
	sort.Slice(all.Items, func(i, j int) bool { return all.Items[i].Name < all.Items[j].Name })
	eligible := make([]corev1.Node, 0, len(all.Items))
	changed := false
	mutations := 0
	for i := range all.Items {
		node := &all.Items[i]
		matches, err := preparedNodeEligible(&base.Spec.Template, node)
		if err != nil {
			return nil, false, err
		}
		if !matches {
			_, hasLabel := node.Labels[key]
			candidateKey := preparedRolloutCandidateAnnotationKey(key)
			_, hasCandidate := node.Annotations[candidateKey]
			if hasLabel || hasCandidate {
				if mutations >= preparedRolloutMutationLimit {
					return eligible, true, nil
				}
				updated := node.DeepCopy()
				delete(updated.Labels, key)
				delete(updated.Annotations, candidateKey)
				if err := r.client.Patch(ctx, updated, client.MergeFrom(node)); err != nil {
					return nil, false, err
				}
				changed = true
				mutations++
			}
			continue
		}
		if node.Labels == nil || !preparedNodeStateValid(node.Labels[key], initialSlot, target) {
			if mutations >= preparedRolloutMutationLimit {
				return eligible, true, nil
			}
			updated := node.DeepCopy()
			if updated.Labels == nil {
				updated.Labels = map[string]string{}
			}
			updated.Labels[key] = initialSlot
			if target != "" && node.Labels[key] == legacyRolloutPendingValue(target) {
				// Schema v2 used a separate target-pending state after source
				// removal. Target Pod availability now carries that information,
				// so normalize the legacy target-only state without re-admitting
				// the source generation.
				updated.Labels[key] = target
				delete(updated.Annotations, preparedRolloutCandidateAnnotationKey(key))
			} else if target == rolloutSlotGreen && node.Labels[key] == "" {
				// A node which appears while green is active must not depend on the
				// inactive blue template being healthy. Admit both slots immediately;
				// blue may provide bootstrap coverage while green prepares.
				updated.Labels[key] = rolloutTransitionValue(rolloutSlotBlue, rolloutSlotGreen)
			}
			if err := r.client.Patch(ctx, updated, client.MergeFrom(node)); err != nil {
				return nil, false, err
			}
			node = updated
			changed = true
			mutations++
		}
		eligible = append(eligible, *node.DeepCopy())
	}
	return eligible, changed, nil
}

func preparedNodeEligible(template *corev1.PodTemplateSpec, node *corev1.Node) (bool, error) {
	if template.Spec.NodeName != "" && template.Spec.NodeName != node.Name {
		return false, nil
	}
	matcher := nodeaffinity.GetRequiredNodeAffinity(&corev1.Pod{Spec: *template.Spec.DeepCopy()})
	matches, err := matcher.Match(node)
	if err != nil || !matches {
		return matches, err
	}

	// DaemonSet Pods receive these tolerations from the native controller even
	// though they are absent from the template. Include them before applying the
	// scheduler's hard NoSchedule/NoExecute taint check.
	tolerations := append([]corev1.Toleration(nil), template.Spec.Tolerations...)
	for _, entry := range []struct {
		key    string
		effect corev1.TaintEffect
	}{
		{corev1.TaintNodeNotReady, corev1.TaintEffectNoExecute},
		{corev1.TaintNodeUnreachable, corev1.TaintEffectNoExecute},
		{corev1.TaintNodeDiskPressure, corev1.TaintEffectNoSchedule},
		{corev1.TaintNodeMemoryPressure, corev1.TaintEffectNoSchedule},
		{corev1.TaintNodePIDPressure, corev1.TaintEffectNoSchedule},
		{corev1.TaintNodeUnschedulable, corev1.TaintEffectNoSchedule},
	} {
		tolerations = append(tolerations, corev1.Toleration{Key: entry.key, Operator: corev1.TolerationOpExists, Effect: entry.effect})
	}
	if template.Spec.HostNetwork {
		tolerations = append(tolerations, corev1.Toleration{Key: corev1.TaintNodeNetworkUnavailable, Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule})
	}
	_, untolerated := corev1helpers.FindMatchingUntoleratedTaint(
		klog.Background(),
		node.Spec.Taints,
		tolerations,
		func(taint *corev1.Taint) bool {
			return taint.Effect == corev1.TaintEffectNoSchedule || taint.Effect == corev1.TaintEffectNoExecute
		},
		true,
	)
	return !untolerated, nil
}

func preparedNodeStateValid(value, source, target string) bool {
	if value == source {
		return true
	}
	if target == "" {
		return false
	}
	return value == target || value == rolloutTransitionValue(source, target)
}

func (r *Reconciler) listPreparedPairPods(ctx context.Context, pair preparedPair) (map[string][]corev1.Pod, error) {
	result := map[string][]corev1.Pod{rolloutSlotBlue: {}, rolloutSlotGreen: {}}
	for _, slot := range []string{rolloutSlotBlue, rolloutSlotGreen} {
		ds := pair.slot(slot)
		if ds == nil {
			continue
		}
		selector, err := metav1.LabelSelectorAsMap(ds.Spec.Selector)
		if err != nil {
			return nil, err
		}
		list := &corev1.PodList{}
		if err := r.client.List(ctx, list, client.InNamespace(ds.Namespace), client.MatchingLabels(selector)); err != nil {
			return nil, err
		}
		for i := range list.Items {
			pod := list.Items[i]
			owner := metav1.GetControllerOf(&pod)
			if owner != nil && owner.Kind == "DaemonSet" && owner.Name == ds.Name && owner.UID == ds.UID {
				result[slot] = append(result[slot], pod)
			}
		}
	}
	return result, nil
}

func (r *Reconciler) advancePreparedNodes(ctx context.Context, nodes []corev1.Node, pods map[string][]corev1.Pod, pair preparedPair, key, target string, budget intstr.IntOrString) (bool, error) {
	if len(nodes) == 0 {
		return false, nil
	}
	limit, err := intstr.GetScaledValueFromIntOrPercent(&budget, len(nodes), true)
	if err != nil || limit <= 0 {
		return false, fmt.Errorf("invalid prepared rollout budget %q", budget.String())
	}
	source := otherSlot(target)
	transition := rolloutTransitionValue(source, target)
	candidateKey := preparedRolloutCandidateAnnotationKey(key)
	states := make([]preparedNodePlanState, 0, len(nodes))
	nodeByName := make(map[string]*corev1.Node, len(nodes))
	for i := range nodes {
		node := &nodes[i]
		nodeByName[node.Name] = node
		sourcePod := podOnNode(pods[source], node.Name)
		targetPod := podOnNode(pods[target], node.Name)
		state := preparedNodePlanState{
			name:              node.Name,
			sourceServing:     podServingReady(sourcePod, pair.slot(source)),
			sourcePrepared:    podPrepared(sourcePod, pair.slot(source)),
			targetServing:     podServingReady(targetPod, pair.slot(target)),
			targetReady:       podReady(targetPod, pair.slot(target)),
			targetPrepared:    podPrepared(targetPod, pair.slot(target)),
			targetRestarted:   podHasRestartedContainer(targetPod),
			targetMatches:     podMatchesPreparedRevision(targetPod, pair.slot(target)),
			targetCapacity:    podUnschedulableForCapacity(targetPod),
			targetExists:      targetPod != nil,
			recordedTargetUID: node.Annotations[candidateKey],
		}
		if targetPod != nil {
			state.targetUID = string(targetPod.UID)
		}
		switch node.Labels[key] {
		case transition:
			state.phase = preparedNodeTransition
		case source:
			state.phase = preparedNodeSource
		case target:
			state.phase = preparedNodeTarget
		default:
			return false, fmt.Errorf("invalid prepared rollout state %q on node %s", node.Labels[key], node.Name)
		}
		states = append(states, state)
	}

	plan := planPreparedRollout(states, limit, preparedRolloutMutationLimit)
	for _, action := range plan.actions {
		node := nodeByName[action.node]
		if node == nil {
			return false, fmt.Errorf("prepared rollout plan references unknown node %s", action.node)
		}
		switch action.kind {
		case preparedActionRecordCandidate:
			targetPod := podOnNode(pods[target], node.Name)
			if targetPod == nil || targetPod.UID == "" {
				return false, fmt.Errorf("prepared rollout candidate disappeared from node %s", node.Name)
			}
			if err := r.setPreparedNodeCandidate(ctx, node, candidateKey, string(targetPod.UID)); err != nil {
				return false, err
			}
		case preparedActionSelectTarget:
			if err := r.setPreparedNodeState(ctx, node, key, target); err != nil {
				return false, err
			}
		case preparedActionRetryTarget:
			targetPod := podOnNode(pods[target], node.Name)
			if targetPod == nil {
				return false, fmt.Errorf("prepared rollout retry target disappeared from node %s", node.Name)
			}
			if err := r.client.Delete(ctx, targetPod); err != nil && !apierrors.IsNotFound(err) {
				return false, err
			}
		case preparedActionStartOverlap:
			if err := r.setPreparedNodeState(ctx, node, key, transition); err != nil {
				return false, err
			}
		default:
			return false, fmt.Errorf("unknown prepared rollout action %d", action.kind)
		}
	}
	return len(plan.actions) > 0, nil
}

func podUnschedulableForCapacity(pod *corev1.Pod) bool {
	if pod == nil || pod.Status.Phase != corev1.PodPending {
		return false
	}
	for i := range pod.Status.Conditions {
		condition := &pod.Status.Conditions[i]
		if condition.Type != corev1.PodScheduled || condition.Status != corev1.ConditionFalse || condition.Reason != corev1.PodReasonUnschedulable {
			continue
		}
		return strings.Contains(condition.Message, "Insufficient ") || strings.Contains(condition.Message, "Too many pods")
	}
	return false
}

func podOnNode(pods []corev1.Pod, node string) *corev1.Pod {
	for i := range pods {
		if podTargetsNode(&pods[i], node) && pods[i].DeletionTimestamp == nil {
			return &pods[i]
		}
	}
	return nil
}

func podTargetsNode(pod *corev1.Pod, node string) bool {
	if pod == nil {
		return false
	}
	if pod.Spec.NodeName != "" {
		return pod.Spec.NodeName == node
	}
	if pod.Spec.Affinity == nil || pod.Spec.Affinity.NodeAffinity == nil || pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution == nil {
		return false
	}
	for _, term := range pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms {
		for _, field := range term.MatchFields {
			if field.Key == metav1.ObjectNameField && field.Operator == corev1.NodeSelectorOpIn && len(field.Values) == 1 && field.Values[0] == node {
				return true
			}
		}
	}
	return false
}

func podPrepared(pod *corev1.Pod, ds *appsv1.DaemonSet) bool {
	if !podMatchesPreparedRevision(pod, ds) || pod.Status.Phase != corev1.PodRunning || len(pod.Status.ContainerStatuses) != len(ds.Spec.Template.Spec.Containers) {
		return false
	}
	for i := range pod.Status.ContainerStatuses {
		status := &pod.Status.ContainerStatuses[i]
		// The lock wrapper keeps the startup probe unsuccessful while it waits for
		// the component lock. Kubelet reports Started=false for a prepared
		// replacement. A stable, running, never-restarted wrapper is the preparation
		// signal. The candidate UID is re-observed before source removal.
		if status.State.Running == nil || status.RestartCount != 0 {
			return false
		}
	}
	return true
}

func podHasRestartedContainer(pod *corev1.Pod) bool {
	if pod == nil {
		return false
	}
	for i := range pod.Status.ContainerStatuses {
		if pod.Status.ContainerStatuses[i].RestartCount > 0 {
			return true
		}
	}
	return false
}

func podReady(pod *corev1.Pod, ds *appsv1.DaemonSet) bool {
	if !podMatchesPreparedRevision(pod, ds) {
		return false
	}
	return podServingReady(pod, ds)
}

func podConditionReady(pod *corev1.Pod, ds *appsv1.DaemonSet) bool {
	return podMatchesPreparedRevision(pod, ds) && podutils.IsPodReady(pod)
}

func podServingReady(pod *corev1.Pod, ds *appsv1.DaemonSet) bool {
	if pod == nil || ds == nil {
		return false
	}
	return podutils.IsPodAvailable(pod, ds.Spec.MinReadySeconds, metav1.Now())
}

func podMatchesPreparedRevision(pod *corev1.Pod, ds *appsv1.DaemonSet) bool {
	if pod == nil || ds == nil {
		return false
	}
	revision := ds.Annotations[preparedRolloutRevisionAnnotation]
	return revision != "" && pod.Annotations[preparedRolloutRevisionAnnotation] == revision
}

func (r *Reconciler) reconcileStalePreparedPods(ctx context.Context, nodes []corev1.Node, pods []corev1.Pod, ds *appsv1.DaemonSet) (bool, bool, error) {
	staleServing := false
	for i := range nodes {
		pod := podOnNode(pods, nodes[i].Name)
		if pod == nil || podMatchesPreparedRevision(pod, ds) {
			continue
		}
		if podServingReady(pod, ds) {
			staleServing = true
			continue
		}
		// The node is already unavailable according to the serving readiness
		// contract. Recreate one stale OnDelete Pod at a time so its active
		// DaemonSet can converge to the recorded template revision.
		if err := r.client.Delete(ctx, pod); err != nil && !apierrors.IsNotFound(err) {
			return false, false, err
		}
		return staleServing, true, nil
	}
	return staleServing, false, nil
}

func (r *Reconciler) setPreparedNodeState(ctx context.Context, node *corev1.Node, key, value string) error {
	updated := node.DeepCopy()
	if updated.Labels == nil {
		updated.Labels = map[string]string{}
	}
	updated.Labels[key] = value
	delete(updated.Annotations, preparedRolloutCandidateAnnotationKey(key))
	return r.client.Patch(ctx, updated, client.MergeFrom(node))
}

func (r *Reconciler) setPreparedNodeCandidate(ctx context.Context, node *corev1.Node, key, uid string) error {
	updated := node.DeepCopy()
	if updated.Annotations == nil {
		updated.Annotations = map[string]string{}
	}
	updated.Annotations[key] = uid
	return r.client.Patch(ctx, updated, client.MergeFrom(node))
}

func (r *Reconciler) setPairTarget(ctx context.Context, pair preparedPair, target, revision string) error {
	return r.patchPreparedPair(ctx, pair, func(annotations map[string]string) {
		if target == "" {
			delete(annotations, preparedRolloutTargetSlotAnnotation)
			delete(annotations, preparedRolloutTargetRevisionAnnotation)
			return
		}
		annotations[preparedRolloutTargetSlotAnnotation] = target
		annotations[preparedRolloutTargetRevisionAnnotation] = revision
	})
}

func (r *Reconciler) completePairTarget(ctx context.Context, pair preparedPair, active string) error {
	return r.patchPreparedPair(ctx, pair, func(annotations map[string]string) {
		annotations[preparedRolloutActiveSlotAnnotation] = active
		delete(annotations, preparedRolloutTargetSlotAnnotation)
		delete(annotations, preparedRolloutTargetRevisionAnnotation)
	})
}

func (r *Reconciler) patchPreparedPair(ctx context.Context, pair preparedPair, mutate func(map[string]string)) error {
	found := false
	for _, ds := range pair.daemonSets() {
		if ds == nil {
			continue
		}
		found = true
		updated := ds.DeepCopy()
		if updated.Annotations == nil {
			updated.Annotations = map[string]string{}
		}
		mutate(updated.Annotations)
		if err := r.client.Patch(ctx, updated, client.MergeFrom(ds)); err != nil {
			return err
		}
	}
	if !found {
		return fmt.Errorf("cannot persist prepared rollout state without a DaemonSet")
	}
	return nil
}

func preparedRolloutComplete(nodes []corev1.Node, pods map[string][]corev1.Pod, pair preparedPair, key, target string) bool {
	source := otherSlot(target)
	for i := range nodes {
		if nodes[i].Labels[key] != target ||
			!podReady(podOnNode(pods[target], nodes[i].Name), pair.slot(target)) ||
			podOnNode(pods[source], nodes[i].Name) != nil {
			return false
		}
	}
	return true
}

func daemonSetObserved(ds *appsv1.DaemonSet) bool {
	return ds != nil && ds.Status.ObservedGeneration == ds.Generation
}

func updatePreparedPairStatus(newStatus *datadoghqv1alpha1.DatadogAgentInternalStatus, pair preparedPair, serving string) {
	ds := pair.slot(serving)
	if ds == nil {
		return
	}
	now := metav1.Now()
	newStatus.Agent = condition.UpdateDaemonSetStatusDDAI(pair.blue.Name, ds, newStatus.Agent, &now)
}

func noOpDaemonSetStatus(string, *appsv1.DaemonSet, *datadoghqv1alpha1.DatadogAgentInternalStatus, metav1.Time, metav1.ConditionStatus, string, string) {
}

func preparedRolloutNodeLabelKey(ddai *datadoghqv1alpha1.DatadogAgentInternal) string {
	sum := sha256.Sum256([]byte(ddai.Namespace + "/" + ddai.Name))
	return fmt.Sprintf("%s%x", preparedRolloutNodeLabelPrefix, sum[:6])
}

func preparedRolloutCandidateAnnotationKey(labelKey string) string {
	return preparedRolloutCandidateAnnotationPrefix + strings.TrimPrefix(labelKey, preparedRolloutNodeLabelPrefix)
}

func preparedPairInitialized(pair preparedPair) bool {
	// Green is created only after the blue arming rollout completes. Its
	// existence is therefore sufficient evidence even if the controller crashed
	// before persisting the pair-initialized annotation on either child.
	if pair.green != nil {
		return true
	}
	for _, ds := range pair.daemonSets() {
		if ds != nil && ds.Annotations[preparedRolloutPairInitializedAnnotation] == preparedBlueGreenArmed {
			return true
		}
	}
	return false
}

func rolloutTransitionValue(source, target string) string { return source + "-to-" + target }
func legacyRolloutPendingValue(target string) string      { return target + "-pending" }

func otherSlot(slot string) string {
	if slot == rolloutSlotGreen {
		return rolloutSlotBlue
	}
	return rolloutSlotGreen
}

func suffixedKubernetesName(name, suffix string) string {
	if len(name)+len(suffix) <= 63 {
		return name + suffix
	}
	return strings.TrimRight(name[:63-len(suffix)], "-") + suffix
}

func requeuePreparedRollout() reconcile.Result {
	return ctrl.Result{RequeueAfter: preparedRolloutRequeue}
}
