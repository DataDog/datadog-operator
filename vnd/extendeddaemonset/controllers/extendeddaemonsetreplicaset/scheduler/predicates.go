// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// Package scheduler contains helper function around Daemonset pod scheduling.
package scheduler

import (
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"

	podaffinity "github.com/DataDog/extendeddaemonset/pkg/controller/utils/affinity"
)

// CheckNodeFitness runs a set of predicates that select candidate nodes for the DaemonSet;
// the predicates include:
//   - PodMatchNodeSelector: checks pod's NodeSelector and NodeAffinity against node
//   - PodToleratesNodeTaints: exclude tainted node unless pod has specific toleration
func CheckNodeFitness(logger logr.Logger, pod *corev1.Pod, node *corev1.Node) bool {
	// Check pod node selector
	// Check if node.Labels match pod.Spec.NodeSelector.
	if !checkNodeSelector(pod, node) {
		logger.V(1).Info("CheckNodeFitness return false", "reason", "node selector missmatch")

		return false
	}

	if !checkPodToleratesNodeTaints(pod, node) {
		logger.V(1).Info("CheckNodeFitness return false", "reason", "node taints")

		return false
	}

	return true
}

func checkNodeSelector(pod *corev1.Pod, node *corev1.Node) bool {
	if len(pod.Spec.NodeSelector) > 0 {
		selector := labels.SelectorFromSet(pod.Spec.NodeSelector)
		if !selector.Matches(labels.Set(node.Labels)) {
			return false
		}
	}

	// check node affinity
	nodeAffinityMatches := true

	if affinity := pod.Spec.Affinity; affinity != nil && affinity.NodeAffinity != nil {
		nodeAffinity := affinity.NodeAffinity
		if nodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution == nil {
			return true
		}

		// Match node selector for requiredDuringSchedulingIgnoredDuringExecution.
		if nodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution != nil {
			nodeSelectorTerms := nodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms
			nodeAffinityMatches = nodeAffinityMatches && nodeMatchesNodeSelectorTerms(node, nodeSelectorTerms)
		}
	}

	return nodeAffinityMatches
}

func checkPodToleratesNodeTaints(pod *corev1.Pod, node *corev1.Node) bool {
	filter := func(t *corev1.Taint) bool {
		return t.Effect == corev1.TaintEffectNoSchedule || t.Effect == corev1.TaintEffectNoExecute
	}

	return TolerationsTolerateTaintsWithFilter(pod.Spec.Tolerations, node.Spec.Taints, filter)
}

// nodeMatchesNodeSelectorTerms checks if a node's labels satisfy a list of node selector terms,
// terms are ORed, and an empty list of terms will match nothing.
func nodeMatchesNodeSelectorTerms(node *corev1.Node, nodeSelectorTerms []corev1.NodeSelectorTerm) bool {
	nodeFields := map[string]string{
		podaffinity.NodeFieldSelectorKeyNodeName: node.Name,
	}

	return MatchNodeSelectorTerms(nodeSelectorTerms, labels.Set(node.Labels), fields.Set(nodeFields))
}

type taintsFilterFunc func(*corev1.Taint) bool

// TolerationsTolerateTaintsWithFilter checks if given tolerations tolerates
// all the taints that apply to the filter in given taint list.
func TolerationsTolerateTaintsWithFilter(tolerations []corev1.Toleration, taints []corev1.Taint, applyFilter taintsFilterFunc) bool {
	if len(taints) == 0 {
		return true
	}

	for i := range taints {
		if applyFilter != nil && !applyFilter(&taints[i]) {
			continue
		}

		if !TolerationsTolerateTaint(tolerations, &taints[i]) {
			return false
		}
	}

	return true
}

// TolerationsTolerateTaint checks if taint is tolerated by any of the tolerations.
func TolerationsTolerateTaint(tolerations []corev1.Toleration, taint *corev1.Taint) bool {
	for i := range tolerations {
		if tolerations[i].ToleratesTaint(taint) {
			return true
		}
	}

	return false
}

// MatchNodeSelectorTerms checks whether the node labels and fields match node selector terms in ORed;
// nil or empty term matches no objects.
func MatchNodeSelectorTerms(
	nodeSelectorTerms []corev1.NodeSelectorTerm,
	nodeLabels, nodeFields labels.Labels,
) bool {
	for _, req := range nodeSelectorTerms {
		// nil or empty term selects no objects
		if len(req.MatchExpressions) == 0 && len(req.MatchFields) == 0 {
			continue
		}

		if len(req.MatchExpressions) != 0 {
			labelSelector, err := NodeSelectorRequirementsAsSelector(req.MatchExpressions)
			if err != nil || !labelSelector.Matches(nodeLabels) {
				continue
			}
		}

		if len(req.MatchFields) != 0 {
			fieldSelector, err := NodeSelectorRequirementsAsFieldSelector(req.MatchFields)
			if err != nil || !fieldSelector.Matches(nodeFields) {
				continue
			}
		}

		return true
	}

	return false
}

// NodeSelectorRequirementsAsSelector converts the []NodeSelectorRequirement api type into a struct that implements
// labels.Selector.
func NodeSelectorRequirementsAsSelector(nsm []corev1.NodeSelectorRequirement) (labels.Selector, error) {
	if len(nsm) == 0 {
		return labels.Nothing(), nil
	}
	selector := labels.NewSelector()
	for _, expr := range nsm {
		var op selection.Operator

		switch expr.Operator {
		case corev1.NodeSelectorOpIn:
			op = selection.In
		case corev1.NodeSelectorOpNotIn:
			op = selection.NotIn
		case corev1.NodeSelectorOpExists:
			op = selection.Exists
		case corev1.NodeSelectorOpDoesNotExist:
			op = selection.DoesNotExist
		case corev1.NodeSelectorOpGt:
			op = selection.GreaterThan
		case corev1.NodeSelectorOpLt:
			op = selection.LessThan
		default:
			return nil, fmt.Errorf("%q is not a valid node selector operator", expr.Operator)
		}
		r, err := labels.NewRequirement(expr.Key, op, expr.Values)
		if err != nil {
			return nil, err
		}
		selector = selector.Add(*r)
	}

	return selector, nil
}

// NodeSelectorRequirementsAsFieldSelector converts the []NodeSelectorRequirement core type into a struct that implements
// fields.Selector.
func NodeSelectorRequirementsAsFieldSelector(nsm []corev1.NodeSelectorRequirement) (fields.Selector, error) {
	if len(nsm) == 0 {
		return fields.Nothing(), nil
	}

	selectors := []fields.Selector{}
	for _, expr := range nsm {
		switch expr.Operator {
		case corev1.NodeSelectorOpIn:
			if len(expr.Values) != 1 {
				return nil, fmt.Errorf("unexpected number of value (%d) for node field selector operator %q",
					len(expr.Values), expr.Operator)
			}
			selectors = append(selectors, fields.OneTermEqualSelector(expr.Key, expr.Values[0]))

		case corev1.NodeSelectorOpNotIn:
			if len(expr.Values) != 1 {
				return nil, fmt.Errorf("unexpected number of value (%d) for node field selector operator %q",
					len(expr.Values), expr.Operator)
			}
			selectors = append(selectors, fields.OneTermNotEqualSelector(expr.Key, expr.Values[0]))

		default:
			return nil, fmt.Errorf("%q is not a valid node field selector operator", expr.Operator)
		}
	}

	return fields.AndSelectors(selectors...), nil
}
