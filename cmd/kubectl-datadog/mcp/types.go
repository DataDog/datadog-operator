// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package mcp

// Tool input types - these define the arguments for each MCP tool
// JSON schema tags are used by the MCP SDK for automatic schema generation

// ListAgentsArgs defines arguments for listing DatadogAgent resources
type ListAgentsArgs struct {
	Namespace     string `json:"namespace,omitempty"     jsonschema:"Kubernetes namespace to list agents from. Empty means current namespace from kubeconfig"`
	AllNamespaces bool   `json:"allNamespaces,omitempty" jsonschema:"List agents from all namespaces"`
}

// GetAgentStatusArgs defines arguments for getting DatadogAgent runtime status
type GetAgentStatusArgs struct {
	Name      string `json:"name"                jsonschema:"Name of the DatadogAgent resource"`
	Namespace string `json:"namespace,omitempty" jsonschema:"Kubernetes namespace. Empty means current namespace from kubeconfig"`
}

// DescribeAgentFeaturesArgs defines arguments for describing agent features
type DescribeAgentFeaturesArgs struct {
	Name      string `json:"name"                jsonschema:"Name of the DatadogAgent resource"`
	Namespace string `json:"namespace,omitempty" jsonschema:"Kubernetes namespace. Empty means current namespace from kubeconfig"`
}

// DescribeAgentComponentsArgs defines arguments for describing agent components
type DescribeAgentComponentsArgs struct {
	Name      string `json:"name"                jsonschema:"Name of the DatadogAgent resource"`
	Namespace string `json:"namespace,omitempty" jsonschema:"Kubernetes namespace. Empty means current namespace from kubeconfig"`
}

// GetClusterAgentLeaderArgs defines arguments for getting the cluster-agent leader pod
type GetClusterAgentLeaderArgs struct {
	Name      string `json:"name"                jsonschema:"Name of the DatadogAgent resource"`
	Namespace string `json:"namespace,omitempty" jsonschema:"Kubernetes namespace. Empty means current namespace from kubeconfig"`
}

// Tool output types - these define the structured responses returned by tools

// AgentSummary provides a high-level summary of a DatadogAgent resource
type AgentSummary struct {
	Name                      string `json:"name"`
	Namespace                 string `json:"namespace"`
	AgentStatus               string `json:"agentStatus,omitempty"`
	ClusterAgentStatus        string `json:"clusterAgentStatus,omitempty"`
	ClusterChecksRunnerStatus string `json:"clusterChecksRunnerStatus,omitempty"`
	Age                       string `json:"age"`
}

// ListAgentsOutput contains the list of DatadogAgent resources
type ListAgentsOutput struct {
	Agents []AgentSummary `json:"agents"`
	Count  int            `json:"count"`
}

// ClusterAgentLeaderOutput contains cluster-agent leader information
type ClusterAgentLeaderOutput struct {
	Name           string `json:"name"`
	Namespace      string `json:"namespace"`
	LeaderPodName  string `json:"leaderPodName"`
	ElectionMethod string `json:"electionMethod"` // "Lease" or "ConfigMap"
}
