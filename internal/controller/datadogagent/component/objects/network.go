// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package objects

import (
	"fmt"
	"strconv"
	"strings"

	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	componentagent "github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/agent"
	componentdca "github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/clusteragent"
	componentccr "github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/clusterchecksrunner"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object"

	cilium "github.com/DataDog/datadog-operator/pkg/cilium/v1"
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
		// agent in the agent namespace, the agent must be allowed to
		// probe any pod.
		egress = []netv1.NetworkPolicyEgressRule{
			{
				Ports: append([]netv1.NetworkPolicyPort{}, ddIntakePort()),
			},
		}
		ingress = []netv1.NetworkPolicyIngressRule{}
	case v2alpha1.ClusterAgentComponentName:
		_, nodeAgentPodSelector := GetNetworkPolicyMetadata(dda, v2alpha1.NodeAgentComponentName)
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

	return policyName, ddaNamespace, podSelector, policyTypes, ingress, egress
}

// GetNetworkPolicyMetadata generates a label selector based on component
func GetNetworkPolicyMetadata(dda metav1.Object, componentName v2alpha1.ComponentName) (policyName string, podSelector metav1.LabelSelector) {
	switch componentName {
	case v2alpha1.NodeAgentComponentName:
		policyName = componentagent.GetAgentName(dda)
	case v2alpha1.ClusterAgentComponentName:
		policyName = componentdca.GetClusterAgentName(dda)
	case v2alpha1.ClusterChecksRunnerComponentName:
		policyName = componentccr.GetClusterChecksRunnerName(dda)
	}
	podSelector = metav1.LabelSelector{
		MatchLabels: map[string]string{
			kubernetes.AppKubernetesInstanceLabelKey: policyName,
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
			egressKubeAPIServer(),
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
						MatchName: fmt.Sprintf("api.%s", site),
					},
					{
						MatchName: fmt.Sprintf("agent-intake.logs.%s", site),
					},
					{
						MatchName: fmt.Sprintf("agent-http-intake.logs.%s", site),
					},
					{
						MatchName: fmt.Sprintf("process.%s", site),
					},
					{
						MatchName: fmt.Sprintf("orchestrator.%s", site),
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
				ToFQDNs: append(defaultDDFQDNs(site, ddURL), cilium.FQDNSelector{MatchName: fmt.Sprintf("orchestrator.%s", site)}),
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
func egressKubeAPIServer() cilium.NetworkPolicySpec {
	return cilium.NetworkPolicySpec{
		Description: "Egress to Kube API Server",
		Egress: []cilium.EgressRule{
			{
				// ToServices works only for endpoints
				// outside of the cluster This section
				// handles the case where the control
				// plane is outside of the cluster.
				ToServices: []cilium.Service{
					{
						K8sService: &cilium.K8sServiceNamespace{
							Namespace:   "default",
							ServiceName: "kubernetes",
						},
					},
				},
				ToEntities: []cilium.Entity{
					cilium.EntityHost,
					cilium.EntityRemoteNode,
				},
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
							kubernetes.AppKubernetesInstanceLabelKey: componentdca.GetClusterAgentName(dda),
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
			MatchPattern: fmt.Sprintf("*-app.agent.%s", site),
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
					kubernetes.AppKubernetesInstanceLabelKey: componentagent.GetAgentName(dda),
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
