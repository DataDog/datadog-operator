// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package merger

import (
	"fmt"

	"github.com/DataDog/datadog-operator/apis/datadoghq/common"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/dependencies"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/object"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"

	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// NetworkPolicyManager use to manage network policy resources.
type NetworkPolicyManager interface {
	AddKubernetesNetworkPolicy(name, namespace string, podSelector metav1.LabelSelector, policyTypes []netv1.PolicyType, ingress []netv1.NetworkPolicyIngressRule, egress []netv1.NetworkPolicyEgressRule) error
	BuildAgentKubernetesNetworkPolicy(dda metav1.Object) error
	BuildDCAKubernetesNetworkPolicy(dda metav1.Object) error
	BuildCCRKubernetesNetworkPolicy(dda metav1.Object) error
}

// NewNetworkPolicyManager return new NetworkPolicyManager instance
func NewNetworkPolicyManager(store dependencies.StoreClient) NetworkPolicyManager {
	manager := &networkPolicyManagerImpl{
		store: store,
	}
	return manager
}

// networkPolicyManagerImpl use to manage RBAC resources.
type networkPolicyManagerImpl struct {
	store dependencies.StoreClient
}

// create kubernetes network policy
func (m *networkPolicyManagerImpl) AddKubernetesNetworkPolicy(name, namespace string, podSelector metav1.LabelSelector, policyTypes []netv1.PolicyType, ingress []netv1.NetworkPolicyIngressRule, egress []netv1.NetworkPolicyEgressRule) error {
	obj, _ := m.store.GetOrCreate(kubernetes.NetworkPoliciesKind, namespace, name)
	policy, ok := obj.(*netv1.NetworkPolicy)
	if !ok {
		return fmt.Errorf("unable to get from the store the NetworkPolicy %s", name)
	}

	policy.Spec.PodSelector = podSelector
	policy.Spec.PolicyTypes = append(policy.Spec.PolicyTypes, policyTypes...)
	policy.Spec.Ingress = append(policy.Spec.Ingress, ingress...)
	policy.Spec.Egress = append(policy.Spec.Egress, egress...)
	m.store.AddOrUpdate(kubernetes.NetworkPoliciesKind, policy)

	return nil
}

// node agent kubernetes network policy
func (m *networkPolicyManagerImpl) BuildAgentKubernetesNetworkPolicy(dda metav1.Object) error {
	policyName := dda.GetName() + common.DefaultAgentResourceSuffix
	podSelector := metav1.LabelSelector{
		MatchLabels: map[string]string{
			kubernetes.AppKubernetesInstanceLabelKey: common.DefaultAgentResourceSuffix,
			kubernetes.AppKubernetesPartOfLabelKey:   object.NewPartOfLabelValue(dda).String(),
		},
	}
	policyTypes := []netv1.PolicyType{
		netv1.PolicyTypeIngress,
		netv1.PolicyTypeEgress,
	}
	egress := []netv1.NetworkPolicyEgressRule{
		// Egress to datadog intake and
		// kubeapi server
		{
			Ports: []netv1.NetworkPolicyPort{
				{
					Port: &intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: 443,
					},
				},
			},
		},

		// The agents are susceptible to connect to any pod that would
		// be annotated with auto-discovery annotations.
		//
		// When a user wants to add a check on one of its pod, they
		// need to
		// * annotate its pod
		// * add an ingress policy from the agent on its own pod
		// In order to not ask end-users to inject NetworkPolicy on the
		// agent in the agent namespace, the agent must be allowed to
		// probe any pod.
		{},
	}

	ingress := []netv1.NetworkPolicyIngressRule{}
	// add ingress for dogstatsd/apm in dogstatsd/apm feature

	return m.AddKubernetesNetworkPolicy(policyName, dda.GetNamespace(), podSelector, policyTypes, ingress, egress)
}

// BuildDCAKubernetesNetworkPolicy
func (m *networkPolicyManagerImpl) BuildDCAKubernetesNetworkPolicy(dda metav1.Object) error {
	policyName := dda.GetName() + common.DefaultClusterAgentResourceSuffix
	podSelector := metav1.LabelSelector{
		MatchLabels: map[string]string{
			kubernetes.AppKubernetesInstanceLabelKey: common.DefaultClusterAgentResourceSuffix,
			kubernetes.AppKubernetesPartOfLabelKey:   object.NewPartOfLabelValue(dda).String(),
		},
	}
	policyTypes := []netv1.PolicyType{
		netv1.PolicyTypeIngress,
		netv1.PolicyTypeEgress,
	}
	egress := []netv1.NetworkPolicyEgressRule{
		// Egress to datadog intake and
		// kubeapi server
		{
			Ports: []netv1.NetworkPolicyPort{
				{
					Port: &intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: 443,
					},
				},
			},
		},
	}

	ingress := []netv1.NetworkPolicyIngressRule{
		// Ingress for the node agents
		{
			Ports: []netv1.NetworkPolicyPort{
				{
					Port: &intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: common.DefaultClusterAgentServicePort,
					},
				},
			},
			From: []netv1.NetworkPolicyPeer{
				{
					PodSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							kubernetes.AppKubernetesInstanceLabelKey: dda.GetName(),
							kubernetes.AppKubernetesPartOfLabelKey:   dda.GetName() + "-" + dda.GetNamespace(),
						},
					},
				},
			},
		},
	}

	// add clusterchecks ingress, port, podselector in clusterchecks feature
	// add metricsprovider ingress, port in clusterchecks feature

	return m.AddKubernetesNetworkPolicy(policyName, dda.GetNamespace(), podSelector, policyTypes, ingress, egress)
}

// BuildCCRKubernetesNetworkPolicy
func (m *networkPolicyManagerImpl) BuildCCRKubernetesNetworkPolicy(dda metav1.Object) error {
	policyName := dda.GetName() + common.DefaultClusterAgentResourceSuffix
	podSelector := metav1.LabelSelector{
		MatchLabels: map[string]string{
			kubernetes.AppKubernetesInstanceLabelKey: common.DefaultClusterChecksRunnerResourceSuffix,
			kubernetes.AppKubernetesPartOfLabelKey:   object.NewPartOfLabelValue(dda).String(),
		},
	}
	policyTypes := []netv1.PolicyType{
		netv1.PolicyTypeIngress,
		netv1.PolicyTypeEgress,
	}
	ingress := []netv1.NetworkPolicyIngressRule{}
	egress := []netv1.NetworkPolicyEgressRule{
		// Egress to datadog intake and kubeapi server
		{
			Ports: []netv1.NetworkPolicyPort{
				{
					Port: &intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: 443,
					},
				},
				{
					Port: &intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: common.DefaultClusterAgentServicePort,
					},
				},
			},
			To: []netv1.NetworkPolicyPeer{
				{
					PodSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": policyName,
						},
					},
				},
			},
		},
	}

	// The cluster check runners are susceptible to connect to any service
	// that would be annotated with auto-discovery annotations.
	//
	// When a user wants to add a check on one of its service, he needs to
	// * annotate its service
	// * add an ingress policy from the CLC on its own pod
	// In order to not ask end-users to inject NetworkPolicy on the agent in
	// the agent namespace, the agent must be allowed to probe any service.

	return m.AddKubernetesNetworkPolicy(policyName, dda.GetNamespace(), podSelector, policyTypes, ingress, egress)
}

// TODO: create cilium network policy
