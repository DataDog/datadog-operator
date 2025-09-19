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
	return rbacv1.PolicyRule{
		NonResourceURLs: []string{
			rbac.MetricsURL,
			rbac.MetricsSLIsURL,
		},
		Verbs: []string{rbac.GetVerb},
	}
}

func getKubeletPolicyRule(useFineGrainedAuthorization bool) rbacv1.PolicyRule {
	var resources []string
	if useFineGrainedAuthorization {
		resources = []string{
			rbac.NodeMetricsResource,
			rbac.NodeSpecResource,
			rbac.NodeStats,
			rbac.NodePodsResource,
			rbac.NodeHealthzResource,
			rbac.NodeConfigzResource,
			rbac.NodeLogsResource,
		}
	} else {
		resources = []string{
			rbac.NodeMetricsResource,
			rbac.NodeSpecResource,
			rbac.NodeProxyResource,
			rbac.NodeStats,
		}
	}

	return rbacv1.PolicyRule{
		APIGroups: []string{rbac.CoreAPIGroup},
		Resources: resources,
		Verbs:     []string{rbac.GetVerb},
	}
}

func getEndpointsPolicyRule() rbacv1.PolicyRule {
	return rbacv1.PolicyRule{
		APIGroups: []string{rbac.CoreAPIGroup},
		Resources: []string{rbac.EndpointsResource},
		Verbs:     []string{rbac.GetVerb},
	}
}

func getLeaderElectionPolicyRule() rbacv1.PolicyRule {
	return rbacv1.PolicyRule{
		APIGroups: []string{rbac.CoordinationAPIGroup},
		Resources: []string{rbac.LeasesResource},
		Verbs:     []string{rbac.GetVerb},
	}
}
