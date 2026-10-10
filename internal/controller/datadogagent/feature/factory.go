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
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

func init() {
	featureBuilders = map[IDType]BuildFunc{}
	featureValidationRules = map[IDType][]common.ValidationRule{}
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
// It also returns support level of each enabled feature for a given provider.
// The caller enforces it (block on Rejected, warn on Degraded); this
// function stays side-effect-free.
func BuildFeatures(dda metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec, ddaRCStatus *v2alpha1.RemoteConfigConfiguration, options *Options) ([]Feature, []Feature, RequiredComponents, []ProviderSupportResult) {
	builderMutex.RLock()
	defer builderMutex.RUnlock()

	var configuredFeatures []Feature
	var enabledFeatures []Feature
	var requiredComponents RequiredComponents
	var enabledFeatureIDs []IDType
	var configuredFeatureIDs []IDType

	// to always return in feature in the same order we need to sort the map keys
	sortedkeys := make([]IDType, 0, len(featureBuilders))
	for key := range featureBuilders {
		sortedkeys = append(sortedkeys, key)
	}
	slices.Sort(sortedkeys)

	for _, id := range sortedkeys {
		feat := featureBuilders[id](options)
		reqComponents := feat.Configure(dda, ddaSpec, ddaRCStatus)
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

	// Enabled features the instance's provider does not fully support (Rejected or Degraded),
	// read from the provider annotation. Only enabled features are considered — a
	// configured-but-disabled feature does not restrict the provider.
	unsupportedFeatures := EvaluateProviderSupport(enabledFeatures, dda.GetAnnotations()[kubernetes.ProviderAnnotationKey])

	if ddaSpec.Global != nil &&
		ddaSpec.Global.ContainerStrategy != nil &&
		*ddaSpec.Global.ContainerStrategy == v2alpha1.SingleContainerStrategy &&
		// All features that need the NodeAgent must include it in their RequiredComponents;
		// otherwise tests will fail when checking `requiredComponents.Agent.IsPrivileged()`.
		requiredComponents.Agent.IsEnabled() &&
		!requiredComponents.Agent.IsPrivileged() {

		requiredComponents.Agent.Containers = []common.AgentContainerName{common.UnprivilegedSingleAgentContainerName}
		return configuredFeatures, enabledFeatures, requiredComponents, unsupportedFeatures
	}
	return configuredFeatures, enabledFeatures, requiredComponents, unsupportedFeatures
}

var (
	featureBuilders map[IDType]BuildFunc
	// featureValidationRules holds each feature's CEL rules, guarded by the
	// same mutex as featureBuilders since both are written from init().
	featureValidationRules map[IDType][]common.ValidationRule
	builderMutex           sync.RWMutex
)

// RegisterValidationRules registers a feature's CEL validation rules, the same
// way Register registers its builder. A feature with no rules calls nothing.
//
// The rules are data; the feature never evaluates them. The DatadogAgent
// controller collects them with ValidationRules and the operator compiles them
// once at startup, so a rule here is evaluated in exactly the same place as a
// non-feature one.
func RegisterValidationRules(id IDType, rules []common.ValidationRule) error {
	builderMutex.Lock()
	defer builderMutex.Unlock()

	if _, found := featureValidationRules[id]; found {
		return fmt.Errorf("validation rules for the Feature %s are registered already", id)
	}
	featureValidationRules[id] = rules
	return nil
}

// ValidationRules returns every registered feature rule, ordered by feature ID
// so the rule set, and so the policy object, is stable between runs.
func ValidationRules() []common.ValidationRule {
	builderMutex.RLock()
	defer builderMutex.RUnlock()

	ids := make([]IDType, 0, len(featureValidationRules))
	total := 0
	for id, rules := range featureValidationRules {
		ids = append(ids, id)
		total += len(rules)
	}
	slices.Sort(ids)

	out := make([]common.ValidationRule, 0, total)
	for _, id := range ids {
		out = append(out, featureValidationRules[id]...)
	}
	return out
}
