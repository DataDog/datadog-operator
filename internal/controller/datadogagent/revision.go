// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"bytes"
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	v2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/pkg/controllerrevisions"
)

// skipRevisionBump returns true when the revision bump should be suppressed.
// During experiment rollback the spec is restored to an older revision; bumping
// its revision number to "latest" would make it appear newer than the experiment
// revision, causing findRollbackTarget to return the experiment revision on the
// next rollback attempt instead of the pre-experiment revision.
func skipRevisionBump(newStatus *v2alpha1.DatadogAgentStatus) bool {
	if newStatus == nil || newStatus.Experiment == nil {
		return false
	}
	phase := newStatus.Experiment.Phase
	return phase == v2alpha1.ExperimentPhaseTerminated
}

// manageRevision creates a ControllerRevision snapshot of the current spec and
// garbage collects old revisions. Must be called after manageExperiment.
//
// rawSpec is the user-submitted spec (before in-memory defaulting is applied)
// and is what gets stored in the ControllerRevision snapshot; instance is
// still used for labels, annotations, and object identity, which are
// unaffected by defaulting.
func (r *Reconciler) manageRevision(ctx context.Context, instance *v2alpha1.DatadogAgent, rawSpec v2alpha1.DatadogAgentSpec, revList []appsv1.ControllerRevision, newStatus *v2alpha1.DatadogAgentStatus) error {
	revName, err := r.ensureRevision(ctx, instance, rawSpec, revList, skipRevisionBump(newStatus))
	if err != nil {
		return err
	}
	if err := r.gcOldRevisions(ctx, map[string]bool{revName: true}, revList); err != nil {
		ctrl.LoggerFrom(ctx).Error(err, "Failed to garbage collect old ControllerRevisions, will retry on next reconcile")
	}
	return nil
}

// ownedByDDA reports whether rev is a ControllerRevision owned by dda: same
// namespace, the agent-name label matches, and a controller owner reference
// points at dda's UID. A revision left behind by a deleted-and-recreated DDA
// (same name, new UID) or a foreign/spoofed object fails this check.
func ownedByDDA(rev *appsv1.ControllerRevision, dda *v2alpha1.DatadogAgent) bool {
	if rev.Namespace != dda.GetNamespace() {
		return false
	}
	if rev.Labels[apicommon.DatadogAgentNameLabelKey] != dda.GetName() {
		return false
	}
	for _, ref := range rev.OwnerReferences {
		if ref.Controller != nil && *ref.Controller && ref.UID == dda.GetUID() {
			return true
		}
	}
	return false
}

// publishCurrentRevisionBarrier ensures a ControllerRevision exists for the
// current raw spec plus Datadog-owned annotations, then durably publishes the
// resulting pointer to status.currentRevision (plus its freshness fields) via
// a dedicated status patch. This runs before experiment handling so Fleet
// always reads a fresh, freshness-checkable baseline rather than waiting for
// the terminal status update at the end of the reconcile.
//
// The patch target is a deep copy of instance, never instance itself: the API
// server's response to the patch carries the persisted, undefaulted spec, and
// decoding that response into instance would clobber the in-memory defaulting
// already applied earlier in the reconcile.
func (r *Reconciler) publishCurrentRevisionBarrier(
	ctx context.Context,
	instance *v2alpha1.DatadogAgent,
	rawSpec v2alpha1.DatadogAgentSpec,
	revList []appsv1.ControllerRevision,
) (revName string, annotationsHash string, err error) {
	revName, err = r.ensureRevision(ctx, instance, rawSpec, revList, false)
	if err != nil {
		return "", "", err
	}

	annotationsHash, err = v2alpha1.DatadogAnnotationsHash(instance.GetAnnotations())
	if err != nil {
		return "", "", fmt.Errorf("failed to hash Datadog annotations: %w", err)
	}

	if instance.Status.CurrentRevision == revName &&
		instance.Status.CurrentRevisionObservedGeneration == instance.Generation &&
		instance.Status.CurrentRevisionObservedAnnotationsHash == annotationsHash {
		return revName, annotationsHash, nil
	}

	patchTarget := instance.DeepCopy()
	original := patchTarget.DeepCopy()
	patchTarget.Status.CurrentRevision = revName
	patchTarget.Status.CurrentRevisionObservedGeneration = instance.Generation
	patchTarget.Status.CurrentRevisionObservedAnnotationsHash = annotationsHash

	if err := r.client.Status().Patch(ctx, patchTarget, client.MergeFrom(original)); err != nil {
		return "", "", fmt.Errorf("failed to publish current revision pointer: %w", err)
	}
	instance.ResourceVersion = patchTarget.ResourceVersion

	return revName, annotationsHash, nil
}

