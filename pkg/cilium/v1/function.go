// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cilium

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// GroupVersionCiliumNetworkPolicyListKind return the schema.GroupVersionKind for CiliumNetworkPolicyList
func GroupVersionCiliumNetworkPolicyListKind() schema.GroupVersionKind {
	return schema.GroupVersionKind{
		Group:   "cilium.io",
		Version: "v2",
		Kind:    "CiliumNetworkPolicyList",
	}
}

// GroupVersionCiliumNetworkPolicyKind return the schema.GroupVersionKind for CiliumNetworkPolicy
func GroupVersionCiliumNetworkPolicyKind() schema.GroupVersionKind {
	return schema.GroupVersionKind{
		Group:   "cilium.io",
		Version: "v2",
		Kind:    "CiliumNetworkPolicy",
	}
}

// EmptyCiliumUnstructuredListPolicy return a new unstructured.UnstructuredList for CiliumNetworkPolicy
func EmptyCiliumUnstructuredListPolicy() *unstructured.UnstructuredList {
	policy := &unstructured.UnstructuredList{}
	policy.SetGroupVersionKind(GroupVersionCiliumNetworkPolicyListKind())

	return policy
}

// EmptyCiliumUnstructuredPolicy return a new unstructured.Unstructured for CiliumNetworkPolicy
func EmptyCiliumUnstructuredPolicy() *unstructured.Unstructured {
	policy := &unstructured.Unstructured{}
	policy.SetGroupVersionKind(GroupVersionCiliumNetworkPolicyKind())

	return policy
}
