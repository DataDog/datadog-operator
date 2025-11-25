// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package objects

import (
	"fmt"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/component"
	componentccr "github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/clusterchecksrunner"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object"
	cilium "github.com/DataDog/datadog-operator/pkg/cilium/v1"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

// BuildKubernetesNetworkPolicy creates the base node agent kubernetes network policy
func BuildKubernetesNetworkPolicy(dda metav1.Object, componentName v2alpha1.ComponentName) (string, string, metav1.LabelSelector, []netv1.PolicyType, []netv1.NetworkPolicyIngressRule, []netv1.NetworkPolicyEgressRule) {
	policyName, podSelector := GetNetworkPolicyMetadata(dda, componentName)
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
		// agent in the agent namespace, the agent has egress to everything to allow checks.
		// The other specific ports/rules are added to highlight what is used by the agent.
		egress = []netv1.NetworkPolicyEgressRule{
			{
				Ports: append([]netv1.NetworkPolicyPort{}, ddIntakePort(), ntpPort(), metadataServerPort(), dnsUDPPort(), dnsTCPPort(), kubeletPort(), allTCPPorts()),
			},
		}
		ingress = []netv1.NetworkPolicyIngressRule{}
	case v2alpha1.ClusterAgentComponentName:
		_, nodeAgentPodSelector := GetNetworkPolicyMetadata(dda, v2alpha1.NodeAgentComponentName)
		_, ccrPodSelector := GetNetworkPolicyMetadata(dda, v2alpha1.ClusterChecksRunnerComponentName)
		egress = []netv1.NetworkPolicyEgressRule{
			// Egress to Datadog intake and API Server
			{
				Ports: append([]netv1.NetworkPolicyPort{}, ddIntakePort()),
			},
			// Egress to 6443 (sometimes for API server)
			{
				Ports: append([]netv1.NetworkPolicyPort{}, netv1.NetworkPolicyPort{
					Port: &intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: 6443,
					},
				}),
			},
			// Egress to metadata server
			{
				Ports: append([]netv1.NetworkPolicyPort{}, metadataServerPort()),
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
			// Egress to DNS
			{
				Ports: append([]netv1.NetworkPolicyPort{}, dnsUDPPort(), dnsTCPPort()),
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
					{
						PodSelector: &ccrPodSelector,
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
		_, dcaPodSelector := GetNetworkPolicyMetadata(dda, v2alpha1.ClusterAgentComponentName)
		egress = []netv1.NetworkPolicyEgressRule{
			// Egress to DCA service
			{
				Ports: append([]netv1.NetworkPolicyPort{}, dcaServicePort()),
				To: []netv1.NetworkPolicyPeer{
					{
						PodSelector: &dcaPodSelector,
					},
				},
			},
			// Egress to everything for checks, intake, DNS
			// This makes the first rule not necessary but we keep it for clarity
			{
				Ports: append([]netv1.NetworkPolicyPort{}, ddIntakePort(), ntpPort(), dnsUDPPort(), dnsTCPPort(), allTCPPorts()),
			},
		}
		ingress = []netv1.NetworkPolicyIngressRule{}
	}

	return policyName, ddaNamespace, podSelector, policyTypes, ingress, egress
}

// GetNetworkPolicyMetadata generates a label selector based on component
func GetNetworkPolicyMetadata(dda metav1.Object, componentName v2alpha1.ComponentName) (policyName string, podSelector metav1.LabelSelector) {
	var comp string
	switch componentName {
	case v2alpha1.NodeAgentComponentName:
		policyName = component.GetAgentName(dda)
		comp = constants.DefaultAgentResourceSuffix
	case v2alpha1.ClusterAgentComponentName:
		policyName = component.GetClusterAgentName(dda)
		comp = constants.DefaultClusterAgentResourceSuffix
	case v2alpha1.ClusterChecksRunnerComponentName:
		policyName = componentccr.GetClusterChecksRunnerName(dda)
		comp = constants.DefaultClusterChecksRunnerResourceSuffix
	}
	podSelector = metav1.LabelSelector{
		MatchLabels: map[string]string{
			apicommon.AgentDeploymentComponentLabelKey: comp,
			kubernetes.AppKubernetesPartOfLabelKey:     object.NewPartOfLabelValue(dda).String(),
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

// ntp UDP port
func ntpPort() netv1.NetworkPolicyPort {
	protocol := corev1.ProtocolUDP
	return netv1.NetworkPolicyPort{
		Protocol: &protocol,
		Port: &intstr.IntOrString{
			Type:   intstr.Int,
			IntVal: 123,
		},
	}
}

// metadata server port

func metadataServerPort() netv1.NetworkPolicyPort {
	return netv1.NetworkPolicyPort{
		Port: &intstr.IntOrString{
			Type:   intstr.Int,
			IntVal: 80,
		},
	}
}

// dns port
func dnsUDPPort() netv1.NetworkPolicyPort {
	protocol := corev1.ProtocolUDP
	return netv1.NetworkPolicyPort{
		Protocol: &protocol,
		Port: &intstr.IntOrString{
			Type:   intstr.Int,
			IntVal: 53,
		},
	}
}

func dnsTCPPort() netv1.NetworkPolicyPort {
	protocol := corev1.ProtocolTCP
	return netv1.NetworkPolicyPort{
		Protocol: &protocol,
		Port: &intstr.IntOrString{
			Type:   intstr.Int,
			IntVal: 53,
		},
	}
}

// kubelet port

func kubeletPort() netv1.NetworkPolicyPort {
	return netv1.NetworkPolicyPort{
		Port: &intstr.IntOrString{
			Type:   intstr.Int,
			IntVal: 10250,
		},
	}
}

// all ports to allow checks

func allTCPPorts() netv1.NetworkPolicyPort {
	return netv1.NetworkPolicyPort{}
}

// BuildCiliumPolicy creates the base node agent, DCA, or CCR cilium network policy
func BuildCiliumPolicy(dda metav1.Object, site string, ddURL string, hostNetwork bool, dnsSelectorEndpoints []metav1.LabelSelector, componentName v2alpha1.ComponentName) (string, string, []cilium.NetworkPolicySpec) {
	policyName, podSelector := GetNetworkPolicyMetadata(dda, componentName)
	var policySpecs []cilium.NetworkPolicySpec

	switch componentName {
	case v2alpha1.NodeAgentComponentName:
		policySpecs = []cilium.NetworkPolicySpec{
			egressECSPorts(podSelector),
			egressNTP(podSelector),
			egressMetadataServerRule(podSelector),
			egressDNS(podSelector, dnsSelectorEndpoints),
			egressAgentDatadogIntake(podSelector, site, ddURL),
			egressKubelet(podSelector),
			ingressDogstatsd(podSelector),
			egressChecks(podSelector),
		}
	case v2alpha1.ClusterAgentComponentName:
		_, nodeAgentPodSelector := GetNetworkPolicyMetadata(dda, v2alpha1.NodeAgentComponentName)
		policySpecs = []cilium.NetworkPolicySpec{
			egressMetadataServerRule(podSelector),
			egressDNS(podSelector, dnsSelectorEndpoints),
			egressDCADatadogIntake(podSelector, site, ddURL),
			egressKubeAPIServer(podSelector),
			ingressAgent(podSelector, dda, hostNetwork),
			ingressDCA(podSelector, nodeAgentPodSelector),
			egressDCA(podSelector, nodeAgentPodSelector),
		}
	case v2alpha1.ClusterChecksRunnerComponentName:
		policySpecs = []cilium.NetworkPolicySpec{
			egressMetadataServerRule(podSelector),
			egressDNS(podSelector, dnsSelectorEndpoints),
			egressCCRDatadogIntake(podSelector, site, ddURL),
			egressCCRToDCA(podSelector, dda),
			egressChecks(podSelector),
		}
	}
	return policyName, dda.GetNamespace(), policySpecs
}

// cilium egress ports for ECS
func egressECSPorts(podSelector metav1.LabelSelector) cilium.NetworkPolicySpec {
	return cilium.NetworkPolicySpec{
		Description:      "Egress to ECS agent port 51678",
		EndpointSelector: podSelector,
		Egress: []cilium.EgressRule{
			{
				ToEntities: []cilium.Entity{cilium.EntityHost},
				ToPorts: []cilium.PortRule{
					{
						Ports: []cilium.PortProtocol{
							{
								Port:     "51678",
								Protocol: cilium.ProtocolTCP,
							},
						},
					},
				},
			},
			{
				ToCIDR: []string{"169.254.0.0/16"},
				ToPorts: []cilium.PortRule{
					{
						Ports: []cilium.PortProtocol{
							{
								Port:     "51678",
								Protocol: cilium.ProtocolTCP,
							},
						},
					},
				},
			},
		},
	}
}

// cilium NTP egress
func egressNTP(podSelector metav1.LabelSelector) cilium.NetworkPolicySpec {
	return cilium.NetworkPolicySpec{
		Description:      "Egress to ntp",
		EndpointSelector: podSelector,
		Egress: []cilium.EgressRule{
			{
				ToPorts: []cilium.PortRule{
					{
						Ports: []cilium.PortProtocol{
							{
								Port:     "123",
								Protocol: cilium.ProtocolUDP,
							},
						},
					},
				},
				ToFQDNs: []cilium.FQDNSelector{
					{
						MatchPattern: "*.datadog.pool.ntp.org",
					},
				},
			},
		},
	}
}

// cilium egress for agent intake endpoints
func egressAgentDatadogIntake(podSelector metav1.LabelSelector, site string, ddURL string) cilium.NetworkPolicySpec {
	return cilium.NetworkPolicySpec{
		Description:      "Egress to Datadog intake",
		EndpointSelector: podSelector,
		Egress: []cilium.EgressRule{
			{
				ToFQDNs: append(defaultDDFQDNs(site, ddURL), []cilium.FQDNSelector{
					{
						MatchName: "api." + site,
					},
					{
						MatchName: "agent-intake.logs." + site,
					},
					{
						MatchName: "agent-http-intake.logs." + site,
					},
					{
						MatchName: "process." + site,
					},
					{
						MatchName: "orchestrator." + site,
					},
				}...),
				ToPorts: []cilium.PortRule{
					{
						Ports: []cilium.PortProtocol{
							{
								Port:     "443",
								Protocol: cilium.ProtocolTCP,
							},
							{
								Port:     "10516",
								Protocol: cilium.ProtocolTCP,
							},
						},
					},
				},
			},
		},
	}
}

// cilium egress for DCA intake endpoints
func egressDCADatadogIntake(podSelector metav1.LabelSelector, site string, ddURL string) cilium.NetworkPolicySpec {
	return cilium.NetworkPolicySpec{
		Description:      "Egress to Datadog intake",
		EndpointSelector: podSelector,
		Egress: []cilium.EgressRule{
			{
				ToFQDNs: append(defaultDDFQDNs(site, ddURL), cilium.FQDNSelector{MatchName: "orchestrator." + site}),
				ToPorts: []cilium.PortRule{
					{
						Ports: []cilium.PortProtocol{
							{
								Port:     "443",
								Protocol: cilium.ProtocolTCP,
							},
						},
					},
				},
			},
		},
	}
}

// cilium egress for CCR intake endpoints
func egressCCRDatadogIntake(podSelector metav1.LabelSelector, site string, ddURL string) cilium.NetworkPolicySpec {
	return cilium.NetworkPolicySpec{
		Description:      "Egress to Datadog intake",
		EndpointSelector: podSelector,
		Egress: []cilium.EgressRule{
			{
				ToFQDNs: defaultDDFQDNs(site, ddURL),
				ToPorts: []cilium.PortRule{
					{
						Ports: []cilium.PortProtocol{
							{
								Port:     "443",
								Protocol: cilium.ProtocolTCP,
							},
						},
					},
				},
			},
		},
	}
}

// cilium egress to kubelet
func egressKubelet(podSelector metav1.LabelSelector) cilium.NetworkPolicySpec {
	return cilium.NetworkPolicySpec{
		Description:      "Egress to kubelet",
		EndpointSelector: podSelector,
		Egress: []cilium.EgressRule{
			{
				ToEntities: []cilium.Entity{
					cilium.EntityHost,
				},
				ToPorts: []cilium.PortRule{
					{
						Ports: []cilium.PortProtocol{
							{
								Port:     "10250",
								Protocol: cilium.ProtocolTCP,
							},
						},
					},
				},
			},
		},
	}
}

// cilium ingress for dogstatsd
func ingressDogstatsd(podSelector metav1.LabelSelector) cilium.NetworkPolicySpec {
	return cilium.NetworkPolicySpec{
		Description:      "Ingress for dogstatsd",
		EndpointSelector: podSelector,
		Ingress: []cilium.IngressRule{
			{
				FromEndpoints: []metav1.LabelSelector{
					{},
				},
				ToPorts: []cilium.PortRule{
					{
						Ports: []cilium.PortProtocol{
							{
								Port:     strconv.Itoa(common.DefaultDogstatsdPort),
								Protocol: cilium.ProtocolUDP,
							},
						},
					},
				},
			},
		},
	}
}

// cilium egress to metadata server for cloud providers
func egressMetadataServerRule(podSelector metav1.LabelSelector) cilium.NetworkPolicySpec {
	return cilium.NetworkPolicySpec{
		Description:      "Egress to metadata server",
		EndpointSelector: podSelector,
		Egress: []cilium.EgressRule{
			{
				ToCIDR: []string{"169.254.169.254/32"},
				ToPorts: []cilium.PortRule{
					{
						Ports: []cilium.PortProtocol{
							{
								Port:     "80",
								Protocol: cilium.ProtocolTCP,
							},
						},
					},
				},
			},
		},
	}
}

// cilium egress to dns endpoints
func egressDNS(podSelector metav1.LabelSelector, dnsSelectorEndpoints []metav1.LabelSelector) cilium.NetworkPolicySpec {
	return cilium.NetworkPolicySpec{
		Description:      "Egress to DNS",
		EndpointSelector: podSelector,
		Egress: []cilium.EgressRule{
			{
				ToEndpoints: dnsSelectorEndpoints,
				ToPorts: []cilium.PortRule{
					{
						Ports: []cilium.PortProtocol{
							{
								Port:     "53",
								Protocol: cilium.ProtocolAny,
							},
						},
						Rules: &cilium.L7Rules{
							DNS: []cilium.FQDNSelector{
								{
									MatchPattern: "*",
								},
							},
						},
					},
				},
			},
		},
	}
}

// The agents are susceptible to connect to any pod that would be annotated
// with auto-discovery annotations.
//
// When a user wants to add a check on one of its pod, he needs to
// * annotate its pod
// * add an ingress policy from the agent on its own pod
//
// In order to not ask end-users to inject NetworkPolicy on the agent in the
// agent namespace, the agent must be allowed to probe any pod.
func egressChecks(podSelector metav1.LabelSelector) cilium.NetworkPolicySpec {
	return cilium.NetworkPolicySpec{
		Description:      "Egress to anything for checks",
		EndpointSelector: podSelector,
		Egress: []cilium.EgressRule{
			{
				ToEndpoints: []metav1.LabelSelector{
					{
						MatchExpressions: []metav1.LabelSelectorRequirement{
							{
								Key:      "k8s:io.kubernetes.pod.namespace",
								Operator: "Exists",
							},
						},
					},
				},
			},
		},
	}
}

// cilium egress to kube api server
func egressKubeAPIServer(podSelector metav1.LabelSelector) cilium.NetworkPolicySpec {
	return cilium.NetworkPolicySpec{
		Description:      "Egress to Kube API Server",
		EndpointSelector: podSelector,
		Egress: []cilium.EgressRule{
			{
				ToEntities: []cilium.Entity{
					cilium.EntityKubeApiServer,
				},
			},
		},
	}
}

// cilium egress for CCR to DCA
func egressCCRToDCA(podSelector metav1.LabelSelector, dda metav1.Object) cilium.NetworkPolicySpec {
	return cilium.NetworkPolicySpec{
		Description:      "Egress to cluster agent",
		EndpointSelector: podSelector,
		Egress: []cilium.EgressRule{
			{
				ToPorts: []cilium.PortRule{
					{
						Ports: []cilium.PortProtocol{
							{
								Port:     "5005",
								Protocol: cilium.ProtocolTCP,
							},
						},
					},
				},
				ToEndpoints: []metav1.LabelSelector{
					{
						MatchLabels: map[string]string{
							kubernetes.AppKubernetesInstanceLabelKey: component.GetClusterAgentName(dda),
							kubernetes.AppKubernetesPartOfLabelKey:   fmt.Sprintf("%s-%s", dda.GetNamespace(), dda.GetName()),
						},
					},
				},
			},
		},
	}
}

func defaultDDFQDNs(site, ddURL string) []cilium.FQDNSelector {
	selectors := []cilium.FQDNSelector{}
	if ddURL != "" {
		selectors = append(selectors, cilium.FQDNSelector{
			MatchName: strings.TrimPrefix(ddURL, "https://"),
		})
	}

	selectors = append(selectors, []cilium.FQDNSelector{
		{
			MatchPattern: "*-app.agent." + site,
		},
	}...)

	return selectors
}

// cilium ingress from agent
func ingressAgent(podSelector metav1.LabelSelector, dda metav1.Object, hostNetwork bool) cilium.NetworkPolicySpec {
	ingress := cilium.IngressRule{
		ToPorts: []cilium.PortRule{
			{
				Ports: []cilium.PortProtocol{
					{
						Port:     "5000",
						Protocol: cilium.ProtocolTCP,
					},
					{
						Port:     "5005",
						Protocol: cilium.ProtocolTCP,
					},
				},
			},
		},
	}

	if hostNetwork {
		ingress.FromEntities = []cilium.Entity{
			cilium.EntityHost,
			cilium.EntityRemoteNode,
		}
	} else {
		ingress.FromEndpoints = []metav1.LabelSelector{
			{
				MatchLabels: map[string]string{
					kubernetes.AppKubernetesInstanceLabelKey: component.GetAgentName(dda),
					kubernetes.AppKubernetesPartOfLabelKey:   fmt.Sprintf("%s-%s", dda.GetNamespace(), dda.GetName()),
				},
			},
		}
	}

	return cilium.NetworkPolicySpec{
		Description:      "Ingress from agent",
		EndpointSelector: podSelector,
		Ingress:          []cilium.IngressRule{ingress},
	}
}

// cilium ingress from DCA
func ingressDCA(podSelector metav1.LabelSelector, nodeAgentPodSelector metav1.LabelSelector) cilium.NetworkPolicySpec {
	return cilium.NetworkPolicySpec{
		Description:      "Ingress from cluster agent",
		EndpointSelector: podSelector,
		Ingress: []cilium.IngressRule{
			{
				FromEndpoints: []metav1.LabelSelector{
					nodeAgentPodSelector,
				},
				ToPorts: []cilium.PortRule{
					{
						Ports: []cilium.PortProtocol{
							{
								Port:     "5005",
								Protocol: cilium.ProtocolTCP,
							},
						},
					},
				},
			},
		},
	}
}

// cilium egress to DCA
func egressDCA(podSelector metav1.LabelSelector, nodeAgentPodSelector metav1.LabelSelector) cilium.NetworkPolicySpec {
	return cilium.NetworkPolicySpec{
		Description:      "Egress to cluster agent",
		EndpointSelector: podSelector,
		Egress: []cilium.EgressRule{
			{
				ToEndpoints: []metav1.LabelSelector{
					nodeAgentPodSelector,
				},
				ToPorts: []cilium.PortRule{
					{
						Ports: []cilium.PortProtocol{
							{
								Port:     "5005",
								Protocol: cilium.ProtocolTCP,
							},
						},
					},
				},
			},
		},
	}
}
