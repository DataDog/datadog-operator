// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kubernetes

import (
	"strings"

	corev1 "k8s.io/api/core/v1"
)

const (
	// DDA/DDAI annotation key for provider used in reconciler to apply provider-specific configs
	ProviderAnnotationKey = "agent.datadoghq.com/cluster-provider"

	// LegacyProvider Legacy Provider (empty name)
	LegacyProvider = ""
	// DefaultProvider Default provider name
	DefaultProvider = "default"

	// GKE provider types: https://cloud.google.com/kubernetes-engine/docs/concepts/node-images#available_node_images
	// GKECosType is the Container-Optimized OS node image offered by GKE
	GKECosType = "cos"

	// GKECloudProvider GKE CloudProvider name
	GKECloudProvider = "gke"

	// GKECosProvider is the full provider string for GKE on Container-Optimized OS
	// nodes (matches the `{cloudProvider}-{value}` convention from
	// generateValidProviderName). Used as a ProviderCapabilityMap key.
	GKECosProvider = "gke-cos"

	// GKEAutopilotProvider is the provider string for GKE Autopilot clusters. It is
	// not node-label-derived; it is declared via the `datadoghq.com/provider`
	// annotation (or mapped from the experimental autopilot opt-in annotation) and
	// used as a NodeAgentProviderCapabilities map key.
	GKEAutopilotProvider = "gke-autopilot"

	// GKEProviderLabel is the GKE node label used to determine the node's provider
	GKEProviderLabel = "cloud.google.com/gke-os-distribution"

	// WindowsProvider marks a DatadogAgent(Internal)/DatadogAgentProfile whose node agent
	// targets Windows nodes. Set via the datadoghq.com/provider annotation on a
	// DatadogAgentProfile that selects Windows nodes; the node-agent reconcile then builds a
	// Windows-compatible DaemonSet (CONTP-1448).
	WindowsProvider = "windows"

	// OpenshiftProvider is the OpenShift Provider name
	OpenshiftProvider = "openshift"

	// OpenShiftProviderLabel is the OpenShift node label used to determine the node's provider
	OpenShiftProviderLabel = "node.openshift.io/os_id"

	// AKSProvider is the Azure Kubernetes Service provider name (mirrors helm's providers.aks).
	AKSProvider = "aks"

	// EKSCloudProvider is the Amazon EKS CloudProvider name
	EKSCloudProvider = "eks"

	// EKSEC2UseHostnameFromFileProvider is the EKS-EC2 provider variant where
	// the agent reads its hostname from the cloud-init instance-id file mounted
	// from the host (mirrors helm's providers.eks.ec2.useHostnameFromFile).
	EKSEC2UseHostnameFromFileProvider = "eks-ec2-use-hostname-from-file"

	// EKSProviderLabel is a common EKS node label containing the AMI ID
	EKSProviderLabel = "eks.amazonaws.com/nodegroup-image"

	// EKS label prefixes for provider detection
	eksLabelPrefix    = "eks.amazonaws.com/"
	eksctlLabelPrefix = "alpha.eksctl.io/"

	// aksLabelPrefix is AKS's reserved node-label prefix (nodes outside AKS
	// cannot set it: https://learn.microsoft.com/en-us/azure/aks/use-labels#reserved-prefixes).
	// kubernetes.azure.com/cluster in particular is applied to every node in an
	// AKS cluster, including virtual (ACI) nodes, so any label under this
	// prefix is a reliable cluster-level AKS signal.
	aksLabelPrefix = "kubernetes.azure.com/"
)

// ProviderValue allowlist
var providerValueAllowlist = map[string]struct{}{
	GKECosType: {},
}

// isEKSProvider checks if a node is an EKS node by looking for EKS-specific labels
func isEKSProvider(labels map[string]string) bool {
	// Check for any eks.amazonaws.com/* or eksctl labels
	for key := range labels {
		if strings.HasPrefix(key, eksLabelPrefix) || strings.HasPrefix(key, eksctlLabelPrefix) {
			return true
		}
	}

	return false
}

// isAKSProvider checks if a node is an AKS node by looking for labels under
// AKS's reserved kubernetes.azure.com/ prefix.
func isAKSProvider(labels map[string]string) bool {
	for key := range labels {
		if strings.HasPrefix(key, aksLabelPrefix) {
			return true
		}
	}

	return false
}

