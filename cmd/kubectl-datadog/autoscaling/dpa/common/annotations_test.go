// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"

	datadoghqcommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"
)

// TestContractStrings pins the strings shared with the Cluster Agent: a typo
// silently turns every command into a no-op.
func TestContractStrings(t *testing.T) {
	assert.Equal(t, "autoscaling.datadoghq.com/pause", PauseAnnotationKey)
	assert.Equal(t, "autoscaling.datadoghq.com/force-fallback", ForceFallbackAnnotationKey)
	assert.Equal(t, "autoscaling.datadoghq.com/force-replicas", ForceReplicasAnnotationKey)
	assert.Equal(t, "autoscaling.datadoghq.com/force-resources", ForceResourcesAnnotationKey)
	assert.Equal(t, "LocallyPaused", LocallyPausedReason)
	assert.Equal(t, "ForcedByAnnotation", ForcedByAnnotationReason)
}

const (
	active     = datadoghqcommon.DatadogPodAutoscalerActiveCondition
	hLimited   = datadoghqcommon.DatadogPodAutoscalerHorizontalScalingLimitedCondition
	vLimited   = datadoghqcommon.DatadogPodAutoscalerVerticalScalingLimitedCondition
	pausedMsg  = "Autoscaling locally paused by the autoscaling.datadoghq.com/pause annotation"
	pinned28   = "replica count pinned to 28 by the autoscaling.datadoghq.com/force-replicas annotation"
	forcedApp  = "resources overridden for containers app by the autoscaling.datadoghq.com/force-resources annotation"
	forcedBoth = "resources overridden for containers app, sidecar by the autoscaling.datadoghq.com/force-resources annotation"
)

// annotationValue maps "" to a removal, for compact test tables.
func annotationValue(v string) AnnotationValue {
	if v == "" {
		return Removed
	}
	return Set(v)
}

// The messages below are copied from datadog-agent.
func TestConfirmations(t *testing.T) {
	cond := func(conditionType datadoghqcommon.DatadogPodAutoscalerConditionType, status corev1.ConditionStatus, reason, message string) *v1alpha2.DatadogPodAutoscaler {
		return withCondition(newDPA("ns", "a", nil, nil), conditionType, status, reason, message)
	}
	source := func(src datadoghqcommon.DatadogPodAutoscalerValueSource) *v1alpha2.DatadogPodAutoscaler {
		dpa := newDPA("ns", "a", nil, nil)
		dpa.Status.Horizontal = &datadoghqcommon.DatadogPodAutoscalerHorizontalStatus{
			Target: &datadoghqcommon.DatadogPodAutoscalerHorizontalRecommendation{Source: src},
		}
		return dpa
	}
	const appValue = `[{"name":"app","requests":{"cpu":"2"}}]`
	const bothValue = `[{"name":"app","requests":{"cpu":"2"}},{"name":"sidecar","limits":{"memory":"1Gi"}}]`

	for _, tc := range []struct {
		name         string
		confirmation *Confirmation
		dpa          *v1alpha2.DatadogPodAutoscaler
		value        string
		want         bool
	}{
		{"pause: paused", PauseConfirmation, cond(active, corev1.ConditionFalse, LocallyPausedReason, pausedMsg), "true", true},
		{"pause: active", PauseConfirmation, cond(active, corev1.ConditionTrue, "", ""), "true", false},
		{"pause: inactive for another reason", PauseConfirmation, cond(active, corev1.ConditionFalse, "", "Target has been scaled to 0 replicas"), "true", false},
		{"pause: no status", PauseConfirmation, newDPA("ns", "a", nil, nil), "true", false},
		{"unpause: still paused", PauseConfirmation, cond(active, corev1.ConditionFalse, LocallyPausedReason, pausedMsg), "", false},
		{"unpause: active", PauseConfirmation, cond(active, corev1.ConditionTrue, "", ""), "", true},

		{"fallback: local source", ForceFallbackConfirmation, source(datadoghqcommon.DatadogPodAutoscalerLocalValueSource), "true", true},
		{"fallback: autoscaling source", ForceFallbackConfirmation, source(datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource), "true", false},
		{"fallback: no horizontal status", ForceFallbackConfirmation, newDPA("ns", "a", nil, nil), "true", false},

		{"replicas: pinned to the count", ForceReplicasConfirmation, cond(hLimited, corev1.ConditionTrue, "", pinned28), "28", true},
		{"replicas: pinned to another count", ForceReplicasConfirmation, cond(hLimited, corev1.ConditionTrue, "", pinned28), "2", false},
		{"replicas: limited by constraints", ForceReplicasConfirmation, cond(hLimited, corev1.ConditionTrue, "", "desired replica count limited to 10 (originally 12) due to max replicas constraint"), "28", false},
		{"replicas: not limited", ForceReplicasConfirmation, cond(hLimited, corev1.ConditionFalse, "", ""), "28", false},
		{"unpin: still pinned", ForceReplicasConfirmation, cond(hLimited, corev1.ConditionTrue, "", pinned28), "", false},
		{"unpin: limited by constraints", ForceReplicasConfirmation, cond(hLimited, corev1.ConditionTrue, "", "desired replica count limited to 10"), "", true},
		{"unpin: not limited", ForceReplicasConfirmation, cond(hLimited, corev1.ConditionFalse, "", ""), "", true},

		{"resources: forced", ForceResourcesConfirmation, cond(vLimited, corev1.ConditionTrue, ForcedByAnnotationReason, forcedApp), appValue, true},
		{"resources: forced and clamped", ForceResourcesConfirmation, cond(vLimited, corev1.ConditionTrue, ForcedByAnnotationReason, forcedApp+"; limited to max"), appValue, true},
		{"resources: forced for both containers", ForceResourcesConfirmation, cond(vLimited, corev1.ConditionTrue, ForcedByAnnotationReason, forcedBoth), bothValue, true},
		{"resources: other containers forced", ForceResourcesConfirmation, cond(vLimited, corev1.ConditionTrue, ForcedByAnnotationReason, forcedApp), bothValue, false},
		{"resources: limited by constraints only", ForceResourcesConfirmation, cond(vLimited, corev1.ConditionTrue, "LimitedByConstraint", "limited"), appValue, false},
		{"resources: not limited", ForceResourcesConfirmation, cond(vLimited, corev1.ConditionFalse, "", ""), appValue, false},
		{"unforce: still forced", ForceResourcesConfirmation, cond(vLimited, corev1.ConditionTrue, ForcedByAnnotationReason, forcedApp), "", false},
		{"unforce: not limited", ForceResourcesConfirmation, cond(vLimited, corev1.ConditionFalse, "", ""), "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.confirmation.Reflects(tc.dpa, annotationValue(tc.value)))
		})
	}
}
