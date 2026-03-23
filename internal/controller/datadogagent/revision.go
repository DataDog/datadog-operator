// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"bytes"
	"context"
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

// manageRevision creates or ensures a ControllerRevision snapshot of the current
// DDA spec and GCs old revisions beyond the current and previous.
func (r *Reconciler) manageRevision(ctx context.Context, instance *v2alpha1.DatadogAgent) error {
	revList, err := r.listRevisions(ctx, instance)
	if err != nil {
		return err
	}
	revName, err := r.ensureRevision(ctx, instance, revList)
	if err != nil {
		return err
	}
	if err := r.gcOldRevisions(ctx, instance, revName, revList); err != nil {
		ctrl.LoggerFrom(ctx).Error(err, "Failed to garbage collect old ControllerRevisions, will retry on next reconcile")
	}
	return nil
}

func (r *Reconciler) listRevisions(ctx context.Context, instance *v2alpha1.DatadogAgent) (*appsv1.ControllerRevisionList, error) {
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
	return revList, nil
}

// ensureRevision creates a ControllerRevision snapshot of the instance spec and
// annotations if it does not already exist.
//
// The Revision field is a monotonic creation counter.
func (r *Reconciler) ensureRevision(
	ctx context.Context,
	instance *v2alpha1.DatadogAgent,
	revList *appsv1.ControllerRevisionList,
) (string, error) {
	logger := ctrl.LoggerFrom(ctx)

	snap := revisionSnapshot{Spec: instance.Spec, Annotations: datadogAnnotations(instance.GetAnnotations())}
	specBytes, err := json.Marshal(snap)
	if err != nil {
		return "", fmt.Errorf("failed to marshal snapshot: %w", err)
	}

	gvks, _, err := r.scheme.ObjectKinds(instance)
	if err != nil {
		return "", fmt.Errorf("failed to get GVK for owner: %w", err)
	}

	// Find any existing revision with identical data, and track the max Revision.
	var matchingRev *appsv1.ControllerRevision
	maxRevision := int64(0)
	for i := range revList.Items {
		existing := &revList.Items[i]
		if bytes.Equal(existing.Data.Raw, specBytes) {
			matchingRev = existing
		}
		if existing.Revision > maxRevision {
			maxRevision = existing.Revision
		}
	}

	if matchingRev != nil {
		// Identical content already snapshotted. Bump Revision to max+1 if it
		// has been superseded (e.g. after a revert) so ordering stays correct.
		if matchingRev.Revision < maxRevision {
			objLogger := logger.WithValues(
				"object.kind", "ControllerRevision",
				"object.namespace", matchingRev.Namespace,
				"object.name", matchingRev.Name,
			)
			objLogger.Info("Bumping ControllerRevision to latest")
			patch := fmt.Appendf(nil, `{"revision":%d}`, maxRevision+1)
			if err := r.client.Patch(ctx, matchingRev, client.RawPatch(types.MergePatchType, patch)); err != nil && !apierrors.IsConflict(err) {
				return "", fmt.Errorf("failed to patch ControllerRevision %s: %w", matchingRev.Name, err)
			}
		}
		return matchingRev.Name, nil
	}

	nextRevision := maxRevision + 1
	data := runtime.RawExtension{Raw: specBytes}
	labels := map[string]string{
		apicommon.DatadogAgentNameLabelKey: instance.GetName(),
	}
	rev := controllerrevisions.NewControllerRevision(instance, gvks[0], labels, data, nextRevision, nil)

	// Check for a name conflict before creating.
	existingByName := make(map[string][]byte, len(revList.Items))
	for i := range revList.Items {
		existingByName[revList.Items[i].Name] = revList.Items[i].Data.Raw
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

// datadogAnnotations returns a copy of annotations filtered to only those
// with `.datadoghq.com/` in the key, which are used for preview features.
func datadogAnnotations(all map[string]string) map[string]string {
	filtered := make(map[string]string)
	for k, v := range all {
		if strings.Contains(k, ".datadoghq.com/") {
			filtered[k] = v
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	return filtered
}

// gcOldRevisions deletes all but the two most recent ControllerRevisions:
// the current and previous.
func (r *Reconciler) gcOldRevisions(
	ctx context.Context,
	instance *v2alpha1.DatadogAgent,
	current string,
	revList *appsv1.ControllerRevisionList,
) error {
	logger := ctrl.LoggerFrom(ctx)

	// Identify the most recent non-current revision to keep as previous.
	previous := ""
	previousRevision := int64(-1)
	for i := range revList.Items {
		rev := &revList.Items[i]
		if rev.Name == current {
			continue
		}
		if rev.Revision > previousRevision {
			previousRevision = rev.Revision
			previous = rev.Name
		}
	}

	for i := range revList.Items {
		rev := &revList.Items[i]
		if rev.Name == current || rev.Name == previous {
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
