package otelcollector

import (
	rbacv1 "k8s.io/api/rbac/v1"

	"github.com/DataDog/datadog-operator/pkg/kubernetes/rbac"
)

// getK8sAttributesRBACPolicyRules generates the cluster role permissions
// required for the OpenTelemetry collector with k8sattributes processor.
func getK8sAttributesRBACPolicyRules() []rbacv1.PolicyRule {
	return []rbacv1.PolicyRule{
		{
			APIGroups: []string{rbac.CoreAPIGroup},
			Resources: []string{
				rbac.PodsResource,
				rbac.NamespaceResource,
			},
			Verbs: []string{
				rbac.GetVerb,
				rbac.ListVerb,
				rbac.WatchVerb,
			},
		},
		{
			APIGroups: []string{rbac.AppsAPIGroup},
			Resources: []string{
				rbac.ReplicasetsResource,
			},
			Verbs: []string{
				rbac.GetVerb,
				rbac.ListVerb,
				rbac.WatchVerb,
			},
		},
		{
			APIGroups: []string{rbac.ExtensionsAPIGroup},
			Resources: []string{
				rbac.ReplicasetsResource,
			},
			Verbs: []string{
				rbac.GetVerb,
				rbac.ListVerb,
				rbac.WatchVerb,
			},
		},
	}
}
