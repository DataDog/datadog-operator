// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package objects

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/pkg/cilium/v1"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/equality"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

func TestGetNetworkPolicyMetadata(t *testing.T) {
	dda := testDatadogAgentObject()

	tests := []struct {
		name       string
		component  v2alpha1.ComponentName
		wantName   string
		wantSuffix string
	}{
		{
			name:       "node agent",
			component:  v2alpha1.NodeAgentComponentName,
			wantName:   "datadog-agent",
			wantSuffix: constants.DefaultAgentResourceSuffix,
		},
		{
			name:       "cluster agent",
			component:  v2alpha1.ClusterAgentComponentName,
			wantName:   "datadog-cluster-agent",
			wantSuffix: constants.DefaultClusterAgentResourceSuffix,
		},
		{
			name:       "cluster checks runner",
			component:  v2alpha1.ClusterChecksRunnerComponentName,
			wantName:   "datadog-cluster-checks-runner",
			wantSuffix: constants.DefaultClusterChecksRunnerResourceSuffix,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policyName, selector := GetNetworkPolicyMetadata(dda, tt.component)

			require.Equal(t, tt.wantName, policyName)
			require.Equal(t, tt.wantSuffix, selector.MatchLabels[apicommon.AgentDeploymentComponentLabelKey])
			require.Equal(t, "agents-datadog", selector.MatchLabels[kubernetes.AppKubernetesPartOfLabelKey])
		})
	}
}

func TestBuildKubernetesNetworkPolicy(t *testing.T) {
	dda := testDatadogAgentObject()

	tests := []struct {
		name      string
		component v2alpha1.ComponentName
	}{
		{name: "node agent", component: v2alpha1.NodeAgentComponentName},
		{name: "cluster agent", component: v2alpha1.ClusterAgentComponentName},
		{name: "cluster checks runner", component: v2alpha1.ClusterChecksRunnerComponentName},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policyName, namespace, selector, policyTypes, ingress, egress := BuildKubernetesNetworkPolicy(dda, tt.component)
			got := &netv1.NetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      policyName,
					Namespace: namespace,
				},
				Spec: netv1.NetworkPolicySpec{
					PodSelector: selector,
					PolicyTypes: policyTypes,
					Ingress:     ingress,
					Egress:      egress,
				},
			}
			want := expectedKubernetesNetworkPolicy(dda, tt.component)

			require.True(t, equality.IsEqualObject(kubernetes.NetworkPoliciesKind, got, want))
		})
	}
}

func TestBuildCiliumPolicy(t *testing.T) {
	dda := testDatadogAgentObject()
	dnsSelector := metav1.LabelSelector{
		MatchLabels: map[string]string{"k8s-app": "kube-dns"},
	}

	tests := []struct {
		name      string
		component v2alpha1.ComponentName
	}{
		{name: "node agent", component: v2alpha1.NodeAgentComponentName},
		{name: "cluster agent", component: v2alpha1.ClusterAgentComponentName},
		{name: "cluster checks runner", component: v2alpha1.ClusterChecksRunnerComponentName},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policyName, namespace, specs := BuildCiliumPolicy(
				dda,
				"datadoghq.com",
				"https://custom-intake.example.com",
				true,
				[]metav1.LabelSelector{dnsSelector},
				tt.component,
			)
			got := ciliumPolicyObject(t, policyName, namespace, specs)
			want := ciliumPolicyObject(t, policyName, namespace, expectedCiliumPolicySpecs(dda, dnsSelector, tt.component))

			require.True(t, equality.IsEqualObject(kubernetes.CiliumNetworkPoliciesKind, got, want))
		})
	}
}

func TestBuildCiliumPolicyUsesHostEntitiesForHostNetworkClusterAgent(t *testing.T) {
	_, _, specs := BuildCiliumPolicy(
		testDatadogAgentObject(),
		"datadoghq.com",
		"",
		true,
		nil,
		v2alpha1.ClusterAgentComponentName,
	)

	ingressFromAgent := findCiliumSpec(t, specs, "Ingress from agent")
	require.Equal(t, []cilium.IngressRule{
		{
			FromEntities: []cilium.Entity{cilium.EntityHost, cilium.EntityRemoteNode},
			ToPorts: []cilium.PortRule{
				{
					Ports: []cilium.PortProtocol{
						{Port: "5000", Protocol: cilium.ProtocolTCP},
						{Port: "5005", Protocol: cilium.ProtocolTCP},
					},
				},
			},
		},
	}, ingressFromAgent.Ingress)
}

