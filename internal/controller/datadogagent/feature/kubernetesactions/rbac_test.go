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
	assert.Len(t, rules, 3, "expected pods, deployments, deployments/status rules")

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
}
