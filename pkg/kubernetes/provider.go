// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kubernetes

import (
	"sort"
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/pkg/constants"
)

const (
	// LegacyProvider Legacy Provider (empty name)
	LegacyProvider = ""
	// DefaultProvider Default provider name
	DefaultProvider = "default"

	// GKE provider types: https://cloud.google.com/kubernetes-engine/docs/concepts/node-images#available_node_images
	// GKECosType is the Container-Optimized OS node image offered by GKE
	GKECosType = "cos"

	// GKECloudProvider GKE CloudProvider name
	GKECloudProvider = "gke"

	// GKEProviderLabel is the GKE node label used to determine the node's provider
	GKEProviderLabel = "cloud.google.com/gke-os-distribution"

	// OpenshiftProvider is the OpenShift Provider name
	OpenshiftProvider = "openshift"

	// OpenShiftProviderLabel is the OpenShift node label used to determine the node's provider
	OpenShiftProviderLabel = "node.openshift.io/os_id"

	// EKSCloudProvider is the Amazon EKS CloudProvider name
	EKSCloudProvider = "eks"

	// EKSProviderLabel is a common EKS node label containing the AMI ID
	EKSProviderLabel = "eks.amazonaws.com/nodegroup-image"

	// EKS label prefixes for provider detection
	eksLabelPrefix    = "eks.amazonaws.com/"
	eksctlLabelPrefix = "alpha.eksctl.io/"

	// Common EKS labels used for node affinity
	eksNodeGroupLabel      = "eks.amazonaws.com/nodegroup"
	eksComputeTypeLabel    = "eks.amazonaws.com/compute-type"
	eksctlClusterNameLabel = "alpha.eksctl.io/cluster-name"
)

// ProviderValue allowlist
var providerValueAllowlist = map[string]struct{}{
	GKECosType: {},
}

// determineProvider creates a Provider based on a map of labels
func determineProvider(labels map[string]string) string {
	if len(labels) > 0 {
		// GKE
		if val, ok := labels[GKEProviderLabel]; ok {
			if provider := generateValidProviderName(GKECloudProvider, val); provider != "" {
				return provider
			}
		}
		// Openshift
		if val, ok := labels[OpenShiftProviderLabel]; ok {
			return generateValidProviderName(OpenshiftProvider, val)
		}
		// EKS - check for any EKS-related labels
		if isEKSProvider(labels) {
			return EKSCloudProvider
		}
	}

	return DefaultProvider
}

// isEKSProvider checks if a node is an EKS node by looking for EKS-specific labels
func isEKSProvider(labels map[string]string) bool {
	// Check for any eks.amazonaws.com/* labels
	for key := range labels {
		if strings.HasPrefix(key, eksLabelPrefix) {
			return true
		}
	}

	// Secondary check for eksctl labels
	for key := range labels {
		if strings.HasPrefix(key, eksctlLabelPrefix) {
			return true
		}
	}

	return false
}

// getProviderNodeAffinity creates NodeSelectorTerms based on the provider
func getProviderNodeAffinity(provider string, providerList map[string]struct{}) *corev1.Affinity {
	if provider == "" || providerList == nil || len(providerList) == 0 {
		return nil
	}
	// if only the default provider exists, there should be no affinity override
	if provider == DefaultProvider && len(providerList) == 1 {
		return nil
	}

	// Special handling for EKS provider
	if provider == EKSCloudProvider {
		return getEKSProviderNodeAffinity()
	}

	// default provider has NodeAffinity to NOT match provider-specific labels
	// build NodeSelectorRequirement list with negative (`OpNotIn`) operator
	nsrList := []corev1.NodeSelectorRequirement{}
	if provider == DefaultProvider {
		// sort providers to get consistently ordered affinity
		sortedProviders := sortProviders(providerList)
		for _, providerDef := range sortedProviders {
			// Special handling for EKS in the default provider's exclusion list
			if providerDef == EKSCloudProvider {
				nsrList = append(nsrList, getEKSExclusionNodeSelectorRequirements()...)
				continue
			}

			key, value := GetProviderLabelKeyValue(providerDef)
			if key != "" && value != "" {
				nsrList = append(nsrList, corev1.NodeSelectorRequirement{
					Key:      key,
					Operator: corev1.NodeSelectorOpNotIn,
					Values: []string{
						value,
					},
				})
			}
		}
	} else {
		// create provider-specific NodeSelectorTerm for NodeAffinity
		key, value := GetProviderLabelKeyValue(provider)
		if key != "" && value != "" {
			nsrList = append(nsrList, corev1.NodeSelectorRequirement{
				Key:      key,
				Operator: corev1.NodeSelectorOpIn,
				Values: []string{
					value,
				},
			})
		}
	}

	return &corev1.Affinity{
		NodeAffinity: &corev1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
				NodeSelectorTerms: []corev1.NodeSelectorTerm{
					{
						MatchExpressions: nsrList,
					},
				},
			},
		},
	}
}

