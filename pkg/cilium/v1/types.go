// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cilium

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Protocol is a Cilium network protocol
type Protocol string

const (
	// ProtocolTCP refers to the TCP network protocol
	ProtocolTCP Protocol = "TCP"
	// ProtocolUDP refers to the UDP network protocol
	ProtocolUDP Protocol = "UDP"
	// ProtocolAny refers to any network protocol
	ProtocolAny Protocol = "ANY"
)

// Entity is a Cilium rule entity
type Entity string

const (
	// EntityHost is a host entity
	EntityHost Entity = "host"
	// EntityRemoteNode is a remote-node entity
	EntityRemoteNode Entity = "remote-node"
	// EntityWorld is a world entity
	EntityWorld Entity = "world"
	// EntityKubeApiServer is a Kube Api Server
	EntityKubeApiServer Entity = "kube-apiserver"
)

// NetworkPolicy is a Cilium network policy
type NetworkPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Specs []NetworkPolicySpec `json:"specs,omitempty"`
}

// NetworkPolicySpec is a Cilium network policy spec
type NetworkPolicySpec struct {
	Description      string               `json:"description,omitempty"`
	EndpointSelector metav1.LabelSelector `json:"endpointSelector,omitempty"`
	Ingress          []IngressRule        `json:"ingress,omitempty"`
	Egress           []EgressRule         `json:"egress,omitempty"`
}

// IngressRule is a Cilium ingress rule
type IngressRule struct {
	FromEndpoints []metav1.LabelSelector `json:"fromEndpoints,omitempty"`
	FromEntities  []Entity               `json:"fromEntities,omitempty"`
	ToPorts       []PortRule             `json:"toPorts,omitempty"`
}

// EgressRule is a Cilium egress rule
type EgressRule struct {
	ToCIDR      []string               `json:"toCIDR,omitempty"`
	ToPorts     []PortRule             `json:"toPorts,omitempty"`
	ToEndpoints []metav1.LabelSelector `json:"toEndpoints,omitempty"`
	ToFQDNs     []FQDNSelector         `json:"toFQDNs,omitempty"`
	ToEntities  []Entity               `json:"toEntities,omitempty"`
	ToServices  []Service              `json:"toServices,omitempty"`
}

// PortRule is a Cilium port rule
type PortRule struct {
	Ports []PortProtocol `json:"ports,omitempty"`
	Rules *L7Rules       `json:"rules,omitempty"`
}

// PortProtocol is a Cilium port protocol
type PortProtocol struct {
	Port     string   `json:"port,omitempty"`
	Protocol Protocol `json:"protocol,omitempty"`
}

// L7Rules is a Cilium L7 port rule
type L7Rules struct {
	DNS []FQDNSelector `json:"dns,omitempty"`
}

// FQDNSelector is a Cilium FQDN selector
type FQDNSelector struct {
	MatchName    string `json:"matchName,omitempty"`
	MatchPattern string `json:"matchPattern,omitempty"`
}

// Service is a Cilium service selector
type Service struct {
	K8sServiceSelector *K8sServiceSelectorNamespace `json:"k8sServiceSelector,omitempty"`
	K8sService         *K8sServiceNamespace         `json:"k8sService,omitempty"`
}

// K8sServiceNamespace is a Cilium service + namespace
type K8sServiceNamespace struct {
	ServiceName string `json:"serviceName,omitempty"`
	Namespace   string `json:"namespace,omitempty"`
}

// K8sServiceSelectorNamespace is a Cilium service selector + namespace
type K8sServiceSelectorNamespace struct {
	Selector  metav1.LabelSelector `json:"selector"`
	Namespace string               `json:"namespace,omitempty"`
}
