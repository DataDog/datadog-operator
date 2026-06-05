// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kubeactions

import (
	rbacv1 "k8s.io/api/rbac/v1"

	"github.com/DataDog/datadog-operator/pkg/kubernetes/rbac"
)

// kubeActionsRBACPolicyRules are the cluster-scoped rules the Cluster Agent
// needs to remediate workloads on behalf of the Kubernetes Actions product.
var kubeActionsRBACPolicyRules = []rbacv1.PolicyRule{
	{
		APIGroups: []string{rbac.CoreAPIGroup},
		Resources: []string{rbac.PodsResource},
		Verbs:     []string{rbac.GetVerb, rbac.ListVerb, rbac.DeleteVerb},
	},
	{
		APIGroups: []string{rbac.AppsAPIGroup},
		Resources: []string{rbac.DeploymentsResource},
		Verbs:     []string{rbac.GetVerb, rbac.ListVerb, rbac.PatchVerb, rbac.UpdateVerb},
	},
	{
		APIGroups: []string{rbac.AppsAPIGroup},
		Resources: []string{"deployments/status"},
		Verbs:     []string{rbac.GetVerb},
	},
}
