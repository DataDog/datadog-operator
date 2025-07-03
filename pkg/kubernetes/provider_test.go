// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kubernetes

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

var (
	defaultProvider = DefaultProvider
	gkeCosProvider  = generateValidProviderName(GKECloudProvider, GKECosType)
	aksRoleProvider = generateValidProviderName(AKSCloudProvider, AKSRoleType)
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
		{
			name: "aks provider",
			labels: map[string]string{
				"foo":            "bar",
				AKSProviderLabel: AKSRoleType,
			},
			provider: generateValidProviderName(AKSCloudProvider, AKSRoleType),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := determineProvider(tt.labels)
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
			name:  "valid value",
			value: AKSRoleType,
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
				gkeCosProvider:  {},
				aksRoleProvider: {},
				"zyxwv":         {},
				"12345":         {},
			},
			wantSortedProviders: []string{
				"12345",
				aksRoleProvider,
				gkeCosProvider,
				"zyxwv",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sortedProviders := sortProviders(tt.existingProviders)
			assert.Equal(t, tt.wantSortedProviders, sortedProviders)
		})
	}
}

func Test_getProviderNodeAffinity(t *testing.T) {
	tests := []struct {
		name              string
		existingProviders map[string]struct{}
		provider          string
		wantAffinity      *corev1.Affinity
	}{
		{
			name:              "nil provider list",
			existingProviders: nil,
			provider:          defaultProvider,
			wantAffinity:      nil,
		},
		{
			name:              "empty provider list",
			existingProviders: map[string]struct{}{},
			provider:          defaultProvider,
			wantAffinity:      nil,
		},
		{
			name: "empty provider",
			existingProviders: map[string]struct{}{
				gkeCosProvider: {},
			},
			provider:     "",
			wantAffinity: nil,
		},
		{
			name: "single default provider",
			existingProviders: map[string]struct{}{
				defaultProvider: {},
			},
			provider:     defaultProvider,
			wantAffinity: nil,
		},
		{
			name: "single cos provider",
			existingProviders: map[string]struct{}{
				gkeCosProvider: {},
			},
			provider: gkeCosProvider,
			wantAffinity: &corev1.Affinity{
				NodeAffinity: &corev1.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
						NodeSelectorTerms: []corev1.NodeSelectorTerm{
							{
								MatchExpressions: []corev1.NodeSelectorRequirement{
									{
										Key:      GKEProviderLabel,
										Operator: corev1.NodeSelectorOpIn,
										Values: []string{
											GKECosType,
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "one other provider, default provider",
			existingProviders: map[string]struct{}{
				gkeCosProvider:  {},
				defaultProvider: {},
			},
			provider: defaultProvider,
			wantAffinity: &corev1.Affinity{
				NodeAffinity: &corev1.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
						NodeSelectorTerms: []corev1.NodeSelectorTerm{
							{
								MatchExpressions: []corev1.NodeSelectorRequirement{
									{
										Key:      GKEProviderLabel,
										Operator: corev1.NodeSelectorOpNotIn,
										Values: []string{
											GKECosType,
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "one other provider, cos provider",
			existingProviders: map[string]struct{}{
				defaultProvider: {},
				gkeCosProvider:  {},
			},
			provider: gkeCosProvider,
			wantAffinity: &corev1.Affinity{
				NodeAffinity: &corev1.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
						NodeSelectorTerms: []corev1.NodeSelectorTerm{
							{
								MatchExpressions: []corev1.NodeSelectorRequirement{
									{
										Key:      GKEProviderLabel,
										Operator: corev1.NodeSelectorOpIn,
										Values: []string{
											GKECosType,
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "multiple providers, default provider",
			existingProviders: map[string]struct{}{
				gkeCosProvider:  {},
				"gke-abcde":     {},
				"gke-zyxwv":     {},
				defaultProvider: {},
			},
			provider: defaultProvider,
			wantAffinity: &corev1.Affinity{
				NodeAffinity: &corev1.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
						NodeSelectorTerms: []corev1.NodeSelectorTerm{
							{
								MatchExpressions: []corev1.NodeSelectorRequirement{
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
						},
					},
				},
			},
		},
		{
			name: "multiple providers, cos provider",
			existingProviders: map[string]struct{}{
				defaultProvider: {},
				"abcdef":        {},
				"lmnop":         {},
				gkeCosProvider:  {},
			},
			provider: gkeCosProvider,
			wantAffinity: &corev1.Affinity{
				NodeAffinity: &corev1.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
						NodeSelectorTerms: []corev1.NodeSelectorTerm{
							{
								MatchExpressions: []corev1.NodeSelectorRequirement{
									{
										Key:      GKEProviderLabel,
										Operator: corev1.NodeSelectorOpIn,
										Values: []string{
											GKECosType,
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "single aks provider",
			existingProviders: map[string]struct{}{
				aksRoleProvider: {},
			},
			provider: aksRoleProvider,
			wantAffinity: &corev1.Affinity{
				NodeAffinity: &corev1.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
						NodeSelectorTerms: []corev1.NodeSelectorTerm{
							{
								MatchExpressions: []corev1.NodeSelectorRequirement{
									{
										Key:      AKSProviderLabel,
										Operator: corev1.NodeSelectorOpIn,
										Values: []string{
											AKSRoleType,
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "one other provider, default provider with aks",
			existingProviders: map[string]struct{}{
				aksRoleProvider: {},
				defaultProvider: {},
			},
			provider: defaultProvider,
			wantAffinity: &corev1.Affinity{
				NodeAffinity: &corev1.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
						NodeSelectorTerms: []corev1.NodeSelectorTerm{
							{
								MatchExpressions: []corev1.NodeSelectorRequirement{
									{
										Key:      AKSProviderLabel,
										Operator: corev1.NodeSelectorOpNotIn,
										Values: []string{
											AKSRoleType,
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "one other provider, aks provider",
			existingProviders: map[string]struct{}{
				defaultProvider: {},
				aksRoleProvider: {},
			},
			provider: aksRoleProvider,
			wantAffinity: &corev1.Affinity{
				NodeAffinity: &corev1.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
						NodeSelectorTerms: []corev1.NodeSelectorTerm{
							{
								MatchExpressions: []corev1.NodeSelectorRequirement{
									{
										Key:      AKSProviderLabel,
										Operator: corev1.NodeSelectorOpIn,
										Values: []string{
											AKSRoleType,
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "multiple providers, default provider with aks and gke",
			existingProviders: map[string]struct{}{
				aksRoleProvider: {},
				gkeCosProvider:  {},
				"gke-abcde":     {},
				defaultProvider: {},
			},
			provider: defaultProvider,
			wantAffinity: &corev1.Affinity{
				NodeAffinity: &corev1.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
						NodeSelectorTerms: []corev1.NodeSelectorTerm{
							{
								MatchExpressions: []corev1.NodeSelectorRequirement{
									{
										Key:      AKSProviderLabel,
										Operator: corev1.NodeSelectorOpNotIn,
										Values: []string{
											AKSRoleType,
										},
									},
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
								},
							},
						},
					},
				},
			},
		},
		{
			name: "multiple providers, aks provider",
			existingProviders: map[string]struct{}{
				defaultProvider: {},
				"gke-xyz":       {},
				gkeCosProvider:  {},
				aksRoleProvider: {},
			},
			provider: aksRoleProvider,
			wantAffinity: &corev1.Affinity{
				NodeAffinity: &corev1.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
						NodeSelectorTerms: []corev1.NodeSelectorTerm{
							{
								MatchExpressions: []corev1.NodeSelectorRequirement{
									{
										Key:      AKSProviderLabel,
										Operator: corev1.NodeSelectorOpIn,
										Values: []string{
											AKSRoleType,
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			affinity := getProviderNodeAffinity(tt.provider, tt.existingProviders)
			assert.Equal(t, tt.wantAffinity, affinity)
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
		{
			name:      "aks provider",
			provider:  aksRoleProvider,
			wantLabel: AKSProviderLabel,
			wantValue: AKSRoleType,
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
