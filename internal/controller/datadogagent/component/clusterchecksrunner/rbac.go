// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package clusterchecksrunner

import (
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/component"
	"github.com/DataDog/datadog-operator/pkg/controller/utils"
	"github.com/DataDog/datadog-operator/pkg/kubernetes/rbac"
)

// RBAC for Cluster Checks Runner

var (
	clusterChecksRunnerClusterRolePolicyRulesBeforeLeaderElection = []rbacv1.PolicyRule{
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
	}

	clusterChecksRunnerClusterRolePolicyRulesAfterLeaderElection = []rbacv1.PolicyRule{
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
		// EKS kube_scheduler and kube_controller_manager control plane metrics
		component.GetEKSControlPlaneMetricsPolicyRule(),
	}

	clusterChecksRunnerMetricsEndpointPolicyRule = rbacv1.PolicyRule{
		NonResourceURLs: []string{
			rbac.MetricsURL,
			rbac.MetricsSLIsURL,
		},
		Verbs: []string{rbac.GetVerb},
	}
)

// GetDefaultClusterChecksRunnerClusterRolePolicyRules returns the default Cluster Role Policy Rules for the Cluster Checks Runner.
func GetDefaultClusterChecksRunnerClusterRolePolicyRules(dda metav1.Object, excludeNonResourceRules bool) []rbacv1.PolicyRule {
	policyRule := rbac.ClonePolicyRules(clusterChecksRunnerClusterRolePolicyRulesBeforeLeaderElection)
	policyRule = append(policyRule, clusterChecksRunnerLeaderElectionPolicyRule(dda))
	policyRule = append(policyRule, rbac.ClonePolicyRules(clusterChecksRunnerClusterRolePolicyRulesAfterLeaderElection)...)

	if !excludeNonResourceRules {
		policyRule = append(policyRule, rbac.ClonePolicyRule(clusterChecksRunnerMetricsEndpointPolicyRule))
	}

	return policyRule
}

func clusterChecksRunnerLeaderElectionPolicyRule(dda metav1.Object) rbacv1.PolicyRule {
	return rbacv1.PolicyRule{
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
	}
}
