// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rbac

import (
	"slices"

	rbacv1 "k8s.io/api/rbac/v1"
)

// ClonePolicyRules returns an independent copy of policy rules templates.
func ClonePolicyRules(rules []rbacv1.PolicyRule) []rbacv1.PolicyRule {
	cloned := make([]rbacv1.PolicyRule, 0, len(rules))
	for _, rule := range rules {
		cloned = append(cloned, ClonePolicyRule(rule))
	}
	return cloned
}

// ClonePolicyRule returns an independent copy of a policy rule template.
func ClonePolicyRule(rule rbacv1.PolicyRule) rbacv1.PolicyRule {
	return rbacv1.PolicyRule{
		Verbs:           slices.Clone(rule.Verbs),
		APIGroups:       slices.Clone(rule.APIGroups),
		Resources:       slices.Clone(rule.Resources),
		ResourceNames:   slices.Clone(rule.ResourceNames),
		NonResourceURLs: slices.Clone(rule.NonResourceURLs),
	}
}
