// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package merger

import (
	"fmt"

	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/store"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"

	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NetworkPolicyManager is used to manage network policy resources.
type NetworkPolicyManager interface {
	AddKubernetesNetworkPolicy(name, namespace string, podSelector metav1.LabelSelector, policyTypes []netv1.PolicyType, ingress []netv1.NetworkPolicyIngressRule, egress []netv1.NetworkPolicyEgressRule) error
}

// NewNetworkPolicyManager returns a new NetworkPolicyManager instance
func NewNetworkPolicyManager(store store.StoreClient) NetworkPolicyManager {
	manager := &networkPolicyManagerImpl{
		store: store,
	}
	return manager
}

// networkPolicyManagerImpl is used to manage network policy resources.
type networkPolicyManagerImpl struct {
	store store.StoreClient
}

// AddKubernetesNetworkPolicy creates or updates a kubernetes network policy
func (m *networkPolicyManagerImpl) AddKubernetesNetworkPolicy(name, namespace string, podSelector metav1.LabelSelector, policyTypes []netv1.PolicyType, ingress []netv1.NetworkPolicyIngressRule, egress []netv1.NetworkPolicyEgressRule) error {
	obj, _ := m.store.GetOrCreate(kubernetes.NetworkPoliciesKind, namespace, name)
	policy, ok := obj.(*netv1.NetworkPolicy)
	if !ok {
		return fmt.Errorf("unable to get from the store the NetworkPolicy %s/%s", namespace, name)
	}

	policy.Spec.PodSelector = podSelector
	policy.Spec.PolicyTypes = append(policy.Spec.PolicyTypes, policyTypes...)
	policy.Spec.Ingress = append(policy.Spec.Ingress, ingress...)
	policy.Spec.Egress = append(policy.Spec.Egress, egress...)
	return m.store.AddOrUpdate(kubernetes.NetworkPoliciesKind, policy)
}