func TestDefaultDDFQDNs(t *testing.T) {
	got := defaultDDFQDNs("datadoghq.com", "https://custom-intake.example.com")

	require.Equal(t, []cilium.FQDNSelector{
		{MatchName: "custom-intake.example.com"},
		{MatchPattern: "*-app.agent.datadoghq.com"},
	}, got)
}

func findCiliumSpec(t *testing.T, specs []cilium.NetworkPolicySpec, description string) cilium.NetworkPolicySpec {
	t.Helper()
	for _, spec := range specs {
		if spec.Description == description {
			return spec
		}
	}
	t.Fatalf("missing cilium spec with description %q", description)
	return cilium.NetworkPolicySpec{}
}

func expectedKubernetesNetworkPolicy(dda metav1.Object, componentName v2alpha1.ComponentName) *netv1.NetworkPolicy {
	policyName, podSelector := GetNetworkPolicyMetadata(dda, componentName)
	dcaPort := intstr.FromInt(common.DefaultClusterAgentServicePort)
	apiServerPort := intstr.FromInt(6443)
	prometheusPort := intstr.FromInt(5000)

	policy := &netv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      policyName,
			Namespace: dda.GetNamespace(),
		},
		Spec: netv1.NetworkPolicySpec{
			PodSelector: podSelector,
			PolicyTypes: []netv1.PolicyType{netv1.PolicyTypeIngress, netv1.PolicyTypeEgress},
		},
	}

	switch componentName {
	case v2alpha1.NodeAgentComponentName:
		policy.Spec.Egress = []netv1.NetworkPolicyEgressRule{
			{Ports: []netv1.NetworkPolicyPort{
				{Port: intstrPtr(intstr.FromInt(443))},
				{Protocol: protocolPtr(corev1.ProtocolUDP), Port: intstrPtr(intstr.FromInt(123))},
				{Port: intstrPtr(intstr.FromInt(80))},
				{Protocol: protocolPtr(corev1.ProtocolUDP), Port: intstrPtr(intstr.FromInt(53))},
				{Protocol: protocolPtr(corev1.ProtocolTCP), Port: intstrPtr(intstr.FromInt(53))},
				{Port: intstrPtr(intstr.FromInt(10250))},
				{},
			}},
		}
	case v2alpha1.ClusterAgentComponentName:
		_, nodeAgentSelector := GetNetworkPolicyMetadata(dda, v2alpha1.NodeAgentComponentName)
		_, ccrSelector := GetNetworkPolicyMetadata(dda, v2alpha1.ClusterChecksRunnerComponentName)
		policy.Spec.Egress = []netv1.NetworkPolicyEgressRule{
			{Ports: []netv1.NetworkPolicyPort{{Port: intstrPtr(intstr.FromInt(443))}}},
			{Ports: []netv1.NetworkPolicyPort{{Port: &apiServerPort}}},
			{Ports: []netv1.NetworkPolicyPort{{Port: intstrPtr(intstr.FromInt(80))}}},
			{
				Ports: []netv1.NetworkPolicyPort{{Port: &dcaPort}},
				To:    []netv1.NetworkPolicyPeer{{PodSelector: &podSelector}},
			},
			{Ports: []netv1.NetworkPolicyPort{
				{Protocol: protocolPtr(corev1.ProtocolUDP), Port: intstrPtr(intstr.FromInt(53))},
				{Protocol: protocolPtr(corev1.ProtocolTCP), Port: intstrPtr(intstr.FromInt(53))},
			}},
		}
		policy.Spec.Ingress = []netv1.NetworkPolicyIngressRule{
			{
				Ports: []netv1.NetworkPolicyPort{{Port: &dcaPort}},
				From: []netv1.NetworkPolicyPeer{
					{PodSelector: &nodeAgentSelector},
					{PodSelector: &podSelector},
					{PodSelector: &ccrSelector},
				},
			},
			{
				Ports: []netv1.NetworkPolicyPort{{Port: &prometheusPort}},
				From:  []netv1.NetworkPolicyPeer{{PodSelector: &nodeAgentSelector}},
			},
		}
	case v2alpha1.ClusterChecksRunnerComponentName:
		_, dcaSelector := GetNetworkPolicyMetadata(dda, v2alpha1.ClusterAgentComponentName)
		policy.Spec.Egress = []netv1.NetworkPolicyEgressRule{
			{
				Ports: []netv1.NetworkPolicyPort{{Port: &dcaPort}},
				To:    []netv1.NetworkPolicyPeer{{PodSelector: &dcaSelector}},
			},
			{Ports: []netv1.NetworkPolicyPort{
				{Port: intstrPtr(intstr.FromInt(443))},
				{Protocol: protocolPtr(corev1.ProtocolUDP), Port: intstrPtr(intstr.FromInt(123))},
				{Protocol: protocolPtr(corev1.ProtocolUDP), Port: intstrPtr(intstr.FromInt(53))},
				{Protocol: protocolPtr(corev1.ProtocolTCP), Port: intstrPtr(intstr.FromInt(53))},
				{},
			}},
		}
	}
	return policy
}

