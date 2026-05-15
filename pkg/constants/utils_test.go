// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.
package constants

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestServiceAccountNameOverride(t *testing.T) {
	customServiceAccount := "fake"
	ddaName := "test-dda"
	tests := []struct {
		name string
		dda  *v2alpha1.DatadogAgent
		want map[v2alpha1.ComponentName]string
	}{
		{
			name: "custom serviceaccount for dca and clc",
			dda: &v2alpha1.DatadogAgent{
				ObjectMeta: v1.ObjectMeta{
					Name: ddaName,
				},
				Spec: v2alpha1.DatadogAgentSpec{
					Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.ClusterAgentComponentName: {
							ServiceAccountName: &customServiceAccount,
						},
						v2alpha1.ClusterChecksRunnerComponentName: {
							ServiceAccountName: &customServiceAccount,
						},
					},
				},
			},
			want: map[v2alpha1.ComponentName]string{
				v2alpha1.ClusterAgentComponentName:        customServiceAccount,
				v2alpha1.NodeAgentComponentName:           fmt.Sprintf("%s-%s", ddaName, DefaultAgentResourceSuffix),
				v2alpha1.ClusterChecksRunnerComponentName: customServiceAccount,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := map[v2alpha1.ComponentName]string{}
			res[v2alpha1.NodeAgentComponentName] = GetAgentServiceAccount(tt.dda.Name, &tt.dda.Spec)
			res[v2alpha1.ClusterChecksRunnerComponentName] = GetClusterChecksRunnerServiceAccount(tt.dda.Name, &tt.dda.Spec)
			res[v2alpha1.ClusterAgentComponentName] = GetClusterAgentServiceAccount(tt.dda.Name, &tt.dda.Spec)
			for name, sa := range tt.want {
				if res[name] != sa {
					t.Errorf("Service Account Override error = %v, want %v", res[name], tt.want[name])
				}
			}
		})
	}
}

func TestGetConfName(t *testing.T) {
	dda := &v1.ObjectMeta{Name: "datadog"}
	configData := "logs_enabled: true"

	require.Equal(t, "custom-config", GetConfName(dda, &v2alpha1.CustomConfig{
		ConfigMap: &v2alpha1.ConfigMapConfig{Name: "custom-config"},
	}, "datadog-config"))
	require.Equal(t, "datadog-datadog-config", GetConfName(dda, &v2alpha1.CustomConfig{
		ConfigData: &configData,
	}, "datadog-config"))
	require.Equal(t, "datadog-datadog-config", GetConfName(dda, nil, "datadog-config"))
}

func TestGetServiceAccountByComponent(t *testing.T) {
	ddaSpec := &v2alpha1.DatadogAgentSpec{Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{}}

	require.Equal(t, "datadog-agent", GetServiceAccountByComponent("datadog", ddaSpec, v2alpha1.NodeAgentComponentName))
	require.Equal(t, "datadog-cluster-agent", GetServiceAccountByComponent("datadog", ddaSpec, v2alpha1.ClusterAgentComponentName))
	require.Equal(t, "datadog-cluster-checks-runner", GetServiceAccountByComponent("datadog", ddaSpec, v2alpha1.ClusterChecksRunnerComponentName))
	require.Empty(t, GetServiceAccountByComponent("datadog", ddaSpec, v2alpha1.ComponentName("unknown")))
}

func TestGetOtelAgentGatewayServiceAccount(t *testing.T) {
	customServiceAccount := "custom-otel-sa"
	ddaSpec := &v2alpha1.DatadogAgentSpec{
		Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
			v2alpha1.OtelAgentGatewayComponentName: {
				ServiceAccountName: &customServiceAccount,
			},
		},
	}

	require.Equal(t, customServiceAccount, GetOtelAgentGatewayServiceAccount("datadog", ddaSpec))
	delete(ddaSpec.Override, v2alpha1.OtelAgentGatewayComponentName)
	require.Equal(t, "datadog-otel-agent-gateway", GetOtelAgentGatewayServiceAccount("datadog", ddaSpec))
}

func TestIsHostNetworkEnabled(t *testing.T) {
	require.False(t, IsHostNetworkEnabled(&v2alpha1.DatadogAgentSpec{}, v2alpha1.NodeAgentComponentName))
	require.False(t, IsHostNetworkEnabled(&v2alpha1.DatadogAgentSpec{
		Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
			v2alpha1.NodeAgentComponentName: {},
		},
	}, v2alpha1.NodeAgentComponentName))
	require.True(t, IsHostNetworkEnabled(&v2alpha1.DatadogAgentSpec{
		Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
			v2alpha1.NodeAgentComponentName: {HostNetwork: ptr.To(true)},
		},
	}, v2alpha1.NodeAgentComponentName))
}