// getEKSProviderNodeAffinity creates node affinity for EKS provider
// Uses multiple NodeSelectorTerms with OR logic to match any EKS node
func getEKSProviderNodeAffinity() *corev1.Affinity {
	// Create multiple NodeSelectorTerms (OR logic between terms)
	// Each term checks for existence of a different EKS label
	nodeSelectorTerms := []corev1.NodeSelectorTerm{
		{
			MatchExpressions: []corev1.NodeSelectorRequirement{
				{
					Key:      eksNodeGroupLabel,
					Operator: corev1.NodeSelectorOpExists,
				},
			},
		},
		{
			MatchExpressions: []corev1.NodeSelectorRequirement{
				{
					Key:      EKSProviderLabel,
					Operator: corev1.NodeSelectorOpExists,
				},
			},
		},
		{
			MatchExpressions: []corev1.NodeSelectorRequirement{
				{
					Key:      eksComputeTypeLabel,
					Operator: corev1.NodeSelectorOpExists,
				},
			},
		},
		{
			MatchExpressions: []corev1.NodeSelectorRequirement{
				{
					Key:      eksctlClusterNameLabel,
					Operator: corev1.NodeSelectorOpExists,
				},
			},
		},
	}

	return &corev1.Affinity{
		NodeAffinity: &corev1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
				NodeSelectorTerms: nodeSelectorTerms,
			},
		},
	}
}

// getEKSExclusionNodeSelectorRequirements creates node selector requirements
// to exclude EKS nodes from the default provider
func getEKSExclusionNodeSelectorRequirements() []corev1.NodeSelectorRequirement {
	return []corev1.NodeSelectorRequirement{
		{
			Key:      eksNodeGroupLabel,
			Operator: corev1.NodeSelectorOpDoesNotExist,
		},
		{
			Key:      EKSProviderLabel,
			Operator: corev1.NodeSelectorOpDoesNotExist,
		},
		{
			Key:      eksComputeTypeLabel,
			Operator: corev1.NodeSelectorOpDoesNotExist,
		},
		{
			Key:      eksctlClusterNameLabel,
			Operator: corev1.NodeSelectorOpDoesNotExist,
		},
	}
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

// sortProviders sorts a map of providers to get a consistently ordered list to create affinity requirements
func sortProviders(providers map[string]struct{}) []string {
	sortedProviders := make([]string, 0, len(providers))
	for provider := range providers {
		sortedProviders = append(sortedProviders, provider)
	}
	sort.Strings(sortedProviders)

	return sortedProviders
}

// ComponentOverrideFromProvider generates a componentOverride with overrides for
// the DatadogAgent name, provider, and label
func ComponentOverrideFromProvider(overrideName, provider string, providerList map[string]struct{}) v2alpha1.DatadogAgentComponentOverride {
	overrideDSName := GetAgentNameWithProvider(overrideName, provider)
	return v2alpha1.DatadogAgentComponentOverride{
		Name:     &overrideDSName,
		Affinity: getProviderNodeAffinity(provider, providerList),
		Labels:   map[string]string{constants.MD5AgentDeploymentProviderLabelKey: provider},
	}
}

// GetAgentNameWithProvider returns the agent name based on the ds name and provider
func GetAgentNameWithProvider(overrideDSName, provider string) string {
	if provider != "" && overrideDSName != "" {
		return overrideDSName + "-" + strings.Replace(provider, "_", "-", -1)
	}
	return overrideDSName
}

// GetProviderListFromNodeList generates a list of providers given a list of nodes
func GetProviderListFromNodeList(nodeList []corev1.Node, logger logr.Logger) map[string]struct{} {
	providerList := make(map[string]struct{})
	for _, node := range nodeList {
		provider := determineProvider(node.Labels)
		if _, ok := providerList[provider]; !ok {
			providerList[provider] = struct{}{}
			logger.V(1).Info("New provider detected", "provider", provider)
		}
	}
	return providerList
}
