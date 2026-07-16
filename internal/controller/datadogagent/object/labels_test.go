// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package object

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

func TestGetDefaultLabels_NoCommonLabels(t *testing.T) {
	dda := &v2alpha1.DatadogAgent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "datadog",
			Namespace: "default",
		},
		Spec: v2alpha1.DatadogAgentSpec{},
	}

	labels := GetDefaultLabels(dda, "datadog", "7.0.0")

	assert.Equal(t, "datadog-agent-deployment", labels[kubernetes.AppKubernetesNameLabelKey])
	assert.Equal(t, "datadog", labels[kubernetes.AppKubernetesInstanceLabelKey])
	assert.Equal(t, "datadog-operator", labels[kubernetes.AppKubernetesManageByLabelKey])
	// No extra labels added
	assert.NotContains(t, labels, "custom-label")
}

func TestGetDefaultLabels_WithCommonLabels_DDA(t *testing.T) {
	dda := &v2alpha1.DatadogAgent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "datadog",
			Namespace: "default",
		},
		Spec: v2alpha1.DatadogAgentSpec{
			Global: &v2alpha1.GlobalConfig{
				CommonLabels: map[string]string{
					"team":           "platform",
					"cost-center":    "ops",
					"app.custom/env": "prod",
				},
			},
		},
	}

	labels := GetDefaultLabels(dda, "datadog", "7.0.0")

	assert.Equal(t, "platform", labels["team"])
	assert.Equal(t, "ops", labels["cost-center"])
	assert.Equal(t, "prod", labels["app.custom/env"])
	// Standard operator labels must still be present
	assert.Equal(t, "datadog-agent-deployment", labels[kubernetes.AppKubernetesNameLabelKey])
	assert.Equal(t, "datadog-operator", labels[kubernetes.AppKubernetesManageByLabelKey])
}

func TestGetDefaultLabels_CommonLabels_CannotOverrideOperatorLabels(t *testing.T) {
	dda := &v2alpha1.DatadogAgent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "datadog",
			Namespace: "default",
		},
		Spec: v2alpha1.DatadogAgentSpec{
			Global: &v2alpha1.GlobalConfig{
				CommonLabels: map[string]string{
					// Attempt to override operator-managed labels
					kubernetes.AppKubernetesManageByLabelKey: "my-operator",
					kubernetes.AppKubernetesNameLabelKey:     "my-name",
					// A custom key that should pass through
					"team": "platform",
				},
			},
		},
	}

	labels := GetDefaultLabels(dda, "datadog", "7.0.0")

	// Operator labels must NOT be overridden
	assert.Equal(t, "datadog-operator", labels[kubernetes.AppKubernetesManageByLabelKey])
	assert.Equal(t, "datadog-agent-deployment", labels[kubernetes.AppKubernetesNameLabelKey])
	// Custom keys that don't conflict are still set
	assert.Equal(t, "platform", labels["team"])
}

func TestGetDefaultLabels_WithCommonLabels_DDAI(t *testing.T) {
	ddai := &v1alpha1.DatadogAgentInternal{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "datadog",
			Namespace: "default",
		},
		Spec: v2alpha1.DatadogAgentSpec{
			Global: &v2alpha1.GlobalConfig{
				CommonLabels: map[string]string{
					"environment": "staging",
				},
			},
		},
	}

	labels := GetDefaultLabels(ddai, "datadog", "7.0.0")

	assert.Equal(t, "staging", labels["environment"])
	assert.Equal(t, "datadog-operator", labels[kubernetes.AppKubernetesManageByLabelKey])
}

func TestGetDefaultLabels_NilGlobal(t *testing.T) {
	dda := &v2alpha1.DatadogAgent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "datadog",
			Namespace: "default",
		},
		Spec: v2alpha1.DatadogAgentSpec{
			Global: nil,
		},
	}

	// Should not panic when Global is nil
	labels := GetDefaultLabels(dda, "datadog", "7.0.0")
	assert.Equal(t, "datadog-operator", labels[kubernetes.AppKubernetesManageByLabelKey])
}

func TestGetDefaultLabels_NonDDAObject(t *testing.T) {
	// A plain ObjectMeta (not DDA/DDAI) should still work without extra labels
	obj := &metav1.ObjectMeta{
		Name:      "other",
		Namespace: "default",
	}

	labels := GetDefaultLabels(obj, "other", "1.0.0")
	assert.Equal(t, "datadog-operator", labels[kubernetes.AppKubernetesManageByLabelKey])
}

func TestGetDefaultLabels_CommonLabels_ReservedPrefixesAreDropped(t *testing.T) {
	dda := &v2alpha1.DatadogAgent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "datadog",
			Namespace: "default",
		},
		Spec: v2alpha1.DatadogAgentSpec{
			Global: &v2alpha1.GlobalConfig{
				CommonLabels: map[string]string{
					// Reserved — must be dropped
					"agent.datadoghq.com/datadogagentprofile": "my-profile",
					"agent.datadoghq.com/name":                "foo",
					"operator.datadoghq.com/managed-by-store": "true",
					"datadoghq.com/custom":                    "value",
					// Non-reserved — must pass through
					"team":        "platform",
					"cost-center": "ops",
				},
			},
		},
	}

	labels := GetDefaultLabels(dda, "datadog", "7.0.0")

	// Reserved keys must not appear
	assert.NotContains(t, labels, "agent.datadoghq.com/datadogagentprofile")
	assert.NotContains(t, labels, "agent.datadoghq.com/name")
	assert.NotContains(t, labels, "operator.datadoghq.com/managed-by-store")
	assert.NotContains(t, labels, "datadoghq.com/custom")
	// Non-reserved keys must be present
	assert.Equal(t, "platform", labels["team"])
	assert.Equal(t, "ops", labels["cost-center"])
}

func TestIsReservedLabelKey(t *testing.T) {
	reserved := []string{
		"agent.datadoghq.com/datadogagentprofile",
		"agent.datadoghq.com/name",
		"operator.datadoghq.com/managed-by-store",
		"operator.datadoghq.com/managed-by-dda-controller",
		"datadoghq.com/foo",
	}
	for _, k := range reserved {
		assert.True(t, isReservedLabelKey(k), "expected %q to be reserved", k)
	}

	notReserved := []string{
		"team",
		"app.kubernetes.io/name",
		"cost-center",
		"my.company.com/env",
	}
	for _, k := range notReserved {
		assert.False(t, isReservedLabelKey(k), "expected %q to not be reserved", k)
	}
}

