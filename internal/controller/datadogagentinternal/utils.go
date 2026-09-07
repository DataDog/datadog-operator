// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagentinternal

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"strings"

	"github.com/gobwas/glob"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

func mergeAnnotationsLabels(ctx context.Context, previousVal map[string]string, newVal map[string]string, filter string) map[string]string {
	var globFilter glob.Glob
	var err error
	if filter != "" {
		globFilter, err = glob.Compile(filter)
		if err != nil {
			ctrl.LoggerFrom(ctx).Error(err, "Unable to parse glob filter for metadata/annotations - discarding everything", "filter", filter)
		}
	}

	mergedMap := make(map[string]string, len(newVal))
	maps.Copy(mergedMap, newVal)

	// Copy from previous if not in new match and matches globfilter
	for k, v := range previousVal {
		if _, found := newVal[k]; !found {
			if (globFilter != nil && globFilter.Match(k)) || strings.Contains(k, "datadoghq.com") {
				mergedMap[k] = v
			}
		}
	}

	return mergedMap
}

// newOwnerRef creates an OwnerReference pointing to the given owner.
func newOwnerRef(owner metav1.Object, gvk schema.GroupVersionKind) *metav1.OwnerReference {
	blockOwnerDeletion := true
	isController := true
	return &metav1.OwnerReference{
		APIVersion:         gvk.GroupVersion().String(),
		Kind:               gvk.Kind,
		Name:               owner.GetName(),
		UID:                owner.GetUID(),
		BlockOwnerDeletion: &blockOwnerDeletion,
		Controller:         &isController,
	}
}

// createOwnerReferencePatch creates a patch from owner references
// We assume there is only one DDA owner reference
func createOwnerReferencePatch(ownerRef []metav1.OwnerReference, owner metav1.Object, gvk schema.GroupVersionKind) ([]byte, error) {
	patchedRefs := make([]metav1.OwnerReference, len(ownerRef))
	copy(patchedRefs, ownerRef)

	// Replace DDA owner reference with new owner reference
	for i, ref := range patchedRefs {
		if ref.Kind == "DatadogAgent" {
			patchedRefs[i] = *newOwnerRef(owner, gvk)
		}
	}

	// Create JSON patch for ownerReferences field
	refBytes, err := json.Marshal(patchedRefs)
	if err != nil {
		return nil, err
	}

	return fmt.Appendf(nil, `{"metadata":{"ownerReferences":%s}}`, string(refBytes)), nil
}

// shouldUpdateOwnerReference returns true if the owner reference is a DatadogAgent
func shouldUpdateOwnerReference(currentOwnerRef []metav1.OwnerReference) bool {
	for _, ownerRef := range currentOwnerRef {
		if ownerRef.Kind == "DatadogAgent" {
			return true
		}
	}
	return false
}

// getReplicas returns the desired replicas of a
// deployment based on the current and new replica values.
func getReplicas(currentReplicas, newReplicas *int32) *int32 {
	if newReplicas == nil {
		if currentReplicas != nil {
			// Do not overwrite the current value
			// It's most likely managed by an autoscaler
			return new(*currentReplicas)
		}

		// Both new and current are nil
		return nil
	}

	return new(*newReplicas)
}

// delete ALL workloads for a given DDA/DDAI and orphan pods
func deleteObjectAndOrphanDependents(ctx context.Context, c client.Client, obj client.Object, component string) error {
	logger := ctrl.LoggerFrom(ctx)
	propagationPolicy := metav1.DeletePropagationOrphan
	selector := labels.SelectorFromSet(labels.Set{
		kubernetes.AppKubernetesPartOfLabelKey:     obj.GetLabels()[kubernetes.AppKubernetesPartOfLabelKey],
		apicommon.AgentDeploymentComponentLabelKey: component,
	})
	logger.Info("deleting all workloads for matching DDAI", "labels", selector.String())
	if err := c.DeleteAllOf(ctx, obj, &client.DeleteAllOfOptions{ListOptions: client.ListOptions{LabelSelector: selector, Namespace: obj.GetNamespace()}, DeleteOptions: client.DeleteOptions{PropagationPolicy: &propagationPolicy}}); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("object not found, skipping deletion", "object", obj.GetName(), "namespace", obj.GetNamespace())
			return nil
		}
		return err
	}
	return nil
}
