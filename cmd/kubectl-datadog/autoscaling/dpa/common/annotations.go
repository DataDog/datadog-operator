// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package common holds the logic shared by the dpa commands.
package common

import (
	"encoding/json"
	"strings"

	corev1 "k8s.io/api/core/v1"

	datadoghqcommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"
)

// MinClusterAgentVersion is the first Cluster Agent release that supports the
// annotations below.
const MinClusterAgentVersion = "7.85.0"

// Must match datadog-agent pkg/clusteragent/autoscaling/workload/model/const.go.
const (
	// PauseAnnotationKey ("true") stops the autoscaler from applying anything.
	PauseAnnotationKey = "autoscaling.datadoghq.com/pause"

	// ForceFallbackAnnotationKey ("true") triggers the local fallback as if
	// recommendations were stale. "false" does not disable the fallback.
	ForceFallbackAnnotationKey = "autoscaling.datadoghq.com/force-fallback"

	// ForceReplicasAnnotationKey (e.g. "28") pins the replica count.
	ForceReplicasAnnotationKey = "autoscaling.datadoghq.com/force-replicas"

	// ForceResourcesAnnotationKey overrides container resources. Its value is a
	// JSON list of container resources, as in the DPA.
	ForceResourcesAnnotationKey = "autoscaling.datadoghq.com/force-resources"
)

// How the Cluster Agent reports the annotations in the status. Must match
// datadog-agent pkg/clusteragent/autoscaling/workload: model/const.go,
// model/pod_autoscaler.go BuildStatus, controller.go getActiveScalingSources,
// controller_horizontal.go performScaling and controller_vertical_helpers.go
// applyForcedResources.
const (
	// LocallyPausedReason is the reason of the Active=False condition while paused.
	LocallyPausedReason = "LocallyPaused"
	// ForcedByAnnotationReason is the reason of the VerticalScalingLimited
	// condition while resources are forced.
	ForcedByAnnotationReason = "ForcedByAnnotation"

	// pinnedReplicasPrefix starts the HorizontalScalingLimited message while
	// the replica count is pinned: "replica count pinned to N by the ... annotation".
	pinnedReplicasPrefix = "replica count pinned to "
	// forcedResourcesPrefix starts the VerticalScalingLimited message while
	// resources are forced: "resources overridden for containers a, b by the ... annotation".
	forcedResourcesPrefix = "resources overridden for containers "
)

// PauseConfirmation confirms the pause annotation from the Active condition.
var PauseConfirmation = &Confirmation{
	Name: "Active=False with reason " + LocallyPausedReason,
	Reflects: func(dpa *v1alpha2.DatadogPodAutoscaler, value AnnotationValue) bool {
		cond := findCondition(dpa, datadoghqcommon.DatadogPodAutoscalerActiveCondition)
		paused := cond != nil && cond.Status == corev1.ConditionFalse && cond.Reason == LocallyPausedReason
		return paused == !value.Remove
	},
}

// ForceFallbackConfirmation confirms the force-fallback annotation from the
// horizontal target source; there is no condition for it. It only confirms a
// set: after a removal, the source stays Local while recommendations from
// Datadog are stale.
var ForceFallbackConfirmation = &Confirmation{
	Name: "status.horizontal.target.source " + string(datadoghqcommon.DatadogPodAutoscalerLocalValueSource),
	Reflects: func(dpa *v1alpha2.DatadogPodAutoscaler, value AnnotationValue) bool {
		local := dpa.Status.Horizontal != nil && dpa.Status.Horizontal.Target != nil &&
			dpa.Status.Horizontal.Target.Source == datadoghqcommon.DatadogPodAutoscalerLocalValueSource
		return local == !value.Remove
	},
	NotReported: "The local recommender only becomes the source once it has fresh values, and never when spec.fallback.horizontal.enabled is false or the applyPolicy disables both scaling directions.",
	// The annotation is applied even while the local recommender has no fresh values.
	Advisory: true,
}

// ForceReplicasConfirmation confirms the force-replicas annotation from the
// HorizontalScalingLimited condition, which carries the pinned count.
var ForceReplicasConfirmation = &Confirmation{
	Name: "HorizontalScalingLimited with the pinned replica count",
	Reflects: func(dpa *v1alpha2.DatadogPodAutoscaler, value AnnotationValue) bool {
		message := trueConditionMessage(dpa, datadoghqcommon.DatadogPodAutoscalerHorizontalScalingLimitedCondition)
		if value.Remove {
			return !strings.HasPrefix(message, pinnedReplicasPrefix)
		}
		return strings.HasPrefix(message, pinnedReplicasPrefix+value.Value+" ")
	},
	NotReported: "The pinned count is only reported while it is applied: not while paused, in Preview mode, or with both scaling directions disabled.",
}

// ForceResourcesConfirmation confirms the force-resources annotation from the
// VerticalScalingLimited condition, which lists the forced containers in the
// annotation order. A change of values on the same containers is not visible.
var ForceResourcesConfirmation = &Confirmation{
	Name: "VerticalScalingLimited with reason " + ForcedByAnnotationReason,
	Reflects: func(dpa *v1alpha2.DatadogPodAutoscaler, value AnnotationValue) bool {
		cond := findCondition(dpa, datadoghqcommon.DatadogPodAutoscalerVerticalScalingLimitedCondition)
		forced := cond != nil && cond.Status == corev1.ConditionTrue && cond.Reason == ForcedByAnnotationReason
		if value.Remove {
			return !forced
		}
		var containers []struct {
			Name string `json:"name"`
		}
		if !forced || json.Unmarshal([]byte(value.Value), &containers) != nil {
			return false
		}
		names := make([]string, 0, len(containers))
		for _, c := range containers {
			names = append(names, c.Name)
		}
		return strings.HasPrefix(cond.Message, forcedResourcesPrefix+strings.Join(names, ", ")+" by the ")
	},
	NotReported: "Forced resources are only reported while the DatadogPodAutoscaler has a vertical recommendation.",
}

func findCondition(dpa *v1alpha2.DatadogPodAutoscaler, conditionType datadoghqcommon.DatadogPodAutoscalerConditionType) *datadoghqcommon.DatadogPodAutoscalerCondition {
	for i := range dpa.Status.Conditions {
		if dpa.Status.Conditions[i].Type == conditionType {
			return &dpa.Status.Conditions[i]
		}
	}
	return nil
}

// trueConditionMessage returns the message of the condition if it is True, "" otherwise.
func trueConditionMessage(dpa *v1alpha2.DatadogPodAutoscaler, conditionType datadoghqcommon.DatadogPodAutoscalerConditionType) string {
	if cond := findCondition(dpa, conditionType); cond != nil && cond.Status == corev1.ConditionTrue {
		return cond.Message
	}
	return ""
}
