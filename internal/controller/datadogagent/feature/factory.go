// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package feature

import (
	"fmt"
	"slices"
	"sync"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/composition"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

func init() {
	featureBuilders = map[IDType]BuildFunc{}
}

// Register use to register a Feature to the Feature factory.
func Register(id IDType, buildFunc BuildFunc) error {
	builderMutex.Lock()
	defer builderMutex.Unlock()

	if _, found := featureBuilders[id]; found {
		return fmt.Errorf("the Feature %s is registered already", id)
	}
	featureBuilders[id] = buildFunc
	return nil
}

// BuildFeatures use to build a list features depending of the v2alpha1.DatadogAgent instance.
// It also returns support level of each enabled feature for a given provider, and the enabled
// features dropped because the node Agent mode disallows what they require.
// The caller enforces it (block on Rejected, warn on Degraded); this
// function stays side-effect-free.
func BuildFeatures(dda metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec, ddaRCStatus *v2alpha1.RemoteConfigConfiguration, options *Options) ([]Feature, []Feature, RequiredComponents, []ProviderSupportResult, []DroppedFeature) {
	builderMutex.RLock()
	defer builderMutex.RUnlock()

	var configuredFeatures []Feature
	var enabledFeatures []Feature
	var requiredComponents RequiredComponents
	var enabledFeatureIDs []IDType
	var configuredFeatureIDs []IDType
	var droppedFeatures []DroppedFeature
	var droppedFeatureIDs []IDType
	policy := composition.PolicyOf(dda)

	// to always return in feature in the same order we need to sort the map keys
	sortedkeys := make([]IDType, 0, len(featureBuilders))
	for key := range featureBuilders {
		sortedkeys = append(sortedkeys, key)
	}
	slices.Sort(sortedkeys)

	for _, id := range sortedkeys {
		feat := featureBuilders[id](options)
		reqComponents := feat.Configure(dda, ddaSpec, ddaRCStatus)
		// Features requiring what the mode disallows are neither managed nor merged.
		// Only enabled ones are reported; configured-only ones are dropped silently.
		if reasons := policyViolations(policy, reqComponents); len(reasons) > 0 {
			droppedFeatureIDs = append(droppedFeatureIDs, feat.ID())
			if reqComponents.IsEnabled() {
				droppedFeatures = append(droppedFeatures, DroppedFeature{ID: feat.ID(), Reasons: reasons})
			}
			continue
		}
		if reqComponents.IsEnabled() {
			// enabled features
			enabledFeatures = append(enabledFeatures, feat)
			enabledFeatureIDs = append(enabledFeatureIDs, feat.ID())
		} else if reqComponents.IsConfigured() {
			// disabled, but still possibly needing configuration features
			configuredFeatures = append(configuredFeatures, feat)
			configuredFeatureIDs = append(configuredFeatureIDs, feat.ID())
		}
		requiredComponents.Merge(&reqComponents)
	}

	options.Logger.V(1).Info("Enabled features", "features", enabledFeatureIDs)
	options.Logger.V(1).Info("Configured features", "features", configuredFeatureIDs)
	if len(droppedFeatureIDs) > 0 {
		options.Logger.V(1).Info("Features dropped by node Agent mode", "mode", policy.Mode, "features", droppedFeatureIDs)
	}

	// Enabled features the instance's provider does not fully support (Rejected or Degraded),
	// read from the provider annotation. Only enabled features are considered — a
	// configured-but-disabled feature does not restrict the provider.
	unsupportedFeatures := EvaluateProviderSupport(enabledFeatures, dda.GetAnnotations()[kubernetes.ProviderAnnotationKey])

	if !policy.Composed() &&
		ddaSpec.Global != nil &&
		ddaSpec.Global.ContainerStrategy != nil &&
		*ddaSpec.Global.ContainerStrategy == v2alpha1.SingleContainerStrategy &&
		// All features that need the NodeAgent must include it in their RequiredComponents;
		// otherwise tests will fail when checking `requiredComponents.Agent.IsPrivileged()`.
		requiredComponents.Agent.IsEnabled() &&
		!requiredComponents.Agent.IsPrivileged() {

		requiredComponents.Agent.Containers = []common.AgentContainerName{common.UnprivilegedSingleAgentContainerName}
	}
	return configuredFeatures, enabledFeatures, requiredComponents, unsupportedFeatures, droppedFeatures
}

var (
	featureBuilders map[IDType]BuildFunc
	builderMutex    sync.RWMutex
)
