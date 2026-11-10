// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package migration

import (
	"encoding/json"
	"fmt"
	"slices"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/structured-merge-diff/v6/fieldpath"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

// replicasPath stays out of the fields given to the cluster: when an HPA scales the workload, the controller does not
// apply spec.replicas, and owning it would remove it on the next apply.
var replicasPath = fieldpath.MakePathOrDie("spec", "replicas")

// jsonPatch is a JSON patch whose last operation replaces the resource version, so that the API server rejects the
// patch when the object changed since it was read.
func jsonPatch(object client.Object, operations ...map[string]any) (client.Patch, error) {
	operations = append(operations, map[string]any{"op": "replace", "path": "/metadata/resourceVersion", "value": object.GetResourceVersion()})
	patch, err := json.Marshal(operations)
	if err != nil {
		return nil, err
	}
	return client.RawPatch(types.JSONPatchType, patch), nil
}

// managedFieldsPatch gives every field of the object to manager, as if manager had applied the whole object, so that
// the next apply of manager removes the fields that it does not apply, such as the labels of the chart. The entries
// of subresources are kept, so that status writers and HPAs keep their fields.
func managedFieldsPatch(object client.Object, manager, apiVersion string) (client.Patch, error) {
	merged, err := mergeManagedFields(object.GetManagedFields(), manager, apiVersion)
	if err != nil {
		return nil, err
	}
	return jsonPatch(object, map[string]any{"op": "replace", "path": "/metadata/managedFields", "value": merged})
}

// mergeManagedFields merges the managedFields entries of every manager, apart from subresource entries, into one
// Apply entry of manager.
func mergeManagedFields(entries []metav1.ManagedFieldsEntry, manager, apiVersion string) ([]metav1.ManagedFieldsEntry, error) {
	fields := &fieldpath.Set{}
	var subresources []metav1.ManagedFieldsEntry
	for _, entry := range entries {
		if entry.Subresource != "" {
			subresources = append(subresources, entry)
			continue
		}
		if entry.FieldsV1 == nil {
			continue
		}
		entryFields := &fieldpath.Set{}
		if err := entryFields.FromJSON(entry.FieldsV1.GetRawReader()); err != nil {
			return nil, fmt.Errorf("decode fields of manager %s: %w", entry.Manager, err)
		}
		fields = fields.Union(entryFields)
	}
	raw, err := fields.Difference(fieldpath.NewSet(replicasPath)).ToJSON()
	if err != nil {
		return nil, err
	}
	merged := metav1.ManagedFieldsEntry{
		Manager:    manager,
		Operation:  metav1.ManagedFieldsOperationApply,
		APIVersion: apiVersion,
		Time:       new(metav1.Now()),
		FieldsType: "FieldsV1",
		FieldsV1:   metav1.NewFieldsV1(string(raw)),
	}
	return append([]metav1.ManagedFieldsEntry{merged}, subresources...), nil
}

// controllerReferencePatch adds the cluster as the controller of the object. It does not block the deletion of the
// cluster, which needs the permission to update the finalizers of the cluster; the next apply of the controller does.
func controllerReferencePatch(object client.Object, cluster *datadoghqv1alpha1.DatadogBYOCCluster) (client.Patch, error) {
	reference := metav1.OwnerReference{
		APIVersion: datadoghqv1alpha1.GroupVersion.String(),
		Kind:       "DatadogBYOCCluster",
		Name:       cluster.Name,
		UID:        cluster.UID,
		Controller: new(true),
	}
	if len(object.GetOwnerReferences()) == 0 {
		// Adding to the end of the list fails when the list does not exist yet.
		return jsonPatch(object, map[string]any{"op": "add", "path": "/metadata/ownerReferences", "value": []metav1.OwnerReference{reference}})
	}
	return jsonPatch(object, map[string]any{"op": "add", "path": "/metadata/ownerReferences/-", "value": reference})
}

// removeReferencesPatch removes the owner references to uid from the object.
func removeReferencesPatch(object client.Object, uid types.UID) (client.Patch, error) {
	var operations []map[string]any
	// Removing from the end keeps the indexes of the references that remain to remove.
	for i, reference := range slices.Backward(object.GetOwnerReferences()) {
		if reference.UID == uid {
			operations = append(operations, map[string]any{"op": "remove", "path": fmt.Sprintf("/metadata/ownerReferences/%d", i)})
		}
	}
	return jsonPatch(object, operations...)
}

func referencesUID(object client.Object, uid types.UID) bool {
	return slices.ContainsFunc(object.GetOwnerReferences(), func(reference metav1.OwnerReference) bool { return reference.UID == uid })
}
