// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package override

import (
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
)

// RequiredComponents overrides a feature.RequiredComponents according to the given override options
func RequiredComponents(reqComponents *feature.RequiredComponents, overrides map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride) {
	for component, overrideOpts := range overrides {
		if apiutils.BoolValue(overrideOpts.Disabled) {
			switch component {
			case v2alpha1.NodeAgentComponentName:
				reqComponents.Agent.IsRequired = apiutils.NewBoolPointer(false)
			case v2alpha1.ClusterAgentComponentName:
				reqComponents.ClusterAgent.IsRequired = apiutils.NewBoolPointer(false)
			case v2alpha1.ClusterChecksRunnerComponentName:
				reqComponents.ClusterChecksRunner.IsRequired = apiutils.NewBoolPointer(false)
			}
		}
	}
}
