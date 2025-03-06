// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
)

func (r *Reconciler) buildRequirements(dda *v2alpha1.DatadogAgent) ([]feature.Feature, feature.RequiredComponents) {
	var requiredComponents feature.RequiredComponents
	disabledComponent := feature.RequiredComponent{
		IsRequired: apiutils.NewBoolPointer(false),
	}

	// check for overrides
	if isComponentDisabledInOverride(dda, v2alpha1.ClusterAgentComponentName) {
		requiredComponents.ClusterAgent = disabledComponent
	}
	if isComponentDisabledInOverride(dda, v2alpha1.NodeAgentComponentName) {
		requiredComponents.Agent = disabledComponent
	}
	if isComponentDisabledInOverride(dda, v2alpha1.ClusterChecksRunnerComponentName) {
		requiredComponents.ClusterChecksRunner = disabledComponent
	}

	// check features
	configuredFeatures, reqComponents := feature.BuildFeatures(dda, requiredComponents, reconcilerOptionsToFeatureOptions(&r.options, r.log))

	// merge required components
	requiredComponents.Merge(&reqComponents)

	return configuredFeatures, requiredComponents
}

func isComponentDisabledInOverride(dda *v2alpha1.DatadogAgent, componentName v2alpha1.ComponentName) bool {
	if override, ok := dda.Spec.Override[componentName]; ok {
		return apiutils.BoolValue(override.Disabled)
	}
	return false
}
