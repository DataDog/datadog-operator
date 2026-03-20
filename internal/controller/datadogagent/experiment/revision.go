// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package experiment

import (
	"context"
	"encoding/json"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
)

// ComputeSpecHash returns a truncated MD5 hash of the DDA spec for ControllerRevision naming.
func ComputeSpecHash(spec *v2alpha1.DatadogAgentSpec) (string, error) {
	hash, err := comparison.GenerateMD5ForSpec(spec)
	if err != nil {
		return "", fmt.Errorf("failed to compute spec hash: %w", err)
	}
	if len(hash) > RevisionHashLength {
		hash = hash[:RevisionHashLength]
	}
	return hash, nil
}

// RevisionName returns the ControllerRevision name for a DDA with the given spec hash.
func RevisionName(ddaName string, specHash string) string {
	return fmt.Sprintf("%s-%s", ddaName, specHash)
}

// serializeSpec serializes a DDA spec to JSON for ControllerRevision storage.
func serializeSpec(spec *v2alpha1.DatadogAgentSpec) ([]byte, error) {
	return json.Marshal(spec)
}

// deserializeSpec deserializes a DDA spec from ControllerRevision data.
func deserializeSpec(data []byte) (*v2alpha1.DatadogAgentSpec, error) {
	spec := &v2alpha1.DatadogAgentSpec{}
	if err := json.Unmarshal(data, spec); err != nil {
		return nil, fmt.Errorf("failed to deserialize spec from revision: %w", err)
	}
	return spec, nil
}

// CreateControllerRevision creates a ControllerRevision for the given DDA spec if one
// with the same hash does not already exist. Returns the revision name and whether it was newly created.
func CreateControllerRevision(ctx context.Context, c client.Client, dda *v2alpha1.DatadogAgent, scheme *runtime.Scheme) (string, bool, error) {
	hash, err := ComputeSpecHash(&dda.Spec)
	if err != nil {
		return "", false, err
	}

	name := RevisionName(dda.Name, hash)

	// Check if it already exists
	existing := &appsv1.ControllerRevision{}
	err = c.Get(ctx, types.NamespacedName{Name: name, Namespace: dda.Namespace}, existing)
	if err == nil {
		return name, false, nil
	}
	if !apierrors.IsNotFound(err) {
		return "", false, fmt.Errorf("failed to check for existing revision %s: %w", name, err)
	}

	// Serialize spec
	data, err := serializeSpec(&dda.Spec)
	if err != nil {
		return "", false, err
	}

	cr := &appsv1.ControllerRevision{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: dda.Namespace,
		},
		Data: runtime.RawExtension{Raw: data},
	}

	// Set owner reference
	if err := controllerutil.SetControllerReference(dda, cr, scheme); err != nil {
		return "", false, fmt.Errorf("failed to set owner reference: %w", err)
	}

	if err := c.Create(ctx, cr); err != nil {
		if apierrors.IsAlreadyExists(err) {
			return name, false, nil
		}
		return "", false, fmt.Errorf("failed to create ControllerRevision %s: %w", name, err)
	}

	return name, true, nil
}

// GetControllerRevision fetches a ControllerRevision by name in the given namespace.
func GetControllerRevision(ctx context.Context, c client.Client, namespace, name string) (*appsv1.ControllerRevision, error) {
	cr := &appsv1.ControllerRevision{}
	if err := c.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, cr); err != nil {
		return nil, err
	}
	return cr, nil
}

// RestoreSpecFromRevision reads a ControllerRevision and patches the DDA spec to match the stored spec.
func RestoreSpecFromRevision(ctx context.Context, c client.Client, dda *v2alpha1.DatadogAgent, revisionName string) error {
	cr, err := GetControllerRevision(ctx, c, dda.Namespace, revisionName)
	if err != nil {
		return fmt.Errorf("failed to get revision %s for restore: %w", revisionName, err)
	}

	restoredSpec, err := deserializeSpec(cr.Data.Raw)
	if err != nil {
		return err
	}

	// Update the DDA spec in the API server
	ddaCopy := dda.DeepCopy()
	ddaCopy.Spec = *restoredSpec
	if err := c.Update(ctx, ddaCopy); err != nil {
		return fmt.Errorf("failed to update DDA spec from revision %s: %w", revisionName, err)
	}

	return nil
}

// ListOwnedRevisions lists all ControllerRevisions in the DDA namespace that are owned by the DDA.
func ListOwnedRevisions(ctx context.Context, c client.Client, dda *v2alpha1.DatadogAgent) ([]appsv1.ControllerRevision, error) {
	revList := &appsv1.ControllerRevisionList{}
	if err := c.List(ctx, revList, client.InNamespace(dda.Namespace)); err != nil {
		return nil, fmt.Errorf("failed to list ControllerRevisions: %w", err)
	}

	var owned []appsv1.ControllerRevision
	for _, rev := range revList.Items {
		if isOwnedBy(&rev, dda) {
			owned = append(owned, rev)
		}
	}
	return owned, nil
}

// GarbageCollectRevisions deletes ControllerRevisions owned by the DDA that are not in the keep set.
func GarbageCollectRevisions(ctx context.Context, c client.Client, dda *v2alpha1.DatadogAgent, keep map[string]bool) error {
	owned, err := ListOwnedRevisions(ctx, c, dda)
	if err != nil {
		return err
	}

	for i := range owned {
		if !keep[owned[i].Name] {
			if err := c.Delete(ctx, &owned[i]); err != nil && !apierrors.IsNotFound(err) {
				return fmt.Errorf("failed to delete ControllerRevision %s: %w", owned[i].Name, err)
			}
		}
	}
	return nil
}

// isOwnedBy checks if a ControllerRevision is owned by the given DDA.
func isOwnedBy(cr *appsv1.ControllerRevision, dda *v2alpha1.DatadogAgent) bool {
	for _, ref := range cr.OwnerReferences {
		if ref.UID == dda.UID {
			return true
		}
	}
	return false
}