// ShouldUseDefaultDaemonset checks if the provider list contains providers that don't support
// provider-specific daemonsets and should use a single default daemonset without node affinity.
// Currently applies to EKS and OpenShift providers.
func ShouldUseDefaultDaemonset(providerList map[string]struct{}) bool {
	for provider := range providerList {
		// Check for EKS directly (has no suffix)
		if provider == EKSCloudProvider {
			return true
		}
		// Check for OpenShift (has format "openshift-{value}")
		if strings.HasPrefix(provider, OpenshiftProvider+"-") {
			return true
		}
	}
	return false
}

// generateValidProviderName creates a provider name from the cloud provider
// and provider value. NOTE: this should not be used to create a resource name
// as it may contain underscores
func generateValidProviderName(cloudProvider, providerValue string) string {
	// For OpenShift and EKS, accept any value
	if cloudProvider == OpenshiftProvider || cloudProvider == EKSCloudProvider {
		return cloudProvider + "-" + providerValue
	}
	// For other providers (like GKE), check the allowlist
	if isProviderValueAllowed(providerValue) {
		return cloudProvider + "-" + providerValue
	}
	return ""
}

// isProviderValueAllowed returns whether the value of a provider is present
// in the allowlist
func isProviderValueAllowed(value string) bool {
	if _, ok := providerValueAllowlist[value]; ok {
		return true
	}
	return false
}

// GetProviderLabelKeyValue gets the corresponding cloud provider label key and value from a provider name
func GetProviderLabelKeyValue(provider string) (string, string) {
	// For EKS, this returns empty values since EKS provider has no suffix.
	// Use direct comparison (provider == EKSCloudProvider) to check for EKS instead.
	if provider == EKSCloudProvider {
		return "", ""
	}

	// cloud provider to label mapping
	providerMapping := map[string]string{
		GKECloudProvider:  GKEProviderLabel,
		OpenshiftProvider: OpenShiftProviderLabel,
	}

	cp, value := splitProviderSuffix(provider)
	if label, ok := providerMapping[cp]; ok {
		return label, value
	}
	return "", ""
}

// splitProviderSuffix splits a provider suffix into the cloud provider and the provider value
func splitProviderSuffix(provider string) (string, string) {
	splitSuffix := strings.SplitN(provider, "-", 2)
	if len(splitSuffix) != 2 {
		return "", ""
	}
	return splitSuffix[0], splitSuffix[1]
}

// GetAgentNameWithProvider returns the agent name based on the ds name and provider
func GetAgentNameWithProvider(overrideDSName, provider string) string {
	if provider != "" && overrideDSName != "" {
		return overrideDSName + "-" + strings.Replace(provider, "_", "-", -1)
	}
	return overrideDSName
}

// IsSpecificProvider reports whether p is a recognized cloud provider (i.e. not
// empty and not the generic "default").
func IsSpecificProvider(p string) bool {
	return p != "" && p != DefaultProvider
}

// ClusterProviderFromNodeLabels maps a single node's labels to the cluster-level
// provider (eks, aks, openshift-<os>, or default). It is the node-label signal for the
// cluster dimension.
//
// Node-OS distinctions such as gke-cos belong to the node dimension and are
// intentionally NOT returned here, so a node-OS variation doesn't appear as a
// cluster provider. GKE therefore maps to default at cluster scope.
func ClusterProviderFromNodeLabels(labels map[string]string) string {
	if len(labels) > 0 {
		// OpenShift keeps its os_id suffix: control plane monitoring resolves the
		// provider label via GetProviderLabelKeyValue, which needs the openshift-<os> form.
		if val, ok := labels[OpenShiftProviderLabel]; ok {
			return generateValidProviderName(OpenshiftProvider, val)
		}
		if isEKSProvider(labels) {
			return EKSCloudProvider
		}
		if isAKSProvider(labels) {
			return AKSProvider
		}
	}
	return DefaultProvider
}

// GetClusterProviderFromNodeList returns the cluster-level provider for a node
// list, preferring a specific provider over the default (first match wins).
func GetClusterProviderFromNodeList(nodeList []corev1.Node) string {
	for i := range nodeList {
		if provider := ClusterProviderFromNodeLabels(nodeList[i].Labels); provider != DefaultProvider {
			return provider
		}
	}
	return DefaultProvider
}
