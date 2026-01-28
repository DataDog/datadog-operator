// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rbac

import (
	"maps"
	"slices"
	"strings"

	rbacv1 "k8s.io/api/rbac/v1"
)

type verbSet map[string]struct{} // set of RBAC verbs (get, list, watch, etc.)

// resourceRuleKey uniquely identifies a resource-based rule
type resourceRuleKey struct {
	apiGroup      string
	resource      string
	resourceNames string // comma-separated resource names or ""
}

// resourceConsolidationKey identifies resource rules that can be merged together
type resourceConsolidationKey struct {
	apiGroup      string
	verbs         string // comma-separated verbs
	resourceNames string // comma-separated resource names or ""
}

// nonResourceRuleKey uniquely identifies a non-resource URL rule
type nonResourceRuleKey struct {
	url string
}

// NormalizePolicyRules takes existing RBAC policy rules and optimizes them:
// - Groups resources by API group and verbs to minimize the number of rules
// - Ensures deterministic sorted output
func NormalizePolicyRules(rules []rbacv1.PolicyRule) []rbacv1.PolicyRule {
	if len(rules) == 0 {
		return nil
	}

	var result []rbacv1.PolicyRule

	// resource rules
	resourceVerbs := mapResourceRulesToVerbs(rules)
	if len(resourceVerbs) > 0 {
		consolidated := consolidateResourceRules(resourceVerbs)
		result = append(result, buildResourcePolicyRules(consolidated)...)
	}

	// non-resource rules
	urlVerbs := mapNonResourceRulesToVerbs(rules)
	if len(urlVerbs) > 0 {
		consolidated := consolidateNonResourceRules(urlVerbs)
		result = append(result, buildNonResourcePolicyRules(consolidated)...)
	}

	return result
}

// mapResourceRulesToVerbs collects all verbs for each unique (apiGroup, resource, resourceNames) combination
func mapResourceRulesToVerbs(rules []rbacv1.PolicyRule) map[resourceRuleKey]verbSet {
	ruleVerbs := make(map[resourceRuleKey]verbSet)

	for _, rule := range rules {
		// Skip non-resource URL rules
		if len(rule.NonResourceURLs) > 0 {
			continue
		}

		var resourceNamesKey string
		if len(rule.ResourceNames) > 0 {
			sortedNames := slices.Clone(rule.ResourceNames)
			slices.Sort(sortedNames)
			resourceNamesKey = strings.Join(sortedNames, ",")
		}

		for _, apiGroup := range rule.APIGroups {
			for _, resource := range rule.Resources {
				key := resourceRuleKey{
					apiGroup:      apiGroup,
					resource:      resource,
					resourceNames: resourceNamesKey,
				}

				if ruleVerbs[key] == nil {
					ruleVerbs[key] = make(verbSet)
				}

				for _, verb := range rule.Verbs {
					ruleVerbs[key][verb] = struct{}{}
				}
			}
		}
	}

	return ruleVerbs
}

// consolidateResourceRules groups resources with the same apiGroup, verbs, and resourceNames
func consolidateResourceRules(ruleVerbs map[resourceRuleKey]verbSet) map[resourceConsolidationKey][]string {
	consolidated := make(map[resourceConsolidationKey][]string)

	for key, verbSet := range ruleVerbs {
		verbs := slices.Sorted(maps.Keys(verbSet))
		verbsKey := strings.Join(verbs, ",")

		consKey := resourceConsolidationKey{
			apiGroup:      key.apiGroup,
			verbs:         verbsKey,
			resourceNames: key.resourceNames,
		}

		consolidated[consKey] = append(consolidated[consKey], key.resource)
	}

	for key := range consolidated {
		slices.Sort(consolidated[key])
	}

	return consolidated
}

// buildResourcePolicyRules converts consolidated resource rules into sorted PolicyRules
func buildResourcePolicyRules(consolidated map[resourceConsolidationKey][]string) []rbacv1.PolicyRule {
	result := make([]rbacv1.PolicyRule, 0, len(consolidated))

	sortedKeys := slices.SortedFunc(maps.Keys(consolidated), func(a, b resourceConsolidationKey) int {
		if a.apiGroup != b.apiGroup {
			return strings.Compare(a.apiGroup, b.apiGroup)
		}
		if a.verbs != b.verbs {
			return strings.Compare(a.verbs, b.verbs)
		}
		return strings.Compare(a.resourceNames, b.resourceNames)
	})

	for _, key := range sortedKeys {
		rule := rbacv1.PolicyRule{
			APIGroups: []string{key.apiGroup},
			Resources: consolidated[key],
			Verbs:     strings.Split(key.verbs, ","),
		}

		if key.resourceNames != "" {
			rule.ResourceNames = strings.Split(key.resourceNames, ",")
		}

		result = append(result, rule)
	}

	return result
}

// mapNonResourceRulesToVerbs collects all verbs for each unique URL
func mapNonResourceRulesToVerbs(rules []rbacv1.PolicyRule) map[nonResourceRuleKey]verbSet {
	urlVerbs := make(map[nonResourceRuleKey]verbSet)

	for _, rule := range rules {
		// Skip resource rules
		if len(rule.NonResourceURLs) == 0 {
			continue
		}

		for _, url := range rule.NonResourceURLs {
			key := nonResourceRuleKey{url: url}

			if urlVerbs[key] == nil {
				urlVerbs[key] = make(verbSet)
			}

			for _, verb := range rule.Verbs {
				urlVerbs[key][verb] = struct{}{}
			}
		}
	}

	return urlVerbs
}

// consolidateNonResourceRules groups non-resource URLs with the same verbs
func consolidateNonResourceRules(urlVerbs map[nonResourceRuleKey]verbSet) map[string][]string {
	urlsByVerbs := make(map[string][]string)

	for key, verbSet := range urlVerbs {
		verbs := slices.Sorted(maps.Keys(verbSet))
		verbsKey := strings.Join(verbs, ",")
		urlsByVerbs[verbsKey] = append(urlsByVerbs[verbsKey], key.url)
	}

	for key := range urlsByVerbs {
		slices.Sort(urlsByVerbs[key])
	}

	return urlsByVerbs
}

// buildNonResourcePolicyRules converts consolidated non-resource URL rules into sorted PolicyRules
func buildNonResourcePolicyRules(urlsByVerbs map[string][]string) []rbacv1.PolicyRule {
	result := make([]rbacv1.PolicyRule, 0, len(urlsByVerbs))

	sortedVerbKeys := slices.Sorted(maps.Keys(urlsByVerbs))

	for _, verbsKey := range sortedVerbKeys {
		result = append(result, rbacv1.PolicyRule{
			Verbs:           strings.Split(verbsKey, ","),
			NonResourceURLs: urlsByVerbs[verbsKey],
		})
	}

	return result
}
