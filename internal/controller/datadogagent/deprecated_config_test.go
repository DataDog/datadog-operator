// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/appsec"
)

func ddaWithAnnotations(annotations map[string]string) *datadoghqv2alpha1.DatadogAgent {
	return &datadoghqv2alpha1.DatadogAgent{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dda",
			Namespace:   "datadog",
			Annotations: annotations,
		},
	}
}

func TestSetDeprecatedConfigStatus(t *testing.T) {
	now := metav1.NewTime(metav1.Now().Time)

	t.Run("appsec annotation present sets the condition", func(t *testing.T) {
		dda := ddaWithAnnotations(map[string]string{appsec.AnnotationInjectorEnabled: "true"})
		status := &datadoghqv2alpha1.DatadogAgentStatus{}

		setDeprecatedConfigStatus(dda, status, now)

		cond := meta.FindStatusCondition(status.Conditions, common.DeprecatedConfigInUseConditionType)
		require.NotNil(t, cond, "condition must be set while a deprecated surface is in use")
		assert.Equal(t, metav1.ConditionTrue, cond.Status)
		assert.Contains(t, cond.Message, "agent.datadoghq.com/appsec.* annotations")
		assert.Contains(t, cond.Message, appsec.DeprecatedConfigReplacement,
			"the message must name the replacement so the user knows where to migrate")
	})

	t.Run("no deprecated config leaves no condition behind", func(t *testing.T) {
		dda := ddaWithAnnotations(nil)
		status := &datadoghqv2alpha1.DatadogAgentStatus{}

		setDeprecatedConfigStatus(dda, status, now)

		assert.Nil(t, meta.FindStatusCondition(status.Conditions, common.DeprecatedConfigInUseConditionType),
			"a DatadogAgent that never used a deprecated surface must not carry the condition")
		assert.Empty(t, status.Conditions)
	})

	t.Run("condition is removed once the annotations are gone", func(t *testing.T) {
		// The migration path: the condition appears, then must disappear rather than
		// linger as a False copy on every migrated DatadogAgent forever.
		dda := ddaWithAnnotations(map[string]string{appsec.AnnotationInjectorEnabled: "true"})
		status := &datadoghqv2alpha1.DatadogAgentStatus{}
		setDeprecatedConfigStatus(dda, status, now)
		require.NotNil(t, meta.FindStatusCondition(status.Conditions, common.DeprecatedConfigInUseConditionType))

		setDeprecatedConfigStatus(ddaWithAnnotations(nil), status, now)

		assert.Nil(t, meta.FindStatusCondition(status.Conditions, common.DeprecatedConfigInUseConditionType),
			"condition must be deleted, not flipped to False")
	})

	t.Run("unrelated conditions are preserved", func(t *testing.T) {
		status := &datadoghqv2alpha1.DatadogAgentStatus{
			Conditions: []metav1.Condition{{
				Type:               common.DatadogAgentReconcileErrorConditionType,
				Status:             metav1.ConditionFalse,
				Reason:             "DatadogAgent_reconcile_ok",
				LastTransitionTime: now,
			}},
		}

		// Deleting the deprecation condition must not disturb its neighbours.
		setDeprecatedConfigStatus(ddaWithAnnotations(nil), status, now)

		require.Len(t, status.Conditions, 1)
		assert.Equal(t, common.DatadogAgentReconcileErrorConditionType, status.Conditions[0].Type)
	})

	t.Run("repeated reconciles produce a stable message", func(t *testing.T) {
		dda := ddaWithAnnotations(map[string]string{appsec.AnnotationInjectorEnabled: "true"})
		status := &datadoghqv2alpha1.DatadogAgentStatus{}

		setDeprecatedConfigStatus(dda, status, now)
		first := meta.FindStatusCondition(status.Conditions, common.DeprecatedConfigInUseConditionType).Message

		for range 50 {
			setDeprecatedConfigStatus(dda, status, now)
		}

		got := meta.FindStatusCondition(status.Conditions, common.DeprecatedConfigInUseConditionType)
		assert.Equal(t, first, got.Message, "message must not churn the status across reconciles")
		assert.Len(t, status.Conditions, 1, "the condition must not be duplicated")
	})

	t.Run("nil inputs are ignored", func(t *testing.T) {
		assert.NotPanics(t, func() {
			setDeprecatedConfigStatus(nil, &datadoghqv2alpha1.DatadogAgentStatus{}, now)
			setDeprecatedConfigStatus(ddaWithAnnotations(nil), nil, now)
		})
	})
}
