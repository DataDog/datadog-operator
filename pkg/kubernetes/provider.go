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

	TalosProvider = "talos"

	// GKE provider types: https://cloud.google.com/kubernetes-engine/docs/concepts/node-images#available_node_images
	// GKECosType is the Container-Optimized OS node image offered by GKE
	GKECosType = "cos"

	// GKECloudProvider GKE CloudProvider name
	GKECloudProvider = "gke"

	// GKEProviderLabel is the GKE node label used to determine the node's provider
	GKEProviderLabel = "cloud.google.com/gke-os-distribution"
)

// ProviderValue allowlist
var providerValueAllowlist = map[string]struct{}{
	GKECosType: {},
}

// determineProvider creates a Provider based on a map of labels
func determineProvider(node *corev1.Node) string {
	if len(node.Labels) > 0 {
		// GKE
		if val, ok := node.Labels[GKEProviderLabel]; ok {
			if provider := generateValidProviderName(GKECloudProvider, val); provider != "" {
				return provider
			}
		}
	}

	if strings.Contains(node.Status.NodeInfo.OSImage, "Talos") {
		return TalosProvider
	}

	return DefaultProvider
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

	// default provider has NodeAffinity to NOT match provider-specific labels
	// build NodeSelectorRequirement list with negative (`OpNotIn`) operator
	nsrList := []corev1.NodeSelectorRequirement{}
	if provider == DefaultProvider {
		// sort providers to get consistently ordered affinity
		sortedProviders := sortProviders(providerList)
		for _, providerDef := range sortedProviders {
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

// generateValidProviderName creates a provider name from the cloud provider
// and provider value. NOTE: this should not be used to create a resource name
// as it may contain underscores
func generateValidProviderName(cloudProvider, providerValue string) string {
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
	// cloud provider to label mapping
	providerMapping := map[string]string{
		GKECloudProvider: GKEProviderLabel,
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
		provider := determineProvider(&node)
		if _, ok := providerList[provider]; !ok {
			providerList[provider] = struct{}{}
			logger.V(1).Info("New provider detected", "provider", provider)
		}
	}
	return providerList
}
