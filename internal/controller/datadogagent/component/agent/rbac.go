// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package agent

import (
	rbacv1 "k8s.io/api/rbac/v1"

	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/component"
	"github.com/DataDog/datadog-operator/pkg/kubernetes/rbac"
)

// RBAC for Agent

var (
	agentMetricsEndpointPolicyRule = rbacv1.PolicyRule{
		NonResourceURLs: []string{
			rbac.MetricsURL,
			rbac.MetricsSLIsURL,
		},
		Verbs: []string{rbac.GetVerb},
	}
	agentFineGrainedKubeletPolicyRule = rbacv1.PolicyRule{
		APIGroups: []string{rbac.CoreAPIGroup},
		Resources: []string{
			rbac.NodeMetricsResource,
			rbac.NodeSpecResource,
			rbac.NodeStats,
			rbac.NodePodsResource,
			rbac.NodeHealthzResource,
			rbac.NodeConfigzResource,
			rbac.NodeLogsResource,
		},
		Verbs: []string{rbac.GetVerb},
	}
	agentKubeletPolicyRule = rbacv1.PolicyRule{
		APIGroups: []string{rbac.CoreAPIGroup},
		Resources: []string{
			rbac.NodeMetricsResource,
			rbac.NodeSpecResource,
			rbac.NodeProxyResource,
			rbac.NodeStats,
		},
		Verbs: []string{rbac.GetVerb},
	}
	agentEndpointPolicyRule = rbacv1.PolicyRule{
		APIGroups: []string{rbac.CoreAPIGroup},
		Resources: []string{rbac.EndpointsResource},
		Verbs:     []string{rbac.GetVerb},
	}
	agentLeaderElectionPolicyRule = rbacv1.PolicyRule{
		APIGroups: []string{rbac.CoordinationAPIGroup},
		Resources: []string{rbac.LeasesResource},
		Verbs:     []string{rbac.GetVerb},
	}
)

// GetDefaultAgentClusterRolePolicyRules returns the default policy rules for the Agent cluster role
func GetDefaultAgentClusterRolePolicyRules(excludeNonResourceRules bool, useFineGrainedAuthorization bool) []rbacv1.PolicyRule {
	policyRule := []rbacv1.PolicyRule{
		getKubeletPolicyRule(useFineGrainedAuthorization),
		getEndpointsPolicyRule(),
		getLeaderElectionPolicyRule(),
		component.GetEKSControlPlaneMetricsPolicyRule(),
	}

	if !excludeNonResourceRules {
		policyRule = append(policyRule, getMetricsEndpointPolicyRule())
	}

	return policyRule
}

func getMetricsEndpointPolicyRule() rbacv1.PolicyRule {
	return rbac.ClonePolicyRule(agentMetricsEndpointPolicyRule)
}

func getKubeletPolicyRule(useFineGrainedAuthorization bool) rbacv1.PolicyRule {
	if useFineGrainedAuthorization {
		return rbac.ClonePolicyRule(agentFineGrainedKubeletPolicyRule)
	}

	return rbac.ClonePolicyRule(agentKubeletPolicyRule)
}

func getEndpointsPolicyRule() rbacv1.PolicyRule {
	return rbac.ClonePolicyRule(agentEndpointPolicyRule)
}

func getLeaderElectionPolicyRule() rbacv1.PolicyRule {
	return rbac.ClonePolicyRule(agentLeaderElectionPolicyRule)
}
