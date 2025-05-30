// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package merger

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/store"
	cilium "github.com/DataDog/datadog-operator/pkg/cilium/v1"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

// CiliumPolicyManager is used to manage cilium policy resources.
type CiliumPolicyManager interface {
	AddCiliumPolicy(name, namespace string, policySpecs []cilium.NetworkPolicySpec) error
}

// NewCiliumPolicyManager returns a new CiliumPolicyManager instance
func NewCiliumPolicyManager(store store.StoreClient) CiliumPolicyManager {
	manager := &ciliumPolicyManagerImpl{
		store: store,
	}
	return manager
}

// ciliumPolicyManagerImpl is used to manage cilium policy resources.
type ciliumPolicyManagerImpl struct {
	store store.StoreClient
}

// AddCiliumPolicy creates a cilium network policy or adds policy specs to a cilium network policy
func (m *ciliumPolicyManagerImpl) AddCiliumPolicy(name, namespace string, policySpecs []cilium.NetworkPolicySpec) error {
	obj, _ := m.store.GetOrCreate(kubernetes.CiliumNetworkPoliciesKind, namespace, name)
	policy, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return fmt.Errorf("unable to get from the store the Cilium Network Policy %s/%s", namespace, name)
	}

	var typedPolicy cilium.NetworkPolicy
	err := runtime.DefaultUnstructuredConverter.FromUnstructured(policy.UnstructuredContent(), &typedPolicy)
	if err != nil {
		return fmt.Errorf("unable to convert unstructured object %s/%s to cilium network policy, err: %w", name, namespace, err)
	}

	typedPolicy.Specs = append(typedPolicy.Specs, policySpecs...)

	unstructuredPolicy := &unstructured.Unstructured{}
	unstructuredPolicy.Object, err = runtime.DefaultUnstructuredConverter.ToUnstructured(&typedPolicy)
	if err != nil {
		return fmt.Errorf("unable to convert cilium network policy %s/%s to unstructured object, err: %w", name, namespace, err)
	}
	unstructuredPolicy.SetGroupVersionKind(cilium.GroupVersionCiliumNetworkPolicyKind())
	return m.store.AddOrUpdate(kubernetes.CiliumNetworkPoliciesKind, unstructuredPolicy)
}
