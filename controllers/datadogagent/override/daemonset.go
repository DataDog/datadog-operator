// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package override

import (
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	edsv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
	v1 "k8s.io/api/apps/v1"
)

// DaemonSet overrides a DaemonSet according to the given override options
func DaemonSet(daemonSet *v1.DaemonSet, override *v2alpha1.DatadogAgentComponentOverride) {
	if override.Name != nil && *override.Name != "" {
		daemonSet.Name = *override.Name
	}
	if override.Strategy != nil {
		if override.Strategy.Type == "RollingUpdate" {
			daemonSet.Spec.UpdateStrategy.RollingUpdate = &v1.RollingUpdateDaemonSet{
				MaxUnavailable: override.Strategy.RollingUpdate.MaxUnavailable,
				MaxSurge:       override.Strategy.RollingUpdate.MaxSurge,
			}
		}
		daemonSet.Spec.UpdateStrategy.Type = v1.DaemonSetUpdateStrategyType(override.Strategy.Type)
	}
}

// ExtendedDaemonSet overrides an ExtendedDaemonSet according to the given override options
func ExtendedDaemonSet(eds *edsv1alpha1.ExtendedDaemonSet, override *v2alpha1.DatadogAgentComponentOverride) {
	if override.Name != nil && *override.Name != "" {
		eds.Name = *override.Name
	}
}
