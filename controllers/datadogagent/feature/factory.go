// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package feature

import (
	"fmt"
	"sort"
	"sync"

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
		return fmt.Errorf("the Feature %d registered already", id)
	}
	featureBuilders[id] = buildFunc
	return nil
}

// BuildFeatures use to build a list features depending of the v1alpha1.DatadogAgent instance
func BuildFeatures(dda *v2alpha1.DatadogAgent, options *Options) ([]Feature, error) {
	builderMutex.RLock()
	defer builderMutex.RUnlock()

	var output []Feature

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
		if enabled := feat.Configure(dda); enabled {
			output = append(output, feat)
		}
	}

	return output, nil
}

// BuildFeaturesV1 use to build a list features depending of the v1alpha1.DatadogAgent instance
func BuildFeaturesV1(dda *v1alpha1.DatadogAgent, options *Options) ([]Feature, error) {
	builderMutex.RLock()
	defer builderMutex.RUnlock()

	var output []Feature

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
		options.Logger.Info("test", "feature", id)
		// only add feat to the output if the feature is enabled
		if enabled := feat.ConfigureV1(dda); enabled {
			output = append(output, feat)
		}
	}

	return output, nil
}

var (
	featureBuilders map[IDType]BuildFunc
	builderMutex    sync.RWMutex
)
