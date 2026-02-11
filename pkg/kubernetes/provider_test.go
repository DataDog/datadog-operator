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
	defaultProvider   = DefaultProvider
	gkeCosProvider    = generateValidProviderName(GKECloudProvider, GKECosType)
	openshiftProvider = generateValidProviderName(OpenshiftProvider, "test")
	eksProvider       = EKSCloudProvider // EKS provider is now just "eks" without suffix
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
			name: "openshift provider",
			labels: map[string]string{
				"foo":                  "bar",
				OpenShiftProviderLabel: "rhcos",
			},
			provider: generateValidProviderName(OpenshiftProvider, "rhcos"),
		},
		{
			name: "eks provider with nodegroup-image label",
			labels: map[string]string{
				"foo":            "bar",
				EKSProviderLabel: "ami-0c7217cdde317cfec", // Example Amazon Linux 2 AMI
			},
			provider: EKSCloudProvider,
		},
		{
			name: "eks provider with different ami (bottlerocket)",
			labels: map[string]string{
				"foo":            "bar",
				EKSProviderLabel: "ami-0c2b8ca1dad447f8a", // Example Bottlerocket AMI
			},
			provider: EKSCloudProvider,
		},
		{
			name: "eks provider with nodegroup label",
			labels: map[string]string{
				"foo":                         "bar",
				"eks.amazonaws.com/nodegroup": "my-nodegroup",
			},
			provider: EKSCloudProvider,
		},
		{
			name: "eks fargate provider with compute-type label",
			labels: map[string]string{
				"foo":                               "bar",
				"eks.amazonaws.com/compute-type":    "fargate",
				"eks.amazonaws.com/fargate-profile": "my-profile",
			},
			provider: EKSCloudProvider,
		},
		{
			name: "eks provider with eksctl label",
			labels: map[string]string{
				"foo":                          "bar",
				"alpha.eksctl.io/cluster-name": "my-cluster",
			},
			provider: EKSCloudProvider,
		},
		{
			name: "eks provider with multiple eks labels",
			labels: map[string]string{
				"eks.amazonaws.com/nodegroup":       "my-nodegroup",
				"eks.amazonaws.com/nodegroup-image": "ami-0c7217cdde317cfec",
				"alpha.eksctl.io/cluster-name":      "my-cluster",
			},
			provider: EKSCloudProvider,
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
			name:  "valid GKE value",
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

func Test_isEKSProvider(t *testing.T) {
	tests := []struct {
		name   string
		labels map[string]string
		want   bool
	}{
		{
			name:   "empty labels",
			labels: map[string]string{},
			want:   false,
		},
		{
			name: "no eks labels",
			labels: map[string]string{
				"foo": "bar",
				"baz": "qux",
			},
			want: false,
		},
		{
			name: "eks nodegroup-image label",
			labels: map[string]string{
				"eks.amazonaws.com/nodegroup-image": "ami-0c7217cdde317cfec",
			},
			want: true,
		},
		{
			name: "eks nodegroup label",
			labels: map[string]string{
				"eks.amazonaws.com/nodegroup": "my-nodegroup",
			},
			want: true,
		},
		{
			name: "eks compute-type label (fargate)",
			labels: map[string]string{
				"eks.amazonaws.com/compute-type": "fargate",
			},
			want: true,
		},
		{
			name: "eks fargate-profile label",
			labels: map[string]string{
				"eks.amazonaws.com/fargate-profile": "my-profile",
			},
			want: true,
		},
		{
			name: "eksctl cluster-name label",
			labels: map[string]string{
				"alpha.eksctl.io/cluster-name": "my-cluster",
			},
			want: true,
		},
		{
			name: "eksctl nodegroup-name label",
			labels: map[string]string{
				"alpha.eksctl.io/nodegroup-name": "my-nodegroup",
			},
			want: true,
		},
		{
			name: "multiple eks labels",
			labels: map[string]string{
				"eks.amazonaws.com/nodegroup":       "my-nodegroup",
				"eks.amazonaws.com/nodegroup-image": "ami-0c7217cdde317cfec",
				"alpha.eksctl.io/cluster-name":      "my-cluster",
			},
			want: true,
		},
		{
			name: "eks label with other labels",
			labels: map[string]string{
				"foo":                               "bar",
				"eks.amazonaws.com/nodegroup-image": "ami-0c7217cdde317cfec",
				"baz":                               "qux",
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isEKSProvider(tt.labels)
			assert.Equal(t, tt.want, result)
		})
	}
}

func Test_ShouldUseDefaultDaemonset(t *testing.T) {
	tests := []struct {
		name         string
		providerList map[string]struct{}
		want         bool
	}{
		{
			name:         "empty provider list",
			providerList: map[string]struct{}{},
			want:         false,
		},
		{
			name: "only default provider",
			providerList: map[string]struct{}{
				"default": {},
			},
			want: false,
		},
		{
			name: "only GKE provider",
			providerList: map[string]struct{}{
				"gke-cos": {},
			},
			want: false,
		},
		{
			name: "only EKS provider",
			providerList: map[string]struct{}{
				eksProvider: {},
			},
			want: true,
		},
		{
			name: "only OpenShift provider",
			providerList: map[string]struct{}{
				"openshift-rhcos": {},
			},
			want: true,
		},
		{
			name: "EKS with other providers",
			providerList: map[string]struct{}{
				eksProvider: {},
				"default":   {},
				"gke-cos":   {},
			},
			want: true,
		},
		{
			name: "OpenShift with other providers",
			providerList: map[string]struct{}{
				"openshift-rhcos": {},
				"default":         {},
				"gke-cos":         {},
			},
			want: true,
		},
		{
			name: "both EKS and OpenShift",
			providerList: map[string]struct{}{
				eksProvider:       {},
				"openshift-rhcos": {},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ShouldUseDefaultDaemonset(tt.providerList)
			assert.Equal(t, tt.want, result)
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
			name: "default provider with eks",
			existingProviders: map[string]struct{}{
				defaultProvider: {},
				eksProvider:     {},
			},
			provider:     defaultProvider,
			wantAffinity: nil,
		},
		{
			name: "default provider with openshift",
			existingProviders: map[string]struct{}{
				defaultProvider:   {},
				openshiftProvider: {},
			},
			provider:     defaultProvider,
			wantAffinity: nil,
		},
		{
			name: "default provider with eks and gke",
			existingProviders: map[string]struct{}{
				defaultProvider: {},
				eksProvider:     {},
				gkeCosProvider:  {},
			},
			provider:     defaultProvider,
			wantAffinity: nil,
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
			name:      "openshift provider",
			provider:  openshiftProvider,
			wantLabel: OpenShiftProviderLabel,
			wantValue: "test",
		},
		{
			name:      "eks provider",
			provider:  eksProvider,
			wantLabel: "", // EKS returns empty - use direct comparison (provider == EKSCloudProvider) instead
			wantValue: "",
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
