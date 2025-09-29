// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagentinternal

import (
	"encoding/json"
	"fmt"
	"maps"
	"strings"

	"github.com/go-logr/logr"
	"github.com/gobwas/glob"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"

	apiutils "github.com/DataDog/datadog-operator/api/utils"
)

func mergeAnnotationsLabels(logger logr.Logger, previousVal map[string]string, newVal map[string]string, filter string) map[string]string {
	var globFilter glob.Glob
	var err error
	if filter != "" {
		globFilter, err = glob.Compile(filter)
		if err != nil {
			logger.Error(err, "Unable to parse glob filter for metadata/annotations - discarding everything", "filter", filter)
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

// CheckOwnerReference return true if owner is the owner of the object
func CheckOwnerReference(owner, object metav1.Object) bool {
	return metav1.IsControlledBy(object, owner)
}

// SetOwnerReference sets owner as a OwnerReference.
func SetOwnerReference(owner, object metav1.Object, scheme *runtime.Scheme) error {
	ro, ok := owner.(runtime.Object)
	if !ok {
		return fmt.Errorf("%T is not a runtime.Object, cannot call SetControllerReference", owner)
	}

	gvk, err := apiutil.GVKForObject(ro, scheme)
	if err != nil {
		return err
	}

	// Create a new ref
	ref := *newOwnerRef(owner, schema.GroupVersionKind{Group: gvk.Group, Version: gvk.Version, Kind: gvk.Kind})

	existingRefs := object.GetOwnerReferences()
	fi := -1
	for i, r := range existingRefs {
		if referSameObject(ref, r) {
			fi = i
		}
	}
	if fi == -1 {
		existingRefs = append(existingRefs, ref)
	} else {
		existingRefs[fi] = ref
	}

	// Update owner references
	object.SetOwnerReferences(existingRefs)
	return nil
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

// Returns true if a and b point to the same object
func referSameObject(a, b metav1.OwnerReference) bool {
	aGV, err := schema.ParseGroupVersion(a.APIVersion)
	if err != nil {
		return false
	}

	bGV, err := schema.ParseGroupVersion(b.APIVersion)
	if err != nil {
		return false
	}

	return aGV == bGV && a.Kind == b.Kind && a.Name == b.Name
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
			return apiutils.NewInt32Pointer(*currentReplicas)
		}

		// Both new and current are nil
		return nil
	}

	return apiutils.NewInt32Pointer(*newReplicas)
}
