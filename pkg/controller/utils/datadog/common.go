// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadog

import (
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var log = logf.Log.WithName("DatadogMetricForwarders")

// getObjKind extracts the object kind from a client.Object in a robust way
func getObjKind(obj client.Object) string {
	// First try to get the kind from GVK
	objKind := obj.GetObjectKind().GroupVersionKind().Kind

	// There is a known bug where the object frequently has empty GVK info. This is a workaround to get object Kind if that happens.
	// Ref: https://github.com/kubernetes-sigs/controller-runtime/issues/1735
	if objKind == "" {
		// Try to get it from the last-applied-configuration annotation
		if annotations := obj.GetAnnotations(); annotations != nil {
			if lastConfig, exists := annotations["kubectl.kubernetes.io/last-applied-configuration"]; exists {
				var config map[string]interface{}
				if err := json.Unmarshal([]byte(lastConfig), &config); err == nil {
					if kind, ok := config["kind"].(string); ok {
						objKind = kind
					}
				}
			}
		}
	}

	// Last fallback after GVK is empty and last-applied-configuration annotation is not present
	if objKind == "" {
		objKind = "Unknown"
	}

	return objKind
}

// getObjID builds an identifier for a given monitored object
func getObjID(obj client.Object) string {
	kind := getObjKind(obj)
	return fmt.Sprintf("%s/%s/%s", kind, obj.GetNamespace(), obj.GetName())
}

// GetNamespacedName builds a NamespacedName for a given monitored object
func GetNamespacedName(obj client.Object) types.NamespacedName {
	return types.NamespacedName{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}
}
