// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import v1 "k8s.io/api/core/v1"

func MergeAffinities(affinity1 *v1.Affinity, affinity2 *v1.Affinity) *v1.Affinity {
	if affinity1 == nil && affinity2 == nil {
		return nil
	}

	if affinity1 == nil {
		return affinity2
	}

	if affinity2 == nil {
		return affinity1
	}

	merged := &v1.Affinity{}

	merged.NodeAffinity = mergeNodeAffinities(affinity1.NodeAffinity, affinity2.NodeAffinity)
	merged.PodAffinity = mergePodAffinities(affinity1.PodAffinity, affinity2.PodAffinity)
	merged.PodAntiAffinity = mergePodAntiAffinities(affinity1.PodAntiAffinity, affinity2.PodAntiAffinity)

	return merged
}

func mergeNodeAffinities(affinity1 *v1.NodeAffinity, affinity2 *v1.NodeAffinity) *v1.NodeAffinity {
	if affinity1 == nil && affinity2 == nil {
		return nil
	}

	if affinity1 == nil {
		return affinity2
	}

	if affinity2 == nil {
		return affinity1
	}

	merged := &v1.NodeAffinity{}

	merged.RequiredDuringSchedulingIgnoredDuringExecution = mergeNodeSelectors(
		affinity1.RequiredDuringSchedulingIgnoredDuringExecution,
		affinity2.RequiredDuringSchedulingIgnoredDuringExecution,
	)

	merged.PreferredDuringSchedulingIgnoredDuringExecution = append(
		merged.PreferredDuringSchedulingIgnoredDuringExecution,
		affinity1.PreferredDuringSchedulingIgnoredDuringExecution...,
	)
	merged.PreferredDuringSchedulingIgnoredDuringExecution = append(
		merged.PreferredDuringSchedulingIgnoredDuringExecution,
		affinity2.PreferredDuringSchedulingIgnoredDuringExecution...,
	)

	return merged
}

func mergeNodeSelectors(selector1 *v1.NodeSelector, selector2 *v1.NodeSelector) *v1.NodeSelector {
	if selector1 == nil && selector2 == nil {
		return nil
	}

	if selector1 == nil {
		return selector2
	}

	if selector2 == nil {
		return selector1
	}

	merged := &v1.NodeSelector{}

	// Note that the NodeSelectorTerms are ORed together.
	for _, term1 := range selector1.NodeSelectorTerms {
		for _, term2 := range selector2.NodeSelectorTerms {
			mergedTerm := v1.NodeSelectorTerm{
				// These are ANDed together.
				MatchExpressions: append(term1.MatchExpressions, term2.MatchExpressions...),
				MatchFields:      append(term1.MatchFields, term2.MatchFields...),
			}
			merged.NodeSelectorTerms = append(merged.NodeSelectorTerms, mergedTerm)
		}
	}

	return merged
}

func mergePodAffinities(affinity1 *v1.PodAffinity, affinity2 *v1.PodAffinity) *v1.PodAffinity {
	if affinity1 == nil && affinity2 == nil {
		return nil
	}

	if affinity1 == nil {
		return affinity2
	}

	if affinity2 == nil {
		return affinity1
	}

	merged := &v1.PodAffinity{}

	merged.RequiredDuringSchedulingIgnoredDuringExecution = append(
		merged.RequiredDuringSchedulingIgnoredDuringExecution,
		affinity1.RequiredDuringSchedulingIgnoredDuringExecution...,
	)
	merged.RequiredDuringSchedulingIgnoredDuringExecution = append(
		merged.RequiredDuringSchedulingIgnoredDuringExecution,
		affinity2.RequiredDuringSchedulingIgnoredDuringExecution...,
	)

	merged.PreferredDuringSchedulingIgnoredDuringExecution = append(
		merged.PreferredDuringSchedulingIgnoredDuringExecution,
		affinity1.PreferredDuringSchedulingIgnoredDuringExecution...,
	)
	merged.PreferredDuringSchedulingIgnoredDuringExecution = append(
		merged.PreferredDuringSchedulingIgnoredDuringExecution,
		affinity2.PreferredDuringSchedulingIgnoredDuringExecution...,
	)

	return merged
}

func mergePodAntiAffinities(affinity1 *v1.PodAntiAffinity, affinity2 *v1.PodAntiAffinity) *v1.PodAntiAffinity {
	if affinity1 == nil && affinity2 == nil {
		return nil
	}

	if affinity1 == nil {
		return affinity2
	}

	if affinity2 == nil {
		return affinity1
	}

	merged := &v1.PodAntiAffinity{}

	merged.RequiredDuringSchedulingIgnoredDuringExecution = append(
		merged.RequiredDuringSchedulingIgnoredDuringExecution,
		affinity1.RequiredDuringSchedulingIgnoredDuringExecution...,
	)
	merged.RequiredDuringSchedulingIgnoredDuringExecution = append(
		merged.RequiredDuringSchedulingIgnoredDuringExecution,
		affinity2.RequiredDuringSchedulingIgnoredDuringExecution...,
	)

	merged.PreferredDuringSchedulingIgnoredDuringExecution = append(
		merged.PreferredDuringSchedulingIgnoredDuringExecution,
		affinity1.PreferredDuringSchedulingIgnoredDuringExecution...,
	)
	merged.PreferredDuringSchedulingIgnoredDuringExecution = append(
		merged.PreferredDuringSchedulingIgnoredDuringExecution,
		affinity2.PreferredDuringSchedulingIgnoredDuringExecution...,
	)

	return merged
}
