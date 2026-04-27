// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"fmt"
	"slices"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// kindOrder defines the dependency-aware ordering for Kubernetes resource kinds.
// Lower index = earlier in output.
var kindOrder = []string{
	"ServiceAccount",
	"ClusterRole",
	"ClusterRoleBinding",
	"Role",
	"RoleBinding",
	"Secret",
	"ConfigMap",
	"Service",
	"NetworkPolicy",
	"CiliumNetworkPolicy",
	"PodDisruptionBudget",
	"APIService",
	"MutatingWebhookConfiguration",
	"ValidatingWebhookConfiguration",
	"DatadogAgentInternal",
	"DaemonSet",
	"Deployment",
}

func kindPriority(kind string) int {
	idx := slices.Index(kindOrder, kind)
	if idx < 0 {
		return len(kindOrder) // unknown kinds go last
	}
	return idx
}

func resourceKey(obj client.Object) string {
	return fmt.Sprintf("%s/%s", obj.GetNamespace(), obj.GetName())
}

// resolveKind returns the Kind for obj. Fake-client objects have TypeMeta stripped,
// so we fall back to scheme.ObjectKinds when the embedded Kind is empty.
func resolveKind(obj client.Object, scheme *runtime.Scheme) string {
	kind := obj.GetObjectKind().GroupVersionKind().Kind
	if kind == "" && scheme != nil {
		if gvks, _, err := scheme.ObjectKinds(obj); err == nil && len(gvks) > 0 {
			kind = gvks[0].Kind
		}
	}
	return kind
}

// sortResources orders resources by dependency order (kindOrder) and then
// alphabetically by namespace/name within each kind.
func sortResources(objects []client.Object, scheme *runtime.Scheme) []client.Object {
	sorted := make([]client.Object, len(objects))
	copy(sorted, objects)
	slices.SortStableFunc(sorted, func(a, b client.Object) int {
		pa := kindPriority(resolveKind(a, scheme))
		pb := kindPriority(resolveKind(b, scheme))
		if pa != pb {
			return pa - pb
		}
		ka := resourceKey(a)
		kb := resourceKey(b)
		if ka < kb {
			return -1
		}
		if ka > kb {
			return 1
		}
		return 0
	})
	return sorted
}
