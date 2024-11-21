// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package enabledefault

import (
	"fmt"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/pkg/controller/utils"
	"github.com/DataDog/datadog-operator/pkg/kubernetes/rbac"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RBAC for Agent

// getDefaultAgentClusterRolePolicyRules returns the default policy rules for the Agent cluster role
func getDefaultAgentClusterRolePolicyRules(excludeNonResourceRules bool) []rbacv1.PolicyRule {
	policyRule := []rbacv1.PolicyRule{
		getKubeletPolicyRule(),
		getEndpointsPolicyRule(),
		getLeaderElectionPolicyRule(),
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

// RBAC for Cluster Agent

// getDefaultClusterAgentRolePolicyRules returns the default policy rules for the Cluster Agent
// Can be used by the Agent if the Cluster Agent is disabled
func getDefaultClusterAgentRolePolicyRules(dda metav1.Object) []rbacv1.PolicyRule {
	rules := []rbacv1.PolicyRule{}

	rules = append(rules, getLeaderElectionPolicyRuleDCA(dda)...)
	rules = append(rules, rbacv1.PolicyRule{
		APIGroups: []string{rbac.CoreAPIGroup},
		Resources: []string{rbac.ConfigMapsResource},
		ResourceNames: []string{
			common.DatadogClusterIDResourceName,
		},
		Verbs: []string{rbac.GetVerb, rbac.UpdateVerb, rbac.CreateVerb},
	})
	rules = append(rules, rbacv1.PolicyRule{
		APIGroups: []string{rbac.DatadogAPIGroup},
		Resources: []string{rbac.DatadogAgentsResource},
		ResourceNames: []string{
			dda.GetName(),
		},
		Verbs: []string{rbac.GetVerb},
	})
	return rules
}

// getDefaultClusterAgentClusterRolePolicyRules returns the default policy rules for the Cluster Agent
// Can be used by the Agent if the Cluster Agent is disabled
func getDefaultClusterAgentClusterRolePolicyRules(dda metav1.Object) []rbacv1.PolicyRule {
	return []rbacv1.PolicyRule{
		{
			APIGroups: []string{rbac.CoreAPIGroup},
			Resources: []string{
				rbac.ServicesResource,
				rbac.EventsResource,
				rbac.EndpointsResource,
				rbac.PodsResource,
				rbac.NodesResource,
				rbac.ComponentStatusesResource,
				rbac.ConfigMapsResource,
				rbac.NamespaceResource,
			},
			Verbs: []string{
				rbac.GetVerb,
				rbac.ListVerb,
				rbac.WatchVerb,
			},
		},
		{
			APIGroups: []string{rbac.OpenShiftQuotaAPIGroup},
			Resources: []string{rbac.ClusterResourceQuotasResource},
			Verbs:     []string{rbac.GetVerb, rbac.ListVerb},
		},
		{
			NonResourceURLs: []string{rbac.VersionURL, rbac.HealthzURL},
			Verbs:           []string{rbac.GetVerb},
		},
		{
			// Horizontal Pod Autoscaling
			APIGroups: []string{rbac.AutoscalingAPIGroup},
			Resources: []string{rbac.HorizontalPodAutoscalersRecource},
			Verbs:     []string{rbac.ListVerb, rbac.WatchVerb},
		},
		{
			APIGroups: []string{rbac.CoreAPIGroup},
			Resources: []string{rbac.NamespaceResource},
			ResourceNames: []string{
				common.KubeSystemResourceName,
			},
			Verbs: []string{rbac.GetVerb},
		},
	}
}

// getLeaderElectionPolicyRuleDCA returns the policy rules for leader election
func getLeaderElectionPolicyRuleDCA(dda metav1.Object) []rbacv1.PolicyRule {
	return []rbacv1.PolicyRule{
		{
			APIGroups: []string{rbac.CoreAPIGroup},
			Resources: []string{rbac.ConfigMapsResource},
			ResourceNames: []string{
				common.DatadogLeaderElectionOldResourceName, // Kept for backward compatibility with agent <7.37.0
				utils.GetDatadogLeaderElectionResourceName(dda),
			},
			Verbs: []string{rbac.GetVerb, rbac.UpdateVerb},
		},
		{
			APIGroups: []string{rbac.CoreAPIGroup},
			Resources: []string{rbac.ConfigMapsResource},
			Verbs:     []string{rbac.CreateVerb},
		},
		{
			APIGroups: []string{rbac.CoordinationAPIGroup},
			Resources: []string{rbac.LeasesResource},
			Verbs:     []string{rbac.CreateVerb},
		},
		{
			APIGroups: []string{rbac.CoordinationAPIGroup},
			Resources: []string{rbac.LeasesResource},
			ResourceNames: []string{
				utils.GetDatadogLeaderElectionResourceName(dda),
			},
			Verbs: []string{rbac.GetVerb, rbac.UpdateVerb},
		},
	}
}

// RBAC for Cluster Checks Runner

// getCCRRbacResourcesName returns the Cluster Checks Runner RBAC resource name
func getCCRRbacResourcesName(dda metav1.Object) string {
	return fmt.Sprintf("%s-%s", dda.GetName(), v2alpha1.DefaultClusterChecksRunnerResourceSuffix)
}

// getDefaultClusterChecksRunnerClusterRolePolicyRules returns the default Cluster Role Policy Rules for the Cluster Checks Runner
func getDefaultClusterChecksRunnerClusterRolePolicyRules(dda metav1.Object, excludeNonResourceRules bool) []rbacv1.PolicyRule {
	policyRule := []rbacv1.PolicyRule{
		{
			APIGroups: []string{rbac.CoreAPIGroup},
			Resources: []string{
				rbac.ServicesResource,
				rbac.EventsResource,
				rbac.EndpointsResource,
				rbac.PodsResource,
				rbac.NodesResource,
				rbac.ComponentStatusesResource,
				rbac.ConfigMapsResource,
				rbac.NamespaceResource,
			},
			Verbs: []string{
				rbac.GetVerb,
				rbac.ListVerb,
				rbac.WatchVerb,
			},
		},
		{
			APIGroups: []string{rbac.CoreAPIGroup},
			Resources: []string{
				rbac.ConfigMapsResource,
			},
			Verbs: []string{
				rbac.CreateVerb,
			},
		},
		{
			APIGroups: []string{rbac.CoreAPIGroup},
			Resources: []string{
				rbac.ConfigMapsResource,
			},
			ResourceNames: []string{
				utils.GetDatadogLeaderElectionResourceName(dda),
			},
			Verbs: []string{
				rbac.GetVerb,
				rbac.UpdateVerb,
			},
		},
		{
			APIGroups: []string{rbac.OpenShiftQuotaAPIGroup},
			Resources: []string{
				rbac.ClusterResourceQuotasResource,
			},
			Verbs: []string{
				rbac.GetVerb,
				rbac.ListVerb,
			},
		},
		{
			NonResourceURLs: []string{
				rbac.VersionURL,
				rbac.HealthzURL,
			},
			Verbs: []string{
				rbac.GetVerb,
			},
		},
		// Leader election that uses Leases, such as kube-controller-manager
		{
			APIGroups: []string{rbac.CoordinationAPIGroup},
			Resources: []string{
				rbac.LeasesResource,
			},
			Verbs: []string{
				rbac.GetVerb,
				rbac.ListVerb,
				rbac.WatchVerb,
			},
		},
		// Horizontal Pod Autoscaling
		{
			APIGroups: []string{rbac.AutoscalingAPIGroup},
			Resources: []string{
				rbac.HorizontalPodAutoscalersRecource,
			},
			Verbs: []string{
				rbac.ListVerb,
				rbac.WatchVerb,
			},
		},
		{
			APIGroups: []string{rbac.CoreAPIGroup},
			Resources: []string{
				rbac.NamespaceResource,
			},
			ResourceNames: []string{
				common.KubeSystemResourceName,
			},
			Verbs: []string{
				rbac.GetVerb,
			},
		},
	}

	if !excludeNonResourceRules {
		policyRule = append(policyRule, rbacv1.PolicyRule{
			NonResourceURLs: []string{
				rbac.MetricsURL,
				rbac.MetricsSLIsURL,
			},
			Verbs: []string{rbac.GetVerb},
		})
	}

	return policyRule
}
