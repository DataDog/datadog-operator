// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package override

import (
	"strings"

	"github.com/DataDog/datadog-operator/pkg/kubernetes/rbac"
	rbacv1 "k8s.io/api/rbac/v1"
)

func extractGroupAndResource(groupResource string) (group string, resource string, ok bool) {
	parts := strings.Split(groupResource, ".")

	switch len(parts) {
	case 1:
		group = ""
		resource = parts[0]
		ok = true
	case 2:
		group = parts[1]
		resource = parts[0]
		ok = true
	default:
		ok = false
	}

	return group, resource, ok
}

func appendGroupResource(groupResourceAccumulator map[string]map[string]struct{}, group string, resource string) map[string]map[string]struct{} {
	if _, exists := groupResourceAccumulator[group]; !exists {
		groupResourceAccumulator[group] = map[string]struct{}{resource: {}}
	} else {
		groupResourceAccumulator[group][resource] = struct{}{}
	}

	return groupResourceAccumulator
}

func getKubernetesResourceMetadataAsTagsPolicyRules(resourcesLabelsAsTags, resourcesAnnotationsAsTags map[string]map[string]string) []rbacv1.PolicyRule {
	// maps group to resource set
	// using map to avoid duplicates
	groupResourceAccumulator := map[string]map[string]struct{}{}

	for groupResource := range resourcesLabelsAsTags {
		if group, resource, ok := extractGroupAndResource(groupResource); ok {
			groupResourceAccumulator = appendGroupResource(groupResourceAccumulator, group, resource)
		}
	}

	for groupResource := range resourcesAnnotationsAsTags {
		if group, resource, ok := extractGroupAndResource(groupResource); ok {
			groupResourceAccumulator = appendGroupResource(groupResourceAccumulator, group, resource)
		}
	}

	policyRules := make([]rbacv1.PolicyRule, 0)

	for group, resources := range groupResourceAccumulator {
		for resource := range resources {
			policyRules = append(policyRules, rbacv1.PolicyRule{
				APIGroups: []string{group},
				Resources: []string{resource},
				Verbs: []string{
					rbac.GetVerb,
					rbac.ListVerb,
					rbac.WatchVerb,
				},
			},
			)
		}
	}

	return policyRules
}
