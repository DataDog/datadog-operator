// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kubernetes

import (
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ProviderStore struct {
	providers map[string]struct{}

	log   logr.Logger
	mutex sync.Mutex
}

const (
	DefaultProvider = "default"
	// GCP provider values https://cloud.google.com/kubernetes-engine/docs/concepts/node-images#available_node_images
	GCPCosContainerdProviderValue = "cos_containerd"
	GCPCosProviderValue           = "cos"
	GKEAutopilotProviderValue     = "autopilot"

	// CloudProvider
	GCPCloudProvider     = "gcp"
	GKEAutopilotProvider = "gke"

	// ProviderLabel
	GCPProviderLabel          = "cloud.google.com/gke-os-distribution"
	GKEAutopilotProviderLabel = "kubernetes.io/hostname"
)

// NewProviderStore generates an empty ProviderStore instance
func NewProviderStore(log logr.Logger) ProviderStore {
	return ProviderStore{
		providers: make(map[string]struct{}),
		log:       log,
	}
}

// determineProvider creates a Provider based on a map of labels
func determineProvider(labels map[string]string) string {
	if len(labels) > 0 {
		// GCP
		if val, ok := labels[GCPProviderLabel]; ok {
			//GKE
			hostname := labels[GKEAutopilotProviderLabel]
			var autopilotRegex = regexp.MustCompile(`^gk3\-.*`)

			//return boolean
			if autopilotRegex.MatchString(hostname) {
				AutopilotProvider := generateProviderName(GKEAutopilotProvider, GKEAutopilotProviderValue)
				return AutopilotProvider
			} else {
				ProviderName := generateProviderName(GCPCloudProvider, val)
				return ProviderName
			}
		}
	}
	return DefaultProvider
}

// SetProvider creates a provider entry for a new provider if needed
func (p *ProviderStore) SetProvider(obj client.Object) {
	labels := obj.GetLabels()
	objProvider := determineProvider(labels)

	p.mutex.Lock()
	defer p.mutex.Unlock()
	// add a new provider hash and provider definition
	if _, ok := p.providers[objProvider]; !ok {
		p.providers[objProvider] = struct{}{}
		p.log.Info("New provider detected", "provider", objProvider)
	}
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

// generateProviderName creates a provider name from the cloud provider and provider value
// this should not be used to create a resource name as it may contain underscores
func generateProviderName(cloudProvider, providerValue string) string {
	return cloudProvider + "-" + providerValue
}

// GetProviderLabelKeyValue gets the corresponding cloud provider label key and value from a provider name
func GetProviderLabelKeyValue(provider string) (string, string) {
	// cloud provider to label mapping
	providerMapping := map[string]string{
		GCPCloudProvider: GCPProviderLabel,
		GKEAutopilotProvider: GKEAutopilotProviderLabel,
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