func expectedCiliumPolicySpecs(dda metav1.Object, dnsSelector metav1.LabelSelector, componentName v2alpha1.ComponentName) []cilium.NetworkPolicySpec {
	policyName, podSelector := GetNetworkPolicyMetadata(dda, componentName)
	_ = policyName
	switch componentName {
	case v2alpha1.NodeAgentComponentName:
		return []cilium.NetworkPolicySpec{
			egressECSPorts(podSelector),
			egressNTP(podSelector),
			egressMetadataServerRule(podSelector),
			egressDNS(podSelector, []metav1.LabelSelector{dnsSelector}),
			egressAgentDatadogIntake(podSelector, "datadoghq.com", "https://custom-intake.example.com"),
			egressKubelet(podSelector),
			ingressDogstatsd(podSelector),
			egressChecks(podSelector),
		}
	case v2alpha1.ClusterAgentComponentName:
		_, nodeAgentPodSelector := GetNetworkPolicyMetadata(dda, v2alpha1.NodeAgentComponentName)
		return []cilium.NetworkPolicySpec{
			egressMetadataServerRule(podSelector),
			egressDNS(podSelector, []metav1.LabelSelector{dnsSelector}),
			egressDCADatadogIntake(podSelector, "datadoghq.com", "https://custom-intake.example.com"),
			egressKubeAPIServer(podSelector),
			ingressAgent(podSelector, dda, true),
			ingressDCA(podSelector, nodeAgentPodSelector),
			egressDCA(podSelector, nodeAgentPodSelector),
		}
	case v2alpha1.ClusterChecksRunnerComponentName:
		return []cilium.NetworkPolicySpec{
			egressMetadataServerRule(podSelector),
			egressDNS(podSelector, []metav1.LabelSelector{dnsSelector}),
			egressCCRDatadogIntake(podSelector, "datadoghq.com", "https://custom-intake.example.com"),
			egressCCRToDCA(podSelector, dda),
			egressChecks(podSelector),
		}
	default:
		return nil
	}
}

func ciliumPolicyObject(t *testing.T, name, namespace string, specs []cilium.NetworkPolicySpec) *unstructured.Unstructured {
	t.Helper()
	policy := &cilium.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Specs: specs,
	}
	object, err := runtime.DefaultUnstructuredConverter.ToUnstructured(policy)
	require.NoError(t, err)
	unstructuredPolicy := &unstructured.Unstructured{Object: object}
	unstructuredPolicy.SetGroupVersionKind(cilium.GroupVersionCiliumNetworkPolicyKind())
	return unstructuredPolicy
}

func protocolPtr(protocol corev1.Protocol) *corev1.Protocol {
	return &protocol
}

func intstrPtr(value intstr.IntOrString) *intstr.IntOrString {
	return &value
}

func testDatadogAgentObject() *v2alpha1.DatadogAgent {
	return &v2alpha1.DatadogAgent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "datadog",
			Namespace: "agents",
		},
	}
}
