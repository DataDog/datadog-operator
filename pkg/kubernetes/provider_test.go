// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kubernetes

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var (
	gcpCosNode = corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "bar",
			Name:      "node1",
			Labels: map[string]string{
				GCPProviderLabel: GCPCosProvider,
			},
		},
	}
	defaultProvider = Provider{
		Name:          DefaultProvider,
		ComponentName: DefaultProvider,
		CloudProvider: DefaultProvider,
		ProviderLabel: DefaultProvider,
	}
	gcpUbuntuProvider = Provider{
		Name:          GCPUbuntuProvider,
		ComponentName: GenerateComponentName(GCPCloudProvider, GCPUbuntuProvider),
		CloudProvider: GCPCloudProvider,
		ProviderLabel: GCPProviderLabel,
	}
)

func Test_determineProvider(t *testing.T) {
	tests := []struct {
		name     string
		labels   map[string]string
		provider Provider
	}{
		{
			name: "random provider",
			labels: map[string]string{
				"foo": "bar",
			},
			provider: defaultProvider,
		},
		{
			name:     "empty labels",
			labels:   map[string]string{},
			provider: defaultProvider,
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
			p := DetermineProvider(tt.labels)
			assert.Equal(t, tt.provider, p)
		})
	}
}
func Test_SetProvider(t *testing.T) {
	tests := []struct {
		name             string
		obj              corev1.Node
		existingProfiles *Profiles
		wantProfile      *Profiles
	}{
		{
			name:             "add new provider",
			obj:              gcpCosNode,
			existingProfiles: nil,
			wantProfile: &Profiles{
				providers: map[string]Provider{
					"da0aef62ac96d9074f4fe800a72a34e8": {
						Name:          GCPCosProvider,
						ComponentName: "gcp-cos",
						CloudProvider: GCPCloudProvider,
						ProviderLabel: GCPProviderLabel,
					},
				},
			},
		},
		{
			name: "add new provider with existing provider",
			obj:  gcpCosNode,
			existingProfiles: &Profiles{
				providers: map[string]Provider{
					"abcdef": {
						Name:          "test2",
						ComponentName: "test2",
					},
				},
			},
			wantProfile: &Profiles{
				providers: map[string]Provider{
					"abcdef": {
						Name:          "test2",
						ComponentName: "test2",
					},
					"da0aef62ac96d9074f4fe800a72a34e8": {
						Name:          GCPCosProvider,
						ComponentName: "gcp-cos",
						CloudProvider: GCPCloudProvider,
						ProviderLabel: GCPProviderLabel,
					},
				},
			},
		},
		{
			name: "add new node name to existing provider",
			obj:  gcpCosNode,
			existingProfiles: &Profiles{
				providers: map[string]Provider{
					"da0aef62ac96d9074f4fe800a72a34e8": {
						Name:          GCPCosProvider,
						ComponentName: "gcp-cos",
						CloudProvider: GCPCloudProvider,
						ProviderLabel: GCPProviderLabel,
					},
				},
			},
			wantProfile: &Profiles{
				providers: map[string]Provider{
					"da0aef62ac96d9074f4fe800a72a34e8": {
						Name:          GCPCosProvider,
						ComponentName: "gcp-cos",
						CloudProvider: GCPCloudProvider,
						ProviderLabel: GCPProviderLabel,
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := logf.Log.WithName(t.Name())
			profile := NewProfiles(logger, ProfilesOptions{NewNodeDelay: 5 * time.Second})
			if tt.existingProfiles != nil && tt.existingProfiles.providers != nil {
				profile.providers = tt.existingProfiles.providers
			}

			profile.SetProvider(&tt.obj)
			assert.Equal(t, tt.wantProfile.providers, profile.providers)
		})
	}
}

func Test_sortProviders(t *testing.T) {
	tests := []struct {
		name                string
		existingProviders   map[string]Provider
		wantSortedProviders []Provider
	}{
		{
			name:                "empty providers",
			existingProviders:   nil,
			wantSortedProviders: []Provider{},
		},
		{
			name: "one provider",
			existingProviders: map[string]Provider{
				"da0aef62ac96d9074f4fe800a72a34e8": {
					Name:          GCPCosProvider,
					ComponentName: "gcp-cos",
					CloudProvider: GCPCloudProvider,
					ProviderLabel: GCPProviderLabel,
				},
			},
			wantSortedProviders: []Provider{
				{
					Name:          GCPCosProvider,
					ComponentName: "gcp-cos",
					CloudProvider: GCPCloudProvider,
					ProviderLabel: GCPProviderLabel,
				},
			},
		},
		{
			name: "multiple providers",
			existingProviders: map[string]Provider{
				"da0aef62ac96d9074f4fe800a72a34e8": {
					Name:          GCPCosProvider,
					ComponentName: "gcp-cos",
					CloudProvider: GCPCloudProvider,
					ProviderLabel: GCPProviderLabel,
				},
				"abcdef": {
					Name:          "test2",
					ComponentName: "test2",
				},
				"lmnop": {
					Name:          "foo",
					ComponentName: "foo-bar",
				},
				"12345": {
					Name:          "defgh",
					ComponentName: "defgh",
				},
			},
			wantSortedProviders: []Provider{
				{
					Name:          GCPCosProvider,
					ComponentName: "gcp-cos",
					CloudProvider: GCPCloudProvider,
					ProviderLabel: GCPProviderLabel,
				},
				{
					Name:          "defgh",
					ComponentName: "defgh",
				},
				{
					Name:          "foo",
					ComponentName: "foo-bar",
				},
				{
					Name:          "test2",
					ComponentName: "test2",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := logf.Log.WithName(t.Name())
			profile := NewProfiles(logger, ProfilesOptions{NewNodeDelay: 5 * time.Second})
			if tt.existingProviders != nil {
				profile.providers = tt.existingProviders
			}

			profile.sortProviders()
			assert.Equal(t, tt.wantSortedProviders, profile.sortedProviders)
		})
	}
}

func Test_GenerateProviderNodeAffinity(t *testing.T) {
	tests := []struct {
		name              string
		existingProviders map[string]Provider
		provider          Provider
		wantNSR           []corev1.NodeSelectorRequirement
	}{
		{
			name:              "empty providers",
			existingProviders: nil,
			provider:          defaultProvider,
			wantNSR:           []corev1.NodeSelectorRequirement{},
		},
		{
			name: "one existing provider, default provider",
			existingProviders: map[string]Provider{
				"da0aef62ac96d9074f4fe800a72a34e8": {
					Name:          GCPCosProvider,
					ComponentName: "gcp-cos",
					CloudProvider: GCPCloudProvider,
					ProviderLabel: GCPProviderLabel,
				},
			},
			provider: defaultProvider,
			wantNSR: []corev1.NodeSelectorRequirement{
				{
					Key:      GCPProviderLabel,
					Operator: corev1.NodeSelectorOpNotIn,
					Values: []string{
						GCPCosProvider,
					},
				},
			},
		},
		{
			name: "one existing provider, ubuntu provider",
			existingProviders: map[string]Provider{
				"da0aef62ac96d9074f4fe800a72a34e8": {
					Name:          GCPCosProvider,
					ComponentName: "gcp-cos",
					CloudProvider: GCPCloudProvider,
					ProviderLabel: GCPProviderLabel,
				},
			},
			provider: gcpUbuntuProvider,
			wantNSR: []corev1.NodeSelectorRequirement{
				{
					Key:      GCPProviderLabel,
					Operator: corev1.NodeSelectorOpIn,
					Values: []string{
						GCPUbuntuProvider,
					},
				},
			},
		},
		{
			name: "multiple providers, default provider",
			existingProviders: map[string]Provider{
				"da0aef62ac96d9074f4fe800a72a34e8": {
					Name:          GCPCosProvider,
					ComponentName: "gcp-cos",
					CloudProvider: GCPCloudProvider,
					ProviderLabel: GCPProviderLabel,
				},
				"abcdef": {
					Name:          "test2",
					ComponentName: "test2",
					ProviderLabel: "test2",
				},
				"lmnop": {
					Name:          "foo",
					ComponentName: "foo-bar",
					ProviderLabel: "foo",
				},
			},
			provider: defaultProvider,
			wantNSR: []corev1.NodeSelectorRequirement{
				{
					Key:      GCPProviderLabel,
					Operator: corev1.NodeSelectorOpNotIn,
					Values: []string{
						GCPCosProvider,
					},
				},
				{
					Key:      "foo",
					Operator: corev1.NodeSelectorOpNotIn,
					Values: []string{
						"foo",
					},
				},
				{
					Key:      "test2",
					Operator: corev1.NodeSelectorOpNotIn,
					Values: []string{
						"test2",
					},
				},
			},
		},
		{
			name: "multiple providers, ubuntu provider",
			existingProviders: map[string]Provider{
				"da0aef62ac96d9074f4fe800a72a34e8": {
					Name:          GCPCosProvider,
					ComponentName: "gcp-cos",
					CloudProvider: GCPCloudProvider,
					ProviderLabel: GCPProviderLabel,
				},
				"abcdef": {
					Name:          "test2",
					ComponentName: "test2",
					ProviderLabel: "test2",
				},
				"lmnop": {
					Name:          "foo",
					ComponentName: "foo-bar",
					ProviderLabel: "foo",
				},
			},
			provider: gcpUbuntuProvider,
			wantNSR: []corev1.NodeSelectorRequirement{
				{
					Key:      GCPProviderLabel,
					Operator: corev1.NodeSelectorOpIn,
					Values: []string{
						GCPUbuntuProvider,
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := logf.Log.WithName(t.Name())
			profile := NewProfiles(logger, ProfilesOptions{NewNodeDelay: 5 * time.Second})
			if tt.existingProviders != nil {
				profile.providers = tt.existingProviders
			}
			profile.sortProviders()

			nsr := profile.GenerateProviderNodeAffinity(tt.provider)
			assert.Equal(t, tt.wantNSR, nsr)
		})
	}
}
