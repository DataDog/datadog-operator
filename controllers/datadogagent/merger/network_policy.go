// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package merger

import (
	"fmt"

	"github.com/DataDog/datadog-operator/apis/datadoghq/common"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/component"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/dependencies"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/object"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"

	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// NetworkPolicyManager is used to manage network policy resources.
type NetworkPolicyManager interface {
	AddKubernetesNetworkPolicy(name, namespace string, podSelector metav1.LabelSelector, policyTypes []netv1.PolicyType, ingress []netv1.NetworkPolicyIngressRule, egress []netv1.NetworkPolicyEgressRule) error
	BuildKubernetesNetworkPolicy(dda metav1.Object, componentName v2alpha1.ComponentName) error
}

// NewNetworkPolicyManager returns a new NetworkPolicyManager instance
func NewNetworkPolicyManager(store dependencies.StoreClient) NetworkPolicyManager {
	manager := &networkPolicyManagerImpl{
		store: store,
	}
	return manager
}

// networkPolicyManagerImpl is used to manage network policy resources.
type networkPolicyManagerImpl struct {
	store dependencies.StoreClient
}

// AddKubernetesNetworkPolicy creates or updates a kubernetes network policy
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

// BuildAgentKubernetesNetworkPolicy creates the base node agent kubernetes network policy
func (m *networkPolicyManagerImpl) BuildKubernetesNetworkPolicy(dda metav1.Object, componentName v2alpha1.ComponentName) error {
	policyName, podSelector := getPolicyMetadata(dda, componentName)
	ddaNamespace := dda.GetNamespace()

	policyTypes := []netv1.PolicyType{
		netv1.PolicyTypeIngress,
		netv1.PolicyTypeEgress,
	}

	var egress []netv1.NetworkPolicyEgressRule
	var ingress []netv1.NetworkPolicyIngressRule

	switch componentName {
	case v2alpha1.NodeAgentComponentName:
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
		egress = []netv1.NetworkPolicyEgressRule{
			{
				Ports: append([]netv1.NetworkPolicyPort{}, ddIntakePort()),
			},
		}
		ingress = []netv1.NetworkPolicyIngressRule{}
	case v2alpha1.ClusterAgentComponentName:
		_, nodeAgentPodSelector := getPolicyMetadata(dda, v2alpha1.NodeAgentComponentName)
		egress = []netv1.NetworkPolicyEgressRule{
			{
				Ports: append([]netv1.NetworkPolicyPort{}, ddIntakePort()),
			},
			// Egress to other cluster agents
			{
				Ports: append([]netv1.NetworkPolicyPort{}, dcaServicePort()),
				To: []netv1.NetworkPolicyPeer{
					{
						PodSelector: &podSelector,
					},
				},
			},
		}
		ingress = []netv1.NetworkPolicyIngressRule{
			// Ingress from the node agents (for the metadata provider) and other cluster agents
			{
				Ports: append([]netv1.NetworkPolicyPort{}, dcaServicePort()),
				From: []netv1.NetworkPolicyPeer{
					{
						PodSelector: &nodeAgentPodSelector,
					},
					{
						PodSelector: &podSelector,
					},
				},
			},
			// Ingress from the node agents (for the prometheus check)
			{
				Ports: []netv1.NetworkPolicyPort{
					{
						Port: &intstr.IntOrString{
							Type:   intstr.Int,
							IntVal: 5000,
						},
					},
				},
				From: []netv1.NetworkPolicyPeer{
					{
						PodSelector: &nodeAgentPodSelector,
					},
				},
			},
		}
	case v2alpha1.ClusterChecksRunnerComponentName:
		// The cluster check runners are susceptible to connect to any service
		// that would be annotated with auto-discovery annotations.
		//
		// When a user wants to add a check on one of its service, he needs to
		// * annotate its service
		// * add an ingress policy from the CLC on its own pod
		// In order to not ask end-users to inject NetworkPolicy on the agent in
		// the agent namespace, the agent must be allowed to probe any service.
		egress = []netv1.NetworkPolicyEgressRule{
			{
				Ports: append([]netv1.NetworkPolicyPort{}, ddIntakePort(), dcaServicePort()),
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
		ingress = []netv1.NetworkPolicyIngressRule{}
	}

	return m.AddKubernetesNetworkPolicy(policyName, ddaNamespace, podSelector, policyTypes, ingress, egress)
}

func getPolicyMetadata(dda metav1.Object, componentName v2alpha1.ComponentName) (policyName string, podSelector metav1.LabelSelector) {
	var suffix string
	switch componentName {
	case v2alpha1.NodeAgentComponentName:
		policyName = component.GetAgentName(dda)
		suffix = common.DefaultAgentResourceSuffix
	case v2alpha1.ClusterAgentComponentName:
		policyName = component.GetClusterAgentName(dda)
		suffix = common.DefaultClusterAgentResourceSuffix
	case v2alpha1.ClusterChecksRunnerComponentName:
		policyName = component.GetClusterChecksRunnerName(dda)
		suffix = common.DefaultClusterChecksRunnerResourceSuffix
	}
	podSelector = metav1.LabelSelector{
		MatchLabels: map[string]string{
			kubernetes.AppKubernetesInstanceLabelKey: suffix,
			kubernetes.AppKubernetesPartOfLabelKey:   object.NewPartOfLabelValue(dda).String(),
		},
	}
	return policyName, podSelector
}

// datadog intake and kubeapi server port
func ddIntakePort() netv1.NetworkPolicyPort {
	return netv1.NetworkPolicyPort{
		Port: &intstr.IntOrString{
			Type:   intstr.Int,
			IntVal: 443,
		},
	}
}

// cluster agent service port
func dcaServicePort() netv1.NetworkPolicyPort {
	return netv1.NetworkPolicyPort{
		Port: &intstr.IntOrString{
			Type:   intstr.Int,
			IntVal: common.DefaultClusterAgentServicePort,
		},
	}
}
