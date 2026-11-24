// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentprofile

import (
	"context"
	"slices"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

// LegacyProfileLabelInUse reports whether an operator-managed DaemonSet still
// selects nodes with the legacy shared profile label
// (agent.datadoghq.com/datadogagentprofile). DaemonSets rendered before the
// namespace-scoped label existed use it. Removing the label from nodes while
// they exist makes the DaemonSet controller delete every profile pod at once,
// ignoring maxUnavailable, and schedule default pods on profile nodes.
//
// ignore, when set, skips DaemonSets that are about to be deleted, so that
// they don't keep the label on nodes forever.
func LegacyProfileLabelInUse(ctx context.Context, c client.Reader, ignore func(*appsv1.DaemonSet) bool) (bool, error) {
	dsList := &appsv1.DaemonSetList{}
	if err := c.List(ctx, dsList, client.MatchingLabels{kubernetes.AppKubernetesManageByLabelKey: "datadog-operator"}); err != nil {
		return false, err
	}
	return slices.ContainsFunc(dsList.Items, func(ds appsv1.DaemonSet) bool {
		if ignore != nil && ignore(&ds) {
			return false
		}
		return affinityUsesNodeLabel(ds.Spec.Template.Spec.Affinity, constants.ProfileLabelKey)
	}), nil
}

func affinityUsesNodeLabel(affinity *corev1.Affinity, key string) bool {
	if affinity == nil || affinity.NodeAffinity == nil {
		return false
	}
	usesKey := func(term corev1.NodeSelectorTerm) bool {
		return slices.ContainsFunc(term.MatchExpressions, func(req corev1.NodeSelectorRequirement) bool {
			return req.Key == key
		})
	}
	if required := affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution; required != nil {
		if slices.ContainsFunc(required.NodeSelectorTerms, usesKey) {
			return true
		}
	}
	return slices.ContainsFunc(affinity.NodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution, func(term corev1.PreferredSchedulingTerm) bool {
		return usesKey(term.Preference)
	})
}
