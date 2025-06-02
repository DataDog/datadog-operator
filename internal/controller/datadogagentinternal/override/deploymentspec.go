// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package override

import (
	v1 "k8s.io/api/apps/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

// Deployment overrides a v1.Deployment according to the given override options
func Deployment(deployment *v1.Deployment, override *v2alpha1.DatadogAgentComponentOverride) {
	if override.Replicas != nil {
		deployment.Spec.Replicas = override.Replicas
	}

	if override.Name != nil {
		deployment.Name = *override.Name
	}

	if override.UpdateStrategy != nil {
		if override.UpdateStrategy.RollingUpdate != nil {
			rollingUpdate := &v1.RollingUpdateDeployment{}
			if override.UpdateStrategy.RollingUpdate.MaxUnavailable != nil {
				rollingUpdate.MaxUnavailable = override.UpdateStrategy.RollingUpdate.MaxUnavailable
			}
			if override.UpdateStrategy.RollingUpdate.MaxSurge != nil {
				rollingUpdate.MaxSurge = override.UpdateStrategy.RollingUpdate.MaxSurge
			}
			deployment.Spec.Strategy.RollingUpdate = rollingUpdate
		}

		deployment.Spec.Strategy.Type = v1.DeploymentStrategyType(override.UpdateStrategy.Type)
	}
}
