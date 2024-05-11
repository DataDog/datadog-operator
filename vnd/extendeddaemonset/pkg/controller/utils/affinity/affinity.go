// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// Package affinity contains Pod affinity functions helper.
package affinity

import (
	v1 "k8s.io/api/core/v1"
)

const (
	// NodeFieldSelectorKeyNodeName field path use to select the Node.
	NodeFieldSelectorKeyNodeName = "metadata.name"
)

// ReplaceNodeNameNodeAffinity replaces the RequiredDuringSchedulingIgnoredDuringExecution
// NodeAffinity of the given affinity with a new NodeAffinity that selects the given nodeName.
// Note that this function assumes that no NodeAffinity conflicts with the selected nodeName.
func ReplaceNodeNameNodeAffinity(affinity *v1.Affinity, nodename string) *v1.Affinity {
	nodeSelReq := v1.NodeSelectorRequirement{
		Key:      NodeFieldSelectorKeyNodeName,
		Operator: v1.NodeSelectorOpIn,
		Values:   []string{nodename},
	}

	nodeSelector := &v1.NodeSelector{
		NodeSelectorTerms: []v1.NodeSelectorTerm{
			{
				MatchFields: []v1.NodeSelectorRequirement{nodeSelReq},
			},
		},
	}

	if affinity == nil {
		return &v1.Affinity{
			NodeAffinity: &v1.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: nodeSelector,
			},
		}
	}

	if affinity.NodeAffinity == nil {
		affinity.NodeAffinity = &v1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: nodeSelector,
		}

		return affinity
	}

	nodeAffinity := affinity.NodeAffinity

	if nodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution == nil {
		nodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution = nodeSelector

		return affinity
	}

	// Build new NodeSelectorTerm list to add the Node name field selector
	newSelectorTerms := []v1.NodeSelectorTerm{}
	for _, term := range nodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms {
		newTerm := term.DeepCopy()

		if len(newTerm.MatchFields) == 0 {
			newTerm.MatchFields = []v1.NodeSelectorRequirement{nodeSelReq}
		} else {
			keyNodeNameFound := false
			for id, matchField := range newTerm.MatchFields {
				if matchField.Key == NodeFieldSelectorKeyNodeName {
					newTerm.MatchFields[id] = nodeSelReq
					keyNodeNameFound = true
				}
			}
			if !keyNodeNameFound {
				newTerm.MatchFields = append(newTerm.MatchFields, nodeSelReq)
			}
		}
		newSelectorTerms = append(newSelectorTerms, *newTerm)
	}
	nodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms = newSelectorTerms

	return affinity
}

// GetNodeNameFromAffinity return the Node name from the pod affinity configuration.
func GetNodeNameFromAffinity(affinity *v1.Affinity) string {
	if affinity == nil {
		return ""
	}

	if affinity.NodeAffinity != nil && affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution != nil {
		selector := affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution
		for _, term := range selector.NodeSelectorTerms {
			for _, field := range term.MatchFields {
				if field.Key == NodeFieldSelectorKeyNodeName && len(field.Values) > 0 {
					return field.Values[0]
				}
			}
		}
	}

	return ""
}
