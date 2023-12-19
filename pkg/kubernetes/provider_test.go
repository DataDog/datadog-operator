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
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var (
	gcpCosNode = corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "bar",
			Name:      "node1",
			Labels: map[string]string{
				GCPProviderLabel: GCPCosProviderValue,
			},
		},
	}
	defaultProvider          = DefaultProvider
	gcpCosContainerdProvider = generateProviderName(GCPCloudProvider, GCPCosContainerdProviderValue)
	gcpCosProvider           = generateProviderName(GCPCloudProvider, GCPCosProviderValue)
)

func Test_determineProvider(t *testing.T) {
	tests := []struct {
		name     string
		labels   map[string]string
		provider string
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
				GCPProviderLabel: GCPCosProviderValue,
			},
			provider: generateProviderName(GCPCloudProvider, GCPCosProviderValue),
		},
		{
			name: "gcp provider, underscore",
			labels: map[string]string{
				"foo":            "bar",
				GCPProviderLabel: GCPCosContainerdProviderValue,
			},
			provider: generateProviderName(GCPCloudProvider, GCPCosContainerdProviderValue),
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
	tests := []struct {
		name              string
		obj               corev1.Node
		existingProviders *ProviderStore
		wantProviders     *ProviderStore
	}{
		{
			name:              "add new provider",
			obj:               gcpCosNode,
			existingProviders: nil,
			wantProviders: &ProviderStore{
				providers: map[string]struct{}{
					gcpCosProvider: {},
				},
			},
		},
		{
			name: "add new provider with existing provider",
			obj:  gcpCosNode,
			existingProviders: &ProviderStore{
				providers: map[string]struct{}{
					"test": {},
				},
			},
			wantProviders: &ProviderStore{
				providers: map[string]struct{}{
					"test":         {},
					gcpCosProvider: {},
				},
			},
		},
		{
			name: "add new node name to existing provider",
			obj:  gcpCosNode,
			existingProviders: &ProviderStore{
				providers: map[string]struct{}{
					gcpCosProvider: {},
				},
			},
			wantProviders: &ProviderStore{
				providers: map[string]struct{}{
					gcpCosProvider: {},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := logf.Log.WithName(t.Name())
			profile := NewProviderStore(logger)
			if tt.existingProviders != nil && tt.existingProviders.providers != nil {
				profile.providers = tt.existingProviders.providers
			}

			profile.SetProvider(&tt.obj)
			assert.Equal(t, tt.wantProviders.providers, profile.providers)
		})
	}
}

func Test_isProviderValueAllowed(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{
			name:  "valid value",
			value: GCPCosContainerdProviderValue,
			want:  true,
		},
		{
			name:  "invalid value",
			value: "foo",
			want:  false,
		},
		{
			name:  "empty value",
			value: "",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allowed := isProviderValueAllowed(tt.value)
			assert.Equal(t, tt.want, allowed)
		})
	}
}

func Test_sortProviders(t *testing.T) {
	tests := []struct {
		name                string
		existingProviders   map[string]struct{}
		wantSortedProviders []string
	}{
		{
			name:                "empty providers",
			existingProviders:   nil,
			wantSortedProviders: []string{},
		},
		{
			name: "one provider",
			existingProviders: map[string]struct{}{
				gcpCosProvider: {},
			},
			wantSortedProviders: []string{gcpCosProvider},
		},
		{
			name: "multiple providers",
			existingProviders: map[string]struct{}{
				gcpCosProvider: {},
				"abcde":        {},
				"zyxwv":        {},
				"12345":        {},
			},
			wantSortedProviders: []string{
				"12345",
				"abcde",
				gcpCosProvider,
				"zyxwv",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := logf.Log.WithName(t.Name())
			p := NewProviderStore(logger)
			if tt.existingProviders != nil {
				p.providers = tt.existingProviders
			}

			sortedProviders := sortProviders(p.providers)
			assert.Equal(t, tt.wantSortedProviders, sortedProviders)
		})
	}
}

func Test_GenerateProviderNodeAffinity(t *testing.T) {
	tests := []struct {
		name              string
		existingProviders map[string]struct{}
		provider          string
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
			existingProviders: map[string]struct{}{
				gcpCosProvider: {},
			},
			provider: defaultProvider,
			wantNSR: []corev1.NodeSelectorRequirement{
				{
					Key:      GCPProviderLabel,
					Operator: corev1.NodeSelectorOpNotIn,
					Values: []string{
						GCPCosProviderValue,
					},
				},
			},
		},
		{
			name: "one existing provider, ubuntu provider",
			existingProviders: map[string]struct{}{
				gcpCosProvider: {},
			},
			provider: gcpCosContainerdProvider,
			wantNSR: []corev1.NodeSelectorRequirement{
				{
					Key:      GCPProviderLabel,
					Operator: corev1.NodeSelectorOpIn,
					Values: []string{
						GCPCosContainerdProviderValue,
					},
				},
			},
		},
		{
			name: "multiple providers, default provider",
			existingProviders: map[string]struct{}{
				gcpCosProvider: {},
				"gcp-abcde":    {},
				"gcp-zyxwv":    {},
			},
			provider: defaultProvider,
			wantNSR: []corev1.NodeSelectorRequirement{
				{
					Key:      GCPProviderLabel,
					Operator: corev1.NodeSelectorOpNotIn,
					Values: []string{
						"abcde",
					},
				},
				{
					Key:      GCPProviderLabel,
					Operator: corev1.NodeSelectorOpNotIn,
					Values: []string{
						GCPCosProviderValue,
					},
				},
				{
					Key:      GCPProviderLabel,
					Operator: corev1.NodeSelectorOpNotIn,
					Values: []string{
						"zyxwv",
					},
				},
			},
		},
		{
			name: "multiple providers, ubuntu provider",
			existingProviders: map[string]struct{}{
				gcpCosProvider: {},
				"abcdef":       {},
				"lmnop":        {},
			},
			provider: gcpCosContainerdProvider,
			wantNSR: []corev1.NodeSelectorRequirement{
				{
					Key:      GCPProviderLabel,
					Operator: corev1.NodeSelectorOpIn,
					Values: []string{
						GCPCosContainerdProviderValue,
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := logf.Log.WithName(t.Name())
			p := NewProviderStore(logger)
			if tt.existingProviders != nil {
				p.providers = tt.existingProviders
			}

			nsr := p.GenerateProviderNodeAffinity(tt.provider)
			assert.Equal(t, tt.wantNSR, nsr)
		})
	}
}

func Test_GetProviderLabelKeyValue(t *testing.T) {
	tests := []struct {
		name      string
		provider  string
		wantLabel string
		wantValue string
	}{
		{
			name:      "empty provider",
			provider:  "",
			wantLabel: "",
			wantValue: "",
		},
		{
			name:      "default provider",
			provider:  defaultProvider,
			wantLabel: "",
			wantValue: "",
		},
		{
			name:      "provider not found in mapping",
			provider:  "test-foo",
			wantLabel: "",
			wantValue: "",
		},
		{
			name:      "incomplete provider 1",
			provider:  "test-",
			wantLabel: "",
			wantValue: "",
		},
		{
			name:      "incomplete provider 2",
			provider:  "-foo",
			wantLabel: "",
			wantValue: "",
		},
		{
			name:      "gcp cos provider",
			provider:  gcpCosProvider,
			wantLabel: GCPProviderLabel,
			wantValue: GCPCosProviderValue,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			label, value := GetProviderLabelKeyValue(tt.provider)
			assert.Equal(t, tt.wantLabel, label)
			assert.Equal(t, tt.wantValue, value)
		})
	}
}
