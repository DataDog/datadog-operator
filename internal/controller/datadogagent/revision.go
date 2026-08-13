// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	v2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/pkg/controllerrevisions"
)

// revisionSnapshot is the payload stored in a ControllerRevision.
// Annotations are included for preview features.
type revisionSnapshot struct {
	Spec        v2alpha1.DatadogAgentSpec `json:"spec"`
	Annotations map[string]string         `json:"annotations,omitempty"`
}

// buildRevisionSnapshot marshals a revisionSnapshot from spec and annotations.
// Spec must be the raw, user-submitted spec (not the in-memory defaulted
// copy) so that snapshot comparisons are unaffected by defaulting.
func buildRevisionSnapshot(spec v2alpha1.DatadogAgentSpec, allAnnotations map[string]string) ([]byte, error) {
	snap := revisionSnapshot{Spec: spec, Annotations: datadogAnnotations(allAnnotations)}
	return json.Marshal(snap)
}

// computeSpecHash returns sha256(revisionSnapshot bytes) hex-encoded. Same
// payload as ControllerRevision storage — spec plus filtered Datadog
// annotations — so "hash matches" and "snapshot content equals" agree by
// construction. Used by experiment logic to detect manual spec changes
// without walking ControllerRevisions.
func computeSpecHash(spec v2alpha1.DatadogAgentSpec, allAnnotations map[string]string) (string, error) {
	snap, err := buildRevisionSnapshot(spec, allAnnotations)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(snap)
	return hex.EncodeToString(sum[:]), nil
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
		for _, ref := range revList.Items[i].OwnerReferences {
			if ref.Controller != nil && *ref.Controller && ref.UID == instance.GetUID() {
				owned = append(owned, revList.Items[i])
				break
			}
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
) (string, error) {
	logger := ctrl.LoggerFrom(ctx)

	specBytes, err := buildRevisionSnapshot(rawSpec, instance.GetAnnotations())
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

		// Identical content already snapshotted. Bump Revision to max+1 if it
		// has been superseded so ordering stays correct.
		if matchingRev.Revision < maxRevision {
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
// datadogAnnotations returns a copy of annotations filtered to only those
// with `.datadoghq.com/` in the key, which are used for preview features.
// Experiment signal annotations (experiment.datadoghq.com/) are excluded
// because they are transient signals, not part of the spec snapshot.
func datadogAnnotations(all map[string]string) map[string]string {
	filtered := make(map[string]string)
	for k, v := range all {
		if strings.Contains(k, ".datadoghq.com/") &&
			!strings.HasPrefix(k, "experiment.datadoghq.com/") &&
			!strings.HasPrefix(k, "fleet.datadoghq.com/") {
			filtered[k] = v
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	return filtered
}

// gcOldRevisions deletes all but the two most recent ControllerRevisions
// (current and previous) plus any names in pinned. Pinned revisions survive GC
// unconditionally; callers include public-status pointers here so retention
// counts cannot prune a revision named by Status.CurrentRevision or an active
// experiment's rollback target.
func (r *Reconciler) gcOldRevisions(
	ctx context.Context,
	current string,
	revList []appsv1.ControllerRevision,
	pinned ...string,
) error {
	logger := ctrl.LoggerFrom(ctx)
	keep := make(map[string]struct{}, len(pinned)+2)
	keep[current] = struct{}{}
	for _, name := range pinned {
		if name != "" {
			keep[name] = struct{}{}
		}
	}

	// Identify the most recent non-kept revision to keep as previous.
	previous := ""
	previousRevision := int64(-1)
	for i := range revList {
		rev := &revList[i]
		if _, isKept := keep[rev.Name]; isKept {
			continue
		}
		if rev.Revision > previousRevision {
			previousRevision = rev.Revision
			previous = rev.Name
		}
	}
	if previous != "" {
		keep[previous] = struct{}{}
	}

	for i := range revList {
		rev := &revList[i]
		if _, isKept := keep[rev.Name]; isKept {
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
