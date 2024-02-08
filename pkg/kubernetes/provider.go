// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kubernetes

import (
	"sort"
	"strings"
	"sync"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
)

type ProviderStore struct {
	providers map[string]struct{}

	log   logr.Logger
	mutex sync.Mutex
}

const (
	// LegacyProvider Legacy Provider (empty name)
	LegacyProvider = ""
	// DefaultProvider Default provider name
	DefaultProvider = "default"

	// GKECosType GKE provider types: https://cloud.google.com/kubernetes-engine/docs/concepts/node-images#available_node_images
	// Default "cos" node runtime is cos_containerd in GKE v1.24+
	GKECosType = "cos"

	// GKECloudProvider GKE CloudProvider name
	GKECloudProvider = "gke"

	// GKEProviderLabel GKE ProviderLabel
	GKEProviderLabel = "cloud.google.com/gke-os-distribution"
)

// ProviderValue allowlist
var providerValueAllowlist = map[string]struct{}{
	GKECosType: {},
}

// NewProviderStore generates an empty ProviderStore instance
func NewProviderStore(log logr.Logger) ProviderStore {
	return ProviderStore{
		providers: make(map[string]struct{}),
		log:       log,
	}
}

// DetermineProvider creates a Provider based on a map of labels
func DetermineProvider(labels map[string]string) string {
	if len(labels) > 0 {
		// GKE
		if val, ok := labels[GKEProviderLabel]; ok {
			if provider := generateValidProviderName(GKECloudProvider, val); provider != "" {
				return provider
			}
		}
	}

	return DefaultProvider
}

// GetProviders gets a list of providers
func (p *ProviderStore) GetProviders() *map[string]struct{} {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	return &p.providers
}

// GenerateProviderNodeAffinity creates NodeSelectorTerms based on the provider
func (p *ProviderStore) GenerateProviderNodeAffinity(provider string) []corev1.NodeSelectorRequirement {
	// default provider has NodeAffinity to NOT match provider-specific labels
	// build NodeSelectorRequirement list with negative (`OpNotIn`) operator
	nsrList := []corev1.NodeSelectorRequirement{}
	if provider == DefaultProvider {
		// sort providers to get consistently ordered affinity
		p.mutex.Lock()
		sortedProviders := sortProviders(p.providers)
		p.mutex.Unlock()

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
		return nsrList
	}
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

	return nsrList
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

// Reset overwrites all providers in the provider store given a list of providers
func (p *ProviderStore) Reset(providersList map[string]struct{}) map[string]struct{} {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if len(providersList) > 0 {
		p.providers = providersList
	}

	return p.providers
}

// IsPresent returns whether the given provider exists in the provider store
func (p *ProviderStore) IsPresent(provider string) bool {
	if provider == "" {
		return false
	}
	p.mutex.Lock()
	defer p.mutex.Unlock()
	if len(p.providers) == 0 {
		return false
	}
	if _, ok := p.providers[provider]; ok {
		return true
	}

	return false
}

// ComponentOverrideFromProvider generates a componentOverride with an override
// for a provider-specific agent name
func ComponentOverrideFromProvider(daemonSetName string, provider string) v2alpha1.DatadogAgentComponentOverride {
	componentOverride := v2alpha1.DatadogAgentComponentOverride{}
	overrideAgentName := GetAgentNameWithProvider(daemonSetName, provider, componentOverride.Name)
	componentOverride.Name = &overrideAgentName
	return componentOverride
}

// GetAgentNameWithProvider returns the agent name based on the ds name,
// provider, and component override settings
func GetAgentNameWithProvider(dsName, provider string, overrideName *string) string {
	baseName := dsName
	if overrideName != nil && *overrideName != "" {
		baseName = *overrideName
	}

	if provider != "" && baseName != "" {
		return baseName + "-" + strings.Replace(provider, "_", "-", -1)
	}
	return baseName
}
