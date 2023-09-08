// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	"fmt"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	"github.com/DataDog/datadog-operator/pkg/kubernetes/rbac"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GetAgentRoleName returns the name of the role for the Agent
func GetAgentRoleName(dda metav1.Object) string {
	return fmt.Sprintf("%s-%s", dda.GetName(), apicommon.DefaultAgentResourceSuffix)
}

// GetDefaultAgentClusterRolePolicyRules returns the default policy rules for the Agent cluster role
func GetDefaultAgentClusterRolePolicyRules() []rbacv1.PolicyRule {
	return []rbacv1.PolicyRule{
		getMetricsEndpointPolicyRule(),
		getKubeletPolicyRule(),
		getEndpointsPolicyRule(),
		getLeaderElectionPolicyRule(),
	}
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

func getKubeletPolicyRule() rbacv1.PolicyRule {
	return rbacv1.PolicyRule{
		APIGroups: []string{rbac.CoreAPIGroup},
		Resources: []string{
			rbac.NodeMetricsResource,
			rbac.NodeSpecResource,
			rbac.NodeProxyResource,
			rbac.NodeStats,
		},
		Verbs: []string{rbac.GetVerb},
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
