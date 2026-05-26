// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package orchestratorexplorer

import (
	"slices"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	rbacv1 "k8s.io/api/rbac/v1"

	"github.com/DataDog/datadog-operator/pkg/kubernetes/rbac"
)

func TestGetRBACPolicyRules(t *testing.T) {
	logger := logr.Discard()

	// Base rules that should always be present
	expectedBaseRules := []rbacv1.PolicyRule{
		{
			APIGroups: []string{rbac.CoreAPIGroup},
			Resources: []string{rbac.PodsResource, rbac.ServicesResource, rbac.NodesResource, rbac.PersistentVolumesResource, rbac.PersistentVolumeClaimsResource, rbac.ServiceAccountResource, rbac.LimitRangesResource},
			Verbs:     []string{rbac.ListVerb, rbac.WatchVerb},
		},
		{
			APIGroups: []string{rbac.AppsAPIGroup},
			Resources: []string{rbac.DeploymentsResource, rbac.ReplicasetsResource, rbac.DaemonsetsResource, rbac.StatefulsetsResource},
			Verbs:     []string{rbac.ListVerb, rbac.WatchVerb},
		},
		{
			APIGroups: []string{rbac.BatchAPIGroup},
			Resources: []string{rbac.JobsResource, rbac.CronjobsResource},
			Verbs:     []string{rbac.ListVerb, rbac.WatchVerb},
		},
		{
			APIGroups: []string{rbac.DatadogAPIGroup},
			Resources: []string{rbac.Wildcard},
			Verbs:     []string{rbac.ListVerb, rbac.WatchVerb},
		},
	}

	tests := []struct {
		name                string
		customResources     []string
		expectedCustomRules []rbacv1.PolicyRule
	}{
		{
			name:                "no custom resources",
			customResources:     []string{},
			expectedCustomRules: []rbacv1.PolicyRule{},
		},
		{
			name:            "with single custom resource",
			customResources: []string{"monitoring.coreos.com/v1/servicemonitors"},
			expectedCustomRules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{"monitoring.coreos.com"},
					Resources: []string{"servicemonitors"},
					Verbs:     []string{rbac.ListVerb, rbac.WatchVerb},
				},
			},
		},
		{
			name:            "with multiple custom resources in same group",
			customResources: []string{"datadoghq.com/v1alpha1/datadogmetrics", "datadoghq.com/v1alpha1/watermarkpodautoscalers"},
			expectedCustomRules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{"datadoghq.com"},
					Resources: []string{"datadogmetrics"},
					Verbs:     []string{rbac.ListVerb, rbac.WatchVerb},
				},
				{
					APIGroups: []string{"datadoghq.com"},
					Resources: []string{"watermarkpodautoscalers"},
					Verbs:     []string{rbac.ListVerb, rbac.WatchVerb},
				},
			},
		},
		{
			name:            "with multiple custom resources in different groups",
			customResources: []string{"monitoring.coreos.com/v1/servicemonitors", "cilium.io/v2/ciliumnetworkpolicies"},
			expectedCustomRules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{"monitoring.coreos.com"},
					Resources: []string{"servicemonitors"},
					Verbs:     []string{rbac.ListVerb, rbac.WatchVerb},
				},
				{
					APIGroups: []string{"cilium.io"},
					Resources: []string{"ciliumnetworkpolicies"},
					Verbs:     []string{rbac.ListVerb, rbac.WatchVerb},
				},
			},
		},
		{
			name:                "with invalid custom resource format",
			customResources:     []string{"invalid-format"},
			expectedCustomRules: []rbacv1.PolicyRule{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rules := getRBACPolicyRules(logger, tt.customResources, false)

			// check base rules
			for _, expectedRule := range expectedBaseRules {
				found := false
				for _, rule := range rules {
					if slices.Equal(rule.APIGroups, expectedRule.APIGroups) &&
						slices.Equal(rule.Resources, expectedRule.Resources) &&
						slices.Equal(rule.Verbs, expectedRule.Verbs) {
						found = true
						break
					}
				}
				assert.True(t, found, "Expected base rule not found: %+v", expectedRule)
			}

			// check custom rules
			for _, expectedRule := range tt.expectedCustomRules {
				found := false
				for _, rule := range rules {
					if slices.Equal(rule.APIGroups, expectedRule.APIGroups) &&
						slices.Equal(rule.Resources, expectedRule.Resources) &&
						slices.Equal(rule.Verbs, expectedRule.Verbs) {
						found = true
						break
					}
				}
				assert.True(t, found, "Expected custom rule not found: %+v", expectedRule)
			}
		})
	}
}