// applyCurrentRevisionPointer mirrors the barrier's published pointer onto a
// status object. Called against instance.Status, newDDAStatus, and
// ddaStatusCopy so that no downstream error path in reconcileInstance can
// write the pointer back to empty by reusing a status copy captured before
// the barrier ran.
func applyCurrentRevisionPointer(status *v2alpha1.DatadogAgentStatus, revName string, observedGeneration int64, annotationsHash string) {
	status.CurrentRevision = revName
	status.CurrentRevisionObservedGeneration = observedGeneration
	status.CurrentRevisionObservedAnnotationsHash = annotationsHash
}

func (r *Reconciler) listRevisions(ctx context.Context, instance *v2alpha1.DatadogAgent) ([]appsv1.ControllerRevision, error) {
	revList := &appsv1.ControllerRevisionList{}
	if err := r.client.List(ctx, revList,
		client.InNamespace(instance.GetNamespace()),
		client.MatchingLabels{apicommon.DatadogAgentNameLabelKey: instance.GetName()},
	); err != nil {
		return nil, fmt.Errorf("failed to list ControllerRevisions: %w", err)
	}

	// Filter to only the revisions owned by this specific DDA instance.
	// A DDA deleted and recreated with the same name gets a new UID, so
	// revisions from the old instance are excluded here rather than being
	// mistaken for the current owner's history.
	owned := revList.Items[:0]
	for i := range revList.Items {
		if ownedByDDA(&revList.Items[i], instance) {
			owned = append(owned, revList.Items[i])
		}
	}
	revList.Items = owned
	return revList.Items, nil
}

// ensureRevision creates a ControllerRevision snapshot of the raw spec and
// annotations if it does not already exist, and returns the revision name.
//
// rawSpec (not instance.Spec, which may carry in-memory defaults) is what
// gets stored, so that revisions reflect only user-intended changes.
//
// The Revision field is a monotonic creation counter. If skipBump is true the
// existing revision is returned as-is without bumping its Revision number.
func (r *Reconciler) ensureRevision(
	ctx context.Context,
	instance *v2alpha1.DatadogAgent,
	rawSpec v2alpha1.DatadogAgentSpec,
	revList []appsv1.ControllerRevision,
	skipBump bool,
) (string, error) {
	logger := ctrl.LoggerFrom(ctx)

	specBytes, err := v2alpha1.BuildRevisionSnapshot(rawSpec, instance.GetAnnotations())
	if err != nil {
		return "", fmt.Errorf("failed to marshal snapshot: %w", err)
	}

	gvks, _, err := r.scheme.ObjectKinds(instance)
	if err != nil {
		return "", fmt.Errorf("failed to get GVK for owner: %w", err)
	}

	data := runtime.RawExtension{Raw: specBytes}
	labels := map[string]string{
		apicommon.DatadogAgentNameLabelKey: instance.GetName(),
	}
	// Merge commonLabels from spec.global so that ControllerRevision objects
	// receive the same labels as all other operator-managed resources. Without
	// this, a Kyverno-style required-labels policy rejects the revision create
	// and stops reconciliation before any DDAI or workload resources are updated.
	// Operator-owned keys already present in labels win on conflicts.
	if instance.Spec.Global != nil {
		for k, v := range instance.Spec.Global.CommonLabels {
			if _, exists := labels[k]; !exists {
				labels[k] = v
			}
		}
	}

	// Find any existing revision with identical data, and track the max Revision.
	var matchingRev *appsv1.ControllerRevision
	maxRevision := int64(0)
	for i := range revList {
		existing := &revList[i]
		if bytes.Equal(existing.Data.Raw, specBytes) {
			matchingRev = existing
		}
		if existing.Revision > maxRevision {
			maxRevision = existing.Revision
		}
	}

	if matchingRev != nil {
		objLogger := logger.WithValues(
			"object.kind", "ControllerRevision",
			"object.namespace", matchingRev.Namespace,
			"object.name", matchingRev.Name,
		)

		if revisionExperimentState(matchingRev) == experimentRevisionStateRolledBack && !skipBump {
			return r.recreateRevision(ctx, matchingRev, instance, gvks[0], labels, data, maxRevision)
		}

		// Identical content already snapshotted. Bump Revision to max+1 if it
		// has been superseded (e.g. after a revert) so ordering stays correct.
		// Skip the bump during experiment rollback: bumping the pre-experiment
		// revision above the experiment revision would cause findRollbackTarget
		// to select the experiment revision as the rollback target on the next
		// stopped signal, reversing the rollback.
		if matchingRev.Revision < maxRevision && !skipBump {
			objLogger.Info("Bumping ControllerRevision to latest")
			patch := fmt.Appendf(nil, `{"revision":%d}`, maxRevision+1)
			if err := r.client.Patch(ctx, matchingRev, client.RawPatch(types.MergePatchType, patch)); err != nil && !apierrors.IsConflict(err) {
				return "", fmt.Errorf("failed to patch ControllerRevision %s: %w", matchingRev.Name, err)
			}
		}
		return matchingRev.Name, nil
	}

	nextRevision := maxRevision + 1
	rev := controllerrevisions.NewControllerRevision(instance, gvks[0], labels, data, nextRevision, nil)

	// Check for a name conflict before creating.
	existingByName := make(map[string][]byte, len(revList))
	for i := range revList {
		existingByName[revList[i].Name] = revList[i].Data.Raw
	}
	if existingData, nameUsed := existingByName[rev.Name]; nameUsed {
		if bytes.Equal(existingData, specBytes) {
			// Another process created this revision between our list and now.
			return rev.Name, nil
		}
		return "", fmt.Errorf("hash collision for ControllerRevision name %s", rev.Name)
	}

	objLogger := logger.WithValues(
		"object.kind", "ControllerRevision",
		"object.namespace", rev.Namespace,
		"object.name", rev.Name,
	)
	objLogger.Info("Creating ControllerRevision")
	if err := r.client.Create(ctx, rev); err != nil {
		if apierrors.IsAlreadyExists(err) {
			// Another process created between our list and create.
			return rev.Name, nil
		}
		return "", fmt.Errorf("failed to create ControllerRevision %s: %w", rev.Name, err)
	}

	return rev.Name, nil
}

