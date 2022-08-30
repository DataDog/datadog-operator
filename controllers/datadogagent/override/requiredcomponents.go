// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package override

import (
	"time"

	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	overrideConflict   = "OverrideConflict"
	noOverrideConflict = "NoOverrideConflict"
)

// RequiredComponents overrides a feature.RequiredComponents according to the given override options
func RequiredComponents(logger logr.Logger, newStatus *v2alpha1.DatadogAgentStatus, reqComponents *feature.RequiredComponents, overrides map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride) {
	now := metav1.NewTime(time.Now())
	conditionStatus := metav1.ConditionFalse
	reason := noOverrideConflict
	message := ""
	for component, overrideConfig := range overrides {
		if apiutils.BoolValue(overrideConfig.Disabled) {
			switch component {
			case v2alpha1.NodeAgentComponentName:
				if *reqComponents.Agent.IsRequired {
					conditionStatus = metav1.ConditionTrue
					reason = overrideConflict
					message = "Agent component is set to disabled"
				}
				reqComponents.Agent.IsRequired = apiutils.NewBoolPointer(false)
				logger.V(1).Info("The Agent component is set to disabled")
			case v2alpha1.ClusterAgentComponentName:
				if *reqComponents.ClusterAgent.IsRequired {
					conditionStatus = metav1.ConditionTrue
					reason = overrideConflict
					message = "ClusterAgent component is set to disabled"
				}
				reqComponents.ClusterAgent.IsRequired = apiutils.NewBoolPointer(false)
				logger.V(1).Info("The ClusterAgent component is set to disabled")
			case v2alpha1.ClusterChecksRunnerComponentName:
				if *reqComponents.ClusterChecksRunner.IsRequired {
					conditionStatus = metav1.ConditionTrue
					reason = overrideConflict
					message = "ClusterChecksRunner component is set to disabled"
				}
				reqComponents.ClusterChecksRunner.IsRequired = apiutils.NewBoolPointer(false)
				logger.V(1).Info("The ClusterChecksRunner component is set to disabled")
			}
		}
		v2alpha1.UpdateDatadogAgentStatusConditions(
			newStatus,
			now,
			v2alpha1.OverrideReconcileConflictConditionType,
			conditionStatus,
			reason,
			message,
			true,
		)
	}
}
