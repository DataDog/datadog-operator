// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package feature

import (
	"fmt"
	"sort"
	"sync"

	"github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
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

// BuildFeatures use to build a list features depending of the v2alpha1.DatadogAgent instance
func BuildFeatures(dda *v2alpha1.DatadogAgent, options *Options, disabledComponents RequiredComponents) ([]Feature, []Feature, RequiredComponents) {
	builderMutex.RLock()
	defer builderMutex.RUnlock()

	var configuredFeatures []Feature
	var enabledFeatures []Feature
	var requiredComponents RequiredComponents

	// to always return in feature in the same order we need to sort the map keys
	sortedkeys := make([]IDType, 0, len(featureBuilders))
	for key := range featureBuilders {
		sortedkeys = append(sortedkeys, key)
	}
	sort.Slice(sortedkeys, func(i, j int) bool {
		return sortedkeys[i] < sortedkeys[j]
	})

	for _, id := range sortedkeys {
		feat := featureBuilders[id](options)
		featureID := feat.ID()
		reqComponents := feat.Configure(dda)
		// merge disabled components into feature components
		reqComponents.Merge(&disabledComponents, IgnoreNilRequiredComponentsMergeFunction)
		if reqComponents.IsEnabled() {
			// enabled features
			enabledFeatures = append(enabledFeatures, feat)
			options.Logger.V(1).Info("Feature enabled", "featureID", featureID)
		} else if reqComponents.IsConfigured() {
			// disabled, but still possibly needing configuration features
			configuredFeatures = append(configuredFeatures, feat)
			options.Logger.V(1).Info("Feature configured", "featureID", featureID)
		}
		// if reqComponents.IsConfigured() {
		// 	// enabled features + disabled due to a component being disabled, but still possibly needing configuration features
		// 	configuredFeatures = append(configuredFeatures, feat)
		// 	if reqComponents.IsEnabled() {
		// 		// enabled features
		// 		enabledFeatures = append(enabledFeatures, feat)
		// 		options.Logger.V(1).Info("Feature enabled", "featureID", featureID)
		// 	} else {
		// 		options.Logger.V(1).Info("Feature configured", "featureID", featureID)
		// 	}
		// }
		requiredComponents.Merge(&reqComponents, DefaultRequiredComponentsMergeFunction)
	}

	if dda.Spec.Global != nil &&
		dda.Spec.Global.ContainerStrategy != nil &&
		*dda.Spec.Global.ContainerStrategy == v2alpha1.SingleContainerStrategy &&
		// All features that need the NodeAgent must include it in their RequiredComponents;
		// otherwise tests will fail when checking `requiredComponents.Agent.IsPrivileged()`.
		requiredComponents.Agent.IsEnabled() &&
		!requiredComponents.Agent.IsPrivileged() {

		requiredComponents.Agent.Containers = []common.AgentContainerName{common.UnprivilegedSingleAgentContainerName}
		return configuredFeatures, enabledFeatures, requiredComponents
	}
	return configuredFeatures, enabledFeatures, requiredComponents
}

var (
	featureBuilders map[IDType]BuildFunc
	builderMutex    sync.RWMutex
)
