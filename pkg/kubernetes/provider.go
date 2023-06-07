// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kubernetes

import (
	"strings"
	"sync"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Profiles struct {
	providers map[Provider]map[string]bool

	mutex sync.Mutex
}

type Provider struct {
	Name string // name of provider, e.g. `cos`
	// https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#dns-subdomain-names
	ComponentName string // suffix to add to component name, e.g. `gcp-cos`
	CloudProvider string // type of cloud provider used
	ProviderLabel string // label used to determine which provider is used
}

const (
	// https://cloud.google.com/kubernetes-engine/docs/concepts/node-images#available_node_images
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
func NewProfiles() Profiles {
	return Profiles{
		providers: make(map[Provider]map[string]bool),
	}
}

func determineProvider(labels map[string]string) Provider {
	p := Provider{}
	// GCP
	if val, ok := labels[GCPProviderLabel]; ok {
		p.Name = val
		p.ComponentName = GCPCloudProvider + "-" + strings.Replace(val, "_", "-", -1)
		p.CloudProvider = GCPCloudProvider
		p.ProviderLabel = GCPProviderLabel
		return p
	}

	return p
}

// SetProvider creates a provider entry for a new provider or adds a node name to an existing provider
func (p *Profiles) SetProvider(obj client.Object) {
	objName := obj.GetName()
	objProvider := determineProvider(obj.GetLabels())

	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.providers == nil {
		p.providers = make(map[Provider]map[string]bool)
	}

	if _, ok := p.providers[objProvider]; !ok {
		p.providers[objProvider] = map[string]bool{
			objName: true,
		}
	} else {
		p.providers[objProvider][objName] = true
	}
}

// DeleteProvider removes a node name from a provider and removes a provider if not used by any nodes
func (p *Profiles) DeleteProvider(obj client.Object) {
	objName := obj.GetName()
	objProvider := determineProvider(obj.GetLabels())

	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.providers != nil || len(p.providers) > 0 {
		if _, ok := p.providers[objProvider]; ok {
			delete(p.providers[objProvider], objName)

			// delete provider if no nodes are using that provider
			if len(p.providers[objProvider]) == 0 {
				delete(p.providers, objProvider)
			}
		}
	}
}

// GetProviders gets a list of providers and the nodes associated with each provider
func (p *Profiles) GetProviders() *map[Provider]map[string]bool {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	return &p.providers
}

// GenerateNodeSelector creates a node selector based on the provider label
func GenerateNodeSelector(p Provider) map[string]string {
	nodeSelector := map[string]string{
		p.ProviderLabel: p.Name,
	}

	return nodeSelector
}
