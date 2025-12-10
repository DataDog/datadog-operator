// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package appsec

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-operator/pkg/kubernetes/rbac"
	"github.com/DataDog/datadog-operator/pkg/testutils"
)

func TestAppsecRBACPolicyRules(t *testing.T) {
	rules := getRBACPolicyRules()

	// Test Events permission
	var foundEventsRule bool
	for _, rule := range rules {
		if len(rule.Resources) > 0 && rule.Resources[0] == rbac.EventsResource {
			assert.Contains(t, rule.Verbs, rbac.CreateVerb, "Events should have create permission")
			foundEventsRule = true
		}
	}
	assert.True(t, foundEventsRule, "Should have Events permissions")

	// Test Ingress permissions
	var foundIngressRule bool
	for _, rule := range rules {
		if len(rule.Resources) > 0 && rule.Resources[0] == rbac.IngressesResource {
			assert.Contains(t, rule.Verbs, rbac.GetVerb)
			assert.Contains(t, rule.Verbs, rbac.ListVerb)
			assert.Contains(t, rule.Verbs, rbac.WatchVerb)
			assert.Contains(t, rule.Verbs, rbac.PatchVerb)
			foundIngressRule = true
		}
	}
	assert.True(t, foundIngressRule, "Should have Ingress permissions")

	// Test CRD detection permission
	var foundCRDRule bool
	for _, rule := range rules {
		if len(rule.APIGroups) > 0 && rule.APIGroups[0] == rbac.APIExtensionsAPIGroup {
			assert.Contains(t, rule.Resources, rbac.CustomResourceDefinitionsResource)
			assert.Contains(t, rule.Verbs, rbac.GetVerb)
			foundCRDRule = true
		}
	}
	assert.True(t, foundCRDRule, "Should have CRD detection permission")

	// Test Gateway API permissions
	var foundGatewayRule bool
	for _, rule := range rules {
		if len(rule.APIGroups) > 0 && rule.APIGroups[0] == rbac.GatewayAPIGroup {
			if len(rule.Resources) > 2 { // The main gateway rule has gateways, gatewayclasses, httproutes
				assert.Contains(t, rule.Resources, "gateways", "Should have gateways permission")
				assert.Contains(t, rule.Resources, "gatewayclasses", "Should have gatewayclasses permission")
				assert.Contains(t, rule.Resources, "httproutes", "Should have httproutes permission")
				assert.Contains(t, rule.Verbs, rbac.GetVerb)
				assert.Contains(t, rule.Verbs, rbac.ListVerb)
				assert.Contains(t, rule.Verbs, rbac.WatchVerb)
				assert.Contains(t, rule.Verbs, rbac.PatchVerb)
				foundGatewayRule = true
			}
		}
	}
	assert.True(t, foundGatewayRule, "Should have Gateway API permissions")

	// Test Istio permissions
	var foundIstioRule bool
	for _, rule := range rules {
		if len(rule.APIGroups) > 0 && rule.APIGroups[0] == "networking.istio.io" {
			assert.Contains(t, rule.Resources, "envoyfilters")
			assert.Contains(t, rule.Verbs, rbac.GetVerb)
			assert.Contains(t, rule.Verbs, rbac.CreateVerb)
			assert.Contains(t, rule.Verbs, rbac.DeleteVerb)
			foundIstioRule = true
		}
	}
	assert.True(t, foundIstioRule, "Should have Istio permissions")

	// Test Envoy Gateway permissions
	var foundEnvoyRule bool
	for _, rule := range rules {
		if len(rule.APIGroups) > 0 && rule.APIGroups[0] == "gateway.envoyproxy.io" {
			assert.Contains(t, rule.Resources, "envoyextensionpolicies")
			assert.Contains(t, rule.Verbs, rbac.GetVerb)
			assert.Contains(t, rule.Verbs, rbac.CreateVerb)
			assert.Contains(t, rule.Verbs, rbac.DeleteVerb)
			foundEnvoyRule = true
		}
	}
	assert.True(t, foundEnvoyRule, "Should have Envoy Gateway permissions")
}

func TestGetAppsecRBACResourceName(t *testing.T) {
	owner := testutils.NewDatadogAgentBuilder().
		WithName("foo").
		Build()
	// Default namespace is empty, so the name will be -foo-appsec-cluster-agent

	rbacName := getAppsecRBACResourceName(owner, "cluster-agent")
	expected := "-foo-appsec-cluster-agent"
	assert.Equal(t, expected, rbacName, "RBAC resource name should follow the correct format")
}
