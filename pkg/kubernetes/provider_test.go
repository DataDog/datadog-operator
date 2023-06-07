// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kubernetes

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_determineProvider(t *testing.T) {
	tests := []struct {
		name     string
		labels   map[string]string
		provider Provider
	}{
		{
			name: "no provider",
			labels: map[string]string{
				"foo": "bar",
			},
			provider: Provider{},
		},
		{
			name: "gcp provider",
			labels: map[string]string{
				"foo":            "bar",
				GCPProviderLabel: "cos",
			},
			provider: Provider{
				Name:          "cos",
				ComponentName: "gcp-cos",
				CloudProvider: GCPCloudProvider,
				ProviderLabel: GCPProviderLabel,
			},
		},
		{
			name: "gcp provider, underscore",
			labels: map[string]string{
				"foo":            "bar",
				GCPProviderLabel: GCPWindowsSACContainerdProvider,
			},
			provider: Provider{
				Name:          GCPWindowsSACContainerdProvider,
				ComponentName: "gcp-windows-sac-containerd",
				CloudProvider: GCPCloudProvider,
				ProviderLabel: GCPProviderLabel,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := determineProvider(tt.labels)
			assert.Equal(t, tt.provider, p)
		})
	}
}
func Test_SetProvider(t *testing.T) {
	dummyNode1 := corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "bar",
			Name:      "node-1",
			Labels: map[string]string{
				GCPProviderLabel: GCPCosProvider,
			},
		},
	}

	dummyProfile := Profiles{}

	dummyProvider := Provider{
		Name: "foo-provider",
	}

	tests := []struct {
		name              string
		obj               corev1.Node
		existingProviders *Profiles
		wantProviders     *Profiles
	}{
		{
			name:              "add new provider",
			obj:               dummyNode1,
			existingProviders: &dummyProfile,
			wantProviders: &Profiles{
				providers: map[Provider]map[string]bool{
					{
						Name:          GCPCosProvider,
						ComponentName: "gcp-cos",
						CloudProvider: GCPCloudProvider,
						ProviderLabel: GCPProviderLabel,
					}: {
						"node-1": true,
					},
				},
			},
		},
		{
			name: "add new provider with existing provider",
			obj:  dummyNode1,
			existingProviders: &Profiles{
				providers: map[Provider]map[string]bool{
					dummyProvider: {
						"node-2": true,
					},
				},
			},
			wantProviders: &Profiles{
				providers: map[Provider]map[string]bool{
					dummyProvider: {
						"node-2": true,
					},
					{
						Name:          GCPCosProvider,
						ComponentName: "gcp-cos",
						CloudProvider: GCPCloudProvider,
						ProviderLabel: GCPProviderLabel,
					}: {
						"node-1": true,
					},
				},
			},
		},
		{
			name: "add new node name to existing provider",
			obj:  dummyNode1,
			existingProviders: &Profiles{
				providers: map[Provider]map[string]bool{
					{
						Name:          GCPCosProvider,
						ComponentName: "gcp-cos",
						CloudProvider: GCPCloudProvider,
						ProviderLabel: GCPProviderLabel,
					}: {
						"node-2": true,
					},
				},
			},
			wantProviders: &Profiles{
				providers: map[Provider]map[string]bool{
					{
						Name:          GCPCosProvider,
						ComponentName: "gcp-cos",
						CloudProvider: GCPCloudProvider,
						ProviderLabel: GCPProviderLabel,
					}: {
						"node-1": true,
						"node-2": true,
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.existingProviders.SetProvider(&tt.obj)
			assert.Equal(t, tt.wantProviders, tt.existingProviders)
		})
	}
}
func Test_DeleteProvider(t *testing.T) {
	dummyNode1 := corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "bar",
			Name:      "node-1",
			Labels: map[string]string{
				GCPProviderLabel: GCPCosProvider,
			},
		},
	}

	dummyProfile := Profiles{}

	dummyProvider := Provider{
		Name: "foo-provider",
	}

	tests := []struct {
		name              string
		obj               corev1.Node
		existingProviders *Profiles
		wantProviders     *Profiles
	}{
		{
			name:              "no existing providers",
			obj:               dummyNode1,
			existingProviders: &dummyProfile,
			wantProviders:     &Profiles{},
		},
		{
			name: "delete node in provider",
			obj:  dummyNode1,
			existingProviders: &Profiles{
				providers: map[Provider]map[string]bool{
					{
						Name:          GCPCosProvider,
						ComponentName: "gcp-cos",
						CloudProvider: GCPCloudProvider,
						ProviderLabel: GCPProviderLabel,
					}: {
						"node-1": true,
						"node-2": true,
					},
				},
			},
			wantProviders: &Profiles{
				providers: map[Provider]map[string]bool{
					{
						Name:          GCPCosProvider,
						ComponentName: "gcp-cos",
						CloudProvider: GCPCloudProvider,
						ProviderLabel: GCPProviderLabel,
					}: {
						"node-2": true,
					},
				},
			},
		},
		{
			name: "delete only node in provider",
			obj:  dummyNode1,
			existingProviders: &Profiles{
				providers: map[Provider]map[string]bool{
					{
						Name:          GCPCosProvider,
						ComponentName: "gcp-cos",
						CloudProvider: GCPCloudProvider,
						ProviderLabel: GCPProviderLabel,
					}: {
						"node-1": true,
					},
				},
			},
			wantProviders: &Profiles{
				providers: map[Provider]map[string]bool{},
			},
		},
		{
			name: "delete nonexistent provider",
			obj:  dummyNode1,
			existingProviders: &Profiles{
				providers: map[Provider]map[string]bool{
					dummyProvider: {
						"node-2": true,
					},
				},
			},
			wantProviders: &Profiles{
				providers: map[Provider]map[string]bool{
					dummyProvider: {
						"node-2": true,
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.existingProviders.DeleteProvider(&tt.obj)
			assert.Equal(t, tt.wantProviders, tt.existingProviders)
		})
	}
}