func TestGetRBACPolicyRulesWithNetworkCRDs(t *testing.T) {
	logger := logr.Discard()
	defaultVerbs := []string{rbac.ListVerb, rbac.WatchVerb}

	expectedNetworkCRDRules := []rbacv1.PolicyRule{
		// Gateway API
		{
			APIGroups: []string{rbac.GatewayAPIGroup},
			Resources: []string{rbac.GatewaysResource, rbac.HTTPRoutesResource, rbac.GRPCRoutesResource, rbac.TLSRoutesResource, rbac.ListenerSetsResource},
			Verbs:     defaultVerbs,
		},
		// Istio
		{
			APIGroups: []string{rbac.IstioNetworkingAPIGroup},
			Resources: []string{rbac.VirtualServicesResource, rbac.GatewaysResource, rbac.DestinationRulesResource, rbac.ServiceEntriesResource, rbac.SidecarsResource},
			Verbs:     defaultVerbs,
		},
		// Envoy Gateway
		{
			APIGroups: []string{rbac.EnvoyGatewayAPIGroup},
			Resources: []string{rbac.Wildcard},
			Verbs:     defaultVerbs,
		},
		// Traefik legacy
		{
			APIGroups: []string{rbac.TraefikLegacyAPIGroup},
			Resources: []string{rbac.Wildcard},
			Verbs:     defaultVerbs,
		},
		// Linkerd
		{
			APIGroups: []string{rbac.LinkerdPolicyAPIGroup},
			Resources: []string{rbac.Wildcard},
			Verbs:     defaultVerbs,
		},
		// Consul
		{
			APIGroups: []string{rbac.ConsulAPIGroup},
			Resources: []string{rbac.Wildcard},
			Verbs:     defaultVerbs,
		},
		{
			APIGroups: []string{rbac.ConsulMeshAPIGroup},
			Resources: []string{rbac.Wildcard},
			Verbs:     defaultVerbs,
		},
		// Kuma
		{
			APIGroups: []string{rbac.KumaAPIGroup},
			Resources: []string{rbac.Wildcard},
			Verbs:     defaultVerbs,
		},
		// NGINX
		{
			APIGroups: []string{rbac.NginxAPIGroup},
			Resources: []string{rbac.VirtualServersResource, rbac.VirtualServerRoutesResource},
			Verbs:     defaultVerbs,
		},
		// Traefik
		{
			APIGroups: []string{rbac.TraefikAPIGroup},
			Resources: []string{rbac.IngressRoutesResource},
			Verbs:     defaultVerbs,
		},
		// Kong
		{
			APIGroups: []string{rbac.KongAPIGroup},
			Resources: []string{rbac.Wildcard},
			Verbs:     defaultVerbs,
		},
		// HAProxy
		{
			APIGroups: []string{rbac.HAProxyCoreAPIGroup},
			Resources: []string{rbac.Wildcard},
			Verbs:     defaultVerbs,
		},
		{
			APIGroups: []string{rbac.HAProxyIngressV1APIGroup},
			Resources: []string{rbac.Wildcard},
			Verbs:     defaultVerbs,
		},
	}

	t.Run("network CRD rules are present when enabled", func(t *testing.T) {
		rules := getRBACPolicyRules(logger, nil, true)
		for _, expectedRule := range expectedNetworkCRDRules {
			found := false
			for _, rule := range rules {
				if slices.Equal(rule.APIGroups, expectedRule.APIGroups) &&
					slices.Equal(rule.Resources, expectedRule.Resources) &&
					slices.Equal(rule.Verbs, expectedRule.Verbs) {
					found = true
					break
				}
			}
			assert.True(t, found, "Expected network CRD rule not found: %+v", expectedRule)
		}
	})

	t.Run("network CRD rules are absent when disabled", func(t *testing.T) {
		rules := getRBACPolicyRules(logger, nil, false)
		networkAPIGroups := map[string]bool{
			rbac.IstioNetworkingAPIGroup: true,
			rbac.EnvoyGatewayAPIGroup:    true,
			rbac.TraefikLegacyAPIGroup:   true,
			rbac.LinkerdPolicyAPIGroup:   true,
			rbac.ConsulAPIGroup:          true,
			rbac.ConsulMeshAPIGroup:      true,
			rbac.KumaAPIGroup:            true,
			rbac.NginxAPIGroup:           true,
			rbac.TraefikAPIGroup:         true,
			rbac.KongAPIGroup:            true,
			rbac.HAProxyCoreAPIGroup:     true,
			rbac.HAProxyIngressV1APIGroup: true,
		}
		for _, rule := range rules {
			if len(rule.APIGroups) > 0 && networkAPIGroups[rule.APIGroups[0]] {
				t.Errorf("Unexpected network CRD rule found when disabled: %+v", rule)
			}
		}
	})

	t.Run("network CRD rules combined with custom resources", func(t *testing.T) {
		rules := getRBACPolicyRules(logger, []string{"monitoring.coreos.com/v1/servicemonitors"}, true)

		foundNetworkRule := false
		foundCustomRule := false
		for _, rule := range rules {
			if len(rule.APIGroups) > 0 && rule.APIGroups[0] == rbac.GatewayAPIGroup &&
				slices.Equal(rule.Resources, []string{rbac.GatewaysResource, rbac.HTTPRoutesResource, rbac.GRPCRoutesResource, rbac.TLSRoutesResource, rbac.ListenerSetsResource}) {
				foundNetworkRule = true
			}
			if len(rule.APIGroups) > 0 && rule.APIGroups[0] == "monitoring.coreos.com" &&
				slices.Equal(rule.Resources, []string{"servicemonitors"}) {
				foundCustomRule = true
			}
		}
		assert.True(t, foundNetworkRule, "Expected Gateway API network CRD rule not found")
		assert.True(t, foundCustomRule, "Expected custom resource rule not found")
	})
}
