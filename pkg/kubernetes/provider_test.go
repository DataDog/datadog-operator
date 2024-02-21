// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kubernetes

import (
	"testing"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var (
	defaultProvider = DefaultProvider
	gkeCosProvider  = generateValidProviderName(GKECloudProvider, GKECosType)
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
			name: "gke provider",
			labels: map[string]string{
				"foo":            "bar",
				GKEProviderLabel: GKECosType,
			},
			provider: generateValidProviderName(GKECloudProvider, GKECosType),
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
			value: GKECosType,
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
				gkeCosProvider: {},
			},
			wantSortedProviders: []string{gkeCosProvider},
		},
		{
			name: "multiple providers",
			existingProviders: map[string]struct{}{
				gkeCosProvider: {},
				"abcde":        {},
				"zyxwv":        {},
				"12345":        {},
			},
			wantSortedProviders: []string{
				"12345",
				"abcde",
				gkeCosProvider,
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
				gkeCosProvider: {},
			},
			provider: defaultProvider,
			wantNSR: []corev1.NodeSelectorRequirement{
				{
					Key:      GKEProviderLabel,
					Operator: corev1.NodeSelectorOpNotIn,
					Values: []string{
						GKECosType,
					},
				},
			},
		},
		{
			name: "multiple providers, default provider",
			existingProviders: map[string]struct{}{
				gkeCosProvider: {},
				"gke-abcde":    {},
				"gke-zyxwv":    {},
			},
			provider: defaultProvider,
			wantNSR: []corev1.NodeSelectorRequirement{
				{
					Key:      GKEProviderLabel,
					Operator: corev1.NodeSelectorOpNotIn,
					Values: []string{
						"abcde",
					},
				},
				{
					Key:      GKEProviderLabel,
					Operator: corev1.NodeSelectorOpNotIn,
					Values: []string{
						GKECosType,
					},
				},
				{
					Key:      GKEProviderLabel,
					Operator: corev1.NodeSelectorOpNotIn,
					Values: []string{
						"zyxwv",
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
			name:      "gke cos provider",
			provider:  gkeCosProvider,
			wantLabel: GKEProviderLabel,
			wantValue: GKECosType,
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
				gkeCosProvider:  {},
				defaultProvider: {},
			},
			existingProviders: nil,
			wantProviders: &ProviderStore{
				providers: map[string]struct{}{
					gkeCosProvider:  {},
					defaultProvider: {},
				},
			},
		},
		{
			name: "replace existing providers",
			newProviders: map[string]struct{}{
				gkeCosProvider:  {},
				defaultProvider: {},
			},
			existingProviders: &ProviderStore{
				providers: map[string]struct{}{
					"test": {},
				},
			},
			wantProviders: &ProviderStore{
				providers: map[string]struct{}{
					gkeCosProvider:  {},
					defaultProvider: {},
				},
			},
		},
		{
			name:         "empty new providers",
			newProviders: map[string]struct{}{},
			existingProviders: &ProviderStore{
				providers: map[string]struct{}{
					gkeCosProvider: {},
				},
			},
			wantProviders: &ProviderStore{
				providers: map[string]struct{}{
					gkeCosProvider: {},
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
					gkeCosProvider:  {},
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
					gkeCosProvider: {},
				},
			},
			want: false,
		},
		{
			name:     "empty provider",
			provider: "",
			existingProviders: &ProviderStore{
				providers: map[string]struct{}{
					gkeCosProvider: {},
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

func Test_ComponentOverrideFromProvider(t *testing.T) {
	tests := []struct {
		name          string
		daemonSetName string
		provider      string
		affinity      *corev1.Affinity
		want          v2alpha1.DatadogAgentComponentOverride
	}{
		{
			name:          "component override",
			daemonSetName: "foo",
			provider:      defaultProvider,
			want: v2alpha1.DatadogAgentComponentOverride{
				Name:   apiutils.NewStringPointer("foo-default"),
				Labels: map[string]string{apicommon.MD5AgentDeploymentProviderLabelKey: defaultProvider},
			},
		},
		{
			name:          "empty provider",
			daemonSetName: "foo",
			provider:      "",
			want: v2alpha1.DatadogAgentComponentOverride{
				Name:   apiutils.NewStringPointer("foo"),
				Labels: map[string]string{apicommon.MD5AgentDeploymentProviderLabelKey: ""},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			componentOverride := ComponentOverrideFromProvider(tt.daemonSetName, tt.provider, tt.affinity)
			assert.Equal(t, tt.want, componentOverride)
		})
	}
}

func Test_GetAgentNameWithProvider(t *testing.T) {
	tests := []struct {
		name         string
		overrideName string
		provider     string
		want         string
	}{
		{
			name:         "override name set, default provider",
			overrideName: "foo",
			provider:     defaultProvider,
			want:         "foo-default",
		},
		{
			name:         "override name set but empty, default provider",
			overrideName: "",
			provider:     defaultProvider,
			want:         "",
		},
		{
			name:         "override name set, no provider",
			overrideName: "foo",
			want:         "foo",
		},
		{
			name:         "override name and provider empty",
			overrideName: "",
			provider:     "",
			want:         "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name := GetAgentNameWithProvider(tt.overrideName, tt.provider)
			assert.Equal(t, tt.want, name)
		})
	}
}
