// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package render

import (
	"encoding/json"
	"fmt"
	"io"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

// serializeObjects writes objects to w in the requested format ("yaml" or "json").
// YAML uses multi-document format with "---" separators.
// The scheme is used to populate apiVersion/kind on each object.
func serializeObjects(objects []client.Object, format string, s *runtime.Scheme, w io.Writer) error {
	for i, obj := range objects {
		setTypeMeta(obj, s)

		var data []byte
		var err error
		switch format {
		case "json":
			data, err = json.MarshalIndent(obj, "", "  ")
			if err != nil {
				return fmt.Errorf("failed to serialize %s/%s to JSON: %w", obj.GetNamespace(), obj.GetName(), err)
			}
		default: // yaml
			data, err = yaml.Marshal(obj)
			if err != nil {
				return fmt.Errorf("failed to serialize %s/%s to YAML: %w", obj.GetNamespace(), obj.GetName(), err)
			}
		}

		if i > 0 {
			fmt.Fprintln(w, "---")
		}
		_, err = w.Write(data)
		if err != nil {
			return err
		}
	}
	return nil
}

// cleanObject strips runtime-only fields that shouldn't appear in rendered output.
func cleanObject(obj client.Object) {
	obj.SetResourceVersion("")
	obj.SetUID("")
	obj.SetCreationTimestamp(metav1.Time{})
	obj.SetManagedFields(nil)
	obj.SetGeneration(0)

	// Remove owner references — they reference the DDA's UID which is fake
	obj.SetOwnerReferences(nil)
}

// setTypeMeta populates apiVersion and kind from the scheme's GVK mappings.
// Typed K8s objects lose their TypeMeta after being read from a client;
// this restores it so that serialized output includes the fields.
func setTypeMeta(obj client.Object, s *runtime.Scheme) {
	gvks, _, err := s.ObjectKinds(obj)
	if err != nil || len(gvks) == 0 {
		return
	}
	gvk := gvks[0]
	obj.GetObjectKind().SetGroupVersionKind(gvk)
}
