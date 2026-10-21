// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kubernetesactions

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-operator/pkg/kubernetes/rbac"
)

func TestKubernetesActionsRBACPolicyRules(t *testing.T) {
	rules := kubernetesActionsRBACPolicyRules
	assert.Len(t, rules, 4, "expected pods, deployments, deployments/status, replicasets rules")

	byResource := map[string]int{}
	for i, r := range rules {
		for _, res := range r.Resources {
			byResource[res] = i
		}
	}

	podRule := rules[byResource[rbac.PodsResource]]
	assert.Equal(t, []string{rbac.CoreAPIGroup}, podRule.APIGroups)
	assert.ElementsMatch(t, []string{rbac.GetVerb, rbac.ListVerb, rbac.DeleteVerb}, podRule.Verbs)

	deployRule := rules[byResource[rbac.DeploymentsResource]]
	assert.Equal(t, []string{rbac.AppsAPIGroup}, deployRule.APIGroups)
	assert.ElementsMatch(t, []string{rbac.GetVerb, rbac.ListVerb, rbac.PatchVerb, rbac.UpdateVerb}, deployRule.Verbs)

	statusRule := rules[byResource["deployments/status"]]
	assert.Equal(t, []string{rbac.AppsAPIGroup}, statusRule.APIGroups)
	assert.ElementsMatch(t, []string{rbac.GetVerb}, statusRule.Verbs)

	// The patch DaemonSet / StatefulSet actions are not exposed in the Datadog UI:
	// do not grant cluster-wide patch on those workloads.
	for _, res := range []string{rbac.DaemonsetsResource, rbac.StatefulsetsResource} {
		_, granted := byResource[res]
		assert.False(t, granted, "%s should not be granted", res)
	}

	rsRule := rules[byResource[rbac.ReplicasetsResource]]
	assert.Equal(t, []string{rbac.AppsAPIGroup}, rsRule.APIGroups)
	assert.ElementsMatch(t, []string{rbac.GetVerb, rbac.ListVerb}, rsRule.Verbs)
}
