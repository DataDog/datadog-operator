// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package feature

import (
	"fmt"
	"sort"
	"sync"

	"github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
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
func BuildFeatures(dda *v2alpha1.DatadogAgent, options *Options) ([]Feature, RequiredComponents) {
	builderMutex.RLock()
	defer builderMutex.RUnlock()

	var output []Feature
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
		reqComponents := feat.Configure(dda)
		// only add feature to the output if one of the components is configured (but not necessarily required)
		if reqComponents.IsConfigured() {
			output = append(output, feat)
		}
		requiredComponents.Merge(&reqComponents)
	}

	if dda.Spec.Global != nil &&
		dda.Spec.Global.ContainerProcessStrategy != nil &&
		dda.Spec.Global.ContainerProcessStrategy.Type == common.UnprivilegedMultiProcessContainer &&
		// All features that need the NodeAgent must include it in their RequiredComponents;
		// otherwise tests will fail when checking `requiredComponents.Agent.IsPrivileged()`.
		requiredComponents.Agent.IsEnabled() &&
		!requiredComponents.Agent.IsPrivileged() {

		requiredComponents.Agent.Containers = []common.AgentContainerName{common.UnprivilegedMultiProcessAgentContainerName}
		return output, requiredComponents
	}
	return output, requiredComponents
}

// BuildFeaturesV1 use to build a list features depending of the v1alpha1.DatadogAgent instance
func BuildFeaturesV1(dda *v1alpha1.DatadogAgent, options *Options) ([]Feature, RequiredComponents) {
	builderMutex.RLock()
	defer builderMutex.RUnlock()

	var output []Feature
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
		// only add feat to the output if the feature is enabled
		config := feat.ConfigureV1(dda)
		if config.IsEnabled() {
			output = append(output, feat)
		}
		requiredComponents.Merge(&config)
	}

	return output, requiredComponents
}

var (
	featureBuilders map[IDType]BuildFunc
	builderMutex    sync.RWMutex
)
