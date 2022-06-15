// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package merger

import (
	"fmt"
	"strconv"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/DataDog/datadog-operator/apis/datadoghq/common"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/component"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/dependencies"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/object"
	cilium "github.com/DataDog/datadog-operator/pkg/cilium/v1"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

// CiliumPolicyManager is used to manage cilium policy resources.
type CiliumPolicyManager interface {
	CreateCiliumPolicy(name, namespace string, policySpecs []cilium.NetworkPolicySpec) error
	BuildCiliumPolicy(dda metav1.Object, component v2alpha1.ComponentName) error
	SetupCiliumManager(site, ddURL string, hostNetwork bool, dnsSelectorEndpoints []metav1.LabelSelector)
}

// NewCiliumPolicyManager returns a new CiliumPolicyManager instance
func NewCiliumPolicyManager(store dependencies.StoreClient) CiliumPolicyManager {
	manager := &ciliumPolicyManagerImpl{
		store: store,
	}
	return manager
}

// ciliumPolicyManagerImpl is used to manage cilium policy resources.
type ciliumPolicyManagerImpl struct {
	store dependencies.StoreClient

	site                 string
	ddURL                string
	hostNetwork          bool
	dnsSelectorEndpoints []metav1.LabelSelector
}

func (m *ciliumPolicyManagerImpl) SetupCiliumManager(site, ddURL string, hostNetwork bool, dnsSelectorEndpoints []metav1.LabelSelector) {
	m.site = site
	m.ddURL = ddURL
	m.hostNetwork = hostNetwork
	m.dnsSelectorEndpoints = dnsSelectorEndpoints
}

// AddCiliumPolicies creates or updates multiple cilium network policies
func (m *ciliumPolicyManagerImpl) CreateCiliumPolicy(name, namespace string, policySpecs []cilium.NetworkPolicySpec) error {
	obj, _ := m.store.GetOrCreate(kubernetes.CiliumNetworkPoliciesKind, namespace, name)
	_, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return fmt.Errorf("unable to get from the store the CiliumPolicy %s", name)
	}

	newPolicy := cilium.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Specs: policySpecs,
	}

	unstructured := &unstructured.Unstructured{}
	var err error
	unstructured.Object, err = runtime.DefaultUnstructuredConverter.ToUnstructured(newPolicy)
	if err != nil {
		return err
	}
	unstructured.SetGroupVersionKind(cilium.GroupVersionCiliumNetworkPolicyKind())
	return m.store.AddOrUpdate(kubernetes.CiliumNetworkPoliciesKind, unstructured)
}

// BuildCiliumPolicy creates the base node agent, DCA, or CCR cilium network policy
func (m *ciliumPolicyManagerImpl) BuildCiliumPolicy(dda metav1.Object, componentName v2alpha1.ComponentName) error {
	policyName, podSelector := getPolicyMetadata(dda, componentName)
	var policySpecs []cilium.NetworkPolicySpec

	switch componentName {
	case v2alpha1.NodeAgentComponentName:
		policySpecs = []cilium.NetworkPolicySpec{
			egressECSPorts(podSelector),
			egressNTP(podSelector),
			egressMetadataServerRule(podSelector),
			egressDNS(podSelector, m.dnsSelectorEndpoints),
			egressAgentDatadogIntake(podSelector, m.site, m.ddURL),
			egressKubelet(podSelector),
			ingressDogstatsd(podSelector),
			egressChecks(podSelector),
		}
	case v2alpha1.ClusterAgentComponentName:
		policySpecs = []cilium.NetworkPolicySpec{
			egressMetadataServerRule(podSelector),
			egressDNS(podSelector, m.dnsSelectorEndpoints),
			egressDCADatadogIntake(podSelector, m.site, m.ddURL),
			egressKubeAPIServer(),
			ciliumIngressAgent(podSelector, dda, m.hostNetwork),
		}
	case v2alpha1.ClusterChecksRunnerComponentName:
		policySpecs = []cilium.NetworkPolicySpec{
			egressMetadataServerRule(podSelector),
			egressDNS(podSelector, m.dnsSelectorEndpoints),
			egressCCRDatadogIntake(podSelector, m.site, m.ddURL),
			egressDCA(podSelector, dda),
			egressChecks(podSelector),
		}
	}
	return m.CreateCiliumPolicy(policyName, dda.GetNamespace(), policySpecs)
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

func egressDCA(podSelector metav1.LabelSelector, dda metav1.Object) cilium.NetworkPolicySpec {
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
							kubernetes.AppKubernetesInstanceLabelKey: common.DefaultClusterAgentResourceSuffix,
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

func ciliumIngressAgent(podSelector metav1.LabelSelector, dda metav1.Object, hostNetwork bool) cilium.NetworkPolicySpec {
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
