// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kubernetes

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var (
	defaultProvider          = DefaultProvider
	gcpCosContainerdProvider = generateValidProviderName(GCPCloudProvider, GCPCosContainerdType)
	gcpCosProvider           = generateValidProviderName(GCPCloudProvider, GCPCosType)
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
				GCPProviderLabel: GCPCosType,
			},
			provider: generateValidProviderName(GCPCloudProvider, GCPCosType),
		},
		{
			name: "gcp provider, underscore",
			labels: map[string]string{
				"foo":            "bar",
				GCPProviderLabel: GCPCosContainerdType,
			},
			provider: generateValidProviderName(GCPCloudProvider, GCPCosContainerdType),
		},
		{
			name: "openshift provider",
			labels: map[string]string{
				"foo":                  "bar",
				OpenShiftProviderLabel: OpenShiftRHCOSProvider,
			},
			provider: Provider{
				Name:          OpenShiftRHCOSProvider,
				ComponentName: "default-rhcos",
				CloudProvider: DefaultProvider,
				ProviderLabel: OpenShiftProviderLabel,
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

func Test_isProviderValueAllowed(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{
			name:  "valid value",
			value: GCPCosContainerdType,
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
						GCPCosType,
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
						GCPCosContainerdType,
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
						GCPCosType,
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
						GCPCosContainerdType,
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
			wantValue: GCPCosType,
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

func Test_Reset(t *testing.T) {
	tests := []struct {
		name              string
		newProviders      map[string]struct{}
		existingProviders *ProviderStore
		wantProviders     *ProviderStore
	}{
		{
			name: "replace empty providers",
			newProviders: map[string]struct{}{
				gcpCosProvider:  {},
				defaultProvider: {},
			},
			existingProviders: nil,
			wantProviders: &ProviderStore{
				providers: map[string]struct{}{
					gcpCosProvider:  {},
					defaultProvider: {},
				},
			},
		},
		{
			name: "replace existing providers",
			newProviders: map[string]struct{}{
				gcpCosProvider:  {},
				defaultProvider: {},
			},
			existingProviders: &ProviderStore{
				providers: map[string]struct{}{
					"test": {},
				},
			},
			wantProviders: &ProviderStore{
				providers: map[string]struct{}{
					gcpCosProvider:  {},
					defaultProvider: {},
				},
			},
		},
		{
			name:         "empty new providers",
			newProviders: map[string]struct{}{},
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
			providerStore := NewProviderStore(logger)
			if tt.existingProviders != nil && tt.existingProviders.providers != nil {
				providerStore.providers = tt.existingProviders.providers
			}

			providerStore.Reset(tt.newProviders)
			assert.Equal(t, tt.wantProviders.providers, providerStore.providers)
		})
	}
}

func Test_IsPresent(t *testing.T) {
	tests := []struct {
		name              string
		provider          string
		existingProviders *ProviderStore
		want              bool
	}{
		{
			name:     "provider in provider store",
			provider: defaultProvider,
			existingProviders: &ProviderStore{
				providers: map[string]struct{}{
					gcpCosProvider:  {},
					defaultProvider: {},
				},
			},
			want: true,
		},
		{
			name:     "provider not in provider store",
			provider: defaultProvider,
			existingProviders: &ProviderStore{
				providers: map[string]struct{}{
					gcpCosProvider: {},
				},
			},
			want: false,
		},
		{
			name:     "empty provider",
			provider: "",
			existingProviders: &ProviderStore{
				providers: map[string]struct{}{
					gcpCosProvider: {},
				},
			},
			want: false,
		},
		{
			name:              "empty provider store",
			provider:          defaultProvider,
			existingProviders: &ProviderStore{},
			want:              false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := logf.Log.WithName(t.Name())
			providerStore := NewProviderStore(logger)
			if tt.existingProviders != nil && tt.existingProviders.providers != nil {
				providerStore.providers = tt.existingProviders.providers
			}

			assert.Equal(t, tt.want, providerStore.IsPresent(tt.provider))
		})
	}
}