// recreateRevision deletes a rolled-back ControllerRevision and creates a
// fresh one with the same content but a new CreationTimestamp. This prevents
// an immediate timeout when the same experiment spec is re-applied, since
// CreationTimestamp is immutable in Kubernetes.
//
// Failure recovery:
//   - Delete fails: error returned, next reconcile retries.
//   - Delete succeeds, Create fails (or operator crashes): the revision is
//     gone, so the next reconcile's ensureRevision takes the normal "no
//     matching revision" path and creates a fresh one.
func (r *Reconciler) recreateRevision(
	ctx context.Context,
	old *appsv1.ControllerRevision,
	instance *v2alpha1.DatadogAgent,
	gvk schema.GroupVersionKind,
	labels map[string]string,
	data runtime.RawExtension,
	maxRevision int64,
) (string, error) {
	logger := ctrl.LoggerFrom(ctx).WithValues(
		"object.kind", "ControllerRevision",
		"object.namespace", old.Namespace,
		"object.name", old.Name,
	)
	logger.Info("Recreating rolled-back ControllerRevision with fresh timestamp")

	if err := r.client.Delete(ctx, old); err != nil && !apierrors.IsNotFound(err) {
		return "", fmt.Errorf("failed to delete rolled-back ControllerRevision %s: %w", old.Name, err)
	}

	fresh := controllerrevisions.NewControllerRevision(instance, gvk, labels, data, maxRevision+1, nil)
	if err := r.client.Create(ctx, fresh); err != nil {
		if apierrors.IsAlreadyExists(err) {
			return fresh.Name, nil
		}
		return "", fmt.Errorf("failed to recreate ControllerRevision %s: %w", fresh.Name, err)
	}
	return fresh.Name, nil
}

// gcOldRevisions deletes all but the pinned revisions and the single most
// recent unpinned one (kept as "previous"). Stale experiment revisions
// (marked with the rollback annotation) are kept here — they are handled by
// ensureRevision which recreates them with a fresh timestamp when the same
// spec is re-applied.
//
// pins is a set of revision names that must never be deleted. Today it only
// carries status.currentRevision; later commits add the active experiment
// checkpoint's rollback target and a pending start signal's rollback-target
// annotation, once those states exist, without needing to reshape this
// signature again.
func (r *Reconciler) gcOldRevisions(
	ctx context.Context,
	pins map[string]bool,
	revList []appsv1.ControllerRevision,
) error {
	logger := ctrl.LoggerFrom(ctx)

	// Identify the most recent unpinned revision to keep as previous.
	previous := ""
	previousRevision := int64(-1)
	for i := range revList {
		rev := &revList[i]
		if pins[rev.Name] {
			continue
		}
		if rev.Revision > previousRevision {
			previousRevision = rev.Revision
			previous = rev.Name
		}
	}

	for i := range revList {
		rev := &revList[i]
		if pins[rev.Name] || rev.Name == previous {
			continue
		}
		objLogger := logger.WithValues(
			"object.kind", "ControllerRevision",
			"object.namespace", rev.Namespace,
			"object.name", rev.Name,
		)
		objLogger.Info("Deleting old ControllerRevision")
		if err := r.client.Delete(ctx, rev); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete ControllerRevision %s: %w", rev.Name, err)
		}
	}

	return nil
}
