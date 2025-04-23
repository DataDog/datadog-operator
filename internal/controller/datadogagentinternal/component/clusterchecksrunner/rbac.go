// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package clusterchecksrunner

import (
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/component"
	"github.com/DataDog/datadog-operator/pkg/controller/utils"
	"github.com/DataDog/datadog-operator/pkg/kubernetes/rbac"
)

// RBAC for Cluster Checks Runner

// GetDefaultClusterChecksRunnerClusterRolePolicyRules returns the default Cluster Role Policy Rules for the Cluster Checks Runner
func GetDefaultClusterChecksRunnerClusterRolePolicyRules(dda metav1.Object, excludeNonResourceRules bool) []rbacv1.PolicyRule {
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
		// EKS kube_scheduler and kube_controller_manager control plane metrics
		component.GetEKSControlPlaneMetricsPolicyRule(),
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
