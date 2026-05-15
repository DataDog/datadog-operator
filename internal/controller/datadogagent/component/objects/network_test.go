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

	t.Run("node agent", func(t *testing.T) {
		policyName, namespace, specs := BuildCiliumPolicy(
			dda,
			"datadoghq.com",
			"https://custom-intake.example.com",
			false,
			[]metav1.LabelSelector{dnsSelector},
			v2alpha1.NodeAgentComponentName,
		)

		require.Equal(t, "datadog-agent", policyName)
		require.Equal(t, "agents", namespace)
		require.ElementsMatch(t, []string{
			"Egress to ECS agent port 51678",
			"Egress to ntp",
			"Egress to metadata server",
			"Egress to DNS",
			"Egress to Datadog intake",
			"Egress to kubelet",
			"Ingress for dogstatsd",
			"Egress to anything for checks",
		}, ciliumSpecDescriptions(specs))
		requireCiliumEndpointSelector(t, specs, constants.DefaultAgentResourceSuffix)

		dns := findCiliumSpec(t, specs, "Egress to DNS")
		require.Equal(t, []metav1.LabelSelector{dnsSelector}, dns.Egress[0].ToEndpoints)
		require.Equal(t, cilium.ProtocolAny, dns.Egress[0].ToPorts[0].Ports[0].Protocol)
		require.Equal(t, []cilium.FQDNSelector{{MatchPattern: "*"}}, dns.Egress[0].ToPorts[0].Rules.DNS)

		intake := findCiliumSpec(t, specs, "Egress to Datadog intake")
		require.Contains(t, intake.Egress[0].ToFQDNs, cilium.FQDNSelector{MatchName: "custom-intake.example.com"})
		require.Contains(t, intake.Egress[0].ToFQDNs, cilium.FQDNSelector{MatchName: "api.datadoghq.com"})
		require.Contains(t, intake.Egress[0].ToFQDNs, cilium.FQDNSelector{MatchName: "agent-http-intake.logs.datadoghq.com"})
		require.Contains(t, intake.Egress[0].ToPorts[0].Ports, cilium.PortProtocol{Port: "10516", Protocol: cilium.ProtocolTCP})

		dogstatsd := findCiliumSpec(t, specs, "Ingress for dogstatsd")
		require.Equal(t, "8125", dogstatsd.Ingress[0].ToPorts[0].Ports[0].Port)
		require.Equal(t, cilium.ProtocolUDP, dogstatsd.Ingress[0].ToPorts[0].Ports[0].Protocol)
	})

	t.Run("cluster agent", func(t *testing.T) {
		policyName, namespace, specs := BuildCiliumPolicy(
			dda,
			"datadoghq.com",
			"https://custom-intake.example.com",
			false,
			[]metav1.LabelSelector{dnsSelector},
			v2alpha1.ClusterAgentComponentName,
		)

		require.Equal(t, "datadog-cluster-agent", policyName)
		require.Equal(t, "agents", namespace)
		require.ElementsMatch(t, []string{
			"Egress to metadata server",
			"Egress to DNS",
			"Egress to Datadog intake",
			"Egress to Kube API Server",
			"Ingress from agent",
			"Ingress from cluster agent",
			"Egress to cluster agent",
		}, ciliumSpecDescriptions(specs))
		requireCiliumEndpointSelector(t, specs, constants.DefaultClusterAgentResourceSuffix)

		kubeAPI := findCiliumSpec(t, specs, "Egress to Kube API Server")
		require.Equal(t, []cilium.Entity{cilium.EntityKubeApiServer}, kubeAPI.Egress[0].ToEntities)

		ingressFromAgent := findCiliumSpec(t, specs, "Ingress from agent")
		require.Empty(t, ingressFromAgent.Ingress[0].FromEntities)
		require.Equal(t, "datadog-agent", ingressFromAgent.Ingress[0].FromEndpoints[0].MatchLabels[kubernetes.AppKubernetesInstanceLabelKey])
		require.Equal(t, "agents-datadog", ingressFromAgent.Ingress[0].FromEndpoints[0].MatchLabels[kubernetes.AppKubernetesPartOfLabelKey])

		intake := findCiliumSpec(t, specs, "Egress to Datadog intake")
		require.Contains(t, intake.Egress[0].ToFQDNs, cilium.FQDNSelector{MatchName: "custom-intake.example.com"})
		require.Contains(t, intake.Egress[0].ToFQDNs, cilium.FQDNSelector{MatchName: "orchestrator.datadoghq.com"})
	})

	t.Run("cluster checks runner", func(t *testing.T) {
		policyName, namespace, specs := BuildCiliumPolicy(
			dda,
			"datadoghq.com",
			"https://custom-intake.example.com",
			false,
			[]metav1.LabelSelector{dnsSelector},
			v2alpha1.ClusterChecksRunnerComponentName,
		)

		require.Equal(t, "datadog-cluster-checks-runner", policyName)
		require.Equal(t, "agents", namespace)
		require.ElementsMatch(t, []string{
			"Egress to metadata server",
			"Egress to DNS",
			"Egress to Datadog intake",
			"Egress to cluster agent",
			"Egress to anything for checks",
		}, ciliumSpecDescriptions(specs))
		requireCiliumEndpointSelector(t, specs, constants.DefaultClusterChecksRunnerResourceSuffix)

		egressToDCA := findCiliumSpec(t, specs, "Egress to cluster agent")
		require.Equal(t, "datadog-cluster-agent", egressToDCA.Egress[0].ToEndpoints[0].MatchLabels[kubernetes.AppKubernetesInstanceLabelKey])
		require.Equal(t, "agents-datadog", egressToDCA.Egress[0].ToEndpoints[0].MatchLabels[kubernetes.AppKubernetesPartOfLabelKey])

		checks := findCiliumSpec(t, specs, "Egress to anything for checks")
		require.Equal(t, "k8s:io.kubernetes.pod.namespace", checks.Egress[0].ToEndpoints[0].MatchExpressions[0].Key)
		require.Equal(t, metav1.LabelSelectorOpExists, checks.Egress[0].ToEndpoints[0].MatchExpressions[0].Operator)
	})
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

func ciliumSpecDescriptions(specs []cilium.NetworkPolicySpec) []string {
	descriptions := make([]string, 0, len(specs))
	for _, spec := range specs {
		descriptions = append(descriptions, spec.Description)
	}
	return descriptions
}

func requireCiliumEndpointSelector(t *testing.T, specs []cilium.NetworkPolicySpec, componentSuffix string) {
	t.Helper()
	for _, spec := range specs {
		require.Equal(t, componentSuffix, spec.EndpointSelector.MatchLabels[apicommon.AgentDeploymentComponentLabelKey])
		require.Equal(t, "agents-datadog", spec.EndpointSelector.MatchLabels[kubernetes.AppKubernetesPartOfLabelKey])
	}
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