func TestClusterChecksFlags(t *testing.T) {
	ddaSpec := &v2alpha1.DatadogAgentSpec{
		Features: &v2alpha1.DatadogFeatures{
			ClusterChecks: &v2alpha1.ClusterChecksFeatureConfig{
				Enabled:                 ptr.To(true),
				UseClusterChecksRunners: ptr.To(true),
			},
		},
	}

	require.True(t, IsClusterChecksEnabled(ddaSpec))
	require.True(t, IsCCREnabled(ddaSpec))

	ddaSpec.Features.ClusterChecks.Enabled = ptr.To(false)
	ddaSpec.Features.ClusterChecks.UseClusterChecksRunners = ptr.To(false)
	require.False(t, IsClusterChecksEnabled(ddaSpec))
	require.False(t, IsCCREnabled(ddaSpec))
}

func TestServiceNames(t *testing.T) {
	customLocalServiceName := "custom-local-agent"

	require.Equal(t, customLocalServiceName, GetLocalAgentServiceName("datadog", &v2alpha1.DatadogAgentSpec{
		Global: &v2alpha1.GlobalConfig{
			LocalService: &v2alpha1.LocalService{NameOverride: &customLocalServiceName},
		},
	}))
	require.Equal(t, "datadog-agent", GetLocalAgentServiceName("datadog", &v2alpha1.DatadogAgentSpec{
		Global: &v2alpha1.GlobalConfig{},
	}))
	require.Equal(t, "datadog-otel-agent-gateway", GetOTelAgentGatewayServiceName("datadog"))
}

func TestIsNetworkPolicyEnabled(t *testing.T) {
	enabled, flavor := IsNetworkPolicyEnabled(&v2alpha1.DatadogAgentSpec{})
	require.False(t, enabled)
	require.Empty(t, flavor)

	enabled, flavor = IsNetworkPolicyEnabled(&v2alpha1.DatadogAgentSpec{
		Global: &v2alpha1.GlobalConfig{
			NetworkPolicy: &v2alpha1.NetworkPolicyConfig{Create: ptr.To(true)},
		},
	})
	require.True(t, enabled)
	require.Equal(t, v2alpha1.NetworkPolicyFlavorKubernetes, flavor)

	enabled, flavor = IsNetworkPolicyEnabled(&v2alpha1.DatadogAgentSpec{
		Global: &v2alpha1.GlobalConfig{
			NetworkPolicy: &v2alpha1.NetworkPolicyConfig{
				Create: ptr.To(true),
				Flavor: v2alpha1.NetworkPolicyFlavorCilium,
			},
		},
	})
	require.True(t, enabled)
	require.Equal(t, v2alpha1.NetworkPolicyFlavorCilium, flavor)
}

func TestDefaultProbesAreIndependentCopies(t *testing.T) {
	liveness := GetDefaultLivenessProbe()
	liveness.HTTPGet.Path = "/changed"
	liveness.HTTPGet.Port = intstr.FromInt(1234)

	freshLiveness := GetDefaultLivenessProbe()
	require.Equal(t, DefaultLivenessProbeHTTPPath, freshLiveness.HTTPGet.Path)
	require.Equal(t, intstr.FromInt32(DefaultAgentHealthPort), freshLiveness.HTTPGet.Port)

	trace := GetDefaultTraceAgentProbe()
	trace.TCPSocket.Port = intstr.FromInt(1234)

	require.Equal(t, intstr.FromInt(DefaultApmPort), GetDefaultTraceAgentProbe().TCPSocket.Port)
}

func TestGetDDAName(t *testing.T) {
	require.Equal(t, "from-label", GetDDAName(&v1.ObjectMeta{
		Name: "object-name",
		Labels: map[string]string{
			apicommon.DatadogAgentNameLabelKey: "from-label",
		},
	}))
	require.Equal(t, "object-name", GetDDAName(&v1.ObjectMeta{
		Name: "object-name",
		Labels: map[string]string{
			apicommon.DatadogAgentNameLabelKey: "",
		},
	}))
	require.Equal(t, "object-name", GetDDAName(&v1.ObjectMeta{Name: "object-name"}))
}
