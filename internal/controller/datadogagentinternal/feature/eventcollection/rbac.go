// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eventcollection

import (
	rbacv1 "k8s.io/api/rbac/v1"

	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/common"
	"github.com/DataDog/datadog-operator/pkg/kubernetes/rbac"
)

// getRBACRules generates the rbac rules required for EventCollection
func getRBACPolicyRules(tokenResourceName string) []rbacv1.PolicyRule {
	rbacRules := []rbacv1.PolicyRule{
		{
			APIGroups: []string{rbac.CoreAPIGroup},
			Resources: []string{rbac.ConfigMapsResource},
			ResourceNames: []string{
				common.DatadogTokenOldResourceName, // agent <7.37 token reource name
				tokenResourceName,
			},
			Verbs: []string{rbac.GetVerb, rbac.UpdateVerb},
		},
	}

	return rbacRules
}

// getLeaderElectionRBACPolicyRules generates the rbac rules required for leader election
func getLeaderElectionRBACPolicyRules(leaderElectionResourceName string) []rbacv1.PolicyRule {
	rbacRules := []rbacv1.PolicyRule{
		{
			APIGroups: []string{rbac.CoreAPIGroup},
			Resources: []string{rbac.ConfigMapsResource},
			ResourceNames: []string{
				common.DatadogLeaderElectionOldResourceName, // agent <7.37 leader election resource name
				leaderElectionResourceName,
			},
			Verbs: []string{rbac.GetVerb, rbac.UpdateVerb},
		},
		{ // To create the leader election token
			APIGroups: []string{rbac.CoreAPIGroup},
			Resources: []string{rbac.ConfigMapsResource},
			Verbs:     []string{rbac.CreateVerb},
		},
	}

	return rbacRules
}
