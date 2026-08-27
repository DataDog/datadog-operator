// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kubernetes

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

var (
	defaultProvider   = DefaultProvider
	gkeCosProvider    = generateValidProviderName(GKECloudProvider, GKECosType)
	openshiftProvider = generateValidProviderName(OpenshiftProvider, "test")
	eksProvider       = EKSCloudProvider // EKS provider is now just "eks" without suffix
)

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

func TestNodeProvider(t *testing.T) {
	tests := []struct {
		name   string
		labels map[string]string
		want   string
	}{
		{
			name:   "nil labels",
			labels: nil,
			want:   "",
		},
		{
			name:   "empty labels",
			labels: map[string]string{},
			want:   "",
		},
		{
			name:   "no matching labels",
			labels: map[string]string{"foo": "bar"},
			want:   "",
		},
		{
			name:   "gke cos node",
			labels: map[string]string{GKEProviderLabel: GKECosType},
			want:   GKECosProvider,
		},
		{
			name:   "gke non-cos node (e.g. ubuntu)",
			labels: map[string]string{GKEProviderLabel: "ubuntu"},
			want:   "",
		},
		{
			name:   "unrelated eks label does not match node-scope rules",
			labels: map[string]string{EKSProviderLabel: "ami-x"},
			want:   "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, NodeProvider(tt.labels))
		})
	}
}

func TestGetNodeProviderRule(t *testing.T) {
	rule, ok := GetNodeProviderRule(GKECosProvider)
	assert.True(t, ok)
	assert.Equal(t, GKEProviderLabel, rule.LabelKey)
	assert.Equal(t, []string{GKECosType}, rule.LabelValues)

	_, ok = GetNodeProviderRule("not-a-provider")
	assert.False(t, ok)
}

func TestClusterProviderFromNodeLabels(t *testing.T) {
	tests := []struct {
		name   string
		labels map[string]string
		want   string
	}{
		{name: "eks", labels: map[string]string{EKSProviderLabel: "ami-x"}, want: EKSCloudProvider},
		{name: "aks", labels: map[string]string{"kubernetes.azure.com/cluster": "MC_rg_aks_westus2"}, want: AKSProvider},
		{name: "openshift keeps os suffix", labels: map[string]string{OpenShiftProviderLabel: "rhcos"}, want: "openshift-rhcos"},
		{
			name:   "gke node-OS does not leak to cluster scope",
			labels: map[string]string{GKEProviderLabel: GKECosType},
			want:   DefaultProvider,
		},
		{name: "unlabeled", labels: nil, want: DefaultProvider},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ClusterProviderFromNodeLabels(tt.labels))
		})
	}
}
