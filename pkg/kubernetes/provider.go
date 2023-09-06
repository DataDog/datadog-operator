// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kubernetes

import (
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ProfilesOptions defines Profiles options
type ProfilesOptions struct {
	NewNodeDelay time.Duration
}

type Profiles struct {
	providers       map[string]Provider // map provider hash to provider definitions to get provider def with hash string
	sortedProviders []Provider          // sorted list to generate affinity expressions in the same order to prevent pod restarts
	newNodes        map[string]bool     // store node name for nodes that may need more time for cloud provider labels to populate
	newNodesDelay   time.Duration

	log   logr.Logger
	mutex sync.Mutex
}

type Provider struct {
	// Name is the name of provider, e.g. `cos`
	Name string
	// https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#dns-subdomain-names
	// ComponentName is the suffix to add to a component name, e.g. `gcp-cos`
	ComponentName string
	// CloudProvider is the type of cloud provider used
	CloudProvider string
	// ProviderLabel is the label used to determine which provider is used
	ProviderLabel string
}

const (
	DefaultProvider = "default"
	// GCP provider names https://cloud.google.com/kubernetes-engine/docs/concepts/node-images#available_node_images
	GCPCosContainerdProvider         = "cos_containerd"
	GCPCosProvider                   = "cos"
	GCPUbuntuContainerdProvider      = "ubuntu_containerd"
	GCPUbuntuProvider                = "ubuntu"
	GCPWindowsLTSCContainerdProvider = "windows_ltsc_containerd"
	GCPWindowsLTSCProvider           = "windows_ltsc"
	GCPWindowsSACContainerdProvider  = "windows_sac_containerd"
	GCPWindowsSACProvider            = "windows_sac"

	// CloudProvider
	GCPCloudProvider   = "gcp"
	AWSCloudProvider   = "aws"
	AzureCloudProvider = "azure"

	// ProviderLabel
	GCPProviderLabel = "cloud.google.com/gke-os-distribution"
)

// NewProfiles generates an empty Profiles instance
func NewProfiles(log logr.Logger, options ProfilesOptions) Profiles {
	return Profiles{
		providers:       make(map[string]Provider),
		sortedProviders: []Provider{},
		newNodes:        make(map[string]bool),
		newNodesDelay:   options.NewNodeDelay,
		log:             log,
	}
}

// DetermineProvider creates a Provider based on a map of labels
func DetermineProvider(labels map[string]string) Provider {
	p := Provider{}
	if len(labels) > 0 {
		// GCP
		if val, ok := labels[GCPProviderLabel]; ok {
			p.Name = val
			p.CloudProvider = GCPCloudProvider
			p.ProviderLabel = GCPProviderLabel
			p.ComponentName = GenerateComponentName(p.CloudProvider, p.Name)
			return p
		}
	}

	// default Provider if a match was not found
	p.Name = DefaultProvider
	p.CloudProvider = DefaultProvider
	p.ProviderLabel = DefaultProvider
	p.ComponentName = DefaultProvider

	return p
}

// SetProvider creates a provider entry for a new provider if needed and returns whether DDAs should be reconciled
func (p *Profiles) SetProvider(obj client.Object) bool {
	objName := obj.GetName()
	// cloud provider labels may not be populated when a node first starts up and is sent via informer
	// if nodes are <newNodesDelay seconds old, don't check for a provider and add to newNodes instead
	// SetProvider is called in the reconcile loop to ensure nodes aren't missed once they are old enough
	creationTime := obj.GetCreationTimestamp().Time
	if time.Since(creationTime) < p.newNodesDelay {
		p.log.Info("Node is too new to have its provider evaluated", "node", objName, "node agent", time.Since(creationTime), "minimum age", p.newNodesDelay)
		p.mutex.Lock()
		p.newNodes[objName] = true
		p.mutex.Unlock()
		return false
	}
	delete(p.newNodes, objName)

	labels := obj.GetLabels()
	objProvider := DetermineProvider(labels)
	providerHash, err := GenerateProviderHash(objProvider)
	if err != nil {
		p.log.Error(err, "Error generating hash for node provider", "node", objName, "provider", objProvider.ComponentName)
		return false
	}

	p.mutex.Lock()
	defer p.mutex.Unlock()
	// add a new provider hash and provider definition
	if _, ok := p.providers[providerHash]; !ok {
		p.providers[providerHash] = objProvider
		p.sortProviders()
		p.log.Info("New provider detected", "provider", objProvider.ComponentName, "hash", providerHash)

		// reconcile if a new provider was added
		return true
	}

	return false
}

// GetProviders gets a list of providers
func (p *Profiles) GetProviders() *map[string]Provider {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	return &p.providers
}

// GetNewNodes gets a list of nodes that had not passed their newNodeDelay time
func (p *Profiles) GetNewNodes() *map[string]bool {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	return &p.newNodes
}

// GenerateProviderNodeAffinity creates NodeSelectorTerms based on the provider
func (p *Profiles) GenerateProviderNodeAffinity(provider Provider) []corev1.NodeSelectorRequirement {
	nsrList := []corev1.NodeSelectorRequirement{}
	// default provider has NodeAffinity to NOT match provider-specific labels
	if provider.Name == DefaultProvider {
		for _, providerDef := range p.sortedProviders {
			if providerDef.Name == DefaultProvider {
				continue
			}
			nsrList = append(nsrList, corev1.NodeSelectorRequirement{
				Key:      providerDef.ProviderLabel,
				Operator: corev1.NodeSelectorOpNotIn,
				Values: []string{
					providerDef.Name,
				},
			})
		}
		return nsrList
	}
	// create provider-specific NodeSelectorTerm for NodeAffinity
	nsrList = append(nsrList, corev1.NodeSelectorRequirement{
		Key:      provider.ProviderLabel,
		Operator: corev1.NodeSelectorOpIn,
		Values: []string{
			provider.Name,
		},
	})

	return nsrList
}

// IsProviderInProfiles returns whether a provider exists in profiles
func (p *Profiles) IsProviderInProfiles(hash string) bool {
	if _, ok := p.providers[hash]; ok {
		return true
	}
	return false
}

func (p *Profiles) sortProviders() {
	// needed to generate NodeSelectorRequirements for NodeAffinity in a consistent order
	// otherwise the order may change each reconcile, causing many pod restarts
	p.sortedProviders = make([]Provider, 0, len(p.providers))
	for _, provider := range p.providers {
		p.sortedProviders = append(p.sortedProviders, provider)
	}
	sort.Slice(p.sortedProviders, func(i, j int) bool {
		return p.sortedProviders[i].Name < p.sortedProviders[j].Name
	})
}

// GenerateComponentName creates a ComponentName from the provider fields
func GenerateComponentName(cloudProvider, providerName string) string {
	return cloudProvider + "-" + strings.Replace(providerName, "_", "-", -1)
}

// GenerateProviderHash creates a md5 hash to identify a provider with
func GenerateProviderHash(provider Provider) (string, error) {
	providerHash, err := comparison.GenerateMD5ForSpec(provider.ComponentName)
	if err != nil {
		return "", err
	}
	return providerHash, nil
}
