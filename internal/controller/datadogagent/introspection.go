// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"sort"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

// applyIntrospection determines which providers are needed and applies provider-specifc configs to ddais.
//
// A provider split can become unneeded between reconciles (e.g. a node drain
// changes a profile's provider mix). Its replacement is created/updated before
// cleanUpUnusedDDAIs removes the old split, so nodes are briefly double-covered
// rather than left uncovered.
func (r *Reconciler) applyIntrospection(nodeList []corev1.Node, nodeToProfile map[string]types.NamespacedName, ddais []*v1alpha1.DatadogAgentInternal) []*v1alpha1.DatadogAgentInternal {
	profileToProviders := computeProfileProviders(nodeList, nodeToProfile)

	appliedDDAIs := make([]*v1alpha1.DatadogAgentInternal, 0, len(ddais))
	for _, ddai := range ddais {
		// ProfileLabelKey is unset for the default profile's DDAI.
		profileName := ddai.Labels[constants.ProfileLabelKey]
		if profileName == "" {
			profileName = "default"
		}
		providers := profileToProviders[profileName]
		if len(providers) == 0 {
			providers = []string{""}
		}

		// Snapshot before any mutation: deriving later copies from ddai itself
		// would copy whatever the first iteration already mutated it into.
		base := ddai.DeepCopy()
		for _, provider := range providers {
			target := ddai
			if provider != "" {
				target = base.DeepCopy()
				// Only the "" split owns the Cluster Agent/CCR/OtelAgentGateway;
				// it always exists (see computeProfileProviders).
				disableComponent(target, v2alpha1.ClusterAgentComponentName)
				disableComponent(target, v2alpha1.ClusterChecksRunnerComponentName)
				disableComponent(target, v2alpha1.OtelAgentGatewayComponentName)
			}
			applyDDAIProvider(target, provider, providers)
			appliedDDAIs = append(appliedDDAIs, target)
		}
	}
	return appliedDDAIs
}

// computeProfileProviders maps each profile name to the providers used by its nodes.
func computeProfileProviders(nodeList []corev1.Node, nodeToProfile map[string]types.NamespacedName) map[string][]string {
	// map[profile]map[provider]struct{}, used to dedupe providers per profile
	profileToProviderSet := make(map[string]map[string]struct{})
	for _, node := range nodeList {
		profile := nodeToProfile[node.Name]
		appliedProviders := profileToProviderSet[profile.Name]
		if appliedProviders == nil {
			// "" is always included so the profile keeps a stable base DDAI
			// (see applyIntrospection) even if every current node is a
			// specific provider.
			appliedProviders = map[string]struct{}{"": {}}
			profileToProviderSet[profile.Name] = appliedProviders
		}

		providers := kubernetes.NodeProviders(node.Labels)
		if len(providers) == 0 {
			providers = []string{""}
		}
		for _, p := range providers {
			appliedProviders[p] = struct{}{}
		}
	}

	// map[profile][]providers
	profileToProviders := make(map[string][]string, len(profileToProviderSet))
	for profileName, appliedProviders := range profileToProviderSet {
		providers := make([]string, 0, len(appliedProviders))
		for p := range appliedProviders {
			providers = append(providers, p)
		}
		// Sort for a stable order; "" (no provider) sorts first.
		sort.Strings(providers)
		profileToProviders[profileName] = providers
	}
	return profileToProviders
}

// applyDDAIProvider scopes ddai to provider: sets its affinity, name, and node provider annotation.
func applyDDAIProvider(ddai *v1alpha1.DatadogAgentInternal, provider string, providers []string) {
	// No provider to apply.
	if provider == "" && len(providers) <= 1 {
		return
	}

	if len(providers) > 1 {
		ensureOverrideExists(ddai, v2alpha1.NodeAgentComponentName)
		override := ddai.Spec.Override[v2alpha1.NodeAgentComponentName]
		override.Affinity = common.MergeAffinities(override.Affinity, providerAffinity(provider, providers))

		if provider != "" {
			ddai.Name += "-" + provider
			if override.Name != nil {
				dsName := *override.Name + "-" + provider
				override.Name = &dsName
			}
		}
	}

	if provider != "" {
		if ddai.Annotations == nil {
			ddai.Annotations = make(map[string]string)
		}
		ddai.Annotations[kubernetes.NodeProviderAnnotationKey] = provider
	}
}

// providerAffinity returns affinity matching provider's nodes, or, if provider
// is "", excluding every other provider's nodes.
func providerAffinity(provider string, providers []string) *corev1.Affinity {
	var matchExpressions []corev1.NodeSelectorRequirement
	if provider != "" {
		rule, ok := kubernetes.GetNodeProviderRule(provider)
		if !ok {
			return nil
		}
		matchExpressions = append(matchExpressions, corev1.NodeSelectorRequirement{
			Key:      rule.LabelKey,
			Operator: corev1.NodeSelectorOpIn,
			Values:   rule.LabelValues,
		})
	} else {
		for _, p := range providers {
			if p == "" {
				continue
			}
			rule, ok := kubernetes.GetNodeProviderRule(p)
			if !ok {
				continue
			}
			matchExpressions = append(matchExpressions, corev1.NodeSelectorRequirement{
				Key:      rule.LabelKey,
				Operator: corev1.NodeSelectorOpNotIn,
				Values:   rule.LabelValues,
			})
		}
	}
	if len(matchExpressions) == 0 {
		return nil
	}
	return &corev1.Affinity{
		NodeAffinity: &corev1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
				NodeSelectorTerms: []corev1.NodeSelectorTerm{
					{
						MatchExpressions: matchExpressions,
					},
				},
			},
		},
	}
}
