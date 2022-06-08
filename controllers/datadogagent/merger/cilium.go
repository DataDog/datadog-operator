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
	"github.com/DataDog/datadog-operator/controllers/datadogagent/component"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/dependencies"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/object"
	cilium "github.com/DataDog/datadog-operator/pkg/cilium/v1"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

// CiliumPolicyManager is used to manage cilium policy resources.
type CiliumPolicyManager interface {
	CreateCiliumPolicy(name, namespace string, policySpecs []cilium.NetworkPolicySpec) error
	BuildAgentCiliumPolicy(dda metav1.Object) error
	BuildDCACiliumPolicy(dda metav1.Object) error
	BuildCCRCiliumPolicy(dda metav1.Object) error
	SetDDASite(site string)
	SetDDAURL(url string)
	SetHostNetwork(hostNetwork bool)
	SetDNSSelectorEndpoints(dnsSelectorEndpoints []metav1.LabelSelector)
	ciliumIngressAgent(podSelector metav1.LabelSelector, dda metav1.Object) cilium.NetworkPolicySpec
	ciliumEgressDNS(podSelector metav1.LabelSelector) cilium.NetworkPolicySpec
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

func (m *ciliumPolicyManagerImpl) SetDDASite(site string) {
	m.site = site
}

func (m *ciliumPolicyManagerImpl) SetDDAURL(url string) {
	m.ddURL = url
}

func (m *ciliumPolicyManagerImpl) SetHostNetwork(hostNetwork bool) {
	m.hostNetwork = hostNetwork
}

func (m *ciliumPolicyManagerImpl) SetDNSSelectorEndpoints(dnsSelectorEndpoints []metav1.LabelSelector) {
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

// BuildAgentCiliumPolicy creates the base node agent kubernetes cilium policy
func (m *ciliumPolicyManagerImpl) BuildAgentCiliumPolicy(dda metav1.Object) error {
	policyName := component.GetAgentName(dda)
	podSelector := metav1.LabelSelector{
		MatchLabels: map[string]string{
			kubernetes.AppKubernetesInstanceLabelKey: common.DefaultAgentResourceSuffix,
			kubernetes.AppKubernetesPartOfLabelKey:   object.NewPartOfLabelValue(dda).String(),
		},
	}
	site := m.site
	policySpecs := []cilium.NetworkPolicySpec{
		{
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
		},
		{
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
		},
		ciliumEgressMetadataServerRule(podSelector),
		m.ciliumEgressDNS(podSelector),
		{
			Description:      "Egress to Datadog intake",
			EndpointSelector: podSelector,
			Egress: []cilium.EgressRule{
				{
					ToFQDNs: append(m.defaultDDFQDNs(), []cilium.FQDNSelector{
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
		},
		{
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
		},
		{
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
		},
		ciliumEgressChecks(podSelector),
	}

	return m.CreateCiliumPolicy(policyName, dda.GetNamespace(), policySpecs)
}

// BuildDCACiliumPolicy creates the base cluster agent cilium policy
func (m *ciliumPolicyManagerImpl) BuildDCACiliumPolicy(dda metav1.Object) error {
	policyName := component.GetClusterAgentName(dda)
	podSelector := metav1.LabelSelector{
		MatchLabels: map[string]string{
			kubernetes.AppKubernetesInstanceLabelKey: common.DefaultClusterAgentResourceSuffix,
			kubernetes.AppKubernetesPartOfLabelKey:   object.NewPartOfLabelValue(dda).String(),
		},
	}
	policySpecs := []cilium.NetworkPolicySpec{
		ciliumEgressMetadataServerRule(podSelector),
		m.ciliumEgressDNS(podSelector),
		{
			Description:      "Egress to Datadog intake",
			EndpointSelector: podSelector,
			Egress: []cilium.EgressRule{
				{
					ToFQDNs: append(m.defaultDDFQDNs(), cilium.FQDNSelector{MatchName: fmt.Sprintf("orchestrator.%s", m.site)}),
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
		},
		{
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
		},
		m.ciliumIngressAgent(podSelector, dda),
	}

	return m.CreateCiliumPolicy(policyName, dda.GetNamespace(), policySpecs)
}

// BuildCCRCiliumPolicy creates the base cluster checks runner cilium policy
func (m *ciliumPolicyManagerImpl) BuildCCRCiliumPolicy(dda metav1.Object) error {
	policyName := component.GetClusterChecksRunnerName(dda)
	podSelector := metav1.LabelSelector{
		MatchLabels: map[string]string{
			kubernetes.AppKubernetesInstanceLabelKey: common.DefaultClusterChecksRunnerResourceSuffix,
			kubernetes.AppKubernetesPartOfLabelKey:   object.NewPartOfLabelValue(dda).String(),
		},
	}
	policySpecs := []cilium.NetworkPolicySpec{
		ciliumEgressMetadataServerRule(podSelector),
		m.ciliumEgressDNS(podSelector),
		{
			Description:      "Egress to Datadog intake",
			EndpointSelector: podSelector,
			Egress: []cilium.EgressRule{
				{
					ToFQDNs: m.defaultDDFQDNs(),
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
		},
		{
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
		},
		ciliumEgressChecks(podSelector),
	}

	return m.CreateCiliumPolicy(policyName, dda.GetNamespace(), policySpecs)
}

func ciliumEgressMetadataServerRule(podSelector metav1.LabelSelector) cilium.NetworkPolicySpec {
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

func (m *ciliumPolicyManagerImpl) ciliumEgressDNS(podSelector metav1.LabelSelector) cilium.NetworkPolicySpec {
	return cilium.NetworkPolicySpec{
		Description:      "Egress to DNS",
		EndpointSelector: podSelector,
		Egress: []cilium.EgressRule{
			{
				ToEndpoints: m.dnsSelectorEndpoints,
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
func ciliumEgressChecks(podSelector metav1.LabelSelector) cilium.NetworkPolicySpec {
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

func (m *ciliumPolicyManagerImpl) defaultDDFQDNs() []cilium.FQDNSelector {
	selectors := []cilium.FQDNSelector{}
	if m.ddURL != "" {
		selectors = append(selectors, cilium.FQDNSelector{
			MatchName: strings.TrimPrefix(m.ddURL, "https://"),
		})
	}

	selectors = append(selectors, []cilium.FQDNSelector{
		{
			MatchPattern: fmt.Sprintf("*-app.agent.%s", m.site),
		},
	}...)

	return selectors
}

func (m *ciliumPolicyManagerImpl) ciliumIngressAgent(podSelector metav1.LabelSelector, dda metav1.Object) cilium.NetworkPolicySpec {
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

	if m.hostNetwork {
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
